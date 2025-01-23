#include "object-state.h"

OBJECT_STATE_ID object_state_id(OBJECT_STATE *os) {
    return __atomic_load_n(&os->state_id, __ATOMIC_ACQUIRE);
}

void object_state_activate(OBJECT_STATE *os) {
    __atomic_add_fetch(&os->state_id, 1, __ATOMIC_RELAXED);

    REFCOUNT expected = __atomic_load_n(&os->state_refcount, __ATOMIC_RELAXED);
    REFCOUNT desired;

    do {
        if(expected != OBJECT_STATE_DEACTIVATED) {
            fatal("OBJECT_STATE: attempt to activate already activated object (state refcount is %d)", expected);
            return;
        }

        desired = 0;

    } while(!__atomic_compare_exchange_n(
        &os->state_refcount, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));
}

void object_state_activate_if_not_activated(OBJECT_STATE *os) {
    __atomic_add_fetch(&os->state_id, 1, __ATOMIC_RELAXED);

    REFCOUNT expected = __atomic_load_n(&os->state_refcount, __ATOMIC_RELAXED);
    REFCOUNT desired;

    do {
        if(expected != OBJECT_STATE_DEACTIVATED)
            return;

        desired = 0;

    } while(!__atomic_compare_exchange_n(
        &os->state_refcount, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));
}

void object_state_deactivate(OBJECT_STATE *os) {
    __atomic_add_fetch(&os->state_id, 1, __ATOMIC_RELAXED);

    REFCOUNT expected = __atomic_load_n(&os->state_refcount, __ATOMIC_RELAXED);
    REFCOUNT desired;

    do {
        if(expected == OBJECT_STATE_DEACTIVATED) {
            fatal("OBJECT_STATE: attempt to deactivate object that is already deactivated (state refcount %d)", expected);
            return;
        }
        else if(expected < 0) {
            fatal("OBJECT_STATE: attempt to deactivate object that is already deactivating (state refcount %d)", expected);
            return;
        }

        // Current (-INT32_MAX) + holders
        desired = OBJECT_STATE_DEACTIVATED + expected;

    } while(!__atomic_compare_exchange_n(
        &os->state_refcount, &expected, desired, false, __ATOMIC_SEQ_CST, __ATOMIC_RELAXED));

    // Now wait for all holders to release
    while(__atomic_load_n(&os->state_refcount, __ATOMIC_ACQUIRE) != OBJECT_STATE_DEACTIVATED)
        tinysleep();   // Busy wait until all holders are gone
}

bool object_state_acquire(OBJECT_STATE *os, OBJECT_STATE_ID wanted_state_id) {
    REFCOUNT expected = __atomic_load_n(&os->state_refcount, __ATOMIC_RELAXED);
    REFCOUNT desired;

    do {
        // If refcount is negative, it means deactivation is in progress or complete
        if(expected < 0)
            return false;

        desired = expected + 1;

    } while(!__atomic_compare_exchange_n(
        &os->state_refcount, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));

    if(object_state_id(os) != wanted_state_id) {
        object_state_release(os);
        return false;
    }

    return true;
}

void object_state_release(OBJECT_STATE *os) {
    __atomic_sub_fetch(&os->state_refcount, 1, __ATOMIC_RELEASE);
}
