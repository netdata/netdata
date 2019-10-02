// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

rrdeng_stats_t global_io_errors = 0;
rrdeng_stats_t global_fs_errors = 0;
rrdeng_stats_t global_pg_cache_warnings = 0;
rrdeng_stats_t global_pg_cache_errors = 0;
rrdeng_stats_t rrdeng_reserved_file_descriptors = 0;

void sanity_check(void)
{
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

    /* page info scratch space must be able to hold 2 32-bit integers */
    BUILD_BUG_ON(sizeof(((struct rrdeng_page_info *)0)->scratch) < 2 * sizeof(uint32_t));
}

void read_extent_cb(uv_fs_t* req)
{
    struct rrdengine_worker_config* wc = req->loop->data;
    struct rrdengine_instance *ctx = wc->ctx;
    struct extent_io_descriptor *xt_io_descr;
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr;
    int ret;
    unsigned i, j, count;
    void *page, *uncompressed_buf = NULL;
    uint32_t payload_length, payload_offset, page_offset, uncompressed_payload_length;
    uint8_t have_read_error = 0;
    /* persistent structures */
    struct rrdeng_df_extent_header *header;
    struct rrdeng_df_extent_trailer *trailer;
    uLong crc;

    xt_io_descr = req->data;
    header = xt_io_descr->buf;
    payload_length = header->payload_length;
    count = header->number_of_pages;
    payload_offset = sizeof(*header) + sizeof(header->descr[0]) * count;
    trailer = xt_io_descr->buf + xt_io_descr->bytes - sizeof(*trailer);

    if (req->result < 0) {
        struct rrdengine_datafile *datafile = xt_io_descr->descr_array[0]->extent->datafile;

        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        have_read_error = 1;
        error("%s: uv_fs_read - %s - extent at offset %"PRIu64"(%u) in datafile %u-%u.", __func__,
              uv_strerror((int)req->result), xt_io_descr->pos, xt_io_descr->bytes, datafile->tier, datafile->fileno);
        goto after_crc_check;
    }
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, xt_io_descr->buf, xt_io_descr->bytes - sizeof(*trailer));
    ret = crc32cmp(trailer->checksum, crc);
#ifdef NETDATA_INTERNAL_CHECKS
    {
        struct rrdengine_datafile *datafile = xt_io_descr->descr_array[0]->extent->datafile;
        debug(D_RRDENGINE, "%s: Extent at offset %"PRIu64"(%u) was read from datafile %u-%u. CRC32 check: %s", __func__,
              xt_io_descr->pos, xt_io_descr->bytes, datafile->tier, datafile->fileno, ret ? "FAILED" : "SUCCEEDED");
    }
#endif
    if (unlikely(ret)) {
        struct rrdengine_datafile *datafile = xt_io_descr->descr_array[0]->extent->datafile;

        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        have_read_error = 1;
        error("%s: Extent at offset %"PRIu64"(%u) was read from datafile %u-%u. CRC32 check: FAILED", __func__,
              xt_io_descr->pos, xt_io_descr->bytes, datafile->tier, datafile->fileno);
    }

