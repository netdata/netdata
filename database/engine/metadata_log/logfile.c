// SPDX-License-Identifier: GPL-3.0-or-later
#include "metadatalog.h"

void mlf_record_insert(struct metalog_record_info *record)
{
    struct metadata_logfile *metalogfile = record->metalogfile;

    if (likely(NULL != metalogfile->records.last)) {
        metalogfile->records.last->next = record;
    }
    if (unlikely(NULL == metalogfile->records.first)) {
        metalogfile->records.first = record;
    }
    metalogfile->records.last = record;
}

static void flush_records_buffer_cb(uv_fs_t* req)
{
    struct generic_io_descriptor *io_descr = req->data;
    struct metalog_worker_config *wc = req->loop->data;
    struct metalog_instance *ctx = wc->ctx;

    debug(D_RRDENGINE, "%s: Metadata log file block was written to disk.", __func__);
    if (req->result < 0) {
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        error("%s: uv_fs_write: %s", __func__, uv_strerror((int)req->result));
    } else {
        debug(D_RRDENGINE, "%s: Metadata log file block was written to disk.", __func__);
    }

    uv_fs_req_cleanup(req);
    free(io_descr->buf);
    freez(io_descr);
}

/* Careful to always call this before creating a new metadata log file */
void mlf_flush_records_buffer(struct metalog_worker_config *wc)
{
    struct metalog_instance *ctx = wc->ctx;
    int ret;
    struct generic_io_descriptor *io_descr;
    unsigned pos, size;
    struct metadata_logfile *metalogfile;

    if (unlikely(NULL == ctx->records_log.buf || 0 == ctx->records_log.buf_pos)) {
        return;
    }
    /* care with outstanding records when switching metadata log files */
    metalogfile = ctx->metadata_logfiles.last;

    io_descr = mallocz(sizeof(*io_descr));
    pos = ctx->records_log.buf_pos;
    size = pos; /* no need to align the I/O when doing buffered writes */
    io_descr->buf = ctx->records_log.buf;
    io_descr->bytes = size;
    io_descr->pos = metalogfile->pos;
    io_descr->req.data = io_descr;
    io_descr->completion = NULL;

    io_descr->iov = uv_buf_init((void *)io_descr->buf, size);
    ret = uv_fs_write(wc->loop, &io_descr->req, metalogfile->file, &io_descr->iov, 1,
                      metalogfile->pos, flush_records_buffer_cb);
    assert (-1 != ret);
    metalogfile->pos += size;
    ctx->disk_space += size;
    ctx->records_log.buf = NULL;
    ctx->stats.io_write_bytes += size;
    ++ctx->stats.io_write_requests;
}

void *mlf_get_records_buffer(struct metalog_worker_config *wc, unsigned size)
{
    struct metalog_instance *ctx = wc->ctx;
    int ret;
    unsigned buf_pos, buf_size;

    assert(size);
    if (ctx->records_log.buf) {
        unsigned remaining;

        buf_pos = ctx->records_log.buf_pos;
        buf_size = ctx->records_log.buf_size;
        remaining = buf_size - buf_pos;
        if (size > remaining) {
            /* we need a new buffer */
            mlf_flush_records_buffer(wc);
        }
    }
    if (NULL == ctx->records_log.buf) {
        buf_size = ALIGN_BYTES_CEILING(size);
        ret = posix_memalign((void *)&ctx->records_log.buf, RRDFILE_ALIGNMENT, buf_size);
        if (unlikely(ret)) {
            fatal("posix_memalign:%s", strerror(ret));
        }
        buf_pos = ctx->records_log.buf_pos = 0;
        ctx->records_log.buf_size =  buf_size;
    }
    ctx->records_log.buf_pos += size;

    return ctx->records_log.buf + buf_pos;
}


void metadata_logfile_list_insert(struct metalog_instance *ctx, struct metadata_logfile *metalogfile)
{
    if (likely(NULL != ctx->metadata_logfiles.last)) {
        ctx->metadata_logfiles.last->next = metalogfile;
    }
    if (unlikely(NULL == ctx->metadata_logfiles.first)) {
        ctx->metadata_logfiles.first = metalogfile;
    }
    ctx->metadata_logfiles.last = metalogfile;
}

void metadata_logfile_list_delete(struct metalog_instance *ctx, struct metadata_logfile *metalogfile)
{
    struct metadata_logfile *next;

    next = metalogfile->next;
    assert((NULL != next) && (ctx->metadata_logfiles.first == metalogfile) &&
           (ctx->metadata_logfiles.last != metalogfile));
    ctx->metadata_logfiles.first = next;
}

