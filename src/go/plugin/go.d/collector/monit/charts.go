// SPDX-License-Identifier: GPL-3.0-or-later

package monit

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioServiceCheckStatus = module.Priority + iota
	prioUptime
)

var baseCharts = module.Charts{
	uptimeChart.Copy(),
}

var (
	uptimeChart = module.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "monit.uptime",
		Priority: prioUptime,
		Dims: module.Dims{
			{ID: "uptime"},
		},
	}
)

var serviceCheckChartsTmpl = module.Charts{
	serviceCheckStatusChartTmpl.Copy(),
}

var (
	serviceCheckStatusChartTmpl = module.Chart{
		ID:       "service_check_type_%s_name_%s_status",
		Title:    "Service Check Status",
		Units:    "status",
		Fam:      "service status",
		Ctx:      "monit.service_check_status",
		Priority: prioServiceCheckStatus,
		Dims: module.Dims{
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
		chart.Labels = []module.Label{
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
