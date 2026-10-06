// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define DNS_MAX_CHART_DIMENSIONS 8

enum dns_counter_id {
    DNS_TCP_QUERY_RECEIVED,
    DNS_UDP_QUERY_RECEIVED,
    DNS_TCP_RESPONSE_SENT,
    DNS_UDP_RESPONSE_SENT,
    DNS_QUERY_DROPPED_POLICY,
    DNS_QUERY_DROPPED_RATE_LIMITING,
    DNS_QUERY_DROPPED_SEND,
    DNS_QUERY_DROPPED_BAD_SOCKET,
    DNS_QUERY_DROPPED_TOTAL,
    DNS_RESPONSES_SUPPRESSED,
    DNS_REMOTE_INFLIGHT_QUERIES,
    DNS_RECURSIVE_QUERIES,
    DNS_RECURSIVE_FAILURES,
    DNS_RECURSIVE_SEND_TIMEOUTS,
    DNS_AXFR_REQUEST_RECEIVED,
    DNS_IXFR_REQUEST_RECEIVED,
    DNS_AXFR_REQUEST_SENT,
    DNS_IXFR_REQUEST_SENT,
    DNS_SOA_REQUEST_SENT,
    DNS_AXFR_RESPONSE_RECEIVED,
    DNS_IXFR_RESPONSE_RECEIVED,
    DNS_AXFR_SUCCESS_RECEIVED,
    DNS_IXFR_TCP_SUCCESS_RECEIVED,
    DNS_IXFR_UDP_SUCCESS_RECEIVED,
    DNS_AXFR_SUCCESS_SENT,
    DNS_IXFR_SUCCESS_SENT,
    DNS_ZONE_TRANSFER_FAILURES,
    DNS_NOTIFY_RECEIVED,
    DNS_NOTIFY_SENT,
    DNS_DYNAMIC_UPDATE_NOOP,
    DNS_DYNAMIC_UPDATE_WRITTEN,
    DNS_DYNAMIC_UPDATE_QUEUED,
    DNS_DYNAMIC_UPDATE_REJECTED,
    DNS_DYNAMIC_UPDATE_TIMEOUTS,
    DNS_SECURE_UPDATE_RECEIVED,
    DNS_SECURE_UPDATE_FAILURES,
    DNS_CACHING_MEMORY,
    DNS_DATABASE_NODE_MEMORY,
    DNS_NBSTAT_MEMORY,
    DNS_RECORD_FLOW_MEMORY,
    DNS_TCP_MESSAGE_MEMORY,
    DNS_UDP_MESSAGE_MEMORY,
    DNS_UNMATCHED_RESPONSES,
    DNS_WINS_LOOKUP_RECEIVED,
    DNS_WINS_REVERSE_LOOKUP_RECEIVED,
    DNS_WINS_RESPONSE_SENT,
    DNS_WINS_REVERSE_RESPONSE_SENT,
    DNS_COUNTER_COUNT,
};

struct dns_chart_dimension {
    enum dns_counter_id counter;
    const char *id;
    const char *name;
    RRDDIM *rd;
};

enum dns_value_kind {
    DNS_VALUE_RATE,
    DNS_VALUE_GAUGE,
};

struct dns_chart {
    const char *id;
    const char *context;
    const char *title;
    const char *units;
    enum dns_value_kind value_kind;
    int priority;
    RRDSET_TYPE type;
    size_t dimensions_count;
    struct dns_chart_dimension dimensions[DNS_MAX_CHART_DIMENSIONS];
    RRDSET *st;
};

