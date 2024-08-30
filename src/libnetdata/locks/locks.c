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

int __netdata_mutex_init(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_init(mutex, NULL);
    if(unlikely(ret != 0))
        netdata_log_error("MUTEX_LOCK: failed to initialize (code %d).", ret);
    return ret;
}

int __netdata_mutex_destroy(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_destroy(mutex);
    if(unlikely(ret != 0))
        netdata_log_error("MUTEX_LOCK: failed to destroy (code %d).", ret);
    return ret;
}

int __netdata_mutex_lock(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_lock(mutex);
    if(unlikely(ret != 0)) {
        netdata_log_error("MUTEX_LOCK: failed to get lock (code %d)", ret);
    }
    else
        nd_thread_mutex_locked();

    return ret;
}

int __netdata_mutex_trylock(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_trylock(mutex);
    if(ret != 0)
        ;
    else
        nd_thread_mutex_locked();

    return ret;
}

int __netdata_mutex_unlock(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_unlock(mutex);
    if(unlikely(ret != 0))
        netdata_log_error("MUTEX_LOCK: failed to unlock (code %d).", ret);
    else
        nd_thread_mutex_unlocked();

    return ret;
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

int __netdata_rwlock_destroy(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_destroy(&rwlock->rwlock_t);
    if(unlikely(ret != 0))
        netdata_log_error("RW_LOCK: failed to destroy lock (code %d)", ret);
    return ret;
}

int __netdata_rwlock_init(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_init(&rwlock->rwlock_t, NULL);
    if(unlikely(ret != 0))
        netdata_log_error("RW_LOCK: failed to initialize lock (code %d)", ret);
    return ret;
}

int __netdata_rwlock_rdlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_rdlock(&rwlock->rwlock_t);
    if(unlikely(ret != 0))
        netdata_log_error("RW_LOCK: failed to obtain read lock (code %d)", ret);
    else
        nd_thread_rwlock_read_locked();

    return ret;
}

int __netdata_rwlock_wrlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_wrlock(&rwlock->rwlock_t);
    if(unlikely(ret != 0))
        netdata_log_error("RW_LOCK: failed to obtain write lock (code %d)", ret);
    else
        nd_thread_rwlock_write_locked();

    return ret;
}

int __netdata_rwlock_rdunlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_unlock(&rwlock->rwlock_t);
    if(unlikely(ret != 0))
        netdata_log_error("RW_LOCK: failed to release lock (code %d)", ret);
    else
        nd_thread_rwlock_read_unlocked();

    return ret;
}

int __netdata_rwlock_wrunlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_unlock(&rwlock->rwlock_t);
    if(unlikely(ret != 0))
        netdata_log_error("RW_LOCK: failed to release lock (code %d)", ret);
    else
        nd_thread_rwlock_write_unlocked();

    return ret;
}

int __netdata_rwlock_tryrdlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_tryrdlock(&rwlock->rwlock_t);
    if(ret != 0)
        ;
    else
        nd_thread_rwlock_read_locked();

    return ret;
}

int __netdata_rwlock_trywrlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_trywrlock(&rwlock->rwlock_t);
    if(ret != 0)
        ;
    else
        nd_thread_rwlock_write_locked();

    return ret;
}

// ----------------------------------------------------------------------------
// spinlock implementation
// https://www.youtube.com/watch?v=rmGJc9PXpuE&t=41s

#ifdef SPINLOCK_IMPL_WITH_MUTEX
void spinlock_init(SPINLOCK *spinlock)
{
    netdata_mutex_init(&spinlock->inner);
}
#else
void spinlock_init(SPINLOCK *spinlock)
{
    memset(spinlock, 0, sizeof(SPINLOCK));
}
#endif

#ifndef SPINLOCK_IMPL_WITH_MUTEX
static inline void spinlock_lock_internal(SPINLOCK *spinlock)
{
    #ifdef NETDATA_INTERNAL_CHECKS
    size_t spins = 0;
    #endif

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
            tinysleep();
        }
    }

    // we have the lock

    #ifdef NETDATA_INTERNAL_CHECKS
    spinlock->spins += spins;
    spinlock->locker_pid = gettid_cached();
    #endif

    nd_thread_spinlock_locked();
}
#endif // SPINLOCK_IMPL_WITH_MUTEX

#ifndef SPINLOCK_IMPL_WITH_MUTEX
static inline void spinlock_unlock_internal(SPINLOCK *spinlock)
{
    #ifdef NETDATA_INTERNAL_CHECKS
    spinlock->locker_pid = 0;
    #endif

    __atomic_clear(&spinlock->locked, __ATOMIC_RELEASE);

    nd_thread_spinlock_unlocked();
}
#endif // SPINLOCK_IMPL_WITH_MUTEX

#ifndef SPINLOCK_IMPL_WITH_MUTEX
static inline bool spinlock_trylock_internal(SPINLOCK *spinlock) {
    if(!__atomic_load_n(&spinlock->locked, __ATOMIC_RELAXED) &&
        !__atomic_test_and_set(&spinlock->locked, __ATOMIC_ACQUIRE)) {
        // we got the lock
        nd_thread_spinlock_locked();
        return true;
    }

    return false;
}
#endif // SPINLOCK_IMPL_WITH_MUTEX

