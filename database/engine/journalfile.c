// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"


// DBENGINE2: Helper

static void update_metric_retention_and_granularity_by_uuid(
        struct rrdengine_instance *ctx, uuid_t *uuid,
        time_t first_time_s, time_t last_time_s,
        time_t update_every_s, time_t now_s)
{
    if(unlikely(last_time_s > now_s)) {
        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "DBENGINE JV2: wrong last time on-disk (%ld - %ld, now %ld), "
                          "fixing last time to now",
                    first_time_s, last_time_s, now_s);
        last_time_s = now_s;
    }

    if (unlikely(first_time_s > last_time_s)) {
        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "DBENGINE JV2: wrong first time on-disk (%ld - %ld, now %ld), "
                          "fixing first time to last time",
                    first_time_s, last_time_s, now_s);

        first_time_s = last_time_s;
    }

    if (unlikely(first_time_s == 0 || last_time_s == 0)) {
        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "DBENGINE JV2: zero on-disk timestamps (%ld - %ld, now %ld), "
                          "using them as-is",
                    first_time_s, last_time_s, now_s);
    }

    bool added = false;
    METRIC *metric = mrg_metric_get_and_acquire(main_mrg, uuid, (Word_t) ctx);
    if (!metric) {
        MRG_ENTRY entry = {
                .section = (Word_t) ctx,
                .first_time_s = first_time_s,
                .last_time_s = last_time_s,
                .latest_update_every_s = (uint32_t) update_every_s
        };
        uuid_copy(entry.uuid, *uuid);
        metric = mrg_metric_add_and_acquire(main_mrg, entry, &added);
    }

    if (likely(!added))
        mrg_metric_expand_retention(main_mrg, metric, first_time_s, last_time_s, update_every_s);

    mrg_metric_release(main_mrg, metric);
}

static void after_extent_write_journalfile_v1_io(uv_fs_t* req)
{
    worker_is_busy(RRDENG_FLUSH_TRANSACTION_BUFFER_CB);

    WAL *wal = req->data;
    struct generic_io_descriptor *io_descr = &wal->io_descr;
    struct rrdengine_instance *ctx = io_descr->ctx;

    debug(D_RRDENGINE, "%s: Journal block was written to disk.", __func__);
    if (req->result < 0) {
        ctx_io_error(ctx);
        error("DBENGINE: %s: uv_fs_write: %s", __func__, uv_strerror((int)req->result));
    } else {
        debug(D_RRDENGINE, "%s: Journal block was written to disk.", __func__);
    }

    uv_fs_req_cleanup(req);
    wal_release(wal);

    __atomic_sub_fetch(&ctx->atomic.extents_currently_being_flushed, 1, __ATOMIC_RELAXED);

    worker_is_idle();
}

/* Careful to always call this before creating a new journal file */
void journalfile_v1_extent_write(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile, WAL *wal, uv_loop_t *loop)
{
    int ret;
    struct generic_io_descriptor *io_descr;
    struct rrdengine_journalfile *journalfile = datafile->journalfile;

    io_descr = &wal->io_descr;
    io_descr->ctx = ctx;
    if (wal->size < wal->buf_size) {
        /* simulate an empty transaction to skip the rest of the block */
        *(uint8_t *) (wal->buf + wal->size) = STORE_PADDING;
    }
    io_descr->buf = wal->buf;
    io_descr->bytes = wal->buf_size;

    netdata_spinlock_lock(&journalfile->unsafe.spinlock);
    io_descr->pos = journalfile->unsafe.pos;
    journalfile->unsafe.pos += wal->buf_size;
    netdata_spinlock_unlock(&journalfile->unsafe.spinlock);

    io_descr->req.data = wal;
    io_descr->data = journalfile;
    io_descr->completion = NULL;

    io_descr->iov = uv_buf_init((void *)io_descr->buf, wal->buf_size);
    ret = uv_fs_write(loop, &io_descr->req, journalfile->file, &io_descr->iov, 1,
                      (int64_t)io_descr->pos, after_extent_write_journalfile_v1_io);
    fatal_assert(-1 != ret);

    ctx_current_disk_space_increase(ctx, wal->buf_size);
    ctx_io_write_op_bytes(ctx, wal->buf_size);
}

void journalfile_v2_generate_path(struct rrdengine_datafile *datafile, char *str, size_t maxlen)
{
    (void) snprintfz(str, maxlen, "%s/" WALFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL WALFILE_EXTENSION_V2,
                    datafile->ctx->config.dbfiles_path, datafile->tier, datafile->fileno);
}

void journalfile_v1_generate_path(struct rrdengine_datafile *datafile, char *str, size_t maxlen)
{
    (void) snprintfz(str, maxlen, "%s/" WALFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL WALFILE_EXTENSION,
                    datafile->ctx->config.dbfiles_path, datafile->tier, datafile->fileno);
}

static struct journal_v2_header *journalfile_v2_mounted_data_get(struct rrdengine_journalfile *journalfile, size_t *data_size) {
    struct journal_v2_header *j2_header = NULL;

    netdata_spinlock_lock(&journalfile->mmap.spinlock);

    if(!journalfile->mmap.data) {
        journalfile->mmap.data = mmap(NULL, journalfile->mmap.size, PROT_READ, MAP_SHARED, journalfile->mmap.fd, 0);
        if (journalfile->mmap.data == MAP_FAILED) {
            internal_fatal(true, "DBENGINE: failed to re-mmap() journal file v2");
            close(journalfile->mmap.fd);
            journalfile->mmap.fd = -1;
            journalfile->mmap.data = NULL;
            journalfile->mmap.size = 0;

            netdata_spinlock_lock(&journalfile->v2.spinlock);
            journalfile->v2.flags &= ~(JOURNALFILE_FLAG_IS_AVAILABLE | JOURNALFILE_FLAG_IS_MOUNTED);
            netdata_spinlock_unlock(&journalfile->v2.spinlock);

            ctx_fs_error(journalfile->datafile->ctx);
        }
        else {
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.journal_v2_mapped, 1, __ATOMIC_RELAXED);

            madvise_dontfork(journalfile->mmap.data, journalfile->mmap.size);
            madvise_dontdump(journalfile->mmap.data, journalfile->mmap.size);
            madvise_random(journalfile->mmap.data, journalfile->mmap.size);
            madvise_dontneed(journalfile->mmap.data, journalfile->mmap.size);

            netdata_spinlock_lock(&journalfile->v2.spinlock);
            journalfile->v2.flags |= JOURNALFILE_FLAG_IS_AVAILABLE | JOURNALFILE_FLAG_IS_MOUNTED;
            netdata_spinlock_unlock(&journalfile->v2.spinlock);
        }
    }

    if(journalfile->mmap.data) {
        j2_header = journalfile->mmap.data;

        if (data_size)
            *data_size = journalfile->mmap.size;
    }

    netdata_spinlock_unlock(&journalfile->mmap.spinlock);

    return j2_header;
}

