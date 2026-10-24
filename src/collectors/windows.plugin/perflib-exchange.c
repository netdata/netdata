// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

static void exchange_instance_chart_id(
    char *id, size_t size, const char *group, const char *instance, const char *metric)
{
    char sanitized[RRD_ID_LENGTH_MAX + 1];
    snprintfz(sanitized, sizeof(sanitized), "%s", instance);
    netdata_fix_chart_id(sanitized);

    // Reserve the chart type, metric, and raw-name hash even when the source name is long.
    size_t id_limit = RRD_ID_LENGTH_MAX - strlen("exchange.");
    if (size > id_limit)
        size = id_limit;
    size_t reserved = strlen("exchange_") + strlen(group) + strlen(metric) + 3 + 16 + 1;
    size_t instance_length = size > reserved ? size - reserved : 0;
    XXH64_hash_t hash = XXH3_64bits(instance, strlen(instance));
    snprintfz(
        id,
        size,
        "exchange_%s_%.*s_%s_%016llx",
        group,
        (int)instance_length,
        sanitized,
        metric,
        (unsigned long long)hash);
}

struct exchange_proxy {
    usec_t last_collected;

    RRDSET *st_exchange_http_proxy_avg_auth_latency;
    RRDSET *st_exchange_http_proxy_avg_cas_processing_latency;
    RRDSET *st_exchange_http_proxy_mailbox_proxy_failure;
    RRDSET *st_exchange_http_proxy_server_location_avg_latency;
    RRDSET *st_exchange_http_proxy_outstanding_proxy_requests;
    RRDSET *st_exchange_http_proxy_requests_total;

    RRDDIM *rd_exchange_http_proxy_avg_auth_latency;
    RRDDIM *rd_exchange_http_proxy_avg_cas_processing_latency;
    RRDDIM *rd_exchange_http_proxy_mailbox_proxy_failure;
    RRDDIM *rd_exchange_http_proxy_server_location_avg_latency;
    RRDDIM *rd_exchange_http_proxy_outstanding_proxy_requests;
    RRDDIM *rd_exchange_http_proxy_requests_total;

    COUNTER_DATA exchangeProxyAvgAuthLatency;
    COUNTER_DATA exchangeProxyAvgCasProcessingLatency;
    COUNTER_DATA exchangeProxyMailboxProxyFailure;
    COUNTER_DATA exchangeProxyMailboxServerLocator;
    COUNTER_DATA exchangeProxyOutstandingProxy;
    COUNTER_DATA exchangeProxyRequestsTotal;
};

struct exchange_workload {
    usec_t last_collected;

    RRDSET *st_exchange_workload_active_tasks;
    RRDSET *st_exchange_workload_complete_tasks;
    RRDSET *st_exchange_workload_queued_tasks;
    RRDSET *st_exchange_workload_yielded_tasks;
    RRDSET *st_exchange_workload_activity_status;

    RRDDIM *rd_exchange_workload_active_tasks;
    RRDDIM *rd_exchange_workload_complete_tasks;
    RRDDIM *rd_exchange_workload_queued_tasks;
    RRDDIM *rd_exchange_workload_yielded_tasks;
    RRDDIM *rd_exchange_workload_activity_active_status;
    RRDDIM *rd_exchange_workload_activity_paused_status;

    COUNTER_DATA exchangeWorkloadActiveTasks;
    COUNTER_DATA exchangeWorkloadCompleteTasks;
    COUNTER_DATA exchangeWorkloadQueueTasks;
    COUNTER_DATA exchangeWorkloadYieldedTasks;
    COUNTER_DATA exchangeWorkloadActivityStatus;
};

struct exchange_queue {
    usec_t last_collected;

    RRDSET *st_exchange_queue_active_mailbox;
    RRDSET *st_exchange_queue_external_active_remote_delivery;
    RRDSET *st_exchange_queue_external_largest_delivery;
    RRDSET *st_exchange_queue_internal_active_remote_delivery;
    RRDSET *st_exchange_queue_internal_largest_delivery;
    RRDSET *st_exchange_queue_retry_mailbox_delivery;
    RRDSET *st_exchange_queue_unreachable;
    RRDSET *st_exchange_queue_poison;
    RRDSET *st_exchange_queue_messages_queued_for_delivery_total;
    RRDSET *st_exchange_queue_messages_submitted_total;
    RRDSET *st_exchange_queue_messages_delayed_total;
    RRDSET *st_exchange_queue_messages_completed_delivery_total;
    RRDSET *st_exchange_queue_aggregate_shadow_queue_length;
    RRDSET *st_exchange_queue_submission_queue_length;
    RRDSET *st_exchange_queue_delay_queue_length;
    RRDSET *st_exchange_queue_items_completed_delivery_total;
    RRDSET *st_exchange_queue_items_queued_for_delivery_expired_total;
    RRDSET *st_exchange_queue_items_queued_for_delivery_total;
    RRDSET *st_exchange_queue_items_resubmitted_total;

    RRDDIM *rd_exchange_queue_active_mailbox;
    RRDDIM *rd_exchange_queue_external_active_remote_delivery;
    RRDDIM *rd_exchange_queue_external_largest_delivery;
    RRDDIM *rd_exchange_queue_internal_active_remote_delivery;
    RRDDIM *rd_exchange_queue_internal_largest_delivery;
    RRDDIM *rd_exchange_queue_retry_mailbox_delivery;
    RRDDIM *rd_exchange_queue_unreachable;
    RRDDIM *rd_exchange_queue_poison;
    RRDDIM *rd_exchange_queue_messages_queued_for_delivery_total;
    RRDDIM *rd_exchange_queue_messages_submitted_total;
    RRDDIM *rd_exchange_queue_messages_delayed_total;
    RRDDIM *rd_exchange_queue_messages_completed_delivery_total;
    RRDDIM *rd_exchange_queue_aggregate_shadow_queue_length;
    RRDDIM *rd_exchange_queue_submission_queue_length;
    RRDDIM *rd_exchange_queue_delay_queue_length;
    RRDDIM *rd_exchange_queue_items_completed_delivery_total;
    RRDDIM *rd_exchange_queue_items_queued_for_delivery_expired_total;
    RRDDIM *rd_exchange_queue_items_queued_for_delivery_total;
    RRDDIM *rd_exchange_queue_items_resubmitted_total;

    COUNTER_DATA exchangeTransportQueuesActiveMailboxDelivery;
    COUNTER_DATA exchangeTransportQueuesExternalActiveRemoteDelivery;
    COUNTER_DATA exchangeTransportQueuesExternalLargestDelivery;
    COUNTER_DATA exchangeTransportQueuesInternalActiveRemoteDelivery;
    COUNTER_DATA exchangeTransportQueuesInternalLargestDelivery;
    COUNTER_DATA exchangeTransportQueuesRetryMailboxDelivery;
    COUNTER_DATA exchangeTransportQueuesUnreachable;
    COUNTER_DATA exchangeTransportQueuesPoison;
    COUNTER_DATA exchangeTransportQueuesMessagesQueuedForDeliveryTotal;
    COUNTER_DATA exchangeTransportQueuesMessagesSubmittedTotal;
    COUNTER_DATA exchangeTransportQueuesMessagesDelayedTotal;
    COUNTER_DATA exchangeTransportQueuesMessagesCompletedDeliveryTotal;
    COUNTER_DATA exchangeTransportQueuesAggregateShadowQueueLength;
    COUNTER_DATA exchangeTransportQueuesSubmissionQueueLength;
    COUNTER_DATA exchangeTransportQueuesDelayQueueLength;
    COUNTER_DATA exchangeTransportQueuesItemsCompletedDeliveryTotal;
    COUNTER_DATA exchangeTransportQueuesItemsQueuedForDeliveryExpiredTotal;
    COUNTER_DATA exchangeTransportQueuesItemsQueuedForDeliveryTotal;
    COUNTER_DATA exchangeTransportQueuesItemsResubmittedTotal;
};

struct exchange_ad_access_process {
    usec_t last_collected;

    RRDSET *st_exchange_ldap_read_time;
    RRDSET *st_exchange_ldap_search_time;
    RRDSET *st_exchange_ldap_write_time;
    RRDSET *st_exchange_ldap_read_operations;
    RRDSET *st_exchange_ldap_search_operations;
    RRDSET *st_exchange_ldap_write_operations;
    RRDSET *st_exchange_ldap_timeout_errors;
    RRDSET *st_exchange_ldap_long_running_ops;

    RRDDIM *rd_exchange_ldap_read_time;
    RRDDIM *rd_exchange_ldap_search_time;
    RRDDIM *rd_exchange_ldap_write_time;
    RRDDIM *rd_exchange_ldap_read_operations;
    RRDDIM *rd_exchange_ldap_search_operations;
    RRDDIM *rd_exchange_ldap_write_operations;
    RRDDIM *rd_exchange_ldap_timeout_errors;
    RRDDIM *rd_exchange_ldap_long_running_ops;

    COUNTER_DATA exchangeLDAPReadTime;
    COUNTER_DATA exchangeLDAPSearchTime;
    COUNTER_DATA exchangeLDAPWriteTime;
    COUNTER_DATA exchangeLDAPTimeoutErrors;
    COUNTER_DATA exchangeLDAPLongRunningOps;
};

DICTIONARY *exchange_proxies;
DICTIONARY *exchange_workloads;
DICTIONARY *exchange_queues;
DICTIONARY *exchange_ad_access_processes;

