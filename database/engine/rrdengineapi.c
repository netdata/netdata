// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

/* Default global database instance */
struct rrdengine_instance multidb_ctx_storage_tier0;
struct rrdengine_instance multidb_ctx_storage_tier1;
struct rrdengine_instance multidb_ctx_storage_tier2;
struct rrdengine_instance multidb_ctx_storage_tier3;
struct rrdengine_instance multidb_ctx_storage_tier4;

#define mrg_metric_ctx(metric) (struct rrdengine_instance *)mrg_metric_section(main_mrg, metric)


#if RRD_STORAGE_TIERS != 5
#error RRD_STORAGE_TIERS is not 5 - you need to add allocations here
#endif
struct rrdengine_instance *multidb_ctx[RRD_STORAGE_TIERS];
uint8_t tier_page_type[RRD_STORAGE_TIERS] = {PAGE_METRICS, PAGE_TIER, PAGE_TIER, PAGE_TIER, PAGE_TIER};

#if PAGE_TYPE_MAX != 1
#error PAGE_TYPE_MAX is not 1 - you need to add allocations here
#endif
size_t page_type_size[256] = {sizeof(storage_number), sizeof(storage_number_tier1_t)};

__attribute__((constructor)) void initialize_multidb_ctx(void) {
    multidb_ctx[0] = &multidb_ctx_storage_tier0;
    multidb_ctx[1] = &multidb_ctx_storage_tier1;
    multidb_ctx[2] = &multidb_ctx_storage_tier2;
    multidb_ctx[3] = &multidb_ctx_storage_tier3;
    multidb_ctx[4] = &multidb_ctx_storage_tier4;
}

int db_engine_use_malloc = 0;
int default_rrdeng_page_fetch_timeout = 3;
int default_rrdeng_page_fetch_retries = 3;
int default_rrdeng_page_cache_mb = 32;
int db_engine_journal_indexing = 1;
int db_engine_journal_check = 0;
int default_rrdeng_disk_quota_mb = 256;
int default_multidb_disk_quota_mb = 256;

// ----------------------------------------------------------------------------
// metrics groups

static inline void rrdeng_page_alignment_acquire(struct pg_alignment *pa) {
    if(unlikely(!pa)) return;
    __atomic_add_fetch(&pa->refcount, 1, __ATOMIC_SEQ_CST);
}

static inline bool rrdeng_page_alignment_release(struct pg_alignment *pa) {
    if(unlikely(!pa)) return true;

    if(__atomic_sub_fetch(&pa->refcount, 1, __ATOMIC_SEQ_CST) == 0) {
        freez(pa);
        return true;
    }

    return false;
}

// charts call this
STORAGE_METRICS_GROUP *rrdeng_metrics_group_get(STORAGE_INSTANCE *db_instance __maybe_unused, uuid_t *uuid __maybe_unused) {
    struct pg_alignment *pa = callocz(1, sizeof(struct pg_alignment));
    rrdeng_page_alignment_acquire(pa);
    return (STORAGE_METRICS_GROUP *)pa;
}

// charts call this
void rrdeng_metrics_group_release(STORAGE_INSTANCE *db_instance __maybe_unused, STORAGE_METRICS_GROUP *smg) {
    if(unlikely(!smg)) return;

    struct pg_alignment *pa = (struct pg_alignment *)smg;
    rrdeng_page_alignment_release(pa);
}

// ----------------------------------------------------------------------------
// metric handle for legacy dbs

/* This UUID is not unique across hosts */
void rrdeng_generate_legacy_uuid(const char *dim_id, const char *chart_id, uuid_t *ret_uuid)
{
    EVP_MD_CTX *evpctx;
    unsigned char hash_value[EVP_MAX_MD_SIZE];
    unsigned int hash_len;

    evpctx = EVP_MD_CTX_create();
    EVP_DigestInit_ex(evpctx, EVP_sha256(), NULL);
    EVP_DigestUpdate(evpctx, dim_id, strlen(dim_id));
    EVP_DigestUpdate(evpctx, chart_id, strlen(chart_id));
    EVP_DigestFinal_ex(evpctx, hash_value, &hash_len);
    EVP_MD_CTX_destroy(evpctx);
    fatal_assert(hash_len > sizeof(uuid_t));
    memcpy(ret_uuid, hash_value, sizeof(uuid_t));
}

/* Transform legacy UUID to be unique across hosts deterministically */
void rrdeng_convert_legacy_uuid_to_multihost(char machine_guid[GUID_LEN + 1], uuid_t *legacy_uuid, uuid_t *ret_uuid)
{
    EVP_MD_CTX *evpctx;
    unsigned char hash_value[EVP_MAX_MD_SIZE];
    unsigned int hash_len;

    evpctx = EVP_MD_CTX_create();
    EVP_DigestInit_ex(evpctx, EVP_sha256(), NULL);
    EVP_DigestUpdate(evpctx, machine_guid, GUID_LEN);
    EVP_DigestUpdate(evpctx, *legacy_uuid, sizeof(uuid_t));
    EVP_DigestFinal_ex(evpctx, hash_value, &hash_len);
    EVP_MD_CTX_destroy(evpctx);
    fatal_assert(hash_len > sizeof(uuid_t));
    memcpy(ret_uuid, hash_value, sizeof(uuid_t));
}

static METRIC *rrdeng_metric_get_legacy(STORAGE_INSTANCE *db_instance, const char *rd_id, const char *st_id) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)db_instance;
    uuid_t legacy_uuid;
    rrdeng_generate_legacy_uuid(rd_id, st_id, &legacy_uuid);
    return mrg_metric_get_and_acquire(main_mrg, &legacy_uuid, (Word_t) ctx);
}

// ----------------------------------------------------------------------------
// metric handle

void rrdeng_metric_release(STORAGE_METRIC_HANDLE *db_metric_handle) {
    METRIC *metric = (METRIC *)db_metric_handle;
    mrg_metric_release(main_mrg, metric);
}

