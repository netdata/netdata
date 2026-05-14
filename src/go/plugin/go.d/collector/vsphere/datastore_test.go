// SPDX-License-Identifier: GPL-3.0-or-later
package vsphere

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
				"datastore-1_capacity":                               1000,
				"datastore-1_free_space":                             400,
				"datastore-1_used_space":                             600,
				"datastore-1_used_space_pct":                         6000,
				"datastore-1_uncommitted":                            250,
				"datastore-1_overall.status.green":                   0,
				"datastore-1_overall.status.red":                     1,
				"datastore-1_overall.status.yellow":                  0,
				"datastore-1_overall.status.gray":                    0,
				"datastore-1_accessible_status.accessible":           1,
				"datastore-1_accessible_status.inaccessible":         0,
				"datastore-1_maintenance.status.normal":              1,
				"datastore-1_maintenance.status.enteringMaintenance": 0,
				"datastore-1_maintenance.status.inMaintenance":       0,
				"datastore-1_maintenance.status.unknown":             0,
				"datastore-1_multiple_host_access.enabled":           1,
				"datastore-1_multiple_host_access.disabled":          0,
				"datastore-1_multiple_host_access.unknown":           0,
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
				"datastore-2_capacity":                               0,
				"datastore-2_free_space":                             0,
				"datastore-2_used_space":                             0,
				"datastore-2_used_space_pct":                         0,
				"datastore-2_uncommitted":                            0,
				"datastore-2_overall.status.green":                   0,
				"datastore-2_overall.status.red":                     0,
				"datastore-2_overall.status.yellow":                  0,
				"datastore-2_overall.status.gray":                    1,
				"datastore-2_accessible_status.accessible":           0,
				"datastore-2_accessible_status.inaccessible":         1,
				"datastore-2_maintenance.status.normal":              0,
				"datastore-2_maintenance.status.enteringMaintenance": 0,
				"datastore-2_maintenance.status.inMaintenance":       0,
				"datastore-2_maintenance.status.unknown":             1,
				"datastore-2_multiple_host_access.enabled":           0,
				"datastore-2_multiple_host_access.disabled":          0,
				"datastore-2_multiple_host_access.unknown":           1,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mx := make(map[string]int64)
			writeDatastoreMetrics(mx, &tc.ds)
			assert.Equal(t, tc.want, mx)
		})
	}
}
