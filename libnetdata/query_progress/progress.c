// SPDX-License-Identifier: GPL-3.0-or-later

#include "progress.h"

#define PROGRESS_CACHE_SIZE 100

// ----------------------------------------------------------------------------
// hashtable for HASHED_KEY

// cleanup hashtable defines
#include "../simple_hashtable_undef.h"

struct query;
#define SIMPLE_HASHTABLE_VALUE_TYPE struct query
#define SIMPLE_HASHTABLE_KEY_TYPE uuid_t
#define SIMPLE_HASHTABLE_NAME _QUERY
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION query_transaction
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION query_compare_keys
#include "../simple_hashtable.h"

// ----------------------------------------------------------------------------

typedef struct query {
    uuid_t transaction;

    BUFFER *query;
    BUFFER *payload;

    usec_t started_ut;
    usec_t finished_ut;

    WEB_CLIENT_ACL acl;

    size_t response_size;
    short response_code;

    size_t updates;
    size_t all;
    size_t done;

    struct query *prev, *next;
} QUERY_PROGRESS;

static inline uuid_t *query_transaction(QUERY_PROGRESS *qp) {
    return qp ? &qp->transaction : NULL;
}

static inline bool query_compare_keys(uuid_t *t1, uuid_t *t2) {
    if(t1 == t2 || (t1 && t2 && memcmp(t1, t2, sizeof(uuid_t)) == 0))
        return true;

    return false;
}

static struct progress {
    SPINLOCK spinlock;
    bool initialized;

    struct {
        size_t available;
        QUERY_PROGRESS *list;
    } cache;

    SIMPLE_HASHTABLE_QUERY hashtable;

} progress = {
        .initialized = false,
};

SIMPLE_HASHTABLE_HASH query_hash(uuid_t *transaction) {
    struct uuid_hi_lo_t {
        uint64_t hi;
        uint64_t lo;
    } *parts = (struct uuid_hi_lo_t *)transaction;

    return parts->lo;
}

static inline void query_progress_init_unsafe(void) {
    if(!progress.initialized) {
        memset(&progress, 0, sizeof(progress));
        simple_hashtable_init_QUERY(&progress.hashtable, PROGRESS_CACHE_SIZE * 4);
        progress.initialized = true;
    }
}

static inline QUERY_PROGRESS *query_progress_find_unsafe(uuid_t *transaction) {
    SIMPLE_HASHTABLE_HASH hash = query_hash(transaction);
    SIMPLE_HASHTABLE_SLOT_QUERY *slot = simple_hashtable_get_slot_QUERY(&progress.hashtable, hash, transaction, true);
    QUERY_PROGRESS *qp = SIMPLE_HASHTABLE_SLOT_DATA(slot);
    return qp;
}

static inline void query_progress_hash(QUERY_PROGRESS *qp) {
    spinlock_lock(&progress.spinlock);
    SIMPLE_HASHTABLE_HASH hash = query_hash(&qp->transaction);
    SIMPLE_HASHTABLE_SLOT_QUERY *slot =
            simple_hashtable_get_slot_QUERY(&progress.hashtable, hash, &qp->transaction, true);

    internal_fatal(SIMPLE_HASHTABLE_SLOT_DATA(slot) != NULL && SIMPLE_HASHTABLE_SLOT_DATA(slot) != qp,
                   "Attempt to overwrite a progress slot, with another value");

    simple_hashtable_set_slot_QUERY(&progress.hashtable, slot, hash, qp);
    spinlock_unlock(&progress.spinlock);
}

static inline void query_progress_unhash_unsafe(QUERY_PROGRESS *qp) {
    SIMPLE_HASHTABLE_HASH hash = query_hash(&qp->transaction);
    SIMPLE_HASHTABLE_SLOT_QUERY *slot =
            simple_hashtable_get_slot_QUERY(&progress.hashtable, hash, &qp->transaction, true);

    internal_fatal(SIMPLE_HASHTABLE_SLOT_DATA(slot) != qp,
                   "Attempt to unhash a progress slot, with a different value");

    if(SIMPLE_HASHTABLE_SLOT_DATA(slot) == qp)
        simple_hashtable_del_slot_QUERY(&progress.hashtable, slot);
}

