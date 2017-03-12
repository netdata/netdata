#ifndef NETDATA_LOCKS_H
#define NETDATA_LOCKS_H

// ----------------------------------------------------------------------------
// mutex

typedef pthread_mutex_t netdata_mutex_t;

#define NETDATA_MUTEX_INITIALIZER PTHREAD_MUTEX_INITIALIZER

static inline int __netdata_mutex_init(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_init(mutex, NULL);
    if(unlikely(ret != 0))
        error("MUTEX_LOCK: failed to initialize (code %d).", ret);
    return ret;
}

static inline int __netdata_mutex_lock(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_lock(mutex);
    if(unlikely(ret != 0))
        error("MUTEX_LOCK: failed to get lock (code %d)", ret);
    return ret;
}

static inline int __netdata_mutex_trylock(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_trylock(mutex);
    return ret;
}

static inline int __netdata_mutex_unlock(netdata_mutex_t *mutex) {
    int ret = pthread_mutex_unlock(mutex);
    if(unlikely(ret != 0))
        error("MUTEX_LOCK: failed to unlock (code %d).", ret);
    return ret;
}

#ifdef NETDATA_INTERNAL_CHECKS

static inline int netdata_mutex_init_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_init(0x%p) from %lu@%s, %s()", mutex, line, file, function);
    }

    int ret = __netdata_mutex_init(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_init(0x%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

static inline int netdata_mutex_lock_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_lock(0x%p) from %lu@%s, %s()", mutex, line, file, function);
    }

    int ret = __netdata_mutex_lock(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_lock(0x%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

static inline int netdata_mutex_trylock_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_trylock(0x%p) from %lu@%s, %s()", mutex, line, file, function);
    }

    int ret = __netdata_mutex_trylock(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_trylock(0x%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

static inline int netdata_mutex_unlock_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_unlock(0x%p) from %lu@%s, %s()", mutex, line, file, function);
    }

    int ret = __netdata_mutex_unlock(mutex);

    debug(D_LOCKS, "MUTEX_LOCK: netdata_mutex_unlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", mutex, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

#define netdata_mutex_init(mutex)    netdata_mutex_init_debug(__FILE__, __FUNCTION__, __LINE__, mutex)
#define netdata_mutex_lock(mutex)    netdata_mutex_lock_debug(__FILE__, __FUNCTION__, __LINE__, mutex)
#define netdata_mutex_trylock(mutex) netdata_mutex_trylock_debug(__FILE__, __FUNCTION__, __LINE__, mutex)
#define netdata_mutex_unlock(mutex)  netdata_mutex_unlock_debug(__FILE__, __FUNCTION__, __LINE__, mutex)

#else // !NETDATA_INTERNAL_CHECKS

#define netdata_mutex_init(mutex)    __netdata_mutex_init(mutex)
#define netdata_mutex_lock(mutex)    __netdata_mutex_lock(mutex)
#define netdata_mutex_trylock(mutex) __netdata_mutex_trylock(mutex)
#define netdata_mutex_unlock(mutex)  __netdata_mutex_unlock(mutex)

#endif // NETDATA_INTERNAL_CHECKS


// ----------------------------------------------------------------------------
// r/w lock

typedef pthread_rwlock_t netdata_rwlock_t;

#define NETDATA_RWLOCK_INITIALIZER PTHREAD_RWLOCK_INITIALIZER

static inline int __netdata_rwlock_destroy(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_destroy(rwlock);
    if(unlikely(ret != 0))
        error("RW_LOCK: failed to destroy lock (code %d)", ret);
    return ret;
}

static inline int __netdata_rwlock_init(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_init(rwlock, NULL);
    if(unlikely(ret != 0))
        error("RW_LOCK: failed to initialize lock (code %d)", ret);
    return ret;
}

static inline int __netdata_rwlock_rdlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_rdlock(rwlock);
    if(unlikely(ret != 0))
        error("RW_LOCK: failed to obtain read lock (code %d)", ret);
    return ret;
}

static inline int __netdata_rwlock_wrlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_wrlock(rwlock);
    if(unlikely(ret != 0))
        error("RW_LOCK: failed to obtain write lock (code %d)", ret);
    return ret;
}

static inline int __netdata_rwlock_unlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_unlock(rwlock);
    if(unlikely(ret != 0))
        error("RW_LOCK: failed to release lock (code %d)", ret);
    return ret;
}

static inline int __netdata_rwlock_tryrdlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_tryrdlock(rwlock);
    return ret;
}

static inline int __netdata_rwlock_trywrlock(netdata_rwlock_t *rwlock) {
    int ret = pthread_rwlock_trywrlock(rwlock);
    return ret;
}


#ifdef NETDATA_INTERNAL_CHECKS

static inline int netdata_rwlock_destroy_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_destroy(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_destroy(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_destroy(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

static inline int netdata_rwlock_init_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_init(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_init(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_init(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

static inline int netdata_rwlock_rdlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_rdlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_rdlock(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_rdlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

static inline int netdata_rwlock_wrlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_wrlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_wrlock(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_wrlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

static inline int netdata_rwlock_unlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_unlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_unlock(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_unlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

static inline int netdata_rwlock_tryrdlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_tryrdlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_tryrdlock(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_tryrdlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

static inline int netdata_rwlock_trywrlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock) {
    usec_t start = 0;

    if(unlikely(debug_flags & D_LOCKS)) {
        start = now_boottime_usec();
        debug(D_LOCKS, "RW_LOCK: netdata_rwlock_trywrlock(0x%p) from %lu@%s, %s()", rwlock, line, file, function);
    }

    int ret = __netdata_rwlock_trywrlock(rwlock);

    debug(D_LOCKS, "RW_LOCK: netdata_rwlock_trywrlock(0x%p) = %d in %llu usec, from %lu@%s, %s()", rwlock, ret, now_boottime_usec() - start, line, file, function);

    return ret;
}

#define netdata_rwlock_destroy(rwlock)   netdata_rwlock_destroy_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)
#define netdata_rwlock_init(rwlock)      netdata_rwlock_init_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)
#define netdata_rwlock_rdlock(rwlock)    netdata_rwlock_rdlock_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)
#define netdata_rwlock_wrlock(rwlock)    netdata_rwlock_wrlock_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)
#define netdata_rwlock_unlock(rwlock)    netdata_rwlock_unlock_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)
#define netdata_rwlock_tryrdlock(rwlock) netdata_rwlock_tryrdlock_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)
#define netdata_rwlock_trywrlock(rwlock) netdata_rwlock_trywrlock_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)

#else // !NETDATA_INTERNAL_CHECKS

#define netdata_rwlock_destroy(rwlock)    __netdata_rwlock_destroy(rwlock)
#define netdata_rwlock_init(rwlock)       __netdata_rwlock_init(rwlock)
#define netdata_rwlock_rdlock(rwlock)     __netdata_rwlock_rdlock(rwlock)
#define netdata_rwlock_wrlock(rwlock)     __netdata_rwlock_wrlock(rwlock)
#define netdata_rwlock_unlock(rwlock)     __netdata_rwlock_unlock(rwlock)
#define netdata_rwlock_tryrdlock(rwlock)  __netdata_rwlock_tryrdlock(rwlock)
#define netdata_rwlock_trywrlock(rwlock)  __netdata_rwlock_trywrlock(rwlock)

#endif // NETDATA_INTERNAL_CHECKS

#endif //NETDATA_LOCKS_H
