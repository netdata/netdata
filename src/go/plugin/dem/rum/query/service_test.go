// SPDX-License-Identifier: GPL-3.0-or-later
package query_test

import (
	"context"
	"testing"
	"time"

	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/httpapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type measurementProcessor struct{ *aggregate.Aggregator }

func (p measurementProcessor) Ingest(b *beacon.Beacon) { p.Aggregator.Ingest(b) }

func addSite(t *testing.T, hub *rumregistry.Registry, key, generation string) (*aggregate.Aggregator, func()) {
	t.Helper()
	a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
		Investigate: aggregate.InvestigateCfg{
			Rate: 1,
		},
		Name:       key,
		PageGroups: 20,
		Countries:  20,
	})
	cfg := config.Site{
		Name:           key,
		DisplayName:    "site opaque-secret",
		AllowedOrigins: []string{"https://example.org"},
	}
	route := httpapi.NewRoute(cfg, measurementProcessor{a}, redact.NewRedactor("opaque-secret"))
	retire, err := hub.Register(
		key,
		&rumregistry.Site{
			Route:      route,
			Aggregator: a,
			Generation: generation,
		},
	)
	require.NoError(t, err)
	t.Cleanup(retire)
	return a, retire
}

func page(a *aggregate.Aggregator, site, path string) {
	a.Ingest(&beacon.Beacon{
		Site:         site,
		ExperienceID: path,
		Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: path, Revision: 1}},
		SessionID:    path,
		PageGroup:    path,
	})
}

func TestSourceRedactsCopiesAndRejectsCancelledReads(t *testing.T) {
	hub := rumregistry.New()
	s := query.New(hub, nil)
	a, _ := addSite(t, hub, "site", "generation")
	page(a, "site", "/opaque-secret")
	rows, _, err := s.Live(context.Background(), "", "", 10, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "/[REDACTED]", rows[0].Page)
	original, _ := a.Live(0, 10, "")
	assert.Equal(t, "/opaque-secret", original[0].Page)
	sites, err := s.Sites(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, sites, 1)
	assert.Equal(t, "site [REDACTED]", sites[0].Label)
	assert.Equal(t, query.Capture{
		Geolocation: "country",
	}, sites[0].Capture)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Pages(ctx, "", "")
	assert.ErrorIs(t, err, context.Canceled)
	_, _, err = s.Live(context.Background(), "", "not-a-cursor", 10, "")
	assert.Error(t, err)
}
