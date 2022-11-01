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
static __thread netdata_rwlock_t *netdata_thread_locks[NETDATA_THREAD_LOCKS_ARRAY_SIZE];


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

#warning NETDATA_TRACE_RWLOCKS ENABLED - EXPECT A LOT OF OUTPUT

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

#ifdef NETDATA_TRACE_RWLOCKS

// ----------------------------------------------------------------------------
// lockers list

void not_supported_by_posix_rwlocks(const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock, char locktype, const char *reason) {
    __netdata_mutex_lock(&rwlock->lockers_mutex);
    fprintf(stderr,
            "RW_LOCK FATAL ON LOCK %p: %d '%s' (function %s() %lu@%s) attempts to acquire a '%c' lock, but it is not supported by POSIX because: %s. At this attempt, the task is holding %zu rwlocks and %zu mutexes. There are %zu readers and %zu writers holding this lock:\n",
            rwlock,
            gettid(), netdata_thread_tag(),
            function, line, file,
            locktype,
            reason,
            netdata_locks_acquired_rwlocks, netdata_locks_acquired_mutexes,
            rwlock->readers, rwlock->writers);

    int i;
    usec_t now = now_monotonic_high_precision_usec();
    netdata_rwlock_locker *p;
    for(i = 1, p = rwlock->lockers; p ;p = p->next, i++) {
        fprintf(stderr,
                "     => %i: RW_LOCK %p: process %d '%s' (function %s() %lu@%s) is having %zu '%c' lock for %llu usec.\n",
                i, rwlock,
                p->pid, p->tag,
                p->function, p->line, p->file,
                p->callers, p->lock,
                (now - p->start_s));
    }
    __netdata_mutex_unlock(&rwlock->lockers_mutex);
}

static void log_rwlock_lockers(const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock, const char *reason, char locktype) {

    // this function can only be used by one thread at a time
    // because otherwise, the threads may deadlock waiting for each other
    static netdata_mutex_t log_lockers_mutex = NETDATA_MUTEX_INITIALIZER;
    __netdata_mutex_lock(&log_lockers_mutex);

    // now work on this locker
    __netdata_mutex_lock(&rwlock->lockers_mutex);
    fprintf(stderr,
            "RW_LOCK ON LOCK %p: %d '%s' (function %s() %lu@%s) %s a '%c' lock (while holding %zu rwlocks and %zu mutexes). There are %zu readers and %zu writers holding this lock:\n",
            rwlock,
            gettid(), netdata_thread_tag(),
            function, line, file,
            reason, locktype,
            netdata_locks_acquired_rwlocks, netdata_locks_acquired_mutexes,
            rwlock->readers, rwlock->writers);

    int i;
    usec_t now = now_monotonic_high_precision_usec();
    netdata_rwlock_locker *p;
    for(i = 1, p = rwlock->lockers; p ;p = p->next, i++) {
        fprintf(stderr,
                "     => %i: RW_LOCK %p: process %d '%s' (function %s() %lu@%s) is having %zu '%c' lock for %llu usec.\n",
                i, rwlock,
                p->pid, p->tag,
                p->function, p->line, p->file,
                p->callers, p->lock,
                (now - p->start_s));

        if(p->all_caller_locks) {
            // find the lock in the netdata_thread_locks[]
            // and remove it
            int k;
            for(k = 0; k < NETDATA_THREAD_LOCKS_ARRAY_SIZE ;k++) {
                if (p->all_caller_locks[k] && p->all_caller_locks[k] != rwlock) {

                    // lock the other lock lockers list
                    __netdata_mutex_lock(&p->all_caller_locks[k]->lockers_mutex);

                    // print the list of lockers of the other lock
                    netdata_rwlock_locker *r;
                    int j;
                    for(j = 1, r = p->all_caller_locks[k]->lockers; r ;r = r->next, j++) {
                        fprintf(
                            stderr,
                            "     ~~~> %i: RW_LOCK %p: process %d '%s' (function %s() %lu@%s) is having %zu '%c' lock for %llu usec.\n",
                            j,
                            p->all_caller_locks[k],
                            r->pid,
                            r->tag,
                            r->function,
                            r->line,
                            r->file,
                            r->callers,
                            r->lock,
                            (now - r->start_s));
                    }

                    // unlock the other lock lockers list
                    __netdata_mutex_unlock(&p->all_caller_locks[k]->lockers_mutex);
                }
            }
        }

    }
    __netdata_mutex_unlock(&rwlock->lockers_mutex);

    // unlock this function for other threads
    __netdata_mutex_unlock(&log_lockers_mutex);
}

