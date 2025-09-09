// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

void datafile_list_insert(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile)
{
    netdata_rwlock_wrlock(&ctx->datafiles.rwlock);
    Pvoid_t *Pvalue = JudyLIns(&ctx->datafiles.JudyL, (Word_t ) datafile->fileno, PJE0);
    if(!Pvalue || Pvalue == PJERR)
        fatal("DBENGINE: cannot insert datafile %u of tier %d into the datafiles list",
              datafile->fileno, ctx->config.tier);
    *Pvalue = datafile;
    netdata_rwlock_wrunlock(&ctx->datafiles.rwlock);
}

void datafile_list_delete_unsafe(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile)
{
    (void) JudyLDel(&ctx->datafiles.JudyL, (Word_t)datafile->fileno, PJE0);
}


static struct rrdengine_datafile *datafile_alloc_and_init(struct rrdengine_instance *ctx, unsigned tier, unsigned fileno)
{
    fatal_assert(tier == 1);

    struct rrdengine_datafile *datafile = callocz(1, sizeof(struct rrdengine_datafile));

    datafile->tier = tier;
    datafile->fileno = fileno;
    fatal_assert(0 == netdata_rwlock_init(&datafile->extent_rwlock));
    datafile->ctx = ctx;
    datafile->magic1 = datafile->magic2 = DATAFILE_MAGIC;

    datafile->users.available = true;

    spinlock_init(&datafile->users.spinlock);
    spinlock_init(&datafile->writers.spinlock);
    rw_spinlock_init(&datafile->extent_epdl.spinlock);

    return datafile;
}

ALWAYS_INLINE bool datafile_acquire(struct rrdengine_datafile *df, DATAFILE_ACQUIRE_REASONS reason) {
    bool ret;

    spinlock_lock(&df->users.spinlock);

    if(df->users.available) {
        ret = true;
        df->users.lockers++;
        df->users.lockers_by_reason[reason]++;
    }
    else
        ret = false;

    spinlock_unlock(&df->users.spinlock);

    return ret;
}

void datafile_release_with_trace(struct rrdengine_datafile *df, DATAFILE_ACQUIRE_REASONS reason, const char *func) {
    spinlock_lock(&df->users.spinlock);
    if(!df->users.lockers)
        fatal("DBENGINE DATAFILE: cannot release datafile %u of tier %u - it is not acquired, called from %s() with reason %u",
              df->fileno, df->tier, func, reason);

    df->users.lockers--;
    df->users.lockers_by_reason[reason]--;
    spinlock_unlock(&df->users.spinlock);
}

