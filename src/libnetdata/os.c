// SPDX-License-Identifier: GPL-3.0-or-later

#include "os.h"

// ----------------------------------------------------------------------------
// system functions
// to retrieve settings of the system

#define CPUS_FOR_COLLECTORS 0
#define CPUS_FOR_NETDATA 1

long get_system_cpus_with_cache(bool cache, bool for_netdata) {
    static long processors[2] = { 0, 0 };

    int index = for_netdata ? CPUS_FOR_NETDATA : CPUS_FOR_COLLECTORS;

    if(likely(cache && processors[index] > 0))
        return processors[index];

#if defined(__APPLE__) || defined(__FreeBSD__)
#if defined(__APPLE__)
#define HW_CPU_NAME "hw.logicalcpu"
#else
#define HW_CPU_NAME "hw.ncpu"
#endif

    int32_t tmp_processors;
    bool error = false;

    if (unlikely(GETSYSCTL_BY_NAME(HW_CPU_NAME, tmp_processors)))
        error = true;
    else
        processors[index] = tmp_processors;

    if(processors[index] < 1) {
        processors[index] = 1;

        if(error)
            netdata_log_error("Assuming system has %d processors.", processors[index]);
    }

    return processors[index];
#else

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/stat",
              (!for_netdata && netdata_configured_host_prefix) ? netdata_configured_host_prefix : "");

    procfile *ff = procfile_open(filename, NULL, PROCFILE_FLAG_DEFAULT);
    if(!ff) {
        processors[index] = 1;
        netdata_log_error("Cannot open file '%s'. Assuming system has %ld processors.", filename, processors[index]);
        return processors[index];
    }

    ff = procfile_readall(ff);
    if(!ff) {
        processors[index] = 1;
        netdata_log_error("Cannot open file '%s'. Assuming system has %ld processors.", filename, processors[index]);
        return processors[index];
    }

    long tmp_processors = 0;
    unsigned int i;
    for(i = 0; i < procfile_lines(ff); i++) {
        if(!procfile_linewords(ff, i)) continue;

        if(strncmp(procfile_lineword(ff, i, 0), "cpu", 3) == 0)
            tmp_processors++;
    }
    procfile_close(ff);

    processors[index] = --tmp_processors;

    if(processors[index] < 1)
        processors[index] = 1;

    netdata_log_debug(D_SYSTEM, "System has %ld processors.", processors[index]);
    return processors[index];

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
            netdata_log_error("Assuming system's maximum pid is %d.", pid_max);
        } else {
            pid_max = tmp_pid_max;
        }

        return pid_max;
#else

    static char read = 0;
    if(unlikely(read)) return pid_max;
    read = 1;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/sys/kernel/pid_max", netdata_configured_host_prefix?netdata_configured_host_prefix:"");

    unsigned long long max = 0;
    if(read_single_number_file(filename, &max) != 0) {
        netdata_log_error("Cannot open file '%s'. Assuming system supports %d pids.", filename, pid_max);
        return pid_max;
    }

    if(!max) {
        netdata_log_error("Cannot parse file '%s'. Assuming system supports %d pids.", filename, pid_max);
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
        netdata_log_error("Cannot get system clock ticks");
    }

    system_hz = (unsigned int) ticks;
}

static inline unsigned long cpuset_str2ul(char **s) {
    unsigned long n = 0;
    char c;
    for(c = **s; c >= '0' && c <= '9' ; c = *(++*s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

unsigned long read_cpuset_cpus(const char *filename, long system_cpus) {
    static char *buf = NULL;
    static size_t buf_size = 0;

    if(!buf) {
        buf_size = 100U + 6 * system_cpus + 1; // taken from kernel/cgroup/cpuset.c
        buf = mallocz(buf_size);
    }

    int ret = read_txt_file(filename, buf, buf_size);

    if(!ret) {
        char *s = buf;
        unsigned long ncpus = 0;

        // parse the cpuset string and calculate the number of cpus the cgroup is allowed to use
        while (*s) {
            if (isspace(*s)) {
                s++;
                continue;
            }
            unsigned long n = cpuset_str2ul(&s);
            ncpus++;
            if(*s == ',') {
                s++;
                continue;
            }
            if(*s == '-') {
                s++;
                unsigned long m = cpuset_str2ul(&s);
                ncpus += m - n; // calculate the number of cpus in the region
            }
            s++;
        }

        if(!ncpus)
            return 0;

        return ncpus;
    }

    return 0;
}

// =====================================================================================================================
// FreeBSD

#if __FreeBSD__

const char *os_type = "freebsd";

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


// =====================================================================================================================
// MacOS

#if __APPLE__

const char *os_type = "macos";

int getsysctl_by_name(const char *name, void *ptr, size_t len) {
    size_t nlen = len;

    if (unlikely(sysctlbyname(name, ptr, &nlen, NULL, 0) == -1)) {
        netdata_log_error("MACOS: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(nlen != len)) {
        netdata_log_error("MACOS: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)len, (unsigned long)nlen);
        return 1;
    }
    return 0;
}

#endif

// =====================================================================================================================
// Linux

#if __linux__

const char *os_type = "linux";

#endif
