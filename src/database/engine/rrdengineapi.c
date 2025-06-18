// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/engine/rrddiskprotocol.h"
#include "rrdengine.h"
#include "dbengine-compression.h"

/* Default global database instance */
struct rrdengine_instance multidb_ctx_storage_tier0 = { 0 };
struct rrdengine_instance multidb_ctx_storage_tier1 = { 0 };
struct rrdengine_instance multidb_ctx_storage_tier2 = { 0 };
struct rrdengine_instance multidb_ctx_storage_tier3 = { 0 };
struct rrdengine_instance multidb_ctx_storage_tier4 = { 0 };

#define mrg_metric_ctx(metric) (struct rrdengine_instance *)mrg_metric_section(main_mrg, metric)

#if RRD_STORAGE_TIERS != 5
#error RRD_STORAGE_TIERS is not 5 - you need to add allocations here
#endif
struct rrdengine_instance *multidb_ctx[RRD_STORAGE_TIERS] = { 0 };
uint8_t tier_page_type[RRD_STORAGE_TIERS] = {
    RRDENG_PAGE_TYPE_GORILLA_32BIT,
    RRDENG_PAGE_TYPE_ARRAY_TIER1,
    RRDENG_PAGE_TYPE_ARRAY_TIER1,
    RRDENG_PAGE_TYPE_ARRAY_TIER1,
    RRDENG_PAGE_TYPE_ARRAY_TIER1};

#if defined(ENV32BIT)
size_t tier_page_size[RRD_STORAGE_TIERS] = {2048, 1024, 192, 192, 192};
size_t tier_quota_mb[RRD_STORAGE_TIERS] = {512, 512, 512, 0, 0};
#else
size_t tier_page_size[RRD_STORAGE_TIERS] = {4096, 2048, 384, 384, 384};
size_t tier_quota_mb[RRD_STORAGE_TIERS] = {1024, 1024, 1024, 128, 64};
#endif

#if RRDENG_PAGE_TYPE_MAX != 2
#error PAGE_TYPE_MAX is not 2 - you need to add allocations here
#endif

size_t page_type_size[256] = {
        [RRDENG_PAGE_TYPE_ARRAY_32BIT] = sizeof(storage_number),
        [RRDENG_PAGE_TYPE_ARRAY_TIER1] = sizeof(storage_number_tier1_t),
        [RRDENG_PAGE_TYPE_GORILLA_32BIT] = sizeof(storage_number)
};

static inline void initialize_single_ctx(struct rrdengine_instance *ctx) {
    memset(ctx, 0, sizeof(*ctx));
    uv_rwlock_init(&ctx->datafiles.rwlock);
    rw_spinlock_init(&ctx->njfv2idx.spinlock);
}

__attribute__((constructor)) void initialize_multidb_ctx(void) {
    multidb_ctx[0] = &multidb_ctx_storage_tier0;
    multidb_ctx[1] = &multidb_ctx_storage_tier1;
    multidb_ctx[2] = &multidb_ctx_storage_tier2;
    multidb_ctx[3] = &multidb_ctx_storage_tier3;
    multidb_ctx[4] = &multidb_ctx_storage_tier4;

    for(int i = 0; i < RRD_STORAGE_TIERS ; i++)
        initialize_single_ctx(multidb_ctx[i]);
}

uint64_t dbengine_out_of_memory_protection = 0;
bool dbengine_use_all_ram_for_caches = false;
int db_engine_journal_check = 0;
bool new_dbengine_defaults = false;
bool legacy_multihost_db_space = false;
int default_rrdeng_disk_quota_mb = RRDENG_DEFAULT_TIER_DISK_SPACE_MB;
int default_multidb_disk_quota_mb = RRDENG_DEFAULT_TIER_DISK_SPACE_MB;
RRD_BACKFILL default_backfill = RRD_BACKFILL_NEW;

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
STORAGE_METRICS_GROUP *rrdeng_metrics_group_get(STORAGE_INSTANCE *si __maybe_unused, nd_uuid_t *uuid __maybe_unused) {
    struct pg_alignment *pa = callocz(1, sizeof(struct pg_alignment));
    rrdeng_page_alignment_acquire(pa);
    return (STORAGE_METRICS_GROUP *)pa;
}

// charts call this
void rrdeng_metrics_group_release(STORAGE_INSTANCE *si __maybe_unused, STORAGE_METRICS_GROUP *smg) {
    if(unlikely(!smg)) return;

    struct pg_alignment *pa = (struct pg_alignment *)smg;
    rrdeng_page_alignment_release(pa);
}

// ----------------------------------------------------------------------------
// metric handle for legacy dbs

/* This UUID is not unique across hosts */
void rrdeng_generate_unittest_uuid(const char *dim_id, const char *chart_id, nd_uuid_t *ret_uuid)
{
    CLEAN_BUFFER *wb = buffer_create(100, NULL);
    buffer_sprintf(wb,"%s.%s", dim_id, chart_id);
    ND_UUID uuid = UUID_generate_from_hash(buffer_tostring(wb), buffer_strlen(wb));
    uuid_copy(*ret_uuid, uuid.uuid);
}

static METRIC *rrdeng_metric_unittest(STORAGE_INSTANCE *si, const char *rd_id, const char *st_id) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    nd_uuid_t legacy_uuid;
    rrdeng_generate_unittest_uuid(rd_id, st_id, &legacy_uuid);
    return mrg_metric_get_and_acquire_by_uuid(main_mrg, &legacy_uuid, (Word_t)ctx);
}

// ----------------------------------------------------------------------------
// metric handle

