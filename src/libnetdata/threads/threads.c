// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#define nd_thread_status_get(nti)           __atomic_load_n(&((nti)->options), __ATOMIC_ACQUIRE)
#define nd_thread_status_check(nti, flag)   (__atomic_load_n(&((nti)->options), __ATOMIC_ACQUIRE) & (flag))
#define nd_thread_status_set(nti, flag)     __atomic_or_fetch(&((nti)->options), flag, __ATOMIC_RELEASE)
#define nd_thread_status_clear(nti, flag)   __atomic_and_fetch(&((nti)->options), ~(flag), __ATOMIC_RELEASE)

typedef void (*nd_thread_canceller)(void *data);

typedef enum __attribute__((packed)) {
    ND_THREAD_LIST_NONE = 0,
    ND_THREAD_LIST_RUNNING,
    ND_THREAD_LIST_EXITED,
} ND_THREAD_LIST;

struct nd_thread {
    void *arg;
    pid_t tid;
    char tag[ND_THREAD_TAG_MAX + 1];
    //void *ret; // the return value of start routine
    void (*start_routine) (void *);
    NETDATA_THREAD_OPTIONS options;
    uv_thread_t thread;
    bool cancel_atomic;

#ifdef NETDATA_INTERNAL_CHECKS
    // keep track of the locks currently held
    // used to detect locks that are left locked during exit
    int rwlocks_read_locks;
    int rwlocks_write_locks;
    int mutex_locks;
    int spinlock_locks;
    int rwspinlock_read_locks;
    int rwspinlock_write_locks;
#endif

    struct {
        SPINLOCK spinlock;
        nd_thread_canceller cb;
        void *data;
    } canceller;

    ND_THREAD_LIST list;
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

    pthread_attr_t attr;
} threads_globals = {
    .exited = {
        .spinlock = SPINLOCK_INITIALIZER,
        .list = NULL,
    },
    .running = {
        .spinlock = SPINLOCK_INITIALIZER,
        .list = NULL,
    },
};

static __thread ND_THREAD *_nd_thread_info = NULL;
static __thread char _nd_thread_os_name[ND_THREAD_TAG_MAX + 1] = "";
static __thread bool _nd_thread_can_run_sql = true;

void nd_thread_can_run_sql(bool run_sql) {
    _nd_thread_can_run_sql = run_sql;
}

bool nd_thread_runs_sql(void) {
    return _nd_thread_can_run_sql;
}


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
static inline void os_set_thread_name(const char *name) {
#if defined(__FreeBSD__)
    pthread_set_name_np(pthread_self(), name);
#elif defined(__APPLE__)
    pthread_setname_np(name);
#else
    pthread_setname_np(pthread_self(), name);
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

const char *nd_thread_tag_async_safe(void) {
    if(nd_thread_has_tag())
        return _nd_thread_info->tag;

    return _nd_thread_os_name;
}

void nd_thread_tag_set(const char *tag) {
    if(!tag || !*tag) return;

    if(_nd_thread_info)
        strncpyz(_nd_thread_info->tag, tag, sizeof(_nd_thread_info->tag) - 1);

    strncpyz(_nd_thread_os_name, tag, sizeof(_nd_thread_os_name) - 1);

    os_set_thread_name(_nd_thread_os_name);
}

// --------------------------------------------------------------------------------------------------------------------

void uv_thread_set_name_np(const char *name) {
    static __thread bool libuv_name_set = false;

    if(!name || !*name || (libuv_name_set && _nd_thread_os_name[0]))
        return;

    strncpyz(_nd_thread_os_name, name, sizeof(_nd_thread_os_name) - 1);
    os_set_thread_name(_nd_thread_os_name);
    libuv_name_set = true;
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
        os_set_thread_name(name);
    }

    nd_thread_get_name(true);
}

// --------------------------------------------------------------------------------------------------------------------
// locks tracking

#ifdef NETDATA_INTERNAL_CHECKS
void nd_thread_rwlock_read_locked(void) { if(_nd_thread_info) _nd_thread_info->rwlocks_read_locks++; }
void nd_thread_rwlock_read_unlocked(void) { if(_nd_thread_info) _nd_thread_info->rwlocks_read_locks--; }
void nd_thread_rwlock_write_locked(void) { if(_nd_thread_info) _nd_thread_info->rwlocks_write_locks++; }
void nd_thread_rwlock_write_unlocked(void) { if(_nd_thread_info) _nd_thread_info->rwlocks_write_locks--; }
void nd_thread_mutex_locked(void) { if(_nd_thread_info) _nd_thread_info->mutex_locks++; }
void nd_thread_mutex_unlocked(void) { if(_nd_thread_info) _nd_thread_info->mutex_locks--; }
void nd_thread_spinlock_locked(void) { if(_nd_thread_info) _nd_thread_info->spinlock_locks++; }
void nd_thread_spinlock_unlocked(void) { if(_nd_thread_info) _nd_thread_info->spinlock_locks--; }
void nd_thread_rwspinlock_read_locked(void) { if(_nd_thread_info) _nd_thread_info->rwspinlock_read_locks++; }
void nd_thread_rwspinlock_read_unlocked(void) { if(_nd_thread_info) _nd_thread_info->rwspinlock_read_locks--; }
void nd_thread_rwspinlock_write_locked(void) { if(_nd_thread_info) _nd_thread_info->rwspinlock_write_locks++; }
void nd_thread_rwspinlock_write_unlocked(void) { if(_nd_thread_info) _nd_thread_info->rwspinlock_write_locks--; }
#endif

