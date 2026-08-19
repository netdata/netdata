// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_macos.h"

#define _COMMON_PLUGIN_NAME "macos.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "iokit"
#include "../common-contexts/common-contexts.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <IOKit/storage/IOBlockStorageDriver.h>
#include <IOKit/IOBSD.h>
// NEEDED BY do_space, do_inodes
#include <sys/mount.h>
// NEEDED BY: struct ifaddrs, getifaddrs()
#include <net/if.h>
#include <ifaddrs.h>

// NEEDED BY: do_bandwidth
#define IFA_DATA(s) (((struct if_data *)ifa->ifa_data)->ifi_ ## s)

#define MAXDRIVENAME 31

#define KILO_FACTOR 1024
#define MEGA_FACTOR 1048576     // 1024 * 1024
#define GIGA_FACTOR 1073741824  // 1024 * 1024 * 1024
#define DISK_UTILIZATION_MAX_PERCENT 100
#define IOKIT_DISK_STATE_PRUNE_AFTER_UT (10ULL * 60ULL * USEC_PER_SEC)

struct iokit_disk_state {
    ND_DISK_UTIL disk_util;
    RRDSET *st_io;
    RRDSET *st_ops;
    RRDSET *st_iotime;
    RRDSET *st_await;
    RRDSET *st_avgsz;
    RRDSET *st_svctm;
    collected_number bytes_read;
    collected_number bytes_write;
    collected_number operations_read;
    collected_number operations_write;
    collected_number duration_read_ns;
    collected_number duration_write_ns;
    collected_number busy_time_ns;
    usec_t last_busy_time_ut;
    usec_t last_seen_ut;
    bool previous_sample_seen;
};

static DICTIONARY *iokit_disk_states = NULL;

static void macos_iokit_disk_state_delete_cb(
    const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct iokit_disk_state *d = value;

    rrdset_is_obsolete___safe_from_collector_thread(d->st_io);
    rrdset_is_obsolete___safe_from_collector_thread(d->st_ops);
    rrdset_is_obsolete___safe_from_collector_thread(d->disk_util.st_util);
    rrdset_is_obsolete___safe_from_collector_thread(d->st_iotime);
    rrdset_is_obsolete___safe_from_collector_thread(d->st_await);
    rrdset_is_obsolete___safe_from_collector_thread(d->st_avgsz);
    rrdset_is_obsolete___safe_from_collector_thread(d->st_svctm);
}

static DICTIONARY *macos_iokit_disk_states_create(void)
{
    if (likely(iokit_disk_states))
        return iokit_disk_states;

    iokit_disk_states = dictionary_create_advanced(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
        NULL,
        sizeof(struct iokit_disk_state));
    if (likely(iokit_disk_states))
        dictionary_register_delete_callback(iokit_disk_states, macos_iokit_disk_state_delete_cb, NULL);

    return iokit_disk_states;
}

static void macos_iokit_disk_util_labels_cb(RRDSET *st, void *data)
{
    rrdlabels_add(st->rrdlabels, "device", data, RRDLABEL_SRC_AUTO);
}

static void macos_iokit_prune_disk_states(usec_t now_ut)
{
    if (unlikely(!iokit_disk_states))
        return;

    struct iokit_disk_state *d;
    dfe_start_write(iokit_disk_states, d) {
        if (d->last_seen_ut && now_ut > d->last_seen_ut &&
            now_ut - d->last_seen_ut > IOKIT_DISK_STATE_PRUNE_AFTER_UT)
            dictionary_del(iokit_disk_states, d_dfe.name);
    }
    dfe_done(d);

    dictionary_garbage_collect(iokit_disk_states);
}

void macos_iokit_cleanup(void)
{
    dictionary_destroy(iokit_disk_states);
    iokit_disk_states = NULL;
}

