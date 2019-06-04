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
    if (likely(NULL != ctx->datafiles.last)) {
        ctx->datafiles.last->next = datafile;
    }
    if (unlikely(NULL == ctx->datafiles.first)) {
        ctx->datafiles.first = datafile;
    }
    ctx->datafiles.last = datafile;
}

void datafile_list_delete(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile)
{
    struct rrdengine_datafile *next;

    next = datafile->next;
    assert((NULL != next) && (ctx->datafiles.first == datafile) && (ctx->datafiles.last != datafile));
    ctx->datafiles.first = next;
}


static void datafile_init(struct rrdengine_datafile *datafile, struct rrdengine_instance *ctx,
                          unsigned tier, unsigned fileno)
{
    assert(tier == 1);
    datafile->tier = tier;
    datafile->fileno = fileno;
    datafile->file = (uv_file)0;
    datafile->pos = 0;
    datafile->extents.first = datafile->extents.last = NULL; /* will be populated by journalfile */
    datafile->journalfile = NULL;
    datafile->next = NULL;
    datafile->ctx = ctx;
}

static void generate_datafilepath(struct rrdengine_datafile *datafile, char *str, size_t maxlen)
{
    (void) snprintf(str, maxlen, "%s/" DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION,
                    datafile->ctx->dbfiles_path, datafile->tier, datafile->fileno);
}

int destroy_data_file(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    int ret, fd;
    char path[1024];

    ret = uv_fs_ftruncate(NULL, &req, datafile->file, 0, NULL);
    if (ret < 0) {
        fatal("uv_fs_ftruncate: %s", uv_strerror(ret));
    }
    assert(0 == req.result);
    uv_fs_req_cleanup(&req);

    ret = uv_fs_close(NULL, &req, datafile->file, NULL);
    if (ret < 0) {
        fatal("uv_fs_close: %s", uv_strerror(ret));
    }
    assert(0 == req.result);
    uv_fs_req_cleanup(&req);

    generate_datafilepath(datafile, path, sizeof(path));
    fd = uv_fs_unlink(NULL, &req, path, NULL);
    if (fd < 0) {
        fatal("uv_fs_fsunlink: %s", uv_strerror(fd));
    }
    assert(0 == req.result);
    uv_fs_req_cleanup(&req);

    ++ctx->stats.datafile_deletions;

    return 0;
}

