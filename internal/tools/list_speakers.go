package tools

import (
	"context"
	"encoding/json"

	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
)

// LicenseInfo is the usage-terms block attached to each speaker. It comes
// from the hand-maintained [[speaker_metadata]] config section; the engine
// API itself does not expose model terms, so anything not registered there
// is reported as "unverified" and must be checked by a human before
// publishing audio that uses the voice.
type LicenseInfo struct {
	Status        string `json:"status"` // "verified" | "unverified"
	Name          string `json:"name,omitempty"`
	URL           string `json:"url,omitempty"`
	Credit        string `json:"credit,omitempty"`
	CommercialUse bool   `json:"commercial_use,omitempty"`
	Notes         string `json:"notes,omitempty"`
}

type speakerOut struct {
	Name        string         `json:"name"`
	SpeakerUUID string         `json:"speaker_uuid"`
	Styles      []engine.Style `json:"styles"`
	License     LicenseInfo    `json:"license"`
}

type listSpeakersOut struct {
	Engine   map[string]string `json:"engine"`
	Speakers []speakerOut      `json:"speakers"`
}

func registerListSpeakers(srv *mcpserver.Server, d *Deps) {
	srv.RegisterTool(mcpserver.Tool{
		Name: "list_speakers",
		Description: "List all installed voice models (speakers) with their styles and usage-terms metadata. " +
			"Use the returned style ids in the casting table. Speakers with license.status=\"unverified\" " +
			"need human license review before publishing audio that uses them.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var in struct{}
		if err := unmarshalStrict(args, &in); err != nil {
			return nil, err
		}
		version, err := d.Client.Version(ctx)
		if err != nil {
			return nil, err
		}
		speakers, err := d.Client.Speakers(ctx)
		if err != nil {
			return nil, err
		}
		out := listSpeakersOut{
			Engine: map[string]string{
				"version": version,
				"url":     d.Client.BaseURL(),
			},
			Speakers: make([]speakerOut, 0, len(speakers)),
		}
		for _, sp := range speakers {
			lic := LicenseInfo{Status: "unverified"}
			if m := d.Cfg.MetadataFor(sp.SpeakerUUID); m != nil {
				lic = LicenseInfo{
					Status:        "verified",
					Name:          m.License,
					URL:           m.LicenseURL,
					Credit:        m.Credit,
					CommercialUse: m.CommercialUse,
					Notes:         m.Notes,
				}
			}
			out.Speakers = append(out.Speakers, speakerOut{
				Name:        sp.Name,
				SpeakerUUID: sp.SpeakerUUID,
				Styles:      sp.Styles,
				License:     lic,
			})
		}
		return out, nil
	})
}
