package master

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine/enginetest"
	"github.com/nlink-jp/voice-studio-mcp/internal/script"
	"github.com/nlink-jp/voice-studio-mcp/internal/synth"
	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
	"github.com/nlink-jp/voice-studio-mcp/internal/workspace"
)

// fakeRunner records ffmpeg invocations and materializes each command's
// output file (the last argument) so subsequent steps find it.
type fakeRunner struct {
	cmds     [][]string
	failAt   int // 1-based index of the call that should fail; 0 = never
	exitCode int
	stderr   string
}

func (f *fakeRunner) Run(ctx context.Context, name string, args []string) ([]byte, []byte, int, error) {
	f.cmds = append(f.cmds, append([]string{name}, args...))
	if f.failAt > 0 && len(f.cmds) == f.failAt {
		return nil, []byte(f.stderr), f.exitCode, errors.New("exit status")
	}
	out := args[len(args)-1]
	_ = os.WriteFile(out, []byte("fake-output"), 0o644)
	return nil, nil, 0, nil
}

func intp(v int) *int { return &v }

var testCastingTable = &script.Casting{Characters: map[string]script.CastEntry{
	"narrator": {SpeakerUUID: "uuid-n", StyleID: 100, Credit: "AivisSpeech:MockNarrator", LicenseChecked: true},
	"美咲":       {SpeakerUUID: "uuid-h", StyleID: 200, Credit: "AivisSpeech:MockHeroine"},
}}

