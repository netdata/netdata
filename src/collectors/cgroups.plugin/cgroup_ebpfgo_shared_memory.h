// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_EBPFGO_SHARED_MEMORY_H
#define NETDATA_CGROUP_EBPFGO_SHARED_MEMORY_H 1

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)

#include "../collectors-ipc/ebpfgo_shared_memory.h"

bool cgroup_ebpfgo_shared_memory_refresh(void);
const struct ebpf_pid_stat *cgroup_ebpfgo_shared_memory_lookup(pid_t pid);
uint32_t cgroup_ebpfgo_shared_memory_flags(void);
void cgroup_ebpfgo_shared_memory_close(void);

struct cgroup;
/* dimension_name NULL shows the dimension id. */
void cgroup_ebpfgo_update_single_chart_named(
    struct cgroup *cg,
    RRDSET **chart_ptr,
    const char *chart_id,
    const char *title,
    const char *family,
    const char *context,
    const char *dimension,
    const char *dimension_name,
    const char *units,
    int priority,
    collected_number divisor,
    collected_number value);

static inline void cgroup_ebpfgo_update_single_chart(
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
    cgroup_ebpfgo_update_single_chart_named(
        cg, chart_ptr, chart_id, title, family, context, dimension, NULL, units, priority, divisor, value);
}

#endif

#endif
