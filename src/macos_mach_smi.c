#include "common.h"

int do_macos_mach_smi(int update_every, usec_t dt) {
    (void)dt;

    static int do_cpu = -1;

    if (unlikely(do_cpu == -1)) {
        do_cpu                  = config_get_boolean("plugin:macos:mach_smi", "cpu utilization", 1);
    }

    RRDSET *st;

    // NEEDED BY: do_cpu, do_cpu_cores
    long cp_time[5];

    // --------------------------------------------------------------------

    if (likely(do_cpu)) {
        if (unlikely(1)) {
            do_cpu = 0;
            error("DISABLED: system.cpu");
        } else {

            st = rrdset_find_bytype("system", "cpu");
            if (unlikely(!st)) {
                st = rrdset_create("system", "cpu", NULL, "cpu", "system.cpu", "Total CPU utilization", "percentage", 100, update_every, RRDSET_TYPE_STACKED);

                rrddim_add(st, "user", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                rrddim_add(st, "nice", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                rrddim_add(st, "system", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                rrddim_add(st, "interrupt", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                rrddim_add(st, "idle", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                rrddim_hide(st, "idle");
            }
            else rrdset_next(st);

            rrddim_set(st, "user", cp_time[0]);
            rrddim_set(st, "nice", cp_time[1]);
            rrddim_set(st, "system", cp_time[2]);
            rrddim_set(st, "interrupt", cp_time[3]);
            rrddim_set(st, "idle", cp_time[4]);
            rrdset_done(st);
        }
     }

    return 0;
}
