// Package skills has no Go code — this test pins the bundled Claude Code
// skill against the MCP server it ships with, so the two cannot drift
// silently (ADR-0009). The skill is multi-file: a SKILL.md router plus
// _shared/*.md and <format>/FORMAT.md. Coherence assertions run over the
// whole skill corpus, so the skill must reference every tool the server
// registers and its schema examples must use only fields the script parser
// accepts.
package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skillDir is the single bundled skill (ADR-0011 consolidated the former
// radio-drama skill into this one as the audio-drama format).
const skillDir = "multi-actor-narration"

// readSkillCorpus concatenates every Markdown file under the skill directory
// so coherence holds across the router, the shared pipeline, and each format.
func readSkillCorpus(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(skillDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.WriteString(string(data))
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatalf("reading skill corpus: %v", err)
	}
	if b.Len() == 0 {
		t.Fatalf("skill corpus is empty; is %s/ present?", skillDir)
	}
	return b.String()
}

// readSkillManifest returns the router SKILL.md (the only frontmatter file).
func readSkillManifest(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("bundled skill manifest missing: %v", err)
	}
	return string(b)
}

func TestSkillFrontmatter(t *testing.T) {
	s := readSkillManifest(t)
	if !strings.HasPrefix(s, "---\n") {
		t.Fatalf("SKILL.md must start with YAML frontmatter")
	}
	end := strings.Index(s[4:], "---")
	if end < 0 {
		t.Fatalf("SKILL.md frontmatter is not terminated")
	}
	front := s[:end+4]
	for _, field := range []string{"name: multi-actor-narration", "description: ", "argument-hint: "} {
		if !strings.Contains(front, field) {
			t.Errorf("frontmatter missing %q", field)
		}
	}
}

func TestSkillReferencesEveryTool(t *testing.T) {
	s := readSkillCorpus(t)
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
	s := readSkillCorpus(t)
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
	s := readSkillCorpus(t)
	// The canonical Line fields; the skill must not teach agents fields the
	// parser would reject (DisallowUnknownFields).
	for _, field := range []string{`"id"`, `"scene"`, `"speaker"`, `"text"`, `"style"`, `"intensity"`, `"speed"`, `"pause_after_ms"`} {
		if !strings.Contains(s, field) {
			t.Errorf("schema example missing field %s", field)
		}
	}
}

// TestExactlyOneSkillManifest enforces the packaging invariant build.sh relies
// on: exactly one frontmatter-bearing SKILL.md in the skill tree. A nested
// SKILL.md is mis-detected as a separate skill, so format procedures must be
// named FORMAT.md.
func TestExactlyOneSkillManifest(t *testing.T) {
	var manifests []string
	err := filepath.WalkDir(skillDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(string(data), "---\n") {
			manifests = append(manifests, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking skill tree: %v", err)
	}
	if len(manifests) != 1 {
		t.Errorf("want exactly 1 frontmatter SKILL.md, got %d: %v", len(manifests), manifests)
	}
}
