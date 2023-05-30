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

#if defined(ENV32BIT)
size_t tier_page_size[RRD_STORAGE_TIERS] = {2048, 1024, 192, 192, 192};
#else
size_t tier_page_size[RRD_STORAGE_TIERS] = {4096, 2048, 384, 384, 384};
#endif

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

int db_engine_journal_check = 0;
int default_rrdeng_disk_quota_mb = 256;
int default_multidb_disk_quota_mb = 256;

#if defined(ENV32BIT)
int default_rrdeng_page_cache_mb = 16;
int default_rrdeng_extent_cache_mb = 0;
#else
int default_rrdeng_page_cache_mb = 32;
int default_rrdeng_extent_cache_mb = 0;
#endif

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
            .first_time_s = 0,
            .last_time_s = 0,
            .latest_update_every_s = 0,
    };
    uuid_copy(entry.uuid, *uuid);

    METRIC *metric = mrg_metric_add_and_acquire(main_mrg, entry, NULL);
    return metric;
}

STORAGE_METRIC_HANDLE *rrdeng_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *db_instance) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)db_instance;
    METRIC *metric;

    metric = mrg_metric_get_and_acquire(main_mrg, &rd->metric_uuid, (Word_t) ctx);

    if(unlikely(!metric)) {
        if(unlikely(ctx->config.legacy)) {
            // this is a single host database
            // generate uuid from the chart and dimensions ids
            // and overwrite the one supplied by rrddim
            metric = rrdeng_metric_get_legacy(db_instance, rrddim_id(rd), rrdset_id(rd->rrdset));
            if (metric)
                uuid_copy(rd->metric_uuid, *mrg_metric_uuid(main_mrg, metric));
        }

        if(likely(!metric))
            metric = rrdeng_metric_create(db_instance, &rd->metric_uuid);
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if(uuid_memcmp(&rd->metric_uuid, mrg_metric_uuid(main_mrg, metric)) != 0) {
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

static inline void check_and_fix_mrg_update_every(struct rrdeng_collect_handle *handle) {
    if(unlikely((time_t)(handle->update_every_ut / USEC_PER_SEC) != mrg_metric_get_update_every_s(main_mrg, handle->metric))) {
        internal_error(true, "DBENGINE: collection handle has update every %ld, but the metric registry has %ld. Fixing it.",
              (time_t)(handle->update_every_ut / USEC_PER_SEC), mrg_metric_get_update_every_s(main_mrg, handle->metric));

        if(unlikely(!handle->update_every_ut))
            handle->update_every_ut = (usec_t)mrg_metric_get_update_every_s(main_mrg, handle->metric) * USEC_PER_SEC;
        else
            mrg_metric_set_update_every(main_mrg, handle->metric, (time_t)(handle->update_every_ut / USEC_PER_SEC));
    }
}

static inline bool check_completed_page_consistency(struct rrdeng_collect_handle *handle __maybe_unused) {
#ifdef NETDATA_INTERNAL_CHECKS
    if (unlikely(!handle->page || !handle->page_entries_max || !handle->page_position || !handle->page_end_time_ut))
        return false;

    struct rrdengine_instance *ctx = mrg_metric_ctx(handle->metric);

    uuid_t *uuid = mrg_metric_uuid(main_mrg, handle->metric);
    time_t start_time_s = pgc_page_start_time_s(handle->page);
    time_t end_time_s = pgc_page_end_time_s(handle->page);
    time_t update_every_s = pgc_page_update_every_s(handle->page);
    size_t page_length = handle->page_position * CTX_POINT_SIZE_BYTES(ctx);
    size_t entries = handle->page_position;
    time_t overwrite_zero_update_every_s = (time_t)(handle->update_every_ut / USEC_PER_SEC);

    if(end_time_s > max_acceptable_collected_time())
        handle->page_flags |= RRDENG_PAGE_COMPLETED_IN_FUTURE;

    VALIDATED_PAGE_DESCRIPTOR vd = validate_page(
            uuid,
            start_time_s,
            end_time_s,
            update_every_s,
            page_length,
            ctx->config.page_type,
            entries,
            0, // do not check for future timestamps - we inherit the timestamps of the children
            overwrite_zero_update_every_s,
            false,
            "collected",
            handle->page_flags);

    return vd.is_valid;
#else
    return true;
#endif
}

/*
 * Gets a handle for storing metrics to the database.
 * The handle must be released with rrdeng_store_metric_final().
 */
STORAGE_COLLECT_HANDLE *rrdeng_store_metric_init(STORAGE_METRIC_HANDLE *db_metric_handle, uint32_t update_every, STORAGE_METRICS_GROUP *smg) {
    METRIC *metric = (METRIC *)db_metric_handle;
    struct rrdengine_instance *ctx = mrg_metric_ctx(metric);

    bool is_1st_metric_writer = true;
    if(!mrg_metric_set_writer(main_mrg, metric)) {
        is_1st_metric_writer = false;
        char uuid[UUID_STR_LEN + 1];
        uuid_unparse(*mrg_metric_uuid(main_mrg, metric), uuid);
        error("DBENGINE: metric '%s' is already collected and should not be collected twice - expect gaps on the charts", uuid);
    }

    metric = mrg_metric_dup(main_mrg, metric);

    struct rrdeng_collect_handle *handle;

    handle = callocz(1, sizeof(struct rrdeng_collect_handle));
    handle->common.backend = STORAGE_ENGINE_BACKEND_DBENGINE;
    handle->metric = metric;
    handle->page = NULL;
    handle->data = NULL;
    handle->data_size = 0;
    handle->page_position = 0;
    handle->page_entries_max = 0;
    handle->update_every_ut = (usec_t)update_every * USEC_PER_SEC;
    handle->options = is_1st_metric_writer ? RRDENG_1ST_METRIC_WRITER : 0;

    __atomic_add_fetch(&ctx->atomic.collectors_running, 1, __ATOMIC_RELAXED);
    if(!is_1st_metric_writer)
        __atomic_add_fetch(&ctx->atomic.collectors_running_duplicate, 1, __ATOMIC_RELAXED);

    mrg_metric_set_update_every(main_mrg, metric, update_every);

    handle->alignment = (struct pg_alignment *)smg;
    rrdeng_page_alignment_acquire(handle->alignment);

    // this is important!
    // if we don't set the page_end_time_ut during the first collection
    // data collection may be able to go back in time and during the addition of new pages
    // clean pages may be found matching ours!

    time_t db_first_time_s, db_last_time_s, db_update_every_s;
    mrg_metric_get_retention(main_mrg, metric, &db_first_time_s, &db_last_time_s, &db_update_every_s);
    handle->page_end_time_ut = (usec_t)db_last_time_s * USEC_PER_SEC;

    return (STORAGE_COLLECT_HANDLE *)handle;
}

/* The page must be populated and referenced */
static bool page_has_only_empty_metrics(struct rrdeng_collect_handle *handle) {
    switch(handle->type) {
        case PAGE_METRICS: {
            size_t slots = handle->page_position;
            storage_number *array = (storage_number *)pgc_page_data(handle->page);
            for (size_t i = 0 ; i < slots; ++i) {
                if(does_storage_number_exist(array[i]))
                    return false;
            }
        }
        break;

        case PAGE_TIER: {
            size_t slots = handle->page_position;
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
                error("DBENGINE: cannot check page for nulls on unknown page type id %d", (mrg_metric_ctx(handle->metric))->config.page_type);
                logged = true;
            }
            return false;
        }
    }

    return true;
}

void rrdeng_store_metric_flush_current_page(STORAGE_COLLECT_HANDLE *collection_handle) {
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)collection_handle;

    if (unlikely(!handle->page))
        return;

    if(!handle->page_position || page_has_only_empty_metrics(handle))
        pgc_page_to_clean_evict_or_release(main_cache, handle->page);

    else {
        check_completed_page_consistency(handle);
        mrg_metric_set_clean_latest_time_s(main_mrg, handle->metric, pgc_page_end_time_s(handle->page));
        pgc_page_hot_to_dirty_and_release(main_cache, handle->page);
    }

    mrg_metric_set_hot_latest_time_s(main_mrg, handle->metric, 0);

    handle->page = NULL;
    handle->page_flags = 0;
    handle->page_position = 0;
    handle->page_entries_max = 0;
    handle->data = NULL;
    handle->data_size = 0;

    // important!
    // we should never zero page end time ut, because this will allow
    // collection to go back in time
    // handle->page_end_time_ut = 0;
    // handle->page_start_time_ut;

    check_and_fix_mrg_update_every(handle);

    timing_step(TIMING_STEP_DBENGINE_FLUSH_PAGE);
}