STORAGE_METRIC_HANDLE *rrdeng_metric_dup(STORAGE_METRIC_HANDLE *db_metric_handle) {
    METRIC *metric = (METRIC *)db_metric_handle;
    return (STORAGE_METRIC_HANDLE *) mrg_metric_dup(main_mrg, metric);
}

STORAGE_METRIC_HANDLE *rrdeng_metric_get(STORAGE_INSTANCE *db_instance, uuid_t *uuid) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)db_instance;
    return (STORAGE_METRIC_HANDLE *) mrg_metric_get_and_acquire(main_mrg, uuid, (Word_t) ctx);
}

static METRIC *rrdeng_metric_create(STORAGE_INSTANCE *db_instance, uuid_t *uuid) {
    internal_fatal(!db_instance, "DBENGINE: db_instance is NULL");

    struct rrdengine_instance *ctx = (struct rrdengine_instance *)db_instance;
    MRG_ENTRY entry = {
            .section = (Word_t)ctx,
            .first_time_t = 0,
            .latest_time_t = 0,
            .latest_update_every = 0,
    };
    uuid_copy(entry.uuid, *uuid);

    METRIC *metric = mrg_metric_add_and_acquire(main_mrg, entry, NULL);
    return metric;
}

STORAGE_METRIC_HANDLE *rrdeng_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *db_instance) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)db_instance;
    METRIC *metric;

    metric = mrg_metric_get_and_acquire(main_mrg, &rd->metric_uuid, (Word_t) ctx);
    if(!metric) {
        metric = rrdeng_metric_get_legacy(db_instance, rrddim_id(rd), rrdset_id(rd->rrdset));
        if(metric)
            uuid_copy(rd->metric_uuid, *mrg_metric_uuid(main_mrg, metric));
    }

    if(!metric)
        metric = rrdeng_metric_create(db_instance, &rd->metric_uuid);

#ifdef NETDATA_INTERNAL_CHECKS
    if(uuid_compare(rd->metric_uuid, *mrg_metric_uuid(main_mrg, metric)) != 0) {
        char uuid1[UUID_STR_LEN + 1];
        char uuid2[UUID_STR_LEN + 1];

        uuid_unparse(rd->metric_uuid, uuid1);
        uuid_unparse(*mrg_metric_uuid(main_mrg, metric), uuid2);
        fatal("DBENGINE: uuids do not match, asked for metric '%s', but got metric '%s'", uuid1, uuid2);
    }

    if(mrg_metric_ctx(metric) != ctx)
        fatal("DBENGINE: mixed up db instances, asked for metric from %p, got from %p",
              ctx, mrg_metric_ctx(metric));
#endif

    return (STORAGE_METRIC_HANDLE *)metric;
}


// ----------------------------------------------------------------------------
// collect ops

/*
 * Gets a handle for storing metrics to the database.
 * The handle must be released with rrdeng_store_metric_final().
 */
STORAGE_COLLECT_HANDLE *rrdeng_store_metric_init(STORAGE_METRIC_HANDLE *db_metric_handle, uint32_t update_every, STORAGE_METRICS_GROUP *smg) {
    METRIC *metric = mrg_metric_dup(main_mrg, (METRIC *)db_metric_handle);
    struct rrdeng_collect_handle *handle;

    handle = callocz(1, sizeof(struct rrdeng_collect_handle));
    handle->metric = metric;
    handle->page = NULL;

    mrg_metric_set_update_every(main_mrg, metric, update_every);

    if(!mrg_metric_get_first_time_t(main_mrg, metric))
        handle->options |= RRDENG_CHO_SET_FIRST_TIME_T;

    handle->alignment = (struct pg_alignment *)smg;
    rrdeng_page_alignment_acquire(handle->alignment);

    return (STORAGE_COLLECT_HANDLE *)handle;
}

/* The page must be populated and referenced */
static bool page_has_only_empty_metrics(struct rrdeng_collect_handle *handle) {
    switch(handle->type) {
        case PAGE_METRICS: {
            size_t slots = handle->page_length / PAGE_POINT_CTX_SIZE_BYTES(mrg_metric_ctx(handle->metric));
            storage_number *array = (storage_number *)pgc_page_data(handle->page);
            for (size_t i = 0 ; i < slots; ++i) {
                if(does_storage_number_exist(array[i]))
                    return false;
            }
        }
        break;

        case PAGE_TIER: {
            size_t slots = handle->page_length / PAGE_POINT_CTX_SIZE_BYTES(mrg_metric_ctx(handle->metric));
            storage_number_tier1_t *array = (storage_number_tier1_t *)pgc_page_data(handle->page);
            for (size_t i = 0 ; i < slots; ++i) {
                if(fpclassify(array[i].sum_value) != FP_NAN)
                    return false;
            }
        }
        break;

        default: {
            static bool logged = false;
            if(!logged) {
                error("DBENGINE: cannot check page for nulls on unknown page type id %d", (mrg_metric_ctx(handle->metric))->page_type);
                logged = true;
            }
            return false;
        }
    }

    return true;
}

void rrdeng_store_metric_flush_current_page(STORAGE_COLLECT_HANDLE *collection_handle) {
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)collection_handle;
    struct rrdengine_instance *ctx = mrg_metric_ctx(handle->metric);

    if (unlikely(!ctx || !handle->page)) return;

    if (likely(handle->page_length)) {
        int page_is_empty;

        page_is_empty = page_has_only_empty_metrics(handle);
        if (page_is_empty) {
            size_t points = handle->page_length / PAGE_POINT_CTX_SIZE_BYTES(ctx);
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "%s: Deleting page with %lu empty points", __func__, points);

            mrg_metric_set_hot_latest_time_t(main_mrg, handle->metric, 0);
            pgc_page_hot_to_clean_empty_and_release(main_cache, handle->page);
        }
        else {
            mrg_metric_set_latest_time_t(main_mrg, handle->metric, pgc_page_end_time_t(handle->page));
            pgc_page_hot_to_dirty_and_release(main_cache, handle->page);
            mrg_metric_set_hot_latest_time_t(main_mrg, handle->metric, 0);
        }
    }
    else {
        mrg_metric_set_hot_latest_time_t(main_mrg, handle->metric, 0);
        pgc_page_hot_to_clean_empty_and_release(main_cache, handle->page);
    }

    handle->page = NULL;
}

