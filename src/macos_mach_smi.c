// SPDX-License-Identifier: GPL-3.0+
#include "common.h"
#include <mach/mach.h>

int do_macos_mach_smi(int update_every, usec_t dt) {
    (void)dt;

    static int do_cpu = -1, do_ram = - 1, do_swapio = -1, do_pgfaults = -1;

    if (unlikely(do_cpu == -1)) {
        do_cpu                  = config_get_boolean("plugin:macos:mach_smi", "cpu utilization", 1);
        do_ram                  = config_get_boolean("plugin:macos:mach_smi", "system ram", 1);
        do_swapio               = config_get_boolean("plugin:macos:mach_smi", "swap i/o", 1);
        do_pgfaults             = config_get_boolean("plugin:macos:mach_smi", "memory page faults", 1);
    }

    RRDSET *st;

	kern_return_t kr;
	mach_msg_type_number_t count;
    host_t host;
    vm_size_t system_pagesize;


    // NEEDED BY: do_cpu
    natural_t cp_time[CPU_STATE_MAX];

    // NEEDED BY: do_ram, do_swapio, do_pgfaults
#if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED >= 1060)
    vm_statistics64_data_t vm_statistics;
#else
    vm_statistics_data_t vm_statistics;
#endif

    host = mach_host_self();
    kr = host_page_size(host, &system_pagesize);
    if (unlikely(kr != KERN_SUCCESS))
        return -1;

    // --------------------------------------------------------------------

    if (likely(do_cpu)) {
        if (unlikely(HOST_CPU_LOAD_INFO_COUNT != 4)) {
            error("MACOS: There are %d CPU states (4 was expected)", HOST_CPU_LOAD_INFO_COUNT);
            do_cpu = 0;
            error("DISABLED: system.cpu");
        } else {
            count = HOST_CPU_LOAD_INFO_COUNT;
            kr = host_statistics(host, HOST_CPU_LOAD_INFO, (host_info_t)cp_time, &count);
            if (unlikely(kr != KERN_SUCCESS)) {
                error("MACOS: host_statistics() failed: %s", mach_error_string(kr));
                do_cpu = 0;
                error("DISABLED: system.cpu");
            } else {

                st = rrdset_find_bytype_localhost("system", "cpu");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "system"
                            , "cpu"
                            , NULL
                            , "cpu"
                            , "system.cpu"
                            , "Total CPU utilization"
                            , "percentage"
                            , "macos"
                            , "mach_smi"
                            , 100
                            , update_every
                            , RRDSET_TYPE_STACKED
                    );

                    rrddim_add(st, "user", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "nice", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "system", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "idle", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_hide(st, "idle");
                }
                else rrdset_next(st);

                rrddim_set(st, "user", cp_time[CPU_STATE_USER]);
                rrddim_set(st, "nice", cp_time[CPU_STATE_NICE]);
                rrddim_set(st, "system", cp_time[CPU_STATE_SYSTEM]);
                rrddim_set(st, "idle", cp_time[CPU_STATE_IDLE]);
                rrdset_done(st);
            }
        }
     }

    // --------------------------------------------------------------------
    
    if (likely(do_ram || do_swapio || do_pgfaults)) {
#if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED >= 1060)
        count = sizeof(vm_statistics64_data_t);
        kr = host_statistics64(host, HOST_VM_INFO64, (host_info64_t)&vm_statistics, &count);
#else
        count = sizeof(vm_statistics_data_t);
        kr = host_statistics(host, HOST_VM_INFO, (host_info_t)&vm_statistics, &count);
#endif
        if (unlikely(kr != KERN_SUCCESS)) {
            error("MACOS: host_statistics64() failed: %s", mach_error_string(kr));
            do_ram = 0;
            error("DISABLED: system.ram");
            do_swapio = 0;
            error("DISABLED: system.swapio");
            do_pgfaults = 0;
            error("DISABLED: mem.pgfaults");
        } else {
            if (likely(do_ram)) {
                st = rrdset_find_localhost("system.ram");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "system"
                            , "ram"
                            , NULL
                            , "ram"
                            , NULL
                            , "System RAM"
                            , "MB"
                            , "macos"
                            , "mach_smi"
                            , 200
                            , update_every
                            , RRDSET_TYPE_STACKED
                    );

                    rrddim_add(st, "active",    NULL, system_pagesize, 1048576, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(st, "wired",     NULL, system_pagesize, 1048576, RRD_ALGORITHM_ABSOLUTE);
#if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED >= 1090)
                    rrddim_add(st, "throttled", NULL, system_pagesize, 1048576, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(st, "compressor", NULL, system_pagesize, 1048576, RRD_ALGORITHM_ABSOLUTE);
#endif
                    rrddim_add(st, "inactive",  NULL, system_pagesize, 1048576, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(st, "purgeable", NULL, system_pagesize, 1048576, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(st, "speculative", NULL, system_pagesize, 1048576, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(st, "free",      NULL, system_pagesize, 1048576, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "active",    vm_statistics.active_count);
                rrddim_set(st, "wired",     vm_statistics.wire_count);
#if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED >= 1090)
                rrddim_set(st, "throttled", vm_statistics.throttled_count);
                rrddim_set(st, "compressor", vm_statistics.compressor_page_count);
#endif
                rrddim_set(st, "inactive",  vm_statistics.inactive_count);
                rrddim_set(st, "purgeable", vm_statistics.purgeable_count);
                rrddim_set(st, "speculative", vm_statistics.speculative_count);
                rrddim_set(st, "free",      (vm_statistics.free_count - vm_statistics.speculative_count));
                rrdset_done(st);
            }

#if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED >= 1090)
            // --------------------------------------------------------------------

            if (likely(do_swapio)) {
                st = rrdset_find_localhost("system.swapio");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "system"
                            , "swapio"
                            , NULL
                            , "swap"
                            , NULL
                            , "Swap I/O"
                            , "kilobytes/s"
                            , "macos"
                            , "mach_smi"
                            , 250
                            , update_every
                            , RRDSET_TYPE_AREA
                    );

                    rrddim_add(st, "in",  NULL, system_pagesize, 1024, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "out", NULL, -system_pagesize, 1024, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "in", vm_statistics.swapins);
                rrddim_set(st, "out", vm_statistics.swapouts);
                rrdset_done(st);
            }
#endif

            // --------------------------------------------------------------------

            if (likely(do_pgfaults)) {
                st = rrdset_find_localhost("mem.pgfaults");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "mem"
                            , "pgfaults"
                            , NULL
                            , "system"
                            , NULL
                            , "Memory Page Faults"
                            , "page faults/s"
                            , "macos"
                            , "mach_smi"
                            , NETDATA_CHART_PRIO_MEM_SYSTEM_PGFAULTS
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rrddim_add(st, "memory",    NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "cow",       NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "pagein",    NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "pageout",   NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
#if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED >= 1090)
                    rrddim_add(st, "compress",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "decompress", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
#endif
                    rrddim_add(st, "zero_fill", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "reactivate", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "purge",     NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "memory", vm_statistics.faults);
                rrddim_set(st, "cow", vm_statistics.cow_faults);
                rrddim_set(st, "pagein", vm_statistics.pageins);
                rrddim_set(st, "pageout", vm_statistics.pageouts);
#if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED >= 1090)
                rrddim_set(st, "compress", vm_statistics.compressions);
                rrddim_set(st, "decompress", vm_statistics.decompressions);
#endif
                rrddim_set(st, "zero_fill", vm_statistics.zero_fill_count);
                rrddim_set(st, "reactivate", vm_statistics.reactivations);
                rrddim_set(st, "purge", vm_statistics.purges);
                rrdset_done(st);
            }
        }
    } 
 
    // --------------------------------------------------------------------

    return 0;
}
