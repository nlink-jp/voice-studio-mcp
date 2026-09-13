package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
)

func TestDefaultValues(t *testing.T) {
	c := config.Default()
	if c.Engine.Mode != config.EngineModeManaged {
		t.Errorf("default engine mode: %q", c.Engine.Mode)
	}
	if c.Engine.URL != "http://127.0.0.1:10101" {
		t.Errorf("default engine url: %q", c.Engine.URL)
	}
	if c.Synthesis.Concurrency != 1 {
		t.Errorf("default concurrency: %d", c.Synthesis.Concurrency)
	}
	if c.Synthesis.OutputSamplingRate != 44100 {
		t.Errorf("default sampling rate: %d", c.Synthesis.OutputSamplingRate)
	}
	if c.Master.FFmpegPath != "ffmpeg" {
		t.Errorf("default ffmpeg path: %q", c.Master.FFmpegPath)
	}
	if c.Synthesis.DefaultVolume != 1.0 {
		t.Errorf("default volume: %v", c.Synthesis.DefaultVolume)
	}
	if c.Master.LoudnormI != -18.0 {
		t.Errorf("default loudnorm target: %v", c.Master.LoudnormI)
	}
}

func TestLoadFullFile(t *testing.T) {
	path := writeConfig(t, `
[server]
log_level = "debug"

[engine]
mode = "external"
url = "http://127.0.0.1:9999"
startup_timeout_seconds = 5

[synthesis]
concurrency = 2
default_speed = 1.1
output_sampling_rate = 24000

[master]
ffmpeg_path = "/opt/homebrew/bin/ffmpeg"
mp3_bitrate = "128k"

[[speaker_metadata]]
speaker_uuid = "uuid-1"
name = "Anneli"
license = "ACML 1.0"
credit = "AivisSpeech:Anneli"
commercial_use = true

[[speaker_metadata]]
speaker_uuid = "uuid-2"
name = "Other"
license = "CC BY-SA 4.0"
`)
	c, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Server.LogLevel != "debug" {
		t.Errorf("log_level: %q", c.Server.LogLevel)
	}
	if c.Engine.Mode != config.EngineModeExternal || c.Engine.URL != "http://127.0.0.1:9999" {
		t.Errorf("engine: %+v", c.Engine)
	}
	// Values not present keep defaults.
	if c.Engine.RequestTimeoutSeconds != 300 {
		t.Errorf("request timeout default lost: %d", c.Engine.RequestTimeoutSeconds)
	}
	if c.Synthesis.Concurrency != 2 || c.Synthesis.OutputSamplingRate != 24000 {
		t.Errorf("synthesis: %+v", c.Synthesis)
	}
	if c.Synthesis.DefaultIntensity != 1.0 {
		t.Errorf("intensity default lost: %v", c.Synthesis.DefaultIntensity)
	}
	// speaker_metadata array parsed and joinable.
	if len(c.SpeakerMetadata) != 2 {
		t.Fatalf("speaker_metadata count: %d", len(c.SpeakerMetadata))
	}
	m := c.MetadataFor("uuid-1")
	if m == nil || m.Credit != "AivisSpeech:Anneli" || !m.CommercialUse {
		t.Errorf("MetadataFor(uuid-1): %+v", m)
	}
	if c.MetadataFor("nope") != nil {
		t.Errorf("MetadataFor(nope) should be nil")
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	path := writeConfig(t, `
[engine]
mode = "external"
prot = 10101
`)
	if _, err := config.Load(path); err == nil || !strings.Contains(err.Error(), "unknown config keys") {
		t.Errorf("expected unknown-key error, got %v", err)
	}
}

func TestLoadRejectsBadMode(t *testing.T) {
	path := writeConfig(t, `
[engine]
mode = "auto"
`)
	if _, err := config.Load(path); err == nil || !strings.Contains(err.Error(), "engine.mode") {
		t.Errorf("expected mode error, got %v", err)
	}
}

func TestLoadRejectsBadVolume(t *testing.T) {
	path := writeConfig(t, `
[synthesis]
default_volume = 0.0
`)
	if _, err := config.Load(path); err == nil || !strings.Contains(err.Error(), "default_volume") {
		t.Errorf("expected volume error, got %v", err)
	}
}

func TestLoadRejectsBadConcurrency(t *testing.T) {
	path := writeConfig(t, `
[synthesis]
concurrency = 0
`)
	if _, err := config.Load(path); err == nil || !strings.Contains(err.Error(), "concurrency") {
		t.Errorf("expected concurrency error, got %v", err)
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	cases := map[string]string{
		"":          "",
		"~":         home,
		"~/x/y":     filepath.Join(home, "x/y"),
		"/abs/path": "/abs/path",
		"rel/path":  "rel/path",
	}
	for in, want := range cases {
		if got := config.ExpandHome(in); got != want {
			t.Errorf("ExpandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A key this server removed must be answered by name: the operator set it
// deliberately, and "unknown config keys" reads like a typo. ADR-0013 removed
// the default root, but the key kept decoding into a field nothing used, so a
// config carrying it loaded and quietly meant nothing.
func TestRemovedWorkspaceDirIsRejectedByName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[workspace]\nworkspace_dir = \"~/.voice-studio\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("a config carrying workspace_dir must fail to load")
	}
	for _, want := range []string{"workspace_dir", "work_dir", "ADR-0013"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