static void exchange_proxy_initialize_variables(struct exchange_proxy *ep)
{
    ep->exchangeProxyAvgAuthLatency.key = "Average Authentication Latency";
    ep->exchangeProxyAvgCasProcessingLatency.key = "Average ClientAccess Server Processing Latency";
    ep->exchangeProxyMailboxProxyFailure.key = "Mailbox Server Proxy Failure Rate";
    ep->exchangeProxyMailboxServerLocator.key = "MailboxServerLocator Average Latency (Moving Average)";
    ep->exchangeProxyOutstandingProxy.key = "Outstanding Proxy Requests";
    ep->exchangeProxyRequestsTotal.key = "Proxy Requests/Sec";
}

static void
dict_exchange_insert_proxy_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct exchange_proxy *ep = value;

    exchange_proxy_initialize_variables(ep);
}

static void dict_exchange_delete_proxy_cb(
    const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct exchange_proxy *ep = value;
    rrdset_is_obsolete___safe_from_collector_thread(ep->st_exchange_http_proxy_avg_auth_latency);
    rrdset_is_obsolete___safe_from_collector_thread(ep->st_exchange_http_proxy_avg_cas_processing_latency);
    rrdset_is_obsolete___safe_from_collector_thread(ep->st_exchange_http_proxy_mailbox_proxy_failure);
    rrdset_is_obsolete___safe_from_collector_thread(ep->st_exchange_http_proxy_server_location_avg_latency);
    rrdset_is_obsolete___safe_from_collector_thread(ep->st_exchange_http_proxy_outstanding_proxy_requests);
    rrdset_is_obsolete___safe_from_collector_thread(ep->st_exchange_http_proxy_requests_total);
}

static void exchange_workload_initialize_variables(struct exchange_workload *ew)
{
    ew->exchangeWorkloadActiveTasks.key = "ActiveTasks";
    ew->exchangeWorkloadCompleteTasks.key = "CompletedTasks";
    ew->exchangeWorkloadQueueTasks.key = "QueuedTasks";
    ew->exchangeWorkloadYieldedTasks.key = "YieldedTasks";
    ew->exchangeWorkloadActivityStatus.key = "Active";
}

static void
dict_exchange_insert_workload_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct exchange_workload *ew = value;

    exchange_workload_initialize_variables(ew);
}

static void dict_exchange_delete_workload_cb(
    const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct exchange_workload *ew = value;
    rrdset_is_obsolete___safe_from_collector_thread(ew->st_exchange_workload_active_tasks);
    rrdset_is_obsolete___safe_from_collector_thread(ew->st_exchange_workload_complete_tasks);
    rrdset_is_obsolete___safe_from_collector_thread(ew->st_exchange_workload_queued_tasks);
    rrdset_is_obsolete___safe_from_collector_thread(ew->st_exchange_workload_yielded_tasks);
    rrdset_is_obsolete___safe_from_collector_thread(ew->st_exchange_workload_activity_status);
}

static void exchange_queue_initialize_variables(struct exchange_queue *eq)
{
    eq->exchangeTransportQueuesActiveMailboxDelivery.key = "Active Mailbox Delivery Queue Length";
    eq->exchangeTransportQueuesExternalActiveRemoteDelivery.key = "External Active Remote Delivery Queue Length";
    eq->exchangeTransportQueuesExternalLargestDelivery.key = "External Largest Delivery Queue Length";
    eq->exchangeTransportQueuesInternalActiveRemoteDelivery.key = "Internal Active Remote Delivery Queue Length";
    eq->exchangeTransportQueuesInternalLargestDelivery.key = "Internal Largest Delivery Queue Length";
    eq->exchangeTransportQueuesRetryMailboxDelivery.key = "Retry Mailbox Delivery Queue Length";
    eq->exchangeTransportQueuesPoison.key = "Poison Queue Length";
    eq->exchangeTransportQueuesUnreachable.key = "Unreachable Queue Length";
    eq->exchangeTransportQueuesMessagesQueuedForDeliveryTotal.key = "Messages Queued For Delivery Total";
    eq->exchangeTransportQueuesMessagesSubmittedTotal.key = "Messages Submitted Total";
    eq->exchangeTransportQueuesMessagesDelayedTotal.key = "Messages Delayed Total";
    eq->exchangeTransportQueuesMessagesCompletedDeliveryTotal.key = "Messages Completed Delivery Total";
    eq->exchangeTransportQueuesAggregateShadowQueueLength.key = "Aggregate Shadow Queue Length";
    eq->exchangeTransportQueuesSubmissionQueueLength.key = "Submission Queue Length";
    eq->exchangeTransportQueuesDelayQueueLength.key = "Delay Queue Length";
    eq->exchangeTransportQueuesItemsCompletedDeliveryTotal.key = "Items Completed Delivery Total";
    eq->exchangeTransportQueuesItemsQueuedForDeliveryExpiredTotal.key = "Items Queued For Delivery Expired Total";
    eq->exchangeTransportQueuesItemsQueuedForDeliveryTotal.key = "Items Queued For Delivery Total";
    eq->exchangeTransportQueuesItemsResubmittedTotal.key = "Items Resubmitted Total";
}

static void
dict_exchange_insert_queue_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct exchange_queue *eq = value;

    exchange_queue_initialize_variables(eq);
}

static void dict_exchange_delete_queue_cb(
    const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct exchange_queue *eq = value;
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_active_mailbox);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_external_active_remote_delivery);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_external_largest_delivery);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_internal_active_remote_delivery);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_internal_largest_delivery);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_retry_mailbox_delivery);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_unreachable);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_poison);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_messages_queued_for_delivery_total);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_messages_submitted_total);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_messages_delayed_total);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_messages_completed_delivery_total);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_aggregate_shadow_queue_length);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_submission_queue_length);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_delay_queue_length);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_items_completed_delivery_total);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_items_queued_for_delivery_expired_total);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_items_queued_for_delivery_total);
    rrdset_is_obsolete___safe_from_collector_thread(eq->st_exchange_queue_items_resubmitted_total);
}

static void exchange_ad_access_initialize_variables(struct exchange_ad_access_process *ea)
{
    ea->exchangeLDAPReadTime.key = "LDAP Read Time";
    ea->exchangeLDAPSearchTime.key = "LDAP Search Time";
    ea->exchangeLDAPWriteTime.key = "LDAP Write Time";
    ea->exchangeLDAPTimeoutErrors.key = "LDAP Timeout Errors/sec";
    ea->exchangeLDAPLongRunningOps.key = "Long Running LDAP Operations/min";
}

static void dict_exchange_insert_ad_access_cb(
    const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct exchange_ad_access_process *ea = value;

    exchange_ad_access_initialize_variables(ea);
}

static void dict_exchange_delete_ad_access_cb(
    const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct exchange_ad_access_process *ea = value;
    rrdset_is_obsolete___safe_from_collector_thread(ea->st_exchange_ldap_read_time);
    rrdset_is_obsolete___safe_from_collector_thread(ea->st_exchange_ldap_search_time);
    rrdset_is_obsolete___safe_from_collector_thread(ea->st_exchange_ldap_write_time);
    rrdset_is_obsolete___safe_from_collector_thread(ea->st_exchange_ldap_read_operations);
    rrdset_is_obsolete___safe_from_collector_thread(ea->st_exchange_ldap_search_operations);
    rrdset_is_obsolete___safe_from_collector_thread(ea->st_exchange_ldap_write_operations);
    rrdset_is_obsolete___safe_from_collector_thread(ea->st_exchange_ldap_timeout_errors);
    rrdset_is_obsolete___safe_from_collector_thread(ea->st_exchange_ldap_long_running_ops);
}

static void initialize(void)
{
    exchange_proxies = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct exchange_proxy));
    dictionary_register_insert_callback(exchange_proxies, dict_exchange_insert_proxy_cb, NULL);
    dictionary_register_delete_callback(exchange_proxies, dict_exchange_delete_proxy_cb, NULL);

    exchange_workloads = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct exchange_workload));
    dictionary_register_insert_callback(exchange_workloads, dict_exchange_insert_workload_cb, NULL);
    dictionary_register_delete_callback(exchange_workloads, dict_exchange_delete_workload_cb, NULL);

    exchange_queues = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct exchange_queue));
    dictionary_register_insert_callback(exchange_queues, dict_exchange_insert_queue_cb, NULL);
    dictionary_register_delete_callback(exchange_queues, dict_exchange_delete_queue_cb, NULL);

    exchange_ad_access_processes = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct exchange_ad_access_process));
    dictionary_register_insert_callback(exchange_ad_access_processes, dict_exchange_insert_ad_access_cb, NULL);
    dictionary_register_delete_callback(exchange_ad_access_processes, dict_exchange_delete_ad_access_cb, NULL);
}

#define EXCHANGE_TIME_PRECISION 1000000

static bool exchange_ldap_time_microseconds(const COUNTER_DATA *counter, collected_number *value)
{
    switch (counter->current.CounterType) {
        case PERF_AVERAGE_TIMER:
        case PERF_AVERAGE_BULK: {
            if (counter->previous.CounterType != counter->current.CounterType ||
                counter->current.Data < counter->previous.Data || counter->current.Time <= counter->previous.Time ||
                (counter->current.CounterType == PERF_AVERAGE_TIMER && counter->current.Frequency <= 0))
                return false;

            double scale = counter->current.CounterType == PERF_AVERAGE_TIMER ?
                               (double)EXCHANGE_TIME_PRECISION / (double)counter->current.Frequency :
                               1000.0;
            double microseconds =
                (double)(counter->current.Data - counter->previous.Data) * scale /
                (double)(counter->current.Time - counter->previous.Time);
            if (microseconds > (double)INT64_MAX)
                return false;

            *value = (collected_number)microseconds;
            return true;
        }

        case PERF_COUNTER_RAWCOUNT:
        case PERF_COUNTER_LARGE_RAWCOUNT:
            if (counter->current.Data > (uint64_t)INT64_MAX / 1000)
                return false;

            *value = (collected_number)(counter->current.Data * 1000);
            return true;

        default:
            return false;
    }
}

