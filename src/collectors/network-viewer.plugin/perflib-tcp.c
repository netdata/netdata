// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"
#include "libnetdata/os/windows-api/windows_api.h"
#include "libnetdata/os/windows-perflib/perflib.h"

#include <signal.h>

#define PLUGIN_NETWORK_VIEWER_NAME "network-viewer.plugin"

// Priority values matching the PERFLIB_PRIO enum in windows_plugin.h
#define PRIO_TCP_IPV4_CONNECTIONS             21069
#define PRIO_TCP_IPV4_SEGMENTS                21070
#define PRIO_TCP_IPV4_CONNECTIONS_STATE_COUNT 21071
#define PRIO_TCP_IPV6_CONNECTIONS             21072
#define PRIO_TCP_IPV6_SEGMENTS                21073
#define PRIO_TCP_IPV6_CONNECTIONS_STATE_COUNT 21074

int do_PerflibUDP(int update_every, usec_t dt);

static volatile sig_atomic_t plugin_should_exit = 0;
static void signal_handler(int sig __maybe_unused) { plugin_should_exit = 1; }

typedef struct {
    const char *af;
    const char *object_name;
    const char *type;
    const char *family;
    const char *title_prefix;
    const char *context_prefix;
    int connections_priority;
    int segments_priority;
    int states_priority;

    COUNTER_DATA connection_failures;
    COUNTER_DATA connections_active;
    COUNTER_DATA connections_established;
    COUNTER_DATA connections_passive;
    COUNTER_DATA connections_reset;
    COUNTER_DATA segments_total;
    COUNTER_DATA segments_received;
    COUNTER_DATA segments_retransmitted;
    COUNTER_DATA segments_sent;

    bool connections_chart_created;
    bool segments_chart_created;
    bool states_chart_created;
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
    if (tcp->connections_chart_created)
        return;
    tcp->connections_chart_created = true;

    char context[64];
    char title[64];
    snprintfz(context, sizeof(context), "%s.connections", tcp->context_prefix);
    snprintfz(title, sizeof(title), "%s TCP Connections", tcp->title_prefix);

    fprintf(stdout,
            "CHART %s.connections '' '%s' connections %s %s line %d %d '' '" PLUGIN_NETWORK_VIEWER_NAME "' PerflibTCP\n",
            tcp->type, title, tcp->family, context, tcp->connections_priority, update_every);
    fprintf(stdout, "DIMENSION failures '' incremental 1 1\n");
    fprintf(stdout, "DIMENSION active '' incremental 1 1\n");
    fprintf(stdout, "DIMENSION established '' incremental 1 1\n");
    fprintf(stdout, "DIMENSION passive '' incremental 1 1\n");
    fprintf(stdout, "DIMENSION reset '' incremental 1 1\n");
}

static void tcp_create_segments_chart(TCP_FAMILY *tcp, int update_every)
{
    if (tcp->segments_chart_created)
        return;
    tcp->segments_chart_created = true;

    char context[64];
    char title[64];
    snprintfz(context, sizeof(context), "%s.segments", tcp->context_prefix);
    snprintfz(title, sizeof(title), "%s TCP Segments", tcp->title_prefix);

    fprintf(stdout,
            "CHART %s.segments '' '%s' segments/s %s %s line %d %d '' '" PLUGIN_NETWORK_VIEWER_NAME "' PerflibTCP\n",
            tcp->type, title, tcp->family, context, tcp->segments_priority, update_every);
    fprintf(stdout, "DIMENSION total '' incremental 1 1\n");
    fprintf(stdout, "DIMENSION received '' incremental 1 1\n");
    fprintf(stdout, "DIMENSION retransmitted '' incremental 1 1\n");
    fprintf(stdout, "DIMENSION sent '' incremental 1 1\n");
}

static void tcp_create_states_chart(TCP_FAMILY *tcp, int update_every)
{
    if (tcp->states_chart_created)
        return;
    tcp->states_chart_created = true;

    char context[64];
    char title[64];
    snprintfz(context, sizeof(context), "%s.connections_state_count", tcp->context_prefix);
    snprintfz(title, sizeof(title), "%s TCP Connection States", tcp->title_prefix);

    fprintf(stdout,
            "CHART %s.connections_state_count '' '%s' connections %s %s stacked %d %d '' '" PLUGIN_NETWORK_VIEWER_NAME "' PerflibTCP\n",
            tcp->type, title, tcp->family, context, tcp->states_priority, update_every);
    fprintf(stdout, "DIMENSION closed '' absolute 1 1\n");
    fprintf(stdout, "DIMENSION listening '' absolute 1 1\n");
    fprintf(stdout, "DIMENSION syn_sent '' absolute 1 1\n");
    fprintf(stdout, "DIMENSION syn_received '' absolute 1 1\n");
    fprintf(stdout, "DIMENSION established '' absolute 1 1\n");
    fprintf(stdout, "DIMENSION fin_wait1 '' absolute 1 1\n");
    fprintf(stdout, "DIMENSION fin_wait2 '' absolute 1 1\n");
    fprintf(stdout, "DIMENSION close_wait '' absolute 1 1\n");
    fprintf(stdout, "DIMENSION closing '' absolute 1 1\n");
    fprintf(stdout, "DIMENSION last_ack '' absolute 1 1\n");
    fprintf(stdout, "DIMENSION time_wait '' absolute 1 1\n");
    fprintf(stdout, "DIMENSION delete_tcb '' absolute 1 1\n");
}

