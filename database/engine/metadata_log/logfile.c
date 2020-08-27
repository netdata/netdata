// SPDX-License-Identifier: GPL-3.0-or-later
#include "metadatalog.h"
#include "metalogpluginsd.h"

static void mlf_record_block_insert(struct metadata_logfile *metalogfile, struct metalog_record_block *record_block)
{

    if (likely(NULL != metalogfile->records.last)) {
        metalogfile->records.last->next = record_block;
    }
    if (unlikely(NULL == metalogfile->records.first)) {
        metalogfile->records.first = record_block;
    }
    metalogfile->records.last = record_block;
}

void mlf_record_insert(struct metadata_logfile *metalogfile, struct metalog_record *record)
{
    struct metalog_record_block *record_block;
    struct metalog_instance *ctx = metalogfile->ctx;

    record_block = metalogfile->records.last;
    if (likely(NULL != record_block && record_block->records_nr < MAX_METALOG_RECORDS_PER_BLOCK)) {
        record_block->record_array[record_block->records_nr++] = *record;
    } else { /* Create new record block, the last one filled up */
        record_block = mallocz(sizeof(*record_block));
        record_block->records_nr = 1;
        record_block->record_array[0] = *record;
        record_block->next = NULL;

        mlf_record_block_insert(metalogfile, record_block);
    }
    rrd_atomic_fetch_add(&ctx->records_nr, 1);
}

struct metalog_record *mlf_record_get_first(struct metadata_logfile *metalogfile)
{
    struct metalog_records *records = &metalogfile->records;
    struct metalog_record_block *record_block = metalogfile->records.first;

    records->iterator.current = record_block;
    records->iterator.record_i = 0;

    if (unlikely(NULL == record_block || !record_block->records_nr)) {
        error("Cannot iterate empty metadata log file %u-%u.", metalogfile->starting_fileno, metalogfile->fileno);
        return NULL;
    }

    return &record_block->record_array[0];
}

/* Must have called mlf_record_get_first before calling this function. */
struct metalog_record *mlf_record_get_next(struct metadata_logfile *metalogfile)
{
    struct metalog_records *records = &metalogfile->records;
    struct metalog_record_block *record_block = records->iterator.current;

    if (unlikely(NULL == record_block)) {
        return NULL;
    }
    if (++records->iterator.record_i >= record_block->records_nr) {
        record_block = record_block->next;
        if (unlikely(NULL == record_block || !record_block->records_nr)) {
            return NULL;
        }
        records->iterator.current = record_block;
        records->iterator.record_i = 0;
        return &record_block->record_array[0];
    }
    return &record_block->record_array[records->iterator.record_i];
}

static void flush_records_buffer_cb(uv_fs_t* req)
{
    struct generic_io_descriptor *io_descr = req->data;
    struct metalog_worker_config *wc = req->loop->data;
    struct metalog_instance *ctx = wc->ctx;

    debug(D_METADATALOG, "%s: Metadata log file block was written to disk.", __func__);
    if (req->result < 0) {
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        error("%s: uv_fs_write: %s", __func__, uv_strerror((int)req->result));
    } else {
        debug(D_METADATALOG, "%s: Metadata log file block was written to disk.", __func__);
    }

    uv_fs_req_cleanup(req);
    free(io_descr->buf);
    freez(io_descr);
}

/* Careful to always call this before creating a new metadata log file to finish writing the old one */
void mlf_flush_records_buffer(struct metalog_worker_config *wc, struct metadata_record_commit_log *records_log,
                              struct metadata_logfile_list *metadata_logfiles)
{
    struct metalog_instance *ctx = wc->ctx;
    int ret;
    struct generic_io_descriptor *io_descr;
    unsigned pos, size;
    struct metadata_logfile *metalogfile;

    if (unlikely(NULL == records_log->buf || 0 == records_log->buf_pos)) {
        return;
    }
    /* care with outstanding records when switching metadata log files */
    metalogfile = metadata_logfiles->last;

    io_descr = mallocz(sizeof(*io_descr));
    pos = records_log->buf_pos;
    size = pos; /* no need to align the I/O when doing buffered writes */
    io_descr->buf = records_log->buf;
    io_descr->bytes = size;
    io_descr->pos = metalogfile->pos;
    io_descr->req.data = io_descr;
    io_descr->completion = NULL;

