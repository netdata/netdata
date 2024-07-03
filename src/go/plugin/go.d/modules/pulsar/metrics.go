// SPDX-License-Identifier: GPL-3.0-or-later

package pulsar

/*
Architecture:
 - https://pulsar.apache.org/docs/en/concepts-overview/

Terminology:
 - https://pulsar.apache.org/docs/en/reference-terminology/

Deploy Monitoring:
 - http://pulsar.apache.org/docs/en/deploy-monitoring/

Metrics Reference:
 - https://github.com/apache/pulsar/blob/master/site2/docs/reference-metrics.md

REST API
 - http://pulsar.apache.org/admin-rest-api/?version=master

Grafana Dashboards:
 - https://github.com/apache/pulsar/tree/master/docker/grafana/dashboards

Stats in the source code:
 - https://github.com/apache/pulsar/blob/master/pulsar-common/src/main/java/org/apache/pulsar/common/policies/data/
 - https://github.com/apache/pulsar/tree/master/pulsar-broker/src/main/java/org/apache/pulsar/broker/stats/prometheus

If !'exposeTopicLevelMetricsInPrometheus:
 - https://github.com/apache/pulsar/blob/master/pulsar-broker/src/main/java/org/apache/pulsar/broker/stats/prometheus/NamespaceStatsAggregator.java
else:
 - https://github.com/apache/pulsar/blob/master/pulsar-broker/src/main/java/org/apache/pulsar/broker/stats/prometheus/TopicStats.java

Metrics updates parameters:
 - statsUpdateFrequencyInSecs=60
 - statsUpdateInitialDelayInSecs=60

Metrics Exposing:
 - Namespace   : 'exposeTopicLevelMetricsInPrometheus' is set to false.
 - Replication : 'replicationMetricsEnabled' is enabled.
 - Topic       : 'exposeTopicLevelMetricsInPrometheus' is set to true.
 - Subscription: 'exposeTopicLevelMetricsInPrometheus' is set to true
 - Consumer    : 'exposeTopicLevelMetricsInPrometheus' and 'exposeConsumerLevelMetricsInPrometheus' are set to true.
 - Publisher   : 'exposePublisherStats' is set to true. RESP API option. (/admin/v2/broker-stats/topics)
*/

/*
TODO:
Unused broker metrics:
 - "pulsar_storage_backlog_size"           : ?? is estimated total unconsumed or backlog size in bytes for the managed ledger, without accounting for replicas.
 - "pulsar_storage_offloaded_size"         : ?? is the size of all ledgers offloaded to 2nd tier storage.
 - "pulsar_storage_backlog_quota_limit"    : ?? is the total amount of the data in this topic that limit the backlog quota.
 - "pulsar_in_bytes_total"                 : use "pulsar_throughput_in" for the same data.
 - "pulsar_in_messages_total"              : use "pulsar_rate_in" for the same data.
 - "pulsar_subscription_unacked_messages"  : negative values (https://github.com/apache/pulsar/issues/6510)
 - "pulsar_subscription_back_log"          : to detailed, we have summary per topic. Part of "pulsar_msg_backlog" (msgBacklog).
 - "pulsar_subscription_msg_rate_out"      : to detailed, we have summary per topic. Part of "pulsar_rate_out".
 - "pulsar_subscription_msg_throughput_out": to detailed, we have summary per topic. Part of "pulsar_throughput_out".

 + All Consumer metrics (for each namespace, topic, subscription).
 + JVM metrics.
 + Zookeeper metrics.
 + Bookkeeper metrics.

Hardcoded update interval? (60)
 - pulsar_storage_write_latency_le_*
 - pulsar_entry_size_le_*
*/

/*
https://github.com/apache/pulsar/blob/master/pulsar-broker/src/main/java/org/apache/pulsar/broker/stats/prometheus/NamespaceStatsAggregator.java
Zero metrics which always present (labels: cluster):
 - "pulsar_topics_count"
 - "pulsar_subscriptions_count"
 - "pulsar_producers_count"
 - "pulsar_consumers_count"
 - "pulsar_rate_in"
 - "pulsar_rate_out"
 - "pulsar_throughput_in"
 - "pulsar_throughput_out"
 - "pulsar_storage_size"
 - "pulsar_storage_write_rate"
 - "pulsar_storage_read_rate"
 - "pulsar_msg_backlog"
*/

const (
	// Namespace metrics (labels: namespace)
	metricPulsarTopicsCount = "pulsar_topics_count"
	// Namespace, Topic metrics (labels: namespace || namespace, topic)
	metricPulsarSubscriptionsCount = "pulsar_subscriptions_count"
	metricPulsarProducersCount     = "pulsar_producers_count"
	metricPulsarConsumersCount     = "pulsar_consumers_count"
	metricPulsarRateIn             = "pulsar_rate_in"
	metricPulsarRateOut            = "pulsar_rate_out"
	metricPulsarThroughputIn       = "pulsar_throughput_in"
	metricPulsarThroughputOut      = "pulsar_throughput_out"
	metricPulsarStorageSize        = "pulsar_storage_size"
	metricPulsarStorageWriteRate   = "pulsar_storage_write_rate" // exposed with labels only if there is Bookie
	metricPulsarStorageReadRate    = "pulsar_storage_read_rate"  // exposed with labels only if there is Bookie
	metricPulsarMsgBacklog         = "pulsar_msg_backlog"        // has 'remote_cluster' label if no topic stats
	// pulsar_storage_write_latency_le_*
	// pulsar_entry_size_le_*

	// Subscriptions metrics (labels: namespace, topic, subscription)
	metricPulsarSubscriptionDelayed                  = "pulsar_subscription_delayed" // Number of delayed messages currently being tracked
	metricPulsarSubscriptionMsgRateRedeliver         = "pulsar_subscription_msg_rate_redeliver"
	metricPulsarSubscriptionBlockedOnUnackedMessages = "pulsar_subscription_blocked_on_unacked_messages"

	// Replication metrics (labels: namespace, remote_cluster || namespace, topic, remote_cluster)
	// Exposed only when replication is enabled.
	metricPulsarReplicationRateIn        = "pulsar_replication_rate_in"
	metricPulsarReplicationRateOut       = "pulsar_replication_rate_out"
	metricPulsarReplicationThroughputIn  = "pulsar_replication_throughput_in"
	metricPulsarReplicationThroughputOut = "pulsar_replication_throughput_out"
	metricPulsarReplicationBacklog       = "pulsar_replication_backlog"
)
