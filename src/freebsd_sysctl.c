#include "common.h"

#include <sys/vmmeter.h>
#include <vm/vm_param.h>

#define _KERNEL
#include <sys/sem.h>
#include <sys/shm.h>
#include <sys/msg.h>
#undef _KERNEL

#include <net/netisr.h>

#include <netinet/ip.h>
#include <netinet/ip_var.h>
#include <netinet/ip_icmp.h>
#include <netinet/icmp_var.h>
#include <netinet6/ip6_var.h>
#include <netinet/icmp6.h>
#include <netinet/tcp_var.h>
#include <netinet/tcp_fsm.h>
#include <netinet/udp.h>
#include <netinet/udp_var.h>

// --------------------------------------------------------------------------------------------------------------------
// common definitions and variables

int system_pagesize = PAGE_SIZE;
int number_of_cpus = 1;
#if __FreeBSD_version >= 1200029
struct __vmmeter {
	uint64_t v_swtch;
	uint64_t v_trap;
	uint64_t v_syscall;
	uint64_t v_intr;
	uint64_t v_soft;
	uint64_t v_vm_faults;
	uint64_t v_io_faults;
	uint64_t v_cow_faults;
	uint64_t v_cow_optim;
	uint64_t v_zfod;
	uint64_t v_ozfod;
	uint64_t v_swapin;
	uint64_t v_swapout;
	uint64_t v_swappgsin;
	uint64_t v_swappgsout;
	uint64_t v_vnodein;
	uint64_t v_vnodeout;
	uint64_t v_vnodepgsin;
	uint64_t v_vnodepgsout;
	uint64_t v_intrans;
	uint64_t v_reactivated;
	uint64_t v_pdwakeups;
	uint64_t v_pdpages;
	uint64_t v_pdshortfalls;
	uint64_t v_dfree;
	uint64_t v_pfree;
	uint64_t v_tfree;
	uint64_t v_forks;
	uint64_t v_vforks;
	uint64_t v_rforks;
	uint64_t v_kthreads;
	uint64_t v_forkpages;
	uint64_t v_vforkpages;
	uint64_t v_rforkpages;
	uint64_t v_kthreadpages;
	u_int v_page_size;
	u_int v_page_count;
	u_int v_free_reserved;
	u_int v_free_target;
	u_int v_free_min;
	u_int v_free_count;
	u_int v_wire_count;
	u_int v_active_count;
	u_int v_inactive_target;
	u_int v_inactive_count;
	u_int v_laundry_count;
	u_int v_pageout_free_min;
	u_int v_interrupt_free_min;
	u_int v_free_severe;
};
typedef struct __vmmeter vmmeter_t;
#else
typedef struct vmmeter vmmeter_t;
#endif

// --------------------------------------------------------------------------------------------------------------------
// FreeBSD plugin initialization