static void rrdeng_store_metric_create_new_page(struct rrdeng_collect_handle *handle,
                struct rrdengine_instance *ctx,
                usec_t point_in_time_ut,
                void *data,
                size_t data_size) {
    time_t point_in_time_s = (time_t)(point_in_time_ut / USEC_PER_SEC);
    const time_t update_every_s = (time_t)(handle->update_every_ut / USEC_PER_SEC);

    PGC_ENTRY page_entry = {
            .section = (Word_t) ctx,
            .metric_id = mrg_metric_id(main_mrg, handle->metric),
            .start_time_s = point_in_time_s,
            .end_time_s = point_in_time_s,
            .size = data_size,
            .data = data,
            .update_every_s = (uint32_t) update_every_s,
            .hot = true
    };

    size_t conflicts = 0;
    bool added = true;
    PGC_PAGE *page = pgc_page_add_and_acquire(main_cache, page_entry, &added);
    while (unlikely(!added)) {
        conflicts++;

        char uuid[UUID_STR_LEN + 1];
        uuid_unparse(*mrg_metric_uuid(main_mrg, handle->metric), uuid);

#ifdef NETDATA_INTERNAL_CHECKS
        internal_error(true,
#else
        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl,
#endif
              "DBENGINE: metric '%s' new page from %ld to %ld, update every %ld, has a conflict in main cache "
              "with existing %s%s page from %ld to %ld, update every %ld - "
              "is it collected more than once?",
              uuid,
              page_entry.start_time_s, page_entry.end_time_s, (time_t)page_entry.update_every_s,
              pgc_is_page_hot(page) ? "hot" : "not-hot",
              pgc_page_data(page) == DBENGINE_EMPTY_PAGE ? " gap" : "",
              pgc_page_start_time_s(page), pgc_page_end_time_s(page), pgc_page_update_every_s(page)
              );

        pgc_page_release(main_cache, page);

        point_in_time_ut -= handle->update_every_ut;
        point_in_time_s = (time_t)(point_in_time_ut / USEC_PER_SEC);
        page_entry.start_time_s = point_in_time_s;
        page_entry.end_time_s = point_in_time_s;
        page = pgc_page_add_and_acquire(main_cache, page_entry, &added);
    }

    handle->page_entries_max = data_size / CTX_POINT_SIZE_BYTES(ctx);
    handle->page_start_time_ut = point_in_time_ut;
    handle->page_end_time_ut = point_in_time_ut;
    handle->page_position = 1; // zero is already in our data
    handle->page = page;
    handle->page_flags = conflicts? RRDENG_PAGE_CONFLICT : 0;

    if(point_in_time_s > max_acceptable_collected_time())
        handle->page_flags |= RRDENG_PAGE_CREATED_IN_FUTURE;

    check_and_fix_mrg_update_every(handle);

    timing_step(TIMING_STEP_DBENGINE_CREATE_NEW_PAGE);
}

static size_t aligned_allocation_entries(size_t max_slots, size_t target_slot, time_t now_s) {
    size_t slots = target_slot;
    size_t pos = (now_s % max_slots);

    if(pos > slots)
        slots += max_slots - pos;

    else if(pos < slots)
        slots -= pos;

    else
        slots = max_slots;

    return slots;
}

static void *rrdeng_alloc_new_metric_data(struct rrdeng_collect_handle *handle, size_t *data_size, usec_t point_in_time_ut) {
    struct rrdengine_instance *ctx = mrg_metric_ctx(handle->metric);

    size_t max_size = tier_page_size[ctx->config.tier];
    size_t max_slots = max_size / CTX_POINT_SIZE_BYTES(ctx);

    size_t slots = aligned_allocation_entries(
            max_slots,
            indexing_partition((Word_t) handle->alignment, max_slots),
            (time_t) (point_in_time_ut / USEC_PER_SEC)
    );

    if(slots < max_slots / 3)
        slots = max_slots / 3;

    if(slots < 3)
        slots = 3;

    size_t size = slots * CTX_POINT_SIZE_BYTES(ctx);

    // internal_error(true, "PAGE ALLOC %zu bytes (%zu max)", size, max_size);

    internal_fatal(slots < 3 || slots > max_slots, "ooops! wrong distribution of metrics across time");
    internal_fatal(size > tier_page_size[ctx->config.tier] || size < CTX_POINT_SIZE_BYTES(ctx) * 2, "ooops! wrong page size");

    *data_size = size;
    void *d = dbengine_page_alloc(size);

    timing_step(TIMING_STEP_DBENGINE_PAGE_ALLOC);

    return d;
}

