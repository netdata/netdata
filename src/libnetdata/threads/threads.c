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

    size_t stack_size;
} threads_globals = {
    .exited = {
        .spinlock = SPINLOCK_INITIALIZER,
        .list = NULL,
    },
    .running = {
        .spinlock = SPINLOCK_INITIALIZER,
        .list = NULL,
    },
    .stack_size = ND_THREAD_STACK_SIZE_DEFAULT,
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
// stack size of the threads created by nd_thread_create()

void nd_thread_set_stack_size(size_t stack_size) {
    if(stack_size < ND_THREAD_STACK_SIZE_MIN) {
        errno_clear();
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "THREADS: stack size %zu bytes is below the minimum, using %zu bytes",
               stack_size, (size_t)ND_THREAD_STACK_SIZE_MIN);
        stack_size = ND_THREAD_STACK_SIZE_MIN;
    }

    __atomic_store_n(&threads_globals.stack_size, stack_size, __ATOMIC_RELAXED);
}

size_t nd_thread_stack_size(void) {
    return __atomic_load_n(&threads_globals.stack_size, __ATOMIC_RELAXED);
}

// ----------------------------------------------------------------------------
// Thread-cleanup callback registry.
//
// Registration is startup-only — callers register before any daemon
// thread is created. libnetdata calls every registered callback at
// thread-exit time, in registration order. The startup-only invariant
// lets nd_thread_run_cleanup_callbacks read the count without holding
// the spinlock; the __ATOMIC_RELEASE/_ACQUIRE pair handles any late
// edge.
//
// Ordering: all registered callbacks run before thread_cache_destroy
// and worker_unregister (called below). A callback that needs to
// observe thread_cache state must do so before that destroy.

#define ND_THREAD_CLEANUP_MAX 16
static struct {
    SPINLOCK spinlock;
    size_t count;
    nd_thread_cleanup_fn fns[ND_THREAD_CLEANUP_MAX];
} nd_thread_cleanup_registry = { .spinlock = SPINLOCK_INITIALIZER };

void nd_thread_register_cleanup(nd_thread_cleanup_fn fn) {
    if(!fn) return;

    spinlock_lock(&nd_thread_cleanup_registry.spinlock);

    // Idempotent: skip if this fn is already registered. Keeps the
    // registry small under repeated rrd_init / unittest re-entry.
    for(size_t i = 0; i < nd_thread_cleanup_registry.count; i++) {
        if(nd_thread_cleanup_registry.fns[i] == fn) {
            spinlock_unlock(&nd_thread_cleanup_registry.spinlock);
            return;
        }
    }

    if(nd_thread_cleanup_registry.count >= ND_THREAD_CLEANUP_MAX) {
        spinlock_unlock(&nd_thread_cleanup_registry.spinlock);
        fatal("nd_thread_register_cleanup: registry full (max %d)", ND_THREAD_CLEANUP_MAX);
    }
    nd_thread_cleanup_registry.fns[nd_thread_cleanup_registry.count] = fn;
    __atomic_store_n(&nd_thread_cleanup_registry.count,
                     nd_thread_cleanup_registry.count + 1,
                     __ATOMIC_RELEASE);
    spinlock_unlock(&nd_thread_cleanup_registry.spinlock);
}

static void nd_thread_run_cleanup_callbacks(void) {
    size_t count = __atomic_load_n(&nd_thread_cleanup_registry.count, __ATOMIC_ACQUIRE);
    for(size_t i = 0; i < count; i++) {
        nd_thread_cleanup_fn fn = nd_thread_cleanup_registry.fns[i];
        if(fn) fn();
    }
}

// ----------------------------------------------------------------------------

static int nd_thread_join_internal(ND_THREAD *nti, bool *wrapper_released, bool already_claimed);

