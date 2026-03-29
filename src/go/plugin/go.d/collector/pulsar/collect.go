// SPDX-License-Identifier: GPL-3.0-or-later

package pulsar

import (
	"errors"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
	"github.com/netdata/netdata/go/plugins/pkg/stm"
)

func isValidPulsarMetrics(pms promscrapemodel.Series) bool {
	return pms.FindByName(metricPulsarTopicsCount).Len() > 0
}

func (c *Collector) resetCurCache() {
	for ns := range c.curCache.namespaces {
		delete(c.curCache.namespaces, ns)
	}
	for top := range c.curCache.topics {
		delete(c.curCache.topics, top)
	}
}

func (c *Collector) collect() (map[string]int64, error) {
	pms, err := c.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}

	if !isValidPulsarMetrics(pms) {
		return nil, errors.New("returned metrics aren't Apache Pulsar metrics")
	}

	c.once.Do(func() {
		c.adjustCharts(pms)
	})

	mx := c.collectMetrics(pms)
	c.updateCharts()
	c.resetCurCache()

	return stm.ToMap(mx), nil
}

func (c *Collector) collectMetrics(pms promscrapemodel.Series) map[string]float64 {
	mx := make(map[string]float64)
	c.collectBroker(mx, pms)
	return mx
}

func (c *Collector) collectBroker(mx map[string]float64, pms promscrapemodel.Series) {
	pms = findPulsarMetrics(pms)
	for _, pm := range pms {
		ns, top := newNamespace(pm), newTopic(pm)
		if ns.name == "" {
			continue
		}

		c.curCache.namespaces[ns] = true

		value := pm.Value * precision(pm.Name())
		mx[pm.Name()] += value
		mx[pm.Name()+"_"+ns.name] += value

		if top.name == "" || !c.topicFilter.MatchString(top.name) {
			continue
		}

		c.curCache.topics[top] = true
		mx[pm.Name()+"_"+top.name] += value
	}
	mx["pulsar_namespaces_count"] = float64(len(c.curCache.namespaces))
}

func newNamespace(pm promscrapemodel.SeriesSample) namespace {
	return namespace{
		name: pm.Labels.Get("namespace"),
	}
}

func newTopic(pm promscrapemodel.SeriesSample) topic {
	return topic{
		namespace: pm.Labels.Get("namespace"),
		name:      pm.Labels.Get("topic"),
	}
}

func findPulsarMetrics(pms promscrapemodel.Series) promscrapemodel.Series {
	var ms promscrapemodel.Series
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

func isPulsarHistogram(pm promscrapemodel.SeriesSample) bool {
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
