// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#ifdef NETDATA_TRACE_RWLOCKS

#ifndef NETDATA_TRACE_RWLOCKS_WAIT_TIME_TO_IGNORE_USEC
#define NETDATA_TRACE_RWLOCKS_WAIT_TIME_TO_IGNORE_USEC 10
#endif

#ifndef NETDATA_TRACE_RWLOCKS_HOLD_TIME_TO_IGNORE_USEC
#define NETDATA_TRACE_RWLOCKS_HOLD_TIME_TO_IGNORE_USEC 10000
#endif

#ifndef NETDATA_THREAD_LOCKS_ARRAY_SIZE
#define NETDATA_THREAD_LOCKS_ARRAY_SIZE 10
#endif

#endif // NETDATA_TRACE_RWLOCKS

// ----------------------------------------------------------------------------
// automatic thread cancelability management, based on locks

static __thread int netdata_thread_first_cancelability = 0;
static __thread int netdata_thread_nested_disables = 0;

static __thread size_t netdata_locks_acquired_rwlocks = 0;
static __thread size_t netdata_locks_acquired_mutexes = 0;

inline void netdata_thread_disable_cancelability(void) {
    int old;
    int ret = pthread_setcancelstate(PTHREAD_CANCEL_DISABLE, &old);
    if(ret != 0)
        error("THREAD_CANCELABILITY: pthread_setcancelstate() on thread %s returned error %d", netdata_thread_tag(), ret);
    else {
        if(!netdata_thread_nested_disables)
            netdata_thread_first_cancelability = old;

        netdata_thread_nested_disables++;
    }
}

inline void netdata_thread_enable_cancelability(void) {
    if(netdata_thread_nested_disables < 1) {
        error("THREAD_CANCELABILITY: netdata_thread_enable_cancelability(): invalid thread cancelability count %d on thread %s - results will be undefined - please report this!",
            netdata_thread_nested_disables, netdata_thread_tag());
    }
    else if(netdata_thread_nested_disables == 1) {
        int old = 1;
        int ret = pthread_setcancelstate(netdata_thread_first_cancelability, &old);
        if(ret != 0)
            error("THREAD_CANCELABILITY: pthread_setcancelstate() on thread %s returned error %d", netdata_thread_tag(), ret);
        else {
            if(old != PTHREAD_CANCEL_DISABLE)
                error("THREAD_CANCELABILITY: netdata_thread_enable_cancelability(): old thread cancelability on thread %s was changed, expected DISABLED (%d), found %s (%d) - please report this!", netdata_thread_tag(), PTHREAD_CANCEL_DISABLE, (old == PTHREAD_CANCEL_ENABLE)?"ENABLED":"UNKNOWN", old);
        }

        netdata_thread_nested_disables = 0;
    }
    else
        netdata_thread_nested_disables--;
}

// ----------------------------------------------------------------------------
// mutex

int __netdata_mutex_init(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_init(mutex, NULL);
    if(unlikely(ret != 0))
        error("MUTEX_LOCK: failed to initialize (code %d).", ret);
    return ret;
}

int __netdata_mutex_destroy(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_destroy(mutex);
    if(unlikely(ret != 0))
        error("MUTEX_LOCK: failed to destroy (code %d).", ret);
    return ret;
}

int __netdata_mutex_lock(netdata_mutex_t *mutex) {
    netdata_thread_disable_cancelability();

    int ret = pthread_mutex_lock(mutex);
    if(unlikely(ret != 0)) {
        netdata_thread_enable_cancelability();
        error("MUTEX_LOCK: failed to get lock (code %d)", ret);
    }
    else
        netdata_locks_acquired_mutexes++;

    return ret;
}

int __netdata_mutex_trylock(netdata_mutex_t *mutex) {
    netdata_thread_disable_cancelability();

    int ret = pthread_mutex_trylock(mutex);
    if(ret != 0)
        netdata_thread_enable_cancelability();
    else
        netdata_locks_acquired_mutexes++;

    return ret;
}

