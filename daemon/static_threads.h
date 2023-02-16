// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STATIC_THREADS_H
#define NETDATA_STATIC_THREADS_H

#include "common.h"

struct netdata_static_thread {
    // the name of the thread as it should appear in the logs
    char *name;

    // the section of netdata.conf to check if this is enabled or not
    char *config_section;

    // the name of the config option to check if it is true or false
    char *config_name;

    // the current status of the thread
    volatile sig_atomic_t enabled;

    // internal use, to maintain a pointer to the created thread
    netdata_thread_t *thread;

    // an initialization function to run before spawning the thread
    void (*init_routine) (void);

    // the threaded worker
    void *(*start_routine) (void *);

    // the environment variable to create
    char *env_name;

    // global variable
    bool *global_variable;
};

#define NETDATA_MAIN_THREAD_RUNNING     CONFIG_BOOLEAN_YES
#define NETDATA_MAIN_THREAD_EXITING     (CONFIG_BOOLEAN_YES + 1)
#define NETDATA_MAIN_THREAD_EXITED      CONFIG_BOOLEAN_NO

extern const struct netdata_static_thread static_threads_common[];
extern const struct netdata_static_thread static_threads_linux[];
extern const struct netdata_static_thread static_threads_freebsd[];
extern const struct netdata_static_thread static_threads_macos[];

struct netdata_static_thread *
static_threads_concat(const struct netdata_static_thread *lhs,
                      const struct netdata_static_thread *rhs);

struct netdata_static_thread *static_threads_get();

#endif /* NETDATA_STATIC_THREADS_H */
