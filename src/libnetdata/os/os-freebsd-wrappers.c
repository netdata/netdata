// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#if defined(OS_FREEBSD)

int getsysctl_by_name(const char *name, void *ptr, size_t len) {
    size_t nlen = len;

    if (unlikely(sysctlbyname(name, ptr, &nlen, NULL, 0) == -1)) {
        netdata_log_error("FREEBSD: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(nlen != len)) {
        netdata_log_error("FREEBSD: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)len, (unsigned long)nlen);
        return 1;
    }
    return 0;
}

int getsysctl_simple(const char *name, int *mib, size_t miblen, void *ptr, size_t len) {
    size_t nlen = len;

    if (unlikely(!mib[0]))
        if (unlikely(getsysctl_mib(name, mib, miblen)))
            return 1;

    if (unlikely(sysctl(mib, miblen, ptr, &nlen, NULL, 0) == -1)) {
        netdata_log_error("FREEBSD: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(nlen != len)) {
        netdata_log_error("FREEBSD: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)len, (unsigned long)nlen);
        return 1;
    }

    return 0;
}

int getsysctl(const char *name, int *mib, size_t miblen, void *ptr, size_t *len) {
    size_t nlen = *len;

    if (unlikely(!mib[0]))
        if (unlikely(getsysctl_mib(name, mib, miblen)))
            return 1;

    if (unlikely(sysctl(mib, miblen, ptr, len, NULL, 0) == -1)) {
        netdata_log_error("FREEBSD: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(ptr != NULL && nlen != *len)) {
        netdata_log_error("FREEBSD: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)*len, (unsigned long)nlen);
        return 1;
    }

    return 0;
}

int getsysctl_mib(const char *name, int *mib, size_t len) {
    size_t nlen = len;

    if (unlikely(sysctlnametomib(name, mib, &nlen) == -1)) {
        netdata_log_error("FREEBSD: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(nlen != len)) {
        netdata_log_error("FREEBSD: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)len, (unsigned long)nlen);
        return 1;
    }
    return 0;
}

#endif