void generate_metadata_logfile_path(struct metadata_logfile *metalogfile, char *str, size_t maxlen)
{
    (void) snprintf(str, maxlen, "%s/" METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL METALOG_EXTENSION,
                    metalogfile->ctx->rrdeng_ctx->dbfiles_path, metalogfile->tier, metalogfile->fileno);
}

void metadata_logfile_init(struct metadata_logfile *metalogfile, struct metalog_instance *ctx, unsigned tier,
                           unsigned fileno)
{
    assert(tier == 1);
    metalogfile->tier = tier;
    metalogfile->fileno = fileno;
    metalogfile->file = (uv_file)0;
    metalogfile->pos = 0;
    metalogfile->records.first = metalogfile->records.last = NULL; /* will be populated later? */
    metalogfile->next = NULL;
    metalogfile->ctx = ctx;
}

int close_metadata_logfile(struct metadata_logfile *metalogfile)
{
    struct metalog_instance *ctx = metalogfile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_metadata_logfile_path(metalogfile, path, sizeof(path));

    ret = uv_fs_close(NULL, &req, metalogfile->file, NULL);
    if (ret < 0) {
        error("uv_fs_close(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    return ret;
}

int destroy_metadata_logfile(struct metadata_logfile *metalogfile)
{
    struct metalog_instance *ctx = metalogfile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_metadata_logfile_path(metalogfile, path, sizeof(path));

    ret = uv_fs_ftruncate(NULL, &req, metalogfile->file, 0, NULL);
    if (ret < 0) {
        error("uv_fs_ftruncate(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    ret = uv_fs_close(NULL, &req, metalogfile->file, NULL);
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

//    ++ctx->stats.metadata_logfile_deletions;

    return ret;
}

int create_metadata_logfile(struct metadata_logfile *metalogfile)
{
    struct metalog_instance *ctx = metalogfile->ctx;
    uv_fs_t req;
    uv_file file;
    int ret, fd;
    struct rrdeng_metalog_sb *superblock;
    uv_buf_t iov;
    char path[RRDENG_PATH_MAX];

    generate_metadata_logfile_path(metalogfile, path, sizeof(path));
    fd = open_file_buffered_io(path, O_CREAT | O_RDWR | O_TRUNC, &file);
    if (fd < 0) {
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return fd;
    }
    metalogfile->file = file;
//    ++ctx->stats.metadata_logfile_creations;

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
        destroy_metadata_logfile(metalogfile);
        return ret;
    }

    metalogfile->pos = sizeof(*superblock);
    ctx->stats.io_write_bytes += sizeof(*superblock);
    ++ctx->stats.io_write_requests;

    return 0;
}

static int check_metadata_logfile_superblock(uv_file file)
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

int load_metadata_logfile(struct metalog_instance *ctx, struct metadata_logfile *metalogfile)
{
    uv_fs_t req;
    uv_file file;
    int ret, fd, error;
    uint64_t file_size, max_id;
    char path[RRDENG_PATH_MAX];

    generate_metadata_logfile_path(metalogfile, path, sizeof(path));
    fd = open_file_buffered_io(path, O_RDWR, &file);
    if (fd < 0) {
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return fd;
    }
    info("Loading metadata log \"%s\".", path);

    ret = check_file_properties(file, &file_size, sizeof(struct rrdeng_metalog_sb));
    if (ret)
        goto error;

    ret = check_metadata_logfile_superblock(file);
    if (ret)
        goto error;
    ctx->stats.io_read_bytes += sizeof(struct rrdeng_jf_sb);
    ++ctx->stats.io_read_requests;

    metalogfile->file = file;
    metalogfile->pos = file_size;

    //TODO: load state
    //ctx->records_log.record_id = MAX(ctx->records_log.record_id, max_id + 1);

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

void init_metadata_record_log(struct metalog_instance *ctx)
{
    ctx->records_log.buf = NULL;
    ctx->records_log.buf_pos = 0;
    ctx->records_log.record_id = 1;
}

static int scan_metalog_files_cmp(const void *a, const void *b)
{
    struct metadata_logfile *file1, *file2;
    char path1[RRDENG_PATH_MAX], path2[RRDENG_PATH_MAX];

    file1 = *(struct metadata_logfile **)a;
    file2 = *(struct metadata_logfile **)b;
    generate_metadata_logfile_path(file1, path1, sizeof(path1));
    generate_metadata_logfile_path(file2, path2, sizeof(path2));
    return strcmp(path1, path2);
}

/* Returns number of metadata logfiles that were loaded or < 0 on error */
static int scan_metalog_files(struct metalog_instance *ctx)
{
    int ret;
    unsigned tier, no, matched_files, i,failed_to_load;
    static uv_fs_t req;
    uv_dirent_t dent;
    struct metadata_logfile **metalogfiles, *metalogfile;
    char *dbfiles_path = ctx->rrdeng_ctx->dbfiles_path;

    ret = uv_fs_scandir(NULL, &req, dbfiles_path, 0, NULL);
    if (ret < 0) {
        assert(req.result < 0);
        uv_fs_req_cleanup(&req);
        error("uv_fs_scandir(%s): %s", dbfiles_path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return ret;
    }
    info("Found %d files in path %s", ret, dbfiles_path);

    metalogfiles = callocz(MIN(ret, MAX_DATAFILES), sizeof(*metalogfiles));
    for (matched_files = 0 ; UV_EOF != uv_fs_scandir_next(&req, &dent) && matched_files < MAX_DATAFILES ; ) {
        info("Scanning file \"%s/%s\"", dbfiles_path, dent.name);
        ret = sscanf(dent.name, METALOG_PREFIX METALOG_FILE_NUMBER_SCAN_TMPL METALOG_EXTENSION, &tier, &no);
        if (2 == ret) {
            info("Matched file \"%s/%s\"", dbfiles_path, dent.name);
            metalogfile = mallocz(sizeof(*metalogfile));
            metadata_logfile_init(metalogfile, ctx, tier, no);
            metalogfiles[matched_files++] = metalogfile;
        }
    }
    uv_fs_req_cleanup(&req);

    if (0 == matched_files) {
        freez(metalogfiles);
        return 0;
    }
    if (matched_files == MAX_DATAFILES) {
        error("Warning: hit maximum database engine file limit of %d files", MAX_DATAFILES);
    }
    qsort(metalogfiles, matched_files, sizeof(*metalogfiles), scan_metalog_files_cmp);
    ctx->last_fileno = metalogfiles[matched_files - 1]->fileno;

    for (failed_to_load = 0, i = 0 ; i < matched_files ; ++i) {
        metalogfile = metalogfiles[i];
        ret = load_metadata_logfile(ctx, metalogfile);
        if (0 != ret) {
            freez(metalogfile);
            ++failed_to_load;
            break;
        }
        metadata_logfile_list_insert(ctx, metalogfile);
        ctx->disk_space += metalogfile->pos + metalogfile->pos;
    }
    freez(metalogfiles);
    if (failed_to_load) {
        error("%u metadata log files failed to load.", failed_to_load);
        finalize_metalog_files(ctx);
        return UV_EIO;
    }

    return matched_files;
}

/* Creates a datafile and a journalfile pair */
int add_new_metadata_logfile(struct metalog_instance *ctx, unsigned tier, unsigned fileno)
{
    struct metadata_logfile *metalogfile;
    int ret;
    char path[RRDENG_PATH_MAX];

    info("Creating new metadata log file in path %s", ctx->rrdeng_ctx->dbfiles_path);
    metalogfile = mallocz(sizeof(*metalogfile));
    metadata_logfile_init(metalogfile, ctx, tier, fileno);
    ret = create_metadata_logfile(metalogfile);
    if (!ret) {
        generate_metadata_logfile_path(metalogfile, path, sizeof(path));
        info("Created metadata log file \"%s\".", path);
    } else {
        freez(metalogfile);
        return ret;
    }
    metadata_logfile_list_insert(ctx, metalogfile);
    ctx->disk_space += metalogfile->pos;

    return 0;
}

/* Return 0 on success. */
int init_metalog_files(struct metalog_instance *ctx)
{
    int ret;
    char *dbfiles_path = ctx->rrdeng_ctx->dbfiles_path;

    ret = scan_metalog_files(ctx);
    if (ret < 0) {
        error("Failed to scan path \"%s\".", dbfiles_path);
        return ret;
    } else if (0 == ret) {
        info("Metadata log files not found, creating in path \"%s\".", dbfiles_path);
        ret = add_new_metadata_logfile(ctx, 1, 1);
        if (ret) {
            error("Failed to create metadata log file in path \"%s\".", dbfiles_path);
            return ret;
        }
        ctx->last_fileno = 1;
    }

    return 0;
}

void finalize_metalog_files(struct metalog_instance *ctx)
{
    struct metadata_logfile *metalogfile, *next_metalogfile;
    struct metalog_record_info *record, *next_record;

    for (metalogfile = ctx->metadata_logfiles.first ; metalogfile != NULL ; metalogfile = next_metalogfile) {
        next_metalogfile = metalogfile->next;

        for (record = metalogfile->records.first ; record != NULL ; record = next_record) {
            next_record = record->next;
            freez(record);
        }
        close_metadata_logfile(metalogfile);
        freez(metalogfile);
    }
}