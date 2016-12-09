#include "common.h"

void *proc_main(void *ptr)
{
    (void)ptr;

    info("PROC Plugin thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    // disable (by default) various interface that are not needed
    config_get_boolean("plugin:proc:/proc/net/dev:lo", "enabled", 0);
    config_get_boolean("plugin:proc:/proc/net/dev:fireqos_monitor", "enabled", 0);

    // when ZERO, attempt to do it
    int vdo_proc_net_dev            = !config_get_boolean("plugin:proc", "/proc/net/dev", 1);
    int vdo_proc_diskstats          = !config_get_boolean("plugin:proc", "/proc/diskstats", 1);
    int vdo_proc_net_snmp           = !config_get_boolean("plugin:proc", "/proc/net/snmp", 1);
    int vdo_proc_net_snmp6          = !config_get_boolean("plugin:proc", "/proc/net/snmp6", 1);
    int vdo_proc_net_netstat        = !config_get_boolean("plugin:proc", "/proc/net/netstat", 1);
    int vdo_proc_net_stat_conntrack = !config_get_boolean("plugin:proc", "/proc/net/stat/conntrack", 1);
    int vdo_proc_net_ip_vs_stats    = !config_get_boolean("plugin:proc", "/proc/net/ip_vs/stats", 1);
    int vdo_proc_net_stat_synproxy  = !config_get_boolean("plugin:proc", "/proc/net/stat/synproxy", 1);
    int vdo_proc_stat               = !config_get_boolean("plugin:proc", "/proc/stat", 1);
    int vdo_proc_meminfo            = !config_get_boolean("plugin:proc", "/proc/meminfo", 1);
    int vdo_proc_vmstat             = !config_get_boolean("plugin:proc", "/proc/vmstat", 1);
    int vdo_proc_net_rpc_nfs        = !config_get_boolean("plugin:proc", "/proc/net/rpc/nfs", 1);
    int vdo_proc_net_rpc_nfsd       = !config_get_boolean("plugin:proc", "/proc/net/rpc/nfsd", 1);
    int vdo_proc_sys_kernel_random_entropy_avail    = !config_get_boolean("plugin:proc", "/proc/sys/kernel/random/entropy_avail", 1);
    int vdo_proc_interrupts         = !config_get_boolean("plugin:proc", "/proc/interrupts", 1);
    int vdo_proc_softirqs           = !config_get_boolean("plugin:proc", "/proc/softirqs", 1);
    int vdo_proc_net_softnet_stat   = !config_get_boolean("plugin:proc", "/proc/net/softnet_stat", 1);
    int vdo_proc_loadavg            = !config_get_boolean("plugin:proc", "/proc/loadavg", 1);
    int vdo_ipc                     = !config_get_boolean("plugin:proc", "ipc", 1);
    int vdo_sys_kernel_mm_ksm       = !config_get_boolean("plugin:proc", "/sys/kernel/mm/ksm", 1);
    int vdo_cpu_netdata             = !config_get_boolean("plugin:proc", "netdata server resources", 1);

    // keep track of the time each module was called
    usec_t sutime_proc_net_dev = 0ULL;
    usec_t sutime_proc_diskstats = 0ULL;
    usec_t sutime_proc_net_snmp = 0ULL;
    usec_t sutime_proc_net_snmp6 = 0ULL;
    usec_t sutime_proc_net_netstat = 0ULL;
    usec_t sutime_proc_net_stat_conntrack = 0ULL;
    usec_t sutime_proc_net_ip_vs_stats = 0ULL;
    usec_t sutime_proc_net_stat_synproxy = 0ULL;
    usec_t sutime_proc_stat = 0ULL;
    usec_t sutime_proc_meminfo = 0ULL;
    usec_t sutime_proc_vmstat = 0ULL;
    usec_t sutime_proc_net_rpc_nfs = 0ULL;
    usec_t sutime_proc_net_rpc_nfsd = 0ULL;
    usec_t sutime_proc_sys_kernel_random_entropy_avail = 0ULL;
    usec_t sutime_proc_interrupts = 0ULL;
    usec_t sutime_proc_softirqs = 0ULL;
    usec_t sutime_proc_net_softnet_stat = 0ULL;
    usec_t sutime_proc_loadavg = 0ULL;
    usec_t sutime_ipc = 0ULL;
    usec_t sutime_sys_kernel_mm_ksm = 0ULL;

    usec_t step = rrd_update_every * USEC_PER_SEC;
    for(;;) {
        usec_t now = now_realtime_usec();
        usec_t next = now - (now % step) + step;

        while(now < next) {
            sleep_usec(next - now);
            now = now_realtime_usec();
        }

        if(unlikely(netdata_exit)) break;

        // BEGIN -- the job to be done

        if(!vdo_sys_kernel_mm_ksm) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_sys_kernel_mm_ksm().");

            now = now_realtime_usec();
            vdo_sys_kernel_mm_ksm = do_sys_kernel_mm_ksm(rrd_update_every, (sutime_sys_kernel_mm_ksm > 0)?now - sutime_sys_kernel_mm_ksm:0ULL);
            sutime_sys_kernel_mm_ksm = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_loadavg) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_loadavg().");
            now = now_realtime_usec();
            vdo_proc_loadavg = do_proc_loadavg(rrd_update_every, (sutime_proc_loadavg > 0)?now - sutime_proc_loadavg:0ULL);
            sutime_proc_loadavg = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_ipc) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_ipc().");
            now = now_realtime_usec();
            vdo_ipc = do_ipc(rrd_update_every, (sutime_ipc > 0)?now - sutime_ipc:0ULL);
            sutime_ipc = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_interrupts) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_interrupts().");
            now = now_realtime_usec();
            vdo_proc_interrupts = do_proc_interrupts(rrd_update_every, (sutime_proc_interrupts > 0)?now - sutime_proc_interrupts:0ULL);
            sutime_proc_interrupts = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_softirqs) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_softirqs().");
            now = now_realtime_usec();
            vdo_proc_softirqs = do_proc_softirqs(rrd_update_every, (sutime_proc_softirqs > 0)?now - sutime_proc_softirqs:0ULL);
            sutime_proc_softirqs = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_net_softnet_stat) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_softnet_stat().");
            now = now_realtime_usec();
            vdo_proc_net_softnet_stat = do_proc_net_softnet_stat(rrd_update_every, (sutime_proc_net_softnet_stat > 0)?now - sutime_proc_net_softnet_stat:0ULL);
            sutime_proc_net_softnet_stat = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_sys_kernel_random_entropy_avail) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_sys_kernel_random_entropy_avail().");
            now = now_realtime_usec();
            vdo_proc_sys_kernel_random_entropy_avail = do_proc_sys_kernel_random_entropy_avail(rrd_update_every, (sutime_proc_sys_kernel_random_entropy_avail > 0)?now - sutime_proc_sys_kernel_random_entropy_avail:0ULL);
            sutime_proc_sys_kernel_random_entropy_avail = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_net_dev) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_dev().");
            now = now_realtime_usec();
            vdo_proc_net_dev = do_proc_net_dev(rrd_update_every, (sutime_proc_net_dev > 0)?now - sutime_proc_net_dev:0ULL);
            sutime_proc_net_dev = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_diskstats) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_diskstats().");
            now = now_realtime_usec();
            vdo_proc_diskstats = do_proc_diskstats(rrd_update_every, (sutime_proc_diskstats > 0)?now - sutime_proc_diskstats:0ULL);
            sutime_proc_diskstats = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_net_snmp) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_snmp().");
            now = now_realtime_usec();
            vdo_proc_net_snmp = do_proc_net_snmp(rrd_update_every, (sutime_proc_net_snmp > 0)?now - sutime_proc_net_snmp:0ULL);
            sutime_proc_net_snmp = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_net_snmp6) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_snmp6().");
            now = now_realtime_usec();
            vdo_proc_net_snmp6 = do_proc_net_snmp6(rrd_update_every, (sutime_proc_net_snmp6 > 0)?now - sutime_proc_net_snmp6:0ULL);
            sutime_proc_net_snmp6 = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_net_netstat) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_netstat().");
            now = now_realtime_usec();
            vdo_proc_net_netstat = do_proc_net_netstat(rrd_update_every, (sutime_proc_net_netstat > 0)?now - sutime_proc_net_netstat:0ULL);
            sutime_proc_net_netstat = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_net_stat_conntrack) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_stat_conntrack().");
            now = now_realtime_usec();
            vdo_proc_net_stat_conntrack = do_proc_net_stat_conntrack(rrd_update_every, (sutime_proc_net_stat_conntrack > 0)?now - sutime_proc_net_stat_conntrack:0ULL);
            sutime_proc_net_stat_conntrack = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_net_ip_vs_stats) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling vdo_proc_net_ip_vs_stats().");
            now = now_realtime_usec();
            vdo_proc_net_ip_vs_stats = do_proc_net_ip_vs_stats(rrd_update_every, (sutime_proc_net_ip_vs_stats > 0)?now - sutime_proc_net_ip_vs_stats:0ULL);
            sutime_proc_net_ip_vs_stats = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_net_stat_synproxy) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling vdo_proc_net_stat_synproxy().");
            now = now_realtime_usec();
            vdo_proc_net_stat_synproxy = do_proc_net_stat_synproxy(rrd_update_every, (sutime_proc_net_stat_synproxy > 0)?now - sutime_proc_net_stat_synproxy:0ULL);
            sutime_proc_net_stat_synproxy = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_stat) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_stat().");
            now = now_realtime_usec();
            vdo_proc_stat = do_proc_stat(rrd_update_every, (sutime_proc_stat > 0)?now - sutime_proc_stat:0ULL);
            sutime_proc_stat = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_meminfo) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling vdo_proc_meminfo().");
            now = now_realtime_usec();
            vdo_proc_meminfo = do_proc_meminfo(rrd_update_every, (sutime_proc_meminfo > 0)?now - sutime_proc_meminfo:0ULL);
            sutime_proc_meminfo = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_vmstat) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling vdo_proc_vmstat().");
            now = now_realtime_usec();
            vdo_proc_vmstat = do_proc_vmstat(rrd_update_every, (sutime_proc_vmstat > 0)?now - sutime_proc_vmstat:0ULL);
            sutime_proc_vmstat = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_net_rpc_nfsd) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_rpc_nfsd().");
            now = now_realtime_usec();
            vdo_proc_net_rpc_nfsd = do_proc_net_rpc_nfsd(rrd_update_every, (sutime_proc_net_rpc_nfsd > 0)?now - sutime_proc_net_rpc_nfsd:0ULL);
            sutime_proc_net_rpc_nfsd = now;
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_proc_net_rpc_nfs) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_rpc_nfs().");
            now = now_realtime_usec();
            vdo_proc_net_rpc_nfs = do_proc_net_rpc_nfs(rrd_update_every, (sutime_proc_net_rpc_nfs > 0)?now - sutime_proc_net_rpc_nfs:0ULL);
            sutime_proc_net_rpc_nfs = now;
        }
        if(unlikely(netdata_exit)) break;

        // END -- the job is done

        // --------------------------------------------------------------------

        if(!vdo_cpu_netdata) {
            global_statistics_charts();
            registry_statistics();
        }
    }

    info("PROC thread exiting");

    pthread_exit(NULL);
    return NULL;
}
