// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOCKS_H
#define NETDATA_LOCKS_H 1

#include "../libnetdata.h"
#include "../clocks/clocks.h"

typedef pthread_mutex_t netdata_mutex_t;
#define NETDATA_MUTEX_INITIALIZER PTHREAD_MUTEX_INITIALIZER

#ifdef NETDATA_TRACE_RWLOCKS
typedef struct netdata_rwlock_locker {
    pid_t pid;
    const char *tag;
    char lock; // 'R', 'W'
    const char *file;
    const char *function;
    unsigned long line;
    size_t callers;
    usec_t start_s;
    struct netdata_rwlock_t **all_caller_locks;
    struct netdata_rwlock_locker *next;
} netdata_rwlock_locker;

typedef struct netdata_rwlock_t {
    pthread_rwlock_t rwlock_t;       // the lock
    size_t readers;                  // the number of reader on the lock
    size_t writers;                  // the number of writers on the lock
    netdata_mutex_t lockers_mutex;   // a mutex to protect the linked list of the lock holding threads
    netdata_rwlock_locker *lockers;  // the linked list of the lock holding threads
} netdata_rwlock_t;

#define NETDATA_RWLOCK_INITIALIZER {                \
        .rwlock_t = PTHREAD_RWLOCK_INITIALIZER,     \
        .readers = 0,                               \
        .writers = 0,                               \
        .lockers_mutex = NETDATA_MUTEX_INITIALIZER, \
        .lockers = NULL                             \
    }

#else // NETDATA_TRACE_RWLOCKS

typedef struct netdata_rwlock_t {
    pthread_rwlock_t rwlock_t;
} netdata_rwlock_t;

#define NETDATA_RWLOCK_INITIALIZER { \
        .rwlock_t = PTHREAD_RWLOCK_INITIALIZER \
    }

#endif // NETDATA_TRACE_RWLOCKS

int __netdata_mutex_init(netdata_mutex_t *mutex);
int __netdata_mutex_destroy(netdata_mutex_t *mutex);
int __netdata_mutex_lock(netdata_mutex_t *mutex);
int __netdata_mutex_trylock(netdata_mutex_t *mutex);
int __netdata_mutex_unlock(netdata_mutex_t *mutex);

int __netdata_rwlock_destroy(netdata_rwlock_t *rwlock);
int __netdata_rwlock_init(netdata_rwlock_t *rwlock);
int __netdata_rwlock_rdlock(netdata_rwlock_t *rwlock);
int __netdata_rwlock_wrlock(netdata_rwlock_t *rwlock);
int __netdata_rwlock_unlock(netdata_rwlock_t *rwlock);
int __netdata_rwlock_tryrdlock(netdata_rwlock_t *rwlock);
int __netdata_rwlock_trywrlock(netdata_rwlock_t *rwlock);

void netdata_thread_disable_cancelability(void);
void netdata_thread_enable_cancelability(void);

#ifdef NETDATA_TRACE_RWLOCKS

int netdata_mutex_init_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex);
int netdata_mutex_destroy_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex);
int netdata_mutex_lock_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex);
int netdata_mutex_trylock_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex);
int netdata_mutex_unlock_debug( const char *file, const char *function, const unsigned long line, netdata_mutex_t *mutex);

int netdata_rwlock_destroy_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock);
int netdata_rwlock_init_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock);
int netdata_rwlock_rdlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock);
int netdata_rwlock_wrlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock);
int netdata_rwlock_unlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock);
int netdata_rwlock_tryrdlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock);
int netdata_rwlock_trywrlock_debug( const char *file, const char *function, const unsigned long line, netdata_rwlock_t *rwlock);

#define netdata_mutex_init(mutex)    netdata_mutex_init_debug(__FILE__, __FUNCTION__, __LINE__, mutex)
#define netdata_mutex_destroy(mutex) netdata_mutex_init_debug(__FILE__, __FUNCTION__, __LINE__, mutex)
#define netdata_mutex_lock(mutex)    netdata_mutex_lock_debug(__FILE__, __FUNCTION__, __LINE__, mutex)
#define netdata_mutex_trylock(mutex) netdata_mutex_trylock_debug(__FILE__, __FUNCTION__, __LINE__, mutex)
#define netdata_mutex_unlock(mutex)  netdata_mutex_unlock_debug(__FILE__, __FUNCTION__, __LINE__, mutex)

#define netdata_rwlock_destroy(rwlock)   netdata_rwlock_destroy_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)
#define netdata_rwlock_init(rwlock)      netdata_rwlock_init_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)
#define netdata_rwlock_rdlock(rwlock)    netdata_rwlock_rdlock_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)
#define netdata_rwlock_wrlock(rwlock)    netdata_rwlock_wrlock_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)
#define netdata_rwlock_unlock(rwlock)    netdata_rwlock_unlock_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)
#define netdata_rwlock_tryrdlock(rwlock) netdata_rwlock_tryrdlock_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)
#define netdata_rwlock_trywrlock(rwlock) netdata_rwlock_trywrlock_debug(__FILE__, __FUNCTION__, __LINE__, rwlock)

#else // !NETDATA_TRACE_RWLOCKS

#define netdata_mutex_init(mutex)    __netdata_mutex_init(mutex)
#define netdata_mutex_destroy(mutex) __netdata_mutex_destroy(mutex)
#define netdata_mutex_lock(mutex)    __netdata_mutex_lock(mutex)
#define netdata_mutex_trylock(mutex) __netdata_mutex_trylock(mutex)
#define netdata_mutex_unlock(mutex)  __netdata_mutex_unlock(mutex)

#define netdata_rwlock_destroy(rwlock)    __netdata_rwlock_destroy(rwlock)
#define netdata_rwlock_init(rwlock)       __netdata_rwlock_init(rwlock)
#define netdata_rwlock_rdlock(rwlock)     __netdata_rwlock_rdlock(rwlock)
#define netdata_rwlock_wrlock(rwlock)     __netdata_rwlock_wrlock(rwlock)
#define netdata_rwlock_unlock(rwlock)     __netdata_rwlock_unlock(rwlock)
#define netdata_rwlock_tryrdlock(rwlock)  __netdata_rwlock_tryrdlock(rwlock)
#define netdata_rwlock_trywrlock(rwlock)  __netdata_rwlock_trywrlock(rwlock)

#endif // NETDATA_TRACE_RWLOCKS

#endif //NETDATA_LOCKS_H