static PGC_PAGE *rrdeng_create_new_hot_page(struct rrdengine_instance *ctx, METRIC *metric, time_t point_in_time_s, time_t update_every_s) {
    PGC_ENTRY page_entry = {
            .section = (Word_t) ctx,
            .metric_id = mrg_metric_id(main_mrg, metric),
            .start_time_t = point_in_time_s,
            .end_time_t = point_in_time_s,
            .size = RRDENG_BLOCK_SIZE,
            .data = dbengine_page_alloc(),
            .update_every = update_every_s,
            .hot = true
    };

    bool added = true;
    PGC_PAGE *page = pgc_page_add_and_acquire(main_cache, page_entry, &added);
    if (false == added) {
        dbengine_page_free(page_entry.data);

        // FIXME - what we should do if the page returned is not hot?
        // FIXME - what we should do if the page returned is also written by another collector?
    }
    else {
        mrg_metric_set_hot_latest_time_t(main_mrg, metric, point_in_time_s);
    }

    return page;
}

static void rrdeng_store_metric_next_internal(STORAGE_COLLECT_HANDLE *collection_handle,
                              usec_t point_in_time_ut,
                              NETDATA_DOUBLE n,
                              NETDATA_DOUBLE min_value,
                              NETDATA_DOUBLE max_value,
                              uint16_t count,
                              uint16_t anomaly_count,
                              SN_FLAGS flags)
{
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)collection_handle;
    struct rrdengine_instance *ctx = mrg_metric_ctx(handle->metric);

    bool must_flush_unaligned_page = false, perfect_page_alignment = false;

    if(handle->page) {
        /* Make alignment decisions */
        if (handle->page_length == handle->alignment->page_length) {
            /* this is the leading dimension that defines chart alignment */
            perfect_page_alignment = true;
        }

        /* is the metric far enough out of alignment with the others? */
        if (unlikely(handle->page_length + PAGE_POINT_CTX_SIZE_BYTES(ctx) < handle->alignment->page_length))
            handle->options |= RRDENG_CHO_UNALIGNED;

        if (unlikely((handle->options & RRDENG_CHO_UNALIGNED) &&
                     /* did the other metrics change page? */
                     handle->alignment->page_length <= PAGE_POINT_CTX_SIZE_BYTES(ctx))) {
            must_flush_unaligned_page = true;
            handle->options &= ~RRDENG_CHO_UNALIGNED;
        }
    }

    if (unlikely(!handle->page ||
                 handle->page_length + PAGE_POINT_CTX_SIZE_BYTES(ctx) > RRDENG_BLOCK_SIZE ||
                 must_flush_unaligned_page)) {

        if(handle->page)
            rrdeng_store_metric_flush_current_page(collection_handle);

        if(handle->options & RRDENG_CHO_SET_FIRST_TIME_T) {
            handle->options &= ~RRDENG_CHO_SET_FIRST_TIME_T;
            mrg_metric_set_first_time_t(main_mrg, handle->metric, (time_t)(point_in_time_ut / USEC_PER_SEC));
        }

        handle->page = rrdeng_create_new_hot_page(ctx, handle->metric, (time_t)(point_in_time_ut / USEC_PER_SEC),
                                                  mrg_metric_get_update_every(main_mrg, handle->metric));
        handle->start_time_ut = point_in_time_ut;
        handle->page_length = 0;

        if (0 == handle->alignment->page_length) {
            /* this is the leading dimension that defines chart alignment */
            perfect_page_alignment = true;
        }
    }

    switch (ctx->page_type) {
        case PAGE_METRICS: {
            storage_number *tier0_metric_data = pgc_page_data(handle->page);
            tier0_metric_data[handle->page_length / PAGE_POINT_CTX_SIZE_BYTES(ctx)] = pack_storage_number(n, flags);
        }
        break;

        case PAGE_TIER: {
            storage_number_tier1_t *tier12_metric_data = pgc_page_data(handle->page);
            storage_number_tier1_t number_tier1;
            number_tier1.sum_value = (float)n;
            number_tier1.min_value = (float)min_value;
            number_tier1.max_value = (float)max_value;
            number_tier1.anomaly_count = anomaly_count;
            number_tier1.count = count;
            tier12_metric_data[handle->page_length / PAGE_POINT_CTX_SIZE_BYTES(ctx)] = number_tier1;
        }
        break;

        default: {
            static bool logged = false;
            if(!logged) {
                error("DBENGINE: cannot store metric on unknown page type id %d", ctx->page_type);
                logged = true;
            }
        }
        break;
    }

    handle->page_length += PAGE_POINT_CTX_SIZE_BYTES(ctx);
    handle->end_time_ut = point_in_time_ut;

    pgc_page_hot_set_end_time_t(main_cache, handle->page, (time_t) (point_in_time_ut / USEC_PER_SEC));

    if (perfect_page_alignment)
        handle->alignment->page_length = handle->page_length;

    mrg_metric_set_hot_latest_time_t(main_mrg, handle->metric, (time_t) (point_in_time_ut / USEC_PER_SEC));
}

