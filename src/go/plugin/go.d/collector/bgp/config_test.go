// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_InitDerivesZebraSocketPathFromSocketPath(t *testing.T) {
	collr := New()
	collr.Backend = " FRR "
	collr.SocketPath = "/run/frr-test/bgpd.vty "
	collr.newClient = func(cfg Config) (bgpClient, error) {
		assert.Equal(t, backendFRR, cfg.Backend)
		assert.Equal(t, "/run/frr-test/bgpd.vty", cfg.SocketPath)
		assert.Equal(t, "/run/frr-test/zebra.vty", cfg.ZebraSocketPath)
		return &mockClient{}, nil
	}

	require.NoError(t, collr.Init(context.Background()))
	assert.Equal(t, backendFRR, collr.Backend)
	assert.Equal(t, "/run/frr-test/bgpd.vty", collr.SocketPath)
	assert.Equal(t, "/run/frr-test/zebra.vty", collr.ZebraSocketPath)
}

func TestCollector_InitKeepsExplicitZebraSocketPath(t *testing.T) {
	collr := New()
	collr.SocketPath = "/run/frr-test/bgpd.vty"
	collr.ZebraSocketPath = "/srv/frr-custom/zebra.vty"
	collr.newClient = func(cfg Config) (bgpClient, error) {
		assert.Equal(t, "/srv/frr-custom/zebra.vty", cfg.ZebraSocketPath)
		return &mockClient{}, nil
	}

	require.NoError(t, collr.Init(context.Background()))
	assert.Equal(t, "/srv/frr-custom/zebra.vty", collr.ZebraSocketPath)
}

func TestCollector_InitUsesBirdDefaults(t *testing.T) {
	collr := New()
	collr.Backend = " bird "
	collr.newClient = func(cfg Config) (bgpClient, error) {
		assert.Equal(t, backendBIRD, cfg.Backend)
		assert.Equal(t, defaultSocketPathBIRD, cfg.SocketPath)
		assert.Equal(t, "", cfg.ZebraSocketPath)
		return &mockClient{}, nil
	}

	require.NoError(t, collr.Init(context.Background()))
	assert.Equal(t, backendBIRD, collr.Backend)
	assert.Equal(t, defaultSocketPathBIRD, collr.SocketPath)
	assert.Equal(t, "", collr.ZebraSocketPath)
}

func TestCollector_InitRequiresOpenBGPDAPIURL(t *testing.T) {
	collr := New()
	collr.Backend = backendOpenBGPD
	collr.SocketPath = ""
	collr.newClient = func(cfg Config) (bgpClient, error) { return &mockClient{}, nil }

	require.Error(t, collr.Init(context.Background()))
}

func TestCollector_InitNormalizesOpenBGPDAPIURL(t *testing.T) {
	collr := New()
	collr.Backend = " OPENBGPD "
	collr.APIURL = " http://127.0.0.1:8080/ "
	collr.newClient = func(cfg Config) (bgpClient, error) {
		assert.Equal(t, backendOpenBGPD, cfg.Backend)
		assert.Equal(t, "http://127.0.0.1:8080", cfg.APIURL)
		assert.Equal(t, "", cfg.SocketPath)
		assert.Equal(t, "", cfg.ZebraSocketPath)
		return &mockClient{}, nil
	}

	require.NoError(t, collr.Init(context.Background()))
	assert.Equal(t, backendOpenBGPD, collr.Backend)
	assert.Equal(t, "http://127.0.0.1:8080", collr.APIURL)
	assert.Equal(t, "", collr.SocketPath)
	assert.Equal(t, "", collr.ZebraSocketPath)
}

func TestCollector_InitAllowsZeroRIBSummaryEveryWhenDisabled(t *testing.T) {
	collr := New()
	collr.Backend = backendGoBGP
	collr.CollectRIBSummaries = false
	collr.RIBSummaryEvery = 0
	collr.newClient = func(cfg Config) (bgpClient, error) { return &mockClient{}, nil }

	require.NoError(t, collr.Init(context.Background()))
	assert.Equal(t, confopt.Duration(0), collr.RIBSummaryEvery)
}

func TestCollector_InitRequiresPositiveRIBSummaryEveryWhenEnabled(t *testing.T) {
	collr := New()
	collr.Backend = backendGoBGP
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = 0
	collr.newClient = func(cfg Config) (bgpClient, error) { return &mockClient{}, nil }

	require.Error(t, collr.Init(context.Background()))
}

func TestCollector_InitAllowsUnlimitedDeepQueryBudget(t *testing.T) {
	collr := New()
	collr.MaxDeepQueriesPerScrape = 0
	collr.newClient = func(cfg Config) (bgpClient, error) { return &mockClient{}, nil }

	require.NoError(t, collr.Init(context.Background()))
	assert.Zero(t, collr.MaxDeepQueriesPerScrape)
}

func TestCollector_InitRejectsNegativeDeepQueryBudget(t *testing.T) {
	collr := New()
	collr.MaxDeepQueriesPerScrape = -1
	collr.newClient = func(cfg Config) (bgpClient, error) { return &mockClient{}, nil }

	require.Error(t, collr.Init(context.Background()))
}

func TestCollector_InitIgnoresRIBSummaryEveryForFRR(t *testing.T) {
	collr := New()
	collr.Backend = backendFRR
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = confopt.Duration(time.Duration(0))
	collr.newClient = func(cfg Config) (bgpClient, error) { return &mockClient{}, nil }

	require.NoError(t, collr.Init(context.Background()))
}
