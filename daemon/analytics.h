// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ANALYTICS_H
#define NETDATA_ANALYTICS_H 1

#include "../daemon/common.h"

/* Max number of seconds before the META analytics is sent */
#define ANALYTICS_MAX_SLEEP_SEC 20

/* Maximum number of hits to log */
#define ANALYTICS_MAX_PROMETHEUS_HITS 255
#define ANALYTICS_MAX_SHELL_HITS 255
#define ANALYTICS_MAX_JSON_HITS 255
#define ANALYTICS_MAX_DASHBOARD_HITS 255

#define NETDATA_PLUGIN_HOOK_ANALYTICS \
    { \
        .name = "ANALYTICS", \
        .config_section = NULL, \
        .config_name = NULL, \
        .enabled = 0, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = analytics_main \
    },

struct analytics_data {
    char *netdata_config_stream_enabled;
    char *netdata_config_memory_mode;
    char *netdata_exporting_connectors;
    char *netdata_config_exporting_enabled;
    char *netdata_allmetrics_prometheus_used;
    char *netdata_allmetrics_shell_used;
    char *netdata_allmetrics_json_used;
    char *netdata_dashboard_used;
    char *netdata_collectors;
    char *netdata_collectors_count;
    char *netdata_buildinfo;
    char *netdata_config_page_cache_size;
    char *netdata_config_multidb_disk_quota;
    char *netdata_config_https_enabled;
    char *netdata_config_web_enabled;
    char *netdata_config_release_channel;
    char *netdata_mirrored_host_count;
    char *netdata_mirrored_hosts_reachable;
    char *netdata_mirrored_hosts_unreachable;

    uint8_t prometheus_hits;
    uint8_t shell_hits;
    uint8_t json_hits;
    uint8_t dashboard_hits;
};

extern void *analytics_main(void *ptr);
extern void analytics_get_data(char *name, BUFFER *wb);
extern void set_late_global_environment();
extern void analytics_free_data();
extern void set_global_environment();
extern void send_statistics( const char *action, const char *action_result, const char *action_data);
extern void analytics_log_shell();
extern void analytics_log_json();
extern void analytics_log_prometheus();
extern void analytics_log_dashboard();
extern void analytics_gather_meta_data();

extern struct analytics_data analytics_data;

#endif //NETDATA_ANALYTICS_H
