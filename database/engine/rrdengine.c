// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

rrdeng_stats_t global_io_errors = 0;
rrdeng_stats_t global_fs_errors = 0;
rrdeng_stats_t rrdeng_reserved_file_descriptors = 0;
rrdeng_stats_t global_pg_cache_over_half_dirty_events = 0;
rrdeng_stats_t global_flushing_pressure_page_deletions = 0;

unsigned rrdeng_pages_per_extent = MAX_PAGES_PER_EXTENT;

struct datafile_extent_list_s {
    uv_file file;
    unsigned fileno;
    Pvoid_t JudyL_datafile_extent_list;
    struct page_details_control *pdc;
};

struct extent_page_list_s {
    uv_file file;
    uint64_t pos;
    uint32_t size;
    unsigned count;
    Pvoid_t JudyL_page_list;
    struct page_details_control *pdc;
    struct rrdengine_datafile *datafile;
};

#if WORKER_UTILIZATION_MAX_JOB_TYPES < (RRDENG_MAX_OPCODE + 2)
#error Please increase WORKER_UTILIZATION_MAX_JOB_TYPES to at least (RRDENG_MAX_OPCODE + 2)
#endif

void *dbengine_page_alloc() {
    void *page = NULL;
    if (unlikely(db_engine_use_malloc))
        page = mallocz(RRDENG_BLOCK_SIZE);
    else {
        page = netdata_mmap(NULL, RRDENG_BLOCK_SIZE, MAP_PRIVATE, enable_ksm, false);
        if(!page) fatal("Cannot allocate dbengine page cache page, with mmap()");
    }
    return page;
}

