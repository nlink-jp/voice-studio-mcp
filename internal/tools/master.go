package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/nlink-jp/voice-studio-mcp/internal/master"
	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

func registerMaster(srv *mcpserver.Server, d *Deps) {
	srv.RegisterTool(mcpserver.Tool{
		Name: "master",
		Description: "Concatenate the synthesized line WAVs of a script (in script order, inserting each line's " +
			"pause_after_ms as silence), normalize loudness, and encode to mp3 or m4b (audiobook with scene " +
			"chapters). Also writes a credits file from the casting table and reports characters whose voice-model " +
			"license was never verified. Fails with master_incomplete if any line has not been synthesized yet.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["workspace_id", "script_path"],
  "properties": {
    "workspace_id": {"type": "string"},
    "workspace_root": {"type": "string", "description": "Absolute path to a workspace root you prepared and can read back. Pass your own session or working directory when you have one: results come back as paths, so a workspace you cannot open leaves you holding a path to nothing. Omitting it uses the server default (~/.voice-studio), which is only useful if that is readable to you."},
    "script_path": {"type": "string", "description": "Script JSONL path relative to the workspace root"},
    "casting_path": {"type": "string", "description": "Casting table path relative to the workspace root (default casting.toml)"},
    "format": {"type": "string", "enum": ["mp3", "m4b"], "description": "Output format (default mp3)"},
    "output_name": {"type": "string", "description": "Output basename without extension (default: script file name)"},
    "chapters": {"type": "boolean", "description": "m4b only: emit scene-boundary chapters (default true)"}
  },
  "additionalProperties": false
}`),
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		in := struct {
			WorkspaceID   string `json:"workspace_id"`
			WorkspaceRoot string `json:"workspace_root"`
			ScriptPath    string `json:"script_path"`
			CastingPath   string `json:"casting_path"`
			Format        string `json:"format"`
			OutputName    string `json:"output_name"`
			Chapters      *bool  `json:"chapters"`
		}{}
		if err := unmarshalStrict(args, &in); err != nil {
			return nil, err
		}
		if in.Format == "" {
			in.Format = "mp3"
		}
		if in.Format != "mp3" && in.Format != "m4b" {
			return nil, toolerr.Newf(toolerr.CodeInvalidArguments, "format must be mp3 or m4b, got %q", in.Format)
		}
		chapters := true
		if in.Chapters != nil {
			chapters = *in.Chapters
		}
		if in.OutputName != "" && (strings.ContainsAny(in.OutputName, "/\\") || strings.Contains(in.OutputName, "..")) {
			return nil, toolerr.Newf(toolerr.CodeInvalidArguments, "output_name must be a plain file name, got %q", in.OutputName)
		}

		ws, err := d.WS.EnsureIn(in.WorkspaceRoot, in.WorkspaceID)
		if err != nil {
			return nil, err
		}
		lines, casting, err := loadValidatedScript(ws, in.ScriptPath, in.CastingPath)
		if err != nil {
			return nil, err
		}

		stem := strings.TrimSuffix(filepath.Base(in.ScriptPath), filepath.Ext(in.ScriptPath))
		m := &master.Master{Runner: d.Runner, Cfg: d.Cfg.Master}
		return m.Build(ctx, ws, stem, lines, casting, master.Options{
			Format:     in.Format,
			OutputName: in.OutputName,
			Chapters:   chapters,
		})
	})
}