int create_data_file(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    uv_file file;
    int ret, fd;
    struct rrdeng_df_sb *superblock;
    uv_buf_t iov;
    char path[1024];

    generate_datafilepath(datafile, path, sizeof(path));
    fd = open_file_direct_io(path, O_CREAT | O_RDWR | O_TRUNC, &file);
    if (fd < 0) {
        fatal("uv_fs_fsopen: %s", uv_strerror(fd));
    }

    ret = posix_memalign((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    if (unlikely(ret)) {
        fatal("posix_memalign:%s", strerror(ret));
    }
    (void) strncpy(superblock->magic_number, RRDENG_DF_MAGIC, RRDENG_MAGIC_SZ);
    (void) strncpy(superblock->version, RRDENG_DF_VER, RRDENG_VER_SZ);
    superblock->tier = 1;

    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_write(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        fatal("uv_fs_write: %s", uv_strerror(ret));
    }
    if (req.result < 0) {
        fatal("uv_fs_write: %s", uv_strerror((int)req.result));
    }
    uv_fs_req_cleanup(&req);
    free(superblock);

    datafile->file = file;
    datafile->pos = sizeof(*superblock);
    ctx->stats.io_write_bytes += sizeof(*superblock);
    ++ctx->stats.io_write_requests;
    ++ctx->stats.datafile_creations;

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
    assert(req.result >= 0);
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
    free(superblock);
    return ret;
}

static int load_data_file(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    uv_file file;
    int ret, fd;
    uint64_t file_size;
    char path[1024];

    generate_datafilepath(datafile, path, sizeof(path));
    fd = open_file_direct_io(path, O_RDWR, &file);
    if (fd < 0) {
        /* if (UV_ENOENT != fd) */
        error("uv_fs_fsopen: %s", uv_strerror(fd));
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
    (void) uv_fs_close(NULL, &req, file, NULL);
    uv_fs_req_cleanup(&req);
    return ret;
}

static int scan_data_files_cmp(const void *a, const void *b)
{
    struct rrdengine_datafile *file1, *file2;
    char path1[1024], path2[1024];

    file1 = *(struct rrdengine_datafile **)a;
    file2 = *(struct rrdengine_datafile **)b;
    generate_datafilepath(file1, path1, sizeof(path1));
    generate_datafilepath(file2, path2, sizeof(path2));
    return strcmp(path1, path2);
}

/* Returns number of datafiles that were loaded */
static int scan_data_files(struct rrdengine_instance *ctx)
{
    int ret;
    unsigned tier, no, matched_files, i,failed_to_load;
    static uv_fs_t req;
    uv_dirent_t dent;
    struct rrdengine_datafile **datafiles, *datafile;
    struct rrdengine_journalfile *journalfile;

    ret = uv_fs_scandir(NULL, &req, ctx->dbfiles_path, 0, NULL);
    assert(ret >= 0);
    assert(req.result >= 0);
    info("Found %d files in path %s", ret, ctx->dbfiles_path);

    datafiles = callocz(MIN(ret, MAX_DATAFILES), sizeof(*datafiles));
    for (matched_files = 0 ; UV_EOF != uv_fs_scandir_next(&req, &dent) && matched_files < MAX_DATAFILES ; ) {
        info("Scanning file \"%s\"", dent.name);
        ret = sscanf(dent.name, DATAFILE_PREFIX RRDENG_FILE_NUMBER_SCAN_TMPL DATAFILE_EXTENSION, &tier, &no);
        if (2 == ret) {
            info("Matched file \"%s\"", dent.name);
            datafile = mallocz(sizeof(*datafile));
            datafile_init(datafile, ctx, tier, no);
            datafiles[matched_files++] = datafile;
        }
    }
    uv_fs_req_cleanup(&req);

    if (matched_files == MAX_DATAFILES) {
        error("Warning: hit maximum database engine file limit of %d files", MAX_DATAFILES);
    }
    qsort(datafiles, matched_files, sizeof(*datafiles), scan_data_files_cmp);
    for (failed_to_load = 0, i = 0 ; i < matched_files ; ++i) {
        datafile = datafiles[i];
        ret = load_data_file(datafile);
        if (0 != ret) {
            free(datafile);
            ++failed_to_load;
            continue;
        }
        journalfile = mallocz(sizeof(*journalfile));
        datafile->journalfile = journalfile;
        journalfile_init(journalfile, datafile);
        ret = load_journal_file(ctx, journalfile, datafile);
        if (0 != ret) {
            free(datafile);
            free(journalfile);
            ++failed_to_load;
            continue;
        }
        datafile_list_insert(ctx, datafile);
        ctx->disk_space += datafile->pos + journalfile->pos;
    }
    if (failed_to_load) {
        error("%u files failed to load.", failed_to_load);
    }
    free(datafiles);

    return matched_files - failed_to_load;
}

/* Creates a datafile and a journalfile pair */
void create_new_datafile_pair(struct rrdengine_instance *ctx, unsigned tier, unsigned fileno)
{
    struct rrdengine_datafile *datafile;
    struct rrdengine_journalfile *journalfile;
    int ret;

    info("Creating new data and journal files.");
    datafile = mallocz(sizeof(*datafile));
    datafile_init(datafile, ctx, tier, fileno);
    ret = create_data_file(datafile);
    assert(!ret);

    journalfile = mallocz(sizeof(*journalfile));
    datafile->journalfile = journalfile;
    journalfile_init(journalfile, datafile);
    ret = create_journal_file(journalfile, datafile);
    assert(!ret);
    datafile_list_insert(ctx, datafile);
    ctx->disk_space += datafile->pos + journalfile->pos;
}

/* Page cache must already be initialized. */
int init_data_files(struct rrdengine_instance *ctx)
{
    int ret;

    ret = scan_data_files(ctx);
    if (0 == ret) {
        info("Data files not found, creating.");
        create_new_datafile_pair(ctx, 1, 1);
    }
    return 0;
}