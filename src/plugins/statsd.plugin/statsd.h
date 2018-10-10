// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STATSD_H
#define NETDATA_STATSD_H 1

#include "../../common.h"

#define STATSD_LISTEN_PORT 8125
#define STATSD_LISTEN_BACKLOG 4096

#define NETDATA_PLUGIN_HOOK_STATSD \
    { \
        .name = "STATSD", \
        .config_section = NULL, \
        .config_name = NULL, \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = statsd_main \
    },


extern void *statsd_main(void *ptr);

#endif //NETDATA_STATSD_H
