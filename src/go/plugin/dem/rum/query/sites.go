// SPDX-License-Identifier: GPL-3.0-or-later
package query

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/diagnostics"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
)

func (s *Service) Sites(ctx context.Context) ([]Site, error) {
	var out []Site
	state := s.registry.Availability()
	err := s.visit(ctx, "", func(key string, site *rumregistry.Site) {
		cfg := site.Route.Config()
		fallback := state.PublicURL
		if fallback == "" && state.Listen != "" {
			fallback = diagnostics.ListenerBase(state.Listen, state.TLS)
		}
		// Explicit receiver configuration takes effect immediately, even while
		// the site's last successful probe still describes an older proxy URL.
		publicBase := strings.TrimRight(state.PublicURL, "/")
		if cfg.PublicURL != "" || publicBase == "" {
			publicBase = site.Diagnostics.PublicBase(fallback)
		}
		reach, _ := site.Diagnostics.Reachability()
		snippet, _ := site.Diagnostics.Snippet()
		rejected, _ := site.Diagnostics.LastRejectedOrigin()
		if reach.State == "" {
			reach.State = diagnostics.ReachUnknown
		}
		if snippet.State == "" {
			snippet.State = diagnostics.SnippetUnchecked
		}
		redact := siteRedactor(site)
		reach.Error = redact.Apply(reach.Error)
		snippet.Detail = redact.Apply(snippet.Detail)
		rejected.Origin = redact.Apply(rejected.Origin)
		cfg.DisplayName = redact.Apply(cfg.DisplayName)
		cfg.AllowedOrigins = append([]string(nil), cfg.AllowedOrigins...)
		for i := range cfg.AllowedOrigins {
			cfg.AllowedOrigins[i] = redact.Apply(cfg.AllowedOrigins[i])
		}
		out = append(
			out,
			Site{
				Name:           cfg.Name,
				Label:          cfg.Label(),
				AllowedOrigins: cfg.AllowedOrigins,
				Sampling: Sampling{
					MeasureRate:     cfg.MeasureRate(),
					InvestigateRate: cfg.InvestigateRate(),
					KeepErrors:      cfg.KeepsErrors(),
					KeepPoorVitals:  cfg.KeepsPoorVitals(),
				},
				Capture: Capture{
					Geolocation:        cfg.GeolocationMode(),
					FrustrationSignals: cfg.FrustrationSignalsOn(),
				},
				Activity: Activity(site.Aggregator.Activity()),
				Reach: Diagnostic{
					State:  reach.State,
					Detail: reach.Error,
				},
				Snippet: Diagnostic{
					State:  snippet.State,
					Detail: snippet.Detail,
				},
				Rejected: Rejection{
					Origin: rejected.Origin,
					At:     rejected.At,
				},
				PublicBase: redact.Apply(publicBase),
			},
		)
	})
	return out, err
}
func (s *Service) Pages(ctx context.Context, filter string) ([]Page, error) {
	var out []Page
	err := s.visit(ctx, filter, func(key string, site *rumregistry.Site) {
		rows := site.Aggregator.Pages()
		redact := siteRedactor(site)
		for i := range rows {
			r := &rows[i]
			r.Page = redact.Apply(r.Page)
			r.LCPElement = redact.Apply(r.LCPElement)
			r.INPElement = redact.Apply(r.INPElement)
			r.CLSElement = redact.Apply(r.CLSElement)
		}
		for _, row := range rows {
			out = append(out, Page(row))
		}
	})
	return out, err
}