void rrdeng_metric_release(STORAGE_METRIC_HANDLE *smh) {
    METRIC *metric = (METRIC *)smh;
    mrg_metric_release(main_mrg, metric);
}

STORAGE_METRIC_HANDLE *rrdeng_metric_dup(STORAGE_METRIC_HANDLE *smh) {
    METRIC *metric = (METRIC *)smh;
    return (STORAGE_METRIC_HANDLE *) mrg_metric_dup(main_mrg, metric);
}

STORAGE_METRIC_HANDLE *rrdeng_metric_get_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *uuid) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    return (STORAGE_METRIC_HANDLE *)mrg_metric_get_and_acquire_by_uuid(main_mrg, uuid, (Word_t)ctx);
}

STORAGE_METRIC_HANDLE *rrdeng_metric_get_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    return (STORAGE_METRIC_HANDLE *)mrg_metric_get_and_acquire_by_id(main_mrg, id, (Word_t)ctx);
}

static METRIC *rrdeng_metric_create(STORAGE_INSTANCE *si, nd_uuid_t *uuid) {
    internal_fatal(!si, "DBENGINE: STORAGE_INSTANCE is NULL");

    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    MRG_ENTRY entry = {
            .uuid = uuid,
            .section = (Word_t)ctx,
            .first_time_s = 0,
            .last_time_s = 0,
            .latest_update_every_s = 0,
    };

    bool added;
    METRIC *metric = mrg_metric_add_and_acquire(main_mrg, entry, &added);
    return metric;
}

