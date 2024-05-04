// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#define nd_thread_status_get(nti)           __atomic_load_n(&((nti)->options), __ATOMIC_ACQUIRE)
#define nd_thread_status_check(nti, flag)   (__atomic_load_n(&((nti)->options), __ATOMIC_ACQUIRE) & (flag))
#define nd_thread_status_set(nti, flag)     __atomic_or_fetch(&((nti)->options), flag, __ATOMIC_RELEASE)
#define nd_thread_status_clear(nti, flag)   __atomic_and_fetch(&((nti)->options), ~(flag), __ATOMIC_RELEASE)

struct nd_thread {
    void *arg;
    pid_t tid;
    char tag[NETDATA_THREAD_NAME_MAX + 1];
    void *ret; // the return value of start routine
    void *(*start_routine) (void *);
    NETDATA_THREAD_OPTIONS options;
    pthread_t thread;
    struct nd_thread *prev, *next;
};

static struct {
    struct {
        SPINLOCK spinlock;
        ND_THREAD *list;
    } exited;

    struct {
        SPINLOCK spinlock;
        ND_THREAD *list;
    } running;

    pthread_attr_t *attr;
} threads_globals = {
    .exited = {
        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
        .list = NULL,
    },
    .running = {
        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
        .list = NULL,
    },
    .attr = NULL,
};

// ----------------------------------------------------------------------------
// per thread data

static __thread ND_THREAD *_nd_thread_info = NULL;

inline int netdata_thread_tag_exists(void) {
    return (_nd_thread_info && _nd_thread_info->tag[0]);
}

static const char *thread_name_get(bool recheck) {
    static __thread char threadname[NETDATA_THREAD_NAME_MAX + 1] = "";

    if(netdata_thread_tag_exists())
        strncpyz(threadname, _nd_thread_info->tag, NETDATA_THREAD_NAME_MAX);
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
    if(!_nd_thread_info && !webrtc_name_set) {
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
// early initialization

size_t netdata_threads_init(void) {
    int i;

    // --------------------------------------------------------------------
    // get the required stack size of the threads of netdata

    if(!threads_globals.attr) {
        threads_globals.attr = callocz(1, sizeof(pthread_attr_t));
        i = pthread_attr_init(threads_globals.attr);
        if (i != 0)
            fatal("pthread_attr_init() failed with code %d.", i);
    }

    size_t stacksize = 0;
    i = pthread_attr_getstacksize(threads_globals.attr, &stacksize);
    if(i != 0)
        fatal("pthread_attr_getstacksize() failed with code %d.", i);

    return stacksize;
}

// ----------------------------------------------------------------------------
// late initialization

void netdata_threads_init_after_fork(size_t stacksize) {
    int i;

    // ------------------------------------------------------------------------
    // set pthread stack size

    if(threads_globals.attr && stacksize > (size_t)PTHREAD_STACK_MIN) {
        i = pthread_attr_setstacksize(threads_globals.attr, stacksize);
        if(i != 0)
            nd_log(NDLS_DAEMON, NDLP_WARNING, "pthread_attr_setstacksize() to %zu bytes, failed with code %d.", stacksize, i);
        else
            nd_log(NDLS_DAEMON, NDLP_DEBUG, "Set threads stack size to %zu bytes", stacksize);
    }
    else
        nd_log(NDLS_DAEMON, NDLP_WARNING, "Invalid pthread stacksize %zu", stacksize);
}

// ----------------------------------------------------------------------------
// threads init for external plugins

void netdata_threads_init_for_external_plugins(size_t stacksize) {
    size_t default_stacksize = netdata_threads_init();
    if(default_stacksize < 1 * 1024 * 1024)
        default_stacksize = 1 * 1024 * 1024;

    netdata_threads_init_after_fork(stacksize ? stacksize : default_stacksize);
}

// ----------------------------------------------------------------------------
// netdata_thread_create

void rrdset_thread_rda_free(void);
void sender_thread_buffer_free(void);
void query_target_free(void);
void service_exits(void);
void rrd_collector_finished(void);

void netdata_thread_set_tag(const char *tag) {
    if(!tag || !*tag)
        return;

    int ret = 0;

    char threadname[NETDATA_THREAD_NAME_MAX+1];
    strncpyz(threadname, tag, NETDATA_THREAD_NAME_MAX);

#if defined(__FreeBSD__)
    pthread_set_name_np(pthread_self(), threadname);
#elif defined(__APPLE__)
    ret = pthread_setname_np(threadname);
#else
    ret = pthread_setname_np(pthread_self(), threadname);
#endif

    if (ret != 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING, "cannot set pthread name of %d to %s. ErrCode: %d", gettid_cached(), threadname, ret);
    else
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "set name of thread %d to %s", gettid_cached(), threadname);

    if(_nd_thread_info) {
        strncpyz(_nd_thread_info->tag, threadname, sizeof(_nd_thread_info->tag) - 1);
    }
}

void uv_thread_set_name_np(uv_thread_t ut, const char* name) {
    int ret = 0;

    char threadname[NETDATA_THREAD_NAME_MAX+1];
    strncpyz(threadname, name, NETDATA_THREAD_NAME_MAX);

#if defined(__FreeBSD__)
    pthread_set_name_np(ut ? ut : pthread_self(), threadname);
#elif defined(__APPLE__)
    // Apple can only set its own name
    UNUSED(ut);
#else
    ret = pthread_setname_np(ut ? ut : pthread_self(), threadname);
#endif

    thread_name_get(true);

    if (ret)
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "cannot set libuv thread name to %s. Err: %d", threadname, ret);
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

static void join_exited_detached_threads(void) {
    while(1) {
        ND_THREAD *nti;

        spinlock_lock(&threads_globals.exited.spinlock);

        nti = threads_globals.exited.list;
        while (nti && nd_thread_status_check(nti, NETDATA_THREAD_OPTION_JOINABLE) == 0)
            nti = nti->next;

        if(nti)
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(threads_globals.exited.list, nti, prev, next);

        spinlock_unlock(&threads_globals.exited.spinlock);

        if(nti) {
            nd_log(NDLS_DAEMON, NDLP_INFO, "Joining detached thread '%s', tid %d", nti->tag, nti->tid);
            nd_thread_join(nti->thread);
        }
        else
            break;
    }
}

static void thread_cleanup(void *ptr) {
    ND_THREAD *nti = _nd_thread_info;

    if(nti != ptr) {
        ND_THREAD *info = (ND_THREAD *)ptr;
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "THREADS: internal error - thread local variable does not match the one passed to this function. "
               "Expected thread '%s', passed thread '%s'",
               nti->tag, info->tag);
    }

    if(nd_thread_status_check(nti, NETDATA_THREAD_OPTION_DONT_LOG_CLEANUP) != NETDATA_THREAD_OPTION_DONT_LOG_CLEANUP)
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "thread with task id %d finished", nti->tid);

    rrd_collector_finished();
    sender_thread_buffer_free();
    rrdset_thread_rda_free();
    query_target_free();
    thread_cache_destroy();
    service_exits();
    worker_unregister();

    nd_thread_status_set(nti, NETDATA_THREAD_STATUS_FINISHED);

    spinlock_lock(&threads_globals.running.spinlock);
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(threads_globals.running.list, nti, prev, next);
    spinlock_unlock(&threads_globals.running.spinlock);

    if (nd_thread_status_check(nti, NETDATA_THREAD_OPTION_JOINABLE) != NETDATA_THREAD_OPTION_JOINABLE) {
        spinlock_lock(&threads_globals.exited.spinlock);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(threads_globals.exited.list, nti, prev, next);
        spinlock_unlock(&threads_globals.exited.spinlock);
    }
}