// --------------------------------------------------------------------------------------------------------------------
// early initialization

size_t netdata_threads_init(void) {
    memset(&threads_globals.attr, 0, sizeof(threads_globals.attr));

    if(pthread_attr_init(&threads_globals.attr) != 0)
        fatal("pthread_attr_init() failed.");

    // get the required stack size of the threads of netdata
    size_t stacksize = 0;
    if(pthread_attr_getstacksize(&threads_globals.attr, &stacksize) != 0)
        fatal("pthread_attr_getstacksize() failed with code.");

    return stacksize;
}

// ----------------------------------------------------------------------------
// late initialization

void netdata_threads_set_stack_size(size_t stacksize) {
    int i;

    // set pthread stack size
    if(stacksize > (size_t)PTHREAD_STACK_MIN) {
        i = pthread_attr_setstacksize(&threads_globals.attr, stacksize);
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

    netdata_threads_set_stack_size(stacksize ? stacksize : default_stacksize);
}

// ----------------------------------------------------------------------------

void rrdset_thread_rda_free(void);
void sender_thread_buffer_free(void);
void query_target_free(void);
void service_exits(void);
void rrd_collector_finished(void);

void nd_thread_join_threads()
{
    ND_THREAD *nti;
    do {
        spinlock_lock(&threads_globals.exited.spinlock);

        nti = threads_globals.exited.list;

        if (nti) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(threads_globals.exited.list, nti, prev, next);
            nti->list = ND_THREAD_LIST_NONE;
            nd_log_daemon(NDLP_DEBUG, "nd_thread_join_threads: Joining thread with id %d (%s) during shutdown", nti->tid, nti->tag);
        }

        spinlock_unlock(&threads_globals.exited.spinlock);

        // handles null
        nd_thread_join(nti);

    } while (nti);
}

static void nd_thread_exit(ND_THREAD *nti) {

    if(nti != _nd_thread_info || !nti || !_nd_thread_info) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "THREADS: internal error - thread local variable does not match the one passed to this function. "
               "Expected thread '%s', passed thread '%s'",
               _nd_thread_info ? _nd_thread_info->tag : "(null)", nti ? nti->tag : "(null)");

        if(!nti) nti = _nd_thread_info;
    }

    if(!nti) return;

    internal_fatal(nti->rwlocks_read_locks != 0,
        "THREAD '%s' WITH PID %d HAS %d RWLOCKS READ ACQUIRED WHILE EXITING !!!",
        (nti) ? nti->tag : "(unset)", gettid_cached(), nti->rwlocks_read_locks);

    internal_fatal(nti->rwlocks_write_locks != 0,
        "THREAD '%s' WITH PID %d HAS %d RWLOCKS WRITE ACQUIRED WHILE EXITING !!!",
        (nti) ? nti->tag : "(unset)", gettid_cached(), nti->rwlocks_write_locks);

    internal_fatal(nti->mutex_locks != 0,
        "THREAD '%s' WITH PID %d HAS %d MUTEXES ACQUIRED WHILE EXITING !!!",
        (nti) ? nti->tag : "(unset)", gettid_cached(), nti->mutex_locks);

    internal_fatal(nti->spinlock_locks != 0,
        "THREAD '%s' WITH PID %d HAS %d SPINLOCKS ACQUIRED WHILE EXITING !!!",
        (nti) ? nti->tag : "(unset)", gettid_cached(), nti->spinlock_locks);

    internal_fatal(nti->rwspinlock_read_locks != 0,
        "THREAD '%s' WITH PID %d HAS %d RWSPINLOCKS READ ACQUIRED WHILE EXITING !!!",
        (nti) ? nti->tag : "(unset)", gettid_cached(), nti->rwspinlock_read_locks);

    internal_fatal(nti->rwspinlock_write_locks != 0,
        "THREAD '%s' WITH PID %d HAS %d RWSPINLOCKS WRITE ACQUIRED WHILE EXITING !!!",
        (nti) ? nti->tag : "(unset)", gettid_cached(), nti->rwspinlock_write_locks);

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
    if(nti->list == ND_THREAD_LIST_RUNNING) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(threads_globals.running.list, nti, prev, next);
        nti->list = ND_THREAD_LIST_NONE;
    }
    spinlock_unlock(&threads_globals.running.spinlock);

    spinlock_lock(&threads_globals.exited.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(threads_globals.exited.list, nti, prev, next);
    nti->list = ND_THREAD_LIST_EXITED;
    spinlock_unlock(&threads_globals.exited.spinlock);
}