static bool journalfile_v2_mounted_data_unmount(struct rrdengine_journalfile *journalfile, bool have_locks, bool wait) {
    bool unmounted = false;

    if(!have_locks) {
        if(!wait) {
            if (!netdata_spinlock_trylock(&journalfile->mmap.spinlock))
                return false;
        }
        else
            netdata_spinlock_lock(&journalfile->mmap.spinlock);

        if(!wait) {
            if(!netdata_spinlock_trylock(&journalfile->v2.spinlock)) {
                netdata_spinlock_unlock(&journalfile->mmap.spinlock);
                return false;
            }
        }
        else
            netdata_spinlock_lock(&journalfile->v2.spinlock);
    }

    if(!journalfile->v2.refcount) {
        if(journalfile->mmap.data) {
            if (munmap(journalfile->mmap.data, journalfile->mmap.size)) {
                char path[RRDENG_PATH_MAX];
                journalfile_v2_generate_path(journalfile->datafile, path, sizeof(path));
                error("DBENGINE: failed to unmap index file '%s'", path);
                internal_fatal(true, "DBENGINE: failed to unmap file '%s'", path);
                ctx_fs_error(journalfile->datafile->ctx);
            }
            else {
                __atomic_add_fetch(&rrdeng_cache_efficiency_stats.journal_v2_unmapped, 1, __ATOMIC_RELAXED);
                journalfile->mmap.data = NULL;
                journalfile->v2.flags &= ~JOURNALFILE_FLAG_IS_MOUNTED;
            }
        }

        unmounted = true;
    }

    if(!have_locks) {
        netdata_spinlock_unlock(&journalfile->v2.spinlock);
        netdata_spinlock_unlock(&journalfile->mmap.spinlock);
    }

    return unmounted;
}

void journalfile_v2_data_unmount_cleanup(time_t now_s) {
    // DO NOT WAIT ON ANY LOCK!!!

    for(size_t tier = 0; tier < (size_t)storage_tiers ;tier++) {
        struct rrdengine_instance *ctx = multidb_ctx[tier];
        if(!ctx) continue;

        struct rrdengine_datafile *datafile;
        if(uv_rwlock_tryrdlock(&ctx->datafiles.rwlock) != 0)
            continue;

        for (datafile = ctx->datafiles.first; datafile; datafile = datafile->next) {
            struct rrdengine_journalfile *journalfile = datafile->journalfile;

            if(!netdata_spinlock_trylock(&journalfile->v2.spinlock))
                continue;

            bool unmount = false;
            if (!journalfile->v2.refcount && (journalfile->v2.flags & JOURNALFILE_FLAG_IS_MOUNTED)) {
                // this journal has no references and it is mounted

                if (!journalfile->v2.not_needed_since_s)
                    journalfile->v2.not_needed_since_s = now_s;

                else if (now_s - journalfile->v2.not_needed_since_s >= 120)
                    // 2 minutes have passed since last use
                    unmount = true;
            }
            netdata_spinlock_unlock(&journalfile->v2.spinlock);

            if (unmount)
                journalfile_v2_mounted_data_unmount(journalfile, false, false);
        }
        uv_rwlock_rdunlock(&ctx->datafiles.rwlock);
    }
}

struct journal_v2_header *journalfile_v2_data_acquire(struct rrdengine_journalfile *journalfile, size_t *data_size, time_t wanted_first_time_s, time_t wanted_last_time_s) {
    netdata_spinlock_lock(&journalfile->v2.spinlock);

    bool has_data = (journalfile->v2.flags & JOURNALFILE_FLAG_IS_AVAILABLE);
    bool is_mounted = (journalfile->v2.flags & JOURNALFILE_FLAG_IS_MOUNTED);
    bool do_we_need_it = false;

    if(has_data) {
        if (!wanted_first_time_s || !wanted_last_time_s ||
            is_page_in_time_range(journalfile->v2.first_time_s, journalfile->v2.last_time_s,
                                  wanted_first_time_s, wanted_last_time_s) == PAGE_IS_IN_RANGE) {

            journalfile->v2.refcount++;

            do_we_need_it = true;

            if (!wanted_first_time_s && !wanted_last_time_s && !is_mounted)
                journalfile->v2.flags |= JOURNALFILE_FLAG_MOUNTED_FOR_RETENTION;
            else
                journalfile->v2.flags &= ~JOURNALFILE_FLAG_MOUNTED_FOR_RETENTION;

        }
    }
    netdata_spinlock_unlock(&journalfile->v2.spinlock);

    if(do_we_need_it)
        return journalfile_v2_mounted_data_get(journalfile, data_size);

    return NULL;
}

void journalfile_v2_data_release(struct rrdengine_journalfile *journalfile) {
    netdata_spinlock_lock(&journalfile->v2.spinlock);

    internal_fatal(!journalfile->mmap.data, "trying to release a journalfile without data");
    internal_fatal(journalfile->v2.refcount < 1, "trying to release a non-acquired journalfile");

    bool unmount = false;

    journalfile->v2.refcount--;

    if(journalfile->v2.refcount == 0) {
        journalfile->v2.not_needed_since_s = 0;

        if(journalfile->v2.flags & JOURNALFILE_FLAG_MOUNTED_FOR_RETENTION)
            unmount = true;
    }
    netdata_spinlock_unlock(&journalfile->v2.spinlock);

    if(unmount)
        journalfile_v2_mounted_data_unmount(journalfile, false, true);
}

bool journalfile_v2_data_available(struct rrdengine_journalfile *journalfile) {

    netdata_spinlock_lock(&journalfile->v2.spinlock);
    bool has_data = (journalfile->v2.flags & JOURNALFILE_FLAG_IS_AVAILABLE);
    netdata_spinlock_unlock(&journalfile->v2.spinlock);

    return has_data;
}

size_t journalfile_v2_data_size_get(struct rrdengine_journalfile *journalfile) {

    netdata_spinlock_lock(&journalfile->mmap.spinlock);
    size_t data_size = journalfile->mmap.size;
    netdata_spinlock_unlock(&journalfile->mmap.spinlock);

    return data_size;
}

void journalfile_v2_data_set(struct rrdengine_journalfile *journalfile, int fd, void *journal_data, uint32_t journal_data_size) {
    netdata_spinlock_lock(&journalfile->mmap.spinlock);
    netdata_spinlock_lock(&journalfile->v2.spinlock);

    internal_fatal(journalfile->mmap.fd != -1, "DBENGINE JOURNALFILE: trying to re-set journal fd");
    internal_fatal(journalfile->mmap.data, "DBENGINE JOURNALFILE: trying to re-set journal_data");
    internal_fatal(journalfile->v2.refcount, "DBENGINE JOURNALFILE: trying to re-set journal_data of referenced journalfile");

    journalfile->mmap.fd = fd;
    journalfile->mmap.data = journal_data;
    journalfile->mmap.size = journal_data_size;
    journalfile->v2.not_needed_since_s = now_monotonic_sec();
    journalfile->v2.flags |= JOURNALFILE_FLAG_IS_AVAILABLE | JOURNALFILE_FLAG_IS_MOUNTED;

    struct journal_v2_header *j2_header = journalfile->mmap.data;
    journalfile->v2.first_time_s = (time_t)(j2_header->start_time_ut / USEC_PER_SEC);
    journalfile->v2.last_time_s = (time_t)(j2_header->end_time_ut / USEC_PER_SEC);

    journalfile_v2_mounted_data_unmount(journalfile, true, true);

    netdata_spinlock_unlock(&journalfile->v2.spinlock);
    netdata_spinlock_unlock(&journalfile->mmap.spinlock);
}

