// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"maps"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type bgpScope string

const (
	bgpScopeAuto         bgpScope = "auto"
	bgpScopePeers        bgpScope = "peers"
	bgpScopePeerFamilies bgpScope = "peer_families"
	bgpScopeDevices      bgpScope = "devices"
)

type bgpRoute struct {
	leaf           string
	dim            string
	copyMultiValue bool
	scope          bgpScope
}

type bgpPublicMetricSpec struct {
	description string
	family      string
	unit        string
	chartType   string
	metricType  ddprofiledefinition.ProfileMetricType
	isTable     bool
}

type bgpPublicNormalizer struct {
	tableMetrics  map[string]*ddsnmp.Metric
	scalarMetrics map[string]*ddsnmp.Metric
}

func normalizeCollectorMetrics(pms []*ddsnmp.ProfileMetrics) []ddsnmp.Metric {
	normalizer := newBGPPublicNormalizer()
	metrics := make([]ddsnmp.Metric, 0)

	for _, pm := range pms {
		for _, metric := range pm.Metrics {
			if metric.Profile == nil {
				metric.Profile = pm
			}
			if normalizer.handle(metric) {
				continue
			}
			metrics = append(metrics, metric)
		}
	}

	return append(metrics, normalizer.metrics()...)
}

func newBGPPublicNormalizer() *bgpPublicNormalizer {
	return &bgpPublicNormalizer{
		tableMetrics:  make(map[string]*ddsnmp.Metric),
		scalarMetrics: make(map[string]*ddsnmp.Metric),
	}
}

func (n *bgpPublicNormalizer) metrics() []ddsnmp.Metric {
	n.buildDeviceSummaries()

	metrics := make([]ddsnmp.Metric, 0, len(n.scalarMetrics)+len(n.tableMetrics))

	scalarKeys := sortedKeys(n.scalarMetrics)
	for _, key := range scalarKeys {
		metrics = append(metrics, *n.scalarMetrics[key])
	}

	tableKeys := sortedKeys(n.tableMetrics)
	for _, key := range tableKeys {
		metrics = append(metrics, *n.tableMetrics[key])
	}

	return metrics
}

func (n *bgpPublicNormalizer) handle(metric ddsnmp.Metric) bool {
	route, ok := routeBGPPublicMetric(metric.Name)
	if !ok {
		return shouldSuppressBGPRawMetric(metric.Name)
	}

	scope := resolveBGPScope(route.scope, metric.Tags)
	name := buildBGPPublicMetricName(scope, route.leaf)
	spec, ok := bgpPublicSpec(name)
	if !ok {
		return false
	}

	if scope == bgpScopeDevices {
		acc := n.scalarMetric(name, spec, metric)
		n.mergeMetricValue(acc, route, metric)
		return true
	}

	publicTags := publicBGPTags(metric, scope)
	acc := n.tableMetric(name, spec, metric, publicTags)
	n.mergeMetricValue(acc, route, metric)

	return true
}

func (n *bgpPublicNormalizer) mergeMetricValue(acc *ddsnmp.Metric, route bgpRoute, metric ddsnmp.Metric) {
	if route.copyMultiValue {
		if acc.MultiValue == nil {
			acc.MultiValue = make(map[string]int64)
		}
		mergeMultiValue(acc.MultiValue, metric.MultiValue)
		return
	}

	if route.dim != "" {
		if acc.MultiValue == nil {
			acc.MultiValue = make(map[string]int64)
		}
		if _, ok := acc.MultiValue[route.dim]; !ok {
			acc.MultiValue[route.dim] = metric.Value
		}
		return
	}

	acc.Value = metric.Value
}

func (n *bgpPublicNormalizer) scalarMetric(name string, spec bgpPublicMetricSpec, sample ddsnmp.Metric) *ddsnmp.Metric {
	if metric, ok := n.scalarMetrics[name]; ok {
		return metric
	}

	metric := newBGPPublicMetric(name, spec, sample, nil)
	n.scalarMetrics[name] = metric
	return metric
}

func (n *bgpPublicNormalizer) tableMetric(name string, spec bgpPublicMetricSpec, sample ddsnmp.Metric, tags map[string]string) *ddsnmp.Metric {
	key := tableMetricKey(ddsnmp.Metric{Name: name, Tags: tags})
	if metric, ok := n.tableMetrics[key]; ok {
		return metric
	}

	metric := newBGPPublicMetric(name, spec, sample, tags)
	n.tableMetrics[key] = metric
	return metric
}

func newBGPPublicMetric(name string, spec bgpPublicMetricSpec, sample ddsnmp.Metric, tags map[string]string) *ddsnmp.Metric {
	metric := &ddsnmp.Metric{
		Profile:     sample.Profile,
		Name:        name,
		Description: spec.description,
		Family:      spec.family,
		Unit:        spec.unit,
		ChartType:   spec.chartType,
		MetricType:  spec.metricType,
		IsTable:     spec.isTable,
	}
	if len(tags) > 0 {
		metric.Tags = maps.Clone(tags)
	}
	return metric
}

func (n *bgpPublicNormalizer) buildDeviceSummaries() {
	n.buildPeerCountSummary()
	n.buildPeerStateSummary()
}

func (n *bgpPublicNormalizer) buildPeerCountSummary() {
	spec, ok := bgpPublicSpec("bgp.devices.peer_counts")
	if !ok {
		return
	}

	var sample ddsnmp.Metric
	found := false
	for _, metric := range n.tableMetrics {
		if metric.Name == "bgp.peers.availability" {
			sample = *metric
			found = true
			break
		}
	}
	if !found {
		return
	}

	acc := n.scalarMetric("bgp.devices.peer_counts", spec, sample)
	if acc.MultiValue == nil {
		acc.MultiValue = make(map[string]int64)
	}

	for _, metric := range n.tableMetrics {
		if metric.Name != "bgp.peers.availability" {
			continue
		}
		acc.MultiValue["configured"]++
		acc.MultiValue["admin_enabled"] += metric.MultiValue["admin_enabled"]
		acc.MultiValue["established"] += metric.MultiValue["established"]
	}
}

func (n *bgpPublicNormalizer) buildPeerStateSummary() {
	spec, ok := bgpPublicSpec("bgp.devices.peer_states")
	if !ok {
		return
	}

	var sample ddsnmp.Metric
	found := false
	for _, metric := range n.tableMetrics {
		if metric.Name == "bgp.peers.connection_state" {
			sample = *metric
			found = true
			break
		}
	}
	if !found {
		return
	}

	acc := n.scalarMetric("bgp.devices.peer_states", spec, sample)
	if acc.MultiValue == nil {
		acc.MultiValue = make(map[string]int64)
	}

	for _, metric := range n.tableMetrics {
		if metric.Name != "bgp.peers.connection_state" {
			continue
		}
		for dim, value := range metric.MultiValue {
			acc.MultiValue[dim] += value
		}
	}
}
