// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"
#include "../storage_engine.h"

/* Default global database instance */
struct rrdengine_instance multidb_ctx_storage_tier0;
struct rrdengine_instance multidb_ctx_storage_tier1;
struct rrdengine_instance multidb_ctx_storage_tier2;
struct rrdengine_instance multidb_ctx_storage_tier3;
struct rrdengine_instance multidb_ctx_storage_tier4;
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
int default_rrdeng_disk_quota_mb = 256;
int default_multidb_disk_quota_mb = 256;
/* Default behaviour is to unblock data collection if the page cache is full of dirty pages by dropping metrics */
uint8_t rrdeng_drop_metrics_under_page_cache_pressure = 1;

// ----------------------------------------------------------------------------
// metrics groups

STORAGE_METRICS_GROUP *rrdeng_metrics_group_get(STORAGE_INSTANCE *db_instance __maybe_unused, uuid_t *uuid __maybe_unused) {
    return callocz(1, sizeof(struct pg_alignment));
}

void rrdeng_metrics_group_release(STORAGE_INSTANCE *db_instance, STORAGE_METRICS_GROUP *smg) {
    if(!smg) return;

    struct rrdengine_instance *ctx = (struct rrdengine_instance *)db_instance;
    struct pg_alignment *pa = (struct pg_alignment *)smg;
    struct page_cache *pg_cache = &ctx->pg_cache;

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    if(pa->refcount == 0)
        freez(pa);
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
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

STORAGE_METRIC_HANDLE *rrdeng_metric_get_legacy(STORAGE_INSTANCE *db_instance, const char *rd_id, const char *st_id, STORAGE_METRICS_GROUP *smg) {
    uuid_t legacy_uuid;
    rrdeng_generate_legacy_uuid(rd_id, st_id, &legacy_uuid);
    return rrdeng_metric_get(db_instance, &legacy_uuid, smg);
}

// ----------------------------------------------------------------------------
// metric handle

void rrdeng_metric_release(STORAGE_METRIC_HANDLE *db_metric_handle) {
    struct pg_cache_page_index *page_index = (struct pg_cache_page_index *)db_metric_handle;

    unsigned short refcount = __atomic_sub_fetch(&page_index->refcount, 1, __ATOMIC_SEQ_CST);
    if(refcount == 0 && page_index->alignment) {
        __atomic_sub_fetch(&page_index->alignment->refcount, 1, __ATOMIC_SEQ_CST);
        page_index->alignment = NULL;
    }
}

STORAGE_METRIC_HANDLE *rrdeng_metric_dup(STORAGE_METRIC_HANDLE *db_metric_handle) {
    struct pg_cache_page_index *page_index = (struct pg_cache_page_index *)db_metric_handle;
    __atomic_add_fetch(&page_index->refcount, 1, __ATOMIC_SEQ_CST);
    return db_metric_handle;
}

STORAGE_METRIC_HANDLE *rrdeng_metric_get(STORAGE_INSTANCE *db_instance, uuid_t *uuid, STORAGE_METRICS_GROUP *smg) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)db_instance;
    struct pg_alignment *pa = (struct pg_alignment *)smg;
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct pg_cache_page_index *page_index = NULL;

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    Pvoid_t *PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, uuid, sizeof(uuid_t));
    if (likely(NULL != PValue))
        page_index = *PValue;
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);

    if (likely(page_index)) {
        __atomic_add_fetch(&page_index->refcount, 1, __ATOMIC_SEQ_CST);

        if(pa) {
            if(page_index->alignment && page_index->alignment != pa && page_index->writers > 0)
                fatal("DBENGINE: page_index has a different alignment (page_index refcount is %u, writers is %u).",
                        page_index->refcount, page_index->writers);

            page_index->alignment = pa;
            __atomic_add_fetch(&pa->refcount, 1, __ATOMIC_SEQ_CST);
        }
    }

    return (STORAGE_METRIC_HANDLE *)page_index;
}

