// SPDX-License-Identifier: GPL-3.0-or-later

#include "progress.h"

#define PROGRESS_CACHE_SIZE 200

// ----------------------------------------------------------------------------
// hashtable for HASHED_KEY

// cleanup hashtable defines
#include "../simple_hashtable/simple_hashtable_undef.h"

struct query;
#define SIMPLE_HASHTABLE_VALUE_TYPE struct query *
#define SIMPLE_HASHTABLE_KEY_TYPE nd_uuid_t
#define SIMPLE_HASHTABLE_NAME _QUERY
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION query_transaction
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION query_compare_keys
#include "../simple_hashtable/simple_hashtable.h"

// ----------------------------------------------------------------------------

typedef struct query {
    nd_uuid_t transaction;

    BUFFER *query;
    BUFFER *payload;
    BUFFER *client;

    usec_t started_ut;
    usec_t finished_ut;

    HTTP_REQUEST_MODE mode;
    HTTP_ACL acl;

    uint32_t sent_size;
    uint32_t response_size;
    short response_code;

    bool indexed;

    uint32_t updates;

    usec_t duration_ut;
    size_t all;
    size_t done;

    struct query *prev, *next;
} QUERY_PROGRESS;

static inline nd_uuid_t *query_transaction(QUERY_PROGRESS *qp) {
    return qp ? &qp->transaction : NULL;
}

static inline bool query_compare_keys(nd_uuid_t *t1, nd_uuid_t *t2) {
    if(t1 == t2 || (t1 && t2 && memcmp(t1, t2, sizeof(nd_uuid_t)) == 0))
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
    .spinlock = SPINLOCK_INITIALIZER,
};

SIMPLE_HASHTABLE_HASH query_hash(nd_uuid_t *transaction) {
    return XXH3_64bits(transaction, sizeof(*transaction));
}

static void query_progress_init_unsafe(void) {
    if(!progress.initialized) {
        simple_hashtable_init_QUERY(&progress.hashtable, PROGRESS_CACHE_SIZE * 4);
        progress.initialized = true;
    }
}

// ----------------------------------------------------------------------------

static inline QUERY_PROGRESS *query_progress_find_in_hashtable_unsafe(nd_uuid_t *transaction) {
    SIMPLE_HASHTABLE_HASH hash = query_hash(transaction);
    SIMPLE_HASHTABLE_SLOT_QUERY *slot = simple_hashtable_get_slot_QUERY(&progress.hashtable, hash, transaction, true);
    QUERY_PROGRESS *qp = SIMPLE_HASHTABLE_SLOT_DATA(slot);

    assert(!qp || qp->indexed);

    return qp;
}

static inline void query_progress_add_to_hashtable_unsafe(QUERY_PROGRESS *qp) {
    assert(!qp->indexed);

    SIMPLE_HASHTABLE_HASH hash = query_hash(&qp->transaction);
    SIMPLE_HASHTABLE_SLOT_QUERY *slot =
            simple_hashtable_get_slot_QUERY(&progress.hashtable, hash, &qp->transaction, true);

    internal_fatal(SIMPLE_HASHTABLE_SLOT_DATA(slot) != NULL && SIMPLE_HASHTABLE_SLOT_DATA(slot) != qp,
                   "Attempt to overwrite a progress slot, with another value");

    simple_hashtable_set_slot_QUERY(&progress.hashtable, slot, hash, qp);

    qp->indexed = true;
}

static inline void query_progress_remove_from_hashtable_unsafe(QUERY_PROGRESS *qp) {
    assert(qp->indexed);

    SIMPLE_HASHTABLE_HASH hash = query_hash(&qp->transaction);
    SIMPLE_HASHTABLE_SLOT_QUERY *slot =
            simple_hashtable_get_slot_QUERY(&progress.hashtable, hash, &qp->transaction, true);

    if(SIMPLE_HASHTABLE_SLOT_DATA(slot) == qp)
        simple_hashtable_del_slot_QUERY(&progress.hashtable, slot);
    else
        internal_fatal(SIMPLE_HASHTABLE_SLOT_DATA(slot) != NULL,
                       "Attempt to remove from the hashtable a progress slot with a different value");

    qp->indexed = false;
}