void dbengine_page_free(void *page) {
    if (unlikely(db_engine_use_malloc))
        freez(page);
    else
        netdata_munmap(page, RRDENG_BLOCK_SIZE);
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

static void pdc_mark_all_unset_pages_as_failed(Pvoid_t JudyL, PDC_PAGE_STATUS reason, size_t *statistics_counter) {
    bool first_then_next = true;
    Pvoid_t *PValue;
    Word_t Index = 0;
    size_t pages_matched = 0;

    while((PValue = JudyLFirstThenNext(JudyL, &Index, &first_then_next))) {
        struct page_details *pd = *PValue;

        if(!pd->page) {
            pdc_page_status_set(pd, PDC_PAGE_FAILED | reason);
            pages_matched++;
        }
    }

    if(pages_matched && statistics_counter)
        __atomic_add_fetch(statistics_counter, pages_matched, __ATOMIC_RELAXED);
}

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

bool pdc_release_and_destroy_if_unreferenced(PDC *pdc, bool worker, bool router) {
    netdata_spinlock_lock(&pdc->refcount_spinlock);

    if(pdc->refcount <= 0)
        fatal("PDC is not referenced and cannot be released");

    pdc->refcount--;

    if(!worker && !router)
        pdc->query_thread_left = true;

    if (pdc->refcount <= 1 && worker) {
        // when 1 refcount is remaining, and we are a worker,
        // we can mark the job completed:
        // - if the remaining refcount is from the query caller, we will wake it up
        // - if the remaining refcount is from a worker, the caller is already away
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
        )
        is_valid = false;

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

// DBENGINE2 extent processing
static void extent_uncompress_and_populate_pages(struct rrdengine_worker_config *wc, void *data, size_t data_length, struct extent_page_list_s *extent_page_list, bool preload_all_pages)
{
    struct rrdengine_instance *ctx = wc->ctx;
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
    if(data_length < sizeof(*header)) {
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
        count > MAX_PAGES_PER_EXTENT ||
        (header->compression_algorithm != RRD_NO_COMPRESSION && header->compression_algorithm != RRD_LZ4) ||
        (payload_length != trailer_offset - payload_offset) ||
        (data_length != payload_offset + payload_length + sizeof(*trailer))
        ) {

        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, but header is INVALID", __func__,
                    extent_page_list->pos, extent_page_list->size, extent_page_list->datafile->fileno);

        pdc_mark_all_unset_pages_as_failed(
                extent_page_list->JudyL_page_list, PDC_PAGE_INVALID_EXTENT,
                &rrdeng_cache_efficiency_stats.pages_load_fail_invalid_extent);

        return;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    void *data_copy = mallocz(data_length);
    memcpy(data_copy, data, data_length);
#endif

    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, data, extent_page_list->size  - sizeof(*trailer));
    ret = crc32cmp(trailer->checksum, crc);
    if (unlikely(ret)) {
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        have_read_error = 1;

        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, but CRC32 check FAILED", __func__,
                    extent_page_list->pos, extent_page_list->size, extent_page_list->datafile->fileno);
    }

    worker_is_busy(UV_EVENT_EXT_DECOMPRESSION);// SPDX-License-Identifier: GPL-3.0-or-later

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

    worker_is_busy(UV_EVENT_PAGE_POPULATION);
    time_t now_s = now_realtime_sec();

    uint32_t page_offset = 0, page_length = 0;
    for (i = 0; i < count; i++, page_offset += page_length) {
        page_length = header->descr[i].page_length;
        time_t start_time_s = (time_t) (header->descr[i].start_time_ut / USEC_PER_SEC);

        if(!page_length || !start_time_s) {
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, having page %u (out of %u) EMPTY",
                        __func__, extent_page_list->pos, extent_page_list->size, extent_page_list->datafile->fileno, i, count);
            continue;
        }

        Pvoid_t *PValue = JudyLGet(extent_page_list->JudyL_page_list, start_time_s, PJE0);
        struct page_details *pd = (NULL == PValue || NULL == *PValue) ? NULL : *PValue;

        // We might have an Index match but check for UUID match as well
        // if there is no match we will add the page in cache but not in
        // the preload response

        if (pd && uuid_compare(pd->uuid, header->descr[i].uuid))
            pd = NULL;

        if(!preload_all_pages && !pd)
            continue;

        VALIDATED_PAGE_DESCRIPTOR vd = validate_extent_page_descr(&header->descr[i], now_s, (pd) ? pd->update_every_s : 0, have_read_error);

        // Find metric id
        METRIC *this_metric = mrg_metric_get_and_acquire(main_mrg, &header->descr[i].uuid, (Word_t) ctx);
        Word_t metric_id = mrg_metric_id(main_mrg, this_metric);
        mrg_metric_release(main_mrg, this_metric);

        PGC_PAGE *page = pgc_page_get_and_acquire(main_cache, (Word_t)ctx, (Word_t)metric_id, start_time_s, true);
        if (!page) {
            void *page_data = mallocz((size_t) vd.page_length);

            if (unlikely(!vd.data_on_disk_valid)) {
                fill_page_with_nulls(page_data, vd.page_length, vd.type);
                __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_fail_invalid_page_in_extent, 1, __ATOMIC_RELAXED);
            }

            else if (RRD_NO_COMPRESSION == header->compression_algorithm) {
                memcpy(page_data, data + payload_offset + page_offset, (size_t) vd.page_length);
                __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_uncompressed, 1, __ATOMIC_RELAXED);
            }

            else {
                if(unlikely(page_offset + vd.page_length > uncompressed_payload_length)) {
                    error_limit_static_global_var(erl, 10, 0);
                    error_limit(&erl,
                                   "DBENGINE: page %u offset %u + page length %zu exceeds the uncompressed buffer size %u",
                                   i, page_offset, vd.page_length, uncompressed_payload_length);

                    fill_page_with_nulls(page_data, vd.page_length, vd.type);
                    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_fail_invalid_page_in_extent, 1, __ATOMIC_RELAXED);
                }
                else {
                    memcpy(page_data, uncompressed_buf + page_offset, vd.page_length);
                    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_compressed, 1, __ATOMIC_RELAXED);
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
                __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_loaded_but_found_in_cache, 1, __ATOMIC_RELAXED);
            }
        }
        else
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_loaded_but_found_in_cache, 1, __ATOMIC_RELAXED);

        if (pd) {
            pd->page = page;
            pd->page_length = pgc_page_data_size(main_cache, page);
            pdc_page_status_set(pd, PDC_PAGE_READY);
        }
        else
            pgc_page_release(main_cache, page);
    }

    pdc_mark_all_unset_pages_as_failed(
            extent_page_list->JudyL_page_list, PDC_PAGE_UUID_NOT_FOUND_IN_EXTENT,
            &rrdeng_cache_efficiency_stats.pages_load_fail_uuid_not_found);

    freez(uncompressed_buf);

#ifdef NETDATA_INTERNAL_CHECKS
    freez(data_copy);