static void netdata_exchange_ldap_operations(
    COUNTER_DATA *counter,
    RRDSET **st,
    RRDDIM **rd,
    const char *process,
    const char *operation,
    const char *title,
    int priority,
    int update_every)
{
    if (counter->current.CounterType != PERF_AVERAGE_TIMER && counter->current.CounterType != PERF_AVERAGE_BULK)
        return;

    if (unlikely(!*st)) {
        char metric[64], context[80], id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(metric, sizeof(metric), "ldap_%s_operations_total", operation);
        snprintfz(context, sizeof(context), "exchange.%s", metric);
        exchange_instance_chart_id(id, sizeof(id), "ad_access", process, metric);

        *st = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "ad_access",
            context,
            title,
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            priority,
            update_every,
            RRDSET_TYPE_LINE);

        *rd = rrddim_add(*st, "operations", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rrdlabels_add((*st)->rrdlabels, "process", process, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(*st, *rd, (collected_number)counter->current.Time);
    rrdset_done(*st);
}

static bool exchange_ldap_rate_format(
    const COUNTER_DATA *counter, bool source_per_minute, RRD_ALGORITHM *algorithm, collected_number *divider)
{
    switch (counter->current.CounterType) {
        case PERF_COUNTER_COUNTER:
        case PERF_COUNTER_BULK_COUNT:
        case PERF_SAMPLE_COUNTER:
            *algorithm = RRD_ALGORITHM_INCREMENTAL;
            *divider = 1;
            return true;

        case PERF_COUNTER_RAWCOUNT:
        case PERF_COUNTER_LARGE_RAWCOUNT:
            *algorithm = RRD_ALGORITHM_ABSOLUTE;
            // Raw values retain the provider's per-minute unit; cumulative values above become per-second rates.
            *divider = source_per_minute ? 60 : 1;
            return true;

        default:
            return false;
    }
}

static void netdata_exchange_ldap_read_time(struct exchange_ad_access_process *ea, char *process, int update_every)
{
    collected_number microseconds;
    if (!exchange_ldap_time_microseconds(&ea->exchangeLDAPReadTime, &microseconds))
        return;

    if (unlikely(!ea->st_exchange_ldap_read_time)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "ad_access", process, "ldap_read_time_sec");

        ea->st_exchange_ldap_read_time = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "ad_access",
            "exchange.ldap_read_time_sec",
            "Time to send an LDAP read request and receive a response.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_LDAP_READ_TIME,
            update_every,
            RRDSET_TYPE_LINE);

        ea->rd_exchange_ldap_read_time = rrddim_add(
            ea->st_exchange_ldap_read_time, "read", NULL, 1, EXCHANGE_TIME_PRECISION, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(ea->st_exchange_ldap_read_time->rrdlabels, "process", process, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(ea->st_exchange_ldap_read_time, ea->rd_exchange_ldap_read_time, microseconds);
    rrdset_done(ea->st_exchange_ldap_read_time);
}

static void netdata_exchange_ldap_search_time(struct exchange_ad_access_process *ea, char *process, int update_every)
{
    collected_number microseconds;
    if (!exchange_ldap_time_microseconds(&ea->exchangeLDAPSearchTime, &microseconds))
        return;

    if (unlikely(!ea->st_exchange_ldap_search_time)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "ad_access", process, "ldap_search_time_sec");

        ea->st_exchange_ldap_search_time = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "ad_access",
            "exchange.ldap_search_time_sec",
            "Time to send an LDAP search request and receive a response.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_LDAP_SEARCH_TIME,
            update_every,
            RRDSET_TYPE_LINE);

        ea->rd_exchange_ldap_search_time = rrddim_add(
            ea->st_exchange_ldap_search_time, "search", NULL, 1, EXCHANGE_TIME_PRECISION, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(ea->st_exchange_ldap_search_time->rrdlabels, "process", process, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(ea->st_exchange_ldap_search_time, ea->rd_exchange_ldap_search_time, microseconds);
    rrdset_done(ea->st_exchange_ldap_search_time);
}

static void netdata_exchange_ldap_write_time(struct exchange_ad_access_process *ea, char *process, int update_every)
{
    collected_number microseconds;
    if (!exchange_ldap_time_microseconds(&ea->exchangeLDAPWriteTime, &microseconds))
        return;

    if (unlikely(!ea->st_exchange_ldap_write_time)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "ad_access", process, "ldap_write_time_sec");

        ea->st_exchange_ldap_write_time = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "ad_access",
            "exchange.ldap_write_time_sec",
            "Time to send an LDAP add, modify, or delete request and receive a response.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_LDAP_WRITE_TIME,
            update_every,
            RRDSET_TYPE_LINE);

        ea->rd_exchange_ldap_write_time = rrddim_add(
            ea->st_exchange_ldap_write_time, "write", NULL, 1, EXCHANGE_TIME_PRECISION, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(ea->st_exchange_ldap_write_time->rrdlabels, "process", process, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(ea->st_exchange_ldap_write_time, ea->rd_exchange_ldap_write_time, microseconds);
    rrdset_done(ea->st_exchange_ldap_write_time);
}

static void netdata_exchange_ldap_timeout_errors(struct exchange_ad_access_process *ea, char *process, int update_every)
{
    RRD_ALGORITHM algorithm;
    collected_number divider;
    if (!exchange_ldap_rate_format(&ea->exchangeLDAPTimeoutErrors, false, &algorithm, &divider))
        return;

    if (unlikely(!ea->st_exchange_ldap_timeout_errors)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "ad_access", process, "ldap_timeout_errors_total");

        ea->st_exchange_ldap_timeout_errors = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "ad_access",
            "exchange.ldap_timeout_errors_total",
            "LDAP timeout errors.",
            "errors/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_LDAP_TIMEOUT_ERRORS,
            update_every,
            RRDSET_TYPE_LINE);

        ea->rd_exchange_ldap_timeout_errors =
            rrddim_add(ea->st_exchange_ldap_timeout_errors, "timeouts", NULL, 1, divider, algorithm);

        rrdlabels_add(ea->st_exchange_ldap_timeout_errors->rrdlabels, "process", process, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ea->st_exchange_ldap_timeout_errors,
        ea->rd_exchange_ldap_timeout_errors,
        (collected_number)ea->exchangeLDAPTimeoutErrors.current.Data);
    rrdset_done(ea->st_exchange_ldap_timeout_errors);
}

static void netdata_exchange_ldap_long_running_ops(
    struct exchange_ad_access_process *ea, char *process, int update_every)
{
    RRD_ALGORITHM algorithm;
    collected_number divider;
    if (!exchange_ldap_rate_format(&ea->exchangeLDAPLongRunningOps, true, &algorithm, &divider))
        return;

    if (unlikely(!ea->st_exchange_ldap_long_running_ops)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "ad_access", process, "ldap_long_running_ops_per_sec");

        ea->st_exchange_ldap_long_running_ops = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "ad_access",
            "exchange.ldap_long_running_ops_per_sec",
            "Long running LDAP operations.",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_LDAP_LONG_RUNNING_OPS,
            update_every,
            RRDSET_TYPE_LINE);

        ea->rd_exchange_ldap_long_running_ops =
            rrddim_add(ea->st_exchange_ldap_long_running_ops, "operations", NULL, 1, divider, algorithm);

        rrdlabels_add(ea->st_exchange_ldap_long_running_ops->rrdlabels, "process", process, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ea->st_exchange_ldap_long_running_ops,
        ea->rd_exchange_ldap_long_running_ops,
        (collected_number)ea->exchangeLDAPLongRunningOps.current.Data);
    rrdset_done(ea->st_exchange_ldap_long_running_ops);
}

static void netdata_exchange_ad_access(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    usec_t now_ut = now_monotonic_usec();
    bool complete = true;
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi) {
            complete = false;
            break;
        }

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer))) {
            complete = false;
            continue;
        }

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct exchange_ad_access_process *ea =
            dictionary_set(exchange_ad_access_processes, windows_shared_buffer, NULL, sizeof(*ea));
        if (!ea) {
            complete = false;
            continue;
        }
        ea->last_collected = now_ut;

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ea->exchangeLDAPReadTime)) {
            netdata_exchange_ldap_read_time(ea, windows_shared_buffer, update_every);
            netdata_exchange_ldap_operations(
                &ea->exchangeLDAPReadTime,
                &ea->st_exchange_ldap_read_operations,
                &ea->rd_exchange_ldap_read_operations,
                windows_shared_buffer,
                "read",
                "LDAP read operations.",
                PRIO_EXCHANGE_LDAP_READ_OPERATIONS,
                update_every);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ea->exchangeLDAPSearchTime)) {
            netdata_exchange_ldap_search_time(ea, windows_shared_buffer, update_every);
            netdata_exchange_ldap_operations(
                &ea->exchangeLDAPSearchTime,
                &ea->st_exchange_ldap_search_operations,
                &ea->rd_exchange_ldap_search_operations,
                windows_shared_buffer,
                "search",
                "LDAP search operations.",
                PRIO_EXCHANGE_LDAP_SEARCH_OPERATIONS,
                update_every);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ea->exchangeLDAPWriteTime)) {
            netdata_exchange_ldap_write_time(ea, windows_shared_buffer, update_every);
            netdata_exchange_ldap_operations(
                &ea->exchangeLDAPWriteTime,
                &ea->st_exchange_ldap_write_operations,
                &ea->rd_exchange_ldap_write_operations,
                windows_shared_buffer,
                "write",
                "LDAP write operations.",
                PRIO_EXCHANGE_LDAP_WRITE_OPERATIONS,
                update_every);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ea->exchangeLDAPTimeoutErrors))
            netdata_exchange_ldap_timeout_errors(ea, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ea->exchangeLDAPLongRunningOps))
            netdata_exchange_ldap_long_running_ops(ea, windows_shared_buffer, update_every);
    }

    if (complete) {
        struct exchange_ad_access_process *ea;
        dfe_start_write(exchange_ad_access_processes, ea)
        {
            if (ea->last_collected < now_ut)
                dictionary_del(exchange_ad_access_processes, ea_dfe.name);
        }
        dfe_done(ea);
        dictionary_garbage_collect(exchange_ad_access_processes);
    }
}

