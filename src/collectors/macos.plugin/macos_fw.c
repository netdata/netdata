// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_macos.h"

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

int do_macos_iokit(int update_every, usec_t dt) {
    (void)dt;

    static int do_io = -1, do_space = -1, do_inodes = -1, do_bandwidth = -1;

    if (unlikely(do_io == -1)) {
        do_io                   = inicfg_get_boolean(&netdata_config, "plugin:macos:iokit", "disk i/o", 1);
        do_space                = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "space usage for all disks", 1);
        do_inodes               = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "inodes usage for all disks", 1);
        do_bandwidth            = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "bandwidth", 1);
    }

    RRDSET *st;

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
    struct prev_diskstat {
        collected_number bytes_read;
        collected_number bytes_write;
        collected_number operations_read;
        collected_number operations_write;
        collected_number duration_read_ns;
        collected_number duration_write_ns;
        collected_number busy_time_ns;
    } prev_diskstat;

    // NEEDED BY: do_space, do_inodes
    struct statfs *mntbuf;
    int mntsize, i;
    char title[4096 + 1];

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
            bzero(&diskstat, sizeof(diskstat));

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
                                , diskstat.name
                                , "disk.io"
                                , "Disk I/O Bandwidth"
                                , "KiB/s"
                                , "macos.plugin"
                                , "iokit"
                                , 2000
                                , update_every
                                , RRDSET_TYPE_AREA
                        );

                        rrddim_add(st, "reads", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
                        rrddim_add(st, "writes", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
                    }

                    prev_diskstat.bytes_read = rrddim_set(st, "reads", diskstat.bytes_read);
                    prev_diskstat.bytes_write = rrddim_set(st, "writes", diskstat.bytes_write);
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
                                , diskstat.name
                                , "disk.ops"
                                , "Disk Completed I/O Operations"
                                , "operations/s"
                                , "macos.plugin"
                                , "iokit"
                                , 2001
                                , update_every
                                , RRDSET_TYPE_LINE
                        );

                        rrddim_add(st, "reads", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rrddim_add(st, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    }

                    prev_diskstat.operations_read = rrddim_set(st, "reads", diskstat.reads);
                    prev_diskstat.operations_write = rrddim_set(st, "writes", diskstat.writes);
                    rrdset_done(st);

                    /* Get reads time. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsTotalReadTimeKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.time_read);
                    }

                    /* Get writes time. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsTotalWriteTimeKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.time_write);
                    }

                    st = rrdset_find_active_bytype_localhost("disk_util", diskstat.name);
                    if (unlikely(!st)) {
                        st = rrdset_create_localhost(
                                "disk_util"
                                , diskstat.name
                                , NULL
                                , diskstat.name
                                , "disk.util"
                                , "Disk Utilization Time"
                                , "% of time working"
                                , "macos.plugin"
                                , "iokit"
                                , 2004
                                , update_every
                                , RRDSET_TYPE_AREA
                        );

                        rrddim_add(st, "utilization", NULL, 1, 10000000, RRD_ALGORITHM_INCREMENTAL);
                    }

                    cur_diskstat.busy_time_ns = (diskstat.time_read + diskstat.time_write);
                    prev_diskstat.busy_time_ns = rrddim_set(st, "utilization", cur_diskstat.busy_time_ns);
                    rrdset_done(st);

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
                                , diskstat.name
                                , "disk.iotime"
                                , "Disk Total I/O Time"
                                , "milliseconds/s"
                                , "macos.plugin"
                                , "iokit"
                                , 2022
                                , update_every
                                , RRDSET_TYPE_LINE
                        );

                        rrddim_add(st, "reads", NULL, 1, 1000000, RRD_ALGORITHM_INCREMENTAL);
                        rrddim_add(st, "writes", NULL, -1, 1000000, RRD_ALGORITHM_INCREMENTAL);
                    }

                    cur_diskstat.duration_read_ns = diskstat.time_read + diskstat.latency_read;
                    cur_diskstat.duration_write_ns = diskstat.time_write + diskstat.latency_write;
                    prev_diskstat.duration_read_ns = rrddim_set(st, "reads", cur_diskstat.duration_read_ns);
                    prev_diskstat.duration_write_ns = rrddim_set(st, "writes", cur_diskstat.duration_write_ns);
                    rrdset_done(st);

                    // calculate differential charts
                    // only if this is not the first time we run

                    if (likely(dt)) {
                        st = rrdset_find_active_bytype_localhost("disk_await", diskstat.name);
                        if (unlikely(!st)) {
                            st = rrdset_create_localhost(
                                    "disk_await"
                                    , diskstat.name
                                    , NULL
                                    , diskstat.name
                                    , "disk.await"
                                    , "Average Completed I/O Operation Time"
                                    , "milliseconds/operation"
                                    , "macos.plugin"
                                    , "iokit"
                                    , 2005
                                    , update_every
                                    , RRDSET_TYPE_LINE
                            );

                            rrddim_add(st, "reads", NULL, 1, 1000000, RRD_ALGORITHM_ABSOLUTE);
                            rrddim_add(st, "writes", NULL, -1, 1000000, RRD_ALGORITHM_ABSOLUTE);
                        }

                        rrddim_set(st, "reads", (diskstat.reads - prev_diskstat.operations_read) ?
                            (cur_diskstat.duration_read_ns - prev_diskstat.duration_read_ns) / (diskstat.reads - prev_diskstat.operations_read) : 0);
                        rrddim_set(st, "writes", (diskstat.writes - prev_diskstat.operations_write) ?
                            (cur_diskstat.duration_write_ns - prev_diskstat.duration_write_ns) / (diskstat.writes - prev_diskstat.operations_write) : 0);
                        rrdset_done(st);

                        st = rrdset_find_active_bytype_localhost("disk_avgsz", diskstat.name);
                        if (unlikely(!st)) {
                            st = rrdset_create_localhost(
                                    "disk_avgsz"
                                    , diskstat.name
                                    , NULL
                                    , diskstat.name
                                    , "disk.avgsz"
                                    , "Average Completed I/O Operation Bandwidth"
                                    , "KiB/operation"
                                    , "macos.plugin"
                                    , "iokit"
                                    , 2006
                                    , update_every
                                    , RRDSET_TYPE_AREA
                            );

                            rrddim_add(st, "reads", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
                            rrddim_add(st, "writes", NULL, -1, 1024, RRD_ALGORITHM_ABSOLUTE);
                        }

                        rrddim_set(st, "reads", (diskstat.reads - prev_diskstat.operations_read) ?
                            (diskstat.bytes_read - prev_diskstat.bytes_read) / (diskstat.reads - prev_diskstat.operations_read) : 0);
                        rrddim_set(st, "writes", (diskstat.writes - prev_diskstat.operations_write) ?
                            (diskstat.bytes_write - prev_diskstat.bytes_write) / (diskstat.writes - prev_diskstat.operations_write) : 0);
                        rrdset_done(st);

                        st = rrdset_find_active_bytype_localhost("disk_svctm", diskstat.name);
                        if (unlikely(!st)) {
                            st = rrdset_create_localhost(
                                    "disk_svctm"
                                    , diskstat.name
                                    , NULL
                                    , diskstat.name
                                    , "disk.svctm"
                                    , "Average Service Time"
                                    , "milliseconds/operation"
                                    , "macos.plugin"
                                    , "iokit"
                                    , 2007
                                    , update_every
                                    , RRDSET_TYPE_LINE
                            );

                            rrddim_add(st, "svctm", NULL, 1, 1000000, RRD_ALGORITHM_ABSOLUTE);
                        }

                        rrddim_set(st, "svctm", ((diskstat.reads - prev_diskstat.operations_read) + (diskstat.writes - prev_diskstat.operations_write)) ?
                            (cur_diskstat.busy_time_ns - prev_diskstat.busy_time_ns) / ((diskstat.reads - prev_diskstat.operations_read) + (diskstat.writes - prev_diskstat.operations_write)) : 0);
                        rrdset_done(st);
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

                // --------------------------------------------------------------------------

                if (likely(do_space)) {
                    st = rrdset_find_active_bytype_localhost("disk_space", mntbuf[i].f_mntonname);
                    if (unlikely(!st)) {
                        snprintfz(title, sizeof(title) - 1, "Disk Space Usage for %s [%s]", mntbuf[i].f_mntonname, mntbuf[i].f_mntfromname);
                        st = rrdset_create_localhost(
                                "disk_space"
                                , mntbuf[i].f_mntonname
                                , NULL
                                , mntbuf[i].f_mntonname
                                , "disk.space"
                                , title
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
                        snprintfz(title, sizeof(title) - 1, "Disk Files (inodes) Usage for %s [%s]", mntbuf[i].f_mntonname, mntbuf[i].f_mntfromname);
                        st = rrdset_create_localhost(
                                "disk_inodes"
                                , mntbuf[i].f_mntonname
                                , NULL
                                , mntbuf[i].f_mntonname
                                , "disk.inodes"
                                , title
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

                st = rrdset_find_active_bytype_localhost("net", ifa->ifa_name);
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "net"
                            , ifa->ifa_name
                            , NULL
                            , ifa->ifa_name
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
                            , ifa->ifa_name
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
                            , ifa->ifa_name
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
                            , ifa->ifa_name
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
                }

                rrddim_set(st, "inbound", IFA_DATA(iqdrops));
                rrdset_done(st);

                st = rrdset_find_active_bytype_localhost("net_events", ifa->ifa_name);
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "net_events"
                            , ifa->ifa_name
                            , NULL
                            , ifa->ifa_name
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
                }

                rrddim_set(st, "collisions", IFA_DATA(collisions));
                rrdset_done(st);
            }

            freeifaddrs(ifap);
        }
    }

    return 0;
}