STORAGE_METRIC_HANDLE *rrdeng_metric_create(STORAGE_INSTANCE *db_instance, uuid_t *uuid, STORAGE_METRICS_GROUP *smg) {
    internal_fatal(!db_instance, "DBENGINE: db_instance is NULL");

    struct rrdengine_instance *ctx = (struct rrdengine_instance *)db_instance;
    struct pg_alignment *pa = (struct pg_alignment *)smg;
    struct pg_cache_page_index *page_index;
    struct page_cache *pg_cache = &ctx->pg_cache;

    uv_rwlock_wrlock(&pg_cache->metrics_index.lock);
    Pvoid_t *PValue = JudyHSIns(&pg_cache->metrics_index.JudyHS_array, uuid, sizeof(uuid_t), PJE0);
    fatal_assert(NULL == *PValue); /* TODO: figure out concurrency model */
    *PValue = page_index = create_page_index(uuid, ctx);
    page_index->prev = pg_cache->metrics_index.last_page_index;
    pg_cache->metrics_index.last_page_index = page_index;
    page_index->alignment = pa;
    page_index->refcount = 1;
    if(pa)
        pa->refcount++;
    uv_rwlock_wrunlock(&pg_cache->metrics_index.lock);

    return (STORAGE_METRIC_HANDLE *)page_index;
}

STORAGE_METRIC_HANDLE *rrdeng_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *db_instance, STORAGE_METRICS_GROUP *smg) {
    STORAGE_METRIC_HANDLE *db_metric_handle;

    db_metric_handle = rrdeng_metric_get(db_instance, &rd->metric_uuid, smg);
    if(!db_metric_handle) {
        db_metric_handle = rrdeng_metric_get_legacy(db_instance, rrddim_id(rd), rrdset_id(rd->rrdset), smg);
        if(db_metric_handle) {
            struct pg_cache_page_index *page_index = (struct pg_cache_page_index *)db_metric_handle;
            uuid_copy(rd->metric_uuid, page_index->id);
        }
    }
    if(!db_metric_handle)
        db_metric_handle = rrdeng_metric_create(db_instance, &rd->metric_uuid, smg);

#ifdef NETDATA_INTERNAL_CHECKS
    struct pg_cache_page_index *page_index = (struct pg_cache_page_index *)db_metric_handle;
    if(uuid_compare(rd->metric_uuid, page_index->id) != 0) {
        char uuid1[UUID_STR_LEN + 1];
        char uuid2[UUID_STR_LEN + 1];

        uuid_unparse(rd->metric_uuid, uuid1);
        uuid_unparse(page_index->id, uuid2);
        fatal("DBENGINE: uuids do not match, asked for metric '%s', but got page_index of metric '%s'", uuid1, uuid2);
    }

    struct rrdengine_instance *ctx = (struct rrdengine_instance *)db_instance;
    if(page_index->ctx != ctx)
        fatal("DBENGINE: mixed up rrdengine instances, asked for metric from %p, got from %p", ctx, page_index->ctx);
#endif

    return db_metric_handle;
}


// ----------------------------------------------------------------------------
// collect ops

/*
 * Gets a handle for storing metrics to the database.
 * The handle must be released with rrdeng_store_metric_final().
 */
STORAGE_COLLECT_HANDLE *rrdeng_store_metric_init(STORAGE_METRIC_HANDLE *db_metric_handle, uint32_t update_every) {
    struct pg_cache_page_index *page_index = (struct pg_cache_page_index *)db_metric_handle;
    struct rrdeng_collect_handle *handle;

    if(!page_index->alignment)
        fatal("DBENGINE: metric group is required for collect operations");

    handle = callocz(1, sizeof(struct rrdeng_collect_handle));
    handle->page_index = page_index;
    handle->descr = NULL;
    handle->unaligned_page = 0;
    page_index->latest_update_every_s = update_every;

    uv_rwlock_wrlock(&page_index->lock);
    ++page_index->writers;
    uv_rwlock_wrunlock(&page_index->lock);

    return (STORAGE_COLLECT_HANDLE *)handle;
}

/* The page must be populated and referenced */
static int page_has_only_empty_metrics(struct rrdeng_page_descr *descr)
{
    switch(descr->type) {
        case PAGE_METRICS: {
            size_t slots = descr->page_length / PAGE_POINT_SIZE_BYTES(descr);
            storage_number *array = (storage_number *)descr->pg_cache_descr->page;
            for (size_t i = 0 ; i < slots; ++i) {
                if(does_storage_number_exist(array[i]))
                    return 0;
            }
        }
        break;

        case PAGE_TIER: {
            size_t slots = descr->page_length / PAGE_POINT_SIZE_BYTES(descr);
            storage_number_tier1_t *array = (storage_number_tier1_t *)descr->pg_cache_descr->page;
            for (size_t i = 0 ; i < slots; ++i) {
                if(fpclassify(array[i].sum_value) != FP_NAN)
                    return 0;
            }
        }
        break;

        default: {
            static bool logged = false;
            if(!logged) {
                error("DBENGINE: cannot check page for nulls on unknown page type id %d", descr->type);
                logged = true;
            }
            return 0;
        }
    }

    return 1;
}