void rrdeng_store_metric_next(STORAGE_COLLECT_HANDLE *collection_handle,
                              usec_t point_in_time_ut,
                              NETDATA_DOUBLE n,
                              NETDATA_DOUBLE min_value,
                              NETDATA_DOUBLE max_value,
                              uint16_t count,
                              uint16_t anomaly_count,
                              SN_FLAGS flags)
{
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)collection_handle;
    usec_t update_every_ut = mrg_metric_get_update_every(main_mrg, handle->metric) * USEC_PER_SEC;

    if(likely(handle->page)) {
        usec_t last_point_in_time_ut = handle->end_time_ut;
        size_t points_gap = (point_in_time_ut <= last_point_in_time_ut) ?
                                (size_t)0 :
                                (size_t)((point_in_time_ut - last_point_in_time_ut) / update_every_ut);

        if(unlikely(points_gap != 1)) {
            if (unlikely(points_gap <= 0)) {
                time_t now = now_realtime_sec();
                static __thread size_t counter = 0;
                static __thread time_t last_time_logged = 0;
                counter++;

                if(now - last_time_logged > 600) {
                    error("DBENGINE: collected point is in the past (repeated %zu times in the last %zu secs). Ignoring these data collection points.",
                          counter, (size_t)(last_time_logged?(now - last_time_logged):0));

                    last_time_logged = now;
                    counter = 0;
                }
                return;
            }

            size_t point_size = PAGE_POINT_CTX_SIZE_BYTES(mrg_metric_ctx(handle->metric));
            size_t page_size_in_points = RRDENG_BLOCK_SIZE / point_size;
            size_t used_points = handle->page_length / point_size;
            size_t remaining_points_in_page = page_size_in_points - used_points;

            bool new_point_is_aligned = true;
            if(unlikely((point_in_time_ut - last_point_in_time_ut) / points_gap != update_every_ut))
                new_point_is_aligned = false;

            if(unlikely(points_gap > remaining_points_in_page || !new_point_is_aligned)) {
                rrdeng_store_metric_flush_current_page(collection_handle);
            }
            else {
                // loop to fill the gap
                usec_t step_ut = update_every_ut;
                usec_t last_point_filled_ut = last_point_in_time_ut + step_ut;

                while (last_point_filled_ut < point_in_time_ut) {
                    rrdeng_store_metric_next_internal(
                        collection_handle, last_point_filled_ut, NAN, NAN, NAN,
                        1, 0, SN_EMPTY_SLOT);

                    last_point_filled_ut += step_ut;
                }
            }
        }
    }

    rrdeng_store_metric_next_internal(collection_handle, point_in_time_ut, n, min_value, max_value, count, anomaly_count, flags);
}


/*
 * Releases the database reference from the handle for storing metrics.
 * Returns 1 if it's safe to delete the dimension.
 */
int rrdeng_store_metric_finalize(STORAGE_COLLECT_HANDLE *collection_handle) {
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)collection_handle;

    rrdeng_store_metric_flush_current_page(collection_handle);
    rrdeng_page_alignment_release(handle->alignment);

    METRIC *metric = handle->metric;
    mrg_metric_release(main_mrg, metric);

    PGC_PAGE *page = handle->page;
    if(page)
        pgc_page_hot_to_dirty_and_release(main_cache, page);

    freez(handle);

    return 0;
}

void rrdeng_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *collection_handle, int update_every) {
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)collection_handle;
    METRIC *metric = handle->metric;
    rrdeng_store_metric_flush_current_page(collection_handle);
    mrg_metric_set_update_every(main_mrg, metric, update_every);
}

// ----------------------------------------------------------------------------
// query ops

/*
 * Gets a handle for loading metrics from the database.
 * The handle must be released with rrdeng_load_metric_final().
 */
void rrdeng_load_metric_init(STORAGE_METRIC_HANDLE *db_metric_handle, struct storage_engine_query_handle *rrdimm_handle, time_t start_time_s, time_t end_time_s)
{
    METRIC *metric = (METRIC *)db_metric_handle;
    struct rrdengine_instance *ctx = mrg_metric_ctx(metric);

    struct rrdeng_query_handle *handle;
    unsigned pages_nr;

    mrg_metric_set_update_every_if_zero(main_mrg, metric, default_rrd_update_every);

    rrdimm_handle->start_time_s = start_time_s;
    rrdimm_handle->end_time_s = end_time_s;

    handle = callocz(1, sizeof(struct rrdeng_query_handle));
    handle->metric = metric;
    handle->wanted_start_time_s = start_time_s;
    handle->now_s = start_time_s;
    handle->position = 0;
    handle->ctx = ctx;
    handle->page = NULL;
    handle->dt_s = mrg_metric_get_update_every(main_mrg, metric);
    rrdimm_handle->handle = (STORAGE_QUERY_HANDLE *)handle;
    if (unlikely(!pg_cache_preload(ctx, handle, start_time_s, end_time_s)))
        // there are no metrics to load
        handle->wanted_start_time_s = INVALID_TIME;
}

