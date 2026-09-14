// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"sort"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/stretchr/testify/require"
)

func firstSortedHost(t *testing.T, collr *Collector) *rs.Host {
	t.Helper()

	hosts := make([]*rs.Host, 0, len(collr.resources.Hosts))
	for _, host := range collr.resources.Hosts {
		hosts = append(hosts, host)
	}
	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].ID < hosts[j].ID
	})
	require.NotEmpty(t, hosts)
	return hosts[0]
}

func firstSortedVM(t *testing.T, collr *Collector) *rs.VM {
	t.Helper()

	vms := sortedVMs(collr.resources.VMs)
	require.NotEmpty(t, vms)
	return vms[0]
}

func countMetricSeries(reader metrix.Reader, name string) (count int) {
	reader.ForEachByName(name, func(metrix.LabelView, metrix.SampleValue) {
		count++
	})
	return count
}

//go:fix inline
func boolPtr(value bool) *bool {
	return new(value)
}