void rrdeng_store_metric_flush_current_page(STORAGE_COLLECT_HANDLE *collection_handle) {
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)collection_handle;
    // struct rrdeng_metric_handle *metric_handle = (struct rrdeng_metric_handle *)handle->metric_handle;
    struct rrdengine_instance *ctx = handle->page_index->ctx;
    struct rrdeng_page_descr *descr = handle->descr;

    if (unlikely(!ctx)) return;
    if (unlikely(!descr)) return;

    if (likely(descr->page_length)) {
        int page_is_empty;

        rrd_stat_atomic_add(&ctx->stats.metric_API_producers, -1);

        page_is_empty = page_has_only_empty_metrics(descr);
        if (page_is_empty) {
            print_page_cache_descr(descr, "Page has empty metrics only, deleting", true);
            pg_cache_put(ctx, descr);
            pg_cache_punch_hole(ctx, descr, 1, 0, NULL, true);
        } else
            rrdeng_commit_page(ctx, descr, handle->page_correlation_id);
    } else {
        dbengine_page_free(descr->pg_cache_descr->page);
        rrdeng_destroy_pg_cache_descr(ctx, descr->pg_cache_descr);
        rrdeng_page_descr_freez(descr);
    }
    handle->descr = NULL;
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
    struct pg_cache_page_index *page_index = handle->page_index;
    struct rrdengine_instance *ctx = handle->page_index->ctx;
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_descr *descr = handle->descr;

    void *page;
    uint8_t must_flush_unaligned_page = 0, perfect_page_alignment = 0;

    if (descr) {
        /* Make alignment decisions */

#ifdef NETDATA_INTERNAL_CHECKS
        if(descr->end_time_ut + page_index->latest_update_every_s * USEC_PER_SEC != point_in_time_ut) {
            char buffer[200 + 1];
            snprintfz(buffer, 200,
                      "metrics collected are %s, end_time_ut = %llu, point_in_time_ut = %llu, update_every = %u, delta = %llu",
                      (point_in_time_ut / USEC_PER_SEC - descr->end_time_ut / USEC_PER_SEC > page_index->latest_update_every_s)?"far apart":"not aligned",
                      descr->end_time_ut / USEC_PER_SEC,
                      point_in_time_ut / USEC_PER_SEC,
                      page_index->latest_update_every_s,
                      point_in_time_ut / USEC_PER_SEC - descr->end_time_ut / USEC_PER_SEC);
            print_page_cache_descr(descr, buffer, false);
        }
#endif

        if (descr->page_length == page_index->alignment->page_length) {
            /* this is the leading dimension that defines chart alignment */
            perfect_page_alignment = 1;
        }
        /* is the metric far enough out of alignment with the others? */
        if (unlikely(descr->page_length + PAGE_POINT_SIZE_BYTES(descr) < page_index->alignment->page_length)) {
            handle->unaligned_page = 1;
            print_page_cache_descr(descr, "Metric page is not aligned with chart", true);
        }
        if (unlikely(handle->unaligned_page &&
                     /* did the other metrics change page? */
                     page_index->alignment->page_length <= PAGE_POINT_SIZE_BYTES(descr))) {
            print_page_cache_descr(descr, "must_flush_unaligned_page = 1", true);
            must_flush_unaligned_page = 1;
            handle->unaligned_page = 0;
        }
    }
    if (unlikely(NULL == descr ||
                 descr->page_length + PAGE_POINT_SIZE_BYTES(descr) > RRDENG_BLOCK_SIZE ||
                 must_flush_unaligned_page)) {

        if(descr) {
            print_page_cache_descr(descr, "flushing metric", true);
            rrdeng_store_metric_flush_current_page(collection_handle);
        }

        page = rrdeng_create_page(ctx, &page_index->id, &descr);
        fatal_assert(page);

        descr->update_every_s = page_index->latest_update_every_s;
        handle->descr = descr;

        handle->page_correlation_id = rrd_atomic_fetch_add(&pg_cache->committed_page_index.latest_corr_id, 1);

        if (0 == page_index->alignment->page_length) {
            /* this is the leading dimension that defines chart alignment */
            perfect_page_alignment = 1;
        }
    }

    page = descr->pg_cache_descr->page;

    switch (descr->type) {
        case PAGE_METRICS: {
            ((storage_number *)page)[descr->page_length / PAGE_POINT_SIZE_BYTES(descr)] = pack_storage_number(n, flags);
        }
        break;

        case PAGE_TIER: {
            storage_number_tier1_t number_tier1;
            number_tier1.sum_value = (float)n;
            number_tier1.min_value = (float)min_value;
            number_tier1.max_value = (float)max_value;
            number_tier1.anomaly_count = anomaly_count;
            number_tier1.count = count;
            ((storage_number_tier1_t *)page)[descr->page_length / PAGE_POINT_SIZE_BYTES(descr)] = number_tier1;
        }
        break;

        default: {
            static bool logged = false;
            if(!logged) {
                error("DBENGINE: cannot store metric on unknown page type id %d", descr->type);
                logged = true;
            }
        }
        break;
    }

    pg_cache_atomic_set_pg_info(descr, point_in_time_ut, descr->page_length + PAGE_POINT_SIZE_BYTES(descr));

    if (perfect_page_alignment)
        page_index->alignment->page_length = descr->page_length;
    if (unlikely(INVALID_TIME == descr->start_time_ut)) {
        unsigned long new_metric_API_producers, old_metric_API_max_producers, ret_metric_API_max_producers;
        descr->start_time_ut = point_in_time_ut;

        new_metric_API_producers = rrd_atomic_add_fetch(&ctx->stats.metric_API_producers, 1);
        while (unlikely(new_metric_API_producers > (old_metric_API_max_producers = ctx->metric_API_max_producers))) {
            /* Increase ctx->metric_API_max_producers */
            ret_metric_API_max_producers = ulong_compare_and_swap(&ctx->metric_API_max_producers,
                                                                  old_metric_API_max_producers,
                                                                  new_metric_API_producers);
            if (old_metric_API_max_producers == ret_metric_API_max_producers) {
                /* success */
                break;
            }
        }

        pg_cache_insert(ctx, page_index, descr, true);
    } else {
        pg_cache_add_new_metric_time(page_index, descr);
    }

