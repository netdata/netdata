// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

void datafile_list_insert(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile)
{
    uv_rwlock_wrlock(&ctx->datafiles.rwlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(ctx->datafiles.first, datafile, prev, next);
    uv_rwlock_wrunlock(&ctx->datafiles.rwlock);
}

void datafile_list_delete_unsafe(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile)
{
    DOUBLE_LINKED_LIST_REMOVE_UNSAFE(ctx->datafiles.first, datafile, prev, next);
}


static struct rrdengine_datafile *datafile_alloc_and_init(struct rrdengine_instance *ctx, unsigned tier, unsigned fileno)
{
    fatal_assert(tier == 1);

    struct rrdengine_datafile *datafile = callocz(1, sizeof(struct rrdengine_datafile));

    datafile->tier = tier;
    datafile->fileno = fileno;
    fatal_assert(0 == uv_rwlock_init(&datafile->extent_rwlock));
    datafile->ctx = ctx;

    datafile->users.spinlock = NETDATA_SPINLOCK_INITIALIZER;
    datafile->users.available = true;

    datafile->extent_exclusive_access.spinlock = NETDATA_SPINLOCK_INITIALIZER;

    datafile->writers.spinlock = NETDATA_SPINLOCK_INITIALIZER;

    return datafile;
}

void datafile_acquire_dup(struct rrdengine_datafile *df) {
    netdata_spinlock_lock(&df->users.spinlock);

    if(!df->users.lockers)
        fatal("DBENGINE: datafile is not acquired to duplicate");

    df->users.lockers++;

    netdata_spinlock_unlock(&df->users.spinlock);
}

bool datafile_acquire(struct rrdengine_datafile *df, DATAFILE_ACQUIRE_REASONS reason) {
    bool ret;

    netdata_spinlock_lock(&df->users.spinlock);

    if(df->users.available) {
        ret = true;
        df->users.lockers++;
        df->users.lockers_by_reason[reason]++;
    }
    else
        ret = false;

    netdata_spinlock_unlock(&df->users.spinlock);

    return ret;
}

void datafile_release(struct rrdengine_datafile *df, DATAFILE_ACQUIRE_REASONS reason) {
    netdata_spinlock_lock(&df->users.spinlock);
    if(!df->users.lockers)
        fatal("DBENGINE DATAFILE: cannot release a datafile that is not acquired");

    df->users.lockers--;
    df->users.lockers_by_reason[reason]--;
    netdata_spinlock_unlock(&df->users.spinlock);
}

bool datafile_acquire_for_deletion(struct rrdengine_datafile *df) {
    bool can_be_deleted = false;

    netdata_spinlock_lock(&df->users.spinlock);
    df->users.available = false;

    if(!df->users.lockers)
        can_be_deleted = true;

    else {
        // there are lockers

        // evict any pages referencing this in the open cache
        netdata_spinlock_unlock(&df->users.spinlock);
        pgc_open_evict_clean_pages_of_datafile(open_cache, df);
        netdata_spinlock_lock(&df->users.spinlock);

        if(!df->users.lockers)
            can_be_deleted = true;

        else {
            // there are lockers still

            // count the number of pages referencing this in the open cache
            netdata_spinlock_unlock(&df->users.spinlock);
            usec_t time_to_scan_ut = now_monotonic_usec();
            size_t clean_pages_in_open_cache = pgc_count_clean_pages_having_data_ptr(open_cache, (Word_t)df->ctx, df);
            size_t hot_pages_in_open_cache = pgc_count_hot_pages_having_data_ptr(open_cache, (Word_t)df->ctx, df);
            time_to_scan_ut = now_monotonic_usec() - time_to_scan_ut;
            netdata_spinlock_lock(&df->users.spinlock);

            if(!df->users.lockers)
                can_be_deleted = true;

            else if(!clean_pages_in_open_cache && !hot_pages_in_open_cache) {
                // no pages in the open cache related to this datafile

                time_t now_s = now_monotonic_sec();

                if(!df->users.time_to_evict) {
                    // first time we did the above
                    df->users.time_to_evict = now_s + 120;
                    internal_error(true, "DBENGINE: datafile %u of tier %d is not used by any open cache pages, "
                                         "but it has %u lockers (oc:%u, pd:%u), "
                                         "%zu clean and %zu hot open cache pages "
                                         "- will be deleted shortly "
                                         "(scanned open cache in %llu usecs)",
                                   df->fileno, df->ctx->tier,
                                   df->users.lockers,
                                   df->users.lockers_by_reason[DATAFILE_ACQUIRE_OPEN_CACHE],
                                   df->users.lockers_by_reason[DATAFILE_ACQUIRE_PAGE_DETAILS],
                                   clean_pages_in_open_cache,
                                   hot_pages_in_open_cache,
                                   time_to_scan_ut);
                }

                else if(now_s > df->users.time_to_evict) {
                    // time expired, lets remove it
                    can_be_deleted = true;
                    internal_error(true, "DBENGINE: datafile %u of tier %d is not used by any open cache pages, "
                                         "but it has %u lockers (oc:%u, pd:%u), "
                                         "%zu clean and %zu hot open cache pages "
                                         "- will be deleted now "
                                         "(scanned open cache in %llu usecs)",
                                   df->fileno, df->ctx->tier,
                                   df->users.lockers,
                                   df->users.lockers_by_reason[DATAFILE_ACQUIRE_OPEN_CACHE],
                                   df->users.lockers_by_reason[DATAFILE_ACQUIRE_PAGE_DETAILS],
                                   clean_pages_in_open_cache,
                                   hot_pages_in_open_cache,
                                   time_to_scan_ut);
                }
            }
            else
                internal_error(true, "DBENGINE: datafile %u of tier %d "
                                     "has %u lockers (oc:%u, pd:%u), "
                                     "%zu clean and %zu hot open cache pages "
                                     "(scanned open cache in %llu usecs)",
                               df->fileno, df->ctx->tier,
                               df->users.lockers,
                               df->users.lockers_by_reason[DATAFILE_ACQUIRE_OPEN_CACHE],
                               df->users.lockers_by_reason[DATAFILE_ACQUIRE_PAGE_DETAILS],
                               clean_pages_in_open_cache,
                               hot_pages_in_open_cache,
                               time_to_scan_ut);
        }
    }
    netdata_spinlock_unlock(&df->users.spinlock);

    return can_be_deleted;
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
        error("DBENGINE: uv_fs_close(%s): %s", path, uv_strerror(ret));
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
        error("DBENGINE: uv_fs_fsunlink(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    ++ctx->stats.datafile_deletions;

    return ret;
}

int destroy_data_file_unsafe(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_datafilepath(datafile, path, sizeof(path));

    ret = uv_fs_ftruncate(NULL, &req, datafile->file, 0, NULL);
    if (ret < 0) {
        error("DBENGINE: uv_fs_ftruncate(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    ret = uv_fs_close(NULL, &req, datafile->file, NULL);
    if (ret < 0) {
        error("DBENGINE: uv_fs_close(%s): %s", path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    ret = uv_fs_unlink(NULL, &req, path, NULL);
    if (ret < 0) {
        error("DBENGINE: uv_fs_fsunlink(%s): %s", path, uv_strerror(ret));
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
        fatal("DBENGINE: posix_memalign:%s", strerror(ret));
    }
    memset(superblock, 0, sizeof(*superblock));
    (void) strncpy(superblock->magic_number, RRDENG_DF_MAGIC, RRDENG_MAGIC_SZ);
    (void) strncpy(superblock->version, RRDENG_DF_VER, RRDENG_VER_SZ);
    superblock->tier = 1;

    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_write(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        fatal_assert(req.result < 0);
        error("DBENGINE: uv_fs_write: %s", uv_strerror(ret));
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
    }
    uv_fs_req_cleanup(&req);
    posix_memfree(superblock);
    if (ret < 0) {
        destroy_data_file_unsafe(datafile);
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
        fatal("DBENGINE: posix_memalign:%s", strerror(ret));
    }
    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_read(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        error("DBENGINE: uv_fs_read: %s", uv_strerror(ret));
        uv_fs_req_cleanup(&req);
        goto error;
    }
    fatal_assert(req.result >= 0);
    uv_fs_req_cleanup(&req);

    if (strncmp(superblock->magic_number, RRDENG_DF_MAGIC, RRDENG_MAGIC_SZ) ||
        strncmp(superblock->version, RRDENG_DF_VER, RRDENG_VER_SZ) ||
        superblock->tier != 1) {
        error("DBENGINE: file has invalid superblock.");
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
    info("DBENGINE: initializing data file \"%s\".", path);

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

    info("DBENGINE: data file \"%s\" initialized (size:%"PRIu64").", path, file_size);
    return 0;

    error:
    error = ret;
    ret = uv_fs_close(NULL, &req, file, NULL);
    if (ret < 0) {
        error("DBENGINE: uv_fs_close(%s): %s", path, uv_strerror(ret));
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
        error("DBENGINE: uv_fs_scandir(%s): %s", ctx->dbfiles_path, uv_strerror(ret));
        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return ret;
    }
    info("DBENGINE: found %d files in path %s", ret, ctx->dbfiles_path);

    datafiles = callocz(MIN(ret, MAX_DATAFILES), sizeof(*datafiles));
    for (matched_files = 0 ; UV_EOF != uv_fs_scandir_next(&req, &dent) && matched_files < MAX_DATAFILES ; ) {
        ret = sscanf(dent.name, DATAFILE_PREFIX RRDENG_FILE_NUMBER_SCAN_TMPL DATAFILE_EXTENSION, &tier, &no);
        if (2 == ret) {
            datafile = datafile_alloc_and_init(ctx, tier, no);
            datafiles[matched_files++] = datafile;
        }
    }
    uv_fs_req_cleanup(&req);

    if (0 == matched_files) {
        freez(datafiles);
        return 0;
    }
    if (matched_files == MAX_DATAFILES) {
        error("DBENGINE: warning: hit maximum database engine file limit of %d files", MAX_DATAFILES);
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
        ret = load_journal_file(ctx, journalfile, datafile);
        if (0 != ret) {
            if (!must_delete_pair) /* If datafile is still open close it */
                close_data_file(datafile);
            must_delete_pair = 1;
        }
        if (must_delete_pair) {
            char path[RRDENG_PATH_MAX];

            error("DBENGINE: deleting invalid data and journal file pair.");
            ret = unlink_journal_file(journalfile);
            if (!ret) {
                generate_journalfilepath(datafile, path, sizeof(path));
                info("DBENGINE: deleted journal file \"%s\".", path);
            }
            ret = unlink_data_file(datafile);
            if (!ret) {
                generate_datafilepath(datafile, path, sizeof(path));
                info("DBENGINE: deleted data file \"%s\".", path);
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
int create_new_datafile_pair(struct rrdengine_instance *ctx)
{
    struct rrdengine_datafile *datafile;
    struct rrdengine_journalfile *journalfile;
    unsigned fileno = __atomic_load_n(&ctx->last_fileno, __ATOMIC_RELAXED) + 1;
    int ret;
    char path[RRDENG_PATH_MAX];

    info("DBENGINE: creating new data and journal files in path %s", ctx->dbfiles_path);
    datafile = datafile_alloc_and_init(ctx, 1, fileno);
    ret = create_data_file(datafile);
    if (!ret) {
        generate_datafilepath(datafile, path, sizeof(path));
        info("DBENGINE: created data file \"%s\".", path);
    } else {
        goto error_after_datafile;
    }

    journalfile = mallocz(sizeof(*journalfile));
    datafile->journalfile = journalfile;
    journalfile_init(journalfile, datafile);
    ret = create_journal_file(journalfile, datafile);
    if (!ret) {
        generate_journalfilepath(datafile, path, sizeof(path));
        info("DBENGINE: created journal file \"%s\".", path);
    } else {
        goto error_after_journalfile;
    }
    datafile_list_insert(ctx, datafile);
    ctx->disk_space += datafile->pos + journalfile->pos;

    __atomic_add_fetch(&ctx->last_fileno, 1, __ATOMIC_RELAXED);

    return 0;

error_after_journalfile:
    destroy_data_file_unsafe(datafile);
    freez(journalfile);
error_after_datafile:
    freez(datafile);

    uv_rwlock_wrunlock(&ctx->datafiles.rwlock);
    return ret;
}

/* Page cache must already be initialized.
 * Return 0 on success.
 */
int init_data_files(struct rrdengine_instance *ctx)
{
    int ret;

    fatal_assert(0 == uv_rwlock_init(&ctx->datafiles.rwlock));
    __atomic_store_n(&ctx->journal_initialization, true, __ATOMIC_RELAXED);
    ret = scan_data_files(ctx);
    if (ret < 0) {
        error("DBENGINE: failed to scan path \"%s\".", ctx->dbfiles_path);
        return ret;
    } else if (0 == ret) {
        info("DBENGINE: data files not found, creating in path \"%s\".", ctx->dbfiles_path);
        ctx->last_fileno = 0;
        ret = create_new_datafile_pair(ctx);
        if (ret) {
            error("DBENGINE: failed to create data and journal files in path \"%s\".", ctx->dbfiles_path);
            return ret;
        }
    }
    else if(ctx->create_new_datafile_pair)
        create_new_datafile_pair(ctx);

    pgc_reset_hot_max(open_cache);
    ctx->create_new_datafile_pair = false;
    __atomic_store_n(&ctx->journal_initialization, false, __ATOMIC_RELAXED);
    return 0;
}

void finalize_data_files(struct rrdengine_instance *ctx)
{
    do {
        struct rrdengine_datafile *datafile = ctx->datafiles.first;
        struct rrdengine_journalfile *journalfile = datafile->journalfile;

        while(!datafile_acquire_for_deletion(datafile) && datafile != ctx->datafiles.first->prev) {
            info("Waiting to acquire data file %u of tier %d to close it...", datafile->fileno, ctx->tier);
            sleep_usec(500 * USEC_PER_MS);
        }

        bool available = false;
        do {
            uv_rwlock_wrlock(&ctx->datafiles.rwlock);
            netdata_spinlock_lock(&datafile->writers.spinlock);
            available = (datafile->writers.running || datafile->writers.flushed_to_open_running) ? false : true;

            if(!available) {
                netdata_spinlock_unlock(&datafile->writers.spinlock);
                uv_rwlock_wrunlock(&ctx->datafiles.rwlock);
                info("Waiting for writers to data file %u of tier %d to finish...", datafile->fileno, ctx->tier);
                sleep_usec(500 * USEC_PER_MS);
            }
        } while(!available);

        close_journal_file(journalfile, datafile);
        close_data_file(datafile);
        datafile_list_delete_unsafe(ctx, datafile);
        netdata_spinlock_unlock(&datafile->writers.spinlock);
        uv_rwlock_wrunlock(&ctx->datafiles.rwlock);

        freez(journalfile);
        freez(datafile);

    } while(ctx->datafiles.first);
}
