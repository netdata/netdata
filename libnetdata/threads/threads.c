// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

static size_t default_stacksize = 0, wanted_stacksize = 0;
static pthread_attr_t *attr = NULL;

// ----------------------------------------------------------------------------
// per thread data

typedef struct {
    void *arg;
    pthread_t *thread;
    const char *tag;
    void *(*start_routine) (void *);
    NETDATA_THREAD_OPTIONS options;
} NETDATA_THREAD;

static __thread NETDATA_THREAD *netdata_thread = NULL;

inline int netdata_thread_tag_exists(void) {
    return (netdata_thread && netdata_thread->tag && *netdata_thread->tag);
}

const char *netdata_thread_tag(void) {
    return (netdata_thread_tag_exists() ? netdata_thread->tag : "MAIN");
}

// ----------------------------------------------------------------------------
// compatibility library functions

pid_t gettid(void) {
#ifdef __FreeBSD__

    return (pid_t)pthread_getthreadid_np();

#elif defined(__APPLE__)

    #if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED >= 1060)
        uint64_t curthreadid;
        pthread_threadid_np(NULL, &curthreadid);
        return (pid_t)curthreadid;
    #else /* __MAC_OS_X_VERSION_MIN_REQUIRED */
        return (pid_t)pthread_self;
    #endif /* __MAC_OS_X_VERSION_MIN_REQUIRED */

#else /* __APPLE__*/

    return (pid_t)syscall(SYS_gettid);

#endif /* __FreeBSD__, __APPLE__*/
}

// ----------------------------------------------------------------------------
// early initialization

size_t netdata_threads_init(void) {
    int i;

    // --------------------------------------------------------------------
    // get the required stack size of the threads of netdata

    attr = callocz(1, sizeof(pthread_attr_t));
    i = pthread_attr_init(attr);
    if(i != 0)
        fatal("pthread_attr_init() failed with code %d.", i);

    i = pthread_attr_getstacksize(attr, &default_stacksize);
    if(i != 0)
        fatal("pthread_attr_getstacksize() failed with code %d.", i);
    else
        debug(D_OPTIONS, "initial pthread stack size is %zu bytes", default_stacksize);

    return default_stacksize;
}

// ----------------------------------------------------------------------------
// late initialization

void netdata_threads_init_after_fork(size_t stacksize) {
    wanted_stacksize = stacksize;
    int i;

    // ------------------------------------------------------------------------
    // set default pthread stack size

    if(attr && default_stacksize < wanted_stacksize && wanted_stacksize > 0) {
        i = pthread_attr_setstacksize(attr, wanted_stacksize);
        if(i != 0)
            fatal("pthread_attr_setstacksize() to %zu bytes, failed with code %d.", wanted_stacksize, i);
        else
            debug(D_SYSTEM, "Successfully set pthread stacksize to %zu bytes", wanted_stacksize);
    }
}


// ----------------------------------------------------------------------------
// netdata_thread_create

static void thread_cleanup(void *ptr) {
    if(netdata_thread != ptr) {
        NETDATA_THREAD *info = (NETDATA_THREAD *)ptr;
        error("THREADS: internal error - thread local variable does not match the one passed to this function. Expected thread '%s', passed thread '%s'", netdata_thread->tag, info->tag);
    }

    if(!(netdata_thread->options & NETDATA_THREAD_OPTION_DONT_LOG_CLEANUP))
        info("thread with task id %d finished", gettid());

    thread_cache_destroy();

    freez((void *)netdata_thread->tag);
    netdata_thread->tag = NULL;

    freez(netdata_thread);
    netdata_thread = NULL;
}

static void thread_set_name_np(NETDATA_THREAD *nt) {

    if (nt->tag) {
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

static void *thread_start(void *ptr) {
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
    info->tag = strdupz(tag);
    info->start_routine = start_routine;
    info->options = options;

    int ret = pthread_create(thread, attr, thread_start, info);
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

int netdata_thread_cancel(netdata_thread_t thread) {
    int ret = pthread_cancel(thread);
    if(ret != 0)
        error("cannot cancel thread. pthread_cancel() failed with code %d.", ret);

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
