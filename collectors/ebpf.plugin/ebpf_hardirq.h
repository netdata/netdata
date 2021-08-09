// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_HARDIRQ_H
#define NETDATA_EBPF_HARDIRQ_H 1

#define NETDATA_EBPF_MODULE_NAME_HARDIRQ "hardirq"
#define NETDATA_HARDIRQ_SLEEP_MS 650000ULL
#define NETDATA_HARDIRQ_CONFIG_FILE "hardirq.conf"

// from kernel-collectors repo definitions.
#define NETDATA_HARDIRQ_NAME_LEN 32
typedef struct hardirq_key {
    int irq;
} hardirq_key_t;
typedef struct hardirq_val {
    u64 latency;
    u64 ts;
    char name[NETDATA_HARDIRQ_NAME_LEN];
} hardirq_val_t;

extern struct config hardirq_config;
extern void *ebpf_hardirq_thread(void *ptr);
extern void ebpf_hardirq_create_apps_charts(struct ebpf_module *em, void *ptr);

#endif /* NETDATA_EBPF_HARDIRQ_H */