int __netdata_mutex_unlock(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_unlock(mutex);
    if(unlikely(ret != 0))
        error("MUTEX_LOCK: failed to unlock (code %d).", ret);
    else {
        netdata_locks_acquired_mutexes--;
        netdata_thread_enable_cancelability();
    }

    return ret;
}

#ifdef NETDATA_TRACE_RWLOCKS

int netdata_mutex_init_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                             const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_init(%p) from %lu@%s, %s()", mutex, line, file, function);

    int ret = __netdata_mutex_init(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_init(%p) = %d, from %lu@%s, %s()", mutex, ret, line, file, function);

    return ret;
}

int netdata_mutex_destroy_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                             const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_destroy(%p) from %lu@%s, %s()", mutex, line, file, function);

    int ret = __netdata_mutex_destroy(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_destroy(%p) = %d, from %lu@%s, %s()", mutex, ret, line, file, function);

    return ret;
}

int netdata_mutex_lock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                             const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_lock(%p) from %lu@%s, %s()", mutex, line, file, function);

    usec_t start_s = now_monotonic_high_precision_usec();
    int ret = __netdata_mutex_lock(mutex);
    usec_t end_s = now_monotonic_high_precision_usec();

    // remove compiler unused variables warning
    (void)start_s;
    (void)end_s;

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_lock(%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, end_s - start_s, line, file, function);

    return ret;
}

int netdata_mutex_trylock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_trylock(%p) from %lu@%s, %s()", mutex, line, file, function);

    usec_t start_s = now_monotonic_high_precision_usec();
    int ret = __netdata_mutex_trylock(mutex);
    usec_t end_s = now_monotonic_high_precision_usec();

    // remove compiler unused variables warning
    (void)start_s;
    (void)end_s;

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_trylock(%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, end_s - start_s, line, file, function);

    return ret;
}

int netdata_mutex_unlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                               const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_unlock(%p) from %lu@%s, %s()", mutex, line, file, function);

    usec_t start_s = now_monotonic_high_precision_usec();
    int ret = __netdata_mutex_unlock(mutex);
    usec_t end_s = now_monotonic_high_precision_usec();

    // remove compiler unused variables warning
    (void)start_s;
    (void)end_s;

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_unlock(%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, end_s - start_s, line, file, function);

    return ret;
}

#endif // NETDATA_TRACE_RWLOCKS

// ----------------------------------------------------------------------------
// rwlock

int __netdata_rwlock_destroy(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_destroy(&rwlock->rwlock_t);
    if(unlikely(ret != 0))
        error("RW_LOCK: failed to destroy lock (code %d)", ret);
    return ret;
}

int __netdata_rwlock_init(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_init(&rwlock->rwlock_t, NULL);
    if(unlikely(ret != 0))
        error("RW_LOCK: failed to initialize lock (code %d)", ret);
    return ret;
}

int __netdata_rwlock_rdlock(netdata_rwlock_t *rwlock) {
    netdata_thread_disable_cancelability();

    int ret = pthread_rwlock_rdlock(&rwlock->rwlock_t);
    if(unlikely(ret != 0)) {
        netdata_thread_enable_cancelability();
        error("RW_LOCK: failed to obtain read lock (code %d)", ret);
    }
    else
        netdata_locks_acquired_rwlocks++;

    return ret;
}

int __netdata_rwlock_wrlock(netdata_rwlock_t *rwlock) {
    netdata_thread_disable_cancelability();

    int ret = pthread_rwlock_wrlock(&rwlock->rwlock_t);
    if(unlikely(ret != 0)) {
        error("RW_LOCK: failed to obtain write lock (code %d)", ret);
        netdata_thread_enable_cancelability();
    }
    else
        netdata_locks_acquired_rwlocks++;

    return ret;
}

int __netdata_rwlock_unlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_unlock(&rwlock->rwlock_t);
    if(unlikely(ret != 0))
        error("RW_LOCK: failed to release lock (code %d)", ret);
    else {
        netdata_thread_enable_cancelability();
        netdata_locks_acquired_rwlocks--;
    }

    return ret;
}

