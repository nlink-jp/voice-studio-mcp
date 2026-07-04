package master

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/script"
	"github.com/nlink-jp/voice-studio-mcp/internal/synth"
	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
	"github.com/nlink-jp/voice-studio-mcp/internal/workspace"
)

// stderrTailBytes bounds how much ffmpeg stderr is attached to errors.
const stderrTailBytes = 512

// Master renders the final audio for a script.
type Master struct {
	Runner Runner
	Cfg    config.MasterConfig
}

// Options control one Build call.
type Options struct {
	Format     string // "mp3" | "m4b"
	OutputName string // basename without extension; default: script file stem
	Chapters   bool   // m4b only: scene boundaries become chapters
}

// Result is the master tool's response payload.
type Result struct {
	MasterPath       string         `json:"master_path"`
	Format           string         `json:"format"`
	DurationSeconds  float64        `json:"duration_seconds"`
	LinesIncluded    int            `json:"lines_included"`
	Chapters         int            `json:"chapters"`
	CreditsPath      string         `json:"credits_path"`
	Credits          []string       `json:"credits"`
	UnverifiedModels []string       `json:"unverified_models,omitempty"`
	Loudnorm         map[string]any `json:"loudnorm"`
}

// Build validates that every line has a synthesized WAV, then concatenates
// them (with per-line trailing silences), loudness-normalizes, and encodes.
func (m *Master) Build(ctx context.Context, ws *workspace.Workspace, scriptStem string, lines []script.Line, casting *script.Casting, opts Options) (Result, error) {
	if _, err := exec.LookPath(m.Cfg.FFmpegPath); err != nil {
		return Result{}, toolerr.Newf(toolerr.CodeFFmpegNotFound,
			"ffmpeg not found at %q — install it (brew install ffmpeg) or set master.ffmpeg_path", m.Cfg.FFmpegPath)
	}

	// 1. Every line must have a WAV; collect formats and durations.
	var (
		pieces  []piece
		missing []int
	)
	for _, ln := range lines {
		p := synth.WavPath(ws, ln.ID)
		b, err := os.ReadFile(p)
		if err != nil {
			missing = append(missing, ln.ID)
			continue
		}
		info, err := synth.ParseWAV(b)
		if err != nil {
			return Result{}, toolerr.Newf(toolerr.CodeMasterIncomplete,
				"wav for line %d is unreadable: %v — re-synthesize it with synthesize_line force=true", ln.ID, err)
		}
		pause := 0
		if ln.PauseAfterMS != nil {
			pause = *ln.PauseAfterMS
		}
		pieces = append(pieces, piece{line: ln, wavPath: p, info: info, pauseMS: pause})
	}
	if len(missing) > 0 {
		return Result{}, toolerr.Newf(toolerr.CodeMasterIncomplete,
			"%d line(s) have no synthesized WAV — run synthesize_script first", len(missing)).
			WithDetails(map[string]any{"missing_line_ids": missing})
	}
	if len(pieces) == 0 {
		return Result{}, toolerr.New(toolerr.CodeMasterIncomplete, "script contains no lines")
	}
	rate := pieces[0].info.SampleRate
	for _, p := range pieces {
		if p.info.SampleRate != rate {
			return Result{}, toolerr.Newf(toolerr.CodeMasterIncomplete,
				"line %d has sample rate %d but line %d has %d — re-run synthesize_script with force=true to unify",
				p.line.ID, p.info.SampleRate, pieces[0].line.ID, rate)
		}
	}

	// 2. Prepare the tmp area (concat list, silences, chapters).
	tmpDir := ws.Path(workspace.DirMaster, "tmp")
	if err := os.RemoveAll(tmpDir); err != nil {
		return Result{}, toolerr.Newf(toolerr.CodeWorkspaceFailed, "clean master tmp: %v", err)
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return Result{}, toolerr.Newf(toolerr.CodeWorkspaceFailed, "create master tmp: %v", err)
	}

	// 3. One silence WAV per distinct pause duration.
	silencePaths := map[int]string{}
	for _, p := range pieces {
		if p.pauseMS <= 0 {
			continue
		}
		if _, ok := silencePaths[p.pauseMS]; ok {
			continue
		}
		out := filepath.Join(tmpDir, fmt.Sprintf("silence_%d.wav", p.pauseMS))
		if err := m.runFFmpeg(ctx, silenceArgs(rate, p.pauseMS, out)); err != nil {
			return Result{}, err
		}
		silencePaths[p.pauseMS] = out
	}

	// 4. Concat list in script order: line WAV, then its trailing silence.
	var concatPaths []string
	totalMS := 0
	for _, p := range pieces {
		concatPaths = append(concatPaths, p.wavPath)
		totalMS += int(p.info.DurationSeconds * 1000)
		if p.pauseMS > 0 {
			concatPaths = append(concatPaths, silencePaths[p.pauseMS])
			totalMS += p.pauseMS
		}
	}
	listPath := filepath.Join(tmpDir, "concat.txt")
	if err := os.WriteFile(listPath, []byte(concatList(concatPaths)), 0o644); err != nil {
		return Result{}, toolerr.Newf(toolerr.CodeWorkspaceFailed, "write concat list: %v", err)
	}

	// 5. Chapters from scene boundaries (m4b only).
	metadataPath := ""
	chapters := 0
	if opts.Format == "m4b" && opts.Chapters {
		chs := sceneChapters(pieces)
		chapters = len(chs)
		metadataPath = filepath.Join(tmpDir, "ffmetadata.txt")
		if err := os.WriteFile(metadataPath, []byte(ffmetadata(chs)), 0o644); err != nil {
			return Result{}, toolerr.Newf(toolerr.CodeWorkspaceFailed, "write chapter metadata: %v", err)
		}
	}

	// 6. Final encode.
	name := opts.OutputName
	if name == "" {
		name = scriptStem
	}
	outPath := ws.Path(workspace.DirMaster, name+"."+opts.Format)
	ln := Loudnorm{I: m.Cfg.LoudnormI, TP: m.Cfg.LoudnormTP, LRA: m.Cfg.LoudnormLRA}
	bitrate := m.Cfg.MP3Bitrate
	if opts.Format == "m4b" {
		bitrate = m.Cfg.M4BBitrate
	}
	if err := m.runFFmpeg(ctx, concatArgs(listPath, metadataPath, opts.Format, outPath, ln, bitrate)); err != nil {
		return Result{}, err
	}

	// 7. Credits from the characters actually used.
	credits, unverified := collectCredits(lines, casting)
	creditsPath := ws.Path(workspace.DirMaster, name+".credits.txt")
	if err := os.WriteFile(creditsPath, []byte(creditsText(credits, unverified)), 0o644); err != nil {
		return Result{}, toolerr.Newf(toolerr.CodeWorkspaceFailed, "write credits: %v", err)
	}

	return Result{
		MasterPath:       outPath,
		Format:           opts.Format,
		DurationSeconds:  float64(totalMS) / 1000.0,
		LinesIncluded:    len(pieces),
		Chapters:         chapters,
		CreditsPath:      creditsPath,
		Credits:          credits,
		UnverifiedModels: unverified,
		Loudnorm:         map[string]any{"i": ln.I, "tp": ln.TP, "lra": ln.LRA},
	}, nil
}

