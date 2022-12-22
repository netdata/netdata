// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

rrdeng_stats_t global_io_errors = 0;
rrdeng_stats_t global_fs_errors = 0;
rrdeng_stats_t rrdeng_reserved_file_descriptors = 0;
rrdeng_stats_t global_pg_cache_over_half_dirty_events = 0;
rrdeng_stats_t global_flushing_pressure_page_deletions = 0;

unsigned rrdeng_pages_per_extent = MAX_PAGES_PER_EXTENT;

typedef struct datafile_extent_list_s {
    uv_file file;
    unsigned fileno;
    Pvoid_t extent_pd_list_by_extent_offset_JudyL;
} DATAFILE_EXTENT_PD_LIST;

typedef struct extent_page_list_s {
    uv_file file;
    uint64_t extent_offset;
    uint32_t extent_size;
    unsigned number_of_pages_in_JudyL;
    Pvoid_t page_details_by_metric_id_JudyL;
    struct page_details_control *pdc;
    struct rrdengine_datafile *datafile;
} EXTENT_PD_LIST;

#if WORKER_UTILIZATION_MAX_JOB_TYPES < (RRDENG_MAX_OPCODE + 2)
#error Please increase WORKER_UTILIZATION_MAX_JOB_TYPES to at least (RRDENG_MAX_OPCODE + 2)
#endif


// ----------------------------------------------------------------------------
// extent page details list

void extent_list_free(EXTENT_PD_LIST *epdl)
{
    Pvoid_t *pd_by_start_time_s_JudyL;
    Word_t metric_id_index = 0;
    bool metric_id_first = true;
    while ((pd_by_start_time_s_JudyL = JudyLFirstThenNext(
            epdl->page_details_by_metric_id_JudyL,
            &metric_id_index, &metric_id_first)))
        JudyLFreeArray(pd_by_start_time_s_JudyL, PJE0);

    JudyLFreeArray(&epdl->page_details_by_metric_id_JudyL, PJE0);
    freez(epdl);
}

static void extent_list_mark_all_not_loaded_pages_as_failed(EXTENT_PD_LIST *epdl, PDC_PAGE_STATUS tags, size_t *statistics_counter)
{
    size_t pages_matched = 0;

    Word_t metric_id_index = 0;
    bool metric_id_first = true;
    Pvoid_t *pd_by_start_time_s_JudyL;
    while((pd_by_start_time_s_JudyL = JudyLFirstThenNext(epdl->page_details_by_metric_id_JudyL, &metric_id_index, &metric_id_first))) {

        Word_t start_time_index = 0;
        bool start_time_first = true;
        Pvoid_t *PValue;
        while ((PValue = JudyLFirstThenNext(*pd_by_start_time_s_JudyL, &start_time_index, &start_time_first))) {
            struct page_details *pd = *PValue;

            if(!pd->page) {
                pdc_page_status_set(pd, PDC_PAGE_FAILED | tags);
                pages_matched++;
            }
        }
    }

    if(pages_matched && statistics_counter)
        __atomic_add_fetch(statistics_counter, pages_matched, __ATOMIC_RELAXED);
}

static bool extent_list_check_if_pages_are_already_in_cache(struct rrdengine_instance *ctx, EXTENT_PD_LIST *epdl, PDC_PAGE_STATUS tags)
{
    size_t count_remaining = 0;
    size_t found = 0;

    Word_t metric_id_index = 0;
    bool metric_id_first = true;
    Pvoid_t *pd_by_start_time_s_JudyL;
    while((pd_by_start_time_s_JudyL = JudyLFirstThenNext(epdl->page_details_by_metric_id_JudyL, &metric_id_index, &metric_id_first))) {

        Word_t start_time_index = 0;
        bool start_time_first = true;
        Pvoid_t *PValue;
        while ((PValue = JudyLFirstThenNext(*pd_by_start_time_s_JudyL, &start_time_index, &start_time_first))) {
            struct page_details *pd = *PValue;
            if (pd->page)
                continue;

            pd->page = pgc_page_get_and_acquire(main_cache, (Word_t) ctx, pd->metric_id, pd->first_time_s, true);
            if (pd->page) {
                found++;
                pdc_page_status_set(pd, PDC_PAGE_READY | tags);
            }
            else
                count_remaining++;
        }
    }

    if(found) {
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_preloaded, found, __ATOMIC_RELAXED);
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_main_cache, found, __ATOMIC_RELAXED);
    }

    return count_remaining == 0;
}

// ----------------------------------------------------------------------------
// page details list

static void pdc_destroy(PDC *pdc) {
    completion_destroy(&pdc->completion);

    Pvoid_t *PValue;
    struct page_details *pd;
    Word_t time_index = 0;
    bool first_then_next = true;
    size_t unroutable = 0;
    while((PValue = JudyLFirstThenNext(pdc->page_list_JudyL, &time_index, &first_then_next))) {
        pd = *PValue;

        // no need for atomics here - we are done...
        PDC_PAGE_STATUS status = pd->status;

        if(status & PDC_PAGE_DATAFILE_ACQUIRED) {
            datafile_release(pd->datafile.ptr);
            pd->datafile.ptr = NULL;
        }

        internal_fatal(pd->datafile.ptr, "DBENGINE: page details has a datafile.ptr that is not released.");

        if(!pd->page && !(status & (PDC_PAGE_READY | PDC_PAGE_FAILED | PDC_PAGE_RELEASED))) {
            // pdc_page_status_set(pd, PDC_PAGE_FAILED);
            unroutable++;
        }

        if(pd->page && !(status & PDC_PAGE_RELEASED)) {
            pgc_page_release(main_cache, pd->page);
            // pdc_page_status_set(pd, PDC_PAGE_RELEASED);
        }

        freez(pd);
    }

    JudyLFreeArray(&pdc->page_list_JudyL, PJE0);

    __atomic_sub_fetch(&pdc->ctx->inflight_queries, 1, __ATOMIC_RELAXED);
    freez(pdc);

    if(unroutable)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_fail_unroutable, unroutable, __ATOMIC_RELAXED);
}

static void pdc_acquire(PDC *pdc) {
    netdata_spinlock_lock(&pdc->refcount_spinlock);

    if(pdc->refcount < 1)
        fatal("PDC is not referenced and cannot be acquired");

    pdc->refcount++;
    netdata_spinlock_unlock(&pdc->refcount_spinlock);
}

bool pdc_release_and_destroy_if_unreferenced(PDC *pdc, bool worker, bool router __maybe_unused) {
    netdata_spinlock_lock(&pdc->refcount_spinlock);

    if(pdc->refcount <= 0)
        fatal("PDC is not referenced and cannot be released");

    pdc->refcount--;

    if (pdc->refcount <= 1 && worker) {
        // when 1 refcount is remaining, and we are a worker,
        // we can mark the job completed:
        // - if the remaining refcount is from the query caller, we will wake it up
        // - if the remaining refcount is from another worker, the query thread is already away
        completion_mark_complete(&pdc->completion);
    }

    if (pdc->refcount == 0) {
        netdata_spinlock_unlock(&pdc->refcount_spinlock);
        pdc_destroy(pdc);
        return true;
    }

    netdata_spinlock_unlock(&pdc->refcount_spinlock);
    return false;
}


