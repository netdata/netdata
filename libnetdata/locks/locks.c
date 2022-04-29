// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// automatic thread cancelability management, based on locks

static __thread int netdata_thread_first_cancelability = 0;
static __thread int netdata_thread_lock_cancelability = 0;

inline void netdata_thread_disable_cancelability(void) {
    int old;
    int ret = pthread_setcancelstate(PTHREAD_CANCEL_DISABLE, &old);
    if(ret != 0)
        error("THREAD_CANCELABILITY: pthread_setcancelstate() on thread %s returned error %d", netdata_thread_tag(), ret);
    else {
        if(!netdata_thread_lock_cancelability)
            netdata_thread_first_cancelability = old;

        netdata_thread_lock_cancelability++;
    }
}

inline void netdata_thread_enable_cancelability(void) {
    if(netdata_thread_lock_cancelability < 1) {
        error("THREAD_CANCELABILITY: netdata_thread_enable_cancelability(): invalid thread cancelability count %d on thread %s - results will be undefined - please report this!", netdata_thread_lock_cancelability, netdata_thread_tag());
    }
    else if(netdata_thread_lock_cancelability == 1) {
        int old = 1;
        int ret = pthread_setcancelstate(netdata_thread_first_cancelability, &old);
        if(ret != 0)
            error("THREAD_CANCELABILITY: pthread_setcancelstate() on thread %s returned error %d", netdata_thread_tag(), ret);
        else {
            if(old != PTHREAD_CANCEL_DISABLE)
                error("THREAD_CANCELABILITY: netdata_thread_enable_cancelability(): old thread cancelability on thread %s was changed, expected DISABLED (%d), found %s (%d) - please report this!", netdata_thread_tag(), PTHREAD_CANCEL_DISABLE, (old == PTHREAD_CANCEL_ENABLE)?"ENABLED":"UNKNOWN", old);
        }

        netdata_thread_lock_cancelability = 0;
    }
    else
        netdata_thread_lock_cancelability--;
}

// ----------------------------------------------------------------------------
// mutex

int __netdata_mutex_init(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_init(mutex, NULL);
    if(unlikely(ret != 0))
        error("MUTEX_LOCK: failed to initialize (code %d).", ret);
    return ret;
}

int __netdata_mutex_lock(netdata_mutex_t *mutex) {
    netdata_thread_disable_cancelability();

    int ret = pthread_mutex_lock(mutex);
    if(unlikely(ret != 0)) {
        netdata_thread_enable_cancelability();
        error("MUTEX_LOCK: failed to get lock (code %d)", ret);
    }
    return ret;
}

int __netdata_mutex_trylock(netdata_mutex_t *mutex) {
    netdata_thread_disable_cancelability();

    int ret = pthread_mutex_trylock(mutex);
    if(ret != 0)
        netdata_thread_enable_cancelability();

    return ret;
}

int __netdata_mutex_unlock(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_unlock(mutex);
    if(unlikely(ret != 0))
        error("MUTEX_LOCK: failed to unlock (code %d).", ret);
    else
        netdata_thread_enable_cancelability();

    return ret;
}