int __netdata_rwlock_tryrdlock(netdata_rwlock_t *rwlock) {
    netdata_thread_disable_cancelability();

    int ret = pthread_rwlock_tryrdlock(&rwlock->rwlock_t);
    if(ret != 0)
        netdata_thread_enable_cancelability();
    else
        netdata_locks_acquired_rwlocks++;

    return ret;
}

int __netdata_rwlock_trywrlock(netdata_rwlock_t *rwlock) {
    netdata_thread_disable_cancelability();

    int ret = pthread_rwlock_trywrlock(&rwlock->rwlock_t);
    if(ret != 0)
        netdata_thread_enable_cancelability();
    else
        netdata_locks_acquired_rwlocks++;

    return ret;
}

// ----------------------------------------------------------------------------
// spinlock implementation
// https://www.youtube.com/watch?v=rmGJc9PXpuE&t=41s

void netdata_spinlock_init(SPINLOCK *spinlock) {
    memset(spinlock, 0, sizeof(SPINLOCK));
}

void netdata_spinlock_lock(SPINLOCK *spinlock) {
    static const struct timespec ns = { .tv_sec = 0, .tv_nsec = 1 };

#ifdef NETDATA_INTERNAL_CHECKS
    size_t spins = 0;
#endif

    netdata_thread_disable_cancelability();

    for(int i = 1;
        __atomic_load_n(&spinlock->locked, __ATOMIC_RELAXED) ||
        __atomic_test_and_set(&spinlock->locked, __ATOMIC_ACQUIRE)
        ; i++
        ) {

#ifdef NETDATA_INTERNAL_CHECKS
        spins++;
#endif
        if(unlikely(i == 8)) {
            i = 0;
            nanosleep(&ns, NULL);
        }
    }

    // we have the lock

#ifdef NETDATA_INTERNAL_CHECKS
    spinlock->spins += spins;
    spinlock->locker_pid = gettid();
#endif
}

void netdata_spinlock_unlock(SPINLOCK *spinlock) {
#ifdef NETDATA_INTERNAL_CHECKS
    spinlock->locker_pid = 0;
#endif
    __atomic_clear(&spinlock->locked, __ATOMIC_RELEASE);
    netdata_thread_enable_cancelability();
}

bool netdata_spinlock_trylock(SPINLOCK *spinlock) {
    netdata_thread_disable_cancelability();

    if(!__atomic_load_n(&spinlock->locked, __ATOMIC_RELAXED) &&
        !__atomic_test_and_set(&spinlock->locked, __ATOMIC_ACQUIRE))
        // we got the lock
        return true;

    // we didn't get the lock
    return false;
}

#ifdef NETDATA_TRACE_RWLOCKS

// ----------------------------------------------------------------------------
// lockers list

static netdata_rwlock_locker *find_rwlock_locker(const char *file __maybe_unused, const char *function __maybe_unused, const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    pid_t pid = gettid();
    netdata_rwlock_locker *locker = NULL;

    __netdata_mutex_lock(&rwlock->lockers_mutex);
    Pvoid_t *PValue = JudyLGet(rwlock->lockers_pid_JudyL, pid, PJE0);
    if(PValue && *PValue)
        locker = *PValue;
    __netdata_mutex_unlock(&rwlock->lockers_mutex);

    return locker;
}

static netdata_rwlock_locker *add_rwlock_locker(const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock, LOCKER_REQUEST lock_type) {
    netdata_rwlock_locker *locker;

    locker = find_rwlock_locker(file, function, line, rwlock);
    if(locker) {
        locker->lock |= lock_type;
        locker->refcount++;
    }
    else {
        locker = mallocz(sizeof(netdata_rwlock_locker));
        locker->pid = gettid();
        locker->tag = netdata_thread_tag();
        locker->refcount = 1;
        locker->lock = lock_type;
        locker->got_it = false;
        locker->file = file;
        locker->function = function;
        locker->line = line;

        __netdata_mutex_lock(&rwlock->lockers_mutex);
        DOUBLE_LINKED_LIST_APPEND_UNSAFE(rwlock->lockers, locker, prev, next);
        Pvoid_t *PValue = JudyLIns(&rwlock->lockers_pid_JudyL, locker->pid, PJE0);
        *PValue = locker;
        if (lock_type == RWLOCK_REQUEST_READ || lock_type == RWLOCK_REQUEST_TRYREAD) rwlock->readers++;
        if (lock_type == RWLOCK_REQUEST_WRITE || lock_type == RWLOCK_REQUEST_TRYWRITE) rwlock->writers++;
        __netdata_mutex_unlock(&rwlock->lockers_mutex);
    }

    return locker;
}

