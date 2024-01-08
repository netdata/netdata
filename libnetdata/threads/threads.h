// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_THREADS_H
#define NETDATA_THREADS_H 1

#include "../libnetdata.h"

pid_t gettid(void);

typedef enum {
    NETDATA_THREAD_OPTION_DEFAULT          = 0 << 0,
    NETDATA_THREAD_OPTION_JOINABLE         = 1 << 0,
    NETDATA_THREAD_OPTION_DONT_LOG_STARTUP = 1 << 1,
    NETDATA_THREAD_OPTION_DONT_LOG_CLEANUP = 1 << 2,
    NETDATA_THREAD_OPTION_DONT_LOG         = NETDATA_THREAD_OPTION_DONT_LOG_STARTUP|NETDATA_THREAD_OPTION_DONT_LOG_CLEANUP,
} NETDATA_THREAD_OPTIONS;

#define netdata_thread_cleanup_push(func, arg) pthread_cleanup_push(func, arg)
#define netdata_thread_cleanup_pop(execute) pthread_cleanup_pop(execute)

void netdata_thread_set_tag(const char *tag);

typedef pthread_t netdata_thread_t;

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

#define NETDATA_THREAD_TAG_MAX 100
const char *netdata_thread_tag(void);
int netdata_thread_tag_exists(void);

#define THREAD_TAG_STREAM_RECEIVER "RCVR"
#define THREAD_TAG_STREAM_SENDER "SNDR"


size_t netdata_threads_init(void);
void netdata_threads_init_after_fork(size_t stacksize);
void netdata_threads_init_for_external_plugins(size_t stacksize);

int netdata_thread_create(netdata_thread_t *thread, const char *tag, NETDATA_THREAD_OPTIONS options, void *(*start_routine) (void *), void *arg);

#ifdef NETDATA_INTERNAL_CHECKS
#define netdata_thread_cancel(thread) netdata_thread_cancel_with_trace(thread, __LINE__, __FILE__, __FUNCTION__)
int netdata_thread_cancel_with_trace(netdata_thread_t thread, int line, const char *file, const char *function);
#else
int netdata_thread_cancel(netdata_thread_t thread);
#endif

int netdata_thread_join(netdata_thread_t thread, void **retval);
int netdata_thread_detach(pthread_t thread);

#define NETDATA_THREAD_NAME_MAX 15
void uv_thread_set_name_np(uv_thread_t ut, const char* name);
void os_thread_get_current_name_np(char threadname[NETDATA_THREAD_NAME_MAX + 1]);

void webrtc_set_thread_name(void);

#define netdata_thread_self pthread_self
#define netdata_thread_testcancel pthread_testcancel

#endif //NETDATA_THREADS_H