static COUNTER_DATA counters[DNS_COUNTER_COUNT] = {
    [DNS_TCP_QUERY_RECEIVED] = {.key = "TCP Query Received"},
    [DNS_UDP_QUERY_RECEIVED] = {.key = "UDP Query Received"},
    [DNS_TCP_RESPONSE_SENT] = {.key = "TCP Response Sent"},
    [DNS_UDP_RESPONSE_SENT] = {.key = "UDP Response Sent"},
    [DNS_QUERY_DROPPED_POLICY] = {.key = "Query Dropped By Policy"},
    [DNS_QUERY_DROPPED_RATE_LIMITING] = {.key = "Query Dropped By Response Rate Limiting"},
    [DNS_QUERY_DROPPED_SEND] = {.key = "Query Dropped Send"},
    [DNS_QUERY_DROPPED_BAD_SOCKET] = {.key = "Query Dropped Bad Socket"},
    [DNS_QUERY_DROPPED_TOTAL] = {.key = "Query Dropped Total"},
    [DNS_RESPONSES_SUPPRESSED] = {.key = "Responses Suppressed"},
    [DNS_REMOTE_INFLIGHT_QUERIES] = {.key = "Total Remote Inflight Queries"},
    [DNS_RECURSIVE_QUERIES] = {.key = "Recursive Queries"},
    [DNS_RECURSIVE_FAILURES] = {.key = "Recursive Query Failure"},
    [DNS_RECURSIVE_SEND_TIMEOUTS] = {.key = "Recursive Send TimeOuts"},
    [DNS_AXFR_REQUEST_RECEIVED] = {.key = "AXFR Request Received"},
    [DNS_IXFR_REQUEST_RECEIVED] = {.key = "IXFR Request Received"},
    [DNS_AXFR_REQUEST_SENT] = {.key = "AXFR Request Sent"},
    [DNS_IXFR_REQUEST_SENT] = {.key = "IXFR Request Sent"},
    [DNS_SOA_REQUEST_SENT] = {.key = "Zone Transfer SOA Request Sent"},
    [DNS_AXFR_RESPONSE_RECEIVED] = {.key = "AXFR Response Received"},
    [DNS_IXFR_RESPONSE_RECEIVED] = {.key = "IXFR Response Received"},
    [DNS_AXFR_SUCCESS_RECEIVED] = {.key = "AXFR Success Received"},
    [DNS_IXFR_TCP_SUCCESS_RECEIVED] = {.key = "IXFR TCP Success Received"},
    [DNS_IXFR_UDP_SUCCESS_RECEIVED] = {.key = "IXFR UDP Success Received"},
    [DNS_AXFR_SUCCESS_SENT] = {.key = "AXFR Success Sent"},
    [DNS_IXFR_SUCCESS_SENT] = {.key = "IXFR Success Sent"},
    [DNS_ZONE_TRANSFER_FAILURES] = {.key = "Zone Transfer Failure"},
    [DNS_NOTIFY_RECEIVED] = {.key = "Notify Received"},
    [DNS_NOTIFY_SENT] = {.key = "Notify Sent"},
    [DNS_DYNAMIC_UPDATE_NOOP] = {.key = "Dynamic Update NoOperation"},
    [DNS_DYNAMIC_UPDATE_WRITTEN] = {.key = "Dynamic Update Written to Database"},
    [DNS_DYNAMIC_UPDATE_QUEUED] = {.key = "Dynamic Update Queued"},
    [DNS_DYNAMIC_UPDATE_REJECTED] = {.key = "Dynamic Update Rejected"},
    [DNS_DYNAMIC_UPDATE_TIMEOUTS] = {.key = "Dynamic Update TimeOuts"},
    [DNS_SECURE_UPDATE_RECEIVED] = {.key = "Secure Update Received"},
    [DNS_SECURE_UPDATE_FAILURES] = {.key = "Secure Update Failure"},
    [DNS_CACHING_MEMORY] = {.key = "Caching Memory"},
    [DNS_DATABASE_NODE_MEMORY] = {.key = "Database Node Memory"},
    [DNS_NBSTAT_MEMORY] = {.key = "Nbstat Memory"},
    [DNS_RECORD_FLOW_MEMORY] = {.key = "Record Flow Memory"},
    [DNS_TCP_MESSAGE_MEMORY] = {.key = "TCP Message Memory"},
    [DNS_UDP_MESSAGE_MEMORY] = {.key = "UDP Message Memory"},
    [DNS_UNMATCHED_RESPONSES] = {.key = "Unmatched Responses Received"},
    [DNS_WINS_LOOKUP_RECEIVED] = {.key = "WINS Lookup Received"},
    [DNS_WINS_REVERSE_LOOKUP_RECEIVED] = {.key = "WINS Reverse Lookup Received"},
    [DNS_WINS_RESPONSE_SENT] = {.key = "WINS Response Sent"},
    [DNS_WINS_REVERSE_RESPONSE_SENT] = {.key = "WINS Reverse Response Sent"},
};

#define DIM(counter_id, dim_id, dim_name) {.counter = counter_id, .id = dim_id, .name = dim_name}

