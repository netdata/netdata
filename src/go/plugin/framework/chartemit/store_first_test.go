// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyPlanStoreFirstOnEveryChartDefinition(t *testing.T) {
	meta := chartengine.ChartMeta{
		Title:   "Requests",
		Context: "requests",
	}
	cases := map[string]struct {
		action   EngineAction
		obsolete bool
	}{
		"creation": {action: CreateChartAction{
			ChartID: "requests",
			Meta:    meta,
		}},
		"labels": {
			action: UpdateChartLabelsAction{
				ChartID: "requests",
				Meta:    meta,
				Labels:  map[string]string{"service": "api"},
			},
		},
		"new dimension": {action: CreateDimensionAction{
			ChartID:   "requests",
			ChartMeta: meta,
			Name:      "new",
		}},
		"drop dimension": {action: RemoveDimensionAction{
			ChartID:   "requests",
			ChartMeta: meta,
			Name:      "old",
		}},
		"obsoletion": {action: RemoveChartAction{
			ChartID: "requests",
			Meta:    meta,
		}, obsolete: true},
	}
	for name, tc := range cases {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/enabled=%t", name, enabled), func(t *testing.T) {
				var out bytes.Buffer
				require.NoError(t, ApplyPlan(netdataapi.New(&out), Plan{
					Actions: []EngineAction{tc.action},
				}, EmitEnv{
					TypeID:      "collector.job",
					UpdateEvery: 1,
					StoreFirst:  enabled,
				}))
				var options []string
				if tc.obsolete {
					options = append(options, "obsolete")
				}
				if enabled {
					options = append(options, "store_first")
				}
				want := fmt.Sprintf(
					"CHART 'collector.job.requests' '' 'Requests' '' '' 'requests' '' '0' '1' '%s' '' ''",
					strings.Join(options, " "),
				)
				assert.Contains(t, out.String(), want+"\n")
				assert.Equal(t, 1, strings.Count(out.String(), "CHART "))
				if dimension := strings.Index(out.String(), "DIMENSION "); dimension >= 0 {
					assert.Less(t, strings.Index(out.String(), "CHART "), dimension)
				}
			})
		}
	}
}