static bool nd_thread_try_claim_join(ND_THREAD *nti)
{
    NETDATA_THREAD_OPTIONS old_options;
    do {
        old_options = __atomic_load_n(&nti->options, __ATOMIC_ACQUIRE);
        if(old_options & NETDATA_THREAD_STATUS_JOINED)
            return false;
    } while(!__atomic_compare_exchange_n(&nti->options, &old_options,
                                          old_options | NETDATA_THREAD_STATUS_JOINED,
                                          false, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE));

    return true;
}

void nd_thread_join_threads()
{
    ND_THREAD *retained = NULL;

    ND_THREAD *nti;
    do {
        spinlock_lock(&threads_globals.exited.spinlock);
        nti = threads_globals.exited.list;
        while(nti) {
            // Manual-join wrappers are owned by their creator. Do not claim JOINED before
            // checking this flag: a direct owner join would otherwise see a false success.
            // The flag is set at creation and never changes, so this check is final.
            if (nd_thread_status_check(nti, NETDATA_THREAD_OPTION_MANUAL_JOIN)) {
                nti = nti->next;
                continue;
            }
            if (nd_thread_try_claim_join(nti))
                break;
            nti = nti->next;
        }
        if(nti) {
            // Claim the wrapper before unlinking it. A direct owner may have the same pointer
            // and can free it immediately after winning the JOINED CAS.
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(threads_globals.exited.list, nti, prev, next);
            nti->list = ND_THREAD_LIST_NONE;
        }
        spinlock_unlock(&threads_globals.exited.spinlock);

        // nd_thread_join_internal() handles NULL and will skip list removal since we already did it
        bool wrapper_released = true;
        nd_thread_join_internal(nti, &wrapper_released, true);

        // a failed join keeps the wrapper alive; we already unlinked it from the
        // exited list, so keep it aside (the loop keeps draining) and restore it
        // below where a later cleanup pass can find it
        if(nti && !wrapper_released)
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(retained, nti, prev, next);

    } while (nti);

    if(retained) {
        spinlock_lock(&threads_globals.exited.spinlock);
        while(retained) {
            ND_THREAD *t = retained;
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(retained, t, prev, next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(threads_globals.exited.list, t, prev, next);
            t->list = ND_THREAD_LIST_EXITED;

            // it is discoverable again - release our CAS ownership so a
            // later join can retry it
            nd_thread_status_clear(t, NETDATA_THREAD_STATUS_JOINED);
        }
        spinlock_unlock(&threads_globals.exited.spinlock);
    }
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

    nd_thread_run_cleanup_callbacks();
    thread_cache_destroy();
    worker_unregister();

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

    nd_thread_status_set(nti, NETDATA_THREAD_STATUS_FINISHED);
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

#if !defined(OS_WINDOWS)
    // on Windows libuv gives the stack size to _beginthreadex() as the initial commit,
    // not as a reservation, so there threads keep the default stack of the executable
    uv_thread_options_t options = {
        .flags = UV_THREAD_HAS_STACK_SIZE,
        .stack_size = nd_thread_stack_size(),
    };
#endif

    do {
#if defined(OS_WINDOWS)
        err = uv_thread_create(thread, thread_func, arg);
#else
        err = uv_thread_create_ex(thread, &options, thread_func, arg);
#endif
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

// returns the join result; sets *wrapper_released to false when the ND_THREAD
// wrapper is intentionally kept alive (failed join with the worker not proven
// finished), so nd_thread_join_threads() can make it discoverable again after
// having unlinked it from the exited list
static int nd_thread_join_internal(ND_THREAD *nti, bool *wrapper_released, bool already_claimed) {
    *wrapper_released = true;

    if(!nti)
        return ESRCH;

    // Atomically check and set JOINED flag to prevent race conditions where two threads both try
    // to join the same thread. The exited-list drain may already have made this claim under its
    // list lock, before unlinking the wrapper.
    if(!already_claimed && !nd_thread_try_claim_join(nti))
        return 0;

    int ret;

    if((ret = uv_thread_join(&nti->thread))) {
        // we can't join the thread

        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "cannot join thread. uv_thread_join() failed with code %d. (tag=%s)",
               ret, nti->tag);

        // On Windows/MSYS2, if the thread exited very quickly, uv_thread_join() can fail with UV_EINVAL
        // because the thread handle becomes invalid before the join executes. However, the thread may
        // still be finishing its cleanup. Wait for it to reach FINISHED state before cleaning up.
        if(ret == UV_EINVAL) {
            nd_log(NDLS_DAEMON, NDLP_INFO,
                   "thread '%s' join returned EINVAL, waiting for thread to finish...", nti->tag);

            // Spin-wait for the thread to mark itself as finished
            size_t retries = 0;
            while(!nd_thread_status_check(nti, NETDATA_THREAD_STATUS_FINISHED) && retries < 1000) {
                sleep_usec(1 * USEC_PER_MS); // 1ms
                retries++;
            }

            if (nd_thread_status_check(nti, NETDATA_THREAD_STATUS_FINISHED)) {
                nd_log(NDLS_DAEMON, NDLP_INFO, "thread '%s' confirmed finished, cleaning up structure", nti->tag);
                ret = 0;
            } else {
                nd_log(NDLS_DAEMON, NDLP_ERR, "thread '%s' did not reach FINISHED state after 1 second", nti->tag);
            }
        }
    }

    // Only release the wrapper once the OS join succeeded, or Netdata's thread
    // exit path has finished and can no longer use the thread-local pointer.
    if(ret != 0 && !nd_thread_status_check(nti, NETDATA_THREAD_STATUS_FINISHED)) {
        // JOINED stays set: the wrapper remains CAS-owned by this caller, so
        // no other joiner can free it while the caller decides how to make it
        // joinable again (see nd_thread_join() and nd_thread_join_threads())
        *wrapper_released = false;
        return ret;
    }

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

    return ret;
}

int nd_thread_join(ND_THREAD *nti) {
    bool wrapper_released;
    int ret = nd_thread_join_internal(nti, &wrapper_released, false);

    // release our CAS ownership of the retained wrapper, so the caller
    // (which still holds the pointer) can retry the join
    if(nti && !wrapper_released)
        nd_thread_status_clear(nti, NETDATA_THREAD_STATUS_JOINED);

    return ret;
}

// ----------------------------------------------------------------------------
// unittest

static void threads_unittest_worker(void *arg __maybe_unused) {
    ;
}

// takes the wrapper's address as an integer, so it can be asked about a wrapper that has been freed
static bool threads_unittest_is_exited(uintptr_t nti) {
    bool found = false;

    spinlock_lock(&threads_globals.exited.spinlock);
    for(ND_THREAD *t = threads_globals.exited.list; t; t = t->next) {
        if((uintptr_t)t == nti) {
            found = true;
            break;
        }
    }
    spinlock_unlock(&threads_globals.exited.spinlock);

    return found;
}

static bool threads_unittest_wait_finished(ND_THREAD *nti) {
    // FINISHED is set after the wrapper is appended to the exited list
    for(size_t i = 0; i < 10000; i++) {
        if(nd_thread_status_check(nti, NETDATA_THREAD_STATUS_FINISHED))
            return true;
        sleep_usec(1 * USEC_PER_MS);
    }
    return false;
}

int threads_unittest(void) {
    int errors = 0;

    fprintf(stderr, "\nTesting that nd_thread_join_threads() leaves manual-join threads to their owner...\n");

    ND_THREAD *manual = nd_thread_create("UT_MANUAL", NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_MANUAL_JOIN,
                                         threads_unittest_worker, NULL);
    ND_THREAD *generic = nd_thread_create("UT_GENERIC", NETDATA_THREAD_OPTION_DONT_LOG,
                                          threads_unittest_worker, NULL);
    if(!manual || !generic) {
        fprintf(stderr, " > FAILED: cannot create the test threads\n");
        nd_thread_join(manual);
        nd_thread_join(generic);
        return 1;
    }

    if(!threads_unittest_wait_finished(manual) || !threads_unittest_wait_finished(generic)) {
        // do not join: a thread that did not finish would block the test forever
        fprintf(stderr, " > FAILED: the test threads did not finish\n");
        return 1;
    }

    uintptr_t manual_id = (uintptr_t)manual;
    uintptr_t generic_id = (uintptr_t)generic;

    nd_thread_join_threads();

    if(!threads_unittest_is_exited(manual_id)) {
        fprintf(stderr, " > FAILED: the drain took a manual-join thread from its owner\n");
        // the drain freed it; joining it here would be a use-after-free
        return 1;
    }

    if(threads_unittest_is_exited(generic_id)) {
        fprintf(stderr, " > FAILED: the drain did not join a default thread\n");
        errors++;
    }

    int rc = nd_thread_join(manual);
    if(rc != 0) {
        fprintf(stderr, " > FAILED: the owner's join returned %d\n", rc);
        errors++;
    }

    if(threads_unittest_is_exited(manual_id)) {
        fprintf(stderr, " > FAILED: the owner's join left the thread on the exited list\n");
        errors++;
    }

    fprintf(stderr, "%s\n", errors ? " > FAILED" : " > OK");
    return errors;
}

// --------------------------------------------------------------------------------------------------------------------
// unittest

#if defined(OS_LINUX)
static void nd_thread_stack_size_unittest_thread(void *arg) {
    size_t *stack_size = arg;
    pthread_attr_t attr;
    if(pthread_getattr_np(pthread_self(), &attr) == 0) {
        if(pthread_attr_getstacksize(&attr, stack_size) != 0)
            *stack_size = 0;
        pthread_attr_destroy(&attr);
    }
}
#endif

int nd_thread_stack_size_unittest(void) {
    int errors = 0;

    fprintf(stderr, "\nrunning thread stack size unittest\n");

#if defined(OS_LINUX)
    size_t saved = nd_thread_stack_size();
    const size_t wanted[] = { ND_THREAD_STACK_SIZE_MIN, 3 * 1024 * 1024, 4096 };

    for(size_t i = 0; i < sizeof(wanted) / sizeof(wanted[0]); i++) {
        nd_thread_set_stack_size(wanted[i]);
        size_t expected = MAX(wanted[i], (size_t)ND_THREAD_STACK_SIZE_MIN);

        size_t got = 0;
        ND_THREAD *t = nd_thread_create("STACKTEST", NETDATA_THREAD_OPTION_DEFAULT, nd_thread_stack_size_unittest_thread, &got);
        if(!t || nd_thread_join(t) != 0) {
            fprintf(stderr, "thread stack size unittest FAILED: cannot run a thread for %zu bytes\n", wanted[i]);
            errors++;
            continue;
        }

        // musl adds the TLS area to the stack it reports, and glibc may give
        // a thread the cached stack of an exited one, up to 4 times bigger
#ifdef __GLIBC__
        size_t max_got = 4 * expected + 64 * 1024;
#else
        size_t max_got = expected + 64 * 1024;
#endif
        if(nd_thread_stack_size() != expected || got < expected || got > max_got) {
            fprintf(stderr, "thread stack size unittest FAILED: asked %zu bytes, configured %zu, thread got %zu, expected %zu\n",
                    wanted[i], nd_thread_stack_size(), got, expected);
            errors++;
        }
        else
            fprintf(stderr, "thread stack size unittest: asked %zu bytes, thread got %zu\n", wanted[i], got);
    }

    nd_thread_set_stack_size(saved);
    fprintf(stderr, "thread stack size unittest %s\n", errors ? "FAILED" : "OK");
#else
    fprintf(stderr, "thread stack size unittest skipped: it needs pthread_getattr_np()\n");
#endif

    return errors;
}
