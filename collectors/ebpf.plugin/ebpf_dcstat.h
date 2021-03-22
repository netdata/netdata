// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_DCSTAT_H
#define NETDATA_EBPF_DCSTAT_H 1

// configuration file
#define NETDATA_DIRECTORY_DCSTAT_CONFIG_FILE "dcstat.conf"

enum directory_cache_indexes {
    NETDATA_DCSTAT_IDX_RATIO,
    NETDATA_DCSTAT_IDX_REFERENCE,
    NETDATA_DCSTAT_IDX_SLOW,
    NETDATA_DCSTAT_IDX_MISS,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_DCSTAT_IDX_END
};

extern void *ebpf_dcstat_thread(void *ptr);
extern void ebpf_dcstat_create_apps_charts(struct ebpf_module *em, void *ptr);

#endif // NETDATA_EBPF_DCSTAT_H