func (m *Master) runFFmpeg(ctx context.Context, args []string) error {
	_, stderr, code, err := m.Runner.Run(ctx, m.Cfg.FFmpegPath, args)
	if err != nil && code <= 0 {
		return toolerr.Newf(toolerr.CodeFFmpegFailed, "ffmpeg did not start: %v", err)
	}
	if code != 0 {
		tail := stderr
		if len(tail) > stderrTailBytes {
			tail = tail[len(tail)-stderrTailBytes:]
		}
		return toolerr.Newf(toolerr.CodeFFmpegFailed, "ffmpeg exited with code %d", code).
			WithDetails(map[string]any{
				"exit_code":   code,
				"stderr_tail": string(tail),
				"args":        args,
			})
	}
	return nil
}

// piece is one script line with its rendered WAV.
type piece struct {
	line    script.Line
	wavPath string
	info    synth.WAVInfo
	pauseMS int
}

// sceneChapters derives chapter boundaries from scene transitions, using the
// accumulated durations (WAV length + trailing pause per line). loudnorm does
// not change duration, so these times are exact.
func sceneChapters(pieces []piece) []Chapter {
	var chapters []Chapter
	cursor := 0
	sceneStart := 0
	currentScene := pieces[0].line.Scene
	for _, p := range pieces {
		if p.line.Scene != currentScene {
			chapters = append(chapters, Chapter{
				Title:   fmt.Sprintf("Scene %d", currentScene),
				StartMS: sceneStart,
				EndMS:   cursor,
			})
			sceneStart = cursor
			currentScene = p.line.Scene
		}
		cursor += int(p.info.DurationSeconds*1000) + p.pauseMS
	}
	chapters = append(chapters, Chapter{
		Title:   fmt.Sprintf("Scene %d", currentScene),
		StartMS: sceneStart,
		EndMS:   cursor,
	})
	return chapters
}

// collectCredits gathers the unique credit lines of the characters used in
// the script, plus the characters whose license was never marked as checked.
func collectCredits(lines []script.Line, casting *script.Casting) (credits, unverified []string) {
	usedCharacters := map[string]bool{}
	for _, ln := range lines {
		usedCharacters[ln.Speaker] = true
	}
	creditSet := map[string]bool{}
	unverifiedSet := map[string]bool{}
	for name := range usedCharacters {
		entry, ok := casting.Characters[name]
		if !ok {
			continue // validated earlier; defensive
		}
		if entry.Credit != "" {
			creditSet[entry.Credit] = true
		}
		if !entry.LicenseChecked {
			unverifiedSet[name] = true
		}
	}
	for c := range creditSet {
		credits = append(credits, c)
	}
	sort.Strings(credits)
	for u := range unverifiedSet {
		unverified = append(unverified, u)
	}
	sort.Strings(unverified)
	return credits, unverified
}

func creditsText(credits, unverified []string) string {
	var b strings.Builder
	b.WriteString("Voice synthesis credits / 音声合成クレジット\n")
	for _, c := range credits {
		b.WriteString(c + "\n")
	}
	if len(unverified) > 0 {
		b.WriteString("\n# WARNING: license not verified for these characters' voice models:\n")
		for _, u := range unverified {
			b.WriteString("# - " + u + "\n")
		}
	}
	return b.String()
}