static void rrdeng_store_metric_append_point(STORAGE_COLLECT_HANDLE *collection_handle,
                                             const usec_t point_in_time_ut,
                                             const NETDATA_DOUBLE n,
                                             const NETDATA_DOUBLE min_value,
                                             const NETDATA_DOUBLE max_value,
                                             const uint16_t count,
                                             const uint16_t anomaly_count,
                                             const SN_FLAGS flags)
{
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)collection_handle;
    struct rrdengine_instance *ctx = mrg_metric_ctx(handle->metric);

    if(unlikely(!handle->data))
        handle->data = rrdeng_alloc_new_metric_data(handle, &handle->data_size, point_in_time_ut);

    timing_step(TIMING_STEP_DBENGINE_CHECK_DATA);

    if(likely(ctx->config.page_type == PAGE_METRICS)) {
        storage_number *tier0_metric_data = handle->data;
        tier0_metric_data[handle->page_position] = pack_storage_number(n, flags);
    }
    else if(likely(ctx->config.page_type == PAGE_TIER)) {
        storage_number_tier1_t *tier12_metric_data = handle->data;
        storage_number_tier1_t number_tier1;
        number_tier1.sum_value = (float) n;
        number_tier1.min_value = (float) min_value;
        number_tier1.max_value = (float) max_value;
        number_tier1.anomaly_count = anomaly_count;
        number_tier1.count = count;
        tier12_metric_data[handle->page_position] = number_tier1;
    }
    else
        fatal("DBENGINE: cannot store metric on unknown page type id %d", ctx->config.page_type);

    timing_step(TIMING_STEP_DBENGINE_PACK);

    if(unlikely(!handle->page)){
        rrdeng_store_metric_create_new_page(handle, ctx, point_in_time_ut, handle->data, handle->data_size);
        // handle->position is set to 1 already
    }
    else {
        // update an existing page
        pgc_page_hot_set_end_time_s(main_cache, handle->page, (time_t) (point_in_time_ut / USEC_PER_SEC));
        handle->page_end_time_ut = point_in_time_ut;

        if(unlikely(++handle->page_position >= handle->page_entries_max)) {
            internal_fatal(handle->page_position > handle->page_entries_max, "DBENGINE: exceeded page max number of points");
            handle->page_flags |= RRDENG_PAGE_FULL;
            rrdeng_store_metric_flush_current_page(collection_handle);
        }
    }

    timing_step(TIMING_STEP_DBENGINE_PAGE_FIN);

    // update the metric information
    mrg_metric_set_hot_latest_time_s(main_mrg, handle->metric, (time_t) (point_in_time_ut / USEC_PER_SEC));

    timing_step(TIMING_STEP_DBENGINE_MRG_UPDATE);
}

static void store_metric_next_error_log(struct rrdeng_collect_handle *handle, usec_t point_in_time_ut, const char *msg) {
    time_t point_in_time_s = (time_t)(point_in_time_ut / USEC_PER_SEC);
    char uuid[UUID_STR_LEN + 1];
    uuid_unparse(*mrg_metric_uuid(main_mrg, handle->metric), uuid);

    BUFFER *wb = NULL;
    if(handle->page && handle->page_flags) {
        wb = buffer_create(0, NULL);
        collect_page_flags_to_buffer(wb, handle->page_flags);
    }

    error_limit_static_global_var(erl, 1, 0);
    error_limit(&erl,
                "DBENGINE: metric '%s' collected point at %ld, %s last collection at %ld, "
                "update every %ld, %s page from %ld to %ld, position %u (of %u), flags: %s",
                uuid,
                point_in_time_s,
                msg,
                (time_t)(handle->page_end_time_ut / USEC_PER_SEC),
                (time_t)(handle->update_every_ut / USEC_PER_SEC),
                handle->page ? "current" : "*LAST*",
                (time_t)(handle->page_start_time_ut / USEC_PER_SEC),
                (time_t)(handle->page_end_time_ut / USEC_PER_SEC),
                handle->page_position, handle->page_entries_max,
                wb ? buffer_tostring(wb) : ""
    );

    buffer_free(wb);
}

void rrdeng_store_metric_next(STORAGE_COLLECT_HANDLE *collection_handle,
                              const usec_t point_in_time_ut,
                              const NETDATA_DOUBLE n,
                              const NETDATA_DOUBLE min_value,
                              const NETDATA_DOUBLE max_value,
                              const uint16_t count,
                              const uint16_t anomaly_count,
                              const SN_FLAGS flags)
{
    timing_step(TIMING_STEP_RRDSET_STORE_METRIC);

    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)collection_handle;

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(point_in_time_ut > (usec_t)max_acceptable_collected_time() * USEC_PER_SEC))
        handle->page_flags |= RRDENG_PAGE_FUTURE_POINT;
