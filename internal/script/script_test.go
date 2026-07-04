package script_test

import (
	"strings"
	"testing"

	"github.com/nlink-jp/voice-studio-mcp/internal/script"
)

func TestParseValidScript(t *testing.T) {
	in := strings.Join([]string{
		`# ラジオドラマ 第1話`,
		``,
		`{"id":1,"scene":1,"speaker":"narrator","text":"夜のとばりが街を包んでいた。","pause_after_ms":800}`,
		`{"id":2,"speaker":"美咲","text":"……本当に、行くの?","style":"悲しみ","intensity":1.4,"speed":0.9}`,
		`{"id":3,"scene":2,"speaker":"narrator","text":"翌朝。"}`,
	}, "\n")

	lines, errs := script.Parse(strings.NewReader(in))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(lines) != 3 {
		t.Fatalf("line count: %d", len(lines))
	}
	if lines[1].Scene != 1 {
		t.Errorf("scene default: %d", lines[1].Scene)
	}
	if lines[1].Style != "悲しみ" || *lines[1].Intensity != 1.4 || *lines[1].Speed != 0.9 {
		t.Errorf("line 2: %+v", lines[1])
	}
	if *lines[0].PauseAfterMS != 800 {
		t.Errorf("pause: %+v", lines[0])
	}
}

func TestParseCollectsAllErrors(t *testing.T) {
	in := strings.Join([]string{
		`{"id":1,"speaker":"a","text":"ok"}`,
		`{"id":0,"speaker":"a","text":"bad id"}`,
		`{"id":2,"text":"missing speaker"}`,
		`{"id":3,"speaker":"a","text":""}`,
		`{"id":1,"speaker":"a","text":"duplicate id"}`,
		`{"id":4,"speaker":"a","text":"x","intensity":3.0}`,
		`{"id":5,"speaker":"a","text":"x","speed":0.1}`,
		`{"id":6,"speaker":"a","text":"x","pause_after_ms":-1}`,
		`not json at all`,
		`{"id":7,"speaker":"a","text":"x","unknown_field":true}`,
		`{"id":8,"speaker":"a","text":"still parsed after errors"}`,
	}, "\n")

	lines, errs := script.Parse(strings.NewReader(in))
	if len(lines) != 2 {
		t.Errorf("valid lines: %d (%+v)", len(lines), lines)
	}
	if len(errs) != 9 {
		t.Fatalf("error count: %d\n%+v", len(errs), errs)
	}
	// Line numbers are physical positions.
	if errs[0].LineNo != 2 || errs[0].LineID != 0 {
		t.Errorf("first error: %+v", errs[0])
	}
	// Duplicate id error mentions the original line.
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "duplicate id 1") && strings.Contains(e.Message, "line 1") {
			found = true
		}
	}
	if !found {
		t.Errorf("duplicate-id error missing: %+v", errs)
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	_, errs := script.Parse(strings.NewReader(`{"id":1,"speaker":"a","text":"x","pitch":2}`))
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "invalid JSON") {
		t.Errorf("errs: %+v", errs)
	}
}

func TestInvalidScriptErrorTruncates(t *testing.T) {
	var errs []script.LineError
	for i := 0; i < 30; i++ {
		errs = append(errs, script.LineError{LineNo: i + 1, Message: "bad"})
	}
	te := script.InvalidScriptError(errs)
	if te.Code != "invalid_script" {
		t.Errorf("code: %s", te.Code)
	}
	if te.Details["error_count"] != 30 {
		t.Errorf("error_count: %v", te.Details["error_count"])
	}
	if te.Details["errors_truncated"] != true {
		t.Errorf("errors_truncated: %v", te.Details["errors_truncated"])
	}
	reported := te.Details["errors"].([]script.LineError)
	if len(reported) != script.MaxReportedErrors {
		t.Errorf("reported: %d", len(reported))
	}
}