static struct dns_chart charts[] = {
    {.id = "queries",
     .context = "windows.dns.queries",
     .title = "DNS queries received",
     .units = "queries/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_QUERIES,
     .type = RRDSET_TYPE_STACKED,
     .dimensions_count = 2,
     .dimensions = {DIM(DNS_TCP_QUERY_RECEIVED, "tcp", "TCP"), DIM(DNS_UDP_QUERY_RECEIVED, "udp", "UDP")}},
    {.id = "responses",
     .context = "windows.dns.responses",
     .title = "DNS responses sent",
     .units = "responses/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_RESPONSES,
     .type = RRDSET_TYPE_STACKED,
     .dimensions_count = 2,
     .dimensions = {DIM(DNS_TCP_RESPONSE_SENT, "tcp", "TCP"), DIM(DNS_UDP_RESPONSE_SENT, "udp", "UDP")}},
    {.id = "query_handling",
     .context = "windows.dns.query_handling",
     .title = "DNS queries dropped",
     .units = "queries/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_QUERY_HANDLING,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 5,
     .dimensions =
         {DIM(DNS_QUERY_DROPPED_TOTAL, "dropped_total", "Dropped total"),
          DIM(DNS_QUERY_DROPPED_POLICY, "dropped_policy", "Dropped by policy"),
          DIM(DNS_QUERY_DROPPED_RATE_LIMITING, "dropped_rate_limiting", "Dropped by rate limiting"),
          DIM(DNS_QUERY_DROPPED_SEND, "dropped_send", "Dropped on send"),
          DIM(DNS_QUERY_DROPPED_BAD_SOCKET, "dropped_bad_socket", "Dropped on bad socket")}},
    {.id = "suppressed_responses",
     .context = "windows.dns.suppressed_responses",
     .title = "DNS responses suppressed",
     .units = "responses/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_SUPPRESSED_RESPONSES,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 1,
     .dimensions = {DIM(DNS_RESPONSES_SUPPRESSED, "suppressed", "Suppressed")}},
    {.id = "remote_queries",
     .context = "windows.dns.remote_queries",
     .title = "DNS remote queries in flight",
     .units = "queries",
     .value_kind = DNS_VALUE_GAUGE,
     .priority = PRIO_DNS_REMOTE_QUERIES,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 1,
     .dimensions = {DIM(DNS_REMOTE_INFLIGHT_QUERIES, "in_flight", "In flight")}},
    {.id = "recursive_queries",
     .context = "windows.dns.recursive_queries",
     .title = "DNS recursive queries",
     .units = "queries/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_RECURSIVE_QUERIES,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 2,
     .dimensions =
         {DIM(DNS_RECURSIVE_QUERIES, "queries", "Queries"), DIM(DNS_RECURSIVE_FAILURES, "failures", "Failures")}},
    {.id = "recursive_send_timeouts",
     .context = "windows.dns.recursive_send_timeouts",
     .title = "DNS recursive send timeouts",
     .units = "timeouts/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_RECURSIVE_SEND_TIMEOUTS,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 1,
     .dimensions = {DIM(DNS_RECURSIVE_SEND_TIMEOUTS, "timeouts", "Timeouts")}},
    {.id = "zone_transfer_requests_received",
     .context = "windows.dns.zone_transfer_requests_received",
     .title = "DNS zone transfer requests received",
     .units = "requests/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_ZONE_REQUESTS_RECEIVED,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 2,
     .dimensions = {DIM(DNS_AXFR_REQUEST_RECEIVED, "axfr", "AXFR"), DIM(DNS_IXFR_REQUEST_RECEIVED, "ixfr", "IXFR")}},
    {.id = "zone_transfer_requests_sent",
     .context = "windows.dns.zone_transfer_requests_sent",
     .title = "DNS zone transfer requests sent",
     .units = "requests/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_ZONE_REQUESTS_SENT,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 3,
     .dimensions =
         {DIM(DNS_AXFR_REQUEST_SENT, "axfr", "AXFR"),
          DIM(DNS_IXFR_REQUEST_SENT, "ixfr", "IXFR"),
          DIM(DNS_SOA_REQUEST_SENT, "soa", "SOA")}},
    {.id = "zone_transfer_responses_received",
     .context = "windows.dns.zone_transfer_responses_received",
     .title = "DNS zone transfer responses received",
     .units = "responses/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_ZONE_RESPONSES_RECEIVED,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 2,
     .dimensions = {DIM(DNS_AXFR_RESPONSE_RECEIVED, "axfr", "AXFR"), DIM(DNS_IXFR_RESPONSE_RECEIVED, "ixfr", "IXFR")}},
    {.id = "zone_transfer_success_received",
     .context = "windows.dns.zone_transfer_success_received",
     .title = "DNS zone transfers completed on secondary",
     .units = "transfers/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_ZONE_SUCCESS_RECEIVED,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 3,
     .dimensions =
         {DIM(DNS_AXFR_SUCCESS_RECEIVED, "axfr", "AXFR"),
          DIM(DNS_IXFR_TCP_SUCCESS_RECEIVED, "ixfr_tcp", "IXFR over TCP"),
          DIM(DNS_IXFR_UDP_SUCCESS_RECEIVED, "ixfr_udp", "IXFR over UDP")}},
    {.id = "zone_transfer_success_sent",
     .context = "windows.dns.zone_transfer_success_sent",
     .title = "DNS zone transfers completed on primary",
     .units = "transfers/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_ZONE_SUCCESS_SENT,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 2,
     .dimensions = {DIM(DNS_AXFR_SUCCESS_SENT, "axfr", "AXFR"), DIM(DNS_IXFR_SUCCESS_SENT, "ixfr", "IXFR")}},
    {.id = "zone_transfer_failures",
     .context = "windows.dns.zone_transfer_failures",
     .title = "DNS zone transfer failures",
     .units = "failures/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_ZONE_FAILURES,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 1,
     .dimensions = {DIM(DNS_ZONE_TRANSFER_FAILURES, "failures", "Failures")}},
    {.id = "notify_received",
     .context = "windows.dns.notify_received",
     .title = "DNS notifications received",
     .units = "notifications/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_NOTIFY_RECEIVED,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 1,
     .dimensions = {DIM(DNS_NOTIFY_RECEIVED, "received", "Received")}},
    {.id = "notify_sent",
     .context = "windows.dns.notify_sent",
     .title = "DNS notifications sent",
     .units = "notifications/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_NOTIFY_SENT,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 1,
     .dimensions = {DIM(DNS_NOTIFY_SENT, "sent", "Sent")}},
    {.id = "dynamic_updates",
     .context = "windows.dns.dynamic_updates",
     .title = "DNS dynamic update events",
     .units = "updates/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_DYNAMIC_UPDATES,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 6,
     .dimensions =
         {DIM(DNS_DYNAMIC_UPDATE_NOOP, "no_operation", "No operation"),
          DIM(DNS_DYNAMIC_UPDATE_WRITTEN, "written", "Written to database"),
          DIM(DNS_DYNAMIC_UPDATE_REJECTED, "rejected", "Rejected"),
          DIM(DNS_DYNAMIC_UPDATE_TIMEOUTS, "timed_out", "Timed out"),
          DIM(DNS_SECURE_UPDATE_RECEIVED, "secure_received", "Secure update received"),
          DIM(DNS_SECURE_UPDATE_FAILURES, "secure_failure", "Secure update failures")}},
    {.id = "dynamic_updates_queued",
     .context = "windows.dns.dynamic_updates_queued",
     .title = "DNS dynamic updates queued",
     .units = "updates",
     .value_kind = DNS_VALUE_GAUGE,
     .priority = PRIO_DNS_DYNAMIC_UPDATES_QUEUED,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 1,
     .dimensions = {DIM(DNS_DYNAMIC_UPDATE_QUEUED, "queued", "Queued")}},
    {.id = "memory_used",
     .context = "windows.dns.memory_used",
     .title = "DNS server memory used",
     .units = "bytes",
     .value_kind = DNS_VALUE_GAUGE,
     .priority = PRIO_DNS_MEMORY_USED,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 6,
     .dimensions =
         {DIM(DNS_CACHING_MEMORY, "caching", "Caching"),
          DIM(DNS_DATABASE_NODE_MEMORY, "database_node", "Database node"),
          DIM(DNS_NBSTAT_MEMORY, "nbstat", "NetBIOS statistics"),
          DIM(DNS_RECORD_FLOW_MEMORY, "record_flow", "Record flow"),
          DIM(DNS_TCP_MESSAGE_MEMORY, "tcp_message", "TCP message"),
          DIM(DNS_UDP_MESSAGE_MEMORY, "udp_message", "UDP message")}},
    {.id = "unmatched_responses",
     .context = "windows.dns.unmatched_responses",
     .title = "DNS unmatched responses",
     .units = "responses/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_UNMATCHED_RESPONSES,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 1,
     .dimensions = {DIM(DNS_UNMATCHED_RESPONSES, "unmatched", "Unmatched")}},
    {.id = "wins_lookups",
     .context = "windows.dns.wins_lookups",
     .title = "DNS WINS lookups received",
     .units = "lookups/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_WINS_LOOKUPS,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 2,
     .dimensions =
         {DIM(DNS_WINS_LOOKUP_RECEIVED, "forward", "Forward"),
          DIM(DNS_WINS_REVERSE_LOOKUP_RECEIVED, "reverse", "Reverse")}},
    {.id = "wins_responses",
     .context = "windows.dns.wins_responses",
     .title = "DNS WINS responses sent",
     .units = "responses/s",
     .value_kind = DNS_VALUE_RATE,
     .priority = PRIO_DNS_WINS_RESPONSES,
     .type = RRDSET_TYPE_LINE,
     .dimensions_count = 2,
     .dimensions =
         {DIM(DNS_WINS_RESPONSE_SENT, "forward", "Forward"),
          DIM(DNS_WINS_REVERSE_RESPONSE_SENT, "reverse", "Reverse")}},
};

