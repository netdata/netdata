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

extern void *analytics_main(void *ptr);
extern void set_late_global_environment();
extern void set_global_environment();
extern void send_statistics( const char *action, const char *action_result, const char *action_data);

#endif //NETDATA_ANALYTICS_H