//    {
//        unsigned char u[16] = { 0x0C, 0x0A, 0x40, 0xD6, 0x2A, 0x43, 0x4A, 0x7C, 0x95, 0xF7, 0xD1, 0x1E, 0x0C, 0x9E, 0x8A, 0xE7 };
//        if(uuid_compare(u, page_index->id) == 0) {
//            char buffer[100];
//            snprintfz(buffer, 100, "store system.cpu, collect:%u, page_index first:%u, last:%u",
//                      (uint32_t)(point_in_time / USEC_PER_SEC),
//                      (uint32_t)(page_index->oldest_time / USEC_PER_SEC),
//                      (uint32_t)(page_index->latest_time / USEC_PER_SEC));
//
//            print_page_cache_descr(descr, buffer, false);
//        }
//    }
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
    struct pg_cache_page_index *page_index = handle->page_index;
    struct rrdeng_page_descr *descr = handle->descr;

    if(likely(descr)) {
        usec_t last_point_in_time_ut = descr->end_time_ut;
        usec_t update_every_ut = page_index->latest_update_every_s * USEC_PER_SEC;
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

            size_t point_size = PAGE_POINT_SIZE_BYTES(descr);
            size_t page_size_in_points = RRDENG_BLOCK_SIZE / point_size;
            size_t used_points = descr->page_length / point_size;
            size_t remaining_points_in_page = page_size_in_points - used_points;

            bool new_point_is_aligned = true;
            if(unlikely((point_in_time_ut - last_point_in_time_ut) / points_gap != update_every_ut))
                new_point_is_aligned = false;

            if(unlikely(points_gap > remaining_points_in_page || !new_point_is_aligned)) {
//                char buffer[200];
//                snprintfz(buffer, 200, "data collection skipped %zu points, last stored point %llu, new point %llu, update every %d. Cutting page.",
//                      points_gap, last_point_in_time_ut / USEC_PER_SEC, point_in_time_ut / USEC_PER_SEC, page_index->latest_update_every_s);
//                print_page_cache_descr(descr, buffer, false);

                rrdeng_store_metric_flush_current_page(collection_handle);
            }
            else {
//                char buffer[200];
//                snprintfz(buffer, 200, "data collection skipped %zu points, last stored point %llu, new point %llu, update every %d. Filling the gap.",
//                      points_gap, last_point_in_time_ut / USEC_PER_SEC, point_in_time_ut / USEC_PER_SEC, page_index->latest_update_every_s);
//                print_page_cache_descr(descr, buffer, false);

                // loop to fill the gap
                usec_t step_ut = page_index->latest_update_every_s * USEC_PER_SEC;
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
    struct pg_cache_page_index *page_index = handle->page_index;

    uint8_t can_delete_metric = 0;

    rrdeng_store_metric_flush_current_page(collection_handle);
    uv_rwlock_wrlock(&page_index->lock);
    if (!--page_index->writers && !page_index->page_count) {
        can_delete_metric = 1;
    }
    uv_rwlock_wrunlock(&page_index->lock);
    freez(handle);

    return can_delete_metric;
}

void rrdeng_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *collection_handle, int update_every) {
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)collection_handle;
    struct pg_cache_page_index *page_index = handle->page_index;
    rrdeng_store_metric_flush_current_page(collection_handle);
    uv_rwlock_rdlock(&page_index->lock);
    page_index->latest_update_every_s = update_every;
    uv_rwlock_rdunlock(&page_index->lock);
}

