#ifndef ML_MUTEX_H
#define ML_MUTEX_H

#include "ml-private.h"

class Mutex {
public:
    Mutex() {
        netdata_mutex_init(&M);
    }

    void lock() {
        netdata_mutex_lock(&M);
    }

    void unlock() {
        netdata_mutex_unlock(&M);
    }

    bool try_lock() {
        return netdata_mutex_trylock(&M) == 0;
    }

    netdata_mutex_t *inner() {
        return &M;
    }

    ~Mutex() {
        netdata_mutex_destroy(&M);
    }

private:
    netdata_mutex_t M;
};

#endif /* ML_MUTEX_H */
