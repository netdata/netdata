// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"
#include "cgroup_ebpfgo_shared_memory.h"

#if defined(OS_LINUX)

#include <fcntl.h>
#include <sys/stat.h>
#include <unistd.h>

// Snapshot readiness is set externally to share the single SHM refresh
// performed by cgroup_ebpfgo_cachestat_refresh() each tick.
static bool cgroup_ebpfgo_socket_snapshot_ready = false;

void cgroup_ebpfgo_socket_set_snapshot_ready(bool ready)
{
    cgroup_ebpfgo_socket_snapshot_ready = ready;
}

static void cgroup_ebpfgo_socket_sum_pids(struct cgroup *cg)
{
    memset(&cg->net, 0, sizeof(cg->net));

    if (!cg->ebpf_pids_count)
        return;

    for (size_t i = 0; i < cg->ebpf_pids_count; i++) {
        pid_t pid = cg->ebpf_pids[i];

        const struct ebpf_pid_stat *item = cgroup_ebpfgo_shared_memory_lookup(pid);
        if (!item)
            continue;

        /* SHM holds per-interval deltas pre-computed by the Go producer;
         * sum directly without further diffing. */
        const struct ebpf_socket_publish_apps *s = &item->socket;
        cg->net.bytes_sent             += s->bytes_sent;
        cg->net.bytes_received         += s->bytes_received;
        cg->net.call_tcp_sent          += s->call_tcp_sent;
        cg->net.call_tcp_received      += s->call_tcp_received;
        cg->net.retransmit             += s->retransmit;
        cg->net.call_udp_sent          += s->call_udp_sent;
        cg->net.call_udp_received      += s->call_udp_received;
        cg->net.call_close             += s->call_close;
        cg->net.call_tcp_v4_connection += s->call_tcp_v4_connection;
        cg->net.call_tcp_v6_connection += s->call_tcp_v6_connection;
        if (s->socket_update_every_s)
            cg->net.socket_update_every_s = s->socket_update_every_s;
    }
}

void cgroup_ebpfgo_socket_update_locked(void)
{
    for (struct cgroup *cg = cgroup_root; cg; cg = cg->next) {
        if (unlikely(!cg->enabled || cg->pending_renames))
            continue;

        cgroup_ebpfgo_socket_sum_pids(cg);
    }
}