    io_descr->iov = uv_buf_init((void *)io_descr->buf, size);
    ret = uv_fs_write(wc->loop, &io_descr->req, metalogfile->file, &io_descr->iov, 1,
                      metalogfile->pos, flush_records_buffer_cb);
    fatal_assert(-1 != ret);
    metalogfile->pos += size;
    rrd_atomic_fetch_add(&ctx->disk_space, size);
    records_log->buf = NULL;
    ctx->stats.io_write_bytes += size;
    ++ctx->stats.io_write_requests;
}

void *mlf_get_records_buffer(struct metalog_worker_config *wc, struct metadata_record_commit_log *records_log,
                             struct metadata_logfile_list *metadata_logfiles, unsigned size)
{
    int ret;
    unsigned buf_pos = 0, buf_size;

    fatal_assert(size);
    if (records_log->buf) {
        unsigned remaining;

        buf_pos = records_log->buf_pos;
        buf_size = records_log->buf_size;
        remaining = buf_size - buf_pos;
        if (size > remaining) {
            /* we need a new buffer */
            mlf_flush_records_buffer(wc, records_log, metadata_logfiles);
        }
    }
    if (NULL == records_log->buf) {
        buf_size = ALIGN_BYTES_CEILING(size);
        ret = posix_memalign((void *)&records_log->buf, RRDFILE_ALIGNMENT, buf_size);
        if (unlikely(ret)) {
            fatal("posix_memalign:%s", strerror(ret));
        }
        buf_pos = records_log->buf_pos = 0;
        records_log->buf_size =  buf_size;
    }
    records_log->buf_pos += size;

    return records_log->buf + buf_pos;
}


void metadata_logfile_list_insert(struct metadata_logfile_list *metadata_logfiles, struct metadata_logfile *metalogfile)
{
    if (likely(NULL != metadata_logfiles->last)) {
        metadata_logfiles->last->next = metalogfile;
    }
    if (unlikely(NULL == metadata_logfiles->first)) {
        metadata_logfiles->first = metalogfile;
    }
    metadata_logfiles->last = metalogfile;
}

void metadata_logfile_list_delete(struct metadata_logfile_list *metadata_logfiles, struct metadata_logfile *metalogfile)
{
    struct metadata_logfile *next;

    next = metalogfile->next;
    fatal_assert((NULL != next) && (metadata_logfiles->first == metalogfile) &&
           (metadata_logfiles->last != metalogfile));
    metadata_logfiles->first = next;
}

void generate_metadata_logfile_path(struct metadata_logfile *metalogfile, char *str, size_t maxlen)
{
    (void) snprintf(str, maxlen, "%s/" METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL METALOG_EXTENSION,
                    metalogfile->ctx->rrdeng_ctx->dbfiles_path, metalogfile->starting_fileno, metalogfile->fileno);
}

void metadata_logfile_init(struct metadata_logfile *metalogfile, struct metalog_instance *ctx, unsigned starting_fileno,
                           unsigned fileno)
{
    metalogfile->starting_fileno = starting_fileno;
    metalogfile->fileno = fileno;
    metalogfile->file = (uv_file)0;
    metalogfile->pos = 0;
    metalogfile->records.first = metalogfile->records.last = NULL;
    metalogfile->next = NULL;
    metalogfile->ctx = ctx;
}