static bool rrdeng_load_page_next(struct storage_engine_query_handle *rrdimm_handle, bool debug_this __maybe_unused) {
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrdimm_handle->handle;

    struct rrdengine_instance *ctx = handle->ctx;

    time_t page_end_time_t;
    time_t page_start_time_t;
    time_t update_every_s;
    unsigned position;

    if (likely(handle->page)) {
        handle->wanted_start_time_s = (time_t)(pgc_page_end_time_t(handle->page) + 1 /* handle->dt_s */);

        pgc_page_release(main_cache, (PGC_PAGE *)handle->page);
        handle->page = NULL;

        if (unlikely(handle->wanted_start_time_s > rrdimm_handle->end_time_s))
            return false;
    }

    if(handle->wanted_start_time_s == INVALID_TIME)
        return false;

    time_t wanted_start_time_t = handle->wanted_start_time_s;
    handle->page = pg_cache_lookup_next(ctx, handle, wanted_start_time_t, rrdimm_handle->end_time_s);

    if (!handle->page)
        return false;

    page_start_time_t = pgc_page_start_time_t((PGC_PAGE *)handle->page);
    page_end_time_t = pgc_page_end_time_t((PGC_PAGE *)handle->page);
    update_every_s = pgc_page_update_every((PGC_PAGE *)handle->page);

    // FIXME: Check atomic requirements
    //    pg_cache_atomic_get_pg_info(handle, &page_end_time_t, &page_length);
    if (unlikely(INVALID_TIME == page_start_time_t || INVALID_TIME == page_end_time_t || 0 == update_every_s)) {
        error("DBENGINE: discarding invalid page (start_time = %ld, end_time = %ld, update_every_s = %ld)",
              page_start_time_t, page_end_time_t, update_every_s);
        return false;
    }

    internal_fatal(page_start_time_t > page_end_time_t,
                   "DBENGINE: page has bigger start time than end time");

    unsigned entries = (page_end_time_t - (page_start_time_t - update_every_s)) / update_every_s;

    internal_fatal(entries > pgc_page_data_size(handle->page) / PAGE_POINT_CTX_SIZE_BYTES(ctx),
                   "DBENGINE: page has more points than its size");

    if (unlikely(page_start_time_t != page_end_time_t && wanted_start_time_t > page_start_time_t)) {
        // we're in the middle of the page somewhere
        position = ((uint64_t)(wanted_start_time_t - page_start_time_t)) * (entries - 1) /
                   (page_end_time_t - page_start_time_t);
    }
    else
        position = 0;

    handle->entries = entries;
    handle->metric_data = pgc_page_data((PGC_PAGE *)handle->page);
    handle->dt_s = update_every_s;
    handle->position = position;
    return true;
}

// Returns the metric and sets its timestamp into current_time
// IT IS REQUIRED TO **ALWAYS** SET ALL RETURN VALUES (current_time, end_time, flags)
// IT IS REQUIRED TO **ALWAYS** KEEP TRACK OF TIME, EVEN OUTSIDE THE DATABASE BOUNDARIES
STORAGE_POINT rrdeng_load_metric_next(struct storage_engine_query_handle *rrddim_handle) {

    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrddim_handle->handle;
    time_t now = handle->now_s + handle->dt_s;

    STORAGE_POINT sp;
    unsigned position = handle->position + 1;
    storage_number_tier1_t tier1_value;

    if (unlikely(INVALID_TIME == handle->wanted_start_time_s)) {
        handle->wanted_start_time_s = INVALID_TIME;
        handle->now_s = now;
        storage_point_empty(sp, now - handle->dt_s, now);
        return sp;
    }

    if (unlikely(!handle->page || position >= handle->entries)) {
        // We need to get a new page
        if(!rrdeng_load_page_next(rrddim_handle, false)) {
            // next calls will not load any more metrics
            handle->wanted_start_time_s = INVALID_TIME;
            handle->now_s = now;
            storage_point_empty(sp, now - handle->dt_s, now);
            return sp;
        }

        position = handle->position;
        time_t start_time_t = pgc_page_start_time_t(handle->page);

        now = (time_t)(start_time_t + position * pgc_page_update_every(handle->page));
    }

    sp.start_time = now - handle->dt_s;
    sp.end_time = now;

    handle->position = position;
    handle->now_s = now;

    switch(handle->ctx->page_type) {
        case PAGE_METRICS: {
            storage_number n = handle->metric_data[position];
            sp.min = sp.max = sp.sum = unpack_storage_number(n);
            sp.flags = n & SN_USER_FLAGS;
            sp.count = 1;
            sp.anomaly_count = is_storage_number_anomalous(n) ? 1 : 0;
        }
        break;

        case PAGE_TIER: {
            tier1_value = ((storage_number_tier1_t *)handle->metric_data)[position];
            sp.flags = tier1_value.anomaly_count ? SN_FLAG_NONE : SN_FLAG_NOT_ANOMALOUS;
            sp.count = tier1_value.count;
            sp.anomaly_count = tier1_value.anomaly_count;
            sp.min = tier1_value.min_value;
            sp.max = tier1_value.max_value;
            sp.sum = tier1_value.sum_value;
        }
        break;

        // we don't know this page type
        default: {
            static bool logged = false;
            if(!logged) {
                error("DBENGINE: unknown page type %d found. Cannot decode it. Ignoring its metrics.", handle->ctx->page_type);
                logged = true;
            }
            storage_point_empty(sp, sp.start_time, sp.end_time);
        }
        break;
    }

    if (unlikely(now >= rrddim_handle->end_time_s)) {
        // next calls will not load any more metrics
        handle->wanted_start_time_s = INVALID_TIME;
    }

    return sp;
}

int rrdeng_load_metric_is_finished(struct storage_engine_query_handle *rrdimm_handle)
{
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrdimm_handle->handle;
    return (INVALID_TIME == handle->wanted_start_time_s);
}

/*
 * Releases the database reference from the handle for loading metrics.
 */
void rrdeng_load_metric_finalize(struct storage_engine_query_handle *rrdimm_handle)
{
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrdimm_handle->handle;

    if (handle->page)
        pgc_page_release(main_cache, (PGC_PAGE *)handle->page);

    freez(handle);
    rrdimm_handle->handle = NULL;
}

// FIXME: Get it from metric registry
time_t rrdeng_metric_latest_time(STORAGE_METRIC_HANDLE *db_metric_handle) {
    METRIC *metric = (METRIC *)db_metric_handle;
    time_t latest_time_t = 0;

    if (metric)
        latest_time_t = mrg_metric_get_latest_time_t(main_mrg, metric);

    return latest_time_t;
}

// FIXME: Get it from metric registry
time_t rrdeng_metric_oldest_time(STORAGE_METRIC_HANDLE *db_metric_handle) {
    METRIC *metric = (METRIC *)db_metric_handle;

    time_t oldest_time_t = 0;
    if (metric)
        oldest_time_t = mrg_metric_get_first_time_t(main_mrg, metric);

    return oldest_time_t;
}

