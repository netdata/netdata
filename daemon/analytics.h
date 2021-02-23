// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ANALYTICS_H
#define NETDATA_ANALYTICS_H 1

#include "../daemon/common.h"

#define NETDATA_PLUGIN_HOOK_ANALYTICS \
    { \
        .name = "ANALYTICS", \
        .config_section = NULL, \
        .config_name = NULL, \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = analytics_main \
    },

struct analytics_data {
    char *NETDATA_CONFIG_STREAM_ENABLED;
    char *NETDATA_CONFIG_IS_PARENT;
    char *NETDATA_CONFIG_MEMORY_MODE;
    char *NETDATA_CONFIG_PAGE_CACHE_SIZE;
    char *NETDATA_CONFIG_MULTIDB_DISK_QUOTA;
    char *NETDATA_CONFIG_HOSTS_AVAILABLE;
    char *NETDATA_CONFIG_ACLK_ENABLED;
    char *NETDATA_CONFIG_WEB_ENABLED;
    char *NETDATA_CONFIG_EXPORTING_ENABLED;
    char *NETDATA_HOST_ACLK_CONNECTED;
    char *NETDATA_HOST_CLAIMED;
    char *NETDATA_CONFIG_HTTPS_ENABLED;
    char *NETDATA_ALARMS_COUNT;
    char *NETDATA_CHARTS_COUNT;
    char *NETDATA_METRICS_COUNT;
    char *NETDATA_COLLECTORS_PLUGINS;
    char *NETDATA_COLLECTORS_MODULES;
    char *NETDATA_COLLECTORS_COUNT;
    char *NETDATA_NOTIFICATIONS_METHODS;
    
    
    
    
} analytics_data;

extern void *analytics_main(void *ptr);
extern void set_late_global_environment();
extern void set_global_environment();
extern void send_statistics( const char *action, const char *action_result, const char *action_data);

#endif //NETDATA_ANALYTICS_H
