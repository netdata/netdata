// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

static void flush_transaction_buffer_cb(uv_fs_t* req)
{
    struct generic_io_descriptor *io_descr;

    fprintf(stderr, "%s: Journal block was written to disk.\n", __func__);
    if (req->result < 0) {
        fprintf(stderr, "%s: uv_fs_write: %s\n", __func__, uv_strerror((int)req->result));
        exit(req->result);
    }
    io_descr = req->data;

    free(io_descr->buf);
    free(io_descr);
    uv_fs_req_cleanup(req);
}

void wal_flush_transaction_buffer(struct rrdengine_worker_config* wc)
{
    int ret;
    struct generic_io_descriptor *io_descr;
    unsigned pos, size;

    if (unlikely(NULL == commit_log.buf || 0 == commit_log.buf_pos)) {
        return;
    }
    io_descr = malloc(sizeof(*io_descr));
    if (unlikely(NULL == io_descr)) {
        fprintf(stderr, "%s: malloc failed.\n", __func__);
        exit(UV_ENOMEM);
    }
    pos = commit_log.buf_pos;
    size = commit_log.buf_size;
    if (pos < size) {
        /* simulate an empty transaction to skip the rest of the block */
        *(uint8_t *) (commit_log.buf + pos) = STORE_PADDING;
    }
    io_descr->buf = commit_log.buf;
    io_descr->bytes = size;
    io_descr->pos = journalfile.pos;
    io_descr->req.data = io_descr;
    io_descr->completion = NULL;

    io_descr->iov = uv_buf_init((void *)io_descr->buf, size);
    ret = uv_fs_write(wc->loop, &io_descr->req, journalfile.file, &io_descr->iov, 1,
                      journalfile.pos, flush_transaction_buffer_cb);
    assert (-1 != ret);
    journalfile.pos += RRDENG_BLOCK_SIZE;
    commit_log.buf = NULL;
}

void * wal_get_transaction_buffer(struct rrdengine_worker_config* wc, unsigned size)
{
    int ret;
    unsigned buf_pos, buf_size;

    assert(size);
    if (commit_log.buf) {
        unsigned remaining;

        buf_pos = commit_log.buf_pos;
        buf_size = commit_log.buf_size;
        remaining = buf_size - buf_pos;
        if (size > remaining) {
            /* we need a new buffer */
            wal_flush_transaction_buffer(wc);
        }
    }
    if (NULL == commit_log.buf) {
        buf_size = ALIGN_BYTES_CEILING(size);
        ret = posix_memalign((void *)&commit_log.buf, RRDFILE_ALIGNMENT, buf_size);
        if (unlikely(ret)) {
            fprintf(stderr, "posix_memalign:%s\n", strerror(ret));
            exit(UV_ENOMEM);
        }
        buf_pos = commit_log.buf_pos = 0;
        commit_log.buf_size =  buf_size;
    }
    commit_log.buf_pos += size;

    return commit_log.buf + buf_pos;
}

