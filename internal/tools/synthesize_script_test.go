package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testScript = `# episode 1
{"id":1,"scene":1,"speaker":"narrator","text":"夜のとばりが街を包んでいた。","pause_after_ms":800}
{"id":2,"scene":1,"speaker":"美咲","text":"……本当に、行くの?","style":"悲しみ","intensity":1.4}
{"id":3,"scene":2,"speaker":"narrator","text":"翌朝。"}
`

func (h *testHarness) seedScript(wsID, name, body string) {
	h.t.Helper()
	ws, err := h.deps.WS.Ensure(wsID)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(ws.Path("script", name), []byte(body), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

type scriptOut struct {
	JobID      string `json:"job_id"`
	TotalLines int    `json:"total_lines"`
	Cached     int    `json:"cached"`
	Queued     int    `json:"queued"`
	OutputDir  string `json:"output_dir"`
}

type jobOut struct {
	State    string `json:"state"`
	Total    int    `json:"total"`
	Done     int    `json:"done"`
	Cached   int    `json:"cached"`
	Failed   int    `json:"failed"`
	Failures []struct {
		LineID int    `json:"line_id"`
		Code   string `json:"code"`
	} `json:"failures"`
}

// runScriptJob invokes synthesize_script and polls check_job to completion.
func (h *testHarness) runScriptJob(args map[string]any) (scriptOut, jobOut) {
	h.t.Helper()
	body, isErr := h.callTool("synthesize_script", args)
	if isErr {
		h.t.Fatalf("synthesize_script error: %s", body)
	}
	var so scriptOut
	if err := json.Unmarshal(body, &so); err != nil {
		h.t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		jb, isErr := h.callTool("check_job", map[string]any{"job_id": so.JobID})
		if isErr {
			h.t.Fatalf("check_job error: %s", jb)
		}
		var jo jobOut
		if err := json.Unmarshal(jb, &jo); err != nil {
			h.t.Fatal(err)
		}
		if jo.State != "running" {
			return so, jo
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("job stuck: %+v", jo)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSynthesizeScriptEndToEnd(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")
	h.seedScript(id, "ep1.jsonl", testScript)

	so, jo := h.runScriptJob(map[string]any{
		"workspace_id": id,
		"script_path":  "script/ep1.jsonl",
	})
	if so.TotalLines != 3 || so.Cached != 0 || so.Queued != 3 {
		t.Errorf("script out: %+v", so)
	}
	if jo.State != "done" || jo.Done != 3 || jo.Failed != 0 {
		t.Errorf("job out: %+v", jo)
	}
	for _, n := range []string{"1.wav", "2.wav", "3.wav"} {
		if _, err := os.Stat(filepath.Join(so.OutputDir, n)); err != nil {
			t.Errorf("missing %s: %v", n, err)
		}
	}

	// Second run is fully cached and never hits the engine again.
	synthCalls := h.mock.CallCount("/synthesis")
	so2, jo2 := h.runScriptJob(map[string]any{
		"workspace_id": id,
		"script_path":  "script/ep1.jsonl",
	})
	if so2.Cached != 3 || so2.Queued != 0 {
		t.Errorf("second run precount: %+v", so2)
	}
	if jo2.Done != 3 || jo2.Cached != 3 {
		t.Errorf("second run job: %+v", jo2)
	}
	if h.mock.CallCount("/synthesis") != synthCalls {
		t.Errorf("cached run must not synthesize")
	}

	// force=true re-synthesizes everything.
	so3, _ := h.runScriptJob(map[string]any{
		"workspace_id": id,
		"script_path":  "script/ep1.jsonl",
		"force":        true,
	})
	if so3.Queued != 3 {
		t.Errorf("force precount: %+v", so3)
	}
	if h.mock.CallCount("/synthesis") != synthCalls+3 {
		t.Errorf("force run should synthesize 3 lines")
	}
}

func TestSynthesizeScriptReportsLineFailures(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")
	h.seedScript(id, "ep1.jsonl", testScript)
	h.mock.FailNext("/audio_query", 500)

	_, jo := h.runScriptJob(map[string]any{
		"workspace_id": id,
		"script_path":  "script/ep1.jsonl",
	})
	if jo.State != "done" || jo.Failed != 1 || jo.Done != 2 {
		t.Errorf("job: %+v", jo)
	}
	if len(jo.Failures) != 1 || jo.Failures[0].Code != "engine_request_failed" {
		t.Errorf("failures: %+v", jo.Failures)
	}
}

func TestSynthesizeScriptInvalidScript(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")
	h.seedScript(id, "bad.jsonl", `{"id":1,"speaker":"narrator"}`+"\n"+`{"id":1,"speaker":"x","text":"dup"}`)

	body, isErr := h.callTool("synthesize_script", map[string]any{
		"workspace_id": id,
		"script_path":  "script/bad.jsonl",
	})
	if !isErr || errCode(t, body) != "invalid_script" {
		t.Fatalf("expected invalid_script, got isErr=%v %s", isErr, body)
	}
	if !strings.Contains(string(body), "error_count") {
		t.Errorf("details missing error_count: %s", body)
	}
}

func TestSynthesizeScriptCastingUnresolved(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")
	h.seedScript(id, "ep1.jsonl", `{"id":1,"speaker":"未登録","text":"x"}`)

	body, isErr := h.callTool("synthesize_script", map[string]any{
		"workspace_id": id,
		"script_path":  "script/ep1.jsonl",
	})
	if !isErr || errCode(t, body) != "casting_unresolved" {
		t.Fatalf("expected casting_unresolved, got isErr=%v %s", isErr, body)
	}
	if !strings.Contains(string(body), "未登録") {
		t.Errorf("details missing speaker name: %s", body)
	}
}

func TestSynthesizeScriptMissingFile(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")
	body, isErr := h.callTool("synthesize_script", map[string]any{
		"workspace_id": id,
		"script_path":  "script/nope.jsonl",
	})
	if !isErr || errCode(t, body) != "invalid_script" {
		t.Fatalf("expected invalid_script, got isErr=%v %s", isErr, body)
	}
}

func TestCheckJobUnknownID(t *testing.T) {
	h := newHarness(t)
	body, isErr := h.callTool("check_job", map[string]any{"job_id": "job_gone"})
	if !isErr || errCode(t, body) != "job_not_found" {
		t.Fatalf("expected job_not_found, got isErr=%v %s", isErr, body)
	}
}
