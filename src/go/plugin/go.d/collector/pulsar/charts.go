// SPDX-License-Identifier: GPL-3.0-or-later

package pulsar

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

type (
	Charts = module.Charts
	Chart  = module.Chart
	Dims   = module.Dims
	Dim    = module.Dim
	Opts   = module.Opts
)

var summaryCharts = Charts{
	sumBrokerComponentsChart.Copy(),

	sumMessagesRateChart.Copy(),
	sumThroughputRateChart.Copy(),

	sumStorageSizeChart.Copy(),
	sumStorageOperationsRateChart.Copy(), // optional
	sumMsgBacklogSizeChart.Copy(),
	sumStorageWriteLatencyChart.Copy(),
	sumEntrySizeChart.Copy(),

	sumSubsDelayedChart.Copy(),
	sumSubsMsgRateRedeliverChart.Copy(),    // optional
	sumSubsBlockedOnUnackedMsgChart.Copy(), // optional

	sumReplicationRateChart.Copy(),           // optional
	sumReplicationThroughputRateChart.Copy(), // optional
	sumReplicationBacklogChart.Copy(),        // optional
}

var (
	sumBrokerComponentsChart = Chart{
		ID:    "broker_components",
		Title: "Broker Components",
		Units: "components",
		Fam:   "ns summary",
		Ctx:   "pulsar.broker_components",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: "pulsar_namespaces_count", Name: "namespaces"},
			{ID: metricPulsarTopicsCount, Name: "topics"},
			{ID: metricPulsarSubscriptionsCount, Name: "subscriptions"},
			{ID: metricPulsarProducersCount, Name: "producers"},
			{ID: metricPulsarConsumersCount, Name: "consumers"},
		},
	}
	sumMessagesRateChart = Chart{
		ID:    "messages_rate",
		Title: "Messages Rate",
		Units: "messages/s",
		Fam:   "ns summary",
		Ctx:   "pulsar.messages_rate",
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: metricPulsarRateIn, Name: "publish", Div: 1000},
			{ID: metricPulsarRateOut, Name: "dispatch", Mul: -1, Div: 1000},
		},
	}
	sumThroughputRateChart = Chart{
		ID:    "throughput_rate",
		Title: "Throughput Rate",
		Units: "KiB/s",
		Fam:   "ns summary",
		Ctx:   "pulsar.throughput_rate",
		Type:  module.Area,
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: metricPulsarThroughputIn, Name: "publish", Div: 1024 * 1000},
			{ID: metricPulsarThroughputOut, Name: "dispatch", Mul: -1, Div: 1024 * 1000},
		},
	}
	sumStorageSizeChart = Chart{
		ID:    "storage_size",
		Title: "Storage Size",
		Units: "KiB",
		Fam:   "ns summary",
		Ctx:   "pulsar.storage_size",
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: metricPulsarStorageSize, Name: "used", Div: 1024},
		},
	}
	sumStorageOperationsRateChart = Chart{
		ID:    "storage_operations_rate",
		Title: "Storage Read/Write Operations Rate",
		Units: "message batches/s",
		Fam:   "ns summary",
		Ctx:   "pulsar.storage_operations_rate",
		Type:  module.Area,
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: metricPulsarStorageReadRate, Name: "read", Div: 1000},
			{ID: metricPulsarStorageWriteRate, Name: "write", Mul: -1, Div: 1000},
		},
	}
	sumMsgBacklogSizeChart = Chart{
		ID:    "msg_backlog",
		Title: "Messages Backlog Size",
		Units: "messages",
		Fam:   "ns summary",
		Ctx:   "pulsar.msg_backlog",
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: metricPulsarMsgBacklog, Name: "backlog"},
		},
	}
	sumStorageWriteLatencyChart = Chart{
		ID:    "storage_write_latency",
		Title: "Storage Write Latency",
		Units: "entries/s",
		Fam:   "ns summary",
		Ctx:   "pulsar.storage_write_latency",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: "pulsar_storage_write_latency_le_0_5", Name: "<=0.5ms", Div: 60},
			{ID: "pulsar_storage_write_latency_le_1", Name: "<=1ms", Div: 60},
			{ID: "pulsar_storage_write_latency_le_5", Name: "<=5ms", Div: 60},
			{ID: "pulsar_storage_write_latency_le_10", Name: "<=10ms", Div: 60},
			{ID: "pulsar_storage_write_latency_le_20", Name: "<=20ms", Div: 60},
			{ID: "pulsar_storage_write_latency_le_50", Name: "<=50ms", Div: 60},
			{ID: "pulsar_storage_write_latency_le_100", Name: "<=100ms", Div: 60},
			{ID: "pulsar_storage_write_latency_le_200", Name: "<=200ms", Div: 60},
			{ID: "pulsar_storage_write_latency_le_1000", Name: "<=1s", Div: 60},
			{ID: "pulsar_storage_write_latency_overflow", Name: ">1s", Div: 60},
		},
	}
	sumEntrySizeChart = Chart{
		ID:    "entry_size",
		Title: "Entry Size",
		Units: "entries/s",
		Fam:   "ns summary",
		Ctx:   "pulsar.entry_size",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: "pulsar_entry_size_le_128", Name: "<=128B", Div: 60},
			{ID: "pulsar_entry_size_le_512", Name: "<=512B", Div: 60},
			{ID: "pulsar_entry_size_le_1_kb", Name: "<=1KB", Div: 60},
			{ID: "pulsar_entry_size_le_2_kb", Name: "<=2KB", Div: 60},
			{ID: "pulsar_entry_size_le_4_kb", Name: "<=4KB", Div: 60},
			{ID: "pulsar_entry_size_le_16_kb", Name: "<=16KB", Div: 60},
			{ID: "pulsar_entry_size_le_100_kb", Name: "<=100KB", Div: 60},
			{ID: "pulsar_entry_size_le_1_mb", Name: "<=1MB", Div: 60},
			{ID: "pulsar_entry_size_le_overflow", Name: ">1MB", Div: 60},
		},
	}
	sumSubsDelayedChart = Chart{
		ID:    "subscription_delayed",
		Title: "Subscriptions Delayed for Dispatching",
		Units: "message batches",
		Fam:   "ns summary",
		Ctx:   "pulsar.subscription_delayed",
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: metricPulsarSubscriptionDelayed, Name: "delayed"},
		},
	}
	sumSubsMsgRateRedeliverChart = Chart{
		ID:    "subscription_msg_rate_redeliver",
		Title: "Subscriptions Redelivered Message Rate",
		Units: "messages/s",
		Fam:   "ns summary",
		Ctx:   "pulsar.subscription_msg_rate_redeliver",
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: metricPulsarSubscriptionMsgRateRedeliver, Name: "redelivered", Div: 1000},
		},
	}
	sumSubsBlockedOnUnackedMsgChart = Chart{
		ID:    "subscription_blocked_on_unacked_messages",
		Title: "Subscriptions Blocked On Unacked Messages",
		Units: "subscriptions",
		Fam:   "ns summary",
		Ctx:   "pulsar.subscription_blocked_on_unacked_messages",
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: metricPulsarSubscriptionBlockedOnUnackedMessages, Name: "blocked"},
		},
	}
	sumReplicationRateChart = Chart{
		ID:    "replication_rate",
		Title: "Replication Rate",
		Units: "messages/s",
		Fam:   "ns summary",
		Ctx:   "pulsar.replication_rate",
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: metricPulsarReplicationRateIn, Name: "in", Div: 1000},
			{ID: metricPulsarReplicationRateOut, Name: "out", Mul: -1, Div: 1000},
		},
	}
	sumReplicationThroughputRateChart = Chart{
		ID:    "replication_throughput_rate",
		Title: "Replication Throughput Rate",
		Units: "KiB/s",
		Fam:   "ns summary",
		Ctx:   "pulsar.replication_throughput_rate",
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: metricPulsarReplicationThroughputIn, Name: "in", Div: 1024 * 1000},
			{ID: metricPulsarReplicationThroughputOut, Name: "out", Mul: -1, Div: 1024 * 1000},
		},
	}
	sumReplicationBacklogChart = Chart{
		ID:    "replication_backlog",
		Title: "Replication Backlog",
		Units: "messages",
		Fam:   "ns summary",
		Ctx:   "pulsar.replication_backlog",
		Opts:  Opts{StoreFirst: true},
		Dims: Dims{
			{ID: metricPulsarReplicationBacklog, Name: "backlog"},
		},
	}
)

