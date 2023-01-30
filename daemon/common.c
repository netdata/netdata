// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

char *netdata_configured_hostname            = NULL;
char *netdata_configured_user_config_dir     = CONFIG_DIR;
char *netdata_configured_stock_config_dir    = LIBCONFIG_DIR;
char *netdata_configured_log_dir             = LOG_DIR;
char *netdata_configured_primary_plugins_dir = NULL;
char *netdata_configured_web_dir             = WEB_DIR;
char *netdata_configured_cache_dir           = CACHE_DIR;
char *netdata_configured_varlib_dir          = VARLIB_DIR;
char *netdata_configured_lock_dir            = NULL;
char *netdata_configured_home_dir            = VARLIB_DIR;
char *netdata_configured_host_prefix         = NULL;
char *netdata_configured_timezone            = NULL;
char *netdata_configured_abbrev_timezone     = NULL;
int32_t netdata_configured_utc_offset        = 0;
int netdata_ready;
int netdata_cloud_setting;

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
        buf_size = 100U + 6 * system_cpus; // taken from kernel/cgroup/cpuset.c
        buf = mallocz(buf_size + 1);
    }

    int ret = read_file(filename, buf, buf_size);

    if(!ret) {
        char *s = buf;
        unsigned long ncpus = 0;

        // parse the cpuset string and calculate the number of cpus the cgroup is allowed to use
        while(*s) {
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

long get_netdata_cpus(void) {
    static long processors = 0;

    if(processors)
        return processors;

    long cores_proc_stat = get_system_cpus_with_cache(false, true);
    long cores_cpuset_v1 = (long)read_cpuset_cpus("/sys/fs/cgroup/cpuset/cpuset.cpus", cores_proc_stat);
    long cores_cpuset_v2 = (long)read_cpuset_cpus("/sys/fs/cgroup/cpuset.cpus", cores_proc_stat);

    if(cores_cpuset_v2)
        processors = cores_cpuset_v2;
    else if(cores_cpuset_v1)
        processors = cores_cpuset_v1;
    else
        processors = cores_proc_stat;

    long cores_user_configured = config_get_number(CONFIG_SECTION_GLOBAL, "cpu cores", processors);

    internal_error(true,
         "System CPUs: %ld, ("
         "system: %ld, cgroups cpuset v1: %ld, cgroups cpuset v2: %ld, netdata.conf: %ld"
         ")"
         , processors
         , cores_proc_stat
         , cores_cpuset_v1
         , cores_cpuset_v2
         , cores_user_configured
         );

    processors = cores_user_configured;

    return processors;
}
