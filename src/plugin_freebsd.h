#ifndef NETDATA_PLUGIN_FREEBSD_H
#define NETDATA_PLUGIN_FREEBSD_H 1

#include <sys/sysctl.h>

void *freebsd_main(void *ptr);

extern int freebsd_plugin_init();

extern int do_vm_loadavg(int update_every, usec_t dt);
extern int do_freebsd_sysctl_old(int update_every, usec_t dt);

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
    if (unlikely(nlen != *len)) {
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