STORAGE_METRIC_HANDLE *rrdeng_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *si) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    METRIC *metric;

    metric = mrg_metric_get_and_acquire_by_id(main_mrg, rd->uuid, (Word_t) ctx);

    if(unlikely(!metric)) {
        if(unlikely(unittest_running)) {
            metric = rrdeng_metric_unittest(si, rrddim_id(rd), rrdset_id(rd->rrdset));
            if (metric)
                rd->uuid = mrg_metric_uuidmap_id_dup(main_mrg, metric);
        }

        if(likely(!metric))
            metric = rrdeng_metric_create(si, uuidmap_uuid_ptr(rd->uuid));
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if(!uuid_eq(*uuidmap_uuid_ptr(rd->uuid), *mrg_metric_uuid(main_mrg, metric))) {
        char uuid1[UUID_STR_LEN + 1];
        char uuid2[UUID_STR_LEN + 1];

        uuid_unparse(*uuidmap_uuid_ptr(rd->uuid), uuid1);
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
    if(unlikely((uint32_t)(handle->update_every_ut / USEC_PER_SEC) != mrg_metric_get_update_every_s(main_mrg, handle->metric))) {
        internal_error(true, "DBENGINE: collection handle has update every %u, but the metric registry has %u. Fixing it.",
              (uint32_t)(handle->update_every_ut / USEC_PER_SEC), mrg_metric_get_update_every_s(main_mrg, handle->metric));

        if(unlikely(!handle->update_every_ut))
            handle->update_every_ut = (usec_t)mrg_metric_get_update_every_s(main_mrg, handle->metric) * USEC_PER_SEC;
        else
            mrg_metric_set_update_every(main_mrg, handle->metric, (uint32_t)(handle->update_every_ut / USEC_PER_SEC));
    }
}

static inline bool check_completed_page_consistency(struct rrdeng_collect_handle *handle __maybe_unused) {
#ifdef NETDATA_INTERNAL_CHECKS
    if (unlikely(!handle->pgc_page || !handle->page_entries_max || !handle->page_position || !handle->page_end_time_ut))
        return false;

    struct rrdengine_instance *ctx = mrg_metric_ctx(handle->metric);

    nd_uuid_t *uuid = mrg_metric_uuid(main_mrg, handle->metric);
    time_t start_time_s = pgc_page_start_time_s(handle->pgc_page);
    time_t end_time_s = pgc_page_end_time_s(handle->pgc_page);
    uint32_t update_every_s = pgc_page_update_every_s(handle->pgc_page);
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
STORAGE_COLLECT_HANDLE *rrdeng_store_metric_init(STORAGE_METRIC_HANDLE *smh, uint32_t update_every, STORAGE_METRICS_GROUP *smg) {
    METRIC *metric = (METRIC *)smh;
    struct rrdengine_instance *ctx = mrg_metric_ctx(metric);

    RRDENG_COLLECT_HANDLE_OPTIONS options = 0;
#ifdef NETDATA_INTERNAL_CHECKS
    bool is_1st_metric_writer = true;
    if(!mrg_metric_set_writer(main_mrg, metric)) {
        is_1st_metric_writer = false;
        char uuid[UUID_STR_LEN + 1];
        uuid_unparse(*mrg_metric_uuid(main_mrg, metric), uuid);
        netdata_log_error("DBENGINE: metric '%s' is already collected and should not be collected twice - expect gaps on the charts", uuid);
    }
    if(is_1st_metric_writer)
        options = RRDENG_1ST_METRIC_WRITER;
    else
        __atomic_add_fetch(&ctx->atomic.collectors_running_duplicate, 1, __ATOMIC_RELAXED);

#endif

    metric = mrg_metric_dup(main_mrg, metric);

    struct rrdeng_collect_handle *handle;

    handle = callocz(1, sizeof(struct rrdeng_collect_handle));
    handle->common.seb = STORAGE_ENGINE_BACKEND_DBENGINE;
    handle->metric = metric;
    
    handle->pgc_page = NULL;
    handle->page_data = NULL;

    handle->page_position = 0;
    handle->page_entries_max = 0;
    handle->update_every_ut = (usec_t)update_every * USEC_PER_SEC;
    handle->options = options;

    __atomic_add_fetch(&ctx->atomic.collectors_running, 1, __ATOMIC_RELAXED);

    mrg_metric_set_update_every(main_mrg, metric, update_every);

    handle->alignment = (struct pg_alignment *)smg;
    rrdeng_page_alignment_acquire(handle->alignment);

    // this is important!
    // if we don't set the page_end_time_ut during the first collection
    // data collection may be able to go back in time and during the addition of new pages
    // clean pages may be found matching ours!

    time_t db_first_time_s, db_last_time_s;
    mrg_metric_get_retention(main_mrg, metric, &db_first_time_s, &db_last_time_s, NULL);
    handle->page_end_time_ut = (usec_t)db_last_time_s * USEC_PER_SEC;

    return (STORAGE_COLLECT_HANDLE *)handle;
}

void rrdeng_store_metric_flush_current_page(STORAGE_COLLECT_HANDLE *sch) {
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)sch;

    if (unlikely(!handle->pgc_page))
        return;

    if(pgd_is_empty(handle->page_data))
        pgc_page_to_clean_evict_or_release(main_cache, handle->pgc_page);

    else {
        check_completed_page_consistency(handle);
        mrg_metric_set_clean_latest_time_s(main_mrg, handle->metric, pgc_page_end_time_s(handle->pgc_page));

        struct rrdengine_instance *ctx = mrg_metric_ctx(handle->metric);
        time_t start_time_s = pgc_page_start_time_s(handle->pgc_page);
        time_t end_time_s = pgc_page_end_time_s(handle->pgc_page);
        uint32_t update_every_s = mrg_metric_get_update_every_s(main_mrg, handle->metric);
        if (end_time_s && start_time_s && end_time_s > start_time_s && update_every_s) {
            uint64_t add_samples = (end_time_s - start_time_s) / update_every_s;
            __atomic_add_fetch(&ctx->atomic.samples, add_samples, __ATOMIC_RELAXED);
        }

        pgc_page_hot_to_dirty_and_release(main_cache, handle->pgc_page, false);
    }

    mrg_metric_set_hot_latest_time_s(main_mrg, handle->metric, 0);

    handle->pgc_page = NULL;
    handle->page_flags = 0;
    handle->page_position = 0;
    handle->page_entries_max = 0;
    handle->page_data = NULL;

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
                                                PGD *data) {
    time_t point_in_time_s = (time_t)(point_in_time_ut / USEC_PER_SEC);
    const uint32_t update_every_s = (uint32_t)(handle->update_every_ut / USEC_PER_SEC);

    PGC_ENTRY page_entry = {
            .section = (Word_t) ctx,
            .metric_id = mrg_metric_id(main_mrg, handle->metric),
            .start_time_s = point_in_time_s,
            .end_time_s = point_in_time_s,
            .size = pgd_memory_footprint(data),
            .data = data,
            .update_every_s = update_every_s,
            .hot = true
    };

    size_t conflicts = 0;
    bool added = true;
    PGC_PAGE *pgc_page = pgc_page_add_and_acquire(main_cache, page_entry, &added);
    while (unlikely(!added)) {
        conflicts++;

        char uuid[UUID_STR_LEN + 1];
        uuid_unparse(*mrg_metric_uuid(main_mrg, handle->metric), uuid);

#ifdef NETDATA_INTERNAL_CHECKS
        internal_error(true,
#else
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
#endif
                    "DBENGINE: metric '%s' new page from %ld to %ld, update every %u, has a conflict in main cache "
                    "with existing %s%s page from %ld to %ld, update every %u - "
                    "is it collected more than once?",
                    uuid,
                    page_entry.start_time_s, page_entry.end_time_s, page_entry.update_every_s,
                    pgc_is_page_hot(pgc_page) ? "hot" : "not-hot",
                    pgc_page_data(pgc_page) == PGD_EMPTY ? " gap" : "",
                    pgc_page_start_time_s(pgc_page), pgc_page_end_time_s(pgc_page), pgc_page_update_every_s(pgc_page)
              );

        pgc_page_release(main_cache, pgc_page);

        point_in_time_ut -= handle->update_every_ut;
        point_in_time_s = (time_t)(point_in_time_ut / USEC_PER_SEC);
        page_entry.start_time_s = point_in_time_s;
        page_entry.end_time_s = point_in_time_s;
        pgc_page = pgc_page_add_and_acquire(main_cache, page_entry, &added);
    }

    handle->page_entries_max = pgd_capacity(data);
    handle->page_start_time_ut = point_in_time_ut;
    handle->page_end_time_ut = point_in_time_ut;
    handle->page_position = 1; // zero is already in our data
    handle->pgc_page = pgc_page;
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

static PGD *rrdeng_alloc_new_page_data(struct rrdeng_collect_handle *handle, usec_t point_in_time_ut) {
    struct rrdengine_instance *ctx = mrg_metric_ctx(handle->metric);

    PGD *d = NULL;
    
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

    size_t size = slots * CTX_POINT_SIZE_BYTES(ctx); (void)size;

    // internal_error(true, "PAGE ALLOC %zu bytes (%zu max)", size, max_size);

    internal_fatal(slots < 3 || slots > max_slots, "ooops! wrong distribution of metrics across time");
    internal_fatal(size > tier_page_size[ctx->config.tier] || size < CTX_POINT_SIZE_BYTES(ctx) * 2, "ooops! wrong page size");

    switch (ctx->config.page_type) {
        case RRDENG_PAGE_TYPE_ARRAY_32BIT:
        case RRDENG_PAGE_TYPE_ARRAY_TIER1:
        case RRDENG_PAGE_TYPE_GORILLA_32BIT:
            d = pgd_create(ctx->config.page_type, slots);
            break;
        default:
            fatal("Unknown page type: %uc\n", ctx->config.page_type);
    }

    timing_step(TIMING_STEP_DBENGINE_PAGE_ALLOC);
    return d;
}

static ALWAYS_INLINE_HOT void rrdeng_store_metric_append_point(STORAGE_COLLECT_HANDLE *sch,
                                             const usec_t point_in_time_ut,
                                             const NETDATA_DOUBLE n,
                                             const NETDATA_DOUBLE min_value,
                                             const NETDATA_DOUBLE max_value,
                                             const uint16_t count,
                                             const uint16_t anomaly_count,
                                             const SN_FLAGS flags)
{
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)sch;
    struct rrdengine_instance *ctx = mrg_metric_ctx(handle->metric);

    if(unlikely(!handle->page_data))
        handle->page_data = rrdeng_alloc_new_page_data(handle, point_in_time_ut);

    timing_step(TIMING_STEP_DBENGINE_CHECK_DATA);

    size_t additional_bytes = pgd_append_point(handle->page_data,
                                               point_in_time_ut,
                                               n, min_value, max_value, count, anomaly_count, flags,
                                               handle->page_position);

    timing_step(TIMING_STEP_DBENGINE_PACK);

    if(unlikely(!handle->pgc_page)) {
        rrdeng_store_metric_create_new_page(handle, ctx, point_in_time_ut, handle->page_data);
        // handle->position is set to 1 already
    }
    else {
        // update an existing page
        pgc_page_hot_set_end_time_s(main_cache, handle->pgc_page,
                                    (time_t) (point_in_time_ut / USEC_PER_SEC), additional_bytes);
        handle->page_end_time_ut = point_in_time_ut;

        if(unlikely(++handle->page_position >= handle->page_entries_max)) {
            internal_fatal(handle->page_position > handle->page_entries_max, "DBENGINE: exceeded page max number of points");
            handle->page_flags |= RRDENG_PAGE_FULL;
            rrdeng_store_metric_flush_current_page(sch);
        }
    }

    timing_step(TIMING_STEP_DBENGINE_PAGE_FIN);

    // update the metric information
    mrg_metric_set_hot_latest_time_s(main_mrg, handle->metric, (time_t) (point_in_time_ut / USEC_PER_SEC));

    timing_step(TIMING_STEP_DBENGINE_MRG_UPDATE);
}

static void store_metric_next_error_log(struct rrdeng_collect_handle *handle __maybe_unused, usec_t point_in_time_ut __maybe_unused, const char *msg __maybe_unused) {
#ifdef NETDATA_INTERNAL_CHECKS
    time_t point_in_time_s = (time_t)(point_in_time_ut / USEC_PER_SEC);
    char uuid[UUID_STR_LEN + 1];
    uuid_unparse(*mrg_metric_uuid(main_mrg, handle->metric), uuid);

    BUFFER *wb = NULL;
    if(handle->pgc_page && handle->page_flags) {
        wb = buffer_create(0, NULL);
        collect_page_flags_to_buffer(wb, handle->page_flags);
    }

    nd_log_limit_static_global_var(erl, 1, 0);
    nd_log_limit(&erl, NDLS_DAEMON, NDLP_NOTICE,
                "DBENGINE: metric '%s' collected point at %ld, %s last collection at %ld, "
                "update every %ld, %s page from %ld to %ld, position %u (of %u), flags: %s",
                uuid,
                point_in_time_s,
                msg,
                (time_t)(handle->page_end_time_ut / USEC_PER_SEC),
                (time_t)(handle->update_every_ut / USEC_PER_SEC),
                handle->pgc_page ? "current" : "*LAST*",
                (time_t)(handle->page_start_time_ut / USEC_PER_SEC),
                (time_t)(handle->page_end_time_ut / USEC_PER_SEC),
                handle->page_position, handle->page_entries_max,
                wb ? buffer_tostring(wb) : ""
                );

    buffer_free(wb);
#else
    ;
#endif
}

ALWAYS_INLINE_HOT void rrdeng_store_metric_next(
    STORAGE_COLLECT_HANDLE *sch,
    const usec_t point_in_time_ut,
    const NETDATA_DOUBLE n,
    const NETDATA_DOUBLE min_value,
    const NETDATA_DOUBLE max_value,
    const uint16_t count,
    const uint16_t anomaly_count,
    const SN_FLAGS flags)
{
    timing_step(TIMING_STEP_RRDSET_STORE_METRIC);

    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)sch;

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
        if(handle->pgc_page) {
            if (unlikely(delta_ut < handle->update_every_ut)) {
                handle->page_flags |= RRDENG_PAGE_STEP_TOO_SMALL;
                rrdeng_store_metric_flush_current_page(sch);
            }
            else if (unlikely(delta_ut % handle->update_every_ut)) {
                handle->page_flags |= RRDENG_PAGE_STEP_UNALIGNED;
                rrdeng_store_metric_flush_current_page(sch);
            }
            else {
                size_t points_gap = delta_ut / handle->update_every_ut;
                size_t page_remaining_points = handle->page_entries_max - handle->page_position;

                if (points_gap >= page_remaining_points) {
                    handle->page_flags |= RRDENG_PAGE_BIG_GAP;
                    rrdeng_store_metric_flush_current_page(sch);
                }
                else {
                    // loop to fill the gap
                    handle->page_flags |= RRDENG_PAGE_GAP;

                    usec_t stop_ut = point_in_time_ut - handle->update_every_ut;
                    for (usec_t this_ut = handle->page_end_time_ut + handle->update_every_ut;
                         this_ut <= stop_ut;
                         this_ut = handle->page_end_time_ut + handle->update_every_ut) {
                        rrdeng_store_metric_append_point(
                                sch,
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

    rrdeng_store_metric_append_point(sch,
                                     point_in_time_ut,
                                     n, min_value, max_value,
                                     count, anomaly_count,
                                     flags);
}

/*
 * Releases the database reference from the handle for storing metrics.
 * Returns 1 if it's safe to delete the dimension.
 */
int rrdeng_store_metric_finalize(STORAGE_COLLECT_HANDLE *sch) {
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)sch;
    struct rrdengine_instance *ctx = mrg_metric_ctx(handle->metric);

    handle->page_flags |= RRDENG_PAGE_COLLECT_FINALIZE;
    rrdeng_store_metric_flush_current_page(sch);
    rrdeng_page_alignment_release(handle->alignment);

    __atomic_sub_fetch(&ctx->atomic.collectors_running, 1, __ATOMIC_RELAXED);

#ifdef NETDATA_INTERNAL_CHECKS
    if(!(handle->options & RRDENG_1ST_METRIC_WRITER))
        __atomic_sub_fetch(&ctx->atomic.collectors_running_duplicate, 1, __ATOMIC_RELAXED);

    if((handle->options & RRDENG_1ST_METRIC_WRITER) && !mrg_metric_clear_writer(main_mrg, handle->metric))
        internal_fatal(true, "DBENGINE: metric is already released");
#endif

    time_t first_time_s, last_time_s;
    mrg_metric_get_retention(main_mrg, handle->metric, &first_time_s, &last_time_s, NULL);

    mrg_metric_release(main_mrg, handle->metric);
    freez(handle);

    if(!first_time_s && !last_time_s)
        return 1;

    return 0;
}

void rrdeng_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every) {
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)sch;
    check_and_fix_mrg_update_every(handle);

    METRIC *metric = handle->metric;
    usec_t update_every_ut = (usec_t)update_every * USEC_PER_SEC;

    if(update_every_ut == handle->update_every_ut)
        return;

    handle->page_flags |= RRDENG_PAGE_UPDATE_EVERY_CHANGE;
    rrdeng_store_metric_flush_current_page(sch);
    mrg_metric_set_update_every(main_mrg, metric, update_every);
    handle->update_every_ut = update_every_ut;
}

// ----------------------------------------------------------------------------
// query ops

#ifdef NETDATA_INTERNAL_CHECKS
SPINLOCK global_query_handle_spinlock = SPINLOCK_INITIALIZER;
static struct rrdeng_query_handle *global_query_handle_ll = NULL;
static ALWAYS_INLINE void register_query_handle(struct rrdeng_query_handle *handle) {
    handle->query_pid = gettid_cached();
    handle->started_time_s = now_realtime_sec();

    spinlock_lock(&global_query_handle_spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(global_query_handle_ll, handle, prev, next);
    spinlock_unlock(&global_query_handle_spinlock);
}
static ALWAYS_INLINE void unregister_query_handle(struct rrdeng_query_handle *handle) {
    spinlock_lock(&global_query_handle_spinlock);
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(global_query_handle_ll, handle, prev, next);
    spinlock_unlock(&global_query_handle_spinlock);
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
ALWAYS_INLINE_HOT void rrdeng_load_metric_init(
    STORAGE_METRIC_HANDLE *smh,
    struct storage_engine_query_handle *seqh,
    time_t start_time_s,
    time_t end_time_s,
    STORAGE_PRIORITY priority)
{
    usec_t started_ut = now_monotonic_usec();

    METRIC *metric = (METRIC *)smh;
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

    time_t db_first_time_s, db_last_time_s;
    uint32_t db_update_every_s;
    mrg_metric_get_retention(main_mrg, metric, &db_first_time_s, &db_last_time_s, &db_update_every_s);

    if(is_page_in_time_range(start_time_s, end_time_s, db_first_time_s, db_last_time_s) == PAGE_IS_IN_RANGE) {
        handle->start_time_s = MAX(start_time_s, db_first_time_s);
        handle->end_time_s = MIN(end_time_s, db_last_time_s);
        handle->now_s = handle->start_time_s;

        handle->dt_s = db_update_every_s;
        if (!handle->dt_s) {
            handle->dt_s = nd_profile.update_every;
            mrg_metric_set_update_every_s_if_zero(main_mrg, metric, nd_profile.update_every);
        }

        seqh->handle = (STORAGE_QUERY_HANDLE *) handle;
        seqh->start_time_s = handle->start_time_s;
        seqh->end_time_s = handle->end_time_s;
        seqh->priority = priority;
        seqh->seb = STORAGE_ENGINE_BACKEND_DBENGINE;

        pg_cache_preload(handle);

        time_and_count_add(&rrdeng_cache_efficiency_stats.query_time_init, now_monotonic_usec() - started_ut);
    }
    else {
        handle->start_time_s = start_time_s;
        handle->end_time_s = end_time_s;
        handle->now_s = start_time_s;
        handle->dt_s = db_update_every_s;

        seqh->handle = (STORAGE_QUERY_HANDLE *) handle;
        seqh->start_time_s = handle->start_time_s;
        seqh->end_time_s = 0;
        seqh->priority = priority;
        seqh->seb = STORAGE_ENGINE_BACKEND_DBENGINE;
    }
}

static ALWAYS_INLINE_HOT bool rrdeng_load_page_next(struct storage_engine_query_handle *seqh, bool debug_this __maybe_unused) {
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)seqh->handle;
    struct rrdengine_instance *ctx = mrg_metric_ctx(handle->metric);

    if (likely(handle->page)) {
        // we have a page to release
        pgc_page_release(main_cache, handle->page);
        handle->page = NULL;
        pgdc_reset(&handle->pgdc, NULL, UINT32_MAX);
    }

    if (unlikely(handle->now_s > seqh->end_time_s))
        return false;

    size_t entries = 0;
    handle->page = pg_cache_lookup_next(ctx, handle->pdc, handle->now_s, handle->dt_s, &entries);

    internal_fatal(handle->page && (pgc_page_data(handle->page) == PGD_EMPTY || !entries),
                   "A page was returned, but it is empty - pg_cache_lookup_next() should be handling this case");

    if (unlikely(!handle->page || pgc_page_data(handle->page) == PGD_EMPTY || !entries))
        return false;

    time_t page_start_time_s = pgc_page_start_time_s(handle->page);
    time_t page_end_time_s = pgc_page_end_time_s(handle->page);
    uint32_t page_update_every_s = pgc_page_update_every_s(handle->page);

    unsigned position;
    if(likely(handle->now_s >= page_start_time_s && handle->now_s <= page_end_time_s)) {

        if(unlikely(entries == 1 || page_start_time_s == page_end_time_s || !page_update_every_s)) {
            position = 0;
            handle->now_s = page_start_time_s;
        }
        else {
            position = (handle->now_s - page_start_time_s) * (entries - 1) / (page_end_time_s - page_start_time_s);
            time_t point_end_time_s = page_start_time_s + position * (time_t) page_update_every_s;
            while(point_end_time_s < handle->now_s && position + 1 < entries) {
                // https://github.com/netdata/netdata/issues/14411
                // we really need a while() here, because the delta may be
                // 2 points at higher tiers
                position++;
                point_end_time_s = page_start_time_s + position * (time_t) page_update_every_s;
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
    handle->dt_s = page_update_every_s;

    pgdc_reset(&handle->pgdc, pgc_page_data(handle->page), handle->position);

    return true;
}

// Returns the metric and sets its timestamp into current_time
// IT IS REQUIRED TO **ALWAYS** SET ALL RETURN VALUES (current_time, end_time, flags)
// IT IS REQUIRED TO **ALWAYS** KEEP TRACK OF TIME, EVEN OUTSIDE THE DATABASE BOUNDARIES
ALWAYS_INLINE_HOT STORAGE_POINT rrdeng_load_metric_next(struct storage_engine_query_handle *seqh) {
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)seqh->handle;
    STORAGE_POINT sp;

    if (unlikely(handle->now_s > seqh->end_time_s)) {
        storage_point_empty(sp, handle->now_s - handle->dt_s, handle->now_s);
        goto prepare_for_next_iteration;
    }

    if (unlikely(!handle->page || handle->position >= handle->entries)) {
        // We need to get a new page

        if (!rrdeng_load_page_next(seqh, false)) {
            handle->now_s = seqh->end_time_s;
            storage_point_empty(sp, handle->now_s - handle->dt_s, handle->now_s);
            goto prepare_for_next_iteration;
        }
    }

    sp.start_time_s = handle->now_s - handle->dt_s;
    sp.end_time_s = handle->now_s;

    pgdc_get_next_point(&handle->pgdc, handle->position, &sp);

prepare_for_next_iteration:
    // internal_fatal(sp.end_time_s < seqh->start_time_s, "DBENGINE: this point is too old for this query");
    internal_fatal(sp.end_time_s < handle->now_s, "DBENGINE: this point is too old for this point in time");

    handle->now_s += handle->dt_s;
    handle->position++;

    return sp;
}

ALWAYS_INLINE int rrdeng_load_metric_is_finished(struct storage_engine_query_handle *seqh) {
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)seqh->handle;
    return (handle->now_s > seqh->end_time_s);
}

/*
 * Releases the database reference from the handle for loading metrics.
 */
ALWAYS_INLINE void rrdeng_load_metric_finalize(struct storage_engine_query_handle *seqh)
{
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)seqh->handle;

    if (handle->page) {
        pgc_page_release(main_cache, handle->page);
        pgdc_reset(&handle->pgdc, NULL, UINT32_MAX);
    }

    if(handle->pdc) {
        __atomic_store_n(&handle->pdc->workers_should_stop, true, __ATOMIC_RELAXED);
        pdc_release_and_destroy_if_unreferenced(handle->pdc, false, false);
    }

    unregister_query_handle(handle);
    rrdeng_query_handle_release(handle);
    seqh->handle = NULL;
}

ALWAYS_INLINE time_t rrdeng_load_align_to_optimal_before(struct storage_engine_query_handle *seqh) {
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)seqh->handle;

    if(handle->pdc) {
        rrdeng_prep_wait(handle->pdc);
        if (handle->pdc->optimal_end_time_s > seqh->end_time_s)
            seqh->end_time_s = handle->pdc->optimal_end_time_s;
    }

    return seqh->end_time_s;
}

ALWAYS_INLINE time_t rrdeng_metric_latest_time(STORAGE_METRIC_HANDLE *smh) {
    METRIC *metric = (METRIC *)smh;
    time_t latest_time_s = 0;

    if (metric)
        latest_time_s = mrg_metric_get_latest_time_s(main_mrg, metric);

    return latest_time_s;
}

ALWAYS_INLINE time_t rrdeng_metric_oldest_time(STORAGE_METRIC_HANDLE *smh) {
    METRIC *metric = (METRIC *)smh;

    time_t oldest_time_s = 0;
    if (metric)
        oldest_time_s = mrg_metric_get_first_time_s(main_mrg, metric);

    return oldest_time_s;
}

bool rrdeng_metric_retention_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *dim_uuid, time_t *first_entry_s, time_t *last_entry_s) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    if (unlikely(!ctx)) {
        netdata_log_error("DBENGINE: invalid STORAGE INSTANCE to %s()", __FUNCTION__);
        return false;
    }

    METRIC *metric = mrg_metric_get_and_acquire_by_uuid(main_mrg, dim_uuid, (Word_t)ctx);
    if (unlikely(!metric))
        return false;

    mrg_metric_get_retention(main_mrg, metric, first_entry_s, last_entry_s, NULL);

    mrg_metric_release(main_mrg, metric);

    return true;
}

