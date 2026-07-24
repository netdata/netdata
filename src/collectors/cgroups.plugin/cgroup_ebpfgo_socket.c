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

static inline uint64_t cgroup_ebpfgo_socket_delta(uint64_t current, uint64_t prev)
{
    return (current >= prev) ? (current - prev) : 0;
}

static void cgroup_ebpfgo_socket_sum_pids(struct cgroup *cg)
{
    char path_buf[FILENAME_MAX + 1];

    cg->net.prev = cg->net.current;
    memset(&cg->net.current, 0, sizeof(cg->net.current));

    procfile *ff = cgroup_ebpfgo_open_nonempty_procs_file(path_buf, sizeof(path_buf), cg->id);
    if (!ff)
        goto done;

    /* cgroup_ebpfgo_open_nonempty_procs_file() returns a procfile that has
     * already been procfile_readall()'d while selecting the best mount. */

    for (size_t l = 0; l < procfile_lines(ff); l++) {
        pid_t pid = (pid_t)str2l(procfile_lineword(ff, l, 0));
        if (pid <= 0)
            continue;

        const struct ebpf_pid_stat *item = cgroup_ebpfgo_shared_memory_lookup(pid);
        if (!item)
            continue;

        const struct ebpf_socket_publish_apps *s = &item->socket;
        cg->net.current.bytes_sent             += s->bytes_sent;
        cg->net.current.bytes_received         += s->bytes_received;
        cg->net.current.call_tcp_sent          += s->call_tcp_sent;
        cg->net.current.call_tcp_received      += s->call_tcp_received;
        cg->net.current.retransmit             += s->retransmit;
        cg->net.current.call_udp_sent          += s->call_udp_sent;
        cg->net.current.call_udp_received      += s->call_udp_received;
        cg->net.current.call_close             += s->call_close;
        cg->net.current.call_tcp_v4_connection += s->call_tcp_v4_connection;
        cg->net.current.call_tcp_v6_connection += s->call_tcp_v6_connection;
    }

    procfile_close(ff);

done:
    /* On the first sample, after a plugin restart, or after container
     * re-discovery, prev is all zeros while current holds the full
     * cumulative eBPF counters.  Setting prev = current here makes every
     * delta zero for this cycle, preventing the spike that would otherwise
     * occur.  Mirrors the guard in cgroup_ebpfgo_cachestat.c:244. */
    if (!cg->net.prev.bytes_sent && !cg->net.prev.bytes_received &&
        !cg->net.prev.call_tcp_sent && !cg->net.prev.call_tcp_received &&
        !cg->net.prev.retransmit && !cg->net.prev.call_udp_sent &&
        !cg->net.prev.call_udp_received && !cg->net.prev.call_close &&
        !cg->net.prev.call_tcp_v4_connection && !cg->net.prev.call_tcp_v6_connection)
        cg->net.prev = cg->net.current;
}