#endif

    usec_t delta_ut = point_in_time_ut - handle->page_end_time_ut;

    if(likely(delta_ut == handle->update_every_ut)) {
        // happy path
        ;
    }
    else if(unlikely(point_in_time_ut > handle->page_end_time_ut)) {
        if(handle->page) {
            if (unlikely(delta_ut < handle->update_every_ut)) {
                handle->page_flags |= RRDENG_PAGE_STEP_TOO_SMALL;
                rrdeng_store_metric_flush_current_page(collection_handle);
            }
            else if (unlikely(delta_ut % handle->update_every_ut)) {
                handle->page_flags |= RRDENG_PAGE_STEP_UNALIGNED;
                rrdeng_store_metric_flush_current_page(collection_handle);
            }
            else {
                size_t points_gap = delta_ut / handle->update_every_ut;
                size_t page_remaining_points = handle->page_entries_max - handle->page_position;

                if (points_gap >= page_remaining_points) {
                    handle->page_flags |= RRDENG_PAGE_BIG_GAP;
                    rrdeng_store_metric_flush_current_page(collection_handle);
                }
                else {
                    // loop to fill the gap
                    handle->page_flags |= RRDENG_PAGE_GAP;

                    usec_t stop_ut = point_in_time_ut - handle->update_every_ut;
                    for (usec_t this_ut = handle->page_end_time_ut + handle->update_every_ut;
                         this_ut <= stop_ut;
                         this_ut = handle->page_end_time_ut + handle->update_every_ut) {
                        rrdeng_store_metric_append_point(
                                collection_handle,
                                this_ut,
                                NAN, NAN, NAN,
                                1, 0,
                                SN_EMPTY_SLOT);
                    }
                }
            }
        }
    }
    else if(unlikely(point_in_time_ut < handle->page_end_time_ut)) {
        handle->page_flags |= RRDENG_PAGE_PAST_COLLECTION;
        store_metric_next_error_log(handle, point_in_time_ut, "is older than the");
        return;
    }

    else /* if(unlikely(point_in_time_ut == handle->page_end_time_ut)) */ {
        handle->page_flags |= RRDENG_PAGE_REPEATED_COLLECTION;
        store_metric_next_error_log(handle, point_in_time_ut, "is at the same time as the");
        return;
    }

    timing_step(TIMING_STEP_DBENGINE_FIRST_CHECK);

    rrdeng_store_metric_append_point(collection_handle,
                                     point_in_time_ut,
                                     n, min_value, max_value,
                                     count, anomaly_count,
                                     flags);
}

/*
 * Releases the database reference from the handle for storing metrics.
 * Returns 1 if it's safe to delete the dimension.
 */
int rrdeng_store_metric_finalize(STORAGE_COLLECT_HANDLE *collection_handle) {
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)collection_handle;
    struct rrdengine_instance *ctx = mrg_metric_ctx(handle->metric);

    handle->page_flags |= RRDENG_PAGE_COLLECT_FINALIZE;
    rrdeng_store_metric_flush_current_page(collection_handle);
    rrdeng_page_alignment_release(handle->alignment);

    __atomic_sub_fetch(&ctx->atomic.collectors_running, 1, __ATOMIC_RELAXED);
    if(!(handle->options & RRDENG_1ST_METRIC_WRITER))
        __atomic_sub_fetch(&ctx->atomic.collectors_running_duplicate, 1, __ATOMIC_RELAXED);

    if((handle->options & RRDENG_1ST_METRIC_WRITER) && !mrg_metric_clear_writer(main_mrg, handle->metric))
        internal_fatal(true, "DBENGINE: metric is already released");

    time_t first_time_s, last_time_s, update_every_s;
    mrg_metric_get_retention(main_mrg, handle->metric, &first_time_s, &last_time_s, &update_every_s);

    mrg_metric_release(main_mrg, handle->metric);
    freez(handle);

    if(!first_time_s && !last_time_s)
        return 1;

    return 0;
}

void rrdeng_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *collection_handle, int update_every) {
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)collection_handle;
    check_and_fix_mrg_update_every(handle);

    METRIC *metric = handle->metric;
    usec_t update_every_ut = (usec_t)update_every * USEC_PER_SEC;

    if(update_every_ut == handle->update_every_ut)
        return;

    handle->page_flags |= RRDENG_PAGE_UPDATE_EVERY_CHANGE;
    rrdeng_store_metric_flush_current_page(collection_handle);
    mrg_metric_set_update_every(main_mrg, metric, update_every);
    handle->update_every_ut = update_every_ut;
}

// ----------------------------------------------------------------------------
// query ops

#ifdef NETDATA_INTERNAL_CHECKS
SPINLOCK global_query_handle_spinlock = NETDATA_SPINLOCK_INITIALIZER;
static struct rrdeng_query_handle *global_query_handle_ll = NULL;
static void register_query_handle(struct rrdeng_query_handle *handle) {
    handle->query_pid = gettid();
    handle->started_time_s = now_realtime_sec();

    netdata_spinlock_lock(&global_query_handle_spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(global_query_handle_ll, handle, prev, next);
    netdata_spinlock_unlock(&global_query_handle_spinlock);
}
static void unregister_query_handle(struct rrdeng_query_handle *handle) {
    netdata_spinlock_lock(&global_query_handle_spinlock);
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(global_query_handle_ll, handle, prev, next);
    netdata_spinlock_unlock(&global_query_handle_spinlock);
}
#else
static void register_query_handle(struct rrdeng_query_handle *handle __maybe_unused) {
    ;
}
static void unregister_query_handle(struct rrdeng_query_handle *handle __maybe_unused) {
    ;
}
#endif

/*
 * Gets a handle for loading metrics from the database.
 * The handle must be released with rrdeng_load_metric_final().
 */
void rrdeng_load_metric_init(STORAGE_METRIC_HANDLE *db_metric_handle,
                             struct storage_engine_query_handle *rrddim_handle,
                             time_t start_time_s,
                             time_t end_time_s,
                             STORAGE_PRIORITY priority)
{
    usec_t started_ut = now_monotonic_usec();

    netdata_thread_disable_cancelability();

    METRIC *metric = (METRIC *)db_metric_handle;
    struct rrdengine_instance *ctx = mrg_metric_ctx(metric);
    struct rrdeng_query_handle *handle;

    handle = rrdeng_query_handle_get();
    register_query_handle(handle);

    if (unlikely(priority < STORAGE_PRIORITY_HIGH))
        priority = STORAGE_PRIORITY_HIGH;
    else if (unlikely(priority >= STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE))
        priority = STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE - 1;

    handle->ctx = ctx;
    handle->metric = metric;
    handle->priority = priority;

    // IMPORTANT!
    // It is crucial not to exceed the db boundaries, because dbengine
    // now has gap caching, so when a gap is detected a negative page
    // is inserted into the main cache, to avoid scanning the journals
    // again for pages matching the gap.

    time_t db_first_time_s, db_last_time_s, db_update_every_s;
    mrg_metric_get_retention(main_mrg, metric, &db_first_time_s, &db_last_time_s, &db_update_every_s);

    if(is_page_in_time_range(start_time_s, end_time_s, db_first_time_s, db_last_time_s) == PAGE_IS_IN_RANGE) {
        handle->start_time_s = MAX(start_time_s, db_first_time_s);
        handle->end_time_s = MIN(end_time_s, db_last_time_s);
        handle->now_s = handle->start_time_s;

        handle->dt_s = db_update_every_s;
        if (!handle->dt_s) {
            handle->dt_s = default_rrd_update_every;
            mrg_metric_set_update_every_s_if_zero(main_mrg, metric, default_rrd_update_every);
        }

        rrddim_handle->handle = (STORAGE_QUERY_HANDLE *) handle;
        rrddim_handle->start_time_s = handle->start_time_s;
        rrddim_handle->end_time_s = handle->end_time_s;
        rrddim_handle->priority = priority;
        rrddim_handle->backend = STORAGE_ENGINE_BACKEND_DBENGINE;

        pg_cache_preload(handle);

        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.query_time_init, now_monotonic_usec() - started_ut, __ATOMIC_RELAXED);
    }
    else {
        handle->start_time_s = start_time_s;
        handle->end_time_s = end_time_s;
        handle->now_s = start_time_s;
        handle->dt_s = db_update_every_s;

        rrddim_handle->handle = (STORAGE_QUERY_HANDLE *) handle;
        rrddim_handle->start_time_s = handle->start_time_s;
        rrddim_handle->end_time_s = 0;
        rrddim_handle->priority = priority;
        rrddim_handle->backend = STORAGE_ENGINE_BACKEND_DBENGINE;
    }
}

