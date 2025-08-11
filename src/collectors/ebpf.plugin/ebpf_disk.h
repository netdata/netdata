// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_DISK_H
#define NETDATA_EBPF_DISK_H 1

// Module name & description
#define NETDATA_EBPF_MODULE_NAME_DISK "disk"
#define NETDATA_EBPF_DISK_MODULE_DESC "Monitor disk latency independent of filesystem."

#include "libnetdata/avl/avl.h"
#include "libbpf_api/ebpf.h"

#define NETDATA_EBPF_PROC_PARTITIONS "/proc/partitions"

// Process configuration name
#define NETDATA_DISK_CONFIG_FILE "disk.conf"

// Decode function extracted from: https://elixir.bootlin.com/linux/v5.10.8/source/include/linux/kdev_t.h#L7
#define MINORBITS 20
#define MKDEV(ma, mi) (((ma) << MINORBITS) | (mi))

enum netdata_latency_disks_flags {
    NETDATA_DISK_NONE = 0,
    NETDATA_DISK_ADDED_TO_PLOT_LIST = 1,
    NETDATA_DISK_CHART_CREATED = 2,
    NETDATA_DISK_IS_HERE = 4,
    NETDATA_DISK_HAS_EFI = 8
};

/*
 * The definition (DISK_NAME_LEN) has been a stable value since Kernel 3.0,
 * I decided to bring it as internal definition, to avoid include linux/genhd.h.
 */
#define NETDATA_DISK_NAME_LEN 32
typedef struct netdata_ebpf_disks {
    // Search
    avl_t avl;
    uint32_t dev;
    uint32_t major;
    uint32_t minor;
    uint32_t bootsector_key;
    uint64_t start; // start sector
    uint64_t end;   // end sector

    // Print information
    char family[NETDATA_DISK_NAME_LEN + 1];
    char *boot_chart;

    netdata_ebpf_histogram_t histogram;

    uint32_t flags;
    time_t last_update;

    struct netdata_ebpf_disks *main;
    struct netdata_ebpf_disks *boot_partition;
    struct netdata_ebpf_disks *next;
} netdata_ebpf_disks_t;

enum ebpf_disk_tables { NETDATA_DISK_IO };

typedef struct block_key {
    uint32_t bin;
    uint32_t dev;
} block_key_t;

typedef struct netdata_ebpf_publish_disk {
    netdata_ebpf_disks_t *plot;
    struct netdata_ebpf_publish_disk *next;
} ebpf_publish_disk_t;

#define NETDATA_EBPF_DISK_LATENCY_CONTEXT "disk.latency_io"

extern struct config disk_config;

void ebpf_disk_thread(void *ptr);

#endif /* NETDATA_EBPF_DISK_H */
