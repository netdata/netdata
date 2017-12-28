#ifndef NETDATA_THREADS_H
#define NETDATA_THREADS_H

extern pid_t gettid(void);

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

#define NETDATA_THREAD_TAG_MAX 50
extern const char *netdata_thread_tag(void);

extern size_t netdata_threads_init(void);
extern void netdata_threads_init_after_fork(size_t stacksize);

extern int netdata_thread_create(netdata_thread_t *thread, const char *tag, NETDATA_THREAD_OPTIONS options, void *(*start_routine) (void *), void *arg);
extern int netdata_thread_cancel(netdata_thread_t thread);
extern int netdata_thread_join(netdata_thread_t thread, void **retval);
extern int netdata_thread_detach(pthread_t thread);

#define netdata_thread_self pthread_self
#define netdata_thread_testcancel pthread_testcancel

#endif //NETDATA_THREADS_H
