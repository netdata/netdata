// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SERVICE_H
#define NETDATA_SERVICE_H 1

#define NETDATA_PLUGIN_HOOK_SERVICE \
    { \
        .name = "SERVICE", \
        .config_section = NULL, \
        .config_name = NULL, \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = service_main \
    },

extern void *service_main(void *ptr);

#endif //NETDATA_SERVICE_H
