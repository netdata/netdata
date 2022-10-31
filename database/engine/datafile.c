// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

void df_extent_insert(struct extent_info *extent)
{
    struct rrdengine_datafile *datafile = extent->datafile;

    if (likely(NULL != datafile->extents.last)) {
        datafile->extents.last->next = extent;
    }
    if (unlikely(NULL == datafile->extents.first)) {
        datafile->extents.first = extent;
    }
    datafile->extents.last = extent;
}

void datafile_list_insert(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile)
{
    uv_rwlock_wrlock(&ctx->datafiles.rwlock);

    if (likely(NULL != ctx->datafiles.last)) {
        ctx->datafiles.last->next = datafile;
    }
    if (unlikely(NULL == ctx->datafiles.first)) {
        ctx->datafiles.first = datafile;
    }
    ctx->datafiles.last = datafile;

    uv_rwlock_wrunlock(&ctx->datafiles.rwlock);
}

void datafile_list_delete(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile)
{
    struct rrdengine_datafile *next;
    uv_rwlock_wrlock(&ctx->datafiles.rwlock);

    next = datafile->next;
    fatal_assert((NULL != next) && (ctx->datafiles.first == datafile) && (ctx->datafiles.last != datafile));
    ctx->datafiles.first = next;

    uv_rwlock_wrunlock(&ctx->datafiles.rwlock);
}


static void datafile_init(struct rrdengine_datafile *datafile, struct rrdengine_instance *ctx,
                          unsigned tier, unsigned fileno)
{
    fatal_assert(tier == 1);
    datafile->tier = tier;
    datafile->fileno = fileno;
    datafile->file = (uv_file)0;
    datafile->pos = 0;
    datafile->extents.first = datafile->extents.last = NULL; /* will be populated by journalfile */
    datafile->journalfile = NULL;
    datafile->next = NULL;
    datafile->ctx = ctx;
}

void generate_datafilepath(struct rrdengine_datafile *datafile, char *str, size_t maxlen)
{
    (void) snprintfz(str, maxlen, "%s/" DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION,
                    datafile->ctx->dbfiles_path, datafile->tier, datafile->fileno);
}

