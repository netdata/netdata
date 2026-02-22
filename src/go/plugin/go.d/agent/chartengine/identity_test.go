// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"
)

func TestRenderChartInstanceIDScenarios(t *testing.T) {
	tests := map[string]struct {
		identity program.ChartIdentity
		labels   map[string]string
		wantID   string
		wantOK   bool
		wantErr  bool
	}{
		"static id without instances": {
			identity: program.ChartIdentity{
				IDTemplate: program.Template{
					Raw: "mysql_queries",
				},
			},
			labels: map[string]string{
				"host": "db1",
			},
			wantID: "mysql_queries",
			wantOK: true,
		},
		"literal id is independent of labels": {
			identity: program.ChartIdentity{
				IDTemplate: program.Template{
					Raw: "win_nic_traffic",
				},
			},
			labels: map[string]string{
				"nic": "eth0",
			},
			wantID: "win_nic_traffic",
			wantOK: true,
		},
		"instances explicit appends stable suffix": {
			identity: program.ChartIdentity{
				IDTemplate: program.Template{
					Raw: "win_nic_traffic",
				},
				InstanceByLabels: []program.InstanceLabelSelector{
					{Key: "nic"},
				},
			},
			labels: map[string]string{
				"nic": "eth0",
			},
			wantID: "win_nic_traffic_eth0",
			wantOK: true,
		},
		"instances wildcard excludes key": {
			identity: program.ChartIdentity{
				IDTemplate: program.Template{
					Raw: "latency_bucket",
				},
				InstanceByLabels: []program.InstanceLabelSelector{
					{IncludeAll: true},
					{Exclude: true, Key: "le"},
				},
			},
			labels: map[string]string{
				"service": "api",
				"zone":    "a",
				"le":      "1",
			},
			wantID: "latency_bucket_api_a",
			wantOK: true,
		},
		"instances explicit missing key drops series": {
			identity: program.ChartIdentity{
				IDTemplate: program.Template{
					Raw: "win_nic_traffic",
				},
				InstanceByLabels: []program.InstanceLabelSelector{
					{Key: "nic"},
				},
			},
			labels: map[string]string{
				"device": "eth0",
			},
			wantOK: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok, err := renderChartInstanceID(tc.identity, tc.labels)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantID, got)
		})
	}
}