after_crc_check:
    if (!have_read_error && RRD_NO_COMPRESSION != header->compression_algorithm) {
        uncompressed_payload_length = 0;
        for (i = 0 ; i < count ; ++i) {
            uncompressed_payload_length += header->descr[i].page_length;
        }
        uncompressed_buf = mallocz(uncompressed_payload_length);
        ret = LZ4_decompress_safe(xt_io_descr->buf + payload_offset, uncompressed_buf,
                                  payload_length, uncompressed_payload_length);
        ctx->stats.before_decompress_bytes += payload_length;
        ctx->stats.after_decompress_bytes += ret;
        debug(D_RRDENGINE, "LZ4 decompressed %u bytes to %d bytes.", payload_length, ret);
        /* care, we don't hold the descriptor mutex */
    }

    for (i = 0 ; i < xt_io_descr->descr_count; ++i) {
        page = mallocz(RRDENG_BLOCK_SIZE);
        descr = xt_io_descr->descr_array[i];
        for (j = 0, page_offset = 0; j < count; ++j) {
            /* care, we don't hold the descriptor mutex */
            if (!uuid_compare(*(uuid_t *) header->descr[j].uuid, *descr->id) &&
                header->descr[j].page_length == descr->page_length &&
                header->descr[j].start_time == descr->start_time &&
                header->descr[j].end_time == descr->end_time) {
                break;
            }
            page_offset += header->descr[j].page_length;
        }
        /* care, we don't hold the descriptor mutex */
        if (have_read_error) {
            /* Applications should make sure NULL values match 0 as does SN_EMPTY_SLOT */
            memset(page, 0, descr->page_length);
        } else if (RRD_NO_COMPRESSION == header->compression_algorithm) {
            (void) memcpy(page, xt_io_descr->buf + payload_offset + page_offset, descr->page_length);
        } else {
            (void) memcpy(page, uncompressed_buf + page_offset, descr->page_length);
        }
        pg_cache_replaceQ_insert(ctx, descr);
        rrdeng_page_descr_mutex_lock(ctx, descr);
        pg_cache_descr = descr->pg_cache_descr;
        pg_cache_descr->page = page;
        pg_cache_descr->flags |= RRD_PAGE_POPULATED;
        pg_cache_descr->flags &= ~RRD_PAGE_READ_PENDING;
        debug(D_RRDENGINE, "%s: Waking up waiters.", __func__);
        if (xt_io_descr->release_descr) {
            pg_cache_put_unsafe(descr);
        } else {
            pg_cache_wake_up_waiters_unsafe(descr);
        }
        rrdeng_page_descr_mutex_unlock(ctx, descr);
    }
    if (!have_read_error && RRD_NO_COMPRESSION != header->compression_algorithm) {
        freez(uncompressed_buf);
    }
    if (xt_io_descr->completion)
        complete(xt_io_descr->completion);
    uv_fs_req_cleanup(req);
    free(xt_io_descr->buf);
    freez(xt_io_descr);
}


static void do_read_extent(struct rrdengine_worker_config* wc,
                           struct rrdeng_page_descr **descr,
                           unsigned count,
                           uint8_t release_descr)
{
    struct rrdengine_instance *ctx = wc->ctx;
    struct page_cache_descr *pg_cache_descr;
    int ret;
    unsigned i, size_bytes, pos, real_io_size;
//    uint32_t payload_length;
    struct extent_io_descriptor *xt_io_descr;
    struct rrdengine_datafile *datafile;

    datafile = descr[0]->extent->datafile;
    pos = descr[0]->extent->offset;
    size_bytes = descr[0]->extent->size;

    xt_io_descr = mallocz(sizeof(*xt_io_descr));
    ret = posix_memalign((void *)&xt_io_descr->buf, RRDFILE_ALIGNMENT, ALIGN_BYTES_CEILING(size_bytes));
    if (unlikely(ret)) {
        fatal("posix_memalign:%s", strerror(ret));
        /* freez(xt_io_descr);
        return;*/
    }
    for (i = 0 ; i < count; ++i) {
        rrdeng_page_descr_mutex_lock(ctx, descr[i]);
        pg_cache_descr = descr[i]->pg_cache_descr;
        pg_cache_descr->flags |= RRD_PAGE_READ_PENDING;
//        payload_length = descr[i]->page_length;
        rrdeng_page_descr_mutex_unlock(ctx, descr[i]);

        xt_io_descr->descr_array[i] = descr[i];
    }
    xt_io_descr->descr_count = count;
    xt_io_descr->bytes = size_bytes;
    xt_io_descr->pos = pos;
    xt_io_descr->req.data = xt_io_descr;
    xt_io_descr->completion = NULL;
    /* xt_io_descr->descr_commit_idx_array[0] */
    xt_io_descr->release_descr = release_descr;

    real_io_size = ALIGN_BYTES_CEILING(size_bytes);
    xt_io_descr->iov = uv_buf_init((void *)xt_io_descr->buf, real_io_size);
    ret = uv_fs_read(wc->loop, &xt_io_descr->req, datafile->file, &xt_io_descr->iov, 1, pos, read_extent_cb);
    assert (-1 != ret);
    ctx->stats.io_read_bytes += real_io_size;
    ++ctx->stats.io_read_requests;
    ctx->stats.io_read_extent_bytes += real_io_size;
    ++ctx->stats.io_read_extents;
    ctx->stats.pg_cache_backfills += count;
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
        assert(type == STORE_DATA);
        break;
    }
}

