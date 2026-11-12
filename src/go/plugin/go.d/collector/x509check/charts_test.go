// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"bytes"
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_ChartContract(t *testing.T) {
	c := New()
	c.Source = "https://example.org:443"
	c.CheckFullChain, c.CheckRevocation = true, true
	c.provider = &mockProvider{
		certs: []*x509.Certificate{
			{Subject: pkix.Name{
				CommonName: "example.org",
			}, NotAfter: time.Now().Add(time.Hour)},
			{Subject: pkix.Name{
				CommonName: "issuer",
			}, NotAfter: time.Now().Add(24 * time.Hour)},
		},
	}
	c.revocationStatus = func(context.Context, *x509.Certificate, []*x509.Certificate, time.Time) (bool, error) {
		return true, nil
	}
	require.NoError(t, collectCycle(t, c))
	plan := preparePlan(t, newEngine(t, c), c)

	type chartContract struct {
		Title, Family, Units, Type string
		Labels                     map[string]string
		Dimensions                 []string
	}
	got := make(map[string]chartContract)
	keys := make(map[string]string)
	for _, action := range plan.Actions {
		switch a := action.(type) {
		case chartengine.CreateChartAction:
			key := a.Meta.Context + "/" + a.Labels["depth"]
			keys[a.ChartID] = key
			got[key] = chartContract{a.Meta.Title, a.Meta.Family, a.Meta.Units, string(a.Meta.Type), a.Labels, nil}
		case chartengine.CreateDimensionAction:
			key := keys[a.ChartID]
			v := got[key]
			v.Dimensions = append(v.Dimensions, a.Name)
			got[key] = v
			assert.Equal(t, "absolute", string(a.Algorithm))
			assert.Equal(t, 1, a.Multiplier)
			assert.Equal(t, 1, a.Divisor)
			assert.False(t, a.Hidden)
			assert.False(t, a.Float)
		}
	}
	// The revocation check status chart has no V1 counterpart; metadata and chart coverage tests cover it.
	require.Contains(t, got, "x509check.revocation_check_status/0")
	delete(got, "x509check.revocation_check_status/0")
	// V1 charts.go contract: IDs may change, but contexts, display names and measurement metadata must survive.
	assert.Equal(t, map[string]chartContract{
		"x509check.time_until_expiration/0": {
			"Time Until Certificate Expiration", "expiration time", "seconds", "line",
			map[string]string{"source": c.Source, "common_name": "example.org", "depth": "0"}, []string{"expiry"},
		},
		"x509check.time_until_expiration/1": {
			"Time Until Certificate Expiration", "expiration time", "seconds", "line",
			map[string]string{"source": c.Source, "common_name": "issuer", "depth": "1"}, []string{"expiry"},
		},
		"x509check.revocation_status/0": {
			"Revocation Status", "revocation", "boolean", "line",
			map[string]string{
				"source":      c.Source,
				"common_name": "example.org",
				"depth":       "0",
			}, []string{"not_revoked", "revoked"},
		},
	}, got)

	creator := collectorapi.DefaultRegistry["x509check"]
	require.NotNil(t, creator.CreateV2)
	assert.Nil(t, creator.Create)
	assert.Equal(t, 60, creator.UpdateEvery)
	require.True(t, creator.StoreFirst)
	var output bytes.Buffer
	require.NoError(t, chartemit.ApplyPlan(netdataapi.New(&output), plan, chartemit.EmitEnv{
		TypeID:      "x509check_test",
		UpdateEvery: 60,
		Plugin:      "go.d",
		Module:      "x509check",
		StoreFirst:  creator.StoreFirst,
	}))
	assert.Contains(t, output.String(), "store_first")
	assert.Contains(t, output.String(), "DIMENSION 'expiry' 'expiry'")
}

func TestCollector_ChartLifecycle(t *testing.T) {
	c := New()
	c.Source = "https://example.org:443"
	c.CheckFullChain, c.CheckRevocation = true, true
	provider := &mockProvider{
		certs: []*x509.Certificate{
			{Subject: pkix.Name{
				CommonName: "shared",
			}, NotAfter: time.Now().Add(time.Hour)},
			{Subject: pkix.Name{
				CommonName: "shared",
			}, NotAfter: time.Now().Add(24 * time.Hour)},
		},
	}
	c.provider = provider
	determined := true
	c.revocationStatus = func(context.Context, *x509.Certificate, []*x509.Certificate, time.Time) (bool, error) {
		if !determined {
			return false, failCheck(checkRequestFailed, errors.New("undetermined"))
		}
		return false, nil
	}
	engine := newEngine(t, c)
	require.NoError(t, collectCycle(t, c))
	initial := preparePlan(t, engine, c)
	contexts := make(map[string]string)
	for _, a := range initial.Actions {
		if create, ok := a.(chartengine.CreateChartAction); ok {
			contexts[create.ChartID] = create.Meta.Context
		}
	}
	assert.Len(t, contexts, 4, "identical common names must not merge distinct depths")

	// Successful observations with a shorter, renewed chain and unavailable revocation must retain charts with gaps.
	chain := provider.certs
	renewed := *chain[0]
	renewed.Subject.CommonName = "renewed"
	provider.certs = []*x509.Certificate{&renewed}
	determined = false
	for cycle := 0; cycle < 20; cycle++ {
		require.NoError(t, collectCycle(t, c))
		plan := preparePlan(t, engine, c)
		var updated []string
		var labels int
		for _, action := range plan.Actions {
			switch a := action.(type) {
			case chartengine.CreateChartAction, chartengine.RemoveChartAction, chartengine.RemoveDimensionAction:
				t.Fatalf("unexpected chart lifecycle action at missing cycle %d: %#v", cycle, a)
			case chartengine.UpdateChartLabelsAction:
				labels++
				assert.Equal(t, "renewed", a.Labels["common_name"])
			case chartengine.UpdateChartAction:
				updated = append(updated, contexts[a.ChartID])
			}
		}
		// The revocation status of the server certificate is a gap; the check status says why.
		assert.ElementsMatch(t, []string{"x509check.time_until_expiration", "x509check.revocation_check_status"}, updated)
		if cycle == 0 {
			assert.Equal(t, 2, labels, "renewal updates metadata without changing identity")
		}
	}
	provider.certs, determined = chain, true
	require.NoError(t, collectCycle(t, c))
	var updates, labels int
	for _, action := range preparePlan(t, engine, c).Actions {
		switch a := action.(type) {
		case chartengine.CreateChartAction, chartengine.RemoveChartAction:
			t.Fatalf("recovery must reuse existing charts: %#v", a)
		case chartengine.UpdateChartLabelsAction:
			labels++
			assert.Equal(t, "shared", a.Labels["common_name"])
		case chartengine.UpdateChartAction:
			updates++
		}
	}
	assert.Equal(t, 4, updates)
	assert.Equal(t, 2, labels, "the restored chain updates the renewed charts' metadata back")
}

func newEngine(t *testing.T, c *Collector) *chartengine.Engine {
	t.Helper()
	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(c.ChartTemplateYAML()), 1))
	return engine
}

func preparePlan(t *testing.T, engine *chartengine.Engine, c *Collector) chartengine.Plan {
	t.Helper()
	attempt, err := engine.PreparePlan(c.MetricStore().Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())
	return plan
}