int rrdeng_metric_retention_by_uuid(STORAGE_INSTANCE *si, uuid_t *dim_uuid, time_t *first_entry_t, time_t *last_entry_t)
{
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    if (unlikely(!ctx)) {
        error("DBENGINE: invalid STORAGE INSTANCE to %s()", __FUNCTION__);
        return 1;
    }

    METRIC *one_metric = mrg_metric_get_and_acquire(main_mrg, dim_uuid, (Word_t) ctx);
    if (unlikely(!one_metric))
        return 1;

    *first_entry_t = mrg_metric_get_first_time_t(main_mrg, one_metric);
    *last_entry_t = mrg_metric_get_latest_time_t(main_mrg, one_metric);
    return 0;
}

/*
 * Gathers Database Engine statistics.
 * Careful when modifying this function.
 * You must not change the indices of the statistics or user code will break.
 * You must not exceed RRDENG_NR_STATS or it will crash.
 */
void rrdeng_get_37_statistics(struct rrdengine_instance *ctx, unsigned long long *array)
{
    if (ctx == NULL)
        return;

    struct page_cache *pg_cache = &ctx->pg_cache;

    array[0] = (uint64_t)ctx->stats.metric_API_producers;
    array[1] = (uint64_t)ctx->stats.metric_API_consumers;
    array[2] = (uint64_t)pg_cache->page_descriptors;
    array[3] = (uint64_t)pg_cache->populated_pages;
    array[4] = 0; //(uint64_t)pg_cache->committed_page_index.nr_committed_pages;
    array[5] = (uint64_t)ctx->stats.pg_cache_insertions;
    array[6] = (uint64_t)ctx->stats.pg_cache_deletions;
    array[7] = (uint64_t)ctx->stats.pg_cache_hits;
    array[8] = (uint64_t)ctx->stats.pg_cache_misses;
    array[9] = (uint64_t)ctx->stats.pg_cache_backfills;
    array[10] = (uint64_t)ctx->stats.pg_cache_evictions;
    array[11] = (uint64_t)ctx->stats.before_compress_bytes;
    array[12] = (uint64_t)ctx->stats.after_compress_bytes;
    array[13] = (uint64_t)ctx->stats.before_decompress_bytes;
    array[14] = (uint64_t)ctx->stats.after_decompress_bytes;
    array[15] = (uint64_t)ctx->stats.io_write_bytes;
    array[16] = (uint64_t)ctx->stats.io_write_requests;
    array[17] = (uint64_t)ctx->stats.io_read_bytes;
    array[18] = (uint64_t)ctx->stats.io_read_requests;
    array[19] = (uint64_t)ctx->stats.io_write_extent_bytes;
    array[20] = (uint64_t)ctx->stats.io_write_extents;
    array[21] = (uint64_t)ctx->stats.io_read_extent_bytes;
    array[22] = (uint64_t)ctx->stats.io_read_extents;
    array[23] = (uint64_t)ctx->stats.datafile_creations;
    array[24] = (uint64_t)ctx->stats.datafile_deletions;
    array[25] = (uint64_t)ctx->stats.journalfile_creations;
    array[26] = (uint64_t)ctx->stats.journalfile_deletions;
    array[27] = (uint64_t)ctx->stats.page_cache_descriptors;
    array[28] = (uint64_t)ctx->stats.io_errors;
    array[29] = (uint64_t)ctx->stats.fs_errors;
    array[30] = (uint64_t)global_io_errors;
    array[31] = (uint64_t)global_fs_errors;
    array[32] = (uint64_t)rrdeng_reserved_file_descriptors;
    array[33] = (uint64_t)ctx->stats.pg_cache_over_half_dirty_events;
    array[34] = (uint64_t)global_pg_cache_over_half_dirty_events;
    array[35] = (uint64_t)ctx->stats.flushing_pressure_page_deletions;
    array[36] = (uint64_t)global_flushing_pressure_page_deletions;
    array[37] = (uint64_t)pg_cache->active_descriptors;

    fatal_assert(RRDENG_NR_STATS == 38);
}

/*
 * Returns 0 on success, negative on error
 */