static int create_journal_file(void)
{
    uv_fs_t req;
    uv_file file;
    int i, ret, fd;
    struct rrdeng_jf_sb *superblock;
    uv_buf_t iov;

    fd = uv_fs_open(NULL, &req, WALFILE, UV_FS_O_DIRECT | UV_FS_O_CREAT | UV_FS_O_RDWR | UV_FS_O_TRUNC,
                    S_IRUSR | S_IWUSR, NULL);
    if (fd < 0) {
        fprintf(stderr, "uv_fs_fsopen: %s\n", uv_strerror(fd));
        exit(fd);
    }
    assert(req.result >= 0);
    file = req.result;
    uv_fs_req_cleanup(&req);

    ret = posix_memalign((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    if (unlikely(ret)) {
        fprintf(stderr, "posix_memalign:%s\n", strerror(ret));
        return UV_ENOMEM;
    }
    (void) strncpy(superblock->magic_number, RRDENG_JF_MAGIC, RRDENG_MAGIC_SZ);
    (void) strncpy(superblock->version, RRDENG_JF_VER, RRDENG_VER_SZ);

    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_write(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        fprintf(stderr, "uv_fs_write: %s\n", uv_strerror(ret));
        exit(ret);
    }
    if (req.result < 0) {
        fprintf(stderr, "uv_fs_write: %s\n", uv_strerror((int)req.result));
        exit(req.result);
    }
    uv_fs_req_cleanup(&req);
    free(superblock);

    journalfile.file = file;
    journalfile.pos = sizeof(*superblock);
}

static int check_journal_file_superblock(uv_file file)
{
    int ret;
    struct rrdeng_jf_sb *superblock;
    uv_buf_t iov;
    uv_fs_t req;

    ret = posix_memalign((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    if (unlikely(ret)) {
        fprintf(stderr, "posix_memalign:%s\n", strerror(ret));
        return UV_ENOMEM;
    }
    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_read(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        fprintf(stderr, "uv_fs_read: %s\n", uv_strerror(ret));
        uv_fs_req_cleanup(&req);
        goto error;
    }
    assert(req.result >= 0);
    uv_fs_req_cleanup(&req);

    if (strncmp(superblock->magic_number, RRDENG_JF_MAGIC, RRDENG_MAGIC_SZ) ||
        strncmp(superblock->version, RRDENG_JF_VER, RRDENG_VER_SZ)) {
        fprintf(stderr, "File has invalid superblock.\n");
        ret = UV_EINVAL;
    } else {
        ret = 0;
    }
    error:
    free(superblock);
    return ret;
}

static void restore_extent_metadata(void *buf, unsigned max_size)
{
    unsigned i, count, payload_length, descr_size;
    struct rrdeng_page_cache_descr *descr;
    struct extent_info *extent;
    /* persistent structures */
    struct rrdeng_jf_store_metric_data *jf_metric_data;

    jf_metric_data = buf;
    count = jf_metric_data->number_of_pages;
    descr_size = sizeof(*jf_metric_data->descr) * count;
    payload_length = sizeof(*jf_metric_data) + descr_size;
    if (payload_length > max_size) {
        fprintf(stderr, "Corrupted transaction payload.\n");
        return;
    }

    extent = malloc(sizeof(*extent) + count * sizeof(extent->pages[0]));
    if (unlikely(NULL == extent)) {
        fprintf(stderr, "malloc failed.\n");
        exit(UV_ENOMEM);
    }
    extent->offset = jf_metric_data->extent_offset;
    extent->size = jf_metric_data->extent_size;
    extent->number_of_pages = count;

    for (i = 0 ; i < count ; ++i) {
        descr = malloc(sizeof(*descr));
        if (unlikely(NULL == descr)) {
            fprintf(stderr, "malloc failed.\n");
            exit(UV_ENOMEM);
        }
        descr->page = NULL;
        descr->page_length = jf_metric_data->descr[i].page_length;
        descr->start_time = jf_metric_data->descr[i].start_time;
        descr->end_time = jf_metric_data->descr[i].end_time;
        /* TODO: this malloc needs to move to a metrics index */
        descr->id = malloc(sizeof(uuid_t));
        if (unlikely(descr->id == NULL)) {
            fprintf(stderr, "malloc failed.\n");
            exit(UV_ENOMEM);
        }
        uuid_copy(*descr->id, *(uuid_t *)jf_metric_data->descr[i].uuid);
        descr->flags = 0;
        descr->refcnt = 0;
        descr->handle = NULL;
        assert(0 == uv_cond_init(&descr->cond));
        assert(0 == uv_mutex_init(&descr->mutex));
        descr->extent = extent;
        extent->pages[i] = descr;
        pg_cache_insert(descr);
    }
}

/*
 * Replays transaction by interpreting up to max_size bytes from buf.
 * Sets id to the current transaction id or to 0 if unknown.
 * Returns size of transaction record or 0 for unknown size.
 */
static unsigned replay_transaction(void *buf, uint64_t *id, unsigned max_size)
{
    unsigned count, payload_length, descr_size, size_bytes;
    int ret;
    /* persistent structures */
    struct rrdeng_df_extent_header *df_header;
    struct rrdeng_jf_transaction_header *jf_header;
    struct rrdeng_jf_store_metric_data *jf_metric_data;
    struct rrdeng_jf_transaction_trailer *jf_trailer;
    uLong crc;

    *id = 0;
    jf_header = buf;
    if (STORE_PADDING == jf_header->type) {
        fprintf(stderr, "Skipping padding.\n");
        return 0;
    }
    if (sizeof(*jf_header) > max_size) {
        fprintf(stderr, "Corrupted transaction record, skipping.\n");
        return 0;
    }
    *id = jf_header->id;
    payload_length = jf_header->payload_length;
    size_bytes = sizeof(*jf_header) + payload_length + sizeof(*jf_trailer);
    if (size_bytes > max_size) {
        fprintf(stderr, "Corrupted transaction record, skipping.\n");
        return 0;
    }
    jf_trailer = buf + sizeof(*jf_header) + payload_length;
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, buf, sizeof(*jf_header) + payload_length);
    ret = crc32cmp(jf_trailer->checksum, crc);
    fprintf(stderr, "Transaction %"PRIu64" was read from disk. CRC32 check: %s\n", *id, ret ? "FAILED" : "SUCCEEDED");
    if (unlikely(ret)) {
        return size_bytes;
    }
    switch (jf_header->type) {
    case STORE_METRIC_DATA:
        fprintf(stderr, "Replaying transaction %"PRIu64"\n", jf_header->id);
        restore_extent_metadata(buf + sizeof(*jf_header), payload_length);
        break;
    default:
        fprintf(stderr, "Unknown transaction type. Skipping record.\n");
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
static uint64_t iterate_transactions(uv_file file, uint64_t file_size, uint64_t data_file_size)
{
    int i, ret, fd;
    uint64_t pos, pos_i, max_id, id;
    unsigned size_bytes;
    void *buf;
    uv_buf_t iov;
    uv_fs_t req;

    max_id = 1;
    ret = posix_memalign((void *)&buf, RRDFILE_ALIGNMENT, READAHEAD_BYTES);
    if (unlikely(ret)) {
        fprintf(stderr, "posix_memalign:%s\n", strerror(ret));
        exit(UV_ENOMEM);
    }

    for (pos = sizeof(struct rrdeng_jf_sb) ; pos < file_size ; pos += READAHEAD_BYTES) {
        size_bytes = MIN(READAHEAD_BYTES, file_size - pos);
        iov = uv_buf_init(buf, size_bytes);
        ret = uv_fs_read(NULL, &req, file, &iov, 1, pos, NULL);
        if (ret < 0) {
            fprintf(stderr, "uv_fs_read: %s\n", uv_strerror(ret));
            uv_fs_req_cleanup(&req);
            exit(ret);
        }
        assert(req.result >= 0);
        uv_fs_req_cleanup(&req);

        //pos_i = pos;
        //while (pos_i < pos + size_bytes) {
        for (pos_i = 0 ; pos_i < size_bytes ; ) {
            unsigned max_size;

            max_size = pos + size_bytes - pos_i;
            ret = replay_transaction(buf + pos_i, &id, max_size);
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

static int load_journal_file(uint64_t data_file_size)
{
    uv_fs_t req;
    uv_file file;
    int ret, fd;
    uint64_t file_size, max_id;

    fd = uv_fs_open(NULL, &req, WALFILE, UV_FS_O_DIRECT | UV_FS_O_RDWR, S_IRUSR | S_IWUSR, NULL);
    if (fd < 0) {
        if (UV_ENOENT != fd) {
            fprintf(stderr, "uv_fs_fsopen: %s\n", uv_strerror(fd));
            /* File exists but something went wrong */
        }
        uv_fs_req_cleanup(&req);
        return fd;
    }
    assert(req.result >= 0);
    file = req.result;
    uv_fs_req_cleanup(&req);
    fprintf(stderr, "Loading journal file \"%s\".\n", WALFILE);

    ret = check_file_properties(file, &file_size, sizeof(struct rrdeng_df_sb));
    if (ret)
        goto error;
    file_size = ALIGN_BYTES_FLOOR(file_size);

    ret = check_journal_file_superblock(file);
    if (ret)
        goto error;

    max_id = iterate_transactions(file, file_size, data_file_size);

    journalfile.file = file;
    journalfile.pos = file_size;
    commit_log.transaction_id = max_id + 1;

    fprintf(stderr, "Journal file \"%s\" loaded (size:%"PRIu64").\n", WALFILE, file_size);
    return 0;

    error:
    (void) uv_fs_close(NULL, &req, file, NULL);
    uv_fs_req_cleanup(&req);
    return ret;
}

static void init_commit_log(void)
{
    commit_log.buf = NULL;
    commit_log.buf_pos = 0;
    commit_log.transaction_id = 1;
}

/* Page cache must already be initialized. */
int init_journal_files(uint64_t data_file_size)
{
    int ret;

    init_commit_log();
    ret = load_journal_file(data_file_size);
    if (UV_ENOENT == ret) {
        fprintf(stderr, "Journal file \"%s\" does not exist, creating.\n", WALFILE);
        ret = create_journal_file();
    }

    return ret;
}