static inline void query_progress_cleanup_unsafe(QUERY_PROGRESS *qp) {
    query_progress_unhash_unsafe(qp);

    buffer_flush(qp->query);
    buffer_flush(qp->payload);

    memset(qp->transaction, 0, sizeof(uuid_t));
    qp->started_ut = qp->finished_ut = 0;
    qp->all = qp->done = 0;
    qp->next = qp->prev = NULL;
}

void query_progress_start(uuid_t *transaction, usec_t started_ut, WEB_CLIENT_ACL acl, const char *query, const char *payload) {
    if(!transaction)
        return;

    spinlock_lock(&progress.spinlock);
    query_progress_init_unsafe();

    QUERY_PROGRESS *qp = query_progress_find_unsafe(transaction);
    if(!qp) {
        // transaction is not found - get the first available
        qp = progress.cache.list;
        if (qp) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(progress.cache.list, qp, prev, next);
            progress.cache.available--;

            query_progress_cleanup_unsafe(qp);
            uuid_copy(qp->transaction, *transaction);
            qp->acl = acl;
            qp->started_ut = started_ut;
            qp->finished_ut = 0;
            qp->done = qp->all = 0;
        }
    }
    spinlock_unlock(&progress.spinlock);

    if(!qp) {
        qp = callocz(1, sizeof(*qp));
        qp->query = buffer_create(0, NULL);
        qp->payload = buffer_create(0, NULL);
        qp->acl = acl;
        uuid_copy(qp->transaction, *transaction);
        qp->started_ut = started_ut;
        qp->finished_ut = 0;
        qp->done = qp->all = 0;
    }

    if(started_ut && started_ut < qp->started_ut)
        qp->started_ut = started_ut;

    if(query && *query && !buffer_strlen(qp->query))
        buffer_strcat(qp->query, query);

    if(payload && *payload && !buffer_strlen(qp->payload))
        buffer_strcat(qp->payload, payload);

    if(!qp->started_ut)
        qp->started_ut = now_realtime_usec();

    qp->acl |= acl;

    query_progress_hash(qp);
}

void query_progress_set_all(uuid_t *transaction, size_t all) {
    if(!transaction)
        return;

    spinlock_lock(&progress.spinlock);
    query_progress_init_unsafe();

    QUERY_PROGRESS *qp = query_progress_find_unsafe(transaction);

    internal_fatal(!qp, "Attempt to update the progress of a transaction that has not been started");

    if(qp) {
        qp->updates++;

        if(all > qp->all)
            qp->all = all;
    }

    spinlock_unlock(&progress.spinlock);
}

void query_progress_done_another(uuid_t *transaction, size_t done) {
    if(!transaction)
        return;

    spinlock_lock(&progress.spinlock);
    query_progress_init_unsafe();

    QUERY_PROGRESS *qp = query_progress_find_unsafe(transaction);

    internal_fatal(!qp, "Attempt to update the progress of a transaction that has not been started");

    if(qp) {
        qp->updates++;
        qp->done += done;
    }

    spinlock_unlock(&progress.spinlock);
}

void query_progress_done(uuid_t *transaction, usec_t finished_ut, short int response_code, size_t response_size) {
    if(!transaction)
        return;

    spinlock_lock(&progress.spinlock);
    query_progress_init_unsafe();

    QUERY_PROGRESS *qp = query_progress_find_unsafe(transaction);
    if(qp) {
        qp->response_code = response_code;
        qp->finished_ut = finished_ut ? finished_ut : now_realtime_usec();
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(progress.cache.list, qp, prev, next);
        progress.cache.available++;
        qp = NULL;
    }

    if(progress.cache.available > PROGRESS_CACHE_SIZE) {
        qp = progress.cache.list;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(progress.cache.list, qp, prev, next);
        progress.cache.available--;
        query_progress_unhash_unsafe(qp);
    }

    spinlock_unlock(&progress.spinlock);

    if(qp) {
        buffer_free(qp->query);
        buffer_free(qp->payload);
        freez(qp);
    }
}
