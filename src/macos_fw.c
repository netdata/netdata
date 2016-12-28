#include "common.h"
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <IOKit/storage/IOBlockStorageDriver.h>

int do_macos_iokit(int update_every, usec_t dt) {
    (void)dt;

    static int do_io = -1;

    if (unlikely(do_io == -1)) {
        do_io                  = config_get_boolean("plugin:macos:iokit", "disk i/o", 1);
    }

    RRDSET *st;

    mach_port_t         master_port;
    io_registry_entry_t drive;
    io_iterator_t       drive_list;
    CFNumberRef         number;
    CFDictionaryRef     properties, statistics;
    UInt64              value;
    collected_number    total_disk_reads = 0;
    collected_number    total_disk_writes = 0;

    /* Get ports and services for drive statistics. */
    if (IOMasterPort(bootstrap_port, &master_port)) {
        error("MACOS: IOMasterPort() failed");
        do_io = 0;
        error("DISABLED: system.io");
    /* Get the list of all drive objects. */
    } else if (IOServiceGetMatchingServices(master_port, IOServiceMatching("IOBlockStorageDriver"), &drive_list)) {
        error("MACOS: IOServiceGetMatchingServices() failed");
        do_io = 0;
        error("DISABLED: system.io");
    } else {
        while ((drive = IOIteratorNext(drive_list)) != 0) {
            number = 0;
            properties = 0;
            statistics = 0;
            value = 0;

            /* Obtain the properties for this drive object. */
            if (IORegistryEntryCreateCFProperties(drive, (CFMutableDictionaryRef *)&properties, kCFAllocatorDefault, 0)) {
                error("MACOS: IORegistryEntryCreateCFProperties() failed");
                do_io = 0;
                error("DISABLED: system.io");
                break;
            } else if (properties != 0) {
                /* Obtain the statistics from the drive properties. */
                statistics = (CFDictionaryRef)CFDictionaryGetValue(properties, CFSTR(kIOBlockStorageDriverStatisticsKey));

                if (statistics != 0) {
                    /* Get bytes read. */
                    number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsBytesReadKey));
                    if (number != 0) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &value);
                        total_disk_reads += value;
                    }

                    /* Get bytes written. */
                    number = (CFNumberRef)CFDictionaryGetValue(statistics, CFSTR(kIOBlockStorageDriverStatisticsBytesWrittenKey));
                    if (number != 0) {
                        CFNumberGetValue(number, kCFNumberSInt64Type, &value);
                        total_disk_writes += value;
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

    if (do_io) {
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