static void netdata_exchange_mapihttp_active_user_count(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_mapihttp_active_user_count = NULL;
    static RRDDIM *rd_exchange_mapihttp_active_user_count = NULL;

    if (unlikely(!st_exchange_mapihttp_active_user_count)) {
        st_exchange_mapihttp_active_user_count = rrdset_create_localhost(
            "exchange",
            "mapihttp_emsmdb_active_user_count",
            NULL,
            "mapihttp",
            "exchange.mapihttp_emsmdb_active_user_count",
            "Unique MAPI HTTP EMSMDB users active in the last 2 minutes.",
            "users",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_MAPIHTTP_ACTIVE_USER_COUNT,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_mapihttp_active_user_count = rrddim_add(
            st_exchange_mapihttp_active_user_count, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_mapihttp_active_user_count,
        rd_exchange_mapihttp_active_user_count,
        (collected_number)value->current.Data);
    rrdset_done(st_exchange_mapihttp_active_user_count);
}

static void netdata_exchange_mapihttp(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA exchangeMapiHTTPActiveUserCount = {.key = "Active User Count"};

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeMapiHTTPActiveUserCount))
        netdata_exchange_mapihttp_active_user_count(&exchangeMapiHTTPActiveUserCount, update_every);
}

static void netdata_exchange_owa_current_unique_users(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_owa_unique_users = NULL;
    static RRDDIM *rd_exchange_owa_unique_users = NULL;

    if (unlikely(!st_exchange_owa_unique_users)) {
        st_exchange_owa_unique_users = rrdset_create_localhost(
            "exchange",
            "owa_current_unique_users",
            NULL,
            "owa",
            "exchange.owa_current_unique_users",
            "Unique users currently logged on to Outlook Web App",
            "users",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_OWA_UNIQUE_USERS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_owa_unique_users =
            rrddim_add(st_exchange_owa_unique_users, "logged-in", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_owa_unique_users, rd_exchange_owa_unique_users, (collected_number)value->current.Data);
    rrdset_done(st_exchange_owa_unique_users);
}

static void netdata_exchange_owa_request_total(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_owa_request_total = NULL;
    static RRDDIM *rd_exchange_owa_request_total = NULL;

    if (unlikely(!st_exchange_owa_request_total)) {
        st_exchange_owa_request_total = rrdset_create_localhost(
            "exchange",
            "owa_requests_total",
            NULL,
            "owa",
            "exchange.owa_requests_total",
            "Requests handled by Outlook Web App.",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_OWA_REQUESTS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_owa_request_total =
            rrddim_add(st_exchange_owa_request_total, "handled", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_owa_request_total, rd_exchange_owa_request_total, (collected_number)value->current.Data);
    rrdset_done(st_exchange_owa_request_total);
}

static void netdata_exchange_owa(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA exchangeOWACurrentUniqueUser = {.key = "Current Unique Users"};
    static COUNTER_DATA exchangeOWARequestsTotal = {.key = "Requests/sec"};

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeOWACurrentUniqueUser))
        netdata_exchange_owa_current_unique_users(&exchangeOWACurrentUniqueUser, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeOWARequestsTotal))
        netdata_exchange_owa_request_total(&exchangeOWARequestsTotal, update_every);
}

static void netdata_exchange_active_ping_cmd(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_active_ping_cmds = NULL;
    static RRDDIM *rd_exchange_active_ping_cmds = NULL;

    if (unlikely(!st_exchange_active_ping_cmds)) {
        st_exchange_active_ping_cmds = rrdset_create_localhost(
            "exchange",
            "activesync_ping_cmds_pending",
            NULL,
            "sync",
            "exchange.activesync_ping_cmds_pending",
            "Ping commands pending in queue.",
            "commands",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_ACTIVE_SYNC_PING_CMDS_PENDING,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_active_ping_cmds =
            rrddim_add(st_exchange_active_ping_cmds, "pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_active_ping_cmds, rd_exchange_active_ping_cmds, (collected_number)value->current.Data);
    rrdset_done(st_exchange_active_ping_cmds);
}

static void netdata_exchange_active_requests(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_received_requests = NULL;
    static RRDDIM *rd_exchange_received_requests = NULL;

    if (unlikely(!st_exchange_received_requests)) {
        st_exchange_received_requests = rrdset_create_localhost(
            "exchange",
            "activesync_requests",
            NULL,
            "sync",
            "exchange.activesync_requests",
            "HTTP requests received from ASP.NET.",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_ACTIVE_SYNC_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_received_requests =
            rrddim_add(st_exchange_received_requests, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_received_requests, rd_exchange_received_requests, (collected_number)value->current.Data);
    rrdset_done(st_exchange_received_requests);
}

static void netdata_exchange_sync_cmds(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_sync_cmds = NULL;
    static RRDDIM *rd_exchange_sync_cmds = NULL;

    if (unlikely(!st_exchange_sync_cmds)) {
        st_exchange_sync_cmds = rrdset_create_localhost(
            "exchange",
            "activesync_sync_cmds",
            NULL,
            "sync",
            "exchange.activesync_sync_cmds",
            "Sync commands processed.",
            "commands/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_ACTIVE_SYNC_CMDS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_sync_cmds = rrddim_add(st_exchange_sync_cmds, "processed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_exchange_sync_cmds, rd_exchange_sync_cmds, (collected_number)value->current.Data);
    rrdset_done(st_exchange_sync_cmds);
}

static void netdata_exchange_active_sync(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA exchangePingCommands = {.key = "Ping Commands Pending"};
    static COUNTER_DATA exchangeSyncCommands = {.key = "Sync Commands/sec"};
    static COUNTER_DATA exchangeActiveRequests = {.key = "Requests/sec"};

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangePingCommands))
        netdata_exchange_active_ping_cmd(&exchangePingCommands, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeSyncCommands))
        netdata_exchange_sync_cmds(&exchangeSyncCommands, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeActiveRequests))
        netdata_exchange_active_requests(&exchangeActiveRequests, update_every);
}

static void netdata_exchange_auto_discover(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA exchangeAutoDiscoverRequestsTotal = {.key = "Requests/sec"};

    static RRDSET *st_exchange_auto_discover_request_total = NULL;
    static RRDDIM *rd_exchange_auto_discover_request_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeAutoDiscoverRequestsTotal)) {
        return;
    }

    if (unlikely(!st_exchange_auto_discover_request_total)) {
        st_exchange_auto_discover_request_total = rrdset_create_localhost(
            "exchange",
            "autodiscover_requests",
            NULL,
            "requests",
            "exchange.autodiscover_requests",
            "Autodiscover service requests processed.",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_AUTO_DISCOVER_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_auto_discover_request_total =
            rrddim_add(st_exchange_auto_discover_request_total, "processed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_auto_discover_request_total,
        rd_exchange_auto_discover_request_total,
        (collected_number)exchangeAutoDiscoverRequestsTotal.current.Data);
    rrdset_done(st_exchange_auto_discover_request_total);
}

static void
netdata_exchange_availability_service(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA exchangeAvailServiceRequests = {.key = "Availability Requests (sec)"};

    static RRDSET *st_exchange_avail_service_requests = NULL;
    static RRDDIM *rd_exchange_avail_service_requests = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeAvailServiceRequests)) {
        return;
    }

    if (unlikely(!st_exchange_avail_service_requests)) {
        st_exchange_avail_service_requests = rrdset_create_localhost(
            "exchange",
            "avail_service_requests",
            NULL,
            "requests",
            "exchange.avail_service_requests",
            "Requests serviced.",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_AUTO_AVAILABILITY_SERVICES,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_avail_service_requests =
            rrddim_add(st_exchange_avail_service_requests, "serviced", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_avail_service_requests,
        rd_exchange_avail_service_requests,
        (collected_number)exchangeAvailServiceRequests.current.Data);
    rrdset_done(st_exchange_avail_service_requests);
}

static void netdata_exchange_rpc_avg_latency(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_rpc_avg_latency = NULL;
    static RRDDIM *rd_exchange_rpc_avg_latency = NULL;

    if (unlikely(!st_exchange_rpc_avg_latency)) {
        st_exchange_rpc_avg_latency = rrdset_create_localhost(
            "exchange",
            "rpc_avg_latency",
            NULL,
            "rpc",
            "exchange.rpc_avg_latency",
            "Average latency.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_AVG_LATENCY,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_rpc_avg_latency =
            rrddim_add(st_exchange_rpc_avg_latency, "latency", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_rpc_avg_latency, rd_exchange_rpc_avg_latency, (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_avg_latency);
}

static void netdata_exchange_rpc_requests(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_rpc_requests = NULL;
    static RRDDIM *rd_exchange_rpc_requests = NULL;

    if (unlikely(!st_exchange_rpc_requests)) {
        st_exchange_rpc_requests = rrdset_create_localhost(
            "exchange",
            "rpc_requests_total",
            NULL,
            "rpc",
            "exchange.rpc_requests",
            "Clients requests currently being processed.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_rpc_requests =
            rrddim_add(st_exchange_rpc_requests, "processed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_exchange_rpc_requests, rd_exchange_rpc_requests, (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_requests);
}

static void netdata_exchange_rpc_active_user_count(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_active_user_account = NULL;
    static RRDDIM *rd_exchange_active_user_account = NULL;

    if (unlikely(!st_exchange_active_user_account)) {
        st_exchange_active_user_account = rrdset_create_localhost(
            "exchange",
            "rpc_active_user",
            NULL,
            "rpc",
            "exchange.rpc_active_user",
            "Active unique users in the last 2 minutes.",
            "users",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_ACTIVE_USERS_COUNT,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_active_user_account =
            rrddim_add(st_exchange_active_user_account, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_active_user_account, rd_exchange_active_user_account, (collected_number)value->current.Data);
    rrdset_done(st_exchange_active_user_account);
}

static void netdata_exchange_rpc_connection_count(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_rpc_connection_count = NULL;
    static RRDDIM *rd_exchange_rpc_connection_count = NULL;

    if (unlikely(!st_exchange_rpc_connection_count)) {
        st_exchange_rpc_connection_count = rrdset_create_localhost(
            "exchange",
            "rpc_connection",
            NULL,
            "rpc",
            "exchange.rpc_connection",
            "Client connections.",
            "connections",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_CONNECTION_COUNT,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_rpc_connection_count =
            rrddim_add(st_exchange_rpc_connection_count, "connections", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_rpc_connection_count, rd_exchange_rpc_connection_count, (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_connection_count);
}

static void netdata_exchange_rpc_op_per_sec(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_rpc_opc_per_sec = NULL;
    static RRDDIM *rd_exchange_rpc_opc_per_sec = NULL;

    if (unlikely(!st_exchange_rpc_opc_per_sec)) {
        st_exchange_rpc_opc_per_sec = rrdset_create_localhost(
            "exchange",
            "rpc_operations",
            NULL,
            "rpc",
            "exchange.rpc_operations",
            "RPC operations.",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_OPERATIONS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_rpc_opc_per_sec =
            rrddim_add(st_exchange_rpc_opc_per_sec, "operations", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_rpc_opc_per_sec, rd_exchange_rpc_opc_per_sec, (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_opc_per_sec);
}

static void netdata_exchange_rpc_user_count(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_rpc_user_count = NULL;
    static RRDDIM *rd_exchange_rpc_user_count = NULL;

    if (unlikely(!st_exchange_rpc_user_count)) {
        st_exchange_rpc_user_count = rrdset_create_localhost(
            "exchange",
            "rpc_user",
            NULL,
            "rpc",
            "exchange.rpc_user_count",
            "RPC users.",
            "users",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_USER_COUNT,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_rpc_user_count =
            rrddim_add(st_exchange_rpc_user_count, "users", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_rpc_user_count, rd_exchange_rpc_user_count, (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_user_count);
}

static void netdata_exchange_rpc(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA exchangeRPCAveragedLatency = {.key = "RPC Averaged Latency"};
    static COUNTER_DATA exchangeRPCRequest = {.key = "RPC Requests"};
    static COUNTER_DATA exchangeRPCActiveUserCount = {.key = "Active User Count"};
    static COUNTER_DATA exchangeRPCConnectionCount = {.key = "Connection Count"};
    static COUNTER_DATA exchangeRPCOperationPerSec = {.key = "RPC Operations/sec"};
    static COUNTER_DATA exchangeRPCUserCount = {.key = "User Count"};

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCAveragedLatency))
        netdata_exchange_rpc_avg_latency(&exchangeRPCAveragedLatency, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCRequest))
        netdata_exchange_rpc_requests(&exchangeRPCRequest, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCActiveUserCount))
        netdata_exchange_rpc_active_user_count(&exchangeRPCActiveUserCount, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCConnectionCount))
        netdata_exchange_rpc_connection_count(&exchangeRPCConnectionCount, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCOperationPerSec))
        netdata_exchange_rpc_op_per_sec(&exchangeRPCOperationPerSec, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCUserCount))
        netdata_exchange_rpc_user_count(&exchangeRPCUserCount, update_every);
}

static void netdata_exchange_proxy_avg_auth_latency(struct exchange_proxy *ep, char *proxy, int update_every)
{
    if (unlikely(!ep->st_exchange_http_proxy_avg_auth_latency)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "proxy", proxy, "avg_auth_latency");

        ep->st_exchange_http_proxy_avg_auth_latency = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "proxy",
            "exchange.http_proxy_avg_auth_latency",
            "Average time spent authenticating CAS requests.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_PROXY_AVG_AUTH_LATENCY,
            update_every,
            RRDSET_TYPE_LINE);

        ep->rd_exchange_http_proxy_avg_auth_latency =
            rrddim_add(ep->st_exchange_http_proxy_avg_auth_latency, "latency", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(ep->st_exchange_http_proxy_avg_auth_latency->rrdlabels, "http_proxy", proxy, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ep->st_exchange_http_proxy_avg_auth_latency,
        ep->rd_exchange_http_proxy_avg_auth_latency,
        (collected_number)ep->exchangeProxyAvgAuthLatency.current.Data);
    rrdset_done(ep->st_exchange_http_proxy_avg_auth_latency);
}

static void netdata_exchange_proxy_avg_cas_processing_latency(struct exchange_proxy *ep, char *proxy, int update_every)
{
    if (unlikely(!ep->st_exchange_http_proxy_avg_cas_processing_latency)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "proxy", proxy, "avg_cas_processing_latency_sec");

        ep->st_exchange_http_proxy_avg_cas_processing_latency = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "proxy",
            "exchange.http_proxy_avg_cas_processing_latency_sec",
            "Average latency (sec) of CAS processing time.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_PROXY_AVG_CAS_PROCESSING_LATENCY,
            update_every,
            RRDSET_TYPE_LINE);

        ep->rd_exchange_http_proxy_avg_cas_processing_latency = rrddim_add(
            ep->st_exchange_http_proxy_avg_cas_processing_latency, "latency", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ep->st_exchange_http_proxy_avg_cas_processing_latency->rrdlabels, "http_proxy", proxy, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ep->st_exchange_http_proxy_avg_cas_processing_latency,
        ep->rd_exchange_http_proxy_avg_cas_processing_latency,
        (collected_number)ep->exchangeProxyAvgCasProcessingLatency.current.Data);
    rrdset_done(ep->st_exchange_http_proxy_avg_cas_processing_latency);
}

static void netdata_exchange_proxy_mailbox_proxy_failure(struct exchange_proxy *ep, char *proxy, int update_every)
{
    if (unlikely(!ep->st_exchange_http_proxy_mailbox_proxy_failure)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "proxy", proxy, "mailbox_proxy_failure_rate");

        ep->st_exchange_http_proxy_mailbox_proxy_failure = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "proxy",
            "exchange.http_proxy_mailbox_proxy_failure_rate",
            "Percentage of failures between this CAS and MBX servers.",
            "percentage",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_PROXY_MAILBOX_PROXY_FAILURE,
            update_every,
            RRDSET_TYPE_LINE);

        ep->rd_exchange_http_proxy_mailbox_proxy_failure = rrddim_add(
            ep->st_exchange_http_proxy_mailbox_proxy_failure, "failures", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ep->st_exchange_http_proxy_mailbox_proxy_failure->rrdlabels, "http_proxy", proxy, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ep->st_exchange_http_proxy_mailbox_proxy_failure,
        ep->rd_exchange_http_proxy_mailbox_proxy_failure,
        (collected_number)ep->exchangeProxyMailboxProxyFailure.current.Data);
    rrdset_done(ep->st_exchange_http_proxy_mailbox_proxy_failure);
}

static void netdata_exchange_proxy_mailbox_server_locator(struct exchange_proxy *ep, char *proxy, int update_every)
{
    if (unlikely(!ep->st_exchange_http_proxy_server_location_avg_latency)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "proxy", proxy, "mailbox_server_locator_avg_latency_sec");

        ep->st_exchange_http_proxy_server_location_avg_latency = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "proxy",
            "exchange.http_proxy_mailbox_server_locator_avg_latency_sec",
            "Average latency of MailboxServerLocator web service calls.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_PROXY_SERVER_LOCATOR_AVG_LATENCY,
            update_every,
            RRDSET_TYPE_LINE);

        ep->rd_exchange_http_proxy_server_location_avg_latency = rrddim_add(
            ep->st_exchange_http_proxy_server_location_avg_latency, "latency", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ep->st_exchange_http_proxy_server_location_avg_latency->rrdlabels, "http_proxy", proxy, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ep->st_exchange_http_proxy_server_location_avg_latency,
        ep->rd_exchange_http_proxy_server_location_avg_latency,
        (collected_number)ep->exchangeProxyMailboxServerLocator.current.Data);
    rrdset_done(ep->st_exchange_http_proxy_server_location_avg_latency);
}

static void netdata_exchange_proxy_outstanding_proxy(struct exchange_proxy *ep, char *proxy, int update_every)
{
    if (unlikely(!ep->st_exchange_http_proxy_outstanding_proxy_requests)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "proxy", proxy, "outstanding_proxy_requests");

        ep->st_exchange_http_proxy_outstanding_proxy_requests = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "proxy",
            "exchange.http_proxy_outstanding_proxy_requests",
            "Concurrent outstanding proxy requests.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_PROXY_OUTSTANDING_PROXY_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        ep->rd_exchange_http_proxy_outstanding_proxy_requests = rrddim_add(
            ep->st_exchange_http_proxy_outstanding_proxy_requests, "requests", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ep->st_exchange_http_proxy_outstanding_proxy_requests->rrdlabels, "http_proxy", proxy, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ep->st_exchange_http_proxy_outstanding_proxy_requests,
        ep->rd_exchange_http_proxy_outstanding_proxy_requests,
        (collected_number)ep->exchangeProxyOutstandingProxy.current.Data);
    rrdset_done(ep->st_exchange_http_proxy_outstanding_proxy_requests);
}

static void netdata_exchange_proxy_request_total(struct exchange_proxy *ep, char *proxy, int update_every)
{
    if (unlikely(!ep->st_exchange_http_proxy_requests_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "proxy", proxy, "requests_total");

        ep->st_exchange_http_proxy_requests_total = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "proxy",
            "exchange.http_proxy_requests",
            "Number of proxy requests processed each second.",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_PROXY_REQUESTS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        ep->rd_exchange_http_proxy_requests_total =
            rrddim_add(ep->st_exchange_http_proxy_requests_total, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ep->st_exchange_http_proxy_requests_total->rrdlabels, "http_proxy", proxy, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ep->st_exchange_http_proxy_requests_total,
        ep->rd_exchange_http_proxy_requests_total,
        (collected_number)ep->exchangeProxyRequestsTotal.current.Data);
    rrdset_done(ep->st_exchange_http_proxy_requests_total);
}

static void netdata_exchange_proxy(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    usec_t now_ut = now_monotonic_usec();
    bool complete = true;
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi) {
            complete = false;
            break;
        }

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer))) {
            complete = false;
            continue;
        }

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct exchange_proxy *ep = dictionary_set(exchange_proxies, windows_shared_buffer, NULL, sizeof(*ep));
        if (!ep) {
            complete = false;
            continue;
        }
        ep->last_collected = now_ut;

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ep->exchangeProxyAvgAuthLatency))
            netdata_exchange_proxy_avg_auth_latency(ep, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ep->exchangeProxyAvgCasProcessingLatency))
            netdata_exchange_proxy_avg_cas_processing_latency(ep, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ep->exchangeProxyMailboxProxyFailure))
            netdata_exchange_proxy_mailbox_proxy_failure(ep, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ep->exchangeProxyMailboxServerLocator))
            netdata_exchange_proxy_mailbox_server_locator(ep, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ep->exchangeProxyOutstandingProxy))
            netdata_exchange_proxy_outstanding_proxy(ep, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ep->exchangeProxyRequestsTotal))
            netdata_exchange_proxy_request_total(ep, windows_shared_buffer, update_every);
    }

    if (complete) {
        struct exchange_proxy *ep;
        dfe_start_write(exchange_proxies, ep)
        {
            if (ep->last_collected < now_ut)
                dictionary_del(exchange_proxies, ep_dfe.name);
        }
        dfe_done(ep);
        dictionary_garbage_collect(exchange_proxies);
    }
}