int freebsd_plugin_init()
{
    system_pagesize = getpagesize();
    if (system_pagesize <= 0) {
        error("FREEBSD: can't get system page size");
        return 1;
    }

    if (unlikely(GETSYSCTL_BY_NAME("kern.smp.cpus", number_of_cpus))) {
        error("FREEBSD: can't get number of cpus");
        return 1;
    }

    if (unlikely(!number_of_cpus)) {
        error("FREEBSD: wrong number of cpus");
        return 1;
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// vm.loadavg

// FreeBSD calculates load averages once every 5 seconds
#define MIN_LOADAVG_UPDATE_EVERY 5

int do_vm_loadavg(int update_every, usec_t dt){
    static usec_t next_loadavg_dt = 0;

    if (next_loadavg_dt <= dt) {
        static int mib[2] = {0, 0};
        struct loadavg sysload;

        if (unlikely(GETSYSCTL_SIMPLE("vm.loadavg", mib, sysload))) {
            error("DISABLED: system.load chart");
            error("DISABLED: vm.loadavg module");
            return 1;
        } else {

            // --------------------------------------------------------------------

            static RRDSET *st = NULL;
            static RRDDIM *rd_load1 = NULL, *rd_load2 = NULL, *rd_load3 = NULL;

            if (unlikely(!st)) {
                st = rrdset_create_localhost("system",
                                             "load",
                                             NULL,
                                             "load",
                                             NULL,
                                             "System Load Average",
                                             "load",
                                             100,
                                             (update_every < MIN_LOADAVG_UPDATE_EVERY) ?
                                             MIN_LOADAVG_UPDATE_EVERY : update_every, RRDSET_TYPE_LINE
                );
                rd_load1 = rrddim_add(st, "load1", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                rd_load2 = rrddim_add(st, "load5", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                rd_load3 = rrddim_add(st, "load15", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
            } else
                rrdset_next(st);

            rrddim_set_by_pointer(st, rd_load1, (collected_number) ((double) sysload.ldavg[0] / sysload.fscale * 1000));
            rrddim_set_by_pointer(st, rd_load2, (collected_number) ((double) sysload.ldavg[1] / sysload.fscale * 1000));
            rrddim_set_by_pointer(st, rd_load3, (collected_number) ((double) sysload.ldavg[2] / sysload.fscale * 1000));
            rrdset_done(st);

            next_loadavg_dt = st->update_every * USEC_PER_SEC;
        }
    }
    else
        next_loadavg_dt -= dt;

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// vm.vmtotal

int do_vm_vmtotal(int update_every, usec_t dt) {
    (void)dt;
    static int do_all_processes = -1, do_processes = -1, do_committed = -1;

    if (unlikely(do_all_processes == -1)) {
        do_all_processes    = config_get_boolean("plugin:freebsd:vm.vmtotal", "enable total processes", 1);
        do_processes        = config_get_boolean("plugin:freebsd:vm.vmtotal", "processes running", 1);
        do_committed        = config_get_boolean("plugin:freebsd:vm.vmtotal", "committed memory", 1);
    }

    if (likely(do_all_processes | do_processes | do_committed)) {
        static int mib[2] = {0, 0};
        struct vmtotal vmtotal_data;

        if (unlikely(GETSYSCTL_SIMPLE("vm.vmtotal", mib, vmtotal_data))) {
            do_all_processes = 0;
            error("DISABLED: system.active_processes chart");
            do_processes = 0;
            error("DISABLED: system.processes chart");
            do_committed = 0;
            error("DISABLED: mem.committed chart");
            error("DISABLED: vm.vmtotal module");
            return 1;
        } else {

            // --------------------------------------------------------------------

            if (likely(do_all_processes)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("system",
                                                 "active_processes",
                                                 NULL,
                                                 "processes",
                                                 NULL,
                                                 "System Active Processes",
                                                 "processes",
                                                 750,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );
                    rd = rrddim_add(st, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd, (vmtotal_data.t_rq + vmtotal_data.t_dw + vmtotal_data.t_pw + vmtotal_data.t_sl + vmtotal_data.t_sw));
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_processes)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_running = NULL, *rd_blocked = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("system",
                                                 "processes",
                                                 NULL,
                                                 "processes",
                                                 NULL,
                                                 "System Processes",
                                                 "processes",
                                                 600,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_running = rrddim_add(st, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    rd_blocked = rrddim_add(st, "blocked", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_running, vmtotal_data.t_rq);
                rrddim_set_by_pointer(st, rd_blocked, (vmtotal_data.t_dw + vmtotal_data.t_pw));
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_committed)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("mem",
                                                 "committed",
                                                 NULL,
                                                 "system",
                                                 NULL,
                                                 "Committed (Allocated) Memory",
                                                 "MB",
                                                 5000,
                                                 update_every,
                                                 RRDSET_TYPE_AREA
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd = rrddim_add(st, "Committed_AS", NULL, system_pagesize, MEGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd, vmtotal_data.t_rm);
                rrdset_done(st);
            }
        }
    } else {
        error("DISABLED: vm.vmtotal module");
        return 1;
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// kern.cp_time

int do_kern_cp_time(int update_every, usec_t dt) {
    (void)dt;

    if (unlikely(CPUSTATES != 5)) {
        error("FREEBSD: There are %d CPU states (5 was expected)", CPUSTATES);
        error("DISABLED: system.cpu chart");
        error("DISABLED: kern.cp_time module");
        return 1;
    } else {
        static int mib[2] = {0, 0};
        long cp_time[CPUSTATES];

        if (unlikely(GETSYSCTL_SIMPLE("kern.cp_time", mib, cp_time))) {
            error("DISABLED: system.cpu chart");
            error("DISABLED: kern.cp_time module");
            return 1;
        } else {

            // --------------------------------------------------------------------

            static RRDSET *st = NULL;
            static RRDDIM *rd_nice = NULL, *rd_system = NULL, *rd_user = NULL, *rd_interrupt = NULL, *rd_idle = NULL;

            if (unlikely(!st)) {
                st = rrdset_create_localhost("system",
                                             "cpu",
                                             NULL,
                                             "cpu",
                                             "system.cpu",
                                             "Total CPU utilization",
                                             "percentage",
                                             100, update_every,
                                             RRDSET_TYPE_STACKED
                );

                rd_nice         = rrddim_add(st, "nice", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                rd_system       = rrddim_add(st, "system", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                rd_user         = rrddim_add(st, "user", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                rd_interrupt    = rrddim_add(st, "interrupt", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                rd_idle         = rrddim_add(st, "idle", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                rrddim_hide(st, "idle");
            }
            else rrdset_next(st);

            rrddim_set_by_pointer(st, rd_nice, cp_time[1]);
            rrddim_set_by_pointer(st, rd_system, cp_time[2]);
            rrddim_set_by_pointer(st, rd_user, cp_time[0]);
            rrddim_set_by_pointer(st, rd_interrupt, cp_time[3]);
            rrddim_set_by_pointer(st, rd_idle, cp_time[4]);
            rrdset_done(st);
        }
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// kern.cp_times

int do_kern_cp_times(int update_every, usec_t dt) {
    (void)dt;

    if (unlikely(CPUSTATES != 5)) {
        error("FREEBSD: There are %d CPU states (5 was expected)", CPUSTATES);
        error("DISABLED: cpu.cpuXX charts");
        error("DISABLED: kern.cp_times module");
        return 1;
    } else {
        static int mib[2] = {0, 0};
        long cp_time[CPUSTATES];
        static long *pcpu_cp_time = NULL;
        static int old_number_of_cpus = 0;

        if(unlikely(number_of_cpus != old_number_of_cpus))
            pcpu_cp_time = reallocz(pcpu_cp_time, sizeof(cp_time) * number_of_cpus);
        if (unlikely(GETSYSCTL_WSIZE("kern.cp_times", mib, pcpu_cp_time, sizeof(cp_time) * number_of_cpus))) {
            error("DISABLED: cpu.cpuXX charts");
            error("DISABLED: kern.cp_times module");
            return 1;
        } else {

            // --------------------------------------------------------------------

            int i;
            static struct cpu_chart {
                char cpuid[MAX_INT_DIGITS + 4];
                RRDSET *st;
                RRDDIM *rd_user;
                RRDDIM *rd_nice;
                RRDDIM *rd_system;
                RRDDIM *rd_interrupt;
                RRDDIM *rd_idle;
            } *all_cpu_charts = NULL;

            if(unlikely(number_of_cpus > old_number_of_cpus)) {
                all_cpu_charts = reallocz(all_cpu_charts, sizeof(struct cpu_chart) * number_of_cpus);
                memset(&all_cpu_charts[old_number_of_cpus], 0, sizeof(struct cpu_chart) * (number_of_cpus - old_number_of_cpus));
            }

            for (i = 0; i < number_of_cpus; i++) {
                if (unlikely(!all_cpu_charts[i].st)) {
                    snprintfz(all_cpu_charts[i].cpuid, MAX_INT_DIGITS, "cpu%d", i);
                    all_cpu_charts[i].st = rrdset_create_localhost("cpu",
                                                                   all_cpu_charts[i].cpuid,
                                                                   NULL,
                                                                   "utilization",
                                                                   "cpu.cpu",
                                                                   "Core utilization",
                                                                   "percentage",
                                                                   1000,
                                                                   update_every,
                                                                   RRDSET_TYPE_STACKED
                    );

                    all_cpu_charts[i].rd_nice       = rrddim_add(all_cpu_charts[i].st, "nice", NULL, 1, 1,
                                                                 RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    all_cpu_charts[i].rd_system     = rrddim_add(all_cpu_charts[i].st, "system", NULL, 1, 1,
                                                                 RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    all_cpu_charts[i].rd_user       = rrddim_add(all_cpu_charts[i].st, "user", NULL, 1, 1,
                                                                 RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    all_cpu_charts[i].rd_interrupt  = rrddim_add(all_cpu_charts[i].st, "interrupt", NULL, 1, 1,
                                                                 RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    all_cpu_charts[i].rd_idle       = rrddim_add(all_cpu_charts[i].st, "idle", NULL, 1, 1,
                                                                 RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_hide(all_cpu_charts[i].st, "idle");
                } else rrdset_next(all_cpu_charts[i].st);

                rrddim_set_by_pointer(all_cpu_charts[i].st, all_cpu_charts[i].rd_nice, pcpu_cp_time[i * 5 + 1]);
                rrddim_set_by_pointer(all_cpu_charts[i].st, all_cpu_charts[i].rd_system, pcpu_cp_time[i * 5 + 2]);
                rrddim_set_by_pointer(all_cpu_charts[i].st, all_cpu_charts[i].rd_user, pcpu_cp_time[i * 5 + 0]);
                rrddim_set_by_pointer(all_cpu_charts[i].st, all_cpu_charts[i].rd_interrupt, pcpu_cp_time[i * 5 + 3]);
                rrddim_set_by_pointer(all_cpu_charts[i].st, all_cpu_charts[i].rd_idle, pcpu_cp_time[i * 5 + 4]);
                rrdset_done(all_cpu_charts[i].st);
            }
        }

        old_number_of_cpus = number_of_cpus;
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// dev.cpu.temperature

int do_dev_cpu_temperature(int update_every, usec_t dt) {
    (void)dt;

    int i;
    static int *mib = NULL;
    static int *pcpu_temperature = NULL;
    static int old_number_of_cpus = 0;
    char char_mib[MAX_INT_DIGITS + 21];
    char char_rd[MAX_INT_DIGITS + 9];

    if (unlikely(number_of_cpus != old_number_of_cpus)) {
        pcpu_temperature = reallocz(pcpu_temperature, sizeof(int) * number_of_cpus);
        mib = reallocz(mib, sizeof(int) * number_of_cpus * 4);
        if (unlikely(number_of_cpus > old_number_of_cpus))
            memset(&mib[old_number_of_cpus * 4], 0, sizeof(RRDDIM) * (number_of_cpus - old_number_of_cpus));
    }
    for (i = 0; i < number_of_cpus; i++) {
        if (unlikely(!(mib[i * 4])))
            sprintf(char_mib, "dev.cpu.%d.temperature", i);
        if (unlikely(getsysctl_simple(char_mib, &mib[i * 4], 4, &pcpu_temperature[i], sizeof(int)))) {
            error("DISABLED: cpu.temperature chart");
            error("DISABLED: dev.cpu.temperature module");
            return 1;
        }
    }

    // --------------------------------------------------------------------

    static RRDSET *st;
    static RRDDIM **rd_pcpu_temperature;

    if (unlikely(number_of_cpus != old_number_of_cpus)) {
        rd_pcpu_temperature = reallocz(rd_pcpu_temperature, sizeof(RRDDIM) * number_of_cpus);
        if (unlikely(number_of_cpus > old_number_of_cpus))
            memset(&rd_pcpu_temperature[old_number_of_cpus], 0, sizeof(RRDDIM) * (number_of_cpus - old_number_of_cpus));
    }

    if (unlikely(!st)) {
        st = rrdset_create_localhost("cpu",
                                     "temperature",
                                     NULL,
                                     "temperature",
                                     "cpu.temperatute",
                                     "Core temperature",
                                     "degree",
                                     1050,
                                     update_every,
                                     RRDSET_TYPE_LINE
        );
    } else rrdset_next(st);

    for (i = 0; i < number_of_cpus; i++) {
        if (unlikely(!rd_pcpu_temperature[i])) {
            sprintf(char_rd, "cpu%d.temp", i);
            rd_pcpu_temperature[i] = rrddim_add(st, char_rd, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st, rd_pcpu_temperature[i], (collected_number) ((double)pcpu_temperature[i] / 10 - 273.15));
    }

    rrdset_done(st);

    old_number_of_cpus = number_of_cpus;

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// dev.cpu.0.freq

int do_dev_cpu_0_freq(int update_every, usec_t dt) {
    (void)dt;
    static int mib[4] = {0, 0, 0, 0};
    int cpufreq;

    if (unlikely(GETSYSCTL_SIMPLE("dev.cpu.0.freq", mib, cpufreq))) {
        error("DISABLED: cpu.scaling_cur_freq chart");
        error("DISABLED: dev.cpu.0.freq module");
        return 1;
    } else {

        // --------------------------------------------------------------------

        static RRDSET *st = NULL;
        static RRDDIM *rd = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost("cpu",
                                         "scaling_cur_freq",
                                         NULL,
                                         "cpufreq",
                                         NULL,
                                         "Current CPU Scaling Frequency",
                                         "MHz",
                                         5003,
                                         update_every,
                                         RRDSET_TYPE_LINE
            );

            rd = rrddim_add(st, "frequency", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd, cpufreq);
        rrdset_done(st);
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// hw.intrcnt

int do_hw_intcnt(int update_every, usec_t dt) {
    (void)dt;
    static int mib_hw_intrcnt[2] = {0, 0};
    size_t intrcnt_size = 0;
    unsigned long i;

    if (unlikely(GETSYSCTL_SIZE("hw.intrcnt", mib_hw_intrcnt, intrcnt_size))) {
        error("DISABLED: system.intr chart");
        error("DISABLED: system.interrupts chart");
        error("DISABLED: hw.intrcnt module");
        return 1;
    } else {
        unsigned long nintr = 0;
        static unsigned long old_nintr = 0;
        static unsigned long *intrcnt = NULL;
        unsigned long long totalintr = 0;

        nintr = intrcnt_size / sizeof(u_long);
        if (unlikely(nintr != old_nintr))
            intrcnt = reallocz(intrcnt, nintr * sizeof(u_long));
        if (unlikely(GETSYSCTL_WSIZE("hw.intrcnt", mib_hw_intrcnt, intrcnt, nintr * sizeof(u_long)))) {
            error("DISABLED: system.intr chart");
            error("DISABLED: system.interrupts chart");
            error("DISABLED: hw.intrcnt module");
            return 1;
        } else {
            for (i = 0; i < nintr; i++)
                totalintr += intrcnt[i];

            // --------------------------------------------------------------------

            static RRDSET *st_intr = NULL;
            static RRDDIM *rd_intr = NULL;

            if (unlikely(!st_intr)) {
                st_intr = rrdset_create_localhost("system",
                                                  "intr",
                                                  NULL,
                                                  "interrupts",
                                                  NULL,
                                                  "Total Hardware Interrupts",
                                                  "interrupts/s",
                                                  900,
                                                  update_every,
                                                  RRDSET_TYPE_LINE
                );
                rrdset_flag_set(st_intr, RRDSET_FLAG_DETAIL);

                rd_intr = rrddim_add(st_intr, "interrupts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            } else
                rrdset_next(st_intr);

            rrddim_set_by_pointer(st_intr, rd_intr, totalintr);
            rrdset_done(st_intr);

            // --------------------------------------------------------------------

            size_t size;
            static int mib_hw_intrnames[2] = {0, 0};
            static char *intrnames = NULL;

            size = nintr * (MAXCOMLEN + 1);
            if (unlikely(nintr != old_nintr))
                intrnames = reallocz(intrnames, size);
            if (unlikely(GETSYSCTL_WSIZE("hw.intrnames", mib_hw_intrnames, intrnames, size))) {
                error("DISABLED: system.intr chart");
                error("DISABLED: system.interrupts chart");
                error("DISABLED: hw.intrcnt module");
                return 1;
            } else {

                // --------------------------------------------------------------------

                static RRDSET *st_interrupts = NULL;
                RRDDIM *rd_interrupts = NULL;
                void *p;

                if (unlikely(!st_interrupts))
                    st_interrupts = rrdset_create_localhost("system",
                                                            "interrupts",
                                                            NULL,
                                                            "interrupts",
                                                            NULL,
                                                            "System interrupts",
                                                            "interrupts/s",
                                                            1000,
                                                            update_every,
                                                            RRDSET_TYPE_STACKED
                    );
                else
                    rrdset_next(st_interrupts);

                for (i = 0; i < nintr; i++) {
                    p = intrnames + i * (MAXCOMLEN + 1);
                    if (unlikely((intrcnt[i] != 0) && (*(char *) p != 0))) {
                        rd_interrupts = rrddim_find(st_interrupts, p);
                        if (unlikely(!rd_interrupts))
                            rd_interrupts = rrddim_add(st_interrupts, p, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rrddim_set_by_pointer(st_interrupts, rd_interrupts, intrcnt[i]);
                    }
                }
                rrdset_done(st_interrupts);
            }
        }

        old_nintr = nintr;
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// vm.stats.sys.v_intr

int do_vm_stats_sys_v_intr(int update_every, usec_t dt) {
    (void)dt;
    static int mib[4] = {0, 0, 0, 0};
    u_int int_number;

    if (unlikely(GETSYSCTL_SIMPLE("vm.stats.sys.v_intr", mib, int_number))) {
        error("DISABLED: system.dev_intr chart");
        error("DISABLED: vm.stats.sys.v_intr module");
        return 1;
    } else {

        // --------------------------------------------------------------------

        static RRDSET *st = NULL;
        static RRDDIM *rd = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost("system",
                                         "dev_intr",
                                         NULL,
                                         "interrupts",
                                         NULL,
                                         "Device Interrupts",
                                         "interrupts/s",
                                         1000,
                                         update_every,
                                         RRDSET_TYPE_LINE
            );

            rd = rrddim_add(st, "interrupts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd, int_number);
        rrdset_done(st);
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// vm.stats.sys.v_soft

int do_vm_stats_sys_v_soft(int update_every, usec_t dt) {
    (void)dt;
    static int mib[4] = {0, 0, 0, 0};
    u_int soft_intr_number;

    if (unlikely(GETSYSCTL_SIMPLE("vm.stats.sys.v_soft", mib, soft_intr_number))) {
        error("DISABLED: system.dev_intr chart");
        error("DISABLED: vm.stats.sys.v_soft module");
        return 1;
    } else {

        // --------------------------------------------------------------------

        static RRDSET *st = NULL;
        static RRDDIM *rd = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost("system",
                                         "soft_intr",
                                         NULL,
                                         "interrupts",
                                         NULL,
                                         "Software Interrupts",
                                         "interrupts/s",
                                         1100,
                                         update_every,
                                         RRDSET_TYPE_LINE
            );

            rd = rrddim_add(st, "interrupts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd, soft_intr_number);
        rrdset_done(st);
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// vm.stats.sys.v_swtch

int do_vm_stats_sys_v_swtch(int update_every, usec_t dt) {
    (void)dt;
    static int mib[4] = {0, 0, 0, 0};
    u_int ctxt_number;

    if (unlikely(GETSYSCTL_SIMPLE("vm.stats.sys.v_swtch", mib, ctxt_number))) {
        error("DISABLED: system.ctxt chart");
        error("DISABLED: vm.stats.sys.v_swtch module");
        return 1;
    } else {

        // --------------------------------------------------------------------

        static RRDSET *st = NULL;
        static RRDDIM *rd = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost("system",
                                         "ctxt",
                                         NULL,
                                         "processes",
                                         NULL,
                                         "CPU Context Switches",
                                         "context switches/s",
                                         800,
                                         update_every,
                                         RRDSET_TYPE_LINE
            );

            rd = rrddim_add(st, "switches", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd, ctxt_number);
        rrdset_done(st);
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// vm.stats.vm.v_forks

int do_vm_stats_sys_v_forks(int update_every, usec_t dt) {
    (void)dt;
    static int mib[4] = {0, 0, 0, 0};
    u_int forks_number;

    if (unlikely(GETSYSCTL_SIMPLE("vm.stats.vm.v_forks", mib, forks_number))) {
        error("DISABLED: system.forks chart");
        error("DISABLED: vm.stats.sys.v_swtch module");
        return 1;
    } else {

        // --------------------------------------------------------------------

        static RRDSET *st = NULL;
        static RRDDIM *rd = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost("system",
                                         "forks",
                                         NULL,
                                         "processes",
                                         NULL,
                                         "Started Processes",
                                         "processes/s",
                                         700,
                                         update_every,
                                         RRDSET_TYPE_LINE
            );

            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd = rrddim_add(st, "started", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd, forks_number);
        rrdset_done(st);
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// vm.swap_info

int do_vm_swap_info(int update_every, usec_t dt) {
    (void)dt;
    static int mib[3] = {0, 0, 0};

    if (unlikely(getsysctl_mib("vm.swap_info", mib, 2))) {
        error("DISABLED: system.swap chart");
        error("DISABLED: vm.swap_info module");
        return 1;
    } else {
        int i;
        struct xswdev xsw;
        struct total_xsw {
            collected_number bytes_used;
            collected_number bytes_total;
        } total_xsw = {0, 0};

        for (i = 0; ; i++) {
            size_t size;

            mib[2] = i;
            size = sizeof(xsw);
            if (unlikely(sysctl(mib, 3, &xsw, &size, NULL, 0) == -1 )) {
                if (unlikely(errno != ENOENT)) {
                    error("FREEBSD: sysctl(%s...) failed: %s", "vm.swap_info", strerror(errno));
                    error("DISABLED: system.swap chart");
                    error("DISABLED: vm.swap_info module");
                    return 1;
                } else {
                    if (unlikely(size != sizeof(xsw))) {
                        error("FREEBSD: sysctl(%s...) expected %lu, got %lu", "vm.swap_info", (unsigned long)sizeof(xsw), (unsigned long)size);
                        error("DISABLED: system.swap chart");
                        error("DISABLED: vm.swap_info module");
                        return 1;
                    } else break;
                }
            }
            total_xsw.bytes_used += xsw.xsw_used;
            total_xsw.bytes_total += xsw.xsw_nblks;
        }

        // --------------------------------------------------------------------

        static RRDSET *st = NULL;
        static RRDDIM *rd_free = NULL, *rd_used = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost("system",
                                         "swap",
                                         NULL,
                                         "swap",
                                         NULL,
                                         "System Swap",
                                         "MB",
                                         201,
                                         update_every,
                                         RRDSET_TYPE_STACKED
            );

            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_free = rrddim_add(st, "free",    NULL, system_pagesize, MEGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
            rd_used = rrddim_add(st, "used",    NULL, system_pagesize, MEGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_free, total_xsw.bytes_total - total_xsw.bytes_used);
        rrddim_set_by_pointer(st, rd_used, total_xsw.bytes_used);
        rrdset_done(st);
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// system.ram

int do_system_ram(int update_every, usec_t dt) {
    (void)dt;
    static int mib_active_count[4] = {0, 0, 0, 0}, mib_inactive_count[4] = {0, 0, 0, 0}, mib_wire_count[4] = {0, 0, 0, 0},
               mib_cache_count[4] = {0, 0, 0, 0}, mib_vfs_bufspace[2] = {0, 0}, mib_free_count[4] = {0, 0, 0, 0};
    vmmeter_t vmmeter_data;
    int vfs_bufspace_count;

    if (unlikely(GETSYSCTL_SIMPLE("vm.stats.vm.v_active_count",   mib_active_count,   vmmeter_data.v_active_count) ||
                 GETSYSCTL_SIMPLE("vm.stats.vm.v_inactive_count", mib_inactive_count, vmmeter_data.v_inactive_count) ||
                 GETSYSCTL_SIMPLE("vm.stats.vm.v_wire_count",     mib_wire_count,     vmmeter_data.v_wire_count) ||
#if __FreeBSD_version < 1200016
                 GETSYSCTL_SIMPLE("vm.stats.vm.v_cache_count",    mib_cache_count,    vmmeter_data.v_cache_count) ||
#endif
                 GETSYSCTL_SIMPLE("vfs.bufspace",                 mib_vfs_bufspace,     vfs_bufspace_count) ||
                 GETSYSCTL_SIMPLE("vm.stats.vm.v_free_count",     mib_free_count,     vmmeter_data.v_free_count))) {
        error("DISABLED: system.ram chart");
        error("DISABLED: System.ram module");
        return 1;
    } else {

        // --------------------------------------------------------------------

        static RRDSET *st = NULL;
        static RRDDIM *rd_free = NULL, *rd_active = NULL, *rd_inactive = NULL,
                      *rd_wired = NULL, *rd_cache = NULL, *rd_buffers = NULL;

        st = rrdset_find_localhost("system.ram");
        if (unlikely(!st)) {
            st = rrdset_create_localhost("system",
                                         "ram",
                                         NULL,
                                         "ram",
                                         NULL,
                                         "System RAM",
                                         "MB",
                                         200,
                                         update_every,
                                         RRDSET_TYPE_STACKED
            );

            rd_free     = rrddim_add(st, "free",     NULL, system_pagesize, MEGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
            rd_active   = rrddim_add(st, "active",   NULL, system_pagesize, MEGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
            rd_inactive = rrddim_add(st, "inactive", NULL, system_pagesize, MEGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
            rd_wired    = rrddim_add(st, "wired",    NULL, system_pagesize, MEGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
#if __FreeBSD_version < 1200016
            rd_cache    = rrddim_add(st, "cache",    NULL, system_pagesize, MEGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
#endif
            rd_buffers  = rrddim_add(st, "buffers",  NULL, 1, MEGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_free,     vmmeter_data.v_free_count);
        rrddim_set_by_pointer(st, rd_active,   vmmeter_data.v_active_count);
        rrddim_set_by_pointer(st, rd_inactive, vmmeter_data.v_inactive_count);
        rrddim_set_by_pointer(st, rd_wired,    vmmeter_data.v_wire_count);
#if __FreeBSD_version < 1200016
        rrddim_set_by_pointer(st, rd_cache,    vmmeter_data.v_cache_count);
#endif
        rrddim_set_by_pointer(st, rd_buffers,  vfs_bufspace_count);
        rrdset_done(st);
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// vm.stats.vm.v_swappgs

int do_vm_stats_sys_v_swappgs(int update_every, usec_t dt) {
    (void)dt;
    static int mib_swappgsin[4] = {0, 0, 0, 0}, mib_swappgsout[4] = {0, 0, 0, 0};
    vmmeter_t vmmeter_data;

    if (unlikely(GETSYSCTL_SIMPLE("vm.stats.vm.v_swappgsin", mib_swappgsin, vmmeter_data.v_swappgsin) ||
                 GETSYSCTL_SIMPLE("vm.stats.vm.v_swappgsout", mib_swappgsout, vmmeter_data.v_swappgsout))) {
        error("DISABLED: system.swapio chart");
        error("DISABLED: vm.stats.vm.v_swappgs module");
        return 1;
    } else {

        // --------------------------------------------------------------------

        static RRDSET *st = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost("system",
                                         "swapio",
                                         NULL,
                                         "swap",
                                         NULL,
                                         "Swap I/O",
                                         "kilobytes/s",
                                         250,
                                         update_every,
                                         RRDSET_TYPE_AREA
            );

            rd_in = rrddim_add(st, "in",  NULL, system_pagesize, KILO_FACTOR, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st, "out", NULL, -system_pagesize, KILO_FACTOR, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_in, vmmeter_data.v_swappgsin);
        rrddim_set_by_pointer(st, rd_out, vmmeter_data.v_swappgsout);
        rrdset_done(st);
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// vm.stats.vm.v_pgfaults

int do_vm_stats_sys_v_pgfaults(int update_every, usec_t dt) {
    (void)dt;
    static int mib_vm_faults[4] = {0, 0, 0, 0}, mib_io_faults[4] = {0, 0, 0, 0}, mib_cow_faults[4] = {0, 0, 0, 0},
               mib_cow_optim[4] = {0, 0, 0, 0}, mib_intrans[4] = {0, 0, 0, 0};
    vmmeter_t vmmeter_data;

    if (unlikely(GETSYSCTL_SIMPLE("vm.stats.vm.v_vm_faults",  mib_vm_faults,  vmmeter_data.v_vm_faults) ||
                 GETSYSCTL_SIMPLE("vm.stats.vm.v_io_faults",  mib_io_faults,  vmmeter_data.v_io_faults) ||
                 GETSYSCTL_SIMPLE("vm.stats.vm.v_cow_faults", mib_cow_faults, vmmeter_data.v_cow_faults) ||
                 GETSYSCTL_SIMPLE("vm.stats.vm.v_cow_optim",  mib_cow_optim,  vmmeter_data.v_cow_optim) ||
                 GETSYSCTL_SIMPLE("vm.stats.vm.v_intrans",    mib_intrans,    vmmeter_data.v_intrans))) {
        error("DISABLED: mem.pgfaults chart");
        error("DISABLED: vm.stats.vm.v_pgfaults module");
        return 1;
    } else {

        // --------------------------------------------------------------------

        static RRDSET *st = NULL;
        static RRDDIM *rd_memory = NULL, *rd_io_requiring = NULL, *rd_cow = NULL,
                      *rd_cow_optimized = NULL, *rd_in_transit = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost("mem",
                                         "pgfaults",
                                         NULL,
                                         "system",
                                         NULL,
                                         "Memory Page Faults",
                                         "page faults/s",
                                         500,
                                         update_every,
                                         RRDSET_TYPE_LINE
            );

            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_memory        = rrddim_add(st, "memory",        NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_io_requiring  = rrddim_add(st, "io_requiring",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cow           = rrddim_add(st, "cow",           NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cow_optimized = rrddim_add(st, "cow_optimized", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_in_transit    = rrddim_add(st, "in_transit",    NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_memory,        vmmeter_data.v_vm_faults);
        rrddim_set_by_pointer(st, rd_io_requiring,  vmmeter_data.v_io_faults);
        rrddim_set_by_pointer(st, rd_cow,           vmmeter_data.v_cow_faults);
        rrddim_set_by_pointer(st, rd_cow_optimized, vmmeter_data.v_cow_optim);
        rrddim_set_by_pointer(st, rd_in_transit,    vmmeter_data.v_intrans);
        rrdset_done(st);
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// kern.ipc.sem

int do_kern_ipc_sem(int update_every, usec_t dt) {
    (void)dt;
    static int mib_semmni[3] = {0, 0, 0}, mib_sema[3] = {0, 0, 0};
    struct ipc_sem {
        int semmni;
        collected_number sets;
        collected_number semaphores;
    } ipc_sem = {0, 0, 0};

    if (unlikely(GETSYSCTL_SIMPLE("kern.ipc.semmni", mib_semmni, ipc_sem.semmni))) {
        error("DISABLED: system.ipc_semaphores chart");
        error("DISABLED: system.ipc_semaphore_arrays chart");
        error("DISABLED: kern.ipc.sem module");
        return 1;
    } else {
        static struct semid_kernel *ipc_sem_data = NULL;
        static int old_semmni = 0;

        if (unlikely(ipc_sem.semmni != old_semmni)) {
            ipc_sem_data = reallocz(ipc_sem_data, sizeof(struct semid_kernel) * ipc_sem.semmni);
            old_semmni = ipc_sem.semmni;
        }
        if (unlikely(GETSYSCTL_WSIZE("kern.ipc.sema", mib_sema, ipc_sem_data, sizeof(struct semid_kernel) * ipc_sem.semmni))) {
            error("DISABLED: system.ipc_semaphores chart");
            error("DISABLED: system.ipc_semaphore_arrays chart");
            error("DISABLED: kern.ipc.sem module");
            return 1;
        } else {
            int i;

            for (i = 0; i < ipc_sem.semmni; i++) {
                if (unlikely(ipc_sem_data[i].u.sem_perm.mode & SEM_ALLOC)) {
                    ipc_sem.sets += 1;
                    ipc_sem.semaphores += ipc_sem_data[i].u.sem_nsems;
                }
            }

            // --------------------------------------------------------------------

            static RRDSET *st_semaphores = NULL, *st_semaphore_arrays = NULL;
            static RRDDIM *rd_semaphores = NULL, *rd_semaphore_arrays = NULL;

            if (unlikely(!st_semaphores)) {
                st_semaphores = rrdset_create_localhost("system",
                                                        "ipc_semaphores",
                                                        NULL,
                                                        "ipc semaphores",
                                                        NULL,
                                                        "IPC Semaphores",
                                                        "semaphores",
                                                        1000,
                                                        update_every,
                                                        RRDSET_TYPE_AREA
                );

                rd_semaphores = rrddim_add(st_semaphores, "semaphores", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(st_semaphores);

            rrddim_set_by_pointer(st_semaphores, rd_semaphores, ipc_sem.semaphores);
            rrdset_done(st_semaphores);

            // --------------------------------------------------------------------

            if (unlikely(!st_semaphore_arrays)) {
                st_semaphore_arrays = rrdset_create_localhost("system",
                                                              "ipc_semaphore_arrays",
                                                              NULL,
                                                              "ipc semaphores",
                                                              NULL,
                                                              "IPC Semaphore Arrays",
                                                              "arrays",
                                                              1000,
                                                              update_every,
                                                              RRDSET_TYPE_AREA
                );

                rd_semaphore_arrays = rrddim_add(st_semaphore_arrays, "arrays", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(st_semaphore_arrays);

            rrddim_set_by_pointer(st_semaphore_arrays, rd_semaphore_arrays, ipc_sem.sets);
            rrdset_done(st_semaphore_arrays);
        }
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// kern.ipc.shm

int do_kern_ipc_shm(int update_every, usec_t dt) {
    (void)dt;
    static int mib_shmmni[3] = {0, 0, 0}, mib_shmsegs[3] = {0, 0, 0};
    struct ipc_shm {
        u_long shmmni;
        collected_number segs;
        collected_number segsize;
    } ipc_shm = {0, 0, 0};

    if (unlikely(GETSYSCTL_SIMPLE("kern.ipc.shmmni", mib_shmmni, ipc_shm.shmmni))) {
        error("DISABLED: system.ipc_shared_mem_segs chart");
        error("DISABLED: system.ipc_shared_mem_size chart");
        error("DISABLED: kern.ipc.shmmodule");
        return 1;
    } else {
        static struct shmid_kernel *ipc_shm_data = NULL;
        static u_long old_shmmni = 0;

        if (unlikely(ipc_shm.shmmni != old_shmmni)) {
            ipc_shm_data = reallocz(ipc_shm_data, sizeof(struct shmid_kernel) * ipc_shm.shmmni);
            old_shmmni = ipc_shm.shmmni;
        }
        if (unlikely(
                GETSYSCTL_WSIZE("kern.ipc.shmsegs", mib_shmsegs, ipc_shm_data, sizeof(struct shmid_kernel) * ipc_shm.shmmni))) {
            error("DISABLED: system.ipc_shared_mem_segs chart");
            error("DISABLED: system.ipc_shared_mem_size chart");
            error("DISABLED: kern.ipc.shmmodule");
            return 1;
        } else {
            unsigned long i;

            for (i = 0; i < ipc_shm.shmmni; i++) {
                if (unlikely(ipc_shm_data[i].u.shm_perm.mode & 0x0800)) {
                    ipc_shm.segs += 1;
                    ipc_shm.segsize += ipc_shm_data[i].u.shm_segsz;
                }
            }

            // --------------------------------------------------------------------

            static RRDSET *st_segs = NULL, *st_size = NULL;
            static RRDDIM *rd_segments = NULL, *rd_allocated = NULL;

            if (unlikely(!st_segs)) {
                st_segs = rrdset_create_localhost("system",
                                             "ipc_shared_mem_segs",
                                             NULL,
                                             "ipc shared memory",
                                             NULL,
                                             "IPC Shared Memory Segments",
                                             "segments",
                                             1000,
                                             update_every,
                                             RRDSET_TYPE_AREA
                );

                rd_segments = rrddim_add(st_segs, "segments", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(st_segs);

            rrddim_set_by_pointer(st_segs, rd_segments, ipc_shm.segs);
            rrdset_done(st_segs);

            // --------------------------------------------------------------------

            if (unlikely(!st_size)) {
                st_size = rrdset_create_localhost("system",
                                             "ipc_shared_mem_size",
                                             NULL,
                                             "ipc shared memory",
                                             NULL,
                                             "IPC Shared Memory Segments Size",
                                             "kilobytes",
                                             1000,
                                             update_every,
                                             RRDSET_TYPE_AREA
                );

                rd_allocated = rrddim_add(st_size, "allocated", NULL, 1, KILO_FACTOR, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(st_size);

            rrddim_set_by_pointer(st_size, rd_allocated, ipc_shm.segsize);
            rrdset_done(st_size);
        }
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// kern.ipc.msq

int do_kern_ipc_msq(int update_every, usec_t dt) {
    (void)dt;
    static int mib_msgmni[3] = {0, 0, 0}, mib_msqids[3] = {0, 0, 0};
    struct ipc_msq {
        int msgmni;
        collected_number queues;
        collected_number messages;
        collected_number usedsize;
        collected_number allocsize;
    } ipc_msq = {0, 0, 0, 0, 0};

    if (unlikely(GETSYSCTL_SIMPLE("kern.ipc.msgmni", mib_msgmni, ipc_msq.msgmni))) {
        error("DISABLED: system.ipc_msq_queues chart");
        error("DISABLED: system.ipc_msq_messages chart");
        error("DISABLED: system.ipc_msq_size chart");
        error("DISABLED: kern.ipc.msg module");
        return 1;
    } else {
        static struct msqid_kernel *ipc_msq_data = NULL;
        static int old_msgmni = 0;

        if (unlikely(ipc_msq.msgmni != old_msgmni)) {
            ipc_msq_data = reallocz(ipc_msq_data, sizeof(struct msqid_kernel) * ipc_msq.msgmni);
            old_msgmni = ipc_msq.msgmni;
        }
        if (unlikely(
                GETSYSCTL_WSIZE("kern.ipc.msqids", mib_msqids, ipc_msq_data, sizeof(struct msqid_kernel) * ipc_msq.msgmni))) {
            error("DISABLED: system.ipc_msq_queues chart");
            error("DISABLED: system.ipc_msq_messages chart");
            error("DISABLED: system.ipc_msq_size chart");
            error("DISABLED: kern.ipc.msg module");
            return 1;
        } else {
            int i;

            for (i = 0; i < ipc_msq.msgmni; i++) {
                if (unlikely(ipc_msq_data[i].u.msg_qbytes != 0)) {
                    ipc_msq.queues += 1;
                    ipc_msq.messages += ipc_msq_data[i].u.msg_qnum;
                    ipc_msq.usedsize += ipc_msq_data[i].u.msg_cbytes;
                    ipc_msq.allocsize += ipc_msq_data[i].u.msg_qbytes;
                }
            }

            // --------------------------------------------------------------------

            static RRDSET *st_queues = NULL, *st_messages = NULL, *st_size = NULL;
            static RRDDIM *rd_queues = NULL, *rd_messages = NULL, *rd_allocated = NULL, *rd_used = NULL;

            if (unlikely(!st_queues)) {
                st_queues = rrdset_create_localhost("system",
                                                    "ipc_msq_queues",
                                                    NULL,
                                                    "ipc message queues",
                                                    NULL,
                                                    "Number of IPC Message Queues",
                                                    "queues",
                                                    990,
                                                    update_every,
                                                    RRDSET_TYPE_AREA
                );

                rd_queues = rrddim_add(st_queues, "queues", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(st_queues);

            rrddim_set_by_pointer(st_queues, rd_queues, ipc_msq.queues);
            rrdset_done(st_queues);

            // --------------------------------------------------------------------

            if (unlikely(!st_messages)) {
                st_messages = rrdset_create_localhost("system",
                                                      "ipc_msq_messages",
                                                      NULL,
                                                      "ipc message queues",
                                                      NULL,
                                                      "Number of Messages in IPC Message Queues",
                                                      "messages",
                                                      1000,
                                                      update_every,
                                                      RRDSET_TYPE_AREA
                );

                rd_messages = rrddim_add(st_messages, "messages", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(st_messages);

            rrddim_set_by_pointer(st_messages, rd_messages, ipc_msq.messages);
            rrdset_done(st_messages);

            // --------------------------------------------------------------------

            if (unlikely(!st_size)) {
                st_size = rrdset_create_localhost("system",
                                             "ipc_msq_size",
                                             NULL,
                                             "ipc message queues",
                                             NULL,
                                             "Size of IPC Message Queues",
                                             "bytes",
                                             1100,
                                             update_every,
                                             RRDSET_TYPE_LINE
                );

                rd_allocated = rrddim_add(st_size, "allocated", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_used = rrddim_add(st_size, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(st_size);

            rrddim_set_by_pointer(st_size, rd_allocated, ipc_msq.allocsize);
            rrddim_set_by_pointer(st_size, rd_used, ipc_msq.usedsize);
            rrdset_done(st_size);
        }
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// uptime

int do_uptime(int update_every, usec_t dt) {
    (void)dt;
    struct timespec up_time;

    clock_gettime(CLOCK_UPTIME, &up_time);

    // --------------------------------------------------------------------

    static RRDSET *st = NULL;
    static RRDDIM *rd = NULL;

    if(unlikely(!st)) {
        st = rrdset_create_localhost("system",
                                     "uptime",
                                     NULL,
                                     "uptime",
                                     NULL,
                                     "System Uptime",
                                     "seconds",
                                     1000,
                                     update_every,
                                     RRDSET_TYPE_LINE
        );

        rd = rrddim_add(st, "uptime", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    else rrdset_next(st);

    rrddim_set_by_pointer(st, rd, up_time.tv_sec);
    rrdset_done(st);

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// net.isr

int do_net_isr(int update_every, usec_t dt) {
    (void)dt;
    static int do_netisr = -1, do_netisr_per_core = -1;

    if (unlikely(do_netisr == -1)) {
        do_netisr =          config_get_boolean("plugin:freebsd:net.isr", "netisr",          1);
        do_netisr_per_core = config_get_boolean("plugin:freebsd:net.isr", "netisr per core", 1);
    }

    static int mib_workstream[3] = {0, 0, 0}, mib_work[3] = {0, 0, 0};
    int common_error = 0;
    size_t netisr_workstream_size = 0, netisr_work_size = 0;
    unsigned long num_netisr_workstreams = 0, num_netisr_works = 0;
    static struct sysctl_netisr_workstream *netisr_workstream = NULL;
    static struct sysctl_netisr_work *netisr_work = NULL;
    static struct netisr_stats {
        collected_number dispatched;
        collected_number hybrid_dispatched;
        collected_number qdrops;
        collected_number queued;
    } *netisr_stats = NULL;

    if (likely(do_netisr || do_netisr_per_core)) {
        if (unlikely(GETSYSCTL_SIZE("net.isr.workstream", mib_workstream, netisr_workstream_size))) {
            common_error = 1;
        } else if (unlikely(GETSYSCTL_SIZE("net.isr.work", mib_work, netisr_work_size))) {
            common_error = 1;
        } else {
            static size_t old_netisr_workstream_size = 0;

            num_netisr_workstreams = netisr_workstream_size / sizeof(struct sysctl_netisr_workstream);
            if (unlikely(netisr_workstream_size != old_netisr_workstream_size)) {
                netisr_workstream = reallocz(netisr_workstream,
                                             num_netisr_workstreams * sizeof(struct sysctl_netisr_workstream));
                old_netisr_workstream_size = netisr_workstream_size;
            }
            if (unlikely(GETSYSCTL_WSIZE("net.isr.workstream", mib_workstream, netisr_workstream,
                                           num_netisr_workstreams * sizeof(struct sysctl_netisr_workstream)))){
                common_error = 1;
            } else {
                static size_t old_netisr_work_size = 0;

                num_netisr_works = netisr_work_size / sizeof(struct sysctl_netisr_work);
                if (unlikely(netisr_work_size != old_netisr_work_size)) {
                    netisr_work = reallocz(netisr_work, num_netisr_works * sizeof(struct sysctl_netisr_work));
                    old_netisr_work_size = netisr_work_size;
                }
                if (unlikely(GETSYSCTL_WSIZE("net.isr.work", mib_work, netisr_work,
                                               num_netisr_works * sizeof(struct sysctl_netisr_work)))){
                    common_error = 1;
                }
            }
        }
        if (unlikely(common_error)) {
            do_netisr = 0;
            error("DISABLED: system.softnet_stat chart");
            do_netisr_per_core = 0;
            error("DISABLED: system.cpuX_softnet_stat chart");
            common_error = 0;
            error("DISABLED: net.isr module");
            return 1;
        } else {
            unsigned long i, n;
            int j;
            static int old_number_of_cpus = 0;

            if (unlikely(number_of_cpus != old_number_of_cpus)) {
                netisr_stats = reallocz(netisr_stats, (number_of_cpus + 1) * sizeof(struct netisr_stats));
                old_number_of_cpus = number_of_cpus;
            }
            memset(netisr_stats, 0, (number_of_cpus + 1) * sizeof(struct netisr_stats));
            for (i = 0; i < num_netisr_workstreams; i++) {
                for (n = 0; n < num_netisr_works; n++) {
                    if (netisr_workstream[i].snws_wsid == netisr_work[n].snw_wsid) {
                        netisr_stats[netisr_workstream[i].snws_cpu].dispatched += netisr_work[n].snw_dispatched;
                        netisr_stats[netisr_workstream[i].snws_cpu].hybrid_dispatched += netisr_work[n].snw_hybrid_dispatched;
                        netisr_stats[netisr_workstream[i].snws_cpu].qdrops += netisr_work[n].snw_qdrops;
                        netisr_stats[netisr_workstream[i].snws_cpu].queued += netisr_work[n].snw_queued;
                    }
                }
            }
            for (j = 0; j < number_of_cpus; j++) {
                netisr_stats[number_of_cpus].dispatched += netisr_stats[j].dispatched;
                netisr_stats[number_of_cpus].hybrid_dispatched += netisr_stats[j].hybrid_dispatched;
                netisr_stats[number_of_cpus].qdrops += netisr_stats[j].qdrops;
                netisr_stats[number_of_cpus].queued += netisr_stats[j].queued;
            }
        }
    } else {
        error("DISABLED: net.isr module");
        return 1;
    }

    // --------------------------------------------------------------------

    if (likely(do_netisr)) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_dispatched = NULL, *rd_hybrid_dispatched = NULL, *rd_qdrops = NULL, *rd_queued = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost("system",
                                         "softnet_stat",
                                         NULL,
                                         "softnet_stat",
                                         NULL,
                                         "System softnet_stat",
                                         "events/s",
                                         955,
                                         update_every,
                                         RRDSET_TYPE_LINE
            );

            rd_dispatched        = rrddim_add(st, "dispatched",        NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_hybrid_dispatched = rrddim_add(st, "hybrid_dispatched", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_qdrops            = rrddim_add(st, "qdrops",            NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_queued            = rrddim_add(st, "queued",            NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_dispatched,        netisr_stats[number_of_cpus].dispatched);
        rrddim_set_by_pointer(st, rd_hybrid_dispatched, netisr_stats[number_of_cpus].hybrid_dispatched);
        rrddim_set_by_pointer(st, rd_qdrops,            netisr_stats[number_of_cpus].qdrops);
        rrddim_set_by_pointer(st, rd_queued,            netisr_stats[number_of_cpus].queued);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if (likely(do_netisr_per_core)) {
        static struct softnet_chart {
            char netisr_cpuid[MAX_INT_DIGITS + 17];
            RRDSET *st;
            RRDDIM *rd_dispatched;
            RRDDIM *rd_hybrid_dispatched;
            RRDDIM *rd_qdrops;
            RRDDIM *rd_queued;
        } *all_softnet_charts = NULL;
        static int old_number_of_cpus = 0;
        int i;

        if(unlikely(number_of_cpus > old_number_of_cpus)) {
            all_softnet_charts = reallocz(all_softnet_charts, sizeof(struct softnet_chart) * number_of_cpus);
            memset(&all_softnet_charts[old_number_of_cpus], 0, sizeof(struct softnet_chart) * (number_of_cpus - old_number_of_cpus));
            old_number_of_cpus = number_of_cpus;
        }

        for (i = 0; i < number_of_cpus ;i++) {
            snprintfz(all_softnet_charts[i].netisr_cpuid, MAX_INT_DIGITS + 17, "cpu%d_softnet_stat", i);

            if (unlikely(!all_softnet_charts[i].st)) {
                all_softnet_charts[i].st = rrdset_create_localhost("cpu",
                                                                   all_softnet_charts[i].netisr_cpuid,
                                                                   NULL,
                                                                   "softnet_stat",
                                                                   NULL,
                                                                   "Per CPU netisr statistics",
                                                                   "events/s",
                                                                   1101 + i,
                                                                   update_every,
                                                                   RRDSET_TYPE_LINE
                );

                all_softnet_charts[i].rd_dispatched        = rrddim_add(all_softnet_charts[i].st, "dispatched",
                                                                NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                all_softnet_charts[i].rd_hybrid_dispatched = rrddim_add(all_softnet_charts[i].st, "hybrid_dispatched",
                                                                NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                all_softnet_charts[i].rd_qdrops            = rrddim_add(all_softnet_charts[i].st, "qdrops",
                                                                NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                all_softnet_charts[i].rd_queued            = rrddim_add(all_softnet_charts[i].st, "queued",
                                                                NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(all_softnet_charts[i].st);

            rrddim_set_by_pointer(all_softnet_charts[i].st, all_softnet_charts[i].rd_dispatched,
                                  netisr_stats[i].dispatched);
            rrddim_set_by_pointer(all_softnet_charts[i].st, all_softnet_charts[i].rd_hybrid_dispatched,
                                  netisr_stats[i].hybrid_dispatched);
            rrddim_set_by_pointer(all_softnet_charts[i].st, all_softnet_charts[i].rd_qdrops,
                                  netisr_stats[i].qdrops);
            rrddim_set_by_pointer(all_softnet_charts[i].st, all_softnet_charts[i].rd_queued,
                                  netisr_stats[i].queued);
            rrdset_done(all_softnet_charts[i].st);
        }
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// net.inet.tcp.states

int do_net_inet_tcp_states(int update_every, usec_t dt) {
    (void)dt;
    static int mib[4] = {0, 0, 0, 0};
    uint64_t tcps_states[TCP_NSTATES];

    // see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
    if (unlikely(GETSYSCTL_SIMPLE("net.inet.tcp.states", mib, tcps_states))) {
        error("DISABLED: ipv4.tcpsock chart");
        error("DISABLED: net.inet.tcp.states module");
        return 1;
    } else {

        // --------------------------------------------------------------------

        static RRDSET *st = NULL;
        static RRDDIM *rd = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost("ipv4",
                                         "tcpsock",
                                         NULL,
                                         "tcp",
                                         NULL,
                                         "IPv4 TCP Connections",
                                         "active connections",
                                         2500,
                                         update_every,
                                         RRDSET_TYPE_LINE
            );

            rd = rrddim_add(st, "CurrEstab", "connections", 1, 1, RRD_ALGORITHM_ABSOLUTE);
        } else
            rrdset_next(st);

        rrddim_set_by_pointer(st, rd, tcps_states[TCPS_ESTABLISHED]);
        rrdset_done(st);
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// net.inet.tcp.stats

int do_net_inet_tcp_stats(int update_every, usec_t dt) {
    (void)dt;
    static int do_tcp_packets = -1, do_tcp_errors = -1, do_tcp_handshake = -1, do_tcpext_connaborts = -1, do_tcpext_ofo = -1, do_tcpext_syncookies = -1, do_ecn = -1;

    if (unlikely(do_tcp_packets == -1)) {
        do_tcp_packets       = config_get_boolean("plugin:freebsd:net.inet.tcp.stats", "ipv4 TCP packets",          1);
        do_tcp_errors        = config_get_boolean("plugin:freebsd:net.inet.tcp.stats", "ipv4 TCP errors",           1);
        do_tcp_handshake     = config_get_boolean("plugin:freebsd:net.inet.tcp.stats", "ipv4 TCP handshake issues", 1);
        do_tcpext_connaborts = config_get_boolean_ondemand("plugin:freebsd:net.inet.tcp.stats", "TCP connection aborts",
                                                           CONFIG_BOOLEAN_AUTO);
        do_tcpext_ofo        = config_get_boolean_ondemand("plugin:freebsd:net.inet.tcp.stats", "TCP out-of-order queue",
                                                           CONFIG_BOOLEAN_AUTO);
        do_tcpext_syncookies = config_get_boolean_ondemand("plugin:freebsd:net.inet.tcp.stats", "TCP SYN cookies",
                                                           CONFIG_BOOLEAN_AUTO);
        do_ecn               = config_get_boolean_ondemand("plugin:freebsd:net.inet.tcp.stats", "ECN packets",
                                                           CONFIG_BOOLEAN_AUTO);
    }

    // see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
    if (likely(do_tcp_packets || do_tcp_errors || do_tcp_handshake || do_tcpext_connaborts || do_tcpext_ofo || do_tcpext_syncookies || do_ecn)) {
        static int mib[4] = {0, 0, 0, 0};
        struct tcpstat tcpstat;

        if (unlikely(GETSYSCTL_SIMPLE("net.inet.tcp.stats", mib, tcpstat))) {
            do_tcp_packets = 0;
            error("DISABLED: ipv4.tcppackets chart");
            do_tcp_errors = 0;
            error("DISABLED: ipv4.tcperrors  chart");
            do_tcp_handshake = 0;
            error("DISABLED: ipv4.tcphandshake  chart");
            do_tcpext_connaborts = 0;
            error("DISABLED: ipv4.tcpconnaborts  chart");
            do_tcpext_ofo = 0;
            error("DISABLED: ipv4.tcpofo chart");
            do_tcpext_syncookies = 0;
            error("DISABLED: ipv4.tcpsyncookies chart");
            do_ecn = 0;
            error("DISABLED: ipv4.ecnpkts chart");
            error("DISABLED: net.inet.tcp.stats module");
            return 1;
        } else {

            // --------------------------------------------------------------------

            if (likely(do_tcp_packets)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_in_segs = NULL, *rd_out_segs = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "tcppackets",
                                                 NULL,
                                                 "tcp",
                                                 NULL,
                                                 "IPv4 TCP Packets",
                                                 "packets/s",
                                                 2600,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_in_segs  = rrddim_add(st, "InSegs",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_segs = rrddim_add(st, "OutSegs", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in_segs,  tcpstat.tcps_rcvtotal);
                rrddim_set_by_pointer(st, rd_out_segs, tcpstat.tcps_sndtotal);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_tcp_errors)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_in_errs = NULL, *rd_in_csum_errs = NULL, *rd_retrans_segs = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "tcperrors",
                                                 NULL,
                                                 "tcp",
                                                 NULL,
                                                 "IPv4 TCP Errors",
                                                 "packets/s",
                                                 2700,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_in_errs      = rrddim_add(st, "InErrs",       NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_csum_errs = rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_retrans_segs = rrddim_add(st, "RetransSegs",  NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

#if __FreeBSD__ >= 11
                rrddim_set_by_pointer(st, rd_in_errs,      tcpstat.tcps_rcvbadoff + tcpstat.tcps_rcvreassfull +
                                                           tcpstat.tcps_rcvshort);
#else
                rrddim_set_by_pointer(st, rd_in_errs,      tcpstat.tcps_rcvbadoff + tcpstat.tcps_rcvshort);
#endif
                rrddim_set_by_pointer(st, rd_in_csum_errs, tcpstat.tcps_rcvbadsum);
                rrddim_set_by_pointer(st, rd_retrans_segs, tcpstat.tcps_sndrexmitpack);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_tcp_handshake)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_estab_resets = NULL, *rd_active_opens = NULL, *rd_passive_opens = NULL,
                              *rd_attempt_fails = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "tcphandshake",
                                                 NULL,
                                                 "tcp",
                                                 NULL,
                                                 "IPv4 TCP Handshake Issues",
                                                 "events/s",
                                                 2900,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_estab_resets  = rrddim_add(st, "EstabResets",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_active_opens  = rrddim_add(st, "ActiveOpens",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_passive_opens = rrddim_add(st, "PassiveOpens", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_attempt_fails = rrddim_add(st, "AttemptFails", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_estab_resets,  tcpstat.tcps_drops);
                rrddim_set_by_pointer(st, rd_active_opens,  tcpstat.tcps_connattempt);
                rrddim_set_by_pointer(st, rd_passive_opens, tcpstat.tcps_accepts);
                rrddim_set_by_pointer(st, rd_attempt_fails, tcpstat.tcps_conndrops);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_tcpext_connaborts == CONFIG_BOOLEAN_YES || (do_tcpext_connaborts == CONFIG_BOOLEAN_AUTO && (tcpstat.tcps_rcvpackafterwin || tcpstat.tcps_rcvafterclose || tcpstat.tcps_rcvmemdrop || tcpstat.tcps_persistdrop || tcpstat.tcps_finwait2_drops))) {
                do_tcpext_connaborts = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_on_data = NULL, *rd_on_close = NULL, *rd_on_memory = NULL,
                              *rd_on_timeout = NULL, *rd_on_linger = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "tcpconnaborts",
                                                 NULL,
                                                 "tcp",
                                                 NULL,
                                                 "TCP Connection Aborts",
                                                 "connections/s",
                                                 3010,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_on_data    = rrddim_add(st, "TCPAbortOnData",    "baddata",     1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_on_close   = rrddim_add(st, "TCPAbortOnClose",   "userclosed",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_on_memory  = rrddim_add(st, "TCPAbortOnMemory",  "nomemory",    1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_on_timeout = rrddim_add(st, "TCPAbortOnTimeout", "timeout",     1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_on_linger  = rrddim_add(st, "TCPAbortOnLinger",  "linger",      1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_on_data,    tcpstat.tcps_rcvpackafterwin);
                rrddim_set_by_pointer(st, rd_on_close,   tcpstat.tcps_rcvafterclose);
                rrddim_set_by_pointer(st, rd_on_memory,  tcpstat.tcps_rcvmemdrop);
                rrddim_set_by_pointer(st, rd_on_timeout, tcpstat.tcps_persistdrop);
                rrddim_set_by_pointer(st, rd_on_linger,  tcpstat.tcps_finwait2_drops);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_tcpext_ofo == CONFIG_BOOLEAN_YES || (do_tcpext_ofo == CONFIG_BOOLEAN_AUTO && tcpstat.tcps_rcvoopack)) {
                do_tcpext_ofo = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_ofo_queue = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "tcpofo",
                                                 NULL,
                                                 "tcp",
                                                 NULL,
                                                 "TCP Out-Of-Order Queue",
                                                 "packets/s",
                                                 3050,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_ofo_queue = rrddim_add(st, "TCPOFOQueue", "inqueue",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_ofo_queue,   tcpstat.tcps_rcvoopack);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_tcpext_syncookies == CONFIG_BOOLEAN_YES || (do_tcpext_syncookies == CONFIG_BOOLEAN_AUTO && (tcpstat.tcps_sc_sendcookie || tcpstat.tcps_sc_recvcookie || tcpstat.tcps_sc_zonefail))) {
                do_tcpext_syncookies = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_recv = NULL, *rd_send = NULL, *rd_failed = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "tcpsyncookies",
                                                 NULL,
                                                 "tcp",
                                                 NULL,
                                                 "TCP SYN Cookies",
                                                 "packets/s",
                                                 3100,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_recv   = rrddim_add(st, "SyncookiesRecv",   "received",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_send   = rrddim_add(st, "SyncookiesSent",   "sent",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_failed = rrddim_add(st, "SyncookiesFailed", "failed",   -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_recv,   tcpstat.tcps_sc_recvcookie);
                rrddim_set_by_pointer(st, rd_send,   tcpstat.tcps_sc_sendcookie);
                rrddim_set_by_pointer(st, rd_failed, tcpstat.tcps_sc_zonefail);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_ecn == CONFIG_BOOLEAN_YES || (do_ecn == CONFIG_BOOLEAN_AUTO && (tcpstat.tcps_ecn_ce || tcpstat.tcps_ecn_ect0 || tcpstat.tcps_ecn_ect1))) {
                do_ecn = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_ce = NULL, *rd_no_ect = NULL, *rd_ect0 = NULL, *rd_ect1 = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "ecnpkts",
                                                 NULL,
                                                 "ecn",
                                                 NULL,
                                                 "IPv4 ECN Statistics",
                                                 "packets/s",
                                                 8700,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_ce     = rrddim_add(st, "InCEPkts", "CEP", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_no_ect = rrddim_add(st, "InNoECTPkts", "NoECTP", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_ect0   = rrddim_add(st, "InECT0Pkts", "ECTP0", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_ect1   = rrddim_add(st, "InECT1Pkts", "ECTP1", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_ce,     tcpstat.tcps_ecn_ce);
                rrddim_set_by_pointer(st, rd_no_ect, tcpstat.tcps_ecn_ce - (tcpstat.tcps_ecn_ect0 +
                                                                            tcpstat.tcps_ecn_ect1));
                rrddim_set_by_pointer(st, rd_ect0,   tcpstat.tcps_ecn_ect0);
                rrddim_set_by_pointer(st, rd_ect1,   tcpstat.tcps_ecn_ect1);
                rrdset_done(st);
            }

        }
    } else {
        error("DISABLED: net.inet.tcp.stats module");
        return 1;
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// net.inet.udp.stats

int do_net_inet_udp_stats(int update_every, usec_t dt) {
    (void)dt;
    static int do_udp_packets = -1, do_udp_errors = -1;

    if (unlikely(do_udp_packets == -1)) {
        do_udp_packets = config_get_boolean("plugin:freebsd:net.inet.udp.stats", "ipv4 UDP packets", 1);
        do_udp_errors  = config_get_boolean("plugin:freebsd:net.inet.udp.stats", "ipv4 UDP errors", 1);
    }

    // see http://net-snmp.sourceforge.net/docs/mibs/udp.html
    if (likely(do_udp_packets || do_udp_errors)) {
        static int mib[4] = {0, 0, 0, 0};
        struct udpstat udpstat;

        if (unlikely(GETSYSCTL_SIMPLE("net.inet.udp.stats", mib, udpstat))) {
            do_udp_packets = 0;
            error("DISABLED: ipv4.udppackets chart");
            do_udp_errors = 0;
            error("DISABLED: ipv4.udperrors chart");
            error("DISABLED: net.inet.udp.stats module");
            return 1;
        } else {

            // --------------------------------------------------------------------

            if (likely(do_udp_packets)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_in = NULL, *rd_out = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "udppackets",
                                                 NULL,
                                                 "udp",
                                                 NULL,
                                                 "IPv4 UDP Packets",
                                                 "packets/s",
                                                 2601,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_in  = rrddim_add(st, "InDatagrams",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out = rrddim_add(st, "OutDatagrams", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in,  udpstat.udps_ipackets);
                rrddim_set_by_pointer(st, rd_out, udpstat.udps_opackets);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_udp_errors)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_in_errors = NULL, *rd_no_ports = NULL, *rd_recv_buf_errors = NULL,
                              *rd_in_csum_errors = NULL, *rd_ignored_multi = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "udperrors",
                                                 NULL,
                                                 "udp",
                                                 NULL,
                                                 "IPv4 UDP Errors",
                                                 "events/s",
                                                 2701,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_in_errors       = rrddim_add(st, "InErrors",     NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_no_ports        = rrddim_add(st, "NoPorts",      NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_recv_buf_errors = rrddim_add(st, "RcvbufErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_csum_errors  = rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_ignored_multi   = rrddim_add(st, "IgnoredMulti", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in_errors,       udpstat.udps_hdrops + udpstat.udps_badlen);
                rrddim_set_by_pointer(st, rd_no_ports,        udpstat.udps_noport);
                rrddim_set_by_pointer(st, rd_recv_buf_errors, udpstat.udps_fullsock);
                rrddim_set_by_pointer(st, rd_in_csum_errors,  udpstat.udps_badsum + udpstat.udps_nosum);
                rrddim_set_by_pointer(st, rd_ignored_multi,   udpstat.udps_filtermcast);
                rrdset_done(st);
            }
        }
    } else {
        error("DISABLED: net.inet.udp.stats module");
        return 1;
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// net.inet.icmp.stats

int do_net_inet_icmp_stats(int update_every, usec_t dt) {
    (void)dt;
    static int do_icmp_packets = -1, do_icmp_errors = -1, do_icmpmsg = -1;

    if (unlikely(do_icmp_packets == -1)) {
        do_icmp_packets = config_get_boolean("plugin:freebsd:net.inet.icmp.stats", "ipv4 ICMP packets",  1);
        do_icmp_errors  = config_get_boolean("plugin:freebsd:net.inet.icmp.stats", "ipv4 ICMP errors",   1);
        do_icmpmsg      = config_get_boolean("plugin:freebsd:net.inet.icmp.stats", "ipv4 ICMP messages", 1);
    }

    if (likely(do_icmp_packets || do_icmp_errors || do_icmpmsg)) {
        static int mib[4] = {0, 0, 0, 0};
        struct icmpstat icmpstat;
        int i;
        struct icmp_total {
            u_long  msgs_in;
            u_long  msgs_out;
        } icmp_total = {0, 0};

        if (unlikely(GETSYSCTL_SIMPLE("net.inet.icmp.stats", mib, icmpstat))) {
            do_icmp_packets = 0;
            error("DISABLED: ipv4.icmp chart");
            do_icmp_errors = 0;
            error("DISABLED: ipv4.icmp_errors chart");
            do_icmpmsg = 0;
            error("DISABLED: ipv4.icmpmsg chart");
            error("DISABLED: net.inet.icmp.stats module");
            return 1;
        } else {
            for (i = 0; i <= ICMP_MAXTYPE; i++) {
                icmp_total.msgs_in += icmpstat.icps_inhist[i];
                icmp_total.msgs_out += icmpstat.icps_outhist[i];
            }
            icmp_total.msgs_in += icmpstat.icps_badcode + icmpstat.icps_badlen + icmpstat.icps_checksum + icmpstat.icps_tooshort;

            // --------------------------------------------------------------------

            if (likely(do_icmp_packets)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_in = NULL, *rd_out = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "icmp",
                                                 NULL,
                                                 "icmp",
                                                 NULL,
                                                 "IPv4 ICMP Packets",
                                                 "packets/s",
                                                 2602,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_in  = rrddim_add(st, "InMsgs",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out = rrddim_add(st, "OutMsgs", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in,  icmp_total.msgs_in);
                rrddim_set_by_pointer(st, rd_out, icmp_total.msgs_out);

                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_icmp_errors)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_in = NULL, *rd_out = NULL, *rd_in_csum = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "icmp_errors", NULL, "icmp", NULL, "IPv4 ICMP Errors",
                                                 "packets/s",
                                                 2603, update_every, RRDSET_TYPE_LINE);

                    rd_in      = rrddim_add(st, "InErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out     = rrddim_add(st, "OutErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_csum = rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in,      icmpstat.icps_badcode + icmpstat.icps_badlen +
                                                      icmpstat.icps_checksum + icmpstat.icps_tooshort);
                rrddim_set_by_pointer(st, rd_out,     icmpstat.icps_error);
                rrddim_set_by_pointer(st, rd_in_csum, icmpstat.icps_checksum);

                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_icmpmsg)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_in_reps = NULL, *rd_out_reps = NULL, *rd_in = NULL, *rd_out = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "icmpmsg", NULL, "icmp", NULL, "IPv4 ICMP Messages",
                                                 "packets/s", 2604, update_every, RRDSET_TYPE_LINE);

                    rd_in_reps  = rrddim_add(st, "InEchoReps",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_reps = rrddim_add(st, "OutEchoReps", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in       = rrddim_add(st, "InEchos",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out      = rrddim_add(st, "OutEchos",    NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in_reps, icmpstat.icps_inhist[ICMP_ECHOREPLY]);
                rrddim_set_by_pointer(st, rd_out_reps, icmpstat.icps_outhist[ICMP_ECHOREPLY]);
                rrddim_set_by_pointer(st, rd_in, icmpstat.icps_inhist[ICMP_ECHO]);
                rrddim_set_by_pointer(st, rd_out, icmpstat.icps_outhist[ICMP_ECHO]);

                rrdset_done(st);
            }
        }
    } else {
        error("DISABLED: net.inet.icmp.stats module");
        return 1;
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// net.inet.ip.stats

int do_net_inet_ip_stats(int update_every, usec_t dt) {
    (void)dt;
    static int do_ip_packets = -1, do_ip_fragsout = -1, do_ip_fragsin = -1, do_ip_errors = -1;

    if (unlikely(do_ip_packets == -1)) {
        do_ip_packets  = config_get_boolean("plugin:freebsd:net.inet.ip.stats", "ipv4 packets", 1);
        do_ip_fragsout = config_get_boolean("plugin:freebsd:net.inet.ip.stats", "ipv4 fragments sent", 1);
        do_ip_fragsin  = config_get_boolean("plugin:freebsd:net.inet.ip.stats", "ipv4 fragments assembly", 1);
        do_ip_errors   = config_get_boolean("plugin:freebsd:net.inet.ip.stats", "ipv4 errors", 1);
    }

    // see also http://net-snmp.sourceforge.net/docs/mibs/ip.html
    if (likely(do_ip_packets || do_ip_fragsout || do_ip_fragsin || do_ip_errors)) {
        static int mib[4] = {0, 0, 0, 0};
        struct ipstat ipstat;

        if (unlikely(GETSYSCTL_SIMPLE("net.inet.ip.stats", mib, ipstat))) {
            do_ip_packets = 0;
            error("DISABLED: ipv4.packets chart");
            do_ip_fragsout = 0;
            error("DISABLED: ipv4.fragsout chart");
            do_ip_fragsin = 0;
            error("DISABLED: ipv4.fragsin chart");
            do_ip_errors = 0;
            error("DISABLED: ipv4.errors chart");
            error("DISABLED: net.inet.ip.stats module");
            return 1;
        } else {

            // --------------------------------------------------------------------

            if (likely(do_ip_packets)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_in_receives = NULL, *rd_out_requests = NULL, *rd_forward_datagrams = NULL,
                              *rd_in_delivers = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "packets",
                                                 NULL,
                                                 "packets",
                                                 NULL,
                                                 "IPv4 Packets",
                                                 "packets/s",
                                                 3000,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_in_receives       = rrddim_add(st, "InReceives",    "received",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_requests      = rrddim_add(st, "OutRequests",   "sent",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_forward_datagrams = rrddim_add(st, "ForwDatagrams", "forwarded", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_delivers       = rrddim_add(st, "InDelivers",    "delivered", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in_receives,       ipstat.ips_total);
                rrddim_set_by_pointer(st, rd_out_requests,      ipstat.ips_localout);
                rrddim_set_by_pointer(st, rd_forward_datagrams, ipstat.ips_forward);
                rrddim_set_by_pointer(st, rd_in_delivers,       ipstat.ips_delivered);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_ip_fragsout)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_ok = NULL, *rd_fails = NULL, *rd_created = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "fragsout",
                                                 NULL,
                                                 "fragments",
                                                 NULL,
                                                 "IPv4 Fragments Sent",
                                                 "packets/s",
                                                 3010,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_ok      = rrddim_add(st, "FragOKs",     "ok",      1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_fails   = rrddim_add(st, "FragFails",   "failed", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_created = rrddim_add(st, "FragCreates", "created", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_ok,      ipstat.ips_fragmented);
                rrddim_set_by_pointer(st, rd_fails,   ipstat.ips_cantfrag);
                rrddim_set_by_pointer(st, rd_created, ipstat.ips_ofragments);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_ip_fragsin)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_ok = NULL, *rd_failed = NULL, *rd_all = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "fragsin",
                                                 NULL,
                                                 "fragments",
                                                 NULL,
                                                 "IPv4 Fragments Reassembly",
                                                 "packets/s",
                                                 3011,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_ok     = rrddim_add(st, "ReasmOKs",   "ok",      1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_failed = rrddim_add(st, "ReasmFails", "failed", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_all    = rrddim_add(st, "ReasmReqds", "all",     1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_ok,     ipstat.ips_fragments);
                rrddim_set_by_pointer(st, rd_failed, ipstat.ips_fragdropped);
                rrddim_set_by_pointer(st, rd_all,    ipstat.ips_reassembled);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_ip_errors)) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_in_discards = NULL, *rd_out_discards = NULL,
                              *rd_in_hdr_errors = NULL, *rd_out_no_routes = NULL,
                              *rd_in_addr_errors = NULL, *rd_in_unknown_protos = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4",
                                                 "errors",
                                                 NULL,
                                                 "errors",
                                                 NULL,
                                                 "IPv4 Errors",
                                                 "packets/s",
                                                 3002,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_in_discards       = rrddim_add(st, "InDiscards",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_discards      = rrddim_add(st, "OutDiscards",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_hdr_errors     = rrddim_add(st, "InHdrErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_no_routes     = rrddim_add(st, "OutNoRoutes",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_addr_errors    = rrddim_add(st, "InAddrErrors",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_unknown_protos = rrddim_add(st, "InUnknownProtos", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in_discards,       ipstat.ips_badsum + ipstat.ips_tooshort +
                                                                ipstat.ips_toosmall + ipstat.ips_toolong);
                rrddim_set_by_pointer(st, rd_out_discards,      ipstat.ips_odropped);
                rrddim_set_by_pointer(st, rd_in_hdr_errors,     ipstat.ips_badhlen + ipstat.ips_badlen +
                                                                ipstat.ips_badoptions + ipstat.ips_badvers);
                rrddim_set_by_pointer(st, rd_out_no_routes,     ipstat.ips_noroute);
                rrddim_set_by_pointer(st, rd_in_addr_errors,    ipstat.ips_badaddr);
                rrddim_set_by_pointer(st, rd_in_unknown_protos, ipstat.ips_noproto);
                rrdset_done(st);
            }
        }
    } else {
        error("DISABLED: net.inet.ip.stats module");
        return 1;
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// net.inet6.ip6.stats

int do_net_inet6_ip6_stats(int update_every, usec_t dt) {
    (void)dt;
    static int do_ip6_packets = -1, do_ip6_fragsout = -1, do_ip6_fragsin = -1, do_ip6_errors = -1;

    if (unlikely(do_ip6_packets == -1)) {
        do_ip6_packets  = config_get_boolean_ondemand("plugin:freebsd:net.inet6.ip6.stats", "ipv6 packets",
                                                      CONFIG_BOOLEAN_AUTO);
        do_ip6_fragsout = config_get_boolean_ondemand("plugin:freebsd:net.inet6.ip6.stats", "ipv6 fragments sent",
                                                      CONFIG_BOOLEAN_AUTO);
        do_ip6_fragsin  = config_get_boolean_ondemand("plugin:freebsd:net.inet6.ip6.stats", "ipv6 fragments assembly",
                                                      CONFIG_BOOLEAN_AUTO);
        do_ip6_errors   = config_get_boolean_ondemand("plugin:freebsd:net.inet6.ip6.stats", "ipv6 errors",
                                                      CONFIG_BOOLEAN_AUTO);
    }

    if (likely(do_ip6_packets || do_ip6_fragsout || do_ip6_fragsin || do_ip6_errors)) {
        static int mib[4] = {0, 0, 0, 0};
        struct ip6stat ip6stat;

        if (unlikely(GETSYSCTL_SIMPLE("net.inet6.ip6.stats", mib, ip6stat))) {
            do_ip6_packets = 0;
            error("DISABLED: ipv6.packets chart");
            do_ip6_fragsout = 0;
            error("DISABLED: ipv6.fragsout chart");
            do_ip6_fragsin = 0;
            error("DISABLED: ipv6.fragsin chart");
            do_ip6_errors = 0;
            error("DISABLED: ipv6.errors chart");
            error("DISABLED: net.inet6.ip6.stats module");
            return 1;
        } else {

            // --------------------------------------------------------------------

            if (do_ip6_packets == CONFIG_BOOLEAN_YES || (do_ip6_packets == CONFIG_BOOLEAN_AUTO &&
                                                         (ip6stat.ip6s_localout || ip6stat.ip6s_total ||
                                                          ip6stat.ip6s_forward || ip6stat.ip6s_delivered))) {
                do_ip6_packets = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_received = NULL, *rd_sent = NULL, *rd_forwarded = NULL, *rd_delivers = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv6",
                                                 "packets",
                                                 NULL,
                                                 "packets",
                                                 NULL,
                                                 "IPv6 Packets",
                                                 "packets/s",
                                                 3000,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_received  = rrddim_add(st, "received",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_sent      = rrddim_add(st, "sent",      NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_forwarded = rrddim_add(st, "forwarded", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_delivers  = rrddim_add(st, "delivers",  NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_sent,      ip6stat.ip6s_localout);
                rrddim_set_by_pointer(st, rd_received,  ip6stat.ip6s_total);
                rrddim_set_by_pointer(st, rd_forwarded, ip6stat.ip6s_forward);
                rrddim_set_by_pointer(st, rd_delivers,  ip6stat.ip6s_delivered);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_ip6_fragsout == CONFIG_BOOLEAN_YES || (do_ip6_fragsout == CONFIG_BOOLEAN_AUTO &&
                                                          (ip6stat.ip6s_fragmented || ip6stat.ip6s_cantfrag ||
                                                           ip6stat.ip6s_ofragments))) {
                do_ip6_fragsout = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_ok = NULL, *rd_failed = NULL, *rd_all = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv6",
                                                 "fragsout",
                                                 NULL,
                                                 "fragments",
                                                 NULL,
                                                 "IPv6 Fragments Sent",
                                                 "packets/s",
                                                 3010,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_ok     = rrddim_add(st, "ok",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_failed = rrddim_add(st, "failed", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_all    = rrddim_add(st, "all",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_ok,     ip6stat.ip6s_fragmented);
                rrddim_set_by_pointer(st, rd_failed, ip6stat.ip6s_cantfrag);
                rrddim_set_by_pointer(st, rd_all,    ip6stat.ip6s_ofragments);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_ip6_fragsin == CONFIG_BOOLEAN_YES || (do_ip6_fragsin == CONFIG_BOOLEAN_AUTO &&
                                                         (ip6stat.ip6s_reassembled || ip6stat.ip6s_fragdropped ||
                                                          ip6stat.ip6s_fragtimeout || ip6stat.ip6s_fragments))) {
                do_ip6_fragsin = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_ok = NULL, *rd_failed = NULL, *rd_timeout = NULL, *rd_all = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv6",
                                                 "fragsin",
                                                 NULL,
                                                 "fragments",
                                                 NULL,
                                                 "IPv6 Fragments Reassembly",
                                                 "packets/s",
                                                 3011,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_ok      = rrddim_add(st, "ok",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_failed  = rrddim_add(st, "failed",  NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_timeout = rrddim_add(st, "timeout", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_all     = rrddim_add(st, "all",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_ok,      ip6stat.ip6s_reassembled);
                rrddim_set_by_pointer(st, rd_failed,  ip6stat.ip6s_fragdropped);
                rrddim_set_by_pointer(st, rd_timeout, ip6stat.ip6s_fragtimeout);
                rrddim_set_by_pointer(st, rd_all,     ip6stat.ip6s_fragments);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_ip6_errors == CONFIG_BOOLEAN_YES || (do_ip6_errors == CONFIG_BOOLEAN_AUTO && (
                    ip6stat.ip6s_toosmall ||
                    ip6stat.ip6s_odropped ||
                    ip6stat.ip6s_badoptions ||
                    ip6stat.ip6s_badvers ||
                    ip6stat.ip6s_exthdrtoolong ||
                    ip6stat.ip6s_sources_none ||
                    ip6stat.ip6s_tooshort ||
                    ip6stat.ip6s_cantforward ||
                    ip6stat.ip6s_noroute))) {
                do_ip6_errors = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_in_discards = NULL, *rd_out_discards = NULL,
                              *rd_in_hdr_errors = NULL, *rd_in_addr_errors = NULL, *rd_in_truncated_pkts = NULL,
                              *rd_in_no_routes = NULL, *rd_out_no_routes = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv6",
                                                 "errors",
                                                 NULL,
                                                 "errors",
                                                 NULL,
                                                 "IPv6 Errors",
                                                 "packets/s",
                                                 3002,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_in_discards       = rrddim_add(st, "InDiscards",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_discards      = rrddim_add(st, "OutDiscards",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_hdr_errors     = rrddim_add(st, "InHdrErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_addr_errors    = rrddim_add(st, "InAddrErrors",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_truncated_pkts = rrddim_add(st, "InTruncatedPkts", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_no_routes      = rrddim_add(st, "InNoRoutes",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_no_routes     = rrddim_add(st, "OutNoRoutes",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in_discards,       ip6stat.ip6s_toosmall);
                rrddim_set_by_pointer(st, rd_out_discards,      ip6stat.ip6s_odropped);
                rrddim_set_by_pointer(st, rd_in_hdr_errors,     ip6stat.ip6s_badoptions + ip6stat.ip6s_badvers +
                                                                ip6stat.ip6s_exthdrtoolong);
                rrddim_set_by_pointer(st, rd_in_addr_errors,    ip6stat.ip6s_sources_none);
                rrddim_set_by_pointer(st, rd_in_truncated_pkts, ip6stat.ip6s_tooshort);
                rrddim_set_by_pointer(st, rd_in_no_routes,      ip6stat.ip6s_cantforward);
                rrddim_set_by_pointer(st, rd_out_no_routes,     ip6stat.ip6s_noroute);
                rrdset_done(st);
            }
        }
    } else {
        error("DISABLED: net.inet6.ip6.stats module");
        return 1;
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// net.inet6.icmp6.stats

int do_net_inet6_icmp6_stats(int update_every, usec_t dt) {
    (void)dt;
    static int do_icmp6 = -1, do_icmp6_redir = -1, do_icmp6_errors = -1, do_icmp6_echos = -1, do_icmp6_router = -1,
            do_icmp6_neighbor = -1, do_icmp6_types = -1;

    if (unlikely(do_icmp6 == -1)) {
        do_icmp6          = config_get_boolean_ondemand("plugin:freebsd:net.inet6.icmp6.stats", "icmp",
                                                        CONFIG_BOOLEAN_AUTO);
        do_icmp6_redir    = config_get_boolean_ondemand("plugin:freebsd:net.inet6.icmp6.stats", "icmp redirects",
                                                        CONFIG_BOOLEAN_AUTO);
        do_icmp6_errors   = config_get_boolean_ondemand("plugin:freebsd:net.inet6.icmp6.stats", "icmp errors",
                                                        CONFIG_BOOLEAN_AUTO);
        do_icmp6_echos    = config_get_boolean_ondemand("plugin:freebsd:net.inet6.icmp6.stats", "icmp echos",
                                                        CONFIG_BOOLEAN_AUTO);
        do_icmp6_router   = config_get_boolean_ondemand("plugin:freebsd:net.inet6.icmp6.stats", "icmp router",
                                                        CONFIG_BOOLEAN_AUTO);
        do_icmp6_neighbor = config_get_boolean_ondemand("plugin:freebsd:net.inet6.icmp6.stats", "icmp neighbor",
                                                        CONFIG_BOOLEAN_AUTO);
        do_icmp6_types    = config_get_boolean_ondemand("plugin:freebsd:net.inet6.icmp6.stats", "icmp types",
                                                        CONFIG_BOOLEAN_AUTO);
    }

    if (likely(do_icmp6 || do_icmp6_redir || do_icmp6_errors || do_icmp6_echos || do_icmp6_router || do_icmp6_neighbor || do_icmp6_types)) {
        static int mib[4] = {0, 0, 0, 0};
        struct icmp6stat icmp6stat;

        if (unlikely(GETSYSCTL_SIMPLE("net.inet6.icmp6.stats", mib, icmp6stat))) {
            do_icmp6 = 0;
            error("DISABLED: ipv6.icmp chart");
            do_icmp6_redir = 0;
            error("DISABLED: ipv6.icmpredir chart");
            do_icmp6_errors = 0;
            error("DISABLED: ipv6.icmperrors chart");
            do_icmp6_echos = 0;
            error("DISABLED: ipv6.icmpechos chart");
            do_icmp6_router = 0;
            error("DISABLED: ipv6.icmprouter chart");
            do_icmp6_neighbor = 0;
            error("DISABLED: ipv6.icmpneighbor chart");
            do_icmp6_types = 0;
            error("DISABLED: ipv6.icmptypes chart");
            error("DISABLED: net.inet6.icmp6.stats module");
            return 1;
        } else {
            int i;
            struct icmp6_total {
                u_long  msgs_in;
                u_long  msgs_out;
            } icmp6_total = {0, 0};

            for (i = 0; i <= ICMP6_MAXTYPE; i++) {
                icmp6_total.msgs_in += icmp6stat.icp6s_inhist[i];
                icmp6_total.msgs_out += icmp6stat.icp6s_outhist[i];
            }
            icmp6_total.msgs_in += icmp6stat.icp6s_badcode + icmp6stat.icp6s_badlen + icmp6stat.icp6s_checksum + icmp6stat.icp6s_tooshort;

            // --------------------------------------------------------------------

            if (do_icmp6 == CONFIG_BOOLEAN_YES || (do_icmp6 == CONFIG_BOOLEAN_AUTO && (icmp6_total.msgs_in || icmp6_total.msgs_out))) {
                do_icmp6 = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_received = NULL, *rd_sent = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv6",
                                                 "icmp",
                                                 NULL,
                                                 "icmp",
                                                 NULL,
                                                 "IPv6 ICMP Messages",
                                                 "messages/s",
                                                 10000,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_received = rrddim_add(st, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_sent     = rrddim_add(st, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_received, icmp6_total.msgs_out);
                rrddim_set_by_pointer(st, rd_sent,     icmp6_total.msgs_in);

                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_icmp6_redir == CONFIG_BOOLEAN_YES || (do_icmp6_redir == CONFIG_BOOLEAN_AUTO && (icmp6stat.icp6s_inhist[ND_REDIRECT] || icmp6stat.icp6s_outhist[ND_REDIRECT]))) {
                do_icmp6_redir = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_received = NULL, *rd_sent = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv6",
                                                 "icmpredir",
                                                 NULL,
                                                 "icmp",
                                                 NULL,
                                                 "IPv6 ICMP Redirects",
                                                 "redirects/s",
                                                 10050,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_received = rrddim_add(st, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_sent     = rrddim_add(st, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_received, icmp6stat.icp6s_outhist[ND_REDIRECT]);
                rrddim_set_by_pointer(st, rd_sent, icmp6stat.icp6s_inhist[ND_REDIRECT]);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_icmp6_errors == CONFIG_BOOLEAN_YES || (do_icmp6_errors == CONFIG_BOOLEAN_AUTO && (
                    icmp6stat.icp6s_badcode ||
                    icmp6stat.icp6s_badlen ||
                    icmp6stat.icp6s_checksum ||
                    icmp6stat.icp6s_tooshort ||
                    icmp6stat.icp6s_error ||
                    icmp6stat.icp6s_inhist[ICMP6_DST_UNREACH] ||
                    icmp6stat.icp6s_inhist[ICMP6_TIME_EXCEEDED] ||
                    icmp6stat.icp6s_inhist[ICMP6_PARAM_PROB] ||
                    icmp6stat.icp6s_outhist[ICMP6_DST_UNREACH] ||
                    icmp6stat.icp6s_outhist[ICMP6_TIME_EXCEEDED] ||
                    icmp6stat.icp6s_outhist[ICMP6_PARAM_PROB]))) {
                do_icmp6_errors = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_in_errors = NULL, *rd_out_errors = NULL, *rd_in_csum_errors = NULL,
                              *rd_in_dest_unreachs = NULL, *rd_in_pkt_too_bigs = NULL, *rd_in_time_excds = NULL,
                              *rd_in_parm_problems = NULL, *rd_out_dest_unreachs = NULL, *rd_out_time_excds = NULL,
                              *rd_out_parm_problems = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv6",
                                                 "icmperrors",
                                                 NULL, "icmp",
                                                 NULL,
                                                 "IPv6 ICMP Errors",
                                                 "errors/s",
                                                 10100,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_in_errors         = rrddim_add(st, "InErrors",        NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_errors        = rrddim_add(st, "OutErrors",       NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_csum_errors    = rrddim_add(st, "InCsumErrors",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_dest_unreachs  = rrddim_add(st, "InDestUnreachs",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_pkt_too_bigs   = rrddim_add(st, "InPktTooBigs",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_time_excds     = rrddim_add(st, "InTimeExcds",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_parm_problems  = rrddim_add(st, "InParmProblems",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_dest_unreachs = rrddim_add(st, "OutDestUnreachs", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_time_excds    = rrddim_add(st, "OutTimeExcds",    NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_parm_problems = rrddim_add(st, "OutParmProblems", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in_errors,         icmp6stat.icp6s_badcode + icmp6stat.icp6s_badlen +
                                                                icmp6stat.icp6s_checksum + icmp6stat.icp6s_tooshort);
                rrddim_set_by_pointer(st, rd_out_errors,        icmp6stat.icp6s_error);
                rrddim_set_by_pointer(st, rd_in_csum_errors,    icmp6stat.icp6s_checksum);
                rrddim_set_by_pointer(st, rd_in_dest_unreachs,  icmp6stat.icp6s_inhist[ICMP6_DST_UNREACH]);
                rrddim_set_by_pointer(st, rd_in_pkt_too_bigs,   icmp6stat.icp6s_badlen);
                rrddim_set_by_pointer(st, rd_in_time_excds,     icmp6stat.icp6s_inhist[ICMP6_TIME_EXCEEDED]);
                rrddim_set_by_pointer(st, rd_in_parm_problems,  icmp6stat.icp6s_inhist[ICMP6_PARAM_PROB]);
                rrddim_set_by_pointer(st, rd_out_dest_unreachs, icmp6stat.icp6s_outhist[ICMP6_DST_UNREACH]);
                rrddim_set_by_pointer(st, rd_out_time_excds,    icmp6stat.icp6s_outhist[ICMP6_TIME_EXCEEDED]);
                rrddim_set_by_pointer(st, rd_out_parm_problems, icmp6stat.icp6s_outhist[ICMP6_PARAM_PROB]);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_icmp6_echos == CONFIG_BOOLEAN_YES || (do_icmp6_echos == CONFIG_BOOLEAN_AUTO && (
                    icmp6stat.icp6s_inhist[ICMP6_ECHO_REQUEST] ||
                    icmp6stat.icp6s_outhist[ICMP6_ECHO_REQUEST] ||
                    icmp6stat.icp6s_inhist[ICMP6_ECHO_REPLY] ||
                    icmp6stat.icp6s_outhist[ICMP6_ECHO_REPLY]))) {
                do_icmp6_echos = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_in = NULL, *rd_out = NULL, *rd_in_replies = NULL, *rd_out_replies = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv6",
                                                 "icmpechos",
                                                 NULL,
                                                 "icmp",
                                                 NULL,
                                                 "IPv6 ICMP Echo",
                                                 "messages/s",
                                                 10200,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_in          = rrddim_add(st, "InEchos",        NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out         = rrddim_add(st, "OutEchos",       NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_replies  = rrddim_add(st, "InEchoReplies",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_replies = rrddim_add(st, "OutEchoReplies", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in,          icmp6stat.icp6s_inhist[ICMP6_ECHO_REQUEST]);
                rrddim_set_by_pointer(st, rd_out,         icmp6stat.icp6s_outhist[ICMP6_ECHO_REQUEST]);
                rrddim_set_by_pointer(st, rd_in_replies,  icmp6stat.icp6s_inhist[ICMP6_ECHO_REPLY]);
                rrddim_set_by_pointer(st, rd_out_replies, icmp6stat.icp6s_outhist[ICMP6_ECHO_REPLY]);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_icmp6_router == CONFIG_BOOLEAN_YES || (do_icmp6_router == CONFIG_BOOLEAN_AUTO && (
                    icmp6stat.icp6s_inhist[ND_ROUTER_SOLICIT] ||
                    icmp6stat.icp6s_outhist[ND_ROUTER_SOLICIT] ||
                    icmp6stat.icp6s_inhist[ND_ROUTER_ADVERT] ||
                    icmp6stat.icp6s_outhist[ND_ROUTER_ADVERT]))) {
                do_icmp6_router = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_in_solicits = NULL, *rd_out_solicits = NULL,
                              *rd_in_advertisements = NULL, *rd_out_advertisements = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv6",
                                                 "icmprouter",
                                                 NULL,
                                                 "icmp",
                                                 NULL,
                                                 "IPv6 Router Messages",
                                                 "messages/s",
                                                 10400,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_in_solicits        = rrddim_add(st, "InSolicits",        NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_solicits       = rrddim_add(st, "OutSolicits",       NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_advertisements  = rrddim_add(st, "InAdvertisements",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_advertisements = rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in_solicits,        icmp6stat.icp6s_inhist[ND_ROUTER_SOLICIT]);
                rrddim_set_by_pointer(st, rd_out_solicits,       icmp6stat.icp6s_outhist[ND_ROUTER_SOLICIT]);
                rrddim_set_by_pointer(st, rd_in_advertisements,  icmp6stat.icp6s_inhist[ND_ROUTER_ADVERT]);
                rrddim_set_by_pointer(st, rd_out_advertisements, icmp6stat.icp6s_outhist[ND_ROUTER_ADVERT]);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_icmp6_neighbor == CONFIG_BOOLEAN_YES || (do_icmp6_neighbor == CONFIG_BOOLEAN_AUTO && (
                    icmp6stat.icp6s_inhist[ND_NEIGHBOR_SOLICIT] ||
                    icmp6stat.icp6s_outhist[ND_NEIGHBOR_SOLICIT] ||
                    icmp6stat.icp6s_inhist[ND_NEIGHBOR_ADVERT] ||
                    icmp6stat.icp6s_outhist[ND_NEIGHBOR_ADVERT]))) {
                do_icmp6_neighbor = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_in_solicits = NULL, *rd_out_solicits = NULL,
                              *rd_in_advertisements = NULL, *rd_out_advertisements = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv6",
                                                 "icmpneighbor",
                                                 NULL,
                                                 "icmp",
                                                 NULL,
                                                 "IPv6 Neighbor Messages",
                                                 "messages/s",
                                                 10500,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_in_solicits        = rrddim_add(st, "InSolicits",        NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_solicits       = rrddim_add(st, "OutSolicits",       NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_advertisements  = rrddim_add(st, "InAdvertisements",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_advertisements = rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in_solicits,        icmp6stat.icp6s_inhist[ND_NEIGHBOR_SOLICIT]);
                rrddim_set_by_pointer(st, rd_out_solicits,       icmp6stat.icp6s_outhist[ND_NEIGHBOR_SOLICIT]);
                rrddim_set_by_pointer(st, rd_in_advertisements,  icmp6stat.icp6s_inhist[ND_NEIGHBOR_ADVERT]);
                rrddim_set_by_pointer(st, rd_out_advertisements, icmp6stat.icp6s_outhist[ND_NEIGHBOR_ADVERT]);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_icmp6_types == CONFIG_BOOLEAN_YES || (do_icmp6_types == CONFIG_BOOLEAN_AUTO && (
                    icmp6stat.icp6s_inhist[1] ||
                    icmp6stat.icp6s_inhist[128] ||
                    icmp6stat.icp6s_inhist[129] ||
                    icmp6stat.icp6s_inhist[136] ||
                    icmp6stat.icp6s_outhist[1] ||
                    icmp6stat.icp6s_outhist[128] ||
                    icmp6stat.icp6s_outhist[129] ||
                    icmp6stat.icp6s_outhist[133] ||
                    icmp6stat.icp6s_outhist[135] ||
                    icmp6stat.icp6s_outhist[136]))) {
                do_icmp6_types = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_in_1 = NULL, *rd_in_128 = NULL, *rd_in_129 = NULL, *rd_in_136 = NULL,
                              *rd_out_1 = NULL, *rd_out_128 = NULL, *rd_out_129 = NULL, *rd_out_133 = NULL,
                              *rd_out_135 = NULL, *rd_out_143 = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("ipv6",
                                                 "icmptypes",
                                                 NULL,
                                                 "icmp",
                                                 NULL,
                                                 "IPv6 ICMP Types",
                                                 "messages/s",
                                                 10700,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                    );

                    rd_in_1    = rrddim_add(st, "InType1",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_128  = rrddim_add(st, "InType128",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_129  = rrddim_add(st, "InType129",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_in_136  = rrddim_add(st, "InType136",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_1   = rrddim_add(st, "OutType1",   NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_128 = rrddim_add(st, "OutType128", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_129 = rrddim_add(st, "OutType129", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_133 = rrddim_add(st, "OutType133", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_135 = rrddim_add(st, "OutType135", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out_143 = rrddim_add(st, "OutType143", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in_1,    icmp6stat.icp6s_inhist[1]);
                rrddim_set_by_pointer(st, rd_in_128,  icmp6stat.icp6s_inhist[128]);
                rrddim_set_by_pointer(st, rd_in_129,  icmp6stat.icp6s_inhist[129]);
                rrddim_set_by_pointer(st, rd_in_136,  icmp6stat.icp6s_inhist[136]);
                rrddim_set_by_pointer(st, rd_out_1,   icmp6stat.icp6s_outhist[1]);
                rrddim_set_by_pointer(st, rd_out_128, icmp6stat.icp6s_outhist[128]);
                rrddim_set_by_pointer(st, rd_out_129, icmp6stat.icp6s_outhist[129]);
                rrddim_set_by_pointer(st, rd_out_133, icmp6stat.icp6s_outhist[133]);
                rrddim_set_by_pointer(st, rd_out_135, icmp6stat.icp6s_outhist[135]);
                rrddim_set_by_pointer(st, rd_out_143, icmp6stat.icp6s_outhist[143]);
                rrdset_done(st);
            }
        }
    } else {
        error("DISABLED: net.inet6.icmp6.stats module");
        return 1;
    }

    return 0;
}
