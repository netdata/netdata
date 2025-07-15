// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ANALYTICS_H
#define NETDATA_ANALYTICS_H 1

#include "daemon/common.h"
#include "database/rrdhost-system-info.h"

/* Max number of seconds before the first META analytics is sent */
#define ANALYTICS_INIT_SLEEP_SEC (120)
#define ANALYTICS_INIT_IMMUTABLE_DATA_SEC (10)


/* Send a META event every X seconds */
#define ANALYTICS_HEARTBEAT (6 * 3600)

/* Maximum number of hits to log */
#define ANALYTICS_MAX_PROMETHEUS_HITS 255
#define ANALYTICS_MAX_SHELL_HITS 255
#define ANALYTICS_MAX_JSON_HITS 255
#define ANALYTICS_MAX_DASHBOARD_HITS 255

/* Needed to calculate the space needed for parameters */
#define ANALYTICS_NO_OF_ITEMS 40

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
    char *netdata_fail_reason;

    size_t data_length;

    size_t prometheus_hits;
    size_t shell_hits;
    size_t json_hits;
    size_t dashboard_hits;

    size_t charts_count;
    size_t metrics_count;
    SPINLOCK spinlock;

    bool exporting_enabled;
};

void set_late_analytics_variables(struct rrdhost_system_info *system_info);
void analytics_free_data(void);
void analytics_log_shell(void);
void analytics_log_json(void);
void analytics_log_prometheus(void);
void analytics_log_dashboard(void);
void analytics_gather_mutable_meta_data(void);
void analytics_report_oom_score(long long int score);
void get_system_timezone(void);
void analytics_reset(void);
void analytics_init(void);

bool analytics_check_enabled(void);

extern struct analytics_data analytics_data;

#endif //NETDATA_ANALYTICS_H