static void netdata_exchange_workload_active_tasks(struct exchange_workload *ew, char *workload, int update_every)
{
    if (unlikely(!ew->st_exchange_workload_active_tasks)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "workload", workload, "tasks");

        ew->st_exchange_workload_active_tasks = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "workload",
            "exchange.workload_active_tasks",
            "Workload active tasks.",
            "tasks",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_WORKLOAD_ACTIVE_TASKS,
            update_every,
            RRDSET_TYPE_LINE);

        ew->rd_exchange_workload_active_tasks =
            rrddim_add(ew->st_exchange_workload_active_tasks, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(ew->st_exchange_workload_active_tasks->rrdlabels, "workload", workload, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ew->st_exchange_workload_active_tasks,
        ew->rd_exchange_workload_active_tasks,
        (collected_number)ew->exchangeWorkloadActiveTasks.current.Data);
    rrdset_done(ew->st_exchange_workload_active_tasks);
}

static void netdata_exchange_workload_completed_tasks(struct exchange_workload *ew, char *workload, int update_every)
{
    if (unlikely(!ew->st_exchange_workload_complete_tasks)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "workload", workload, "completed_tasks");

        ew->st_exchange_workload_complete_tasks = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "workload",
            "exchange.workload_completed_tasks",
            "Workload completed tasks.",
            "tasks/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_WORKLOAD_COMPLETE_TASKS,
            update_every,
            RRDSET_TYPE_LINE);

        ew->rd_exchange_workload_complete_tasks =
            rrddim_add(ew->st_exchange_workload_complete_tasks, "completed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ew->st_exchange_workload_complete_tasks->rrdlabels, "workload", workload, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ew->st_exchange_workload_complete_tasks,
        ew->rd_exchange_workload_complete_tasks,
        (collected_number)ew->exchangeWorkloadCompleteTasks.current.Data);
    rrdset_done(ew->st_exchange_workload_complete_tasks);
}