void cgroup_ebpfgo_socket_update_charts(struct cgroup *cg)
{
    if (!cg)
        return;

    if (!cg->enabled || cg->pending_renames)
        return;

    if (unlikely(!cgroup_ebpfgo_socket_snapshot_ready))
        return;

    // Don't create charts until the cgroup has actual socket activity.
    // Once st_net_conn_ipv4 exists the guard is skipped — charts persist even on idle.
    if (!cg->st_net_conn_ipv4 &&
        !cg->net.call_tcp_v4_connection && !cg->net.call_tcp_v6_connection &&
        !cg->net.bytes_received && !cg->net.bytes_sent &&
        !cg->net.call_tcp_received && !cg->net.call_tcp_sent &&
        !cg->net.retransmit && !cg->net.call_udp_received && !cg->net.call_udp_sent)
        return;

    const bool is_service = is_cgroup_systemd_service(cg);
    const int prio = (is_service ? NETDATA_CHART_PRIO_CGROUPS_SYSTEMD : NETDATA_CHART_PRIO_CGROUPS_CONTAINERS) + 5300;

    /* Socket deltas are collected on socket.conf's cadence, which may differ
     * from the SHM publisher cadence when cachestat owns publishing. */
    long ebpf_divisor = (cg->net.socket_update_every_s > 0) ?
        (long)cg->net.socket_update_every_s : (long)cgroup_update_every;

    uint64_t call_v4  = cg->net.call_tcp_v4_connection;
    uint64_t call_v6  = cg->net.call_tcp_v6_connection;
    uint64_t bytes_rx = cg->net.bytes_received;
    uint64_t bytes_tx = cg->net.bytes_sent;
    uint64_t tcp_rx   = cg->net.call_tcp_received;
    uint64_t tcp_tx   = cg->net.call_tcp_sent;
    uint64_t retrans  = cg->net.retransmit;
    uint64_t udp_rx   = cg->net.call_udp_received;
    uint64_t udp_tx   = cg->net.call_udp_sent;

    /* Update baked divisors when socket_update_every_s changes (e.g. ebpfgo.plugin
     * restarted with a different update_every).  Rare path: fires at most once per
     * divisor change.  rrddim_set_divisor returns 0 if divisor is unchanged. */
    if (unlikely(ebpf_divisor != cg->last_socket_divisor)) {
        if (cg->st_net_conn_ipv4) {
            RRDDIM *rd;
            if ((rd = rrddim_find_active(cg->st_net_conn_ipv4,  "connections"))) rrddim_set_divisor(cg->st_net_conn_ipv4,  rd, (int32_t)ebpf_divisor);
            if ((rd = rrddim_find_active(cg->st_net_conn_ipv6,  "connections"))) rrddim_set_divisor(cg->st_net_conn_ipv6,  rd, (int32_t)ebpf_divisor);
            if (cg->st_net_bw_rd_received) rrddim_set_divisor(cg->st_net_total_bandwidth, cg->st_net_bw_rd_received, (int32_t)(ebpf_divisor * 1000));
            if (cg->st_net_bw_rd_sent)     rrddim_set_divisor(cg->st_net_total_bandwidth, cg->st_net_bw_rd_sent,     (int32_t)(ebpf_divisor * 1000));
            if ((rd = rrddim_find_active(cg->st_net_tcp_recv,   "calls"))) rrddim_set_divisor(cg->st_net_tcp_recv,   rd, (int32_t)ebpf_divisor);
            if ((rd = rrddim_find_active(cg->st_net_tcp_send,   "calls"))) rrddim_set_divisor(cg->st_net_tcp_send,   rd, (int32_t)ebpf_divisor);
            if ((rd = rrddim_find_active(cg->st_net_retransmit, "calls"))) rrddim_set_divisor(cg->st_net_retransmit, rd, (int32_t)ebpf_divisor);
            if ((rd = rrddim_find_active(cg->st_net_udp_send,   "calls"))) rrddim_set_divisor(cg->st_net_udp_send,   rd, (int32_t)ebpf_divisor);
            if ((rd = rrddim_find_active(cg->st_net_udp_recv,   "calls"))) rrddim_set_divisor(cg->st_net_udp_recv,   rd, (int32_t)ebpf_divisor);
        }
        cg->last_socket_divisor = ebpf_divisor;
    }

    cgroup_ebpfgo_update_single_chart(
        cg, &cg->st_net_conn_ipv4, "outbound_conn_v4",
        "TCP v4 outbound connections",
        "network",
        is_service ? "systemd.service.net_conn_ipv4" : "cgroup.net_conn_ipv4",
        "connections", "connections/s", prio, ebpf_divisor, (collected_number)call_v4);

    cgroup_ebpfgo_update_single_chart(
        cg, &cg->st_net_conn_ipv6, "outbound_conn_v6",
        "TCP v6 outbound connections",
        "network",
        is_service ? "systemd.service.net_conn_ipv6" : "cgroup.net_conn_ipv6",
        "connections", "connections/s", prio + 1, ebpf_divisor, (collected_number)call_v6);

    // Bandwidth chart: two dimensions (received + sent), kilobits/s from bytes/interval
    {
        RRDSET *chart = cg->st_net_total_bandwidth;
        if (unlikely(!chart)) {
            char buff[RRD_ID_LENGTH_MAX + 1];
            chart = cg->st_net_total_bandwidth = rrdset_create_localhost(
                cgroup_chart_type(buff, cg),
                "total_bandwidth",
                NULL,
                "network",
                is_service ? "systemd.service.net_total_bandwidth" : "cgroup.net_total_bandwidth",
                "Bandwidth",
                "kilobits/s",
                PLUGIN_CGROUPS_NAME,
                is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
                prio + 2,
                cgroup_update_every,
                RRDSET_TYPE_LINE);
            rrdset_update_rrdlabels(chart, cg->chart_labels);
            cg->st_net_bw_rd_received = rrddim_add(chart, "received", NULL, 8, ebpf_divisor * 1000, RRD_ALGORITHM_ABSOLUTE);
            cg->st_net_bw_rd_sent     = rrddim_add(chart, "sent",     NULL, 8, ebpf_divisor * 1000, RRD_ALGORITHM_ABSOLUTE);
        }
        rrddim_set_by_pointer(chart, cg->st_net_bw_rd_received, (collected_number)bytes_rx);
        rrddim_set_by_pointer(chart, cg->st_net_bw_rd_sent,     (collected_number)bytes_tx);
        rrdset_done(chart);
    }

    cgroup_ebpfgo_update_single_chart(
        cg, &cg->st_net_tcp_recv, "bandwidth_tcp_recv",
        "TCP calls to cleanup buffer",
        "network",
        is_service ? "systemd.service.net_tcp_recv" : "cgroup.net_tcp_recv",
        "calls", "calls/s", prio + 3, ebpf_divisor, (collected_number)tcp_rx);

    cgroup_ebpfgo_update_single_chart(
        cg, &cg->st_net_tcp_send, "bandwidth_tcp_send",
        "TCP calls to send",
        "network",
        is_service ? "systemd.service.net_tcp_send" : "cgroup.net_tcp_send",
        "calls", "calls/s", prio + 4, ebpf_divisor, (collected_number)tcp_tx);

    cgroup_ebpfgo_update_single_chart(
        cg, &cg->st_net_retransmit, "bandwidth_tcp_retransmit",
        "TCP retransmits",
        "network",
        is_service ? "systemd.service.net_retransmit" : "cgroup.net_retransmit",
        "calls", "calls/s", prio + 5, ebpf_divisor, (collected_number)retrans);

    cgroup_ebpfgo_update_single_chart(
        cg, &cg->st_net_udp_send, "bandwidth_udp_send",
        "UDP calls to send",
        "network",
        is_service ? "systemd.service.net_udp_send" : "cgroup.net_udp_send",
        "calls", "calls/s", prio + 6, ebpf_divisor, (collected_number)udp_tx);

    cgroup_ebpfgo_update_single_chart(
        cg, &cg->st_net_udp_recv, "bandwidth_udp_recv",
        "UDP calls to receive",
        "network",
        is_service ? "systemd.service.net_udp_recv" : "cgroup.net_udp_recv",
        "calls", "calls/s", prio + 7, ebpf_divisor, (collected_number)udp_rx);
}

#endif
