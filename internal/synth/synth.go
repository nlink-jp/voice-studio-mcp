// Package synth turns one script line into a WAV file on disk, with a
// content-hash cache so unchanged lines are never re-synthesized.
package synth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/script"
	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
	"github.com/nlink-jp/voice-studio-mcp/internal/workspace"
)

// Synthesizer renders script lines through the engine.
type Synthesizer struct {
	Client *engine.Client
	Cfg    config.SynthesisConfig
	// EngineVersion is baked into cache keys; pass the value from
	// Client.Version() at startup.
	EngineVersion string
}

// LineResult describes one rendered (or cache-hit) line.
type LineResult struct {
	LineID          int     `json:"line_id"`
	WavPath         string  `json:"wav_path"`
	DurationSeconds float64 `json:"duration_seconds"`
	Cached          bool    `json:"cached"`
	StyleID         int     `json:"style_id"`
}

// Resolved carries the casting resolution for a line.
type Resolved struct {
	StyleID     int
	SpeakerUUID string
}

// WavPath returns the output path for a line inside ws.
func WavPath(ws *workspace.Workspace, lineID int) string {
	return ws.Path(workspace.DirWav, strconv.Itoa(lineID)+".wav")
}

// CacheIndexPath returns the cache index path inside ws.
func CacheIndexPath(ws *workspace.Workspace) string {
	return ws.Path(workspace.DirCache, "index.json")
}

// Key computes the cache key for a line under the synthesizer's settings.
func (s *Synthesizer) Key(line script.Line, res Resolved) string {
	speed, intensity, volume := s.effective(line)
	return CacheKey(s.EngineVersion, res.SpeakerUUID, res.StyleID,
		speed, intensity, volume, s.Cfg.PrePhonemeLength, s.Cfg.PostPhonemeLength,
		s.Cfg.OutputSamplingRate, line.Text)
}

// SynthesizeLine renders one line into ws (wav/<id>.wav), honoring the cache
// unless force is set.
func (s *Synthesizer) SynthesizeLine(ctx context.Context, ws *workspace.Workspace, store *CacheStore, line script.Line, res Resolved, force bool) (LineResult, error) {
	wavPath := WavPath(ws, line.ID)
	hash := s.Key(line, res)

	if !force {
		if e, hit := store.Lookup(line.ID, hash, wavPath); hit {
			return LineResult{
				LineID:          line.ID,
				WavPath:         wavPath,
				DurationSeconds: e.DurationSeconds,
				Cached:          true,
				StyleID:         res.StyleID,
			}, nil
		}
	}

	q, err := s.Client.AudioQuery(ctx, line.Text, res.StyleID)
	if err != nil {
		return LineResult{}, err
	}
	speed, intensity, volume := s.effective(line)
	// Override only the keys we own; engine-specific fields (e.g.
	// tempoDynamicsScale) pass through untouched. The sampling rate is forced
	// to one value across all lines so mastering can concat losslessly.
	q["speedScale"] = speed
	q["intonationScale"] = intensity
	q["volumeScale"] = volume
	q["prePhonemeLength"] = s.Cfg.PrePhonemeLength
	q["postPhonemeLength"] = s.Cfg.PostPhonemeLength
	q["outputSamplingRate"] = s.Cfg.OutputSamplingRate
	q["outputStereo"] = false

	wav, err := s.Client.Synthesis(ctx, q, res.StyleID)
	if err != nil {
		return LineResult{}, err
	}
	info, err := ParseWAV(wav)
	if err != nil {
		return LineResult{}, toolerr.Newf(toolerr.CodeEngineRequest,
			"engine returned an unreadable WAV for line %d: %v", line.ID, err)
	}

	if err := writeFileAtomic(wavPath, wav); err != nil {
		return LineResult{}, toolerr.Newf(toolerr.CodeWorkspaceFailed,
			"write %s: %v", wavPath, err)
	}
	if err := store.Put(line.ID, CacheEntry{Hash: hash, DurationSeconds: info.DurationSeconds}); err != nil {
		return LineResult{}, toolerr.Newf(toolerr.CodeWorkspaceFailed,
			"update cache index: %v", err)
	}

	return LineResult{
		LineID:          line.ID,
		WavPath:         wavPath,
		DurationSeconds: info.DurationSeconds,
		Cached:          false,
		StyleID:         res.StyleID,
	}, nil
}

func (s *Synthesizer) effective(line script.Line) (speed, intensity, volume float64) {
	speed = s.Cfg.DefaultSpeed
	if line.Speed != nil {
		speed = *line.Speed
	}
	intensity = s.Cfg.DefaultIntensity
	if line.Intensity != nil {
		intensity = *line.Intensity
	}
	volume = s.Cfg.DefaultVolume
	if line.Volume != nil {
		volume = *line.Volume
	}
	return speed, intensity, volume
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp", path)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