#endif
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
        descr->id = &descr->uuid;   // FIXME:

        METRIC *this_metric = mrg_metric_get_and_acquire(main_mrg, &descr->uuid, (Word_t) ctx);
        Word_t metric_id = mrg_metric_id(main_mrg, this_metric);

        struct extent_io_data ext_io_data = {
            .fileno = datafile->fileno,
            .file  = datafile->file,
            .pos = xt_io_descr->pos,
            .bytes = xt_io_descr->bytes
        };

        PGC_ENTRY page_entry = {
            .hot = true,
            .section = (Word_t)ctx,
            .metric_id = metric_id,
            .start_time_t = (time_t) (descr->start_time_ut / USEC_PER_SEC),
            .end_time_t =  (time_t) (descr->end_time_ut / USEC_PER_SEC),
            .update_every = descr->update_every_s,
            .size = 0,
            .data = datafile,
            .custom_data = (uint8_t *) &ext_io_data
        };

        bool added = true;
        int tries = 100;
        PGC_PAGE *page = pgc_page_add_and_acquire(open_cache, page_entry, &added);
        while(!added && tries--) {
            pgc_page_hot_to_clean_empty_and_release(open_cache, page);
            page = pgc_page_add_and_acquire(open_cache, page_entry, &added);
        }
        fatal_assert(true == added);
        pgc_page_release(open_cache, (PGC_PAGE *)page);
        mrg_metric_release(main_mrg, this_metric);
    }
    if (xt_io_descr->completion)
        completion_mark_complete(xt_io_descr->completion);

    uv_fs_req_cleanup(req);
    posix_memfree(xt_io_descr->buf);
    freez(xt_io_descr);
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
        uuid_copy(*(uuid_t *)header->descr[i].uuid, descr->uuid);
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
    return ALIGN_BYTES_CEILING(size_bytes);
}

static void after_delete_old_data(struct rrdengine_worker_config* wc)
{
    struct rrdengine_instance *ctx = wc->ctx;
    struct rrdengine_datafile *datafile;
    struct rrdengine_journalfile *journalfile;
    unsigned deleted_bytes, journalfile_bytes, datafile_bytes;
    int ret, error;
    char path[RRDENG_PATH_MAX];

    uv_rwlock_wrlock(&ctx->datafiles.rwlock);

    datafile = ctx->datafiles.first;
    journalfile = datafile->journalfile;
    datafile_bytes = datafile->pos;
    journalfile_bytes = journalfile->pos;
    deleted_bytes = journalfile->journal_data_size;

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

    error = uv_thread_join(wc->now_deleting_files);
    if (error) {
        error("uv_thread_join(): %s", uv_strerror(error));
    }
    freez(wc->now_deleting_files);
    /* unfreeze command processing */
    wc->now_deleting_files = NULL;

    wc->cleanup_thread_deleting_files = 0;

    uv_rwlock_wrunlock(&ctx->datafiles.rwlock);

    rrdcontext_db_rotation();

    /* interrupt event loop */
    uv_stop(wc->loop);
}

static void delete_old_data(void *arg)
{
    struct rrdengine_instance *ctx = arg;
    struct rrdengine_worker_config *wc = &ctx->worker_config;

    // FIXME: Check if metadata needs to be handled
    wc->cleanup_thread_deleting_files = 1;
    /* wake up event loop */
    fatal_assert(0 == uv_async_send(&wc->async));
}