#ifdef SPINLOCK_IMPL_WITH_MUTEX
void spinlock_lock(SPINLOCK *spinlock)
{
    netdata_mutex_lock(&spinlock->inner);
}
#else
void spinlock_lock(SPINLOCK *spinlock)
{
    spinlock_lock_internal(spinlock);
}
#endif

#ifdef SPINLOCK_IMPL_WITH_MUTEX
void spinlock_unlock(SPINLOCK *spinlock)
{
    netdata_mutex_unlock(&spinlock->inner);
}
#else
void spinlock_unlock(SPINLOCK *spinlock)
{
    spinlock_unlock_internal(spinlock);
}
#endif

#ifdef SPINLOCK_IMPL_WITH_MUTEX
bool spinlock_trylock(SPINLOCK *spinlock)
{
    return netdata_mutex_trylock(&spinlock->inner) == 0;
}
#else
bool spinlock_trylock(SPINLOCK *spinlock)
{
    return spinlock_trylock_internal(spinlock);
}
#endif

#ifdef SPINLOCK_IMPL_WITH_MUTEX
void spinlock_lock_cancelable(SPINLOCK *spinlock)
{
    netdata_mutex_lock(&spinlock->inner);
}
#else
void spinlock_lock_cancelable(SPINLOCK *spinlock)
{
    spinlock_lock_internal(spinlock);
}
#endif

#ifdef SPINLOCK_IMPL_WITH_MUTEX
void spinlock_unlock_cancelable(SPINLOCK *spinlock)
{
    netdata_mutex_unlock(&spinlock->inner);
}
#else
void spinlock_unlock_cancelable(SPINLOCK *spinlock)
{
    spinlock_unlock_internal(spinlock);
}
#endif

#ifdef SPINLOCK_IMPL_WITH_MUTEX
bool spinlock_trylock_cancelable(SPINLOCK *spinlock)
{
    return netdata_mutex_trylock(&spinlock->inner) == 0;
}
#else
bool spinlock_trylock_cancelable(SPINLOCK *spinlock)
{
    return spinlock_trylock_internal(spinlock);
}
#endif

// ----------------------------------------------------------------------------
// rw_spinlock implementation

void rw_spinlock_init(RW_SPINLOCK *rw_spinlock) {
    rw_spinlock->readers = 0;
    spinlock_init(&rw_spinlock->spinlock);
}

void rw_spinlock_read_lock(RW_SPINLOCK *rw_spinlock) {
    spinlock_lock(&rw_spinlock->spinlock);
    __atomic_add_fetch(&rw_spinlock->readers, 1, __ATOMIC_RELAXED);
    spinlock_unlock(&rw_spinlock->spinlock);

    nd_thread_rwspinlock_read_locked();
}

void rw_spinlock_read_unlock(RW_SPINLOCK *rw_spinlock) {
#ifndef NETDATA_INTERNAL_CHECKS
    __atomic_sub_fetch(&rw_spinlock->readers, 1, __ATOMIC_RELAXED);
#else
    int32_t x = __atomic_sub_fetch(&rw_spinlock->readers, 1, __ATOMIC_RELAXED);
    if(x < 0)
        fatal("RW_SPINLOCK: readers is negative %d", x);
#endif

    nd_thread_rwspinlock_read_unlocked();
}

void rw_spinlock_write_lock(RW_SPINLOCK *rw_spinlock) {
    size_t spins = 0;
    while(1) {
        spins++;
        spinlock_lock(&rw_spinlock->spinlock);

        if(__atomic_load_n(&rw_spinlock->readers, __ATOMIC_RELAXED) == 0)
            break;

        // Busy wait until all readers have released their locks.
        spinlock_unlock(&rw_spinlock->spinlock);
        tinysleep();
    }

    (void)spins;

    nd_thread_rwspinlock_write_locked();
}

void rw_spinlock_write_unlock(RW_SPINLOCK *rw_spinlock) {
    spinlock_unlock(&rw_spinlock->spinlock);
    nd_thread_rwspinlock_write_unlocked();
}

bool rw_spinlock_tryread_lock(RW_SPINLOCK *rw_spinlock) {
    if(spinlock_trylock(&rw_spinlock->spinlock)) {
        __atomic_add_fetch(&rw_spinlock->readers, 1, __ATOMIC_RELAXED);
        spinlock_unlock(&rw_spinlock->spinlock);
        nd_thread_rwspinlock_read_locked();
        return true;
    }

    return false;
}

bool rw_spinlock_trywrite_lock(RW_SPINLOCK *rw_spinlock) {
    if(spinlock_trylock(&rw_spinlock->spinlock)) {
        if (__atomic_load_n(&rw_spinlock->readers, __ATOMIC_RELAXED) == 0) {
            // No readers, we've successfully acquired the write lock
            nd_thread_rwspinlock_write_locked();
            return true;
        }
        else {
            // There are readers, unlock the spinlock and return false
            spinlock_unlock(&rw_spinlock->spinlock);
        }
    }

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