typedef void (*execute_extent_page_details_list_t)(struct rrdengine_instance *ctx, EXTENT_PD_LIST *extent_page_list);
static void pdc_to_extent_page_details_list(struct rrdengine_instance *ctx, struct page_details_control *pdc, execute_extent_page_details_list_t exec_first_extent_list, execute_extent_page_details_list_t exec_rest_extent_list)
{
    Pvoid_t *PValue;
    Pvoid_t *PValue1;
    Pvoid_t *PValue2;
    Word_t time_index = 0;
    struct page_details *pd = NULL;

    // this is the entire page list
    // Lets do some deduplication
    // 1. Per datafile
    // 2. Per extent
    // 3. Pages per extent will be added to the cache either as acquired or not

    Pvoid_t JudyL_datafile_list = NULL;

    DATAFILE_EXTENT_PD_LIST *datafile_extent_list;
    EXTENT_PD_LIST *extent_page_list;

    if (pdc->page_list_JudyL) {
        bool first_then_next = true;
        while((PValue = JudyLFirstThenNext(pdc->page_list_JudyL, &time_index, &first_then_next))) {
            pd = *PValue;

            if (!pd || pd->page)
                continue;

            PValue1 = JudyLIns(&JudyL_datafile_list, pd->datafile.fileno, PJE0);
            if (PValue1 && !*PValue1) {
                *PValue1 = datafile_extent_list = mallocz(sizeof(*datafile_extent_list));
                datafile_extent_list->extent_pd_list_by_extent_offset_JudyL = NULL;
                datafile_extent_list->fileno = pd->datafile.fileno;
            }
            else
                datafile_extent_list = *PValue1;

            PValue2 = JudyLIns(&datafile_extent_list->extent_pd_list_by_extent_offset_JudyL, pd->datafile.extent.pos, PJE0);
            if (PValue2 && !*PValue2) {
                *PValue2 = extent_page_list = mallocz( sizeof(*extent_page_list));
                extent_page_list->page_details_by_metric_id_JudyL = NULL;
                extent_page_list->number_of_pages_in_JudyL = 0;
                extent_page_list->file = pd->datafile.file;
                extent_page_list->extent_offset = pd->datafile.extent.pos;
                extent_page_list->extent_size = pd->datafile.extent.bytes;
                extent_page_list->datafile = pd->datafile.ptr;
            }
            else
                extent_page_list = *PValue2;

            extent_page_list->number_of_pages_in_JudyL++;

            Pvoid_t *pd_by_first_time_s_judyL = JudyLIns(&extent_page_list->page_details_by_metric_id_JudyL, pd->metric_id, PJE0);
            Pvoid_t *pd_pptr = JudyLIns(pd_by_first_time_s_judyL, pd->first_time_s, PJE0);
            *pd_pptr = pd;
        }

        size_t extent_list_no = 0;
        Word_t datafile_no = 0;
        first_then_next = true;
        while((PValue = JudyLFirstThenNext(JudyL_datafile_list, &datafile_no, &first_then_next))) {
            datafile_extent_list = *PValue;

            bool first_then_next_extent = true;
            Word_t pos = 0;
            while ((PValue = JudyLFirstThenNext(datafile_extent_list->extent_pd_list_by_extent_offset_JudyL, &pos, &first_then_next_extent))) {
                extent_page_list = *PValue;
                internal_fatal(!extent_page_list, "DBENGINE: extent_list is not populated properly");

                // The extent page list can be dispatched to a worker
                // It will need to populate the cache with "acquired" pages that are in the list (pd) only
                // the rest of the extent pages will be added to the cache butnot acquired

                pdc_acquire(pdc); // we do this for the next worker: do_read_extent_work()
                extent_page_list->pdc = pdc;

                if(extent_list_no++ == 0)
                    exec_first_extent_list(ctx, extent_page_list);
                else
                    exec_rest_extent_list(ctx, extent_page_list);
            }
            JudyLFreeArray(&datafile_extent_list->extent_pd_list_by_extent_offset_JudyL, PJE0);
            freez(datafile_extent_list);
        }
        JudyLFreeArray(&JudyL_datafile_list, PJE0);
    }

    pdc_release_and_destroy_if_unreferenced(pdc, true, true);
}

// ----------------------------------------------------------------------------

void *dbengine_page_alloc(struct rrdengine_instance *ctx __maybe_unused, size_t size) {
    void *page = mallocz(size);
    return page;
}

void dbengine_page_free(void *page) {
    freez(page);
}

static void sanity_check(void)
{
    BUILD_BUG_ON(WORKER_UTILIZATION_MAX_JOB_TYPES < (RRDENG_MAX_OPCODE + 2));

    /* Magic numbers must fit in the super-blocks */
    BUILD_BUG_ON(strlen(RRDENG_DF_MAGIC) > RRDENG_MAGIC_SZ);
    BUILD_BUG_ON(strlen(RRDENG_JF_MAGIC) > RRDENG_MAGIC_SZ);

    /* Version strings must fit in the super-blocks */
    BUILD_BUG_ON(strlen(RRDENG_DF_VER) > RRDENG_VER_SZ);
    BUILD_BUG_ON(strlen(RRDENG_JF_VER) > RRDENG_VER_SZ);

    /* Data file super-block cannot be larger than RRDENG_BLOCK_SIZE */
    BUILD_BUG_ON(RRDENG_DF_SB_PADDING_SZ < 0);

    BUILD_BUG_ON(sizeof(uuid_t) != UUID_SZ); /* check UUID size */

    /* page count must fit in 8 bits */
    BUILD_BUG_ON(MAX_PAGES_PER_EXTENT > 255);

    /* extent cache count must fit in 32 bits */
//    BUILD_BUG_ON(MAX_CACHED_EXTENTS > 32);

    /* page info scratch space must be able to hold 2 32-bit integers */
    BUILD_BUG_ON(sizeof(((struct rrdeng_page_info *)0)->scratch) < 2 * sizeof(uint32_t));
}

static void fill_page_with_nulls(void *page, uint32_t page_length, uint8_t type) {
    switch(type) {
        case PAGE_METRICS: {
            storage_number n = pack_storage_number(NAN, SN_FLAG_NONE);
            storage_number *array = (storage_number *)page;
            size_t slots = page_length / sizeof(n);
            for(size_t i = 0; i < slots ; i++)
                array[i] = n;
        }
            break;

        case PAGE_TIER: {
            storage_number_tier1_t n = {
                .min_value = NAN,
                .max_value = NAN,
                .sum_value = NAN,
                .count = 1,
                .anomaly_count = 0,
            };
            storage_number_tier1_t *array = (storage_number_tier1_t *)page;
            size_t slots = page_length / sizeof(n);
            for(size_t i = 0; i < slots ; i++)
                array[i] = n;
        }
            break;

        default: {
            static bool logged = false;
            if(!logged) {
                error("DBENGINE: cannot fill page with nulls on unknown page type id %d", type);
                logged = true;
            }
            memset(page, 0, page_length);
        }
    }
}



inline VALIDATED_PAGE_DESCRIPTOR validate_extent_page_descr(const struct rrdeng_extent_page_descr *descr, time_t now_s, time_t overwrite_zero_update_every_s, bool have_read_error) {
    VALIDATED_PAGE_DESCRIPTOR vd = {
            .start_time_s = (time_t) (descr->start_time_ut / USEC_PER_SEC),
            .end_time_s = (time_t) (descr->end_time_ut / USEC_PER_SEC),
            .page_length = descr->page_length,
            .type = descr->type,
    };
    vd.point_size = page_type_size[vd.type];
    vd.entries = (size_t) vd.page_length / vd.point_size;
    vd.update_every_s = (vd.entries > 1) ? ((vd.end_time_s - vd.start_time_s) / (time_t)(vd.entries - 1)) : overwrite_zero_update_every_s;

    bool is_valid = true;

    if( have_read_error                                         ||
        vd.page_length == 0                                     ||
        vd.page_length > RRDENG_BLOCK_SIZE                      ||
        vd.start_time_s > vd.end_time_s                         ||
        vd.end_time_s > now_s                                   ||
        vd.start_time_s == 0                                    ||
        vd.end_time_s == 0                                      ||
        (vd.start_time_s == vd.end_time_s && vd.entries > 1)    ||
        (vd.update_every_s == 0 && vd.entries > 1)
        ) {
        is_valid = false;

        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "DBENGINE: ignoring invalid page of type %u from %ld to %ld (now %ld), update every %ld, page length %zu, point size %zu, entries %zu.",
                    vd.type, vd.start_time_s, vd.end_time_s, now_s, vd.update_every_s, vd.page_length, vd.point_size, vd.entries);
    }
    else {
        if (vd.update_every_s) {
            size_t entries_by_time = (vd.start_time_s - (vd.start_time_s - vd.update_every_s)) / vd.update_every_s;

            if (vd.entries != entries_by_time)
                vd.end_time_s = (time_t) (vd.start_time_s + (vd.entries - 1) * vd.update_every_s);
        }
    }

    if(!is_valid) {
        if(vd.start_time_s == vd.end_time_s) {
            vd.page_length = vd.point_size;
            vd.entries = 1;
        }
        else {
            vd.page_length = vd.point_size * 2;
            vd.update_every_s = vd.end_time_s - vd.start_time_s;
            vd.entries = 2;
        }
    }

    vd.data_on_disk_valid = is_valid;
    return vd;
}

