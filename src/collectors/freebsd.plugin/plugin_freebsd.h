// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGIN_FREEBSD_H
#define NETDATA_PLUGIN_FREEBSD_H 1

#include "database/rrd.h"

#include <sys/sysctl.h>

#define KILO_FACTOR 1024
#define MEGA_FACTOR 1048576     // 1024 * 1024
#define GIGA_FACTOR 1073741824  // 1024 * 1024 * 1024

#define MAX_INT_DIGITS 10 // maximum number of digits for int

int freebsd_plugin_init();

int do_vm_loadavg(int update_every, usec_t dt);
int do_vm_vmtotal(int update_every, usec_t dt);
int do_kern_cp_time(int update_every, usec_t dt);
int do_kern_cp_times(int update_every, usec_t dt);
int do_dev_cpu_temperature(int update_every, usec_t dt);
int do_dev_cpu_0_freq(int update_every, usec_t dt);
int do_hw_intcnt(int update_every, usec_t dt);
int do_vm_stats_sys_v_intr(int update_every, usec_t dt);
int do_vm_stats_sys_v_soft(int update_every, usec_t dt);
int do_vm_stats_sys_v_swtch(int update_every, usec_t dt);
int do_vm_stats_sys_v_forks(int update_every, usec_t dt);
int do_vm_swap_info(int update_every, usec_t dt);
int do_system_ram(int update_every, usec_t dt);
int do_vm_stats_sys_v_swappgs(int update_every, usec_t dt);
int do_vm_stats_sys_v_pgfaults(int update_every, usec_t dt);
int do_kern_ipc_sem(int update_every, usec_t dt);
int do_kern_ipc_shm(int update_every, usec_t dt);
int do_kern_ipc_msq(int update_every, usec_t dt);
int do_uptime(int update_every, usec_t dt);
int do_net_isr(int update_every, usec_t dt);
int do_net_inet_tcp_states(int update_every, usec_t dt);
int do_net_inet_tcp_stats(int update_every, usec_t dt);
int do_net_inet_udp_stats(int update_every, usec_t dt);
int do_net_inet_icmp_stats(int update_every, usec_t dt);
int do_net_inet_ip_stats(int update_every, usec_t dt);
int do_net_inet6_ip6_stats(int update_every, usec_t dt);
int do_net_inet6_icmp6_stats(int update_every, usec_t dt);
int do_getifaddrs(int update_every, usec_t dt);
int do_getmntinfo(int update_every, usec_t dt);
int do_kern_devstat(int update_every, usec_t dt);
int do_kstat_zfs_misc_arcstats(int update_every, usec_t dt);
int do_kstat_zfs_misc_zio_trim(int update_every, usec_t dt);
int do_ipfw(int update_every, usec_t dt);

// metrics that need to be shared among data collectors
extern unsigned long long zfs_arcstats_shrinkable_cache_size_bytes;

#endif /* NETDATA_PLUGIN_FREEBSD_H */
