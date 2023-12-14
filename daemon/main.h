// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MAIN_H
#define NETDATA_MAIN_H 1

#include "common.h"

extern struct config netdata_config;

/**
 * This struct contains information about command line options.
 */
struct option_def {
    /** The option character */
    const char val;
    /** The name of the long option. */
    const char *description;
    /** Short description what the option does */
    /** Name of the argument displayed in SYNOPSIS */
    const char *arg_name;
    /** Default value if not set */
    const char *default_value;
};

void cancel_main_threads(void);
int killpid(pid_t pid);

typedef enum {
    ABILITY_DATA_QUERIES          = (1 << 0),
    ABILITY_WEB_REQUESTS          = (1 << 1),
    ABILITY_STREAMING_CONNECTIONS = (1 << 2),
    SERVICE_MAINTENANCE           = (1 << 3),
    SERVICE_COLLECTORS            = (1 << 4),
    SERVICE_REPLICATION           = (1 << 5),
    SERVICE_WEB_SERVER            = (1 << 6),
    SERVICE_ACLK                  = (1 << 7),
    SERVICE_HEALTH                = (1 << 8),
    SERVICE_STREAMING             = (1 << 9),
    SERVICE_CONTEXT               = (1 << 10),
    SERVICE_ANALYTICS             = (1 << 11),
    SERVICE_EXPORTERS             = (1 << 12),
    SERVICE_ACLKSYNC              = (1 << 13),
    SERVICE_HTTPD                 = (1 << 14)
} SERVICE_TYPE;

typedef enum {
    SERVICE_THREAD_TYPE_NETDATA,
    SERVICE_THREAD_TYPE_LIBUV,
    SERVICE_THREAD_TYPE_EVENT_LOOP,
} SERVICE_THREAD_TYPE;

typedef void (*force_quit_t)(void *data);
typedef void (*request_quit_t)(void *data);

void service_exits(void);
bool service_running(SERVICE_TYPE service);
struct service_thread *service_register(SERVICE_THREAD_TYPE thread_type, request_quit_t request_quit_callback, force_quit_t force_quit_callback, void *data, bool update __maybe_unused);

#endif /* NETDATA_MAIN_H */
