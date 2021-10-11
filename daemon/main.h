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

extern void cancel_main_threads(void);
extern int killpid(pid_t pid);
extern void netdata_cleanup_and_exit(int ret) NORETURN;
extern void send_statistics(const char *action, const char *action_result, const char *action_data);

#endif /* NETDATA_MAIN_H */
