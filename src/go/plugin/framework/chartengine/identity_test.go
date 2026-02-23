// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	program2 "github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderChartInstanceIDScenarios(t *testing.T) {
	tests := map[string]struct {
		identity program2.ChartIdentity
		labels   map[string]string
		wantID   string
		wantOK   bool
		wantErr  bool
	}{
		"static id without instances": {
			identity: program2.ChartIdentity{
				IDTemplate: program2.Template{
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
			identity: program2.ChartIdentity{
				IDTemplate: program2.Template{
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
			identity: program2.ChartIdentity{
				IDTemplate: program2.Template{
					Raw: "win_nic_traffic",
				},
				InstanceByLabels: []program2.InstanceLabelSelector{
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
			identity: program2.ChartIdentity{
				IDTemplate: program2.Template{
					Raw: "latency_bucket",
				},
				InstanceByLabels: []program2.InstanceLabelSelector{
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
			identity: program2.ChartIdentity{
				IDTemplate: program2.Template{
					Raw: "win_nic_traffic",
				},
				InstanceByLabels: []program2.InstanceLabelSelector{
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