static void journalfile_v2_data_unmap_permanently(struct rrdengine_journalfile *journalfile) {
    bool has_references = false;

    do {
        if (has_references)
            sleep_usec(10 * USEC_PER_MS);

        netdata_spinlock_lock(&journalfile->mmap.spinlock);
        netdata_spinlock_lock(&journalfile->v2.spinlock);

        if(journalfile_v2_mounted_data_unmount(journalfile, true, true)) {
            if(journalfile->mmap.fd != -1)
                close(journalfile->mmap.fd);

            journalfile->mmap.fd = -1;
            journalfile->mmap.data = NULL;
            journalfile->mmap.size = 0;
            journalfile->v2.first_time_s = 0;
            journalfile->v2.last_time_s = 0;
            journalfile->v2.flags = 0;
        }
        else {
            has_references = true;
            internal_error(true, "DBENGINE JOURNALFILE: waiting for journalfile to be available to unmap...");
        }

        netdata_spinlock_unlock(&journalfile->v2.spinlock);
        netdata_spinlock_unlock(&journalfile->mmap.spinlock);

    } while(has_references);
}

struct rrdengine_journalfile *journalfile_alloc_and_init(struct rrdengine_datafile *datafile)
{
    struct rrdengine_journalfile *journalfile = callocz(1, sizeof(struct rrdengine_journalfile));
    journalfile->datafile = datafile;
    netdata_spinlock_init(&journalfile->mmap.spinlock);
    netdata_spinlock_init(&journalfile->v2.spinlock);
    netdata_spinlock_init(&journalfile->unsafe.spinlock);
    journalfile->mmap.fd = -1;
    datafile->journalfile = journalfile;
    return journalfile;
}

static int close_uv_file(struct rrdengine_datafile *datafile, uv_file file)
{
    int ret;
    char path[RRDENG_PATH_MAX];

    uv_fs_t req;
    ret = uv_fs_close(NULL, &req, file, NULL);
    if (ret < 0) {
        journalfile_v1_generate_path(datafile, path, sizeof(path));
        error("DBENGINE: uv_fs_close(%s): %s", path, uv_strerror(ret));
        ctx_fs_error(datafile->ctx);
    }
    uv_fs_req_cleanup(&req);
    return ret;
}

int journalfile_close(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile)
{
    if(journalfile_v2_data_available(journalfile)) {
        journalfile_v2_data_unmap_permanently(journalfile);
        return 0;
    }

    return close_uv_file(datafile, journalfile->file);
}

int journalfile_unlink(struct rrdengine_journalfile *journalfile)
{
    struct rrdengine_datafile *datafile = journalfile->datafile;
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    journalfile_v1_generate_path(datafile, path, sizeof(path));

    ret = uv_fs_unlink(NULL, &req, path, NULL);
    if (ret < 0) {
        error("DBENGINE: uv_fs_fsunlink(%s): %s", path, uv_strerror(ret));
        ctx_fs_error(ctx);
    }
    uv_fs_req_cleanup(&req);

    __atomic_add_fetch(&ctx->stats.journalfile_deletions, 1, __ATOMIC_RELAXED);

    return ret;
}

int journalfile_destroy_unsafe(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];
    char path_v2[RRDENG_PATH_MAX];

    journalfile_v1_generate_path(datafile, path, sizeof(path));
    journalfile_v2_generate_path(datafile, path_v2, sizeof(path));

    if (journalfile->file) {
    ret = uv_fs_ftruncate(NULL, &req, journalfile->file, 0, NULL);
    if (ret < 0) {
        error("DBENGINE: uv_fs_ftruncate(%s): %s", path, uv_strerror(ret));
        ctx_fs_error(ctx);
    }
    uv_fs_req_cleanup(&req);
        (void) close_uv_file(datafile, journalfile->file);
    }

    // This is the new journal v2 index file
    ret = uv_fs_unlink(NULL, &req, path_v2, NULL);
    if (ret < 0) {
        error("DBENGINE: uv_fs_fsunlink(%s): %s", path, uv_strerror(ret));
        ctx_fs_error(ctx);
    }
    uv_fs_req_cleanup(&req);

    ret = uv_fs_unlink(NULL, &req, path, NULL);
    if (ret < 0) {
        error("DBENGINE: uv_fs_fsunlink(%s): %s", path, uv_strerror(ret));
        ctx_fs_error(ctx);
    }
    uv_fs_req_cleanup(&req);

    __atomic_add_fetch(&ctx->stats.journalfile_deletions, 2, __ATOMIC_RELAXED);

    if(journalfile_v2_data_available(journalfile))
        journalfile_v2_data_unmap_permanently(journalfile);

    return ret;
}

