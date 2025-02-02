// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_HARDIRQ_H
#define NETDATA_EBPF_HARDIRQ_H 1

// Module description
#define NETDATA_EBPF_HARDIRQ_MODULE_DESC "Show time spent servicing individual hardware interrupt requests (hard IRQs)."

#include <stdint.h>
#include "libnetdata/avl/avl.h"

/*****************************************************************
 *  copied from kernel-collectors repo, with modifications needed
 *  for inclusion here.
 *****************************************************************/

#define NETDATA_HARDIRQ_NAME_LEN 32
#define NETDATA_HARDIRQ_MAX_IRQS 1024L

typedef struct hardirq_ebpf_key {
    int irq;
} hardirq_ebpf_key_t;

enum hardirq_ebpf_static {
    HARDIRQ_EBPF_STATIC_APIC_THERMAL,
    HARDIRQ_EBPF_STATIC_APIC_THRESHOLD,
    HARDIRQ_EBPF_STATIC_APIC_ERROR,
    HARDIRQ_EBPF_STATIC_APIC_DEFERRED_ERROR,
    HARDIRQ_EBPF_STATIC_APIC_SPURIOUS,
    HARDIRQ_EBPF_STATIC_FUNC_CALL,
    HARDIRQ_EBPF_STATIC_FUNC_CALL_SINGLE,
    HARDIRQ_EBPF_STATIC_RESCHEDULE,
    HARDIRQ_EBPF_STATIC_LOCAL_TIMER,
    HARDIRQ_EBPF_STATIC_IRQ_WORK,
    HARDIRQ_EBPF_STATIC_X86_PLATFORM_IPI,

    HARDIRQ_EBPF_STATIC_END
};

enum hardirq_maps { HARDIRQ_MAP_LATENCY, HARDIRQ_MAP_LATENCY_STATIC };

typedef struct hardirq_ebpf_static_val {
    uint64_t latency;
    uint64_t ts;
} hardirq_ebpf_static_val_t;

/*****************************************************************
 * below this is eBPF plugin-specific code.
 *****************************************************************/

// ARAL Name
#define NETDATA_EBPF_HARDIRQ_ARAL_NAME "ebpf_harddirq"

#define NETDATA_EBPF_MODULE_NAME_HARDIRQ "hardirq"
#define NETDATA_HARDIRQ_CONFIG_FILE "hardirq.conf"

typedef struct hardirq_val {
    // must be at top for simplified AVL tree usage.
    // if it's not at the top, we need to use `containerof` for almost all ops.
    avl_t avl;

    int irq;
    bool dim_exists; // keep this after `int irq` for alignment byte savings.
    uint64_t latency;
    char name[NETDATA_HARDIRQ_NAME_LEN];
} hardirq_val_t;

typedef struct hardirq_static_val {
    enum hardirq_ebpf_static idx;
    char *name;
    uint64_t latency;
} hardirq_static_val_t;

#define NETDATA_EBPF_SYSTEM_HARDIRQ_LATENCY_CTX "system.hardirq_latency"

extern struct config hardirq_config;
void *ebpf_hardirq_thread(void *ptr);

#endif /* NETDATA_EBPF_HARDIRQ_H */