int rename_metadata_logfile(struct metadata_logfile *metalogfile, unsigned new_starting_fileno, unsigned new_fileno)
{
    struct metalog_instance *ctx = metalogfile->ctx;
    uv_fs_t req;
    int ret;
    char oldpath[RRDENG_PATH_MAX], newpath[RRDENG_PATH_MAX];
    unsigned backup_starting_fileno, backup_fileno;

    backup_starting_fileno = metalogfile->starting_fileno;
    backup_fileno = metalogfile->fileno;
    generate_metadata_logfile_path(metalogfile, oldpath, sizeof(oldpath));
    metalogfile->starting_fileno = new_starting_fileno;
    metalogfile->fileno = new_fileno;
    generate_metadata_logfile_path(metalogfile, newpath, sizeof(newpath));

    info("Renaming metadata log file \"%s\" to \"%s\".", oldpath, newpath);
    ret = uv_fs_rename(NULL, &req, oldpath, newpath, NULL);
    if (ret < 0) {
        error("uv_fs_rename(%s): %s", oldpath, uv_strerror(ret));
        ++ctx->stats.fs_errors; /* this is racy, may miss some errors */
        rrd_stat_atomic_add(&global_fs_errors, 1);
        /* restore previous values */
        metalogfile->starting_fileno = backup_starting_fileno;
        metalogfile->fileno = backup_fileno;
    }
    uv_fs_req_cleanup(&req);

    return ret;
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

int fsync_metadata_logfile(struct metadata_logfile *metalogfile)
{
    struct metalog_instance *ctx = metalogfile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_metadata_logfile_path(metalogfile, path, sizeof(path));

    ret = uv_fs_fsync(NULL, &req, metalogfile->file, NULL);
    if (ret < 0) {
        error("uv_fs_close(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    return ret;
}

int unlink_metadata_logfile(struct metadata_logfile *metalogfile)
{
    struct metalog_instance *ctx = metalogfile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_metadata_logfile_path(metalogfile, path, sizeof(path));

    ret = uv_fs_unlink(NULL, &req, path, NULL);
    if (ret < 0) {
        error("uv_fs_fsunlink(%s): %s", path, uv_strerror(ret));
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
    memset(superblock, 0, sizeof(*superblock));
    (void) strncpy(superblock->magic_number, RRDENG_METALOG_MAGIC, RRDENG_MAGIC_SZ);
    superblock->version = RRDENG_METALOG_VER;

    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_write(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        fatal_assert(req.result < 0);
        error("uv_fs_write: %s", uv_strerror(ret));
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    ret = uv_fs_fsync(NULL, &req, metalogfile->file, NULL);
    if (ret < 0) {
        error("uv_fs_close(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
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
    fatal_assert(req.result >= 0);
    uv_fs_req_cleanup(&req);

    if (strncmp(superblock->magic_number, RRDENG_METALOG_MAGIC, RRDENG_MAGIC_SZ)) {
        error("File has invalid superblock.");
        ret = UV_EINVAL;
    } else {
        ret = 0;
    }
    if (superblock->version > RRDENG_METALOG_VER) {
        error("File has unknown version %"PRIu16". Compatibility is not guaranteed.", superblock->version);
    }
error:
    free(superblock);
    return ret;
}

void replay_record(struct metadata_logfile *metalogfile, struct rrdeng_metalog_record_header *header, void *payload)
{
    struct metalog_instance *ctx = metalogfile->ctx;
    char *line, *nextline, *record_end;
    int ret;

    debug(D_METADATALOG, "RECORD contents: %.*s", (int)header->payload_length, (char *)payload);
    record_end = (char *)payload + header->payload_length - 1;
    *record_end = '\0';

    for (line = payload ; line ; line = nextline) {
        nextline = strchr(line, '\n');
        if (nextline) {
            *nextline++ = '\0';
        }
        ret = parser_action(ctx->metalog_parser_object->parser, line);
        debug(D_METADATALOG, "parser_action ret:%d", ret);
        if (ret)
            return; /* skip record due to error */
    };
}

/* This function only works with buffered I/O */
static inline int metalogfile_read(struct metadata_logfile *metalogfile, void *buf, size_t len, uint64_t offset)
{
    struct metalog_instance *ctx;
    uv_file file;
    uv_buf_t iov;
    uv_fs_t req;
    int ret;

    ctx = metalogfile->ctx;
    file = metalogfile->file;
    iov = uv_buf_init(buf, len);
    ret = uv_fs_read(NULL, &req, file, &iov, 1, offset, NULL);
    if (unlikely(ret < 0 && ret != req.result)) {
        fatal("uv_fs_read: %s", uv_strerror(ret));
    }
    if (req.result < 0) {
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        error("%s: uv_fs_read - %s - record at offset %"PRIu64"(%u) in metadata logfile %u-%u.", __func__,
              uv_strerror((int)req.result), offset, (unsigned)len, metalogfile->starting_fileno, metalogfile->fileno);
    }
    uv_fs_req_cleanup(&req);
    ctx->stats.io_read_bytes += len;
    ++ctx->stats.io_read_requests;

    return ret;
}

/* Return 0 on success */
static int metadata_record_integrity_check(void *record)
{
    int ret;
    uint32_t data_size;
    struct rrdeng_metalog_record_header *header;
    struct rrdeng_metalog_record_trailer *trailer;
    uLong crc;

    header = record;
    data_size = header->header_length + header->payload_length;
    trailer = record + data_size;

    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, record, data_size);
    ret = crc32cmp(trailer->checksum, crc);

    return ret;
}

#define MAX_READ_BYTES (RRDENG_BLOCK_SIZE * 32) /* no record should be over 128KiB in this version */

/*
 * Iterates metadata log file records and creates database objects (host/chart/dimension)
 */
static void iterate_records(struct metadata_logfile *metalogfile)
{
    uint32_t file_size, pos, bytes_remaining, record_size;
    void *buf;
    struct rrdeng_metalog_record_header *header;
    struct metalog_instance *ctx = metalogfile->ctx;
    struct metalog_pluginsd_state *state = ctx->metalog_parser_object->private;
    const size_t min_header_size = offsetof(struct rrdeng_metalog_record_header, header_length) +
                                   sizeof(header->header_length);

    file_size = metalogfile->pos;
    state->metalogfile = metalogfile;

    buf = mallocz(MAX_READ_BYTES);

    for (pos = sizeof(struct rrdeng_metalog_sb) ; pos < file_size ; pos += record_size) {
        bytes_remaining = file_size - pos;
        if (bytes_remaining < min_header_size) {
            error("%s: unexpected end of file in metadata logfile %u-%u.", __func__, metalogfile->starting_fileno,
                  metalogfile->fileno);
            break;
        }
        if (metalogfile_read(metalogfile, buf, min_header_size, pos) < 0)
            break;
        header = (struct rrdeng_metalog_record_header *)buf;
        if (METALOG_STORE_PADDING == header->type) {
            info("%s: Skipping padding in metadata logfile %u-%u.", __func__, metalogfile->starting_fileno,
                 metalogfile->fileno);
            record_size = ALIGN_BYTES_FLOOR(pos + RRDENG_BLOCK_SIZE) - pos;
            continue;
        }
        if (metalogfile_read(metalogfile, buf + min_header_size, sizeof(*header) - min_header_size,
                             pos + min_header_size) < 0)
            break;
        record_size = header->header_length + header->payload_length + sizeof(struct rrdeng_metalog_record_trailer);
        if (header->header_length < min_header_size || record_size > bytes_remaining) {
            error("%s: Corrupted record in metadata logfile %u-%u.", __func__, metalogfile->starting_fileno,
                  metalogfile->fileno);
            break;
        }
        if (record_size > MAX_READ_BYTES) {
            error("%s: Record is too long (%u bytes) in metadata logfile %u-%u.", __func__, record_size,
                  metalogfile->starting_fileno, metalogfile->fileno);
            continue;
        }
        if (metalogfile_read(metalogfile, buf + sizeof(*header), record_size - sizeof(*header),
                             pos + sizeof(*header)) < 0)
            break;
        if (metadata_record_integrity_check(buf)) {
            error("%s: Record at offset %"PRIu32" was read from disk. CRC32 check: FAILED", __func__, pos);
            continue;
        }
        debug(D_METADATALOG, "%s: Record at offset %"PRIu32" was read from disk. CRC32 check: SUCCEEDED", __func__,
              pos);

        replay_record(metalogfile, header, buf + header->header_length);
    }

    freez(buf);
}

int load_metadata_logfile(struct metalog_instance *ctx, struct metadata_logfile *metalogfile)
{
    uv_fs_t req;
    uv_file file;
    int ret, fd, error;
    uint64_t file_size;
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

    iterate_records(metalogfile);

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

void init_metadata_record_log(struct metadata_record_commit_log *records_log)
{
    records_log->buf = NULL;
    records_log->buf_pos = 0;
    records_log->record_id = 1;
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
    unsigned starting_no, no, matched_files, i, failed_to_load;
    static uv_fs_t req;
    uv_dirent_t dent;
    struct metadata_logfile **metalogfiles, *metalogfile;
    char *dbfiles_path = ctx->rrdeng_ctx->dbfiles_path;

    ret = uv_fs_scandir(NULL, &req, dbfiles_path, 0, NULL);
    if (ret < 0) {
        fatal_assert(req.result < 0);
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
        ret = sscanf(dent.name, METALOG_PREFIX METALOG_FILE_NUMBER_SCAN_TMPL METALOG_EXTENSION, &starting_no, &no);
        if (2 == ret) {
            info("Matched file \"%s/%s\"", dbfiles_path, dent.name);
            metalogfile = mallocz(sizeof(*metalogfile));
            metadata_logfile_init(metalogfile, ctx, starting_no, no);
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
    ret = compaction_failure_recovery(ctx, metalogfiles, &matched_files);
    if (ret) { /* If the files are corrupted fail */
        for (i = 0 ; i < matched_files ; ++i) {
            freez(metalogfiles[i]);
        }
        freez(metalogfiles);
        return UV_EINVAL;
    }
    ctx->last_fileno = metalogfiles[matched_files - 1]->fileno;

    struct plugind cd = {
        .enabled = 1,
        .update_every = 0,
        .pid = 0,
        .serial_failures = 0,
        .successful_collections = 0,
        .obsolete = 0,
        .started_t = INVALID_TIME,
        .next = NULL,
        .version = 0,
    };

    struct metalog_pluginsd_state metalog_parser_state;
    metalog_pluginsd_state_init(&metalog_parser_state, ctx);

    PARSER_USER_OBJECT metalog_parser_object;
    metalog_parser_object.enabled = cd.enabled;
    metalog_parser_object.host = ctx->rrdeng_ctx->host;
    metalog_parser_object.cd = &cd;
    metalog_parser_object.trust_durations = 0;
    metalog_parser_object.private = &metalog_parser_state;

    PARSER *parser = parser_init(metalog_parser_object.host, &metalog_parser_object, NULL, PARSER_INPUT_SPLIT);
    if (unlikely(!parser)) {
        error("Failed to initialize metadata log parser.");
        failed_to_load = matched_files;
        goto after_failed_to_parse;
    }
    parser_add_keyword(parser, PLUGINSD_KEYWORD_HOST, metalog_pluginsd_host);
    parser_add_keyword(parser, PLUGINSD_KEYWORD_GUID, pluginsd_guid);
    parser_add_keyword(parser, PLUGINSD_KEYWORD_CONTEXT, pluginsd_context);
    parser_add_keyword(parser, PLUGINSD_KEYWORD_TOMBSTONE, pluginsd_tombstone);
    parser->plugins_action->dimension_action = &metalog_pluginsd_dimension_action;
    parser->plugins_action->chart_action     = &metalog_pluginsd_chart_action;
    parser->plugins_action->guid_action      = &metalog_pluginsd_guid_action;
    parser->plugins_action->context_action   = &metalog_pluginsd_context_action;
    parser->plugins_action->tombstone_action = &metalog_pluginsd_tombstone_action;
    parser->plugins_action->host_action      = &metalog_pluginsd_host_action;


    metalog_parser_object.parser = parser;
    ctx->metalog_parser_object = &metalog_parser_object;

    for (failed_to_load = 0, i = 0 ; i < matched_files ; ++i) {
        metalogfile = metalogfiles[i];
        ret = load_metadata_logfile(ctx, metalogfile);
        if (0 != ret) {
            error("Deleting invalid metadata log file \"%s/"METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL
                      METALOG_EXTENSION"\"", dbfiles_path, metalogfile->starting_fileno, metalogfile->fileno);
            unlink_metadata_logfile(metalogfile);
            freez(metalogfile);
            ++failed_to_load;
            continue;
        }
        metadata_logfile_list_insert(&ctx->metadata_logfiles, metalogfile);
        rrd_atomic_fetch_add(&ctx->disk_space, metalogfile->pos);
    }
    matched_files -= failed_to_load;
    debug(D_METADATALOG, "PARSER ended");

    parser_destroy(parser);

    size_t count = metalog_parser_object.count;

    debug(D_METADATALOG, "Parsing count=%u", (unsigned)count);
after_failed_to_parse:

    freez(metalogfiles);

    return matched_files;
}

/* Creates a metadata log file */
int add_new_metadata_logfile(struct metalog_instance *ctx, struct metadata_logfile_list *logfile_list,
                             unsigned starting_fileno, unsigned fileno)
{
    struct metadata_logfile *metalogfile;
    int ret;
    char path[RRDENG_PATH_MAX];

    info("Creating new metadata log file in path %s", ctx->rrdeng_ctx->dbfiles_path);
    metalogfile = mallocz(sizeof(*metalogfile));
    metadata_logfile_init(metalogfile, ctx, starting_fileno, fileno);
    ret = create_metadata_logfile(metalogfile);
    if (!ret) {
        generate_metadata_logfile_path(metalogfile, path, sizeof(path));
        info("Created metadata log file \"%s\".", path);
    } else {
        freez(metalogfile);
        return ret;
    }
    metadata_logfile_list_insert(logfile_list, metalogfile);
    rrd_atomic_fetch_add(&ctx->disk_space, metalogfile->pos);

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
        ret = add_new_metadata_logfile(ctx, &ctx->metadata_logfiles, 0, 1);
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
    struct metalog_record_block *record_block, *next_record_block;

    for (metalogfile = ctx->metadata_logfiles.first ; metalogfile != NULL ; metalogfile = next_metalogfile) {
        next_metalogfile = metalogfile->next;

        for (record_block = metalogfile->records.first ; record_block != NULL ; record_block = next_record_block) {
            next_record_block = record_block->next;
            freez(record_block);
        }
        close_metadata_logfile(metalogfile);
        freez(metalogfile);
    }
}
