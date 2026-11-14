// SPDX-License-Identifier: GPL-3.0-or-later

#include "retention-summary.h"

_Static_assert(sizeof(struct retention_summary_file_identity) == 56, "retention summary identity size changed");
_Static_assert(sizeof(struct retention_summary_record) == 32, "retention summary record size changed");
_Static_assert(sizeof(struct retention_summary_header) <= RETENTION_SUMMARY_HEADER_SIZE, "retention summary header too big");

struct retention_summary_block {
    struct rrdengine_instance *ctx;
    struct retention_summary_header header;
    struct rrdengine_datafile *datafiles[RETENTION_SUMMARY_FILES_PER_BLOCK];
    uint64_t file_size;
};

// ----------------------------------------------------------------------------
// helpers

// Blocks are numbered from the first datafile, 1: block N covers datafiles
// N * 24 + 1 .. N * 24 + 24. A datafile numbered 0 belongs to no block.
static uint64_t rs_block_first_fileno(uint32_t block_id) {
    return (uint64_t)block_id * RETENTION_SUMMARY_FILES_PER_BLOCK + 1;
}

static bool rs_block_of(uint64_t fileno, uint32_t *block_id) {
    if (!fileno || (fileno - 1) / RETENTION_SUMMARY_FILES_PER_BLOCK > UINT32_MAX)
        return false;
    *block_id = (uint32_t)((fileno - 1) / RETENTION_SUMMARY_FILES_PER_BLOCK);
    return true;
}

static void rs_path(struct rrdengine_instance *ctx, uint32_t block_id, bool tmp, char *buf, size_t len) {
    snprintfz(buf, len, "%s/" RETENTION_SUMMARY_PREFIX "%10.10u%s",
              ctx->config.dbfiles_path, block_id, tmp ? RETENTION_SUMMARY_TMP_EXTENSION : RETENTION_SUMMARY_EXTENSION);
}

static uint64_t rs_chunks(uint64_t records) {
    return (records + RETENTION_SUMMARY_CHUNK_RECORDS - 1) / RETENTION_SUMMARY_CHUNK_RECORDS;
}

static uint64_t rs_expected_file_size(uint64_t records) {
    return RETENTION_SUMMARY_HEADER_SIZE +
           records * sizeof(struct retention_summary_record) +
           rs_chunks(records) * sizeof(uint32_t) +
           sizeof(uint32_t);
}

static uint32_t rs_crc(const void *data, size_t len) {
    uLong crc = crc32(0L, Z_NULL, 0);
    return (uint32_t)crc32(crc, data, len);
}

static uint32_t rs_header_crc(const struct retention_summary_header *h) {
    return rs_crc(h, offsetof(struct retention_summary_header, crc));
}

static bool rs_pread_full(int fd, void *buf, size_t len, uint64_t offset) {
    uint8_t *p = buf;
    while (len) {
        ssize_t rc = pread(fd, p, len, (off_t)offset);
        if (rc < 0 && errno == EINTR)
            continue;
        if (rc <= 0)
            return false;
        p += rc;
        len -= (size_t)rc;
        offset += (uint64_t)rc;
    }
    return true;
}

static bool rs_write_full(int fd, const void *buf, size_t len) {
    const uint8_t *p = buf;
    while (len) {
        ssize_t rc = write(fd, p, len);
        if (rc < 0 && errno == EINTR)
            continue;
        if (rc <= 0)
            return false;
        p += rc;
        len -= (size_t)rc;
    }
    return true;
}

static bool rs_fsync_dir(const char *path) {
    int fd = open(path, O_RDONLY | O_CLOEXEC);
    if (fd == -1)
        return false;
    bool ok = fsync(fd) == 0;
    close(fd);
    return ok;
}

// a block given up on: cannot be summarised, or failed this many times (until restart)
#define RS_MAX_FAILURES 3

static bool rs_known(struct rrdengine_instance *ctx, uint32_t block_id, bool skip_list) {
    spinlock_lock(&ctx->summaries.spinlock);
    Pvoid_t *PValue = JudyLGet(skip_list ? ctx->summaries.skip_JudyL : ctx->summaries.JudyL, block_id, PJE0);
    bool known = PValue && (!skip_list || (uintptr_t)*PValue >= RS_MAX_FAILURES);
    spinlock_unlock(&ctx->summaries.spinlock);
    return known;
}

static void rs_remember(struct rrdengine_instance *ctx, uint32_t block_id, uint64_t file_size) {
    spinlock_lock(&ctx->summaries.spinlock);
    Pvoid_t *PValue = JudyLIns(&ctx->summaries.JudyL, block_id, PJE0);
    if (PValue && PValue != PJERR)
        *PValue = (void *)(uintptr_t)file_size;
    spinlock_unlock(&ctx->summaries.spinlock);

    ctx_current_disk_space_increase(ctx, file_size);
}

// returns the failures so far
static uintptr_t rs_failed(struct rrdengine_instance *ctx, uint32_t block_id, bool forever) {
    spinlock_lock(&ctx->summaries.spinlock);
    Pvoid_t *PValue = JudyLIns(&ctx->summaries.skip_JudyL, block_id, PJE0);
    uintptr_t failures = 0;
    if (PValue && PValue != PJERR) {
        failures = forever ? RS_MAX_FAILURES : (uintptr_t)*PValue + 1;
        *PValue = (void *)failures;
    }
    spinlock_unlock(&ctx->summaries.spinlock);
    return failures;
}

// unlinks the published summary of a block, if there is one; it stays known
// (and on the disk-space account) if it cannot be unlinked, so a later call retries
static void rs_forget_and_unlink(struct rrdengine_instance *ctx, uint32_t block_id) {
    if (!rs_known(ctx, block_id, false))
        return;

    char path[RRDENG_PATH_MAX];
    rs_path(ctx, block_id, false, path, sizeof(path));
    if (unlink(path) != 0 && errno != ENOENT) {
        nd_log_daemon(NDLP_WARNING, "DBENGINE: tier %d: cannot delete retention summary \"%s\"", ctx->config.tier, path);
        return;
    }

    uint64_t file_size = 0;
    spinlock_lock(&ctx->summaries.spinlock);
    Pvoid_t *PValue = JudyLGet(ctx->summaries.JudyL, block_id, PJE0);
    if (PValue) {
        file_size = (uint64_t)(uintptr_t)*PValue;
        (void)JudyLDel(&ctx->summaries.JudyL, block_id, PJE0);
    }
    spinlock_unlock(&ctx->summaries.spinlock);

    if (PValue)
        ctx_current_disk_space_decrease(ctx, file_size);
}

void retention_summary_datafile_deleted(struct rrdengine_instance *ctx, unsigned fileno) {
    uint32_t block_id;
    if (rs_block_of(fileno, &block_id))
        rs_forget_and_unlink(ctx, block_id);
}

void retention_summary_ctx_cleanup(struct rrdengine_instance *ctx) {
    spinlock_lock(&ctx->summaries.spinlock);
    JudyLFreeArray(&ctx->summaries.JudyL, PJE0);
    JudyLFreeArray(&ctx->summaries.skip_JudyL, PJE0);
    spinlock_unlock(&ctx->summaries.spinlock);
}

// ----------------------------------------------------------------------------
// building

#define RS_CURSOR_ENTRIES 4096    // per file: 144 KiB, 3.4 MiB for a block

typedef enum {
    RS_BUILD_OK,
    RS_BUILD_SKIP,      // the block cannot be summarised (content); do not retry until restart
    RS_BUILD_FAILED,    // I/O failure or aborted; retry later
} RS_BUILD_RESULT;

