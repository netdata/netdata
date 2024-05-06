// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#define nd_thread_status_get(nti)           __atomic_load_n(&((nti)->options), __ATOMIC_ACQUIRE)
#define nd_thread_status_check(nti, flag)   (__atomic_load_n(&((nti)->options), __ATOMIC_ACQUIRE) & (flag))
#define nd_thread_status_set(nti, flag)     __atomic_or_fetch(&((nti)->options), flag, __ATOMIC_RELEASE)
#define nd_thread_status_clear(nti, flag)   __atomic_and_fetch(&((nti)->options), ~(flag), __ATOMIC_RELEASE)

struct nd_thread {
    void *arg;
    pid_t tid;
    char tag[ND_THREAD_TAG_MAX + 1];
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

static __thread ND_THREAD *_nd_thread_info = NULL;
static __thread char _nd_thread_os_name[ND_THREAD_TAG_MAX + 1] = "";

// --------------------------------------------------------------------------------------------------------------------
// O/S abstraction

// get the thread name from the operating system
static inline void os_get_thread_name(char *out, size_t size) {
#if defined(__FreeBSD__)
    pthread_get_name_np(pthread_self(), out, size);
    if(strcmp(_nd_thread_os_name, "netdata") == 0)
        strncpyz(out, "MAIN", size - 1);
#elif defined(HAVE_PTHREAD_GETNAME_NP)
    pthread_getname_np(pthread_self(), out, size - 1);
    if(strcmp(out, "netdata") == 0)
        strncpyz(out, "MAIN", size - 1);
#else
    strncpyz(out, "MAIN", size - 1);
#endif
}

// set the thread name to the operating system
static inline void os_set_thread_name_of_self(const char *name) {
#if defined(__FreeBSD__)
    pthread_set_name_np(pthread_self(), name);
#elif defined(__APPLE__)
    pthread_setname_np(name);
#else
    pthread_setname_np(pthread_self(), name);
#endif
}

static inline void os_set_pthread_name(pthread_t thread, const char *name) {
#if defined(__FreeBSD__)
    pthread_set_name_np(thread, name);
#elif defined(__APPLE__)
    // Apple can only set the name from the same thread
    UNUSED(ut);
#else
    pthread_setname_np(thread, name);
#endif
}

// --------------------------------------------------------------------------------------------------------------------
// internal API for managing names

inline int nd_thread_has_tag(void) {
    return (_nd_thread_info && _nd_thread_info->tag[0]);
}

// For threads created by netdata, return the tag of the thread.
// For threads created by others (libuv, webrtc, etc), return the tag of the operating system.
// This caches the response, so that it won't query the operating system multiple times.
static inline const char *nd_thread_get_name(bool recheck) {
    if(nd_thread_has_tag())
        return _nd_thread_info->tag;

    if(!recheck && _nd_thread_os_name[0])
        return _nd_thread_os_name;

    os_get_thread_name(_nd_thread_os_name, sizeof(_nd_thread_os_name));

    return _nd_thread_os_name;
}

const char *nd_thread_tag(void) {
    return nd_thread_get_name(false);
}

void nd_thread_tag_set(const char *tag) {
    if(!tag || !*tag) return;

    if(_nd_thread_info)
        strncpyz(_nd_thread_info->tag, tag, sizeof(_nd_thread_info->tag) - 1);

    strncpyz(_nd_thread_os_name, tag, sizeof(_nd_thread_os_name) - 1);

    os_set_thread_name_of_self(_nd_thread_os_name);
}

// --------------------------------------------------------------------------------------------------------------------

static __thread bool libuv_name_set = false;
void uv_thread_set_name_np(uv_thread_t ut, const char* name) {
    if(!ut && libuv_name_set) return;

    strncpyz(_nd_thread_os_name, name, sizeof(_nd_thread_os_name) - 1);

    if(!ut) {
        os_set_thread_name_of_self(_nd_thread_os_name);
        libuv_name_set = true;
    }
    else
        os_set_pthread_name(ut, _nd_thread_os_name);
}

// --------------------------------------------------------------------------------------------------------------------

static size_t webrtc_id = 0;
static __thread bool webrtc_name_set = false;
void webrtc_set_thread_name(void) {
    if(_nd_thread_info || webrtc_name_set) return;

    webrtc_name_set = true;

    char tmp[ND_THREAD_TAG_MAX + 1] = "";
    os_get_thread_name(tmp, sizeof(tmp));

    if(!tmp[0] || strcmp(tmp, "netdata") == 0) {
        char name[ND_THREAD_TAG_MAX + 1];
        snprintfz(name, ND_THREAD_TAG_MAX, "WEBRTC[%zu]", __atomic_fetch_add(&webrtc_id, 1, __ATOMIC_RELAXED));
        os_set_thread_name_of_self(name);
    }

    nd_thread_get_name(true);
}

// --------------------------------------------------------------------------------------------------------------------
// early initialization

size_t netdata_threads_init(void) {
    int i;

    if(!threads_globals.attr) {
        threads_globals.attr = callocz(1, sizeof(pthread_attr_t));
        i = pthread_attr_init(threads_globals.attr);
        if (i != 0)
            fatal("pthread_attr_init() failed with code %d.", i);
    }

    // get the required stack size of the threads of netdata
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

void rrdset_thread_rda_free(void);
void sender_thread_buffer_free(void);
void query_target_free(void);
void service_exits(void);
void rrd_collector_finished(void);

static void nd_thread_join_exited_detached_threads(void) {
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
            nd_thread_join(nti);
        }
        else
            break;
    }
}