static void netdata_exchange_workload_queued_tasks(struct exchange_workload *ew, char *workload, int update_every)
{
    if (unlikely(!ew->st_exchange_workload_queued_tasks)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "workload", workload, "queued_tasks");

        ew->st_exchange_workload_queued_tasks = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "workload",
            "exchange.workload_queued_tasks",
            "Workload queued tasks.",
            "tasks",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_WORKLOAD_QUEUE_TASKS,
            update_every,
            RRDSET_TYPE_LINE);

        ew->rd_exchange_workload_queued_tasks =
            rrddim_add(ew->st_exchange_workload_queued_tasks, "queued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(ew->st_exchange_workload_queued_tasks->rrdlabels, "workload", workload, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ew->st_exchange_workload_queued_tasks,
        ew->rd_exchange_workload_queued_tasks,
        (collected_number)ew->exchangeWorkloadQueueTasks.current.Data);
    rrdset_done(ew->st_exchange_workload_queued_tasks);
}

static void netdata_exchange_workload_yielded_tasks(struct exchange_workload *ew, char *workload, int update_every)
{
    if (unlikely(!ew->st_exchange_workload_yielded_tasks)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "workload", workload, "yielded_tasks");

        ew->st_exchange_workload_yielded_tasks = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "workload",
            "exchange.workload_yielded_tasks",
            "Workload yielded tasks.",
            "tasks/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_WORKLOAD_YIELDED_TASKS,
            update_every,
            RRDSET_TYPE_LINE);

        ew->rd_exchange_workload_yielded_tasks =
            rrddim_add(ew->st_exchange_workload_yielded_tasks, "yielded", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ew->st_exchange_workload_yielded_tasks->rrdlabels, "workload", workload, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ew->st_exchange_workload_yielded_tasks,
        ew->rd_exchange_workload_yielded_tasks,
        (collected_number)ew->exchangeWorkloadYieldedTasks.current.Data);
    rrdset_done(ew->st_exchange_workload_yielded_tasks);
}

static void netdata_exchange_workload_activity_status(struct exchange_workload *ew, char *workload, int update_every)
{
    if (unlikely(!ew->st_exchange_workload_activity_status)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "workload", workload, "activity_status");

        ew->st_exchange_workload_activity_status = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "workload",
            "exchange.workload_activity_status",
            "Workload activity status.",
            "status",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_WORKLOAD_ACTIVITY,
            update_every,
            RRDSET_TYPE_LINE);

        ew->rd_exchange_workload_activity_active_status =
            rrddim_add(ew->st_exchange_workload_activity_status, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        ew->rd_exchange_workload_activity_paused_status =
            rrddim_add(ew->st_exchange_workload_activity_status, "paused", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(ew->st_exchange_workload_activity_status->rrdlabels, "workload", workload, RRDLABEL_SRC_AUTO);
    }

    bool value = (bool)ew->exchangeWorkloadActivityStatus.current.Data;
    rrddim_set_by_pointer(
        ew->st_exchange_workload_activity_status,
        ew->rd_exchange_workload_activity_active_status,
        (collected_number)value);

    value = !value;
    rrddim_set_by_pointer(
        ew->st_exchange_workload_activity_status,
        ew->rd_exchange_workload_activity_paused_status,
        (collected_number)value);
    rrdset_done(ew->st_exchange_workload_activity_status);
}

static void netdata_exchange_workload(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    usec_t now_ut = now_monotonic_usec();
    bool complete = true;
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi) {
            complete = false;
            break;
        }

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer))) {
            complete = false;
            continue;
        }

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct exchange_workload *ew = dictionary_set(exchange_workloads, windows_shared_buffer, NULL, sizeof(*ew));
        if (!ew) {
            complete = false;
            continue;
        }
        ew->last_collected = now_ut;

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ew->exchangeWorkloadActiveTasks))
            netdata_exchange_workload_active_tasks(ew, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ew->exchangeWorkloadCompleteTasks))
            netdata_exchange_workload_completed_tasks(ew, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ew->exchangeWorkloadQueueTasks))
            netdata_exchange_workload_queued_tasks(ew, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ew->exchangeWorkloadYieldedTasks))
            netdata_exchange_workload_yielded_tasks(ew, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &ew->exchangeWorkloadActivityStatus))
            netdata_exchange_workload_activity_status(ew, windows_shared_buffer, update_every);
    }

    if (complete) {
        struct exchange_workload *ew;
        dfe_start_write(exchange_workloads, ew)
        {
            if (ew->last_collected < now_ut)
                dictionary_del(exchange_workloads, ew_dfe.name);
        }
        dfe_done(ew);
        dictionary_garbage_collect(exchange_workloads);
    }
}

