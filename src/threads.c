#include "common.h"

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

const char *netdata_thread_tag(void) {
    return ((netdata_thread && netdata_thread->tag && *netdata_thread->tag)?netdata_thread->tag:"MAIN");
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
        info("%s: thread with task id %d finished", netdata_thread_tag(), gettid());

    freez((void *)netdata_thread->tag);
    netdata_thread->tag = NULL;

    freez(netdata_thread);
    netdata_thread = NULL;
}

static void *thread_start(void *ptr) {
    netdata_thread = (NETDATA_THREAD *)ptr;

    if(!(netdata_thread->options & NETDATA_THREAD_OPTION_DONT_LOG_STARTUP))
        info("%s: thread created with task id %d", netdata_thread_tag(), gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("%s: cannot set pthread cancel type to DEFERRED.", netdata_thread_tag());

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("%s: cannot set pthread cancel state to ENABLE.", netdata_thread_tag());

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
        error("%s: failed to create new thread for %s. pthread_create() failed with code %d", netdata_thread_tag(), tag, ret);

    else {
        if (!(options & NETDATA_THREAD_OPTION_JOINABLE)) {
            int ret2 = pthread_detach(*thread);
            if (ret2 != 0)
                error("%s: cannot request detach of newly created %s thread. pthread_detach() failed with code %d", netdata_thread_tag(), tag, ret2);
        }
    }

    return ret;
}

// ----------------------------------------------------------------------------
// netdata_thread_cancel

int netdata_thread_cancel(netdata_thread_t thread) {
    int ret = pthread_cancel(thread);
    if(ret != 0)
        error("%s: cannot cancel thread. pthread_cancel() failed with code %d.", netdata_thread_tag(), ret);

    return ret;
}

// ----------------------------------------------------------------------------
// netdata_thread_join

int netdata_thread_join(netdata_thread_t thread, void **retval) {
    int ret = pthread_join(thread, retval);
    if(ret != 0)
        error("%s: cannot join thread. pthread_join() failed with code %d.", netdata_thread_tag(), ret);

    return ret;
}

int netdata_thread_detach(pthread_t thread) {
    int ret = pthread_detach(thread);
    if(ret != 0)
        error("%s: cannot detach thread. pthread_detach() failed with code %d.", netdata_thread_tag(), ret);

    return ret;
}
