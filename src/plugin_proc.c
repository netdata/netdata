#include <pthread.h>
#include <sys/time.h>
#include <sys/resource.h>
#include <strings.h>
#include <unistd.h>

#include "global_statistics.h"
#include "common.h"
#include "config.h"
#include "log.h"
#include "rrd.h"
#include "plugin_proc.h"

void *proc_main(void *ptr)
{
	if(ptr) { ; }

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	struct rusage me, me_last;
	struct timeval last, now;

	gettimeofday(&last, NULL);
	last.tv_sec -= rrd_update_every;
	
	// disable (by default) various interface that are not needed
	config_get_boolean("plugin:proc:/proc/net/dev", "interface lo", 0);
	config_get_boolean("plugin:proc:/proc/net/dev", "interface fireqos_monitor", 0);

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
	int vdo_cpu_netdata 			= !config_get_boolean("plugin:proc", "netdata server resources", 1);

	RRDSET *stcpu = NULL, *stclients = NULL, *streqs = NULL, *stbytes = NULL;

	gettimeofday(&last, NULL);
	getrusage(RUSAGE_SELF, &me_last);

	unsigned long long usec = 0, susec = 0;
	for(;1;) {
		
		// BEGIN -- the job to be done
		
		if(!vdo_proc_sys_kernel_random_entropy_avail) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_sys_kernel_random_entropy_avail().");
			vdo_proc_sys_kernel_random_entropy_avail = do_proc_sys_kernel_random_entropy_avail(rrd_update_every, usec+susec);
		}
		if(!vdo_proc_net_dev) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_dev().");
			vdo_proc_net_dev = do_proc_net_dev(rrd_update_every, usec+susec);
		}
		if(!vdo_proc_diskstats) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_diskstats().");
			vdo_proc_diskstats = do_proc_diskstats(rrd_update_every, usec+susec);
		}
		if(!vdo_proc_net_snmp) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_snmp().");
			vdo_proc_net_snmp = do_proc_net_snmp(rrd_update_every, usec+susec);
		}

		if(!vdo_proc_net_netstat) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_netstat().");
			vdo_proc_net_netstat = do_proc_net_netstat(rrd_update_every, usec+susec);
		}

		if(!vdo_proc_net_stat_conntrack) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_stat_conntrack().");
			vdo_proc_net_stat_conntrack	= do_proc_net_stat_conntrack(rrd_update_every, usec+susec);
		}

		if(!vdo_proc_net_ip_vs_stats) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling vdo_proc_net_ip_vs_stats().");
			vdo_proc_net_ip_vs_stats = do_proc_net_ip_vs_stats(rrd_update_every, usec+susec);
		}

		if(!vdo_proc_stat) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_stat().");
			vdo_proc_stat = do_proc_stat(rrd_update_every, usec+susec);
		}

		if(!vdo_proc_meminfo) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling vdo_proc_meminfo().");
			vdo_proc_meminfo = do_proc_meminfo(rrd_update_every, usec+susec);
		}

		if(!vdo_proc_vmstat) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling vdo_proc_vmstat().");
			vdo_proc_vmstat = do_proc_vmstat(rrd_update_every, usec+susec);
		}

		if(!vdo_proc_net_rpc_nfsd) {
			debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_proc_net_rpc_nfsd().");
			vdo_proc_net_rpc_nfsd = do_proc_net_rpc_nfsd(rrd_update_every, usec+susec);
		}

		// END -- the job is done
		
		// find the time to sleep in order to wait exactly update_every seconds
		gettimeofday(&now, NULL);
		usec = usecdiff(&now, &last) - susec;
		debug(D_PROCNETDEV_LOOP, "PROCNETDEV: last loop took %llu usec (worked for %llu, sleeped for %llu).", usec + susec, usec, susec);
		
		if(usec < (rrd_update_every * 1000000ULL / 2ULL)) susec = (rrd_update_every * 1000000ULL) - usec;
		else susec = rrd_update_every * 1000000ULL / 2ULL;
		
		// --------------------------------------------------------------------

		if(!vdo_cpu_netdata && getrusage(RUSAGE_SELF, &me) == 0) {
		
			unsigned long long cpuuser = me.ru_utime.tv_sec * 1000000ULL + me.ru_utime.tv_usec;
			unsigned long long cpusyst = me.ru_stime.tv_sec * 1000000ULL + me.ru_stime.tv_usec;

			if(!stcpu) stcpu = rrdset_find("netdata.server_cpu");
			if(!stcpu) {
				stcpu = rrdset_create("netdata", "server_cpu", NULL, "netdata", "NetData CPU usage", "milliseconds/s", 9999, rrd_update_every, RRDSET_TYPE_STACKED);

				rrddim_add(stcpu, "user",  NULL,  1, 1000 * rrd_update_every, RRDDIM_INCREMENTAL);
				rrddim_add(stcpu, "system", NULL, 1, 1000 * rrd_update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(stcpu);

			rrddim_set(stcpu, "user", cpuuser);
			rrddim_set(stcpu, "system", cpusyst);
			rrdset_done(stcpu);
			
			bcopy(&me, &me_last, sizeof(struct rusage));

			// ----------------------------------------------------------------

			if(!stclients) stclients = rrdset_find("netdata.clients");
			if(!stclients) {
				stclients = rrdset_create("netdata", "clients", NULL, "netdata", "NetData Web Clients", "connected clients", 11000, rrd_update_every, RRDSET_TYPE_LINE);

				rrddim_add(stclients, "clients",  NULL,  1, 1, RRDDIM_ABSOLUTE);
			}
			else rrdset_next(stclients);

			rrddim_set(stclients, "clients", global_statistics.connected_clients);
			rrdset_done(stclients);

			// ----------------------------------------------------------------

			if(!streqs) streqs = rrdset_find("netdata.requests");
			if(!streqs) {
				streqs = rrdset_create("netdata", "requests", NULL, "netdata", "NetData Web Requests", "requests/s", 12000, rrd_update_every, RRDSET_TYPE_LINE);

				rrddim_add(streqs, "requests",  NULL,  1, 1 * rrd_update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(streqs);

			rrddim_set(streqs, "requests", global_statistics.web_requests);
			rrdset_done(streqs);

			// ----------------------------------------------------------------

			if(!stbytes) stbytes = rrdset_find("netdata.net");
			if(!stbytes) {
				stbytes = rrdset_create("netdata", "net", NULL, "netdata", "NetData Network Traffic", "kilobits/s", 13000, rrd_update_every, RRDSET_TYPE_AREA);

				rrddim_add(stbytes, "in",  NULL,  8, 1024 * rrd_update_every, RRDDIM_INCREMENTAL);
				rrddim_add(stbytes, "out",  NULL,  -8, 1024 * rrd_update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(stbytes);

			rrddim_set(stbytes, "in", global_statistics.bytes_received);
			rrddim_set(stbytes, "out", global_statistics.bytes_sent);
			rrdset_done(stbytes);
		}

		usleep(susec);
		
		// copy current to last
		bcopy(&now, &last, sizeof(struct timeval));
	}

	return NULL;
}