static void netdata_exchange_queue_active_mailbox(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_active_mailbox)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "active_mailbox_delivery");

        eq->st_exchange_queue_active_mailbox = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_active_mail_box_delivery",
            "Active Mailbox Delivery Queue length.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_ACTIVE_MAILBOX_DELIVERY,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_active_mailbox =
            rrddim_add(eq->st_exchange_queue_active_mailbox, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(eq->st_exchange_queue_active_mailbox->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_active_mailbox,
        eq->rd_exchange_queue_active_mailbox,
        (collected_number)eq->exchangeTransportQueuesActiveMailboxDelivery.current.Data);
    rrdset_done(eq->st_exchange_queue_active_mailbox);
}

static void
netdata_exchange_queue_external_active_remote_delivery(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_external_active_remote_delivery)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "external_active_remote_delivery");

        eq->st_exchange_queue_external_active_remote_delivery = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_external_active_remote_delivery",
            "External Active Remote Delivery Queue length.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_EXTERNAL_ACTIVE_REMOTE_DELIVERY,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_external_active_remote_delivery = rrddim_add(
            eq->st_exchange_queue_external_active_remote_delivery, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            eq->st_exchange_queue_external_active_remote_delivery->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_external_active_remote_delivery,
        eq->rd_exchange_queue_external_active_remote_delivery,
        (collected_number)eq->exchangeTransportQueuesExternalActiveRemoteDelivery.current.Data);
    rrdset_done(eq->st_exchange_queue_external_active_remote_delivery);
}

static void
netdata_exchange_queue_internal_active_remote_delivery(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_internal_active_remote_delivery)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "internal_active_remote_delivery");

        eq->st_exchange_queue_internal_active_remote_delivery = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_internal_active_remote_delivery",
            "Internal Active Remote Delivery Queue length.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_INTERNAL_ACTIVE_REMOTE_DELIVERY,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_internal_active_remote_delivery = rrddim_add(
            eq->st_exchange_queue_internal_active_remote_delivery, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            eq->st_exchange_queue_internal_active_remote_delivery->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_internal_active_remote_delivery,
        eq->rd_exchange_queue_internal_active_remote_delivery,
        (collected_number)eq->exchangeTransportQueuesInternalActiveRemoteDelivery.current.Data);
    rrdset_done(eq->st_exchange_queue_internal_active_remote_delivery);
}

static void netdata_exchange_queue_unreachable(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_unreachable)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "unreachable");

        eq->st_exchange_queue_unreachable = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_unreachable",
            "Unreachable Queue length.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_UNREACHABLE,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_unreachable =
            rrddim_add(eq->st_exchange_queue_unreachable, "unreachable", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(eq->st_exchange_queue_unreachable->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_unreachable,
        eq->rd_exchange_queue_unreachable,
        (collected_number)eq->exchangeTransportQueuesUnreachable.current.Data);
    rrdset_done(eq->st_exchange_queue_unreachable);
}

static void netdata_exchange_queue_poison(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_poison)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "poison");

        eq->st_exchange_queue_poison = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_poison",
            "Poison Queue Length.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_POISON,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_poison =
            rrddim_add(eq->st_exchange_queue_poison, "poison", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(eq->st_exchange_queue_poison->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_poison,
        eq->rd_exchange_queue_poison,
        (collected_number)eq->exchangeTransportQueuesPoison.current.Data);
    rrdset_done(eq->st_exchange_queue_poison);
}

static void netdata_exchange_queue_retry_mailbox_delivery(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_retry_mailbox_delivery)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "retry_mailbox_delivery");

        eq->st_exchange_queue_retry_mailbox_delivery = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_retry_mailbox_delivery",
            "Retry Mailbox Delivery Queue length.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_RETRY_MAILBOX_DELIVERY,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_retry_mailbox_delivery = rrddim_add(
            eq->st_exchange_queue_retry_mailbox_delivery, "retry", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            eq->st_exchange_queue_retry_mailbox_delivery->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_retry_mailbox_delivery,
        eq->rd_exchange_queue_retry_mailbox_delivery,
        (collected_number)eq->exchangeTransportQueuesRetryMailboxDelivery.current.Data);
    rrdset_done(eq->st_exchange_queue_retry_mailbox_delivery);
}

static void netdata_exchange_queue_external_largest_delivery(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_external_largest_delivery)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "external_largest_delivery");

        eq->st_exchange_queue_external_largest_delivery = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_external_largest_delivery",
            "External Largest Delivery Queue length.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_EXTERNAL_LARGEST_DELIVERY,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_external_largest_delivery = rrddim_add(
            eq->st_exchange_queue_external_largest_delivery, "largest", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            eq->st_exchange_queue_external_largest_delivery->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_external_largest_delivery,
        eq->rd_exchange_queue_external_largest_delivery,
        (collected_number)eq->exchangeTransportQueuesExternalLargestDelivery.current.Data);
    rrdset_done(eq->st_exchange_queue_external_largest_delivery);
}

static void netdata_exchange_queue_internal_largest_delivery(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_internal_largest_delivery)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "internal_largest_delivery");

        eq->st_exchange_queue_internal_largest_delivery = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_internal_largest_delivery",
            "Internal Largest Delivery Queue length.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_INTERNAL_LARGEST_DELIVERY,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_internal_largest_delivery = rrddim_add(
            eq->st_exchange_queue_internal_largest_delivery, "largest", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            eq->st_exchange_queue_internal_largest_delivery->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_internal_largest_delivery,
        eq->rd_exchange_queue_internal_largest_delivery,
        (collected_number)eq->exchangeTransportQueuesInternalLargestDelivery.current.Data);
    rrdset_done(eq->st_exchange_queue_internal_largest_delivery);
}

static void netdata_exchange_queue_messages_queued_for_delivery_total(
    struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_messages_queued_for_delivery_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "messages_queued_for_delivery_total");

        eq->st_exchange_queue_messages_queued_for_delivery_total = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_messages_queued_for_delivery_total",
            "Messages queued for delivery.",
            "messages/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_MESSAGES_QUEUED_FOR_DELIVERY_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_messages_queued_for_delivery_total = rrddim_add(
            eq->st_exchange_queue_messages_queued_for_delivery_total, "queued", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(
            eq->st_exchange_queue_messages_queued_for_delivery_total->rrdlabels,
            "priority",
            priority,
            RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_messages_queued_for_delivery_total,
        eq->rd_exchange_queue_messages_queued_for_delivery_total,
        (collected_number)eq->exchangeTransportQueuesMessagesQueuedForDeliveryTotal.current.Data);
    rrdset_done(eq->st_exchange_queue_messages_queued_for_delivery_total);
}

static void netdata_exchange_queue_messages_submitted_total(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_messages_submitted_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "messages_submitted_total");

        eq->st_exchange_queue_messages_submitted_total = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_messages_submitted_total",
            "Messages submitted.",
            "messages/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_MESSAGES_SUBMITTED_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_messages_submitted_total = rrddim_add(
            eq->st_exchange_queue_messages_submitted_total, "submitted", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(
            eq->st_exchange_queue_messages_submitted_total->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_messages_submitted_total,
        eq->rd_exchange_queue_messages_submitted_total,
        (collected_number)eq->exchangeTransportQueuesMessagesSubmittedTotal.current.Data);
    rrdset_done(eq->st_exchange_queue_messages_submitted_total);
}

static void netdata_exchange_queue_messages_delayed_total(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_messages_delayed_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "messages_delayed_total");

        eq->st_exchange_queue_messages_delayed_total = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_messages_delayed_total",
            "Messages delayed.",
            "messages/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_MESSAGES_DELAYED_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_messages_delayed_total = rrddim_add(
            eq->st_exchange_queue_messages_delayed_total, "delayed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(eq->st_exchange_queue_messages_delayed_total->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_messages_delayed_total,
        eq->rd_exchange_queue_messages_delayed_total,
        (collected_number)eq->exchangeTransportQueuesMessagesDelayedTotal.current.Data);
    rrdset_done(eq->st_exchange_queue_messages_delayed_total);
}

static void netdata_exchange_queue_messages_completed_delivery_total(
    struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_messages_completed_delivery_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "messages_completed_delivery_total");

        eq->st_exchange_queue_messages_completed_delivery_total = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_messages_completed_delivery_total",
            "Messages completed delivery.",
            "messages/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_MESSAGES_COMPLETED_DELIVERY_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_messages_completed_delivery_total = rrddim_add(
            eq->st_exchange_queue_messages_completed_delivery_total,
            "delivered",
            NULL,
            1,
            1,
            RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(
            eq->st_exchange_queue_messages_completed_delivery_total->rrdlabels,
            "priority",
            priority,
            RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_messages_completed_delivery_total,
        eq->rd_exchange_queue_messages_completed_delivery_total,
        (collected_number)eq->exchangeTransportQueuesMessagesCompletedDeliveryTotal.current.Data);
    rrdset_done(eq->st_exchange_queue_messages_completed_delivery_total);
}

static void netdata_exchange_queue_aggregate_shadow_queue_length(
    struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_aggregate_shadow_queue_length)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "aggregate_shadow_queue_length");

        eq->st_exchange_queue_aggregate_shadow_queue_length = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_aggregate_shadow_queue_length",
            "Current number of messages in shadow queues.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_AGGREGATE_SHADOW_QUEUE_LENGTH,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_aggregate_shadow_queue_length = rrddim_add(
            eq->st_exchange_queue_aggregate_shadow_queue_length, "queued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            eq->st_exchange_queue_aggregate_shadow_queue_length->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_aggregate_shadow_queue_length,
        eq->rd_exchange_queue_aggregate_shadow_queue_length,
        (collected_number)eq->exchangeTransportQueuesAggregateShadowQueueLength.current.Data);
    rrdset_done(eq->st_exchange_queue_aggregate_shadow_queue_length);
}