struct rs_cursor {
    int fd;
    struct rrdengine_datafile *datafile;
    struct journal_metric_list *entries;
    size_t used;                    // entries in the buffer
    size_t pos;                     // the current entry in the buffer
    uint32_t unread;                // entries of the file not read into the buffer yet
    uint64_t next_offset;
    uLong crc;
    time_t header_start_s;
    bool samples_known;
    uint64_t samples;
    bool have_previous;
    nd_uuid_t previous;
    bool unsorted;
};

static bool rs_datafile_is_being_deleted(struct rrdengine_datafile *df) {
    spinlock_tracked_lock(&df->users.spinlock);
    bool pending = df->users.pending_deletion;
    spinlock_tracked_unlock(&df->users.spinlock);
    return pending;
}

// false on a read error, or when the build must stop: the tier is shutting
// down, or the datafile is waiting to be deleted (it waits for this build)
static bool rs_cursor_fill(struct rs_cursor *c) {
    if (!ctx_is_available_for_queries(datafile_ctx(c->datafile)) || rs_datafile_is_being_deleted(c->datafile))
        return false;

    size_t n = c->unread > RS_CURSOR_ENTRIES ? RS_CURSOR_ENTRIES : c->unread;
    size_t bytes = n * sizeof(struct journal_metric_list);
    if (!rs_pread_full(c->fd, c->entries, bytes, c->next_offset))
        return false;

    c->crc = crc32(c->crc, (const void *)c->entries, bytes);
    c->next_offset += bytes;
    c->unread -= (uint32_t)n;
    c->used = n;
    c->pos = 0;
    return true;
}

// the current entry, or NULL when the cursor is exhausted; *io_error is set on a read failure
static struct journal_metric_list *rs_cursor_current(struct rs_cursor *c, bool *io_error) {
    if (c->pos >= c->used) {
        if (!c->unread)
            return NULL;
        if (!rs_cursor_fill(c)) {
            *io_error = true;
            return NULL;
        }
    }
    return &c->entries[c->pos];
}

static void rs_cursor_advance(struct rs_cursor *c) {
    struct journal_metric_list *m = &c->entries[c->pos];

    if (c->have_previous && memcmp(c->previous, m->uuid, sizeof(nd_uuid_t)) >= 0)
        c->unsorted = true;
    uuid_copy(c->previous, m->uuid);
    c->have_previous = true;

    if (!c->samples_known)
        c->samples += journalfile_v2_metric_estimated_samples(m);

    c->pos++;
}

// opens a journal v2 of the block and checks its header the way journalfile_v2_validate() does
static RS_BUILD_RESULT rs_cursor_open(struct rs_cursor *c, struct rrdengine_datafile *datafile,
                                      struct retention_summary_file_identity *id, time_t now_s) {
    char path[RRDENG_PATH_MAX];
    journalfile_v2_generate_path(datafile, path, sizeof(path));

    c->datafile = datafile;
    c->fd = open(path, O_RDONLY | O_CLOEXEC);
    if (c->fd == -1)
        return RS_BUILD_FAILED;

    struct stat st;
    if (fstat(c->fd, &st) != 0 || (uint64_t)st.st_size < RRDENG_BLOCK_SIZE + sizeof(struct journal_v2_block_trailer))
        return RS_BUILD_SKIP;

    // the journal is complete; make it durable before a summary claims to
    // describe it (through a writable descriptor: some platforms refuse to
    // flush a read-only one)
    int wfd = open(path, O_WRONLY | O_CLOEXEC);
    int sync_rc = fsync(wfd != -1 ? wfd : c->fd);
    if (wfd != -1)
        close(wfd);
    if (sync_rc != 0)
        return RS_BUILD_FAILED;

    uint8_t page[RRDENG_BLOCK_SIZE];
    struct journal_v2_block_trailer file_trailer, metric_trailer;
    if (!rs_pread_full(c->fd, page, sizeof(page), 0) ||
        !rs_pread_full(c->fd, &file_trailer, sizeof(file_trailer), (uint64_t)st.st_size - sizeof(file_trailer)))
        return RS_BUILD_FAILED;

    const struct journal_v2_header *h = (const void *)page;
    size_t file_size = (size_t)st.st_size;

    if (h->magic != JOURVAL_V2_MAGIC || h->journal_v2_file_size != file_size ||
        crc32cmp(file_trailer.checksum, rs_crc(h, sizeof(*h))))
        return RS_BUILD_SKIP;

    if ((size_t)h->metric_offset > file_size ||
        (size_t)h->metric_count > (file_size - (size_t)h->metric_offset) / sizeof(struct journal_metric_list) ||
        (size_t)h->metric_trailer_offset > file_size ||
        file_size - (size_t)h->metric_trailer_offset < sizeof(struct journal_v2_block_trailer) ||
        (size_t)h->metric_trailer_offset != (size_t)h->metric_offset + (size_t)h->metric_count * sizeof(struct journal_metric_list))
        return RS_BUILD_SKIP;

    // the per-file walk fixes these on the fly (zero start, last time in the
    // future); a summary cannot reproduce that, so such blocks stay per-file
    time_t start_s = (time_t)(h->start_time_ut / USEC_PER_SEC);
    if (!start_s || (time_t)(h->end_time_ut / USEC_PER_SEC) > now_s || !h->metric_count)
        return RS_BUILD_SKIP;

    if (!rs_pread_full(c->fd, &metric_trailer, sizeof(metric_trailer), h->metric_trailer_offset))
        return RS_BUILD_FAILED;

    struct journal_v2_samples_descriptor descriptor;
    c->samples_known = journalfile_v2_samples_descriptor(page, file_size, &descriptor);
    c->samples = c->samples_known ? descriptor.samples : 0;

    c->header_start_s = start_s;
    c->unread = h->metric_count;
    c->next_offset = h->metric_offset;
    c->crc = crc32(0L, Z_NULL, 0);
    c->entries = mallocz(RS_CURSOR_ENTRIES * sizeof(struct journal_metric_list));

    *id = (struct retention_summary_file_identity) {
        .fileno = datafile->fileno,
        .metric_count = h->metric_count,
        .v2_size = file_size,
        .start_time_ut = h->start_time_ut,
        .end_time_ut = h->end_time_ut,
        .header_crc = file_trailer.crc,
        .metric_list_crc = metric_trailer.crc,
    };

    return RS_BUILD_OK;
}

struct rs_writer {
    int fd;
    struct retention_summary_record *chunk;
    size_t used;
    uint32_t *chunk_crcs;
    size_t chunks, chunks_size;
    uint64_t records;
};

static bool rs_writer_flush(struct rs_writer *w) {
    if (!w->used)
        return true;

    size_t bytes = w->used * sizeof(struct retention_summary_record);
    if (!rs_write_full(w->fd, w->chunk, bytes))
        return false;

    if (w->chunks == w->chunks_size) {
        w->chunks_size = w->chunks_size ? w->chunks_size * 2 : 64;
        w->chunk_crcs = reallocz(w->chunk_crcs, w->chunks_size * sizeof(uint32_t));
    }
    w->chunk_crcs[w->chunks++] = rs_crc(w->chunk, bytes);
    w->used = 0;
    return true;
}

static bool rs_writer_add(struct rs_writer *w, const struct retention_summary_record *r) {
    w->chunk[w->used++] = *r;
    w->records++;
    if (w->used == RETENTION_SUMMARY_CHUNK_RECORDS)
        return rs_writer_flush(w);
    return true;
}

