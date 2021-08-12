// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_HARDIRQ_H
#define NETDATA_EBPF_HARDIRQ_H 1

#define NETDATA_EBPF_MODULE_NAME_HARDIRQ "hardirq"
#define NETDATA_HARDIRQ_SLEEP_MS 650000ULL
#define NETDATA_HARDIRQ_CONFIG_FILE "hardirq.conf"
#define NETDATA_HARDIRQ_NAME_LEN 32
#define NETDATA_HARDIRQ_MAX_IRQS 1024L

// these must match the kernel-collectors repo side, since they're used to get
// from the eBPF map.
typedef struct hardirq_ebpf_key {
    int irq;
} hardirq_ebpf_key_t;
typedef struct hardirq_ebpf_val {
    uint64_t latency;
    uint64_t ts;
    char name[NETDATA_HARDIRQ_NAME_LEN];
} hardirq_ebpf_val_t;

typedef struct hardirq_val {
    // must be at top for simplified AVL tree usage.
    // if it's not at the top, we need to use `containerof` for almost all ops.
    avl_t avl;

    int irq;
    uint64_t latency;
    bool dim_exists;
    char name[NETDATA_HARDIRQ_NAME_LEN];
} hardirq_val_t;

extern struct config hardirq_config;
extern void *ebpf_hardirq_thread(void *ptr);
extern void ebpf_hardirq_create_apps_charts(struct ebpf_module *em, void *ptr);

#endif /* NETDATA_EBPF_HARDIRQ_H */