void rrdeng_test_quota(struct rrdengine_worker_config* wc)
{
    struct rrdengine_instance *ctx = wc->ctx;
    struct rrdengine_datafile *datafile;
    unsigned current_size, target_size;
    uint8_t out_of_space, only_one_datafile;
    int ret, error;

    out_of_space = 0;
    /* Do not allow the pinned pages to exceed the disk space quota to avoid deadlocks */
    if (unlikely(ctx->disk_space > MAX(ctx->max_disk_space, 2 * ctx->metric_API_max_producers * RRDENG_BLOCK_SIZE))) {
        out_of_space = 1;
    }
    datafile = ctx->datafiles.first->prev;
    current_size = datafile->pos;
    target_size = ctx->max_disk_space / TARGET_DATAFILES;
    target_size = MIN(target_size, MAX_DATAFILE_SIZE);
    target_size = MAX(target_size, MIN_DATAFILE_SIZE);
    only_one_datafile = (datafile == ctx->datafiles.first) ? 1 : 0;

    if (unlikely(current_size >= target_size || (out_of_space && only_one_datafile))) {
        /* Finalize data and journal file and create a new pair */
        struct rrdengine_journalfile *journalfile = unlikely(NULL == ctx->datafiles.first) ? NULL : ctx->datafiles.first->prev->journalfile;
        wal_flush_transaction_buffer(wc);
        ret = create_new_datafile_pair(ctx, 1, ctx->last_fileno + 1);
        if (likely(!ret)) {
            ++ctx->last_fileno;
            if (likely(journalfile && db_engine_journal_indexing))
                wc->run_indexing = true;
        }
    }

    if (unlikely(out_of_space && NO_QUIESCE == ctx->quiesce && false == ctx->journal_initialization)) {
        /* delete old data */
        if (wc->now_deleting_files) {
            /* already deleting data */
            return;
        }
        if (NULL == ctx->datafiles.first->next) {
            error("Cannot delete data file \"%s/"DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION"\""
                 " to reclaim space, there are no other file pairs left.",
                 ctx->dbfiles_path, ctx->datafiles.first->tier, ctx->datafiles.first->fileno);
            return;
        }
        if(!datafile_acquire_for_deletion(ctx->datafiles.first, false)) {
            error("Cannot delete data file \"%s/"DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION"\""
                  " to reclaim space, it is in use currently, but it has been marked as not available for queries to stop using it.",
                  ctx->dbfiles_path, ctx->datafiles.first->tier, ctx->datafiles.first->fileno);
            return;
        }
        info("Deleting data file \"%s/"DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION"\".",
             ctx->dbfiles_path, ctx->datafiles.first->tier, ctx->datafiles.first->fileno);
        wc->now_deleting_files = mallocz(sizeof(*wc->now_deleting_files));
        wc->cleanup_thread_deleting_files = 0;

        error = uv_thread_create(wc->now_deleting_files, delete_old_data, ctx);
        if (error) {
            error("uv_thread_create(): %s", uv_strerror(error));
            freez(wc->now_deleting_files);
            wc->now_deleting_files = NULL;
        }
    }
}

static inline int rrdeng_threads_alive(struct rrdengine_worker_config* wc)
{
    if (wc->now_deleting_files) {
        return 1;
    }
    return 0;
}

