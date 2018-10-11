// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata.h"

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

int netdata_mutex_init_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_init(0x%p) from %lu@%s, %s()", mutex, line, file, function);
    }

    int ret = __netdata_mutex_init(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_init(0x%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_mutex_lock_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_lock(0x%p) from %lu@%s, %s()", mutex, line, file, function);
    }

    int ret = __netdata_mutex_lock(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_lock(0x%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_mutex_trylock_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_trylock(0x%p) from %lu@%s, %s()", mutex, line, file, function);
    }

    int ret = __netdata_mutex_trylock(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_trylock(0x%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_mutex_unlock_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_unlock(0x%p) from %lu@%s, %s()", mutex, line, file, function);
    }

    int ret = __netdata_mutex_unlock(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_unlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}


// ----------------------------------------------------------------------------
// r/w lock

int __netdata_rwlock_destroy(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_destroy(rwlock);
    if(unlikely(ret != 0))
        error("RW_LOCK: failed to destroy lock (code %d)", ret);
    return ret;
}

int __netdata_rwlock_init(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_init(rwlock, NULL);
    if(unlikely(ret != 0))
        error("RW_LOCK: failed to initialize lock (code %d)", ret);
    return ret;
}

int __netdata_rwlock_rdlock(netdata_rwlock_t *rwlock) {
    netdata_thread_disable_cancelability();

    int ret = pthread_rwlock_rdlock(rwlock);
    if(unlikely(ret != 0)) {
        netdata_thread_enable_cancelability();
        error("RW_LOCK: failed to obtain read lock (code %d)", ret);
    }

    return ret;
}

int __netdata_rwlock_wrlock(netdata_rwlock_t *rwlock) {
    netdata_thread_disable_cancelability();

    int ret = pthread_rwlock_wrlock(rwlock);
    if(unlikely(ret != 0)) {
        error("RW_LOCK: failed to obtain write lock (code %d)", ret);
        netdata_thread_enable_cancelability();
    }

    return ret;
}

int __netdata_rwlock_unlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_unlock(rwlock);
    if(unlikely(ret != 0))
        error("RW_LOCK: failed to release lock (code %d)", ret);
    else
        netdata_thread_enable_cancelability();

    return ret;
}

int __netdata_rwlock_tryrdlock(netdata_rwlock_t *rwlock) {
    netdata_thread_disable_cancelability();

    int ret = pthread_rwlock_tryrdlock(rwlock);
    if(ret != 0)
        netdata_thread_enable_cancelability();

    return ret;
}

int __netdata_rwlock_trywrlock(netdata_rwlock_t *rwlock) {
    netdata_thread_disable_cancelability();

    int ret = pthread_rwlock_trywrlock(rwlock);
    if(ret != 0)
        netdata_thread_enable_cancelability();

    return ret;
}


int netdata_rwlock_destroy_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_destroy(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_destroy(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_destroy(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_rwlock_init_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_init(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_init(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_init(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_rwlock_rdlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_rdlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_rdlock(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_rdlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_rwlock_wrlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_wrlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_wrlock(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_wrlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_rwlock_unlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_unlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_unlock(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_unlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_rwlock_tryrdlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_tryrdlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_tryrdlock(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_tryrdlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

int netdata_rwlock_trywrlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_trywrlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_trywrlock(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_trywrlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}