static bool extent_uncompress_and_populate_pages(
                struct rrdengine_instance *ctx,
                void *data,
                size_t data_length,
                EXTENT_PD_LIST *extent_page_list,
                bool preload_all_pages,
                bool worker,
                PDC_PAGE_STATUS tags,
                bool cached_extent)
{
    int ret;
    unsigned i, count;
    void *uncompressed_buf = NULL;
    uint32_t payload_length, payload_offset, trailer_offset, uncompressed_payload_length = 0;
    uint8_t have_read_error = 0;
    /* persistent structures */
    struct rrdeng_df_extent_header *header;
    struct rrdeng_df_extent_trailer *trailer;
    uLong crc;

    bool can_use_data = true;
    if(data_length < sizeof(*header) + sizeof(header->descr[0]) + sizeof(*trailer)) {
        can_use_data = false;
    }
    else {
        header = data;
        payload_length = header->payload_length;
        count = header->number_of_pages;
        payload_offset = sizeof(*header) + sizeof(header->descr[0]) * count;
        trailer_offset = data_length - sizeof(*trailer);
        trailer = data + trailer_offset;
    }

    if( !can_use_data ||
        count < 1 ||
        count > MAX_PAGES_PER_EXTENT ||
        (header->compression_algorithm != RRD_NO_COMPRESSION && header->compression_algorithm != RRD_LZ4) ||
        (payload_length != trailer_offset - payload_offset) ||
        (data_length != payload_offset + payload_length + sizeof(*trailer))
        ) {

        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, but header is INVALID", __func__,
                    extent_page_list->extent_offset, extent_page_list->extent_size, extent_page_list->datafile->fileno);

        return false;
    }

    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, data, extent_page_list->extent_size - sizeof(*trailer));
    ret = crc32cmp(trailer->checksum, crc);
    if (unlikely(ret)) {
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        have_read_error = 1;

        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, but CRC32 check FAILED", __func__,
                    extent_page_list->extent_offset, extent_page_list->extent_size, extent_page_list->datafile->fileno);
    }

    if(worker)
        worker_is_busy(UV_EVENT_EXT_DECOMPRESSION);

    if (!have_read_error && RRD_NO_COMPRESSION != header->compression_algorithm) {
        uncompressed_payload_length = 0;
        for (i = 0; i < count; ++i)
            uncompressed_payload_length += header->descr[i].page_length;

        uncompressed_buf = mallocz(uncompressed_payload_length);
        ret = LZ4_decompress_safe(data + payload_offset, uncompressed_buf,
                                  (int)payload_length, (int)uncompressed_payload_length);
        ctx->stats.before_decompress_bytes += payload_length;
        ctx->stats.after_decompress_bytes += ret;
        debug(D_RRDENGINE, "LZ4 decompressed %u bytes to %d bytes.", payload_length, ret);
    }

    size_t stats_data_from_main_cache = 0;
    size_t stats_data_from_extent = 0;
    size_t stats_load_compressed = 0;
    size_t stats_load_uncompressed = 0;
    size_t stats_load_invalid_page = 0;
    size_t stats_cache_hit_while_inserting = 0;
    size_t stats_cache_hit_before_allocation = 0;

    uint32_t page_offset = 0, page_length;
    time_t now_s = now_realtime_sec();
    for (i = 0; i < count; i++, page_offset += page_length) {
        page_length = header->descr[i].page_length;
        time_t start_time_s = (time_t) (header->descr[i].start_time_ut / USEC_PER_SEC);

        if(!page_length || !start_time_s) {
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, having page %u (out of %u) EMPTY",
                        __func__, extent_page_list->extent_offset, extent_page_list->extent_size, extent_page_list->datafile->fileno, i, count);
            continue;
        }

        if(worker)
            worker_is_busy(UV_EVENT_METRIC_LOOKUP);

        METRIC *metric = mrg_metric_get_and_acquire(main_mrg, &header->descr[i].uuid, (Word_t)ctx);
        Word_t metric_id = (Word_t)metric;
        if(!metric) {
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, having page %u (out of %u) for unknown UUID",
                        __func__, extent_page_list->extent_offset, extent_page_list->extent_size, extent_page_list->datafile->fileno, i, count);
            continue;
        }
        mrg_metric_release(main_mrg, metric);

        if(worker)
            worker_is_busy(UV_EVENT_PAGE_LOOKUP);

        struct page_details *pd = NULL;
        Pvoid_t *pd_by_start_time_s_judyL = JudyLGet(extent_page_list->page_details_by_metric_id_JudyL, metric_id, PJE0);
        internal_fatal(pd_by_start_time_s_judyL == PJERR, "DBENGINE: corrupted extent metrics JudyL");

        if(pd_by_start_time_s_judyL && *pd_by_start_time_s_judyL) {
            Pvoid_t *pd_pptr = JudyLGet(*pd_by_start_time_s_judyL, start_time_s, PJE0);
            internal_fatal(pd_pptr == PJERR, "DBENGINE: corrupted metric page details JudyHS");

            if(pd_pptr && *pd_pptr) {
                pd = *pd_pptr;
                internal_fatal(metric_id != pd->metric_id, "DBENGINE: metric ids do not match");
            }
        }

        if(!pd && !preload_all_pages)
            continue;

        VALIDATED_PAGE_DESCRIPTOR vd = validate_extent_page_descr(
                &header->descr[i], now_s,
                (pd) ? pd->update_every_s : 0,
                have_read_error);

        if(worker)
            worker_is_busy(UV_EVENT_PAGE_POPULATION);

        PGC_PAGE *page = pgc_page_get_and_acquire(main_cache, (Word_t)ctx, metric_id, start_time_s, true);
        if (!page) {
            void *page_data = mallocz((size_t) vd.page_length);

            if (unlikely(!vd.data_on_disk_valid)) {
                fill_page_with_nulls(page_data, vd.page_length, vd.type);
                stats_load_invalid_page++;
            }

            else if (RRD_NO_COMPRESSION == header->compression_algorithm) {
                memcpy(page_data, data + payload_offset + page_offset, (size_t) vd.page_length);
                stats_load_uncompressed++;
            }

            else {
                if(unlikely(page_offset + vd.page_length > uncompressed_payload_length)) {
                    error_limit_static_global_var(erl, 10, 0);
                    error_limit(&erl,
                                   "DBENGINE: page %u offset %u + page length %zu exceeds the uncompressed buffer size %u",
                                   i, page_offset, vd.page_length, uncompressed_payload_length);

                    fill_page_with_nulls(page_data, vd.page_length, vd.type);
                    stats_load_invalid_page++;
                }
                else {
                    memcpy(page_data, uncompressed_buf + page_offset, vd.page_length);
                    stats_load_compressed++;
                }
            }

            PGC_ENTRY page_entry = {
                .hot = false,
                .section = (Word_t)ctx,
                .metric_id = metric_id,
                .start_time_t = vd.start_time_s,
                .end_time_t = vd.end_time_s,
                .update_every = vd.update_every_s,
                .size = (size_t) vd.page_length,
                .data = page_data
            };

            bool added = true;
            page = pgc_page_add_and_acquire(main_cache, page_entry, &added);
            if (false == added) {
                freez(page_data);
                stats_cache_hit_while_inserting++;
                stats_data_from_main_cache++;
            }
            else
                stats_data_from_extent++;
        }
        else {
            stats_cache_hit_before_allocation++;
            stats_data_from_main_cache++;
        }

        if (pd) {
            pd->page = page;
            pd->page_length = pgc_page_data_size(main_cache, page);
            pdc_page_status_set(pd, PDC_PAGE_READY | tags);
        }
        else
            pgc_page_release(main_cache, page);
    }

    if(stats_data_from_main_cache)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_main_cache, stats_data_from_main_cache, __ATOMIC_RELAXED);

    if(cached_extent)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_extent_cache, stats_data_from_extent, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_disk, stats_data_from_extent, __ATOMIC_RELAXED);

    if(stats_cache_hit_before_allocation)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_loaded_but_cache_hit_before_allocation, stats_cache_hit_before_allocation, __ATOMIC_RELAXED);

    if(stats_cache_hit_while_inserting)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_loaded_but_cache_hit_while_inserting, stats_cache_hit_while_inserting, __ATOMIC_RELAXED);

    if(stats_load_compressed)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_compressed, stats_load_compressed, __ATOMIC_RELAXED);

    if(stats_load_uncompressed)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_uncompressed, stats_load_uncompressed, __ATOMIC_RELAXED);

    if(stats_load_invalid_page)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_fail_invalid_page_in_extent, stats_load_invalid_page, __ATOMIC_RELAXED);

    if(worker)
        worker_is_idle();

    freez(uncompressed_buf);

    return true;
}

static void commit_data_extent(struct rrdengine_worker_config* wc, struct extent_io_descriptor *xt_io_descr)
{
    struct rrdengine_instance *ctx = wc->ctx;
    unsigned count, payload_length, descr_size, size_bytes;
    void *buf;
    /* persistent structures */
    struct rrdeng_df_extent_header *df_header;
    struct rrdeng_jf_transaction_header *jf_header;
    struct rrdeng_jf_store_data *jf_metric_data;
    struct rrdeng_jf_transaction_trailer *jf_trailer;
    uLong crc;

    df_header = xt_io_descr->buf;
    count = df_header->number_of_pages;
    descr_size = sizeof(*jf_metric_data->descr) * count;
    payload_length = sizeof(*jf_metric_data) + descr_size;
    size_bytes = sizeof(*jf_header) + payload_length + sizeof(*jf_trailer);

    buf = wal_get_transaction_buffer(wc, size_bytes);

    jf_header = buf;
    jf_header->type = STORE_DATA;
    jf_header->reserved = 0;
    jf_header->id = ctx->commit_log.transaction_id++;
    jf_header->payload_length = payload_length;

    jf_metric_data = buf + sizeof(*jf_header);
    jf_metric_data->extent_offset = xt_io_descr->pos;
    jf_metric_data->extent_size = xt_io_descr->bytes;
    jf_metric_data->number_of_pages = count;
    memcpy(jf_metric_data->descr, df_header->descr, descr_size);

    jf_trailer = buf + sizeof(*jf_header) + payload_length;
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, buf, sizeof(*jf_header) + payload_length);
    crc32set(jf_trailer->checksum, crc);
}