static netdata_rwlock_locker *add_rwlock_locker(const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock, char lock_type) {
    netdata_rwlock_locker *p = mallocz(sizeof(netdata_rwlock_locker));
    p->pid = gettid();
    p->tag = netdata_thread_tag();
    p->lock = lock_type;
    p->file = file;
    p->function = function;
    p->line = line;
    p->callers = 1;
    p->all_caller_locks = netdata_thread_locks;
    p->start_s = now_monotonic_high_precision_usec();

    // find a slot in the netdata_thread_locks[]
    int i;
    for(i = 0; i < NETDATA_THREAD_LOCKS_ARRAY_SIZE ;i++) {
        if (!netdata_thread_locks[i]) {
            netdata_thread_locks[i] = rwlock;
            break;
        }
    }

    __netdata_mutex_lock(&rwlock->lockers_mutex);
    p->next = rwlock->lockers;
    rwlock->lockers = p;
    if(lock_type == 'R') rwlock->readers++;
    if(lock_type == 'W') rwlock->writers++;
    __netdata_mutex_unlock(&rwlock->lockers_mutex);

    return p;
}

static void remove_rwlock_locker(const char *file __maybe_unused, const char *function __maybe_unused, const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock, netdata_rwlock_locker *locker) {
    usec_t end_s = now_monotonic_high_precision_usec();

    if(locker->callers == 0)
        fprintf(stderr,
                "RW_LOCK ON LOCK %p: %d, '%s' (function %s() %lu@%s) callers should be positive but it is zero\n",
                rwlock,
                locker->pid, locker->tag,
                locker->function, locker->line, locker->file);

    if(locker->callers > 1 && locker->lock != 'R')
        fprintf(stderr,
                "RW_LOCK ON LOCK %p: %d, '%s' (function %s() %lu@%s) only 'R' locks support multiple holders, but here we have %zu callers holding a '%c' lock.\n",
                rwlock,
                locker->pid, locker->tag,
                locker->function, locker->line, locker->file,
                locker->callers, locker->lock);

    __netdata_mutex_lock(&rwlock->lockers_mutex);
    locker->callers--;

    if(!locker->callers) {
        int doit = 0;

        if (rwlock->lockers == locker) {
            rwlock->lockers = locker->next;
            doit = 1;
        } else {
            netdata_rwlock_locker *p;
            for (p = rwlock->lockers; p && p->next != locker; p = p->next)
                ;
            if (p && p->next == locker) {
                p->next = locker->next;
                doit = 1;
            }
        }
        if(doit) {
            if(locker->lock == 'R') rwlock->readers--;
            if(locker->lock == 'W') rwlock->writers--;
        }

        if(!doit) {
            fprintf(stderr,
                    "RW_LOCK ON LOCK %p: %d, '%s' (function %s() %lu@%s) with %zu x '%c' lock is not found.\n",
                    rwlock,
                    locker->pid, locker->tag,
                    locker->function, locker->line, locker->file,
                    locker->callers, locker->lock);
        }
        else {
            // find the lock in the netdata_thread_locks[]
            // and remove it
            int i;
            for(i = 0; i < NETDATA_THREAD_LOCKS_ARRAY_SIZE ;i++) {
                if (netdata_thread_locks[i] == rwlock)
                    netdata_thread_locks[i] = NULL;
            }

            if(end_s - locker->start_s >= NETDATA_TRACE_RWLOCKS_HOLD_TIME_TO_IGNORE_USEC)
                fprintf(stderr,
                        "RW_LOCK ON LOCK %p: %d, '%s' (function %s() %lu@%s) holded a '%c' for %llu usec.\n",
                        rwlock,
                        locker->pid, locker->tag,
                        locker->function, locker->line, locker->file,
                        locker->lock, end_s - locker->start_s);

            freez(locker);
        }
    }

    __netdata_mutex_unlock(&rwlock->lockers_mutex);
}

static netdata_rwlock_locker *find_rwlock_locker(const char *file __maybe_unused, const char *function __maybe_unused, const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    pid_t pid = gettid();
    netdata_rwlock_locker *p;

    __netdata_mutex_lock(&rwlock->lockers_mutex);
    for(p = rwlock->lockers; p ;p = p->next) {
        if(p->pid == pid) break;
    }
    __netdata_mutex_unlock(&rwlock->lockers_mutex);

    return p;
}

