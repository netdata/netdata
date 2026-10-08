// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"
#include "cgroup_ebpfgo_shared_memory.h"

#if defined(OS_LINUX)

static netdata_ebpfgo_shared_pid_memory_t cgroup_ebpfgo_shared_memory_ctx = {
    .shm_fd = -1,
    .sem = SEM_FAILED,
};

/* Creates (on first use) and updates one single-dimension eBPFGo chart for a
 * cgroup.  Every ebpfgo cgroup consumer needs exactly this, differing only in
 * the chart family, so the three modules share one implementation.
 *
 * divisor is the dimension divisor: pass the publisher's interval to turn a
 * per-interval delta into a rate, or 1 for an absolute value. */
void cgroup_ebpfgo_update_single_chart(
    struct cgroup *cg,
    RRDSET **chart_ptr,
    const char *chart_id,
    const char *title,
    const char *family,
    const char *context,
    const char *dimension,
    const char *units,
    int priority,
    collected_number divisor,
    collected_number value)
{
    RRDSET *chart = *chart_ptr;

    if (unlikely(!chart)) {
        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = *chart_ptr = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            chart_id,
            NULL,
            family,
            context,
            title,
            units,
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            priority,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, dimension, NULL, 1, divisor ? divisor : 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set(chart, dimension, value);
    rrdset_done(chart);
}

bool cgroup_ebpfgo_shared_memory_refresh(void)
{
    return netdata_ebpfgo_shared_pid_memory_refresh(
        &cgroup_ebpfgo_shared_memory_ctx,
        NETDATA_EBPFGO_INTEGRATION_NAME,
        NETDATA_EBPFGO_SHM_INTEGRATION_NAME);
}

const struct ebpf_pid_stat *cgroup_ebpfgo_shared_memory_lookup(pid_t pid)
{
    return netdata_ebpfgo_shared_pid_memory_lookup(&cgroup_ebpfgo_shared_memory_ctx, pid);
}

uint32_t cgroup_ebpfgo_shared_memory_flags(void)
{
    return netdata_ebpfgo_shared_pid_memory_flags(&cgroup_ebpfgo_shared_memory_ctx);
}

void cgroup_ebpfgo_shared_memory_close(void)
{
    netdata_ebpfgo_shared_pid_memory_close(&cgroup_ebpfgo_shared_memory_ctx);
}

#endif