bool rrdeng_metric_retention_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id, time_t *first_entry_s, time_t *last_entry_s) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    if (unlikely(!ctx)) {
        netdata_log_error("DBENGINE: invalid STORAGE INSTANCE to %s()", __FUNCTION__);
        return false;
    }

    METRIC *metric = mrg_metric_get_and_acquire_by_id(main_mrg, id, (Word_t)ctx);
    if (unlikely(!metric))
        return false;

    mrg_metric_get_retention(main_mrg, metric, first_entry_s, last_entry_s, NULL);

    mrg_metric_release(main_mrg, metric);

    return true;
}

void rrdeng_metric_retention_delete_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    if (unlikely(!ctx)) {
        netdata_log_error("DBENGINE: invalid STORAGE INSTANCE to %s()", __FUNCTION__);
        return;
    }

    METRIC *metric = mrg_metric_get_and_acquire_by_id(main_mrg, id, (Word_t)ctx);
    if (unlikely(!metric))
        return;

    mrg_metric_clear_retention(main_mrg, metric);
    mrg_metric_release(main_mrg, metric);
}

uint64_t rrdeng_disk_space_max(STORAGE_INSTANCE *si) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    return ctx->config.max_disk_space;
}

uint64_t rrdeng_disk_space_used(STORAGE_INSTANCE *si) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    return __atomic_load_n(&ctx->atomic.current_disk_space, __ATOMIC_RELAXED);
}

