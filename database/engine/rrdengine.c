// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

rrdeng_stats_t global_io_errors = 0;
rrdeng_stats_t global_fs_errors = 0;
rrdeng_stats_t rrdeng_reserved_file_descriptors = 0;
rrdeng_stats_t global_pg_cache_over_half_dirty_events = 0;
rrdeng_stats_t global_flushing_pressure_page_deletions = 0;

unsigned rrdeng_pages_per_extent = MAX_PAGES_PER_EXTENT;

// DBENGINE 2
struct datafile_extent_list_s {
    uv_file file;
    unsigned count;
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

static void pdc_acquire(PDC *pdc) {
    int32_t expected, desired;

    expected = __atomic_load_n(&pdc->refcount, __ATOMIC_RELAXED);

    do {

        if(expected < 1)
            fatal("PDC is not referenced and cannot be acquired");

        desired = expected + 1;

    } while(!__atomic_compare_exchange_n(&pdc->refcount, &expected, desired,
                                         false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));
}

bool pdc_release_and_destroy_if_unreferenced(PDC *pdc, bool worker) {
    int32_t expected, desired;

    expected = __atomic_load_n(&pdc->refcount, __ATOMIC_RELAXED);

    do {

        if(expected <= 0)
            fatal("PDC is not referenced and cannot be released");

        desired = expected - 1;

    } while(!__atomic_compare_exchange_n(&pdc->refcount, &expected, desired,
                                         false, __ATOMIC_RELEASE, __ATOMIC_RELAXED));

    if (desired <= 1 && worker)
        // when 1 refcount is remaining, and we are a worker,
        // we can mark the job completed:
        // - if the remaining refcount is from the query caller, we will wake it up
        // - if the remaining refcount is from a worker, the caller is already away
        completion_mark_complete(&pdc->completion);

    if (desired == 0) {
        completion_destroy(&pdc->completion);
        free_judyl_page_list(pdc->page_list_JudyL);
        freez(pdc);
        return true;
    }

    return false;
}

VALIDATED_PAGE_DESCRIPTOR validate_extent_page_descr(const struct rrdeng_extent_page_descr *descr, time_t now_s, time_t overwrite_zero_update_every_s, bool have_read_error) {
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
static void extent_uncompress_and_populate_pages(struct rrdengine_worker_config *wc, void *data, struct extent_page_list_s *extent_page_list)
{
    struct rrdengine_instance *ctx = wc->ctx;
    int ret;
    unsigned i, count;
    void *uncompressed_buf = NULL;
    uint32_t payload_length, payload_offset, page_offset, uncompressed_payload_length = 0;
    uint8_t have_read_error = 0;
    /* persistent structures */
    struct rrdeng_df_extent_header *header;
    struct rrdeng_df_extent_trailer *trailer;
    uLong crc;

    header = data;
    payload_length = header->payload_length;
    count = header->number_of_pages;
    payload_offset = sizeof(*header) + sizeof(header->descr[0]) * count;
    trailer = data + extent_page_list->size - sizeof(*trailer);

    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, data, extent_page_list->size  - sizeof(*trailer));
    ret = crc32cmp(trailer->checksum, crc);
    if (unlikely(ret)) {
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        have_read_error = 1;

        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "%s: Extent at offset %"PRIu64"(%u) was read from datafile %u. CRC32 check: FAILED", __func__,
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

    for (i = 0, page_offset = 0; i < count; page_offset += header->descr[i].page_length, i++) {

        time_t start_time_s = (time_t) (header->descr[i].start_time_ut / USEC_PER_SEC);

        Pvoid_t *PValue = JudyLGet(extent_page_list->JudyL_page_list, start_time_s, PJE0);
        struct page_details *pd = (NULL == PValue || NULL == *PValue) ? NULL : *PValue;

        // We might have an Index match but check for UUID match as well
        // if there is no match we will add the page in cache but not in
        // the preload response

        if (pd && uuid_compare(pd->uuid, header->descr[i].uuid))
            pd = NULL;

        VALIDATED_PAGE_DESCRIPTOR vd = validate_extent_page_descr(&header->descr[i], now_s, (pd) ? pd->update_every_s : 0, have_read_error);

        // Find metric id
        METRIC *this_metric = mrg_metric_get_and_acquire(main_mrg, &header->descr[i].uuid, (Word_t) ctx);
        Word_t metric_id = mrg_metric_id(main_mrg, this_metric);
        mrg_metric_release(main_mrg, this_metric);

        PGC_PAGE *page = pgc_page_get_and_acquire(main_cache, (Word_t)ctx, (Word_t)metric_id, start_time_s, true);
        if (!page) {
            void *page_data = mallocz((size_t) vd.page_length);

            if (!vd.data_on_disk_valid)
                fill_page_with_nulls(page_data, vd.page_length, vd.type);

            else if (RRD_NO_COMPRESSION == header->compression_algorithm)
                memcpy(page_data, data + payload_offset + page_offset, (size_t) vd.page_length);

            else
                memcpy(page_data, uncompressed_buf + page_offset, (size_t) vd.page_length);

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
            if (false == added)
                freez(page_data);
        }

        if (pd) {
            pd->page = page;
            pd->page_length = pgc_page_data_size(main_cache, page);
            __atomic_store_n(&pd->page_is_loaded, true, __ATOMIC_RELEASE);
        }
        else
            pgc_page_release(main_cache, page);
    }

    if (!have_read_error && RRD_NO_COMPRESSION != header->compression_algorithm)
        freez(uncompressed_buf);
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

struct pg_cache_page_index *get_page_index(struct page_cache *pg_cache, uuid_t *uuid)
{
    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    Pvoid_t *PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, uuid, sizeof(uuid_t));
    struct pg_cache_page_index *page_index = (NULL == PValue) ? NULL : *PValue;
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
    return page_index;
}

struct rrdeng_page_descr *get_descriptor(struct pg_cache_page_index *page_index, time_t start_time_s)
{
    if (unlikely(!page_index))
        return NULL;

    uv_rwlock_rdlock(&page_index->lock);
    Pvoid_t *PValue = JudyLGet(page_index->JudyL_array, start_time_s, PJE0);
    struct rrdeng_page_descr *descr = unlikely(NULL == PValue) ? NULL : *PValue;
    uv_rwlock_rdunlock(&page_index->lock);
    return descr;
};

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
        (void) pgc_page_add_and_acquire(open_cache, page_entry, &added);

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
    struct extent_info *extent;
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

    extent = mallocz(sizeof(*extent) + count * sizeof(extent->pages[0]));
    datafile = ctx->datafiles.last;  /* TODO: check for exceeded size quota */
    extent->offset = datafile->pos;
    extent->number_of_pages = count;
    extent->datafile = datafile;
    extent->next = NULL;
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
        /* care, we don't hold the descriptor mutex */
        (void) memcpy(xt_io_descr->buf + pos, descr->page, descr->page_length);
        descr->extent = extent;
        extent->pages[i] = descr;

        pos += descr->page_length;
    }
    df_extent_insert(extent);

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
    extent->size = size_bytes;
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
    struct rrdengine_worker_config* wc = &ctx->worker_config;
    struct rrdengine_datafile *datafile;
    struct extent_info *extent, *next;
    struct rrdeng_page_descr *descr;
    unsigned count, i;
//    uint8_t can_delete_metric;
//    uuid_t metric_id;

