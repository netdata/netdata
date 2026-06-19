// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/snmptopologyfunc"
	"github.com/stretchr/testify/require"
)

func TestFuncDepsAdapterSnapshotUnavailable(t *testing.T) {
	tests := map[string]struct {
		adapter funcDepsAdapter
	}{
		"nil registry": {
			adapter: funcDepsAdapter{},
		},
		"empty registry": {
			adapter: funcDepsAdapter{registry: newTopologyRegistry()},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data, ok, err := tc.adapter.Snapshot(snmptopologyfunc.QueryOptions{})

			require.NoError(t, err)
			require.False(t, ok)
			require.Equal(t, topologyv1.Data{}, data)
		})
	}
}

func TestFuncDepsAdapterManagedDeviceFocusTargetsNilRegistry(t *testing.T) {
	require.Nil(t, funcDepsAdapter{}.ManagedDeviceFocusTargets())
}
