// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

const char *netdata_configured_hostname            = NULL;
const char *netdata_configured_user_config_dir     = CONFIG_DIR;
const char *netdata_configured_stock_config_dir    = LIBCONFIG_DIR;
const char *netdata_configured_log_dir             = LOG_DIR;
const char *netdata_configured_primary_plugins_dir = PLUGINS_DIR;
const char *netdata_configured_web_dir             = WEB_DIR;
const char *netdata_configured_cache_dir           = CACHE_DIR;
const char *netdata_configured_varlib_dir          = VARLIB_DIR;
const char *netdata_configured_lock_dir            = VARLIB_DIR "/lock";
const char *netdata_configured_cloud_dir           = VARLIB_DIR "/cloud.d";
const char *netdata_configured_home_dir            = VARLIB_DIR;
const char *netdata_configured_host_prefix         = NULL;
const char *netdata_configured_timezone            = NULL;
const char *netdata_configured_abbrev_timezone     = NULL;
int32_t netdata_configured_utc_offset              = 0;

bool netdata_ready = false;

long get_netdata_cpus(void) {
    static long processors = 0;

    if(processors)
        return processors;

    long cores_proc_stat = os_get_system_cpus_cached(false, true);
    long cores_cpuset_v1 = (long)os_read_cpuset_cpus("/sys/fs/cgroup/cpuset/cpuset.cpus", cores_proc_stat);
    long cores_cpuset_v2 = (long)os_read_cpuset_cpus("/sys/fs/cgroup/cpuset.cpus", cores_proc_stat);

    if(cores_cpuset_v2)
        processors = cores_cpuset_v2;
    else if(cores_cpuset_v1)
        processors = cores_cpuset_v1;
    else
        processors = cores_proc_stat;

    long cores_user_configured = config_get_number(CONFIG_SECTION_GLOBAL, "cpu cores", processors);

    errno_clear();
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

    if(processors < 1)
        processors = 1;

    return processors;
}
