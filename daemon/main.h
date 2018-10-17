// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MAIN_H
#define NETDATA_MAIN_H 1

#include "common.h"

extern struct config netdata_config;

#define NETDATA_MAIN_THREAD_RUNNING   CONFIG_BOOLEAN_YES
#define NETDATA_MAIN_THREAD_EXITING  (CONFIG_BOOLEAN_YES + 1)
#define NETDATA_MAIN_THREAD_EXITED    CONFIG_BOOLEAN_NO

/**
 * This struct contains information about command line options.
 */
struct option_def {
    /** The option character */
    const char val;
    /** The name of the long option. */
    const char *description;
    /** Short descripton what the option does */
    /** Name of the argument displayed in SYNOPSIS */
    const char *arg_name;
    /** Default value if not set */
    const char *default_value;
};

struct netdata_static_thread {
    char *name;                         // the name of the thread as it should appear in the logs

    char *config_section;               // the section of netdata.conf to check if this is enabled or not
    char *config_name;                  // the name of the config option to check if it is true or false

    volatile sig_atomic_t enabled;      // the current status of the thread

    netdata_thread_t *thread;           // internal use, to maintain a pointer to the created thread

    void (*init_routine) (void);        // an initialization function to run before spawning the thread
    void *(*start_routine) (void *);    // the threaded worker
};

extern void cancel_main_threads(void);
extern int killpid(pid_t pid, int signal);
extern void netdata_cleanup_and_exit(int ret) NORETURN;

#endif /* NETDATA_MAIN_H */