// ----------------------------------------------------------------------------

static QUERY_PROGRESS *query_progress_alloc(nd_uuid_t *transaction) {
    QUERY_PROGRESS *qp;
    qp = callocz(1, sizeof(*qp));
    uuid_copy(qp->transaction, *transaction);
    qp->query = buffer_create(0, NULL);
    qp->payload = buffer_create(0, NULL);
    qp->client = buffer_create(0, NULL);
    return qp;
}

static void query_progress_free(QUERY_PROGRESS *qp) {
    if(!qp) return;

    buffer_free(qp->query);
    buffer_free(qp->payload);
    buffer_free(qp->client);
    freez(qp);
}

static void query_progress_cleanup_to_reuse(QUERY_PROGRESS *qp, nd_uuid_t *transaction) {
    assert(qp && qp->prev == NULL && qp->next == NULL);
    assert(!transaction || !qp->indexed);

    buffer_flush(qp->query);
    buffer_flush(qp->payload);
    buffer_flush(qp->client);
    qp->started_ut = qp->finished_ut = qp->duration_ut = 0;
    qp->all = qp->done = qp->updates = 0;
    qp->acl = 0;
    qp->next = qp->prev = NULL;
    qp->response_size = qp->sent_size = 0;
    qp->response_code = 0;

    if(transaction)
        uuid_copy(qp->transaction, *transaction);
}

static inline void query_progress_update(QUERY_PROGRESS *qp, usec_t started_ut, HTTP_REQUEST_MODE mode, HTTP_ACL acl, const char *query, BUFFER *payload, const char *client) {
    qp->mode = mode;
    qp->acl = acl;
    qp->started_ut = started_ut ? started_ut : now_realtime_usec();
    qp->finished_ut = 0;
    qp->duration_ut = 0;
    qp->response_size = 0;
    qp->sent_size = 0;
    qp->response_code = 0;

    if(query && *query && !buffer_strlen(qp->query))
        buffer_strcat(qp->query, query);

    if(payload && !buffer_strlen(qp->payload))
        buffer_copy(qp->payload, payload);

    if(client && *client && !buffer_strlen(qp->client))
        buffer_strcat(qp->client, client);
}

// ----------------------------------------------------------------------------

static inline void query_progress_link_to_cache_unsafe(QUERY_PROGRESS *qp) {
    assert(!qp->prev && !qp->next);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(progress.cache.list, qp, prev, next);
    progress.cache.available++;
}

static inline void query_progress_unlink_from_cache_unsafe(QUERY_PROGRESS *qp) {
    assert(qp->prev);
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(progress.cache.list, qp, prev, next);
    progress.cache.available--;
}

// ----------------------------------------------------------------------------
// Progress API

void query_progress_start_or_update(nd_uuid_t *transaction, usec_t started_ut, HTTP_REQUEST_MODE mode, HTTP_ACL acl, const char *query, BUFFER *payload, const char *client) {
    if(!transaction)
        return;

    spinlock_lock(&progress.spinlock);
    query_progress_init_unsafe();

    QUERY_PROGRESS *qp = query_progress_find_in_hashtable_unsafe(transaction);
    if(qp) {
        // the transaction is already there
        if(qp->prev) {
            // reusing a finished transaction
            query_progress_unlink_from_cache_unsafe(qp);
            query_progress_cleanup_to_reuse(qp, NULL);
        }
    }
    else if (progress.cache.available >= PROGRESS_CACHE_SIZE && progress.cache.list) {
        // transaction is not found - get the first available, if any.
        qp = progress.cache.list;
        query_progress_unlink_from_cache_unsafe(qp);

        query_progress_remove_from_hashtable_unsafe(qp);
        query_progress_cleanup_to_reuse(qp, transaction);
    }
    else {
        qp = query_progress_alloc(transaction);
    }

    query_progress_update(qp, started_ut, mode, acl, query, payload, client);

    if(!qp->indexed)
        query_progress_add_to_hashtable_unsafe(qp);

    spinlock_unlock(&progress.spinlock);
}

