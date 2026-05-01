// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

var vniChartsTmpl = Charts{
	{
		ID:       "vni_%s_entries",
		Title:    "BGP EVPN VNI entries",
		Units:    "entries",
		Fam:      "%s",
		Ctx:      "bgp.evpn_vni_entries",
		Type:     collectorapi.Stacked,
		Priority: prioVNIEntries,
		Dims: Dims{
			{ID: "vni_%s_macs", Name: "macs"},
			{ID: "vni_%s_arp_nd", Name: "arp_nd"},
		},
	},
	{
		ID:       "vni_%s_remote_vteps",
		Title:    "BGP EVPN VNI remote VTEPs",
		Units:    "VTEPs",
		Fam:      "%s",
		Ctx:      "bgp.evpn_vni_remote_vteps",
		Priority: prioVNIRemoteVTEPs,
		Dims: Dims{
			{ID: "vni_%s_remote_vteps", Name: "remote_vteps"},
		},
	},
}

func newVNICharts(vni vniStats) *Charts {
	charts := vniChartsTmpl.Copy()
	fam := vniDisplay(vni)
	labels := vniLabels(vni)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, vni.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, fam)
		chart.Labels = labels
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, vni.ID)
		}
	}

	return charts
}

func (c *Collector) addVNICharts(vni vniStats) {
	charts := newVNICharts(vni)
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeVNICharts(id string) {
	c.removeCharts("vni_" + id)
}

func vniLabels(vni vniStats) []collectorapi.Label {
	labels := []collectorapi.Label{
		{Key: "backend", Value: vni.Backend},
		{Key: "scope_kind", Value: "vrf"},
		{Key: "scope_name", Value: emptyToDefault(vni.TenantVRF)},
		{Key: "tenant_vrf", Value: vni.TenantVRF},
		{Key: "vni", Value: strconv.FormatInt(vni.VNI, 10)},
		{Key: "vni_type", Value: vni.Type},
	}
	if vni.VXLANIf != "" {
		labels = append(labels, collectorapi.Label{Key: "vxlan_interface", Value: vni.VXLANIf})
	}
	return labels
}
