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
#include <net/if.h>
#include <ifaddrs.h>
// NEEDED BY do_tcp...
#include <netinet/tcp_var.h>
#include <netinet/tcp_fsm.h>
// NEEDED BY do_udp..., do_ip...
#include <netinet/ip_var.h>
// NEEDED BY do_udp...
#include <netinet/udp.h>
#include <netinet/udp_var.h>
// NEEDED BY do_icmp...
#include <netinet/ip.h>
#include <netinet/ip_icmp.h>
#include <netinet/icmp_var.h>
// NEEDED BY do_ip6...
#include <netinet6/ip6_var.h>
// NEEDED BY do_icmp6...
#include <netinet/icmp6.h>
// NEEDED BY do_space, do_inodes
#include <sys/mount.h>
// NEEDED BY do_uptime
#include <time.h>

#define KILO_FACTOR 1024
#define MEGA_FACTOR 1048576     // 1024 * 1024
#define GIGA_FACTOR 1073741824  // 1024 * 1024 * 1024

#define MAX_INT_DIGITS 10 // maximum number of digits for int

// NEEDED BY: do_disk_io
#define RRD_TYPE_DISK "disk"

// FreeBSD calculates load averages once every 5 seconds
#define MIN_LOADAVG_UPDATE_EVERY 5

// NEEDED BY: do_bandwidth
#define IFA_DATA(s) (((struct if_data *)ifa->ifa_data)->ifi_ ## s)

