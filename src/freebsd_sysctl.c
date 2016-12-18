#include "common.h"

// NEEDED BY: struct vmtotal, struct vmmeter
#include <sys/vmmeter.h>
// NEEDED BY: struct devstat
#include <sys/devicestat.h>
// NEEDED BY: struct xswdev
#include <vm/vm_param.h>
// NEEDED BY: struct semid_kernel, struct shmid_kernel, struct msqid_kernel
#define _KERNEL
#include <sys/sem.h>
#include <sys/shm.h>
#include <sys/msg.h>
#undef _KERNEL
// NEEDED BY: struct sysctl_netisr_workstream, struct sysctl_netisr_work
#include <net/netisr.h>
// NEEDED BY: struct ifaddrs, getifaddrs()
#define _IFI_OQDROPS // It is for FreeNAS only. Most probably in future releases of FreeNAS it will be removed
#include <net/if.h>
#include <ifaddrs.h>
// NEEDED BY: do_disk_io
#define RRD_TYPE_DISK "disk"

// FreeBSD calculates load averages once every 5 seconds
#define MIN_LOADAVG_UPDATE_EVERY 5

// NEEDED BY: do_bandwidth
#define IFA_DATA(s) (((struct if_data *)ifa->ifa_data)->ifi_ ## s)