static bool rrdeng_load_page_next(struct storage_engine_query_handle *rrddim_handle, bool debug_this __maybe_unused) {
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrddim_handle->handle;
    struct rrdengine_instance *ctx = handle->ctx;

    if (likely(handle->page)) {
        // we have a page to release
        pgc_page_release(main_cache, handle->page);
        handle->page = NULL;
    }

    if (unlikely(handle->now_s > rrddim_handle->end_time_s))
        return false;

    size_t entries;
    handle->page = pg_cache_lookup_next(ctx, handle->pdc, handle->now_s, handle->dt_s, &entries);
    if (unlikely(!handle->page))
        return false;

    internal_fatal(pgc_page_data(handle->page) == DBENGINE_EMPTY_PAGE, "Empty page returned");

    time_t page_start_time_s = pgc_page_start_time_s(handle->page);
    time_t page_end_time_s = pgc_page_end_time_s(handle->page);
    time_t page_update_every_s = pgc_page_update_every_s(handle->page);

    unsigned position;
    if(likely(handle->now_s >= page_start_time_s && handle->now_s <= page_end_time_s)) {

        if(unlikely(entries == 1 || page_start_time_s == page_end_time_s || !page_update_every_s)) {
            position = 0;
            handle->now_s = page_start_time_s;
        }
        else {
            position = (handle->now_s - page_start_time_s) * (entries - 1) / (page_end_time_s - page_start_time_s);
            time_t point_end_time_s = page_start_time_s + position * page_update_every_s;
            while(point_end_time_s < handle->now_s && position + 1 < entries) {
                // https://github.com/netdata/netdata/issues/14411
                // we really need a while() here, because the delta may be
                // 2 points at higher tiers
                position++;
                point_end_time_s = page_start_time_s + position * page_update_every_s;
            }
            handle->now_s = point_end_time_s;
        }

        internal_fatal(position >= entries, "DBENGINE: wrong page position calculation");
    }
    else if(handle->now_s < page_start_time_s) {
        handle->now_s = page_start_time_s;
        position = 0;
    }
    else {
        internal_fatal(true, "DBENGINE: this page is entirely in our past and should not be accepted for this query in the first place");
        handle->now_s = page_end_time_s;
        position = entries - 1;
    }

    handle->entries = entries;
    handle->position = position;
    handle->metric_data = pgc_page_data((PGC_PAGE *)handle->page);
    handle->dt_s = page_update_every_s;
    return true;
}

// Returns the metric and sets its timestamp into current_time
// IT IS REQUIRED TO **ALWAYS** SET ALL RETURN VALUES (current_time, end_time, flags)
// IT IS REQUIRED TO **ALWAYS** KEEP TRACK OF TIME, EVEN OUTSIDE THE DATABASE BOUNDARIES
STORAGE_POINT rrdeng_load_metric_next(struct storage_engine_query_handle *rrddim_handle) {
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrddim_handle->handle;
    STORAGE_POINT sp;

    if (unlikely(handle->now_s > rrddim_handle->end_time_s)) {
        storage_point_empty(sp, handle->now_s - handle->dt_s, handle->now_s);
        goto prepare_for_next_iteration;
    }

    if (unlikely(!handle->page || handle->position >= handle->entries)) {
        // We need to get a new page

        if (!rrdeng_load_page_next(rrddim_handle, false)) {
            handle->now_s = rrddim_handle->end_time_s;
            storage_point_empty(sp, handle->now_s - handle->dt_s, handle->now_s);
            goto prepare_for_next_iteration;
        }
    }

    sp.start_time_s = handle->now_s - handle->dt_s;
    sp.end_time_s = handle->now_s;

    switch(handle->ctx->config.page_type) {
        case PAGE_METRICS: {
            storage_number n = handle->metric_data[handle->position];
            sp.min = sp.max = sp.sum = unpack_storage_number(n);
            sp.flags = n & SN_USER_FLAGS;
            sp.count = 1;
            sp.anomaly_count = is_storage_number_anomalous(n) ? 1 : 0;
        }
        break;

        case PAGE_TIER: {
            storage_number_tier1_t tier1_value = ((storage_number_tier1_t *)handle->metric_data)[handle->position];
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
                error("DBENGINE: unknown page type %d found. Cannot decode it. Ignoring its metrics.", handle->ctx->config.page_type);
                logged = true;
            }
            storage_point_empty(sp, sp.start_time_s, sp.end_time_s);
        }
        break;
    }

prepare_for_next_iteration:
    internal_fatal(sp.end_time_s < rrddim_handle->start_time_s, "DBENGINE: this point is too old for this query");
    internal_fatal(sp.end_time_s < handle->now_s, "DBENGINE: this point is too old for this point in time");

    handle->now_s += handle->dt_s;
    handle->position++;

    return sp;
}

