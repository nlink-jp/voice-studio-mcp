package tools

import (
	"context"
	"encoding/json"

	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/voice-studio-mcp/internal/script"
	"github.com/nlink-jp/voice-studio-mcp/internal/synth"
	"github.com/nlink-jp/voice-studio-mcp/internal/workspace"
)

const defaultCastingPath = "casting.toml"

var lineSchema = `{
  "type": "object",
  "required": ["id", "speaker", "text"],
  "properties": {
    "id": {"type": "integer", "description": "Stable line id (>0). Output is wav/<id>.wav"},
    "scene": {"type": "integer", "description": "Chapter/scene number (default 1)"},
    "speaker": {"type": "string", "description": "Character name, resolved via the casting table"},
    "text": {"type": "string"},
    "style": {"type": "string", "description": "Named style from the casting entry (e.g. 悲しみ); empty = default style"},
    "intensity": {"type": "number", "description": "Emotional strength 0.0-2.0 (engine intonationScale)"},
    "speed": {"type": "number", "description": "Speech speed 0.5-2.0"},
    "volume": {"type": "number", "description": "Per-line volume 0.0-2.0 (engine volumeScale); for relative balance, since master re-levels overall loudness"},
    "pause_after_ms": {"type": "integer", "description": "Silence after this line, applied at mastering time"}
  },
  "additionalProperties": false
}`

func registerSynthesizeLine(srv *mcpserver.Server, d *Deps) {
	srv.RegisterTool(mcpserver.Tool{
		Name: "synthesize_line",
		Description: "Synthesize one script line to wav/<id>.wav in the workspace (synchronous; for retakes and " +
			"voice auditioning). Set style_id to bypass the casting table when auditioning voices. " +
			"Returns the wav path and duration; audio bytes are never returned.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["workspace_id", "line"],
  "properties": {
    "workspace_id": {"type": "string"},
    "line": ` + lineSchema + `,
    "casting_path": {"type": "string", "description": "Casting table path relative to the workspace root (default casting.toml)"},
    "style_id": {"type": "integer", "description": "Explicit global style id; overrides casting resolution"},
    "force": {"type": "boolean", "description": "Re-synthesize even on cache hit"}
  },
  "additionalProperties": false
}`),
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var in struct {
			WorkspaceID string      `json:"workspace_id"`
			Line        script.Line `json:"line"`
			CastingPath string      `json:"casting_path"`
			StyleID     *int        `json:"style_id"`
			Force       bool        `json:"force"`
		}
		if err := unmarshalStrict(args, &in); err != nil {
			return nil, err
		}
		ws, err := d.WS.Ensure(in.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if err := in.Line.Validate(); err != nil {
			return nil, err
		}
		res, err := resolveLine(ws, in.Line, in.CastingPath, in.StyleID)
		if err != nil {
			return nil, err
		}
		store, err := synth.OpenCacheStore(synth.CacheIndexPath(ws))
		if err != nil {
			return nil, err
		}
		return d.Synth.SynthesizeLine(ctx, ws, store, in.Line, res, in.Force)
	})
}

// resolveLine maps a line to a style id: an explicit style_id wins (voice
// auditioning), otherwise the casting table decides.
func resolveLine(ws *workspace.Workspace, line script.Line, castingPath string, styleID *int) (synth.Resolved, error) {
	if styleID != nil {
		// No speaker UUID in this path; the global style id alone keys the
		// cache correctly because style ids are unique across models.
		return synth.Resolved{StyleID: *styleID}, nil
	}
	if castingPath == "" {
		castingPath = defaultCastingPath
	}
	path, err := ws.ResolveInside(castingPath)
	if err != nil {
		return synth.Resolved{}, err
	}
	casting, err := script.LoadCasting(path)
	if err != nil {
		return synth.Resolved{}, err
	}
	id, entry, err := casting.Resolve(line.Speaker, line.Style)
	if err != nil {
		return synth.Resolved{}, err
	}
	return synth.Resolved{StyleID: id, SpeakerUUID: entry.SpeakerUUID}, nil
}
