// SPDX-License-Identifier: GPL-3.0-or-later

package pulsar

import (
	"errors"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

func isValidPulsarMetrics(pms prometheus.Series) bool {
	return pms.FindByName(metricPulsarTopicsCount).Len() > 0
}

func (p *Pulsar) resetCurCache() {
	for ns := range p.curCache.namespaces {
		delete(p.curCache.namespaces, ns)
	}
	for top := range p.curCache.topics {
		delete(p.curCache.topics, top)
	}
}

func (p *Pulsar) collect() (map[string]int64, error) {
	pms, err := p.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}

	if !isValidPulsarMetrics(pms) {
		return nil, errors.New("returned metrics aren't Apache Pulsar metrics")
	}

	p.once.Do(func() {
		p.adjustCharts(pms)
	})

	mx := p.collectMetrics(pms)
	p.updateCharts()
	p.resetCurCache()

	return stm.ToMap(mx), nil
}

func (p *Pulsar) collectMetrics(pms prometheus.Series) map[string]float64 {
	mx := make(map[string]float64)
	p.collectBroker(mx, pms)
	return mx
}

func (p *Pulsar) collectBroker(mx map[string]float64, pms prometheus.Series) {
	pms = findPulsarMetrics(pms)
	for _, pm := range pms {
		ns, top := newNamespace(pm), newTopic(pm)
		if ns.name == "" {
			continue
		}

		p.curCache.namespaces[ns] = true

		value := pm.Value * precision(pm.Name())
		mx[pm.Name()] += value
		mx[pm.Name()+"_"+ns.name] += value

		if top.name == "" || !p.topicFilter.MatchString(top.name) {
			continue
		}

		p.curCache.topics[top] = true
		mx[pm.Name()+"_"+top.name] += value
	}
	mx["pulsar_namespaces_count"] = float64(len(p.curCache.namespaces))
}

func newNamespace(pm prometheus.SeriesSample) namespace {
	return namespace{
		name: pm.Labels.Get("namespace"),
	}
}

func newTopic(pm prometheus.SeriesSample) topic {
	return topic{
		namespace: pm.Labels.Get("namespace"),
		name:      pm.Labels.Get("topic"),
	}
}

func findPulsarMetrics(pms prometheus.Series) prometheus.Series {
	var ms prometheus.Series
	for _, pm := range pms {
		if isPulsarHistogram(pm) {
			ms = append(ms, pm)
		}
	}
	pms = pms.FindByNames(
		metricPulsarTopicsCount,
		metricPulsarSubscriptionDelayed,
		metricPulsarSubscriptionsCount,
		metricPulsarProducersCount,
		metricPulsarConsumersCount,
		metricPulsarRateIn,
		metricPulsarRateOut,
		metricPulsarThroughputIn,
		metricPulsarThroughputOut,
		metricPulsarStorageSize,
		metricPulsarStorageWriteRate,
		metricPulsarStorageReadRate,
		metricPulsarMsgBacklog,
		metricPulsarSubscriptionMsgRateRedeliver,
		metricPulsarSubscriptionBlockedOnUnackedMessages,
	)
	return append(ms, pms...)
}

func isPulsarHistogram(pm prometheus.SeriesSample) bool {
	s := pm.Name()
	return strings.HasPrefix(s, "pulsar_storage_write_latency") || strings.HasPrefix(s, "pulsar_entry_size")
}

func precision(metric string) float64 {
	switch metric {
	case metricPulsarRateIn,
		metricPulsarRateOut,
		metricPulsarThroughputIn,
		metricPulsarThroughputOut,
		metricPulsarStorageWriteRate,
		metricPulsarStorageReadRate,
		metricPulsarSubscriptionMsgRateRedeliver,
		metricPulsarReplicationRateIn,
		metricPulsarReplicationRateOut,
		metricPulsarReplicationThroughputIn,
		metricPulsarReplicationThroughputOut:
		return 1000
	}
	return 1
}