static void netdata_exchange_queue_submission_queue_length(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_submission_queue_length)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "submission_queue_length");

        eq->st_exchange_queue_submission_queue_length = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_submission_queue_length",
            "Submission Queue Length.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_SUBMISSION_QUEUE_LENGTH,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_submission_queue_length = rrddim_add(
            eq->st_exchange_queue_submission_queue_length, "queued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            eq->st_exchange_queue_submission_queue_length->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_submission_queue_length,
        eq->rd_exchange_queue_submission_queue_length,
        (collected_number)eq->exchangeTransportQueuesSubmissionQueueLength.current.Data);
    rrdset_done(eq->st_exchange_queue_submission_queue_length);
}

static void netdata_exchange_queue_delay_queue_length(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_delay_queue_length)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "delay_queue_length");

        eq->st_exchange_queue_delay_queue_length = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_delay_queue_length",
            "Delay Queue Length.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_DELAY_QUEUE_LENGTH,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_delay_queue_length =
            rrddim_add(eq->st_exchange_queue_delay_queue_length, "queued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(eq->st_exchange_queue_delay_queue_length->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_delay_queue_length,
        eq->rd_exchange_queue_delay_queue_length,
        (collected_number)eq->exchangeTransportQueuesDelayQueueLength.current.Data);
    rrdset_done(eq->st_exchange_queue_delay_queue_length);
}

static void netdata_exchange_queue_items_completed_delivery_total(
    struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_items_completed_delivery_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "items_completed_delivery_total");

        eq->st_exchange_queue_items_completed_delivery_total = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_items_completed_delivery_total",
            "Items completed delivery.",
            "items/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_ITEMS_COMPLETED_DELIVERY_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_items_completed_delivery_total = rrddim_add(
            eq->st_exchange_queue_items_completed_delivery_total, "completed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(
            eq->st_exchange_queue_items_completed_delivery_total->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_items_completed_delivery_total,
        eq->rd_exchange_queue_items_completed_delivery_total,
        (collected_number)eq->exchangeTransportQueuesItemsCompletedDeliveryTotal.current.Data);
    rrdset_done(eq->st_exchange_queue_items_completed_delivery_total);
}

static void netdata_exchange_queue_items_queued_for_delivery_expired_total(
    struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_items_queued_for_delivery_expired_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "items_queued_for_delivery_expired_total");

        eq->st_exchange_queue_items_queued_for_delivery_expired_total = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_items_queued_for_delivery_expired_total",
            "Items queued for delivery that expired.",
            "items/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_ITEMS_QUEUED_FOR_DELIVERY_EXPIRED_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_items_queued_for_delivery_expired_total = rrddim_add(
            eq->st_exchange_queue_items_queued_for_delivery_expired_total,
            "expired",
            NULL,
            1,
            1,
            RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(
            eq->st_exchange_queue_items_queued_for_delivery_expired_total->rrdlabels,
            "priority",
            priority,
            RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_items_queued_for_delivery_expired_total,
        eq->rd_exchange_queue_items_queued_for_delivery_expired_total,
        (collected_number)eq->exchangeTransportQueuesItemsQueuedForDeliveryExpiredTotal.current.Data);
    rrdset_done(eq->st_exchange_queue_items_queued_for_delivery_expired_total);
}

static void netdata_exchange_queue_items_queued_for_delivery_total(
    struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_items_queued_for_delivery_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "items_queued_for_delivery_total");

        eq->st_exchange_queue_items_queued_for_delivery_total = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_items_queued_for_delivery_total",
            "Items queued for delivery.",
            "items/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_ITEMS_QUEUED_FOR_DELIVERY_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_items_queued_for_delivery_total = rrddim_add(
            eq->st_exchange_queue_items_queued_for_delivery_total, "queued", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(
            eq->st_exchange_queue_items_queued_for_delivery_total->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_items_queued_for_delivery_total,
        eq->rd_exchange_queue_items_queued_for_delivery_total,
        (collected_number)eq->exchangeTransportQueuesItemsQueuedForDeliveryTotal.current.Data);
    rrdset_done(eq->st_exchange_queue_items_queued_for_delivery_total);
}

static void netdata_exchange_queue_items_resubmitted_total(struct exchange_queue *eq, char *priority, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_items_resubmitted_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        exchange_instance_chart_id(id, sizeof(id), "transport_queues", priority, "items_resubmitted_total");

        eq->st_exchange_queue_items_resubmitted_total = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_items_resubmitted_total",
            "Items resubmitted.",
            "items/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_ITEMS_RESUBMITTED_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_items_resubmitted_total = rrddim_add(
            eq->st_exchange_queue_items_resubmitted_total, "resubmitted", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(
            eq->st_exchange_queue_items_resubmitted_total->rrdlabels, "priority", priority, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_items_resubmitted_total,
        eq->rd_exchange_queue_items_resubmitted_total,
        (collected_number)eq->exchangeTransportQueuesItemsResubmittedTotal.current.Data);
    rrdset_done(eq->st_exchange_queue_items_resubmitted_total);
}

static void netdata_exchange_queues(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    usec_t now_ut = now_monotonic_usec();
    bool complete = true;
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi) {
            complete = false;
            break;
        }

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer))) {
            complete = false;
            continue;
        }

        if (strcasecmp(windows_shared_buffer, "_Total") == 0 || strcasecmp(windows_shared_buffer, "total") == 0)
            continue;

        struct exchange_queue *eq = dictionary_set(exchange_queues, windows_shared_buffer, NULL, sizeof(*eq));
        if (!eq) {
            complete = false;
            continue;
        }
        eq->last_collected = now_ut;

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesActiveMailboxDelivery))
            netdata_exchange_queue_active_mailbox(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesExternalActiveRemoteDelivery))
            netdata_exchange_queue_external_active_remote_delivery(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesExternalLargestDelivery))
            netdata_exchange_queue_external_largest_delivery(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesInternalActiveRemoteDelivery))
            netdata_exchange_queue_internal_active_remote_delivery(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesInternalLargestDelivery))
            netdata_exchange_queue_internal_largest_delivery(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesRetryMailboxDelivery))
            netdata_exchange_queue_retry_mailbox_delivery(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesUnreachable))
            netdata_exchange_queue_unreachable(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesPoison))
            netdata_exchange_queue_poison(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesMessagesQueuedForDeliveryTotal))
            netdata_exchange_queue_messages_queued_for_delivery_total(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesMessagesSubmittedTotal))
            netdata_exchange_queue_messages_submitted_total(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesMessagesDelayedTotal))
            netdata_exchange_queue_messages_delayed_total(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(
                pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesMessagesCompletedDeliveryTotal))
            netdata_exchange_queue_messages_completed_delivery_total(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesAggregateShadowQueueLength))
            netdata_exchange_queue_aggregate_shadow_queue_length(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesSubmissionQueueLength))
            netdata_exchange_queue_submission_queue_length(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesDelayQueueLength))
            netdata_exchange_queue_delay_queue_length(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesItemsCompletedDeliveryTotal))
            netdata_exchange_queue_items_completed_delivery_total(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(
                pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesItemsQueuedForDeliveryExpiredTotal))
            netdata_exchange_queue_items_queued_for_delivery_expired_total(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesItemsQueuedForDeliveryTotal))
            netdata_exchange_queue_items_queued_for_delivery_total(eq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &eq->exchangeTransportQueuesItemsResubmittedTotal))
            netdata_exchange_queue_items_resubmitted_total(eq, windows_shared_buffer, update_every);
    }

    if (complete) {
        struct exchange_queue *eq;
        dfe_start_write(exchange_queues, eq)
        {
            if (eq->last_collected < now_ut)
                dictionary_del(exchange_queues, eq_dfe.name);
        }
        dfe_done(eq);
        dictionary_garbage_collect(exchange_queues);
    }
}

struct netdata_exchange_objects {
    void (*fnct)(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int);
    char *object;
} exchange_obj[] = {
    {.fnct = netdata_exchange_owa, .object = "MSExchange OWA"},
    {.fnct = netdata_exchange_active_sync, .object = "MSExchange ActiveSync"},
    {.fnct = netdata_exchange_ad_access, .object = "MSExchange ADAccess Processes"},
    {.fnct = netdata_exchange_auto_discover, .object = "MSExchangeAutodiscover"},
    {.fnct = netdata_exchange_availability_service, .object = "MSExchange Availability Service"},
    {.fnct = netdata_exchange_mapihttp, .object = "MSExchange MapiHttp Emsmdb"},
    {.fnct = netdata_exchange_rpc, .object = "MSExchange RpcClientAccess"},
    {.fnct = netdata_exchange_proxy, .object = "MSExchange HttpProxy"},
    {.fnct = netdata_exchange_workload, .object = "MSExchange WorkloadManagement Workloads"},
    {.fnct = netdata_exchange_queues, .object = "MSExchangeTransport Queues"},

    // This is the end of the loop
    {.fnct = NULL, .object = NULL}};

int do_PerflibExchange(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    for (int i = 0; exchange_obj[i].fnct; i++) {
        DWORD id = RegistryFindIDByName(exchange_obj[i].object);
        if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
            continue;

        PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
        if (!pDataBlock)
            continue;

        PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, exchange_obj[i].object);
        if (!pObjectType)
            continue;

        exchange_obj[i].fnct(pDataBlock, pObjectType, update_every);
    }

    return 0;
}