static void do_commit_transaction(struct rrdengine_worker_config* wc, uint8_t type, void *data)
{
    switch (type) {
    case STORE_DATA:
        commit_data_extent(wc, (struct extent_io_descriptor *)data);
        break;
    default:
        fatal_assert(type == STORE_DATA);
        break;
    }
}

// Main event loop callback
static void do_flush_extent_cb(uv_fs_t *req)
{
    struct rrdengine_worker_config *wc = req->loop->data;
    struct rrdengine_instance *ctx = wc->ctx;
    struct extent_io_descriptor *xt_io_descr;
    struct rrdeng_page_descr *descr;
    struct rrdengine_datafile *datafile;
    unsigned i;

    xt_io_descr = req->data;
    if (req->result < 0) {
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        error("%s: uv_fs_write: %s", __func__, uv_strerror((int)req->result));
    }
    datafile = xt_io_descr->datafile;

    // Descriptors need to be freed when migration to V2 happens

    for (i = 0 ; i < xt_io_descr->descr_count ; ++i) {
        descr = xt_io_descr->descr_array[i];

        pgc_open_add_hot_page(
                (Word_t)ctx, descr->metric_id,
                (time_t) (descr->start_time_ut / USEC_PER_SEC),
                (time_t) (descr->end_time_ut / USEC_PER_SEC),
                descr->update_every_s,
                datafile,
                xt_io_descr->pos, xt_io_descr->bytes, descr->page_length);

        freez(descr->page);
        freez(descr);
    }

    uv_fs_req_cleanup(req);
    posix_memfree(xt_io_descr->buf);
    freez(xt_io_descr);
    wc->cache_flush_can_run = true;
    wc->outstanding_flush_requests--;
}

/*
 * Take a page list in a judy array and write them
 */
static int do_flush_extent(struct rrdengine_worker_config *wc, Pvoid_t Judy_page_list, struct completion *completion)
{
    struct rrdengine_instance *ctx = wc->ctx;
    int ret;
    int compressed_size, max_compressed_size = 0;
    unsigned i, count, size_bytes, pos, real_io_size;
    uint32_t uncompressed_payload_length, payload_offset;
    struct rrdeng_page_descr *descr, *eligible_pages[MAX_PAGES_PER_EXTENT];
    struct extent_io_descriptor *xt_io_descr;
    void *compressed_buf = NULL;
    Word_t descr_commit_idx_array[MAX_PAGES_PER_EXTENT];
    Pvoid_t *PValue;
    Word_t Index;
    uint8_t compression_algorithm = ctx->global_compress_alg;
    struct rrdengine_datafile *datafile;
    /* persistent structures */
    struct rrdeng_df_extent_header *header;
    struct rrdeng_df_extent_trailer *trailer;
    uLong crc;

    for (Index = 0, count = 0, uncompressed_payload_length = 0,
         PValue = JudyLFirst(Judy_page_list, &Index, PJE0),
         descr = unlikely(NULL == PValue) ? NULL : *PValue ;

         descr != NULL && count != rrdeng_pages_per_extent;

         PValue = JudyLNext(Judy_page_list, &Index, PJE0),
         descr = unlikely(NULL == PValue) ? NULL : *PValue) {

         uncompressed_payload_length += descr->page_length;
         descr_commit_idx_array[count] = Index;
         eligible_pages[count++] = descr;

         ret = JudyLDel(&Judy_page_list, Index, PJE0);
         fatal_assert(1 == ret);
    }

    if (!count) {
        if (completion)
            completion_mark_complete(completion);
        return 0;
    }

    xt_io_descr = mallocz(sizeof(*xt_io_descr));
    payload_offset = sizeof(*header) + count * sizeof(header->descr[0]);
    switch (compression_algorithm) {
        case RRD_NO_COMPRESSION:
            size_bytes = payload_offset + uncompressed_payload_length + sizeof(*trailer);
        break;
        default: /* Compress */
            fatal_assert(uncompressed_payload_length < LZ4_MAX_INPUT_SIZE);
            max_compressed_size = LZ4_compressBound(uncompressed_payload_length);
            compressed_buf = mallocz(max_compressed_size);
            size_bytes = payload_offset + MAX(uncompressed_payload_length, (unsigned)max_compressed_size) + sizeof(*trailer);
        break;
    }
    ret = posix_memalign((void *)&xt_io_descr->buf, RRDFILE_ALIGNMENT, ALIGN_BYTES_CEILING(size_bytes));
    if (unlikely(ret)) {
        fatal("posix_memalign:%s", strerror(ret));
        /* freez(xt_io_descr);*/
    }
    memset(xt_io_descr->buf, 0, ALIGN_BYTES_CEILING(size_bytes));
    (void) memcpy(xt_io_descr->descr_array, eligible_pages, sizeof(struct rrdeng_page_descr *) * count);
    xt_io_descr->descr_count = count;

    pos = 0;
    header = xt_io_descr->buf;
    header->compression_algorithm = compression_algorithm;
    header->number_of_pages = count;
    pos += sizeof(*header);

    datafile = ctx->datafiles.first->prev;  /* TODO: check for exceeded size quota */
    xt_io_descr->datafile = datafile;

    for (i = 0 ; i < count ; ++i) {
        /* This is here for performance reasons */
        xt_io_descr->descr_commit_idx_array[i] = descr_commit_idx_array[i];

        descr = xt_io_descr->descr_array[i];
        header->descr[i].type = descr->type;
        uuid_copy(*(uuid_t *)header->descr[i].uuid, *descr->id);
        header->descr[i].page_length = descr->page_length;
        header->descr[i].start_time_ut = descr->start_time_ut;
        header->descr[i].end_time_ut = descr->end_time_ut;
        pos += sizeof(header->descr[i]);
    }
    for (i = 0 ; i < count ; ++i) {
        descr = xt_io_descr->descr_array[i];
        (void) memcpy(xt_io_descr->buf + pos, descr->page, descr->page_length);
        pos += descr->page_length;
    }

    switch (compression_algorithm) {
        case RRD_NO_COMPRESSION:
            header->payload_length = uncompressed_payload_length;
            break;
        default: /* Compress */
            compressed_size = LZ4_compress_default(xt_io_descr->buf + payload_offset, compressed_buf,
                                               uncompressed_payload_length, max_compressed_size);
            ctx->stats.before_compress_bytes += uncompressed_payload_length;
            ctx->stats.after_compress_bytes += compressed_size;
            debug(D_RRDENGINE, "LZ4 compressed %"PRIu32" bytes to %d bytes.", uncompressed_payload_length, compressed_size);
            (void) memcpy(xt_io_descr->buf + payload_offset, compressed_buf, compressed_size);
            freez(compressed_buf);
            size_bytes = payload_offset + compressed_size + sizeof(*trailer);
            header->payload_length = compressed_size;
        break;
    }
    xt_io_descr->bytes = size_bytes;
    xt_io_descr->pos = datafile->pos;
    xt_io_descr->req.data = xt_io_descr;
    xt_io_descr->completion = completion;

    trailer = xt_io_descr->buf + size_bytes - sizeof(*trailer);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, xt_io_descr->buf, size_bytes - sizeof(*trailer));
    crc32set(trailer->checksum, crc);

    real_io_size = ALIGN_BYTES_CEILING(size_bytes);
    xt_io_descr->iov = uv_buf_init((void *)xt_io_descr->buf, real_io_size);
    ret = uv_fs_write(wc->loop, &xt_io_descr->req, datafile->file, &xt_io_descr->iov, 1, datafile->pos, do_flush_extent_cb);
    fatal_assert(-1 != ret);
    ctx->stats.io_write_bytes += real_io_size;
    ++ctx->stats.io_write_requests;
    ctx->stats.io_write_extent_bytes += real_io_size;
    ++ctx->stats.io_write_extents;
    do_commit_transaction(wc, STORE_DATA, xt_io_descr);
    datafile->pos += ALIGN_BYTES_CEILING(size_bytes);
    ctx->disk_space += ALIGN_BYTES_CEILING(size_bytes);

    if (completion)
        completion_mark_complete(completion);
    return ALIGN_BYTES_CEILING(size_bytes);
}

