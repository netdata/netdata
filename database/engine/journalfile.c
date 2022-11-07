// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

static void flush_transaction_buffer_cb(uv_fs_t* req)
{
    struct generic_io_descriptor *io_descr = req->data;
    struct rrdengine_worker_config* wc = req->loop->data;
    struct rrdengine_instance *ctx = wc->ctx;

    debug(D_RRDENGINE, "%s: Journal block was written to disk.", __func__);
    if (req->result < 0) {
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        error("%s: uv_fs_write: %s", __func__, uv_strerror((int)req->result));
    } else {
        debug(D_RRDENGINE, "%s: Journal block was written to disk.", __func__);
    }

    uv_fs_req_cleanup(req);
    posix_memfree(io_descr->buf);

    // This is the journalfile we are processing
    struct rrdengine_journalfile *journalfile = io_descr->data;

    // Determine if we have created a new datafile / journal pair
    if (journalfile->datafile->fileno < journalfile->datafile->ctx->last_fileno) {
        if ((NULL == ctx->commit_log.buf || 0 == ctx->commit_log.buf_pos)) {
            // We can index the journal file
            info("Journal file %u is ready to be indexed", journalfile->datafile->fileno);

            struct rrdeng_work *work_request;
            work_request = mallocz(sizeof(*work_request));
            work_request->req.data = work_request;
            work_request->journalfile = journalfile;
            work_request->wc = wc;
            wc->running_journal_migration = 1;
            if (unlikely(uv_queue_work(wc->loop, &work_request->req, start_journal_indexing, after_journal_indexing))) {
                freez(work_request);
                wc->running_journal_migration = 0;
            }
        }
    }
    freez(io_descr);
}

/* Careful to always call this before creating a new journal file */
void wal_flush_transaction_buffer(struct rrdengine_worker_config* wc)
{
    struct rrdengine_instance *ctx = wc->ctx;
    int ret;
    struct generic_io_descriptor *io_descr;
    unsigned pos, size;
    struct rrdengine_journalfile *journalfile;

    if (unlikely(NULL == ctx->commit_log.buf || 0 == ctx->commit_log.buf_pos)) {
        return;
    }
    /* care with outstanding transactions when switching journal files */
    journalfile = ctx->datafiles.last->journalfile;

    io_descr = mallocz(sizeof(*io_descr));
    pos = ctx->commit_log.buf_pos;
    size = ctx->commit_log.buf_size;
    if (pos < size) {
        /* simulate an empty transaction to skip the rest of the block */
        *(uint8_t *) (ctx->commit_log.buf + pos) = STORE_PADDING;
    }
    io_descr->buf = ctx->commit_log.buf;
    io_descr->bytes = size;
    io_descr->pos = journalfile->pos;
    io_descr->req.data = io_descr;
    io_descr->data = journalfile;
    io_descr->completion = NULL;

    io_descr->iov = uv_buf_init((void *)io_descr->buf, size);
    ret = uv_fs_write(wc->loop, &io_descr->req, journalfile->file, &io_descr->iov, 1,
                      journalfile->pos, flush_transaction_buffer_cb);
    fatal_assert(-1 != ret);
    journalfile->pos += RRDENG_BLOCK_SIZE;
    ctx->disk_space += RRDENG_BLOCK_SIZE;
    ctx->commit_log.buf = NULL;
    ctx->stats.io_write_bytes += RRDENG_BLOCK_SIZE;
    ++ctx->stats.io_write_requests;
}

void * wal_get_transaction_buffer(struct rrdengine_worker_config* wc, unsigned size)
{
    struct rrdengine_instance *ctx = wc->ctx;
    int ret;
    unsigned buf_pos = 0, buf_size;

    fatal_assert(size);
    if (ctx->commit_log.buf) {
        unsigned remaining;

        buf_pos = ctx->commit_log.buf_pos;
        buf_size = ctx->commit_log.buf_size;
        remaining = buf_size - buf_pos;
        if (size > remaining) {
            /* we need a new buffer */
            wal_flush_transaction_buffer(wc);
        }
    }
    if (NULL == ctx->commit_log.buf) {
        buf_size = ALIGN_BYTES_CEILING(size);
        ret = posix_memalign((void *)&ctx->commit_log.buf, RRDFILE_ALIGNMENT, buf_size);
        if (unlikely(ret)) {
            fatal("posix_memalign:%s", strerror(ret));
        }
        memset(ctx->commit_log.buf, 0, buf_size);
        buf_pos = ctx->commit_log.buf_pos = 0;
        ctx->commit_log.buf_size =  buf_size;
    }
    ctx->commit_log.buf_pos += size;

    return ctx->commit_log.buf + buf_pos;
}

void generate_journalfilepath_v2(struct rrdengine_datafile *datafile, char *str, size_t maxlen)
{
    (void) snprintfz(str, maxlen, "%s/" WALFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL WALFILE_EXTENSION_V2,
                    datafile->ctx->dbfiles_path, datafile->tier, datafile->fileno);
}

void generate_journalfilepath(struct rrdengine_datafile *datafile, char *str, size_t maxlen)
{
    (void) snprintfz(str, maxlen, "%s/" WALFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL WALFILE_EXTENSION,
                    datafile->ctx->dbfiles_path, datafile->tier, datafile->fileno);
}

void journalfile_init(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile)
{
    journalfile->file = (uv_file)0;
    journalfile->pos = 0;
    journalfile->datafile = datafile;
    journalfile->journal_data = NULL;
    journalfile->journal_data_size = 0;
    journalfile->JudyL_array = (Pvoid_t) NULL;
    journalfile->data = NULL;
    journalfile->file_index = 0;
    journalfile->last_access = 0;
}

static int close_uv_file(struct rrdengine_datafile *datafile, uv_file file)
{
    int ret;
    char path[RRDENG_PATH_MAX];

    uv_fs_t req;
    ret = uv_fs_close(NULL, &req, file, NULL);
    if (ret < 0) {
        generate_journalfilepath(datafile, path, sizeof(path));
        error("uv_fs_close(%s): %s", path, uv_strerror(ret));
        ++datafile->ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);
    return ret;
}

int close_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    char path[RRDENG_PATH_MAX];

    if (likely(journalfile->journal_data)) {
        if (munmap(journalfile->journal_data, journalfile->journal_data_size)) {
            generate_journalfilepath_v2(datafile, path, sizeof(path));
            error("Failed to unmap journal index file for %s", path);
            ++ctx->stats.fs_errors;
            rrd_stat_atomic_add(&global_fs_errors, 1);
        }
        journalfile->journal_data = NULL;
        journalfile->journal_data_size = 0;
        return 0;
    }

    return close_uv_file(datafile, journalfile->file);
}

