// SPDX-License-Identifier: GPL-3.0-or-later

#include "../windows.plugin/windows_plugin.h"
#include "daemon/daemon-service.h"
#include "libnetdata/required_dummies.h"
#include "libnetdata/os/windows-api/windows_api.h"
#include "libnetdata/os/windows-perflib/perflib.h"

#define PLUGIN_NETWORK_VIEWER_NAME "network-viewer.plugin"

typedef struct {
    const char *af;
    const char *object_name;
    const char *type;
    const char *family;
    const char *title_prefix;
    const char *context_prefix;
    long connections_priority;
    long segments_priority;
    long states_priority;

    COUNTER_DATA connection_failures;
    COUNTER_DATA connections_active;
    COUNTER_DATA connections_established;
    COUNTER_DATA connections_passive;
    COUNTER_DATA connections_reset;
    COUNTER_DATA segments_total;
    COUNTER_DATA segments_received;
    COUNTER_DATA segments_retransmitted;
    COUNTER_DATA segments_sent;

    RRDSET *connections_chart;
    RRDDIM *rd_connection_failures;
    RRDDIM *rd_connections_active;
    RRDDIM *rd_connections_established;
    RRDDIM *rd_connections_passive;
    RRDDIM *rd_connections_reset;

    RRDSET *segments_chart;
    RRDDIM *rd_segments_total;
    RRDDIM *rd_segments_received;
    RRDDIM *rd_segments_retransmitted;
    RRDDIM *rd_segments_sent;

    RRDSET *states_chart;
    RRDDIM *rd_state_closed;
    RRDDIM *rd_state_listening;
    RRDDIM *rd_state_syn_sent;
    RRDDIM *rd_state_syn_received;
    RRDDIM *rd_state_established;
    RRDDIM *rd_state_fin_wait1;
    RRDDIM *rd_state_fin_wait2;
    RRDDIM *rd_state_close_wait;
    RRDDIM *rd_state_closing;
    RRDDIM *rd_state_last_ack;
    RRDDIM *rd_state_time_wait;
    RRDDIM *rd_state_delete_tcb;
} TCP_FAMILY;

static TCP_FAMILY tcp_ipv4 = {
    .af = "ipv4",
    .object_name = "TCPv4",
    .type = "ipv4",
    .family = "tcp",
    .title_prefix = "IPv4",
    .context_prefix = "ipv4.tcp",
    .connections_priority = PRIO_TCP_IPV4_CONNECTIONS,
    .segments_priority = PRIO_TCP_IPV4_SEGMENTS,
    .states_priority = PRIO_TCP_IPV4_CONNECTIONS_STATE_COUNT,
};

static TCP_FAMILY tcp_ipv6 = {
    .af = "ipv6",
    .object_name = "TCPv6",
    .type = "ipv6",
    .family = "tcp6",
    .title_prefix = "IPv6",
    .context_prefix = "ipv6.tcp",
    .connections_priority = PRIO_TCP_IPV6_CONNECTIONS,
    .segments_priority = PRIO_TCP_IPV6_SEGMENTS,
    .states_priority = PRIO_TCP_IPV6_CONNECTIONS_STATE_COUNT,
};

static void initialize_tcp_keys(TCP_FAMILY *tcp)
{
    tcp->connection_failures.key = "Connection Failures";
    tcp->connections_active.key = "Connections Active";
    tcp->connections_established.key = "Connections Established";
    tcp->connections_passive.key = "Connections Passive";
    tcp->connections_reset.key = "Connections Reset";
    tcp->segments_total.key = "Segments/sec";
    tcp->segments_received.key = "Segments Received/sec";
    tcp->segments_retransmitted.key = "Segments Retransmitted/sec";
    tcp->segments_sent.key = "Segments Sent/sec";
}

static void initialize(void)
{
    initialize_tcp_keys(&tcp_ipv4);
    initialize_tcp_keys(&tcp_ipv6);
}

