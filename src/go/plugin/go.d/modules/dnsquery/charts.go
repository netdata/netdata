// SPDX-License-Identifier: GPL-3.0-or-later

package dnsquery

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioDNSQueryStatus = module.Priority + iota
	prioDNSQueryTime
)

var (
	dnsChartsTmpl = module.Charts{
		dnsQueryStatusChartTmpl.Copy(),
		dnsQueryTimeChartTmpl.Copy(),
	}
	dnsQueryStatusChartTmpl = module.Chart{
		ID:       "server_%s_record_%s_query_status",
		Title:    "DNS Query Status",
		Units:    "status",
		Fam:      "query status",
		Ctx:      "dns_query.query_status",
		Priority: prioDNSQueryStatus,
		Dims: module.Dims{
			{ID: "server_%s_record_%s_query_status_success", Name: "success"},
			{ID: "server_%s_record_%s_query_status_network_error", Name: "network_error"},
			{ID: "server_%s_record_%s_query_status_dns_error", Name: "dns_error"},
		},
	}
	dnsQueryTimeChartTmpl = module.Chart{
		ID:       "server_%s_record_%s_query_time",
		Title:    "DNS Query Time",
		Units:    "seconds",
		Fam:      "query time",
		Ctx:      "dns_query.query_time",
		Priority: prioDNSQueryTime,
		Dims: module.Dims{
			{ID: "server_%s_record_%s_query_time", Name: "query_time", Div: 1e9},
		},
	}
)

func newDNSServerCharts(server, network, rtype string) *module.Charts {
	charts := dnsChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ReplaceAll(server, ".", "_"), rtype)
		chart.Labels = []module.Label{
			{Key: "server", Value: server},
			{Key: "network", Value: network},
			{Key: "record_type", Value: rtype},
		}
		for _, d := range chart.Dims {
			d.ID = fmt.Sprintf(d.ID, server, rtype)
		}
	}

	return charts
}
