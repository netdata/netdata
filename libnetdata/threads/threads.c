// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

static pthread_attr_t *netdata_threads_attr = NULL;

// ----------------------------------------------------------------------------
// per thread data

typedef struct {
    void *arg;
    pthread_t *thread;
    char tag[NETDATA_THREAD_NAME_MAX + 1];
    void *(*start_routine) (void *);
    NETDATA_THREAD_OPTIONS options;
} NETDATA_THREAD;

static __thread NETDATA_THREAD *netdata_thread = NULL;

inline int netdata_thread_tag_exists(void) {
    return (netdata_thread && *netdata_thread->tag);
}

static const char *thread_name_get(bool recheck) {
    static __thread char threadname[NETDATA_THREAD_NAME_MAX + 1] = "";

    if(netdata_thread_tag_exists())
        strncpyz(threadname, netdata_thread->tag, NETDATA_THREAD_NAME_MAX);
    else {
        if(!recheck && threadname[0])
            return threadname;

#if defined(__FreeBSD__)
        pthread_get_name_np(pthread_self(), threadname, NETDATA_THREAD_NAME_MAX + 1);
        if(strcmp(threadname, "netdata") == 0)
            strncpyz(threadname, "MAIN", NETDATA_THREAD_NAME_MAX);
#elif defined(__APPLE__)
        strncpyz(threadname, "MAIN", NETDATA_THREAD_NAME_MAX);
#elif defined(HAVE_PTHREAD_GETNAME_NP)
        pthread_getname_np(pthread_self(), threadname, NETDATA_THREAD_NAME_MAX + 1);
        if(strcmp(threadname, "netdata") == 0)
            strncpyz(threadname, "MAIN", NETDATA_THREAD_NAME_MAX);
#else
        strncpyz(threadname, "MAIN", NETDATA_THREAD_NAME_MAX);
#endif
    }

    return threadname;
}

const char *netdata_thread_tag(void) {
    return thread_name_get(false);
}

static size_t webrtc_id = 0;
static __thread bool webrtc_name_set = false;
void webrtc_set_thread_name(void) {
    if(!netdata_thread && !webrtc_name_set) {
        webrtc_name_set = true;
        char threadname[NETDATA_THREAD_NAME_MAX + 1];

#if defined(__FreeBSD__)
        snprintfz(threadname, NETDATA_THREAD_NAME_MAX, "WEBRTC[%zu]", __atomic_fetch_add(&webrtc_id, 1, __ATOMIC_RELAXED));
        pthread_set_name_np(pthread_self(), threadname);
#elif defined(__APPLE__)
        snprintfz(threadname, NETDATA_THREAD_NAME_MAX, "WEBRTC[%zu]", __atomic_fetch_add(&webrtc_id, 1, __ATOMIC_RELAXED));
        pthread_setname_np(threadname);
#elif defined(HAVE_PTHREAD_GETNAME_NP)
        pthread_getname_np(pthread_self(), threadname, NETDATA_THREAD_NAME_MAX+1);
        if(strcmp(threadname, "netdata") == 0) {
            snprintfz(threadname, NETDATA_THREAD_NAME_MAX, "WEBRTC[%zu]", __atomic_fetch_add(&webrtc_id, 1, __ATOMIC_RELAXED));
            pthread_setname_np(pthread_self(), threadname);
        }
#else
        snprintfz(threadname, NETDATA_THREAD_NAME_MAX, "WEBRTC[%zu]", __atomic_fetch_add(&webrtc_id, 1, __ATOMIC_RELAXED));
        pthread_setname_np(pthread_self(), threadname);
#endif

        thread_name_get(true);
    }
}

// ----------------------------------------------------------------------------
// compatibility library functions

static __thread pid_t gettid_cached_tid = 0;
pid_t gettid(void) {
    pid_t tid = 0;

    if(likely(gettid_cached_tid > 0))
        return gettid_cached_tid;

#ifdef __FreeBSD__

    tid = (pid_t)pthread_getthreadid_np();

#elif defined(__APPLE__)

    #if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED >= 1060)
        uint64_t curthreadid;
        pthread_threadid_np(NULL, &curthreadid);
        tid = (pid_t)curthreadid;
    #else /* __MAC_OS_X_VERSION_MIN_REQUIRED */
        tid = (pid_t)pthread_self;
    #endif /* __MAC_OS_X_VERSION_MIN_REQUIRED */

#else /* __APPLE__*/

    tid = (pid_t)syscall(SYS_gettid);

#endif /* __FreeBSD__, __APPLE__*/

    gettid_cached_tid = tid;
    return tid;
}

// ----------------------------------------------------------------------------
// early initialization

size_t netdata_threads_init(void) {
    int i;

    // --------------------------------------------------------------------
    // get the required stack size of the threads of netdata

    netdata_threads_attr = callocz(1, sizeof(pthread_attr_t));
    i = pthread_attr_init(netdata_threads_attr);
    if(i != 0)
        fatal("pthread_attr_init() failed with code %d.", i);

    size_t stacksize = 0;
    i = pthread_attr_getstacksize(netdata_threads_attr, &stacksize);
    if(i != 0)
        fatal("pthread_attr_getstacksize() failed with code %d.", i);
    else
        debug(D_OPTIONS, "initial pthread stack size is %zu bytes", stacksize);

    return stacksize;
}

// ----------------------------------------------------------------------------
// late initialization

void netdata_threads_init_after_fork(size_t stacksize) {
    int i;

    // ------------------------------------------------------------------------
    // set pthread stack size

    if(netdata_threads_attr && stacksize > (size_t)PTHREAD_STACK_MIN) {
        i = pthread_attr_setstacksize(netdata_threads_attr, stacksize);
        if(i != 0)
            error("pthread_attr_setstacksize() to %zu bytes, failed with code %d.", stacksize, i);
        else
            info("Set threads stack size to %zu bytes", stacksize);
    }
    else
        error("Invalid pthread stacksize %zu", stacksize);
}

