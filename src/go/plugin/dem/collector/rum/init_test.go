// SPDX-License-Identifier: GPL-3.0-or-later
package rum_test

import (
	"context"
	"fmt"
	"math"
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

func TestInitSamplingRates(t *testing.T) {
	db, err := demjournal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, rate := range []float64{0, .25, 1, -.1, 1.1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, field := range []string{"measure", "investigate"} {
			t.Run(fmt.Sprintf("%s/%v", field, rate), func(t *testing.T) {
				c := rum.New(rum.Dependencies{
					Registry: rumregistry.New(),
					History:  rumhistory.NewStore(db),
				})
				c.Name = "shop"
				c.AllowedOrigins = []string{"https://shop.example.org"}
				if field == "measure" {
					c.MeasureSampleRate = &rate
				} else {
					c.Investigate.SampleRate = &rate
				}
				err := c.Init(context.Background())
				if !math.IsNaN(rate) && rate >= 0 && rate <= 1 {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "sample_rate must be between 0 and 1")
				}
			})
		}
	}
}