bool datafile_acquire_for_deletion(struct rrdengine_datafile *df, bool is_shutdown)
{
    bool can_be_deleted = false;

    spinlock_lock(&df->users.spinlock);

    if(!df->users.lockers) {
        can_be_deleted = true;
        df->users.available = false;
    }
    else {
        // there are lockers

        // evict any pages referencing this in the open cache
        spinlock_unlock(&df->users.spinlock);
        pgc_open_evict_clean_pages_of_datafile(open_cache, df);
        spinlock_lock(&df->users.spinlock);

        if(!df->users.lockers) {
            can_be_deleted = true;
            df->users.available = false;
        }
        else {
            // there are lockers still

            // count the number of pages referencing this in the open cache
            spinlock_unlock(&df->users.spinlock);
            usec_t time_to_scan_ut = now_monotonic_usec();
            size_t clean_pages_in_open_cache = pgc_count_clean_pages_having_data_ptr(open_cache, (Word_t)datafile_ctx(df), df);
            size_t hot_pages_in_open_cache = pgc_count_hot_pages_having_data_ptr(open_cache, (Word_t)datafile_ctx(df), df);
            time_to_scan_ut = now_monotonic_usec() - time_to_scan_ut;
            spinlock_lock(&df->users.spinlock);

            if(!df->users.lockers) {
                can_be_deleted = true;
                df->users.available = false;
            }

            else if(!clean_pages_in_open_cache && !hot_pages_in_open_cache) {
                // no pages in the open cache related to this datafile

                time_t now_s = now_monotonic_sec();

                if(!df->users.time_to_evict) {
                    // first time we did the above
                    df->users.time_to_evict = now_s + (is_shutdown ? DATAFILE_DELETE_TIMEOUT_SHORT : DATAFILE_DELETE_TIMEOUT_LONG);
                    internal_error(true, "DBENGINE: datafile %u of tier %d is not used by any open cache pages, "
                                         "but it has %u lockers (oc:%u, pd:%u), "
                                         "%zu clean and %zu hot open cache pages "
                                         "- will be deleted shortly "
                                         "(scanned open cache in %"PRIu64" usecs)",
                                   df->fileno, datafile_ctx(df)->config.tier,
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
                    df->users.available = false;
                    internal_error(true, "DBENGINE: datafile %u of tier %d is not used by any open cache pages, "
                                         "but it has %u lockers (oc:%u, pd:%u), "
                                         "%zu clean and %zu hot open cache pages "
                                         "- will be deleted now "
                                         "(scanned open cache in %"PRIu64" usecs)",
                                   df->fileno, datafile_ctx(df)->config.tier,
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
                                     "(scanned open cache in %"PRIu64" usecs)",
                               df->fileno, datafile_ctx(df)->config.tier,
                               df->users.lockers,
                               df->users.lockers_by_reason[DATAFILE_ACQUIRE_OPEN_CACHE],
                               df->users.lockers_by_reason[DATAFILE_ACQUIRE_PAGE_DETAILS],
                               clean_pages_in_open_cache,
                               hot_pages_in_open_cache,
                               time_to_scan_ut);
        }
    }
    spinlock_unlock(&df->users.spinlock);

    return can_be_deleted;
}

void generate_datafilepath(struct rrdengine_datafile *datafile, char *str, size_t maxlen)
{
    (void) snprintfz(str, maxlen - 1, "%s/" DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION,
                    datafile_ctx(datafile)->config.dbfiles_path, datafile->tier, datafile->fileno);
}

int close_data_file(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile_ctx(datafile);
    int ret;
    char path[RRDENG_PATH_MAX];
    generate_datafilepath(datafile, path, sizeof(path));
    CLOSE_FILE(ctx, path, datafile->file, ret);
    return ret;
}

int unlink_data_file(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile_ctx(datafile);
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_datafilepath(datafile, path, sizeof(path));

    UNLINK_FILE(ctx, path, ret);
    if (ret == 0)
        __atomic_add_fetch(&ctx->stats.datafile_deletions, 1, __ATOMIC_RELAXED);

    return ret;
}

int destroy_data_file_unsafe(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile_ctx(datafile);
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_datafilepath(datafile, path, sizeof(path));

    CLOSE_FILE(ctx, path,  datafile->file, ret);
    ret = unlink_data_file(datafile);

    return ret;
}

int create_data_file(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile_ctx(datafile);
    uv_fs_t req;
    uv_file file;
    int ret, fd;
    struct rrdeng_df_sb *superblock = NULL;
    uv_buf_t iov;
    char path[RRDENG_PATH_MAX];

    generate_datafilepath(datafile, path, sizeof(path));
    fd = open_file_for_io(path, O_CREAT | O_RDWR | O_TRUNC, &file, dbengine_use_direct_io);
    if (fd < 0) {
        ctx_fs_error(ctx);
        return fd;
    }
    datafile->file = file;

    (void)posix_memalignz((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    memset(superblock, 0, sizeof(*superblock));
    (void) strncpy(superblock->magic_number, RRDENG_DF_MAGIC, RRDENG_MAGIC_SZ);
    (void) strncpy(superblock->version, RRDENG_DF_VER, RRDENG_VER_SZ);
    superblock->tier = 1;

    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    int retries = 10;
    ret = -1;
    while (ret < 0 && --retries) {
        ret = uv_fs_write(NULL, &req, file, &iov, 1, 0, NULL);
        uv_fs_req_cleanup(&req);
        if (ret < 0) {
            if (ret == -ENOSPC || ret == -EBADF || ret == -EACCES || ret == -EROFS || ret == -EINVAL)
                break;
            sleep_usec(300 * USEC_PER_MS);
        }
    }

    posix_memalign_freez(superblock);
    if (ret < 0) {
        (void) destroy_data_file_unsafe(datafile);
        ctx_io_error(ctx);
        nd_log_limit_static_global_var(dbengine_erl, 10, 0);
        nd_log_limit(&dbengine_erl, NDLS_DAEMON, NDLP_ERR, "DBENGINE: Failed to create datafile %s", path);
        return ret;
    }

    __atomic_add_fetch(&ctx->stats.datafile_creations, 1, __ATOMIC_RELAXED);
    datafile->pos = sizeof(*superblock);
    ctx_io_write_op_bytes(ctx, sizeof(*superblock));

    return 0;
}

static int check_data_file_superblock(uv_file file)
{
    int ret;
    struct rrdeng_df_sb *superblock = NULL;
    uv_buf_t iov;
    uv_fs_t req;

    (void)posix_memalignz((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_read(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        netdata_log_error("DBENGINE: uv_fs_read: %s", uv_strerror(ret));
        uv_fs_req_cleanup(&req);
        goto error;
    }
    fatal_assert(req.result >= 0);
    uv_fs_req_cleanup(&req);

    if (strncmp(superblock->magic_number, RRDENG_DF_MAGIC, RRDENG_MAGIC_SZ) ||
        strncmp(superblock->version, RRDENG_DF_VER, RRDENG_VER_SZ) ||
        superblock->tier != 1) {
        netdata_log_error("DBENGINE: file has invalid superblock.");
        ret = UV_EINVAL;
    } else {
        ret = 0;
    }
    error:
        posix_memalign_freez(superblock);
    return ret;
}

static int load_data_file(struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile_ctx(datafile);
    uv_file file;
    int ret, fd, error;
    uint64_t file_size;
    char path[RRDENG_PATH_MAX];

    generate_datafilepath(datafile, path, sizeof(path));
    fd = open_file_for_io(path, O_RDWR, &file, dbengine_use_direct_io);
    if (fd < 0) {
        ctx_fs_error(ctx);
        return fd;
    }
    
    nd_log_daemon(NDLP_DEBUG, "DBENGINE: initializing data file \"%s\".", path);

    ret = check_file_properties(file, &file_size, sizeof(struct rrdeng_df_sb));
    if (ret)
        goto err_exit;
    file_size = ALIGN_BYTES_CEILING(file_size);

    ret = check_data_file_superblock(file);
    if (ret)
        goto err_exit;

    ctx_io_read_op_bytes(ctx, sizeof(struct rrdeng_df_sb));

    datafile->file = file;
    datafile->pos = file_size;

    nd_log_daemon(NDLP_DEBUG, "DBENGINE: data file \"%s\" initialized (size:%" PRIu64 ").", path, file_size);

    return 0;

err_exit:
    error = ret;
    CLOSE_FILE(ctx, path, file, ret);
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
    int ret, matched_files, failed_to_load, i;
    unsigned tier, fileno;
    uv_fs_t req;
    uv_dirent_t dent;
    struct rrdengine_datafile **datafiles, *datafile;
    struct rrdengine_journalfile *journalfile;

    ret = uv_fs_scandir(NULL, &req, ctx->config.dbfiles_path, 0, NULL);
    if (ret < 0) {
        fatal_assert(req.result < 0);
        uv_fs_req_cleanup(&req);
        netdata_log_error("DBENGINE: uv_fs_scandir(%s): %s", ctx->config.dbfiles_path, uv_strerror(ret));
        ctx_fs_error(ctx);
        return ret;
    }
    netdata_log_info("DBENGINE: found %d files in path %s", ret, ctx->config.dbfiles_path);

    Pvoid_t datafiles_JudyL = NULL;
    Pvoid_t journafile_JudyL = NULL;
    datafiles = callocz(MIN(ret, MAX_DATAFILES), sizeof(*datafiles));
    bool validate_files = true;
    for (matched_files = 0 ; UV_EOF != uv_fs_scandir_next(&req, &dent) && matched_files < MAX_DATAFILES ; ) {
        ret = sscanf(dent.name, DATAFILE_PREFIX RRDENG_FILE_NUMBER_SCAN_TMPL DATAFILE_EXTENSION, &tier, &fileno);

        // This is a datafile
        if (2 == ret) {
            datafile = datafile_alloc_and_init(ctx, tier, fileno);
            datafiles[matched_files++] = datafile;
            Pvoid_t *Pvalue = JudyLIns(&datafiles_JudyL, (Word_t)fileno, PJE0);
            if (!Pvalue || Pvalue == PJERR)
                validate_files = false;
            continue;
        }

        // Check for journal v1 or v2
        char expected_name[RRDENG_PATH_MAX];
        ret = sscanf(dent.name, WALFILE_PREFIX RRDENG_FILE_NUMBER_SCAN_TMPL WALFILE_EXTENSION, &tier, &fileno);
        bool unknown_file = true;
        if (2 == ret) {
            (void) snprintfz(expected_name, sizeof(expected_name), WALFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL WALFILE_EXTENSION,
                            1U, fileno);

            unknown_file = (strcmp(dent.name, expected_name) != 0);
            if (unknown_file) {
                (void) snprintfz(expected_name, sizeof(expected_name), WALFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL WALFILE_EXTENSION_V2,
                                1U, fileno);
                unknown_file = (strcmp(dent.name, expected_name) != 0);
            }

            if (!unknown_file)
                (void) JudyLIns(&journafile_JudyL, (Word_t)fileno, PJE0);
        }

        if (unknown_file)
            nd_log_daemon(NDLP_WARNING, "Unknown file detected : \"%s/%s\"", ctx->config.dbfiles_path, dent.name);
    }
    uv_fs_req_cleanup(&req);

    if (0 == matched_files) {
        freez(datafiles);
        return 0;
    }

    if (matched_files == MAX_DATAFILES)
        netdata_log_error("DBENGINE: warning: hit maximum database engine file limit of %d files", MAX_DATAFILES);

    qsort(datafiles, matched_files, sizeof(*datafiles), scan_data_files_cmp);

    ctx->atomic.last_fileno = datafiles[matched_files - 1]->fileno;

    // Remove journal files that do not have a matching data file
    // by scanning the judy array of the journal files
    if (validate_files) {
        bool first_then_next = true;
        Word_t idx = 0;
        Pvoid_t *PValue;
        size_t deleted_journals = 0;
        while ((PValue = JudyLFirstThenNext(journafile_JudyL, &idx, &first_then_next))) {
            char path[RRDENG_PATH_MAX];
            if (unlikely(!JudyLGet(datafiles_JudyL, (Word_t)idx, PJE0))) {
                (void)snprintfz(
                    path,
                    sizeof(path),
                    "%s/" WALFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL WALFILE_EXTENSION,
                    datafile_ctx(datafile)->config.dbfiles_path,
                    1U,
                    (unsigned)idx);

                UNLINK_FILE(ctx, path, ret);
                if (ret == 0) {
                    netdata_log_info("DBENGINE: deleting journal file without matching data file: %s", path);
                    __atomic_add_fetch(&ctx->stats.journalfile_deletions, 1, __ATOMIC_RELAXED);
                    deleted_journals++;
                }

                (void)snprintfz(
                    path,
                    sizeof(path),
                    "%s/" WALFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL WALFILE_EXTENSION_V2,
                    datafile_ctx(datafile)->config.dbfiles_path,
                    1U,
                    (unsigned)idx);

                UNLINK_FILE(ctx, path, ret);
                if (ret == 0) {
                    netdata_log_info("DBENGINE: deleting journal file without matching data file: %s", path);
                    __atomic_add_fetch(&ctx->stats.journalfile_deletions, 1, __ATOMIC_RELAXED);
                    deleted_journals++;
                }
            }
        }

        if (deleted_journals)
            netdata_log_info("DBENGINE: deleted %zu journal files without matching data files", deleted_journals);
    }

    (void) JudyLFreeArray(&journafile_JudyL, NULL);
    (void) JudyLFreeArray(&datafiles_JudyL, NULL);


    netdata_log_info("DBENGINE: loading %d data/journal of tier %d...", matched_files, ctx->config.tier);
    for (failed_to_load = 0, i = 0 ; i < matched_files ; ++i) {
        uint8_t must_delete_pair = 0;

        datafile = datafiles[i];
        ret = load_data_file(datafile);
        if (0 != ret)
            must_delete_pair = 1;

        journalfile = journalfile_alloc_and_init(datafile);
        ret = journalfile_load(ctx, journalfile, datafile);
        if (0 != ret) {
            if (!must_delete_pair) /* If datafile is still open close it */
                close_data_file(datafile);
            must_delete_pair = 1;
        }

        if (must_delete_pair) {
            char path[RRDENG_PATH_MAX];

            netdata_log_error("DBENGINE: deleting invalid data and journal file pair.");
            ret = journalfile_unlink(journalfile);
            if (!ret) {
                journalfile_v1_generate_path(datafile, path, sizeof(path));
                netdata_log_info("DBENGINE: deleted journal file \"%s\".", path);
            }
            ret = unlink_data_file(datafile);
            if (!ret) {
                generate_datafilepath(datafile, path, sizeof(path));
                netdata_log_info("DBENGINE: deleted data file \"%s\".", path);
            }
            freez(journalfile);
            freez(datafile);
            ++failed_to_load;
            continue;
        }

        ctx_current_disk_space_increase(ctx, datafile->pos + journalfile->unsafe.pos);
        datafile_list_insert(ctx, datafile);
    }

    matched_files -= failed_to_load;
    freez(datafiles);

    return matched_files;
}

/* Creates a datafile and a journalfile pair */
int create_new_datafile_pair(struct rrdengine_instance *ctx)
{
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.datafile_creation_started, 1, __ATOMIC_RELAXED);

    struct rrdengine_datafile *datafile;
    struct rrdengine_journalfile *journalfile;
    unsigned fileno = ctx_last_fileno_get(ctx) + 1;
    int ret;
    char path[RRDENG_PATH_MAX];

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "DBENGINE: creating new data and journal files in path \"%s\"",
           ctx->config.dbfiles_path);

    datafile = datafile_alloc_and_init(ctx, 1, fileno);
    ret = create_data_file(datafile);
    if(ret)
        goto error_after_datafile;

    generate_datafilepath(datafile, path, sizeof(path));
    nd_log(NDLS_DAEMON, NDLP_INFO, "DBENGINE: created data file \"%s\".", path);

    journalfile = journalfile_alloc_and_init(datafile);
    ret = journalfile_create(journalfile, datafile);
    if (ret)
        goto error_after_journalfile;

    journalfile_v1_generate_path(datafile, path, sizeof(path));
    nd_log(NDLS_DAEMON, NDLP_INFO, "DBENGINE: created journal file \"%s\".", path);

    ctx_current_disk_space_increase(ctx, datafile->pos + journalfile->unsafe.pos);
    datafile_list_insert(ctx, datafile);
    ctx_last_fileno_increment(ctx);

    return 0;

error_after_journalfile:
    (void) destroy_data_file_unsafe(datafile);
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

    ret = scan_data_files(ctx);
    if (ret < 0) {
        netdata_log_error("DBENGINE: failed to scan path \"%s\".", ctx->config.dbfiles_path);
        return ret;
    } else if (0 == ret) {
        netdata_log_info("DBENGINE: data files not found, creating in path \"%s\".", ctx->config.dbfiles_path);
        ctx->atomic.last_fileno = 0;
        ret = create_new_datafile_pair(ctx);
        if (ret) {
            netdata_log_error("DBENGINE: failed to create data and journal files in path \"%s\".", ctx->config.dbfiles_path);
            return ret;
        }
    }
    else {
        if (ctx->loading.create_new_datafile_pair)
            create_new_datafile_pair(ctx);

        while(rrdeng_ctx_tier_cap_exceeded(ctx)) {
            Word_t Index = 0;
            Pvoid_t *PValue = JudyLFirst(ctx->datafiles.JudyL, &Index, PJE0);
            if (PValue && *PValue) {
                struct rrdengine_datafile *datafile = *PValue;
                datafile_delete(ctx, datafile, false, true, false);
            }
        }
    }

    pgc_reset_hot_max(open_cache);
    ctx->loading.create_new_datafile_pair = false;
    return 0;
}

void cleanup_datafile_epdl_structures(struct rrdengine_datafile *datafile)
{
    rw_spinlock_write_lock(&datafile->extent_epdl.spinlock);
    bool first = true;
    Word_t idx = 0;
    Pvoid_t *PValue;
    while ((PValue = JudyLFirstThenNext(datafile->extent_epdl.epdl_per_extent, &idx, &first))) {
        EPDL_EXTENT *e = *PValue;
        internal_error(e->base, "DBENGINE: unexpected active EPDLs during datafile cleanup");
        epdl_extent_release(e);
        *PValue = NULL;
    }
    JudyLFreeArray(&datafile->extent_epdl.epdl_per_extent, PJE0);
    rw_spinlock_write_unlock(&datafile->extent_epdl.spinlock);
}

void finalize_data_files(struct rrdengine_instance *ctx)
{
    bool logged = false;

    if (!ctx->datafiles.JudyL)
        return;

    while(__atomic_load_n(&ctx->atomic.extents_currently_being_flushed, __ATOMIC_RELAXED)) {
        if(!logged) {
            netdata_log_info("Waiting for inflight flush to finish on tier %d...", ctx->config.tier);
            logged = true;
        }
        sleep_usec(100 * USEC_PER_MS);
    }

    bool first_then_next = true;
    Pvoid_t *PValue;
    Word_t Index = 0;

    while ((PValue = JudyLFirstThenNext(ctx->datafiles.JudyL, &Index, &first_then_next))) {
        struct rrdengine_datafile *datafile = *PValue;
        struct rrdengine_journalfile *journalfile = datafile->journalfile;

        logged = false;
        size_t iterations = 10;
        while(!datafile_acquire_for_deletion(datafile, true) && --iterations > 0) {
            if(!logged) {
                netdata_log_info("Waiting to acquire data file %u of tier %d to close it...", datafile->fileno, ctx->config.tier);
                logged = true;
            }
            sleep_usec(100 * USEC_PER_MS);
        }

        logged = false;
        bool available = false;
        do {
            netdata_rwlock_wrlock(&ctx->datafiles.rwlock);
            spinlock_lock(&datafile->writers.spinlock);
            available = (datafile->writers.running || datafile->writers.flushed_to_open_running) ? false : true;

            if(!available) {
                spinlock_unlock(&datafile->writers.spinlock);
                netdata_rwlock_wrunlock(&ctx->datafiles.rwlock);
                if(!logged) {
                    netdata_log_info("Waiting for writers to data file %u of tier %d to finish...", datafile->fileno, ctx->config.tier);
                    logged = true;
                }
                sleep_usec(100 * USEC_PER_MS);
            }
        } while(!available);

        journalfile_close(journalfile, datafile);
        close_data_file(datafile);
        datafile_list_delete_unsafe(ctx, datafile);
        spinlock_unlock(&datafile->writers.spinlock);
        netdata_rwlock_wrunlock(&ctx->datafiles.rwlock);

        // Clean up EPDL_EXTENT structures
        cleanup_datafile_epdl_structures(datafile);

        memset(journalfile, 0, sizeof(*journalfile));
        memset(datafile, 0, sizeof(*datafile));

        freez(journalfile);
        freez(datafile);
    }
}