#undef DIM

static bool counter_type_matches(enum dns_value_kind kind, DWORD counter_type)
{
    switch (kind) {
        // DNS publishes event counts as PERF_COUNTER_RAWCOUNT running totals, and only some have a "/sec" twin,
        // so rates are derived incrementally from the raw value. Counter types also carry a running total.
        case DNS_VALUE_RATE:
            return counter_type == PERF_COUNTER_RAWCOUNT || counter_type == PERF_COUNTER_LARGE_RAWCOUNT ||
                   counter_type == PERF_COUNTER_COUNTER || counter_type == PERF_COUNTER_BULK_COUNT;

        case DNS_VALUE_GAUGE:
            return counter_type == PERF_COUNTER_RAWCOUNT || counter_type == PERF_COUNTER_LARGE_RAWCOUNT;
    }

    return false;
}

static void update_chart(struct dns_chart *chart, int update_every, const bool available[])
{
    bool has_data = false;
    for (size_t i = 0; i < chart->dimensions_count; i++)
        has_data |= available[chart->dimensions[i].counter] &&
                    counter_type_matches(chart->value_kind, counters[chart->dimensions[i].counter].current.CounterType);

    if (!has_data)
        return;

    if (unlikely(!chart->st)) {
        chart->st = rrdset_create_localhost(
            "dns_server",
            chart->id,
            NULL,
            "dns",
            chart->context,
            chart->title,
            chart->units,
            PLUGIN_WINDOWS_NAME,
            "PerflibDNS",
            chart->priority,
            update_every,
            chart->type);
    }

    for (size_t i = 0; i < chart->dimensions_count; i++) {
        struct dns_chart_dimension *dimension = &chart->dimensions[i];
        if (!available[dimension->counter] ||
            !counter_type_matches(chart->value_kind, counters[dimension->counter].current.CounterType))
            continue;

        if (unlikely(!dimension->rd))
            dimension->rd = rrddim_add(
                chart->st,
                dimension->id,
                dimension->name,
                1,
                1,
                chart->value_kind == DNS_VALUE_RATE ? RRD_ALGORITHM_INCREMENTAL : RRD_ALGORITHM_ABSOLUTE);

        rrddim_set_by_pointer(chart->st, dimension->rd, (collected_number)counters[dimension->counter].current.Data);
    }

    rrdset_done(chart->st);
}

static void do_dns(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "DNS");
    if (!pObjectType)
        return;

    bool available[DNS_COUNTER_COUNT] = {0};
    for (size_t i = 0; i < DNS_COUNTER_COUNT; i++)
        available[i] = perflibGetObjectCounter(pDataBlock, pObjectType, &counters[i]);

    for (size_t i = 0; i < sizeof(charts) / sizeof(charts[0]); i++)
        update_chart(&charts[i], update_every, available);
}

int do_PerflibDNS(int update_every, usec_t dt __maybe_unused)
{
    DWORD id = RegistryFindIDByName("DNS");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return 0;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return 0;

    do_dns(pDataBlock, update_every);
    return 0;
}