static netdata_rwlock_locker *update_or_add_rwlock_locker(const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock, netdata_rwlock_locker *locker, char locktype) {
    if(!locker) {
        return add_rwlock_locker(file, function, line, rwlock, locktype);
    }
    else if(locker->lock == 'R' && locktype == 'R') {
        __netdata_mutex_lock(&rwlock->lockers_mutex);
        locker->callers++;
        __netdata_mutex_unlock(&rwlock->lockers_mutex);
        return locker;
    }
    else {
        not_supported_by_posix_rwlocks(file, function, line, rwlock, locktype, "DEADLOCK - WANTS TO CHANGE LOCK TYPE BUT ALREADY HAS THIS LOCKED");
        return locker;
    }
}

// ----------------------------------------------------------------------------
// debug versions of rwlock

int netdata_rwlock_destroy_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                 const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_destroy(%p) from %lu@%s, %s()", rwlock, line, file, function);

    if(rwlock->readers)
        error("RW_LOCK: destroying a rwlock with %zu readers in it", rwlock->readers);
    if(rwlock->writers)
        error("RW_LOCK: destroying a rwlock with %zu writers in it", rwlock->writers);

    int ret = __netdata_rwlock_destroy(rwlock);
    if(!ret) {
        while (rwlock->lockers)
            remove_rwlock_locker(file, function, line, rwlock, rwlock->lockers);

        if (rwlock->readers)
            error("RW_LOCK: internal error - empty rwlock with %zu readers in it", rwlock->readers);
        if (rwlock->writers)
            error("RW_LOCK: internal error - empty rwlock with %zu writers in it", rwlock->writers);
    }

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_destroy(%p) = %d, from %lu@%s, %s()", rwlock, ret, line, file, function);

    return ret;
}

int netdata_rwlock_init_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                              const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_init(%p) from %lu@%s, %s()", rwlock, line, file, function);

    int ret = __netdata_rwlock_init(rwlock);
    if(!ret) {
        __netdata_mutex_init(&rwlock->lockers_mutex);
        rwlock->lockers = NULL;
        rwlock->readers = 0;
        rwlock->writers = 0;
    }

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_init(%p) = %d, from %lu@%s, %s()", rwlock, ret, line, file, function);

    return ret;
}

int netdata_rwlock_rdlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_rdlock(%p) from %lu@%s, %s()", rwlock, line, file, function);

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);

#ifdef NETDATA_TRACE_RWLOCKS_LOG_NESTED
    if(locker && locker->lock == 'R') {
        log_rwlock_lockers(file, function, line, rwlock, "NESTED READ LOCK REQUEST", 'R');
    }
#endif // NETDATA_TRACE_RWLOCKS_LOG_NESTED

    int log = 0;
    if(rwlock->writers) {
        log_rwlock_lockers(file, function, line, rwlock, "WANTS", 'R');
        log = 1;
    }

    usec_t start_s = now_monotonic_high_precision_usec();
    int ret = __netdata_rwlock_rdlock(rwlock);
    usec_t end_s = now_monotonic_high_precision_usec();

    if(!ret) {
        locker = update_or_add_rwlock_locker(file, function, line, rwlock, locker, 'R');
        if(log) log_rwlock_lockers(file, function, line, rwlock, "GOT", 'R');

    }

    if(end_s - start_s >= NETDATA_TRACE_RWLOCKS_WAIT_TIME_TO_IGNORE_USEC)
        fprintf(stderr,
                "RW_LOCK ON LOCK %p: %d, '%s' (function %s() %lu@%s) WAITED for a READ lock for %llu usec.\n",
                rwlock,
                gettid(), netdata_thread_tag(),
                function, line, file,
                end_s - start_s);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_rdlock(%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, end_s - start_s, line, file, function);

    return ret;
}