static void nd_thread_exit(void *ptr) {
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

static void *nd_thread_starting_point(void *ptr) {
    ND_THREAD *nti = _nd_thread_info = (ND_THREAD *)ptr;
    nd_thread_status_set(nti, NETDATA_THREAD_STATUS_STARTED);

    nti->tid = gettid_cached();
    nd_thread_tag_set(nti->tag);

    if(nd_thread_status_check(nti, NETDATA_THREAD_OPTION_DONT_LOG_STARTUP) != NETDATA_THREAD_OPTION_DONT_LOG_STARTUP)
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "thread created with task id %d", gettid_cached());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING, "cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING, "cannot set pthread cancel state to ENABLE.");

    pthread_cleanup_push(nd_thread_exit, ptr) {
        // run the thread code
        nti->ret = nti->start_routine(nti->arg);
    }
    pthread_cleanup_pop(1);

    return nti;
}

ND_THREAD *nd_thread_self(void) {
    return _nd_thread_info;
}

bool nd_thread_is_me(ND_THREAD *nti) {
    return nti && nti->thread == pthread_self();
}

ND_THREAD *nd_thread_create(const char *tag, NETDATA_THREAD_OPTIONS options, void *(*start_routine)(void *), void *arg) {
    nd_thread_join_exited_detached_threads();

    ND_THREAD *nti = callocz(1, sizeof(*nti));
    nti->arg = arg;
    nti->start_routine = start_routine;
    nti->options = options & NETDATA_THREAD_OPTIONS_ALL;
    strncpyz(nti->tag, tag, ND_THREAD_TAG_MAX);

    spinlock_lock(&threads_globals.running.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(threads_globals.running.list, nti, prev, next);
    spinlock_unlock(&threads_globals.running.spinlock);

    int ret = pthread_create(&nti->thread, threads_globals.attr, nd_thread_starting_point, nti);
    if(ret != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "failed to create new thread for %s. pthread_create() failed with code %d",
               tag, ret);

        spinlock_lock(&threads_globals.running.spinlock);
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(threads_globals.running.list, nti, prev, next);
        spinlock_unlock(&threads_globals.running.spinlock);
        freez(nti);
        return NULL;
    }

    return nti;
}

// ----------------------------------------------------------------------------

#ifdef NETDATA_INTERNAL_CHECKS
int nd_thread_cancel_with_trace(ND_THREAD *nti, int line, const char *file, const char *function) {
#else
int nd_thread_cancel(ND_THREAD * nti) {
#endif
    int ret = pthread_cancel(nti->thread);
    if(ret != 0)
#ifdef NETDATA_INTERNAL_CHECKS
        nd_log(NDLS_DAEMON, NDLP_WARNING, "cannot cancel thread. pthread_cancel() failed with code %d at %d@%s, function %s()", ret, line, file, function);
#else
        nd_log(NDLS_DAEMON, NDLP_WARNING, "cannot cancel thread. pthread_cancel() failed with code %d.", ret);
#endif

    return ret;
}

void nd_thread_testcancel(void) {
    // work only on our own threads
    if(!_nd_thread_info) return;

    pthread_testcancel();
}

// ----------------------------------------------------------------------------
// nd_thread_join

void nd_thread_join(ND_THREAD *nti) {
    if(!nti) return;

    int ret = pthread_join(nti->thread, NULL);
    if(ret != 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING, "cannot join thread. pthread_join() failed with code %d.", ret);
    else {
        nd_thread_status_set(nti, NETDATA_THREAD_STATUS_JOINED);

        nd_log(NDLS_DAEMON, NDLP_WARNING, "joined thread '%s', tid %d", nti->tag, nti->tid);

        spinlock_lock(&threads_globals.exited.spinlock);
        if(nti->prev)
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(threads_globals.exited.list, nti, prev, next);
        spinlock_unlock(&threads_globals.exited.spinlock);

        freez(nti);
    }
}
