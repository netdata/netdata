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

    errno = 0;
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
