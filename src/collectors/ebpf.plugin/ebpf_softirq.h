// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_SOFTIRQ_H
#define NETDATA_EBPF_SOFTIRQ_H 1

// Module observation
#define NETDATA_EBPF_SOFTIRQ_MODULE_DESC "Show time spent servicing individual software interrupt requests (soft IRQs)."

/*****************************************************************
 *  copied from kernel-collectors repo, with modifications needed
 *  for inclusion here.
 *****************************************************************/

#define NETDATA_SOFTIRQ_MAX_IRQS 10

typedef struct softirq_ebpf_val {
    uint64_t latency;
    uint64_t ts;
} softirq_ebpf_val_t;

/*****************************************************************
 * below this is eBPF plugin-specific code.
 *****************************************************************/

#define NETDATA_EBPF_MODULE_NAME_SOFTIRQ "softirq"
#define NETDATA_SOFTIRQ_CONFIG_FILE "softirq.conf"

typedef struct sofirq_val {
    uint64_t latency;
    char *name;
} softirq_val_t;

extern struct config softirq_config;
void ebpf_softirq_thread(void *ptr);

#endif /* NETDATA_EBPF_SOFTIRQ_H */
