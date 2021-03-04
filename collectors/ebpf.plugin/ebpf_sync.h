// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_SYNC_H
#define NETDATA_EBPF_SYNC_H 1

// charts
#define NETDATA_EBPF_SYNC_CHART "sync"
#define NETDATA_EBPF_SYNC_SUBMENU "synchronization"

#define NETDATA_EBPF_SYNC_SLEEP_MS 800000ULL

enum netdata_sync_charts {
    NETDATA_SYNC_CALL,
    NETDATA_SYNC_ERROR,

    NETDATA_SYNC_END
};

enum netdata_sync_table {
    NETDATA_SYNC_GLOBLAL_TABLE
};

extern void *ebpf_sync_thread(void *ptr);

#endif