static void after_delete_old_data(uv_work_t *req, int status __maybe_unused)
{
    struct rrdeng_work  *work_request = req->data;
    struct rrdengine_worker_config *wc = work_request->wc;

    struct rrdengine_instance *ctx = wc->ctx;
    struct rrdengine_datafile *datafile;
    struct rrdengine_journalfile *journalfile;
    unsigned deleted_bytes, journalfile_bytes, datafile_bytes;
    int ret;
    char path[RRDENG_PATH_MAX];

    uv_rwlock_wrlock(&ctx->datafiles.rwlock);

    datafile = ctx->datafiles.first;
    journalfile = datafile->journalfile;
    datafile_bytes = datafile->pos;
    journalfile_bytes = journalfile->pos;
    deleted_bytes = GET_JOURNAL_DATA_SIZE(journalfile);

    info("Deleting data and journal files");
    datafile_list_delete_unsafe(ctx, datafile);
    ret = destroy_journal_file_unsafe(journalfile, datafile);
    if (!ret) {
        generate_journalfilepath(datafile, path, sizeof(path));
        info("Deleted journal file \"%s\".", path);
        generate_journalfilepath_v2(datafile, path, sizeof(path));
        info("Deleted journal file \"%s\".", path);
        deleted_bytes += journalfile_bytes;
    }
    ret = destroy_data_file_unsafe(datafile);
    if (!ret) {
        generate_datafilepath(datafile, path, sizeof(path));
        info("Deleted data file \"%s\".", path);
        deleted_bytes += datafile_bytes;
    }
    freez(journalfile);
    freez(datafile);

    ctx->disk_space -= deleted_bytes;
    info("Reclaimed %u bytes of disk space.", deleted_bytes);
    uv_rwlock_wrunlock(&ctx->datafiles.rwlock);

    rrdcontext_db_rotation();
    wc->now_deleting_files = 0;
    freez(work_request);
}

struct uuid_first_time_s {
    uuid_t *uuid;
    time_t first_time_t;
    time_t last_time_t;
    METRIC *metric;
};


static int journal_metric_uuid_compare(const void *key, const void *metric)
{
    return uuid_compare(*(uuid_t *) key, ((struct journal_metric_list *) metric)->uuid);
}

void find_uuid_first_time(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile, Pvoid_t metric_first_time_JudyL)
{
    if (unlikely(!datafile))
        return;

    unsigned v2_count = 0;
    unsigned journalfile_count = 0;
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    while (datafile) {
        struct journal_v2_header *journal_header = (struct journal_v2_header *) GET_JOURNAL_DATA(datafile->journalfile);
        if (!journal_header || !datafile->users.available) {
            datafile = datafile->next;
            continue;
        }

        time_t journal_start_time_t = (time_t) (journal_header->start_time_ut / USEC_PER_SEC);
        size_t journal_metric_count = (size_t)journal_header->metric_count;
        struct journal_metric_list *uuid_list = (struct journal_metric_list *)((uint8_t *) journal_header + journal_header->metric_offset);

        Word_t index = 0;
        bool first_then_next = true;
        Pvoid_t *PValue;
        while ((PValue = JudyLFirstThenNext(metric_first_time_JudyL, &index, &first_then_next))) {
            struct uuid_first_time_s *uuid_first_t_entry = *PValue;

            struct journal_metric_list *uuid_entry = bsearch(uuid_first_t_entry->uuid,uuid_list,journal_metric_count,sizeof(*uuid_list), journal_metric_uuid_compare);

            if (unlikely(!uuid_entry))
                continue;

            time_t first_time_t = uuid_entry->delta_start + journal_start_time_t;
            time_t last_time_t = uuid_entry->delta_end + journal_start_time_t;
            uuid_first_t_entry->first_time_t = MIN(uuid_first_t_entry->first_time_t , first_time_t);
            uuid_first_t_entry->last_time_t = MAX(uuid_first_t_entry->last_time_t , last_time_t);
            v2_count++;
        }
        journalfile_count++;
        datafile = datafile->next;
    }
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    // Lets scan the open cache for almost exact match
    bool first_then_next = true;
    Pvoid_t *PValue;
    Word_t index = 0;
    unsigned open_cache_count = 0;
    while ((PValue = JudyLFirstThenNext(metric_first_time_JudyL, &index, &first_then_next))) {
        struct uuid_first_time_s *uuid_first_t_entry = *PValue;

        PGC_PAGE *page = pgc_page_get_and_acquire(open_cache, (Word_t)ctx, (Word_t)uuid_first_t_entry->metric, uuid_first_t_entry->last_time_t, false);
        if (page) {
            time_t first_time_t = pgc_page_start_time_t(page);
            time_t last_time_t = pgc_page_end_time_t(page);
            uuid_first_t_entry->first_time_t = MIN(uuid_first_t_entry->first_time_t, first_time_t);
            uuid_first_t_entry->last_time_t = MAX(uuid_first_t_entry->last_time_t, last_time_t);
            pgc_page_release(open_cache, page);
            open_cache_count++;
        }
    }
    info("Processed %u journalfiles and matched %u metrics in v2 files and %u in open cache", journalfile_count,
        v2_count, open_cache_count);
}

static void delete_old_data(uv_work_t *req)
{
    static __thread int worker = -1;
    if (unlikely(worker == -1))
        register_libuv_worker_jobs();

    worker_is_busy(UV_EVENT_ANALYZE_V2);

    struct rrdeng_work  *work_request = req->data;
    struct rrdengine_worker_config *wc = work_request->wc;
    struct rrdengine_instance *ctx = wc->ctx;

    struct rrdengine_journalfile *journalfile = ctx->datafiles.first->journalfile;

    struct journal_v2_header *journal_header = (struct journal_v2_header *)GET_JOURNAL_DATA(journalfile);
    struct journal_metric_list *uuid_list = (struct journal_metric_list *)((uint8_t *) journal_header + journal_header->metric_offset);

    Pvoid_t metric_first_time_JudyL = (Pvoid_t) NULL;
    Pvoid_t *PValue;

    unsigned count = 0;
    struct uuid_first_time_s *uuid_first_t_entry;
    for (uint32_t index = 0; index < journal_header->metric_count; ++index) {
        METRIC *metric = mrg_metric_get_and_acquire(main_mrg, &uuid_list[index].uuid, (Word_t) ctx);
        if (!metric)
            continue;

        PValue = JudyLIns(&metric_first_time_JudyL, (Word_t) index, PJE0);
        fatal_assert(NULL != PValue);
        if (!*PValue) {
            uuid_first_t_entry = mallocz(sizeof(*uuid_first_t_entry));
            uuid_first_t_entry->metric = metric;
            uuid_first_t_entry->first_time_t = mrg_metric_get_first_time_t(main_mrg, metric);
            uuid_first_t_entry->last_time_t = mrg_metric_get_latest_time_t(main_mrg, metric);
            uuid_first_t_entry->uuid = mrg_metric_uuid(main_mrg, metric);
            *PValue = uuid_first_t_entry;
            count++;
        }
    }
    info("Recalculating retention for %u metrics", count);
    // Update the first time / last time for all metrics we plan to delete
    worker_is_busy(UV_EVENT_RETENTION_V2);
    find_uuid_first_time(ctx, ctx->datafiles.first->next, metric_first_time_JudyL);

    info("Recalculating retention for %u metrics, done", count);
    worker_is_busy(UV_EVENT_RETENTION_UPDATE);

    Word_t index = 0;
    bool first_then_next = true;
    info("Updating metric registry retention for %u metrics", count);
    while ((PValue = JudyLFirstThenNext(metric_first_time_JudyL, &index, &first_then_next))) {
        uuid_first_t_entry = *PValue;
        mrg_metric_set_first_time_t(main_mrg, uuid_first_t_entry->metric, uuid_first_t_entry->first_time_t);
        mrg_metric_release(main_mrg, uuid_first_t_entry->metric);
        freez(uuid_first_t_entry);
    }
    info("Updating metric registry retention for %u metrics, done", count);
    JudyLFreeArray(&metric_first_time_JudyL, PJE0);
    worker_is_idle();
}


static void do_delete_files(struct rrdengine_worker_config *wc )
{
    struct rrdeng_work *work_request;
    work_request = callocz(1, sizeof(*work_request));
    work_request->req.data = work_request;
    work_request->wc = wc;
    work_request->completion = NULL;
    wc->now_deleting_files = 1;

    int ret = uv_queue_work(wc->loop, &work_request->req, delete_old_data, after_delete_old_data);
    if (ret) {
        wc->now_deleting_files = 0;
        freez(work_request);
    }
}

static void after_do_cache_flush_and_evict(uv_work_t *req, int status __maybe_unused)
{
    struct rrdeng_work  *work_request = req->data;
    struct rrdengine_worker_config *wc = work_request->wc;

    wc->running_cache_flush_evictions = 0;
    freez(work_request);
}