int journalfile_create(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile)
{
    struct rrdengine_instance *ctx = datafile->ctx;
    uv_fs_t req;
    uv_file file;
    int ret, fd;
    struct rrdeng_jf_sb *superblock;
    uv_buf_t iov;
    char path[RRDENG_PATH_MAX];

    journalfile_v1_generate_path(datafile, path, sizeof(path));
    fd = open_file_for_io(path, O_CREAT | O_RDWR | O_TRUNC, &file, use_direct_io);
    if (fd < 0) {
        ctx_fs_error(ctx);
        return fd;
    }
    journalfile->file = file;
    __atomic_add_fetch(&ctx->stats.journalfile_creations, 1, __ATOMIC_RELAXED);

    ret = posix_memalign((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    if (unlikely(ret)) {
        fatal("DBENGINE: posix_memalign:%s", strerror(ret));
    }
    memset(superblock, 0, sizeof(*superblock));
    (void) strncpy(superblock->magic_number, RRDENG_JF_MAGIC, RRDENG_MAGIC_SZ);
    (void) strncpy(superblock->version, RRDENG_JF_VER, RRDENG_VER_SZ);

    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_write(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        fatal_assert(req.result < 0);
        error("DBENGINE: uv_fs_write: %s", uv_strerror(ret));
        ctx_io_error(ctx);
    }
    uv_fs_req_cleanup(&req);
    posix_memfree(superblock);
    if (ret < 0) {
        journalfile_destroy_unsafe(journalfile, datafile);
        return ret;
    }

    journalfile->unsafe.pos = sizeof(*superblock);

    ctx_io_write_op_bytes(ctx, sizeof(*superblock));

    return 0;
}

static int journalfile_check_superblock(uv_file file)
{
    int ret;
    struct rrdeng_jf_sb *superblock;
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

    if (strncmp(superblock->magic_number, RRDENG_JF_MAGIC, RRDENG_MAGIC_SZ) ||
        strncmp(superblock->version, RRDENG_JF_VER, RRDENG_VER_SZ)) {
        error("DBENGINE: File has invalid superblock.");
        ret = UV_EINVAL;
    } else {
        ret = 0;
    }
    error:
    posix_memfree(superblock);
    return ret;
}

static void journalfile_restore_extent_metadata(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile, void *buf, unsigned max_size)
{
    static BITMAP256 page_error_map;
    unsigned i, count, payload_length, descr_size;
    struct rrdeng_jf_store_data *jf_metric_data;

    jf_metric_data = buf;
    count = jf_metric_data->number_of_pages;
    descr_size = sizeof(*jf_metric_data->descr) * count;
    payload_length = sizeof(*jf_metric_data) + descr_size;
    if (payload_length > max_size) {
        error("DBENGINE: corrupted transaction payload.");
        return;
    }

    time_t now_s = max_acceptable_collected_time();
    for (i = 0; i < count ; ++i) {
        uuid_t *temp_id;
        uint8_t page_type = jf_metric_data->descr[i].type;

        if (page_type > PAGE_TYPE_MAX) {
            if (!bitmap256_get_bit(&page_error_map, page_type)) {
                error("DBENGINE: unknown page type %d encountered.", page_type);
                bitmap256_set_bit(&page_error_map, page_type, 1);
            }
            continue;
        }

        temp_id = (uuid_t *)jf_metric_data->descr[i].uuid;
        METRIC *metric = mrg_metric_get_and_acquire(main_mrg, temp_id, (Word_t) ctx);

        struct rrdeng_extent_page_descr *descr = &jf_metric_data->descr[i];
        VALIDATED_PAGE_DESCRIPTOR vd = validate_extent_page_descr(
                descr, now_s,
                (metric) ? mrg_metric_get_update_every_s(main_mrg, metric) : 0,
                false);

        if(!vd.is_valid) {
            if(metric)
                mrg_metric_release(main_mrg, metric);

            continue;
        }

        bool update_metric_time = true;
        if (!metric) {
            MRG_ENTRY entry = {
                    .section = (Word_t)ctx,
                    .first_time_s = vd.start_time_s,
                    .last_time_s = vd.end_time_s,
                    .latest_update_every_s = (uint32_t) vd.update_every_s,
            };
            uuid_copy(entry.uuid, *temp_id);

            bool added;
            metric = mrg_metric_add_and_acquire(main_mrg, entry, &added);
            if(added)
                update_metric_time = false;
        }
        Word_t metric_id = mrg_metric_id(main_mrg, metric);

        if (update_metric_time)
            mrg_metric_expand_retention(main_mrg, metric, vd.start_time_s, vd.end_time_s, vd.update_every_s);

        pgc_open_add_hot_page(
                (Word_t)ctx, metric_id, vd.start_time_s, vd.end_time_s, vd.update_every_s,
                journalfile->datafile,
                jf_metric_data->extent_offset, jf_metric_data->extent_size, jf_metric_data->descr[i].page_length);

        mrg_metric_release(main_mrg, metric);
    }
}

/*
 * Replays transaction by interpreting up to max_size bytes from buf.
 * Sets id to the current transaction id or to 0 if unknown.
 * Returns size of transaction record or 0 for unknown size.
 */
static unsigned journalfile_replay_transaction(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile,
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
        error("DBENGINE: corrupted transaction record, skipping.");
        return 0;
    }
    *id = jf_header->id;
    payload_length = jf_header->payload_length;
    size_bytes = sizeof(*jf_header) + payload_length + sizeof(*jf_trailer);
    if (size_bytes > max_size) {
        error("DBENGINE: corrupted transaction record, skipping.");
        return 0;
    }
    jf_trailer = buf + sizeof(*jf_header) + payload_length;
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, buf, sizeof(*jf_header) + payload_length);
    ret = crc32cmp(jf_trailer->checksum, crc);
    debug(D_RRDENGINE, "Transaction %"PRIu64" was read from disk. CRC32 check: %s", *id, ret ? "FAILED" : "SUCCEEDED");
    if (unlikely(ret)) {
        error("DBENGINE: transaction %"PRIu64" was read from disk. CRC32 check: FAILED", *id);
        return size_bytes;
    }
    switch (jf_header->type) {
    case STORE_DATA:
        debug(D_RRDENGINE, "Replaying transaction %"PRIu64"", jf_header->id);
            journalfile_restore_extent_metadata(ctx, journalfile, buf + sizeof(*jf_header), payload_length);
        break;
    default:
        error("DBENGINE: unknown transaction type, skipping record.");
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
static uint64_t journalfile_iterate_transactions(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile)
{
    uv_file file;
    uint64_t file_size;
    int ret;
    uint64_t pos, pos_i, max_id, id;
    unsigned size_bytes;
    void *buf;
    uv_buf_t iov;
    uv_fs_t req;

    file = journalfile->file;
    file_size = journalfile->unsafe.pos;

    max_id = 1;
    ret = posix_memalign((void *)&buf, RRDFILE_ALIGNMENT, READAHEAD_BYTES);
    if (unlikely(ret))
        fatal("DBENGINE: posix_memalign:%s", strerror(ret));

    for (pos = sizeof(struct rrdeng_jf_sb); pos < file_size; pos += READAHEAD_BYTES) {
        size_bytes = MIN(READAHEAD_BYTES, file_size - pos);
        iov = uv_buf_init(buf, size_bytes);
        ret = uv_fs_read(NULL, &req, file, &iov, 1, pos, NULL);
        if (ret < 0) {
            error("DBENGINE: uv_fs_read: pos=%" PRIu64 ", %s", pos, uv_strerror(ret));
            uv_fs_req_cleanup(&req);
            goto skip_file;
        }
        fatal_assert(req.result >= 0);
        uv_fs_req_cleanup(&req);
        ctx_io_read_op_bytes(ctx, size_bytes);

        for (pos_i = 0; pos_i < size_bytes;) {
            unsigned max_size;

            max_size = pos + size_bytes - pos_i;
            ret = journalfile_replay_transaction(ctx, journalfile, buf + pos_i, &id, max_size);
            if (!ret) /* TODO: support transactions bigger than 4K */
                /* unknown transaction size, move on to the next block */
                pos_i = ALIGN_BYTES_FLOOR(pos_i + RRDENG_BLOCK_SIZE);
            else
                pos_i += ret;
            max_id = MAX(max_id, id);
        }
    }
skip_file:
    posix_memfree(buf);
    return max_id;
}

// Checks that the extent list checksum is valid
static int journalfile_check_v2_extent_list (void *data_start, size_t file_size)
{
    UNUSED(file_size);
    uLong crc;

    struct journal_v2_header *j2_header = (void *) data_start;
    struct journal_v2_block_trailer *journal_v2_trailer;

    journal_v2_trailer = (struct journal_v2_block_trailer *) ((uint8_t *) data_start + j2_header->extent_trailer_offset);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (uint8_t *) data_start + j2_header->extent_offset, j2_header->extent_count * sizeof(struct journal_extent_list));
    if (unlikely(crc32cmp(journal_v2_trailer->checksum, crc))) {
        error("DBENGINE: extent list CRC32 check: FAILED");
        return 1;
    }

    return 0;
}

// Checks that the metric list (UUIDs) checksum is valid
static int journalfile_check_v2_metric_list(void *data_start, size_t file_size)
{
    UNUSED(file_size);
    uLong crc;

    struct journal_v2_header *j2_header = (void *) data_start;
    struct journal_v2_block_trailer *journal_v2_trailer;

    journal_v2_trailer = (struct journal_v2_block_trailer *) ((uint8_t *) data_start + j2_header->metric_trailer_offset);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (uint8_t *) data_start + j2_header->metric_offset, j2_header->metric_count * sizeof(struct journal_metric_list));
    if (unlikely(crc32cmp(journal_v2_trailer->checksum, crc))) {
        error("DBENGINE: metric list CRC32 check: FAILED");
        return 1;
    }
    return 0;
}