var namespaceCharts = Charts{
	nsBrokerComponentsChart.Copy(),
	topicProducersChart.Copy(),
	topicSubscriptionsChart.Copy(),
	topicConsumersChart.Copy(),

	nsMessagesRateChart.Copy(),
	topicMessagesRateInChart.Copy(),
	topicMessagesRateOutChart.Copy(),
	nsThroughputRateCharts.Copy(),
	topicThroughputRateInChart.Copy(),
	topicThroughputRateOutChart.Copy(),

	nsStorageSizeChart.Copy(),
	topicStorageSizeChart.Copy(),
	nsStorageOperationsChart.Copy(),   // optional
	topicStorageReadRateChart.Copy(),  // optional
	topicStorageWriteRateChart.Copy(), // optional
	nsMsgBacklogSizeChart.Copy(),
	topicMsgBacklogSizeChart.Copy(),
	nsStorageWriteLatencyChart.Copy(),
	nsEntrySizeChart.Copy(),

	nsSubsDelayedChart.Copy(),
	topicSubsDelayedChart.Copy(),
	nsSubsMsgRateRedeliverChart.Copy(),       // optional
	topicSubsMsgRateRedeliverChart.Copy(),    // optional
	nsSubsBlockedOnUnackedMsgChart.Copy(),    // optional
	topicSubsBlockedOnUnackedMsgChart.Copy(), // optional

	nsReplicationRateChart.Copy(),                 // optional
	topicReplicationRateInChart.Copy(),            // optional
	topicReplicationRateOutChart.Copy(),           // optional
	nsReplicationThroughputChart.Copy(),           // optional
	topicReplicationThroughputRateInChart.Copy(),  // optional
	topicReplicationThroughputRateOutChart.Copy(), // optional
	nsReplicationBacklogChart.Copy(),              // optional
	topicReplicationBacklogChart.Copy(),           // optional
}