static void rrdeng_cleanup_finished_threads(struct rrdengine_worker_config* wc)
{
    struct rrdengine_instance *ctx = wc->ctx;

    if (unlikely(wc->cleanup_thread_deleting_files)) {
        after_delete_old_data(wc);
    }

    if (unlikely(SET_QUIESCE == ctx->quiesce && !rrdeng_threads_alive(wc))) {
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

/* Flushes dirty pages when timer expires */
#define TIMER_PERIOD_MS (1000)

void timer_cb(uv_timer_t* handle)
{
    worker_is_busy(RRDENG_MAX_OPCODE + 1);

    struct rrdengine_worker_config* wc = handle->data;
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    rrdeng_test_quota(wc);

    debug(D_RRDENGINE, "%s: timeout reached.", __func__);

    if (true == wc->run_indexing) {
        wc->run_indexing = false;
        queue_journalfile_v2_migration(wc);
    }

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

static bool datafile_get_exclusive_access_to_extent(struct extent_page_list_s *extent_page_list) {
    struct rrdengine_datafile *df = extent_page_list->datafile;
    bool is_it_mine = false;

    while(!is_it_mine) {
        netdata_spinlock_lock(&df->extent_exclusive_access.spinlock);
        if(!df->users.available) {
            netdata_spinlock_unlock(&df->extent_exclusive_access.spinlock);
            return false;
        }
        Pvoid_t *PValue = JudyLIns(&df->extent_exclusive_access.extents_JudyL, extent_page_list->pos, PJE0);
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

static void datafile_release_exclusive_access_to_extent(struct extent_page_list_s *extent_page_list) {
    struct rrdengine_datafile *df = extent_page_list->datafile;

    netdata_spinlock_lock(&df->extent_exclusive_access.spinlock);

#ifdef NETDATA_INTERNAL_CHECKS
    Pvoid_t *PValue = JudyLGet(df->extent_exclusive_access.extents_JudyL, extent_page_list->pos, PJE0);
    if (*(Word_t *) PValue != (Word_t)gettid())
        fatal("DBENGINE: exclusive extent access is not mine");
#endif

    int rc = JudyLDel(&df->extent_exclusive_access.extents_JudyL, extent_page_list->pos, PJE0);
    if (!rc)
        fatal("DBENGINE: cannot find my exclusive access");

    df->extent_exclusive_access.lockers--;
    netdata_spinlock_unlock(&df->extent_exclusive_access.spinlock);
}

static bool pdc_check_if_pages_are_already_in_cache(struct rrdengine_instance *ctx, struct extent_page_list_s *extent_page_list, PDC_PAGE_STATUS reason) {
    Word_t Index = 0;
    Pvoid_t *PValue;
    bool first = true;
    size_t count_remaining = 0;
    size_t found = 0;
    while((PValue = JudyLFirstThenNext(extent_page_list->JudyL_page_list, &Index, &first))) {
        struct page_details *pd = *PValue;
        if(pd->page)
            continue;

        pd->page = pgc_page_get_and_acquire(main_cache, (Word_t)ctx, pd->metric_id, pd->first_time_s, true);
        if(pd->page) {
            found++;
            pdc_page_status_set(pd, PDC_PAGE_READY | reason);
        }
        else
            count_remaining++;
    }

    if(found)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_preloaded, found, __ATOMIC_RELAXED);

    return count_remaining == 0;
}

static void do_read_extent_work(uv_work_t *req)
{
    static __thread int worker = -1;
    if (unlikely(worker == -1))
        register_libuv_worker_jobs();

    struct rrdeng_work *work_request = req->data;
    struct rrdengine_worker_config *wc = work_request->wc;

    struct extent_page_list_s *extent_page_list = work_request->data;
    struct page_details_control *pdc = extent_page_list->pdc;

    bool datafile_acquired = false, extent_exclusive = false;

    if(!datafile_acquire(extent_page_list->datafile)) {
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_fail_datafile_not_available, 1, __ATOMIC_RELAXED);
        goto cleanup;
    }
    datafile_acquired = true;

    if(pdc->preload_all_extent_pages) {
        if (!datafile_get_exclusive_access_to_extent(extent_page_list)) {
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_fail_datafile_not_available, 1, __ATOMIC_RELAXED);
            goto cleanup;
        }
        extent_exclusive = true;

        if (pdc_check_if_pages_are_already_in_cache(wc->ctx, extent_page_list, PDC_PAGE_FOUND_IN_CACHE_BY_WORKER))
            goto cleanup;
    }

    worker_is_busy(UV_EVENT_EXT_DECOMPRESSION);

    off_t map_start =  ALIGN_BYTES_FLOOR(extent_page_list->pos);
    size_t length = ALIGN_BYTES_CEILING(extent_page_list->pos + extent_page_list->size) - map_start;

    void *data = mmap(NULL, length, PROT_READ, MAP_SHARED, extent_page_list->file, map_start);
    if(data != MAP_FAILED) {
        // Need to decompress and then process the pagelist
        void *extent_data = data + (extent_page_list->pos - map_start);
        extent_uncompress_and_populate_pages(wc, extent_data, extent_page_list->size, extent_page_list, pdc->preload_all_extent_pages);

        int ret = munmap(data, length);
        fatal_assert(0 == ret);
    }
    else
        // we failed to map the data
        pdc_mark_all_unset_pages_as_failed(
                extent_page_list->JudyL_page_list, PDC_PAGE_FAILED_TO_MAP_EXTENT,
                &rrdeng_cache_efficiency_stats.pages_load_fail_cant_mmap_extent);

cleanup:
    if(extent_exclusive)
        datafile_release_exclusive_access_to_extent(extent_page_list);

    if(datafile_acquired)
        datafile_release(extent_page_list->datafile);

    completion_mark_complete_a_job(&extent_page_list->pdc->completion);
    pdc_release_and_destroy_if_unreferenced(pdc, true, false);

    // Free the Judy that holds the requested pagelist and the extents
    JudyLFreeArray(&extent_page_list->JudyL_page_list, PJE0);
    freez(extent_page_list);

    worker_is_idle();
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
    Pvoid_t *PValue;
    Pvoid_t *PValue1;
    Pvoid_t *PValue2;
    Pvoid_t *PValue3;
    Word_t time_index = 0;
    struct page_details *pd = NULL;

    // this is the entire page list
    // Lets do some deduplication
    // 1. Per datafile
    // 2. Per extent
    // 3. Pages per extent will be added to the cache either as acquired or not

    Pvoid_t JudyL_datafile_list = NULL;

    struct datafile_extent_list_s *datafile_extent_list;
    struct extent_page_list_s *extent_page_list;

    if (pdc->page_list_JudyL) {
        bool first_then_next = true;
        while((PValue = JudyLFirstThenNext(pdc->page_list_JudyL, &time_index, &first_then_next))) {
            pd = *PValue;

            if (!pd || pd->page)
                continue;

            PValue1 = JudyLIns(&JudyL_datafile_list, pd->datafile.fileno, PJE0);
            if (PValue1 && !*PValue1) {
                *PValue1 = datafile_extent_list = mallocz(sizeof(*datafile_extent_list));
                datafile_extent_list->JudyL_datafile_extent_list = NULL;
                datafile_extent_list->fileno = pd->datafile.fileno;
            }
            else
                datafile_extent_list = *PValue1;

            PValue2 = JudyLIns(&datafile_extent_list->JudyL_datafile_extent_list, pd->datafile.extent.pos, PJE0);
            if (PValue2 && !*PValue2) {
                *PValue2 = extent_page_list = mallocz( sizeof(*extent_page_list));
                extent_page_list->JudyL_page_list = NULL;
                extent_page_list->count = 0;
                extent_page_list->file = pd->datafile.file;
                extent_page_list->pos = pd->datafile.extent.pos;
                extent_page_list->size = pd->datafile.extent.bytes;
                extent_page_list->datafile = pd->datafile.ptr;
            }
            else
                extent_page_list = *PValue2;

            extent_page_list->count++;

            PValue3 = JudyLIns(&extent_page_list->JudyL_page_list, pd->first_time_s, PJE0);
            *PValue3 = pd;
        }

        Word_t datafile_no = 0;
        first_then_next = true;
        while((PValue = JudyLFirstThenNext(JudyL_datafile_list, &datafile_no, &first_then_next))) {
            datafile_extent_list = *PValue;

            bool first_then_next_extent = true;
            Word_t pos;
            while ((PValue = JudyLFirstThenNext(datafile_extent_list->JudyL_datafile_extent_list, &pos, &first_then_next_extent))) {
                extent_page_list = *PValue;
                internal_fatal(!extent_page_list, "DBENGINE: extent_list is not populated properly");

                // The extent page list can be dispatched to a worker
                // It will need to populate the cache with "acquired" pages that are in the list (pd) only
                // the rest of the extent pages will be added to the cache butnot acquired

                struct rrdeng_cmd cmd;
                cmd.opcode = RRDENG_READ_EXTENT;
                pdc_acquire(pdc); // we do this for the next worker: do_read_extent_work()
                extent_page_list->pdc = pdc;
                cmd.data = extent_page_list;
                rrdeng_enq_cmd(&ctx->worker_config, &cmd);
            }
            freez(datafile_extent_list);
        }
        JudyLFreeArray(&JudyL_datafile_list, PJE0);
    }

    pdc_release_and_destroy_if_unreferenced(pdc, true, true);
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
    struct completion read_page_received;

    completion_init(&read_page_received);
    struct rrdeng_cmd cmd;
    cmd.opcode = RRDENG_READ_PAGE_LIST;
    cmd.data = pdc;
    cmd.completion = &read_page_received;
    rrdeng_enq_cmd(&ctx->worker_config, &cmd);

    completion_wait_for(&read_page_received);
    completion_destroy(&read_page_received);
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

    wc->now_deleting_files = NULL;
    wc->cleanup_thread_deleting_files = 0;
    wc->running_journal_migration = 0;
    wc->run_indexing = false;

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
                fatal_assert(0 == uv_timer_stop(&timer_req));
                uv_close((uv_handle_t *)&timer_req, NULL);
                info("Shutdown command received. Flushing all pages to disk");
                // FIXME: Check if we need to ask page cache if flushing is done
                wal_flush_transaction_buffer(wc);
                if (!rrdeng_threads_alive(wc)) {
                    ctx->quiesce = QUIESCED;
                    completion_mark_complete(&ctx->rrdengine_completion);
                }
                break;
            case RRDENG_COMMIT_PAGE:
                do_commit_transaction(wc, STORE_DATA, NULL);
                break;
            case RRDENG_FLUSH_PAGES: {
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