//
// Return
//   0 Ok
//   1 Invalid
//   2 Force rebuild
//   3 skip

static int journalfile_v2_validate(void *data_start, size_t journal_v2_file_size, size_t journal_v1_file_size)
{
    int rc;
    uLong crc;

    struct journal_v2_header *j2_header = (void *) data_start;
    struct journal_v2_block_trailer *journal_v2_trailer;

    if (j2_header->magic == JOURVAL_V2_REBUILD_MAGIC)
        return 2;

    if (j2_header->magic == JOURVAL_V2_SKIP_MAGIC)
        return 3;

    // Magic failure
    if (j2_header->magic != JOURVAL_V2_MAGIC)
        return 1;

    if (j2_header->journal_v2_file_size != journal_v2_file_size)
        return 1;

    if (journal_v1_file_size && j2_header->journal_v1_file_size != journal_v1_file_size)
        return 1;

    journal_v2_trailer = (struct journal_v2_block_trailer *) ((uint8_t *) data_start + journal_v2_file_size - sizeof(*journal_v2_trailer));

    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (void *) j2_header, sizeof(*j2_header));

    rc = crc32cmp(journal_v2_trailer->checksum, crc);
    if (unlikely(rc)) {
        error("DBENGINE: file CRC32 check: FAILED");
        return 1;
    }

    rc = journalfile_check_v2_extent_list(data_start, journal_v2_file_size);
    if (rc) return 1;

    rc = journalfile_check_v2_metric_list(data_start, journal_v2_file_size);
    if (rc) return 1;

    if (!db_engine_journal_check)
        return 0;

    // Verify complete UUID chain

    struct journal_metric_list *metric = (void *) (data_start + j2_header->metric_offset);

    unsigned verified = 0;
    unsigned entries;
    unsigned total_pages = 0;

    info("DBENGINE: checking %u metrics that exist in the journal", j2_header->metric_count);
    for (entries = 0; entries < j2_header->metric_count; entries++) {

        char uuid_str[UUID_STR_LEN];
        uuid_unparse_lower(metric->uuid, uuid_str);
        struct journal_page_header *metric_list_header = (void *) (data_start + metric->page_offset);
        struct journal_page_header local_metric_list_header = *metric_list_header;

        local_metric_list_header.crc = JOURVAL_V2_MAGIC;

        crc = crc32(0L, Z_NULL, 0);
        crc = crc32(crc, (void *) &local_metric_list_header, sizeof(local_metric_list_header));
        rc = crc32cmp(metric_list_header->checksum, crc);

        if (!rc) {
            struct journal_v2_block_trailer *journal_trailer =
                (void *) data_start + metric->page_offset + sizeof(struct journal_page_header) + (metric_list_header->entries * sizeof(struct journal_page_list));

            crc = crc32(0L, Z_NULL, 0);
            crc = crc32(crc, (uint8_t *) metric_list_header + sizeof(struct journal_page_header), metric_list_header->entries * sizeof(struct journal_page_list));
            rc = crc32cmp(journal_trailer->checksum, crc);
            internal_error(rc, "DBENGINE: index %u : %s entries %u at offset %u verified, DATA CRC computed %lu, stored %u", entries, uuid_str, metric->entries, metric->page_offset,
                           crc, metric_list_header->crc);
            if (!rc) {
                total_pages += metric_list_header->entries;
                verified++;
            }
        }

        metric++;
        if ((uint32_t)((uint8_t *) metric - (uint8_t *) data_start) > (uint32_t) journal_v2_file_size) {
            info("DBENGINE: verification failed EOF reached -- total entries %u, verified %u", entries, verified);
            return 1;
        }
    }

    if (entries != verified) {
        info("DBENGINE: verification failed -- total entries %u, verified %u", entries, verified);
        return 1;
    }
    info("DBENGINE: verification succeeded -- total entries %u, verified %u (%u total pages)", entries, verified, total_pages);

    return 0;
}

static inline time_t get_metric_latest_update_every(struct journal_page_header *metric_list_header)
{
    struct journal_page_list *metric_page =
        (struct journal_page_list *)((uint8_t *)metric_list_header + sizeof(*metric_list_header));
    uint32_t entries = metric_list_header->entries;
    if (unlikely(!entries))
        return 0;
    return (time_t)metric_page[entries - 1].update_every_s;
}

void journalfile_v2_populate_retention_to_mrg(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile) {
    usec_t started_ut = now_monotonic_usec();

    size_t data_size = 0;
    struct journal_v2_header *j2_header = journalfile_v2_data_acquire(journalfile, &data_size, 0, 0);
    if(!j2_header)
        return;

    uint8_t *data_start = (uint8_t *)j2_header;
    uint32_t entries = j2_header->metric_count;

    struct journal_metric_list *metric = (struct journal_metric_list *) (data_start + j2_header->metric_offset);
    time_t header_start_time_s  = (time_t) (j2_header->start_time_ut / USEC_PER_SEC);
    time_t now_s = max_acceptable_collected_time();
    for (size_t i=0; i < entries; i++) {
        time_t start_time_s = header_start_time_s + metric->delta_start_s;
        time_t end_time_s = header_start_time_s + metric->delta_end_s;
        time_t update_every_s = get_metric_latest_update_every((struct journal_page_header *) (data_start + metric->page_offset));
        update_metric_retention_and_granularity_by_uuid(
                ctx, &metric->uuid, start_time_s, end_time_s, update_every_s, now_s);

#ifdef NETDATA_INTERNAL_CHECKS
        struct journal_page_header *metric_list_header = (void *) (data_start + metric->page_offset);
        fatal_assert(uuid_compare(metric_list_header->uuid, metric->uuid) == 0);
        fatal_assert(metric->entries == metric_list_header->entries);
#endif
        metric++;
    }

    journalfile_v2_data_release(journalfile);
    usec_t ended_ut = now_monotonic_usec();

    info("DBENGINE: journal v2 of tier %d, datafile %u populated, size: %0.2f MiB, metrics: %0.2f k, %0.2f ms"
        , ctx->config.tier, journalfile->datafile->fileno
        , (double)data_size / 1024 / 1024
        , (double)entries / 1000
        , ((double)(ended_ut - started_ut) / USEC_PER_MS)
        );
}