static void do_cache_flush_and_evict(uv_work_t *req)
{
    static __thread int worker = -1;
    if (unlikely(worker == -1))
        register_libuv_worker_jobs();

    struct rrdeng_work *work_request = req->data;
    struct rrdengine_worker_config *wc = work_request->wc;

    if (wc->cache_flush_can_run) {
        if (main_cache) {
            worker_is_busy(UV_EVENT_FLUSH_MAIN);
            pgc_flush_pages(main_cache, 0);
        }
        if (open_cache) {
            worker_is_busy(UV_EVENT_FLUSH_OPEN);
            pgc_flush_pages(open_cache, 0);
        }
    }

    if (0 == wc->ctx->tier) {
        if (main_cache) {
            worker_is_busy(UV_EVENT_EVICT_MAIN);
            pgc_evict_pages(main_cache, 0, 0);
        }
    }

    worker_is_idle();
}

static void cache_flush_and_evict(struct rrdengine_worker_config *wc)
{
    struct rrdeng_work *work_request;

    if (unlikely(wc->running_cache_flush_evictions))
        return;

    work_request = callocz(1, sizeof(*work_request));
    work_request->req.data = work_request;
    work_request->wc = wc;
    wc->running_cache_flush_evictions = 1;
    if (unlikely(uv_queue_work(wc->loop, &work_request->req, do_cache_flush_and_evict, after_do_cache_flush_and_evict))) {
        freez(work_request);
        wc->running_cache_flush_evictions = 0;
    }
}

unsigned rrdeng_target_data_file_size(struct rrdengine_instance *ctx) {
    unsigned target_size = ctx->max_disk_space / TARGET_DATAFILES;
    target_size = MIN(target_size, MAX_DATAFILE_SIZE);
    target_size = MAX(target_size, MIN_DATAFILE_SIZE);
    return target_size;
}

static void rrdeng_test_quota(struct rrdengine_worker_config* wc)
{
    struct rrdengine_instance *ctx = wc->ctx;
    struct rrdengine_datafile *datafile;
    unsigned current_size, target_size;
    uint8_t out_of_space, only_one_datafile;
    int ret;

    out_of_space = 0;
    /* Do not allow the pinned pages to exceed the disk space quota to avoid deadlocks */
    if (unlikely(ctx->disk_space > MAX(ctx->max_disk_space, 2 * ctx->metric_API_max_producers * RRDENG_BLOCK_SIZE))) {
        out_of_space = 1;
    }
    datafile = ctx->datafiles.first->prev;
    current_size = datafile->pos;
    target_size = rrdeng_target_data_file_size(ctx);
    only_one_datafile = (datafile == ctx->datafiles.first) ? 1 : 0;

    if (unlikely(current_size >= target_size || (out_of_space && only_one_datafile))) {
        /* Finalize data and journal file and create a new pair */
        struct rrdengine_journalfile *journalfile = unlikely(NULL == ctx->datafiles.first) ? NULL : ctx->datafiles.first->prev->journalfile;
        wal_flush_transaction_buffer(wc);
        ret = create_new_datafile_pair(ctx, 1, ctx->last_fileno + 1);
        if (likely(!ret)) {
            ++ctx->last_fileno;
            if (likely(journalfile))
                wc->run_indexing = true;
        }
    }

    if (unlikely(out_of_space && NO_QUIESCE == ctx->quiesce && false == __atomic_load_n(&ctx->journal_initialization, __ATOMIC_RELAXED))) {
        /* delete old data */
        if (wc->now_deleting_files) {
            /* already deleting data */
            return;
        }
        if (NULL == ctx->datafiles.first->next) {
            error("Cannot delete data file \"%s/" DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION "\""
                  " to reclaim space, there are no other file pairs left.",
                  ctx->dbfiles_path, ctx->datafiles.first->tier, ctx->datafiles.first->fileno);
            return;
        }
        struct rrdengine_datafile *df = ctx->datafiles.first;
        if(!datafile_acquire_for_deletion(df, false)) {
            error("Cannot delete data file \"%s/" DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION "\""
                  " to reclaim space, it is in use currently by %u users, but it has been marked as not available for queries to stop using it."
                  " There are %zu inflight queries and %zu open cache pages reference this datafile.",
                  ctx->dbfiles_path, df->tier, df->fileno, df->users.lockers,
                  __atomic_load_n(&ctx->inflight_queries, __ATOMIC_RELAXED),
                  pgc_count_pages_having_data_ptr(open_cache, datafile));
            return;
        }
        info("Deleting data file \"%s/" DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION "\".",
             ctx->dbfiles_path, ctx->datafiles.first->tier, ctx->datafiles.first->fileno);

        do_delete_files(wc);
    }
}

static inline int rrdeng_threads_alive(struct rrdengine_worker_config* wc)
{
    if (wc->now_deleting_files)
        return 1;

    return 0;
}

static void rrdeng_cleanup_finished_threads(struct rrdengine_worker_config* wc)
{
    struct rrdengine_instance *ctx = wc->ctx;

    if (unlikely(SET_QUIESCE == ctx->quiesce && !rrdeng_threads_alive(wc) && !wc->outstanding_flush_requests)) {
        wal_flush_transaction_buffer(wc);
        ctx->quiesce = QUIESCED;
        completion_mark_complete(&ctx->rrdengine_completion);
    }
}

/* return 0 on success */
int init_rrd_files(struct rrdengine_instance *ctx)
{
    return init_data_files(ctx);
}

void finalize_rrd_files(struct rrdengine_instance *ctx)
{
    return finalize_data_files(ctx);
}

void rrdeng_init_cmd_queue(struct rrdengine_worker_config* wc)
{
    sanity_check();
    wc->cmd_queue.head = wc->cmd_queue.tail = 0;
    wc->queue_size = 0;
    fatal_assert(0 == uv_cond_init(&wc->cmd_cond));
    fatal_assert(0 == uv_mutex_init(&wc->cmd_mutex));
}

void rrdeng_enq_cmd(struct rrdengine_worker_config* wc, struct rrdeng_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&wc->cmd_mutex);
    while ((queue_size = wc->queue_size) == RRDENG_CMD_Q_MAX_SIZE) {
        uv_cond_wait(&wc->cmd_cond, &wc->cmd_mutex);
    }
    fatal_assert(queue_size < RRDENG_CMD_Q_MAX_SIZE);
    /* enqueue command */
    wc->cmd_queue.cmd_array[wc->cmd_queue.tail] = *cmd;
    wc->cmd_queue.tail = wc->cmd_queue.tail != RRDENG_CMD_Q_MAX_SIZE - 1 ?
                         wc->cmd_queue.tail + 1 : 0;
    wc->queue_size = queue_size + 1;
    uv_mutex_unlock(&wc->cmd_mutex);

    /* wake up event loop */
    fatal_assert(0 == uv_async_send(&wc->async));
}

struct rrdeng_cmd rrdeng_deq_cmd(struct rrdengine_worker_config* wc)
{
    struct rrdeng_cmd ret;
    unsigned queue_size;

    uv_mutex_lock(&wc->cmd_mutex);
    queue_size = wc->queue_size;
    if (queue_size == 0) {
        ret.opcode = RRDENG_NOOP;
    } else {
        /* dequeue command */
        ret = wc->cmd_queue.cmd_array[wc->cmd_queue.head];
        if (queue_size == 1) {
            wc->cmd_queue.head = wc->cmd_queue.tail = 0;
        } else {
            wc->cmd_queue.head = wc->cmd_queue.head != RRDENG_CMD_Q_MAX_SIZE - 1 ?
                                 wc->cmd_queue.head + 1 : 0;
        }
        wc->queue_size = queue_size - 1;

        /* wake up producers */
        uv_cond_signal(&wc->cmd_cond);
    }
    uv_mutex_unlock(&wc->cmd_mutex);

    return ret;
}

void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    debug(D_RRDENGINE, "%s called, active=%d.", __func__, uv_is_active((uv_handle_t *)handle));
}

#define TIMER_PERIOD_MS (1000)

void timer_cb(uv_timer_t* handle)
{
    worker_is_busy(RRDENG_MAX_OPCODE + 1);

    struct rrdengine_worker_config *wc = handle->data;
    uv_stop(handle->loop);
    uv_update_time(handle->loop);

    if (wc->ctx->quiesce != NO_QUIESCE) {
        worker_is_idle();
        return;
    }

    rrdeng_test_quota(wc);

    debug(D_RRDENGINE, "%s: timeout reached.", __func__);

    if (true == wc->run_indexing && !wc->now_deleting_files && !wc->running_cache_flush_evictions) {
        wc->run_indexing = false;
        queue_journalfile_v2_migration(wc);
    }

    cache_flush_and_evict(wc);

#ifdef NETDATA_INTERNAL_CHECKS
    {
        char buf[4096];
        debug(D_RRDENGINE, "%s", get_rrdeng_statistics(wc->ctx, buf, sizeof(buf)));
    }
#endif
    worker_is_idle();
}