int netdata_mutex_init_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                             const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    usec_t start = 0;
    (void)start;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_init(0x%p) from %lu@%s, %s()", mutex, line, file, function);
    }

    int ret = __netdata_mutex_init(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_init(0x%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_mutex_lock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                             const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    usec_t start = 0;
    (void)start;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_lock(0x%p) from %lu@%s, %s()", mutex, line, file, function);
    }

    int ret = __netdata_mutex_lock(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_lock(0x%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_mutex_trylock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    usec_t start = 0;
    (void)start;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_trylock(0x%p) from %lu@%s, %s()", mutex, line, file, function);
    }

    int ret = __netdata_mutex_trylock(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_trylock(0x%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_mutex_unlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                               const unsigned long line __maybe_unused, netdata_mutex_t *mutex) {
    usec_t start = 0;
    (void)start;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_unlock(0x%p) from %lu@%s, %s()", mutex, line, file, function);
    }

    int ret = __netdata_mutex_unlock(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_unlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

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

    return ret;
}

int __netdata_rwlock_wrlock(netdata_rwlock_t *rwlock) {
    netdata_thread_disable_cancelability();

    int ret = pthread_rwlock_wrlock(&rwlock->rwlock_t);
    if(unlikely(ret != 0)) {
        error("RW_LOCK: failed to obtain write lock (code %d)", ret);
        netdata_thread_enable_cancelability();
    }

    return ret;
}

int __netdata_rwlock_unlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_unlock(&rwlock->rwlock_t);
    if(unlikely(ret != 0))
        error("RW_LOCK: failed to release lock (code %d)", ret);
    else
        netdata_thread_enable_cancelability();

    return ret;
}

int __netdata_rwlock_tryrdlock(netdata_rwlock_t *rwlock) {
    netdata_thread_disable_cancelability();

    int ret = pthread_rwlock_tryrdlock(&rwlock->rwlock_t);
    if(ret != 0)
        netdata_thread_enable_cancelability();

    return ret;
}

int __netdata_rwlock_trywrlock(netdata_rwlock_t *rwlock) {
    netdata_thread_disable_cancelability();

    int ret = pthread_rwlock_trywrlock(&rwlock->rwlock_t);
    if(ret != 0)
        netdata_thread_enable_cancelability();

    return ret;
}

#ifdef NETDATA_INTERNAL_CHECKS

// ----------------------------------------------------------------------------
// lockers list

void not_supported_by_posix_rwlocks(const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock, char locktype, const char *reason) {
    __netdata_mutex_lock(&rwlock->lockers_mutex);
    fprintf(stderr,
            "RW_LOCK FATAL ON LOCK 0x%08x: %zu, '%s' (function %s() %llu@%s) attempts to acquire a '%c' lock but is not supported by POSIX because: %s\n"
            "There are %zu readers and %zu writers are holding the lock:\n",
            (uintptr_t)rwlock,
            gettid(), netdata_thread_tag(),
            function, line, file,
            locktype,
            reason,
            rwlock->readers, rwlock->writers);

    int i;
    usec_t now = now_monotonic_usec();
    netdata_rwlock_locker *p;
    for(i = 1, p = rwlock->lockers; p ;p = p->next, i++) {
        fprintf(stderr,
                "     => %i: RW_LOCK: process %zu '%s' (function %s() %llu@%s) is having %zu '%c' lock for %llu usec.\n",
                i,
                p->pid, p->tag,
                p->function, p->line, p->file,
                p->callers, p->lock,
                (now - p->start_s));
    }
    __netdata_mutex_unlock(&rwlock->lockers_mutex);
}

static void log_rwlock_lockers(const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock, const char *reason, char locktype) {
    __netdata_mutex_lock(&rwlock->lockers_mutex);
    fprintf(stderr,
            "RW_LOCK ON LOCK 0x%08x: %zu, '%s' (function %s() %llu@%s) %s a '%c' lock.\n"
            "There are %zu readers and %zu writers are holding the lock:\n",
            (uintptr_t)rwlock,
            gettid(), netdata_thread_tag(),
            function, line, file,
            reason, locktype,
            rwlock->readers, rwlock->writers);

    int i;
    usec_t now = now_monotonic_usec();
    netdata_rwlock_locker *p;
    for(i = 1, p = rwlock->lockers; p ;p = p->next, i++) {
        fprintf(stderr,
                "     => %i: RW_LOCK: process %zu '%s' (function %s() %llu@%s) is having %zu '%c' lock for %llu usec.\n",
                i,
                p->pid, p->tag,
                p->function, p->line, p->file,
                p->callers, p->lock,
                (now - p->start_s));
    }
    __netdata_mutex_unlock(&rwlock->lockers_mutex);
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
    p->start_s = now_monotonic_usec();

    __netdata_mutex_lock(&rwlock->lockers_mutex);
    p->next = rwlock->lockers;
    rwlock->lockers = p;
    if(lock_type == 'R') rwlock->readers++;
    if(lock_type == 'W') rwlock->writers++;
    __netdata_mutex_unlock(&rwlock->lockers_mutex);

    return p;
}

static void remove_rwlock_locker(const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock, netdata_rwlock_locker *locker) {
    if(locker->callers == 0)
        fprintf(stderr,
                "RW_LOCK ON LOCK 0x%08x: %d, '%s' (function %s() %lu@%s) callers should be positive but it is zero\n",
                (uintptr_t)rwlock,
                locker->pid, locker->tag,
                locker->function, locker->line, locker->file);

    if(locker->callers > 1 && locker->lock != 'R')
        fprintf(stderr,
                "RW_LOCK ON LOCK 0x%08x: %d, '%s' (function %s() %lu@%s) only 'R' locks support nesting, but here we have %zu on '%c' lock.\n",
                (uintptr_t)rwlock,
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
                    "RW_LOCK ON LOCK 0x%08x: %d, '%s' (function %s() %lu@%s) with %zu '%c' lock is not found.\n",
                    (uintptr_t)rwlock,
                    locker->pid, locker->tag,
                    locker->function, locker->line, locker->file,
                    locker->callers, locker->lock);
        }
        else {
            freez(locker);
        }
    }

    __netdata_mutex_unlock(&rwlock->lockers_mutex);
}