int journalfile_v2_load(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile)
{
    int ret, fd;
    char path_v1[RRDENG_PATH_MAX];
    char path_v2[RRDENG_PATH_MAX];
    struct stat statbuf;
    size_t journal_v1_file_size = 0;
    size_t journal_v2_file_size;

    journalfile_v1_generate_path(datafile, path_v1, sizeof(path_v1));
    ret = stat(path_v1, &statbuf);
    if (!ret)
        journal_v1_file_size = (uint32_t)statbuf.st_size;

    journalfile_v2_generate_path(datafile, path_v2, sizeof(path_v2));
    fd = open(path_v2, O_RDONLY);
    if (fd < 0) {
        if (errno == ENOENT)
            return 1;
        ctx_fs_error(ctx);
        error("DBENGINE: failed to open '%s'", path_v2);
        return 1;
    }

    ret = fstat(fd, &statbuf);
    if (ret) {
        error("DBENGINE: failed to get file information for '%s'", path_v2);
        close(fd);
        return 1;
    }

    journal_v2_file_size = (size_t)statbuf.st_size;

    if (journal_v2_file_size < sizeof(struct journal_v2_header)) {
        error_report("Invalid file %s. Not the expected size", path_v2);
        close(fd);
        return 1;
    }

    usec_t mmap_start_ut = now_monotonic_usec();
    uint8_t *data_start = mmap(NULL, journal_v2_file_size, PROT_READ, MAP_SHARED, fd, 0);
    if (data_start == MAP_FAILED) {
        close(fd);
        return 1;
    }

    info("DBENGINE: checking integrity of '%s'", path_v2);
    usec_t validation_start_ut = now_monotonic_usec();
    int rc = journalfile_v2_validate(data_start, journal_v2_file_size, journal_v1_file_size);
    if (unlikely(rc)) {
        if (rc == 2)
            error_report("File %s needs to be rebuilt", path_v2);
        else if (rc == 3)
            error_report("File %s will be skipped", path_v2);
        else
            error_report("File %s is invalid and it will be rebuilt", path_v2);

        if (unlikely(munmap(data_start, journal_v2_file_size)))
            error("DBENGINE: failed to unmap '%s'", path_v2);

        close(fd);
        return rc;
    }

    struct journal_v2_header *j2_header = (void *) data_start;
    uint32_t entries = j2_header->metric_count;

    if (unlikely(!entries)) {
        if (unlikely(munmap(data_start, journal_v2_file_size)))
            error("DBENGINE: failed to unmap '%s'", path_v2);

        close(fd);
        return 1;
    }

    usec_t finished_ut = now_monotonic_usec();

    info("DBENGINE: journal v2 '%s' loaded, size: %0.2f MiB, metrics: %0.2f k, "
         "mmap: %0.2f ms, validate: %0.2f ms"
         , path_v2
         , (double)journal_v2_file_size / 1024 / 1024
         , (double)entries / 1000
         , ((double)(validation_start_ut - mmap_start_ut) / USEC_PER_MS)
         , ((double)(finished_ut - validation_start_ut) / USEC_PER_MS)
         );

    // Initialize the journal file to be able to access the data
    journalfile_v2_data_set(journalfile, fd, data_start, journal_v2_file_size);

    ctx_current_disk_space_increase(ctx, journal_v2_file_size);

    // File is OK load it
    return 0;
}

struct journal_metric_list_to_sort {
    struct jv2_metrics_info *metric_info;
};

static int journalfile_metric_compare (const void *item1, const void *item2)
{
    const struct jv2_metrics_info *metric1 = ((struct journal_metric_list_to_sort *) item1)->metric_info;
    const struct jv2_metrics_info *metric2 = ((struct journal_metric_list_to_sort *) item2)->metric_info;

    return uuid_compare(*(metric1->uuid), *(metric2->uuid));
}


// Write list of extents for the journalfile
void *journalfile_v2_write_extent_list(Pvoid_t JudyL_extents_pos, void *data)
{
    Pvoid_t *PValue;
    struct journal_extent_list *j2_extent_base = (void *) data;
    struct jv2_extents_info *ext_info;

    bool first = true;
    Word_t pos = 0;
    size_t count = 0;
    while ((PValue = JudyLFirstThenNext(JudyL_extents_pos, &pos, &first))) {
        ext_info = *PValue;
        size_t index = ext_info->index;
        j2_extent_base[index].file_index = 0;
        j2_extent_base[index].datafile_offset = ext_info->pos;
        j2_extent_base[index].datafile_size = ext_info->bytes;
        j2_extent_base[index].pages = ext_info->number_of_pages;
        count++;
    }
    return j2_extent_base + count;
}

static int journalfile_verify_space(struct journal_v2_header *j2_header, void *data, uint32_t bytes)
{
    if ((unsigned long)(((uint8_t *) data - (uint8_t *)  j2_header->data) + bytes) > (j2_header->journal_v2_file_size - sizeof(struct journal_v2_block_trailer)))
        return 1;

    return 0;
}

void *journalfile_v2_write_metric_page(struct journal_v2_header *j2_header, void *data, struct jv2_metrics_info *metric_info, uint32_t pages_offset)
{
    struct journal_metric_list *metric = (void *) data;

    if (journalfile_verify_space(j2_header, data, sizeof(*metric)))
        return NULL;

    uuid_copy(metric->uuid, *metric_info->uuid);
    metric->entries = metric_info->number_of_pages;
    metric->page_offset = pages_offset;
    metric->delta_start_s = (uint32_t)(metric_info->first_time_s - (time_t)(j2_header->start_time_ut / USEC_PER_SEC));
    metric->delta_end_s = (uint32_t)(metric_info->last_time_s - (time_t)(j2_header->start_time_ut / USEC_PER_SEC));

    return ++metric;
}

void *journalfile_v2_write_data_page_header(struct journal_v2_header *j2_header __maybe_unused, void *data, struct jv2_metrics_info *metric_info, uint32_t uuid_offset)
{
    struct journal_page_header *data_page_header = (void *) data;
    uLong crc;

    uuid_copy(data_page_header->uuid, *metric_info->uuid);
    data_page_header->entries = metric_info->number_of_pages;
    data_page_header->uuid_offset = uuid_offset;        // data header OFFSET poings to METRIC in the directory
    data_page_header->crc = JOURVAL_V2_MAGIC;
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (void *) data_page_header, sizeof(*data_page_header));
    crc32set(data_page_header->checksum, crc);
    return ++data_page_header;
}

