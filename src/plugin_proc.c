#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <pthread.h>
#include <sys/time.h>
#include <sys/resource.h>
#include <strings.h>
#include <unistd.h>

#include "global_statistics.h"
#include "common.h"
#include "appconfig.h"
#include "log.h"
#include "rrd.h"
#include "plugin_proc.h"
#include "main.h"

unsigned long long sutime() {
	struct timeval now;
	gettimeofday(&now, NULL);
	return now.tv_sec * 1000000ULL + now.tv_usec;
}

void *proc_main(void *ptr)
{
	if(ptr) { ; }

	info("PROC Plugin thread created with task id %d", gettid());

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	struct rusage me, thread;

	// disable (by default) various interface that are not needed
	config_get_boolean("plugin:proc:/proc/net/dev:lo", "enabled", 0);
	config_get_boolean("plugin:proc:/proc/net/dev:fireqos_monitor", "enabled", 0);

	// when ZERO, attempt to do it
	int vdo_proc_net_dev 			= !config_get_boolean("plugin:proc", "/proc/net/dev", 1);
	int vdo_proc_diskstats 			= !config_get_boolean("plugin:proc", "/proc/diskstats", 1);
	int vdo_proc_net_snmp 			= !config_get_boolean("plugin:proc", "/proc/net/snmp", 1);
	int vdo_proc_net_netstat 		= !config_get_boolean("plugin:proc", "/proc/net/netstat", 1);
	int vdo_proc_net_stat_conntrack = !config_get_boolean("plugin:proc", "/proc/net/stat/conntrack", 1);
	int vdo_proc_net_ip_vs_stats 	= !config_get_boolean("plugin:proc", "/proc/net/ip_vs/stats", 1);
	int vdo_proc_stat 				= !config_get_boolean("plugin:proc", "/proc/stat", 1);
	int vdo_proc_meminfo 			= !config_get_boolean("plugin:proc", "/proc/meminfo", 1);
	int vdo_proc_vmstat 			= !config_get_boolean("plugin:proc", "/proc/vmstat", 1);
	int vdo_proc_net_rpc_nfsd		= !config_get_boolean("plugin:proc", "/proc/net/rpc/nfsd", 1);
	int vdo_proc_sys_kernel_random_entropy_avail	= !config_get_boolean("plugin:proc", "/proc/sys/kernel/random/entropy_avail", 1);
	int vdo_proc_interrupts			= !config_get_boolean("plugin:proc", "/proc/interrupts", 1);
	int vdo_proc_softirqs			= !config_get_boolean("plugin:proc", "/proc/softirqs", 1);
	int vdo_proc_loadavg			= !config_get_boolean("plugin:proc", "/proc/loadavg", 1);
	int vdo_sys_kernel_mm_ksm		= !config_get_boolean("plugin:proc", "/sys/kernel/mm/ksm", 1);
	int vdo_cpu_netdata 			= !config_get_boolean("plugin:proc", "netdata server resources", 1);

	// keep track of the time each module was called
	unsigned long long sutime_proc_net_dev = 0ULL;
	unsigned long long sutime_proc_diskstats = 0ULL;
	unsigned long long sutime_proc_net_snmp = 0ULL;
	unsigned long long sutime_proc_net_netstat = 0ULL;
	unsigned long long sutime_proc_net_stat_conntrack = 0ULL;
	unsigned long long sutime_proc_net_ip_vs_stats = 0ULL;
	unsigned long long sutime_proc_stat = 0ULL;
	unsigned long long sutime_proc_meminfo = 0ULL;
	unsigned long long sutime_proc_vmstat = 0ULL;
	unsigned long long sutime_proc_net_rpc_nfsd = 0ULL;
	unsigned long long sutime_proc_sys_kernel_random_entropy_avail = 0ULL;
	unsigned long long sutime_proc_interrupts = 0ULL;
	unsigned long long sutime_proc_softirqs = 0ULL;
	unsigned long long sutime_proc_loadavg = 0ULL;
	unsigned long long sutime_sys_kernel_mm_ksm = 0ULL;

	// the next time we will run - aligned properly
	unsigned long long sunext = (time(NULL) - (time(NULL) % rrd_update_every) + rrd_update_every) * 1000000ULL;
	unsigned long long sunow;

	RRDSET *stcpu = NULL, *stcpu_thread = NULL, *stclients = NULL, *streqs = NULL, *stbytes = NULL;

	for(;1;) {
		if(unlikely(netdata_exit)) break;

		// delay until it is our time to run
		while((sunow = sutime()) < sunext)
			usleep((useconds_t)(sunext - sunow));

		// find the next time we need to run
		while(sutime() > sunext)
			sunext += rrd_update_every * 1000000ULL;

		if(unlikely(netdata_exit)) break;

		// BEGIN -- the job to be done

		if(!vdo_sys_kernel_mm_ksm) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_sys_kernel_mm_ksm().");

			sunow = sutime();
			vdo_sys_kernel_mm_ksm = do_sys_kernel_mm_ksm(rrd_update_every, (sutime_sys_kernel_mm_ksm > 0)?sunow - sutime_sys_kernel_mm_ksm:0ULL);
			sutime_sys_kernel_mm_ksm = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_loadavg) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_loadavg().");
			sunow = sutime();
			vdo_proc_loadavg = do_proc_loadavg(rrd_update_every, (sutime_proc_loadavg > 0)?sunow - sutime_proc_loadavg:0ULL);
			sutime_proc_loadavg = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_interrupts) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_interrupts().");
			sunow = sutime();
			vdo_proc_interrupts = do_proc_interrupts(rrd_update_every, (sutime_proc_interrupts > 0)?sunow - sutime_proc_interrupts:0ULL);
			sutime_proc_interrupts = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_softirqs) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_softirqs().");
			sunow = sutime();
			vdo_proc_softirqs = do_proc_softirqs(rrd_update_every, (sutime_proc_softirqs > 0)?sunow - sutime_proc_softirqs:0ULL);
			sutime_proc_softirqs = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_sys_kernel_random_entropy_avail) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_sys_kernel_random_entropy_avail().");
			sunow = sutime();
			vdo_proc_sys_kernel_random_entropy_avail = do_proc_sys_kernel_random_entropy_avail(rrd_update_every, (sutime_proc_sys_kernel_random_entropy_avail > 0)?sunow - sutime_proc_sys_kernel_random_entropy_avail:0ULL);
			sutime_proc_sys_kernel_random_entropy_avail = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_net_dev) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_dev().");
			sunow = sutime();
			vdo_proc_net_dev = do_proc_net_dev(rrd_update_every, (sutime_proc_net_dev > 0)?sunow - sutime_proc_net_dev:0ULL);
			sutime_proc_net_dev = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_diskstats) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_diskstats().");
			sunow = sutime();
			vdo_proc_diskstats = do_proc_diskstats(rrd_update_every, (sutime_proc_diskstats > 0)?sunow - sutime_proc_diskstats:0ULL);
			sutime_proc_diskstats = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_net_snmp) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_snmp().");
			sunow = sutime();
			vdo_proc_net_snmp = do_proc_net_snmp(rrd_update_every, (sutime_proc_net_snmp > 0)?sunow - sutime_proc_net_snmp:0ULL);
			sutime_proc_net_snmp = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_net_netstat) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_netstat().");
			sunow = sutime();
			vdo_proc_net_netstat = do_proc_net_netstat(rrd_update_every, (sutime_proc_net_netstat > 0)?sunow - sutime_proc_net_netstat:0ULL);
			sutime_proc_net_netstat = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_net_stat_conntrack) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_stat_conntrack().");
			sunow = sutime();
			vdo_proc_net_stat_conntrack	= do_proc_net_stat_conntrack(rrd_update_every, (sutime_proc_net_stat_conntrack > 0)?sunow - sutime_proc_net_stat_conntrack:0ULL);
			sutime_proc_net_stat_conntrack = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_net_ip_vs_stats) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling vdo_proc_net_ip_vs_stats().");
			sunow = sutime();
			vdo_proc_net_ip_vs_stats = do_proc_net_ip_vs_stats(rrd_update_every, (sutime_proc_net_ip_vs_stats > 0)?sunow - sutime_proc_net_ip_vs_stats:0ULL);
			sutime_proc_net_ip_vs_stats = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_stat) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_stat().");
			sunow = sutime();
			vdo_proc_stat = do_proc_stat(rrd_update_every, (sutime_proc_stat > 0)?sunow - sutime_proc_stat:0ULL);
			sutime_proc_stat = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_meminfo) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling vdo_proc_meminfo().");
			sunow = sutime();
			vdo_proc_meminfo = do_proc_meminfo(rrd_update_every, (sutime_proc_meminfo > 0)?sunow - sutime_proc_meminfo:0ULL);
			sutime_proc_meminfo = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_vmstat) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling vdo_proc_vmstat().");
			sunow = sutime();
			vdo_proc_vmstat = do_proc_vmstat(rrd_update_every, (sutime_proc_vmstat > 0)?sunow - sutime_proc_vmstat:0ULL);
			sutime_proc_vmstat = sunow;
		}
		if(unlikely(netdata_exit)) break;

		if(!vdo_proc_net_rpc_nfsd) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_rpc_nfsd().");
			sunow = sutime();
			vdo_proc_net_rpc_nfsd = do_proc_net_rpc_nfsd(rrd_update_every, (sutime_proc_net_rpc_nfsd > 0)?sunow - sutime_proc_net_rpc_nfsd:0ULL);
			sutime_proc_net_rpc_nfsd = sunow;
		}
		if(unlikely(netdata_exit)) break;

		// END -- the job is done

		// --------------------------------------------------------------------

		if(!vdo_cpu_netdata) {
			getrusage(RUSAGE_THREAD, &thread);
			getrusage(RUSAGE_SELF, &me);

			if(!stcpu_thread) stcpu_thread = rrdset_find("netdata.plugin_proc_cpu");
			if(!stcpu_thread) {
				stcpu_thread = rrdset_create("netdata", "plugin_proc_cpu", NULL, "proc.internal", NULL, "NetData Proc Plugin CPU usage", "milliseconds/s", 131000, rrd_update_every, RRDSET_TYPE_STACKED);

				rrddim_add(stcpu_thread, "user",  NULL,  1, 1000, RRDDIM_INCREMENTAL);
				rrddim_add(stcpu_thread, "system", NULL, 1, 1000, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(stcpu_thread);

			rrddim_set(stcpu_thread, "user"  , thread.ru_utime.tv_sec * 1000000ULL + thread.ru_utime.tv_usec);
			rrddim_set(stcpu_thread, "system", thread.ru_stime.tv_sec * 1000000ULL + thread.ru_stime.tv_usec);
			rrdset_done(stcpu_thread);

			// ----------------------------------------------------------------

			if(!stcpu) stcpu = rrdset_find("netdata.server_cpu");
			if(!stcpu) {
				stcpu = rrdset_create("netdata", "server_cpu", NULL, "netdata", NULL, "NetData CPU usage", "milliseconds/s", 130000, rrd_update_every, RRDSET_TYPE_STACKED);

				rrddim_add(stcpu, "user",  NULL,  1, 1000, RRDDIM_INCREMENTAL);
				rrddim_add(stcpu, "system", NULL, 1, 1000, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(stcpu);

			rrddim_set(stcpu, "user"  , me.ru_utime.tv_sec * 1000000ULL + me.ru_utime.tv_usec);
			rrddim_set(stcpu, "system", me.ru_stime.tv_sec * 1000000ULL + me.ru_stime.tv_usec);
			rrdset_done(stcpu);

			// ----------------------------------------------------------------

			if(!stclients) stclients = rrdset_find("netdata.clients");
			if(!stclients) {
				stclients = rrdset_create("netdata", "clients", NULL, "netdata", NULL, "NetData Web Clients", "connected clients", 131000, rrd_update_every, RRDSET_TYPE_LINE);

				rrddim_add(stclients, "clients",  NULL,  1, 1, RRDDIM_ABSOLUTE);
			}
			else rrdset_next(stclients);

			rrddim_set(stclients, "clients", global_statistics.connected_clients);
			rrdset_done(stclients);

			// ----------------------------------------------------------------

			if(!streqs) streqs = rrdset_find("netdata.requests");
			if(!streqs) {
				streqs = rrdset_create("netdata", "requests", NULL, "netdata", NULL, "NetData Web Requests", "requests/s", 131100, rrd_update_every, RRDSET_TYPE_LINE);

				rrddim_add(streqs, "requests",  NULL,  1, 1, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(streqs);

			rrddim_set(streqs, "requests", global_statistics.web_requests);
			rrdset_done(streqs);

			// ----------------------------------------------------------------

			if(!stbytes) stbytes = rrdset_find("netdata.net");
			if(!stbytes) {
				stbytes = rrdset_create("netdata", "net", NULL, "netdata", NULL, "NetData Network Traffic", "kilobits/s", 131200, rrd_update_every, RRDSET_TYPE_AREA);

				rrddim_add(stbytes, "in",  NULL,  8, 1024, RRDDIM_INCREMENTAL);
				rrddim_add(stbytes, "out",  NULL,  -8, 1024, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(stbytes);

			rrddim_set(stbytes, "in", global_statistics.bytes_received);
			rrddim_set(stbytes, "out", global_statistics.bytes_sent);
			rrdset_done(stbytes);
		}
	}

	pthread_exit(NULL);
	return NULL;
}
