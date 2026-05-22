// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"testing"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/stretchr/testify/require"
)

func firstSortedVM(t *testing.T, collr *Collector) *rs.VM {
	t.Helper()

	vms := sortedVMs(collr.resources.VMs)
	require.NotEmpty(t, vms)
	return vms[0]
}