int unlink_journal_file(struct rrdengine_journalfile *journalfile)
{
    struct rrdengine_datafile *datafile = journalfile->datafile;
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_journalfilepath(datafile, path, sizeof(path));

    ret = uv_fs_unlink(NULL, &req, path, NULL);
    if (ret < 0) {
        error("uv_fs_fsunlink(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    ++ctx->stats.journalfile_deletions;

    return ret;
}

int destroy_journal_file_unsafe(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];
    char path_v2[RRDENG_PATH_MAX];

    generate_journalfilepath(datafile, path, sizeof(path));
    generate_journalfilepath_v2(datafile, path_v2, sizeof(path));

    ret = uv_fs_ftruncate(NULL, &req, journalfile->file, 0, NULL);
    if (ret < 0) {
        error("uv_fs_ftruncate(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    (void) close_uv_file(datafile, journalfile->file);

    // This is the new journal v2 index file
    ret = uv_fs_unlink(NULL, &req, path_v2, NULL);
    if (ret < 0) {
        error("uv_fs_fsunlink(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    ret = uv_fs_unlink(NULL, &req, path, NULL);
    if (ret < 0) {
        error("uv_fs_fsunlink(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    ++ctx->stats.journalfile_deletions;
    ++ctx->stats.journalfile_deletions;

    if (journalfile->journal_data) {
        if (munmap(journalfile->journal_data, journalfile->journal_data_size)) {
            error("Failed to unmap index file %s", path_v2);
        }
    }

    return ret;
}

int create_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    uv_file file;
    int ret, fd;
    struct rrdeng_jf_sb *superblock;
    uv_buf_t iov;
    char path[RRDENG_PATH_MAX];

    generate_journalfilepath(datafile, path, sizeof(path));
    fd = open_file_direct_io(path, O_CREAT | O_RDWR | O_TRUNC, &file);
    if (fd < 0) {
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return fd;
    }
    journalfile->file = file;
    ++ctx->stats.journalfile_creations;

    ret = posix_memalign((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    if (unlikely(ret)) {
        fatal("posix_memalign:%s", strerror(ret));
    }
    memset(superblock, 0, sizeof(*superblock));
    (void) strncpy(superblock->magic_number, RRDENG_JF_MAGIC, RRDENG_MAGIC_SZ);
    (void) strncpy(superblock->version, RRDENG_JF_VER, RRDENG_VER_SZ);

    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_write(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        fatal_assert(req.result < 0);
        error("uv_fs_write: %s", uv_strerror(ret));
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
    }
    uv_fs_req_cleanup(&req);
    posix_memfree(superblock);
    if (ret < 0) {
        destroy_journal_file_unsafe(journalfile, datafile);
        return ret;
    }

    journalfile->pos = sizeof(*superblock);
    ctx->stats.io_write_bytes += sizeof(*superblock);
    ++ctx->stats.io_write_requests;

    return 0;
}

static int check_journal_file_superblock(uv_file file)
{
    int ret;
    struct rrdeng_jf_sb *superblock;
    uv_buf_t iov;
    uv_fs_t req;

    ret = posix_memalign((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    if (unlikely(ret)) {
        fatal("posix_memalign:%s", strerror(ret));
    }
    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_read(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        error("uv_fs_read: %s", uv_strerror(ret));
        uv_fs_req_cleanup(&req);
        goto error;
    }
    fatal_assert(req.result >= 0);
    uv_fs_req_cleanup(&req);

    if (strncmp(superblock->magic_number, RRDENG_JF_MAGIC, RRDENG_MAGIC_SZ) ||
        strncmp(superblock->version, RRDENG_JF_VER, RRDENG_VER_SZ)) {
        error("File has invalid superblock.");
        ret = UV_EINVAL;
    } else {
        ret = 0;
    }
    error:
    posix_memfree(superblock);
    return ret;
}

static void restore_extent_metadata(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile,
                                    void *buf, unsigned max_size)
{
    static BITMAP256 page_error_map;
    struct page_cache *pg_cache = &ctx->pg_cache;
    unsigned i, count, payload_length, descr_size, valid_pages;
    struct rrdeng_page_descr *descr;
    struct extent_info *extent;
    /* persistent structures */
    struct rrdeng_jf_store_data *jf_metric_data;

    jf_metric_data = buf;
    count = jf_metric_data->number_of_pages;
    descr_size = sizeof(*jf_metric_data->descr) * count;
    payload_length = sizeof(*jf_metric_data) + descr_size;
    if (payload_length > max_size) {
        error("Corrupted transaction payload.");
        return;
    }

    extent = mallocz(sizeof(*extent) + count * sizeof(extent->pages[0]));
    extent->offset = jf_metric_data->extent_offset;
    extent->size = jf_metric_data->extent_size;
    extent->datafile = journalfile->datafile;
    extent->next = NULL;

    for (i = 0, valid_pages = 0 ; i < count ; ++i) {
        uuid_t *temp_id;
        Pvoid_t *PValue;
        struct pg_cache_page_index *page_index = NULL;
        uint8_t page_type = jf_metric_data->descr[i].type;

        if (page_type > PAGE_TYPE_MAX) {
            if (!bitmap256_get_bit(&page_error_map, page_type)) {
                error("Unknown page type %d encountered.", page_type);
                bitmap256_set_bit(&page_error_map, page_type, 1);
            }
            continue;
        }
        uint64_t start_time_ut = jf_metric_data->descr[i].start_time_ut;
        uint64_t end_time_ut = jf_metric_data->descr[i].end_time_ut;
        size_t entries = jf_metric_data->descr[i].page_length / page_type_size[page_type];
        time_t update_every_s = (entries > 1) ? ((end_time_ut - start_time_ut) / USEC_PER_SEC / (entries - 1)) : 0;

        if (unlikely(start_time_ut > end_time_ut)) {
            ctx->load_errors[LOAD_ERRORS_PAGE_FLIPPED_TIME].counter++;
            if(ctx->load_errors[LOAD_ERRORS_PAGE_FLIPPED_TIME].latest_end_time_ut < end_time_ut)
                ctx->load_errors[LOAD_ERRORS_PAGE_FLIPPED_TIME].latest_end_time_ut = end_time_ut;
            continue;
        }

        if (unlikely(start_time_ut == end_time_ut && entries != 1)) {
            ctx->load_errors[LOAD_ERRORS_PAGE_EQUAL_TIME].counter++;
            if(ctx->load_errors[LOAD_ERRORS_PAGE_EQUAL_TIME].latest_end_time_ut < end_time_ut)
                ctx->load_errors[LOAD_ERRORS_PAGE_EQUAL_TIME].latest_end_time_ut = end_time_ut;
            continue;
        }

        if (unlikely(!entries)) {
            ctx->load_errors[LOAD_ERRORS_PAGE_ZERO_ENTRIES].counter++;
            if(ctx->load_errors[LOAD_ERRORS_PAGE_ZERO_ENTRIES].latest_end_time_ut < end_time_ut)
                ctx->load_errors[LOAD_ERRORS_PAGE_ZERO_ENTRIES].latest_end_time_ut = end_time_ut;
            continue;
        }

        if(entries > 1 && update_every_s == 0) {
            ctx->load_errors[LOAD_ERRORS_PAGE_UPDATE_ZERO].counter++;
            if(ctx->load_errors[LOAD_ERRORS_PAGE_UPDATE_ZERO].latest_end_time_ut < end_time_ut)
                ctx->load_errors[LOAD_ERRORS_PAGE_UPDATE_ZERO].latest_end_time_ut = end_time_ut;
            continue;
        }

        if(start_time_ut + update_every_s * USEC_PER_SEC * (entries - 1) != end_time_ut) {
            ctx->load_errors[LOAD_ERRORS_PAGE_FLEXY_TIME].counter++;
            if(ctx->load_errors[LOAD_ERRORS_PAGE_FLEXY_TIME].latest_end_time_ut < end_time_ut)
                ctx->load_errors[LOAD_ERRORS_PAGE_FLEXY_TIME].latest_end_time_ut = end_time_ut;

            // let this be
            // end_time_ut = start_time_ut + update_every_s * USEC_PER_SEC * (entries - 1);
        }

        temp_id = (uuid_t *)jf_metric_data->descr[i].uuid;

        uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
        PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, temp_id, sizeof(uuid_t));
        if (likely(NULL != PValue)) {
            page_index = *PValue;
        }
        uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
        if (NULL == PValue) {
            /* First time we see the UUID */
            uv_rwlock_wrlock(&pg_cache->metrics_index.lock);
            PValue = JudyHSIns(&pg_cache->metrics_index.JudyHS_array, temp_id, sizeof(uuid_t), PJE0);
            fatal_assert(NULL == *PValue); /* TODO: figure out concurrency model */
                *PValue = page_index = create_page_index(temp_id, ctx);
                page_index->prev = pg_cache->metrics_index.last_page_index;
                pg_cache->metrics_index.last_page_index = page_index;
            uv_rwlock_wrunlock(&pg_cache->metrics_index.lock);
        }

        // Lookup this descriptor
        Word_t p_time = start_time_ut / USEC_PER_SEC;
        uv_rwlock_rdlock(&page_index->lock);
        PValue = JudyLFirst(page_index->JudyL_array, &p_time, PJE0);
        descr = (NULL == PValue) ? NULL: *PValue;
        uv_rwlock_rdunlock(&page_index->lock);

        bool descr_found = false;
        if (unlikely(descr && descr->start_time_ut == start_time_ut)) {
            // We have this descriptor already
            descr_found = true;

#ifdef  NETDATA_INTERNAL_CHECKS
            char uuid_str[UUID_STR_LEN];
            uuid_unparse_lower(page_index->id, uuid_str);
            internal_error(true, "REMOVING UUID %s with %lu, %lu length=%u (fileno=%u), extent database offset = %lu, size = %u",
                           uuid_str, start_time_ut, end_time_ut, jf_metric_data->descr[i].page_length, descr->extent->datafile->fileno,
                           descr->extent->offset, descr->extent->size);
            internal_error(true, "APPLYING UUID %s with %lu, %lu length=%u (fileno=%u), extent database offset = %lu, size = %u",
                           uuid_str, start_time_ut, end_time_ut, jf_metric_data->descr[i].page_length, extent->datafile->fileno,
                           extent->offset, extent->size);
#endif
            // Remove entry from previous extent
            struct extent_info *old_extent = descr->extent;
            for (uint8_t index = 0; index < old_extent->number_of_pages; index++) {
                if (old_extent->pages[index] == descr) {
                    old_extent->pages[index] = NULL;
                    internal_error(true, "REMOVING UUID %s with %lu OK", uuid_str, start_time_ut);
                    break;
                }
            }
        }
        else {
            descr = pg_cache_create_descr();
            descr->id = &page_index->id;
        }

        descr->page_length = jf_metric_data->descr[i].page_length;
        descr->start_time_ut = start_time_ut;
        descr->end_time_ut = end_time_ut;
        descr->update_every_s = (update_every_s > 0) ? (uint32_t)update_every_s : (page_index->latest_update_every_s);

        descr->extent = extent;
        descr->type = page_type;
        extent->pages[valid_pages++] = descr;
        if (likely(!descr_found))
            (void) pg_cache_insert(ctx, page_index, descr, true);

        if (page_index->latest_time_ut == descr->end_time_ut)
            page_index->latest_update_every_s = descr->update_every_s;

        if(descr->update_every_s == 0)
            fatal("DBENGINE: page descriptor update every is zero, end_time_ut = %llu, start_time_ut = %llu, entries = %zu",
                (unsigned long long)end_time_ut, (unsigned long long)start_time_ut, entries);
    }

    extent->number_of_pages = valid_pages;

    if (likely(valid_pages)) {
        df_extent_insert(extent);
    }
    else {
        freez(extent);
        ctx->load_errors[LOAD_ERRORS_DROPPED_EXTENT].counter++;
    }
}

/*
 * Replays transaction by interpreting up to max_size bytes from buf.
 * Sets id to the current transaction id or to 0 if unknown.
 * Returns size of transaction record or 0 for unknown size.
 */
static unsigned replay_transaction(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile,
                                   void *buf, uint64_t *id, unsigned max_size)
{
    unsigned payload_length, size_bytes;
    int ret;
    /* persistent structures */
    struct rrdeng_jf_transaction_header *jf_header;
    struct rrdeng_jf_transaction_trailer *jf_trailer;
    uLong crc;

    *id = 0;
    jf_header = buf;
    if (STORE_PADDING == jf_header->type) {
        debug(D_RRDENGINE, "Skipping padding.");
        return 0;
    }
    if (sizeof(*jf_header) > max_size) {
        error("Corrupted transaction record, skipping.");
        return 0;
    }
    *id = jf_header->id;
    payload_length = jf_header->payload_length;
    size_bytes = sizeof(*jf_header) + payload_length + sizeof(*jf_trailer);
    if (size_bytes > max_size) {
        error("Corrupted transaction record, skipping.");
        return 0;
    }
    jf_trailer = buf + sizeof(*jf_header) + payload_length;
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, buf, sizeof(*jf_header) + payload_length);
    ret = crc32cmp(jf_trailer->checksum, crc);
    debug(D_RRDENGINE, "Transaction %"PRIu64" was read from disk. CRC32 check: %s", *id, ret ? "FAILED" : "SUCCEEDED");
    if (unlikely(ret)) {
        error("Transaction %"PRIu64" was read from disk. CRC32 check: FAILED", *id);
        return size_bytes;
    }
    switch (jf_header->type) {
        case STORE_DATA:
            debug(D_RRDENGINE, "Replaying transaction %"PRIu64"", jf_header->id);
            restore_extent_metadata(ctx, journalfile, buf + sizeof(*jf_header), payload_length);
            break;
        default:
            error("Unknown transaction type. Skipping record.");
            break;
    }

    return size_bytes;
}


#define READAHEAD_BYTES (RRDENG_BLOCK_SIZE * 256)
/*
 * Iterates journal file transactions and populates the page cache.
 * Page cache must already be initialized.
 * Returns the maximum transaction id it discovered.
 */
static uint64_t iterate_transactions(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile)
{
    uv_file file;
    uint64_t file_size;//, data_file_size;
    int ret;
    uint64_t pos, pos_i, max_id, id;
    unsigned size_bytes;
    void *buf;
    uv_buf_t iov;
    uv_fs_t req;

    file = journalfile->file;
    file_size = journalfile->pos;
    //data_file_size = journalfile->datafile->pos; TODO: utilize this?

    max_id = 1;
    bool journal_is_mmapped = (journalfile->data != NULL);
    if (unlikely(!journal_is_mmapped)) {
        ret = posix_memalign((void *)&buf, RRDFILE_ALIGNMENT, READAHEAD_BYTES);
        if (unlikely(ret))
            fatal("posix_memalign:%s", strerror(ret));
    }
    else
        buf = journalfile->data +  sizeof(struct rrdeng_jf_sb);
    for (pos = sizeof(struct rrdeng_jf_sb) ; pos < file_size ; pos += READAHEAD_BYTES) {
        size_bytes = MIN(READAHEAD_BYTES, file_size - pos);
        if (unlikely(!journal_is_mmapped)) {
            iov = uv_buf_init(buf, size_bytes);
            ret = uv_fs_read(NULL, &req, file, &iov, 1, pos, NULL);
            if (ret < 0) {
                error("uv_fs_read: pos=%" PRIu64 ", %s", pos, uv_strerror(ret));
                uv_fs_req_cleanup(&req);
                goto skip_file;
            }
            fatal_assert(req.result >= 0);
            uv_fs_req_cleanup(&req);
            ++ctx->stats.io_read_requests;
            ctx->stats.io_read_bytes += size_bytes;
        }

        for (pos_i = 0 ; pos_i < size_bytes ; ) {
            unsigned max_size;

            max_size = pos + size_bytes - pos_i;
            ret = replay_transaction(ctx, journalfile, buf + pos_i, &id, max_size);
            if (!ret) /* TODO: support transactions bigger than 4K */
                /* unknown transaction size, move on to the next block */
                pos_i = ALIGN_BYTES_FLOOR(pos_i + RRDENG_BLOCK_SIZE);
            else
                pos_i += ret;
            max_id = MAX(max_id, id);
        }
        if (likely(journal_is_mmapped))
            buf += size_bytes;
    }
    skip_file:
    if (unlikely(!journal_is_mmapped))
        posix_memfree(buf);
    return max_id;
}

// Checks that the extent list checksum is valid
static int check_journal_v2_extent_list (void *data_start, size_t file_size)
{
    UNUSED(file_size);
    uLong crc;

    struct journal_v2_header *j2_header = (void *) data_start;
    struct journal_v2_block_trailer *journal_v2_trailer;

    journal_v2_trailer = (struct journal_v2_block_trailer *) ((uint8_t *) data_start + j2_header->extent_trailer_offset);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (uint8_t *) data_start + j2_header->extent_offset, j2_header->extent_count * sizeof(struct journal_extent_list));
    if (unlikely(crc32cmp(journal_v2_trailer->checksum, crc))) {
        error("Extent list CRC32 check: FAILED");
        return 1;
    }

    return 0;
}

// Checks that the metric list (UUIDs) checksum is valid
static int check_journal_v2_metric_list(void *data_start, size_t file_size)
{
    UNUSED(file_size);
    uLong crc;

    struct journal_v2_header *j2_header = (void *) data_start;
    struct journal_v2_block_trailer *journal_v2_trailer;

    journal_v2_trailer = (struct journal_v2_block_trailer *) ((uint8_t *) data_start + j2_header->metric_trailer_offset);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (uint8_t *) data_start + j2_header->metric_offset, j2_header->metric_count * sizeof(struct journal_metric_list));
    if (unlikely(crc32cmp(journal_v2_trailer->checksum, crc))) {
        error("Metric list CRC32 check: FAILED");
        return 1;
    }
    return 0;
}

static int check_journal_v2_file(void *data_start, size_t file_size, uint32_t original_size)
{
    int rc;
    uLong crc;

    struct journal_v2_header *j2_header = (void *) data_start;
    struct journal_v2_block_trailer *journal_v2_trailer;

    // Magic failure
    if (j2_header->magic != JOURVAL_V2_MAGIC)
        return 1;

    if (j2_header->total_file_size != file_size)
        return 1;

    if (original_size && j2_header->original_file_size != original_size)
        return 1;

    journal_v2_trailer = (struct journal_v2_block_trailer *) ((uint8_t *) data_start + file_size - sizeof(*journal_v2_trailer));

    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (void *) j2_header, sizeof(*j2_header));

    rc = crc32cmp(journal_v2_trailer->checksum, crc);
    if (unlikely(rc)) {
        error("File CRC32 check: FAILED");
        return 1;
    }

    rc = check_journal_v2_extent_list(data_start, file_size);
    if (rc) return 1;

    rc = check_journal_v2_metric_list(data_start, file_size);
    if (rc) return 1;

    return 0;
}

int load_journal_file_v2(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct pg_cache_page_index *page_index;
    int ret, fd;
    uint64_t file_size;
    char path[RRDENG_PATH_MAX];
    struct stat statbuf;
    uint32_t original_file_size = 0;

    generate_journalfilepath(datafile, path, sizeof(path));
    ret = stat(path, &statbuf);
    if (!ret)
        original_file_size = (uint32_t)statbuf.st_size;

    generate_journalfilepath_v2(datafile, path, sizeof(path));

    fd = open(path, O_RDONLY);
    if (fd < 0) {
        if (errno == ENOENT)
            return 1;
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        error("Failed to open %s", path);
        return 1;
    }

    ret = fstat(fd, &statbuf);
    if (ret) {
        error("Failed to get file information for %s", path);
        close(fd);
        return 1;
    }

    file_size = (size_t)statbuf.st_size;

    if (file_size < sizeof(struct journal_v2_header)) {
        error_report("Invalid file %s. Not the expected size", path);
        close(fd);
        return 1;
    }

    usec_t start_loading = now_realtime_usec();
    uint8_t *data_start = mmap(NULL, file_size, PROT_READ, MAP_SHARED, fd, 0);
    if (data_start == MAP_FAILED) {
        close(fd);
        return 1;
    }
    close(fd);

    int rc = check_journal_v2_file(data_start, file_size, original_file_size);
    if (rc) {
        error_report("File %s is invalid", path);
        if (unlikely(munmap(data_start, file_size)))
            error("Failed to unmap %s", path);
        return rc;
    }

    struct journal_v2_header *j2_header = (void *) data_start;

    size_t entries = j2_header->metric_count;

    if (!entries) {
        if (unlikely(munmap(data_start, file_size)))
            error("Failed to unmap %s", path);
        return 1;
    }

    struct journal_metric_list *metric = (struct journal_metric_list *) (data_start + j2_header->metric_offset);

    uv_rwlock_wrlock(&pg_cache->metrics_index.lock);

    // Initialize the journal file to be able to access the data
    journalfile->journal_data = data_start;
    journalfile->journal_data_size = file_size;

    usec_t header_start_time = j2_header->start_time_ut;
    for (size_t i=0; i < entries; i++) {
        Pvoid_t *PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, metric->uuid, sizeof(uuid_t));
        if (likely(NULL != PValue)) {
            page_index = *PValue;
        }
        else {
            PValue = JudyHSIns(&pg_cache->metrics_index.JudyHS_array, metric->uuid, sizeof(uuid_t), PJE0);
            fatal_assert(NULL == *PValue);
            *PValue = page_index = create_page_index(&metric->uuid, ctx);
            page_index->oldest_time_ut = LLONG_MAX;
            page_index->latest_time_ut = 0;
        }

        usec_t metric_delta_start = header_start_time + (usec_t ) metric->delta_start;
        usec_t metric_delta_end = header_start_time + (usec_t ) metric->delta_end;

        if (page_index->oldest_time_ut > metric_delta_start)
            page_index->oldest_time_ut = metric_delta_start;

        if (page_index->latest_time_ut < metric_delta_end)
            page_index->latest_time_ut = metric_delta_end;

        page_index->prev = pg_cache->metrics_index.last_page_index;
        pg_cache->metrics_index.last_page_index = page_index;
        metric++;
    }

    uv_rwlock_wrunlock(&pg_cache->metrics_index.lock);

    info("Journal file \"%s\" loaded (size:%"PRIu64") with %lu metrics in %d ms",
         path, file_size, entries,
         (int) ((now_realtime_usec() - start_loading) / USEC_PER_MS));
    return 0;
}


struct metric_info_s {
    uuid_t *id;
    struct pg_cache_page_index *page_index;
    uint32_t entries;
    time_t min_index_time_s;
    time_t max_index_time_s;
    usec_t min_time_ut;
    usec_t max_time_ut;
    uint32_t extent_index;
    uint32_t page_list_header;
};

struct journal_metric_list_to_sort {
    struct metric_info_s *metric_info;
};

static int journal_metric_compare (const void *item1, const void *item2)
{
    const struct metric_info_s *metric1 = ((struct journal_metric_list_to_sort *) item1)->metric_info;
    const struct metric_info_s *metric2 = ((struct journal_metric_list_to_sort *) item2)->metric_info;

    return uuid_compare(*(metric1->id), *(metric2->id));
}


// Write list of extents for the journalfile
void *journal_v2_write_extent_list(struct rrdengine_journalfile *journalfile, void *data)
{
    struct extent_info *extent = journalfile->datafile->extents.first;
    struct journal_extent_list *j2_extent = (void *) data;
    while (extent) {
        j2_extent->datafile_offset = extent->offset;
        j2_extent->datafile_size = extent->size;
        j2_extent->pages = extent->number_of_pages;
        j2_extent->file_index = journalfile->file_index;
        j2_extent++;
        extent = extent->next;
    };
    return j2_extent;
}

static int verify_journal_space(struct journal_v2_header *j2_header, void *data, uint32_t bytes)
{
    if ((unsigned long)(((uint8_t *) data - (uint8_t *)  j2_header->data) + bytes) > (j2_header->total_file_size - sizeof(struct journal_v2_block_trailer)))
        return 1;

    return 0;
}

void *journal_v2_write_metric_page(struct journal_v2_header *j2_header, void *data, struct metric_info_s *metric_info, uint32_t pages_offset)
{
    struct journal_metric_list *metric = (void *) data;

    if (verify_journal_space(j2_header, data, sizeof(*metric)))
        return NULL;

    uuid_copy(metric->uuid, *metric_info->id);
    metric->entries = metric_info->entries;
    metric->page_offset = pages_offset;
    metric->delta_start = (metric_info->min_time_ut - j2_header->start_time_ut) / USEC_PER_SEC;
    metric->delta_end =   (metric_info->max_time_ut - j2_header->start_time_ut) / USEC_PER_SEC;

    return ++metric;
}

void *journal_v2_write_data_page_header(struct journal_v2_header *j2_header __maybe_unused, void *data, struct metric_info_s *metric_info, uint32_t uuid_offset)
{
    struct journal_page_header *data_page_header = (void *) data;
    uLong crc;

    uuid_copy(data_page_header->uuid, *metric_info->id);
    data_page_header->entries = metric_info->entries;
    data_page_header->uuid_offset = uuid_offset;        // data header OFFSET poings to METRIC in the directory
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (void *) data_page_header, sizeof(*data_page_header));
    crc32set(data_page_header->checksum, crc);
    return ++data_page_header;
}

int page_header_is_corrupted(struct journal_v2_header *j2_header, void *page_header)
{
    struct journal_page_header *data_page_header = page_header;

    uint32_t bytes = data_page_header->entries * sizeof(struct journal_page_list) + sizeof(struct journal_page_header);
    // Check if trailer overshoots the file

    struct journal_v2_header j2_header_local = { .data = j2_header};

    if (verify_journal_space(&j2_header_local, page_header, bytes))
        return 1;

    struct journal_v2_block_trailer *journal_trailer = ( struct journal_v2_block_trailer *) ((uint8_t *) page_header + bytes);

    uLong crc;
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (uint8_t *) page_header + sizeof(struct journal_page_header), data_page_header->entries * sizeof(struct journal_page_list));

    return crc32cmp(journal_trailer->checksum, crc);
}

void *journal_v2_write_data_page_trailer(struct journal_v2_header *j2_header __maybe_unused, void *data, void *page_header)
{
    struct journal_page_header *data_page_header = (void *) page_header;
    struct journal_v2_block_trailer *journal_trailer = (void *) data;
    uLong crc;

    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (uint8_t *) page_header + sizeof(struct journal_page_header), data_page_header->entries * sizeof(struct journal_page_list));
    crc32set(journal_trailer->checksum, crc);
    return ++journal_trailer;
}

void *journal_v2_write_data_page(struct journal_v2_header *j2_header, void *data, struct rrdeng_page_descr *descr)
{
    if (unlikely(!descr))
        return data;

    struct journal_page_list *data_page = data;

    // verify that we can write number of bytes
    if (verify_journal_space(j2_header, data, sizeof(*data_page)))
        return NULL;

    data_page->delta_start_s = (descr->start_time_ut - j2_header->start_time_ut) / USEC_PER_SEC;
    data_page->delta_end_s = (descr->end_time_ut - j2_header->start_time_ut) / USEC_PER_SEC;
    data_page->extent_index = descr->extent->index;
    data_page->update_every_s = descr->update_every_s;
    data_page->page_length = descr->page_length;
    data_page->type = descr->type;

    return ++data_page;
}

// For a page_index write all descr @ time entries
// Must be recorded in metric_info->entries
void *journal_v2_write_descriptors(struct journal_v2_header *j2_header, void *data, struct metric_info_s *metric_info, struct rrdengine_journalfile *journalfile)
{
    struct rrdeng_page_descr *descr;
    Pvoid_t *PValue;

    struct journal_page_list *data_page = (void *)data;
    struct page_cache *pg_cache = &journalfile->datafile->ctx->pg_cache;
    struct pg_cache_page_index *page_index;

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, metric_info->id, sizeof(uuid_t));
    page_index = (NULL == PValue) ? NULL : *PValue;
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);

    if (page_index == NULL)
        return data_page;

    // We need to write all descriptors with index metric_info->min_index_time_s, metric_info->max_index_time_s

    Word_t index_time = metric_info->min_index_time_s;
    for (PValue = JudyLFirst(page_index->JudyL_array, &index_time, PJE0),
        descr = unlikely(NULL == PValue) ? NULL : *PValue;
         descr != NULL;
         PValue = JudyLNext(page_index->JudyL_array, &index_time, PJE0),
        descr = unlikely(NULL == PValue) ? NULL : *PValue) {

        if (unlikely((time_t) index_time > metric_info->max_index_time_s))
            break;

        // Write one descriptor and return the next data page location
        data_page = journal_v2_write_data_page(j2_header, (void *)data_page, descr);

        if (unlikely(!data_page))
            break;
    }
    return data_page;
}

