// SPDX-License-Identifier: GPL-3.0-or-later
package rum_test

import (
	"context"
	"testing"

	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitRejectsURLsWithoutHostname(t *testing.T) {
	db, err := demjournal.Open(context.Background(), "")
	require.NoError(t, err)
	defer db.Close()
	for _, field := range []string{"public_url", "allowed_origins"} {
		t.Run(field, func(t *testing.T) {
			c := rum.New(rum.Dependencies{
				Registry: rumregistry.New(),
				History:  rumhistory.NewStore(db),
			})
			c.Name = "shop"
			c.AllowedOrigins = []string{"https://example.org"}
			if field == "public_url" {
				c.PublicURL = "http://:19938"
			} else {
				c.AllowedOrigins = []string{"https://:443"}
			}
			require.Error(t, c.Init(context.Background()))
		})
	}
}

func TestCancelledPlaintextPreparationAndRun(t *testing.T) {
	db, err := demjournal.Open(context.Background(), "")
	require.NoError(t, err)
	defer db.Close()
	c := rum.New(rum.Dependencies{
		Registry: rumregistry.New(),
		History:  rumhistory.NewStore(db),
	})
	c.Name = "shop"
	c.AllowedOrigins = []string{"https://example.org"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, c.Init(ctx), context.Canceled)
	require.NoError(t, c.Init(context.Background()))
	ready := false
	require.ErrorIs(t, c.Run(ctx, func() { ready = true }), context.Canceled)
	assert.False(t, ready)
}
