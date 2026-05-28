// SPDX-License-Identifier: GPL-3.0-or-later

package monit

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioServiceCheckStatus = collectorapi.Priority + iota
	prioUptime
)

var baseCharts = collectorapi.Charts{
	uptimeChart.Copy(),
}

var (
	uptimeChart = collectorapi.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "monit.uptime",
		Priority: prioUptime,
		Dims: collectorapi.Dims{
			{ID: "uptime"},
		},
	}
)

var serviceCheckChartsTmpl = collectorapi.Charts{
	serviceCheckStatusChartTmpl.Copy(),
}

var (
	serviceCheckStatusChartTmpl = collectorapi.Chart{
		ID:       "service_check_type_%s_name_%s_status",
		Title:    "Service Check Status",
		Units:    "status",
		Fam:      "service status",
		Ctx:      "monit.service_check_status",
		Priority: prioServiceCheckStatus,
		Dims: collectorapi.Dims{
			{ID: "service_check_type_%s_name_%s_status_ok", Name: "ok"},
			{ID: "service_check_type_%s_name_%s_status_error", Name: "error"},
			{ID: "service_check_type_%s_name_%s_status_initializing", Name: "initializing"},
			{ID: "service_check_type_%s_name_%s_status_not_monitored", Name: "not_monitored"},
		},
	}
)

func (c *Collector) addServiceCheckCharts(svc statusServiceCheck, srv *statusServer) {
	charts := serviceCheckChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartId(fmt.Sprintf(chart.ID, svc.svcType(), svc.Name))
		chart.Labels = []collectorapi.Label{
			{Key: "server_hostname", Value: srv.LocalHostname},
			{Key: "service_check_name", Value: svc.Name},
			{Key: "service_check_type", Value: svc.svcType()},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, svc.svcType(), svc.Name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeServiceCharts(svc statusServiceCheck) {
	px := fmt.Sprintf("service_check_type_%s_name_%s_", svc.svcType(), svc.Name)
	px = cleanChartId(px)

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanChartId(s string) string {
	r := strings.NewReplacer(" ", "_", ".", "_", ",", "_")
	return r.Replace(s)
}