    /* Safe to use since it will be deleted after we are done */

    datafile = ctx->datafiles.first;
    // If this is migrated then do special cleanup
    if (datafile->journalfile->journal_data && datafile->journalfile->is_valid) {

        // loop and find decriptors from file
        void *data_start = datafile->journalfile->journal_data;
        struct journal_v2_header *j2_header = (void *) data_start;

        struct journal_metric_list *metric = (void *) (data_start + j2_header->metric_offset);

        time_t journal_start_time_s = (time_t) (j2_header->start_time_ut / USEC_PER_SEC);

        info("Removing %u metrics that exist in the journalfile", j2_header->metric_count);
        unsigned entries;
        unsigned evicted = 0;
        unsigned deleted = 0;
        unsigned delete_check = 0;

        uv_rwlock_wrlock(&ctx->datafiles.rwlock);
        datafile->journalfile->is_valid = false;
        uv_rwlock_wrunlock(&ctx->datafiles.rwlock);

        for (entries = 0; entries < j2_header->metric_count; entries++) {
            struct journal_page_header *metric_list_header = (void *) (data_start + metric->page_offset);
            struct journal_page_list *descr_page = (struct journal_page_list *) ((uint8_t *) metric_list_header + sizeof(struct journal_page_header));
            for (uint32_t index = 0; index < metric->entries; index++) {
                struct pg_cache_page_index *page_index = get_page_index(&ctx->pg_cache, &metric->uuid);

                if (unlikely(!page_index))
                    continue;

                time_t start_time_s = journal_start_time_s + descr_page->delta_start_s;

                descr = get_descriptor(page_index, start_time_s);
                if (unlikely(descr)) {
                    // FIXME: DBENGINE2
//                    can_delete_metric = pg_cache_punch_hole(ctx, descr, 0, 0, &metric_id);
//                    if (unlikely(can_delete_metric)) {
//                        metaqueue_delete_dimension_uuid(&metric_id);
//                        ++delete_check;
//                    }
                    ++evicted;
                }
                ++deleted;
                descr_page++;
            }
            metric++;
        }
        info("Removed %u pages from %u metrics (evicted %u), %u queued for final deletion check", deleted,  entries, evicted, delete_check);
        wc->cleanup_thread_deleting_files = 1;
        /* wake up event loop */
        fatal_assert(0 == uv_async_send(&wc->async));
        return;
    }