int netdata_rwlock_wrlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_wrlock(%p) from %lu@%s, %s()", rwlock, line, file, function);

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);
    if(locker)
        not_supported_by_posix_rwlocks(file, function, line, rwlock, 'W', "DEADLOCK - WANTS A WRITE LOCK BUT ALREADY HAVE THIS LOCKED");

    int log = 0;
    if(rwlock->readers) {
        log_rwlock_lockers(file, function, line, rwlock, "WANTS", 'W');
        log = 1;
    }

    usec_t start_s = now_monotonic_high_precision_usec();
    int ret = __netdata_rwlock_wrlock(rwlock);
    usec_t end_s = now_monotonic_high_precision_usec();

    if(!ret){
        locker = update_or_add_rwlock_locker(file, function, line, rwlock, locker, 'W');
        if(log) log_rwlock_lockers(file, function, line, rwlock, "GOT", 'W');
    }

    if(end_s - start_s >= NETDATA_TRACE_RWLOCKS_WAIT_TIME_TO_IGNORE_USEC)
        fprintf(stderr,
                "RW_LOCK ON LOCK %p: %d, '%s' (function %s() %lu@%s) WAITED for a WRITE lock for %llu usec.\n",
                rwlock,
                gettid(), netdata_thread_tag(),
                function, line, file,
                end_s - start_s);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_wrlock(%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, end_s - start_s, line, file, function);

    return ret;
}

int netdata_rwlock_unlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_unlock(%p) from %lu@%s, %s()", rwlock, line, file, function);

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);
    if(unlikely(!locker))
        not_supported_by_posix_rwlocks(file, function, line, rwlock, 'U', "UNLOCK WITHOUT LOCK");

    usec_t start_s = now_monotonic_high_precision_usec();
    int ret = __netdata_rwlock_unlock(rwlock);
    usec_t end_s = now_monotonic_high_precision_usec();

    if(end_s - start_s >= NETDATA_TRACE_RWLOCKS_WAIT_TIME_TO_IGNORE_USEC)
        fprintf(stderr,
                "RW_LOCK ON LOCK %p: %d, '%s' (function %s() %lu@%s) WAITED to UNLOCK for %llu usec.\n",
                rwlock,
                gettid(), netdata_thread_tag(),
                function, line, file,
                end_s - start_s);

    if(likely(!ret && locker)) remove_rwlock_locker(file, function, line, rwlock, locker);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_unlock(%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, end_s - start_s, line, file, function);

    return ret;
}

int netdata_rwlock_tryrdlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                   const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_tryrdlock(%p) from %lu@%s, %s()", rwlock, line, file, function);

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);
    if(locker && locker->lock == 'W')
        not_supported_by_posix_rwlocks(file, function, line, rwlock, 'R', "DEADLOCK - WANTS A READ LOCK BUT IT HAS A WRITE LOCK ALREADY");

    usec_t start_s = now_monotonic_high_precision_usec();
    int ret = __netdata_rwlock_tryrdlock(rwlock);
    usec_t end_s = now_monotonic_high_precision_usec();

    if(!ret)
        locker = update_or_add_rwlock_locker(file, function, line, rwlock, locker, 'R');

    if(end_s - start_s >= NETDATA_TRACE_RWLOCKS_WAIT_TIME_TO_IGNORE_USEC)
        fprintf(stderr,
                "RW_LOCK ON LOCK %p: %d, '%s' (function %s() %lu@%s) WAITED to TRYREAD for %llu usec.\n",
                rwlock,
                gettid(), netdata_thread_tag(),
                function, line, file,
                end_s - start_s);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_tryrdlock(%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, end_s - start_s, line, file, function);

    return ret;
}

int netdata_rwlock_trywrlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                   const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_trywrlock(%p) from %lu@%s, %s()", rwlock, line, file, function);

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);
    if(locker)
        not_supported_by_posix_rwlocks(file, function, line, rwlock, 'W', "ALREADY HAS THIS LOCK");

    usec_t start_s = now_monotonic_high_precision_usec();
    int ret = __netdata_rwlock_trywrlock(rwlock);
    usec_t end_s = now_monotonic_high_precision_usec();

    if(!ret)
        locker = update_or_add_rwlock_locker(file, function, line, rwlock, locker, 'W');

    if(end_s - start_s >= NETDATA_TRACE_RWLOCKS_WAIT_TIME_TO_IGNORE_USEC)
        fprintf(stderr,
                "RW_LOCK ON LOCK %p: %d, '%s' (function %s() %lu@%s) WAITED to TRYWRITE for %llu usec.\n",
                rwlock,
                gettid(), netdata_thread_tag(),
                function, line, file,
                end_s - start_s);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_trywrlock(%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, end_s - start_s, line, file, function);

    return ret;
}

#endif // NETDATA_TRACE_RWLOCKS