static RS_BUILD_RESULT rs_build_block(struct rrdengine_instance *ctx, uint32_t block_id,
                                      struct rrdengine_datafile *datafiles[RETENTION_SUMMARY_FILES_PER_BLOCK]) {
    struct rs_cursor cursors[RETENTION_SUMMARY_FILES_PER_BLOCK] = { 0 };
    struct retention_summary_header header = {
        .magic = RETENTION_SUMMARY_MAGIC,
        .version = RETENTION_SUMMARY_VERSION,
        .files_per_block = RETENTION_SUMMARY_FILES_PER_BLOCK,
        .tier = (uint32_t)ctx->config.tier,
        .block_id = block_id,
        .chunk_records = RETENTION_SUMMARY_CHUNK_RECORDS,
    };
    struct rs_writer w = { .fd = -1 };
    RS_BUILD_RESULT result = RS_BUILD_OK;
    time_t now_s = max_acceptable_collected_time();
    usec_t started_ut = now_monotonic_usec();

    char tmp_path[RRDENG_PATH_MAX], path[RRDENG_PATH_MAX];
    rs_path(ctx, block_id, true, tmp_path, sizeof(tmp_path));
    rs_path(ctx, block_id, false, path, sizeof(path));

    for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++)
        cursors[i].fd = -1;

    time_t base_s = LONG_MAX;
    for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++) {
        result = rs_cursor_open(&cursors[i], datafiles[i], &header.files[i], now_s);
        if (result != RS_BUILD_OK)
            goto cleanup;
        base_s = MIN(base_s, cursors[i].header_start_s);
    }
    header.base_time_s = base_s;

    w.fd = open(tmp_path, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, S_IRUSR | S_IWUSR);
    if (w.fd == -1 || lseek(w.fd, RETENTION_SUMMARY_HEADER_SIZE, SEEK_SET) != RETENTION_SUMMARY_HEADER_SIZE) {
        result = RS_BUILD_FAILED;
        goto cleanup;
    }
    w.chunk = mallocz(RETENTION_SUMMARY_CHUNK_RECORDS * sizeof(struct retention_summary_record));

    time_t max_last_s = 0;
    bool io_error = false;
    while (true) {
        // the smallest UUID among the cursors
        struct journal_metric_list *min = NULL;
        for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++) {
            struct journal_metric_list *m = rs_cursor_current(&cursors[i], &io_error);
            if (m && (!min || memcmp(m->uuid, min->uuid, sizeof(nd_uuid_t)) < 0))
                min = m;
        }
        if (io_error) {
            result = RS_BUILD_FAILED;
            goto cleanup;
        }
        if (!min)
            break;

        nd_uuid_t uuid;
        uuid_copy(uuid, min->uuid);

        // merge the entries of this UUID the way the per-file walk applies them
        // in fileno order (see struct retention_summary_record)
        time_t first_s = LONG_MAX, last_s = 0, update_every_last_s = 0;
        uint32_t update_every_s = 0;
        for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++) {
            struct journal_metric_list *m = rs_cursor_current(&cursors[i], &io_error);
            if (!m || memcmp(m->uuid, uuid, sizeof(nd_uuid_t)) != 0)
                continue;

            // the walk fixes these on the fly; one merged record cannot
            if (m->delta_start_s > m->delta_end_s) {
                result = RS_BUILD_SKIP;
                goto cleanup;
            }

            time_t s = cursors[i].header_start_s + m->delta_start_s;
            time_t e = cursors[i].header_start_s + m->delta_end_s;
            first_s = MIN(first_s, s);
            if (e > last_s) {
                last_s = e;
                if (m->update_every_s) {
                    update_every_s = m->update_every_s;
                    update_every_last_s = e;
                }
            }

            rs_cursor_advance(&cursors[i]);
        }

        if (last_s > now_s || last_s - base_s > UINT32_MAX) {
            result = RS_BUILD_SKIP;
            goto cleanup;
        }
        max_last_s = MAX(max_last_s, last_s);

        struct retention_summary_record r = {
            .first_delta_s = (uint32_t)(first_s - base_s),
            .last_delta_s = (uint32_t)(last_s - base_s),
            .update_every_s = update_every_s,
            .update_every_last_delta_s = (uint32_t)((update_every_s ? update_every_last_s : last_s) - base_s),
        };
        uuid_copy(r.uuid, uuid);

        if (!rs_writer_add(&w, &r)) {
            result = RS_BUILD_FAILED;
            goto cleanup;
        }
    }

    // every metric list must be sorted and match the CRC its journal carries
    for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++) {
        if (cursors[i].unsorted || (uint32_t)cursors[i].crc != header.files[i].metric_list_crc) {
            result = RS_BUILD_SKIP;
            goto cleanup;
        }
        header.files[i].samples = cursors[i].samples;
        header.files[i].samples_estimated = cursors[i].samples_known ? 0 : 1;
    }

    if (!rs_writer_flush(&w)) {
        result = RS_BUILD_FAILED;
        goto cleanup;
    }

    header.record_count = w.records;
    header.max_last_time_s = max_last_s;
    header.crc = rs_header_crc(&header);

    uint32_t trailer = rs_crc(w.chunk_crcs, w.chunks * sizeof(uint32_t));
    uint8_t header_page[RETENTION_SUMMARY_HEADER_SIZE] = { 0 };
    memcpy(header_page, &header, sizeof(header));

    if (!rs_write_full(w.fd, w.chunk_crcs, w.chunks * sizeof(uint32_t)) ||
        !rs_write_full(w.fd, &trailer, sizeof(trailer)) ||
        pwrite(w.fd, header_page, sizeof(header_page), 0) != (ssize_t)sizeof(header_page) ||
        fsync(w.fd) != 0) {
        result = RS_BUILD_FAILED;
        goto cleanup;
    }
    close(w.fd);
    w.fd = -1;

    if (rename(tmp_path, path) != 0) {
        result = RS_BUILD_FAILED;
        goto cleanup;
    }
    // without it the rename may not survive a crash; the summary is still
    // correct, and a lost one is rebuilt
    if (!rs_fsync_dir(ctx->config.dbfiles_path))
        nd_log_daemon(NDLP_WARNING, "DBENGINE: tier %d: cannot sync directory \"%s\" after publishing \"%s\"",
                      ctx->config.tier, ctx->config.dbfiles_path, path);

    uint64_t file_size = rs_expected_file_size(header.record_count);
    rs_remember(ctx, block_id, file_size);

    nd_log_daemon(NDLP_INFO,
                  "DBENGINE: tier %d: built retention summary of datafiles %u-%u: %" PRIu64 " metrics, %" PRIu64 " bytes, %.2f s",
                  ctx->config.tier, header.files[0].fileno, header.files[RETENTION_SUMMARY_FILES_PER_BLOCK - 1].fileno,
                  header.record_count, file_size, (double)(now_monotonic_usec() - started_ut) / USEC_PER_SEC);

cleanup:
    for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++) {
        if (cursors[i].fd != -1)
            close(cursors[i].fd);
        freez(cursors[i].entries);
    }
    if (w.fd != -1)
        close(w.fd);
    freez(w.chunk);
    freez(w.chunk_crcs);

    if (result != RS_BUILD_OK)
        (void)unlink(tmp_path);

    return result;
}