static void tcp_create_connection_chart(TCP_FAMILY *tcp, int update_every)
{
    if (tcp->connections_chart)
        return;

    char context[64];
    char title[64];

    snprintfz(context, sizeof(context), "%s.connections", tcp->context_prefix);
    snprintfz(title, sizeof(title), "%s TCP Connections", tcp->title_prefix);

    tcp->connections_chart = rrdset_create_localhost(
        tcp->type,
        "connections",
        NULL,
        tcp->family,
        context,
        title,
        "connections",
        PLUGIN_NETWORK_VIEWER_NAME,
        "PerflibTCP",
        tcp->connections_priority,
        update_every,
        RRDSET_TYPE_LINE);

    tcp->rd_connection_failures = rrddim_add(tcp->connections_chart, "failures", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    tcp->rd_connections_active = rrddim_add(tcp->connections_chart, "active", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    tcp->rd_connections_established = rrddim_add(tcp->connections_chart, "established", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    tcp->rd_connections_passive = rrddim_add(tcp->connections_chart, "passive", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    tcp->rd_connections_reset = rrddim_add(tcp->connections_chart, "reset", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
}

static void tcp_create_segments_chart(TCP_FAMILY *tcp, int update_every)
{
    if (tcp->segments_chart)
        return;

    char context[64];
    char title[64];

    snprintfz(context, sizeof(context), "%s.segments", tcp->context_prefix);
    snprintfz(title, sizeof(title), "%s TCP Segments", tcp->title_prefix);

    tcp->segments_chart = rrdset_create_localhost(
        tcp->type,
        "segments",
        NULL,
        tcp->family,
        context,
        title,
        "segments/s",
        PLUGIN_NETWORK_VIEWER_NAME,
        "PerflibTCP",
        tcp->segments_priority,
        update_every,
        RRDSET_TYPE_LINE);

    tcp->rd_segments_total = rrddim_add(tcp->segments_chart, "total", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    tcp->rd_segments_received = rrddim_add(tcp->segments_chart, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    tcp->rd_segments_retransmitted =
        rrddim_add(tcp->segments_chart, "retransmitted", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    tcp->rd_segments_sent = rrddim_add(tcp->segments_chart, "sent", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
}

static void tcp_create_states_chart(TCP_FAMILY *tcp, int update_every)
{
    if (tcp->states_chart)
        return;

    char context[64];
    char title[64];

    snprintfz(context, sizeof(context), "%s.connections_state_count", tcp->context_prefix);
    snprintfz(title, sizeof(title), "%s TCP Connection States", tcp->title_prefix);

    tcp->states_chart = rrdset_create_localhost(
        tcp->type,
        "connections_state_count",
        NULL,
        tcp->family,
        context,
        title,
        "connections",
        PLUGIN_NETWORK_VIEWER_NAME,
        "PerflibTCP",
        tcp->states_priority,
        update_every,
        RRDSET_TYPE_STACKED);

    tcp->rd_state_closed = rrddim_add(tcp->states_chart, "closed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    tcp->rd_state_listening = rrddim_add(tcp->states_chart, "listening", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    tcp->rd_state_syn_sent = rrddim_add(tcp->states_chart, "syn_sent", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    tcp->rd_state_syn_received = rrddim_add(tcp->states_chart, "syn_received", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    tcp->rd_state_established = rrddim_add(tcp->states_chart, "established", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    tcp->rd_state_fin_wait1 = rrddim_add(tcp->states_chart, "fin_wait1", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    tcp->rd_state_fin_wait2 = rrddim_add(tcp->states_chart, "fin_wait2", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    tcp->rd_state_close_wait = rrddim_add(tcp->states_chart, "close_wait", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    tcp->rd_state_closing = rrddim_add(tcp->states_chart, "closing", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    tcp->rd_state_last_ack = rrddim_add(tcp->states_chart, "last_ack", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    tcp->rd_state_time_wait = rrddim_add(tcp->states_chart, "time_wait", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    tcp->rd_state_delete_tcb = rrddim_add(tcp->states_chart, "delete_tcb", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
}

static void tcp_update_state_chart(TCP_FAMILY *tcp, uint32_t state_counts[NETDATA_WIN_TCP_STATE_COUNT])
{
    rrddim_set_by_pointer(tcp->states_chart, tcp->rd_state_closed, (collected_number)state_counts[NETDATA_WIN_TCP_STATE_CLOSED]);
    rrddim_set_by_pointer(tcp->states_chart, tcp->rd_state_listening, (collected_number)state_counts[NETDATA_WIN_TCP_STATE_LISTENING]);
    rrddim_set_by_pointer(tcp->states_chart, tcp->rd_state_syn_sent, (collected_number)state_counts[NETDATA_WIN_TCP_STATE_SYN_SENT]);
    rrddim_set_by_pointer(tcp->states_chart, tcp->rd_state_syn_received, (collected_number)state_counts[NETDATA_WIN_TCP_STATE_SYN_RECEIVED]);
    rrddim_set_by_pointer(tcp->states_chart, tcp->rd_state_established, (collected_number)state_counts[NETDATA_WIN_TCP_STATE_ESTABLISHED]);
    rrddim_set_by_pointer(tcp->states_chart, tcp->rd_state_fin_wait1, (collected_number)state_counts[NETDATA_WIN_TCP_STATE_FIN_WAIT1]);
    rrddim_set_by_pointer(tcp->states_chart, tcp->rd_state_fin_wait2, (collected_number)state_counts[NETDATA_WIN_TCP_STATE_FIN_WAIT2]);
    rrddim_set_by_pointer(tcp->states_chart, tcp->rd_state_close_wait, (collected_number)state_counts[NETDATA_WIN_TCP_STATE_CLOSE_WAIT]);
    rrddim_set_by_pointer(tcp->states_chart, tcp->rd_state_closing, (collected_number)state_counts[NETDATA_WIN_TCP_STATE_CLOSING]);
    rrddim_set_by_pointer(tcp->states_chart, tcp->rd_state_last_ack, (collected_number)state_counts[NETDATA_WIN_TCP_STATE_LAST_ACK]);
    rrddim_set_by_pointer(tcp->states_chart, tcp->rd_state_time_wait, (collected_number)state_counts[NETDATA_WIN_TCP_STATE_TIME_WAIT]);
    rrddim_set_by_pointer(tcp->states_chart, tcp->rd_state_delete_tcb, (collected_number)state_counts[NETDATA_WIN_TCP_STATE_DELETE_TCB]);
}

static bool tcp_collect_family(TCP_FAMILY *tcp, int update_every)
{
    DWORD id = RegistryFindIDByName(tcp->object_name);
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return false;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return false;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, tcp->object_name);
    if (!pObjectType)
        return false;

    bool have_any = false;

    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->connection_failures);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->connections_active);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->connections_established);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->connections_passive);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->connections_reset);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->segments_total);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->segments_received);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->segments_retransmitted);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->segments_sent);

    tcp_create_connection_chart(tcp, update_every);
    tcp_create_segments_chart(tcp, update_every);
    tcp_create_states_chart(tcp, update_every);

    rrddim_set_by_pointer(tcp->connections_chart, tcp->rd_connection_failures, (collected_number)tcp->connection_failures.current.Data);
    rrddim_set_by_pointer(tcp->connections_chart, tcp->rd_connections_active, (collected_number)tcp->connections_active.current.Data);
    rrddim_set_by_pointer(tcp->connections_chart, tcp->rd_connections_established, (collected_number)tcp->connections_established.current.Data);
    rrddim_set_by_pointer(tcp->connections_chart, tcp->rd_connections_passive, (collected_number)tcp->connections_passive.current.Data);
    rrddim_set_by_pointer(tcp->connections_chart, tcp->rd_connections_reset, (collected_number)tcp->connections_reset.current.Data);
    rrdset_done(tcp->connections_chart);

    rrddim_set_by_pointer(tcp->segments_chart, tcp->rd_segments_total, (collected_number)tcp->segments_total.current.Data);
    rrddim_set_by_pointer(tcp->segments_chart, tcp->rd_segments_received, (collected_number)tcp->segments_received.current.Data);
    rrddim_set_by_pointer(tcp->segments_chart, tcp->rd_segments_retransmitted, (collected_number)tcp->segments_retransmitted.current.Data);
    rrddim_set_by_pointer(tcp->segments_chart, tcp->rd_segments_sent, (collected_number)tcp->segments_sent.current.Data);
    rrdset_done(tcp->segments_chart);

    uint32_t state_counts[NETDATA_WIN_TCP_STATE_COUNT] = {0};
    if (netdata_win_collect_tcp_state_counts(strcmp(tcp->af, "ipv4") == 0 ? AF_INET : AF_INET6, state_counts)) {
        tcp_update_state_chart(tcp, state_counts);
        rrdset_done(tcp->states_chart);
        have_any = true;
    }

    return have_any;
}

int do_PerflibTCP(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    bool have_any = false;
    have_any |= tcp_collect_family(&tcp_ipv4, update_every);
    have_any |= tcp_collect_family(&tcp_ipv6, update_every);

    return have_any ? 0 : -1;
}

int main(int argc __maybe_unused, char **argv __maybe_unused)
{
    nd_log_initialize_for_external_plugins("network-viewer.plugin");
    netdata_threads_init_for_external_plugins(0);

    int update_every = localhost->rrd_update_every;
    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);

    while(service_running(SERVICE_COLLECTORS)) {
        usec_t dt = heartbeat_next(&hb);
        do_PerflibTCP(update_every, dt);
    }

    return 0;
}