uint64_t rrdeng_metrics(STORAGE_INSTANCE *si) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    return __atomic_load_n(&ctx->atomic.metrics, __ATOMIC_RELAXED);
}

uint64_t rrdeng_samples(STORAGE_INSTANCE *si) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    return __atomic_load_n(&ctx->atomic.samples, __ATOMIC_RELAXED);
}

time_t rrdeng_global_first_time_s(STORAGE_INSTANCE *si) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;

    time_t t = __atomic_load_n(&ctx->atomic.first_time_s, __ATOMIC_RELAXED);
    if(t == LONG_MAX || t < 0)
        t = 0;

    return t;
}

size_t rrdeng_currently_collected_metrics(STORAGE_INSTANCE *si) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    return __atomic_load_n(&ctx->atomic.collectors_running, __ATOMIC_RELAXED);
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
    array[30] = (uint64_t)__atomic_load_n(&global_stats.global_io_errors, __ATOMIC_RELAXED); // used
    array[31] = (uint64_t)__atomic_load_n(&global_stats.global_fs_errors, __ATOMIC_RELAXED); // used
    array[32] = (uint64_t)__atomic_load_n(&global_stats.rrdeng_reserved_file_descriptors, __ATOMIC_RELAXED); // used
    array[33] = 0; // (uint64_t)__atomic_load_n(&ctx->stats.pg_cache_over_half_dirty_events, __ATOMIC_RELAXED);
    array[34] = (uint64_t)__atomic_load_n(&global_stats.global_pg_cache_over_half_dirty_events, __ATOMIC_RELAXED); // used
    array[35] = 0; // (uint64_t)__atomic_load_n(&ctx->stats.flushing_pressure_page_deletions, __ATOMIC_RELAXED);
    array[36] = (uint64_t)__atomic_load_n(&global_stats.global_flushing_pressure_page_deletions, __ATOMIC_RELAXED); // used
    array[37] = 0; //(uint64_t)pg_cache->active_descriptors;

    fatal_assert(RRDENG_NR_STATS == 38);
}

