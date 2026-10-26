// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_CollectIPSec(t *testing.T) {
	const activeKey = "ipsec_tunnels_active"
	lifetimeKey := func(name, remote, tid, encryption string) string {
		return metricKey("ipsec_tunnel_sa_lifetime", ipsecLabels(name, "gw-a", remote, tid, "ESP", encryption))
	}
	entry := func(name, remote, tid, encryption, remain, spi string) string {
		return fmt.Sprintf(
			`<entry><name>%s</name><gateway>gw-a</gateway><remote>%s</remote><tid>%s</tid><proto>ESP</proto><enc>%s</enc><remain>%s</remain><i_spi>%s</i_spi></entry>`,
			name,
			remote,
			tid,
			encryption,
			remain,
			spi,
		)
	}
	withEntries := func(rows, ntun string) []byte {
		return []byte(
			`<response status="success"><result><entries>` + rows + `</entries><ntun>` + ntun + `</ntun></result></response>`,
		)
	}
	oldSA := entry("branch-a", "198.51.100.10", "66", "G256", "60", "1")
	newSA := entry("branch-a", "198.51.100.10", "66", "G256", "7200", "2")
	badSA := entry("branch-a", "198.51.100.10", "66", "G256", "soon", "3")
	oldWithoutID := entry("branch-a", "198.51.100.10", "", "G256", "60", "1")
	newWithoutID := entry("branch-a", "198.51.100.10", "", "G256", "7200", "2")
	dottedName := entry("branch.a", "198.51.100.10", "66", "G256", "60", "1")
	underscoredName := entry("branch_a", "198.51.100.10", "67", "G256", "90", "2")
	knownLifetime := lifetimeKey("branch-a", "198.51.100.10", "66", "G256")

	tests := map[string]struct {
		body          []byte
		previousBody  []byte
		want          map[string]metrix.SampleValue
		wantLog       string
		wantLifetimes []int64
		mixedCrypto   bool
	}{
		"captured configured count differs from established count": {
			body: dataIPSecSAPartial,
			want: map[string]metrix.SampleValue{
				activeKey: 2,
				metricKey("ipsec_tunnel_sa_lifetime", ipsecLabels("branch-d", "gw-branch-d", "198.51.100.40", "3", "ESP", "G256")): 5145,
				metricKey("ipsec_tunnel_sa_lifetime", ipsecLabels("branch-c", "gw-branch-c", "198.51.100.30", "4", "ESP", "G256")): 4743,
			},
			wantLifetimes: []int64{4743, 5145},
		},
		"explicit empty entries": {
			body:         []byte(`<response status="success"><result><entries/><ntun>6</ntun></result></response>`),
			previousBody: withEntries(oldSA, "1"),
			want:         map[string]metrix.SampleValue{activeKey: 0},
		},
		"entries without configured count": {
			body: []byte(
				`<response status="success"><result><entries>` + oldSA + `</entries></result></response>`,
			),
			want:          map[string]metrix.SampleValue{activeKey: 1, knownLifetime: 60},
			wantLifetimes: []int64{60},
		},
		"malformed configured count is ignored": {
			body:          withEntries(oldSA, "six"),
			want:          map[string]metrix.SampleValue{activeKey: 1, knownLifetime: 60},
			wantLifetimes: []int64{60},
		},
		"configured count alone is not telemetry": {
			body:         []byte(`<response status="success"><result><ntun>6</ntun></result></response>`),
			previousBody: withEntries(oldSA, "1"),
			wantLog:      "expected <entries>",
		},
		"zero configured count alone is not telemetry": {
			body:    []byte(`<response status="success"><result><ntun>0</ntun></result></response>`),
			wantLog: "expected <entries>",
		},
		"malformed configured count alone is not telemetry": {
			body:    []byte(`<response status="success"><result><ntun>six</ntun></result></response>`),
			wantLog: "expected <entries>",
		},
		"missing payload": {
			body:    []byte(`<response status="success"><result/></response>`),
			wantLog: "expected <entries>",
		},
		"rekey old then new": {
			body:          withEntries(oldSA+newSA, "1"),
			want:          map[string]metrix.SampleValue{activeKey: 1, knownLifetime: 7200},
			wantLifetimes: []int64{7200},
		},
		"rekey new then old": {
			body:          withEntries(newSA+oldSA, "1"),
			want:          map[string]metrix.SampleValue{activeKey: 1, knownLifetime: 7200},
			wantLifetimes: []int64{7200},
		},
		"rekey padded fields": {
			body:          withEntries(newSA+entry(" branch-a ", " 198.51.100.10 ", " 66 ", " G256 ", "60", "1"), "1"),
			want:          map[string]metrix.SampleValue{activeKey: 1, knownLifetime: 7200},
			wantLifetimes: []int64{7200},
		},
		"rekey without tunnel ID": {
			body: withEntries(newWithoutID+oldWithoutID, "1"),
			want: map[string]metrix.SampleValue{
				activeKey: 1,
				lifetimeKey("branch-a", "198.51.100.10", "unknown", "G256"): 7200,
			},
			wantLifetimes: []int64{7200},
		},
		"rekey changes encryption": {
			body: withEntries(newSA+entry("branch-a", "198.51.100.10", "66", "AES128", "60", "1"), "1"),
			want: map[string]metrix.SampleValue{
				activeKey:     1,
				knownLifetime: 7200,
				lifetimeKey("branch-a", "198.51.100.10", "66", "AES128"): 60,
			},
			wantLifetimes: []int64{7200},
			mixedCrypto:   true,
		},
		"invalid lifetime preserves active count": {
			body:         withEntries(badSA, "1"),
			previousBody: withEntries(newSA, "1"),
			want:         map[string]metrix.SampleValue{activeKey: 1},
			wantLog:      "IPsec tunnel branch-a remain: invalid integer",
		},
		"invalid then valid lifetime": {
			body:          withEntries(badSA+newSA, "1"),
			want:          map[string]metrix.SampleValue{activeKey: 1, knownLifetime: 7200},
			wantLog:       "IPsec tunnel branch-a remain: invalid integer",
			wantLifetimes: []int64{7200},
		},
		"valid then invalid lifetime": {
			body:          withEntries(newSA+badSA, "1"),
			want:          map[string]metrix.SampleValue{activeKey: 1, knownLifetime: 7200},
			wantLog:       "IPsec tunnel branch-a remain: invalid integer",
			wantLifetimes: []int64{7200},
		},
		"zero lifetime is observed": {
			body:          withEntries(entry("branch-a", "198.51.100.10", "66", "G256", "0", "1"), "1"),
			want:          map[string]metrix.SampleValue{activeKey: 1, knownLifetime: 0},
			wantLifetimes: []int64{0},
		},
		"punctuation preserves distinct tunnels": {
			body: withEntries(dottedName+underscoredName, "2"),
			want: map[string]metrix.SampleValue{
				activeKey: 2,
				lifetimeKey("branch.a", "198.51.100.10", "66", "G256"): 60,
				lifetimeKey("branch_a", "198.51.100.10", "67", "G256"): 90,
			},
			wantLifetimes: []int64{60, 90},
		},
		"remote peers retain separate lifetime charts": {
			body: withEntries(oldSA+entry("branch-a", "198.51.100.20", "66", "G256", "7200", "2"), "1"),
			want: map[string]metrix.SampleValue{
				activeKey:     1,
				knownLifetime: 60,
				lifetimeKey("branch-a", "198.51.100.20", "66", "G256"): 7200,
			},
			wantLifetimes: []int64{60, 7200},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			var logs bytes.Buffer
			c.Logger = logger.NewWithWriter(&logs)
			api := &mockAPIClient{
				responses: map[string][]byte{ipsecSACommand: tc.previousBody},
			}
			c.apiClient = api
			if tc.previousBody != nil {
				_, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				logs.Reset()
			}
			api.responses[ipsecSACommand] = tc.body
			mx, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			got := map[string]metrix.SampleValue{}
			for key, value := range mx {
				if strings.HasPrefix(key, "ipsec_") {
					got[key] = value
				}
			}
			if tc.want == nil {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.want, got)
			}
			if tc.wantLog != "" {
				assert.Contains(t, logs.String(), tc.wantLog)
			} else {
				assert.NotContains(t, logs.String(), "IPsec")
			}
			// Other metricsets still publish when the IPsec response is incomplete.
			assert.Contains(t, mx, metricKey("system_uptime", systemLabels()))

			source, err := collectorapi.NewChartTemplateSource(c)
			require.NoError(t, err)
			set, err := source.Capture()
			require.NoError(t, err)
			engine, err := chartengine.New(chartengine.WithRuntimeStore(nil))
			require.NoError(t, err)
			attempt, err := engine.PreparePlanWithOptions(
				c.store.Read(metrix.ReadFlatten()),
				chartengine.PlanOptions{
					TemplateSet: set,
				},
			)
			require.NoError(t, err)
			defer attempt.Abort()
			lifetimes := make(map[string]struct{})
			for _, action := range attempt.Plan().Actions {
				if chart, ok := action.(chartengine.CreateChartAction); ok &&
					chart.Meta.Context == "panos.ipsec.tunnel.sa_lifetime" {
					lifetimes[chart.ChartID] = struct{}{}
					if tc.mixedCrypto {
						assert.NotContains(t, chart.Labels, "encryption")
						assert.Equal(t, "ESP", chart.Labels["protocol"])
					}
				}
			}
			var values []int64
			for _, action := range attempt.Plan().Actions {
				if update, ok := action.(chartengine.UpdateChartAction); ok {
					if _, ok := lifetimes[update.ChartID]; ok {
						require.Len(t, update.Values, 1)
						values = append(values, update.Values[0].Int64)
					}
				}
			}
			sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
			assert.Equal(t, tc.wantLifetimes, values)
		})
	}
}

func TestCountIPSecTunnels_Punctuation(t *testing.T) {
	// Summary identities must remain distinct even when their sanitized forms match.
	tunnels := []ipsecTunnel{
		{Name: "branch.a", Gateway: "gw-a", TID: "66"},
		{Name: "branch_a", Gateway: "gw-a", TID: "66"},
	}
	assert.Equal(t, int64(2), countIPSecTunnels(tunnels))
}
