// SPDX-License-Identifier: GPL-3.0-or-later

package dnsquery

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioDNSQueryStatus = collectorapi.Priority + iota
	prioDNSQueryTime
)

var (
	dnsChartsTmpl = collectorapi.Charts{
		dnsQueryStatusChartTmpl.Copy(),
		dnsQueryTimeChartTmpl.Copy(),
	}
	dnsQueryStatusChartTmpl = collectorapi.Chart{
		ID:       "server_%s_record_%s_query_status",
		Title:    "DNS Query Status",
		Units:    "status",
		Fam:      "query status",
		Ctx:      "dns_query.query_status",
		Priority: prioDNSQueryStatus,
		Dims: collectorapi.Dims{
			{ID: "server_%s_record_%s_query_status_success", Name: "success"},
			{ID: "server_%s_record_%s_query_status_network_error", Name: "network_error"},
			{ID: "server_%s_record_%s_query_status_dns_error", Name: "dns_error"},
		},
	}
	dnsQueryTimeChartTmpl = collectorapi.Chart{
		ID:       "server_%s_record_%s_query_time",
		Title:    "DNS Query Time",
		Units:    "seconds",
		Fam:      "query time",
		Ctx:      "dns_query.query_time",
		Priority: prioDNSQueryTime,
		Dims: collectorapi.Dims{
			{ID: "server_%s_record_%s_query_time", Name: "query_time", Div: 1e9},
		},
	}
)

func newDNSServerCharts(server, network, rtype string) *collectorapi.Charts {
	charts := dnsChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ReplaceAll(server, ".", "_"), rtype)
		chart.Labels = []collectorapi.Label{
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
