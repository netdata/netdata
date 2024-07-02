// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioNetIfaceTraffic = module.Priority + iota
	prioNetIfaceUnicast
	prioNetIfaceMulticast
	prioNetIfaceBroadcast
	prioNetIfaceErrors
	prioNetIfaceDiscards
	prioNetIfaceAdminStatus
	prioNetIfaceOperStatus
	prioSysUptime
)

var netIfaceChartsTmpl = module.Charts{
	netIfaceTrafficChartTmpl.Copy(),
	netIfacePacketsChartTmpl.Copy(),
	netIfaceMulticastChartTmpl.Copy(),
	netIfaceBroadcastChartTmpl.Copy(),
	netIfaceErrorsChartTmpl.Copy(),
	netIfaceDiscardsChartTmpl.Copy(),
	netIfaceAdminStatusChartTmpl.Copy(),
	netIfaceOperStatusChartTmpl.Copy(),
}

var (
	netIfaceTrafficChartTmpl = module.Chart{
		ID:       "snmp_device_net_iface_%s_traffic",
		Title:    "SNMP device network interface traffic",
		Units:    "kilobits/s",
		Fam:      "traffic",
		Ctx:      "snmp.device_net_interface_traffic",
		Priority: prioNetIfaceTraffic,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "net_iface_%s_traffic_in", Name: "received", Algo: module.Incremental},
			{ID: "net_iface_%s_traffic_out", Name: "sent", Mul: -1, Algo: module.Incremental},
		},
	}

	netIfacePacketsChartTmpl = module.Chart{
		ID:       "snmp_device_net_iface_%s_unicast",
		Title:    "SNMP device network interface unicast packets",
		Units:    "packets/s",
		Fam:      "packets",
		Ctx:      "snmp.device_net_interface_unicast",
		Priority: prioNetIfaceUnicast,
		Dims: module.Dims{
			{ID: "net_iface_%s_ucast_in", Name: "received", Algo: module.Incremental},
			{ID: "net_iface_%s_ucast_out", Name: "sent", Mul: -1, Algo: module.Incremental},
		},
	}
	netIfaceMulticastChartTmpl = module.Chart{
		ID:       "snmp_device_net_iface_%s_multicast",
		Title:    "SNMP device network interface multicast packets",
		Units:    "packets/s",
		Fam:      "packets",
		Ctx:      "snmp.device_net_interface_multicast",
		Priority: prioNetIfaceMulticast,
		Dims: module.Dims{
			{ID: "net_iface_%s_mcast_in", Name: "received", Algo: module.Incremental},
			{ID: "net_iface_%s_mcast_out", Name: "sent", Mul: -1, Algo: module.Incremental},
		},
	}
	netIfaceBroadcastChartTmpl = module.Chart{
		ID:       "snmp_device_net_iface_%s_broadcast",
		Title:    "SNMP device network interface broadcast packets",
		Units:    "packets/s",
		Fam:      "packets",
		Ctx:      "snmp.device_net_interface_broadcast",
		Priority: prioNetIfaceBroadcast,
		Dims: module.Dims{
			{ID: "net_iface_%s_bcast_in", Name: "received", Algo: module.Incremental},
			{ID: "net_iface_%s_bcast_out", Name: "sent", Mul: -1, Algo: module.Incremental},
		},
	}

	netIfaceErrorsChartTmpl = module.Chart{
		ID:       "snmp_device_net_iface_%s_errors",
		Title:    "SNMP device network interface errors",
		Units:    "errors/s",
		Fam:      "errors",
		Ctx:      "snmp.device_net_interface_errors",
		Priority: prioNetIfaceErrors,
		Dims: module.Dims{
			{ID: "net_iface_%s_errors_in", Name: "inbound", Algo: module.Incremental},
			{ID: "net_iface_%s_errors_out", Name: "outbound", Mul: -1, Algo: module.Incremental},
		},
	}

	netIfaceDiscardsChartTmpl = module.Chart{
		ID:       "snmp_device_net_iface_%s_discards",
		Title:    "SNMP device network interface discards",
		Units:    "discards/s",
		Fam:      "discards",
		Ctx:      "snmp.device_net_interface_discards",
		Priority: prioNetIfaceDiscards,
		Dims: module.Dims{
			{ID: "net_iface_%s_discards_in", Name: "inbound", Algo: module.Incremental},
			{ID: "net_iface_%s_discards_out", Name: "outbound", Mul: -1, Algo: module.Incremental},
		},
	}

	netIfaceAdminStatusChartTmpl = module.Chart{
		ID:       "snmp_device_net_iface_%s_admin_status",
		Title:    "SNMP device network interface administrative status",
		Units:    "status",
		Fam:      "status",
		Ctx:      "snmp.device_net_interface_admin_status",
		Priority: prioNetIfaceAdminStatus,
		Dims: module.Dims{
			{ID: "net_iface_%s_admin_status_up", Name: "up"},
			{ID: "net_iface_%s_admin_status_down", Name: "down"},
			{ID: "net_iface_%s_admin_status_testing", Name: "testing"},
		},
	}
	netIfaceOperStatusChartTmpl = module.Chart{
		ID:       "snmp_device_net_iface_%s_oper_status",
		Title:    "SNMP device network interface operational status",
		Units:    "status",
		Fam:      "status",
		Ctx:      "snmp.device_net_interface_oper_status",
		Priority: prioNetIfaceOperStatus,
		Dims: module.Dims{
			{ID: "net_iface_%s_oper_status_up", Name: "up"},
			{ID: "net_iface_%s_oper_status_down", Name: "down"},
			{ID: "net_iface_%s_oper_status_testing", Name: "testing"},
			{ID: "net_iface_%s_oper_status_unknown", Name: "unknown"},
			{ID: "net_iface_%s_oper_status_dormant", Name: "dormant"},
			{ID: "net_iface_%s_oper_status_notPresent", Name: "not_present"},
			{ID: "net_iface_%s_oper_status_lowerLayerDown", Name: "lower_layer_down"},
		},
	}
)

