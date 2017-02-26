#include "common.h"

int do_proc_stat(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_cpu = -1, do_cpu_cores = -1, do_interrupts = -1, do_context = -1, do_forks = -1, do_processes = -1;
    static uint32_t hash_intr, hash_ctxt, hash_processes, hash_procs_running, hash_procs_blocked;

    if(unlikely(do_cpu == -1)) {
        do_cpu          = config_get_boolean("plugin:proc:/proc/stat", "cpu utilization", 1);
        do_cpu_cores    = config_get_boolean("plugin:proc:/proc/stat", "per cpu core utilization", 1);
        do_interrupts   = config_get_boolean("plugin:proc:/proc/stat", "cpu interrupts", 1);
        do_context      = config_get_boolean("plugin:proc:/proc/stat", "context switches", 1);
        do_forks        = config_get_boolean("plugin:proc:/proc/stat", "processes started", 1);
        do_processes    = config_get_boolean("plugin:proc:/proc/stat", "processes running", 1);

        hash_intr = simple_hash("intr");
        hash_ctxt = simple_hash("ctxt");
        hash_processes = simple_hash("processes");
        hash_procs_running = simple_hash("procs_running");
        hash_procs_blocked = simple_hash("procs_blocked");
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/stat");
        ff = procfile_open(config_get("plugin:proc:/proc/stat", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;
    size_t words;

    unsigned long long processes = 0, running = 0 , blocked = 0;
    RRDSET *st;

    for(l = 0; l < lines ;l++) {
        char *row_key = procfile_lineword(ff, l, 0);
        uint32_t hash = simple_hash(row_key);

        // faster strncmp(row_key, "cpu", 3) == 0
        if(likely(row_key[0] == 'c' && row_key[1] == 'p' && row_key[2] == 'u')) {
            words = procfile_linewords(ff, l);
            if(unlikely(words < 9)) {
                error("Cannot read /proc/stat cpu line. Expected 9 params, read %zu.", words);
                continue;
            }

            char *id;
            unsigned long long user = 0, nice = 0, system = 0, idle = 0, iowait = 0, irq = 0, softirq = 0, steal = 0, guest = 0, guest_nice = 0;

            id          = row_key;
            user        = str2ull(procfile_lineword(ff, l, 1));
            nice        = str2ull(procfile_lineword(ff, l, 2));
            system      = str2ull(procfile_lineword(ff, l, 3));
            idle        = str2ull(procfile_lineword(ff, l, 4));
            iowait      = str2ull(procfile_lineword(ff, l, 5));
            irq         = str2ull(procfile_lineword(ff, l, 6));
            softirq     = str2ull(procfile_lineword(ff, l, 7));
            steal       = str2ull(procfile_lineword(ff, l, 8));

            guest       = str2ull(procfile_lineword(ff, l, 9));
            user -= guest;

            guest_nice  = str2ull(procfile_lineword(ff, l, 10));
            nice -= guest_nice;

            char *title, *type, *context, *family;
            long priority;
            int isthistotal;

            if(unlikely(strcmp(id, "cpu")) == 0) {
                title = "Total CPU utilization";
                type = "system";
                context = "system.cpu";
                family = id;
                priority = 100;
                isthistotal = 1;
            }
            else {
                title = "Core utilization";
                type = "cpu";
                context = "cpu.cpu";
                family = "utilization";
                priority = 1000;
                isthistotal = 0;
            }

            if(likely((isthistotal && do_cpu) || (!isthistotal && do_cpu_cores))) {
                st = rrdset_find_bytype_localhost(type, id);
                if(unlikely(!st)) {
                    st = rrdset_create_localhost(type, id, NULL, family, context, title, "percentage", priority
                                                 , update_every, RRDSET_TYPE_STACKED);

                    long multiplier = 1;
                    long divisor = 1; // sysconf(_SC_CLK_TCK);

                    rrddim_add(st, "guest_nice", NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "guest", NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "steal", NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "softirq", NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "irq", NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "user", NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "system", NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "nice", NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_add(st, "iowait", NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);

                    rrddim_add(st, "idle", NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_hide(st, "idle");
                }
                else rrdset_next(st);

                rrddim_set(st, "user", user);
                rrddim_set(st, "nice", nice);
                rrddim_set(st, "system", system);
                rrddim_set(st, "idle", idle);
                rrddim_set(st, "iowait", iowait);
                rrddim_set(st, "irq", irq);
                rrddim_set(st, "softirq", softirq);
                rrddim_set(st, "steal", steal);
                rrddim_set(st, "guest", guest);
                rrddim_set(st, "guest_nice", guest_nice);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_intr && strcmp(row_key, "intr") == 0)) {
            unsigned long long value = str2ull(procfile_lineword(ff, l, 1));

            // --------------------------------------------------------------------

            if(likely(do_interrupts)) {
                st = rrdset_find_bytype_localhost("system", "intr");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("system", "intr", NULL, "interrupts", NULL, "CPU Interrupts"
                                                 , "interrupts/s", 900, update_every, RRDSET_TYPE_LINE);
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rrddim_add(st, "interrupts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "interrupts", value);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_ctxt && strcmp(row_key, "ctxt") == 0)) {
            unsigned long long value = str2ull(procfile_lineword(ff, l, 1));

            // --------------------------------------------------------------------

            if(likely(do_context)) {
                st = rrdset_find_bytype_localhost("system", "ctxt");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("system", "ctxt", NULL, "processes", NULL, "CPU Context Switches"
                                                 , "context switches/s", 800, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "switches", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "switches", value);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_processes && !processes && strcmp(row_key, "processes") == 0)) {
            processes = str2ull(procfile_lineword(ff, l, 1));
        }
        else if(unlikely(hash == hash_procs_running && !running && strcmp(row_key, "procs_running") == 0)) {
            running = str2ull(procfile_lineword(ff, l, 1));
        }
        else if(unlikely(hash == hash_procs_blocked && !blocked && strcmp(row_key, "procs_blocked") == 0)) {
            blocked = str2ull(procfile_lineword(ff, l, 1));
        }
    }

    // --------------------------------------------------------------------

    if(likely(do_forks)) {
        st = rrdset_find_bytype_localhost("system", "forks");
        if(unlikely(!st)) {
            st = rrdset_create_localhost("system", "forks", NULL, "processes", NULL, "Started Processes", "processes/s"
                                         , 700, update_every, RRDSET_TYPE_LINE);
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "started", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "started", processes);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(likely(do_processes)) {
        st = rrdset_find_bytype_localhost("system", "processes");
        if(unlikely(!st)) {
            st = rrdset_create_localhost("system", "processes", NULL, "processes", NULL, "System Processes", "processes"
                                         , 600, update_every, RRDSET_TYPE_LINE);

            rrddim_add(st, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "blocked", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "running", running);
        rrddim_set(st, "blocked", blocked);
        rrdset_done(st);
    }

    return 0;
}