static void remove_rwlock_locker(const char *file __maybe_unused, const char *function __maybe_unused, const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock, netdata_rwlock_locker *locker) {
    __netdata_mutex_lock(&rwlock->lockers_mutex);
    locker->refcount--;
    if(!locker->refcount) {
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rwlock->lockers, locker, prev, next);
        JudyLDel(&rwlock->lockers_pid_JudyL, locker->pid, PJE0);
        if (locker->lock == RWLOCK_REQUEST_READ || locker->lock == RWLOCK_REQUEST_TRYREAD) rwlock->readers--;
        else if (locker->lock == RWLOCK_REQUEST_WRITE || locker->lock == RWLOCK_REQUEST_TRYWRITE) rwlock->writers--;
        freez(locker);
    }
    __netdata_mutex_unlock(&rwlock->lockers_mutex);
}

// ----------------------------------------------------------------------------
// debug versions of rwlock

int netdata_rwlock_destroy_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                 const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {

    int ret = __netdata_rwlock_destroy(rwlock);
    if(!ret) {
        while (rwlock->lockers)
            remove_rwlock_locker(file, function, line, rwlock, rwlock->lockers);
    }

    return ret;
}

int netdata_rwlock_init_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                              const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {

    int ret = __netdata_rwlock_init(rwlock);
    if(!ret) {
        __netdata_mutex_init(&rwlock->lockers_mutex);
        rwlock->lockers_pid_JudyL = NULL;
        rwlock->lockers = NULL;
        rwlock->readers = 0;
        rwlock->writers = 0;
    }

    return ret;
}

int netdata_rwlock_rdlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {

    netdata_rwlock_locker *locker = add_rwlock_locker(file, function, line, rwlock, RWLOCK_REQUEST_READ);

    int ret = __netdata_rwlock_rdlock(rwlock);
    if(!ret)
        locker->got_it = true;
    else
        remove_rwlock_locker(file, function, line, rwlock, locker);

    return ret;
}

int netdata_rwlock_wrlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {

    netdata_rwlock_locker *locker = add_rwlock_locker(file, function, line, rwlock, RWLOCK_REQUEST_WRITE);

    int ret = __netdata_rwlock_wrlock(rwlock);
    if(!ret)
        locker->got_it = true;
    else
        remove_rwlock_locker(file, function, line, rwlock, locker);

    return ret;
}

int netdata_rwlock_unlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);

    if(unlikely(!locker))
        fatal("UNLOCK WITHOUT LOCK");

    int ret = __netdata_rwlock_unlock(rwlock);
    if(likely(!ret))
        remove_rwlock_locker(file, function, line, rwlock, locker);

    return ret;
}

int netdata_rwlock_tryrdlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                   const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {

    netdata_rwlock_locker *locker = add_rwlock_locker(file, function, line, rwlock, RWLOCK_REQUEST_TRYREAD);

    int ret = __netdata_rwlock_tryrdlock(rwlock);
    if(!ret)
        locker->got_it = true;
    else
        remove_rwlock_locker(file, function, line, rwlock, locker);

    return ret;
}

int netdata_rwlock_trywrlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                   const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {

    netdata_rwlock_locker *locker = add_rwlock_locker(file, function, line, rwlock, RWLOCK_REQUEST_TRYWRITE);

    int ret = __netdata_rwlock_trywrlock(rwlock);
    if(!ret)
        locker->got_it = true;
    else
        remove_rwlock_locker(file, function, line, rwlock, locker);

    return ret;
}

#endif // NETDATA_TRACE_RWLOCKS
