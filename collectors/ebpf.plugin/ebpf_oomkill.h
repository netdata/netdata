// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_OOMKILL_H
#define NETDATA_EBPF_OOMKILL_H 1

/*****************************************************************
 *  copied from kernel-collectors repo, with modifications needed
 *  for inclusion here.
 *****************************************************************/

#define NETDATA_OOMKILL_MAX_ENTRIES 128
#define NETDATA_OOMKILL_TASK_COMM_LEN 16

typedef struct oomkill_ebpf_val {
    // how many times a process was killed.
    uint32_t killcnt;

    // command of the process as obtained from the kernel's task_struct for the
    // OOM killed process.
    char comm[NETDATA_OOMKILL_TASK_COMM_LEN];
} oomkill_ebpf_val_t;

/*****************************************************************
 * below this is eBPF plugin-specific code.
 *****************************************************************/

#define NETDATA_EBPF_MODULE_NAME_OOMKILL "oomkill"
#define NETDATA_OOMKILL_SLEEP_MS 650000ULL
#define NETDATA_OOMKILL_CONFIG_FILE "oomkill.conf"

extern struct config oomkill_config;
extern void *ebpf_oomkill_thread(void *ptr);

#endif /* NETDATA_EBPF_OOMKILL_H */