static void journal_v2_remove_active_descriptors(struct rrdengine_journalfile *journalfile, struct metric_info_s *metric_info, bool startup)
{
    if (true == startup) {
        // This is during startup, so we are the only ones accessing the structures
        // thats why we can safely remote the entire page_index->JudyL_array
        struct rrdeng_page_descr *descr;
        Word_t index_time;
        Pvoid_t *PValue;
        struct pg_cache_page_index *page_index;

        page_index = metric_info->page_index;

        for (index_time = 0, PValue = JudyLFirst(page_index->JudyL_array, &index_time, PJE0),
             descr = unlikely(NULL == PValue) ? NULL : *PValue; descr != NULL;
             PValue = JudyLNext(page_index->JudyL_array, &index_time, PJE0),
                 descr = unlikely(NULL == PValue) ? NULL : *PValue) {

            rrdeng_page_descr_freez(descr);
        }
        (void)JudyLFreeArray(&page_index->JudyL_array, PJE0);
    }
    else {
        // This is during startup, so we are the only ones accessing the structures
        // thats why we can safely remote the entire page_index->JudyL_array
        struct rrdeng_page_descr *descr;
        Pvoid_t *PValue;
        struct pg_cache_page_index *page_index;
        page_index = metric_info->page_index;

        struct rrdengine_instance *ctx = page_index->ctx;
        struct page_cache *pg_cache = &ctx->pg_cache;

        Word_t  index_time = metric_info->min_index_time_s;
        uint32_t metric_info_offset = metric_info->page_list_header;

        struct journal_page_header *page_list_header = (struct journal_page_header *) ((uint8_t *) journalfile->journal_data + metric_info_offset);
        struct journal_v2_header *journal_header = (struct journal_v2_header *) journalfile->journal_data;
        // Sanity check that we refer to the same UUID
        fatal_assert(uuid_compare(page_list_header->uuid, *metric_info->id) == 0);

        struct journal_page_list *page_list = (struct journal_page_list *)((uint8_t *) page_list_header + sizeof(*page_list_header));
        struct journal_extent_list *extent_list = (void *)((uint8_t *)journal_header + journal_header->extent_offset);

        uint32_t index = 0;
        uint32_t entries = page_list_header->entries;

        uv_file file = journalfile->datafile->file;

        uv_rwlock_rdlock(&page_index->lock);

        for (PValue = JudyLFirst(page_index->JudyL_array, &index_time, PJE0),
            descr = unlikely(NULL == PValue) ? NULL : *PValue;
             descr != NULL;
             PValue = JudyLNext(page_index->JudyL_array, &index_time, PJE0),
            descr = unlikely(NULL == PValue) ? NULL : *PValue) {

            if (unlikely((time_t) index_time > metric_info->max_index_time_s))
                break;

            if (index == entries)
                break;

            struct journal_page_list *page_entry = &page_list[index++];

            descr->extent_entry = &extent_list[page_entry->extent_index];
            descr->file = file;

            fatal_assert(descr->extent->offset == extent_list[page_entry->extent_index].datafile_offset);
            fatal_assert(descr->extent->size == extent_list[page_entry->extent_index].datafile_size);
        }
        uint32_t page_offset = (uint8_t *) page_list_header - (uint8_t *) journalfile->journal_data;
        mark_journalfile_descriptor(pg_cache, journalfile, page_offset, 1);

        uv_rwlock_rdunlock(&page_index->lock);
    }
}