static void rrdeng_populate_mrg(struct rrdengine_instance *ctx)
{
    size_t datafiles = datafile_count(ctx, false);

    ssize_t cpus = (ssize_t)netdata_conf_cpus();
    if(cpus < 1)
        cpus = 1;

    netdata_log_info("DBENGINE: populating retention to MRG from %zu journal files of tier %d, using a shared pool of %zd threads...", datafiles, ctx->config.tier, cpus);

    completion_init(&ctx->loading.load_mrg);
    rrdeng_enq_cmd(
        ctx,
        RRDENG_OPCODE_CTX_POPULATE_MRG,
        NULL,
        &ctx->loading.load_mrg,
        STORAGE_PRIORITY_INTERNAL_DBENGINE,
        NULL,
        NULL);
}

void rrdeng_readiness_wait(struct rrdengine_instance *ctx) {
    completion_wait_for(&ctx->loading.load_mrg);
    completion_destroy(&ctx->loading.load_mrg);

    if(__atomic_load_n(&ctx->atomic.first_time_s, __ATOMIC_RELAXED) == LONG_MAX)
        __atomic_store_n(&ctx->atomic.first_time_s, now_realtime_sec(), __ATOMIC_RELAXED);

    netdata_log_info("DBENGINE: tier %d is ready for data collection and queries", ctx->config.tier);
}

