// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// system functions
// to retrieve settings of the system

unsigned int system_hz;
void os_get_system_HZ(void) {
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

unsigned long os_read_cpuset_cpus(const char *filename, long system_cpus) {
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

// =====================================================================================================================
// os_type

#if defined(OS_LINUX)
const char *os_type = "linux";
#endif

#if defined(OS_FREEBSD)
const char *os_type = "freebsd";
#endif

#if defined(OS_MACOS)
const char *os_type = "macos";
#endif

#if defined(OS_WINDOWS)
const char *os_type = "windows";
#endif

