package tools

// The bundled multi-actor-narration skill is installed into an agent's skill
// directory and read as instruction, so a name in it that the server does not
// have is the agent being told to make a call the server refuses. These tests
// hold the skill to the server in both directions (organization ADR-027,
// project ADR-0015):
//
//   - every tool the server registers is named by the skill, and every
//     identifier the skill puts in backticks exists in the server — taken from
//     the registered tools, their input schemas, the toolerr codes and the
//     json/toml tags of the server's own types, not from a hand-copied list;
//   - every retired name is held absent;
//   - the templates the skill hands the agent parse with the server's own
//     parsers.

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/nlink-jp/voice-studio-mcp/internal/script"
)

// skillDir is the skill at the repository root, where the organization's
// validator (tests/validate-skill.sh) and `make package-skill` find it.
const skillDir = "../../multi-actor-narration"

// readSkillFiles returns every file under the skill whose name ends with
// suffix, keyed by its path relative to the skill.
func readSkillFiles(t *testing.T, suffix string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(skillDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, suffix) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(skillDir, path)
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("reading the skill: %v", err)
	}
	return out
}

// readSkillCorpus concatenates every Markdown file of the skill, so a name
// counts wherever it is taught: the router, the shared pipeline or a format.
func readSkillCorpus(t *testing.T) string {
	t.Helper()
	files := readSkillFiles(t, ".md")
	if len(files) == 0 {
		t.Fatalf("the skill has no Markdown; is %s present?", skillDir)
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		b.WriteString(files[n])
		b.WriteByte('\n')
	}
	return b.String()
}

func TestSkillFrontmatter(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("the skill has no SKILL.md: %v", err)
	}
	s := string(b)
	if !strings.HasPrefix(s, "---\n") {
		t.Fatalf("SKILL.md must start with YAML frontmatter")
	}
	end := strings.Index(s[4:], "\n---")
	if end < 0 {
		t.Fatalf("SKILL.md frontmatter is not terminated")
	}
	front := s[:end+5]
	for _, field := range []string{"name: multi-actor-narration\n", "description: ", "argument-hint: "} {
		if !strings.Contains(front, field) {
			t.Errorf("frontmatter missing %q", field)
		}
	}
}

// A nested SKILL.md is detected as a second skill by the app's importer and
// by the validator, so format procedures are named FORMAT.md.
func TestExactlyOneSkillManifest(t *testing.T) {
	var manifests []string
	for rel := range readSkillFiles(t, "SKILL.md") {
		manifests = append(manifests, rel)
	}
	if len(manifests) != 1 || manifests[0] != "SKILL.md" {
		t.Errorf("want exactly one SKILL.md, at the skill root; got %v", manifests)
	}
}

// Every tool the model is shown, except get_usage (the manual the skill
// builds on), is part of the pipeline the skill drives.
func TestSkillNamesEveryRegisteredTool(t *testing.T) {
	s := readSkillCorpus(t)
	for _, tool := range registeredTools(t) {
		if tool.Name == "get_usage" {
			continue
		}
		if !strings.Contains(s, "`"+tool.Name+"`") {
			t.Errorf("the skill does not name the tool %q", tool.Name)
		}
	}
}

// notServerNames are backticked identifiers the skill uses that are not the
// server's: each is listed with what it is, so a new one is a decision, not
// an accident.
var notServerNames = map[string]string{
	"key_point":         "a field of the skill's own source-ingest cards (SOURCE-INGEST.md)",
	"listener_question": "a field of the skill's own source-ingest cards (SOURCE-INGEST.md)",
	"session_info":      "a tool of the calling client, used to read the chat session as source material",
	"read_transcript":   "a tool of the calling client, used to read the chat session as source material",
}

// snakeIdent is an identifier in backticks shaped like a tool, code, argument
// or field name: lower-case words joined by underscores.
var snakeIdent = regexp.MustCompile("`([a-z]+(?:_[a-z0-9]+)+)`")

// TestSkillNamesOnlyWhatTheServerHas is the direction a presence test cannot
// cover: a retired or misspelled name in the skill still passes every
// "does the skill mention X" check.
func TestSkillNamesOnlyWhatTheServerHas(t *testing.T) {
	known := serverVocabulary(t)
	seen := map[string]bool{}
	for _, m := range snakeIdent.FindAllStringSubmatch(readSkillCorpus(t), -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		if known[name] {
			continue
		}
		if _, ok := notServerNames[name]; ok {
			continue
		}
		t.Errorf("the skill names `%s`, which is not a tool, argument, error code or field of this server; "+
			"fix the skill, or list it in notServerNames with what it is", name)
	}
	for name := range notServerNames {
		if known[name] {
			t.Errorf("notServerNames lists %q, but the server has it; remove it from the list", name)
		}
		if !seen[name] {
			t.Errorf("notServerNames lists %q, which the skill no longer uses; remove it from the list", name)
		}
	}
}