int rrdeng_load_metric_is_finished(struct storage_engine_query_handle *rrddim_handle) {
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrddim_handle->handle;
    return (handle->now_s > rrddim_handle->end_time_s);
}

/*
 * Releases the database reference from the handle for loading metrics.
 */
void rrdeng_load_metric_finalize(struct storage_engine_query_handle *rrddim_handle)
{
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrddim_handle->handle;

    if (handle->page)
        pgc_page_release(main_cache, handle->page);

    if(!pdc_release_and_destroy_if_unreferenced(handle->pdc, false, false))
        __atomic_store_n(&handle->pdc->workers_should_stop, true, __ATOMIC_RELAXED);

    unregister_query_handle(handle);
    rrdeng_query_handle_release(handle);
    rrddim_handle->handle = NULL;
    netdata_thread_enable_cancelability();
}

time_t rrdeng_load_align_to_optimal_before(struct storage_engine_query_handle *rrddim_handle) {
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrddim_handle->handle;

    if(handle->pdc) {
        rrdeng_prep_wait(handle->pdc);
        if (handle->pdc->optimal_end_time_s > rrddim_handle->end_time_s)
            rrddim_handle->end_time_s = handle->pdc->optimal_end_time_s;
    }

    return rrddim_handle->end_time_s;
}

time_t rrdeng_metric_latest_time(STORAGE_METRIC_HANDLE *db_metric_handle) {
    METRIC *metric = (METRIC *)db_metric_handle;
    time_t latest_time_s = 0;

    if (metric)
        latest_time_s = mrg_metric_get_latest_time_s(main_mrg, metric);

    return latest_time_s;
}

time_t rrdeng_metric_oldest_time(STORAGE_METRIC_HANDLE *db_metric_handle) {
    METRIC *metric = (METRIC *)db_metric_handle;

    time_t oldest_time_s = 0;
    if (metric)
        oldest_time_s = mrg_metric_get_first_time_s(main_mrg, metric);

    return oldest_time_s;
}

bool rrdeng_metric_retention_by_uuid(STORAGE_INSTANCE *db_instance, uuid_t *dim_uuid, time_t *first_entry_s, time_t *last_entry_s)
{
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)db_instance;
    if (unlikely(!ctx)) {
        error("DBENGINE: invalid STORAGE INSTANCE to %s()", __FUNCTION__);
        return false;
    }

    METRIC *metric = mrg_metric_get_and_acquire(main_mrg, dim_uuid, (Word_t) ctx);
    if (unlikely(!metric))
        return false;

    time_t update_every_s;
    mrg_metric_get_retention(main_mrg, metric, first_entry_s, last_entry_s, &update_every_s);

    mrg_metric_release(main_mrg, metric);

    return true;
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

    array[0] = (uint64_t)__atomic_load_n(&ctx->atomic.collectors_running, __ATOMIC_RELAXED); // API producers
    array[1] = (uint64_t)__atomic_load_n(&ctx->atomic.inflight_queries, __ATOMIC_RELAXED);   // API consumers
    array[2] = 0;
    array[3] = 0;
    array[4] = 0;
    array[5] = 0; // (uint64_t)ctx->stats.pg_cache_insertions;
    array[6] = 0; // (uint64_t)ctx->stats.pg_cache_deletions;
    array[7] = 0; // (uint64_t)ctx->stats.pg_cache_hits;
    array[8] = 0; // (uint64_t)ctx->stats.pg_cache_misses;
    array[9] = 0; // (uint64_t)ctx->stats.pg_cache_backfills;
    array[10] = 0; // (uint64_t)ctx->stats.pg_cache_evictions;
    array[11] = (uint64_t)__atomic_load_n(&ctx->stats.before_compress_bytes, __ATOMIC_RELAXED); // used
    array[12] = (uint64_t)__atomic_load_n(&ctx->stats.after_compress_bytes, __ATOMIC_RELAXED); // used
    array[13] = (uint64_t)__atomic_load_n(&ctx->stats.before_decompress_bytes, __ATOMIC_RELAXED);
    array[14] = (uint64_t)__atomic_load_n(&ctx->stats.after_decompress_bytes, __ATOMIC_RELAXED);
    array[15] = (uint64_t)__atomic_load_n(&ctx->stats.io_write_bytes, __ATOMIC_RELAXED); // used
    array[16] = (uint64_t)__atomic_load_n(&ctx->stats.io_write_requests, __ATOMIC_RELAXED); // used
    array[17] = (uint64_t)__atomic_load_n(&ctx->stats.io_read_bytes, __ATOMIC_RELAXED);
    array[18] = (uint64_t)__atomic_load_n(&ctx->stats.io_read_requests, __ATOMIC_RELAXED); // used
    array[19] = 0; // (uint64_t)__atomic_load_n(&ctx->stats.io_write_extent_bytes, __ATOMIC_RELAXED);
    array[20] = 0; // (uint64_t)__atomic_load_n(&ctx->stats.io_write_extents, __ATOMIC_RELAXED);
    array[21] = 0; // (uint64_t)__atomic_load_n(&ctx->stats.io_read_extent_bytes, __ATOMIC_RELAXED);
    array[22] = 0; // (uint64_t)__atomic_load_n(&ctx->stats.io_read_extents, __ATOMIC_RELAXED);
    array[23] = (uint64_t)__atomic_load_n(&ctx->stats.datafile_creations, __ATOMIC_RELAXED);
    array[24] = (uint64_t)__atomic_load_n(&ctx->stats.datafile_deletions, __ATOMIC_RELAXED);
    array[25] = (uint64_t)__atomic_load_n(&ctx->stats.journalfile_creations, __ATOMIC_RELAXED);
    array[26] = (uint64_t)__atomic_load_n(&ctx->stats.journalfile_deletions, __ATOMIC_RELAXED);
    array[27] = 0; // (uint64_t)__atomic_load_n(&ctx->stats.page_cache_descriptors, __ATOMIC_RELAXED);
    array[28] = (uint64_t)__atomic_load_n(&ctx->stats.io_errors, __ATOMIC_RELAXED);
    array[29] = (uint64_t)__atomic_load_n(&ctx->stats.fs_errors, __ATOMIC_RELAXED);
    array[30] = (uint64_t)__atomic_load_n(&global_io_errors, __ATOMIC_RELAXED); // used
    array[31] = (uint64_t)__atomic_load_n(&global_fs_errors, __ATOMIC_RELAXED); // used
    array[32] = (uint64_t)__atomic_load_n(&rrdeng_reserved_file_descriptors, __ATOMIC_RELAXED); // used
    array[33] = 0; // (uint64_t)__atomic_load_n(&ctx->stats.pg_cache_over_half_dirty_events, __ATOMIC_RELAXED);
    array[34] = (uint64_t)__atomic_load_n(&global_pg_cache_over_half_dirty_events, __ATOMIC_RELAXED); // used
    array[35] = 0; // (uint64_t)__atomic_load_n(&ctx->stats.flushing_pressure_page_deletions, __ATOMIC_RELAXED);
    array[36] = (uint64_t)__atomic_load_n(&global_flushing_pressure_page_deletions, __ATOMIC_RELAXED); // used
    array[37] = 0; //(uint64_t)pg_cache->active_descriptors;

    fatal_assert(RRDENG_NR_STATS == 38);
}

