// Package skills has no Go code — this test pins the bundled Claude Code
// skill against the MCP server it ships with, so the two cannot drift
// silently: the skill must reference every tool the server registers, and
// its schema examples must use only fields the script parser accepts.
package skills

import (
	"os"
	"strings"
	"testing"
)

func readSkill(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("radio-drama/SKILL.md")
	if err != nil {
		t.Fatalf("bundled skill missing: %v", err)
	}
	return string(b)
}

func TestSkillFrontmatter(t *testing.T) {
	s := readSkill(t)
	if !strings.HasPrefix(s, "---\n") {
		t.Fatalf("SKILL.md must start with YAML frontmatter")
	}
	for _, field := range []string{"name: radio-drama", "description: ", "argument-hint: "} {
		if !strings.Contains(s[:strings.Index(s[4:], "---")+4], field) {
			t.Errorf("frontmatter missing %q", field)
		}
	}
}

func TestSkillReferencesEveryTool(t *testing.T) {
	s := readSkill(t)
	for _, tool := range []string{
		"list_speakers", "register_dictionary", "synthesize_script",
		"synthesize_line", "check_job", "master",
	} {
		if !strings.Contains(s, "`"+tool+"`") {
			t.Errorf("skill does not reference tool %q", tool)
		}
	}
}

func TestSkillReferencesEveryErrorCode(t *testing.T) {
	s := readSkill(t)
	for _, code := range []string{
		"invalid_script", "casting_unresolved", "engine_unavailable",
		"engine_request_failed", "job_not_found", "master_incomplete",
		"ffmpeg_not_found", "path_not_allowed",
	} {
		if !strings.Contains(s, code) {
			t.Errorf("error dispatch missing %q", code)
		}
	}
}

func TestSkillSchemaExampleFieldsAreValid(t *testing.T) {
	s := readSkill(t)
	// The canonical Line fields; the skill must not teach agents fields the
	// parser would reject (DisallowUnknownFields).
	for _, field := range []string{`"id"`, `"scene"`, `"speaker"`, `"text"`, `"style"`, `"intensity"`, `"speed"`, `"pause_after_ms"`} {
		if !strings.Contains(s, field) {
			t.Errorf("schema example missing field %s", field)
		}
	}
}