func toNamespaceChart(chart Chart) Chart {
	chart = *chart.Copy()
	if chart.ID == sumBrokerComponentsChart.ID {
		_ = chart.RemoveDim("pulsar_namespaces_count")
	}
	chart.ID += "_namespace_%s"
	chart.Fam = "ns %s"
	if idx := strings.IndexByte(chart.Ctx, '.'); idx > 0 {
		// pulsar.messages_rate => pulsar.namespace_messages_rate
		chart.Ctx = chart.Ctx[:idx+1] + "namespace_" + chart.Ctx[idx+1:]
	}
	for _, dim := range chart.Dims {
		dim.ID += "_%s"
	}
	return chart
}

var (
	nsBrokerComponentsChart        = toNamespaceChart(sumBrokerComponentsChart)
	nsMessagesRateChart            = toNamespaceChart(sumMessagesRateChart)
	nsThroughputRateCharts         = toNamespaceChart(sumThroughputRateChart)
	nsStorageSizeChart             = toNamespaceChart(sumStorageSizeChart)
	nsStorageOperationsChart       = toNamespaceChart(sumStorageOperationsRateChart)
	nsMsgBacklogSizeChart          = toNamespaceChart(sumMsgBacklogSizeChart)
	nsStorageWriteLatencyChart     = toNamespaceChart(sumStorageWriteLatencyChart)
	nsEntrySizeChart               = toNamespaceChart(sumEntrySizeChart)
	nsSubsDelayedChart             = toNamespaceChart(sumSubsDelayedChart)
	nsSubsMsgRateRedeliverChart    = toNamespaceChart(sumSubsMsgRateRedeliverChart)
	nsSubsBlockedOnUnackedMsgChart = toNamespaceChart(sumSubsBlockedOnUnackedMsgChart)
	nsReplicationRateChart         = toNamespaceChart(sumReplicationRateChart)
	nsReplicationThroughputChart   = toNamespaceChart(sumReplicationThroughputRateChart)
	nsReplicationBacklogChart      = toNamespaceChart(sumReplicationBacklogChart)

	topicProducersChart = Chart{
		ID:    "topic_producers_namespace_%s",
		Title: "Topic Producers",
		Units: "producers",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_producers",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicSubscriptionsChart = Chart{
		ID:    "topic_subscriptions_namespace_%s",
		Title: "Topic Subscriptions",
		Units: "subscriptions",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_subscriptions",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicConsumersChart = Chart{
		ID:    "topic_consumers_namespace_%s",
		Title: "Topic Consumers",
		Units: "consumers",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_consumers",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicMessagesRateInChart = Chart{
		ID:    "topic_messages_rate_in_namespace_%s",
		Title: "Topic Publish Messages Rate",
		Units: "publishes/s",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_messages_rate_in",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicMessagesRateOutChart = Chart{
		ID:    "topic_messages_rate_out_namespace_%s",
		Title: "Topic Dispatch Messages Rate",
		Units: "dispatches/s",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_messages_rate_out",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicThroughputRateInChart = Chart{
		ID:    "topic_throughput_rate_in_namespace_%s",
		Title: "Topic Publish Throughput Rate",
		Units: "KiB/s",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_throughput_rate_in",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicThroughputRateOutChart = Chart{
		ID:    "topic_throughput_rate_out_namespace_%s",
		Title: "Topic Dispatch Throughput Rate",
		Units: "KiB/s",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_throughput_rate_out",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicStorageSizeChart = Chart{
		ID:    "topic_storage_size_namespace_%s",
		Title: "Topic Storage Size",
		Units: "KiB",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_storage_size",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicStorageReadRateChart = Chart{
		ID:    "topic_storage_read_rate_namespace_%s",
		Title: "Topic Storage Read Rate",
		Units: "message batches/s",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_storage_read_rate",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicStorageWriteRateChart = Chart{
		ID:    "topic_storage_write_rate_namespace_%s",
		Title: "Topic Storage Write Rate",
		Units: "message batches/s",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_storage_write_rate",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicMsgBacklogSizeChart = Chart{
		ID:    "topic_msg_backlog_namespace_%s",
		Title: "Topic Messages Backlog Size",
		Units: "messages",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_msg_backlog",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicSubsDelayedChart = Chart{
		ID:    "topic_subscription_delayed_namespace_%s",
		Title: "Topic Subscriptions Delayed for Dispatching",
		Units: "message batches",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_subscription_delayed",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicSubsMsgRateRedeliverChart = Chart{
		ID:    "topic_subscription_msg_rate_redeliver_namespace_%s",
		Title: "Topic Subscriptions Redelivered Message Rate",
		Units: "messages/s",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_subscription_msg_rate_redeliver",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicSubsBlockedOnUnackedMsgChart = Chart{
		ID:    "topic_subscription_blocked_on_unacked_messages_namespace_%s",
		Title: "Topic Subscriptions Blocked On Unacked Messages",
		Units: "blocked subscriptions",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_subscription_blocked_on_unacked_messages",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicReplicationRateInChart = Chart{
		ID:    "topic_replication_rate_in_namespace_%s",
		Title: "Topic Replication Rate From Remote Cluster",
		Units: "messages/s",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_replication_rate_in",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicReplicationRateOutChart = Chart{
		ID:    "replication_rate_out_namespace_%s",
		Title: "Topic Replication Rate To Remote Cluster",
		Units: "messages/s",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_replication_rate_out",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicReplicationThroughputRateInChart = Chart{
		ID:    "topic_replication_throughput_rate_in_namespace_%s",
		Title: "Topic Replication Throughput Rate From Remote Cluster",
		Units: "KiB/s",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_replication_throughput_rate_in",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicReplicationThroughputRateOutChart = Chart{
		ID:    "topic_replication_throughput_rate_out_namespace_%s",
		Title: "Topic Replication Throughput Rate To Remote Cluster",
		Units: "KiB/s",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_replication_throughput_rate_out",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
	topicReplicationBacklogChart = Chart{
		ID:    "topic_replication_backlog_namespace_%s",
		Title: "Topic Replication Backlog",
		Units: "messages",
		Fam:   "ns %s",
		Ctx:   "pulsar.topic_replication_backlog",
		Type:  module.Stacked,
		Opts:  Opts{StoreFirst: true},
	}
)

func (c *Collector) adjustCharts(pms prometheus.Series) {
	if pms := pms.FindByName(metricPulsarStorageReadRate); pms.Len() == 0 || pms[0].Labels.Get("namespace") == "" {
		c.removeSummaryChart(sumStorageOperationsRateChart.ID)
		c.removeNamespaceChart(nsStorageOperationsChart.ID)
		c.removeNamespaceChart(topicStorageReadRateChart.ID)
		c.removeNamespaceChart(topicStorageWriteRateChart.ID)
		delete(c.topicChartsMapping, topicStorageReadRateChart.ID)
		delete(c.topicChartsMapping, topicStorageWriteRateChart.ID)
	}
	if pms.FindByName(metricPulsarSubscriptionMsgRateRedeliver).Len() == 0 {
		c.removeSummaryChart(sumSubsMsgRateRedeliverChart.ID)
		c.removeSummaryChart(sumSubsBlockedOnUnackedMsgChart.ID)
		c.removeNamespaceChart(nsSubsMsgRateRedeliverChart.ID)
		c.removeNamespaceChart(nsSubsBlockedOnUnackedMsgChart.ID)
		c.removeNamespaceChart(topicSubsMsgRateRedeliverChart.ID)
		c.removeNamespaceChart(topicSubsBlockedOnUnackedMsgChart.ID)
		delete(c.topicChartsMapping, topicSubsMsgRateRedeliverChart.ID)
		delete(c.topicChartsMapping, topicSubsBlockedOnUnackedMsgChart.ID)
	}
	if pms.FindByName(metricPulsarReplicationBacklog).Len() == 0 {
		c.removeSummaryChart(sumReplicationRateChart.ID)
		c.removeSummaryChart(sumReplicationThroughputRateChart.ID)
		c.removeSummaryChart(sumReplicationBacklogChart.ID)
		c.removeNamespaceChart(nsReplicationRateChart.ID)
		c.removeNamespaceChart(nsReplicationThroughputChart.ID)
		c.removeNamespaceChart(nsReplicationBacklogChart.ID)
		c.removeNamespaceChart(topicReplicationRateInChart.ID)
		c.removeNamespaceChart(topicReplicationRateOutChart.ID)
		c.removeNamespaceChart(topicReplicationThroughputRateInChart.ID)
		c.removeNamespaceChart(topicReplicationThroughputRateOutChart.ID)
		c.removeNamespaceChart(topicReplicationBacklogChart.ID)
		delete(c.topicChartsMapping, topicReplicationRateInChart.ID)
		delete(c.topicChartsMapping, topicReplicationRateOutChart.ID)
		delete(c.topicChartsMapping, topicReplicationThroughputRateInChart.ID)
		delete(c.topicChartsMapping, topicReplicationThroughputRateOutChart.ID)
		delete(c.topicChartsMapping, topicReplicationBacklogChart.ID)
	}
}

func (c *Collector) removeSummaryChart(chartID string) {
	if err := c.Charts().Remove(chartID); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeNamespaceChart(chartID string) {
	if err := c.nsCharts.Remove(chartID); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) updateCharts() {
	// NOTE: order is important
	for ns := range c.curCache.namespaces {
		if !c.cache.namespaces[ns] {
			c.cache.namespaces[ns] = true
			c.addNamespaceCharts(ns)
		}
	}
	for top := range c.curCache.topics {
		if !c.cache.topics[top] {
			c.cache.topics[top] = true
			c.addTopicToCharts(top)
		}
	}
	for top := range c.cache.topics {
		if c.curCache.topics[top] {
			continue
		}
		delete(c.cache.topics, top)
		c.removeTopicFromCharts(top)
	}
	for ns := range c.cache.namespaces {
		if c.curCache.namespaces[ns] {
			continue
		}
		delete(c.cache.namespaces, ns)
		c.removeNamespaceFromCharts(ns)
	}
}

func (c *Collector) addNamespaceCharts(ns namespace) {
	charts := c.nsCharts.Copy()
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, ns.name)
		chart.Fam = fmt.Sprintf(chart.Fam, ns.name)
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, ns.name)
		}
	}
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeNamespaceFromCharts(ns namespace) {
	for _, chart := range *c.nsCharts {
		id := fmt.Sprintf(chart.ID, ns.name)
		if chart = c.Charts().Get(id); chart != nil {
			chart.MarkRemove()
		} else {
			c.Warningf("could not remove namespace chart '%s'", id)
		}
	}
}

