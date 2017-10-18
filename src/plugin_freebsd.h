#ifndef NETDATA_PLUGIN_FREEBSD_H
#define NETDATA_PLUGIN_FREEBSD_H 1

#include <sys/sysctl.h>

#define KILO_FACTOR 1024
#define MEGA_FACTOR 1048576     // 1024 * 1024
#define GIGA_FACTOR 1073741824  // 1024 * 1024 * 1024

#define MAX_INT_DIGITS 10 // maximum number of digits for int

void *freebsd_main(void *ptr);

extern int freebsd_plugin_init();

extern int do_vm_loadavg(int update_every, usec_t dt);
extern int do_vm_vmtotal(int update_every, usec_t dt);
extern int do_kern_cp_time(int update_every, usec_t dt);
extern int do_kern_cp_times(int update_every, usec_t dt);
extern int do_dev_cpu_temperature(int update_every, usec_t dt);
extern int do_dev_cpu_0_freq(int update_every, usec_t dt);
extern int do_hw_intcnt(int update_every, usec_t dt);
extern int do_vm_stats_sys_v_intr(int update_every, usec_t dt);
extern int do_vm_stats_sys_v_soft(int update_every, usec_t dt);
extern int do_vm_stats_sys_v_swtch(int update_every, usec_t dt);
extern int do_vm_stats_sys_v_forks(int update_every, usec_t dt);
extern int do_vm_swap_info(int update_every, usec_t dt);
extern int do_system_ram(int update_every, usec_t dt);
extern int do_vm_stats_sys_v_swappgs(int update_every, usec_t dt);
extern int do_vm_stats_sys_v_pgfaults(int update_every, usec_t dt);
extern int do_kern_ipc_sem(int update_every, usec_t dt);
extern int do_kern_ipc_shm(int update_every, usec_t dt);
extern int do_kern_ipc_msq(int update_every, usec_t dt);
extern int do_uptime(int update_every, usec_t dt);
extern int do_net_isr(int update_every, usec_t dt);
extern int do_net_inet_tcp_states(int update_every, usec_t dt);
extern int do_net_inet_tcp_stats(int update_every, usec_t dt);
extern int do_net_inet_udp_stats(int update_every, usec_t dt);
extern int do_net_inet_icmp_stats(int update_every, usec_t dt);
extern int do_net_inet_ip_stats(int update_every, usec_t dt);
extern int do_net_inet6_ip6_stats(int update_every, usec_t dt);
extern int do_net_inet6_icmp6_stats(int update_every, usec_t dt);
extern int do_getifaddrs(int update_every, usec_t dt);
extern int do_getmntinfo(int update_every, usec_t dt);
extern int do_kern_devstat(int update_every, usec_t dt);
extern int do_kstat_zfs_misc_arcstats(int update_every, usec_t dt);
extern int do_kstat_zfs_misc_zio_trim(int update_every, usec_t dt);
extern int do_ipfw(int update_every, usec_t dt);

#define GETSYSCTL_MIB(name, mib) getsysctl_mib(name, mib, sizeof(mib)/sizeof(int))

static inline int getsysctl_mib(const char *name, int *mib, size_t len)
{
    size_t nlen = len;

    if (unlikely(sysctlnametomib(name, mib, &nlen) == -1)) {
        error("FREEBSD: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(nlen != len)) {
        error("FREEBSD: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)len, (unsigned long)nlen);
        return 1;
    }
    return 0;
}

#define GETSYSCTL_SIMPLE(name, mib, var) getsysctl_simple(name, mib, sizeof(mib)/sizeof(int), &(var), sizeof(var))
#define GETSYSCTL_WSIZE(name, mib, var, size) getsysctl_simple(name, mib, sizeof(mib)/sizeof(int), var, size)

static inline int getsysctl_simple(const char *name, int *mib, size_t miblen, void *ptr, size_t len)
{
    size_t nlen = len;

    if (unlikely(!mib[0]))
        if (unlikely(getsysctl_mib(name, mib, miblen)))
            return 1;

    if (unlikely(sysctl(mib, miblen, ptr, &nlen, NULL, 0) == -1)) {
        error("FREEBSD: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(nlen != len)) {
        error("FREEBSD: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)len, (unsigned long)nlen);
        return 1;
    }

    return 0;
}

#define GETSYSCTL_SIZE(name, mib, size) getsysctl(name, mib, sizeof(mib)/sizeof(int), NULL, &(size))
#define GETSYSCTL(name, mib, var, size) getsysctl(name, mib, sizeof(mib)/sizeof(int), &(var), &(size))

static inline int getsysctl(const char *name, int *mib, size_t miblen, void *ptr, size_t *len)
{
    size_t nlen = *len;

    if (unlikely(!mib[0]))
        if (unlikely(getsysctl_mib(name, mib, miblen)))
            return 1;

    if (unlikely(sysctl(mib, miblen, ptr, len, NULL, 0) == -1)) {
        error("FREEBSD: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(ptr != NULL && nlen != *len)) {
        error("FREEBSD: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)*len, (unsigned long)nlen);
        return 1;
    }

    return 0;
}

#define GETSYSCTL_BY_NAME(name, var) getsysctl_by_name(name, &(var), sizeof(var))

static inline int getsysctl_by_name(const char *name, void *ptr, size_t len)
{
    size_t nlen = len;

    if (unlikely(sysctlbyname(name, ptr, &nlen, NULL, 0) == -1)) {
        error("FREEBSD: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(nlen != len)) {
        error("FREEBSD: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)len, (unsigned long)nlen);
        return 1;
    }
    return 0;
}

#endif /* NETDATA_PLUGIN_FREEBSD_H */