var (
	uptimeChart = module.Chart{
		ID:       "snmp_device_uptime",
		Title:    "SNMP device uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "snmp.device_uptime",
		Priority: prioSysUptime,
		Dims: module.Dims{
			{ID: "uptime", Name: "uptime"},
		},
	}
)

func (s *SNMP) addNetIfaceCharts(iface *netInterface) {
	charts := netIfaceChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanIfaceName(iface.ifName))
		chart.Labels = []module.Label{
			{Key: "sysName", Value: s.sysName},
			{Key: "ifDescr", Value: iface.ifDescr},
			{Key: "ifName", Value: iface.ifName},
			{Key: "ifType", Value: ifTypeMapping[iface.ifType]},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, iface.ifName)
		}
	}

	if err := s.Charts().Add(*charts...); err != nil {
		s.Warning(err)
	}
}

func (s *SNMP) removeNetIfaceCharts(iface *netInterface) {
	px := fmt.Sprintf("snmp_device_net_iface_%s_", cleanIfaceName(iface.ifName))
	for _, chart := range *s.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (s *SNMP) addSysUptimeChart() {
	chart := uptimeChart.Copy()
	chart.Labels = []module.Label{
		{Key: "sysName", Value: s.sysName},
	}
	if err := s.Charts().Add(chart); err != nil {
		s.Warning(err)
	}
}

func cleanIfaceName(name string) string {
	r := strings.NewReplacer(".", "_", " ", "_")
	return r.Replace(name)
}

func newUserInputCharts(configs []ChartConfig) (*module.Charts, error) {
	charts := &module.Charts{}
	for _, cfg := range configs {
		if len(cfg.IndexRange) == 2 {
			cs, err := newUserInputChartsFromIndexRange(cfg)
			if err != nil {
				return nil, err
			}
			if err := charts.Add(*cs...); err != nil {
				return nil, err
			}
		} else {
			chart, err := newUserInputChart(cfg)
			if err != nil {
				return nil, err
			}
			if err = charts.Add(chart); err != nil {
				return nil, err
			}
		}
	}
	return charts, nil
}

func newUserInputChartsFromIndexRange(cfg ChartConfig) (*module.Charts, error) {
	var addPrio int
	charts := &module.Charts{}
	for i := cfg.IndexRange[0]; i <= cfg.IndexRange[1]; i++ {
		chart, err := newUserInputChartWithOIDIndex(i, cfg)
		if err != nil {
			return nil, err
		}
		chart.Priority += addPrio
		addPrio += 1
		if err = charts.Add(chart); err != nil {
			return nil, err
		}
	}
	return charts, nil
}

func newUserInputChartWithOIDIndex(oidIndex int, cfg ChartConfig) (*module.Chart, error) {
	chart, err := newUserInputChart(cfg)
	if err != nil {
		return nil, err
	}

	chart.ID = fmt.Sprintf("%s_%d", chart.ID, oidIndex)
	chart.Title = fmt.Sprintf("%s %d", chart.Title, oidIndex)
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf("%s.%d", dim.ID, oidIndex)
	}

	return chart, nil
}

func newUserInputChart(cfg ChartConfig) (*module.Chart, error) {
	chart := &module.Chart{
		ID:       cfg.ID,
		Title:    cfg.Title,
		Units:    cfg.Units,
		Fam:      cfg.Family,
		Ctx:      fmt.Sprintf("snmp.%s", cfg.ID),
		Type:     module.ChartType(cfg.Type),
		Priority: cfg.Priority,
	}

	if chart.Title == "" {
		chart.Title = "Untitled chart"
	}
	if chart.Units == "" {
		chart.Units = "num"
	}
	if chart.Priority < module.Priority {
		chart.Priority += module.Priority
	}

	seen := make(map[string]struct{})
	var a string
	for _, cfg := range cfg.Dimensions {
		if cfg.Algorithm != "" {
			seen[cfg.Algorithm] = struct{}{}
			a = cfg.Algorithm
		}
		dim := &module.Dim{
			ID:   strings.TrimPrefix(cfg.OID, "."),
			Name: cfg.Name,
			Algo: module.DimAlgo(cfg.Algorithm),
			Mul:  cfg.Multiplier,
			Div:  cfg.Divisor,
		}
		if err := chart.AddDim(dim); err != nil {
			return nil, err
		}
	}
	if len(seen) == 1 && a != "" && len(chart.Dims) > 1 {
		for _, d := range chart.Dims {
			if d.Algo == "" {
				d.Algo = module.DimAlgo(a)
			}
		}
	}

	return chart, nil
}
