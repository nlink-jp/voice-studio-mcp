package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/job"
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

// The files the server names itself are judged too. A link planted at a
// line's cached WAV to a place on the floor inside the workspace does not
// count as cached, whether or not the file behind it is there; and a link
// planted at the dictionary record is not read and written back into the
// workspace, where the caller could read it.
func TestServerNamedFilesAreJudged(t *testing.T) {
	base := realDir(t, t.TempDir())
	home := filepath.Join(base, "home")
	t.Setenv("HOME", home)
	work := filepath.Join(base, "sync")
	ws := filepath.Join(work, "ws")
	server := filepath.Join(ws, "srv")
	for _, d := range []string{filepath.Join(home, ".ssh"), server} {
		mkdirAll(t, d)
	}
	target := filepath.Join(ws, "ssh_config.wav")
	symlink(t, target, filepath.Join(home, ".ssh", "config"))
	writeFileAt(t, filepath.Join(ws, "casting.toml"), testCasting)
	writeFileAt(t, filepath.Join(ws, "ok.jsonl"), `{"id":1,"speaker":"narrator","text":"テスト"}`+"\n")

	h := newHarness(t)
	h.deps.WorkDir = workdir.NewResolver(server)
	h.deps.WS = workspace.NewManager(h.deps.WorkDir.CheckBeneath, h.deps.WorkDir.LocalPath)
	args := func() map[string]any {
		return map[string]any{"work_dir": work, "workspace_id": "ws", "script_path": "ok.jsonl"}
	}
	if _, jo := h.runScriptJob(args()); jo.State != "done" || jo.Failed != 0 {
		t.Fatalf("first synthesis: %+v", jo)
	}
	// Each run re-synthesizes the line and replaces the link with a real WAV,
	// so the link is planted again before each one, and each job is waited for.
	wav := filepath.Join(ws, "wav", "1.wav")
	cached := func() int {
		if err := os.Remove(wav); err != nil {
			t.Fatal(err)
		}
		symlink(t, filepath.Join("..", "ssh_config.wav"), wav)
		so, jo := h.runScriptJob(args())
		if jo.State != "done" {
			t.Fatalf("synthesis: %+v", jo)
		}
		return so.Cached
	}
	writeFileAt(t, target, "RIFF-not-really")
	e := cached()
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if m := cached(); e != 0 || m != 0 {
		t.Errorf("a cached WAV linked to a floor place: cached %d with the file, %d without; want 0 both", e, m)
	}

	secret := filepath.Join(server, "words.json")
	writeFileAt(t, secret, `{"SECRETWORD":{"pronunciation":"シークレット","accent_type":1,"engine_uuid":"x"}}`)
	record := filepath.Join(ws, "dict", "words.json")
	_ = os.Remove(record)
	symlink(t, filepath.Join("..", "srv", "words.json"), record)
	body, isErr := h.callTool("register_dictionary", map[string]any{
		"work_dir": work, "workspace_id": "ws",
		"words": []map[string]any{{"surface": "美咲", "pronunciation": "ミサキ", "accent_type": 1}},
	})
	if isErr {
		t.Fatalf("register_dictionary: %s", body)
	}
	b, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "SECRETWORD") {
		t.Errorf("the planted record's contents were written into the workspace: %s", b)
	}
}

// A job runs after the call that queued it, behind other jobs; what the call
// judged is not remembered for it. Here the reply finds line 1's WAV cached, a
// held job slot keeps the job waiting, the WAV is swapped for a link to a place
// on the floor, and the job must not count it as cached — whether or not the
// file behind the link is there.
func TestAJobJudgesAfresh(t *testing.T) {
	base := realDir(t, t.TempDir())
	home := filepath.Join(base, "home")
	t.Setenv("HOME", home)
	work := filepath.Join(base, "sync")
	ws := filepath.Join(work, "ws")
	mkdirAll(t, filepath.Join(home, ".ssh"))
	target := filepath.Join(ws, "ssh_config.wav")
	symlink(t, target, filepath.Join(home, ".ssh", "config"))
	writeFileAt(t, filepath.Join(ws, "casting.toml"), testCasting)
	writeFileAt(t, filepath.Join(ws, "ok.jsonl"), `{"id":1,"speaker":"narrator","text":"テスト"}`+"\n")

	h := newHarness(t)
	h.deps.WorkDir = workdir.NewResolver(filepath.Join(base, "server"))
	h.deps.WS = workspace.NewManager(h.deps.WorkDir.CheckBeneath, h.deps.WorkDir.LocalPath)
	h.deps.Jobs = job.NewManager(1)
	args := map[string]any{"work_dir": work, "workspace_id": "ws", "script_path": "ok.jsonl"}
	if _, jo := h.runScriptJob(args); jo.State != "done" {
		t.Fatalf("first synthesis: %+v", jo)
	}
	wav := filepath.Join(ws, "wav", "1.wav")
	for _, present := range []bool{true, false} {
		started, release := make(chan struct{}), make(chan struct{})
		h.deps.Jobs.Submit(context.Background(), ws, []job.Item{{LineID: 0, Run: func(context.Context) (bool, error) {
			close(started)
			<-release
			return false, nil
		}}})
		<-started // the slot is held before the call queues its job
		so, jo := func() (scriptOut, jobOut) {
			body, isErr := h.callTool("synthesize_script", args)
			if isErr {
				t.Fatalf("synthesize_script: %s", body)
			}
			var so scriptOut
			if err := json.Unmarshal(body, &so); err != nil {
				t.Fatal(err)
			}
			// The reply has judged the real WAV; now swap it while the job waits.
			if err := os.Remove(wav); err != nil {
				t.Fatal(err)
			}
			symlink(t, filepath.Join("..", "ssh_config.wav"), wav)
			if present {
				writeFileAt(t, target, "RIFF-not-really")
			} else {
				_ = os.Remove(target)
			}
			close(release)
			return so, h.awaitJob(so.JobID)
		}()
		if so.Cached != 1 {
			t.Fatalf("the reply found %d cached, want 1 (the real WAV)", so.Cached)
		}
		if jo.Cached != 0 {
			t.Errorf("file behind the link present=%v: the job counted %d cached, want 0", present, jo.Cached)
		}
	}
}

// awaitJob polls check_job until the job is no longer running.
func (h *testHarness) awaitJob(id string) jobOut {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		jb, isErr := h.callTool("check_job", map[string]any{"job_id": id})
		if isErr {
			h.t.Fatalf("check_job: %s", jb)
		}
		var jo jobOut
		if err := json.Unmarshal(jb, &jo); err != nil {
			h.t.Fatal(err)
		}
		if jo.State != "running" {
			return jo
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("job stuck: %+v", jo)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
