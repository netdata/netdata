// SPDX-License-Identifier: GPL-3.0-or-later

#include "os.h"

// ----------------------------------------------------------------------------
// system functions
// to retrieve settings of the system

int processors = 1;
long get_system_cpus(void) {
    processors = 1;

#ifdef __APPLE__
    int32_t tmp_processors;

        if (unlikely(GETSYSCTL_BY_NAME("hw.logicalcpu", tmp_processors))) {
            error("Assuming system has %d processors.", processors);
        } else {
            processors = tmp_processors;
        }

        return processors;
#elif __FreeBSD__
    int32_t tmp_processors;

        if (unlikely(GETSYSCTL_BY_NAME("hw.ncpu", tmp_processors))) {
            error("Assuming system has %d processors.", processors);
        } else {
            processors = tmp_processors;
        }

        return processors;
#else

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/stat", netdata_configured_host_prefix);

    procfile *ff = procfile_open(filename, NULL, PROCFILE_FLAG_DEFAULT);
    if(!ff) {
        error("Cannot open file '%s'. Assuming system has %d processors.", filename, processors);
        return processors;
    }

    ff = procfile_readall(ff);
    if(!ff) {
        error("Cannot open file '%s'. Assuming system has %d processors.", filename, processors);
        return processors;
    }

    processors = 0;
    unsigned int i;
    for(i = 0; i < procfile_lines(ff); i++) {
        if(!procfile_linewords(ff, i)) continue;

        if(strncmp(procfile_lineword(ff, i, 0), "cpu", 3) == 0) processors++;
    }
    processors--;
    if(processors < 1) processors = 1;

    procfile_close(ff);

    debug(D_SYSTEM, "System has %d processors.", processors);
    return processors;

#endif /* __APPLE__, __FreeBSD__ */
}

pid_t pid_max = 32768;
pid_t get_system_pid_max(void) {
#ifdef __APPLE__
    // As we currently do not know a solution to query pid_max from the os
        // we use the number defined in bsd/sys/proc_internal.h in XNU sources
        pid_max = 99999;
        return pid_max;
#elif __FreeBSD__
    int32_t tmp_pid_max;

        if (unlikely(GETSYSCTL_BY_NAME("kern.pid_max", tmp_pid_max))) {
            pid_max = 99999;
            error("Assuming system's maximum pid is %d.", pid_max);
        } else {
            pid_max = tmp_pid_max;
        }

        return pid_max;
#else

    static char read = 0;
    if(unlikely(read)) return pid_max;
    read = 1;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/sys/kernel/pid_max", netdata_configured_host_prefix);

    unsigned long long max = 0;
    if(read_single_number_file(filename, &max) != 0) {
        error("Cannot open file '%s'. Assuming system supports %d pids.", filename, pid_max);
        return pid_max;
    }

    if(!max) {
        error("Cannot parse file '%s'. Assuming system supports %d pids.", filename, pid_max);
        return pid_max;
    }

    pid_max = (pid_t) max;
    return pid_max;

#endif /* __APPLE__, __FreeBSD__ */
}

unsigned int system_hz;
void get_system_HZ(void) {
    long ticks;

    if ((ticks = sysconf(_SC_CLK_TCK)) == -1) {
        error("Cannot get system clock ticks");
    }

    system_hz = (unsigned int) ticks;
}

// =====================================================================================================================
// FreeBSD

#if (TARGET_OS == OS_FREEBSD)

int getsysctl_by_name(const char *name, void *ptr, size_t len) {
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

int getsysctl_simple(const char *name, int *mib, size_t miblen, void *ptr, size_t len) {
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

int getsysctl(const char *name, int *mib, size_t miblen, void *ptr, size_t *len) {
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

int getsysctl_mib(const char *name, int *mib, size_t len) {
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


#endif


// =====================================================================================================================
// MacOS

#if (TARGET_OS == OS_MACOS)

int getsysctl_by_name(const char *name, void *ptr, size_t len) {
    size_t nlen = len;

    if (unlikely(sysctlbyname(name, ptr, &nlen, NULL, 0) == -1)) {
        error("MACOS: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(nlen != len)) {
        error("MACOS: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)len, (unsigned long)nlen);
        return 1;
    }
    return 0;
}

#endif // (TARGET_OS == OS_MACOS)