bool retention_summary_build_one(struct rrdengine_instance *ctx) {
    struct rrdengine_datafile *datafiles[RETENTION_SUMMARY_FILES_PER_BLOCK];
    uint32_t block_id = 0;
    bool found = false;

    netdata_rwlock_rdlock(&ctx->datafiles.rwlock);

    unsigned last_fileno = ctx_last_fileno_get(ctx);
    unsigned last_flush_fileno = ctx_last_flush_fileno_get(ctx);

    Word_t first = 0, last = 0;
    bool have_first = JudyLFirst(ctx->datafiles.JudyL, &first, PJE0) != NULL;
    last = (Word_t)-1;
    bool have_last = JudyLLast(ctx->datafiles.JudyL, &last, PJE0) != NULL;

    uint32_t first_block, last_block;
    if (have_first && have_last && rs_block_of(first ? first : 1, &first_block) && rs_block_of(last, &last_block)) {
        // newest complete block first
        for (int64_t b = last_block; b >= (int64_t)first_block && !found; b--) {
            uint32_t candidate = (uint32_t)b;
            if (rs_known(ctx, candidate, false) || rs_known(ctx, candidate, true))
                continue;

            size_t acquired = 0;
            for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++) {
                Word_t fileno = (Word_t)(rs_block_first_fileno(candidate) + i);
                Pvoid_t *PValue = JudyLGet(ctx->datafiles.JudyL, fileno, PJE0);
                struct rrdengine_datafile *df = PValue ? *PValue : NULL;
                if (!df || df->fileno == last_fileno || df->fileno == last_flush_fileno ||
                    !journalfile_v2_data_available(df->journalfile) ||
                    !datafile_acquire(df, DATAFILE_ACQUIRE_RETENTION))
                    break;
                datafiles[acquired++] = df;
            }

            if (acquired == RETENTION_SUMMARY_FILES_PER_BLOCK) {
                block_id = candidate;
                found = true;
            }
            else {
                while (acquired)
                    datafile_release(datafiles[--acquired], DATAFILE_ACQUIRE_RETENTION);
            }
        }
    }

    netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if (!found)
        return false;

    RS_BUILD_RESULT rc = rs_build_block(ctx, block_id, datafiles);

    for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++)
        datafile_release(datafiles[i], DATAFILE_ACQUIRE_RETENTION);

    if (rc == RS_BUILD_SKIP) {
        nd_log_daemon(NDLP_INFO,
                      "DBENGINE: tier %d: datafiles %" PRIu64 "-%" PRIu64 " cannot be summarised; they will be loaded one by one",
                      ctx->config.tier, rs_block_first_fileno(block_id),
                      rs_block_first_fileno(block_id) + RETENTION_SUMMARY_FILES_PER_BLOCK - 1);
        rs_failed(ctx, block_id, true);
    }
    else if (rc == RS_BUILD_FAILED && ctx_is_available_for_queries(ctx)) {
        uintptr_t failures = rs_failed(ctx, block_id, false);
        nd_log_daemon(NDLP_WARNING,
                      "DBENGINE: tier %d: building the retention summary of datafiles %" PRIu64 "-%" PRIu64 " failed (%zu of %d attempts)",
                      ctx->config.tier, rs_block_first_fileno(block_id),
                      rs_block_first_fileno(block_id) + RETENTION_SUMMARY_FILES_PER_BLOCK - 1,
                      (size_t)failures, RS_MAX_FAILURES);
    }

    return true;
}

// ----------------------------------------------------------------------------
// loading at startup

// does the loaded journal v2 of this datafile match what the summary was built from?
static bool rs_identity_matches(struct rrdengine_datafile *df, const struct retention_summary_file_identity *id) {
    struct rrdengine_journalfile *journalfile = df->journalfile;
    if (!journalfile_v2_data_available(journalfile))
        return false;

    size_t data_size = 0;
    struct journal_v2_header *j2_header = journalfile_v2_data_acquire_with_hint(
        journalfile, &data_size, 0, 0, JOURNALFILE_V2_ACCESS_RANDOM);
    if (!j2_header)
        return false;

    // modified inside the protected region and read after it (siglongjmp)
    volatile bool matches = false;
    char path[RRDENG_PATH_MAX];
    journalfile_v2_generate_path(df, path, sizeof(path));

    PROTECTED_ACCESS_SETUP(journalfile->mmap.data, journalfile->mmap.size, path, "summary-identity");
    if (no_signal_received) {
        const uint8_t *data_start = (const uint8_t *)j2_header;
        if (data_size == id->v2_size &&
            data_size >= sizeof(struct journal_v2_block_trailer) &&
            j2_header->metric_count == id->metric_count &&
            j2_header->start_time_ut == id->start_time_ut &&
            j2_header->end_time_ut == id->end_time_ut &&
            (size_t)j2_header->metric_trailer_offset <= data_size - sizeof(struct journal_v2_block_trailer)) {
            const struct journal_v2_block_trailer *file_trailer = (const void *)(data_start + data_size - sizeof(*file_trailer));
            const struct journal_v2_block_trailer *metric_trailer = (const void *)(data_start + j2_header->metric_trailer_offset);
            // the samples descriptor is outside the header CRC: it must still say what it said at build time
            struct journal_v2_samples_descriptor descriptor;
            bool samples_known = journalfile_v2_samples_descriptor(data_start, data_size, &descriptor);
            bool samples_match = samples_known ? (!id->samples_estimated && descriptor.samples == id->samples)
                                               : (id->samples_estimated != 0);
            matches = file_trailer->crc == id->header_crc && metric_trailer->crc == id->metric_list_crc && samples_match;
        }
    }

    journalfile_v2_data_release(journalfile);
    return matches;
}

static bool rs_header_valid(struct rrdengine_instance *ctx, const struct retention_summary_header *h,
                            uint32_t block_id, uint64_t file_size) {
    if (h->magic != RETENTION_SUMMARY_MAGIC || h->version != RETENTION_SUMMARY_VERSION ||
        h->files_per_block != RETENTION_SUMMARY_FILES_PER_BLOCK || h->tier != (uint32_t)ctx->config.tier ||
        h->block_id != block_id || h->chunk_records != RETENTION_SUMMARY_CHUNK_RECORDS ||
        h->crc != rs_header_crc(h))
        return false;

    if (h->record_count > (UINT64_MAX - RETENTION_SUMMARY_HEADER_SIZE) / (sizeof(struct retention_summary_record) + sizeof(uint32_t)) ||
        rs_expected_file_size(h->record_count) != file_size)
        return false;

    // the per-file walk clamps a last time in the future; a summary cannot
    if (h->max_last_time_s > max_acceptable_collected_time())
        return false;

    for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++)
        if ((uint64_t)h->files[i].fileno != rs_block_first_fileno(block_id) + i)
            return false;

    return true;
}

// a summary that cannot be used for this block is deleted; the builder makes a new one
static RETENTION_SUMMARY_BLOCK *rs_claim_one(struct rrdengine_instance *ctx, const char *name) {
    uint32_t block_id;
    char tail[16];
    if (sscanf(name, RETENTION_SUMMARY_PREFIX "%10u%15s", &block_id, tail) != 2 || strcmp(tail, RETENTION_SUMMARY_EXTENSION) != 0)
        return NULL;

    char path[RRDENG_PATH_MAX];
    rs_path(ctx, block_id, false, path, sizeof(path));
    if (strcmp(strrchr(path, '/') + 1, name) != 0)
        return NULL;

    RETENTION_SUMMARY_BLOCK *block = callocz(1, sizeof(*block));
    block->ctx = ctx;

    const char *reason = NULL;
    uint8_t header_page[RETENTION_SUMMARY_HEADER_SIZE];
    struct stat st;
    int fd = open(path, O_RDONLY | O_CLOEXEC);
    if (fd == -1 || fstat(fd, &st) != 0 || !rs_pread_full(fd, header_page, sizeof(header_page), 0))
        reason = "cannot be read";
    if (fd != -1)
        close(fd);

    if (!reason) {
        memcpy(&block->header, header_page, sizeof(block->header));
        block->file_size = (uint64_t)st.st_size;
        if (!rs_header_valid(ctx, &block->header, block_id, block->file_size))
            reason = "is invalid or of another version";
    }

    size_t acquired = 0;
    if (!reason) {
        netdata_rwlock_rdlock(&ctx->datafiles.rwlock);
        for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK && !reason; i++) {
            Pvoid_t *PValue = JudyLGet(ctx->datafiles.JudyL, block->header.files[i].fileno, PJE0);
            struct rrdengine_datafile *df = PValue ? *PValue : NULL;
            if (!df || df->fileno == ctx_last_fileno_get(ctx))
                reason = "covers a datafile that does not exist";
            else if (!rs_identity_matches(df, &block->header.files[i]))
                reason = "does not match the journal files";
            // held until the records are applied: a datafile deleted meanwhile
            // (rotation can run during startup) waits for it, as for the builder
            else if (!datafile_acquire(df, DATAFILE_ACQUIRE_RETENTION))
                reason = "covers a datafile that is being deleted";
            else
                block->datafiles[acquired++] = df;
        }

        // claim the datafiles, so the per-file population skips them
        for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK && !reason; i++) {
            spinlock_lock(&block->datafiles[i]->populate_mrg.spinlock);
            if (block->datafiles[i]->populate_mrg.populated)
                reason = "covers a datafile that is already loaded";
            spinlock_unlock(&block->datafiles[i]->populate_mrg.spinlock);
        }
        if (!reason) {
            for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++) {
                spinlock_lock(&block->datafiles[i]->populate_mrg.spinlock);
                block->datafiles[i]->populate_mrg.populated = true;
                spinlock_unlock(&block->datafiles[i]->populate_mrg.spinlock);
            }
        }
        netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);
    }

    if (reason) {
        while (acquired)
            datafile_release(block->datafiles[--acquired], DATAFILE_ACQUIRE_RETENTION);
        nd_log_daemon(NDLP_NOTICE, "DBENGINE: tier %d: retention summary \"%s\" %s; deleting it",
                      ctx->config.tier, path, reason);
        (void)unlink(path);
        freez(block);
        return NULL;
    }

    rs_remember(ctx, block_id, block->file_size);
    return block;
}