// ----------------------------------------------------------------------------
// query ops

//static inline uint32_t *pginfo_to_dt(struct rrdeng_page_info *page_info)
//{
//    return (uint32_t *)&page_info->scratch[0];
//}
//
//static inline uint32_t *pginfo_to_points(struct rrdeng_page_info *page_info)
//{
//    return (uint32_t *)&page_info->scratch[sizeof(uint32_t)];
//}
//
/*
 * Gets a handle for loading metrics from the database.
 * The handle must be released with rrdeng_load_metric_final().
 */
void rrdeng_load_metric_init(STORAGE_METRIC_HANDLE *db_metric_handle, struct storage_engine_query_handle *rrdimm_handle, time_t start_time_s, time_t end_time_s)
{
    struct pg_cache_page_index *page_index = (struct pg_cache_page_index *)db_metric_handle;
    struct rrdengine_instance *ctx = page_index->ctx;

    // fprintf(stderr, "%s: %s/%s start time %ld, end time %ld\n", __FUNCTION__ , rd->rrdset->name, rd->name, start_time, end_time);

    struct rrdeng_query_handle *handle;
    unsigned pages_nr;

    if(!page_index->latest_update_every_s)
        page_index->latest_update_every_s = default_rrd_update_every;

    rrdimm_handle->start_time_s = start_time_s;
    rrdimm_handle->end_time_s = end_time_s;

    handle = callocz(1, sizeof(struct rrdeng_query_handle));
    handle->wanted_start_time_s = start_time_s;
    handle->now_s = start_time_s;
    handle->position = 0;
    handle->ctx = ctx;
    handle->descr = NULL;
    handle->dt_s = page_index->latest_update_every_s;
    rrdimm_handle->handle = (STORAGE_QUERY_HANDLE *)handle;
    pages_nr = pg_cache_preload(ctx, &page_index->id, start_time_s * USEC_PER_SEC, end_time_s * USEC_PER_SEC,
                                NULL, &handle->page_index);
    if (unlikely(NULL == handle->page_index || 0 == pages_nr))
        // there are no metrics to load
        handle->wanted_start_time_s = INVALID_TIME;
}

static int rrdeng_load_page_next(struct storage_engine_query_handle *rrdimm_handle, bool debug_this __maybe_unused) {
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrdimm_handle->handle;

    struct rrdengine_instance *ctx = handle->ctx;
    struct rrdeng_page_descr *descr = handle->descr;

    uint32_t page_length;
    usec_t page_end_time_ut;
    unsigned position;

    if (likely(descr)) {
        // Drop old page's reference

#ifdef NETDATA_INTERNAL_CHECKS
        rrd_stat_atomic_add(&ctx->stats.metric_API_consumers, -1);
#endif

        pg_cache_put(ctx, descr);
        handle->descr = NULL;
        handle->wanted_start_time_s = (time_t)((handle->page_end_time_ut / USEC_PER_SEC) + handle->dt_s);

        if (unlikely(handle->wanted_start_time_s > rrdimm_handle->end_time_s))
            return 1;
    }

    usec_t wanted_start_time_ut = handle->wanted_start_time_s * USEC_PER_SEC;
    descr = pg_cache_lookup_next(ctx, handle->page_index, &handle->page_index->id,
        wanted_start_time_ut, rrdimm_handle->end_time_s * USEC_PER_SEC);
    if (NULL == descr)
        return 1;

#ifdef NETDATA_INTERNAL_CHECKS
    rrd_stat_atomic_add(&ctx->stats.metric_API_consumers, 1);
#endif

    handle->descr = descr;
    pg_cache_atomic_get_pg_info(descr, &page_end_time_ut, &page_length);
    if (unlikely(INVALID_TIME == descr->start_time_ut || INVALID_TIME == page_end_time_ut || 0 == descr->update_every_s)) {
        error("DBENGINE: discarding invalid page descriptor (start_time = %llu, end_time = %llu, update_every_s = %d)",
              descr->start_time_ut, page_end_time_ut, descr->update_every_s);
        return 1;
    }

    if (unlikely(descr->start_time_ut != page_end_time_ut && wanted_start_time_ut > descr->start_time_ut)) {
        // we're in the middle of the page somewhere
        unsigned entries = page_length / PAGE_POINT_SIZE_BYTES(descr);
        position = ((uint64_t)(wanted_start_time_ut - descr->start_time_ut)) * (entries - 1) /
                   (page_end_time_ut - descr->start_time_ut);
    }
    else
        position = 0;

    handle->page_end_time_ut = page_end_time_ut;
    handle->page_length = page_length;
    handle->entries = page_length / PAGE_POINT_SIZE_BYTES(descr);
    handle->page = descr->pg_cache_descr->page;
    handle->dt_s = descr->update_every_s;
    handle->position = position;

//    if(debug_this)
//        info("DBENGINE: rrdeng_load_page_next(), "
//             "position:%d, "
//             "start_time_ut:%llu, "
//             "page_end_time_ut:%llu, "
//             "next_page_time_ut:%llu, "
//             "in_out:%s"
//             , position
//             , descr->start_time_ut
//             , page_end_time_ut
//             ,
//            wanted_start_time_ut, in_out?"true":"false"
//             );

    return 0;
}