int do_macos_iokit(int update_every, usec_t dt) {
    static SIMPLE_PATTERN *excluded_mountpoints = NULL;
    static SIMPLE_PATTERN *disabled_net_interfaces = NULL;

    static int do_io = -1, do_space = -1, do_inodes = -1, do_bandwidth = -1;

    if (unlikely(do_io == -1)) {
        do_io                   = inicfg_get_boolean(&netdata_config, "plugin:macos:iokit", "disk i/o", 1);
        do_space                = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "space usage for all disks", 1);
        do_inodes               = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "inodes usage for all disks", 1);
        do_bandwidth            = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "bandwidth", 1);

        excluded_mountpoints = simple_pattern_create(
            inicfg_get(
                &netdata_config,
                "plugin:macos:iokit",
                "exclude mountpoints by path",
                "/System/Volumes/* /private/var/folders/* /Volumes/Recovery"),
            NULL,
            SIMPLE_PATTERN_EXACT,
            true);
        disabled_net_interfaces = simple_pattern_create(
            inicfg_get(
                &netdata_config,
                "plugin:macos:iokit",
                "disable by default network interfaces matching",
                "lo* awdl* llw* anpi* gif* bridge* ap*"),
            NULL,
            SIMPLE_PATTERN_EXACT,
            true);
    }

    RRDSET *st;
    usec_t now_ut = now_monotonic_usec();

    mach_port_t         main_port;
    io_registry_entry_t drive, drive_media;
    io_iterator_t       drive_list;
    CFDictionaryRef     properties, statistics;
    CFStringRef         name;
    CFNumberRef         number;
    kern_return_t       status;
    collected_number    total_disk_reads = 0;
    collected_number    total_disk_writes = 0;
    struct diskstat {
        char name[MAXDRIVENAME];
        collected_number bytes_read;
        collected_number bytes_write;
        collected_number reads;
        collected_number writes;
        collected_number time_read;
        collected_number time_write;
        collected_number latency_read;
        collected_number latency_write;
    } diskstat;
    struct cur_diskstat {
        collected_number duration_read_ns;
        collected_number duration_write_ns;
        collected_number busy_time_ns;
    } cur_diskstat;
    // NEEDED BY: do_space, do_inodes
    struct statfs *mntbuf;
    int mntsize, i;

    // NEEDED BY: do_bandwidth
    struct ifaddrs *ifa, *ifap;