static void after_do_read_extent_work(uv_work_t *req, int status __maybe_unused)
{
    struct rrdeng_work  *work_request = req->data;
    freez(work_request);
}

static bool datafile_get_exclusive_access_to_extent(EXTENT_PD_LIST *extent_page_list) {
    struct rrdengine_datafile *df = extent_page_list->datafile;
    bool is_it_mine = false;

    while(!is_it_mine) {
        netdata_spinlock_lock(&df->extent_exclusive_access.spinlock);
        if(!df->users.available) {
            netdata_spinlock_unlock(&df->extent_exclusive_access.spinlock);
            return false;
        }
        Pvoid_t *PValue = JudyLIns(&df->extent_exclusive_access.extents_JudyL, extent_page_list->extent_offset, PJE0);
        if (!*PValue) {
            *(Word_t *) PValue = gettid();
            df->extent_exclusive_access.lockers++;
            is_it_mine = true;
        }
        netdata_spinlock_unlock(&df->extent_exclusive_access.spinlock);

        if(!is_it_mine) {
            static const struct timespec ns = { .tv_sec = 0, .tv_nsec = 1 };
            nanosleep(&ns, NULL);
        }
    }
    return true;
}

static void datafile_release_exclusive_access_to_extent(EXTENT_PD_LIST *extent_page_list) {
    struct rrdengine_datafile *df = extent_page_list->datafile;

    netdata_spinlock_lock(&df->extent_exclusive_access.spinlock);

#ifdef NETDATA_INTERNAL_CHECKS
    Pvoid_t *PValue = JudyLGet(df->extent_exclusive_access.extents_JudyL, extent_page_list->extent_offset, PJE0);
    if (*(Word_t *) PValue != (Word_t)gettid())
        fatal("DBENGINE: exclusive extent access is not mine");
#endif

    int rc = JudyLDel(&df->extent_exclusive_access.extents_JudyL, extent_page_list->extent_offset, PJE0);
    if (!rc)
        fatal("DBENGINE: cannot find my exclusive access");

    df->extent_exclusive_access.lockers--;
    netdata_spinlock_unlock(&df->extent_exclusive_access.spinlock);
}

static void load_pages_from_an_extent_list(struct rrdengine_instance *ctx, EXTENT_PD_LIST *extent_page_list, bool worker) {
    struct page_details_control *pdc = extent_page_list->pdc;

    bool extent_exclusive = false;

    if(pdc->preload_all_extent_pages) {
        if (!datafile_get_exclusive_access_to_extent(extent_page_list)) {
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_fail_datafile_not_available, 1, __ATOMIC_RELAXED);
            goto cleanup;
        }
        extent_exclusive = true;
    }

    if (extent_list_check_if_pages_are_already_in_cache(ctx, extent_page_list, PDC_PAGE_PRELOADED_WORKER))
        goto cleanup;

    if(__atomic_load_n(&pdc->workers_should_stop, __ATOMIC_RELAXED))
        goto cleanup;

    if(worker)
        worker_is_busy(UV_EVENT_EXTENT_CACHE);

    PDC_PAGE_STATUS not_loaded_pages_tag = 0, loaded_pages_tag = 0;
    bool extent_found_in_cache = false;

    void *extent_compressed_data = NULL;
    PGC_PAGE *extent_cache_page = pgc_page_get_and_acquire(extent_cache, (Word_t)ctx, (Word_t)extent_page_list->datafile->fileno, (time_t)extent_page_list->extent_offset, true);
    if(extent_cache_page) {
        extent_compressed_data = pgc_page_data(extent_cache_page);
        internal_fatal(extent_page_list->extent_size != pgc_page_data_size(extent_cache, extent_cache_page),
                       "DBENGINE: cache size does not match the expected size");

        loaded_pages_tag |= PDC_PAGE_LOADED_FROM_EXTENT_CACHE;
        not_loaded_pages_tag |= PDC_PAGE_LOADED_FROM_EXTENT_CACHE;
        extent_found_in_cache = true;
    }
    else {
        if(worker)
            worker_is_busy(UV_EVENT_EXTENT_MMAP);

        off_t map_start =  ALIGN_BYTES_FLOOR(extent_page_list->extent_offset);
        size_t length = ALIGN_BYTES_CEILING(extent_page_list->extent_offset + extent_page_list->extent_size) - map_start;

        void *mmap_data = mmap(NULL, length, PROT_READ, MAP_SHARED, extent_page_list->file, map_start);
        if(mmap_data != MAP_FAILED) {
            extent_compressed_data = mmap_data + (extent_page_list->extent_offset - map_start);

            void *copied_extent_compressed_data = mallocz(extent_page_list->extent_size);
            memcpy(copied_extent_compressed_data, extent_compressed_data, extent_page_list->extent_size);

            int ret = munmap(mmap_data, length);
            fatal_assert(0 == ret);

            if(worker)
                worker_is_busy(UV_EVENT_EXTENT_CACHE);

            bool added = false;
            extent_cache_page = pgc_page_add_and_acquire(extent_cache, (PGC_ENTRY) {
                    .hot = false,
                    .section = (Word_t) ctx,
                    .metric_id = (Word_t) extent_page_list->datafile->fileno,
                    .start_time_t = (time_t) extent_page_list->extent_offset,
                    .size = extent_page_list->extent_size,
                    .end_time_t = 0,
                    .update_every = 0,
                    .data = copied_extent_compressed_data,
            }, &added);

            if (!added) {
                freez(copied_extent_compressed_data);
                internal_fatal(extent_page_list->extent_size != pgc_page_data_size(extent_cache, extent_cache_page),
                               "DBENGINE: cache size does not match the expected size");
            }

            extent_compressed_data = pgc_page_data(extent_cache_page);

            loaded_pages_tag |= PDC_PAGE_LOADED_FROM_DISK;
            not_loaded_pages_tag |= PDC_PAGE_LOADED_FROM_DISK;
        }
    }

    if(extent_compressed_data) {
        // Need to decompress and then process the pagelist
        bool extent_used = extent_uncompress_and_populate_pages(
                ctx, extent_compressed_data, extent_page_list->extent_size,
                extent_page_list, pdc->preload_all_extent_pages,
                worker, loaded_pages_tag, extent_found_in_cache);

        if(extent_used) {
            // since the extent was used, all the pages that are not
            // loaded from this extent, were not found in the extent
            not_loaded_pages_tag |= PDC_PAGE_FAILED_UUID_NOT_IN_EXTENT;
        }
        else
            not_loaded_pages_tag |= PDC_PAGE_FAILED_INVALID_EXTENT;
    }
    else
        not_loaded_pages_tag |= PDC_PAGE_FAILED_TO_MAP_EXTENT;


    // mark all pending pages as failed
    extent_list_mark_all_not_loaded_pages_as_failed(
            extent_page_list, not_loaded_pages_tag,
            &rrdeng_cache_efficiency_stats.pages_load_fail_cant_mmap_extent);

    if(extent_cache_page)
        pgc_page_release(extent_cache, extent_cache_page);

cleanup:
    if(extent_exclusive)
        datafile_release_exclusive_access_to_extent(extent_page_list);

    completion_mark_complete_a_job(&extent_page_list->pdc->completion);
    pdc_release_and_destroy_if_unreferenced(pdc, true, false);

    // Free the Judy that holds the requested pagelist and the extents
    extent_list_free(extent_page_list);

    if(worker)
        worker_is_idle();
}

static void do_read_extent_work(uv_work_t *req)
{
    static __thread int worker = -1;
    if (unlikely(worker == -1))
        register_libuv_worker_jobs();

    struct rrdeng_work *work_request = req->data;
    struct rrdengine_worker_config *wc = work_request->wc;
    struct rrdengine_instance *ctx = wc->ctx;

    EXTENT_PD_LIST *extent_page_list = work_request->data;

    load_pages_from_an_extent_list(ctx, extent_page_list, true);
}

static void do_read_extent(struct rrdengine_worker_config *wc, void *data)
{
    struct rrdeng_work *work_request;

    work_request = mallocz(sizeof(*work_request));
    work_request->req.data = work_request;
    work_request->wc = wc;
    work_request->data = data;
    work_request->completion = NULL;

    int ret = uv_queue_work(wc->loop, &work_request->req, do_read_extent_work, after_do_read_extent_work);
    if (ret)
        freez(work_request);
    fatal_assert(ret == 0);
}

void after_do_read_page_list_work(uv_work_t *req, int status __maybe_unused)
{
    struct rrdeng_work  *work_request = req->data;

    // Execute callback
    if (work_request->completion)
        completion_mark_complete(work_request->completion);

    freez(work_request);
}