void *journalfile_v2_write_data_page_trailer(struct journal_v2_header *j2_header __maybe_unused, void *data, void *page_header)
{
    struct journal_page_header *data_page_header = (void *) page_header;
    struct journal_v2_block_trailer *journal_trailer = (void *) data;
    uLong crc;

    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (uint8_t *) page_header + sizeof(struct journal_page_header), data_page_header->entries * sizeof(struct journal_page_list));
    crc32set(journal_trailer->checksum, crc);
    return ++journal_trailer;
}

void *journalfile_v2_write_data_page(struct journal_v2_header *j2_header, void *data, struct jv2_page_info *page_info)
{
    struct journal_page_list *data_page = data;

    if (journalfile_verify_space(j2_header, data, sizeof(*data_page)))
        return NULL;

    struct extent_io_data *ei = page_info->custom_data;

    data_page->delta_start_s = (uint32_t) (page_info->start_time_s - (time_t) (j2_header->start_time_ut) / USEC_PER_SEC);
    data_page->delta_end_s = (uint32_t) (page_info->end_time_s - (time_t) (j2_header->start_time_ut) / USEC_PER_SEC);
    data_page->extent_index = page_info->extent_index;

    data_page->update_every_s = (uint32_t) page_info->update_every_s;
    data_page->page_length = (uint16_t) (ei ? ei->page_length : page_info->page_length);
    data_page->type = 0;

    return ++data_page;
}

// Must be recorded in metric_info->entries
void *journalfile_v2_write_descriptors(struct journal_v2_header *j2_header, void *data, struct jv2_metrics_info *metric_info)
{
    Pvoid_t *PValue;

    struct journal_page_list *data_page = (void *)data;
    // We need to write all descriptors with index metric_info->min_index_time_s, metric_info->max_index_time_s
    // that belong to this journal file
    Pvoid_t JudyL_array = metric_info->JudyL_pages_by_start_time;

    Word_t index_time = 0;
    bool first = true;
    struct jv2_page_info *page_info;
    while ((PValue = JudyLFirstThenNext(JudyL_array, &index_time, &first))) {
        page_info = *PValue;
        // Write one descriptor and return the next data page location
        data_page = journalfile_v2_write_data_page(j2_header, (void *) data_page, page_info);
        if (NULL == data_page)
            break;
    }
    return data_page;
}

// Migrate the journalfile pointed by datafile
// activate : make the new file active immediately
//            journafile data will be set and descriptors (if deleted) will be repopulated as needed
// startup  : if the migration is done during agent startup
//            this will allow us to optimize certain things

void journalfile_migrate_to_v2_callback(Word_t section, unsigned datafile_fileno __maybe_unused, uint8_t type __maybe_unused,
                                        Pvoid_t JudyL_metrics, Pvoid_t JudyL_extents_pos,
                                        size_t number_of_extents, size_t number_of_metrics, size_t number_of_pages, void *user_data)
{
    char path[RRDENG_PATH_MAX];
    Pvoid_t *PValue;
    struct rrdengine_instance *ctx = (struct rrdengine_instance *) section;
    struct rrdengine_journalfile *journalfile = (struct rrdengine_journalfile *) user_data;
    struct rrdengine_datafile *datafile = journalfile->datafile;
    time_t min_time_s = LONG_MAX;
    time_t max_time_s = 0;
    struct jv2_metrics_info *metric_info;

    journalfile_v2_generate_path(datafile, path, sizeof(path));

    info("DBENGINE: indexing file '%s': extents %zu, metrics %zu, pages %zu",
        path,
        number_of_extents,
        number_of_metrics,
        number_of_pages);

#ifdef NETDATA_INTERNAL_CHECKS
    usec_t start_loading = now_monotonic_usec();
#endif

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

    int fd_v2;
    uint8_t *data_start = netdata_mmap(path, total_file_size, MAP_SHARED, 0, false, &fd_v2);
    uint8_t *data = data_start;

    memset(data_start, 0, extent_offset);

    // Write header
    struct journal_v2_header j2_header;
    memset(&j2_header, 0, sizeof(j2_header));

    j2_header.magic = JOURVAL_V2_MAGIC;
    j2_header.start_time_ut = 0;
    j2_header.end_time_ut = 0;
    j2_header.extent_count = number_of_extents;
    j2_header.extent_offset = extent_offset;
    j2_header.metric_count = number_of_metrics;
    j2_header.metric_offset = metrics_offset;
    j2_header.page_count = number_of_pages;
    j2_header.page_offset = pages_offset;
    j2_header.extent_trailer_offset = extent_offset_trailer;
    j2_header.metric_trailer_offset = metric_offset_trailer;
    j2_header.journal_v2_file_size = total_file_size;
    j2_header.journal_v1_file_size = (uint32_t)journalfile_current_size(journalfile);
    j2_header.data = data_start;                        // Used during migration

    struct journal_v2_block_trailer *journal_v2_trailer;

    data = journalfile_v2_write_extent_list(JudyL_extents_pos, data_start + extent_offset);
    internal_error(true, "DBENGINE: write extent list so far %llu", (now_monotonic_usec() - start_loading) / USEC_PER_MS);

    fatal_assert(data == data_start + extent_offset_trailer);

    // Calculate CRC for extents
    journal_v2_trailer = (struct journal_v2_block_trailer *) (data_start + extent_offset_trailer);
    uLong crc;
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (uint8_t *) data_start + extent_offset, number_of_extents * sizeof(struct journal_extent_list));
    crc32set(journal_v2_trailer->checksum, crc);

    internal_error(true, "DBENGINE: CALCULATE CRC FOR EXTENT %llu", (now_monotonic_usec() - start_loading) / USEC_PER_MS);
    // Skip the trailer, point to the metrics off
    data += sizeof(struct journal_v2_block_trailer);

    // Sanity check -- we must be at the metrics_offset
    fatal_assert(data == data_start + metrics_offset);

    // Allocate array to sort UUIDs and keep them sorted in the journal because we want to do binary search when we do lookups
    struct journal_metric_list_to_sort *uuid_list = mallocz(number_of_metrics * sizeof(struct journal_metric_list_to_sort));

    Word_t Index = 0;
    size_t count = 0;
    bool first_then_next = true;
    while ((PValue = JudyLFirstThenNext(JudyL_metrics, &Index, &first_then_next))) {
        metric_info = *PValue;

        fatal_assert(count < number_of_metrics);
        uuid_list[count++].metric_info = metric_info;
        min_time_s = MIN(min_time_s, metric_info->first_time_s);
        max_time_s = MAX(max_time_s, metric_info->last_time_s);
    }

    // Store in the header
    j2_header.start_time_ut = min_time_s * USEC_PER_SEC;
    j2_header.end_time_ut = max_time_s * USEC_PER_SEC;

    qsort(&uuid_list[0], number_of_metrics, sizeof(struct journal_metric_list_to_sort), journalfile_metric_compare);
    internal_error(true, "DBENGINE: traverse and qsort  UUID %llu", (now_monotonic_usec() - start_loading) / USEC_PER_MS);

    uint32_t resize_file_to = total_file_size;

    for (Index = 0; Index < number_of_metrics; Index++) {
        metric_info = uuid_list[Index].metric_info;

        // Calculate current UUID offset from start of file. We will store this in the data page header
        uint32_t uuid_offset = data - data_start;

        // Write the UUID we are processing
        data  = (void *) journalfile_v2_write_metric_page(&j2_header, data, metric_info, pages_offset);
        if (unlikely(!data))
            break;

        // Next we will write
        //   Header
        //   Detailed entries (descr @ time)
        //   Trailer (checksum)

        // Keep the page_list_header, to be used for migration when where agent is running
        metric_info->page_list_header = pages_offset;
        // Write page header
        void *metric_page = journalfile_v2_write_data_page_header(&j2_header, data_start + pages_offset, metric_info,
                                                                  uuid_offset);

        // Start writing descr @ time
        void *page_trailer = journalfile_v2_write_descriptors(&j2_header, metric_page, metric_info);
        if (unlikely(!page_trailer))
            break;

        // Trailer (checksum)
        uint8_t *next_page_address = journalfile_v2_write_data_page_trailer(&j2_header, page_trailer,
                                                                            data_start + pages_offset);

        // Calculate start of the pages start for next descriptor
        pages_offset += (metric_info->number_of_pages * (sizeof(struct journal_page_list)) + sizeof(struct journal_page_header) + sizeof(struct journal_v2_block_trailer));
        // Verify we are at the right location
        if (pages_offset != (uint32_t)(next_page_address - data_start)) {
            // make sure checks fail so that we abort
            data = data_start;
            break;
        }
    }

    if (data == data_start + metric_offset_trailer) {
        internal_error(true, "DBENGINE: WRITE METRICS AND PAGES  %llu", (now_monotonic_usec() - start_loading) / USEC_PER_MS);

        // Calculate CRC for metrics
        journal_v2_trailer = (struct journal_v2_block_trailer *)(data_start + metric_offset_trailer);
        crc = crc32(0L, Z_NULL, 0);
        crc =
            crc32(crc, (uint8_t *)data_start + metrics_offset, number_of_metrics * sizeof(struct journal_metric_list));
        crc32set(journal_v2_trailer->checksum, crc);
        internal_error(true, "DBENGINE: CALCULATE CRC FOR UUIDs  %llu", (now_monotonic_usec() - start_loading) / USEC_PER_MS);

        // Prepare to write checksum for the file
        j2_header.data = NULL;
        journal_v2_trailer = (struct journal_v2_block_trailer *)(data_start + trailer_offset);
        crc = crc32(0L, Z_NULL, 0);
        crc = crc32(crc, (void *)&j2_header, sizeof(j2_header));
        crc32set(journal_v2_trailer->checksum, crc);

        // Write header to the file
        memcpy(data_start, &j2_header, sizeof(j2_header));

        internal_error(true, "DBENGINE: FILE COMPLETED --------> %llu", (now_monotonic_usec() - start_loading) / USEC_PER_MS);

        info("DBENGINE: migrated journal file '%s', file size %zu", path, total_file_size);

        // msync(data_start, total_file_size, MS_SYNC);
        journalfile_v2_data_set(journalfile, fd_v2, data_start, total_file_size);

        internal_error(true, "DBENGINE: ACTIVATING NEW INDEX JNL %llu", (now_monotonic_usec() - start_loading) / USEC_PER_MS);
        ctx_current_disk_space_increase(ctx, total_file_size);
        freez(uuid_list);
        return;
    }
    else {
        info("DBENGINE: failed to build index '%s', file will be skipped", path);
        j2_header.data = NULL;
        j2_header.magic = JOURVAL_V2_SKIP_MAGIC;
        memcpy(data_start, &j2_header, sizeof(j2_header));
        resize_file_to = sizeof(j2_header);
    }

    netdata_munmap(data_start, total_file_size);
    freez(uuid_list);

    if (likely(resize_file_to == total_file_size))
        return;

    int ret = truncate(path, (long) resize_file_to);
    if (ret < 0) {
        ctx_current_disk_space_increase(ctx, total_file_size);
        ctx_fs_error(ctx);
        error("DBENGINE: failed to resize file '%s'", path);
    }
    else
        ctx_current_disk_space_increase(ctx, resize_file_to);
}