static void rrdeng_populate_mrg(struct rrdengine_instance *ctx) {
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    size_t datafiles = 0;
    for(struct rrdengine_datafile *df = ctx->datafiles.first; df ;df = df->next)
        datafiles++;
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    size_t cpus = get_netdata_cpus() / storage_tiers;
    if(cpus > datafiles)
        cpus = datafiles;

    if(cpus < 1)
        cpus = 1;

    if(cpus > (size_t)libuv_worker_threads)
        cpus = (size_t)libuv_worker_threads;

    if(cpus > MRG_PARTITIONS)
        cpus = MRG_PARTITIONS;

    info("DBENGINE: populating retention to MRG from %zu journal files of tier %d, using %zu threads...", datafiles, ctx->config.tier, cpus);

    if(datafiles > 2) {
        struct rrdengine_datafile *datafile;

        datafile = ctx->datafiles.first->prev;
        if(!(datafile->journalfile->v2.flags & JOURNALFILE_FLAG_IS_AVAILABLE))
            datafile = datafile->prev;

        if(datafile->journalfile->v2.flags & JOURNALFILE_FLAG_IS_AVAILABLE) {
            journalfile_v2_populate_retention_to_mrg(ctx, datafile->journalfile);
            datafile->populate_mrg.populated = true;
        }

        datafile = ctx->datafiles.first;
        if(datafile->journalfile->v2.flags & JOURNALFILE_FLAG_IS_AVAILABLE) {
            journalfile_v2_populate_retention_to_mrg(ctx, datafile->journalfile);
            datafile->populate_mrg.populated = true;
        }
    }

    ctx->loading.populate_mrg.size = cpus;
    ctx->loading.populate_mrg.array = callocz(ctx->loading.populate_mrg.size, sizeof(struct completion));

    for (size_t i = 0; i < ctx->loading.populate_mrg.size; i++) {
        completion_init(&ctx->loading.populate_mrg.array[i]);
        rrdeng_enq_cmd(ctx, RRDENG_OPCODE_CTX_POPULATE_MRG, NULL, &ctx->loading.populate_mrg.array[i],
                       STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
    }
}

void rrdeng_readiness_wait(struct rrdengine_instance *ctx) {
    for (size_t i = 0; i < ctx->loading.populate_mrg.size; i++) {
        completion_wait_for(&ctx->loading.populate_mrg.array[i]);
        completion_destroy(&ctx->loading.populate_mrg.array[i]);
    }

    freez(ctx->loading.populate_mrg.array);
    ctx->loading.populate_mrg.array = NULL;
    ctx->loading.populate_mrg.size = 0;

    info("DBENGINE: tier %d is ready for data collection and queries", ctx->config.tier);
}

bool rrdeng_is_legacy(STORAGE_INSTANCE *db_instance) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)db_instance;
    return ctx->config.legacy;
}

void rrdeng_exit_mode(struct rrdengine_instance *ctx) {
    __atomic_store_n(&ctx->quiesce.exit_mode, true, __ATOMIC_RELAXED);
}
/*
 * Returns 0 on success, negative on error
 */
int rrdeng_init(struct rrdengine_instance **ctxp, const char *dbfiles_path,
                unsigned disk_space_mb, size_t tier) {
    struct rrdengine_instance *ctx;
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
        ctx->config.legacy = false;
    }
    else {
        *ctxp = ctx = callocz(1, sizeof(*ctx));
        ctx->config.legacy = true;
    }

    ctx->config.tier = (int)tier;
    ctx->config.page_type = tier_page_type[tier];
    ctx->config.global_compress_alg = RRD_LZ4;
    if (disk_space_mb < RRDENG_MIN_DISK_SPACE_MB)
        disk_space_mb = RRDENG_MIN_DISK_SPACE_MB;
    ctx->config.max_disk_space = disk_space_mb * 1048576LLU;
    strncpyz(ctx->config.dbfiles_path, dbfiles_path, sizeof(ctx->config.dbfiles_path) - 1);
    ctx->config.dbfiles_path[sizeof(ctx->config.dbfiles_path) - 1] = '\0';

    ctx->atomic.transaction_id = 1;
    ctx->quiesce.enabled = false;

    if (rrdeng_dbengine_spawn(ctx) && !init_rrd_files(ctx)) {
        // success - we run this ctx too
        rrdeng_populate_mrg(ctx);
        return 0;
    }

    if (ctx->config.legacy) {
        freez(ctx);
        if (ctxp)
            *ctxp = NULL;
    }

    rrd_stat_atomic_add(&rrdeng_reserved_file_descriptors, -RRDENG_FD_BUDGET_PER_INSTANCE);
    return UV_EIO;
}

size_t rrdeng_collectors_running(struct rrdengine_instance *ctx) {
    return __atomic_load_n(&ctx->atomic.collectors_running, __ATOMIC_RELAXED);
}

/*
 * Returns 0 on success, 1 on error
 */
