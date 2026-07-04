package tools

import (
	"context"
	"encoding/json"

	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
)

// LicenseInfo is the usage-terms block attached to each speaker.
//
// Status semantics (ADR-0008):
//   - "verified":   a human reviewed the terms and recorded them in the
//     [[speaker_metadata]] config section — safe to rely on.
//   - "declared":   the model's AIVM manifest embeds author-declared license
//     text (name/credit below are extracted from it); a human still has to
//     read it before publishing. Use the `licenses` subcommand.
//   - "unverified": no config entry and no manifest declaration.
type LicenseInfo struct {
	Status        string `json:"status"` // "verified" | "declared" | "unverified"
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
			"Use the returned style ids in the casting table. license.status is \"verified\" (human-reviewed " +
			"config entry), \"declared\" (license text embedded in the model manifest — run the `licenses` " +
			"subcommand to review it), or \"unverified\". Only \"verified\" models should be used in published audio.",
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
		declared := declaredLicenses(ctx, d)
		out := listSpeakersOut{
			Engine: map[string]string{
				"version": version,
				"url":     d.Client.BaseURL(),
			},
			Speakers: make([]speakerOut, 0, len(speakers)),
		}
		for _, sp := range speakers {
			lic := LicenseInfo{Status: "unverified"}
			if decl, ok := declared[sp.SpeakerUUID]; ok {
				lic = decl
			}
			// A human-reviewed config entry always wins over a declaration.
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

// declaredLicenses joins AIVM manifests to speaker UUIDs. Best-effort: the
// endpoint is AivisSpeech-specific, so any failure just means no
// declarations (a VOICEVOX engine would land here, for instance).
func declaredLicenses(ctx context.Context, d *Deps) map[string]LicenseInfo {
	models, err := d.Client.AivmModels(ctx)
	if err != nil {
		d.Logger.Debug("aivm_models unavailable; skipping declared licenses", "err", err)
		return nil
	}
	out := make(map[string]LicenseInfo)
	for _, model := range models {
		for _, sp := range model.Speakers {
			name := engine.LicenseNameFromText(model.LicenseTextFor(sp))
			if name == "" {
				continue
			}
			uuid := sp.Speaker.SpeakerUUID
			out[uuid] = LicenseInfo{
				Status: "declared",
				Name:   name,
				Credit: model.Manifest.CreditLine(),
				Notes:  "declared in the model's AIVM manifest — review the full text (`voice-studio-mcp licenses --full " + uuid + "`) and record it in [[speaker_metadata]] before publishing",
			}
		}
	}
	return out
}