// seed prepares a workspace with synthesized WAVs (1s each at 44100Hz) for
// the given lines.
func seed(t *testing.T, lines []script.Line) *workspace.Workspace {
	t.Helper()
	ws, err := workspace.NewManager(t.TempDir()).Ensure("ep1")
	if err != nil {
		t.Fatal(err)
	}
	for _, ln := range lines {
		if err := os.WriteFile(synth.WavPath(ws, ln.ID), enginetest.WAV(44100, 1000), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return ws
}

func newMaster(r Runner) *Master {
	cfg := config.Default().Master
	cfg.FFmpegPath = "/bin/ls" // exists; fakeRunner never actually executes it
	return &Master{Runner: r, Cfg: cfg}
}

var testLines = []script.Line{
	{ID: 1, Scene: 1, Speaker: "narrator", Text: "a", PauseAfterMS: intp(800)},
	{ID: 2, Scene: 1, Speaker: "美咲", Text: "b"},
	{ID: 3, Scene: 2, Speaker: "narrator", Text: "c", PauseAfterMS: intp(800)},
}

func TestBuildMP3(t *testing.T) {
	ws := seed(t, testLines)
	fr := &fakeRunner{}
	m := newMaster(fr)

	res, err := m.Build(context.Background(), ws, "ep1", testLines, testCastingTable, Options{Format: "mp3", Chapters: true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if filepath.Base(res.MasterPath) != "ep1.mp3" || res.LinesIncluded != 3 {
		t.Errorf("result: %+v", res)
	}
	// mp3 never gets chapters.
	if res.Chapters != 0 {
		t.Errorf("chapters on mp3: %d", res.Chapters)
	}
	// Duration = 3×1000ms audio + 2×800ms pauses.
	if res.DurationSeconds < 4.5 || res.DurationSeconds > 4.7 {
		t.Errorf("duration: %v", res.DurationSeconds)
	}

	// Invocations: 1 distinct silence (800ms reused) + 1 final encode.
	if len(fr.cmds) != 2 {
		t.Fatalf("ffmpeg calls: %d\n%v", len(fr.cmds), fr.cmds)
	}
	silence := strings.Join(fr.cmds[0], " ")
	if !strings.Contains(silence, "anullsrc=r=44100:cl=mono") || !strings.Contains(silence, "-t 0.800") {
		t.Errorf("silence args: %s", silence)
	}
	final := strings.Join(fr.cmds[1], " ")
	for _, want := range []string{"-f concat", "-safe 0", "loudnorm=I=-18:TP=-1.5:LRA=11", "-c:a libmp3lame", "-b:a 192k", "ep1.mp3"} {
		if !strings.Contains(final, want) {
			t.Errorf("final args missing %q: %s", want, final)
		}
	}
	if strings.Contains(final, "map_metadata") {
		t.Errorf("mp3 must not reference chapter metadata: %s", final)
	}

	// Concat list interleaves audio and silences in script order.
	list, err := os.ReadFile(ws.Path(workspace.DirMaster, "tmp", "concat.txt"))
	if err != nil {
		t.Fatal(err)
	}
	entries := strings.Split(strings.TrimSpace(string(list)), "\n")
	if len(entries) != 5 { // 1.wav, silence, 2.wav, 3.wav, silence
		t.Errorf("concat entries: %v", entries)
	}
	if !strings.Contains(entries[0], "1.wav") || !strings.Contains(entries[1], "silence_800.wav") ||
		!strings.Contains(entries[2], "2.wav") || !strings.Contains(entries[3], "3.wav") {
		t.Errorf("concat order: %v", entries)
	}

	// Credits: both characters' credit lines, 美咲 unverified.
	if len(res.Credits) != 2 || res.Credits[0] != "AivisSpeech:MockHeroine" {
		t.Errorf("credits: %v", res.Credits)
	}
	if len(res.UnverifiedModels) != 1 || res.UnverifiedModels[0] != "美咲" {
		t.Errorf("unverified: %v", res.UnverifiedModels)
	}
	creditsBody, err := os.ReadFile(res.CreditsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(creditsBody), "AivisSpeech:MockNarrator") ||
		!strings.Contains(string(creditsBody), "WARNING") {
		t.Errorf("credits file: %s", creditsBody)
	}
}

func TestBuildM4BWithChapters(t *testing.T) {
	ws := seed(t, testLines)
	fr := &fakeRunner{}
	m := newMaster(fr)

	res, err := m.Build(context.Background(), ws, "ep1", testLines, testCastingTable, Options{Format: "m4b", Chapters: true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.Chapters != 2 || filepath.Base(res.MasterPath) != "ep1.m4b" {
		t.Errorf("result: %+v", res)
	}
	final := strings.Join(fr.cmds[len(fr.cmds)-1], " ")
	for _, want := range []string{"-map_metadata 1", "-c:a aac", "-b:a 128k", "-f ipod", "ep1.m4b"} {
		if !strings.Contains(final, want) {
			t.Errorf("final args missing %q: %s", want, final)
		}
	}

	// Chapter times: scene 1 = 1000+800+1000 = 2800ms, scene 2 = 1000+800.
	meta, err := os.ReadFile(ws.Path(workspace.DirMaster, "tmp", "ffmetadata.txt"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(meta)
	for _, want := range []string{";FFMETADATA1", "TIMEBASE=1/1000", "START=0", "END=2800", "START=2800", "END=4600", "title=Scene 1", "title=Scene 2"} {
		if !strings.Contains(s, want) {
			t.Errorf("ffmetadata missing %q:\n%s", want, s)
		}
	}
}

func TestBuildM4BChaptersDisabled(t *testing.T) {
	ws := seed(t, testLines)
	fr := &fakeRunner{}
	m := newMaster(fr)

	res, err := m.Build(context.Background(), ws, "ep1", testLines, testCastingTable, Options{Format: "m4b", Chapters: false})
	if err != nil {
		t.Fatal(err)
	}
	if res.Chapters != 0 {
		t.Errorf("chapters: %d", res.Chapters)
	}
	final := strings.Join(fr.cmds[len(fr.cmds)-1], " ")
	if strings.Contains(final, "map_metadata") {
		t.Errorf("chapters disabled must not add metadata: %s", final)
	}
}

func TestBuildMissingWavs(t *testing.T) {
	ws := seed(t, testLines[:1]) // only line 1 synthesized
	m := newMaster(&fakeRunner{})

	_, err := m.Build(context.Background(), ws, "ep1", testLines, testCastingTable, Options{Format: "mp3"})
	var te *toolerr.Error
	if !errors.As(err, &te) || te.Code != toolerr.CodeMasterIncomplete {
		t.Fatalf("expected master_incomplete, got %v", err)
	}
	ids := te.Details["missing_line_ids"].([]int)
	if len(ids) != 2 || ids[0] != 2 || ids[1] != 3 {
		t.Errorf("missing ids: %v", ids)
	}
}

func TestBuildFFmpegFailureSurfacesStderr(t *testing.T) {
	ws := seed(t, testLines)
	fr := &fakeRunner{failAt: 2, exitCode: 1, stderr: "Invalid data found when processing input"}
	m := newMaster(fr)

	_, err := m.Build(context.Background(), ws, "ep1", testLines, testCastingTable, Options{Format: "mp3"})
	var te *toolerr.Error
	if !errors.As(err, &te) || te.Code != toolerr.CodeFFmpegFailed {
		t.Fatalf("expected ffmpeg_failed, got %v", err)
	}
	if te.Details["exit_code"] != 1 || !strings.Contains(te.Details["stderr_tail"].(string), "Invalid data") {
		t.Errorf("details: %v", te.Details)
	}
}

func TestBuildFFmpegNotFound(t *testing.T) {
	ws := seed(t, testLines)
	m := newMaster(&fakeRunner{})
	m.Cfg.FFmpegPath = "definitely-no-such-ffmpeg"

	_, err := m.Build(context.Background(), ws, "ep1", testLines, testCastingTable, Options{Format: "mp3"})
	if !errors.Is(err, toolerr.New(toolerr.CodeFFmpegNotFound, "")) {
		t.Fatalf("expected ffmpeg_not_found, got %v", err)
	}
}

func TestConcatListQuoting(t *testing.T) {
	got := concatList([]string{"/a/it's.wav"})
	if got != "file '/a/it'\\''s.wav'\n" {
		t.Errorf("quoting: %q", got)
	}
}

func TestFFMetadataEscaping(t *testing.T) {
	s := ffmetadata([]Chapter{{Title: "a=b;c#d", StartMS: 0, EndMS: 10}})
	if !strings.Contains(s, `title=a\=b\;c\#d`) {
		t.Errorf("escaping: %s", s)
	}
}