// Migrate the journalfile pointed by datafile
// activate : make the new file active immediately
//            journafile data will be set and descriptors (if deleted) will be repopulated as needed
// startup  : if the migration is done during agent startup
//            this will allow us to optimize certain things
void migrate_journal_file_v2(struct rrdengine_datafile *datafile, bool activate, bool startup)
{
    char path[RRDENG_PATH_MAX];
    size_t number_of_extents = 0;        // Number of extents
    size_t number_of_metrics = 0;        // Number of unique metrics (UUIDS)
    size_t number_of_pages = 0;    // Total number of descriptors @ time
    Pvoid_t *PValue;
    Pvoid_t metrics_JudyL_array = NULL;
    Pvoid_t metrics_JudyHS_array = NULL;
    struct rrdengine_instance *ctx = datafile->ctx;
    struct rrdengine_journalfile *journalfile = datafile->journalfile;
    usec_t min_time_ut = LLONG_MAX;
    usec_t max_time_ut = 0;
    struct metric_info_s *metric_info;

    // Do nothing if we already have a mmaped file
    if (unlikely(journalfile->journal_data))
        return;

    generate_journalfilepath_v2(datafile, path, sizeof(path));
    info("Indexing file %s", path);


#ifdef NETDATA_INTERNAL_CHECKS
    usec_t start_loading = now_realtime_usec();
#endif

    struct extent_info *extent = datafile->extents.first;
    while (extent) {
        uint8_t extent_pages = extent->number_of_pages;
        for (uint8_t index = 0; index < extent_pages; index++) {
            struct rrdeng_page_descr *descr = extent->pages[index];

            if (unlikely(!descr))
                continue;

            PValue = JudyHSGet(metrics_JudyHS_array, descr->id, sizeof(uuid_t));
            if (likely(NULL != PValue)) {
                metric_info = *PValue;
            }
            else {
                PValue = JudyHSIns(&metrics_JudyHS_array, descr->id, sizeof(uuid_t), PJE0);
                *PValue = metric_info = mallocz(sizeof(*metric_info));

                metric_info->entries =0;
                metric_info->min_time_ut = LLONG_MAX;
                metric_info->max_time_ut = 0;
                metric_info->min_index_time_s = LLONG_MAX;
                metric_info->max_index_time_s = 0;
                metric_info->id = descr->id;
                metric_info->extent_index = number_of_extents;

                PValue = JudyLIns(&metrics_JudyL_array,number_of_metrics, PJE0);

                *PValue = metric_info;
                number_of_metrics++;
            }

            time_t current_index_time_s = (time_t) (descr->start_time_ut / USEC_PER_SEC);

            if (metric_info->min_time_ut > descr->start_time_ut) {
                metric_info->min_time_ut = descr->start_time_ut;
                metric_info->min_index_time_s = current_index_time_s;
            }

            if (metric_info->max_index_time_s < current_index_time_s)
                    metric_info->max_index_time_s =current_index_time_s;

            if (metric_info->max_time_ut < descr->end_time_ut)
                metric_info->max_time_ut = descr->end_time_ut;

            number_of_pages++;

            // Maintain the min max times to add to the journal header
            if (min_time_ut > descr->start_time_ut)
                min_time_ut = descr->start_time_ut;

            if (max_time_ut < descr->end_time_ut)
                max_time_ut = descr->end_time_ut;
        }
        extent->index = number_of_extents++;
        extent = extent->next;
    }

    internal_error(true, "Scan and extbuild metric %llu", (now_realtime_usec() - start_loading) / USEC_PER_MS);

    // Calculate total jourval file size
    size_t total_file_size = 0;
    total_file_size  += (sizeof(struct journal_v2_header) + JOURNAL_V2_HEADER_PADDING_SZ);

    // Extents will start here
    uint32_t extent_offset = total_file_size;
    total_file_size  += (number_of_extents * sizeof(struct journal_extent_list));

    uint32_t extent_offset_trailer = total_file_size;
    total_file_size  += sizeof(struct journal_v2_block_trailer);

    // UUID list will start here
    uint32_t metrics_offset = total_file_size;
    total_file_size  += (number_of_metrics * sizeof(struct journal_metric_list));

    // UUID list trailer
    uint32_t metric_offset_trailer = total_file_size;
    total_file_size  += sizeof(struct journal_v2_block_trailer);

    // descr @ time will start here
    uint32_t pages_offset = total_file_size;
    total_file_size  += (number_of_pages * (sizeof(struct journal_page_list) + sizeof(struct journal_page_header) + sizeof(struct journal_v2_block_trailer)));

    // File trailer
    uint32_t trailer_offset = total_file_size;
    total_file_size  += sizeof(struct journal_v2_block_trailer);

    uint8_t *data_start = netdata_mmap(path, total_file_size, MAP_SHARED, 0, false);
    uint8_t *data = data_start;

    memset(data_start, 0, extent_offset);

    // Write header
    struct journal_v2_header j2_header;

    j2_header.magic = JOURVAL_V2_MAGIC;
    j2_header.start_time_ut = min_time_ut;
    j2_header.end_time_ut = max_time_ut;
    j2_header.extent_count = number_of_extents;
    j2_header.extent_offset = extent_offset;
    j2_header.metric_count = number_of_metrics;
    j2_header.metric_offset = metrics_offset;
    j2_header.page_count = number_of_pages;
    j2_header.page_offset = pages_offset;
    j2_header.extent_trailer_offset = extent_offset_trailer;
    j2_header.metric_trailer_offset = metric_offset_trailer;
    j2_header.total_file_size = total_file_size;
    j2_header.original_file_size = (uint32_t) journalfile->pos;
    j2_header.data = data_start;                        // Used during migration

    struct journal_v2_block_trailer *journal_v2_trailer;

    // Write all the extents we have
    data = journal_v2_write_extent_list(journalfile, data_start + extent_offset);
    internal_error(true, "Write extent list so far %llu", (now_realtime_usec() - start_loading) / USEC_PER_MS);

    fatal_assert(data == data_start + extent_offset_trailer);

    // Calculate CRC for extents
    journal_v2_trailer = (struct journal_v2_block_trailer *) (data_start + extent_offset_trailer);
    uLong crc;
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (uint8_t *) data_start + extent_offset, number_of_extents * sizeof(struct journal_extent_list));
    crc32set(journal_v2_trailer->checksum, crc);

    internal_error(true, "CALCULATE CRC FOR EXTENT %llu", (now_realtime_usec() - start_loading) / USEC_PER_MS);
    // Skip the trailer, point to the metrics off
    data += sizeof(struct journal_v2_block_trailer);

    // Sanity check -- we must be at the metrics_offset
    fatal_assert(data == data_start + metrics_offset);

    // Allocate array to sort UUIDs and keep them sorted in the journal because we want to do binary search when we do lookups
    struct journal_metric_list_to_sort *uuid_list = mallocz(number_of_metrics * sizeof(struct journal_metric_list_to_sort));

    Word_t Index;
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct pg_cache_page_index *page_index;
    for (Index = 0, PValue = JudyLFirst(metrics_JudyL_array, &Index, PJE0),
         metric_info = unlikely(NULL == PValue) ? NULL : *PValue;
         metric_info != NULL;
         PValue = JudyLNext(metrics_JudyL_array, &Index, PJE0),
             metric_info = unlikely(NULL == PValue) ? NULL : *PValue) {

        fatal_assert(Index < number_of_metrics);
        uuid_list[Index].metric_info = metric_info;

        uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
        PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, metric_info->id, sizeof(uuid_t));
        page_index = (NULL == PValue) ? NULL : *PValue;
        uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
        fatal_assert(NULL != page_index);

        metric_info->page_index = page_index;

        // Count all entries that exist within a timerange
         metric_info->entries = JudyLCount(page_index->JudyL_array, metric_info->min_index_time_s, metric_info->max_index_time_s, PJE0);
    }
    // Cleanup judy arrays we no longer need
    JudyLFreeArray(&metrics_JudyL_array, PJE0);
    JudyHSFreeArray(&metrics_JudyHS_array, PJE0);

    qsort(&uuid_list[0], number_of_metrics, sizeof(struct journal_metric_list_to_sort), journal_metric_compare);
    internal_error(true, "Traverse and qsort  UUID %llu", (now_realtime_usec() - start_loading) / USEC_PER_MS);
    // Write sorted UUID LIST
    // The loop will write a single UUID entry then
    //   Write all entries (descr @ time) for that UUID at the proper location (header, number of entries, trailer)
    // Move on to write the next UUID
    // Write trailer after the UUID list
    for (Index = 0; Index < number_of_metrics; Index++) {
        metric_info = uuid_list[Index].metric_info;

        // Calculate current UUID offset from start of file. We will store this in the data page header
        uint32_t uuid_offset = data - data_start;

        // Write the UUID we are processing
        data  = (void *) journal_v2_write_metric_page(&j2_header, data, metric_info, pages_offset);
        if (unlikely(!data))
            break;

        // Next we will write
        //   Header
        //   Detailed entries (descr @ time)
        //   Trailer (checksum)

        // Keep the page_list_header, to be used for migration when where agent is running
        metric_info->page_list_header = pages_offset;
        // Write page header
        void *metric_page = journal_v2_write_data_page_header(&j2_header, data_start + pages_offset, metric_info, uuid_offset);

        // Start writing descr @ time
        void *page_trailer = journal_v2_write_descriptors(&j2_header, metric_page, metric_info, journalfile);
        if (unlikely(!page_trailer))
            break;

        // Trailer (checksum)
        uint8_t *next_page_address = journal_v2_write_data_page_trailer(&j2_header, page_trailer, data_start + pages_offset);

        // Calculate start of the pages start for next descriptor
        pages_offset += (metric_info->entries * (sizeof(struct journal_page_list)) + sizeof(struct journal_page_header) + sizeof(struct journal_v2_block_trailer));
        // Verify we are at the right location
        fatal_assert(pages_offset == (next_page_address - data_start));
    }
    // Data should be at the UUID trailer offset
    fatal_assert(data == data_start + metric_offset_trailer);

    internal_error(true, "WRITE METRICS AND PAGES  %llu", (now_realtime_usec() - start_loading) / USEC_PER_MS);

    // Calculate CRC for metrics
    journal_v2_trailer = (struct journal_v2_block_trailer *) (data_start + metric_offset_trailer);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (uint8_t *) data_start + metrics_offset, number_of_metrics * sizeof(struct journal_metric_list));
    crc32set(journal_v2_trailer->checksum, crc);
    internal_error(true, "CALCULATE CRC FOR UUIDs  %llu", (now_realtime_usec() - start_loading) / USEC_PER_MS);

    // Prepare to write checksum for the file
    j2_header.data = NULL;
    journal_v2_trailer = (struct journal_v2_block_trailer *) (data_start + trailer_offset);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (void *) &j2_header, sizeof(j2_header));
    crc32set(journal_v2_trailer->checksum, crc);

    // Write header to the file
    memcpy(data_start, &j2_header, sizeof(j2_header));

    internal_error(true, "FILE COMPLETED --------> %llu", (now_realtime_usec() - start_loading) / USEC_PER_MS);

    info("Migrated journal file %s, File size %lu", path, total_file_size);

    if (activate) {
        journalfile->journal_data = data_start;
        journalfile->journal_data_size = total_file_size;

        // HERE we need to remove old descriptors and activate the new ones
        {
            for (Index = 0; Index < number_of_metrics; Index++) {
                journal_v2_remove_active_descriptors(journalfile, uuid_list[Index].metric_info, startup);
                freez(uuid_list[Index].metric_info);
            }
            internal_error(true, "ACTIVATING NEW INDEX JNL %llu", (now_realtime_usec() - start_loading) / USEC_PER_MS);

            if (false == startup)
                uv_rwlock_wrlock(&journalfile->datafile->ctx->datafiles.rwlock);

            df_extent_delete_all_unsafe(journalfile->datafile);

            if (false == startup)
                uv_rwlock_wrunlock(&journalfile->datafile->ctx->datafiles.rwlock);
        }

        ctx->disk_space += total_file_size;
    }
    else {
        // If we failed and didnt process the entire list, free the rest
        for (Index = 0; Index < number_of_metrics; Index++)
            freez(uuid_list[Index].metric_info);
        netdata_munmap(data_start, total_file_size);
    }
    freez(uuid_list);
}