void query_progress_set_finish_line(nd_uuid_t *transaction, size_t all) {
    if(!transaction)
        return;

    spinlock_lock(&progress.spinlock);
    query_progress_init_unsafe();

    QUERY_PROGRESS *qp = query_progress_find_in_hashtable_unsafe(transaction);
    if(qp) {
        qp->updates++;

        if(all > qp->all)
            qp->all = all;
    }

    spinlock_unlock(&progress.spinlock);
}

void query_progress_done_step(nd_uuid_t *transaction, size_t done) {
    if(!transaction)
        return;

    spinlock_lock(&progress.spinlock);
    query_progress_init_unsafe();

    QUERY_PROGRESS *qp = query_progress_find_in_hashtable_unsafe(transaction);
    if(qp) {
        qp->updates++;
        qp->done += done;
    }

    spinlock_unlock(&progress.spinlock);
}

void query_progress_finished(nd_uuid_t *transaction, usec_t finished_ut, short int response_code, usec_t duration_ut, size_t response_size, size_t sent_size) {
    if(!transaction)
        return;

    spinlock_lock(&progress.spinlock);
    query_progress_init_unsafe();

    // find this transaction to update it
    {
        QUERY_PROGRESS *qp = query_progress_find_in_hashtable_unsafe(transaction);
        if(qp) {
            qp->sent_size = sent_size;
            qp->response_size = response_size;
            qp->response_code = response_code;
            qp->duration_ut = duration_ut;
            qp->finished_ut = finished_ut ? finished_ut : now_realtime_usec();

            if(qp->prev)
                query_progress_unlink_from_cache_unsafe(qp);

            query_progress_link_to_cache_unsafe(qp);
        }
    }

    // find an item to free
    {
        QUERY_PROGRESS *qp_to_free = NULL;
        if(progress.cache.available > PROGRESS_CACHE_SIZE && progress.cache.list) {
            qp_to_free = progress.cache.list;
            query_progress_unlink_from_cache_unsafe(qp_to_free);
            query_progress_remove_from_hashtable_unsafe(qp_to_free);
        }

        spinlock_unlock(&progress.spinlock);

        query_progress_free(qp_to_free);
    }
}

void query_progress_functions_update(nd_uuid_t *transaction, size_t done, size_t all) {
    // functions send to the total 'done', not the increment

    if(!transaction)
        return;

    spinlock_lock(&progress.spinlock);
    query_progress_init_unsafe();

    QUERY_PROGRESS *qp = query_progress_find_in_hashtable_unsafe(transaction);

    if(qp) {
        if(all)
            qp->all = all;

        if(done)
            qp->done = done;

        qp->updates++;
    }

    spinlock_unlock(&progress.spinlock);
}

// ----------------------------------------------------------------------------
// /api/v2/progress - to get the progress of a transaction

int web_api_v2_report_progress(nd_uuid_t *transaction, BUFFER *wb) {
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    if(!transaction) {
        buffer_json_member_add_uint64(wb, "status", 400);
        buffer_json_member_add_string(wb, "message", "No transaction given");
        buffer_json_finalize(wb);
        return 400;
    }

    spinlock_lock(&progress.spinlock);
    query_progress_init_unsafe();

    QUERY_PROGRESS *qp = query_progress_find_in_hashtable_unsafe(transaction);
    if(!qp) {
        spinlock_unlock(&progress.spinlock);
        buffer_json_member_add_uint64(wb, "status", HTTP_RESP_NOT_FOUND);
        buffer_json_member_add_string(wb, "message", "Transaction not found");
        buffer_json_finalize(wb);
        return HTTP_RESP_NOT_FOUND;
    }

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);

    buffer_json_member_add_uint64(wb, "started_ut", qp->started_ut);
    if(qp->finished_ut) {
        buffer_json_member_add_uint64(wb, "finished_ut", qp->finished_ut);
        buffer_json_member_add_double(wb, "progress", 100.0);
        buffer_json_member_add_uint64(wb, "age_ut", qp->finished_ut - qp->started_ut);
    }
    else {
        usec_t now_ut = now_realtime_usec();
        buffer_json_member_add_uint64(wb, "now_ut", now_ut);
        buffer_json_member_add_uint64(wb, "age_ut", now_ut - qp->started_ut);

        if   (qp->all)
            buffer_json_member_add_double(wb, "progress", (double) qp->done * 100.0 / (double) qp->all);
        else
            buffer_json_member_add_uint64(wb, "working", qp->done);
    }

    buffer_json_finalize(wb);

    spinlock_unlock(&progress.spinlock);

    return 200;
}