static void cgroup_ebpfgo_socket_update_single_chart(
    struct cgroup *cg,
    RRDSET **chart_ptr,
    const char *chart_id,
    const char *title,
    const char *context,
    const char *dimension,
    const char *units,
    int priority,
    collected_number value)
{
    RRDSET *chart = *chart_ptr;

    if (unlikely(!chart)) {
        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = *chart_ptr = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            chart_id,
            NULL,
            "network",
            context,
            title,
            units,
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            priority,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, dimension, NULL, 1, cgroup_update_every, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set(chart, dimension, value);
    rrdset_done(chart);
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

    const bool is_service = is_cgroup_systemd_service(cg);
    const int prio = (is_service ? NETDATA_CHART_PRIO_CGROUPS_SYSTEMD : NETDATA_CHART_PRIO_CGROUPS_CONTAINERS) + 5300;

    uint64_t call_v4  = cgroup_ebpfgo_socket_delta(cg->net.current.call_tcp_v4_connection,
                                                    cg->net.prev.call_tcp_v4_connection);
    uint64_t call_v6  = cgroup_ebpfgo_socket_delta(cg->net.current.call_tcp_v6_connection,
                                                    cg->net.prev.call_tcp_v6_connection);
    uint64_t bytes_rx = cgroup_ebpfgo_socket_delta(cg->net.current.bytes_received,
                                                    cg->net.prev.bytes_received);
    uint64_t bytes_tx = cgroup_ebpfgo_socket_delta(cg->net.current.bytes_sent,
                                                    cg->net.prev.bytes_sent);
    uint64_t tcp_rx   = cgroup_ebpfgo_socket_delta(cg->net.current.call_tcp_received,
                                                    cg->net.prev.call_tcp_received);
    uint64_t tcp_tx   = cgroup_ebpfgo_socket_delta(cg->net.current.call_tcp_sent,
                                                    cg->net.prev.call_tcp_sent);
    uint64_t retrans  = cgroup_ebpfgo_socket_delta(cg->net.current.retransmit,
                                                    cg->net.prev.retransmit);
    uint64_t udp_rx   = cgroup_ebpfgo_socket_delta(cg->net.current.call_udp_received,
                                                    cg->net.prev.call_udp_received);
    uint64_t udp_tx   = cgroup_ebpfgo_socket_delta(cg->net.current.call_udp_sent,
                                                    cg->net.prev.call_udp_sent);

    cgroup_ebpfgo_socket_update_single_chart(
        cg, &cg->st_net_conn_ipv4, "outbound_conn_v4",
        "TCP v4 outbound connections",
        is_service ? "systemd.service.net_conn_ipv4" : "cgroup.net_conn_ipv4",
        "connections", "connections/s", prio, (collected_number)call_v4);

    cgroup_ebpfgo_socket_update_single_chart(
        cg, &cg->st_net_conn_ipv6, "outbound_conn_v6",
        "TCP v6 outbound connections",
        is_service ? "systemd.service.net_conn_ipv6" : "cgroup.net_conn_ipv6",
        "connections", "connections/s", prio + 1, (collected_number)call_v6);

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
            // bytes/interval * 8 / 1000 / cgroup_update_every = kilobits/s
            cg->st_net_bw_rd_received = rrddim_add(chart, "received", NULL, 8, (long)(cgroup_update_every) * 1000, RRD_ALGORITHM_ABSOLUTE);
            cg->st_net_bw_rd_sent     = rrddim_add(chart, "sent",     NULL, 8, (long)(cgroup_update_every) * 1000, RRD_ALGORITHM_ABSOLUTE);
        }
        rrddim_set_by_pointer(chart, cg->st_net_bw_rd_received, (collected_number)bytes_rx);
        rrddim_set_by_pointer(chart, cg->st_net_bw_rd_sent,     (collected_number)bytes_tx);
        rrdset_done(chart);
    }

    cgroup_ebpfgo_socket_update_single_chart(
        cg, &cg->st_net_tcp_recv, "bandwidth_tcp_recv",
        "TCP calls to cleanup buffer",
        is_service ? "systemd.service.net_tcp_recv" : "cgroup.net_tcp_recv",
        "calls", "calls/s", prio + 3, (collected_number)tcp_rx);

    cgroup_ebpfgo_socket_update_single_chart(
        cg, &cg->st_net_tcp_send, "bandwidth_tcp_send",
        "TCP calls to send",
        is_service ? "systemd.service.net_tcp_send" : "cgroup.net_tcp_send",
        "calls", "calls/s", prio + 4, (collected_number)tcp_tx);

    cgroup_ebpfgo_socket_update_single_chart(
        cg, &cg->st_net_retransmit, "bandwidth_tcp_retransmit",
        "TCP retransmits",
        is_service ? "systemd.service.net_retransmit" : "cgroup.net_retransmit",
        "calls", "calls/s", prio + 5, (collected_number)retrans);

    cgroup_ebpfgo_socket_update_single_chart(
        cg, &cg->st_net_udp_send, "bandwidth_udp_send",
        "UDP calls to send",
        is_service ? "systemd.service.net_udp_send" : "cgroup.net_udp_send",
        "calls", "calls/s", prio + 6, (collected_number)udp_tx);

    cgroup_ebpfgo_socket_update_single_chart(
        cg, &cg->st_net_udp_recv, "bandwidth_udp_recv",
        "UDP calls to receive",
        is_service ? "systemd.service.net_udp_recv" : "cgroup.net_udp_recv",
        "calls", "calls/s", prio + 7, (collected_number)udp_rx);
}

#endif
