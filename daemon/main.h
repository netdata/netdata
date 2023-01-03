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
void netdata_cleanup_and_exit(int ret) NORETURN;
void send_statistics(const char *action, const char *action_result, const char *action_data);

typedef enum {
    SERVICE_MAINTENANCE           = (1 << 0),
    SERVICE_COLLECTORS            = (1 << 1),
    SERVICE_ML_TRAINING           = (1 << 2),
    SERVICE_ML_PREDICTION         = (1 << 3),
    SERVICE_REPLICATION           = (1 << 4),
    SERVICE_DATA_QUERIES          = (1 << 5), // ABILITY
    SERVICE_WEB_REQUESTS          = (1 << 6), // ABILITY
    SERVICE_WEB_SERVER            = (1 << 7),
    SERVICE_ACLK                  = (1 << 8),
    SERVICE_HEALTH                = (1 << 9),
    SERVICE_STREAMING             = (1 << 10),
    SERVICE_STREAMING_CONNECTIONS = (1 << 11), // ABILITY
    SERVICE_CONTEXT               = (1 << 12),
    SERVICE_ANALYTICS             = (1 << 13),
    SERVICE_EXPORTERS             = (1 << 14),
} SERVICE_TYPE;

typedef enum {
    SERVICE_THREAD_TYPE_NETDATA,
    SERVICE_THREAD_TYPE_LIBUV,
} SERVICE_THREAD_TYPE;

typedef void (*force_quit_t)(void *data);

void service_exits(void);
bool service_running(SERVICE_TYPE service);

#endif /* NETDATA_MAIN_H */
