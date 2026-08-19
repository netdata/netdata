// SPDX-License-Identifier: GPL-3.0-or-later
package vsphere

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/vim25/types"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func TestWriteDatastoreMetrics(t *testing.T) {
	yes := true

	tests := map[string]struct {
		ds   rs.Datastore
		want map[string]int64
	}{
		"accessible datastore": {
			ds: rs.Datastore{
				ID:                 "datastore-1",
				OverallStatus:      "red",
				Capacity:           1000,
				FreeSpace:          400,
				Uncommitted:        250,
				Accessible:         true,
				MaintenanceMode:    string(types.DatastoreSummaryMaintenanceModeStateNormal),
				MultipleHostAccess: &yes,
			},
			want: map[string]int64{
				"datastore_space_usage_capacity":                    1000,
				"datastore_space_usage_free":                        400,
				"datastore_space_usage_used":                        600,
				"datastore_space_utilization_used":                  6000,
				"datastore_space_usage_uncommitted":                 250,
				"datastore_overall_status_green":                    0,
				"datastore_overall_status_red":                      1,
				"datastore_overall_status_yellow":                   0,
				"datastore_overall_status_gray":                     0,
				"datastore_accessibility_status_accessible":         1,
				"datastore_accessibility_status_inaccessible":       0,
				"datastore_maintenance_status_normal":               1,
				"datastore_maintenance_status_entering_maintenance": 0,
				"datastore_maintenance_status_in_maintenance":       0,
				"datastore_maintenance_status_unknown":              0,
				"datastore_multiple_host_access_enabled":            1,
				"datastore_multiple_host_access_disabled":           0,
				"datastore_multiple_host_access_unknown":            0,
			},
		},
		"inaccessible datastore": {
			ds: rs.Datastore{
				ID:            "datastore-2",
				OverallStatus: "gray",
				Capacity:      1000,
				FreeSpace:     400,
				Uncommitted:   250,
			},
			want: map[string]int64{
				"datastore_space_usage_capacity":                    0,
				"datastore_space_usage_free":                        0,
				"datastore_space_usage_used":                        0,
				"datastore_space_utilization_used":                  0,
				"datastore_space_usage_uncommitted":                 0,
				"datastore_overall_status_green":                    0,
				"datastore_overall_status_red":                      0,
				"datastore_overall_status_yellow":                   0,
				"datastore_overall_status_gray":                     1,
				"datastore_accessibility_status_accessible":         0,
				"datastore_accessibility_status_inaccessible":       1,
				"datastore_maintenance_status_normal":               0,
				"datastore_maintenance_status_entering_maintenance": 0,
				"datastore_maintenance_status_in_maintenance":       0,
				"datastore_maintenance_status_unknown":              1,
				"datastore_multiple_host_access_enabled":            0,
				"datastore_multiple_host_access_disabled":           0,
				"datastore_multiple_host_access_unknown":            1,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			series := runMetricWriteForTest(t, collr, func() { collr.writeDatastoreMetrics(&tc.ds) })
			require.Len(t, series, len(tc.want))
			for metric, want := range tc.want {
				requireScalarSeriesValue(t, series, metric, tc.ds.ID, want)
			}
		})
	}
}
