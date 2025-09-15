// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"maps"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
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

	prioProfileChart
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

func (c *Collector) addNetIfaceCharts(iface *netInterface) {
	charts := netIfaceChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanMetricName.Replace(iface.ifName))
		chart.Labels = []module.Label{
			{Key: "vendor", Value: c.sysInfo.Organization},
			{Key: "sysName", Value: c.sysInfo.Name},
			{Key: "ifDescr", Value: iface.ifDescr},
			{Key: "ifName", Value: iface.ifName},
			{Key: "ifType", Value: ifTypeMapping[iface.ifType]},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, iface.ifName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeNetIfaceCharts(iface *netInterface) {
	px := fmt.Sprintf("snmp_device_net_iface_%s_", cleanMetricName.Replace(iface.ifName))
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (c *Collector) addSysUptimeChart() {
	chart := uptimeChart.Copy()
	chart.Labels = []module.Label{
		{Key: "vendor", Value: c.sysInfo.Organization},
		{Key: "sysName", Value: c.sysInfo.Name},
	}
	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
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

func (c *Collector) addProfileScalarMetricChart(m ddsnmp.Metric) {
	if m.Name == "" {
		return
	}

	chart := &module.Chart{
		ID:       fmt.Sprintf("snmp_device_prof_%s", cleanMetricName.Replace(m.Name)),
		Title:    m.Description,
		Type:     module.ChartType(m.ChartType),
		Units:    m.Unit,
		Fam:      m.Family,
		Ctx:      fmt.Sprintf("snmp.device_prof_%s", cleanMetricName.Replace(m.Name)),
		Priority: prioProfileChart,
	}
	if chart.Title == "" {
		chart.Title = fmt.Sprintf("SNMP metric %s", m.Name)
	}
	if chart.Units == "" {
		chart.Units = "1"
	}
	if chart.Fam == "" {
		chart.Fam = m.Name
	}
	if chart.Units == "bit/s" {
		chart.Type = module.Area
	}

	tags := map[string]string{
		"vendor":  c.sysInfo.Organization,
		"sysName": c.sysInfo.Name,
	}

	maps.Copy(tags, m.Profile.Tags)
	for k, v := range tags {
		chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
	}

	if len(m.MultiValue) > 0 {
		seen := make(map[string]bool)
		for k := range m.MultiValue {
			if !seen[k] {
				seen[k] = true
				id := fmt.Sprintf("snmp_device_prof_%s_%s", m.Name, k)
				chart.Dims = append(chart.Dims, &module.Dim{ID: id, Name: k, Algo: dimAlgoFromDdSnmpType(m)})
			}
		}
	} else {
		id := fmt.Sprintf("snmp_device_prof_%s", m.Name)
		chart.Dims = module.Dims{
			{ID: id, Name: m.Name, Algo: dimAlgoFromDdSnmpType(m)},
		}
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addProfileTableMetricChart(m ddsnmp.Metric) {
	if m.Name == "" {
		return
	}

	key := tableMetricKey(m)

	chart := &module.Chart{
		ID:       fmt.Sprintf("snmp_device_prof_%s", cleanMetricName.Replace(key)),
		Title:    m.Description,
		Units:    m.Unit,
		Fam:      m.Family,
		Ctx:      fmt.Sprintf("snmp.device_prof_%s", cleanMetricName.Replace(m.Name)),
		Priority: prioProfileChart,
	}
	if chart.Title == "" {
		chart.Title = fmt.Sprintf("SNMP metric %s", m.Name)
	}
	if chart.Units == "" {
		chart.Units = "1"
	}
	if chart.Fam == "" {
		chart.Fam = m.Name
	}
	if chart.Units == "bit/s" {
		chart.Type = module.Area
	}

	tags := map[string]string{
		"vendor":  c.sysInfo.Organization,
		"sysName": c.sysInfo.Name,
	}
	maps.Copy(tags, m.Profile.Tags)
	for k, v := range m.Tags {
		newKey := strings.TrimPrefix(k, "_")
		tags[newKey] = v
	}

	for k, v := range tags {
		chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
	}

	if len(m.MultiValue) > 0 {
		seen := make(map[string]bool)
		for k := range m.MultiValue {
			if !seen[k] {
				seen[k] = true
				id := fmt.Sprintf("snmp_device_prof_%s_%s", key, k)
				chart.Dims = append(chart.Dims, &module.Dim{ID: id, Name: k, Algo: module.Absolute})
			}
		}
	} else {
		id := fmt.Sprintf("snmp_device_prof_%s", key)
		chart.Dims = module.Dims{
			{ID: id, Name: m.Name, Algo: dimAlgoFromDdSnmpType(m)},
		}
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeProfileTableMetricChart(key string) {
	id := fmt.Sprintf("snmp_device_prof_%s", cleanMetricName.Replace(key))
	if chart := c.Charts().Get(id); chart != nil {
		chart.MarkRemove()
		chart.MarkNotCreated()
	}
}

func dimAlgoFromDdSnmpType(m ddsnmp.Metric) module.DimAlgo {
	switch m.MetricType {
	case ddprofiledefinition.ProfileMetricTypeGauge,
		ddprofiledefinition.ProfileMetricTypeMonotonicCount,
		ddprofiledefinition.ProfileMetricTypeMonotonicCountAndRate:
		return module.Absolute
	default:
		return module.Incremental
	}
}

var cleanMetricName = strings.NewReplacer(".", "_", " ", "_")
