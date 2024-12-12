// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdhost-state-id.h"
#include "rrd.h"

uint32_t rrdhost_state_id(struct rrdhost *host) {
    return __atomic_load_n(&host->state_id, __ATOMIC_RELAXED);
}

bool rrdhost_state_connected(RRDHOST *host) {
    int32_t expected = __atomic_load_n(&host->state_refcount, __ATOMIC_RELAXED);
    int32_t desired;

    do {
        if(expected >= 0) {
            internal_fatal(true, "Cannot get the node connected");
            return false;
        }

        desired = 0;

    } while(!__atomic_compare_exchange_n(
        &host->state_refcount, &expected, desired, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    return true;
}

bool rrdhost_state_disconnected(RRDHOST *host) {
    int32_t expected = __atomic_load_n(&host->state_refcount, __ATOMIC_RELAXED);
    int32_t desired;

    do {
        if(expected < 0) {
            internal_fatal(true, "Cannot get the node disconnected");
            return false;
        }

        desired = -1;

    } while(!__atomic_compare_exchange_n(
        &host->state_refcount, &expected, desired, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    return true;
}

bool rrdhost_state_acquire(RRDHOST *host) {
    int32_t expected = __atomic_load_n(&host->state_refcount, __ATOMIC_RELAXED);
    int32_t desired;

    do {
        if(expected < 0)
            return false;

        desired = expected + 1;

    } while(!__atomic_compare_exchange_n(
        &host->state_refcount, &expected, desired, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    return true;
}

void rrdhost_state_release(RRDHOST *host) {
    __atomic_sub_fetch(&host->state_refcount, 1, __ATOMIC_RELAXED);
}