int load_journal_file(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile,
                      struct rrdengine_datafile *datafile)
{
    uv_fs_t req;
    uv_file file;
    int ret, fd, error;
    uint64_t file_size, max_id;
    char path[RRDENG_PATH_MAX];
    int should_try_migration = 0;

    // Do not try to load the latest file (always rebuild and live migrate)
    if (datafile->fileno != ctx->last_fileno) {
        if (!(should_try_migration = load_journal_file_v2(ctx, journalfile, datafile))) {
            return 0;
        }
    }

    generate_journalfilepath(datafile, path, sizeof(path));

    // If it is not the last file, open read only
    fd = open_file_direct_io(path, datafile->fileno == ctx->last_fileno ? O_RDWR : O_RDONLY, &file);
    if (fd < 0) {
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return fd;
    }

    ret = check_file_properties(file, &file_size, sizeof(struct rrdeng_df_sb));
    if (ret)
        goto error;
    file_size = ALIGN_BYTES_FLOOR(file_size);

    ret = check_journal_file_superblock(file);
    if (ret) {
        info("Invalid journal file \"%s\" ; superblock check failed.", path);
        goto error;
    }
    ctx->stats.io_read_bytes += sizeof(struct rrdeng_jf_sb);
    ++ctx->stats.io_read_requests;