int do_freebsd_sysctl(int update_every, usec_t dt) {
    static int do_cpu = -1, do_cpu_cores = -1, do_interrupts = -1, do_context = -1, do_forks = -1, do_processes = -1,
        do_loadavg = -1, do_all_processes = -1, do_disk_io = -1, do_swap = -1, do_ram = -1, do_swapio = -1,
        do_pgfaults = -1, do_committed = -1, do_ipc_semaphores = -1, do_ipc_shared_mem = -1, do_ipc_msg_queues = -1,
        do_dev_intr = -1, do_soft_intr = -1, do_netisr = -1, do_netisr_per_core = -1, do_bandwidth = -1,
        do_tcp_sockets = -1, do_tcp_packets = -1, do_tcp_errors = -1, do_tcp_handshake = -1,
        do_ecn = -1, do_tcpext_syscookies = -1, do_tcpext_ofo = -1, do_tcpext_connaborts = -1,
        do_udp_packets = -1, do_udp_errors = -1, do_icmp_packets = -1, do_icmpmsg = -1,
        do_ip_packets = -1, do_ip_fragsout = -1, do_ip_fragsin = -1, do_ip_errors = -1,
        do_ip6_packets = -1, do_ip6_fragsout = -1, do_ip6_fragsin = -1, do_ip6_errors = -1,
        do_icmp6 = -1, do_icmp6_redir = -1, do_icmp6_errors = -1, do_icmp6_echos = -1, do_icmp6_router = -1,
        do_icmp6_neighbor = -1, do_icmp6_types = -1, do_space = -1, do_inodes = -1, do_uptime = -1;

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
        do_tcp_sockets          = config_get_boolean("plugin:freebsd:sysctl", "ipv4 TCP connections", 1);
        do_tcp_packets          = config_get_boolean("plugin:freebsd:sysctl", "ipv4 TCP packets", 1);
        do_tcp_errors           = config_get_boolean("plugin:freebsd:sysctl", "ipv4 TCP errors", 1);
        do_tcp_handshake        = config_get_boolean("plugin:freebsd:sysctl", "ipv4 TCP handshake issues", 1);
        do_ecn                  = config_get_boolean_ondemand("plugin:freebsd:sysctl", "ECN packets", CONFIG_ONDEMAND_ONDEMAND);
        do_tcpext_syscookies    = config_get_boolean_ondemand("plugin:freebsd:sysctl", "TCP SYN cookies", CONFIG_ONDEMAND_ONDEMAND);
        do_tcpext_ofo           = config_get_boolean_ondemand("plugin:freebsd:sysctl", "TCP out-of-order queue", CONFIG_ONDEMAND_ONDEMAND);
        do_tcpext_connaborts    = config_get_boolean_ondemand("plugin:freebsd:sysctl", "TCP connection aborts", CONFIG_ONDEMAND_ONDEMAND);
        do_udp_packets          = config_get_boolean("plugin:freebsd:sysctl", "ipv4 UDP packets", 1);
        do_udp_errors           = config_get_boolean("plugin:freebsd:sysctl", "ipv4 UDP errors", 1);
        do_icmp_packets         = config_get_boolean("plugin:freebsd:sysctl", "ipv4 ICMP packets", 1);
        do_icmpmsg              = config_get_boolean("plugin:freebsd:sysctl", "ipv4 ICMP messages", 1);
        do_ip_packets           = config_get_boolean("plugin:freebsd:sysctl", "ipv4 packets", 1);
        do_ip_fragsout          = config_get_boolean("plugin:freebsd:sysctl", "ipv4 fragments sent", 1);
        do_ip_fragsin           = config_get_boolean("plugin:freebsd:sysctl", "ipv4 fragments assembly", 1);
        do_ip_errors            = config_get_boolean("plugin:freebsd:sysctl", "ipv4 errors", 1);
        do_ip6_packets          = config_get_boolean_ondemand("plugin:freebsd:sysctl", "ipv6 packets", CONFIG_ONDEMAND_ONDEMAND);
        do_ip6_fragsout         = config_get_boolean_ondemand("plugin:freebsd:sysctl", "ipv6 fragments sent", CONFIG_ONDEMAND_ONDEMAND);
        do_ip6_fragsin          = config_get_boolean_ondemand("plugin:freebsd:sysctl", "ipv6 fragments assembly", CONFIG_ONDEMAND_ONDEMAND);
        do_ip6_errors           = config_get_boolean_ondemand("plugin:freebsd:sysctl", "ipv6 errors", CONFIG_ONDEMAND_ONDEMAND);
        do_icmp6                = config_get_boolean_ondemand("plugin:freebsd:sysctl", "icmp", CONFIG_ONDEMAND_ONDEMAND);
        do_icmp6_redir          = config_get_boolean_ondemand("plugin:freebsd:sysctl", "icmp redirects", CONFIG_ONDEMAND_ONDEMAND);
        do_icmp6_errors         = config_get_boolean_ondemand("plugin:freebsd:sysctl", "icmp errors", CONFIG_ONDEMAND_ONDEMAND);
        do_icmp6_echos          = config_get_boolean_ondemand("plugin:freebsd:sysctl", "icmp echos", CONFIG_ONDEMAND_ONDEMAND);
        do_icmp6_router         = config_get_boolean_ondemand("plugin:freebsd:sysctl", "icmp router", CONFIG_ONDEMAND_ONDEMAND);
        do_icmp6_neighbor       = config_get_boolean_ondemand("plugin:freebsd:sysctl", "icmp neighbor", CONFIG_ONDEMAND_ONDEMAND);
        do_icmp6_types          = config_get_boolean_ondemand("plugin:freebsd:sysctl", "icmp types", CONFIG_ONDEMAND_ONDEMAND);
        do_space                = config_get_boolean("plugin:freebsd:sysctl", "space usage for all disks", 1);
        do_inodes               = config_get_boolean("plugin:freebsd:sysctl", "inodes usage for all disks", 1);
        do_uptime               = config_get_boolean("plugin:freebsd:sysctl", "system uptime", 1);
    }

    RRDSET *st;
    RRDDIM *rd;

    int system_pagesize = getpagesize(); // wouldn't it be better to get value directly from hw.pagesize?
    int i, n;
    void *p;
    int common_error = 0;
    size_t size;
    char title[4096 + 1];

    // NEEDED BY: do_loadavg
    static usec_t next_loadavg_dt = 0;
    struct loadavg sysload;

    // NEEDED BY: do_cpu, do_cpu_cores
    long cp_time[CPUSTATES];

    // NEEDED BY: du_cpu_cores, do_netisr, do_netisr_per_core
    int ncpus;

    // NEEDED BY: do_cpu_cores
    static long *pcpu_cp_time = NULL;
    char cpuid[MAX_INT_DIGITS + 1];

    // NEEDED BY: do_all_processes, do_processes
    struct vmtotal vmtotal_data;

    // NEEDED BY: do_context, do_forks
    u_int u_int_data;

    // NEEDED BY: do_interrupts
    size_t intrcnt_size;
    unsigned long nintr = 0;
    static unsigned long *intrcnt = NULL;
    static char *intrnames = NULL;
    unsigned long long totalintr = 0;

    // NEEDED BY: do_disk_io
    #define BINTIME_SCALE 5.42101086242752217003726400434970855712890625e-17 // this is 1000/2^64
    int numdevs;
    static void *devstat_data = NULL;
    struct devstat *dstat;
    char disk[DEVSTAT_NAME_LEN + MAX_INT_DIGITS + 1];
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

    // NEEDED BY: do_tcp...
    struct tcpstat tcpstat;
    uint64_t tcps_states[TCP_NSTATES];

    // NEEDED BY: do_udp...
    struct udpstat udpstat;

    // NEEDED BY: do_icmp...
    struct icmpstat icmpstat;
    struct icmp_total {
        u_long  msgs_in;
        u_long  msgs_out;
    } icmp_total = {0, 0};

    // NEEDED BY: do_ip...
    struct ipstat ipstat;

    // NEEDED BY: do_ip6...
    struct ip6stat ip6stat;

    // NEEDED BY: do_icmp6...
    struct icmp6stat icmp6stat;
    struct icmp6_total {
        u_long  msgs_in;
        u_long  msgs_out;
    } icmp6_total = {0, 0};

    // NEEDED BY: do_space, do_inodes
    struct statfs *mntbuf;
    int mntsize;
    char mntonname[MNAMELEN + 1];

    // NEEDED BY: do_uptime
    struct timespec boot_time, cur_time;

    // --------------------------------------------------------------------

    if (next_loadavg_dt <= dt) {
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

                next_loadavg_dt = st->update_every * USEC_PER_SEC;
            }
        }
    }
    else next_loadavg_dt -= dt;

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

                    rrddim_add(st, "Committed_AS", NULL, system_pagesize, MEGA_FACTOR, RRDDIM_ABSOLUTE);
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
                if (unlikely(getsysctl("kern.cp_times", pcpu_cp_time, sizeof(cp_time) * ncpus))) {
                    do_cpu_cores = 0;
                    error("DISABLED: cpu.cpuXX");
                } else {
                    for (i = 0; i < ncpus; i++) {
                        snprintfz(cpuid, MAX_INT_DIGITS, "cpu%d", i);
                        st = rrdset_find_bytype("cpu", cpuid);
                        if (unlikely(!st)) {
                            st = rrdset_create("cpu", cpuid, NULL, "utilization", "cpu.cpu", "Core utilization",
                                               "percentage", 1000, update_every, RRDSET_TYPE_STACKED);

                            rrddim_add(st, "user", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                            rrddim_add(st, "nice", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                            rrddim_add(st, "system", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                            rrddim_add(st, "interrupt", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                            rrddim_add(st, "idle", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
                            rrddim_hide(st, "idle");
                        } else
                            rrdset_next(st);

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

                // --------------------------------------------------------------------

                size = nintr * (MAXCOMLEN +1);
                intrnames = reallocz(intrnames, size);
                if (unlikely(getsysctl("hw.intrnames", intrnames, size))) {
                    do_interrupts = 0;
                    error("DISABLED: system.intr");
                } else {
                    st = rrdset_find_bytype("system", "interrupts");
                    if (unlikely(!st))
                        st = rrdset_create("system", "interrupts", NULL, "interrupts", NULL, "System interrupts", "interrupts/s",
                                           1000, update_every, RRDSET_TYPE_STACKED);
                    else
                        rrdset_next(st);

                    for (i = 0; i < nintr; i++) {
                        p = intrnames + i * (MAXCOMLEN + 1);
                        if (unlikely((intrcnt[i] != 0) && (*(char*)p != 0))) {
                            rd = rrddim_find(st, p);
                            if (unlikely(!rd))
                                rd = rrddim_add(st, p, NULL, 1, 1, RRDDIM_INCREMENTAL);
                            rrddim_set_by_pointer(st, rd, intrcnt[i]);
                        }
                    }
                    rrdset_done(st);
                }
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
                collected_number total_disk_kbytes_read = 0;
                collected_number total_disk_kbytes_write = 0;

                for (i = 0; i < numdevs; i++) {
                    if (((dstat[i].device_type & DEVSTAT_TYPE_MASK) == DEVSTAT_TYPE_DIRECT) || ((dstat[i].device_type & DEVSTAT_TYPE_MASK) == DEVSTAT_TYPE_STORARRAY)) {
                        sprintf(disk, "%s%d", dstat[i].device_name, dstat[i].unit_number);

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype(RRD_TYPE_DISK, disk);
                        if (unlikely(!st)) {
                            st = rrdset_create(RRD_TYPE_DISK, disk, NULL, disk, "disk.io", "Disk I/O Bandwidth", "kilobytes/s", 2000, update_every, RRDSET_TYPE_AREA);

                            rrddim_add(st, "reads", NULL, 1, 1024, RRDDIM_INCREMENTAL);
                            rrddim_add(st, "writes", NULL, -1, 1024, RRDDIM_INCREMENTAL);
                        }
                        else rrdset_next(st);

                        total_disk_kbytes_read += dstat[i].bytes[DEVSTAT_READ]/KILO_FACTOR;
                        total_disk_kbytes_write += dstat[i].bytes[DEVSTAT_WRITE]/KILO_FACTOR;
                        prev_dstat.bytes_read = rrddim_set(st, "reads", dstat[i].bytes[DEVSTAT_READ]);
                        prev_dstat.bytes_write = rrddim_set(st, "writes", dstat[i].bytes[DEVSTAT_WRITE]);
                        rrdset_done(st);

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype("disk_ops", disk);
                        if (unlikely(!st)) {
                            st = rrdset_create("disk_ops", disk, NULL, disk, "disk.ops", "Disk Completed I/O Operations", "operations/s", 2001, update_every, RRDSET_TYPE_LINE);
                            st->isdetail = 1;

                            rrddim_add(st, "reads", NULL, 1, 1, RRDDIM_INCREMENTAL);
                            rrddim_add(st, "writes", NULL, -1, 1, RRDDIM_INCREMENTAL);
                        }
                        else rrdset_next(st);

                        prev_dstat.operations_read = rrddim_set(st, "reads", dstat[i].operations[DEVSTAT_READ]);
                        prev_dstat.operations_write = rrddim_set(st, "writes", dstat[i].operations[DEVSTAT_WRITE]);
                        rrdset_done(st);

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype("disk_qops", disk);
                        if (unlikely(!st)) {
                            st = rrdset_create("disk_qops", disk, NULL, disk, "disk.qops", "Disk Current I/O Operations", "operations", 2002, update_every, RRDSET_TYPE_LINE);
                            st->isdetail = 1;

                            rrddim_add(st, "operations", NULL, 1, 1, RRDDIM_ABSOLUTE);
                        }
                        else rrdset_next(st);

                        rrddim_set(st, "operations", dstat[i].start_count - dstat[i].end_count);
                        rrdset_done(st);

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype("disk_util", disk);
                        if (unlikely(!st)) {
                            st = rrdset_create("disk_util", disk, NULL, disk, "disk.util", "Disk Utilization Time", "% of time working", 2004, update_every, RRDSET_TYPE_AREA);
                            st->isdetail = 1;

                            rrddim_add(st, "utilization", NULL, 1, 10, RRDDIM_INCREMENTAL);
                        }
                        else rrdset_next(st);

                        cur_dstat.busy_time_ms = dstat[i].busy_time.sec * 1000 + dstat[i].busy_time.frac * BINTIME_SCALE;
                        prev_dstat.busy_time_ms = rrddim_set(st, "utilization", cur_dstat.busy_time_ms);
                        rrdset_done(st);

                        // --------------------------------------------------------------------

                        st = rrdset_find_bytype("disk_iotime", disk);
                        if (unlikely(!st)) {
                            st = rrdset_create("disk_iotime", disk, NULL, disk, "disk.iotime", "Disk Total I/O Time", "milliseconds/s", 2022, update_every, RRDSET_TYPE_LINE);
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

                            st = rrdset_find_bytype("disk_await", disk);
                            if (unlikely(!st)) {
                                st = rrdset_create("disk_await", disk, NULL, disk, "disk.await", "Average Completed I/O Operation Time", "ms per operation", 2005, update_every, RRDSET_TYPE_LINE);
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

                            st = rrdset_find_bytype("disk_avgsz", disk);
                            if (unlikely(!st)) {
                                st = rrdset_create("disk_avgsz", disk, NULL, disk, "disk.avgsz", "Average Completed I/O Operation Bandwidth", "kilobytes per operation", 2006, update_every, RRDSET_TYPE_AREA);
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

                            st = rrdset_find_bytype("disk_svctm", disk);
                            if (unlikely(!st)) {
                                st = rrdset_create("disk_svctm", disk, NULL, disk, "disk.svctm", "Average Service Time", "ms per operation", 2007, update_every, RRDSET_TYPE_LINE);
                                st->isdetail = 1;

                                rrddim_add(st, "svctm", NULL, 1, 1, RRDDIM_ABSOLUTE);
                            }
                            else rrdset_next(st);

                            rrddim_set(st, "svctm", ((dstat[i].operations[DEVSTAT_READ] - prev_dstat.operations_read) + (dstat[i].operations[DEVSTAT_WRITE] - prev_dstat.operations_write)) ?
                                (cur_dstat.busy_time_ms - prev_dstat.busy_time_ms) / ((dstat[i].operations[DEVSTAT_READ] - prev_dstat.operations_read) + (dstat[i].operations[DEVSTAT_WRITE] - prev_dstat.operations_write)) : 0);
                            rrdset_done(st);
                        }
                    }
                }

                // --------------------------------------------------------------------

                st = rrdset_find_bytype("system", "io");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "io", NULL, "disk", NULL, "Disk I/O", "kilobytes/s", 150, update_every, RRDSET_TYPE_AREA);
                    rrddim_add(st, "in",  NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "out", NULL, -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "in", total_disk_kbytes_read);
                rrddim_set(st, "out", total_disk_kbytes_write);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------


    if (likely(do_swap)) {
        mibsize = sizeof mib / sizeof mib[0];
        if (unlikely(sysctlnametomib("vm.swap_info", mib, &mibsize) == -1)) {
            error("FREEBSD: sysctl(%s...) failed: %s", "vm.swap_info", strerror(errno));
            do_swap = 0;
            error("DISABLED: system.swap");
        } else {
            for (i = 0; ; i++) {
                mib[mibsize] = i;
                size = sizeof(xsw);
                if (unlikely(sysctl(mib, mibsize + 1, &xsw, &size, NULL, 0) == -1 )) {
                    if (unlikely(errno != ENOENT)) {
                        error("FREEBSD: sysctl(%s...) failed: %s", "vm.swap_info", strerror(errno));
                        do_swap = 0;
                        error("DISABLED: system.swap");
                    } else {
                        if (unlikely(size != sizeof(xsw))) {
                            error("FREEBSD: sysctl(%s...) expected %lu, got %lu", "vm.swap_info", (unsigned long)sizeof(xsw), (unsigned long)size);
                            do_swap = 0;
                            error("DISABLED: system.swap");
                        } else break;
                    }
                }
                total_xsw.bytes_used += xsw.xsw_used;
                total_xsw.bytes_total += xsw.xsw_nblks;
            }

            if (likely(do_swap)) {
                st = rrdset_find("system.swap");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "swap", NULL, "swap", NULL, "System Swap", "MB", 201, update_every, RRDSET_TYPE_STACKED);
                    st->isdetail = 1;

                    rrddim_add(st, "free",    NULL, system_pagesize, MEGA_FACTOR, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "used",    NULL, system_pagesize, MEGA_FACTOR, RRDDIM_ABSOLUTE);
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
#if __FreeBSD_version < 1200016
                     GETSYSCTL("vm.stats.vm.v_cache_count",     vmmeter_data.v_cache_count) ||
#endif
                     GETSYSCTL("vfs.bufspace",                  vfs_bufspace_count) ||
                     GETSYSCTL("vm.stats.vm.v_free_count",      vmmeter_data.v_free_count))) {
            do_ram = 0;
            error("DISABLED: system.ram");
        } else {
            st = rrdset_find("system.ram");
            if (unlikely(!st)) {
                st = rrdset_create("system", "ram", NULL, "ram", NULL, "System RAM", "MB", 200, update_every, RRDSET_TYPE_STACKED);

                rrddim_add(st, "active",    NULL, system_pagesize, MEGA_FACTOR, RRDDIM_ABSOLUTE);
                rrddim_add(st, "inactive",  NULL, system_pagesize, MEGA_FACTOR, RRDDIM_ABSOLUTE);
                rrddim_add(st, "wired",     NULL, system_pagesize, MEGA_FACTOR, RRDDIM_ABSOLUTE);
#if __FreeBSD_version < 1200016
                rrddim_add(st, "cache",     NULL, system_pagesize, MEGA_FACTOR, RRDDIM_ABSOLUTE);
#endif
                rrddim_add(st, "buffers",   NULL, 1, MEGA_FACTOR, RRDDIM_ABSOLUTE);
                rrddim_add(st, "free",      NULL, system_pagesize, MEGA_FACTOR, RRDDIM_ABSOLUTE);
            }
            else rrdset_next(st);

            rrddim_set(st, "active",    vmmeter_data.v_active_count);
            rrddim_set(st, "inactive",  vmmeter_data.v_inactive_count);
            rrddim_set(st, "wired",     vmmeter_data.v_wire_count);
#if __FreeBSD_version < 1200016
            rrddim_set(st, "cache",     vmmeter_data.v_cache_count);
#endif
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
#if __FreeBSD__ >= 11
                    rrddim_add(st, "outbound", NULL, -1, 1, RRDDIM_INCREMENTAL);
#endif
                }
                else rrdset_next(st);

                rrddim_set(st, "inbound", IFA_DATA(iqdrops));
#if __FreeBSD__ >= 11
                rrddim_set(st, "outbound", IFA_DATA(oqdrops));
#endif
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

    // --------------------------------------------------------------------

    // see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
    if (likely(do_tcp_sockets)) {
        if (unlikely(GETSYSCTL("net.inet.tcp.states", tcps_states))) {
            do_tcp_sockets = 0;
            error("DISABLED: ipv4.tcpsock");
        } else {
            if (likely(do_tcp_sockets)) {
                st = rrdset_find("ipv4.tcpsock");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcpsock", NULL, "tcp", NULL, "IPv4 TCP Connections",
                                       "active connections", 2500, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "CurrEstab", "connections", 1, 1, RRDDIM_ABSOLUTE);
                } else
                    rrdset_next(st);

                rrddim_set(st, "CurrEstab", tcps_states[TCPS_ESTABLISHED]);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------

    // see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
    if (likely(do_tcp_packets || do_tcp_errors || do_tcp_handshake || do_tcpext_connaborts || do_tcpext_ofo || do_tcpext_syscookies || do_ecn)) {
        if (unlikely(GETSYSCTL("net.inet.tcp.stats", tcpstat))){
            do_tcp_packets = 0;
            error("DISABLED: ipv4.tcppackets");
            do_tcp_errors = 0;
            error("DISABLED: ipv4.tcperrors");
            do_tcp_handshake = 0;
            error("DISABLED: ipv4.tcphandshake");
            do_tcpext_connaborts = 0;
            error("DISABLED: ipv4.tcpconnaborts");
            do_tcpext_ofo = 0;
            error("DISABLED: ipv4.tcpofo");
            do_tcpext_syscookies = 0;
            error("DISABLED: ipv4.tcpsyncookies");
            do_ecn = 0;
            error("DISABLED: ipv4.ecnpkts");
        } else {
            if (likely(do_tcp_packets)) {
                st = rrdset_find("ipv4.tcppackets");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcppackets", NULL, "tcp", NULL, "IPv4 TCP Packets",
                                       "packets/s",
                                       2600, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InSegs", "received", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutSegs", "sent", -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InSegs", tcpstat.tcps_rcvtotal);
                rrddim_set(st, "OutSegs", tcpstat.tcps_sndtotal);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_tcp_errors)) {
                st = rrdset_find("ipv4.tcperrors");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcperrors", NULL, "tcp", NULL, "IPv4 TCP Errors",
                                       "packets/s",
                                       2700, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "InErrs", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "RetransSegs", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

#if __FreeBSD__ >= 11
                rrddim_set(st, "InErrs", tcpstat.tcps_rcvbadoff + tcpstat.tcps_rcvreassfull + tcpstat.tcps_rcvshort);
#else
                rrddim_set(st, "InErrs", tcpstat.tcps_rcvbadoff + tcpstat.tcps_rcvshort);
#endif
                rrddim_set(st, "InCsumErrors", tcpstat.tcps_rcvbadsum);
                rrddim_set(st, "RetransSegs", tcpstat.tcps_sndrexmitpack);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_tcp_handshake)) {
                st = rrdset_find("ipv4.tcphandshake");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcphandshake", NULL, "tcp", NULL,
                                       "IPv4 TCP Handshake Issues",
                                       "events/s", 2900, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "EstabResets", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "ActiveOpens", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "PassiveOpens", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "AttemptFails", NULL, 1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "EstabResets", tcpstat.tcps_drops);
                rrddim_set(st, "ActiveOpens", tcpstat.tcps_connattempt);
                rrddim_set(st, "PassiveOpens", tcpstat.tcps_accepts);
                rrddim_set(st, "AttemptFails", tcpstat.tcps_conndrops);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_tcpext_connaborts == CONFIG_ONDEMAND_YES || (do_tcpext_connaborts == CONFIG_ONDEMAND_ONDEMAND && (tcpstat.tcps_rcvpackafterwin || tcpstat.tcps_rcvafterclose || tcpstat.tcps_rcvmemdrop || tcpstat.tcps_persistdrop || tcpstat.tcps_finwait2_drops))) {
                do_tcpext_connaborts = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.tcpconnaborts");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcpconnaborts", NULL, "tcp", NULL, "TCP Connection Aborts", "connections/s", 3010, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "TCPAbortOnData",    "baddata",     1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnClose",   "userclosed",  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnMemory",  "nomemory",    1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnTimeout", "timeout",     1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnLinger",  "linger",      1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "TCPAbortOnData",    tcpstat.tcps_rcvpackafterwin);
                rrddim_set(st, "TCPAbortOnClose",   tcpstat.tcps_rcvafterclose);
                rrddim_set(st, "TCPAbortOnMemory",  tcpstat.tcps_rcvmemdrop);
                rrddim_set(st, "TCPAbortOnTimeout", tcpstat.tcps_persistdrop);
                rrddim_set(st, "TCPAbortOnLinger",  tcpstat.tcps_finwait2_drops);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_tcpext_ofo == CONFIG_ONDEMAND_YES || (do_tcpext_ofo == CONFIG_ONDEMAND_ONDEMAND && tcpstat.tcps_rcvoopack)) {
                do_tcpext_ofo = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.tcpofo");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcpofo", NULL, "tcp", NULL, "TCP Out-Of-Order Queue", "packets/s", 3050, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "TCPOFOQueue", "inqueue",  1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "TCPOFOQueue",   tcpstat.tcps_rcvoopack);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_tcpext_syscookies == CONFIG_ONDEMAND_YES || (do_tcpext_syscookies == CONFIG_ONDEMAND_ONDEMAND && (tcpstat.tcps_sc_sendcookie || tcpstat.tcps_sc_recvcookie || tcpstat.tcps_sc_zonefail))) {
                do_tcpext_syscookies = CONFIG_ONDEMAND_YES;

                st = rrdset_find("ipv4.tcpsyncookies");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcpsyncookies", NULL, "tcp", NULL, "TCP SYN Cookies", "packets/s", 3100, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "SyncookiesRecv",   "received",  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "SyncookiesSent",   "sent",     -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "SyncookiesFailed", "failed",   -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "SyncookiesRecv",   tcpstat.tcps_sc_recvcookie);
                rrddim_set(st, "SyncookiesSent",   tcpstat.tcps_sc_sendcookie);
                rrddim_set(st, "SyncookiesFailed", tcpstat.tcps_sc_zonefail);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_ecn == CONFIG_ONDEMAND_YES || (do_ecn == CONFIG_ONDEMAND_ONDEMAND && (tcpstat.tcps_ecn_ce || tcpstat.tcps_ecn_ect0 || tcpstat.tcps_ecn_ect1))) {
                do_ecn = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.ecnpkts");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "ecnpkts", NULL, "ecn", NULL, "IPv4 ECN Statistics", "packets/s", 8700, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "InCEPkts", "CEP", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InNoECTPkts", "NoECTP", -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InECT0Pkts", "ECTP0", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InECT1Pkts", "ECTP1", 1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InCEPkts", tcpstat.tcps_ecn_ce);
                rrddim_set(st, "InNoECTPkts", tcpstat.tcps_ecn_ce - (tcpstat.tcps_ecn_ect0 + tcpstat.tcps_ecn_ect1));
                rrddim_set(st, "InECT0Pkts", tcpstat.tcps_ecn_ect0);
                rrddim_set(st, "InECT1Pkts", tcpstat.tcps_ecn_ect1);
                rrdset_done(st);
            }

        }
    }

    // --------------------------------------------------------------------

    // see http://net-snmp.sourceforge.net/docs/mibs/udp.html
    if (likely(do_udp_packets || do_udp_errors)) {
        if (unlikely(GETSYSCTL("net.inet.udp.stats", udpstat))) {
            do_udp_packets = 0;
            error("DISABLED: ipv4.udppackets");
            do_udp_errors = 0;
            error("DISABLED: ipv4.udperrors");
        } else {
            if (likely(do_udp_packets)) {
                st = rrdset_find("ipv4.udppackets");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "udppackets", NULL, "udp", NULL, "IPv4 UDP Packets",
                                       "packets/s", 2601, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InDatagrams", "received", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutDatagrams", "sent", -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InDatagrams", udpstat.udps_ipackets);
                rrddim_set(st, "OutDatagrams", udpstat.udps_opackets);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_udp_errors)) {
                st = rrdset_find("ipv4.udperrors");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "udperrors", NULL, "udp", NULL, "IPv4 UDP Errors", "events/s",
                                       2701, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "RcvbufErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "NoPorts", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "IgnoredMulti", NULL, 1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InErrors", udpstat.udps_hdrops + udpstat.udps_badlen);
                rrddim_set(st, "NoPorts", udpstat.udps_noport);
                rrddim_set(st, "RcvbufErrors", udpstat.udps_fullsock);
                rrddim_set(st, "InCsumErrors", udpstat.udps_badsum + udpstat.udps_nosum);
                rrddim_set(st, "IgnoredMulti", udpstat.udps_filtermcast);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_icmp_packets || do_icmpmsg)) {
        if (unlikely(GETSYSCTL("net.inet.icmp.stats", icmpstat))) {
            do_icmp_packets = 0;
            error("DISABLED: ipv4.icmp");
            error("DISABLED: ipv4.icmp_errors");
            do_icmpmsg = 0;
            error("DISABLED: ipv4.icmpmsg");
        } else {
            for (i = 0; i <= ICMP_MAXTYPE; i++) {
                icmp_total.msgs_in += icmpstat.icps_inhist[i];
                icmp_total.msgs_out += icmpstat.icps_outhist[i];
            }
            icmp_total.msgs_in += icmpstat.icps_badcode + icmpstat.icps_badlen + icmpstat.icps_checksum + icmpstat.icps_tooshort;

            // --------------------------------------------------------------------

            if (likely(do_icmp_packets)) {
                st = rrdset_find("ipv4.icmp");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "icmp", NULL, "icmp", NULL, "IPv4 ICMP Packets", "packets/s",
                                       2602,
                                       update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InMsgs", "received", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutMsgs", "sent", -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InMsgs", icmp_total.msgs_in);
                rrddim_set(st, "OutMsgs", icmp_total.msgs_out);

                rrdset_done(st);

                // --------------------------------------------------------------------

                st = rrdset_find("ipv4.icmp_errors");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "icmp_errors", NULL, "icmp", NULL, "IPv4 ICMP Errors",
                                       "packets/s",
                                       2603, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutErrors", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InErrors", icmpstat.icps_badcode + icmpstat.icps_badlen + icmpstat.icps_checksum + icmpstat.icps_tooshort);
                rrddim_set(st, "OutErrors", icmpstat.icps_error);
                rrddim_set(st, "InCsumErrors", icmpstat.icps_checksum);

                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_icmpmsg)) {
                st = rrdset_find("ipv4.icmpmsg");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "icmpmsg", NULL, "icmp", NULL, "IPv4 ICMP Messsages",
                                       "packets/s", 2604, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InEchoReps", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutEchoReps", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InEchos", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutEchos", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                    rrddim_set(st, "InEchoReps", icmpstat.icps_inhist[ICMP_ECHOREPLY]);
                    rrddim_set(st, "OutEchoReps", icmpstat.icps_outhist[ICMP_ECHOREPLY]);
                    rrddim_set(st, "InEchos", icmpstat.icps_inhist[ICMP_ECHO]);
                    rrddim_set(st, "OutEchos", icmpstat.icps_outhist[ICMP_ECHO]);

                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------

    // see also http://net-snmp.sourceforge.net/docs/mibs/ip.html
    if (likely(do_ip_packets || do_ip_fragsout || do_ip_fragsin || do_ip_errors)) {
        if (unlikely(GETSYSCTL("net.inet.ip.stats", ipstat))) {
            do_ip_packets = 0;
            error("DISABLED: ipv4.packets");
            do_ip_fragsout = 0;
            error("DISABLED: ipv4.fragsout");
            do_ip_fragsin = 0;
            error("DISABLED: ipv4.fragsin");
            do_ip_errors = 0;
            error("DISABLED: ipv4.errors");
        } else {
            if (likely(do_ip_packets)) {
                st = rrdset_find("ipv4.packets");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "packets", NULL, "packets", NULL, "IPv4 Packets", "packets/s",
                                       3000, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InReceives", "received", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutRequests", "sent", -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "ForwDatagrams", "forwarded", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InDelivers", "delivered", 1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "OutRequests", ipstat.ips_localout);
                rrddim_set(st, "InReceives", ipstat.ips_total);
                rrddim_set(st, "ForwDatagrams", ipstat.ips_forward);
                rrddim_set(st, "InDelivers", ipstat.ips_delivered);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_ip_fragsout)) {
                st = rrdset_find("ipv4.fragsout");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "fragsout", NULL, "fragments", NULL, "IPv4 Fragments Sent",
                                       "packets/s", 3010, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "FragOKs", "ok", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "FragFails", "failed", -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "FragCreates", "created", 1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "FragOKs", ipstat.ips_fragmented);
                rrddim_set(st, "FragFails", ipstat.ips_cantfrag);
                rrddim_set(st, "FragCreates", ipstat.ips_ofragments);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_ip_fragsin)) {
                st = rrdset_find("ipv4.fragsin");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "fragsin", NULL, "fragments", NULL,
                                       "IPv4 Fragments Reassembly",
                                       "packets/s", 3011, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "ReasmOKs", "ok", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "ReasmFails", "failed", -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "ReasmReqds", "all", 1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "ReasmOKs", ipstat.ips_fragments);
                rrddim_set(st, "ReasmFails", ipstat.ips_fragdropped);
                rrddim_set(st, "ReasmReqds", ipstat.ips_reassembled);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_ip_errors)) {
                st = rrdset_find("ipv4.errors");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "errors", NULL, "errors", NULL, "IPv4 Errors", "packets/s",
                                       3002,
                                       update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "InDiscards", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutDiscards", NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InHdrErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutNoRoutes", NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InAddrErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InUnknownProtos", NULL, 1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InDiscards", ipstat.ips_badsum + ipstat.ips_tooshort + ipstat.ips_toosmall + ipstat.ips_toolong);
                rrddim_set(st, "OutDiscards", ipstat.ips_odropped);
                rrddim_set(st, "InHdrErrors", ipstat.ips_badhlen + ipstat.ips_badlen + ipstat.ips_badoptions + ipstat.ips_badvers);
                rrddim_set(st, "InAddrErrors", ipstat.ips_badaddr);
                rrddim_set(st, "InUnknownProtos", ipstat.ips_noproto);
                rrddim_set(st, "OutNoRoutes", ipstat.ips_noroute);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_ip6_packets || do_ip6_fragsout || do_ip6_fragsin || do_ip6_errors)) {
        if (unlikely(GETSYSCTL("net.inet6.ip6.stats", ip6stat))) {
            do_ip6_packets = 0;
            error("DISABLED: ipv6.packets");
            do_ip6_fragsout = 0;
            error("DISABLED: ipv6.fragsout");
            do_ip6_fragsin = 0;
            error("DISABLED: ipv6.fragsin");
            do_ip6_errors = 0;
            error("DISABLED: ipv6.errors");
        } else {
            if (do_ip6_packets == CONFIG_ONDEMAND_YES || (do_ip6_packets == CONFIG_ONDEMAND_ONDEMAND &&
                                                          (ip6stat.ip6s_localout || ip6stat.ip6s_total ||
                                                           ip6stat.ip6s_forward || ip6stat.ip6s_delivered))) {
                do_ip6_packets = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv6.packets");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv6", "packets", NULL, "packets", NULL, "IPv6 Packets", "packets/s", 3000,
                                       update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "forwarded", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "delivers", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "sent", ip6stat.ip6s_localout);
                rrddim_set(st, "received", ip6stat.ip6s_total);
                rrddim_set(st, "forwarded", ip6stat.ip6s_forward);
                rrddim_set(st, "delivers", ip6stat.ip6s_delivered);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_ip6_fragsout == CONFIG_ONDEMAND_YES || (do_ip6_fragsout == CONFIG_ONDEMAND_ONDEMAND &&
                                                           (ip6stat.ip6s_fragmented || ip6stat.ip6s_cantfrag ||
                                                            ip6stat.ip6s_ofragments))) {
                do_ip6_fragsout = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv6.fragsout");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv6", "fragsout", NULL, "fragments", NULL, "IPv6 Fragments Sent",
                                       "packets/s", 3010, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "ok", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "failed", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "all", NULL, 1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "ok", ip6stat.ip6s_fragmented);
                rrddim_set(st, "failed", ip6stat.ip6s_cantfrag);
                rrddim_set(st, "all", ip6stat.ip6s_ofragments);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_ip6_fragsin == CONFIG_ONDEMAND_YES || (do_ip6_fragsin == CONFIG_ONDEMAND_ONDEMAND &&
                                                          (ip6stat.ip6s_reassembled || ip6stat.ip6s_fragdropped ||
                                                           ip6stat.ip6s_fragtimeout || ip6stat.ip6s_fragments))) {
                do_ip6_fragsin = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv6.fragsin");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv6", "fragsin", NULL, "fragments", NULL, "IPv6 Fragments Reassembly",
                                       "packets/s", 3011, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "ok", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "failed", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "timeout", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "all", NULL, 1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "ok", ip6stat.ip6s_reassembled);
                rrddim_set(st, "failed", ip6stat.ip6s_fragdropped);
                rrddim_set(st, "timeout", ip6stat.ip6s_fragtimeout);
                rrddim_set(st, "all", ip6stat.ip6s_fragments);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_ip6_errors == CONFIG_ONDEMAND_YES || (do_ip6_errors == CONFIG_ONDEMAND_ONDEMAND && (
                    ip6stat.ip6s_toosmall ||
                    ip6stat.ip6s_odropped ||
                    ip6stat.ip6s_badoptions ||
                    ip6stat.ip6s_badvers ||
                    ip6stat.ip6s_exthdrtoolong ||
                    ip6stat.ip6s_sources_none ||
                    ip6stat.ip6s_tooshort ||
                    ip6stat.ip6s_cantforward ||
                    ip6stat.ip6s_noroute))) {
                do_ip6_errors = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv6.errors");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv6", "errors", NULL, "errors", NULL, "IPv6 Errors", "packets/s", 3002,
                                       update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "InDiscards", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutDiscards", NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InHdrErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InAddrErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InTruncatedPkts", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InNoRoutes", NULL, 1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "OutNoRoutes", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InDiscards", ip6stat.ip6s_toosmall);
                rrddim_set(st, "OutDiscards", ip6stat.ip6s_odropped);

                rrddim_set(st, "InHdrErrors",
                           ip6stat.ip6s_badoptions + ip6stat.ip6s_badvers + ip6stat.ip6s_exthdrtoolong);
                rrddim_set(st, "InAddrErrors", ip6stat.ip6s_sources_none);
                rrddim_set(st, "InTruncatedPkts", ip6stat.ip6s_tooshort);
                rrddim_set(st, "InNoRoutes", ip6stat.ip6s_cantforward);

                rrddim_set(st, "OutNoRoutes", ip6stat.ip6s_noroute);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_icmp6 || do_icmp6_redir || do_icmp6_errors || do_icmp6_echos || do_icmp6_router || do_icmp6_neighbor || do_icmp6_types)) {
        if (unlikely(GETSYSCTL("net.inet6.icmp6.stats", icmp6stat))) {
            do_icmp6 = 0;
            error("DISABLED: ipv6.icmp");
        } else {
            for (i = 0; i <= ICMP6_MAXTYPE; i++) {
                icmp6_total.msgs_in += icmp6stat.icp6s_inhist[i];
                icmp6_total.msgs_out += icmp6stat.icp6s_outhist[i];
            }
            icmp6_total.msgs_in += icmp6stat.icp6s_badcode + icmp6stat.icp6s_badlen + icmp6stat.icp6s_checksum + icmp6stat.icp6s_tooshort;
            if (do_icmp6 == CONFIG_ONDEMAND_YES || (do_icmp6 == CONFIG_ONDEMAND_ONDEMAND && (icmp6_total.msgs_in || icmp6_total.msgs_out))) {
                do_icmp6 = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv6.icmp");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv6", "icmp", NULL, "icmp", NULL, "IPv6 ICMP Messages",
                                       "messages/s", 10000, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "sent", icmp6_total.msgs_in);
                rrddim_set(st, "received", icmp6_total.msgs_out);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_icmp6_redir == CONFIG_ONDEMAND_YES || (do_icmp6_redir == CONFIG_ONDEMAND_ONDEMAND && (icmp6stat.icp6s_inhist[ND_REDIRECT] || icmp6stat.icp6s_outhist[ND_REDIRECT]))) {
                do_icmp6_redir = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv6.icmpredir");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv6", "icmpredir", NULL, "icmp", NULL, "IPv6 ICMP Redirects",
                                       "redirects/s", 10050, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "sent", icmp6stat.icp6s_inhist[ND_REDIRECT]);
                rrddim_set(st, "received", icmp6stat.icp6s_outhist[ND_REDIRECT]);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_icmp6_errors == CONFIG_ONDEMAND_YES || (do_icmp6_errors == CONFIG_ONDEMAND_ONDEMAND && (
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
                do_icmp6_errors = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv6.icmperrors");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv6", "icmperrors", NULL, "icmp", NULL, "IPv6 ICMP Errors", "errors/s", 10100, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutErrors", NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InDestUnreachs", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InPktTooBigs", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InTimeExcds", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InParmProblems", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutDestUnreachs", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutTimeExcds", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutParmProblems", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InErrors", icmp6stat.icp6s_badcode + icmp6stat.icp6s_badlen + icmp6stat.icp6s_checksum + icmp6stat.icp6s_tooshort);
                rrddim_set(st, "OutErrors", icmp6stat.icp6s_error);
                rrddim_set(st, "InCsumErrors", icmp6stat.icp6s_checksum);
                rrddim_set(st, "InDestUnreachs", icmp6stat.icp6s_inhist[ICMP6_DST_UNREACH]);
                rrddim_set(st, "InPktTooBigs", icmp6stat.icp6s_badlen);
                rrddim_set(st, "InTimeExcds", icmp6stat.icp6s_inhist[ICMP6_TIME_EXCEEDED]);
                rrddim_set(st, "InParmProblems", icmp6stat.icp6s_inhist[ICMP6_PARAM_PROB]);
                rrddim_set(st, "OutDestUnreachs", icmp6stat.icp6s_outhist[ICMP6_DST_UNREACH]);
                rrddim_set(st, "OutTimeExcds", icmp6stat.icp6s_outhist[ICMP6_TIME_EXCEEDED]);
                rrddim_set(st, "OutParmProblems", icmp6stat.icp6s_outhist[ICMP6_PARAM_PROB]);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_icmp6_echos == CONFIG_ONDEMAND_YES || (do_icmp6_echos == CONFIG_ONDEMAND_ONDEMAND && (
                                                                 icmp6stat.icp6s_inhist[ICMP6_ECHO_REQUEST] ||
                                                                 icmp6stat.icp6s_outhist[ICMP6_ECHO_REQUEST] ||
                                                                 icmp6stat.icp6s_inhist[ICMP6_ECHO_REPLY] ||
                                                                 icmp6stat.icp6s_outhist[ICMP6_ECHO_REPLY]))) {
                do_icmp6_echos = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv6.icmpechos");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv6", "icmpechos", NULL, "icmp", NULL, "IPv6 ICMP Echo", "messages/s", 10200, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InEchos", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutEchos", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InEchoReplies", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutEchoReplies", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InEchos", icmp6stat.icp6s_inhist[ICMP6_ECHO_REQUEST]);
                rrddim_set(st, "OutEchos", icmp6stat.icp6s_outhist[ICMP6_ECHO_REQUEST]);
                rrddim_set(st, "InEchoReplies", icmp6stat.icp6s_inhist[ICMP6_ECHO_REPLY]);
                rrddim_set(st, "OutEchoReplies", icmp6stat.icp6s_outhist[ICMP6_ECHO_REPLY]);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_icmp6_router == CONFIG_ONDEMAND_YES || (do_icmp6_router == CONFIG_ONDEMAND_ONDEMAND && (
                                                                    icmp6stat.icp6s_inhist[ND_ROUTER_SOLICIT] ||
                                                                    icmp6stat.icp6s_outhist[ND_ROUTER_SOLICIT] ||
                                                                    icmp6stat.icp6s_inhist[ND_ROUTER_ADVERT] ||
                                                                    icmp6stat.icp6s_outhist[ND_ROUTER_ADVERT]))) {
                do_icmp6_router = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv6.icmprouter");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv6", "icmprouter", NULL, "icmp", NULL, "IPv6 Router Messages", "messages/s", 10400, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InSolicits", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutSolicits", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InAdvertisements", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InSolicits", icmp6stat.icp6s_inhist[ND_ROUTER_SOLICIT]);
                rrddim_set(st, "OutSolicits", icmp6stat.icp6s_outhist[ND_ROUTER_SOLICIT]);
                rrddim_set(st, "InAdvertisements", icmp6stat.icp6s_inhist[ND_ROUTER_ADVERT]);
                rrddim_set(st, "OutAdvertisements", icmp6stat.icp6s_outhist[ND_ROUTER_ADVERT]);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_icmp6_neighbor == CONFIG_ONDEMAND_YES || (do_icmp6_neighbor == CONFIG_ONDEMAND_ONDEMAND && (
                                                                    icmp6stat.icp6s_inhist[ND_NEIGHBOR_SOLICIT] ||
                                                                    icmp6stat.icp6s_outhist[ND_NEIGHBOR_SOLICIT] ||
                                                                    icmp6stat.icp6s_inhist[ND_NEIGHBOR_ADVERT] ||
                                                                    icmp6stat.icp6s_outhist[ND_NEIGHBOR_ADVERT]))) {
                do_icmp6_neighbor = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv6.icmpneighbor");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv6", "icmpneighbor", NULL, "icmp", NULL, "IPv6 Neighbor Messages", "messages/s", 10500, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InSolicits", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutSolicits", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InAdvertisements", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InSolicits", icmp6stat.icp6s_inhist[ND_NEIGHBOR_SOLICIT]);
                rrddim_set(st, "OutSolicits", icmp6stat.icp6s_outhist[ND_NEIGHBOR_SOLICIT]);
                rrddim_set(st, "InAdvertisements", icmp6stat.icp6s_inhist[ND_NEIGHBOR_ADVERT]);
                rrddim_set(st, "OutAdvertisements", icmp6stat.icp6s_outhist[ND_NEIGHBOR_ADVERT]);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_icmp6_types == CONFIG_ONDEMAND_YES || (do_icmp6_types == CONFIG_ONDEMAND_ONDEMAND && (
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
                do_icmp6_types = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv6.icmptypes");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv6", "icmptypes", NULL, "icmp", NULL, "IPv6 ICMP Types",
                                       "messages/s", 10700, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InType1", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InType128", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InType129", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InType136", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutType1", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutType128", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutType129", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutType133", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutType135", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutType143", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InType1", icmp6stat.icp6s_inhist[1]);
                rrddim_set(st, "InType128", icmp6stat.icp6s_inhist[128]);
                rrddim_set(st, "InType129", icmp6stat.icp6s_inhist[129]);
                rrddim_set(st, "InType136", icmp6stat.icp6s_inhist[136]);
                rrddim_set(st, "OutType1", icmp6stat.icp6s_outhist[1]);
                rrddim_set(st, "OutType128", icmp6stat.icp6s_outhist[128]);
                rrddim_set(st, "OutType129", icmp6stat.icp6s_outhist[129]);
                rrddim_set(st, "OutType133", icmp6stat.icp6s_outhist[133]);
                rrddim_set(st, "OutType135", icmp6stat.icp6s_outhist[135]);
                rrddim_set(st, "OutType143", icmp6stat.icp6s_outhist[143]);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------------

    if (likely(do_space || do_inodes)) {
        // there is no mount info in sysctl MIBs
        if (unlikely(!(mntsize = getmntinfo(&mntbuf, MNT_NOWAIT)))) {
            error("FREEBSD: getmntinfo() failed");
            do_space = 0;
            error("DISABLED: disk_space.X");
            do_inodes = 0;
            error("DISABLED: disk_inodes.X");
        } else {
            for (i = 0; i < mntsize; i++) {
                if (mntbuf[i].f_flags == MNT_RDONLY ||
                        mntbuf[i].f_blocks == 0 ||
                        // taken from gnulib/mountlist.c and shortened to FreeBSD related fstypes
                        strcmp(mntbuf[i].f_fstypename, "autofs") == 0 ||
                        strcmp(mntbuf[i].f_fstypename, "procfs") == 0 ||
                        strcmp(mntbuf[i].f_fstypename, "subfs") == 0 ||
                        strcmp(mntbuf[i].f_fstypename, "devfs") == 0 ||
                        strcmp(mntbuf[i].f_fstypename, "none") == 0)
                    continue;

                // --------------------------------------------------------------------------

                if (likely(do_space)) {
                    st = rrdset_find_bytype("disk_space", mntbuf[i].f_mntonname);
                    if (unlikely(!st)) {
                        snprintfz(title, 4096, "Disk Space Usage for %s [%s]", mntbuf[i].f_mntonname, mntbuf[i].f_mntfromname);
                        st = rrdset_create("disk_space", mntbuf[i].f_mntonname, NULL, mntbuf[i].f_mntonname, "disk.space", title, "GB", 2023,
                                           update_every,
                                           RRDSET_TYPE_STACKED);

                        rrddim_add(st, "avail", NULL, mntbuf[i].f_bsize, GIGA_FACTOR, RRDDIM_ABSOLUTE);
                        rrddim_add(st, "used", NULL, mntbuf[i].f_bsize, GIGA_FACTOR, RRDDIM_ABSOLUTE);
                        rrddim_add(st, "reserved_for_root", "reserved for root", mntbuf[i].f_bsize, GIGA_FACTOR,
                                   RRDDIM_ABSOLUTE);
                    } else
                        rrdset_next(st);

                    rrddim_set(st, "avail", (collected_number) mntbuf[i].f_bavail);
                    rrddim_set(st, "used", (collected_number) (mntbuf[i].f_blocks - mntbuf[i].f_bfree));
                    rrddim_set(st, "reserved_for_root", (collected_number) (mntbuf[i].f_bfree - mntbuf[i].f_bavail));
                    rrdset_done(st);
                }

                // --------------------------------------------------------------------------

                if (likely(do_inodes)) {
                    st = rrdset_find_bytype("disk_inodes", mntbuf[i].f_mntonname);
                    if (unlikely(!st)) {
                        snprintfz(title, 4096, "Disk Files (inodes) Usage for %s [%s]", mntbuf[i].f_mntonname, mntbuf[i].f_mntfromname);
                        st = rrdset_create("disk_inodes", mntbuf[i].f_mntonname, NULL, mntbuf[i].f_mntonname, "disk.inodes", title, "Inodes", 2024,
                                           update_every, RRDSET_TYPE_STACKED);

                        rrddim_add(st, "avail", NULL, 1, 1, RRDDIM_ABSOLUTE);
                        rrddim_add(st, "used", NULL, 1, 1, RRDDIM_ABSOLUTE);
                        rrddim_add(st, "reserved_for_root", "reserved for root", 1, 1, RRDDIM_ABSOLUTE);
                    } else
                        rrdset_next(st);

                    rrddim_set(st, "avail", (collected_number) mntbuf[i].f_ffree);
                    rrddim_set(st, "used", (collected_number) (mntbuf[i].f_files - mntbuf[i].f_ffree));
                    rrdset_done(st);
                }
            }
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_uptime)) {
        if (unlikely(GETSYSCTL("kern.boottime", boot_time))) {
            do_uptime = 0;
            error("DISABLED: system.uptime");
        } else {
            clock_gettime(CLOCK_REALTIME, &cur_time);
            st = rrdset_find("system.uptime");

            if(unlikely(!st)) {
                st = rrdset_create("system", "uptime", NULL, "uptime", NULL, "System Uptime", "seconds", 1000, update_every, RRDSET_TYPE_LINE);
                rrddim_add(st, "uptime", NULL, 1, 1, RRDDIM_ABSOLUTE);
            }
            else rrdset_next(st);

            rrddim_set(st, "uptime", cur_time.tv_sec - boot_time.tv_sec);
            rrdset_done(st);
        }
    }

    return 0;
}
