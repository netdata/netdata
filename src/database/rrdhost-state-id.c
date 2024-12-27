// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdhost-state-id.h"
#include "rrd.h"

RRDHOST_STATE rrdhost_state_id(struct rrdhost *host) {
    return __atomic_load_n(&host->state_id, __ATOMIC_ACQUIRE);
}

void rrdhost_state_connected(RRDHOST *host) {
    __atomic_add_fetch(&host->state_id, 1, __ATOMIC_RELAXED);

    REFCOUNT expected = __atomic_load_n(&host->state_refcount, __ATOMIC_RELAXED);
    REFCOUNT desired;

    do {
        if(expected != RRDHOST_STATE_DISCONNECTED) {
            fatal("Attempt to connect node '%s' which is already connected (state refcount is %d)",
                  rrdhost_hostname(host), expected);
            return;
        }

        desired = 0;

    } while(!__atomic_compare_exchange_n(
        &host->state_refcount, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));
}

void rrdhost_state_disconnected(RRDHOST *host) {
    __atomic_add_fetch(&host->state_id, 1, __ATOMIC_RELAXED);

    REFCOUNT expected = __atomic_load_n(&host->state_refcount, __ATOMIC_RELAXED);
    REFCOUNT desired;

    do {
        if(expected == RRDHOST_STATE_DISCONNECTED) {
            fatal("Attempt to disconnect node '%s' which is already disconnected (state refcount is %d)",
                  rrdhost_hostname(host), expected);
            return;
        }

        desired = RRDHOST_STATE_DISCONNECTED;

    } while(!__atomic_compare_exchange_n(
        &host->state_refcount, &expected, desired, false, __ATOMIC_SEQ_CST, __ATOMIC_RELAXED));
}

bool rrdhost_state_acquire(RRDHOST *host, RRDHOST_STATE wanted_state_id) {
    REFCOUNT expected = __atomic_load_n(&host->state_refcount, __ATOMIC_RELAXED);
    REFCOUNT desired;

    do {
        if(expected == RRDHOST_STATE_DISCONNECTED)
            return false;

        if(expected < 0) {
            fatal("Attempted to acquire the state of host '%s', with a negative state refcount %d",
                  rrdhost_hostname(host), expected);
            return false;
        }

        desired = expected + 1;

    } while(!__atomic_compare_exchange_n(
        &host->state_refcount, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));

    if(rrdhost_state_id(host) != wanted_state_id) {
        rrdhost_state_release(host);
        return false;
    }

    return true;
}

void rrdhost_state_release(RRDHOST *host) {
    REFCOUNT final = __atomic_sub_fetch(&host->state_refcount, 1, __ATOMIC_RELEASE);

    if(final < 0)
        fatal("Released the state of host '%s', but it now has a negative refcount %d",
              rrdhost_hostname(host), final);
}
