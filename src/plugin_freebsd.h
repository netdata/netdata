#ifndef NETDATA_PLUGIN_FREEBSD_H
#define NETDATA_PLUGIN_FREEBSD_H 1

#include <sys/sysctl.h>

#define GETSYSCTL(name, var) getsysctl(name, &(var), sizeof(var))

void *freebsd_main(void *ptr);

extern int do_freebsd_sysctl(int update_every, usec_t dt);

static inline int getsysctl(const char *name, void *ptr, size_t len)
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