size_t retention_summary_claim(struct rrdengine_instance *ctx, RETENTION_SUMMARY_BLOCK ***blocks) {
    *blocks = NULL;
    size_t count = 0, size = 0;

    DIR *dir = opendir(ctx->config.dbfiles_path);
    if (!dir)
        return 0;

    struct dirent *de;
    while ((de = readdir(dir))) {
        if (strncmp(de->d_name, RETENTION_SUMMARY_PREFIX, sizeof(RETENTION_SUMMARY_PREFIX) - 1) != 0)
            continue;

        size_t len = strlen(de->d_name);
        size_t tmp_len = sizeof(RETENTION_SUMMARY_TMP_EXTENSION) - 1;
        if (len > tmp_len && strcmp(de->d_name + len - tmp_len, RETENTION_SUMMARY_TMP_EXTENSION) == 0) {
            // a build that never completed
            char path[RRDENG_PATH_MAX];
            snprintfz(path, sizeof(path), "%s/%s", ctx->config.dbfiles_path, de->d_name);
            (void)unlink(path);
            continue;
        }

        RETENTION_SUMMARY_BLOCK *block = rs_claim_one(ctx, de->d_name);
        if (!block)
            continue;

        if (count == size) {
            size = size ? size * 2 : 64;
            *blocks = reallocz(*blocks, size * sizeof(**blocks));
        }
        (*blocks)[count++] = block;
    }
    closedir(dir);

    if (count)
        nd_log_daemon(NDLP_INFO, "DBENGINE: tier %d: loading the retention of %zu journal files from %zu summaries",
                      ctx->config.tier, count * RETENTION_SUMMARY_FILES_PER_BLOCK, count);

    return count;
}

// applies the records; false if the file is damaged (records already applied are valid: their chunks passed their CRC)
static bool rs_apply_records(RETENTION_SUMMARY_BLOCK *block) {
    struct rrdengine_instance *ctx = block->ctx;
    const struct retention_summary_header *h = &block->header;
    char path[RRDENG_PATH_MAX];
    rs_path(ctx, h->block_id, false, path, sizeof(path));

    int fd = open(path, O_RDONLY | O_CLOEXEC);
    if (fd == -1)
        return false;

    bool ok = false;
    uint64_t chunks = rs_chunks(h->record_count);
    uint64_t records_offset = RETENTION_SUMMARY_HEADER_SIZE;
    uint64_t crcs_offset = records_offset + h->record_count * sizeof(struct retention_summary_record);
    uint32_t *chunk_crcs = mallocz((chunks + 1) * sizeof(uint32_t));
    struct retention_summary_record *records = mallocz(RETENTION_SUMMARY_CHUNK_RECORDS * sizeof(*records));

    if (!rs_pread_full(fd, chunk_crcs, (chunks + 1) * sizeof(uint32_t), crcs_offset) ||
        chunk_crcs[chunks] != rs_crc(chunk_crcs, chunks * sizeof(uint32_t)))
        goto done;

    time_t now_s = max_acceptable_collected_time();
    uint64_t remaining = h->record_count;
    for (uint64_t c = 0; c < chunks; c++) {
        size_t n = remaining > RETENTION_SUMMARY_CHUNK_RECORDS ? RETENTION_SUMMARY_CHUNK_RECORDS : (size_t)remaining;
        size_t bytes = n * sizeof(*records);
        if (!rs_pread_full(fd, records, bytes, records_offset + c * RETENTION_SUMMARY_CHUNK_RECORDS * sizeof(*records)) ||
            rs_crc(records, bytes) != chunk_crcs[c])
            goto done;

        for (size_t i = 0; i < n; i++) {
            const struct retention_summary_record *r = &records[i];
            if (r->first_delta_s > r->update_every_last_delta_s || r->update_every_last_delta_s > r->last_delta_s)
                goto done;

            nd_uuid_t uuid;
            uuid_copy(uuid, r->uuid);
            time_t first_s = h->base_time_s + r->first_delta_s;

            // the entry that set update_every, then the newer ones that only
            // raised the last time (see struct retention_summary_record)
            mrg_update_metric_retention_and_granularity_by_uuid(
                main_mrg, (Word_t)ctx, &uuid, first_s,
                h->base_time_s + r->update_every_last_delta_s, r->update_every_s, now_s);

            if (r->update_every_last_delta_s < r->last_delta_s)
                mrg_update_metric_retention_and_granularity_by_uuid(
                    main_mrg, (Word_t)ctx, &uuid, first_s,
                    h->base_time_s + r->last_delta_s, 0, now_s);
        }
        remaining -= n;
    }
    ok = true;

done:
    close(fd);
    freez(chunk_crcs);
    freez(records);
    return ok;
}

size_t retention_summary_apply(RETENTION_SUMMARY_BLOCK *block) {
    struct rrdengine_instance *ctx = block->ctx;
    const struct retention_summary_header *h = &block->header;
    size_t records = 0;

    if (rs_apply_records(block)) {
        records = h->record_count;

        // what journalfile_v2_populate_retention_to_mrg() does per file, besides the walk
        time_t first_s = LONG_MAX;
        for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++) {
            rrdeng_datafile_samples_set(block->datafiles[i], h->files[i].samples, h->files[i].samples_estimated != 0);
            time_t s = (time_t)(h->files[i].start_time_ut / USEC_PER_SEC);
            if (s > 0)
                first_s = MIN(first_s, s);
        }

        time_t old = __atomic_load_n(&ctx->atomic.first_time_s, __ATOMIC_RELAXED);
        do {
            if (old <= first_s)
                break;
        } while (!__atomic_compare_exchange_n(&ctx->atomic.first_time_s, &old, first_s, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));
    }
    else {
        nd_log_daemon(NDLP_WARNING,
                      "DBENGINE: tier %d: retention summary of datafiles %u-%u is damaged; loading them one by one",
                      ctx->config.tier, h->files[0].fileno, h->files[RETENTION_SUMMARY_FILES_PER_BLOCK - 1].fileno);
        rs_forget_and_unlink(ctx, h->block_id);

        for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++)
            journalfile_v2_populate_retention_to_mrg(ctx, block->datafiles[i]->journalfile);
    }

    for (size_t i = 0; i < RETENTION_SUMMARY_FILES_PER_BLOCK; i++)
        datafile_release(block->datafiles[i], DATAFILE_ACQUIRE_RETENTION);

    freez(block);
    return records;
}

