function component(name) {
    return {
        type: "component",
        name: name,
    }
}

function application(name) {
    return {
        type: "app",
        name: name,
    }
}

let itemIcons = {
    "none": { },
    "sectionSummary": {
        type: "image",
        url: "summary-icon.png",
    },
    "storage": {
        type: "image",
        url: "storage-icon.png"
    }
}

let itemProperties = {
    topLevelCategory: {
        top: true,
        alwaysVisible: true,
        interactive: true,
    },
    keyCategoryExpandable: {
        top: false,
        bold: true,
        interactive: true,
        alwaysVisible: false,
        spacingBefore: false,
        spacingAfter: false,
        startExpanded: false,
    },
    keyCategoryExpanded: {
        top: false,
        bold: true,
        interactive: false,
        alwaysVisible: false,
        spacingBefore: true,
        spacingAfter: false,
        startExpanded: true,
    },
    simpleCategoryExpandable: {
        top: false,
        bold: false,
        interactive: true,
        alwaysVisible: false,
        spacingBefore: false,
        spacingAfter: false,
        startExpanded: false,
    },
    leafItem: {
        top: false,
        bold: false,
        interactive: true,
        alwaysVisible: false,
        spacingBefore: false,
        spacingAfter: false,
        startExpanded: false,
    }
}

let virtualContexts = {
    "virtual.storage.disks.table": {
        title: "Block Devices Summary",
        contexts: [
            "disk.io",
            "disk.await",
            "disk.ops",
            "disk.util"
        ],
        groupBy: [
            [ "label:device", "node" ]
        ],
        rows: [ "label:device", "node" ],
        actions: [
            {
                on: "label:device",
                icon: "filter",
                do: {
                    type: "section-filter",
                }
            },
            {
                on: "label:device",
                icon: "logs",
                do: {
                    type: "modal",
                    call: {
                        dashboard: "unixBlockDevice",
                        parameters: [
                            {
                                "node": "node",
                                "device": "label:device"
                            }
                        ]
                    }
                }
            },
        ]
    }
}

let systemSection = {
    items: {
        title: "Storage",
        contexts: [ "system.io" ],

    }
}

let storageSectionBlockDevices = {
    title: "Block Devices",
    properties: itemProperties.keyCategoryExpandable,
    priorityOrder: false,
    families: false,
    sectionFilters: [ "nodes", "label:device" ],
    contexts: [ virtualContexts["virtual.storage.disks.table"] ],
    items: [
        {
            title: "Throughput",
            tooltip: "The I/O throughput of your disks.",
            properties: itemProperties.leafItem,
            priorityOrder: false,
            families: false,
            contexts: [
                "disk.io",
                "disk_ext.io",
                "disk.avgsz",
                "disk_ext.avgsz"
            ]
        },
        {
            title: "Operations",
            tooltip: "The I/O operations your disks perform.",
            properties: itemProperties.leafItem,
            priorityOrder: false,
            families: false,
            contexts: [
                "disk.ops",
                "disk_ext.ops",
                "disk.mops",
                "disk_ext.mops",
            ]
        },
        {
            title: "Latency",
            tooltip: "The latency of your disks.",
            properties: itemProperties.leafItem,
            priorityOrder: false,
            families: false,
            contexts: [
                "disk.await",
                "disk_ext.await",
                "disk.svctm",
                "disk.latency_io",
            ]
        },
        {
            title: "Utilization",
            tooltip: "The utilization of your disks.",
            properties: itemProperties.leafItem,
            priorityOrder: false,
            families: false,
            contexts: [
                "disk.util",
                "disk.busy",
                "disk.iotime",
                "disk_ext.iotime",
            ]
        },
        {
            title: "Backlog",
            tooltip: "The amount of work that is queued.",
            properties: itemProperties.leafItem,
            priorityOrder: false,
            families: false,
            contexts: [
                "disk.qops",
                "disk.backlog",
            ]
        },
        {
            title: "BCache",
            tooltip: "The performance of bcache on your disks.",
            properties: itemProperties.leafItem,
            priorityOrder: true,
            families: false,
            contexts: [
                "disk.bcache",
                "disk.bcache_bypass",
                "disk.bcache_cache_alloc",
                "disk.bcache_cache_read_races",
                "disk.bcache_hit_ratio",
                "disk.bcache_rates",
                "disk.bcache_size",
                "disk.bcache_usage",
            ]
        },
    ]
}

