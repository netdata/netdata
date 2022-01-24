// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ANALYTICS_H
#define NETDATA_ANALYTICS_H 1

#include "daemon/common.h"

/* Max number of seconds before the first META analytics is sent */
#define ANALYTICS_INIT_SLEEP_SEC 30

/* Send a META event every X seconds */
#define ANALYTICS_HEARTBEAT 7200

/* Maximum number of hits to log */
#define ANALYTICS_MAX_PROMETHEUS_HITS 255
#define ANALYTICS_MAX_SHELL_HITS 255
#define ANALYTICS_MAX_JSON_HITS 255
#define ANALYTICS_MAX_DASHBOARD_HITS 255

/* Needed to calculate the space needed for parameters */
#define ANALYTICS_NO_OF_ITEMS 39

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
    char *netdata_notification_methods;
    char *netdata_alarms_normal;
    char *netdata_alarms_warning;
    char *netdata_alarms_critical;
    char *netdata_charts_count;
    char *netdata_metrics_count;
    char *netdata_config_is_parent;
    char *netdata_config_hosts_available;
    char *netdata_host_cloud_available;
    char *netdata_host_aclk_available;
    char *netdata_host_aclk_protocol;
    char *netdata_host_aclk_implementation;
    char *netdata_host_agent_claimed;
    char *netdata_host_cloud_enabled;
    char *netdata_config_https_available;
    char *netdata_install_type;
    char *netdata_config_is_private_registry;
    char *netdata_config_use_private_registry;
    char *netdata_config_oom_score;
    char *netdata_prebuilt_distro;

    size_t data_length;

    uint8_t prometheus_hits;
    uint8_t shell_hits;
    uint8_t json_hits;
    uint8_t dashboard_hits;
};

extern void analytics_get_data(char *name, BUFFER *wb);
extern void set_late_global_environment(void);
extern void analytics_free_data(void);
extern void set_global_environment(void);
extern void send_statistics(const char *action, const char *action_result, const char *action_data);
extern void analytics_log_shell(void);
extern void analytics_log_json(void);
extern void analytics_log_prometheus(void);
extern void analytics_log_dashboard(void);
extern void analytics_gather_mutable_meta_data(void);
extern void analytics_report_oom_score(long long int score);
extern void get_system_timezone(void);

extern struct analytics_data analytics_data;

#endif //NETDATA_ANALYTICS_H