static void nd_thread_starting_point(void *ptr) {
    ND_THREAD *nti = _nd_thread_info = (ND_THREAD *)ptr;
    nd_thread_status_set(nti, NETDATA_THREAD_STATUS_STARTED);

    nti->tid = gettid_cached();
    nd_thread_tag_set(nti->tag);

    if(nd_thread_status_check(nti, NETDATA_THREAD_OPTION_DONT_LOG_STARTUP) != NETDATA_THREAD_OPTION_DONT_LOG_STARTUP)
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "thread created with task id %d", gettid_cached());

    signals_block_all_except_deadly();

    spinlock_lock(&threads_globals.running.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(threads_globals.running.list, nti, prev, next);
    nti->list = ND_THREAD_LIST_RUNNING;
    spinlock_unlock(&threads_globals.running.spinlock);

    // run the thread code
    nti->start_routine(nti->arg);

    nd_thread_exit(nti);
}

ND_THREAD *nd_thread_self(void) {
    return _nd_thread_info;
}

bool nd_thread_is_me(ND_THREAD *nti) {
    return nti && nti->thread == pthread_self();
}


// utils
#define MAX_THREAD_CREATE_RETRIES (10)
#define MAX_THREAD_CREATE_WAIT_MS (1000)

static int create_uv_thread(uv_thread_t *thread, uv_thread_cb thread_func, void *arg, int *retries)
{
    int err;

    do {
        err = uv_thread_create(thread, thread_func, arg);
        if (err == 0)
            break;

        sleep_usec(MAX_THREAD_CREATE_WAIT_MS * USEC_PER_MS);
    } while (err == UV_EAGAIN && ++(*retries) < MAX_THREAD_CREATE_RETRIES);

    return err;
}

ND_THREAD *nd_thread_create(const char *tag, NETDATA_THREAD_OPTIONS options, void (*start_routine)(void *), void *arg)
{
    ND_THREAD *nti = callocz(1, sizeof(*nti));
    spinlock_init(&nti->canceller.spinlock);
    nti->arg = arg;
    nti->start_routine = start_routine;
    nti->options = (options & NETDATA_THREAD_OPTIONS_ALL);
    strncpyz(nti->tag, tag, ND_THREAD_TAG_MAX);

    int retries = 0;
    int ret = create_uv_thread(&nti->thread, nd_thread_starting_point, nti, &retries);
    if(ret != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "failed to create new thread for %s. uv_thread_create() failed with code %d",
               tag, ret);

        freez(nti);
        return NULL;
    }
    if (retries)
        nd_log_daemon(NDLP_WARNING, "nd_thread_create required %d attempts", retries);

    return nti;
}

// --------------------------------------------------------------------------------------------------------------------

void nd_thread_register_canceller(nd_thread_canceller cb, void *data) {
    ND_THREAD *nti = _nd_thread_info;
    if(!nti) return;

    spinlock_lock(&nti->canceller.spinlock);
    nti->canceller.cb = cb;
    nti->canceller.data = data;
    spinlock_unlock(&nti->canceller.spinlock);
}

void nd_thread_signal_cancel(ND_THREAD *nti) {
    if(!nti) return;

    __atomic_store_n(&nti->cancel_atomic, true, __ATOMIC_RELAXED);

    spinlock_lock(&nti->canceller.spinlock);
    if(nti->canceller.cb)
        nti->canceller.cb(nti->canceller.data);
    spinlock_unlock(&nti->canceller.spinlock);
}

ALWAYS_INLINE
bool nd_thread_signaled_to_cancel(void) {
    if(!_nd_thread_info) return false;
    return __atomic_load_n(&_nd_thread_info->cancel_atomic, __ATOMIC_RELAXED);
}

// ----------------------------------------------------------------------------
// nd_thread_join

int nd_thread_join(ND_THREAD *nti) {
    if(!nti)
        return ESRCH;

    if(nd_thread_status_check(nti, NETDATA_THREAD_STATUS_JOINED))
        return 0;

    int ret;
    if((ret = uv_thread_join(&nti->thread))) {
        // we can't join the thread

        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "cannot join thread. uv_thread_join() failed with code %d. (tag=%s)",
               ret, nti->tag);
    }
    else {
        // we successfully joined the thread
       nd_thread_status_set(nti, NETDATA_THREAD_STATUS_JOINED);

        spinlock_lock(&threads_globals.running.spinlock);
        if(nti->list == ND_THREAD_LIST_RUNNING) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(threads_globals.running.list, nti, prev, next);
            nti->list = ND_THREAD_LIST_NONE;
        }
        spinlock_unlock(&threads_globals.running.spinlock);

        spinlock_lock(&threads_globals.exited.spinlock);
        if(nti->list == ND_THREAD_LIST_EXITED) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(threads_globals.exited.list, nti, prev, next);
            nti->list = ND_THREAD_LIST_NONE;
        }
        spinlock_unlock(&threads_globals.exited.spinlock);

        freez(nti);
    }

    return ret;
}
