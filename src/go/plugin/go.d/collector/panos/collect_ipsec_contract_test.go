// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"io"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This wire contract also passes at 8b34c5b90d, before the internal state instrument migration.
func TestCollector_IPSecStateChartContract(t *testing.T) {
	type dimension struct {
		Name       string
		Algorithm  string
		Multiplier int
		Divisor    int
		Hidden     bool
		Float      bool
	}
	type chart struct {
		Title      string
		Family     string
		Context    string
		Units      string
		Type       string
		Labels     map[string]string
		Dimensions []dimension
	}
	want := map[string]chart{}
	for _, tunnel := range [][2]string{
		{"branch-a", "1"}, {"branch-a-2", "2"}, {"branch-b", "5"},
		{"branch-b-2", "6"}, {"branch-d", "3"}, {"branch-c", "4"},
	} {
		want["ipsec_tunnel_state_"+tunnel[0]+"_"+tunnel[1]] = chart{
			Title:   "IPsec Tunnel State",
			Family:  "ipsec",
			Context: "panos.ipsec.tunnel.state",
			Units:   "state",
			Type:    "stacked",
			Labels:  map[string]string{"tunnel": tunnel[0], "tunnel_id": tunnel[1]},
			Dimensions: []dimension{
				{Name: "active", Algorithm: "absolute", Multiplier: 1, Divisor: 1},
				{Name: "inactive", Algorithm: "absolute", Multiplier: 1, Divisor: 1},
				{Name: "init", Algorithm: "absolute", Multiplier: 1, Divisor: 1},
				{Name: "unknown", Algorithm: "absolute", Multiplier: 1, Divisor: 1},
			},
		}
	}
	for _, gateway := range [][2]string{
		{"gw-branch-a", "ikev1"}, {"gw-branch-b", "ikev1"}, {"gw-branch-c", "ikev2"}, {"gw-branch-d", "ikev2"},
	} {
		want["ipsec_gateway_state_"+gateway[0]] = chart{
			Title:   "IPsec Gateway State",
			Family:  "ipsec",
			Context: "panos.ipsec.gateway.state",
			Units:   "state",
			Type:    "stacked",
			Labels:  map[string]string{"gateway": gateway[0], "ike_version": gateway[1]},
			Dimensions: []dimension{
				{Name: "down", Algorithm: "absolute", Multiplier: 1, Divisor: 1},
				{Name: "up", Algorithm: "absolute", Multiplier: 1, Divisor: 1},
			},
		}
	}
	c := New()
	c.Logger = logger.NewWithWriter(io.Discard)
	c.apiClient = &mockAPIClient{}
	_, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	engine, err := chartengine.New(chartengine.WithRuntimeStore(nil))
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(c.ChartTemplateYAML()), 1))
	attempt, err := engine.PreparePlan(c.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	got := map[string]chart{}
	for _, action := range attempt.Plan().Actions {
		switch a := action.(type) {
		case chartengine.CreateChartAction:
			if !strings.HasPrefix(a.ChartID, "ipsec_tunnel_state_") && !strings.HasPrefix(a.ChartID, "ipsec_gateway_state_") {
				continue
			}
			got[a.ChartID] = chart{
				Title:   a.Meta.Title,
				Family:  a.Meta.Family,
				Context: a.Meta.Context,
				Units:   a.Meta.Units,
				Type:    string(a.Meta.Type),
				Labels:  a.Labels,
			}
		case chartengine.CreateDimensionAction:
			if current, ok := got[a.ChartID]; ok {
				current.Dimensions = append(current.Dimensions, dimension{
					Name:       a.Name,
					Algorithm:  string(a.Algorithm),
					Multiplier: a.Multiplier,
					Divisor:    a.Divisor,
					Hidden:     a.Hidden,
					Float:      a.Float,
				})
				got[a.ChartID] = current
			}
		}
	}
	assert.Equal(t, want, got)
	require.NoError(t, attempt.Commit())
}