int journalfile_load(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile,
                     struct rrdengine_datafile *datafile)
{
    uv_fs_t req;
    uv_file file;
    int ret, fd, error;
    uint64_t file_size, max_id;
    char path[RRDENG_PATH_MAX];
    bool loaded_v2 = false;

    // Do not try to load jv2 of the latest file
    if (datafile->fileno != ctx_last_fileno_get(ctx))
        loaded_v2 = journalfile_v2_load(ctx, journalfile, datafile) == 0;

    journalfile_v1_generate_path(datafile, path, sizeof(path));

    fd = open_file_for_io(path, O_RDWR, &file, use_direct_io);
    if (fd < 0) {
        ctx_fs_error(ctx);

        if(loaded_v2)
            return 0;

        return fd;
    }

    ret = check_file_properties(file, &file_size, sizeof(struct rrdeng_df_sb));
    if (ret) {
        error = ret;
        goto cleanup;
    }

    if(loaded_v2) {
        journalfile->unsafe.pos = file_size;
        error = 0;
        goto cleanup;
    }

    file_size = ALIGN_BYTES_FLOOR(file_size);
    journalfile->unsafe.pos = file_size;
    journalfile->file = file;

    ret = journalfile_check_superblock(file);
    if (ret) {
        info("DBENGINE: invalid journal file '%s' ; superblock check failed.", path);
        error = ret;
        goto cleanup;
    }
    ctx_io_read_op_bytes(ctx, sizeof(struct rrdeng_jf_sb));

    info("DBENGINE: loading journal file '%s'", path);

    max_id = journalfile_iterate_transactions(ctx, journalfile);

    __atomic_store_n(&ctx->atomic.transaction_id, MAX(__atomic_load_n(&ctx->atomic.transaction_id, __ATOMIC_RELAXED), max_id + 1), __ATOMIC_RELAXED);

    info("DBENGINE: journal file '%s' loaded (size:%"PRIu64").", path, file_size);

    bool is_last_file = (ctx_last_fileno_get(ctx) == journalfile->datafile->fileno);
    if (is_last_file && journalfile->datafile->pos <= rrdeng_target_data_file_size(ctx) / 3) {
        ctx->loading.create_new_datafile_pair = false;
        return 0;
    }

    pgc_open_cache_to_journal_v2(open_cache, (Word_t) ctx, (int) datafile->fileno, ctx->config.page_type,
                                 journalfile_migrate_to_v2_callback, (void *) datafile->journalfile);

    if (is_last_file)
        ctx->loading.create_new_datafile_pair = true;

    return 0;

cleanup:
    ret = uv_fs_close(NULL, &req, file, NULL);
    if (ret < 0) {
        error("DBENGINE: uv_fs_close(%s): %s", path, uv_strerror(ret));
        ctx_fs_error(ctx);
    }
    uv_fs_req_cleanup(&req);
    return error;
}