// ----------------------------------------------------------------------------
// unit test: summaries + per-file walk == full per-file walk

#define RS_UT_FIRST_FILENO 1    // block 0: 1..24 (numbering starts at 1), and 25 outside a complete block
#define RS_UT_FILES 25
#define RS_UT_METRICS 300

struct rs_ut_env {
    char dir[FILENAME_MAX + 1];
    struct rrdengine_instance *ctx;
    struct rrdengine_datafile *datafiles[RS_UT_FILES];
    time_t t0;
    MRG *saved_main_mrg;
};

static void rs_ut_uuid(uint32_t i, nd_uuid_t uuid) {
    memset(uuid, 0xa5, sizeof(nd_uuid_t));
    uuid[0] = (uint8_t)(i >> 24); uuid[1] = (uint8_t)(i >> 16); uuid[2] = (uint8_t)(i >> 8); uuid[3] = (uint8_t)i;
}

static uint32_t rs_ut_rnd(uint64_t *state) {
    *state = *state * 6364136223846793005ULL + 1442695040888963407ULL;
    return (uint32_t)(*state >> 33);
}

// a minimal valid journal v2: header, one extent, the metric list, no pages,
// and optionally a samples section (exact samples)
static bool rs_ut_write_v2(const char *path, time_t hs, struct journal_metric_list *metrics, uint32_t count, bool with_samples) {
    size_t extent_offset = RRDENG_BLOCK_SIZE;
    size_t extent_trailer_offset = extent_offset + sizeof(struct journal_extent_list);
    size_t metric_offset = extent_trailer_offset + sizeof(struct journal_v2_block_trailer);
    size_t metric_trailer_offset = metric_offset + (size_t)count * sizeof(struct journal_metric_list);
    size_t page_offset = metric_trailer_offset + sizeof(struct journal_v2_block_trailer);
    size_t samples_offset = page_offset;
    size_t samples_size = with_samples ? (size_t)count * sizeof(uint32_t) + sizeof(struct journal_v2_block_trailer) : 0;
    size_t size = page_offset + samples_size + sizeof(struct journal_v2_block_trailer);

    uint8_t *buf = callocz(1, size);
    uint32_t max_end = 0;
    for (uint32_t i = 0; i < count; i++) {
        metrics[i].entries = 1;
        metrics[i].page_offset = (uint32_t)page_offset;
        max_end = MAX(max_end, metrics[i].delta_end_s);
    }

    struct journal_v2_header h = {
        .magic = JOURVAL_V2_MAGIC,
        .start_time_ut = (usec_t)hs * USEC_PER_SEC,
        .end_time_ut = (usec_t)(hs + max_end) * USEC_PER_SEC,
        .extent_count = 1,
        .extent_offset = (uint32_t)extent_offset,
        .metric_count = count,
        .metric_offset = (uint32_t)metric_offset,
        .page_count = 0,
        .page_offset = (uint32_t)page_offset,
        .extent_trailer_offset = (uint32_t)extent_trailer_offset,
        .metric_trailer_offset = (uint32_t)metric_trailer_offset,
        .journal_v1_file_size = 0,
        .journal_v2_file_size = (uint32_t)size,
        .data = NULL,
    };
    memcpy(buf, &h, sizeof(h));

    struct journal_extent_list extent = { .datafile_offset = 0, .datafile_size = RRDENG_BLOCK_SIZE };
    memcpy(buf + extent_offset, &extent, sizeof(extent));
    uint32_t crc = rs_crc(buf + extent_offset, sizeof(extent));
    memcpy(buf + extent_trailer_offset, &crc, sizeof(crc));

    memcpy(buf + metric_offset, metrics, (size_t)count * sizeof(*metrics));
    crc = rs_crc(buf + metric_offset, (size_t)count * sizeof(*metrics));
    memcpy(buf + metric_trailer_offset, &crc, sizeof(crc));

    if (with_samples) {
        uint32_t *samples = (uint32_t *)(buf + samples_offset);
        uint64_t total = 0;
        for (uint32_t i = 0; i < count; i++) {
            samples[i] = 1 + (metrics[i].delta_end_s - metrics[i].delta_start_s) / 3;
            total += samples[i];
        }
        crc = rs_crc(samples, (size_t)count * sizeof(uint32_t));
        memcpy(buf + samples_offset + (size_t)count * sizeof(uint32_t), &crc, sizeof(crc));

        struct journal_v2_samples_descriptor d = {
            .magic = JOURNAL_V2_SAMPLES_MAGIC, .count = count, .samples = total, .offset = (uint32_t)samples_offset,
        };
        d.crc = rs_crc(&d, offsetof(struct journal_v2_samples_descriptor, crc));
        memcpy(buf + JOURNAL_V2_SAMPLES_DESCRIPTOR_OFFSET, &d, sizeof(d));
    }

    crc = rs_crc(&h, sizeof(h));
    memcpy(buf + size - sizeof(crc), &crc, sizeof(crc));

    int fd = open(path, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, S_IRUSR | S_IWUSR);
    bool ok = fd != -1 && rs_write_full(fd, buf, size);
    if (fd != -1)
        close(fd);
    freez(buf);
    return ok;
}

static bool rs_ut_env_init(struct rs_ut_env *env) {
    memset(env, 0, sizeof(*env));
    if (!dbengine_unittest_make_tmpdir(env->dir, sizeof(env->dir))) {
        env->dir[0] = '\0';   // tells the caller to skip
        return false;
    }

    struct rrdengine_instance *ctx = env->ctx = callocz(1, sizeof(*ctx));
    ctx->config.tier = 0;
    strncpyz(ctx->config.dbfiles_path, env->dir, sizeof(ctx->config.dbfiles_path) - 1);
    netdata_rwlock_init(&ctx->datafiles.rwlock);
    rw_spinlock_init(&ctx->njfv2idx.spinlock);
    spinlock_init(&ctx->summaries.spinlock);
    ctx->atomic.first_time_s = LONG_MAX;
    ctx->atomic.last_fileno = RS_UT_FIRST_FILENO + RS_UT_FILES;        // the "current" datafile, not in the list
    ctx->atomic.last_flush_fileno = RS_UT_FIRST_FILENO + RS_UT_FILES;

    env->t0 = now_realtime_sec() - 100000;
    uint64_t rnd = 12345;
    struct journal_metric_list *metrics = callocz(RS_UT_METRICS, sizeof(*metrics));

    for (size_t f = 0; f < RS_UT_FILES; f++) {
        unsigned fileno = RS_UT_FIRST_FILENO + f;
        time_t hs = env->t0 + (time_t)(fileno % 5) * 100;   // not monotonic in fileno, like on parents

        uint32_t count = 0;
        for (uint32_t i = 0; i < RS_UT_METRICS; i++) {
            if (rs_ut_rnd(&rnd) % 4 == 0)
                continue;
            struct journal_metric_list *m = &metrics[count++];
            memset(m, 0, sizeof(*m));
            rs_ut_uuid(i + 1, m->uuid);
            m->delta_start_s = rs_ut_rnd(&rnd) % 400;
            m->delta_end_s = m->delta_start_s + rs_ut_rnd(&rnd) % 600;
            if (i % 7 == 0) {
                // the same last time in many files, with different update_every
                m->delta_end_s = (uint32_t)(env->t0 + 1500 - hs);
                m->delta_start_s = MIN(m->delta_start_s, m->delta_end_s);
            }
            m->update_every_s = (rs_ut_rnd(&rnd) % 5 == 0) ? 0 : 1 + rs_ut_rnd(&rnd) % 3;
        }

        struct rrdengine_datafile *df = env->datafiles[f] = callocz(1, sizeof(*df));
        df->tier = 1;
        df->fileno = fileno;
        df->ctx = ctx;
        df->magic1 = df->magic2 = DATAFILE_MAGIC;
        df->users.available = true;
        netdata_rwlock_init(&df->extent_rwlock);
        spinlock_tracked_init(&df->users.spinlock);
        spinlock_init(&df->writers.spinlock);
        spinlock_init(&df->populate_mrg.spinlock);
        rw_spinlock_init(&df->extent_epdl.spinlock);
        journalfile_alloc_and_init(df);

        char path[RRDENG_PATH_MAX];
        journalfile_v2_generate_path(df, path, sizeof(path));
        if (!rs_ut_write_v2(path, hs, metrics, count, fileno % 2 == 0) || journalfile_v2_load(ctx, df->journalfile, df) != 0) {
            freez(metrics);
            return false;
        }
        datafile_list_insert(ctx, df);
    }

    freez(metrics);
    env->saved_main_mrg = main_mrg;
    return true;
}