// Returns the metric and sets its timestamp into current_time
// IT IS REQUIRED TO **ALWAYS** SET ALL RETURN VALUES (current_time, end_time, flags)
// IT IS REQUIRED TO **ALWAYS** KEEP TRACK OF TIME, EVEN OUTSIDE THE DATABASE BOUNDARIES
STORAGE_POINT rrdeng_load_metric_next(struct storage_engine_query_handle *rrddim_handle) {
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrddim_handle->handle;
    // struct rrdeng_metric_handle *metric_handle = handle->metric_handle;

    struct rrdeng_page_descr *descr = handle->descr;
    time_t now = handle->now_s + handle->dt_s;

//    bool debug_this = false;
//    {
//        unsigned char u[16] = { 0x0C, 0x0A, 0x40, 0xD6, 0x2A, 0x43, 0x4A, 0x7C, 0x95, 0xF7, 0xD1, 0x1E, 0x0C, 0x9E, 0x8A, 0xE7 };
//        if(uuid_compare(u, handle->page_index->id) == 0) {
//            char buffer[100];
//            snprintfz(buffer, 100, "load system.cpu, now:%u, dt:%u, position:%u page_index first:%u, last:%u",
//                      (uint32_t)(now),
//                      (uint32_t)(handle->dt_s),
//                      (uint32_t)(handle->position),
//                      (uint32_t)(handle->page_index->oldest_time / USEC_PER_SEC),
//                      (uint32_t)(handle->page_index->latest_time / USEC_PER_SEC));
//
//            print_page_cache_descr(descr, buffer, false);
//            debug_this = true;
//        }
//    }

    STORAGE_POINT sp;
    unsigned position = handle->position + 1;
    storage_number_tier1_t tier1_value;

    if (unlikely(INVALID_TIME == handle->wanted_start_time_s)) {
        handle->wanted_start_time_s = INVALID_TIME;
        handle->now_s = now;
        storage_point_empty(sp, now - handle->dt_s, now);
        return sp;
    }

    if (unlikely(!descr || position >= handle->entries)) {
        // We need to get a new page
        if(rrdeng_load_page_next(rrddim_handle, false)) {
            // next calls will not load any more metrics
            handle->wanted_start_time_s = INVALID_TIME;
            handle->now_s = now;
            storage_point_empty(sp, now - handle->dt_s, now);
            return sp;
        }

        descr = handle->descr;
        position = handle->position;
        now = (time_t)((descr->start_time_ut / USEC_PER_SEC) + position * descr->update_every_s);

//        if(debug_this) {
//            char buffer[100];
//            snprintfz(buffer, 100, "NEW PAGE system.cpu, now:%u, dt:%u, position:%u page_index first:%u, last:%u",
//                      (uint32_t)(now),
//                      (uint32_t)(handle->dt_s),
//                      (uint32_t)(handle->position),
//                      (uint32_t)(handle->page_index->oldest_time / USEC_PER_SEC),
//                      (uint32_t)(handle->page_index->latest_time / USEC_PER_SEC));
//
//            print_page_cache_descr(descr, buffer, false);
//        }
    }

    sp.start_time = now - handle->dt_s;
    sp.end_time = now;

    handle->position = position;
    handle->now_s = now;

    switch(descr->type) {
        case PAGE_METRICS: {
            storage_number n = handle->page[position];
            sp.min = sp.max = sp.sum = unpack_storage_number(n);
            sp.flags = n & SN_USER_FLAGS;
            sp.count = 1;
            sp.anomaly_count = is_storage_number_anomalous(n) ? 1 : 0;
        }
        break;

        case PAGE_TIER: {
            tier1_value = ((storage_number_tier1_t *)handle->page)[position];
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
                error("DBENGINE: unknown page type %d found. Cannot decode it. Ignoring its metrics.", descr->type);
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

//    if(debug_this)
//        info("DBENGINE: returning point: "
//             "time from %ld to %ld   //  query from %ld to %ld   //   wanted_start_time_s %ld"
//             , sp.start_time, sp.end_time
//             , rrddim_handle->start_time_s, rrddim_handle->end_time_s
//             , handle->wanted_start_time_s
//             );

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
    struct rrdengine_instance *ctx = handle->ctx;
    struct rrdeng_page_descr *descr = handle->descr;

    if (descr) {
#ifdef NETDATA_INTERNAL_CHECKS
        rrd_stat_atomic_add(&ctx->stats.metric_API_consumers, -1);
#endif
        pg_cache_put(ctx, descr);
    }

    // whatever is allocated at rrdeng_load_metric_init() should be freed here
    freez(handle);
    rrdimm_handle->handle = NULL;
}

time_t rrdeng_metric_latest_time(STORAGE_METRIC_HANDLE *db_metric_handle) {
    struct pg_cache_page_index *page_index = (struct pg_cache_page_index *)db_metric_handle;
    return (time_t)(page_index->latest_time_ut / USEC_PER_SEC);
}
time_t rrdeng_metric_oldest_time(STORAGE_METRIC_HANDLE *db_metric_handle) {
    struct pg_cache_page_index *page_index = (struct pg_cache_page_index *)db_metric_handle;
    return (time_t)(page_index->oldest_time_ut / USEC_PER_SEC);
}

int rrdeng_metric_retention_by_uuid(STORAGE_INSTANCE *si, uuid_t *dim_uuid, time_t *first_entry_t, time_t *last_entry_t)
{
    struct page_cache *pg_cache;
    struct rrdengine_instance *ctx;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index = NULL;

    ctx = (struct rrdengine_instance *)si;
    if (unlikely(!ctx)) {
        error("DBENGINE: invalid STORAGE INSTANCE to %s()", __FUNCTION__);
        return 1;
    }
    pg_cache = &ctx->pg_cache;

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, dim_uuid, sizeof(uuid_t));
    if (likely(NULL != PValue)) {
        page_index = *PValue;
    }
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);

    if (likely(page_index)) {
        *first_entry_t = page_index->oldest_time_ut / USEC_PER_SEC;
        *last_entry_t = page_index->latest_time_ut / USEC_PER_SEC;
        return 0;
    }

    return 1;
}