void flush_pages_cb(uv_fs_t* req)
{
    struct rrdengine_worker_config* wc = req->loop->data;
    struct rrdengine_instance *ctx = wc->ctx;
    struct extent_io_descriptor *xt_io_descr;
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr;
    unsigned i, count;

    xt_io_descr = req->data;
    if (req->result < 0) {
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        error("%s: uv_fs_write: %s", __func__, uv_strerror((int)req->result));
    }
#ifdef NETDATA_INTERNAL_CHECKS
    {
        struct rrdengine_datafile *datafile = xt_io_descr->descr_array[0]->extent->datafile;
        debug(D_RRDENGINE, "%s: Extent at offset %"PRIu64"(%u) was written to datafile %u-%u. Waking up waiters.",
              __func__, xt_io_descr->pos, xt_io_descr->bytes, datafile->tier, datafile->fileno);
    }
#endif
    count = xt_io_descr->descr_count;
    for (i = 0 ; i < count ; ++i) {
        /* care, we don't hold the descriptor mutex */
        descr = xt_io_descr->descr_array[i];

        pg_cache_replaceQ_insert(ctx, descr);

        rrdeng_page_descr_mutex_lock(ctx, descr);
        pg_cache_descr = descr->pg_cache_descr;
        pg_cache_descr->flags &= ~(RRD_PAGE_DIRTY | RRD_PAGE_WRITE_PENDING);
        /* wake up waiters, care no reference being held */
        pg_cache_wake_up_waiters_unsafe(descr);
        rrdeng_page_descr_mutex_unlock(ctx, descr);
    }
    if (xt_io_descr->completion)
        complete(xt_io_descr->completion);
    uv_fs_req_cleanup(req);
    free(xt_io_descr->buf);
    freez(xt_io_descr);
}

/*
 * completion must be NULL or valid.
 * Returns 0 when no flushing can take place.
 * Returns datafile bytes to be written on successful flushing initiation.
 */