static void queue_extent_list(struct rrdengine_instance *ctx, EXTENT_PD_LIST *extent_page_list) {
    struct rrdeng_cmd cmd;
    cmd.opcode = RRDENG_READ_EXTENT;
    cmd.data = extent_page_list;
    rrdeng_enq_cmd(&ctx->worker_config, &cmd);
}

static void do_read_page_list_work(uv_work_t *req)
{
    static __thread int worker = -1;
    if (unlikely(worker == -1))
        register_libuv_worker_jobs();

    worker_is_busy(UV_EVENT_PAGE_DISPATCH);

    struct rrdeng_work *work_request = req->data;
    struct rrdengine_worker_config *wc = work_request->wc;
    struct rrdengine_instance *ctx = wc->ctx;
    struct page_details_control *pdc = work_request->data;

    pdc_to_extent_page_details_list(ctx, pdc, queue_extent_list, queue_extent_list);
    worker_is_idle();
}

static void do_read_page_list(struct rrdengine_worker_config *wc, struct page_details_control *pdc, struct completion *completion)
{
    struct rrdeng_work *work_request;

    work_request = mallocz(sizeof(*work_request));
    work_request->req.data = work_request;
    work_request->wc = wc;
    work_request->data = pdc;
    work_request->completion = completion;

    int ret = uv_queue_work(wc->loop, &work_request->req, do_read_page_list_work, after_do_read_page_list_work);
    if (ret)
        freez(work_request);

    fatal_assert(0 == ret);
}

// List of pages to preload
// Just queue to dbengine
void dbengine_load_page_list(struct rrdengine_instance *ctx, struct page_details_control *pdc)
{

    pdc_to_extent_page_details_list(ctx, pdc, queue_extent_list, queue_extent_list);

//    struct rrdeng_cmd cmd;
//    cmd.opcode = RRDENG_READ_PAGE_LIST;
//    cmd.data = pdc;
//    cmd.completion = NULL;
//    rrdeng_enq_cmd(&ctx->worker_config, &cmd);
}

void load_pages_from_an_extent_list_directly(struct rrdengine_instance *ctx, EXTENT_PD_LIST *extent_list) {
    load_pages_from_an_extent_list(ctx, extent_list, false);
}

void dbengine_load_page_list_directly(struct rrdengine_instance *ctx, struct page_details_control *pdc) {
    pdc_to_extent_page_details_list(ctx, pdc, load_pages_from_an_extent_list_directly, queue_extent_list);
}

#define MAX_CMD_BATCH_SIZE (256)

void rrdeng_worker(void* arg)
{
    worker_register("DBENGINE");
    worker_register_job_name(RRDENG_NOOP,                          "noop");
    worker_register_job_name(RRDENG_READ_EXTENT,                   "extent read");
    worker_register_job_name(RRDENG_COMMIT_PAGE,                   "commit");
    worker_register_job_name(RRDENG_FLUSH_PAGES,                   "flush");
    worker_register_job_name(RRDENG_SHUTDOWN,                      "shutdown");
    worker_register_job_name(RRDENG_QUIESCE,                       "quiesce");
    worker_register_job_name(RRDENG_READ_PAGE_LIST,                "query page list");
    worker_register_job_name(RRDENG_MAX_OPCODE,                    "cleanup");
    worker_register_job_name(RRDENG_MAX_OPCODE + 1,                "timer");

    struct rrdengine_worker_config* wc = arg;
    struct rrdengine_instance *ctx = wc->ctx;
    uv_loop_t* loop;
    int shutdown, ret;
    enum rrdeng_opcode opcode;
    uv_timer_t timer_req;
    struct rrdeng_cmd cmd;
    unsigned cmd_batch_size;

    rrdeng_init_cmd_queue(wc);

    loop = wc->loop = mallocz(sizeof(uv_loop_t));
    ret = uv_loop_init(loop);
    if (ret) {
        error("uv_loop_init(): %s", uv_strerror(ret));
        goto error_after_loop_init;
    }
    loop->data = wc;

    ret = uv_async_init(wc->loop, &wc->async, async_cb);
    if (ret) {
        error("uv_async_init(): %s", uv_strerror(ret));
        goto error_after_async_init;
    }
    wc->async.data = wc;

    wc->now_deleting_files = 0;
    wc->running_journal_migration = 0;
    wc->running_cache_flush_evictions = 0;
    wc->run_indexing = false;
    wc->cache_flush_can_run = true;

    /* dirty page flushing timer */
    ret = uv_timer_init(loop, &timer_req);
    if (ret) {
        error("uv_timer_init(): %s", uv_strerror(ret));
        goto error_after_timer_init;
    }
    timer_req.data = wc;

    wc->error = 0;
    /* wake up initialization thread */
    completion_mark_complete(&ctx->rrdengine_completion);

    fatal_assert(0 == uv_timer_start(&timer_req, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));
    shutdown = 0;
    int set_name = 0;
    wc->outstanding_flush_requests = 0;
    while (likely(shutdown == 0 || rrdeng_threads_alive(wc))) {
        worker_is_idle();
        uv_run(loop, UV_RUN_DEFAULT);
        worker_is_busy(RRDENG_MAX_OPCODE);
        rrdeng_cleanup_finished_threads(wc);

        /* wait for commands */
        cmd_batch_size = 0;
        do {
            /*
             * Avoid starving the loop when there are too many commands coming in.
             * timer_cb will interrupt the loop again to allow serving more commands.
             */
            if (unlikely(cmd_batch_size >= MAX_CMD_BATCH_SIZE))
                break;

            cmd = rrdeng_deq_cmd(wc);
            opcode = cmd.opcode;
            ++cmd_batch_size;

            if(likely(opcode != RRDENG_NOOP))
                worker_is_busy(opcode);

            switch (opcode) {
                case RRDENG_NOOP:
                    /* the command queue was empty, do nothing */
                    break;
                case RRDENG_SHUTDOWN:
                    shutdown = 1;
                    break;
                case RRDENG_QUIESCE:
                    ctx->quiesce = SET_QUIESCE;
                    info("Shutdown command received. Flushing all pages to disk. %u flush requests pending", wc->outstanding_flush_requests);
                    wal_flush_transaction_buffer(wc);
                    if (!rrdeng_threads_alive(wc) && !wc->outstanding_flush_requests) {
                        wal_flush_transaction_buffer(wc);
                        ctx->quiesce = QUIESCED;
                        completion_mark_complete(&ctx->rrdengine_completion);
                    }
                    break;
                case RRDENG_COMMIT_PAGE:
                    do_commit_transaction(wc, STORE_DATA, NULL);
                    break;
                case RRDENG_FLUSH_PAGES: {
                    wc->cache_flush_can_run = false;
                    wc->outstanding_flush_requests++;
                    (void)do_flush_extent(wc, cmd.data, cmd.completion);
                    break;
                }
                case RRDENG_READ_EXTENT:
                    if (unlikely(!set_name)) {
                        set_name = 1;
                        uv_thread_set_name_np(ctx->worker_config.thread, "DBENGINE");
                    }
                    do_read_extent(wc, cmd.data);
                    break;
                case RRDENG_READ_PAGE_LIST:
                    do_read_page_list(wc, cmd.data, cmd.completion);
                    break;
                default:
                    debug(D_RRDENGINE, "%s: default.", __func__);
                    break;
            }
        } while (opcode != RRDENG_NOOP);
    }

    /* cleanup operations of the event loop */
    info("Shutting down RRD engine event loop for tier %d", ctx->tier);

    /*
     * uv_async_send after uv_close does not seem to crash in linux at the moment,
     * it is however undocumented behaviour and we need to be aware if this becomes
     * an issue in the future.
     */
    uv_close((uv_handle_t *)&wc->async, NULL);

    fatal_assert(0 == uv_timer_stop(&timer_req));
    uv_close((uv_handle_t *)&timer_req, NULL);

    // FIXME: Check if we need to ask page cache if flushing is done
    wal_flush_transaction_buffer(wc);
    uv_run(loop, UV_RUN_DEFAULT);

    info("Shutting down RRD engine event loop for tier %d complete", ctx->tier);
    /* TODO: don't let the API block by waiting to enqueue commands */
    uv_cond_destroy(&wc->cmd_cond);
/*  uv_mutex_destroy(&wc->cmd_mutex); */
    fatal_assert(0 == uv_loop_close(loop));
    freez(loop);

    worker_unregister();
    return;

    error_after_timer_init:
    uv_close((uv_handle_t *)&wc->async, NULL);
    error_after_async_init:
    fatal_assert(0 == uv_loop_close(loop));
    error_after_loop_init:
    freez(loop);

    wc->error = UV_EAGAIN;
    /* wake up initialization thread */
    completion_mark_complete(&ctx->rrdengine_completion);
    worker_unregister();
}
