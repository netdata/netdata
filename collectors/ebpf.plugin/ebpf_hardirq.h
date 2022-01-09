// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_HARDIRQ_H
#define NETDATA_EBPF_HARDIRQ_H 1

/*****************************************************************
 *  copied from kernel-collectors repo, with modifications needed
 *  for inclusion here.
 *****************************************************************/

#define NETDATA_HARDIRQ_NAME_LEN 32
#define NETDATA_HARDIRQ_MAX_IRQS 1024L

typedef struct hardirq_ebpf_key {
    int irq;
} hardirq_ebpf_key_t;

typedef struct hardirq_ebpf_val {
    uint64_t latency;
    uint64_t ts;
    char name[NETDATA_HARDIRQ_NAME_LEN];
} hardirq_ebpf_val_t;

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

typedef struct hardirq_ebpf_static_val {
    uint64_t latency;
    uint64_t ts;
} hardirq_ebpf_static_val_t;

/*****************************************************************
 * below this is eBPF plugin-specific code.
 *****************************************************************/

#define NETDATA_EBPF_MODULE_NAME_HARDIRQ "hardirq"
#define NETDATA_HARDIRQ_SLEEP_MS 650000ULL
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

extern struct config hardirq_config;
extern void *ebpf_hardirq_thread(void *ptr);

#endif /* NETDATA_EBPF_HARDIRQ_H */