/* Also gets a reference for the page */
void *rrdeng_create_page(struct rrdengine_instance *ctx, uuid_t *id, struct rrdeng_page_descr **ret_descr)
{
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr;
    void *page;
    /* TODO: check maximum number of pages in page cache limit */

    descr = pg_cache_create_descr();
    descr->id = id; /* TODO: add page type: metric, log, something? */
    descr->type = ctx->page_type;
    page = dbengine_page_alloc(); /*TODO: add page size */
    rrdeng_page_descr_mutex_lock(ctx, descr);
    pg_cache_descr = descr->pg_cache_descr;
    pg_cache_descr->page = page;
    pg_cache_descr->flags = RRD_PAGE_DIRTY /*| RRD_PAGE_LOCKED */ | RRD_PAGE_POPULATED /* | BEING_COLLECTED */;
    pg_cache_descr->refcnt = 1;

    debug(D_RRDENGINE, "Created new page:");
    if (unlikely(debug_flags & D_RRDENGINE))
        print_page_cache_descr(descr, "", true);
    rrdeng_page_descr_mutex_unlock(ctx, descr);
    *ret_descr = descr;
    return page;
}

/* The page must not be empty */
void rrdeng_commit_page(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr,
                        Word_t page_correlation_id)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    Pvoid_t *PValue;
    unsigned nr_committed_pages;

    if (unlikely(NULL == descr)) {
        debug(D_RRDENGINE, "%s: page descriptor is NULL, page has already been force-committed.", __func__);
        return;
    }
    fatal_assert(descr->page_length);

    uv_rwlock_wrlock(&pg_cache->committed_page_index.lock);
    PValue = JudyLIns(&pg_cache->committed_page_index.JudyL_array, page_correlation_id, PJE0);
    *PValue = descr;
    nr_committed_pages = ++pg_cache->committed_page_index.nr_committed_pages;
    uv_rwlock_wrunlock(&pg_cache->committed_page_index.lock);

    if (nr_committed_pages >= pg_cache_hard_limit(ctx) / 2) {
        /* over 50% of pages have not been committed yet */

        if (ctx->drop_metrics_under_page_cache_pressure &&
            nr_committed_pages >= pg_cache_committed_hard_limit(ctx)) {
            /* 100% of pages are dirty */
            struct rrdeng_cmd cmd;

            cmd.opcode = RRDENG_INVALIDATE_OLDEST_MEMORY_PAGE;
            rrdeng_enq_cmd(&ctx->worker_config, &cmd);
        } else {
            if (0 == (unsigned long) ctx->stats.pg_cache_over_half_dirty_events) {
                /* only print the first time */
                errno = 0;
                error("Failed to flush dirty buffers quickly enough in dbengine instance \"%s\". "
                      "Metric data at risk of not being stored in the database, "
                      "please reduce disk load or use a faster disk.", ctx->dbfiles_path);
            }
            rrd_stat_atomic_add(&ctx->stats.pg_cache_over_half_dirty_events, 1);
            rrd_stat_atomic_add(&global_pg_cache_over_half_dirty_events, 1);
        }
    }

    pg_cache_put(ctx, descr);
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
    array[4] = (uint64_t)pg_cache->committed_page_index.nr_committed_pages;
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
    fatal_assert(RRDENG_NR_STATS == 37);
}