static void rs_ut_env_destroy(struct rs_ut_env *env) {
    main_mrg = env->saved_main_mrg;
    for (size_t f = 0; f < RS_UT_FILES; f++) {
        struct rrdengine_datafile *df = env->datafiles[f];
        if (!df)
            continue;
        char path[RRDENG_PATH_MAX];
        journalfile_v2_generate_path(df, path, sizeof(path));
        if (df->journalfile) {
            journalfile_close(df->journalfile, df);
            freez(df->journalfile);
        }
        unlink(path);
        netdata_rwlock_destroy(&df->extent_rwlock);
        freez(df);
    }
    if (env->ctx) {
        DIR *dir = opendir(env->dir);
        if (dir) {
            struct dirent *de;
            while ((de = readdir(dir))) {
                if (de->d_name[0] == '.')
                    continue;
                char path[RRDENG_PATH_MAX];
                snprintfz(path, sizeof(path), "%s/%s", env->dir, de->d_name);
                unlink(path);
            }
            closedir(dir);
        }
        JudyLFreeArray(&env->ctx->datafiles.JudyL, PJE0);
        retention_summary_ctx_cleanup(env->ctx);
        netdata_rwlock_destroy(&env->ctx->datafiles.rwlock);
        freez(env->ctx);
    }
    rmdir(env->dir);
}

// what a population leaves behind, to compare two of them
struct rs_ut_state {
    time_t first[RS_UT_METRICS + 1], last[RS_UT_METRICS + 1];
    uint32_t ue[RS_UT_METRICS + 1];
    bool present[RS_UT_METRICS + 1];
    uint64_t samples[RS_UT_FILES];
    bool estimated[RS_UT_FILES];
    time_t tier_first_s;
};

static void rs_ut_reset(struct rs_ut_env *env, bool seed) {
    struct rrdengine_instance *ctx = env->ctx;
    for (size_t f = 0; f < RS_UT_FILES; f++) {
        env->datafiles[f]->populate_mrg.populated = false;
        rrdeng_datafile_samples_set(env->datafiles[f], 0, true);
    }
    ctx->atomic.first_time_s = LONG_MAX;

    main_mrg = mrg_create_for_unittest();

    // a prior state (as the SQLite prepopulation or another block would leave):
    // the merged records must compose with it exactly like the per-file entries
    if (seed) {
        time_t now_s = max_acceptable_collected_time();
        for (uint32_t i = 1; i <= RS_UT_METRICS; i += 3) {
            nd_uuid_t uuid;
            rs_ut_uuid(i, uuid);
            mrg_update_metric_retention_and_granularity_by_uuid(
                main_mrg, (Word_t)ctx, &uuid, env->t0 + 50, env->t0 + (time_t)((i * 37) % 2000), 7, now_s);
        }
    }
}

static int rs_ut_capture_and_destroy(struct rs_ut_env *env, struct rs_ut_state *st) {
    memset(st, 0, sizeof(*st));
    for (uint32_t i = 1; i <= RS_UT_METRICS; i++) {
        nd_uuid_t uuid;
        rs_ut_uuid(i, uuid);
        METRIC *m = mrg_metric_get_and_acquire_by_uuid(main_mrg, &uuid, (Word_t)env->ctx);
        if (!m)
            continue;
        st->present[i] = true;
        mrg_metric_get_retention(main_mrg, m, &st->first[i], &st->last[i], &st->ue[i]);
        mrg_metric_release(main_mrg, m);
    }
    for (size_t f = 0; f < RS_UT_FILES; f++) {
        st->samples[f] = env->datafiles[f]->samples.charged;
        st->estimated[f] = env->datafiles[f]->samples.estimated;
    }
    st->tier_first_s = env->ctx->atomic.first_time_s;

    int errors = 0;
    if (mrg_destroy(main_mrg)) {
        fprintf(stderr, "ERROR: retention summary test: MRG metrics still referenced\n");
        errors++;
    }
    main_mrg = NULL;
    return errors;
}

static void rs_ut_full_walk(struct rs_ut_env *env) {
    for (size_t f = 0; f < RS_UT_FILES; f++)
        journalfile_v2_populate_retention_to_mrg(env->ctx, env->datafiles[f]->journalfile);
}

// claims and applies the summaries, then walks whatever they did not cover; returns the blocks claimed
static size_t rs_ut_summary_walk(struct rs_ut_env *env, size_t *records_applied) {
    RETENTION_SUMMARY_BLOCK **blocks = NULL;
    size_t n = retention_summary_claim(env->ctx, &blocks);
    size_t records = 0;
    for (size_t i = 0; i < n; i++)
        records += retention_summary_apply(blocks[i]);
    freez(blocks);
    if (records_applied)
        *records_applied = records;

    for (size_t f = 0; f < RS_UT_FILES; f++)
        if (!env->datafiles[f]->populate_mrg.populated)
            journalfile_v2_populate_retention_to_mrg(env->ctx, env->datafiles[f]->journalfile);
    return n;
}

static int rs_ut_compare(const struct rs_ut_state *want, const struct rs_ut_state *got, const char *when) {
    int errors = 0;
    for (uint32_t i = 1; i <= RS_UT_METRICS; i++) {
        if (want->present[i] != got->present[i] || want->first[i] != got->first[i] ||
            want->last[i] != got->last[i] || want->ue[i] != got->ue[i]) {
            if (errors < 5)
                fprintf(stderr, "ERROR: %s: metric %u: want %d %ld..%ld ue %u, got %d %ld..%ld ue %u\n", when, i,
                        want->present[i], (long)want->first[i], (long)want->last[i], want->ue[i],
                        got->present[i], (long)got->first[i], (long)got->last[i], got->ue[i]);
            errors++;
        }
    }
    for (size_t f = 0; f < RS_UT_FILES; f++) {
        if (want->samples[f] != got->samples[f] || want->estimated[f] != got->estimated[f]) {
            fprintf(stderr, "ERROR: %s: datafile %zu samples want %" PRIu64 "%s, got %" PRIu64 "%s\n", when, f,
                    want->samples[f], want->estimated[f] ? " (estimated)" : "",
                    got->samples[f], got->estimated[f] ? " (estimated)" : "");
            errors++;
        }
    }
    if (want->tier_first_s != got->tier_first_s) {
        fprintf(stderr, "ERROR: %s: tier first time want %ld, got %ld\n", when, (long)want->tier_first_s, (long)got->tier_first_s);
        errors++;
    }
    return errors;
}

