// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_THREADS_H
#define NETDATA_THREADS_H 1

#include "../libnetdata.h"

typedef enum __attribute__((packed)) {
    NETDATA_THREAD_OPTION_DEFAULT          = 0 << 0,
    NETDATA_THREAD_OPTION_JOINABLE         = 1 << 0,
    NETDATA_THREAD_OPTION_DONT_LOG_STARTUP = 1 << 1,
    NETDATA_THREAD_OPTION_DONT_LOG_CLEANUP = 1 << 2,
    NETDATA_THREAD_STATUS_STARTED          = 1 << 3,
    NETDATA_THREAD_STATUS_FINISHED         = 1 << 4,
    NETDATA_THREAD_STATUS_JOINED           = 1 << 5,
} NETDATA_THREAD_OPTIONS;

#define NETDATA_THREAD_OPTIONS_ALL (NETDATA_THREAD_OPTION_JOINABLE | NETDATA_THREAD_OPTION_DONT_LOG_STARTUP | NETDATA_THREAD_OPTION_DONT_LOG_CLEANUP)
#define NETDATA_THREAD_OPTION_DONT_LOG (NETDATA_THREAD_OPTION_DONT_LOG_STARTUP | NETDATA_THREAD_OPTION_DONT_LOG_CLEANUP)

#define netdata_thread_cleanup_push(func, arg) pthread_cleanup_push(func, arg)
#define netdata_thread_cleanup_pop(execute) pthread_cleanup_pop(execute)

void nd_thread_tag_set(const char *tag);

typedef struct nd_thread ND_THREAD;

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
    ND_THREAD *thread;

    // a function to call to check it should be enabled or not
    bool (*enable_routine) (void);

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
const char *nd_thread_tag(void);
int nd_thread_has_tag(void);

#define THREAD_TAG_STREAM_RECEIVER "RCVR"
#define THREAD_TAG_STREAM_SENDER "SNDR"

size_t netdata_threads_init(void);
void netdata_threads_init_after_fork(size_t stacksize);
void netdata_threads_init_for_external_plugins(size_t stacksize);

ND_THREAD *nd_thread_create(const char *tag, NETDATA_THREAD_OPTIONS options, void *(*start_routine) (void *), void *arg);
int nd_thread_join(ND_THREAD * nti);
ND_THREAD *nd_thread_self(void);
bool nd_thread_is_me(ND_THREAD *nti);

typedef void (*nd_thread_canceller)(void *data);
void nd_thread_register_canceller(nd_thread_canceller cb, void *data);
void nd_thread_signal_cancel(ND_THREAD *nti);
bool nd_thread_signaled_to_cancel(void);

#define ND_THREAD_TAG_MAX 15
void uv_thread_set_name_np(const char* name);
void webrtc_set_thread_name(void);

#ifdef NETDATA_INTERNAL_CHECKS
void nd_thread_rwlock_read_locked(void);
void nd_thread_rwlock_read_unlocked(void);
void nd_thread_rwlock_write_locked(void);
void nd_thread_rwlock_write_unlocked(void);
void nd_thread_mutex_locked(void);
void nd_thread_mutex_unlocked(void);
void nd_thread_spinlock_locked(void);
void nd_thread_spinlock_unlocked(void);
void nd_thread_rwspinlock_read_locked(void);
void nd_thread_rwspinlock_read_unlocked(void);
void nd_thread_rwspinlock_write_locked(void);
void nd_thread_rwspinlock_write_unlocked(void);
#else
#define nd_thread_rwlock_read_locked() debug_dummy()
#define nd_thread_rwlock_read_unlocked() debug_dummy()
#define nd_thread_rwlock_write_locked() debug_dummy()
#define nd_thread_rwlock_write_unlocked() debug_dummy()
#define nd_thread_mutex_locked() debug_dummy()
#define nd_thread_mutex_unlocked() debug_dummy()
#define nd_thread_spinlock_locked() debug_dummy()
#define nd_thread_spinlock_unlocked() debug_dummy()
#define nd_thread_rwspinlock_read_locked() debug_dummy()
#define nd_thread_rwspinlock_read_unlocked() debug_dummy()
#define nd_thread_rwspinlock_write_locked() debug_dummy()
#define nd_thread_rwspinlock_write_unlocked() debug_dummy()
#endif

void nd_thread_can_run_sql(bool exclude);
bool nd_thread_runs_sql(void);

#endif //NETDATA_THREADS_H
