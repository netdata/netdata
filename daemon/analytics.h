// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ANALYTICS_H
#define NETDATA_ANALYTICS_H 1

#include "../daemon/common.h"

/* Max number of seconds before the META analytics is sent */
#define ANALYTICS_MAX_SLEEP_SEC 20

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

extern void *analytics_main(void *ptr);
extern void set_global_environment();
extern void send_statistics( const char *action, const char *action_result, const char *action_data);

#endif //NETDATA_ANALYTICS_H