// The codes an agent following the skill's pipeline must be able to act on.
// The list is checked against the server, so it cannot name a code that does
// not exist.
func TestSkillDispatchesThePipelineErrors(t *testing.T) {
	codes := toolerrCodes(t)
	s := readSkillCorpus(t)
	for _, code := range []string{
		"invalid_script", "casting_unresolved", "engine_unavailable",
		"engine_request_failed", "job_not_found", "master_incomplete",
		"ffmpeg_not_found", "path_not_allowed", "work_dir_required",
	} {
		if !codes[code] {
			t.Errorf("this test expects the code %q, which the server no longer has", code)
			continue
		}
		if !strings.Contains(s, "`"+code+"`") {
			t.Errorf("the skill does not tell the agent what to do on %q", code)
		}
	}
}

// v0.5.0 made work_dir required and the skill went on teaching
// `workspace_root` (described as optional, with ~/.voice-studio as the
// fallback) across eight files, because every test here checked for the
// presence of something and none for the absence of a retired thing.
func TestSkillNamesNoRetiredName(t *testing.T) {
	retired := append([]string{
		"~/.voice-studio",              // the server-owned default root ADR-0013 removed
		"/radio-drama",                 // the skill ADR-0011 replaced with this one
		"skills/multi-actor-narration", // the path ADR-0015 moved the skill from
		"multi-actor-narration.skill",  // the package name ADR-0015 retired
	}, retiredWorkDirNames...)
	for rel, body := range readSkillFiles(t, "") {
		for _, old := range retired {
			if strings.Contains(body, old) {
				t.Errorf("%s still teaches %q", rel, old)
			}
		}
	}
}

// Naming work_dir is not enough: an agent told it is optional will omit it.
func TestSkillTeachesWorkDirAsRequired(t *testing.T) {
	s := readSkillCorpus(t)
	if !strings.Contains(s, "work_dir") {
		t.Fatal("the skill never mentions work_dir, which every workspace tool requires")
	}
	if !strings.Contains(s, "必須") {
		t.Error("the skill does not say work_dir is required")
	}
}

// The templates are copied into a workspace and handed to synthesize_script
// as they are, so they must pass the server's own parsers — the JSONL parser
// rejects unknown fields and the casting parser rejects unknown keys — and
// every speaker and style a script uses must resolve in its casting table.
func TestSkillTemplatesParseWithTheServersParsers(t *testing.T) {
	scripts := readSkillFiles(t, "script.template.jsonl")
	castings := readSkillFiles(t, "casting.template.toml")
	if len(scripts) == 0 || len(castings) == 0 {
		t.Fatalf("no templates found (%d scripts, %d castings)", len(scripts), len(castings))
	}
	for rel, body := range castings {
		if _, err := script.ParseCasting([]byte(body), rel); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
	}
	for rel, body := range scripts {
		lines, errs := script.Parse(strings.NewReader(body))
		for _, e := range errs {
			t.Errorf("%s:%d: %s", rel, e.LineNo, e.Message)
		}
		castRel := filepath.Join(filepath.Dir(rel), "casting.template.toml")
		castBody, ok := castings[castRel]
		if !ok {
			t.Errorf("%s has no casting.template.toml beside it", rel)
			continue
		}
		cast, err := script.ParseCasting([]byte(castBody), castRel)
		if err != nil {
			continue // reported above
		}
		if err := cast.Validate(lines); err != nil {
			t.Errorf("%s does not resolve against %s: %v", rel, castRel, err)
		}
	}
}

// serverVocabulary is every name the server exposes: its tools, every
// property of their input schemas, its error codes, and the json/toml tags of
// its own types (the script and casting formats, and the results it returns).
func serverVocabulary(t *testing.T) map[string]bool {
	t.Helper()
	known := map[string]bool{}
	for _, tool := range registeredTools(t) {
		known[tool.Name] = true
		var schema any
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s: input schema: %v", tool.Name, err)
		}
		collectSchemaProperties(schema, known)
	}
	for code := range toolerrCodes(t) {
		known[code] = true
	}
	tag := regexp.MustCompile(`(?:json|toml):"([a-z0-9_]+)`)
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range tag.FindAllSubmatch(src, -1) {
			known[string(m[1])] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return known
}

func collectSchemaProperties(v any, into map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		if props, ok := x["properties"].(map[string]any); ok {
			for name := range props {
				into[name] = true
			}
		}
		for _, child := range x {
			collectSchemaProperties(child, into)
		}
	case []any:
		for _, child := range x {
			collectSchemaProperties(child, into)
		}
	}
}

// toolerrCodes reads the Code* constants of internal/toolerr: the codes the
// server can return, as declared, not as copied.
func toolerrCodes(t *testing.T) map[string]bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "../toolerr/toolerr.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range vs.Names {
			if !strings.HasPrefix(name.Name, "Code") || i >= len(vs.Values) {
				continue
			}
			if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if v, err := strconv.Unquote(lit.Value); err == nil {
					codes[v] = true
				}
			}
		}
		return true
	})
	if len(codes) < 10 {
		t.Fatalf("found only %d toolerr codes; has internal/toolerr changed shape?", len(codes))
	}
	return codes
}
