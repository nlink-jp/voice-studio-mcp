package tools

import (
	"context"
	"encoding/json"

	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
)

func registerCheckJob(srv *mcpserver.Server, d *Deps) {
	srv.RegisterTool(mcpserver.Tool{
		Name: "check_job",
		Description: "Check the progress of a synthesize_script job: state (running/done/failed), line counts, " +
			"and per-line failures. Jobs do not survive a server restart — if the job_id is unknown, re-run " +
			"synthesize_script (the cache makes that cheap).",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["job_id"],
  "properties": {
    "job_id": {"type": "string"}
  },
  "additionalProperties": false
}`),
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var in struct {
			JobID string `json:"job_id"`
		}
		if err := unmarshalStrict(args, &in); err != nil {
			return nil, err
		}
		return d.Jobs.Get(in.JobID)
	})
}