func (c *Collector) addTopicToCharts(top topic) {
	for id, metric := range c.topicChartsMapping {
		id = fmt.Sprintf(id, top.namespace)
		chart := c.Charts().Get(id)
		if chart == nil {
			c.Warningf("could not add topic '%s' to chart '%s': chart not found", top.name, id)
			continue
		}

		dim := Dim{ID: metric + "_" + top.name, Name: extractTopicName(top)}
		switch metric {
		case metricPulsarThroughputIn,
			metricPulsarThroughputOut,
			metricPulsarReplicationThroughputIn,
			metricPulsarReplicationThroughputOut:
			dim.Div = 1024 * 1000
		case metricPulsarRateIn,
			metricPulsarRateOut,
			metricPulsarStorageWriteRate,
			metricPulsarStorageReadRate,
			metricPulsarSubscriptionMsgRateRedeliver,
			metricPulsarReplicationRateIn,
			metricPulsarReplicationRateOut:
			dim.Div = 1000
		case metricPulsarStorageSize:
			dim.Div = 1024
		}

		if err := chart.AddDim(&dim); err != nil {
			c.Warning(err)
		}
		chart.MarkNotCreated()
	}
}

func (c *Collector) removeTopicFromCharts(top topic) {
	for id, metric := range c.topicChartsMapping {
		id = fmt.Sprintf(id, top.namespace)
		chart := c.Charts().Get(id)
		if chart == nil {
			c.Warningf("could not remove topic '%s' from chart '%s': chart not found", top.name, id)
			continue
		}

		if err := chart.MarkDimRemove(metric+"_"+top.name, true); err != nil {
			c.Warning(err)
		}
		chart.MarkNotCreated()
	}
}

