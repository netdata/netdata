// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

char *netdata_configured_hostname            = NULL;
char *netdata_configured_user_config_dir     = CONFIG_DIR;
char *netdata_configured_stock_config_dir    = LIBCONFIG_DIR;
char *netdata_configured_log_dir             = LOG_DIR;
char *netdata_configured_primary_plugins_dir = PLUGINS_DIR;
char *netdata_configured_web_dir             = WEB_DIR;
char *netdata_configured_cache_dir           = CACHE_DIR;
char *netdata_configured_varlib_dir          = VARLIB_DIR;
char *netdata_configured_lock_dir            = VARLIB_DIR "/lock";
char *netdata_configured_home_dir            = VARLIB_DIR;
char *netdata_configured_host_prefix         = NULL;
char *netdata_configured_timezone            = NULL;
char *netdata_configured_abbrev_timezone     = NULL;
int32_t netdata_configured_utc_offset        = 0;

bool netdata_ready = false;

#if defined( DISABLE_CLOUD ) || !defined( ENABLE_ACLK )
int netdata_cloud_enabled = CONFIG_BOOLEAN_NO;
#else
int netdata_cloud_enabled = CONFIG_BOOLEAN_AUTO;
#endif

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

const char *cloud_status_to_string(CLOUD_STATUS status) {
    switch(status) {
        default:
        case CLOUD_STATUS_UNAVAILABLE:
            return "unavailable";

        case CLOUD_STATUS_AVAILABLE:
            return "available";

        case CLOUD_STATUS_DISABLED:
            return "disabled";

        case CLOUD_STATUS_BANNED:
            return "banned";

        case CLOUD_STATUS_OFFLINE:
            return "offline";

        case CLOUD_STATUS_ONLINE:
            return "online";
    }
}

CLOUD_STATUS cloud_status(void) {
#ifdef ENABLE_ACLK
    if(aclk_disable_runtime)
        return CLOUD_STATUS_BANNED;

    if(aclk_connected)
        return CLOUD_STATUS_ONLINE;

    if(netdata_cloud_enabled == CONFIG_BOOLEAN_YES) {
        char *agent_id = get_agent_claimid();
        bool claimed = agent_id != NULL;
        freez(agent_id);

        if(claimed)
            return CLOUD_STATUS_OFFLINE;
    }

    if(netdata_cloud_enabled != CONFIG_BOOLEAN_NO)
        return CLOUD_STATUS_AVAILABLE;

    return CLOUD_STATUS_DISABLED;
#else
    return CLOUD_STATUS_UNAVAILABLE;
#endif
}

time_t cloud_last_change(void) {
#ifdef ENABLE_ACLK
    time_t ret = MAX(last_conn_time_mqtt, last_disconnect_time);
    if(!ret) ret = netdata_start_time;
    return ret;
#else
    return netdata_start_time;
#endif
}

time_t cloud_next_connection_attempt(void) {
#ifdef ENABLE_ACLK
    return next_connection_attempt;
#else
    return 0;
#endif
}

size_t cloud_connection_id(void) {
#ifdef ENABLE_ACLK
    return aclk_connection_counter;
#else
    return 0;
#endif
}

const char *cloud_offline_reason() {
#ifdef ENABLE_ACLK
    if(!netdata_cloud_enabled)
        return "disabled";

    if(aclk_disable_runtime)
        return "banned";

    return aclk_status_to_string();
#else
    return "disabled";
#endif
}

const char *cloud_base_url() {
#ifdef ENABLE_ACLK
    return aclk_cloud_base_url;
#else
    return NULL;
#endif
}

CLOUD_STATUS buffer_json_cloud_status(BUFFER *wb, time_t now_s) {
    CLOUD_STATUS status = cloud_status();

    buffer_json_member_add_object(wb, "cloud");
    {
        size_t id = cloud_connection_id();
        time_t last_change = cloud_last_change();
        time_t next_connect = cloud_next_connection_attempt();
        buffer_json_member_add_uint64(wb, "id", id);
        buffer_json_member_add_string(wb, "status", cloud_status_to_string(status));
        buffer_json_member_add_time_t(wb, "since", last_change);
        buffer_json_member_add_time_t(wb, "age", now_s - last_change);

        if (status != CLOUD_STATUS_ONLINE)
            buffer_json_member_add_string(wb, "reason", cloud_offline_reason());

        if (status == CLOUD_STATUS_OFFLINE && next_connect > now_s) {
            buffer_json_member_add_time_t(wb, "next_check", next_connect);
            buffer_json_member_add_time_t(wb, "next_in", next_connect - now_s);
        }

        if (cloud_base_url())
            buffer_json_member_add_string(wb, "url", cloud_base_url());

        char *claim_id = get_agent_claimid();
        if(claim_id) {
            buffer_json_member_add_string(wb, "claim_id", claim_id);
            freez(claim_id);
        }
    }
    buffer_json_object_close(wb); // cloud

    return status;
}