// ----------------------------------------------------------------------------
// function to show the progress of all current queries
// and the recent few completed queries

int progress_function_result(BUFFER *wb, const char *hostname) {
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", hostname);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_PROGRESS_HELP);
    buffer_json_member_add_array(wb, "data");

    spinlock_lock(&progress.spinlock);
    query_progress_init_unsafe();

    usec_t now_ut = now_realtime_usec();
    usec_t max_duration_ut = 0;
    size_t max_size = 0, max_sent = 0;
    size_t archived = 0, running = 0;
    SIMPLE_HASHTABLE_FOREACH_READ_ONLY(&progress.hashtable, sl, _QUERY) {
        QUERY_PROGRESS *qp = SIMPLE_HASHTABLE_FOREACH_READ_ONLY_VALUE(sl);
        if(unlikely(!qp)) continue; // not really needed, just for completeness

        if(qp->prev)
            archived++;
        else
            running++;

        bool finished = qp->finished_ut ? true : false;
        usec_t duration_ut = finished ? qp->duration_ut : now_ut - qp->started_ut;
        if(duration_ut > max_duration_ut)
            max_duration_ut = duration_ut;

        if(finished) {
            if(qp->response_size > max_size)
                max_size = qp->response_size;

            if(qp->sent_size > max_sent)
                max_sent = qp->sent_size;
        }

        buffer_json_add_array_item_array(wb); // row

        buffer_json_add_array_item_uuid_compact(wb, &qp->transaction);
        buffer_json_add_array_item_uint64(wb, qp->started_ut);
        buffer_json_add_array_item_string(wb, HTTP_REQUEST_MODE_2str(qp->mode));
        buffer_json_add_array_item_string(wb, buffer_tostring(qp->query));

        if(!buffer_strlen(qp->client)) {
            if(qp->acl & HTTP_ACL_ACLK)
                buffer_json_add_array_item_string(wb, "ACLK");
            else if(qp->acl & HTTP_ACL_WEBRTC)
                buffer_json_add_array_item_string(wb, "WEBRTC");
            else
                buffer_json_add_array_item_string(wb, "unknown");
        }
        else
            buffer_json_add_array_item_string(wb, buffer_tostring(qp->client));

        if(finished) {
            buffer_json_add_array_item_string(wb, "finished");
            buffer_json_add_array_item_string(wb, "100.00 %%");
        }
        else {
            char buf[50];

            buffer_json_add_array_item_string(wb, "in-progress");

            if (qp->all)
                snprintfz(buf, sizeof(buf), "%0.2f %%", (double) qp->done * 100.0 / (double) qp->all);
            else
                snprintfz(buf, sizeof(buf), "%zu", qp->done);

            buffer_json_add_array_item_string(wb, buf);
        }

        buffer_json_add_array_item_double(wb, (double)duration_ut / USEC_PER_MS);

        if(finished) {
            buffer_json_add_array_item_uint64(wb, qp->response_code);
            buffer_json_add_array_item_uint64(wb, qp->response_size);
            buffer_json_add_array_item_uint64(wb, qp->sent_size);
        }
        else {
            buffer_json_add_array_item_string(wb, NULL);
            buffer_json_add_array_item_string(wb, NULL);
            buffer_json_add_array_item_string(wb, NULL);
        }

        buffer_json_add_array_item_object(wb); // row options
        {
            char *severity = "notice";
            if(finished) {
                if(qp->response_code == HTTP_RESP_NOT_MODIFIED ||
                    qp->response_code == HTTP_RESP_CLIENT_CLOSED_REQUEST ||
                    qp->response_code == HTTP_RESP_CONFLICT)
                    severity = "debug";
                else if(qp->response_code >= 500 && qp->response_code <= 599)
                    severity = "error";
                else if(qp->response_code >= 400 && qp->response_code <= 499)
                    severity = "warning";
                else if(qp->response_code >= 300 && qp->response_code <= 399)
                    severity = "notice";
                else
                    severity = "normal";
            }
            buffer_json_member_add_string(wb, "severity", severity);
        }
        buffer_json_object_close(wb); // row options

        buffer_json_array_close(wb); // row
    }

    assert(archived == progress.cache.available);

    spinlock_unlock(&progress.spinlock);

    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        // transaction
        buffer_rrdf_table_add_field(wb, field_id++, "Transaction", "Transaction ID",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                                    NULL);

        // timestamp
        buffer_rrdf_table_add_field(wb, field_id++, "Started", "Query Start Timestamp",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_USEC,
                                    0, NULL, NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // request method
        buffer_rrdf_table_add_field(wb, field_id++, "Method", "Request Method",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // query
        buffer_rrdf_table_add_field(wb, field_id++, "Query", "Query",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_FULL_WIDTH | RRDF_FIELD_OPTS_WRAP, NULL);

        // client
        buffer_rrdf_table_add_field(wb, field_id++, "Client", "Client",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // status
        buffer_rrdf_table_add_field(wb, field_id++, "Status", "Query Status",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // progress
        buffer_rrdf_table_add_field(wb, field_id++, "Progress", "Query Progress",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // duration
        buffer_rrdf_table_add_field(wb, field_id++, "Duration", "Query Duration",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "ms", (double)max_duration_ut / USEC_PER_MS, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // response code
        buffer_rrdf_table_add_field(wb, field_id++, "Response", "Query Response Code",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // response size
        buffer_rrdf_table_add_field(wb, field_id++, "Size", "Query Response Size",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, "bytes", (double)max_size, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // sent size
        buffer_rrdf_table_add_field(wb, field_id++, "Sent", "Query Response Final Size",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, "bytes", (double)max_sent, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // row options
        buffer_rrdf_table_add_field(wb, field_id++, "rowOptions", "rowOptions",
                                    RRDF_FIELD_TYPE_NONE, RRDR_FIELD_VISUAL_ROW_OPTIONS, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_FIXED, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_DUMMY, NULL);
    }

    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "Started");

    buffer_json_member_add_time_t(wb, "expires", (time_t)((now_ut / USEC_PER_SEC) + 1));
    buffer_json_finalize(wb);

    return 200;
}


// ----------------------------------------------------------------------------

int progress_unittest(void) {
    size_t permanent = 100;
    nd_uuid_t valid[permanent];

    usec_t started = now_monotonic_usec();

    for(size_t i = 0; i < permanent ;i++) {
        uuid_generate_random(valid[i]);
        query_progress_start_or_update(&valid[i], 0, HTTP_REQUEST_MODE_GET, HTTP_ACL_ACLK, "permanent", NULL, "test");
    }

    for(size_t n = 0; n < 5000000 ;n++) {
        nd_uuid_t t;
        uuid_generate_random(t);
        query_progress_start_or_update(&t, 0, HTTP_REQUEST_MODE_OPTIONS, HTTP_ACL_WEBRTC, "ephemeral", NULL, "test");
        query_progress_finished(&t, 0, 200, 1234, 123, 12);

        QUERY_PROGRESS *qp;
        for(size_t i = 0; i < permanent ;i++) {
            qp = query_progress_find_in_hashtable_unsafe(&valid[i]);
            assert(qp);
            (void)qp;
        }
    }

    usec_t ended = now_monotonic_usec();
    usec_t duration = ended - started;

    printf("progress hashtable resizes: %zu, size: %zu, used: %zu, deleted: %zu, searches: %zu, collisions: %zu, additions: %zu, deletions: %zu\n",
           progress.hashtable.resizes,
           progress.hashtable.size, progress.hashtable.used, progress.hashtable.deleted,
           progress.hashtable.searches, progress.hashtable.collisions, progress.hashtable.additions, progress.hashtable.deletions);

    double d = (double)duration / USEC_PER_SEC;
    printf("hashtable ops: %0.2f / sec\n", (double)progress.hashtable.searches / d);

    return 0;
}