#if !defined(MAC_OS_VERSION_12_0) || (MAC_OS_X_VERSION_MIN_REQUIRED < MAC_OS_VERSION_12_0)
#define IOMainPort IOMasterPort
#endif

    /* Get ports and services for drive statistics. */
    if (unlikely(IOMainPort(bootstrap_port, &main_port))) {
        collector_error("MACOS: IOMasterPort() failed");
        do_io = 0;
        collector_error("DISABLED: system.io");
    /* Get the list of all drive objects. */
    } else if (unlikely(IOServiceGetMatchingServices(main_port, IOServiceMatching("IOBlockStorageDriver"), &drive_list))) {
        collector_error("MACOS: IOServiceGetMatchingServices() failed");
        do_io = 0;
        collector_error("DISABLED: system.io");
    } else {
        while ((drive = IOIteratorNext(drive_list)) != 0) {
            properties = 0;
            statistics = 0;
            number = 0;
            memset(&diskstat, 0, sizeof(diskstat));

            /* Get drive media object. */
            status = IORegistryEntryGetChildEntry(drive, kIOServicePlane, &drive_media);
            if (unlikely(status != KERN_SUCCESS)) {
                IOObjectRelease(drive);
                continue;
            }

            /* Get drive media properties. */
            if (likely(!IORegistryEntryCreateCFProperties(drive_media, (CFMutableDictionaryRef *)&properties, kCFAllocatorDefault, 0))) {
                /* Get disk name. */
                if (likely(name = (CFStringRef)CFDictionaryGetValue(properties, CFSTR(kIOBSDNameKey)))) {
                    CFStringGetCString(name, diskstat.name, MAXDRIVENAME, kCFStringEncodingUTF8);
                }
            }

            /* Release. */
            CFRelease(properties);
            IOObjectRelease(drive_media);

            if(unlikely(!*diskstat.name)) {
                IOObjectRelease(drive);
                continue;
            }

            /* Obtain the properties for this drive object. */
            if (unlikely(IORegistryEntryCreateCFProperties(drive, (CFMutableDictionaryRef *)&properties, kCFAllocatorDefault, 0))) {
                IOObjectRelease(drive);
                collector_error("MACOS: IORegistryEntryCreateCFProperties() failed");
                do_io = 0;
                collector_error("DISABLED: system.io");
                break;
            } else if (likely(properties)) {
                /* Obtain the statistics from the drive properties. */
                if (likely(statistics = (CFDictionaryRef)CFDictionaryGetValue(properties, CFSTR(kIOBlockStorageDriverStatisticsKey)))) {

                    // --------------------------------------------------------------------

                    DICTIONARY *disk_states = macos_iokit_disk_states_create();
                    struct iokit_disk_state *disk_state =
                        disk_states ? dictionary_set(disk_states, diskstat.name, NULL, sizeof(*disk_state)) : NULL;
                    bool have_previous_disk_sample = disk_state && disk_state->previous_sample_seen;

                    /* Get bytes read. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsBytesReadKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.bytes_read);
                        total_disk_reads += diskstat.bytes_read;
                    }

                    /* Get bytes written. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsBytesWrittenKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.bytes_write);
                        total_disk_writes += diskstat.bytes_write;
                    }

                    st = rrdset_find_active_bytype_localhost("disk", diskstat.name);
                    if (unlikely(!st)) {
                        st = rrdset_create_localhost(
                                "disk"
                                , diskstat.name
                                , NULL
                                , "io"
                                , "disk.io"
                                , "Disk I/O Bandwidth"
                                , "KiB/s"
                                , "macos.plugin"
                                , "iokit"
                                , NETDATA_CHART_PRIO_DISK_IO
                                , update_every
                                , RRDSET_TYPE_AREA
                        );

                        rrddim_add(st, "reads", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
                        rrddim_add(st, "writes", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
                        rrdlabels_add(st->rrdlabels, "device", diskstat.name, RRDLABEL_SRC_AUTO);
                    }

                    if (likely(disk_state))
                        disk_state->st_io = st;

                    rrddim_set(st, "reads", diskstat.bytes_read);
                    rrddim_set(st, "writes", diskstat.bytes_write);
                    rrdset_done(st);

                    /* Get number of reads. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsReadsKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.reads);
                    }

                    /* Get number of writes. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsWritesKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.writes);
                    }

                    st = rrdset_find_active_bytype_localhost("disk_ops", diskstat.name);
                    if (unlikely(!st)) {
                        st = rrdset_create_localhost(
                                "disk_ops"
                                , diskstat.name
                                , NULL
                                , "ops"
                                , "disk.ops"
                                , "Disk Completed I/O Operations"
                                , "operations/s"
                                , "macos.plugin"
                                , "iokit"
                                , NETDATA_CHART_PRIO_DISK_OPS
                                , update_every
                                , RRDSET_TYPE_LINE
                        );

                        rrddim_add(st, "reads", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rrddim_add(st, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rrdlabels_add(st->rrdlabels, "device", diskstat.name, RRDLABEL_SRC_AUTO);
                    }

                    if (likely(disk_state))
                        disk_state->st_ops = st;

                    rrddim_set(st, "reads", diskstat.reads);
                    rrddim_set(st, "writes", diskstat.writes);
                    rrdset_done(st);

                    /* Get reads time. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsTotalReadTimeKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.time_read);
                    }

                    /* Get writes time. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsTotalWriteTimeKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.time_write);
                    }

                    cur_diskstat.busy_time_ns = (diskstat.time_read + diskstat.time_write);
                    collected_number disk_ops_delta = 0;
                    if (likely(
                            have_previous_disk_sample && diskstat.reads >= disk_state->operations_read &&
                            diskstat.writes >= disk_state->operations_write))
                        disk_ops_delta =
                            (diskstat.reads - disk_state->operations_read) +
                            (diskstat.writes - disk_state->operations_write);

                    bool have_busy_time_delta = false;
                    collected_number busy_time_delta_ns = 0;

                    if (likely(disk_state)) {
                        collected_number previous_busy_time_ns = disk_state->busy_time_ns;
                        bool busy_time_monotonic = cur_diskstat.busy_time_ns >= previous_busy_time_ns;
                        usec_t elapsed_ut = disk_state->last_busy_time_ut && now_ut > disk_state->last_busy_time_ut ?
                                                now_ut - disk_state->last_busy_time_ut :
                                                0;

                        if (likely(have_previous_disk_sample && busy_time_monotonic)) {
                            busy_time_delta_ns = cur_diskstat.busy_time_ns - previous_busy_time_ns;
                            have_busy_time_delta = true;
                        }

                        uint64_t disk_utilization = 0;
                        if (likely(have_busy_time_delta && elapsed_ut)) {
                            nsec_t elapsed_ns = elapsed_ut * NSEC_PER_USEC;
                            NETDATA_DOUBLE utilization =
                                (NETDATA_DOUBLE)busy_time_delta_ns * DISK_UTILIZATION_MAX_PERCENT /
                                (NETDATA_DOUBLE)elapsed_ns;

                            if (utilization > DISK_UTILIZATION_MAX_PERCENT)
                                utilization = DISK_UTILIZATION_MAX_PERCENT;

                            disk_utilization = (uint64_t)utilization;
                        }

                        common_disk_util(
                            &disk_state->disk_util,
                            diskstat.name,
                            NULL,
                            disk_utilization,
                            update_every,
                            macos_iokit_disk_util_labels_cb,
                            diskstat.name);
                    }

                    /* Get reads latency. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsLatentReadTimeKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.latency_read);
                    }

                    /* Get writes latency. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsLatentWriteTimeKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.latency_write);
                    }

                    st = rrdset_find_active_bytype_localhost("disk_iotime", diskstat.name);
                    if (unlikely(!st)) {
                        st = rrdset_create_localhost(
                                "disk_iotime"
                                , diskstat.name
                                , NULL
                                , "utilization"
                                , "disk.iotime"
                                , "Disk Total I/O Time"
                                , "milliseconds/s"
                                , "macos.plugin"
                                , "iokit"
                                , NETDATA_CHART_PRIO_DISK_IOTIME
                                , update_every
                                , RRDSET_TYPE_LINE
                        );

                        rrddim_add(st, "reads", NULL, 1, 1000000, RRD_ALGORITHM_INCREMENTAL);
                        rrddim_add(st, "writes", NULL, -1, 1000000, RRD_ALGORITHM_INCREMENTAL);
                        rrdlabels_add(st->rrdlabels, "device", diskstat.name, RRDLABEL_SRC_AUTO);
                    }

                    if (likely(disk_state))
                        disk_state->st_iotime = st;

                    cur_diskstat.duration_read_ns = diskstat.time_read + diskstat.latency_read;
                    cur_diskstat.duration_write_ns = diskstat.time_write + diskstat.latency_write;
                    rrddim_set(st, "reads", cur_diskstat.duration_read_ns);
                    rrddim_set(st, "writes", cur_diskstat.duration_write_ns);
                    rrdset_done(st);

                    // calculate differential charts
                    // only if this is not the first time we run

                    if (likely(dt)) {
                        collected_number reads_delta = 0;
                        collected_number writes_delta = 0;
                        collected_number duration_read_delta_ns = 0;
                        collected_number duration_write_delta_ns = 0;
                        collected_number bytes_read_delta = 0;
                        collected_number bytes_write_delta = 0;

                        if (likely(have_previous_disk_sample)) {
                            if (diskstat.reads > disk_state->operations_read) {
                                reads_delta = diskstat.reads - disk_state->operations_read;
                                if (cur_diskstat.duration_read_ns >= disk_state->duration_read_ns)
                                    duration_read_delta_ns =
                                        cur_diskstat.duration_read_ns - disk_state->duration_read_ns;
                                if (diskstat.bytes_read >= disk_state->bytes_read)
                                    bytes_read_delta = diskstat.bytes_read - disk_state->bytes_read;
                            }

                            if (diskstat.writes > disk_state->operations_write) {
                                writes_delta = diskstat.writes - disk_state->operations_write;
                                if (cur_diskstat.duration_write_ns >= disk_state->duration_write_ns)
                                    duration_write_delta_ns =
                                        cur_diskstat.duration_write_ns - disk_state->duration_write_ns;
                                if (diskstat.bytes_write >= disk_state->bytes_write)
                                    bytes_write_delta = diskstat.bytes_write - disk_state->bytes_write;
                            }
                        }

                        st = rrdset_find_active_bytype_localhost("disk_await", diskstat.name);
                        if (unlikely(!st)) {
                            st = rrdset_create_localhost(
                                    "disk_await"
                                    , diskstat.name
                                    , NULL
                                    , "latency"
                                    , "disk.await"
                                    , "Average Completed I/O Operation Time"
                                    , "milliseconds/operation"
                                    , "macos.plugin"
                                    , "iokit"
                                    , NETDATA_CHART_PRIO_DISK_AWAIT
                                    , update_every
                                    , RRDSET_TYPE_LINE
                            );

                            rrddim_add(st, "reads", NULL, 1, 1000000, RRD_ALGORITHM_ABSOLUTE);
                            rrddim_add(st, "writes", NULL, -1, 1000000, RRD_ALGORITHM_ABSOLUTE);
                            rrdlabels_add(st->rrdlabels, "device", diskstat.name, RRDLABEL_SRC_AUTO);
                        }

                        if (likely(disk_state))
                            disk_state->st_await = st;

                        rrddim_set(st, "reads", reads_delta > 0 ? duration_read_delta_ns / reads_delta : 0);
                        rrddim_set(st, "writes", writes_delta > 0 ? duration_write_delta_ns / writes_delta : 0);
                        rrdset_done(st);

                        st = rrdset_find_active_bytype_localhost("disk_avgsz", diskstat.name);
                        if (unlikely(!st)) {
                            st = rrdset_create_localhost(
                                    "disk_avgsz"
                                    , diskstat.name
                                    , NULL
                                    , "io"
                                    , "disk.avgsz"
                                    , "Average Completed I/O Operation Bandwidth"
                                    , "KiB/operation"
                                    , "macos.plugin"
                                    , "iokit"
                                    , NETDATA_CHART_PRIO_DISK_AVGSZ
                                    , update_every
                                    , RRDSET_TYPE_AREA
                            );

                            rrddim_add(st, "reads", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
                            rrddim_add(st, "writes", NULL, -1, 1024, RRD_ALGORITHM_ABSOLUTE);
                            rrdlabels_add(st->rrdlabels, "device", diskstat.name, RRDLABEL_SRC_AUTO);
                        }

                        if (likely(disk_state))
                            disk_state->st_avgsz = st;

                        rrddim_set(st, "reads", reads_delta > 0 ? bytes_read_delta / reads_delta : 0);
                        rrddim_set(st, "writes", writes_delta > 0 ? bytes_write_delta / writes_delta : 0);
                        rrdset_done(st);

                        st = rrdset_find_active_bytype_localhost("disk_svctm", diskstat.name);
                        if (unlikely(!st)) {
                            st = rrdset_create_localhost(
                                    "disk_svctm"
                                    , diskstat.name
                                    , NULL
                                    , "latency"
                                    , "disk.svctm"
                                    , "Average Service Time"
                                    , "milliseconds/operation"
                                    , "macos.plugin"
                                    , "iokit"
                                    , NETDATA_CHART_PRIO_DISK_SVCTM
                                    , update_every
                                    , RRDSET_TYPE_LINE
                            );

                            rrddim_add(st, "svctm", NULL, 1, 1000000, RRD_ALGORITHM_ABSOLUTE);
                            rrdlabels_add(st->rrdlabels, "device", diskstat.name, RRDLABEL_SRC_AUTO);
                        }

                        if (likely(disk_state))
                            disk_state->st_svctm = st;

                        rrddim_set(
                            st, "svctm",
                            have_busy_time_delta && disk_ops_delta > 0 ? busy_time_delta_ns / disk_ops_delta : 0);
                        rrdset_done(st);
                    }

                    if (likely(disk_state)) {
                        disk_state->bytes_read = diskstat.bytes_read;
                        disk_state->bytes_write = diskstat.bytes_write;
                        disk_state->operations_read = diskstat.reads;
                        disk_state->operations_write = diskstat.writes;
                        disk_state->duration_read_ns = cur_diskstat.duration_read_ns;
                        disk_state->duration_write_ns = cur_diskstat.duration_write_ns;
                        disk_state->busy_time_ns = cur_diskstat.busy_time_ns;
                        disk_state->last_busy_time_ut = now_ut;
                        disk_state->last_seen_ut = now_ut;
                        disk_state->previous_sample_seen = true;
                    }
                }

                /* Release. */
                CFRelease(properties);
            }

            /* Release. */
            IOObjectRelease(drive);
        }
        IOIteratorReset(drive_list);

        /* Release. */
        IOObjectRelease(drive_list);

        macos_iokit_prune_disk_states(now_ut);
    }

    if (likely(do_io)) {
        st = rrdset_find_active_bytype_localhost("system", "io");
        if (unlikely(!st)) {
            st = rrdset_create_localhost(
                    "system"
                    , "io"
                    , NULL
                    , "disk"
                    , NULL
                    , "Disk I/O"
                    , "KiB/s"
                    , "macos.plugin"
                    , "iokit"
                    , 150
                    , update_every
                    , RRDSET_TYPE_AREA
            );
            rrddim_add(st, "in",  NULL,  1, 1024, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "out", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set(st, "in", total_disk_reads);
        rrddim_set(st, "out", total_disk_writes);
        rrdset_done(st);
    }

    // Can be merged with FreeBSD plugin

    if (likely(do_space || do_inodes)) {
        // there is no mount info in sysctl MIBs
        if (unlikely(!(mntsize = getmntinfo(&mntbuf, MNT_NOWAIT)))) {
            collector_error("MACOS: getmntinfo() failed");
            do_space = 0;
            collector_error("DISABLED: disk_space.X");
            do_inodes = 0;
            collector_error("DISABLED: disk_inodes.X");
        } else {
            for (i = 0; i < mntsize; i++) {
                if (mntbuf[i].f_flags == MNT_RDONLY ||
                        mntbuf[i].f_blocks == 0 ||
                        // taken from gnulib/mountlist.c and shortened to FreeBSD related fstypes
                        strcmp(mntbuf[i].f_fstypename, "autofs") == 0 ||
                        strcmp(mntbuf[i].f_fstypename, "procfs") == 0 ||
                        strcmp(mntbuf[i].f_fstypename, "subfs") == 0 ||
                        strcmp(mntbuf[i].f_fstypename, "devfs") == 0 ||
                        strcmp(mntbuf[i].f_fstypename, "none") == 0)
                    continue;

                if (simple_pattern_matches(excluded_mountpoints, mntbuf[i].f_mntonname)) {
                    continue;
                }

                // --------------------------------------------------------------------------

                if (likely(do_space)) {
                    st = rrdset_find_active_bytype_localhost("disk_space", mntbuf[i].f_mntonname);
                    if (unlikely(!st)) {
                        st = rrdset_create_localhost(
                                "disk_space"
                                , mntbuf[i].f_mntonname
                                , NULL
                                , "used space"
                                , "disk.space"
                                , "Disk Space Usage"
                                , "GiB"
                                , "macos.plugin"
                                , "iokit"
                                , 2023
                                , update_every
                                , RRDSET_TYPE_STACKED
                        );

                        rrddim_add(st, "avail", NULL, mntbuf[i].f_bsize, GIGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
                        rrddim_add(st, "used", NULL, mntbuf[i].f_bsize, GIGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
                        rrddim_add(st, "reserved_for_root", "reserved for root", mntbuf[i].f_bsize, GIGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
                        rrdlabels_add(st->rrdlabels, "mount_point", mntbuf[i].f_mntonname, RRDLABEL_SRC_AUTO);
                        rrdlabels_add(st->rrdlabels, "filesystem", mntbuf[i].f_fstypename, RRDLABEL_SRC_AUTO);
                    }

                    rrddim_set(st, "avail", (collected_number) mntbuf[i].f_bavail);
                    rrddim_set(st, "used", (collected_number) (mntbuf[i].f_blocks - mntbuf[i].f_bfree));
                    rrddim_set(st, "reserved_for_root", (collected_number) (mntbuf[i].f_bfree - mntbuf[i].f_bavail));
                    rrdset_done(st);
                }

                // --------------------------------------------------------------------------

                if (likely(do_inodes)) {
                    st = rrdset_find_active_bytype_localhost("disk_inodes", mntbuf[i].f_mntonname);
                    if (unlikely(!st)) {
                        st = rrdset_create_localhost(
                                "disk_inodes"
                                , mntbuf[i].f_mntonname
                                , NULL
                                , "used inodes"
                                , "disk.inodes"
                                , "Disk Files (inodes) Usage"
                                , "inodes"
                                , "macos.plugin"
                                , "iokit"
                                , 2024
                                , update_every
                                , RRDSET_TYPE_STACKED
                        );

                        rrddim_add(st, "avail", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        rrddim_add(st, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        rrddim_add(st, "reserved_for_root", "reserved for root", 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        rrdlabels_add(st->rrdlabels, "mount_point", mntbuf[i].f_mntonname, RRDLABEL_SRC_AUTO);
                        rrdlabels_add(st->rrdlabels, "filesystem", mntbuf[i].f_fstypename, RRDLABEL_SRC_AUTO);
                    }

                    rrddim_set(st, "avail", (collected_number) mntbuf[i].f_ffree);
                    rrddim_set(st, "used", (collected_number) (mntbuf[i].f_files - mntbuf[i].f_ffree));
                    rrdset_done(st);
                }
            }
        }
    }

    // Can be merged with FreeBSD plugin

    if (likely(do_bandwidth)) {
        if (unlikely(getifaddrs(&ifap))) {
            collector_error("MACOS: getifaddrs()");
            do_bandwidth = 0;
            collector_error("DISABLED: system.ipv4");
        } else {
            for (ifa = ifap; ifa; ifa = ifa->ifa_next) {
                if (ifa->ifa_addr->sa_family != AF_LINK)
                        continue;

                if (simple_pattern_matches(disabled_net_interfaces, ifa->ifa_name)) {
                    continue;
                }

                st = rrdset_find_active_bytype_localhost("net", ifa->ifa_name);
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "net"
                            , ifa->ifa_name
                            , NULL
                            , "traffic"
                            , "net.net"
                            , "Bandwidth"
                            , "kilobits/s"
                            , "macos.plugin"
                            , "iokit"
                            , 7000
                            , update_every
                            , RRDSET_TYPE_AREA
                    );

                    rrddim_add(st, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rrdlabels_add(st->rrdlabels, "device", ifa->ifa_name, RRDLABEL_SRC_AUTO);
                }

                rrddim_set(st, "received", IFA_DATA(ibytes));
                rrddim_set(st, "sent", IFA_DATA(obytes));
                rrdset_done(st);

                st = rrdset_find_active_bytype_localhost("net_packets", ifa->ifa_name);
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "net_packets"
                            , ifa->ifa_name
                            , NULL
                            , "packets"
                            , "net.packets"
                            , "Packets"
                            , "packets/s"
                            , "macos.plugin"
                            , "iokit"
                            , 7001
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "multicast_received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "multicast_sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrdlabels_add(st->rrdlabels, "device", ifa->ifa_name, RRDLABEL_SRC_AUTO);
                }

                rrddim_set(st, "received", IFA_DATA(ipackets));
                rrddim_set(st, "sent", IFA_DATA(opackets));
                rrddim_set(st, "multicast_received", IFA_DATA(imcasts));
                rrddim_set(st, "multicast_sent", IFA_DATA(omcasts));
                rrdset_done(st);

                st = rrdset_find_active_bytype_localhost("net_errors", ifa->ifa_name);
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "net_errors"
                            , ifa->ifa_name
                            , NULL
                            , "errors"
                            , "net.errors"
                            , "Interface Errors"
                            , "errors/s"
                            , "macos.plugin"
                            , "iokit"
                            , 7002
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "inbound", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrdlabels_add(st->rrdlabels, "device", ifa->ifa_name, RRDLABEL_SRC_AUTO);
                }

                rrddim_set(st, "inbound", IFA_DATA(ierrors));
                rrddim_set(st, "outbound", IFA_DATA(oerrors));
                rrdset_done(st);

                st = rrdset_find_active_bytype_localhost("net_drops", ifa->ifa_name);
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "net_drops"
                            , ifa->ifa_name
                            , NULL
                            , "drops"
                            , "net.drops"
                            , "Interface Drops"
                            , "drops/s"
                            , "macos.plugin"
                            , "iokit"
                            , 7003
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "inbound", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrdlabels_add(st->rrdlabels, "device", ifa->ifa_name, RRDLABEL_SRC_AUTO);
                }

                rrddim_set(st, "inbound", IFA_DATA(iqdrops));
                rrdset_done(st);

                st = rrdset_find_active_bytype_localhost("net_events", ifa->ifa_name);
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "net_events"
                            , ifa->ifa_name
                            , NULL
                            , "errors"
                            , "net.events"
                            , "Network Interface Events"
                            , "events/s"
                            , "macos.plugin"
                            , "iokit"
                            , 7006
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "frames", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "collisions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "carrier", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrdlabels_add(st->rrdlabels, "device", ifa->ifa_name, RRDLABEL_SRC_AUTO);
                }

                rrddim_set(st, "collisions", IFA_DATA(collisions));
                rrdset_done(st);
            }

            freeifaddrs(ifap);
        }
    }

    return 0;
}
