// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_SERVICE_H
#define NETDATA_DAEMON_SERVICE_H

#include "libnetdata/libnetdata.h"

typedef enum {
    ABILITY_WEB_REQUESTS          = (1 << 0),
    ABILITY_STREAMING_CONNECTIONS = (1 << 1),
    SERVICE_COLLECTORS            = (1 << 2),
    SERVICE_REPLICATION           = (1 << 3),
    SERVICE_WEB_SERVER            = (1 << 4),
    SERVICE_ACLK                  = (1 << 5),
    SERVICE_HEALTH                = (1 << 6),
    SERVICE_STREAMING             = (1 << 7),
    SERVICE_STREAMING_CONNECTOR   = (1 << 8),
    SERVICE_CONTEXT               = (1 << 9),
    SERVICE_ANALYTICS             = (1 << 10),
    SERVICE_EXPORTERS             = (1 << 11),
    SERVICE_HTTPD                 = (1 << 12),
    SERVICE_SYSTEMD               = (1 << 13),
} SERVICE_TYPE;

typedef void (*force_quit_t)(void *data);
typedef void (*request_quit_t)(void *data);

void service_exits(void);
bool service_running(SERVICE_TYPE service);
struct service_thread *service_register(request_quit_t request_quit_callback, force_quit_t force_quit_callback, void *data);

void service_signal_exit(SERVICE_TYPE service);
bool service_wait_exit(SERVICE_TYPE service, usec_t timeout_ut);

#endif //NETDATA_DAEMON_SERVICE_H
