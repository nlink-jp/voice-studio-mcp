package script

import (
	"fmt"
	"sort"

	"github.com/BurntSushi/toml"

	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

// CastEntry maps one character to an engine voice.
type CastEntry struct {
	// SpeakerUUID identifies the voice model (from list_speakers). It is part
	// of the synthesis cache key, so recasting a character invalidates the
	// character's cached lines.
	SpeakerUUID string `toml:"speaker_uuid"`
	// StyleID is the default (global) style id used when a line has no style.
	StyleID int `toml:"style_id"`
	// Styles maps a script line's style name to a global style id.
	Styles map[string]int `toml:"styles"`
	// Credit is the attribution line required by the model's terms.
	Credit string `toml:"credit"`
	// LicenseChecked records that a human verified the model's terms allow
	// this production. master reports characters with false here.
	LicenseChecked bool `toml:"license_checked"`
}

// Casting is the casting table (casting.toml in the workspace root).
type Casting struct {
	Characters map[string]CastEntry `toml:"characters"`
}

// LoadCasting reads a casting table. Unknown keys are an error.
func LoadCasting(path string) (*Casting, error) {
	var c Casting
	meta, err := toml.DecodeFile(path, &c)
	if err != nil {
		return nil, toolerr.Newf(toolerr.CodeCastingUnresolved,
			"load casting table %s: %v", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return nil, toolerr.Newf(toolerr.CodeCastingUnresolved,
			"casting table %s has unknown keys: %v", path, undecoded)
	}
	for name, e := range c.Characters {
		if e.SpeakerUUID == "" {
			return nil, toolerr.Newf(toolerr.CodeCastingUnresolved,
				"casting entry %q is missing speaker_uuid", name)
		}
	}
	return &c, nil
}

// Resolve returns the style id and cast entry for a line.
func (c *Casting) Resolve(speaker, style string) (int, CastEntry, error) {
	entry, ok := c.Characters[speaker]
	if !ok {
		return 0, CastEntry{}, toolerr.Newf(toolerr.CodeCastingUnresolved,
			"speaker %q is not in the casting table", speaker)
	}
	if style == "" {
		return entry.StyleID, entry, nil
	}
	id, ok := entry.Styles[style]
	if !ok {
		return 0, CastEntry{}, toolerr.Newf(toolerr.CodeCastingUnresolved,
			"speaker %q has no style %q in the casting table", speaker, style)
	}
	return id, entry, nil
}

// Validate checks that every (speaker, style) used in lines resolves. All
// problems are collected into one casting_unresolved error so the agent can
// fix the casting table in a single pass.
func (c *Casting) Validate(lines []Line) error {
	unmappedSpeakers := map[string]bool{}
	type ss struct{ speaker, style string }
	unmappedStyles := map[ss]bool{}

	for _, ln := range lines {
		entry, ok := c.Characters[ln.Speaker]
		if !ok {
			unmappedSpeakers[ln.Speaker] = true
			continue
		}
		if ln.Style != "" {
			if _, ok := entry.Styles[ln.Style]; !ok {
				unmappedStyles[ss{ln.Speaker, ln.Style}] = true
			}
		}
	}
	if len(unmappedSpeakers) == 0 && len(unmappedStyles) == 0 {
		return nil
	}

	speakers := make([]string, 0, len(unmappedSpeakers))
	for s := range unmappedSpeakers {
		speakers = append(speakers, s)
	}
	sort.Strings(speakers)

	styles := make([]map[string]string, 0, len(unmappedStyles))
	for k := range unmappedStyles {
		styles = append(styles, map[string]string{"speaker": k.speaker, "style": k.style})
	}
	sort.Slice(styles, func(i, j int) bool {
		return fmt.Sprint(styles[i]) < fmt.Sprint(styles[j])
	})

	return toolerr.Newf(toolerr.CodeCastingUnresolved,
		"casting table cannot resolve %d speaker(s) and %d style reference(s)",
		len(speakers), len(styles)).
		WithDetails(map[string]any{
			"unmapped_speakers": speakers,
			"unmapped_styles":   styles,
		})
}