int do_freebsd_sysctl(int update_every, usec_t dt) {
    (void)dt;

    static int do_cpu = -1, do_cpu_cores = -1, do_interrupts = -1, do_context = -1, do_forks = -1, do_processes = -1,
        do_loadavg = -1, do_all_processes = -1, do_disk_io = -1, do_swap = -1, do_ram = -1, do_swapio = -1,
        do_pgfaults = -1, do_committed = -1, do_ipc_semaphores = -1, do_ipc_shared_mem = -1, do_ipc_msg_queues = -1,
        do_dev_intr = -1, do_soft_intr = -1, do_netisr = -1, do_netisr_per_core = -1, do_bandwidth = -1;

    if (unlikely(do_cpu == -1)) {
        do_cpu                  = config_get_boolean("plugin:freebsd:sysctl", "cpu utilization", 1);
        do_cpu_cores            = config_get_boolean("plugin:freebsd:sysctl", "per cpu core utilization", 1);
        do_interrupts           = config_get_boolean("plugin:freebsd:sysctl", "cpu interrupts", 1);
        do_dev_intr             = config_get_boolean("plugin:freebsd:sysctl", "device interrupts", 1);
        do_soft_intr            = config_get_boolean("plugin:freebsd:sysctl", "software interrupts", 1);
        do_context              = config_get_boolean("plugin:freebsd:sysctl", "context switches", 1);
        do_forks                = config_get_boolean("plugin:freebsd:sysctl", "processes started", 1);
        do_processes            = config_get_boolean("plugin:freebsd:sysctl", "processes running", 1);
        do_loadavg              = config_get_boolean("plugin:freebsd:sysctl", "enable load average", 1);
        do_all_processes        = config_get_boolean("plugin:freebsd:sysctl", "enable total processes", 1);
        do_disk_io              = config_get_boolean("plugin:freebsd:sysctl", "stats for all disks", 1);
        do_swap                 = config_get_boolean("plugin:freebsd:sysctl", "system swap", 1);
        do_ram                  = config_get_boolean("plugin:freebsd:sysctl", "system ram", 1);
        do_swapio               = config_get_boolean("plugin:freebsd:sysctl", "swap i/o", 1);
        do_pgfaults             = config_get_boolean("plugin:freebsd:sysctl", "memory page faults", 1);
        do_committed            = config_get_boolean("plugin:freebsd:sysctl", "committed memory", 1);
        do_ipc_semaphores       = config_get_boolean("plugin:freebsd:sysctl", "ipc semaphores", 1);
        do_ipc_shared_mem       = config_get_boolean("plugin:freebsd:sysctl", "ipc shared memory", 1);
        do_ipc_msg_queues       = config_get_boolean("plugin:freebsd:sysctl", "ipc message queues", 1);
        do_netisr               = config_get_boolean("plugin:freebsd:sysctl", "netisr", 1);
        do_netisr_per_core      = config_get_boolean("plugin:freebsd:sysctl", "netisr per core", 1);
        do_bandwidth            = config_get_boolean("plugin:freebsd:sysctl", "bandwidth", 1);
    }

    RRDSET *st;

    int system_pagesize = getpagesize(); // wouldn't it be better to get value directly from hw.pagesize?
    int i, n;
    int common_error = 0;
    size_t size;

    // NEEDED BY: do_loadavg
    static usec_t last_loadavg_usec = 0;
    struct loadavg sysload;

    // NEEDED BY: do_cpu, do_cpu_cores
    long cp_time[CPUSTATES];

    // NEEDED BY: du_cpu_cores, do_netisr, do_netisr_per_core
    int ncpus;

    // NEEDED BY: do_cpu_cores
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

    // NEEDED BY: do_disk_io
    #define BINTIME_SCALE 5.42101086242752217003726400434970855712890625e-17 // this is 1000/2^64
    int numdevs;
    static void *devstat_data = NULL;
    struct devstat *dstat;
    struct cur_dstat {
        collected_number duration_read_ms;
        collected_number duration_write_ms;
        collected_number busy_time_ms;
    } cur_dstat;
    struct prev_dstat {
        collected_number bytes_read;
        collected_number bytes_write;
        collected_number operations_read;
        collected_number operations_write;
        collected_number duration_read_ms;
        collected_number duration_write_ms;
        collected_number busy_time_ms;
    } prev_dstat;

    // NEEDED BY: do_swap
    size_t mibsize;
    int mib[3]; // CTL_MAXNAME = 24 maximum mib components (sysctl.h)
    struct xswdev xsw;
    struct total_xsw {
        collected_number bytes_used;
        collected_number bytes_total;
    } total_xsw = {0, 0};

    // NEEDED BY: do_swapio, do_ram
    struct vmmeter vmmeter_data;

    // NEEDED BY: do_ram
    int vfs_bufspace_count;

    // NEEDED BY: do_ipc_semaphores
    struct ipc_sem {
        int semmni;
        collected_number sets;
        collected_number semaphores;
    } ipc_sem = {0, 0, 0};
    static struct semid_kernel *ipc_sem_data = NULL;

    // NEEDED BY: do_ipc_shared_mem
    struct ipc_shm {
        u_long shmmni;
        collected_number segs;
        collected_number segsize;
    } ipc_shm = {0, 0, 0};
    static struct shmid_kernel *ipc_shm_data = NULL;

    // NEEDED BY: do_ipc_msg_queues
    struct ipc_msq {
        int msgmni;
        collected_number queues;
        collected_number messages;
        collected_number usedsize;
        collected_number allocsize;
    } ipc_msq = {0, 0, 0, 0, 0};
    static struct msqid_kernel *ipc_msq_data = NULL;

    // NEEDED BY: do_netisr, do_netisr_per_core
    size_t netisr_workstream_size;
    size_t netisr_work_size;
    unsigned long num_netisr_workstreams = 0, num_netisr_works = 0;
    static struct sysctl_netisr_workstream *netisr_workstream = NULL;
    static struct sysctl_netisr_work *netisr_work = NULL;
    static struct netisr_stats {
        collected_number dispatched;
        collected_number hybrid_dispatched;
        collected_number qdrops;
        collected_number queued;
    } *netisr_stats = NULL;
    char netstat_cpuid[21]; // no more than 4 digits expected

    // NEEDED BY: do_bandwidth
    struct ifaddrs *ifa, *ifap;
    struct iftot {
        u_long  ift_ibytes;
        u_long  ift_obytes;
    } iftot = {0, 0};

    // --------------------------------------------------------------------

    if (last_loadavg_usec <= dt) {
        if (likely(do_loadavg)) {
            if (unlikely(GETSYSCTL("vm.loadavg", sysload))) {
                do_loadavg = 0;
                error("DISABLED: system.load");
            } else {

                st = rrdset_find_bytype("system", "load");
                if (unlikely(!st)) {
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

        last_loadavg_usec = st->update_every * USEC_PER_SEC;
    }
    else last_loadavg_usec -= dt;

    // --------------------------------------------------------------------

    if (likely(do_all_processes | do_processes | do_committed)) {
        if (unlikely(GETSYSCTL("vm.vmtotal", vmtotal_data))) {
            do_all_processes = 0;
            error("DISABLED: system.active_processes");
            do_processes = 0;
            error("DISABLED: system.processes");
            do_committed = 0;
            error("DISABLED: mem.committed");
        } else {
            if (likely(do_all_processes)) {

                st = rrdset_find_bytype("system", "active_processes");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "active_processes", NULL, "processes", NULL, "System Active Processes", "processes", 750, update_every, RRDSET_TYPE_LINE);
                    rrddim_add(st, "active", NULL, 1, 1, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "active", (vmtotal_data.t_rq + vmtotal_data.t_dw + vmtotal_data.t_pw + vmtotal_data.t_sl + vmtotal_data.t_sw));
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_processes)) {

                st = rrdset_find_bytype("system", "processes");
                if (unlikely(!st)) {
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

            if (likely(do_committed)) {
                st = rrdset_find("mem.committed");
                if (unlikely(!st)) {
                    st = rrdset_create("mem", "committed", NULL, "system", NULL, "Committed (Allocated) Memory", "MB", 5000, update_every, RRDSET_TYPE_AREA);
                    st->isdetail = 1;

                    rrddim_add(st, "Committed_AS", NULL, system_pagesize, 1024, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "Committed_AS", vmtotal_data.t_rm);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_cpu)) {
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
    }

    // --------------------------------------------------------------------

    if (likely(do_cpu_cores)) {
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
                    if (unlikely(!st)) {
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

    if (likely(do_interrupts)) {
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
                if (unlikely(!st)) {
                    st = rrdset_create("system", "intr", NULL, "interrupts", NULL, "Total Hardware Interrupts", "interrupts/s", 900, update_every, RRDSET_TYPE_LINE);
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

    if (likely(do_dev_intr)) {
        if (unlikely(GETSYSCTL("vm.stats.sys.v_intr", u_int_data))) {
            do_dev_intr = 0;
            error("DISABLED: system.dev_intr");
        } else {

            st = rrdset_find_bytype("system", "dev_intr");
            if (unlikely(!st)) {
                st = rrdset_create("system", "dev_intr", NULL, "interrupts", NULL, "Device Interrupts", "interrupts/s", 1000, update_every, RRDSET_TYPE_LINE);

                rrddim_add(st, "interrupts", NULL, 1, 1, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "interrupts", u_int_data);
            rrdset_done(st);
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_soft_intr)) {
        if (unlikely(GETSYSCTL("vm.stats.sys.v_soft", u_int_data))) {
            do_soft_intr = 0;
            error("DISABLED: system.dev_intr");
        } else {

            st = rrdset_find_bytype("system", "soft_intr");
            if (unlikely(!st)) {
                st = rrdset_create("system", "soft_intr", NULL, "interrupts", NULL, "Software Interrupts", "interrupts/s", 1100, update_every, RRDSET_TYPE_LINE);

                rrddim_add(st, "interrupts", NULL, 1, 1, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "interrupts", u_int_data);
            rrdset_done(st);
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_context)) {
        if (unlikely(GETSYSCTL("vm.stats.sys.v_swtch", u_int_data))) {
            do_context = 0;
            error("DISABLED: system.ctxt");
        } else {

            st = rrdset_find_bytype("system", "ctxt");
            if (unlikely(!st)) {
                st = rrdset_create("system", "ctxt", NULL, "processes", NULL, "CPU Context Switches", "context switches/s", 800, update_every, RRDSET_TYPE_LINE);

                rrddim_add(st, "switches", NULL, 1, 1, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "switches", u_int_data);
            rrdset_done(st);
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_forks)) {
        if (unlikely(GETSYSCTL("vm.stats.vm.v_forks", u_int_data))) {
            do_forks = 0;
            error("DISABLED: system.forks");
        } else {

            st = rrdset_find_bytype("system", "forks");
            if (unlikely(!st)) {
                st = rrdset_create("system", "forks", NULL, "processes", NULL, "Started Processes", "processes/s", 700, update_every, RRDSET_TYPE_LINE);
                st->isdetail = 1;

                rrddim_add(st, "started", NULL, 1, 1, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "started", u_int_data);
            rrdset_done(st);
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_disk_io)) {
        if (unlikely(GETSYSCTL("kern.devstat.numdevs", numdevs))) {
            do_disk_io = 0;
            error("DISABLED: disk.io");
        } else {
            devstat_data = reallocz(devstat_data, sizeof(long) + sizeof(struct devstat) * numdevs); // there is generation number before devstat structures
            if (unlikely(getsysctl("kern.devstat.all", devstat_data, sizeof(long) + sizeof(struct devstat) * numdevs))) {
                do_disk_io = 0;
                error("DISABLED: disk.io");
            } else {
                dstat = devstat_data + sizeof(long); // skip generation number
                collected_number total_disk_reads = 0;
                collected_number total_disk_writes = 0;

                for (i = 0; i < numdevs; i++) {
                    if ((dstat[i].device_type == (DEVSTAT_TYPE_IF_SCSI | DEVSTAT_TYPE_DIRECT)) || (dstat[i].device_type == (DEVSTAT_TYPE_IF_IDE | DEVSTAT_TYPE_DIRECT))) {

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype(RRD_TYPE_DISK, dstat[i].device_name);
                        if (unlikely(!st)) {
                            st = rrdset_create(RRD_TYPE_DISK, dstat[i].device_name, NULL, dstat[i].device_name, "disk.io", "Disk I/O Bandwidth", "kilobytes/s", 2000, update_every, RRDSET_TYPE_AREA);

                            rrddim_add(st, "reads", NULL, 1, 1024, RRDDIM_INCREMENTAL);
                            rrddim_add(st, "writes", NULL, -1, 1024, RRDDIM_INCREMENTAL);
                        }
                        else rrdset_next(st);

                        total_disk_reads += dstat[i].bytes[DEVSTAT_READ];
                        total_disk_writes += dstat[i].bytes[DEVSTAT_WRITE];
                        prev_dstat.bytes_read = rrddim_set(st, "reads", dstat[i].bytes[DEVSTAT_READ]);
                        prev_dstat.bytes_write = rrddim_set(st, "writes", dstat[i].bytes[DEVSTAT_WRITE]);
                        rrdset_done(st);

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype("disk_ops", dstat[i].device_name);
                        if (unlikely(!st)) {
                            st = rrdset_create("disk_ops", dstat[i].device_name, NULL, dstat[i].device_name, "disk.ops", "Disk Completed I/O Operations", "operations/s", 2001, update_every, RRDSET_TYPE_LINE);
                            st->isdetail = 1;

                            rrddim_add(st, "reads", NULL, 1, 1, RRDDIM_INCREMENTAL);
                            rrddim_add(st, "writes", NULL, -1, 1, RRDDIM_INCREMENTAL);
                        }
                        else rrdset_next(st);

                        prev_dstat.operations_read = rrddim_set(st, "reads", dstat[i].operations[DEVSTAT_READ]);
                        prev_dstat.operations_write = rrddim_set(st, "writes", dstat[i].operations[DEVSTAT_WRITE]);
                        rrdset_done(st);

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype("disk_qops", dstat[i].device_name);
                        if (unlikely(!st)) {
                            st = rrdset_create("disk_qops", dstat[i].device_name, NULL, dstat[i].device_name, "disk.qops", "Disk Current I/O Operations", "operations", 2002, update_every, RRDSET_TYPE_LINE);
                            st->isdetail = 1;

                            rrddim_add(st, "operations", NULL, 1, 1, RRDDIM_ABSOLUTE);
                        }
                        else rrdset_next(st);

                        rrddim_set(st, "operations", dstat[i].start_count - dstat[i].end_count);
                        rrdset_done(st);

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype("disk_util", dstat[i].device_name);
                        if (unlikely(!st)) {
                            st = rrdset_create("disk_util", dstat[i].device_name, NULL, dstat[i].device_name, "disk.util", "Disk Utilization Time", "% of time working", 2004, update_every, RRDSET_TYPE_AREA);
                            st->isdetail = 1;

                            rrddim_add(st, "utilization", NULL, 1, 10, RRDDIM_INCREMENTAL);
                        }
                        else rrdset_next(st);

                        cur_dstat.busy_time_ms = dstat[i].busy_time.sec * 1000 + dstat[i].busy_time.frac * BINTIME_SCALE;
                        prev_dstat.busy_time_ms = rrddim_set(st, "utilization", cur_dstat.busy_time_ms);
                        rrdset_done(st);

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype("disk_iotime", dstat[i].device_name);
                        if (unlikely(!st)) {
                            st = rrdset_create("disk_iotime", dstat[i].device_name, NULL, dstat[i].device_name, "disk.iotime", "Disk Total I/O Time", "milliseconds/s", 2022, update_every, RRDSET_TYPE_LINE);
                            st->isdetail = 1;

                            rrddim_add(st, "reads", NULL, 1, 1, RRDDIM_INCREMENTAL);
                            rrddim_add(st, "writes", NULL, -1, 1, RRDDIM_INCREMENTAL);
                        }
                        else rrdset_next(st);

                        cur_dstat.duration_read_ms = dstat[i].duration[DEVSTAT_READ].sec * 1000 + dstat[i].duration[DEVSTAT_READ].frac * BINTIME_SCALE;
                        cur_dstat.duration_write_ms = dstat[i].duration[DEVSTAT_WRITE].sec * 1000 + dstat[i].duration[DEVSTAT_READ].frac * BINTIME_SCALE;
                        prev_dstat.duration_read_ms = rrddim_set(st, "reads", cur_dstat.duration_read_ms);
                        prev_dstat.duration_write_ms = rrddim_set(st, "writes", cur_dstat.duration_write_ms);
                        rrdset_done(st);

                        // --------------------------------------------------------------------
                        // calculate differential charts
                        // only if this is not the first time we run

                        if (likely(dt)) {

                            // --------------------------------------------------------------------

                            st = rrdset_find_bytype("disk_await", dstat[i].device_name);
                            if (unlikely(!st)) {
                                st = rrdset_create("disk_await", dstat[i].device_name, NULL, dstat[i].device_name, "disk.await", "Average Completed I/O Operation Time", "ms per operation", 2005, update_every, RRDSET_TYPE_LINE);
                                st->isdetail = 1;

                                rrddim_add(st, "reads", NULL, 1, 1, RRDDIM_ABSOLUTE);
                                rrddim_add(st, "writes", NULL, -1, 1, RRDDIM_ABSOLUTE);
                            }
                            else rrdset_next(st);

                            rrddim_set(st, "reads", (dstat[i].operations[DEVSTAT_READ] - prev_dstat.operations_read) ? 
                                (cur_dstat.duration_read_ms - prev_dstat.duration_read_ms) / (dstat[i].operations[DEVSTAT_READ] - prev_dstat.operations_read) : 0);
                            rrddim_set(st, "writes", (dstat[i].operations[DEVSTAT_WRITE] - prev_dstat.operations_write) ?
                                (cur_dstat.duration_write_ms - prev_dstat.duration_write_ms) / (dstat[i].operations[DEVSTAT_WRITE] - prev_dstat.operations_write) : 0);
                            rrdset_done(st);

                            // --------------------------------------------------------------------

                            st = rrdset_find_bytype("disk_avgsz", dstat[i].device_name);
                            if (unlikely(!st)) {
                                st = rrdset_create("disk_avgsz", dstat[i].device_name, NULL, dstat[i].device_name, "disk.avgsz", "Average Completed I/O Operation Bandwidth", "kilobytes per operation", 2006, update_every, RRDSET_TYPE_AREA);
                                st->isdetail = 1;

                                rrddim_add(st, "reads", NULL, 1, 1024, RRDDIM_ABSOLUTE);
                                rrddim_add(st, "writes", NULL, -1, 1024, RRDDIM_ABSOLUTE);
                            }
                            else rrdset_next(st);

                            rrddim_set(st, "reads", (dstat[i].operations[DEVSTAT_READ] - prev_dstat.operations_read) ?
                                (dstat[i].bytes[DEVSTAT_READ] - prev_dstat.bytes_read) / (dstat[i].operations[DEVSTAT_READ] - prev_dstat.operations_read) : 0);
                            rrddim_set(st, "writes", (dstat[i].operations[DEVSTAT_WRITE] - prev_dstat.operations_write) ?
                                (dstat[i].bytes[DEVSTAT_WRITE] - prev_dstat.bytes_write) / (dstat[i].operations[DEVSTAT_WRITE] - prev_dstat.operations_write) : 0);
                            rrdset_done(st);

                            // --------------------------------------------------------------------

                            st = rrdset_find_bytype("disk_svctm", dstat[i].device_name);
                            if (unlikely(!st)) {
                                st = rrdset_create("disk_svctm", dstat[i].device_name, NULL, dstat[i].device_name, "disk.svctm", "Average Service Time", "ms per operation", 2007, update_every, RRDSET_TYPE_LINE);
                                st->isdetail = 1;

                                rrddim_add(st, "svctm", NULL, 1, 1, RRDDIM_ABSOLUTE);
                            }
                            else rrdset_next(st);

                            rrddim_set(st, "svctm", ((dstat[i].operations[DEVSTAT_READ] - prev_dstat.operations_read) + (dstat[i].operations[DEVSTAT_WRITE] - prev_dstat.operations_write)) ?
                                (cur_dstat.busy_time_ms - prev_dstat.busy_time_ms) / ((dstat[i].operations[DEVSTAT_READ] - prev_dstat.operations_read) + (dstat[i].operations[DEVSTAT_WRITE] - prev_dstat.operations_write)) : 0);
                            rrdset_done(st);
                        }
                    }

                    // --------------------------------------------------------------------

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
            }
        }
    }

    // --------------------------------------------------------------------


    if (likely(do_swap)) {
        mibsize = sizeof mib / sizeof mib[0];
        if (unlikely(sysctlnametomib("vm.swap_info", mib, &mibsize) == -1)) {
            error("FREEBSD: sysctl(%s...) failed: %s", "vm.swap_info", strerror(errno));
            do_swap = 0;
            error("DISABLED: disk.io");
        } else {
            for (i = 0; ; i++) {
                mib[mibsize] = i;
                size = sizeof(xsw);
                if (unlikely(sysctl(mib, mibsize + 1, &xsw, &size, NULL, 0) == -1 )) {
                    if (unlikely(errno != ENOENT)) {
                        error("FREEBSD: sysctl(%s...) failed: %s", "vm.swap_info", strerror(errno));
                        do_swap = 0;
                        error("DISABLED: disk.io");
                    } else {
                        if (unlikely(size != sizeof(xsw))) {
                            error("FREEBSD: sysctl(%s...) expected %lu, got %lu", "vm.swap_info", (unsigned long)sizeof(xsw), (unsigned long)size);
                            do_swap = 0;
                            error("DISABLED: disk.io");
                        } else break;
                    }
                }
                total_xsw.bytes_used += xsw.xsw_used * system_pagesize;
                total_xsw.bytes_total += xsw.xsw_nblks * system_pagesize;
            }

            if (likely(do_swap)) {
                st = rrdset_find("system.swap");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "swap", NULL, "swap", NULL, "System Swap", "MB", 201, update_every, RRDSET_TYPE_STACKED);
                    st->isdetail = 1;

                    rrddim_add(st, "free",    NULL, 1, 1048576, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "used",    NULL, 1, 1048576, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "used", total_xsw.bytes_used);
                rrddim_set(st, "free", total_xsw.bytes_total - total_xsw.bytes_used);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_ram)) {
        if (unlikely(GETSYSCTL("vm.stats.vm.v_active_count",    vmmeter_data.v_active_count) ||
                     GETSYSCTL("vm.stats.vm.v_inactive_count",  vmmeter_data.v_inactive_count) ||
                     GETSYSCTL("vm.stats.vm.v_wire_count",      vmmeter_data.v_wire_count) ||
                     GETSYSCTL("vm.stats.vm.v_cache_count",     vmmeter_data.v_cache_count) ||
                     GETSYSCTL("vfs.bufspace",                  vfs_bufspace_count) ||
                     GETSYSCTL("vm.stats.vm.v_free_count",      vmmeter_data.v_free_count))) {
            do_swapio = 0;
            error("DISABLED: system.swapio");
        } else {
            st = rrdset_find("system.ram");
            if (unlikely(!st)) {
                st = rrdset_create("system", "ram", NULL, "ram", NULL, "System RAM", "MB", 200, update_every, RRDSET_TYPE_STACKED);

                rrddim_add(st, "active",    NULL, system_pagesize, 1024, RRDDIM_ABSOLUTE);
                rrddim_add(st, "inactive",  NULL, system_pagesize, 1024, RRDDIM_ABSOLUTE);
                rrddim_add(st, "wired",     NULL, system_pagesize, 1024, RRDDIM_ABSOLUTE);
                rrddim_add(st, "cache",     NULL, system_pagesize, 1024, RRDDIM_ABSOLUTE);
                rrddim_add(st, "buffers",   NULL, 1, 1024, RRDDIM_ABSOLUTE);
                rrddim_add(st, "free",      NULL, system_pagesize, 1024, RRDDIM_ABSOLUTE);
            }
            else rrdset_next(st);

            rrddim_set(st, "active",    vmmeter_data.v_active_count);
            rrddim_set(st, "inactive",  vmmeter_data.v_inactive_count);
            rrddim_set(st, "wired",     vmmeter_data.v_wire_count);
            rrddim_set(st, "cache",     vmmeter_data.v_cache_count);
            rrddim_set(st, "buffers",   vfs_bufspace_count);
            rrddim_set(st, "free",      vmmeter_data.v_free_count);
            rrdset_done(st);
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_swapio)) {
        if (unlikely(GETSYSCTL("vm.stats.vm.v_swappgsin", vmmeter_data.v_swappgsin) || GETSYSCTL("vm.stats.vm.v_swappgsout", vmmeter_data.v_swappgsout))) {
            do_swapio = 0;
            error("DISABLED: system.swapio");
        } else {
            st = rrdset_find("system.swapio");
            if (unlikely(!st)) {
                st = rrdset_create("system", "swapio", NULL, "swap", NULL, "Swap I/O", "kilobytes/s", 250, update_every, RRDSET_TYPE_AREA);

                rrddim_add(st, "in",  NULL, system_pagesize, 1024, RRDDIM_INCREMENTAL);
                rrddim_add(st, "out", NULL, -system_pagesize, 1024, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "in", vmmeter_data.v_swappgsin);
            rrddim_set(st, "out", vmmeter_data.v_swappgsout);
            rrdset_done(st);
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_pgfaults)) {
        if (unlikely(GETSYSCTL("vm.stats.vm.v_vm_faults",   vmmeter_data.v_vm_faults) ||
                     GETSYSCTL("vm.stats.vm.v_io_faults",   vmmeter_data.v_io_faults) ||
                     GETSYSCTL("vm.stats.vm.v_cow_faults",  vmmeter_data.v_cow_faults) ||
                     GETSYSCTL("vm.stats.vm.v_cow_optim",   vmmeter_data.v_cow_optim) ||
                     GETSYSCTL("vm.stats.vm.v_intrans",     vmmeter_data.v_intrans))) {
            do_pgfaults = 0;
            error("DISABLED: mem.pgfaults");
        } else {
            st = rrdset_find("mem.pgfaults");
            if (unlikely(!st)) {
                st = rrdset_create("mem", "pgfaults", NULL, "system", NULL, "Memory Page Faults", "page faults/s", 500, update_every, RRDSET_TYPE_LINE);
                st->isdetail = 1;

                rrddim_add(st, "memory", NULL, 1, 1, RRDDIM_INCREMENTAL);
                rrddim_add(st, "io_requiring", NULL, 1, 1, RRDDIM_INCREMENTAL);
                rrddim_add(st, "cow", NULL, 1, 1, RRDDIM_INCREMENTAL);
                rrddim_add(st, "cow_optimized", NULL, 1, 1, RRDDIM_INCREMENTAL);
                rrddim_add(st, "in_transit", NULL, 1, 1, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "memory", vmmeter_data.v_vm_faults);
            rrddim_set(st, "io_requiring", vmmeter_data.v_io_faults);
            rrddim_set(st, "cow", vmmeter_data.v_cow_faults);
            rrddim_set(st, "cow_optimized", vmmeter_data.v_cow_optim);
            rrddim_set(st, "in_transit", vmmeter_data.v_intrans);
            rrdset_done(st);
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_ipc_semaphores)) {
        if (unlikely(GETSYSCTL("kern.ipc.semmni", ipc_sem.semmni))) {
            do_ipc_semaphores = 0;
            error("DISABLED: system.ipc_semaphores");
            error("DISABLED: system.ipc_semaphore_arrays");
        } else {
            ipc_sem_data = reallocz(ipc_sem_data, sizeof(struct semid_kernel) * ipc_sem.semmni);
            if (unlikely(getsysctl("kern.ipc.sema", ipc_sem_data, sizeof(struct semid_kernel) * ipc_sem.semmni))) {
                do_ipc_semaphores = 0;
                error("DISABLED: system.ipc_semaphores");
                error("DISABLED: system.ipc_semaphore_arrays");
            } else {
                for (i = 0; i < ipc_sem.semmni; i++) {
                    if (unlikely(ipc_sem_data[i].u.sem_perm.mode & SEM_ALLOC)) {
                        ipc_sem.sets += 1;
                        ipc_sem.semaphores += ipc_sem_data[i].u.sem_nsems;
                    }
                }

                // --------------------------------------------------------------------

                st = rrdset_find("system.ipc_semaphores");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "ipc_semaphores", NULL, "ipc semaphores", NULL, "IPC Semaphores", "semaphores", 1000, rrd_update_every, RRDSET_TYPE_AREA);
                    rrddim_add(st, "semaphores", NULL, 1, 1, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "semaphores", ipc_sem.semaphores);
                rrdset_done(st);

                // --------------------------------------------------------------------

                st = rrdset_find("system.ipc_semaphore_arrays");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "ipc_semaphore_arrays", NULL, "ipc semaphores", NULL, "IPC Semaphore Arrays", "arrays", 1000, rrd_update_every, RRDSET_TYPE_AREA);
                    rrddim_add(st, "arrays", NULL, 1, 1, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "arrays", ipc_sem.sets);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_ipc_shared_mem)) {
        if (unlikely(GETSYSCTL("kern.ipc.shmmni", ipc_shm.shmmni))) {
            do_ipc_shared_mem = 0;
            error("DISABLED: system.ipc_shared_mem_segs");
            error("DISABLED: system.ipc_shared_mem_size");
        } else {
            ipc_shm_data = reallocz(ipc_shm_data, sizeof(struct shmid_kernel) * ipc_shm.shmmni);
            if (unlikely(getsysctl("kern.ipc.shmsegs", ipc_shm_data, sizeof(struct shmid_kernel) * ipc_shm.shmmni))) {
                do_ipc_shared_mem = 0;
                error("DISABLED: system.ipc_shared_mem_segs");
                error("DISABLED: system.ipc_shared_mem_size");
            } else {
                for (i = 0; i < ipc_shm.shmmni; i++) {
                    if (unlikely(ipc_shm_data[i].u.shm_perm.mode & 0x0800)) {
                        ipc_shm.segs += 1;
                        ipc_shm.segsize += ipc_shm_data[i].u.shm_segsz;
                    }
                }

                // --------------------------------------------------------------------

                st = rrdset_find("system.ipc_shared_mem_segs");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "ipc_shared_mem_segs", NULL, "ipc shared memory", NULL, "IPC Shared Memory Segments", "segments", 1000, rrd_update_every, RRDSET_TYPE_AREA);
                    rrddim_add(st, "segments", NULL, 1, 1, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "segments", ipc_shm.segs);
                rrdset_done(st);

                // --------------------------------------------------------------------

                st = rrdset_find("system.ipc_shared_mem_size");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "ipc_shared_mem_size", NULL, "ipc shared memory", NULL, "IPC Shared Memory Segments Size", "kilobytes", 1000, rrd_update_every, RRDSET_TYPE_AREA);
                    rrddim_add(st, "allocated", NULL, 1, 1024, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "allocated", ipc_shm.segsize);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_ipc_msg_queues)) {
        if (unlikely(GETSYSCTL("kern.ipc.msgmni", ipc_msq.msgmni))) {
            do_ipc_msg_queues = 0;
            error("DISABLED: system.ipc_msq_queues");
            error("DISABLED: system.ipc_msq_messages");
            error("DISABLED: system.ipc_msq_size");
        } else {
            ipc_msq_data = reallocz(ipc_msq_data, sizeof(struct msqid_kernel) * ipc_msq.msgmni);
            if (unlikely(getsysctl("kern.ipc.msqids", ipc_msq_data, sizeof(struct msqid_kernel) * ipc_msq.msgmni))) {
                do_ipc_msg_queues = 0;
                error("DISABLED: system.ipc_msq_queues");
                error("DISABLED: system.ipc_msq_messages");
                error("DISABLED: system.ipc_msq_size");
            } else {
                for (i = 0; i < ipc_msq.msgmni; i++) {
                    if (unlikely(ipc_msq_data[i].u.msg_qbytes != 0)) {
                        ipc_msq.queues += 1;
                        ipc_msq.messages += ipc_msq_data[i].u.msg_qnum;
                        ipc_msq.usedsize += ipc_msq_data[i].u.msg_cbytes;
                        ipc_msq.allocsize += ipc_msq_data[i].u.msg_qbytes;
                    }
                }

                // --------------------------------------------------------------------

                st = rrdset_find("system.ipc_msq_queues");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "ipc_msq_queues", NULL, "ipc message queues", NULL, "Number of IPC Message Queues", "queues", 990, rrd_update_every, RRDSET_TYPE_AREA);
                    rrddim_add(st, "queues", NULL, 1, 1, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "queues", ipc_msq.queues);
                rrdset_done(st);

                // --------------------------------------------------------------------

                st = rrdset_find("system.ipc_msq_messages");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "ipc_msq_messages", NULL, "ipc message queues", NULL, "Number of Messages in IPC Message Queues", "messages", 1000, rrd_update_every, RRDSET_TYPE_AREA);
                    rrddim_add(st, "messages", NULL, 1, 1, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "messages", ipc_msq.messages);
                rrdset_done(st);

                // --------------------------------------------------------------------

                st = rrdset_find("system.ipc_msq_size");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "ipc_msq_size", NULL, "ipc message queues", NULL, "Size of IPC Message Queues", "bytes", 1100, rrd_update_every, RRDSET_TYPE_LINE);
                    rrddim_add(st, "allocated", NULL, 1, 1, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "used", NULL, 1, 1, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "allocated", ipc_msq.allocsize);
                rrddim_set(st, "used", ipc_msq.usedsize);
                rrdset_done(st);

            }
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_netisr || do_netisr_per_core)) {
        if (unlikely(GETSYSCTL("kern.smp.cpus", ncpus))) {
            common_error = 1;
        } else if (unlikely(ncpus > 9999)) {
            error("FREEBSD: There are more than 4 digits in cpu cores number");
            common_error = 1;
        } else if (unlikely(sysctlbyname("net.isr.workstream", NULL, &netisr_workstream_size, NULL, 0) == -1)) {
            error("FREEBSD: sysctl(net.isr.workstream...) failed: %s", strerror(errno));
            common_error = 1;
        } else if (unlikely(sysctlbyname("net.isr.work", NULL, &netisr_work_size, NULL, 0) == -1)) {
            error("FREEBSD: sysctl(net.isr.work...) failed: %s", strerror(errno));
            common_error = 1;
        } else {
            num_netisr_workstreams = netisr_workstream_size / sizeof(struct sysctl_netisr_workstream);
            netisr_workstream = reallocz(netisr_workstream, num_netisr_workstreams * sizeof(struct sysctl_netisr_workstream));
            if (unlikely(getsysctl("net.isr.workstream", netisr_workstream, num_netisr_workstreams * sizeof(struct sysctl_netisr_workstream)))){
                common_error = 1;
            } else {
                num_netisr_works = netisr_work_size / sizeof(struct sysctl_netisr_work);
                netisr_work = reallocz(netisr_work, num_netisr_works * sizeof(struct sysctl_netisr_work));
                if (unlikely(getsysctl("net.isr.work", netisr_work, num_netisr_works * sizeof(struct sysctl_netisr_work)))){
                    common_error = 1;
                }
            }
        }
        if (unlikely(common_error)) {
            do_netisr = 0;
            error("DISABLED: system.softnet_stat");
            do_netisr_per_core = 0;
            error("DISABLED: system.cpuX_softnet_stat");
            common_error = 0;
        } else {
            netisr_stats = reallocz(netisr_stats, (ncpus + 1) * sizeof(struct netisr_stats));
            bzero(netisr_stats, (ncpus + 1) * sizeof(struct netisr_stats));
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
            for (i = 0; i < ncpus; i++) {
                netisr_stats[ncpus].dispatched += netisr_stats[i].dispatched;
                netisr_stats[ncpus].hybrid_dispatched += netisr_stats[i].hybrid_dispatched;
                netisr_stats[ncpus].qdrops += netisr_stats[i].qdrops;
                netisr_stats[ncpus].queued += netisr_stats[i].queued;
            }
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_netisr)) {
        st = rrdset_find_bytype("system", "softnet_stat");
        if (unlikely(!st)) {
            st = rrdset_create("system", "softnet_stat", NULL, "softnet_stat", NULL, "System softnet_stat", "events/s", 955, update_every, RRDSET_TYPE_LINE);
            rrddim_add(st, "dispatched", NULL, 1, 1, RRDDIM_INCREMENTAL);
            rrddim_add(st, "hybrid_dispatched", NULL, 1, 1, RRDDIM_INCREMENTAL);
            rrddim_add(st, "qdrops", NULL, 1, 1, RRDDIM_INCREMENTAL);
            rrddim_add(st, "queued", NULL, 1, 1, RRDDIM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "dispatched", netisr_stats[ncpus].dispatched);
        rrddim_set(st, "hybrid_dispatched", netisr_stats[ncpus].hybrid_dispatched);
        rrddim_set(st, "qdrops", netisr_stats[ncpus].qdrops);
        rrddim_set(st, "queued", netisr_stats[ncpus].queued);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if (likely(do_netisr_per_core)) {
        for (i = 0; i < ncpus ;i++) {
            snprintfz(netstat_cpuid, 21, "cpu%d_softnet_stat", i);

            st = rrdset_find_bytype("cpu", netstat_cpuid);
            if (unlikely(!st)) {
                st = rrdset_create("cpu", netstat_cpuid, NULL, "softnet_stat", NULL, "Per CPU netisr statistics", "events/s", 1101 + i, update_every, RRDSET_TYPE_LINE);
                rrddim_add(st, "dispatched", NULL, 1, 1, RRDDIM_INCREMENTAL);
                rrddim_add(st, "hybrid_dispatched", NULL, 1, 1, RRDDIM_INCREMENTAL);
                rrddim_add(st, "qdrops", NULL, 1, 1, RRDDIM_INCREMENTAL);
                rrddim_add(st, "queued", NULL, 1, 1, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "dispatched", netisr_stats[i].dispatched);
            rrddim_set(st, "hybrid_dispatched", netisr_stats[i].hybrid_dispatched);
            rrddim_set(st, "qdrops", netisr_stats[i].qdrops);
            rrddim_set(st, "queued", netisr_stats[i].queued);
            rrdset_done(st);
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_bandwidth)) {
        if (unlikely(getifaddrs(&ifap))) {
            error("FREEBSD: getifaddrs()");
            do_bandwidth = 0;
            error("DISABLED: system.ipv4");
        } else {
            iftot.ift_ibytes = iftot.ift_obytes = 0;
            for (ifa = ifap; ifa; ifa = ifa->ifa_next) {
                if (ifa->ifa_addr->sa_family != AF_INET)
                        continue;
                iftot.ift_ibytes += IFA_DATA(ibytes);
                iftot.ift_obytes += IFA_DATA(obytes);
            }

            st = rrdset_find("system.ipv4");
            if (unlikely(!st)) {
                st = rrdset_create("system", "ipv4", NULL, "network", NULL, "IPv4 Bandwidth", "kilobits/s", 500, update_every, RRDSET_TYPE_AREA);

                rrddim_add(st, "InOctets", "received", 8, 1024, RRDDIM_INCREMENTAL);
                rrddim_add(st, "OutOctets", "sent", -8, 1024, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "InOctets", iftot.ift_ibytes);
            rrddim_set(st, "OutOctets", iftot.ift_obytes);
            rrdset_done(st);

            // --------------------------------------------------------------------

            iftot.ift_ibytes = iftot.ift_obytes = 0;
            for (ifa = ifap; ifa; ifa = ifa->ifa_next) {
                if (ifa->ifa_addr->sa_family != AF_INET6)
                        continue;
                iftot.ift_ibytes += IFA_DATA(ibytes);
                iftot.ift_obytes += IFA_DATA(obytes);
            }

            st = rrdset_find("system.ipv6");
            if (unlikely(!st)) {
                st = rrdset_create("system", "ipv6", NULL, "network", NULL, "IPv6 Bandwidth", "kilobits/s", 500, update_every, RRDSET_TYPE_AREA);

                rrddim_add(st, "received", NULL, 8, 1024, RRDDIM_INCREMENTAL);
                rrddim_add(st, "sent", NULL, -8, 1024, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "sent", iftot.ift_obytes);
            rrddim_set(st, "received", iftot.ift_ibytes);
            rrdset_done(st);

            for (ifa = ifap; ifa; ifa = ifa->ifa_next) {
                if (ifa->ifa_addr->sa_family != AF_LINK)
                        continue;

                // --------------------------------------------------------------------

                st = rrdset_find_bytype("net", ifa->ifa_name);
                if (unlikely(!st)) {
                    st = rrdset_create("net", ifa->ifa_name, NULL, ifa->ifa_name, "net.net", "Bandwidth", "kilobits/s", 7000, update_every, RRDSET_TYPE_AREA);

                    rrddim_add(st, "received", NULL, 8, 1024, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "sent", NULL, -8, 1024, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "received", IFA_DATA(ibytes));
                rrddim_set(st, "sent", IFA_DATA(obytes));
                rrdset_done(st);

                // --------------------------------------------------------------------

                st = rrdset_find_bytype("net_packets", ifa->ifa_name);
                if (unlikely(!st)) {
                    st = rrdset_create("net_packets", ifa->ifa_name, NULL, ifa->ifa_name, "net.packets", "Packets", "packets/s", 7001, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "multicast_received", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "multicast_sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "received", IFA_DATA(ipackets));
                rrddim_set(st, "sent", IFA_DATA(opackets));
                rrddim_set(st, "multicast_received", IFA_DATA(imcasts));
                rrddim_set(st, "multicast_sent", IFA_DATA(omcasts));
                rrdset_done(st);

                // --------------------------------------------------------------------

                st = rrdset_find_bytype("net_errors", ifa->ifa_name);
                if (unlikely(!st)) {
                    st = rrdset_create("net_errors", ifa->ifa_name, NULL, ifa->ifa_name, "net.errors", "Interface Errors", "errors/s", 7002, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "inbound", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "outbound", NULL, -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "inbound", IFA_DATA(ierrors));
                rrddim_set(st, "outbound", IFA_DATA(oerrors));
                rrdset_done(st);

                // --------------------------------------------------------------------

                st = rrdset_find_bytype("net_drops", ifa->ifa_name);
                if (unlikely(!st)) {
                    st = rrdset_create("net_drops", ifa->ifa_name, NULL, ifa->ifa_name, "net.drops", "Interface Drops", "drops/s", 7003, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "inbound", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "outbound", NULL, -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "inbound", IFA_DATA(iqdrops));
                rrddim_set(st, "outbound", IFA_DATA(oqdrops));
                rrdset_done(st);

                // --------------------------------------------------------------------

                st = rrdset_find_bytype("net_events", ifa->ifa_name);
                if (unlikely(!st)) {
                    st = rrdset_create("net_events", ifa->ifa_name, NULL, ifa->ifa_name, "net.events", "Network Interface Events", "events/s", 7006, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "frames", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "collisions", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "carrier", NULL, -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "collisions", IFA_DATA(collisions));
                rrdset_done(st);
            }

            freeifaddrs(ifap);
        }
    }

    return 0;
}