    journalfile->file = file;
    journalfile->pos = file_size;

    journalfile->data = netdata_mmap(path, file_size, MAP_SHARED, 0, !(datafile->fileno == ctx->last_fileno));
    info("Loading journal file \"%s\" using %s.", path, journalfile->data?"MMAP":"uv_fs_read");

    max_id = iterate_transactions(ctx, journalfile);

    ctx->commit_log.transaction_id = MAX(ctx->commit_log.transaction_id, max_id + 1);

    info("Journal file \"%s\" loaded (size:%"PRIu64").", path, file_size);
    if (likely(journalfile->data))
        netdata_munmap(journalfile->data, file_size);

    // Don't Index the last file
    if (ctx->last_fileno == journalfile->datafile->fileno)
        return 0;

    int migrate_on_startup = 1;

    if (migrate_on_startup) {
        if (should_try_migration == 1)
            migrate_journal_file_v2(datafile, true, true);
        else
            error_report("File %s cannot be migrated to the new journal format. Index will be allocated in memory", path);
    }
    else
        info("Migration will happen as the agent is running");
    return 0;

    error:
    error = ret;
    ret = uv_fs_close(NULL, &req, file, NULL);
    if (ret < 0) {
        error("uv_fs_close(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);
    return error;
}

void after_journal_indexing(uv_work_t *req, int status)
{
    struct rrdeng_work  *work_request = req->data;
    struct rrdengine_worker_config *wc = work_request->wc;

    if (likely(status != UV_ECANCELED)) {
        internal_error(true, "Journal indexing done");
    }
    wc->running_journal_migration = 0;
    freez(work_request);
}

void start_journal_indexing(uv_work_t *req)
{
    struct rrdeng_work *work_request = req->data;
    struct rrdengine_journalfile *journalfile = work_request->journalfile;
    migrate_journal_file_v2(journalfile->datafile, true, false);
}

void init_commit_log(struct rrdengine_instance *ctx)
{
    ctx->commit_log.buf = NULL;
    ctx->commit_log.buf_pos = 0;
    ctx->commit_log.transaction_id = 1;
}
