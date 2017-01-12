#include "common.h"
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <IOKit/storage/IOBlockStorageDriver.h>
#include <IOKit/IOBSD.h>

#define MAXDRIVENAME 31

int do_macos_iokit(int update_every, usec_t dt) {
    (void)dt;

    static int do_io = -1;

    if (unlikely(do_io == -1)) {
        do_io                  = config_get_boolean("plugin:macos:iokit", "disk i/o", 1);
    }

    RRDSET *st;

    mach_port_t         master_port;
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

    /* Get ports and services for drive statistics. */
    if (unlikely(IOMasterPort(bootstrap_port, &master_port))) {
        error("MACOS: IOMasterPort() failed");
        do_io = 0;
        error("DISABLED: system.io");
    /* Get the list of all drive objects. */
    } else if (unlikely(IOServiceGetMatchingServices(master_port, IOServiceMatching("IOBlockStorageDriver"), &drive_list))) {
        error("MACOS: IOServiceGetMatchingServices() failed");
        do_io = 0;
        error("DISABLED: system.io");
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

            /* Obtain the properties for this drive object. */
            if (unlikely(IORegistryEntryCreateCFProperties(drive, (CFMutableDictionaryRef *)&properties, kCFAllocatorDefault, 0))) {
                error("MACOS: IORegistryEntryCreateCFProperties() failed");
                do_io = 0;
                error("DISABLED: system.io");
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

                    st = rrdset_find_bytype("disk", diskstat.name);
                    if (unlikely(!st)) {
                        st = rrdset_create("disk", diskstat.name, NULL, diskstat.name, "disk.io", "Disk I/O Bandwidth", "kilobytes/s", 2000, update_every, RRDSET_TYPE_AREA);

                        rrddim_add(st, "reads", NULL, 1, 1024, RRDDIM_INCREMENTAL);
                        rrddim_add(st, "writes", NULL, -1, 1024, RRDDIM_INCREMENTAL);
                    }
                    else rrdset_next(st);

                    prev_diskstat.bytes_read = rrddim_set(st, "reads", diskstat.bytes_read);
                    prev_diskstat.bytes_write = rrddim_set(st, "writes", diskstat.bytes_write);
                    rrdset_done(st);

                    // --------------------------------------------------------------------

                    /* Get number of reads. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsReadsKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.reads);
                    }

                    /* Get number of writes. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsWritesKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.writes);
                    }

                    st = rrdset_find_bytype("disk_ops", diskstat.name);
                    if (unlikely(!st)) {
                        st = rrdset_create("disk_ops", diskstat.name, NULL, diskstat.name, "disk.ops", "Disk Completed I/O Operations", "operations/s", 2001, update_every, RRDSET_TYPE_LINE);
                        st->isdetail = 1;

                        rrddim_add(st, "reads", NULL, 1, 1, RRDDIM_INCREMENTAL);
                        rrddim_add(st, "writes", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    }
                    else rrdset_next(st);

                    prev_diskstat.operations_read = rrddim_set(st, "reads", diskstat.reads);
                    prev_diskstat.operations_write = rrddim_set(st, "writes", diskstat.writes);
                    rrdset_done(st);

                    // --------------------------------------------------------------------

                    /* Get reads time. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsTotalReadTimeKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.time_read);
                    }

                    /* Get writes time. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsTotalWriteTimeKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.time_write);
                    }

                    st = rrdset_find_bytype("disk_util", diskstat.name);
                    if (unlikely(!st)) {
                        st = rrdset_create("disk_util", diskstat.name, NULL, diskstat.name, "disk.util", "Disk Utilization Time", "% of time working", 2004, update_every, RRDSET_TYPE_AREA);
                        st->isdetail = 1;

                        rrddim_add(st, "utilization", NULL, 1, 10000000, RRDDIM_INCREMENTAL);
                    }
                    else rrdset_next(st);

                    cur_diskstat.busy_time_ns = (diskstat.time_read + diskstat.time_write);
                    prev_diskstat.busy_time_ns = rrddim_set(st, "utilization", cur_diskstat.busy_time_ns);
                    rrdset_done(st);

                    // --------------------------------------------------------------------

                    /* Get reads latency. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsLatentReadTimeKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.latency_read);
                    }

                    /* Get writes latency. */
                    if (likely(number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsLatentWriteTimeKey)))) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &diskstat.latency_write);
                    }

                    st = rrdset_find_bytype("disk_iotime", diskstat.name);
                    if (unlikely(!st)) {
                        st = rrdset_create("disk_iotime", diskstat.name, NULL, diskstat.name, "disk.iotime", "Disk Total I/O Time", "milliseconds/s", 2022, update_every, RRDSET_TYPE_LINE);
                        st->isdetail = 1;

                        rrddim_add(st, "reads", NULL, 1, 1000000, RRDDIM_INCREMENTAL);
                        rrddim_add(st, "writes", NULL, -1, 1000000, RRDDIM_INCREMENTAL);
                    }
                    else rrdset_next(st);

                    cur_diskstat.duration_read_ns = diskstat.time_read + diskstat.latency_read;
                    cur_diskstat.duration_write_ns = diskstat.time_write + diskstat.latency_write;
                    prev_diskstat.duration_read_ns = rrddim_set(st, "reads", cur_diskstat.duration_read_ns);
                    prev_diskstat.duration_write_ns = rrddim_set(st, "writes", cur_diskstat.duration_write_ns);
                    rrdset_done(st);

                    // --------------------------------------------------------------------
                    // calculate differential charts
                    // only if this is not the first time we run

                    if (likely(dt)) {

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype("disk_await", diskstat.name);
                        if (unlikely(!st)) {
                            st = rrdset_create("disk_await", diskstat.name, NULL, diskstat.name, "disk.await", "Average Completed I/O Operation Time", "ms per operation", 2005, update_every, RRDSET_TYPE_LINE);
                            st->isdetail = 1;

                            rrddim_add(st, "reads", NULL, 1, 1000000, RRDDIM_ABSOLUTE);
                            rrddim_add(st, "writes", NULL, -1, 1000000, RRDDIM_ABSOLUTE);
                        }
                        else rrdset_next(st);

                        rrddim_set(st, "reads", (diskstat.reads - prev_diskstat.operations_read) ?
                            (cur_diskstat.duration_read_ns - prev_diskstat.duration_read_ns) / (diskstat.reads - prev_diskstat.operations_read) : 0);
                        rrddim_set(st, "writes", (diskstat.writes - prev_diskstat.operations_write) ?
                            (cur_diskstat.duration_write_ns - prev_diskstat.duration_write_ns) / (diskstat.writes - prev_diskstat.operations_write) : 0);
                        rrdset_done(st);

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype("disk_avgsz", diskstat.name);
                        if (unlikely(!st)) {
                            st = rrdset_create("disk_avgsz", diskstat.name, NULL, diskstat.name, "disk.avgsz", "Average Completed I/O Operation Bandwidth", "kilobytes per operation", 2006, update_every, RRDSET_TYPE_AREA);
                            st->isdetail = 1;

                            rrddim_add(st, "reads", NULL, 1, 1024, RRDDIM_ABSOLUTE);
                            rrddim_add(st, "writes", NULL, -1, 1024, RRDDIM_ABSOLUTE);
                        }
                        else rrdset_next(st);

                        rrddim_set(st, "reads", (diskstat.reads - prev_diskstat.operations_read) ?
                            (diskstat.bytes_read - prev_diskstat.bytes_read) / (diskstat.reads - prev_diskstat.operations_read) : 0);
                        rrddim_set(st, "writes", (diskstat.writes - prev_diskstat.operations_write) ?
                            (diskstat.bytes_write - prev_diskstat.bytes_write) / (diskstat.writes - prev_diskstat.operations_write) : 0);
                        rrdset_done(st);

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype("disk_svctm", diskstat.name);
                        if (unlikely(!st)) {
                            st = rrdset_create("disk_svctm", diskstat.name, NULL, diskstat.name, "disk.svctm", "Average Service Time", "ms per operation", 2007, update_every, RRDSET_TYPE_LINE);
                            st->isdetail = 1;

                            rrddim_add(st, "svctm", NULL, 1, 1000000, RRDDIM_ABSOLUTE);
                        }
                        else rrdset_next(st);

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
        st = rrdset_find_bytype("system", "io");
        if (unlikely(!st)) {
            st = rrdset_create("system", "io", NULL, "disk", NULL, "Disk I/O", "kilobytes/s", 150, update_every, RRDSET_TYPE_AREA);
            rrddim_add(st, "in",  NULL,  1, 1024, RRDDIM_INCREMENTAL);
            rrddim_add(st, "out", NULL, -1, 1024, RRDDIM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "in", total_disk_reads);
        rrddim_set(st, "out", total_disk_writes);
        rrdset_done(st);
    }

    return 0;
}