static bool tcp_collect_family(TCP_FAMILY *tcp, int update_every, usec_t dt)
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

    fprintf(stdout, "BEGIN %s.connections %" PRIu64 "\n", tcp->type, dt);
    fprintf(stdout, "SET failures = %lld\n", (long long)tcp->connection_failures.current.Data);
    fprintf(stdout, "SET active = %lld\n", (long long)tcp->connections_active.current.Data);
    fprintf(stdout, "SET established = %lld\n", (long long)tcp->connections_established.current.Data);
    fprintf(stdout, "SET passive = %lld\n", (long long)tcp->connections_passive.current.Data);
    fprintf(stdout, "SET reset = %lld\n", (long long)tcp->connections_reset.current.Data);
    fprintf(stdout, "END\n");

    fprintf(stdout, "BEGIN %s.segments %" PRIu64 "\n", tcp->type, dt);
    fprintf(stdout, "SET total = %lld\n", (long long)tcp->segments_total.current.Data);
    fprintf(stdout, "SET received = %lld\n", (long long)tcp->segments_received.current.Data);
    fprintf(stdout, "SET retransmitted = %lld\n", (long long)tcp->segments_retransmitted.current.Data);
    fprintf(stdout, "SET sent = %lld\n", (long long)tcp->segments_sent.current.Data);
    fprintf(stdout, "END\n");

    uint32_t state_counts[NETDATA_WIN_TCP_STATE_COUNT] = {0};
    if (netdata_win_collect_tcp_state_counts(strcmp(tcp->af, "ipv4") == 0 ? AF_INET : AF_INET6, state_counts)) {
        fprintf(stdout, "BEGIN %s.connections_state_count %" PRIu64 "\n", tcp->type, dt);
        fprintf(stdout, "SET closed = %lld\n",       (long long)state_counts[NETDATA_WIN_TCP_STATE_CLOSED]);
        fprintf(stdout, "SET listening = %lld\n",    (long long)state_counts[NETDATA_WIN_TCP_STATE_LISTENING]);
        fprintf(stdout, "SET syn_sent = %lld\n",     (long long)state_counts[NETDATA_WIN_TCP_STATE_SYN_SENT]);
        fprintf(stdout, "SET syn_received = %lld\n", (long long)state_counts[NETDATA_WIN_TCP_STATE_SYN_RECEIVED]);
        fprintf(stdout, "SET established = %lld\n",  (long long)state_counts[NETDATA_WIN_TCP_STATE_ESTABLISHED]);
        fprintf(stdout, "SET fin_wait1 = %lld\n",    (long long)state_counts[NETDATA_WIN_TCP_STATE_FIN_WAIT1]);
        fprintf(stdout, "SET fin_wait2 = %lld\n",    (long long)state_counts[NETDATA_WIN_TCP_STATE_FIN_WAIT2]);
        fprintf(stdout, "SET close_wait = %lld\n",   (long long)state_counts[NETDATA_WIN_TCP_STATE_CLOSE_WAIT]);
        fprintf(stdout, "SET closing = %lld\n",      (long long)state_counts[NETDATA_WIN_TCP_STATE_CLOSING]);
        fprintf(stdout, "SET last_ack = %lld\n",     (long long)state_counts[NETDATA_WIN_TCP_STATE_LAST_ACK]);
        fprintf(stdout, "SET time_wait = %lld\n",    (long long)state_counts[NETDATA_WIN_TCP_STATE_TIME_WAIT]);
        fprintf(stdout, "SET delete_tcb = %lld\n",   (long long)state_counts[NETDATA_WIN_TCP_STATE_DELETE_TCB]);
        fprintf(stdout, "END\n");
        have_any = true;
    }

    return have_any;
}

int do_PerflibTCP(int update_every, usec_t dt)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    bool have_any = false;
    have_any |= tcp_collect_family(&tcp_ipv4, update_every, dt);
    have_any |= tcp_collect_family(&tcp_ipv6, update_every, dt);

    return have_any ? 0 : -1;
}

int main(int argc, char **argv)
{
    nd_log_initialize_for_external_plugins("network-viewer.plugin");
    netdata_threads_init_for_external_plugins(0);

    signal(SIGTERM, signal_handler);
    signal(SIGINT, signal_handler);

    int update_every = 1;
    if (argc >= 2) {
        update_every = atoi(argv[1]);
        if (update_every < 1)
            update_every = 1;
    }

    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);

    while (!plugin_should_exit) {
        usec_t dt = heartbeat_next(&hb);
        do_PerflibTCP(update_every, dt);
        do_PerflibUDP(update_every, dt);
        fflush(stdout);
    }

    return 0;
}
