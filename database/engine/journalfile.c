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
    free(io_descr->buf);
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
    io_descr->completion = NULL;

    io_descr->iov = uv_buf_init((void *)io_descr->buf, size);
    ret = uv_fs_write(wc->loop, &io_descr->req, journalfile->file, &io_descr->iov, 1,
                      journalfile->pos, flush_transaction_buffer_cb);
    assert (-1 != ret);
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
    unsigned buf_pos, buf_size;

    assert(size);
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
        buf_pos = ctx->commit_log.buf_pos = 0;
        ctx->commit_log.buf_size =  buf_size;
    }
    ctx->commit_log.buf_pos += size;

    return ctx->commit_log.buf + buf_pos;
}

void generate_journalfilepath(struct rrdengine_datafile *datafile, char *str, size_t maxlen)
{
    (void) snprintf(str, maxlen, "%s/" WALFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL WALFILE_EXTENSION,
                    datafile->ctx->dbfiles_path, datafile->tier, datafile->fileno);
}

void journalfile_init(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile)
{
    journalfile->file = (uv_file)0;
    journalfile->pos = 0;
    journalfile->datafile = datafile;
}

int close_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_journalfilepath(datafile, path, sizeof(path));

    ret = uv_fs_close(NULL, &req, journalfile->file, NULL);
    if (ret < 0) {
        error("uv_fs_close(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    return ret;
}

int destroy_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_journalfilepath(datafile, path, sizeof(path));

    ret = uv_fs_ftruncate(NULL, &req, journalfile->file, 0, NULL);
    if (ret < 0) {
        error("uv_fs_ftruncate(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    ret = uv_fs_close(NULL, &req, journalfile->file, NULL);
    if (ret < 0) {
        error("uv_fs_close(%s): %s", path, uv_strerror(ret));
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
    (void) strncpy(superblock->magic_number, RRDENG_JF_MAGIC, RRDENG_MAGIC_SZ);
    (void) strncpy(superblock->version, RRDENG_JF_VER, RRDENG_VER_SZ);

    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_write(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        assert(req.result < 0);
        error("uv_fs_write: %s", uv_strerror(ret));
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
    }
    uv_fs_req_cleanup(&req);
    free(superblock);
    if (ret < 0) {
        destroy_journal_file(journalfile, datafile);
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
    assert(req.result >= 0);
    uv_fs_req_cleanup(&req);

    if (strncmp(superblock->magic_number, RRDENG_JF_MAGIC, RRDENG_MAGIC_SZ) ||
        strncmp(superblock->version, RRDENG_JF_VER, RRDENG_VER_SZ)) {
        error("File has invalid superblock.");
        ret = UV_EINVAL;
    } else {
        ret = 0;
    }
    error:
    free(superblock);
    return ret;
}

static void restore_extent_metadata(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile,
                                    void *buf, unsigned max_size)
{
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
        struct pg_cache_page_index *page_index;

        if (PAGE_METRICS != jf_metric_data->descr[i].type) {
            error("Unknown page type encountered.");
            continue;
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
            assert(NULL == *PValue); /* TODO: figure out concurrency model */
            *PValue = page_index = create_page_index(temp_id);
            page_index->prev = pg_cache->metrics_index.last_page_index;
            pg_cache->metrics_index.last_page_index = page_index;
            uv_rwlock_wrunlock(&pg_cache->metrics_index.lock);
        }

        descr = pg_cache_create_descr();
        descr->page_length = jf_metric_data->descr[i].page_length;
        descr->start_time = jf_metric_data->descr[i].start_time;
        descr->end_time = jf_metric_data->descr[i].end_time;
        descr->id = &page_index->id;
        descr->extent = extent;
        extent->pages[valid_pages++] = descr;
        pg_cache_insert(ctx, page_index, descr);
    }

    extent->number_of_pages = valid_pages;

    if (likely(valid_pages))
        df_extent_insert(extent);
    else
        freez(extent);
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
    ret = posix_memalign((void *)&buf, RRDFILE_ALIGNMENT, READAHEAD_BYTES);
    if (unlikely(ret)) {
        fatal("posix_memalign:%s", strerror(ret));
    }

    for (pos = sizeof(struct rrdeng_jf_sb) ; pos < file_size ; pos += READAHEAD_BYTES) {
        size_bytes = MIN(READAHEAD_BYTES, file_size - pos);
        iov = uv_buf_init(buf, size_bytes);
        ret = uv_fs_read(NULL, &req, file, &iov, 1, pos, NULL);
        if (ret < 0) {
            fatal("uv_fs_read: %s", uv_strerror(ret));
            /*uv_fs_req_cleanup(&req);*/
        }
        assert(req.result >= 0);
        uv_fs_req_cleanup(&req);
        ctx->stats.io_read_bytes += size_bytes;
        ++ctx->stats.io_read_requests;

        //pos_i = pos;
        //while (pos_i < pos + size_bytes) {
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
    }

    free(buf);
    return max_id;
}

int load_journal_file(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile,
                      struct rrdengine_datafile *datafile)
{
    uv_fs_t req;
    uv_file file;
    int ret, fd, error;
    uint64_t file_size, max_id;
    char path[RRDENG_PATH_MAX];

    generate_journalfilepath(datafile, path, sizeof(path));
    fd = open_file_direct_io(path, O_RDWR, &file);
    if (fd < 0) {
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return fd;
    }
    info("Loading journal file \"%s\".", path);

    ret = check_file_properties(file, &file_size, sizeof(struct rrdeng_df_sb));
    if (ret)
        goto error;
    file_size = ALIGN_BYTES_FLOOR(file_size);

    ret = check_journal_file_superblock(file);
    if (ret)
        goto error;
    ctx->stats.io_read_bytes += sizeof(struct rrdeng_jf_sb);
    ++ctx->stats.io_read_requests;

    journalfile->file = file;
    journalfile->pos = file_size;

    max_id = iterate_transactions(ctx, journalfile);

    ctx->commit_log.transaction_id = MAX(ctx->commit_log.transaction_id, max_id + 1);

    info("Journal file \"%s\" loaded (size:%"PRIu64").", path, file_size);
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

void init_commit_log(struct rrdengine_instance *ctx)
{
    ctx->commit_log.buf = NULL;
    ctx->commit_log.buf_pos = 0;
    ctx->commit_log.transaction_id = 1;
}