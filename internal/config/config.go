// Package config loads the voice-studio-mcp TOML configuration.
//
// Unknown keys are rejected (typo detection via toml.MetaData.Undecoded),
// and "~" is expanded in path-valued fields. Defaults are chosen so the
// server runs out of the box on a Mac with AivisSpeech installed.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the root configuration.
type Config struct {
	Server          ServerConfig      `toml:"server"`
	Workspace       WorkspaceConfig   `toml:"workspace"`
	Engine          EngineConfig      `toml:"engine"`
	Synthesis       SynthesisConfig   `toml:"synthesis"`
	Master          MasterConfig      `toml:"master"`
	SpeakerMetadata []SpeakerMetadata `toml:"speaker_metadata"`
}

// ServerConfig controls logging.
type ServerConfig struct {
	LogLevel string `toml:"log_level"` // debug|info|warn|error
	LogFile  string `toml:"log_file"`  // empty = stderr only
}

// WorkspaceConfig controls where per-work state lives.
type WorkspaceConfig struct {
	Dir string `toml:"workspace_dir"`
}

// EngineConfig controls how the AivisSpeech Engine is reached.
//
// mode="managed" spawns Command as a child process and reaps it on shutdown.
// mode="external" connects to URL only (manual start, tests, remote engine).
type EngineConfig struct {
	Mode                   string   `toml:"mode"`
	URL                    string   `toml:"url"`
	Command                string   `toml:"command"`
	Args                   []string `toml:"args"`
	StartupTimeoutSeconds  int      `toml:"startup_timeout_seconds"`
	RequestTimeoutSeconds  int      `toml:"request_timeout_seconds"`
	ShutdownTimeoutSeconds int      `toml:"shutdown_timeout_seconds"`
}

// SynthesisConfig sets synthesis defaults applied to every line.
type SynthesisConfig struct {
	Concurrency        int     `toml:"concurrency"`
	DefaultSpeed       float64 `toml:"default_speed"`
	DefaultIntensity   float64 `toml:"default_intensity"`
	DefaultVolume      float64 `toml:"default_volume"`
	PrePhonemeLength   float64 `toml:"pre_phoneme_length"`
	PostPhonemeLength  float64 `toml:"post_phoneme_length"`
	OutputSamplingRate int     `toml:"output_sampling_rate"`
}

// MasterConfig controls the ffmpeg mastering step.
type MasterConfig struct {
	FFmpegPath  string  `toml:"ffmpeg_path"`
	LoudnormI   float64 `toml:"loudnorm_i"`
	LoudnormTP  float64 `toml:"loudnorm_tp"`
	LoudnormLRA float64 `toml:"loudnorm_lra"`
	MP3Bitrate  string  `toml:"mp3_bitrate"`
	M4BBitrate  string  `toml:"m4b_bitrate"`
}

// SpeakerMetadata is the hand-maintained license record for one voice model.
// The engine API does not expose usage terms, so this config section is the
// source of truth; list_speakers joins it by speaker_uuid.
type SpeakerMetadata struct {
	SpeakerUUID   string `toml:"speaker_uuid"`
	Name          string `toml:"name"`
	License       string `toml:"license"`
	LicenseURL    string `toml:"license_url"`
	Credit        string `toml:"credit"`
	CommercialUse bool   `toml:"commercial_use"`
	Notes         string `toml:"notes"`
}

// Engine modes.
const (
	EngineModeManaged  = "managed"
	EngineModeExternal = "external"
)

// DefaultEngineCommand is where the AivisSpeech.app bundle ships the engine
// binary on macOS.
const DefaultEngineCommand = "/Applications/AivisSpeech.app/Contents/Resources/AivisSpeech-Engine/run"

// Default returns the built-in configuration.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			LogLevel: "info",
		},
		Workspace: WorkspaceConfig{
			Dir: ExpandHome("~/.voice-studio"),
		},
		Engine: EngineConfig{
			Mode:                   EngineModeManaged,
			URL:                    "http://127.0.0.1:10101",
			Command:                DefaultEngineCommand,
			StartupTimeoutSeconds:  180,
			RequestTimeoutSeconds:  300,
			ShutdownTimeoutSeconds: 10,
		},
		Synthesis: SynthesisConfig{
			Concurrency:        1,
			DefaultSpeed:       1.0,
			DefaultIntensity:   1.0,
			DefaultVolume:      1.0,
			PrePhonemeLength:   0.1,
			PostPhonemeLength:  0.1,
			OutputSamplingRate: 44100,
		},
		Master: MasterConfig{
			FFmpegPath: "ffmpeg",
			// -18 LUFS (audiobook range) rather than the louder -16 podcast
			// standard: narration listened to for long stretches wants the
			// extra headroom.
			LoudnormI:   -18.0,
			LoudnormTP:  -1.5,
			LoudnormLRA: 11.0,
			MP3Bitrate:  "192k",
			M4BBitrate:  "128k",
		},
	}
}

// Load reads path over the defaults. Unknown keys are an error.
func Load(path string) (*Config, error) {
	cfg := Default()
	meta, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("load %s: unknown config keys: %v", path, undecoded)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	cfg.Workspace.Dir = ExpandHome(cfg.Workspace.Dir)
	cfg.Server.LogFile = ExpandHome(cfg.Server.LogFile)
	cfg.Engine.Command = ExpandHome(cfg.Engine.Command)
	cfg.Master.FFmpegPath = ExpandHome(cfg.Master.FFmpegPath)
	return cfg, nil
}

func (c *Config) validate() error {
	switch c.Engine.Mode {
	case EngineModeManaged, EngineModeExternal:
	default:
		return fmt.Errorf("engine.mode must be %q or %q, got %q",
			EngineModeManaged, EngineModeExternal, c.Engine.Mode)
	}
	if c.Synthesis.Concurrency < 1 {
		return fmt.Errorf("synthesis.concurrency must be >= 1, got %d", c.Synthesis.Concurrency)
	}
	if c.Synthesis.OutputSamplingRate < 8000 {
		return fmt.Errorf("synthesis.output_sampling_rate must be >= 8000, got %d", c.Synthesis.OutputSamplingRate)
	}
	if c.Synthesis.DefaultVolume <= 0 || c.Synthesis.DefaultVolume > 2.0 {
		return fmt.Errorf("synthesis.default_volume must be within (0.0, 2.0], got %g", c.Synthesis.DefaultVolume)
	}
	return nil
}

// MetadataFor returns the SpeakerMetadata entry for uuid, or nil.
func (c *Config) MetadataFor(uuid string) *SpeakerMetadata {
	for i := range c.SpeakerMetadata {
		if c.SpeakerMetadata[i].SpeakerUUID == uuid {
			return &c.SpeakerMetadata[i]
		}
	}
	return nil
}

// ExpandHome expands a leading "~" to the user's home directory.
func ExpandHome(p string) string {
	if p == "" || p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	if len(p) > 1 && p[1] == '/' {
		return filepath.Join(home, p[2:])
	}
	return p
}
