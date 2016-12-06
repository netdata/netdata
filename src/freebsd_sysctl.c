#include "common.h"

// NEEDED BY: struct vmstat
#include <sys/vmmeter.h>

// FreeBSD calculates load averages once every 5 seconds
#define MIN_LOADAVG_UPDATE_EVERY 5

int do_freebsd_sysctl(int update_every, unsigned long long dt) {
    (void)dt;

    static int do_cpu = -1, do_cpu_cores = -1, do_interrupts = -1, do_context = -1, do_forks = -1, do_processes = -1,
        do_loadavg = -1, do_all_processes = -1;

    if(unlikely(do_cpu == -1)) {
        do_cpu                  = config_get_boolean("plugin:freebsd:sysctl", "cpu utilization", 1);
        do_cpu_cores            = config_get_boolean("plugin:freebsd:sysctl", "per cpu core utilization", 1);
        do_interrupts           = config_get_boolean("plugin:freebsd:sysctl", "cpu interrupts", 1);
        do_context              = config_get_boolean("plugin:freebsd:sysctl", "context switches", 1);
        do_forks                = config_get_boolean("plugin:freebsd:sysctl", "processes started", 1);
        do_processes            = config_get_boolean("plugin:freebsd:sysctl", "processes running", 1);
        do_loadavg              = config_get_boolean("plugin:freebsd:sysctl", "enable load average", 1);
        do_all_processes        = config_get_boolean("plugin:freebsd:sysctl", "enable total processes", 1);
    }

    RRDSET *st;

    int i;

// NEEDED BY: do_loadavg
    static unsigned long long last_loadavg_usec = 0;
    struct loadavg sysload;

// NEEDED BY: do_cpu, do_cpu_cores
    long cp_time[CPUSTATES];

// NEEDED BY: do_cpu_cores
    int ncpus;
    static long *pcpu_cp_time = NULL;
    char cpuid[8]; // no more than 4 digits expected

// NEEDED BY: do_all_processes, do_processes
    struct vmtotal vmtotal_data;

// NEEDED BY: do_context, do_forks
    u_int u_int_data;

// NEEDED BY: do_interrupts
    size_t intrcnt_size;
    unsigned long nintr = 0;
    static unsigned long *intrcnt = NULL;
    unsigned long long totalintr = 0;

    // --------------------------------------------------------------------

    if(last_loadavg_usec <= dt) {
        if(likely(do_loadavg)) {
            if (unlikely(GETSYSCTL("vm.loadavg", sysload))) {
                do_loadavg = 0;
                error("DISABLED: system.load");
            } else {

                st = rrdset_find_bytype("system", "load");
                if(unlikely(!st)) {
                    st = rrdset_create("system", "load", NULL, "load", NULL, "System Load Average", "load", 100, (update_every < MIN_LOADAVG_UPDATE_EVERY) ? MIN_LOADAVG_UPDATE_EVERY : update_every, RRDSET_TYPE_LINE);
                    rrddim_add(st, "load1", NULL, 1, 1000, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "load5", NULL, 1, 1000, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "load15", NULL, 1, 1000, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "load1", (collected_number) ((double)sysload.ldavg[0] / sysload.fscale * 1000));
                rrddim_set(st, "load5", (collected_number) ((double)sysload.ldavg[1] / sysload.fscale * 1000));
                rrddim_set(st, "load15", (collected_number) ((double)sysload.ldavg[2] / sysload.fscale * 1000));
                rrdset_done(st);
            }
        }

        last_loadavg_usec = st->update_every * 1000000ULL;
    }
    else last_loadavg_usec -= dt;

    // --------------------------------------------------------------------

    if(likely(do_all_processes | do_processes)) {
        if (unlikely(GETSYSCTL("vm.vmtotal", vmtotal_data))) {
            do_all_processes = 0;
            error("DISABLED: system.active_processes");
            do_processes = 0;
            error("DISABLED: system.processes");
        } else {
            if(likely(do_processes)) {

                st = rrdset_find_bytype("system", "active_processes");
                if(unlikely(!st)) {
                    st = rrdset_create("system", "active_processes", NULL, "processes", NULL, "System Active Processes", "processes", 750, update_every, RRDSET_TYPE_LINE);
                    rrddim_add(st, "active", NULL, 1, 1, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "active", (vmtotal_data.t_rq + vmtotal_data.t_dw + vmtotal_data.t_pw + vmtotal_data.t_sl + vmtotal_data.t_sw));
                rrdset_done(st);
            }
            if(likely(do_processes)) {

                st = rrdset_find_bytype("system", "processes");
                if(unlikely(!st)) {
                    st = rrdset_create("system", "processes", NULL, "processes", NULL, "System Processes", "processes", 600, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "running", NULL, 1, 1, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "blocked", NULL, -1, 1, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "running", vmtotal_data.t_rq);
                rrddim_set(st, "blocked", (vmtotal_data.t_dw + vmtotal_data.t_pw));
                rrdset_done(st);
            }

        }
    }

    // --------------------------------------------------------------------

    if(likely(do_processes)) {

            st = rrdset_find_bytype("system", "processes");
            if(unlikely(!st)) {
                st = rrdset_create("system", "processes", NULL, "processes", NULL, "System Processes", "processes", 600, update_every, RRDSET_TYPE_LINE);

                rrddim_add(st, "running", NULL, 1, 1, RRDDIM_ABSOLUTE);
                rrddim_add(st, "blocked", NULL, -1, 1, RRDDIM_ABSOLUTE);
            }
            else rrdset_next(st);

            rrddim_set(st, "running", vmtotal_data.t_rq);
            rrddim_set(st, "blocked", (vmtotal_data.t_dw + vmtotal_data.t_pw));
            rrdset_done(st);
        }

    // --------------------------------------------------------------------

    if(likely(do_cpu)) {
        if (unlikely(CPUSTATES != 5)) {
            error("FREEBSD: There are %d CPU states (5 was expected)", CPUSTATES);
            do_cpu = 0;
            error("DISABLED: system.cpu");
        } else {
            if (unlikely(GETSYSCTL("kern.cp_time", cp_time))) {
                do_cpu = 0;
                error("DISABLED: system.cpu");
            } else {

                st = rrdset_find_bytype("system", "cpu");
                if(unlikely(!st)) {
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
    }

    // --------------------------------------------------------------------

    if(likely(do_cpu_cores)) {
        if (unlikely(CPUSTATES != 5)) {
            error("FREEBSD: There are %d CPU states (5 was expected)", CPUSTATES);
            do_cpu_cores = 0;
            error("DISABLED: cpu.cpuXX");
        } else {
            if (unlikely(GETSYSCTL("kern.smp.cpus", ncpus))) {
                do_cpu_cores = 0;
                error("DISABLED: cpu.cpuXX");
            } else {
                pcpu_cp_time = reallocz(pcpu_cp_time, sizeof(cp_time) * ncpus);

                for (i = 0; i < ncpus; i++) {
                    if (unlikely(getsysctl("kern.cp_times", pcpu_cp_time, sizeof(cp_time) * ncpus))) {
                        do_cpu_cores = 0;
                        error("DISABLED: cpu.cpuXX");
                        break;
                    }
                    if (unlikely(ncpus > 9999)) {
                        error("FREEBSD: There are more than 4 digits in cpu cores number");
                        do_cpu_cores = 0;
                        error("DISABLED: cpu.cpuXX");
                        break;
                    }
                    snprintfz(cpuid, 8, "cpu%d", i);

                    st = rrdset_find_bytype("cpu", cpuid);
                    if(unlikely(!st)) {
                        st = rrdset_create("cpu", cpuid, NULL, "utilization", "cpu.cpu", "Core utilization", "percentage", 1000, update_every, RRDSET_TYPE_STACKED);

                        rrddim_add(st, "user", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                        rrddim_add(st, "nice", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                        rrddim_add(st, "system", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                        rrddim_add(st, "interrupt", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                        rrddim_add(st, "idle", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                        rrddim_hide(st, "idle");
                    }
                    else rrdset_next(st);

                    rrddim_set(st, "user", pcpu_cp_time[i * 5 + 0]);
                    rrddim_set(st, "nice", pcpu_cp_time[i * 5 + 1]);
                    rrddim_set(st, "system", pcpu_cp_time[i * 5 + 2]);
                    rrddim_set(st, "interrupt", pcpu_cp_time[i * 5 + 3]);
                    rrddim_set(st, "idle", pcpu_cp_time[i * 5 + 4]);
                    rrdset_done(st);
                }
            }
        }
    }

    // --------------------------------------------------------------------

    if(likely(do_interrupts)) {
        if (unlikely(sysctlbyname("hw.intrcnt", NULL, &intrcnt_size, NULL, 0) == -1)) {
            error("FREEBSD: sysctl(hw.intrcnt...) failed: %s", strerror(errno));
            do_interrupts = 0;
            error("DISABLED: system.intr");
        } else {
            nintr = intrcnt_size / sizeof(u_long);
            intrcnt = reallocz(intrcnt, nintr * sizeof(u_long));
            if (unlikely(getsysctl("hw.intrcnt", intrcnt, nintr * sizeof(u_long)))){
                do_interrupts = 0;
                error("DISABLED: system.intr");
            } else {
                for (i = 0; i < nintr; i++)
                    totalintr += intrcnt[i];

                st = rrdset_find_bytype("system", "intr");
                if(unlikely(!st)) {
                    st = rrdset_create("system", "intr", NULL, "interrupts", NULL, "Total Device Interrupts", "interrupts/s", 900, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "interrupts", NULL, 1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "interrupts", totalintr);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------

    if(likely(do_context)) {
        if (unlikely(GETSYSCTL("vm.stats.sys.v_swtch", u_int_data))) {
            do_context = 0;
            error("DISABLED: system.ctxt");
        } else {

            st = rrdset_find_bytype("system", "ctxt");
            if(unlikely(!st)) {
                st = rrdset_create("system", "ctxt", NULL, "processes", NULL, "CPU Context Switches", "context switches/s", 800, update_every, RRDSET_TYPE_LINE);

                rrddim_add(st, "switches", NULL, 1, 1, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "switches", u_int_data);
            rrdset_done(st);
        }
    }

    // --------------------------------------------------------------------

    if(likely(do_forks)) {
        if (unlikely(GETSYSCTL("vm.stats.vm.v_forks", u_int_data))) {
            do_forks = 0;
            error("DISABLED: system.forks");
        } else {

            st = rrdset_find_bytype("system", "forks");
            if(unlikely(!st)) {
                st = rrdset_create("system", "forks", NULL, "processes", NULL, "Started Processes", "processes/s", 700, update_every, RRDSET_TYPE_LINE);
                st->isdetail = 1;

                rrddim_add(st, "started", NULL, 1, 1, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "started", u_int_data);
            rrdset_done(st);
        }
    }

    return 0;
}
