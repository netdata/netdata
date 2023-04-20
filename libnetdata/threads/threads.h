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

typedef pthread_t netdata_thread_t;

#define NETDATA_THREAD_TAG_MAX 100
const char *netdata_thread_tag(void);
int netdata_thread_tag_exists(void);

size_t netdata_threads_init(void);
void netdata_threads_init_after_fork(size_t stacksize);

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
