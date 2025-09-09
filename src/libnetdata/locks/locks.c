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
// mutex

ALWAYS_INLINE int __netdata_cond_init(netdata_cond_t *cond) {
    int ret = uv_cond_init(cond);
    if(unlikely(ret != 0))
        netdata_log_error("COND: failed to initialize (code %d).", ret);
    return ret;
}

ALWAYS_INLINE void __netdata_cond_destroy(netdata_cond_t *cond) {
    uv_cond_destroy(cond);
}

ALWAYS_INLINE void __netdata_cond_signal(netdata_cond_t *cond) {
    uv_cond_signal(cond);
}

ALWAYS_INLINE void __netdata_cond_broadcast(netdata_cond_t *cond) {
    uv_cond_broadcast(cond);
}

ALWAYS_INLINE void __netdata_cond_wait(netdata_cond_t *cond, netdata_mutex_t *mutex)
{
    uv_cond_wait(cond, mutex);
}

ALWAYS_INLINE int __netdata_cond_timedwait(netdata_cond_t *cond, netdata_mutex_t *mutex, uint64_t timeout_ns)
{
    int ret = uv_cond_timedwait(cond, mutex, timeout_ns);
    return ret;
}

ALWAYS_INLINE int __netdata_mutex_init(netdata_mutex_t *mutex) {
    int ret = uv_mutex_init(mutex);
    if(unlikely(ret != 0))
        netdata_log_error("MUTEX_LOCK: failed to initialize (code %d).", ret);
    return ret;
}

ALWAYS_INLINE void __netdata_mutex_destroy(netdata_mutex_t *mutex) {
    uv_mutex_destroy(mutex);
}

ALWAYS_INLINE void __netdata_mutex_lock(netdata_mutex_t *mutex) {
    uv_mutex_lock(mutex);
    nd_thread_mutex_locked();
}

ALWAYS_INLINE int __netdata_mutex_trylock(netdata_mutex_t *mutex) {
    int ret = uv_mutex_trylock(mutex);
    if(ret != 0)
        ;
    else
        nd_thread_mutex_locked();

    return ret;
}

ALWAYS_INLINE void __netdata_mutex_unlock(netdata_mutex_t *mutex) {
    uv_mutex_unlock(mutex);
    nd_thread_mutex_unlocked();
}

#ifdef NETDATA_TRACE_RWLOCKS

int netdata_mutex_init_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                             const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    netdata_log_debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_init(%p) from %lu@%s, %s()", mutex, line, file, function);

    int ret = __netdata_mutex_init(mutex);

   netdata_log_debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_init(%p) = %d, from %lu@%s, %s()", mutex, ret, line, file, function);

    return ret;
}

int netdata_mutex_destroy_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                             const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    netdata_log_debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_destroy(%p) from %lu@%s, %s()", mutex, line, file, function);

    int ret = __netdata_mutex_destroy(mutex);

    netdata_log_debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_destroy(%p) = %d, from %lu@%s, %s()", mutex, ret, line, file, function);

    return ret;
}

int netdata_mutex_lock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                             const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    netdata_log_debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_lock(%p) from %lu@%s, %s()", mutex, line, file, function);

    usec_t start_s = now_monotonic_high_precision_usec();
    int ret = __netdata_mutex_lock(mutex);
    usec_t end_s = now_monotonic_high_precision_usec();

    // remove compiler unused variables warning
    (void)start_s;
    (void)end_s;

    netdata_log_debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_lock(%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, end_s - start_s, line, file, function);

    return ret;
}

int netdata_mutex_trylock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    netdata_log_debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_trylock(%p) from %lu@%s, %s()", mutex, line, file, function);

    usec_t start_s = now_monotonic_high_precision_usec();
    int ret = __netdata_mutex_trylock(mutex);
    usec_t end_s = now_monotonic_high_precision_usec();

    // remove compiler unused variables warning
    (void)start_s;
    (void)end_s;

    netdata_log_debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_trylock(%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, end_s - start_s, line, file, function);

    return ret;
}

int netdata_mutex_unlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                               const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    netdata_log_debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_unlock(%p) from %lu@%s, %s()", mutex, line, file, function);

    usec_t start_s = now_monotonic_high_precision_usec();
    int ret = __netdata_mutex_unlock(mutex);
    usec_t end_s = now_monotonic_high_precision_usec();

    // remove compiler unused variables warning
    (void)start_s;
    (void)end_s;

    netdata_log_debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_unlock(%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, end_s - start_s, line, file, function);

    return ret;
}

#endif // NETDATA_TRACE_RWLOCKS

// ----------------------------------------------------------------------------
// rwlock

ALWAYS_INLINE void __netdata_rwlock_destroy(netdata_rwlock_t *rwlock) {
    uv_rwlock_destroy(&rwlock->rwlock_t);
}

ALWAYS_INLINE int __netdata_rwlock_init(netdata_rwlock_t *rwlock) {
    int ret = uv_rwlock_init(&rwlock->rwlock_t);
    if(unlikely(ret != 0))
        netdata_log_error("RW_LOCK: failed to initialize lock (code %d)", ret);
    return ret;
}

ALWAYS_INLINE void __netdata_rwlock_rdlock(netdata_rwlock_t *rwlock) {
    uv_rwlock_rdlock(&rwlock->rwlock_t);
    nd_thread_rwlock_read_locked();
}

ALWAYS_INLINE void __netdata_rwlock_wrlock(netdata_rwlock_t *rwlock) {
    uv_rwlock_wrlock(&rwlock->rwlock_t);
    nd_thread_rwlock_write_locked();
}

ALWAYS_INLINE void __netdata_rwlock_rdunlock(netdata_rwlock_t *rwlock) {
    uv_rwlock_rdunlock(&rwlock->rwlock_t);
    nd_thread_rwlock_read_unlocked();
}

ALWAYS_INLINE void __netdata_rwlock_wrunlock(netdata_rwlock_t *rwlock) {
    uv_rwlock_wrunlock(&rwlock->rwlock_t);
    nd_thread_rwlock_write_unlocked();
}

ALWAYS_INLINE int __netdata_rwlock_tryrdlock(netdata_rwlock_t *rwlock) {
    int ret = uv_rwlock_tryrdlock(&rwlock->rwlock_t);
    if(ret != 0)
        ;
    else
        nd_thread_rwlock_read_locked();

    return ret;
}

ALWAYS_INLINE int __netdata_rwlock_trywrlock(netdata_rwlock_t *rwlock) {
    int ret = uv_rwlock_trywrlock(&rwlock->rwlock_t);
    if(ret != 0)
        ;
    else
        nd_thread_rwlock_write_locked();

    return ret;
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

int netdata_rwlock_rdunlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);

    if(unlikely(!locker))
        fatal("UNLOCK WITHOUT LOCK");

    int ret = __netdata_rwlock_rdunlock(rwlock);
    if(likely(!ret))
        remove_rwlock_locker(file, function, line, rwlock, locker);

    return ret;
}

int netdata_rwlock_wrunlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);

    if(unlikely(!locker))
        fatal("UNLOCK WITHOUT LOCK");

    int ret = __netdata_rwlock_wrunlock(rwlock);
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