static int do_flush_pages(struct rrdengine_worker_config* wc, int force, struct completion *completion)
{
    struct rrdengine_instance *ctx = wc->ctx;
    struct page_cache *pg_cache = &ctx->pg_cache;
    int ret;
    int compressed_size, max_compressed_size = 0;
    unsigned i, count, size_bytes, pos, real_io_size;
    uint32_t uncompressed_payload_length, payload_offset;
    struct rrdeng_page_descr *descr, *eligible_pages[MAX_PAGES_PER_EXTENT];
    struct page_cache_descr *pg_cache_descr;
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

    if (force) {
        debug(D_RRDENGINE, "Asynchronous flushing of extent has been forced by page pressure.");
    }
    uv_rwlock_wrlock(&pg_cache->commited_page_index.lock);
    for (Index = 0, count = 0, uncompressed_payload_length = 0,
         PValue = JudyLFirst(pg_cache->commited_page_index.JudyL_array, &Index, PJE0),
         descr = unlikely(NULL == PValue) ? NULL : *PValue ;

         descr != NULL && count != MAX_PAGES_PER_EXTENT ;

         PValue = JudyLNext(pg_cache->commited_page_index.JudyL_array, &Index, PJE0),
         descr = unlikely(NULL == PValue) ? NULL : *PValue) {
        uint8_t page_write_pending;

        assert(0 != descr->page_length);
        page_write_pending = 0;

        rrdeng_page_descr_mutex_lock(ctx, descr);
        pg_cache_descr = descr->pg_cache_descr;
        if (!(pg_cache_descr->flags & RRD_PAGE_WRITE_PENDING)) {
            page_write_pending = 1;
            /* care, no reference being held */
            pg_cache_descr->flags |= RRD_PAGE_WRITE_PENDING;
            uncompressed_payload_length += descr->page_length;
            descr_commit_idx_array[count] = Index;
            eligible_pages[count++] = descr;
        }
        rrdeng_page_descr_mutex_unlock(ctx, descr);

        if (page_write_pending) {
            ret = JudyLDel(&pg_cache->commited_page_index.JudyL_array, Index, PJE0);
            assert(1 == ret);
            --pg_cache->commited_page_index.nr_commited_pages;
        }
    }
    uv_rwlock_wrunlock(&pg_cache->commited_page_index.lock);

    if (!count) {
        debug(D_RRDENGINE, "%s: no pages eligible for flushing.", __func__);
        if (completion)
            complete(completion);
        return 0;
    }
    xt_io_descr = mallocz(sizeof(*xt_io_descr));
    payload_offset = sizeof(*header) + count * sizeof(header->descr[0]);
    switch (compression_algorithm) {
    case RRD_NO_COMPRESSION:
        size_bytes = payload_offset + uncompressed_payload_length + sizeof(*trailer);
        break;
    default: /* Compress */
        assert(uncompressed_payload_length < LZ4_MAX_INPUT_SIZE);
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

    for (i = 0 ; i < count ; ++i) {
        /* This is here for performance reasons */
        xt_io_descr->descr_commit_idx_array[i] = descr_commit_idx_array[i];

        descr = xt_io_descr->descr_array[i];
        header->descr[i].type = PAGE_METRICS;
        uuid_copy(*(uuid_t *)header->descr[i].uuid, *descr->id);
        header->descr[i].page_length = descr->page_length;
        header->descr[i].start_time = descr->start_time;
        header->descr[i].end_time = descr->end_time;
        pos += sizeof(header->descr[i]);
    }
    for (i = 0 ; i < count ; ++i) {
        descr = xt_io_descr->descr_array[i];
        /* care, we don't hold the descriptor mutex */
        (void) memcpy(xt_io_descr->buf + pos, descr->pg_cache_descr->page, descr->page_length);
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
    ret = uv_fs_write(wc->loop, &xt_io_descr->req, datafile->file, &xt_io_descr->iov, 1, datafile->pos, flush_pages_cb);
    assert (-1 != ret);
    ctx->stats.io_write_bytes += real_io_size;
    ++ctx->stats.io_write_requests;
    ctx->stats.io_write_extent_bytes += real_io_size;
    ++ctx->stats.io_write_extents;
    do_commit_transaction(wc, STORE_DATA, xt_io_descr);
    datafile->pos += ALIGN_BYTES_CEILING(size_bytes);
    ctx->disk_space += ALIGN_BYTES_CEILING(size_bytes);
    rrdeng_test_quota(wc);

    return ALIGN_BYTES_CEILING(size_bytes);
}

static void after_delete_old_data(uv_work_t *req, int status)
{
    struct rrdengine_instance *ctx = req->data;
    struct rrdengine_worker_config* wc = &ctx->worker_config;
    struct rrdengine_datafile *datafile;
    struct rrdengine_journalfile *journalfile;
    unsigned deleted_bytes, journalfile_bytes, datafile_bytes;
    int ret;
    char path[RRDENG_PATH_MAX];

    (void)status;
    datafile = ctx->datafiles.first;
    journalfile = datafile->journalfile;
    datafile_bytes = datafile->pos;
    journalfile_bytes = journalfile->pos;
    deleted_bytes = 0;

    info("Deleting data and journal file pair.");
    datafile_list_delete(ctx, datafile);
    ret = destroy_journal_file(journalfile, datafile);
    if (!ret) {
        generate_journalfilepath(datafile, path, sizeof(path));
        info("Deleted journal file \"%s\".", path);
        deleted_bytes += journalfile_bytes;
    }
    ret = destroy_data_file(datafile);
    if (!ret) {
        generate_datafilepath(datafile, path, sizeof(path));
        info("Deleted data file \"%s\".", path);
        deleted_bytes += datafile_bytes;
    }
    freez(journalfile);
    freez(datafile);

    ctx->disk_space -= deleted_bytes;
    info("Reclaimed %u bytes of disk space.", deleted_bytes);

    /* unfreeze command processing */
    wc->now_deleting.data = NULL;
    /* wake up event loop */
    assert(0 == uv_async_send(&wc->async));
}

static void delete_old_data(uv_work_t *req)
{
    struct rrdengine_instance *ctx = req->data;
    struct rrdengine_datafile *datafile;
    struct extent_info *extent, *next;
    struct rrdeng_page_descr *descr;
    unsigned count, i;

    /* Safe to use since it will be deleted after we are done */
    datafile = ctx->datafiles.first;

    for (extent = datafile->extents.first ; extent != NULL ; extent = next) {
        count = extent->number_of_pages;
        for (i = 0 ; i < count ; ++i) {
            descr = extent->pages[i];
            pg_cache_punch_hole(ctx, descr, 0);
        }
        next = extent->next;
        freez(extent);
    }
}

void rrdeng_test_quota(struct rrdengine_worker_config* wc)
{
    struct rrdengine_instance *ctx = wc->ctx;
    struct rrdengine_datafile *datafile;
    unsigned current_size, target_size;
    uint8_t out_of_space, only_one_datafile;
    int ret;

    out_of_space = 0;
    if (unlikely(ctx->disk_space > ctx->max_disk_space)) {
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
        wal_flush_transaction_buffer(wc);
        ret = create_new_datafile_pair(ctx, 1, ctx->last_fileno + 1);
        if (likely(!ret)) {
            ++ctx->last_fileno;
        }
    }
    if (unlikely(out_of_space)) {
        /* delete old data */
        if (wc->now_deleting.data) {
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
        wc->now_deleting.data = ctx;
        assert(0 == uv_queue_work(wc->loop, &wc->now_deleting, delete_old_data, after_delete_old_data));
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
    assert(0 == uv_cond_init(&wc->cmd_cond));
    assert(0 == uv_mutex_init(&wc->cmd_mutex));
}

void rrdeng_enq_cmd(struct rrdengine_worker_config* wc, struct rrdeng_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&wc->cmd_mutex);
    while ((queue_size = wc->queue_size) == RRDENG_CMD_Q_MAX_SIZE) {
        uv_cond_wait(&wc->cmd_cond, &wc->cmd_mutex);
    }
    assert(queue_size < RRDENG_CMD_Q_MAX_SIZE);
    /* enqueue command */
    wc->cmd_queue.cmd_array[wc->cmd_queue.tail] = *cmd;
    wc->cmd_queue.tail = wc->cmd_queue.tail != RRDENG_CMD_Q_MAX_SIZE - 1 ?
                         wc->cmd_queue.tail + 1 : 0;
    wc->queue_size = queue_size + 1;
    uv_mutex_unlock(&wc->cmd_mutex);

    /* wake up event loop */
    assert(0 == uv_async_send(&wc->async));
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

void timer_cb(uv_timer_t* handle)
{
    struct rrdengine_worker_config* wc = handle->data;

    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    rrdeng_test_quota(wc);
    debug(D_RRDENGINE, "%s: timeout reached.", __func__);
    if (likely(!wc->now_deleting.data)) {
        unsigned total_bytes, bytes_written;

        /* There is free space so we can write to disk */
        debug(D_RRDENGINE, "Flushing pages to disk.");
        for (total_bytes = bytes_written = do_flush_pages(wc, 0, NULL) ;
             bytes_written && (total_bytes < DATAFILE_IDEAL_IO_SIZE) ;
             total_bytes += bytes_written) {
            bytes_written = do_flush_pages(wc, 0, NULL);
        }
    }
#ifdef NETDATA_INTERNAL_CHECKS
    {
        char buf[4096];
        debug(D_RRDENGINE, "%s", get_rrdeng_statistics(wc->ctx, buf, sizeof(buf)));
    }
#endif
}

/* Flushes dirty pages when timer expires */
#define TIMER_PERIOD_MS (1000)

#define MAX_CMD_BATCH_SIZE (256)

void rrdeng_worker(void* arg)
{
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

    wc->now_deleting.data = NULL;

    /* dirty page flushing timer */
    ret = uv_timer_init(loop, &timer_req);
    if (ret) {
        error("uv_timer_init(): %s", uv_strerror(ret));
        goto error_after_timer_init;
    }
    timer_req.data = wc;

    wc->error = 0;
    /* wake up initialization thread */
    complete(&ctx->rrdengine_completion);

    assert(0 == uv_timer_start(&timer_req, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));
    shutdown = 0;
    while (shutdown == 0 || uv_loop_alive(loop)) {
        uv_run(loop, UV_RUN_DEFAULT);

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

            switch (opcode) {
            case RRDENG_NOOP:
                /* the command queue was empty, do nothing */
                break;
            case RRDENG_SHUTDOWN:
                shutdown = 1;
                /*
                 * uv_async_send after uv_close does not seem to crash in linux at the moment,
                 * it is however undocumented behaviour and we need to be aware if this becomes
                 * an issue in the future.
                 */
                uv_close((uv_handle_t *)&wc->async, NULL);
                assert(0 == uv_timer_stop(&timer_req));
                uv_close((uv_handle_t *)&timer_req, NULL);
                break;
            case RRDENG_READ_PAGE:
                do_read_extent(wc, &cmd.read_page.page_cache_descr, 1, 0);
                break;
            case RRDENG_READ_EXTENT:
                do_read_extent(wc, cmd.read_extent.page_cache_descr, cmd.read_extent.page_count, 1);
                break;
            case RRDENG_COMMIT_PAGE:
                do_commit_transaction(wc, STORE_DATA, NULL);
                break;
            case RRDENG_FLUSH_PAGES: {
                unsigned bytes_written;

                /* First I/O should be enough to call completion */
                bytes_written = do_flush_pages(wc, 1, cmd.completion);
                if (bytes_written) {
                    while (do_flush_pages(wc, 1, NULL)) {
                        ; /* Force flushing of all commited pages. */
                    }
                }
                break;
            }
            default:
                debug(D_RRDENGINE, "%s: default.", __func__);
                break;
            }
        } while (opcode != RRDENG_NOOP);
    }
    /* cleanup operations of the event loop */
    if (unlikely(wc->now_deleting.data)) {
        info("Postponing shutting RRD engine event loop down until after datafile deletion is finished.");
    }
    info("Shutting down RRD engine event loop.");
    while (do_flush_pages(wc, 1, NULL)) {
        ; /* Force flushing of all commited pages. */
    }
    wal_flush_transaction_buffer(wc);
    uv_run(loop, UV_RUN_DEFAULT);

    info("Shutting down RRD engine event loop complete.");
    /* TODO: don't let the API block by waiting to enqueue commands */
    uv_cond_destroy(&wc->cmd_cond);
/*  uv_mutex_destroy(&wc->cmd_mutex); */
    assert(0 == uv_loop_close(loop));
    freez(loop);

    return;

error_after_timer_init:
    uv_close((uv_handle_t *)&wc->async, NULL);
error_after_async_init:
    assert(0 == uv_loop_close(loop));
error_after_loop_init:
    freez(loop);

    wc->error = UV_EAGAIN;
    /* wake up initialization thread */
    complete(&ctx->rrdengine_completion);
}

/* C entry point for development purposes
 * make "LDFLAGS=-errdengine_main"
 */
void rrdengine_main(void)
{
    int ret;
    struct rrdengine_instance *ctx;

    ret = rrdeng_init(&ctx, "/tmp", RRDENG_MIN_PAGE_CACHE_SIZE_MB, RRDENG_MIN_DISK_SPACE_MB);
    if (ret) {
        exit(ret);
    }
    rrdeng_exit(ctx);
    fprintf(stderr, "Hello world!");
    exit(0);
}