static void *netdata_thread_starting_point(void *ptr) {
    ND_THREAD *nti = _nd_thread_info = (ND_THREAD *)ptr;
    nd_thread_status_set(nti, NETDATA_THREAD_STATUS_STARTED);

    nti->thread = pthread_self();
    nti->tid = gettid_cached();
    netdata_thread_set_tag(nti->tag);

    if(nd_thread_status_check(nti, NETDATA_THREAD_OPTION_DONT_LOG_STARTUP) != NETDATA_THREAD_OPTION_DONT_LOG_STARTUP)
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "thread created with task id %d", gettid_cached());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING, "cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING, "cannot set pthread cancel state to ENABLE.");

    pthread_cleanup_push(thread_cleanup, ptr) {
        // run the thread code
        nti->ret = nti->start_routine(nti->arg);
    }
    pthread_cleanup_pop(1);

    return nti;
}

int netdata_thread_create(netdata_thread_t *thread, const char *tag, NETDATA_THREAD_OPTIONS options, void *(*start_routine) (void *), void *arg) {
    join_exited_detached_threads();

    ND_THREAD *nti = callocz(1, sizeof(*nti));
    nti->arg = arg;
    nti->start_routine = start_routine;
    nti->options = options & NETDATA_THREAD_OPTIONS_ALL;
    strncpyz(nti->tag, tag, NETDATA_THREAD_NAME_MAX);

    spinlock_lock(&threads_globals.running.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(threads_globals.running.list, nti, prev, next);
    spinlock_unlock(&threads_globals.running.spinlock);

    int ret = pthread_create(thread, threads_globals.attr, netdata_thread_starting_point, nti);
    if(ret != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "failed to create new thread for %s. pthread_create() failed with code %d",
               tag, ret);

        spinlock_lock(&threads_globals.running.spinlock);
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(threads_globals.running.list, nti, prev, next);
        spinlock_unlock(&threads_globals.running.spinlock);
        freez(nti);
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
        nd_log(NDLS_DAEMON, NDLP_WARNING, "cannot cancel thread. pthread_cancel() failed with code %d at %d@%s, function %s()", ret, line, file, function);
#else
        nd_log(NDLS_DAEMON, NDLP_WARNING, "cannot cancel thread. pthread_cancel() failed with code %d.", ret);
#endif

    return ret;
}

// ----------------------------------------------------------------------------
// nd_thread_join

void nd_thread_join(netdata_thread_t thread) {
    void *ptr = NULL; // will receive NETDATA_THREAD * here
    int ret = pthread_join(thread, &ptr);
    if(ret != 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING, "cannot join thread. pthread_join() failed with code %d.", ret);
    else if(ptr) {
        ND_THREAD *nti = (ND_THREAD *)ptr;
        nd_thread_status_set(nti, NETDATA_THREAD_STATUS_JOINED);

        nd_log(NDLS_DAEMON, NDLP_WARNING, "joined thread '%s', tid %d", nti->tag, nti->tid);

        spinlock_lock(&threads_globals.exited.spinlock);
        if(nti->prev)
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(threads_globals.exited.list, nti, prev, next);
        spinlock_unlock(&threads_globals.exited.spinlock);

        freez(nti);
    }
    else {
        internal_fatal(true, "pthread_join() returned NULL ptr.");
        nd_log(NDLS_DAEMON, NDLP_WARNING, "pthread_join() returned NULL ptr.");
    }
}