// ----------------------------------------------------------------------------
// netdata_thread_create

void rrdset_thread_rda_free(void);
void sender_thread_buffer_free(void);
void query_target_free(void);
void service_exits(void);

static void thread_cleanup(void *ptr) {
    if(netdata_thread != ptr) {
        NETDATA_THREAD *info = (NETDATA_THREAD *)ptr;
        error("THREADS: internal error - thread local variable does not match the one passed to this function. Expected thread '%s', passed thread '%s'", netdata_thread->tag, info->tag);
    }

    if(!(netdata_thread->options & NETDATA_THREAD_OPTION_DONT_LOG_CLEANUP))
        info("thread with task id %d finished", gettid());

    sender_thread_buffer_free();
    rrdset_thread_rda_free();
    query_target_free();
    thread_cache_destroy();
    service_exits();
    worker_unregister();

    netdata_thread->tag[0] = '\0';

    freez(netdata_thread);
    netdata_thread = NULL;
}

static void thread_set_name_np(NETDATA_THREAD *nt) {

    if (nt && nt->tag[0]) {
        int ret = 0;

        char threadname[NETDATA_THREAD_NAME_MAX+1];
        strncpyz(threadname, nt->tag, NETDATA_THREAD_NAME_MAX);

#if defined(__FreeBSD__)
        pthread_set_name_np(pthread_self(), threadname);
#elif defined(__APPLE__)
        ret = pthread_setname_np(threadname);
#else
        ret = pthread_setname_np(pthread_self(), threadname);
#endif

        if (ret != 0)
            error("cannot set pthread name of %d to %s. ErrCode: %d", gettid(), threadname, ret);
        else
            info("set name of thread %d to %s", gettid(), threadname);

    }
}

void uv_thread_set_name_np(uv_thread_t ut, const char* name) {
    int ret = 0;

    char threadname[NETDATA_THREAD_NAME_MAX+1];
    strncpyz(threadname, name, NETDATA_THREAD_NAME_MAX);

#if defined(__FreeBSD__)
    pthread_set_name_np(ut, threadname);
#elif defined(__APPLE__)
    // Apple can only set its own name
    UNUSED(ut);
#else
    ret = pthread_setname_np(ut, threadname);
#endif

    thread_name_get(true);

    if (ret)
        info("cannot set libuv thread name to %s. Err: %d", threadname, ret);
}

void os_thread_get_current_name_np(char threadname[NETDATA_THREAD_NAME_MAX + 1])
{
    threadname[0] = '\0';
#if defined(__FreeBSD__)
    pthread_get_name_np(pthread_self(), threadname, NETDATA_THREAD_NAME_MAX + 1);
#elif defined(HAVE_PTHREAD_GETNAME_NP) /* Linux & macOS */
    (void)pthread_getname_np(pthread_self(), threadname, NETDATA_THREAD_NAME_MAX + 1);
#endif
}

static void *netdata_thread_init(void *ptr) {
    netdata_thread = (NETDATA_THREAD *)ptr;

    if(!(netdata_thread->options & NETDATA_THREAD_OPTION_DONT_LOG_STARTUP))
        info("thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("cannot set pthread cancel state to ENABLE.");

    thread_set_name_np(ptr);

    void *ret = NULL;
    pthread_cleanup_push(thread_cleanup, ptr);
            ret = netdata_thread->start_routine(netdata_thread->arg);
    pthread_cleanup_pop(1);

    return ret;
}

int netdata_thread_create(netdata_thread_t *thread, const char *tag, NETDATA_THREAD_OPTIONS options, void *(*start_routine) (void *), void *arg) {
    NETDATA_THREAD *info = mallocz(sizeof(NETDATA_THREAD));
    info->arg = arg;
    info->thread = thread;
    info->start_routine = start_routine;
    info->options = options;
    strncpyz(info->tag, tag, NETDATA_THREAD_NAME_MAX);

    int ret = pthread_create(thread, netdata_threads_attr, netdata_thread_init, info);
    if(ret != 0)
        error("failed to create new thread for %s. pthread_create() failed with code %d", tag, ret);

    else {
        if (!(options & NETDATA_THREAD_OPTION_JOINABLE)) {
            int ret2 = pthread_detach(*thread);
            if (ret2 != 0)
                error("cannot request detach of newly created %s thread. pthread_detach() failed with code %d", tag, ret2);
        }
    }

    return ret;
}

// ----------------------------------------------------------------------------
// netdata_thread_cancel
#ifdef NETDATA_INTERNAL_CHECKS
int netdata_thread_cancel_with_trace(netdata_thread_t thread, int line, const char *file, const char *function) {
#else
int netdata_thread_cancel(netdata_thread_t thread) {
#endif
    int ret = pthread_cancel(thread);
    if(ret != 0)
#ifdef NETDATA_INTERNAL_CHECKS
        error("cannot cancel thread. pthread_cancel() failed with code %d at %d@%s, function %s()", ret, line, file, function);
#else
        error("cannot cancel thread. pthread_cancel() failed with code %d.", ret);
#endif

    return ret;
}

// ----------------------------------------------------------------------------
// netdata_thread_join

int netdata_thread_join(netdata_thread_t thread, void **retval) {
    int ret = pthread_join(thread, retval);
    if(ret != 0)
        error("cannot join thread. pthread_join() failed with code %d.", ret);

    return ret;
}

int netdata_thread_detach(pthread_t thread) {
    int ret = pthread_detach(thread);
    if(ret != 0)
        error("cannot detach thread. pthread_detach() failed with code %d.", ret);

    return ret;
}