let storageSectionNVMe = {
    title: "NVMe Disks",
    app: component("nvme"),
    tooltip: "Performance of your NVMe Disks",
    dyncfg: "go.d:collector:nvme",
    properties: itemProperties.keyCategoryExpandable,
    priorityOrder: true,
    families: true,
    contexts: [
        "nvme.device_available_spare_perc",
        "nvme.device_composite_temperature",
        "nvme.device_critical_composite_temperature_time",
        "nvme.device_critical_warnings_state",
        "nvme.device_error_log_entries_rate",
        "nvme.device_estimated_endurance_perc",
        "nvme.device_io_transferred_count",
        "nvme.device_media_errors_rate",
        "nvme.device_power_cycles_count",
        "nvme.device_power_on_time",
        "nvme.device_thermal_mgmt_temp1_time",
        "nvme.device_thermal_mgmt_temp1_transitions_rate",
        "nvme.device_thermal_mgmt_temp2_time",
        "nvme.device_thermal_mgmt_temp2_transitions_rate",
        "nvme.device_unsafe_shutdowns_count",
        "nvme.device_warning_composite_temperature_time",
    ]
}


let storageSection = {
    title: "Storage",
    icon: itemIcons.storage,
    properties: itemProperties.category,
    contexts: [],
    items: [
        {
            title: "Overall",
            icon: itemIcons.sectionSummary,
            properties: itemProperties.keyCategoryExpandable,
            priorityOrder: false,
            families: false,
            contexts: [ "system.io" ],
            items: [],
        },
        storageSectionBlockDevices,
        {
            title: "Mounted Filesystems",
            tooltip: "Usage of the mounted file systems, per mount point.",
            properties: itemProperties.keyCategoryExpandable,
            header: [

            ],
            items: [
                virtualContexts["virtual.storage.mounts.table"],
                {
                    title: "Disk Space Usage",
                    tooltip: "Disk space usage per mount point.",
                    priorityOrder: false,
                    families: false,
                    contexts: [
                        "disk.space" // should be renamed to system.storage.fs.space
                    ]
                },
                {
                    title: "Disk Inodes Usage",
                    tooltip: "Disk inodes usage per mount point.",
                    priorityOrder: false,
                    families: false,
                    contexts: [
                        "disk.inodes" // should be renamed to system.storage.fs.inodes
                    ]
                },
                {
                    title: "Mount and unmount Calls",
                    tooltip: "Mount and unmount calls status",
                    priorityOrder: false,
                    families: false,
                    contexts: [
                        "mount_points.call", // should be renamed to system.storage.fs.mount.calls (dimension: ok)
                        "mount_points.error", // should be renamed to system.storage.fs.mount.calls (dimension: failed)
                    ]
                }
            ],
        },
        {
            title: "Storage Pressure",
            properties: itemProperties.keyCategoryExpandable,
            items: [
                {
                    title: "Some Pressure",
                    priorityOrder: false,
                    families: false,
                    contexts: [
                        "system.io_some_pressure",
                        "system.io_some_pressure_stall_time",
                    ],
                },
                {
                    title: "Full Pressure",
                    priorityOrder: false,
                    families: false,
                    contexts: [
                        "system.io_full_pressure",
                        "system.io_full_pressure_stall_time",
                    ]
                }
            ]
        },
        {
            title: "kernel Storage Layer",
            properties: itemProperties.simpleCategoryExpandable,
            items: [
                {
                    title: "Directory Cache",
                    priorityOrder: false,
                    families: false,
                    contexts: [
                        "filesystem.dc_hit_ratio",
                        "filesystem.dc_reference",
                    ],
                },
                {
                    title: "Calls and Latency",
                    priorityOrder: false,
                    families: false,
                    contexts: [
                        "filesystem.file_descriptor",
                        "filesystem.file_error",
                        "filesystem.open_latency",
                        "filesystem.read_latency",
                        "filesystem.write_latency",
                        "filesystem.sync_latency",
                    ]
                },
                {
                    title: "VFS - Virtual File System",
                    priorityOrder: false,
                    families: false,
                    contexts: [
                        "filesystem.vfs_create",
                        "filesystem.vfs_create_error",
                        "filesystem.vfs_deleted_objects",
                        "filesystem.vfs_fsync",
                        "filesystem.vfs_fsync_error",
                        "filesystem.vfs_io",
                        "filesystem.vfs_io_bytes",
                        "filesystem.vfs_io_error",
                        "filesystem.vfs_open",
                        "filesystem.vfs_open_error",
                    ]
                }
            ]
        },
        {
            title: "Storage Technologies",
            properties: itemProperties.keyCategoryExpanded,
            items: [
                {
                    title: "BTRFS",
                    app: component("btrfs"),
                    tooltip: "Performance of your BTRFS disks",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "btrfs.commit_timings",
                        "btrfs.commits",
                        "btrfs.commits_perc_time",
                        "btrfs.data",
                        "btrfs.device_errors",
                        "btrfs.disk",
                        "btrfs.metadata",
                        "btrfs.system",
                    ]
                },
                {
                    title: "ZFS",
                    app: component("zfs"),
                    tooltip: "Performance of your ZFS",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "zfs.actual_hits",
                        "zfs.actual_hits_rate",
                        "zfs.arc_size",
                        "zfs.arc_size_breakdown",
                        "zfs.bytes",
                        "zfs.demand_data_hits",
                        "zfs.demand_data_hits_rate",
                        "zfs.dhits",
                        "zfs.dhits_rate",
                        "zfs.hash_chains",
                        "zfs.hash_elements",
                        "zfs.hits",
                        "zfs.hits_rate",
                        "zfs.important_ops",
                        "zfs.l2_size",
                        "zfs.l2hits",
                        "zfs.l2hits_rate",
                        "zfs.list_hits",
                        "zfs.memory_ops",
                        "zfs.mhits",
                        "zfs.mhits_rate",
                        "zfs.phits",
                        "zfs.phits_rate",
                        "zfs.prefetch_data_hits",
                        "zfs.prefetch_data_hits_rate",
                        "zfs.reads",
                        "zfs.trim_bytes",
                        "zfs.trim_requests",
                        "zfspool.state",
                    ]
                },
                {
                    title: "NFS Server",
                    app: component("nfs-server"),
                    tooltip: "Performance of your NFS Server",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "nfsd.filehandles",
                        "nfsd.io",
                        "nfsd.net",
                        "nfsd.proc2",
                        "nfsd.proc3",
                        "nfsd.proc4",
                        "nfsd.proc4ops",
                        "nfsd.readcache",
                        "nfsd.rpc",
                        "nfsd.threads",
                    ]
                },
                {
                    title: "NFS Client",
                    app: component("nfs-client"),
                    tooltip: "Performance of your NFS Mounts",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "nfs.net",
                        "nfs.proc2",
                        "nfs.proc3",
                        "nfs.proc4",
                        "nfs.rpc",
                    ]
                },
                {
                    title: "SMBFS",
                    app: component("samba2"),
                    tooltip: "Samba2 related metrics.",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "smb2.create_close",
                        "smb2.find",
                        "smb2.get_set_info",
                        "smb2.notify",
                        "smb2.rw",
                        "smb2.sm_counters",
                        "syscall.rw",
                    ]
                },
                {
                    title: "HDFS",
                    app: component("hdfs"),
                    tooltip: "Performance of your HDFS",
                    dyncfg: "go.d:collector:hdfs",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "hdfs.avg_processing_time",
                        "hdfs.avg_queue_time",
                        "hdfs.blocks",
                        "hdfs.blocks_total",
                        "hdfs.call_queue_length",
                        "hdfs.capacity",
                        "hdfs.data_nodes",
                        "hdfs.datanode_bandwidth",
                        "hdfs.datanode_capacity",
                        "hdfs.datanode_failed_volumes",
                        "hdfs.datanode_used_capacity",
                        "hdfs.files_total",
                        "hdfs.gc_count_total",
                        "hdfs.gc_threshold",
                        "hdfs.gc_time_total",
                        "hdfs.heap_memory",
                        "hdfs.load",
                        "hdfs.logs_total",
                        "hdfs.open_connections",
                        "hdfs.rpc_bandwidth",
                        "hdfs.rpc_calls",
                        "hdfs.threads",
                        "hdfs.used_capacity",
                        "hdfs.volume_failures_total",
                    ]
                },
                {
                    title: "IPFS",
                    app: component("ipfs"),
                    tooltip: "Performance of your IPFS",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "ipfs.bandwidth",
                        "ipfs.peers",
                        "ipfs.repo_objects",
                        "ipfs.repo_size",
                    ]
                },
                {
                    title: "Ceph",
                    app: component("ceph"),
                    tooltip: "Metric for Ceph",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "ceph.apply_latency",
                        "ceph.commit_latency",
                        "ceph.general_bytes",
                        "ceph.general_latency",
                        "ceph.general_objects",
                        "ceph.general_operations",
                        "ceph.general_usage",
                        "ceph.osd_size",
                        "ceph.osd_usage",
                        "ceph.pool_objects",
                        "ceph.pool_read_bytes",
                        "ceph.pool_read_operations",
                        "ceph.pool_usage",
                        "ceph.pool_write_bytes",
                        "ceph.pool_write_operations",
                    ]
                },
                {
                    title: "Software RAID",
                    app: component("md"),
                    tooltip: "Status and performance of your MD devices.",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "md.disks",
                        "md.expected_time_until_operation_finish",
                        "md.health",
                        "md.mismatch_cnt",
                        "md.nonredundant",
                        "md.operation_speed",
                        "md.status",
                        "mdstat.mdstat_flush",
                    ]
                },
            ]
        },
        {
            title: "Storage Hardware",
            properties: itemProperties.keyCategoryExpanded,
            items: [
                storageSectionNVMe,
                {
                    title: "Smartd Log",
                    app: component("smartd"),
                    tooltip: "Metrics retrieved by parsing smartd log.",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "smartd_log.airflow_temperature_celsius",
                        "smartd_log.calibration_retries",
                        "smartd_log.current_pending_sector_count",
                        "smartd_log.erase_fail_count",
                        "smartd_log.media_wearout_indicator",
                        "smartd_log.nand_writes_1gib",
                        "smartd_log.offline_uncorrectable_sector_count",
                        "smartd_log.percent_lifetime_used",
                        "smartd_log.power_cycle_count",
                        "smartd_log.power_on_hours_count",
                        "smartd_log.program_fail_count",
                        "smartd_log.read_error_rate",
                        "smartd_log.read_total_err_corrected",
                        "smartd_log.read_total_unc_errors",
                        "smartd_log.reallocated_sectors_count",
                        "smartd_log.reallocation_event_count",
                        "smartd_log.reserved_block_count",
                        "smartd_log.sata_interface_downshift",
                        "smartd_log.seek_error_rate",
                        "smartd_log.seek_time_performance",
                        "smartd_log.soft_read_error_rate",
                        "smartd_log.spin_up_retries",
                        "smartd_log.spin_up_time",
                        "smartd_log.start_stop_count",
                        "smartd_log.temperature_celsius",
                        "smartd_log.throughput_performance",
                        "smartd_log.udma_crc_error_count",
                        "smartd_log.unexpected_power_loss",
                        "smartd_log.unused_reserved_nand_blocks",
                        "smartd_log.verify_total_err_corrected",
                        "smartd_log.verify_total_unc_errors",
                        "smartd_log.wear_leveller_worst_case_erase_count",
                        "smartd_log.write_error_rate",
                        "smartd_log.write_total_err_corrected",
                        "smartd_log.write_total_unc_errors",
                    ]
                },
                {
                    title: "HDDTemp",
                    app: component("hddtemp"),
                    tooltip: "Hard drive temperatures via hddtemp.",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "hddtemp.temperatures",
                    ]
                },
                {
                    title: "Adaptec RAID",
                    app: component("adaptec"),
                    tooltip: "Performance of your Adaptec RAID",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "adaptec_raid.ld_status",
                        "adaptec_raid.pd_state",
                        "adaptec_raid.smart_warnings",
                        "adaptec_raid.temperature",
                    ]
                },
                {
                    title: "HP SSA",
                    app: component("hpssa"),
                    tooltip: "HP Smart Storage Administrator.",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "hpssa.ctrl_status",
                        "hpssa.ctrl_temperature",
                        "hpssa.ld_status",
                        "hpssa.pd_status",
                        "hpssa.pd_temperature",
                    ]
                },
                {
                    title: "Adaptec MegaCLI",
                    app: component("megacli"),
                    tooltip: "Metric for Adaptec RAID, via the megacli command.",
                    properties: itemProperties.keyCategoryExpandable,
                    priorityOrder: true,
                    families: true,
                    contexts: [
                        "megacli.adapter_degraded",
                        "megacli.bbu_cycle_count",
                        "megacli.bbu_relative_charge",
                        "megacli.pd_media_error",
                        "megacli.pd_predictive_failure",
                    ]
                },
            ]
        }
    ]
}

