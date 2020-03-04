// SPDX-License-Identifier: GPL-3.0-or-later
#include "../rrdengine.h"

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

void generate_metadata_log_path(struct rrdengine_metadata_log *metadatalog, char *str, size_t maxlen)
{
    (void) snprintf(str, maxlen, "%s/" METALOG_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL METALOG_EXTENSION,
                    metadatalog->ctx->dbfiles_path, metadatalog->tier, metadatalog->fileno);
}

void metadata_log_init(struct rrdengine_metadata_log *metadatalog, struct rrdengine_instance *ctx,
                       unsigned tier, unsigned fileno)
{
    assert(tier == 1);
    metadatalog->tier = tier;
    metadatalog->fileno = fileno;
    metadatalog->file = (uv_file)0;
    metadatalog->pos = 0;
    metadatalog->records.first = metadatalog->records.last = NULL; /* will be populated later? */
    metadatalog->next = NULL;
    metadatalog->ctx = ctx;
}

int close_metadata_log(struct rrdengine_metadata_log *metadatalog)
{
    struct rrdengine_instance *ctx = metadatalog->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_metadata_log_path(metadatalog, path, sizeof(path));

    ret = uv_fs_close(NULL, &req, metadatalog->file, NULL);
    if (ret < 0) {
        error("uv_fs_close(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    return ret;
}

int destroy_metadata_log(struct rrdengine_metadata_log *metadatalog)
{
    struct rrdengine_instance *ctx = metadatalog->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_metadata_log_path(metadatalog, path, sizeof(path));

    ret = uv_fs_ftruncate(NULL, &req, metadatalog->file, 0, NULL);
    if (ret < 0) {
        error("uv_fs_ftruncate(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    ret = uv_fs_close(NULL, &req, metadatalog->file, NULL);
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

//    ++ctx->stats.metadatalog_deletions;

    return ret;
}

int create_metadata_log(struct rrdengine_metadata_log *metadatalog)
{
    struct rrdengine_instance *ctx = metadatalog->ctx;
    uv_fs_t req;
    uv_file file;
    int ret, fd;
    struct rrdeng_metalog_sb *superblock;
    uv_buf_t iov;
    char path[RRDENG_PATH_MAX];

    generate_metadata_log_path(metadatalog, path, sizeof(path));
    fd = open_file_direct_io(path, O_CREAT | O_RDWR | O_TRUNC, &file);
    if (fd < 0) {
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return fd;
    }
    metadatalog->file = file;
//    ++ctx->stats.metadatalog_creations;

    ret = posix_memalign((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    if (unlikely(ret)) {
        fatal("posix_memalign:%s", strerror(ret));
    }
    (void) strncpy(superblock->magic_number, RRDENG_METALOG_MAGIC, RRDENG_MAGIC_SZ);
    (void) strncpy(superblock->version, RRDENG_METALOG_VER, RRDENG_VER_SZ);
    superblock->tier = 1;

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
        destroy_metadata_log(metadatalog);
        return ret;
    }

    metadatalog->pos = sizeof(*superblock);
    ctx->stats.io_write_bytes += sizeof(*superblock);
    ++ctx->stats.io_write_requests;

    return 0;
}

static int check_metadata_log_superblock(uv_file file)
{
    int ret;
    struct rrdeng_metalog_sb *superblock;
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

    if (strncmp(superblock->magic_number, RRDENG_METALOG_MAGIC, RRDENG_MAGIC_SZ) ||
        strncmp(superblock->version, RRDENG_METALOG_VER, RRDENG_VER_SZ)) {
        error("File has invalid superblock.");
        ret = UV_EINVAL;
    } else {
        ret = 0;
    }
    error:
    free(superblock);
    return ret;
}

int load_metadata_log(struct rrdengine_instance *ctx,struct rrdengine_metadata_log *metadatalog)
{
    uv_fs_t req;
    uv_file file;
    int ret, fd, error;
    uint64_t file_size, max_id;
    char path[RRDENG_PATH_MAX];

    generate_metadata_log_path(metadatalog, path, sizeof(path));
    fd = open_file_direct_io(path, O_RDWR, &file);
    if (fd < 0) {
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return fd;
    }
    info("Loading metadata log \"%s\".", path);

    ret = check_file_properties(file, &file_size, sizeof(struct rrdeng_metalog_sb));
    if (ret)
        goto error;
    file_size = ALIGN_BYTES_FLOOR(file_size);

    ret = check_metadata_log_superblock(file);
    if (ret)
        goto error;
    ctx->stats.io_read_bytes += sizeof(struct rrdeng_jf_sb);
    ++ctx->stats.io_read_requests;

    metadatalog->file = file;
    metadatalog->pos = file_size;

    //TODO: load state

    ctx->commit_log.transaction_id = MAX(ctx->commit_log.transaction_id, max_id + 1);

    info("Metadata log \"%s\" loaded (size:%"PRIu64").", path, file_size);
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

void init_metadata_record_log(struct rrdengine_instance *ctx)
{
    ctx->metadata_record_log.buf = NULL;
    ctx->metadata_record_log.buf_pos = 0;
    ctx->metadata_record_log.record_id = 1;
}