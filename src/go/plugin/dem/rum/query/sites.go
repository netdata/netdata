// SPDX-License-Identifier: GPL-3.0-or-later
package query

import (
	"context"
	"strings"

	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
)

func (s *Service) Sites(ctx context.Context, vnodeGUID string) ([]Site, error) {
	var out []Site
	state := s.registry.Availability()
	err := s.visit(ctx, "", func(key string, site *rumregistry.Site) {
		activity := site.Aggregator.Activity()
		if vnodeGUID != "" && activity.VnodeGUID != vnodeGUID {
			return
		}
		cfg := site.Route.Config()
		publicBase := cfg.PublicURL
		if publicBase == "" {
			publicBase = state.PublicURL
		}
		scriptURL := ""
		if publicBase != "" {
			scriptURL = strings.TrimRight(publicBase, "/") + "/rum/" + cfg.Name + ".js"
		}
		rejected := site.Route.LastRejectedOrigin()
		redact := siteRedactor(site)
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
				VnodeGUID:      activity.VnodeGUID,
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
				Activity:          Activity(activity),
				Generation:        site.Generation,
				CollectionEnabled: cfg.MeasureRate() > 0,
				Rejected: Rejection{
					Origin: rejected.Origin,
					At:     rejected.At,
				},
				ScriptURL: redact.Apply(scriptURL),
			},
		)
	})
	return out, err
}
func (s *Service) Pages(ctx context.Context, filter, vnodeGUID string) ([]Page, error) {
	var out []Page
	err := s.visit(ctx, filter, func(key string, site *rumregistry.Site) {
		rows := site.Aggregator.Pages(vnodeGUID)
		if len(rows) == 0 {
			return
		}
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
