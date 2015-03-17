#ifndef NETDATA_PLUGIN_PROC_H
#define NETDATA_PLUGIN_PROC_H 1

void *proc_main(void *ptr);

extern int do_proc_net_dev(int update_every, unsigned long long dt);
extern int do_proc_diskstats(int update_every, unsigned long long dt);
extern int do_proc_net_snmp(int update_every, unsigned long long dt);
extern int do_proc_net_netstat(int update_every, unsigned long long dt);
extern int do_proc_net_stat_conntrack(int update_every, unsigned long long dt);
extern int do_proc_net_ip_vs_stats(int update_every, unsigned long long dt);
extern int do_proc_stat(int update_every, unsigned long long dt);
extern int do_proc_meminfo(int update_every, unsigned long long dt);
extern int do_proc_vmstat(int update_every, unsigned long long dt);
extern int do_proc_net_rpc_nfsd(int update_every, unsigned long long dt);

#endif /* NETDATA_PLUGIN_PROC_H */
