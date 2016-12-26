#include "common.h"
#include <mach/mach_host.h>

int do_macos_mach_smi(int update_every, usec_t dt) {
    (void)dt;

    static int do_cpu = -1;

    if (unlikely(do_cpu == -1)) {
        do_cpu                  = config_get_boolean("plugin:macos:mach_smi", "cpu utilization", 1);
    }

    RRDSET *st;

    // NEEDED BY: do_cpu, do_cpu_cores
    natural_t cp_time[CPU_STATE_MAX];
	kern_return_t kr;
	mach_msg_type_number_t count;

    // --------------------------------------------------------------------

    if (likely(do_cpu)) {
        if (unlikely(HOST_CPU_LOAD_INFO_COUNT != 4)) {
            error("FREEBSD: There are %d CPU states (4 was expected)", HOST_CPU_LOAD_INFO_COUNT);
            do_cpu = 0;
            error("DISABLED: system.cpu");
        } else {
            count = HOST_CPU_LOAD_INFO_COUNT;
            if (unlikely(kr = host_statistics(mach_host_self(), HOST_CPU_LOAD_INFO, (host_info_t)cp_time, &count))) {
                if (kr != KERN_SUCCESS) {
                    error("MACOS: host_statistics() failed: %s", mach_error_string(kr));
                    do_cpu = 0;
                    error("DISABLED: system.cpu");
                }
            } else {

                st = rrdset_find_bytype("system", "cpu");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "cpu", NULL, "cpu", "system.cpu", "Total CPU utilization", "percentage", 100, update_every, RRDSET_TYPE_STACKED);

                    rrddim_add(st, "user", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "nice", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "system", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "idle", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
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

    return 0;
}