int rrdeng_init(RRDHOST *host, struct rrdengine_instance **ctxp, char *dbfiles_path, unsigned page_cache_mb,
                unsigned disk_space_mb, size_t tier) {
    struct rrdengine_instance *ctx;
    int error;
    uint32_t max_open_files;

    max_open_files = rlimit_nofile.rlim_cur / 4;

    /* reserve RRDENG_FD_BUDGET_PER_INSTANCE file descriptors for this instance */
    rrd_stat_atomic_add(&rrdeng_reserved_file_descriptors, RRDENG_FD_BUDGET_PER_INSTANCE);
    if (rrdeng_reserved_file_descriptors > max_open_files) {
        error(
            "Exceeded the budget of available file descriptors (%u/%u), cannot create new dbengine instance.",
            (unsigned)rrdeng_reserved_file_descriptors,
            (unsigned)max_open_files);

        rrd_stat_atomic_add(&global_fs_errors, 1);
        rrd_stat_atomic_add(&rrdeng_reserved_file_descriptors, -RRDENG_FD_BUDGET_PER_INSTANCE);
        return UV_EMFILE;
    }

    if(NULL == ctxp) {
        ctx = multidb_ctx[tier];
        memset(ctx, 0, sizeof(*ctx));
    }
    else {
        *ctxp = ctx = callocz(1, sizeof(*ctx));
    }
    ctx->tier = tier;
    ctx->page_type = tier_page_type[tier];
    ctx->global_compress_alg = RRD_LZ4;
    if (page_cache_mb < RRDENG_MIN_PAGE_CACHE_SIZE_MB)
        page_cache_mb = RRDENG_MIN_PAGE_CACHE_SIZE_MB;
    ctx->max_cache_pages = page_cache_mb * (1048576LU / RRDENG_BLOCK_SIZE);
    /* try to keep 5% of the page cache free */
//    ctx->cache_pages_low_watermark = (ctx->max_cache_pages * 95LLU) / 100;
    if (disk_space_mb < RRDENG_MIN_DISK_SPACE_MB)
        disk_space_mb = RRDENG_MIN_DISK_SPACE_MB;
    ctx->max_disk_space = disk_space_mb * 1048576LLU;
    strncpyz(ctx->dbfiles_path, dbfiles_path, sizeof(ctx->dbfiles_path) - 1);
    ctx->dbfiles_path[sizeof(ctx->dbfiles_path) - 1] = '\0';
    if (NULL == host)
        strncpyz(ctx->machine_guid, registry_get_this_machine_guid(), GUID_LEN);
    else
        strncpyz(ctx->machine_guid, host->machine_guid, GUID_LEN);

//    ctx->drop_metrics_under_page_cache_pressure = rrdeng_drop_metrics_under_page_cache_pressure;
    ctx->metric_API_max_producers = 0;
    ctx->quiesce = NO_QUIESCE;
    ctx->host = host;

    memset(&ctx->worker_config, 0, sizeof(ctx->worker_config));
    ctx->worker_config.ctx = ctx;
    init_page_cache(ctx);
    init_commit_log(ctx);
    error = init_rrd_files(ctx);
    if (error) {
        goto error_after_init_rrd_files;
    }

    completion_init(&ctx->rrdengine_completion);
    fatal_assert(0 == uv_thread_create(&ctx->worker_config.thread, rrdeng_worker, &ctx->worker_config));
    /* wait for worker thread to initialize */
    completion_wait_for(&ctx->rrdengine_completion);
    completion_destroy(&ctx->rrdengine_completion);
    uv_thread_set_name_np(ctx->worker_config.thread, "LIBUV_WORKER");
    if (ctx->worker_config.error) {
        goto error_after_rrdeng_worker;
    }

    return 0;

error_after_rrdeng_worker:
    finalize_rrd_files(ctx);
error_after_init_rrd_files:
    // FIXME: DBENGINE2 must shutodwn the page cache
//    free_page_cache(ctx);
    if (!is_storage_engine_shared((STORAGE_INSTANCE *)ctx)) {
        freez(ctx);
        if (ctxp)
            *ctxp = NULL;
    }
    rrd_stat_atomic_add(&rrdeng_reserved_file_descriptors, -RRDENG_FD_BUDGET_PER_INSTANCE);
    return UV_EIO;
}

/*
 * Returns 0 on success, 1 on error
 */
int rrdeng_exit(struct rrdengine_instance *ctx)
{
    struct rrdeng_cmd cmd;

    if (NULL == ctx) {
        return 1;
    }

    /* TODO: add page to page cache */
    cmd.opcode = RRDENG_SHUTDOWN;
    rrdeng_enq_cmd(&ctx->worker_config, &cmd);

    fatal_assert(0 == uv_thread_join(&ctx->worker_config.thread));

    finalize_rrd_files(ctx);
    //metalog_exit(ctx->metalog_ctx);
    // FIXME: DBENGINE2 free entire page cache for everyone
//    free_page_cache(ctx);

    if(!is_storage_engine_shared((STORAGE_INSTANCE *)ctx))
        freez(ctx);

    rrd_stat_atomic_add(&rrdeng_reserved_file_descriptors, -RRDENG_FD_BUDGET_PER_INSTANCE);
    return 0;
}

void rrdeng_prepare_exit(struct rrdengine_instance *ctx)
{
    struct rrdeng_cmd cmd;

    if (NULL == ctx) {
        return;
    }

    completion_init(&ctx->rrdengine_completion);
    cmd.opcode = RRDENG_QUIESCE;
    rrdeng_enq_cmd(&ctx->worker_config, &cmd);

    /* wait for dbengine to quiesce */
    completion_wait_for(&ctx->rrdengine_completion);
    completion_destroy(&ctx->rrdengine_completion);
}

static void populate_v2_statistics(struct rrdengine_datafile *datafile, RRDENG_SIZE_STATS *stats)
{
    void *data_start = datafile->journalfile->journal_data;
    if (unlikely(!data_start))
        return;

    struct journal_v2_header *j2_header = (void *) data_start;

    stats->extents += j2_header->extent_count;

    unsigned entries;
    struct journal_extent_list *extent_list = (void *) (data_start + j2_header->extent_offset);
    for (entries = 0; entries < j2_header->extent_count; entries++) {
        stats->extents_compressed_bytes += extent_list->datafile_size;
        stats->extents_pages += extent_list->pages;
        extent_list++;
    }

    struct journal_metric_list *metric = (void *) (data_start + j2_header->metric_offset);
    usec_t journal_start_time_ut = j2_header->start_time_ut;

    for (entries = 0; entries < j2_header->metric_count; entries++) {

        struct journal_page_header *metric_list_header = (void *) (data_start + metric->page_offset);
        struct journal_page_list *descr =  (void *) (data_start + metric->page_offset + sizeof(struct journal_page_header));
        for (uint32_t idx=0; idx < metric_list_header->entries; idx++) {

            usec_t update_every_usec;

            size_t points = descr->page_length / PAGE_POINT_CTX_SIZE_BYTES(datafile->ctx);

            usec_t start_time_ut = journal_start_time_ut + ((usec_t) descr->delta_start_s * USEC_PER_SEC);
            usec_t end_time_ut = journal_start_time_ut + ((usec_t) descr->delta_end_s * USEC_PER_SEC);

            if(likely(points > 1))
                update_every_usec = (end_time_ut - start_time_ut) / (points - 1);
            else {
                update_every_usec = default_rrd_update_every * get_tier_grouping(datafile->ctx->tier) * USEC_PER_SEC;
                stats->single_point_pages++;
            }

            time_t duration_secs = (time_t)((end_time_ut - start_time_ut + update_every_usec)/USEC_PER_SEC);

            stats->pages_uncompressed_bytes += descr->page_length;
            stats->pages_duration_secs += duration_secs;
            stats->points += points;

            stats->page_types[descr->type].pages++;
            stats->page_types[descr->type].pages_uncompressed_bytes += descr->page_length;
            stats->page_types[descr->type].pages_duration_secs += duration_secs;
            stats->page_types[descr->type].points += points;

            if(!stats->first_t || (start_time_ut - update_every_usec) < stats->first_t)
                stats->first_t = (start_time_ut - update_every_usec) / USEC_PER_SEC;

            if(!stats->last_t || end_time_ut > stats->last_t)
                stats->last_t = end_time_ut / USEC_PER_SEC;

            descr++;
        }
        metric++;
    }
}

