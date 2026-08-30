package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/nlink-jp/voice-studio-mcp/internal/job"
	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/voice-studio-mcp/internal/script"
	"github.com/nlink-jp/voice-studio-mcp/internal/synth"
	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
	"github.com/nlink-jp/voice-studio-mcp/internal/workspace"
)

func registerSynthesizeScript(srv *mcpserver.Server, d *Deps) {
	srv.RegisterTool(mcpserver.Tool{
		Name: "synthesize_script",
		Description: "Batch-synthesize a script JSONL file (one utterance per line) into wav/<id>.wav files. " +
			"Validates the whole script and casting table first, then runs asynchronously — returns a job_id " +
			"immediately; poll with check_job. Unchanged lines are skipped via a content-hash cache, so " +
			"re-running after editing a few lines only synthesizes those lines.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["workspace_id", "script_path"],
  "properties": {
    "workspace_id": {"type": "string"},
    "workspace_root": {"type": "string", "description": "Absolute path to a workspace root you prepared and can read back. Pass your own session or working directory when you have one: results come back as paths, so a workspace you cannot open leaves you holding a path to nothing. Omitting it uses the server default (~/.voice-studio), which is only useful if that is readable to you."},
    "script_path": {"type": "string", "description": "Script JSONL path relative to the workspace root (e.g. script/episode1.jsonl)"},
    "casting_path": {"type": "string", "description": "Casting table path relative to the workspace root (default casting.toml)"},
    "force": {"type": "boolean", "description": "Ignore the cache and re-synthesize every line"}
  },
  "additionalProperties": false
}`),
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var in struct {
			WorkspaceID   string `json:"workspace_id"`
			WorkspaceRoot string `json:"workspace_root"`
			ScriptPath    string `json:"script_path"`
			CastingPath   string `json:"casting_path"`
			Force         bool   `json:"force"`
		}
		if err := unmarshalStrict(args, &in); err != nil {
			return nil, err
		}
		ws, err := d.WS.EnsureIn(in.WorkspaceRoot, in.WorkspaceID)
		if err != nil {
			return nil, err
		}
		lines, casting, err := loadValidatedScript(ws, in.ScriptPath, in.CastingPath)
		if err != nil {
			return nil, err
		}

		store, err := synth.OpenCacheStore(ws)
		if err != nil {
			return nil, err
		}

		// Pre-count cache hits for the immediate response (pure lookups, no
		// engine calls). The job re-checks per line, so the numbers stay
		// consistent even if state changes in between.
		cached := 0
		items := make([]job.Item, 0, len(lines))
		for _, ln := range lines {
			id, entry, rerr := casting.Resolve(ln.Speaker, ln.Style)
			if rerr != nil {
				return nil, rerr // unreachable after Validate, kept as a guard
			}
			res := synth.Resolved{StyleID: id, SpeakerUUID: entry.SpeakerUUID}
			if !in.Force {
				if _, hit := store.Lookup(ln.ID, d.Synth.Key(ln, res)); hit {
					cached++
				}
			}
			ln := ln
			items = append(items, job.Item{LineID: ln.ID, Run: func(ctx context.Context) (bool, error) {
				r, err := d.Synth.SynthesizeLine(ctx, ws, store, ln, res, in.Force)
				if err != nil {
					return false, err
				}
				return r.Cached, nil
			}})
		}

		outputDir := ws.Path(workspace.DirWav)
		jobID := d.Jobs.Submit(d.JobCtx, outputDir, items)
		return map[string]any{
			"job_id":       jobID,
			"workspace_id": in.WorkspaceID,
			"total_lines":  len(lines),
			"cached":       cached,
			"queued":       len(lines) - cached,
			"output_dir":   outputDir,
			"hint":         "poll check_job with this job_id until state is no longer \"running\"",
		}, nil
	})
}

// loadValidatedScript parses the script file and fully validates it against
// the casting table. Every problem is reported at once so the agent can fix
// the inputs in a single pass.
func loadValidatedScript(ws *workspace.Workspace, scriptPath, castingPath string) ([]script.Line, *script.Casting, error) {
	sp, err := ws.ResolveInside(scriptPath)
	if err != nil {
		return nil, nil, err
	}
	raw, err := ws.ReadFile(sp)
	if err != nil {
		if errors.Is(err, toolerr.New(toolerr.CodePathNotAllowed, "")) {
			return nil, nil, err
		}
		return nil, nil, toolerr.Newf(toolerr.CodeInvalidScript, "read script: %v", err).
			WithDetails(map[string]any{"script_path": scriptPath})
	}
	lines, lineErrs := script.Parse(bytes.NewReader(raw))
	if len(lineErrs) > 0 {
		return nil, nil, script.InvalidScriptError(lineErrs)
	}
	if len(lines) == 0 {
		return nil, nil, toolerr.New(toolerr.CodeInvalidScript, "script contains no lines")
	}

	casting, err := loadCasting(ws, castingPath)
	if err != nil {
		return nil, nil, err
	}
	if err := casting.Validate(lines); err != nil {
		return nil, nil, err
	}
	return lines, casting, nil
}

// loadCasting reads and parses the casting table through workspace containment.
func loadCasting(ws *workspace.Workspace, castingPath string) (*script.Casting, error) {
	if castingPath == "" {
		castingPath = defaultCastingPath
	}
	cp, err := ws.ResolveInside(castingPath)
	if err != nil {
		return nil, err
	}
	raw, err := ws.ReadFile(cp)
	if err != nil {
		if errors.Is(err, toolerr.New(toolerr.CodePathNotAllowed, "")) {
			return nil, err
		}
		return nil, toolerr.Newf(toolerr.CodeCastingUnresolved,
			"read casting table %s: %v", castingPath, err)
	}
	return script.ParseCasting(raw, castingPath)
}