let dynamicSections = {
    // match all the sections dynamically derived from the remaining contexts
}

let dashboards = {
    room: {
        parameters: [
            {
                name: "room",
                required: true,
            },
        ],
        header: {
            // somehow define the header (above the tabs)
        },
        firstLevelAsTabs: true,
        items: [
            systemSection,
            storageSection,
            dynamicSections,
        ],
    },
    node: {
        parameters: [
            {
                name: "node",
                required: true,
                scope: "node"
            },
        ],
        header: {
            // somehow define the header (above the tabs)
        },
        firstLevelAsTabs: true,
        items: [
            systemSection,
            storageSection,
            dynamicSections,
        ],
    },
    unixBlockDevice: {
        parameters: [
            {
                name: "node",
                required: true,
                scope: "node"
            },
            {
                name: "device",
                required: true,
                scope: "label:device"
            }
        ],
        header: {
            // somehow define the header (above the tabs)
        },
        firstLevelAsTabs: true,
        items: [
            storageSectionBlockDevices,
            storageSectionNVMe,
            {
                title: "Logs",
                // somehow bring logs widget with filter: _MACHINE_ID=${machine id} AND q=${device}
                // there may be multiple sources for logs (multiple agents) having logs for
                // this node. We may need to find which node has information about this node.
                // logs have a _MACHINE_ID to a GUID representing this node.
            }
        ]
    },
    unixMountPoint: {

    },
    unixNetworkInterface: {

    },
    unixProcessesCategory: {

    },
    unixProcessesPID: {

    },
    unixProcessesUID: {

    },
    unixProcessesGID: {

    },
    systemdService: {

    },
    cgroup: {

    },
    dynamic: {
        items: [
            dynamicSections,
        ],
    }
}