static netdata_rwlock_locker *find_rwlock_locker(const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    pid_t pid = gettid();
    netdata_rwlock_locker *p;

    __netdata_mutex_lock(&rwlock->lockers_mutex);
    for(p = rwlock->lockers; p ;p = p->next) {
        if(p->pid == pid) break;
    }
    __netdata_mutex_unlock(&rwlock->lockers_mutex);

    return p;
}

static void update_or_add_rwlock_locker(const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock, netdata_rwlock_locker *locker, char locktype) {
    if(!locker) {
        (void)add_rwlock_locker(file, function, line, rwlock, locktype);
    }
    else if(locker->lock == 'R' && locktype == 'R') {
        __netdata_mutex_lock(&rwlock->lockers_mutex);
        locker->callers++;
        __netdata_mutex_unlock(&rwlock->lockers_mutex);
    }
    else {
        not_supported_by_posix_rwlocks(file, function, line, rwlock, locktype, "DEADLOCK - WANTS TO CHANGE LOCK TYPE BUT ALREADY HAS THIS LOCKED");
    }
}

// ----------------------------------------------------------------------------
// debug versions of rwlock

int netdata_rwlock_destroy_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                 const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    usec_t start = 0;
    (void)start;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_destroy(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

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

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_destroy(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_rwlock_init_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                              const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    usec_t start = 0;
    (void)start;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_init(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_init(rwlock);
    if(!ret) {
        __netdata_mutex_init(&rwlock->lockers_mutex);
        rwlock->lockers = NULL;
        rwlock->readers = 0;
        rwlock->writers = 0;
    }

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_init(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_rwlock_rdlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    usec_t start = 0;
    (void)start;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_rdlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);
    if(locker && locker->lock == 'R') {
        log_rwlock_lockers(file, function, line, rwlock, "NESTED READ LOCK REQUEST", 'R');
    }

    int log = 0;
    if(rwlock->writers) {
        log_rwlock_lockers(file, function, line, rwlock, "WANTS", 'R');
        log = 1;
    }

    int ret = __netdata_rwlock_rdlock(rwlock);
    if(!ret) {
        update_or_add_rwlock_locker(file, function, line, rwlock, locker, 'R');
        if(log) log_rwlock_lockers(file, function, line, rwlock, "GOT", 'R');
    }

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_rdlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_rwlock_wrlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    usec_t start = 0;
    (void)start;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_wrlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);
    if(locker)
        not_supported_by_posix_rwlocks(file, function, line, rwlock, 'W', "DEADLOCK - WANTS A WRITE LOCK BUT ALREADY HAVE THIS LOCKED");

    int log = 0;
    if(rwlock->readers) {
        log_rwlock_lockers(file, function, line, rwlock, "WANTS", 'W');
        log = 1;
    }

    int ret = __netdata_rwlock_wrlock(rwlock);
    if(!ret){
        update_or_add_rwlock_locker(file, function, line, rwlock, locker, 'W');
        if(log) log_rwlock_lockers(file, function, line, rwlock, "GOT", 'W');
    }

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_wrlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_rwlock_unlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    usec_t start = 0;
    (void)start;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_unlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);
    if(unlikely(!locker))
        not_supported_by_posix_rwlocks(file, function, line, rwlock, 'U', "UNLOCK WITHOUT LOCK");

    int ret = __netdata_rwlock_unlock(rwlock);

    if(likely(!ret && locker)) remove_rwlock_locker(file, function, line, rwlock, locker);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_unlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_rwlock_tryrdlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                   const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    usec_t start = 0;
    (void)start;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_tryrdlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);
    if(locker && locker->lock == 'W')
        not_supported_by_posix_rwlocks(file, function, line, rwlock, 'R', "DEADLOCK - WANTS A READ LOCK BUT IT HAS A WRITE LOCK ALREADY");

    int ret = __netdata_rwlock_tryrdlock(rwlock);
    if(!ret)
        update_or_add_rwlock_locker(file, function, line, rwlock, locker, 'R');

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_tryrdlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_rwlock_trywrlock_debug(const char *file __maybe_unused, const char *function __maybe_unused,
                                   const unsigned long line __maybe_unused, netdata_rwlock_t *rwlock) {
    usec_t start = 0;
    (void)start;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_trywrlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    netdata_rwlock_locker *locker = find_rwlock_locker(file, function, line, rwlock);
    if(locker)
        not_supported_by_posix_rwlocks(file, function, line, rwlock, 'W', "ALREADY HAS THIS LOCK");

    int ret = __netdata_rwlock_trywrlock(rwlock);
    if(!ret)
        update_or_add_rwlock_locker(file, function, line, rwlock, locker, 'W');

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_trywrlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

#endif // NETDATA_INTERNAL_CHECKS