/*
 * Returns 0 on success, negative on error
 */
int rrdeng_init(
    struct rrdengine_instance **ctxp,
    const char *dbfiles_path,
    unsigned disk_space_mb,
    size_t tier,
    time_t max_retention_s)
{
    struct rrdengine_instance *ctx;
    uint32_t max_open_files;

    max_open_files = rlimit_nofile.rlim_cur / 4;

    /* reserve RRDENG_FD_BUDGET_PER_INSTANCE file descriptors for this instance */
    rrd_stat_atomic_add(&global_stats.rrdeng_reserved_file_descriptors, RRDENG_FD_BUDGET_PER_INSTANCE);
    if (global_stats.rrdeng_reserved_file_descriptors > max_open_files) {
        netdata_log_error(
            "Exceeded the budget of available file descriptors (%u/%u), cannot create new dbengine instance.",
            (unsigned)global_stats.rrdeng_reserved_file_descriptors,
            (unsigned)max_open_files);

        rrd_stat_atomic_add(&global_stats.global_fs_errors, 1);
        rrd_stat_atomic_add(&global_stats.rrdeng_reserved_file_descriptors, -RRDENG_FD_BUDGET_PER_INSTANCE);
        return UV_EMFILE;
    }

    if(ctxp) {
        *ctxp = ctx = mallocz(sizeof(*ctx));
        initialize_single_ctx(ctx);
    }
    else
        ctx = multidb_ctx[tier];

    ctx->config.tier = (int)tier;
    ctx->config.page_type = tier_page_type[tier];
    ctx->config.global_compress_alg = dbengine_default_compression();

    strncpyz(ctx->config.dbfiles_path, dbfiles_path, sizeof(ctx->config.dbfiles_path) - 1);
    ctx->config.dbfiles_path[sizeof(ctx->config.dbfiles_path) - 1] = '\0';

    if (disk_space_mb && disk_space_mb < RRDENG_MIN_DISK_SPACE_MB)
        disk_space_mb = RRDENG_MIN_DISK_SPACE_MB;

    ctx->config.max_disk_space = disk_space_mb * 1048576LLU;

    ctx->config.max_retention_s = max_retention_s;

    ctx->atomic.transaction_id = 1;
    ctx->quiesce.enabled = false;

    ctx->atomic.first_time_s = LONG_MAX;
    ctx->atomic.metrics = 0;
    ctx->atomic.samples = 0;

    if (rrdeng_dbengine_spawn(ctx) && !init_rrd_files(ctx)) {
        // success - we run this ctx too
        rrdeng_populate_mrg(ctx);
        return 0;
    }

    if (unittest_running) {
        freez(ctx);
        if (ctxp)
            *ctxp = NULL;
    }

    rrd_stat_atomic_add(&global_stats.rrdeng_reserved_file_descriptors, -RRDENG_FD_BUDGET_PER_INSTANCE);
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
    size_t count = 10;
    while(__atomic_load_n(&ctx->atomic.collectors_running, __ATOMIC_RELAXED) && count && !unittest_running) {
        if(!logged) {
            netdata_log_info("DBENGINE: waiting for collectors to finish on tier %d...", ctx->config.tier);
            logged = true;
        }
        sleep_usec(100 * USEC_PER_MS);
        count--;
    }

    pgc_flush_all_hot_and_dirty_pages(main_cache, (Word_t)ctx);

    struct completion completion = {};
    completion_init(&completion);
    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_CTX_SHUTDOWN, NULL, &completion, STORAGE_PRIORITY_BEST_EFFORT, NULL, NULL);

    completion_wait_for(&completion);
    completion_destroy(&completion);

    if(unittest_running)
        freez(ctx);

    rrd_stat_atomic_add(&global_stats.rrdeng_reserved_file_descriptors, -RRDENG_FD_BUDGET_PER_INSTANCE);
    return 0;
}

