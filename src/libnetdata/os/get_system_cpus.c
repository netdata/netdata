// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

size_t os_get_system_cpus_cached(bool cache) {
    static size_t processors = 0;

    if(likely(cache && processors > 0))
        return processors;

    SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);

    long p = 0;

#if defined(_SC_NPROCESSORS_ONLN)
    // currently online processors
    p = sysconf(_SC_NPROCESSORS_ONLN);
    if(p > 1) goto done; // if it is 1, we will try harder below
#endif

#if defined(_SC_NPROCESSORS_CONF)
    // all configured processors (online and offline)
    p = sysconf(_SC_NPROCESSORS_CONF);
    if(p > 1) goto done; // if it is 1, we will try harder below
#endif

#if defined(OS_FREEBSD) || defined(OS_MACOS)
    #if defined(OS_MACOS)
        #define HW_CPU_NAME "hw.logicalcpu"
    #else
        #define HW_CPU_NAME "kern.smp.cpus"
    #endif

    if (unlikely(GETSYSCTL_BY_NAME(HW_CPU_NAME, p) || p < 1))
        goto error;

#elif defined(OS_LINUX)
    // we will count the number of cpus in /proc/stat

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/stat",
              (netdata_configured_host_prefix) ? netdata_configured_host_prefix : "");

    procfile *ff = procfile_open(filename, NULL, PROCFILE_FLAG_DEFAULT);
    if(!ff || !(ff = procfile_readall(ff))) goto error;

    p = 0;
    unsigned int i;
    for(i = 0; i < procfile_lines(ff); i++) {
        if(!procfile_linewords(ff, i)) continue;

        const char *starting = procfile_lineword(ff, i, 0);
        if(strncmp(starting, "cpu", 3) == 0 && isdigit((uint8_t)starting[3]))
            p++;
    }
    procfile_close(ff);
    if(p < 1) goto error;

#elif defined(OS_WINDOWS)

    SYSTEM_INFO sysInfo;
    GetSystemInfo(&sysInfo);
    p = sysInfo.dwNumberOfProcessors;
    if(p < 1) goto error;

#else

    p = 1;

#endif

done:
    processors = (size_t)p;
    spinlock_unlock(&spinlock);
    return processors;

error:
    spinlock_unlock(&spinlock);
    processors = 1;
    netdata_log_error("Cannot detect number of CPU cores. Assuming the system has %zu processors.", processors);
    return processors;
}

// --------------------------------------------------------------------------------------------------------------------
// cpuset cpus

static inline unsigned long cpuset_str2ul(char **s) {
    unsigned long n = 0;
    char c;
    for(c = **s; c >= '0' && c <= '9' ; c = *(++*s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

#if defined(OS_LINUX)
size_t os_read_cpuset_cpus(const char *filename, size_t system_cpus) {
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
            if (isspace((uint8_t)*s)) {
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
#endif