/* Releases reference to page */
void rrdeng_put_page(struct rrdengine_instance *ctx, void *handle)
{
    (void)ctx;
    pg_cache_put(ctx, (struct rrdeng_page_descr *)handle);
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
    ctx->cache_pages_low_watermark = (ctx->max_cache_pages * 95LLU) / 100;
    if (disk_space_mb < RRDENG_MIN_DISK_SPACE_MB)
        disk_space_mb = RRDENG_MIN_DISK_SPACE_MB;
    ctx->max_disk_space = disk_space_mb * 1048576LLU;
    strncpyz(ctx->dbfiles_path, dbfiles_path, sizeof(ctx->dbfiles_path) - 1);
    ctx->dbfiles_path[sizeof(ctx->dbfiles_path) - 1] = '\0';
    if (NULL == host)
        strncpyz(ctx->machine_guid, registry_get_this_machine_guid(), GUID_LEN);
    else
        strncpyz(ctx->machine_guid, host->machine_guid, GUID_LEN);

    ctx->drop_metrics_under_page_cache_pressure = rrdeng_drop_metrics_under_page_cache_pressure;
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
//    error = metalog_init(ctx);
//    if (error) {
//        error("Failed to initialize metadata log file event loop.");
//        goto error_after_rrdeng_worker;
//    }

    return 0;

error_after_rrdeng_worker:
    finalize_rrd_files(ctx);
error_after_init_rrd_files:
    free_page_cache(ctx);
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
    free_page_cache(ctx);

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

    //metalog_prepare_exit(ctx->metalog_ctx);
}

RRDENG_SIZE_STATS rrdeng_size_statistics(struct rrdengine_instance *ctx) {
    RRDENG_SIZE_STATS stats = { 0 };

    for(struct pg_cache_page_index *page_index = ctx->pg_cache.metrics_index.last_page_index;
        page_index != NULL ;page_index = page_index->prev) {
        stats.metrics++;
        stats.metrics_pages += page_index->page_count;
    }

    for(struct rrdengine_datafile *df = ctx->datafiles.first; df ;df = df->next) {
        stats.datafiles++;

        for(struct extent_info *ei = df->extents.first; ei ; ei = ei->next) {
            stats.extents++;
            stats.extents_compressed_bytes += ei->size;

            for(int p = 0; p < ei->number_of_pages ;p++) {
                struct rrdeng_page_descr *descr = ei->pages[p];

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
    stats.sizeof_page_in_cache = struct_natural_alignment(sizeof(struct page_cache_descr));
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