func topicChartsMapping() map[string]string {
	return map[string]string{
		topicSubscriptionsChart.ID:                metricPulsarSubscriptionsCount,
		topicProducersChart.ID:                    metricPulsarProducersCount,
		topicConsumersChart.ID:                    metricPulsarConsumersCount,
		topicMessagesRateInChart.ID:               metricPulsarRateIn,
		topicMessagesRateOutChart.ID:              metricPulsarRateOut,
		topicThroughputRateInChart.ID:             metricPulsarThroughputIn,
		topicThroughputRateOutChart.ID:            metricPulsarThroughputOut,
		topicStorageSizeChart.ID:                  metricPulsarStorageSize,
		topicStorageReadRateChart.ID:              metricPulsarStorageReadRate,
		topicStorageWriteRateChart.ID:             metricPulsarStorageWriteRate,
		topicMsgBacklogSizeChart.ID:               metricPulsarMsgBacklog,
		topicSubsDelayedChart.ID:                  metricPulsarSubscriptionDelayed,
		topicSubsMsgRateRedeliverChart.ID:         metricPulsarSubscriptionMsgRateRedeliver,
		topicSubsBlockedOnUnackedMsgChart.ID:      metricPulsarSubscriptionBlockedOnUnackedMessages,
		topicReplicationRateInChart.ID:            metricPulsarReplicationRateIn,
		topicReplicationRateOutChart.ID:           metricPulsarReplicationRateOut,
		topicReplicationThroughputRateInChart.ID:  metricPulsarReplicationThroughputIn,
		topicReplicationThroughputRateOutChart.ID: metricPulsarReplicationThroughputOut,
		topicReplicationBacklogChart.ID:           metricPulsarReplicationBacklog,
	}
}

func extractTopicName(top topic) string {
	// persistent://sample/ns1/demo-1 => p:demo-1
	if idx := strings.LastIndexByte(top.name, '/'); idx > 0 {
		return top.name[:1] + ":" + top.name[idx+1:]
	}
	return top.name
}