int rrdeng_exit(struct rrdengine_instance *ctx) {
    if (NULL == ctx)
        return 1;

    // FIXME - ktsaou - properly cleanup ctx
    // 1. make sure all collectors are stopped
    // 2. make new queries will not be accepted (this is quiesce that has already run)
    // 3. flush this section of the main cache
    // 4. then wait for completion

    bool logged = false;
    while(__atomic_load_n(&ctx->atomic.collectors_running, __ATOMIC_RELAXED) && !unittest_running) {
        if(!logged) {
            info("DBENGINE: waiting for collectors to finish on tier %d...", (ctx->config.legacy) ? -1 : ctx->config.tier);
            logged = true;
        }
        sleep_usec(100 * USEC_PER_MS);
    }

    info("DBENGINE: flushing main cache for tier %d", (ctx->config.legacy) ? -1 : ctx->config.tier);
    pgc_flush_all_hot_and_dirty_pages(main_cache, (Word_t)ctx);

    info("DBENGINE: shutting down tier %d", (ctx->config.legacy) ? -1 : ctx->config.tier);
    struct completion completion = {};
    completion_init(&completion);
    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_CTX_SHUTDOWN, NULL, &completion, STORAGE_PRIORITY_BEST_EFFORT, NULL, NULL);
    completion_wait_for(&completion);
    completion_destroy(&completion);

    finalize_rrd_files(ctx);

    if(ctx->config.legacy)
        freez(ctx);

    rrd_stat_atomic_add(&rrdeng_reserved_file_descriptors, -RRDENG_FD_BUDGET_PER_INSTANCE);
    return 0;
}

void rrdeng_prepare_exit(struct rrdengine_instance *ctx) {
    if (NULL == ctx)
        return;

    // FIXME - ktsaou - properly cleanup ctx
    // 1. make sure all collectors are stopped

    completion_init(&ctx->quiesce.completion);
    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_CTX_QUIESCE, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
}

static void populate_v2_statistics(struct rrdengine_datafile *datafile, RRDENG_SIZE_STATS *stats)
{
    struct journal_v2_header *j2_header = journalfile_v2_data_acquire(datafile->journalfile, NULL, 0, 0);
    void *data_start = (void *)j2_header;

    if(unlikely(!j2_header))
        return;

    stats->extents += j2_header->extent_count;

    unsigned entries;
    struct journal_extent_list *extent_list = (void *) (data_start + j2_header->extent_offset);
    for (entries = 0; entries < j2_header->extent_count; entries++) {
        stats->extents_compressed_bytes += extent_list->datafile_size;
        stats->extents_pages += extent_list->pages;
        extent_list++;
    }

    struct journal_metric_list *metric = (void *) (data_start + j2_header->metric_offset);
    time_t journal_start_time_s = (time_t) (j2_header->start_time_ut / USEC_PER_SEC);

    stats->metrics += j2_header->metric_count;
    for (entries = 0; entries < j2_header->metric_count; entries++) {

        struct journal_page_header *metric_list_header = (void *) (data_start + metric->page_offset);
        stats->metrics_pages += metric_list_header->entries;
        struct journal_page_list *descr =  (void *) (data_start + metric->page_offset + sizeof(struct journal_page_header));
        for (uint32_t idx=0; idx < metric_list_header->entries; idx++) {

            time_t update_every_s;

            size_t points = descr->page_length / CTX_POINT_SIZE_BYTES(datafile->ctx);

            time_t start_time_s = journal_start_time_s + descr->delta_start_s;
            time_t end_time_s = journal_start_time_s + descr->delta_end_s;

            if(likely(points > 1))
                update_every_s = (time_t) ((end_time_s - start_time_s) / (points - 1));
            else {
                update_every_s = (time_t) (default_rrd_update_every * get_tier_grouping(datafile->ctx->config.tier));
                stats->single_point_pages++;
            }

            time_t duration_s = (time_t)((end_time_s - start_time_s + update_every_s));

            stats->pages_uncompressed_bytes += descr->page_length;
            stats->pages_duration_secs += duration_s;
            stats->points += points;

            stats->page_types[descr->type].pages++;
            stats->page_types[descr->type].pages_uncompressed_bytes += descr->page_length;
            stats->page_types[descr->type].pages_duration_secs += duration_s;
            stats->page_types[descr->type].points += points;

            if(!stats->first_time_s || (start_time_s - update_every_s) < stats->first_time_s)
                stats->first_time_s = (start_time_s - update_every_s);

            if(!stats->last_time_s || end_time_s > stats->last_time_s)
                stats->last_time_s = end_time_s;

            descr++;
        }
        metric++;
    }

    journalfile_v2_data_release(datafile->journalfile);
}

RRDENG_SIZE_STATS rrdeng_size_statistics(struct rrdengine_instance *ctx) {
    RRDENG_SIZE_STATS stats = { 0 };

    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    for(struct rrdengine_datafile *df = ctx->datafiles.first; df ;df = df->next) {
        stats.datafiles++;
        populate_v2_statistics(df, &stats);
    }
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    stats.currently_collected_metrics = __atomic_load_n(&ctx->atomic.collectors_running, __ATOMIC_RELAXED);

    internal_error(stats.metrics_pages != stats.extents_pages + stats.currently_collected_metrics,
                   "DBENGINE: metrics pages is %zu, but extents pages is %zu and API consumers is %zu",
                   stats.metrics_pages, stats.extents_pages, stats.currently_collected_metrics);

    stats.disk_space = ctx_current_disk_space_get(ctx);
    stats.max_disk_space = ctx->config.max_disk_space;

    stats.database_retention_secs = (time_t)(stats.last_time_s - stats.first_time_s);

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

//    stats.sizeof_metric = 0;
    stats.sizeof_datafile = struct_natural_alignment(sizeof(struct rrdengine_datafile)) + struct_natural_alignment(sizeof(struct rrdengine_journalfile));
    stats.sizeof_page_in_cache = 0; // struct_natural_alignment(sizeof(struct page_cache_descr));
    stats.sizeof_point_data = page_type_size[ctx->config.page_type];
    stats.sizeof_page_data = tier_page_size[ctx->config.tier];
    stats.pages_per_extent = rrdeng_pages_per_extent;

//    stats.sizeof_metric_in_index = 40;
//    stats.sizeof_page_in_index = 24;

    stats.default_granularity_secs = (size_t)default_rrd_update_every * get_tier_grouping(ctx->config.tier);

    return stats;
}

struct rrdeng_cache_efficiency_stats rrdeng_get_cache_efficiency_stats(void) {
    // FIXME - make cache efficiency stats atomic
    return rrdeng_cache_efficiency_stats;
}
