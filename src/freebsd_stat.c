#include "common.h"
#include <sys/vmmeter.h>

int do_proc_stat(int update_every, unsigned long long dt) {
    (void)dt;

    static int do_cpu = -1, do_cpu_cores = -1, do_interrupts = -1, do_context = -1, do_forks = -1, do_processes = -1;

    if(unlikely(do_cpu == -1)) {
        do_cpu          = config_get_boolean("plugin:proc:/proc/stat", "cpu utilization", 1);
        do_cpu_cores    = config_get_boolean("plugin:proc:/proc/stat", "per cpu core utilization", 1);
        do_interrupts   = config_get_boolean("plugin:proc:/proc/stat", "cpu interrupts", 1);
        do_context      = config_get_boolean("plugin:proc:/proc/stat", "context switches", 1);
        do_forks        = config_get_boolean("plugin:proc:/proc/stat", "processes started", 1);
        do_processes    = config_get_boolean("plugin:proc:/proc/stat", "processes running", 1);
    }
    
    RRDSET *st;

    char cpuid[5] = "cpu0";
    unsigned long long user = 0, nice = 0, system = 0, interrupt = 0, idle = 0;
    u_int vm_stat;
    size_t intrcnt_size;
    unsigned long nintr = 0;
    unsigned long *intrcnt;
    unsigned long long totalintr = 0;
    unsigned long long running = 0 , blocked = 0;
    
    long multiplier = 1;
    long divisor = 1; // sysconf(_SC_CLK_TCK);
    
    long cp_time[CPUSTATES];
    long *pcpu_cp_time;
    
    int i, ncpus;
    
    struct vmtotal total;
    
    if(likely(do_cpu)) {
        if (unlikely(CPUSTATES != 5)) {
            error("There are %d CPU states (5 was expected)", CPUSTATES);
            do_cpu = 0;
            error("Total CPU utilization stats was switched off");
            return 0;
        }
        if (unlikely(GETSYSCTL("kern.cp_time", cp_time))) {
            do_cpu = 0;
            error("Total CPU utilization stats was switched off");
            return 0;
        }
        user = cp_time[0];
        nice = cp_time[1];
        system = cp_time[2];
        interrupt = cp_time[3];
        idle = cp_time[4];

        st = rrdset_find_bytype("system", "cpu");
        if(unlikely(!st)) {
            st = rrdset_create("system", "cpu", NULL, "cpu", "system.cpu", "Total CPU utilization", "percentage", 100, update_every, RRDSET_TYPE_STACKED);

            rrddim_add(st, "user", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
            rrddim_add(st, "system", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
            rrddim_add(st, "interrupt", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
            rrddim_add(st, "nice", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
            rrddim_add(st, "idle", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
            rrddim_hide(st, "idle");
        }
        else rrdset_next(st);

        rrddim_set(st, "user", user);
        rrddim_set(st, "system", system);
        rrddim_set(st, "interrupt", interrupt);
        rrddim_set(st, "nice", nice);
        rrddim_set(st, "idle", idle);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------
    
    if(likely(do_cpu_cores)) {
        if (unlikely(CPUSTATES != 5)) {
            error("There are %d CPU states (5 was expected)", CPUSTATES);
            do_cpu_cores = 0;
            error("CPU cores utilization stats was switched off");
            return 0;
        }
        if (unlikely(GETSYSCTL("kern.smp.cpus", ncpus))) {
            do_cpu_cores = 0;
            error("CPU cores utilization stats was switched off");
            return 0;
        }
        pcpu_cp_time = malloc(sizeof(cp_time) * ncpus);
        
        for (i = 0; i < ncpus; i++) {            
            if (unlikely(getsysctl("kern.cp_times", pcpu_cp_time, sizeof(cp_time) * ncpus))) {
                do_cpu_cores = 0;
                error("CPU cores utilization stats was switched off");
                return 0;
            }
            cpuid[3] = '0' + i;
            user = pcpu_cp_time[i * 5 + 0];
            nice = pcpu_cp_time[i * 5 + 1];
            system = pcpu_cp_time[i * 5 + 2];
            interrupt = pcpu_cp_time[i * 5 + 3];
            idle = pcpu_cp_time[i * 5 + 4];

            st = rrdset_find_bytype("cpu", cpuid);
            if(unlikely(!st)) {
                st = rrdset_create("cpu", cpuid, NULL, "utilization", "cpu.cpu", "Core utilization", "percentage", 1000, update_every, RRDSET_TYPE_STACKED);

                rrddim_add(st, "user", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                rrddim_add(st, "system", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                rrddim_add(st, "interrupt", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);            
                rrddim_add(st, "nice", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                rrddim_add(st, "idle", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                rrddim_hide(st, "idle");
            }
            else rrdset_next(st);

            rrddim_set(st, "user", user);
            rrddim_set(st, "system", system);
            rrddim_set(st, "interrupt", interrupt);        
            rrddim_set(st, "nice", nice);        
            rrddim_set(st, "idle", idle);
            rrdset_done(st);
        }
        free(pcpu_cp_time);
    }

    // --------------------------------------------------------------------

    if(likely(do_interrupts)) {
        if (unlikely(sysctlbyname("hw.intrcnt", NULL, &intrcnt_size, NULL, 0) == -1)) {
            error("sysctl(hw.intrcnt...) failed: %s", strerror(errno));
            do_interrupts = 0;
            error("Total device interrupts stats was switched off");
            return 0;
        }
        nintr = intrcnt_size / sizeof(u_long);
        intrcnt = malloc(nintr * sizeof(u_long));
        if (unlikely(getsysctl("hw.intrcnt", intrcnt, nintr * sizeof(u_long)))){
            do_interrupts = 0;
            error("Total device interrupts stats was switched off");
            return 0;
        }
        for (i = 0; i < nintr; i++)
            totalintr += intrcnt[i];
        free(intrcnt);

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

/* Temporarily switched off
    if(likely(do_interrupts)) {
        if (unlikely(GETSYSCTL("vm.stats.sys.v_intr", vm_stat))) {
            do_interrupts = 0;
            error("Device interrupts utilization stats was switched off");
            return 0;
        }

        st = rrdset_find_bytype("system", "intr");
        if(unlikely(!st)) {
            st = rrdset_create("system", "intr", NULL, "interrupts", NULL, "Device Interrupts", "interrupts/s", 900, update_every, RRDSET_TYPE_LINE);
            st->isdetail = 1;

            rrddim_add(st, "interrupts", NULL, 1, 1, RRDDIM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "interrupts", vm_stat);
        rrdset_done(st);
    }
*/
    // --------------------------------------------------------------------

    if(likely(do_context)) {
        if (unlikely(GETSYSCTL("vm.stats.sys.v_swtch", vm_stat))) {
            do_context = 0;
            error("CPU context switches stats was switched off");
            return 0;
        }
        
        st = rrdset_find_bytype("system", "ctxt");
        if(unlikely(!st)) {
            st = rrdset_create("system", "ctxt", NULL, "processes", NULL, "CPU Context Switches", "context switches/s", 800, update_every, RRDSET_TYPE_LINE);

            rrddim_add(st, "switches", NULL, 1, 1, RRDDIM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "switches", vm_stat);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(likely(do_forks)) {
        if (unlikely(GETSYSCTL("vm.stats.vm.v_forks", vm_stat))) {
            do_forks = 0;
            error("Fork stats was switched off");
            return 0;
        }
        
        st = rrdset_find_bytype("system", "forks");
        if(unlikely(!st)) {
            st = rrdset_create("system", "forks", NULL, "processes", NULL, "Started Processes", "processes/s", 700, update_every, RRDSET_TYPE_LINE);
            st->isdetail = 1;

            rrddim_add(st, "started", NULL, 1, 1, RRDDIM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "started", vm_stat);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(likely(do_processes)) {
        if (unlikely(GETSYSCTL("vm.vmtotal", total))) {
            do_processes = 0;
            error("System processes stats was switched off");
            return 0;
        }
        
        running = total.t_rq;
        blocked = total.t_dw + total.t_pw;
        
        st = rrdset_find_bytype("system", "processes");
        if(unlikely(!st)) {
            st = rrdset_create("system", "processes", NULL, "processes", NULL, "System Processes", "processes", 600, update_every, RRDSET_TYPE_LINE);

            rrddim_add(st, "running", NULL, 1, 1, RRDDIM_ABSOLUTE);
            rrddim_add(st, "blocked", NULL, -1, 1, RRDDIM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "running", running);
        rrddim_set(st, "blocked", blocked);
        rrdset_done(st);
    }

    return 0;
}