package tools

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
	"github.com/nlink-jp/voice-studio-mcp/internal/workspace"
)

// dictRecord is the per-workspace record of registered words
// (dict/words.json). The engine's user dictionary is global; this record
// keeps the workspace reproducible and makes re-registration idempotent.
type dictRecord struct {
	Pronunciation string `json:"pronunciation"`
	AccentType    int    `json:"accent_type"`
	WordType      string `json:"word_type,omitempty"`
	Priority      *int   `json:"priority,omitempty"`
	EngineUUID    string `json:"engine_uuid"`
}

type dictWordIn struct {
	Surface       string `json:"surface"`
	Pronunciation string `json:"pronunciation"`
	AccentType    *int   `json:"accent_type"`
	WordType      string `json:"word_type,omitempty"`
	Priority      *int   `json:"priority,omitempty"`
}

func registerRegisterDictionary(srv *mcpserver.Server, d *Deps) {
	srv.RegisterTool(mcpserver.Tool{
		Name: "register_dictionary",
		Description: "Register work-specific pronunciations (proper nouns, coined words) in the engine's user " +
			"dictionary so they are read correctly during synthesis. Words already registered with identical " +
			"values are skipped. Run this BEFORE synthesize_script. pronunciation must be katakana; accent_type " +
			"is the mora index where the pitch falls (0 = flat).",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["workspace_id", "words"],
  "properties": {
    "workspace_id": {"type": "string"},
    "workspace_root": {"type": "string", "description": "Absolute path to a workspace root you prepared and can read back. Pass your own session or working directory when you have one: results come back as paths, so a workspace you cannot open leaves you holding a path to nothing. Omitting it uses the server default (~/.voice-studio), which is only useful if that is readable to you."},
    "words": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "required": ["surface", "pronunciation", "accent_type"],
        "properties": {
          "surface": {"type": "string", "description": "The written form as it appears in the script"},
          "pronunciation": {"type": "string", "description": "Reading in katakana"},
          "accent_type": {"type": "integer", "minimum": 0},
          "word_type": {"type": "string", "enum": ["PROPER_NOUN", "COMMON_NOUN", "VERB", "ADJECTIVE", "SUFFIX"]},
          "priority": {"type": "integer", "minimum": 0, "maximum": 10}
        },
        "additionalProperties": false
      }
    }
  },
  "additionalProperties": false
}`),
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var in struct {
			WorkspaceID   string       `json:"workspace_id"`
			WorkspaceRoot string       `json:"workspace_root"`
			Words         []dictWordIn `json:"words"`
		}
		if err := unmarshalStrict(args, &in); err != nil {
			return nil, err
		}
		if len(in.Words) == 0 {
			return nil, toolerr.New(toolerr.CodeMissingArgument, "words must not be empty")
		}
		ws, err := d.WS.EnsureIn(in.WorkspaceRoot, in.WorkspaceID)
		if err != nil {
			return nil, err
		}

		recordRel := filepath.Join(workspace.DirDict, "words.json")
		record, err := loadDictRecord(ws, recordRel)
		if err != nil {
			return nil, err
		}

		registered, skipped := 0, 0
		var failed []map[string]string
		for _, w := range in.Words {
			if w.Surface == "" || w.Pronunciation == "" || w.AccentType == nil {
				failed = append(failed, map[string]string{
					"surface": w.Surface, "error": "surface, pronunciation, and accent_type are required",
				})
				continue
			}
			if prev, ok := record[w.Surface]; ok && sameWord(prev, w) {
				skipped++
				continue
			}
			uuid, err := d.Client.AddUserDictWord(ctx, engine.UserDictWord{
				Surface:       w.Surface,
				Pronunciation: w.Pronunciation,
				AccentType:    *w.AccentType,
				WordType:      w.WordType,
				Priority:      w.Priority,
			})
			if err != nil {
				failed = append(failed, map[string]string{"surface": w.Surface, "error": err.Error()})
				continue
			}
			record[w.Surface] = dictRecord{
				Pronunciation: w.Pronunciation,
				AccentType:    *w.AccentType,
				WordType:      w.WordType,
				Priority:      w.Priority,
				EngineUUID:    uuid,
			}
			registered++
		}

		if err := saveDictRecord(ws, recordRel, record); err != nil {
			return nil, err
		}
		out := map[string]any{
			"workspace_id":    in.WorkspaceID,
			"registered":      registered,
			"skipped":         skipped,
			"dictionary_path": ws.Path(recordRel),
		}
		if len(failed) > 0 {
			out["failed"] = failed
		}
		return out, nil
	})
}

func sameWord(prev dictRecord, w dictWordIn) bool {
	if prev.Pronunciation != w.Pronunciation || prev.AccentType != *w.AccentType || prev.WordType != w.WordType {
		return false
	}
	switch {
	case prev.Priority == nil && w.Priority == nil:
		return true
	case prev.Priority != nil && w.Priority != nil:
		return *prev.Priority == *w.Priority
	default:
		return false
	}
}

func loadDictRecord(ws *workspace.Workspace, rel string) (map[string]dictRecord, error) {
	record := map[string]dictRecord{}
	b, err := ws.ReadFile(rel)
	if err != nil {
		// Missing or unreadable record only costs duplicate registrations;
		// start fresh (writes go through containment anyway).
		return record, nil
	}
	if err := json.Unmarshal(b, &record); err != nil {
		return map[string]dictRecord{}, nil
	}
	return record, nil
}

func saveDictRecord(ws *workspace.Workspace, rel string, record map[string]dictRecord) error {
	b, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return toolerr.Newf(toolerr.CodeWorkspaceFailed, "encode dictionary record: %v", err)
	}
	return ws.WriteFileAtomic(rel, b)
}
