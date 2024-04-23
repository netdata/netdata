// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_LOCKS_H
#define NETDATA_DICTIONARY_LOCKS_H

#include "dictionary-internals.h"

// ----------------------------------------------------------------------------
// dictionary locks

static inline size_t dictionary_locks_init(DICTIONARY *dict) {
    if(likely(!is_dictionary_single_threaded(dict))) {
        rw_spinlock_init(&dict->index.rw_spinlock);
        rw_spinlock_init(&dict->items.rw_spinlock);
    }

    return 0;
}

static inline size_t dictionary_locks_destroy(DICTIONARY *dict __maybe_unused) {
    return 0;
}

static inline void ll_recursive_lock_set_thread_as_writer(DICTIONARY *dict) {
    pid_t expected = 0, desired = gettid_cached();
    if(!__atomic_compare_exchange_n(&dict->items.writer_pid, &expected, desired, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED))
        fatal("DICTIONARY: Cannot set thread %d as exclusive writer, expected %d, desired %d, found %d.", gettid_cached(), expected, desired, __atomic_load_n(&dict->items.writer_pid, __ATOMIC_RELAXED));
}

static inline void ll_recursive_unlock_unset_thread_writer(DICTIONARY *dict) {
    pid_t expected = gettid_cached(), desired = 0;
    if(!__atomic_compare_exchange_n(&dict->items.writer_pid, &expected, desired, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED))
        fatal("DICTIONARY: Cannot unset thread %d as exclusive writer, expected %d, desired %d, found %d.", gettid_cached(), expected, desired, __atomic_load_n(&dict->items.writer_pid, __ATOMIC_RELAXED));
}

static inline bool ll_recursive_lock_is_thread_the_writer(DICTIONARY *dict) {
    pid_t tid = gettid_cached();
    return tid > 0 && tid == __atomic_load_n(&dict->items.writer_pid, __ATOMIC_RELAXED);
}

static inline void ll_recursive_lock(DICTIONARY *dict, char rw) {
    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    if(ll_recursive_lock_is_thread_the_writer(dict)) {
        dict->items.writer_depth++;
        return;
    }

    if(rw == DICTIONARY_LOCK_READ || rw == DICTIONARY_LOCK_REENTRANT || rw == 'R') {
        // read lock
        rw_spinlock_read_lock(&dict->items.rw_spinlock);
    }
    else {
        // write lock
        rw_spinlock_write_lock(&dict->items.rw_spinlock);
        ll_recursive_lock_set_thread_as_writer(dict);
    }
}

static inline void ll_recursive_unlock(DICTIONARY *dict, char rw) {
    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    if(ll_recursive_lock_is_thread_the_writer(dict) && dict->items.writer_depth > 0) {
        dict->items.writer_depth--;
        return;
    }

    if(rw == DICTIONARY_LOCK_READ || rw == DICTIONARY_LOCK_REENTRANT || rw == 'R') {
        // read unlock

        rw_spinlock_read_unlock(&dict->items.rw_spinlock);
    }
    else {
        // write unlock

        ll_recursive_unlock_unset_thread_writer(dict);

        rw_spinlock_write_unlock(&dict->items.rw_spinlock);
    }
}

static inline void dictionary_index_lock_rdlock(DICTIONARY *dict) {
    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    rw_spinlock_read_lock(&dict->index.rw_spinlock);
}

static inline void dictionary_index_rdlock_unlock(DICTIONARY *dict) {
    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    rw_spinlock_read_unlock(&dict->index.rw_spinlock);
}

static inline void dictionary_index_lock_wrlock(DICTIONARY *dict) {
    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    rw_spinlock_write_lock(&dict->index.rw_spinlock);
}
static inline void dictionary_index_wrlock_unlock(DICTIONARY *dict) {
    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    rw_spinlock_write_unlock(&dict->index.rw_spinlock);
}


#endif //NETDATA_DICTIONARY_LOCKS_H
