// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_SWAP_H
#define NETDATA_EBPF_SWAP_H 1

// Module name
#define NETDATA_EBPF_MODULE_NAME_SWAP "swap"

#define NETDATA_SWAP_SLEEP_MS 850000ULL

// charts
#define NETDATA_MEM_SWAP_CHART "swapcalls"
#define NETDATA_MEM_SWAP_READ_CHART "swap_read_call"
#define NETDATA_MEM_SWAP_WRITE_CHART "swap_write_call"
#define NETDATA_SWAP_SUBMENU "swap"

// configuration file
#define NETDATA_DIRECTORY_SWAP_CONFIG_FILE "swap.conf"

// Contexts
#define NETDATA_CGROUP_SWAP_READ_CONTEXT "cgroup.swap_read"
#define NETDATA_CGROUP_SWAP_WRITE_CONTEXT "cgroup.swap_write"
#define NETDATA_SYSTEMD_SWAP_READ_CONTEXT "services.swap_read"
#define NETDATA_SYSTEMD_SWAP_WRITE_CONTEXT "services.swap_write"

typedef struct netdata_publish_swap {
    uint64_t read;
    uint64_t write;
} netdata_publish_swap_t;

enum swap_tables {
    NETDATA_PID_SWAP_TABLE,
    NETDATA_SWAP_CONTROLLER,
    NETDATA_SWAP_GLOBAL_TABLE
};

enum swap_counters {
    NETDATA_KEY_SWAP_READPAGE_CALL,
    NETDATA_KEY_SWAP_WRITEPAGE_CALL,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_SWAP_END
};

extern netdata_publish_swap_t **swap_pid;

extern void *ebpf_swap_thread(void *ptr);
extern void ebpf_swap_create_apps_charts(struct ebpf_module *em, void *ptr);
extern void clean_swap_pid_structures();

extern struct config swap_config;

#endif