static bool rs_ut_summary_exists(struct rs_ut_env *env, uint32_t block_id) {
    char path[RRDENG_PATH_MAX];
    rs_path(env->ctx, block_id, false, path, sizeof(path));
    return access(path, F_OK) == 0;
}

static bool rs_ut_patch(struct rs_ut_env *env, uint32_t block_id, uint64_t offset, const void *data, size_t len) {
    char path[RRDENG_PATH_MAX];
    rs_path(env->ctx, block_id, false, path, sizeof(path));
    int fd = open(path, O_WRONLY | O_CLOEXEC);
    bool ok = fd != -1 && pwrite(fd, data, len, (off_t)offset) == (ssize_t)len;
    if (fd != -1)
        close(fd);
    return ok;
}

int retention_summary_unittest(void) {
    fprintf(stderr, "\nTesting retention summaries (summary + per-file walk == full walk)...\n");
    int errors = 0;
    struct rs_ut_env env;
    struct rs_ut_state want, want_seeded, got;

    if (!rs_ut_env_init(&env)) {
        if (!env.dir[0]) {
            // no usable temporary directory: an environment problem, skipped like the jv2 writer tests
            fprintf(stderr, "SKIPPED: retention summary test: no usable temporary directory\n");
            return 0;
        }
        fprintf(stderr, "ERROR: retention summary test: cannot create the journal files\n");
        rs_ut_env_destroy(&env);
        return 1;
    }
    const uint32_t block = (RS_UT_FIRST_FILENO - 1) / RETENTION_SUMMARY_FILES_PER_BLOCK;

    // the reference: every file walked, in fileno order
    rs_ut_reset(&env, false);
    rs_ut_full_walk(&env);
    errors += rs_ut_capture_and_destroy(&env, &want);

    rs_ut_reset(&env, true);
    rs_ut_full_walk(&env);
    errors += rs_ut_capture_and_destroy(&env, &want_seeded);

    // build: one complete block (1..24); 25 is alone in the next block
    if (!retention_summary_build_one(env.ctx) || !rs_ut_summary_exists(&env, block)) {
        fprintf(stderr, "ERROR: retention summary test: the complete block was not summarised\n");
        errors++;
    }
    if (retention_summary_build_one(env.ctx)) {
        fprintf(stderr, "ERROR: retention summary test: built a second summary, but only one block is complete\n");
        errors++;
    }

    // 1. summary + walk of the rest == full walk, from empty and from a seeded MRG
    // the summary must really be used (not a silent fallback to the walk)
    for (int seeded = 0; seeded < 2; seeded++) {
        size_t records = 0;
        rs_ut_reset(&env, seeded);
        if (rs_ut_summary_walk(&env, &records) != 1 || !records || !rs_ut_summary_exists(&env, block)) {
            fprintf(stderr, "ERROR: retention summary test: the summary was not applied (%zu records)\n", records);
            errors++;
        }
        errors += rs_ut_capture_and_destroy(&env, &got);
        errors += rs_ut_compare(seeded ? &want_seeded : &want, &got, seeded ? "summary over a seeded MRG" : "summary");
    }

    // both kinds of datafiles are covered: exact (samples section) and estimated
    {
        size_t exact = 0, estimated = 0;
        for (size_t f = 0; f < RS_UT_FILES; f++)
            want.estimated[f] ? estimated++ : exact++;
        if (!exact || !estimated) {
            fprintf(stderr, "ERROR: retention summary test: %zu exact and %zu estimated datafiles, want both\n", exact, estimated);
            errors++;
        }
    }

    // 2. a damaged record chunk: the block falls back to the walk, the summary is deleted
    {
        uint8_t garbage[8] = { 0xde, 0xad, 0xbe, 0xef, 0xde, 0xad, 0xbe, 0xef };
        rs_ut_patch(&env, block, RETENTION_SUMMARY_HEADER_SIZE + 40, garbage, sizeof(garbage));
        rs_ut_reset(&env, false);
        rs_ut_summary_walk(&env, NULL);
        errors += rs_ut_capture_and_destroy(&env, &got);
        errors += rs_ut_compare(&want, &got, "damaged records");
        if (rs_ut_summary_exists(&env, block)) {
            fprintf(stderr, "ERROR: retention summary test: a damaged summary was not deleted\n");
            errors++;
        }
    }

    // 3. a damaged header, and a summary of other journals: rejected at claim and deleted
    for (int variant = 0; variant < 2; variant++) {
        if (!retention_summary_build_one(env.ctx) || !rs_ut_summary_exists(&env, block)) {
            fprintf(stderr, "ERROR: retention summary test: cannot rebuild the summary\n");
            errors++;
            break;
        }
        if (variant == 0) {
            uint32_t bad = 0xffffffff;
            rs_ut_patch(&env, block, offsetof(struct retention_summary_header, record_count), &bad, sizeof(bad));
        }
        else {
            // a valid header whose identity says another metric-list CRC
            struct retention_summary_header h;
            char path[RRDENG_PATH_MAX];
            rs_path(env.ctx, block, false, path, sizeof(path));
            int fd = open(path, O_RDONLY | O_CLOEXEC);
            bool ok = fd != -1 && rs_pread_full(fd, &h, sizeof(h), 0);
            if (fd != -1)
                close(fd);
            if (ok) {
                h.files[5].metric_list_crc ^= 1;
                h.crc = rs_header_crc(&h);
                rs_ut_patch(&env, block, 0, &h, sizeof(h));
            }
        }
        rs_ut_reset(&env, false);
        if (rs_ut_summary_walk(&env, NULL) != 0) {
            fprintf(stderr, "ERROR: retention summary test: a %s summary was claimed\n", variant ? "stale" : "damaged");
            errors++;
        }
        errors += rs_ut_capture_and_destroy(&env, &got);
        errors += rs_ut_compare(&want, &got, variant ? "stale identity" : "damaged header");
        if (rs_ut_summary_exists(&env, block)) {
            fprintf(stderr, "ERROR: retention summary test: an unusable summary was not deleted\n");
            errors++;
        }
        // the builder must be allowed to build it again
        spinlock_lock(&env.ctx->summaries.spinlock);
        (void)JudyLDel(&env.ctx->summaries.JudyL, block, PJE0);
        spinlock_unlock(&env.ctx->summaries.spinlock);
    }

    // 4. a leftover temp file is deleted at claim
    {
        char path[RRDENG_PATH_MAX];
        rs_path(env.ctx, block, true, path, sizeof(path));
        int fd = open(path, O_WRONLY | O_CREAT | O_CLOEXEC, S_IRUSR | S_IWUSR);
        if (fd != -1)
            close(fd);
        rs_ut_reset(&env, false);
        rs_ut_summary_walk(&env, NULL);
        errors += rs_ut_capture_and_destroy(&env, &got);
        if (access(path, F_OK) == 0) {
            fprintf(stderr, "ERROR: retention summary test: a temporary file was not deleted\n");
            errors++;
        }
    }

    // 5. deleting a datafile of the block deletes its summary
    if (!retention_summary_build_one(env.ctx) || !rs_ut_summary_exists(&env, block)) {
        fprintf(stderr, "ERROR: retention summary test: cannot rebuild the summary\n");
        errors++;
    }
    else {
        retention_summary_datafile_deleted(env.ctx, RS_UT_FIRST_FILENO + 3);
        if (rs_ut_summary_exists(&env, block)) {
            fprintf(stderr, "ERROR: retention summary test: deleting a datafile kept its block summary\n");
            errors++;
        }
    }

    rs_ut_env_destroy(&env);

    fprintf(stderr, "retention summary test: %s (%d errors)\n", errors ? "FAILED" : "OK", errors);
    return errors;
}