void rrdeng_flush_dirty(struct rrdengine_instance *ctx)
{
    if (NULL == ctx)
        return;

    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_CTX_FLUSH_DIRTY, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
}

void rrdeng_flush_all(struct rrdengine_instance *ctx)
{
    if (NULL == ctx)
        return;

    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_CTX_FLUSH_HOT_DIRTY, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
}

void rrdeng_quiesce(struct rrdengine_instance *ctx)
{
    if (NULL == ctx)
        return;

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

            size_t points = descr->page_length / CTX_POINT_SIZE_BYTES(datafile_ctx(datafile));

            time_t start_time_s = journal_start_time_s + descr->delta_start_s;
            time_t end_time_s = journal_start_time_s + descr->delta_end_s;

            if(likely(points > 1))
                update_every_s = (time_t) ((end_time_s - start_time_s) / (points - 1));
            else {
                update_every_s = (time_t) (nd_profile.update_every * get_tier_grouping(datafile_ctx(datafile)->config.tier));
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
    struct rrdengine_datafile *df = NULL;

    while ((df = get_next_datafile(df, ctx, true))) {
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
    stats.sizeof_datafile =
        natural_alignment(sizeof(struct rrdengine_datafile)) +
                            natural_alignment(sizeof(struct rrdengine_journalfile));
    stats.sizeof_page_in_cache = 0; // struct_natural_alignment(sizeof(struct page_cache_descr));
    stats.sizeof_point_data = page_type_size[ctx->config.page_type];
    stats.sizeof_page_data = tier_page_size[ctx->config.tier];
    stats.pages_per_extent = rrdeng_pages_per_extent;

//    stats.sizeof_metric_in_index = 40;
//    stats.sizeof_page_in_index = 24;

    stats.default_granularity_secs = (size_t)nd_profile.update_every * get_tier_grouping(ctx->config.tier);

    return stats;
}

struct rrdeng_cache_efficiency_stats rrdeng_get_cache_efficiency_stats(void) {
    // FIXME - make cache efficiency stats atomic
    return rrdeng_cache_efficiency_stats;
}