int close_data_file(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_datafilepath(datafile, path, sizeof(path));

    ret = uv_fs_close(NULL, &req, datafile->file, NULL);
    if (ret < 0) {
        error("uv_fs_close(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    return ret;
}

int unlink_data_file(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_datafilepath(datafile, path, sizeof(path));

    ret = uv_fs_unlink(NULL, &req, path, NULL);
    if (ret < 0) {
        error("uv_fs_fsunlink(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    ++ctx->stats.datafile_deletions;

    return ret;
}

int destroy_data_file(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_datafilepath(datafile, path, sizeof(path));

    ret = uv_fs_ftruncate(NULL, &req, datafile->file, 0, NULL);
    if (ret < 0) {
        error("uv_fs_ftruncate(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    ret = uv_fs_close(NULL, &req, datafile->file, NULL);
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

    ++ctx->stats.datafile_deletions;

    return ret;
}

int create_data_file(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    uv_file file;
    int ret, fd;
    struct rrdeng_df_sb *superblock;
    uv_buf_t iov;
    char path[RRDENG_PATH_MAX];

    generate_datafilepath(datafile, path, sizeof(path));
    fd = open_file_direct_io(path, O_CREAT | O_RDWR | O_TRUNC, &file);
    if (fd < 0) {
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return fd;
    }
    datafile->file = file;
    ++ctx->stats.datafile_creations;

    ret = posix_memalign((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    if (unlikely(ret)) {
        fatal("posix_memalign:%s", strerror(ret));
    }
    memset(superblock, 0, sizeof(*superblock));
    (void) strncpy(superblock->magic_number, RRDENG_DF_MAGIC, RRDENG_MAGIC_SZ);
    (void) strncpy(superblock->version, RRDENG_DF_VER, RRDENG_VER_SZ);
    superblock->tier = 1;

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
        destroy_data_file(datafile);
        return ret;
    }

    datafile->pos = sizeof(*superblock);
    ctx->stats.io_write_bytes += sizeof(*superblock);
    ++ctx->stats.io_write_requests;

    return 0;
}

static int check_data_file_superblock(uv_file file)
{
    int ret;
    struct rrdeng_df_sb *superblock;
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

    if (strncmp(superblock->magic_number, RRDENG_DF_MAGIC, RRDENG_MAGIC_SZ) ||
        strncmp(superblock->version, RRDENG_DF_VER, RRDENG_VER_SZ) ||
        superblock->tier != 1) {
        error("File has invalid superblock.");
        ret = UV_EINVAL;
    } else {
        ret = 0;
    }
    error:
    posix_memfree(superblock);
    return ret;
}

static int load_data_file(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    uv_file file;
    int ret, fd, error;
    uint64_t file_size;
    char path[RRDENG_PATH_MAX];

    generate_datafilepath(datafile, path, sizeof(path));
    fd = open_file_direct_io(path, O_RDWR, &file);
    if (fd < 0) {
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return fd;
    }
    info("Initializing data file \"%s\".", path);

    ret = check_file_properties(file, &file_size, sizeof(struct rrdeng_df_sb));
    if (ret)
        goto error;
    file_size = ALIGN_BYTES_CEILING(file_size);

    ret = check_data_file_superblock(file);
    if (ret)
        goto error;
    ctx->stats.io_read_bytes += sizeof(struct rrdeng_df_sb);
    ++ctx->stats.io_read_requests;

    datafile->file = file;
    datafile->pos = file_size;

    info("Data file \"%s\" initialized (size:%"PRIu64").", path, file_size);
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

static int scan_data_files_cmp(const void *a, const void *b)
{
    struct rrdengine_datafile *file1, *file2;
    char path1[RRDENG_PATH_MAX], path2[RRDENG_PATH_MAX];

    file1 = *(struct rrdengine_datafile **)a;
    file2 = *(struct rrdengine_datafile **)b;
    generate_datafilepath(file1, path1, sizeof(path1));
    generate_datafilepath(file2, path2, sizeof(path2));
    return strcmp(path1, path2);
}

/* Returns number of datafiles that were loaded or < 0 on error */
static int scan_data_files(struct rrdengine_instance *ctx)
{
    int ret;
    unsigned tier, no, matched_files, i,failed_to_load;
    static uv_fs_t req;
    uv_dirent_t dent;
    struct rrdengine_datafile **datafiles, *datafile;
    struct rrdengine_journalfile *journalfile;

    ret = uv_fs_scandir(NULL, &req, ctx->dbfiles_path, 0, NULL);
    if (ret < 0) {
        fatal_assert(req.result < 0);
        uv_fs_req_cleanup(&req);
        error("uv_fs_scandir(%s): %s", ctx->dbfiles_path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return ret;
    }
    info("Found %d files in path %s", ret, ctx->dbfiles_path);

    datafiles = callocz(MIN(ret, MAX_DATAFILES), sizeof(*datafiles));
    for (matched_files = 0 ; UV_EOF != uv_fs_scandir_next(&req, &dent) && matched_files < MAX_DATAFILES ; ) {
        ret = sscanf(dent.name, DATAFILE_PREFIX RRDENG_FILE_NUMBER_SCAN_TMPL DATAFILE_EXTENSION, &tier, &no);
        if (2 == ret) {
            datafile = mallocz(sizeof(*datafile));
            datafile_init(datafile, ctx, tier, no);
            datafiles[matched_files++] = datafile;
        }
    }
    uv_fs_req_cleanup(&req);

    if (0 == matched_files) {
        freez(datafiles);
        return 0;
    }
    if (matched_files == MAX_DATAFILES) {
        error("Warning: hit maximum database engine file limit of %d files", MAX_DATAFILES);
    }
    qsort(datafiles, matched_files, sizeof(*datafiles), scan_data_files_cmp);
    /* TODO: change this when tiering is implemented */
    ctx->last_fileno = datafiles[matched_files - 1]->fileno;

    for (failed_to_load = 0, i = 0 ; i < matched_files ; ++i) {
        uint8_t must_delete_pair = 0;

        datafile = datafiles[i];
        ret = load_data_file(datafile);
        if (0 != ret) {
            must_delete_pair = 1;
        }
        journalfile = mallocz(sizeof(*journalfile));
        datafile->journalfile = journalfile;
        journalfile_init(journalfile, datafile);
        journalfile->file_index = i;
        ret = load_journal_file(ctx, journalfile, datafile);
        if (0 != ret) {
            if (!must_delete_pair) /* If datafile is still open close it */
                close_data_file(datafile);
            must_delete_pair = 1;
        }
        if (must_delete_pair) {
            char path[RRDENG_PATH_MAX];

            // TODO: Also delete the version 2
            error("Deleting invalid data and journal file pair.");
            ret = unlink_journal_file(journalfile);
            if (!ret) {
                generate_journalfilepath(datafile, path, sizeof(path));
                info("Deleted journal file \"%s\".", path);
            }
            ret = unlink_data_file(datafile);
            if (!ret) {
                generate_datafilepath(datafile, path, sizeof(path));
                info("Deleted data file \"%s\".", path);
            }
            freez(journalfile);
            freez(datafile);
            ++failed_to_load;
            continue;
        }

        datafile_list_insert(ctx, datafile);
        ctx->disk_space += datafile->pos + journalfile->pos;
    }
    matched_files -= failed_to_load;
    freez(datafiles);

    return matched_files;
}

/* Creates a datafile and a journalfile pair */
int create_new_datafile_pair(struct rrdengine_instance *ctx, unsigned tier, unsigned fileno)
{
    struct rrdengine_datafile *datafile;
    struct rrdengine_journalfile *journalfile;
    int ret;
    char path[RRDENG_PATH_MAX];

    info("Creating new data and journal files in path %s", ctx->dbfiles_path);
    datafile = mallocz(sizeof(*datafile));
    datafile_init(datafile, ctx, tier, fileno);
    ret = create_data_file(datafile);
    if (!ret) {
        generate_datafilepath(datafile, path, sizeof(path));
        info("Created data file \"%s\".", path);
    } else {
        goto error_after_datafile;
    }

    journalfile = mallocz(sizeof(*journalfile));
    datafile->journalfile = journalfile;
    journalfile_init(journalfile, datafile);
    ret = create_journal_file(journalfile, datafile);
    if (!ret) {
        generate_journalfilepath(datafile, path, sizeof(path));
        info("Created journal file \"%s\".", path);
    } else {
        goto error_after_journalfile;
    }
    datafile_list_insert(ctx, datafile);
    ctx->disk_space += datafile->pos + journalfile->pos;

    return 0;

error_after_journalfile:
    destroy_data_file(datafile);
    freez(journalfile);
error_after_datafile:
    freez(datafile);
    return ret;
}

/* Page cache must already be initialized.
 * Return 0 on success.
 */
int init_data_files(struct rrdengine_instance *ctx)
{
    int ret;

    fatal_assert(0 == uv_rwlock_init(&ctx->datafiles.rwlock));
    ret = scan_data_files(ctx);
    if (ret < 0) {
        error("Failed to scan path \"%s\".", ctx->dbfiles_path);
        return ret;
    } else if (0 == ret) {
        info("Data files not found, creating in path \"%s\".", ctx->dbfiles_path);
        ret = create_new_datafile_pair(ctx, 1, 1);
        if (ret) {
            error("Failed to create data and journal files in path \"%s\".", ctx->dbfiles_path);
            return ret;
        }
        ctx->last_fileno = 1;
    }

    return 0;
}

void finalize_data_files(struct rrdengine_instance *ctx)
{
    struct rrdengine_datafile *datafile, *next_datafile;
    struct rrdengine_journalfile *journalfile;
    struct extent_info *extent, *next_extent;

    for (datafile = ctx->datafiles.first ; datafile != NULL ; datafile = next_datafile) {
        journalfile = datafile->journalfile;
        next_datafile = datafile->next;

        for (extent = datafile->extents.first ; extent != NULL ; extent = next_extent) {
            next_extent = extent->next;
            freez(extent);
        }
        close_journal_file(journalfile, datafile);
        close_data_file(datafile);
        freez(journalfile);
        freez(datafile);
    }
}
