package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nlink-jp/voice-studio-mcp/internal/workdir"
	"github.com/nlink-jp/voice-studio-mcp/internal/workspace"
)

// Whether a file exists is never the difference between two answers. The
// script and casting table are workspace-relative and read through an os.Root,
// but a place the floor refuses can lie inside a workspace: a .env, this
// server's own directory, or the file a link in ~/.ssh leads to when the
// workspace is in that sync folder. Read as a script, such a file would come
// back in a parse error, and a link planted at wav/<id>.wav reached one
// through master. Each case names one path twice — once while a file is there
// and once after it is removed — and the whole answer must be the same both
// times: a refusal that returns none of the file's contents. (The workspace
// judges every read, internal/workspace TestEveryReadIsJudgedBeforeItLooks.)
//
// The layer observed is the tool call, the answer a caller receives. The home
// directory is a temporary one: nothing is created, read or written in a real
// credential directory (pathguard still lists the account's own, for the links
// inside them).
func TestExistenceIsNotRevealed(t *testing.T) {
	base := realDir(t, t.TempDir())
	home := filepath.Join(base, "home")
	t.Setenv("HOME", home)
	work := filepath.Join(base, "sync")
	ws := filepath.Join(work, "ws")
	server := filepath.Join(ws, "srv") // this server's own directory, inside the workspace
	for _, d := range []string{filepath.Join(home, ".aws"), filepath.Join(home, ".ssh"), ws, server} {
		mkdirAll(t, d)
	}
	symlink(t, filepath.Join(ws, "ssh_config.jsonl"), filepath.Join(home, ".ssh", "config"))
	symlink(t, filepath.Join(home, ".aws", "planted.jsonl"), filepath.Join(ws, "lnk_file.jsonl"))
	symlink(t, filepath.Join(home, ".aws"), filepath.Join(ws, "lnk_dir"))
	symlink(t, filepath.Join("sub2", ".env"), filepath.Join(ws, "to_env.jsonl"))

	h := newHarness(t)
	h.deps.WorkDir = workdir.NewResolver(server)
	h.deps.WS = workspace.NewManager(h.deps.WorkDir.CheckBeneath, h.deps.WorkDir.LocalPath)
	line := map[string]any{"id": 1, "speaker": "narrator", "text": "テスト"}
	answer := func(tool, key, rel string) string {
		args := map[string]any{"work_dir": work, "workspace_id": "ws", key: rel}
		switch tool {
		case "synthesize_line":
			args["line"] = line
		case "synthesize_script", "master":
			if key == "casting_path" {
				args["script_path"] = "ok.jsonl"
			}
		}
		body, isErr := h.callTool(tool, args)
		if !isErr {
			return "accepted: " + string(body)
		}
		return string(body)
	}
	// A script that parses, for the cases that name the casting table.
	writeFileAt(t, filepath.Join(ws, "ok.jsonl"), `{"id":1,"speaker":"narrator","text":"テスト"}`+"\n")

	for _, c := range []struct{ name, rel, leaf string }{
		{"a .env file", filepath.Join("sub", ".env"), filepath.Join(ws, "sub", ".env")},
		{"this server's own directory", filepath.Join("srv", "config.toml"), filepath.Join(server, "config.toml")},
		{"where a link in ~/.ssh leads", "ssh_config.jsonl", filepath.Join(ws, "ssh_config.jsonl")},
		{"a planted link to a credential file", "lnk_file.jsonl", filepath.Join(home, ".aws", "planted.jsonl")},
		{"through a planted link to a credential directory", filepath.Join("lnk_dir", "via.jsonl"), filepath.Join(home, ".aws", "via.jsonl")},
		{"a .env.local file", ".env.local", filepath.Join(ws, ".env.local")},
		{"a link in the workspace to a .env", "to_env.jsonl", filepath.Join(ws, "sub2", ".env")},
	} {
		for _, call := range []struct{ tool, key string }{
			{"synthesize_script", "script_path"}, {"synthesize_script", "casting_path"},
			{"synthesize_line", "casting_path"}, {"master", "script_path"},
		} {
			t.Run(call.tool+"/"+call.key+"/"+c.name, func(t *testing.T) {
				writeFileAt(t, c.leaf, "SECRET=1\n")
				e := answer(call.tool, call.key, c.rel)
				if err := os.Remove(c.leaf); err != nil {
					t.Fatal(err)
				}
				m := answer(call.tool, call.key, c.rel)
				if !strings.Contains(e, `"path_not_allowed"`) || strings.Contains(e, "SECRET") {
					t.Errorf("existing: %s\n  want path_not_allowed, unread", e)
				}
				if e != m {
					t.Errorf("the answer tells them apart\n  existing: %s\n  missing:  %s", e, m)
				}
			})
		}
	}
}

// master reads wav/<id>.wav for every line of the script; a link planted
// there reaches a place on the floor inside the workspace. Refused, it counts
// as missing — the same answer as no file at all — and nothing of it comes back.
func TestMasterDoesNotReadAFloorFileThroughAWav(t *testing.T) {
	base := realDir(t, t.TempDir())
	home := filepath.Join(base, "home")
	t.Setenv("HOME", home)
	work := filepath.Join(base, "sync")
	ws := filepath.Join(work, "ws")
	for _, d := range []string{filepath.Join(home, ".ssh"), filepath.Join(ws, "wav")} {
		mkdirAll(t, d)
	}
	target := filepath.Join(ws, "ssh_config.jsonl")
	symlink(t, target, filepath.Join(home, ".ssh", "config"))
	symlink(t, filepath.Join("..", "ssh_config.jsonl"), filepath.Join(ws, "wav", "1.wav"))
	writeFileAt(t, filepath.Join(ws, "casting.toml"), testCasting)
	writeFileAt(t, filepath.Join(ws, "ok.jsonl"), `{"id":1,"speaker":"narrator","text":"テスト"}`+"\n")

	h := newHarness(t)
	h.deps.WorkDir = workdir.NewResolver(filepath.Join(base, "server"))
	h.deps.WS = workspace.NewManager(h.deps.WorkDir.CheckBeneath, h.deps.WorkDir.LocalPath)
	h.deps.Runner = &fakeFFmpegRunner{}
	h.deps.Cfg.Master.FFmpegPath = "/bin/ls" // exists; the fake runner intercepts
	answer := func() string {
		body, isErr := h.callTool("master", map[string]any{"work_dir": work, "workspace_id": "ws", "script_path": "ok.jsonl"})
		if !isErr {
			return "accepted: " + string(body)
		}
		return string(body)
	}
	writeFileAt(t, target, "Host x\n  IdentityFile ~/.ssh/k\n")
	e := answer()
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	m := answer()
	if !strings.Contains(e, `"master_incomplete"`) || strings.Contains(e, "RIFF") || strings.Contains(e, "Host") {
		t.Errorf("existing: %s\n  want it counted as missing, unread", e)
	}
	if e != m {
		t.Errorf("the answer tells them apart\n  existing: %s\n  missing:  %s", e, m)
	}
}

func realDir(t *testing.T, d string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(d)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func mkdirAll(t *testing.T, d string) {
	t.Helper()
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, at string) {
	t.Helper()
	if err := os.Symlink(target, at); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func writeFileAt(t *testing.T, p, body string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(p))
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
