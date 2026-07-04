// Package script defines the canonical script (台本) JSONL format and the
// casting table that maps characters to engine voices.
//
// The JSONL schema defined here is the contract consumed by agent-side
// skills: one line = one utterance, engine-independent.
package script

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

// Line is one utterance of a script.
type Line struct {
	// ID is the stable line identifier (> 0, unique within a file). Output
	// WAVs are named <ID>.wav, so retakes overwrite deterministically.
	ID int `json:"id"`
	// Scene groups lines into chapters (m4b chapter boundaries). Default 1.
	Scene int `json:"scene,omitempty"`
	// Speaker is the character name, resolved via the casting table.
	Speaker string `json:"speaker"`
	// Text is the utterance text.
	Text string `json:"text"`
	// Style optionally selects a named style from the casting entry
	// (e.g. "悲しみ"); empty uses the character's default style.
	Style string `json:"style,omitempty"`
	// Intensity maps to the engine's intonationScale (0.0–2.0).
	Intensity *float64 `json:"intensity,omitempty"`
	// Speed maps to the engine's speedScale (0.5–2.0).
	Speed *float64 `json:"speed,omitempty"`
	// PauseAfterMS is silence inserted after this line at mastering time.
	// It does not affect synthesis (and therefore not the synthesis cache).
	PauseAfterMS *int `json:"pause_after_ms,omitempty"`
}

// LineError describes one invalid script line.
type LineError struct {
	LineNo  int    `json:"line_no"` // 1-based physical line number in the file
	LineID  int    `json:"line_id,omitempty"`
	Message string `json:"message"`
}

// MaxReportedErrors bounds how many line errors are attached to a tool error.
const MaxReportedErrors = 20

// Parse reads a script JSONL stream. Blank lines and lines starting with '#'
// are skipped. All lines are scanned; errors are collected, not fail-fast,
// so the agent can fix a script in one pass.
func Parse(r io.Reader) ([]Line, []LineError) {
	var (
		lines  []Line
		errs   []LineError
		seen   = map[int]int{} // id -> first line no
		lineNo int
	)
	sc := newLongScanner(r)
	for sc.Scan() {
		lineNo++
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 || raw[0] == '#' {
			continue
		}
		var ln Line
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&ln); err != nil {
			errs = append(errs, LineError{LineNo: lineNo, Message: "invalid JSON: " + err.Error()})
			continue
		}
		if ln.Scene == 0 {
			ln.Scene = 1
		}
		if msg := validateLine(ln); msg != "" {
			errs = append(errs, LineError{LineNo: lineNo, LineID: ln.ID, Message: msg})
			continue
		}
		if first, dup := seen[ln.ID]; dup {
			errs = append(errs, LineError{LineNo: lineNo, LineID: ln.ID,
				Message: fmt.Sprintf("duplicate id %d (first used at line %d)", ln.ID, first)})
			continue
		}
		seen[ln.ID] = lineNo
		lines = append(lines, ln)
	}
	if err := sc.Err(); err != nil {
		errs = append(errs, LineError{LineNo: lineNo + 1, Message: "read: " + err.Error()})
	}
	return lines, errs
}

// ParseFile reads a script JSONL file.
func ParseFile(path string) ([]Line, []LineError, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	lines, errs := Parse(f)
	return lines, errs, nil
}

func validateLine(ln Line) string {
	switch {
	case ln.ID <= 0:
		return "id is required and must be > 0"
	case strings.TrimSpace(ln.Speaker) == "":
		return "speaker is required"
	case strings.TrimSpace(ln.Text) == "":
		return "text is required"
	case ln.Scene < 1:
		return "scene must be >= 1"
	case ln.Intensity != nil && (*ln.Intensity < 0.0 || *ln.Intensity > 2.0):
		return "intensity must be within 0.0–2.0"
	case ln.Speed != nil && (*ln.Speed < 0.5 || *ln.Speed > 2.0):
		return "speed must be within 0.5–2.0"
	case ln.PauseAfterMS != nil && *ln.PauseAfterMS < 0:
		return "pause_after_ms must be >= 0"
	}
	return ""
}

// InvalidScriptError converts collected line errors into a structured tool
// error (first MaxReportedErrors entries attached).
func InvalidScriptError(errs []LineError) *toolerr.Error {
	reported := errs
	truncated := false
	if len(reported) > MaxReportedErrors {
		reported = reported[:MaxReportedErrors]
		truncated = true
	}
	return toolerr.Newf(toolerr.CodeInvalidScript,
		"script has %d invalid line(s); fix them and retry", len(errs)).
		WithDetails(map[string]any{
			"errors":           reported,
			"error_count":      len(errs),
			"errors_truncated": truncated,
		})
}

// newLongScanner returns a line scanner sized for long utterances (1MB/line).
func newLongScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	return sc
}
