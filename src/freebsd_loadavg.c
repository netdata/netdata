#include "common.h"
#include <sys/vmmeter.h>

// FreeBSD calculates this once every 5 seconds
#define MIN_LOADAVG_UPDATE_EVERY 5

int do_proc_loadavg(int update_every, unsigned long long dt) {
    (void)dt;

    static int do_loadavg = -1, do_all_processes = -1;
    static unsigned long long last_loadavg_usec = 0;
    static RRDSET *load_chart = NULL, *processes_chart = NULL;

    if(unlikely(do_loadavg == -1)) {
        do_loadavg          = config_get_boolean("plugin:proc:/proc/loadavg", "enable load average", 1);
        do_all_processes    = config_get_boolean("plugin:proc:/proc/loadavg", "enable total processes", 1);
    }

    struct loadavg sysload;
    
    if (unlikely(GETSYSCTL("vm.loadavg", sysload)))
        return 1;
    
    double load1 = (double)sysload.ldavg[0] / sysload.fscale;
    double load5 = (double)sysload.ldavg[1] / sysload.fscale;
    double load15 = (double)sysload.ldavg[2] / sysload.fscale;

    struct vmtotal total;
    
    if (unlikely(GETSYSCTL("vm.vmtotal", total)))
        return 1;
    
    unsigned long long active_processes     = total.t_rq + total.t_dw + total.t_pw + total.t_sl + total.t_sw;

    // --------------------------------------------------------------------

    if(last_loadavg_usec <= dt) {
        if(likely(do_loadavg)) {
            if(unlikely(!load_chart)) {
                load_chart = rrdset_find_byname("system.load");
                if(unlikely(!load_chart)) {
                    load_chart = rrdset_create("system", "load", NULL, "load", NULL, "System Load Average", "load", 100, (update_every < MIN_LOADAVG_UPDATE_EVERY) ? MIN_LOADAVG_UPDATE_EVERY : update_every, RRDSET_TYPE_LINE);
                    rrddim_add(load_chart, "load1", NULL, 1, 1000, RRDDIM_ABSOLUTE);
                    rrddim_add(load_chart, "load5", NULL, 1, 1000, RRDDIM_ABSOLUTE);
                    rrddim_add(load_chart, "load15", NULL, 1, 1000, RRDDIM_ABSOLUTE);
                }
            }
            else
                rrdset_next(load_chart);

            rrddim_set(load_chart, "load1", (collected_number) (load1 * 1000));
            rrddim_set(load_chart, "load5", (collected_number) (load5 * 1000));
            rrddim_set(load_chart, "load15", (collected_number) (load15 * 1000));
            rrdset_done(load_chart);
        }

        last_loadavg_usec = load_chart->update_every * 1000000ULL;
    }
    else last_loadavg_usec -= dt;

    // --------------------------------------------------------------------

    if(likely(do_all_processes)) {
        if(unlikely(!processes_chart)) {
            processes_chart = rrdset_find_byname("system.active_processes");
            if(unlikely(!processes_chart)) {
                processes_chart = rrdset_create("system", "active_processes", NULL, "processes", NULL, "System Active Processes", "processes", 750, update_every, RRDSET_TYPE_LINE);
                rrddim_add(processes_chart, "active", NULL, 1, 1, RRDDIM_ABSOLUTE);
            }
        }
        else rrdset_next(processes_chart);

        rrddim_set(processes_chart, "active", active_processes);
        rrdset_done(processes_chart);
    }

    return 0;
}