RRDENG_SIZE_STATS rrdeng_size_statistics(struct rrdengine_instance *ctx) {
    RRDENG_SIZE_STATS stats = { 0 };

//    for(struct pg_cache_page_index *page_index = ctx->pg_cache.metrics_index.last_page_index;
//        page_index != NULL ;page_index = page_index->prev) {
//        stats.metrics++;
//        stats.metrics_pages += page_index->page_count;
//    }

    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    for(struct rrdengine_datafile *df = ctx->datafiles.first; df ;df = df->next) {
        stats.datafiles++;

        if (df->journalfile->journal_data) {
            populate_v2_statistics(df, &stats);
        }
        else
        for(struct extent_info *ei = df->extents.first; ei ; ei = ei->next) {
            stats.extents++;
            stats.extents_compressed_bytes += ei->size;

            for(int p = 0; p < ei->number_of_pages ;p++) {
                struct rrdeng_page_descr *descr = ei->pages[p];

                if (unlikely(!descr))
                    continue;

                usec_t update_every_usec;

                size_t points = descr->page_length / PAGE_POINT_SIZE_BYTES(descr);

                if(likely(points > 1))
                    update_every_usec = (descr->end_time_ut - descr->start_time_ut) / (points - 1);
                else {
                    update_every_usec = default_rrd_update_every * get_tier_grouping(ctx->tier) * USEC_PER_SEC;
                    stats.single_point_pages++;
                }

                time_t duration_secs = (time_t)((descr->end_time_ut - descr->start_time_ut + update_every_usec)/USEC_PER_SEC);

                stats.extents_pages++;
                stats.pages_uncompressed_bytes += descr->page_length;
                stats.pages_duration_secs += duration_secs;
                stats.points += points;

                stats.page_types[descr->type].pages++;
                stats.page_types[descr->type].pages_uncompressed_bytes += descr->page_length;
                stats.page_types[descr->type].pages_duration_secs += duration_secs;
                stats.page_types[descr->type].points += points;

                if(!stats.first_t || (descr->start_time_ut - update_every_usec) < stats.first_t)
                    stats.first_t = (descr->start_time_ut - update_every_usec) / USEC_PER_SEC;

                if(!stats.last_t || descr->end_time_ut > stats.last_t)
                    stats.last_t = descr->end_time_ut / USEC_PER_SEC;
            }
        }
    }
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    stats.currently_collected_metrics = ctx->stats.metric_API_producers;
    stats.max_concurrently_collected_metrics = ctx->metric_API_max_producers;

    internal_error(stats.metrics_pages != stats.extents_pages + stats.currently_collected_metrics,
                   "DBENGINE: metrics pages is %zu, but extents pages is %zu and API consumers is %zu",
                   stats.metrics_pages, stats.extents_pages, stats.currently_collected_metrics);

    stats.disk_space = ctx->disk_space;
    stats.max_disk_space = ctx->max_disk_space;

    stats.database_retention_secs = (time_t)(stats.last_t - stats.first_t);

    if(stats.extents_pages)
        stats.average_page_size_bytes = (double)stats.pages_uncompressed_bytes / (double)stats.extents_pages;

    if(stats.pages_uncompressed_bytes > 0)
        stats.average_compression_savings = 100.0 - ((double)stats.extents_compressed_bytes * 100.0 / (double)stats.pages_uncompressed_bytes);

    if(stats.points)
        stats.average_point_duration_secs = (double)stats.pages_duration_secs / (double)stats.points;

    if(stats.metrics) {
        stats.average_metric_retention_secs = (double)stats.pages_duration_secs / (double)stats.metrics;

        if(stats.database_retention_secs) {
            double metric_coverage = stats.average_metric_retention_secs / (double)stats.database_retention_secs;
            double db_retention_days = (double)stats.database_retention_secs / 86400.0;

            stats.estimated_concurrently_collected_metrics = stats.metrics * metric_coverage;

            stats.ephemeral_metrics_per_day_percent = ((double)stats.metrics * 100.0 / (double)stats.estimated_concurrently_collected_metrics - 100.0) / (double)db_retention_days;
        }
    }

    stats.sizeof_metric = struct_natural_alignment(sizeof(struct pg_cache_page_index) + sizeof(struct pg_alignment));
    stats.sizeof_page = struct_natural_alignment(sizeof(struct rrdeng_page_descr));
    stats.sizeof_datafile = struct_natural_alignment(sizeof(struct rrdengine_datafile)) + struct_natural_alignment(sizeof(struct rrdengine_journalfile));
    stats.sizeof_page_in_cache = 0; // struct_natural_alignment(sizeof(struct page_cache_descr));
    stats.sizeof_point_data = page_type_size[ctx->page_type];
    stats.sizeof_page_data = RRDENG_BLOCK_SIZE;
    stats.pages_per_extent = rrdeng_pages_per_extent;

    stats.sizeof_extent = sizeof(struct extent_info);
    stats.sizeof_page_in_extent = sizeof(struct rrdeng_page_descr *);

    stats.sizeof_metric_in_index = 40;
    stats.sizeof_page_in_index = 24;

    stats.default_granularity_secs = (size_t)default_rrd_update_every * get_tier_grouping(ctx->tier);

    return stats;
}