    uv_rwlock_wrlock(&datafile->extent_rwlock);
    for (extent = datafile->extents.first ; extent != NULL ; extent = next) {
        count = extent->number_of_pages;
        for (i = 0 ; i < count ; ++i) {
            descr = extent->pages[i];
            if (unlikely(!descr))
                continue;

            // FIXME: DBENGINE2
//            can_delete_metric = pg_cache_punch_hole(ctx, descr, 0, 0, &metric_id);
//            if (unlikely(can_delete_metric)) {
//                /*
//                 * If the metric is empty, has no active writers and if the metadata log has been initialized then
//                 * attempt to delete the corresponding netdata dimension.
//                 */
//                metaqueue_delete_dimension_uuid(&metric_id);
//            }
        }
        next = extent->next;
        freez(extent);
    }
    uv_rwlock_wrunlock(&datafile->extent_rwlock);

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
    datafile = ctx->datafiles.last;
    current_size = datafile->pos;
    target_size = ctx->max_disk_space / TARGET_DATAFILES;
    target_size = MIN(target_size, MAX_DATAFILE_SIZE);
    target_size = MAX(target_size, MIN_DATAFILE_SIZE);
    only_one_datafile = (datafile == ctx->datafiles.first) ? 1 : 0;

    if (unlikely(current_size >= target_size || (out_of_space && only_one_datafile))) {
        /* Finalize data and journal file and create a new pair */
        struct rrdengine_journalfile *journalfile = unlikely(NULL == ctx->datafiles.last) ? NULL : ctx->datafiles.last->journalfile;
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

// FIXME: Expire cached extents per datafile -- Improve so that it doesnt need to scan all the items
static void expire_extent_caching(struct rrdengine_worker_config *wc)
{
    struct rrdengine_datafile *datafile = wc->ctx->datafiles.first;
    struct rrdengine_instance *ctx = wc->ctx;

    time_t now = now_realtime_sec();

    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    while (datafile) {
        uv_rwlock_wrlock(&datafile->JudyL_extent_rwlock);
        Word_t pos = 0;
        bool first = true;
        Pvoid_t *PValue;
        do {
            PValue = JudyLFirstThenNext(datafile->JudyL_extent_expire_array, &pos, &first);
            if (PValue && *PValue) {
                time_t expire_time_t = (time_t) * (Word_t *)PValue;

                if (expire_time_t <= now) {
                    // Find the address of memory to free
                    PValue = JudyLGet(datafile->JudyL_extent_offset_array, pos, PJE0);
                    fatal_assert(NULL != PValue && NULL != *PValue);
                    freez(*PValue);

                    PValue = JudyLGet(datafile->JudyL_extent_size_array, pos, PJE0);
                    fatal_assert(NULL != PValue && NULL != *PValue);
                    //size_t bytes = (size_t) *(Word_t *) PValue;
                    //info("DEBUG: Extent caching -- Releasing extent @ pos %lu (%lu bytes)", pos, bytes);
                    JudyLDel(&datafile->JudyL_extent_offset_array, pos, PJE0);
                    JudyLDel(&datafile->JudyL_extent_size_array, pos, PJE0);
                    JudyLDel(&datafile->JudyL_extent_expire_array, pos, PJE0);
                }
            }
        } while (PValue);
        uv_rwlock_wrunlock(&datafile->JudyL_extent_rwlock);
        datafile = datafile->next;
    }
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);
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

    if (NO_QUIESCE == wc->ctx->quiesce && !wc->now_deleting_files)
        expire_extent_caching(wc);

#ifdef NETDATA_INTERNAL_CHECKS
    {
        char buf[4096];
        debug(D_RRDENGINE, "%s", get_rrdeng_statistics(wc->ctx, buf, sizeof(buf)));
    }
#endif
    worker_is_idle();
}

// ******************
// DBENGINE2 -- start
// ******************

void after_do_read_datafile_extent_list_work(uv_work_t *req, int status __maybe_unused)
{
    struct rrdeng_work  *work_request = req->data;
    freez(work_request);
}

static void do_read_datafile_extent_list_work(uv_work_t *req)
{
    struct rrdeng_work *work_request = req->data;
    struct rrdengine_worker_config *wc = work_request->wc;
    struct rrdengine_instance *ctx = wc->ctx;

    struct datafile_extent_list_s *datafile_extent_list = work_request->data;
    struct extent_page_list_s *extent_page_list;
    struct page_details_control *pdc = datafile_extent_list->pdc;

    Pvoid_t *PValue;
    Word_t pos = 0;
    // Check and send datafile_extent_list->count  extent requests per datafile
    for (PValue = JudyLFirst(datafile_extent_list->JudyL_datafile_extent_list, &pos, PJE0),
        extent_page_list = unlikely(NULL == PValue) ? NULL : *PValue;
         extent_page_list != NULL;
         PValue = JudyLNext(datafile_extent_list->JudyL_datafile_extent_list, &pos, PJE0),
        extent_page_list = unlikely(NULL == PValue) ? NULL : *PValue) {

        // The extent page list can be dispatched to a worker
        // The worker will add the extent pages to the cache
        // The worker will try to check if the extent is cached and if so, reuse the data
        // The check for precache should be per datafile
        // It will need to populate the cache with "acquired" pages that are in the list (pd) only
        // the rest of the extent pages will be added to the cache butnot acquired

        struct rrdeng_cmd cmd;
        cmd.opcode = RRDENG_READ_EXTENT;
        pdc_acquire(pdc); // we do this for the next worker: do_read_extent2_work()
        extent_page_list->pdc = pdc;
        cmd.data = extent_page_list;
        rrdeng_enq_cmd(&ctx->worker_config, &cmd);
    }
    JudyLFreeArray(&datafile_extent_list->JudyL_datafile_extent_list, PJE0);

    pdc_release_and_destroy_if_unreferenced(pdc, true);
}

// New version of READ EXTENT
static void do_read_datafile_extent_list(struct rrdengine_worker_config *wc, void *data)
{
    struct rrdeng_work *work_request;

    work_request = mallocz(sizeof(*work_request));
    work_request->req.data = work_request;
    work_request->wc = wc;
    work_request->data = data;
    work_request->completion = NULL;

    int ret = uv_queue_work(wc->loop, &work_request->req, do_read_datafile_extent_list_work, after_do_read_datafile_extent_list_work);
    if (ret)
        freez(work_request);

    fatal_assert(0 == ret);
}


void after_do_read_extent2_work(uv_work_t *req, int status)
{
    struct rrdeng_work  *work_request = req->data;
    //    struct rrdengine_worker_config *wc = work_request->wc;

    if (likely(status != UV_ECANCELED)) {
        ;
    }
    freez(work_request);
}

static void do_read_extent2_work(uv_work_t *req)
{
    struct rrdeng_work *work_request = req->data;
    struct rrdengine_worker_config *wc = work_request->wc;

    struct extent_page_list_s *extent_page_list = work_request->data;
    struct page_details_control *pdc = extent_page_list->pdc;

    // We have one extent to read from file, pos, size
    // Then we need to scan the judy extent_page_list->JudyL_page_list to see exactly the pages we need (extent processing)
    // We first need to check if under the datafile we have the extent "cached"
    // 1. extent_page_list->datafile->JudyL_extent_offset_array
    //    Index the file offset and value the actual extent data (compressed)
    //    Note
    // 2. extent_page_list->datafile->JudyL_extent_rwlock (lock)

    off_t map_start =  ALIGN_BYTES_FLOOR(extent_page_list->pos);
    size_t length = ALIGN_BYTES_CEILING(extent_page_list->pos + extent_page_list->size) - map_start;

    void *extent_data;

    // FIXME: DBENGINE2 add an extent cache cleanup
    uv_rwlock_wrlock(&extent_page_list->datafile->JudyL_extent_rwlock);
    Pvoid_t *PValue = JudyLIns(&extent_page_list->datafile->JudyL_extent_offset_array, extent_page_list->pos, PJE0);
    fatal_assert(NULL != PValue);
    if (NULL == *PValue) {
        void *data = mmap(NULL, length, PROT_READ, MAP_SHARED, extent_page_list->file, map_start);
        fatal_assert(MAP_FAILED != data);

        extent_data = mallocz(extent_page_list->size);
        memcpy(extent_data, data + (extent_page_list->pos - map_start), extent_page_list->size);
        *PValue = extent_data;

        int ret = munmap(data, length);
        fatal_assert(0 == ret);

        // Store the extent length here
        PValue = JudyLIns(&extent_page_list->datafile->JudyL_extent_size_array, extent_page_list->pos, PJE0);
        *(Word_t *) PValue = (Word_t) extent_page_list->size;
    } else  // extent is cached, reuse it
        extent_data = *PValue;

    // Store an "expire after" timestamp sometime in the future
    // When checking for extents to expire, get the lock and check if the entry needs to be deleted
    PValue = JudyLIns(&extent_page_list->datafile->JudyL_extent_expire_array, extent_page_list->pos, PJE0);
    *(Word_t *) PValue = now_realtime_sec() + 60;

    uv_rwlock_wrunlock(&extent_page_list->datafile->JudyL_extent_rwlock);

    // Extent is now cached and *data contains the compressed extent
    // Need to decompress and then process the pagelist
    extent_uncompress_and_populate_pages(wc, extent_data, extent_page_list);

    completion_mark_complete_a_job(&extent_page_list->pdc->completion);
    pdc_release_and_destroy_if_unreferenced(pdc, true);

    // Free the Judy that holds the requested pagelist and the extents
    JudyLFreeArray(&extent_page_list->JudyL_page_list, PJE0);
    freez(extent_page_list);
}

// New version of READ EXTENT
static void do_read_extent2(struct rrdengine_worker_config *wc, void *data)
{
    struct rrdeng_work *work_request;

    work_request = mallocz(sizeof(*work_request));
    work_request->req.data = work_request;
    work_request->wc = wc;
    work_request->data = data;
    work_request->completion = NULL;

    int ret = uv_queue_work(wc->loop, &work_request->req, do_read_extent2_work, after_do_read_extent2_work);
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
    struct rrdeng_work *work_request = req->data;
    struct rrdengine_worker_config *wc = work_request->wc;
    struct rrdengine_instance *ctx = wc->ctx;

    struct page_details_control *pdc = work_request->data;
    Pvoid_t JudyL_page_list = pdc->page_list_JudyL;
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

    if (JudyL_page_list) {
        for (PValue = JudyLFirst(JudyL_page_list, &time_index, PJE0),
            pd = unlikely(NULL == PValue) ? NULL : *PValue;
             pd != NULL;
             PValue = JudyLNext(JudyL_page_list, &time_index, PJE0),
            pd = unlikely(NULL == PValue) ? NULL : *PValue) {

            if (pd->page)
                continue;

            PValue1 = JudyLIns(&JudyL_datafile_list, pd->datafile.fileno, PJE0);

            if (PValue1 && !*PValue1) {
                *PValue1 = datafile_extent_list = mallocz(sizeof(*datafile_extent_list));
                datafile_extent_list->JudyL_datafile_extent_list = NULL;
                datafile_extent_list->count = 0;
                datafile_extent_list->fileno = pd->datafile.fileno;
            }
            else
                datafile_extent_list = *PValue1;
            datafile_extent_list->count++;

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
        for (PValue = JudyLFirst(JudyL_datafile_list, &datafile_no, PJE0),
            datafile_extent_list = unlikely(NULL == PValue) ? NULL : *PValue;
             datafile_extent_list != NULL;
             PValue = JudyLNext(JudyL_datafile_list, &datafile_no, PJE0),
            datafile_extent_list = unlikely(NULL == PValue) ? NULL : *PValue) {

            // List of datafiles
            // Now submit each datafile_extent_list back to the engine
            pdc_acquire(pdc); // we get this for the next worker: do_read_datafile_extent_list_work()
            struct rrdeng_cmd cmd;
            datafile_extent_list->pdc = pdc;
            cmd.opcode = RRDENG_READ_DF_EXTENT_LIST;
            cmd.data = datafile_extent_list;
            rrdeng_enq_cmd(&ctx->worker_config, &cmd);
        }
        JudyLFreeArray(&JudyL_datafile_list, PJE0);
    }

    pdc_release_and_destroy_if_unreferenced(pdc, true);
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

// DBENGINE2 -- end


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
//                usec_t start_flush = now_realtime_usec();

//                unsigned long total_bytes, bytes_written; 9
//                total_bytes = 0;
//                while ((bytes_written = do_flush_pages(wc, 1, NULL))) {
//                    total_bytes += bytes_written;
//                    /* Force flushing of all committed pages. */
//                }
//                info("Pages flushed to disk in %llu usecs (%lu bytes written)", now_realtime_usec() - start_flush, total_bytes);
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
            case RRDENG_READ_DF_EXTENT_LIST:
                do_read_datafile_extent_list(wc, cmd.data);
                break;
            case RRDENG_READ_EXTENT:
                if (unlikely(!set_name)) {
                    set_name = 1;
                    uv_thread_set_name_np(ctx->worker_config.thread, "DBENGINE");
                }
                do_read_extent2(wc, cmd.data);
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

//    while (do_flush_pages(wc, 1, NULL)) {
//        ; /* Force flushing of all committed pages. */
//    }
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
