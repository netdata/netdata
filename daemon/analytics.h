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
    char *NETDATA_CONFIG_STREAM_ENABLED;
    char *NETDATA_CONFIG_MEMORY_MODE;
    char *NETDATA_EXPORTING_CONNECTORS;
    char *NETDATA_CONFIG_EXPORTING_ENABLED;
    char *NETDATA_ALLMETRICS_PROMETHEUS_USED;
    char *NETDATA_ALLMETRICS_SHELL_USED;
    char *NETDATA_ALLMETRICS_JSON_USED;

    uint8_t prometheus_hits;
    uint8_t shell_hits;
    uint8_t json_hits;
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

extern struct analytics_data analytics_data;

#endif //NETDATA_ANALYTICS_H
