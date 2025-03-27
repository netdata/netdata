// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-dump.h"

// MRG file format base timestamp (Jan 1st, 2010)
#define MRG_FILE_BASE_TIMESTAMP 1262304000

// ZSTD compression level (1-22, higher = better compression but slower)
#define MRG_FILE_COMPRESSION_LEVEL 3

// Initialize the file context for saving
static mrg_file_ctx_t *mrg_file_ctx_create(void) {
    mrg_file_ctx_t *ctx = callocz(1, sizeof(mrg_file_ctx_t));

    // Initialize header
    memcpy(ctx->header.magic, "NETDMRG", 8);
    ctx->header.version = 1;
    ctx->header.base_time = MRG_FILE_BASE_TIMESTAMP;
    ctx->header.compression_level = MRG_FILE_COMPRESSION_LEVEL;
    ctx->header.tiers_count = nd_profile.storage_tiers;

    // Allocate buffers
    ctx->metric_pages.buffer = mallocz(MRG_FILE_PAGE_SIZE);
    ctx->file_pages.buffer = mallocz(MRG_FILE_PAGE_SIZE);

    // Allocate compressed buffer (worst case ZSTD size)
    size_t max_compressed_size = ZSTD_compressBound(MRG_FILE_PAGE_SIZE);
    ctx->compressed_buffer = mallocz(max_compressed_size);

    return ctx;
}

// Clean up the file context
static void mrg_file_ctx_destroy(mrg_file_ctx_t *ctx) {
    if (!ctx) return;

    // Free all buffers
    freez(ctx->metric_pages.buffer);
    freez(ctx->file_pages.buffer);
    freez(ctx->compressed_buffer);

    if (ctx->fd >= 0)
        close(ctx->fd);

    freez(ctx);
}

// Write the file header
static bool mrg_file_write_header(mrg_file_ctx_t *ctx) {
    if (lseek(ctx->fd, 0, SEEK_SET) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to seek to beginning of file: %s", strerror(errno));
        return false;
    }

    if (write(ctx->fd, &ctx->header, sizeof(mrg_file_header_t)) != sizeof(mrg_file_header_t)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to write file header: %s", strerror(errno));
        return false;
    }

    return true;
}

// Write a compressed page to the file
static bool mrg_file_write_page(mrg_file_ctx_t *ctx, mrg_page_type_t type,
                                void *data, uint32_t data_size, uint32_t entries_count,
                                uint64_t *prev_offset) {
    if (data_size == 0) return true;

    // Ensure we're at the correct file position
    if (lseek(ctx->fd, ctx->file_size, SEEK_SET) != (off_t)ctx->file_size) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to seek to position %"PRIu64": %s",
               ctx->file_size, strerror(errno));
        return false;
    }

    // Store the current position as the starting offset for this page
    uint64_t current_page_offset = ctx->file_size;

    // Prepare page header with MRGP magic
    mrg_page_header_t page_header;
    memcpy(page_header.magic, "MRGP", 4);
    page_header.type = type;
    page_header.prev_offset = *prev_offset;
    page_header.compressed_size = 0; // Will be set after compression
    page_header.uncompressed_size = data_size;
    page_header.entries_count = entries_count;
    memset(page_header.reserved, 0, sizeof(page_header.reserved));

    // Compress the data
    size_t compressed_size = ZSTD_compress(
        ctx->compressed_buffer,
        ZSTD_compressBound(data_size),
        data,
        data_size,
        ctx->header.compression_level
    );

    if (ZSTD_isError(compressed_size)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: ZSTD compression failed: %s",
               ZSTD_getErrorName(compressed_size));
        return false;
    }

    // Update the compressed size in the header
    page_header.compressed_size = (uint32_t)compressed_size;

    // Write page header
    if (write(ctx->fd, &page_header, sizeof(mrg_page_header_t)) != sizeof(mrg_page_header_t)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to write page header: %s", strerror(errno));
        return false;
    }

    // Write compressed data
    if (write(ctx->fd, ctx->compressed_buffer, compressed_size) != (ssize_t)compressed_size) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to write compressed data: %s", strerror(errno));
        return false;
    }

    // Update the previous offset to the current page's offset
    *prev_offset = current_page_offset;

    // Update file size
    ctx->file_size = current_page_offset + sizeof(mrg_page_header_t) + compressed_size;

    return true;
}

// Flush the metric buffer to disk
static bool mrg_file_flush_metric_buffer(mrg_file_ctx_t *ctx) {
    if (ctx->metric_pages.size == 0) return true;

    uint64_t *prev_offset = &ctx->header.metric_pages.last_offset;
    bool success = mrg_file_write_page(
        ctx,
        MRG_PAGE_TYPE_METRIC,
        ctx->metric_pages.buffer,
        ctx->metric_pages.size,
        ctx->metric_pages.entries,
        prev_offset
    );

    if (success) {
        ctx->header.metric_pages.page_count++;
        ctx->header.metrics_count += ctx->metric_pages.entries;
        ctx->metric_pages.size = 0;
        ctx->metric_pages.entries = 0;
    }

    return success;
}

// Flush the file buffer to disk
static bool mrg_file_flush_file_buffer(mrg_file_ctx_t *ctx) {
    if (ctx->file_pages.size == 0) return true;

    uint64_t *prev_offset = &ctx->header.file_pages.last_offset;
    bool success = mrg_file_write_page(
        ctx,
        MRG_PAGE_TYPE_FILE,
        ctx->file_pages.buffer,
        ctx->file_pages.size,
        ctx->file_pages.entries,
        prev_offset
    );

    if (success) {
        ctx->header.file_pages.page_count++;
        ctx->header.files_count += ctx->file_pages.entries;
        ctx->file_pages.size = 0;
        ctx->file_pages.entries = 0;
    }

    return success;
}

// Add a metric to the buffer
static bool mrg_file_add_metric(mrg_file_ctx_t *ctx, size_t tier, ND_UUID uuid,
                                time_t first_time_s, time_t last_time_s, uint32_t update_every_s) {
    // Check if we need to flush the buffer
    if (ctx->metric_pages.size + sizeof(mrg_file_metric_t) > MRG_FILE_PAGE_SIZE) {
        if (!mrg_file_flush_metric_buffer(ctx))
            return false;
    }

    // Add the metric to the buffer
    mrg_file_metric_t *metric = (mrg_file_metric_t *)(ctx->metric_pages.buffer + ctx->metric_pages.size);
    metric->uuid = uuid;
    metric->tier = tier;
    metric->first_time = first_time_s - ctx->header.base_time;
    metric->last_time = last_time_s - ctx->header.base_time;
    metric->update_every = update_every_s;

    ctx->metric_pages.size += sizeof(mrg_file_metric_t);
    ctx->metric_pages.entries++;

    return true;
}

// Add a file entry to the file buffer
static bool mrg_file_add_file(mrg_file_ctx_t *ctx, size_t tier,
                              size_t fileno, size_t size, usec_t mtime) {
    // Check if we need to flush the buffer
    if (ctx->file_pages.size + sizeof(mrg_file_entry_t) > MRG_FILE_PAGE_SIZE) {
        if (!mrg_file_flush_file_buffer(ctx))
            return false;
    }

    // Add the file to the buffer
    mrg_file_entry_t *file = (mrg_file_entry_t *)(ctx->file_pages.buffer + ctx->file_pages.size);
    file->tier = tier;
    file->fileno = fileno;
    file->size = size;
    file->mtime = mtime;

    ctx->file_pages.size += sizeof(mrg_file_entry_t);
    ctx->file_pages.entries++;

    return true;
}

// Main function to save metrics to file
bool mrg_dump_save(MRG *mrg) {
    usec_t started = now_monotonic_usec();

    // Create the temporary file
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/" MRG_FILE_TMP_NAME, netdata_configured_cache_dir);

    int fd = open(filename, O_RDWR | O_CREAT | O_TRUNC, 0664);
    if (fd < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to create file %s: %s",
               filename, strerror(errno));
        return false;
    }

    // Initialize context
    mrg_file_ctx_t *ctx = mrg_file_ctx_create();
    ctx->fd = fd;

    // Write a placeholder header
    if (!mrg_file_write_header(ctx)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to write placeholder header");
        mrg_file_ctx_destroy(ctx);
        return false;
    }

    // Set initial file size to header size
    ctx->file_size = sizeof(mrg_file_header_t);

    // Ensure we're at the correct position after the header
    if (lseek(fd, ctx->file_size, SEEK_SET) != (off_t)ctx->file_size) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to seek past header: %s", strerror(errno));
        mrg_file_ctx_destroy(ctx);
        return false;
    }

    // Process metrics (iterate through all partitions)
    uint32_t metrics_added = 0;
    uint32_t metrics_skipped = 0;

    for (size_t i = 0; i < UUIDMAP_PARTITIONS; i++) {
        mrg_index_read_lock(mrg, i);

        Word_t uuid_index = 0;

        // Traverse all UUIDs in this partition
        for (Pvoid_t *uuid_pvalue = JudyLFirst(mrg->index[i].uuid_judy, &uuid_index, PJE0);
             uuid_pvalue != NULL && uuid_pvalue != PJERR;
             uuid_pvalue = JudyLNext(mrg->index[i].uuid_judy, &uuid_index, PJE0)) {

            Pvoid_t sections_judy = *uuid_pvalue;
            Word_t section_index = 0;

            for (Pvoid_t *section_pvalue = JudyLFirst(sections_judy, &section_index, PJE0);
                 section_pvalue != NULL && section_pvalue != PJERR;
                 section_pvalue = JudyLNext(sections_judy, &section_index, PJE0)) {

                METRIC *m = *section_pvalue;

                if (unlikely(!m->first_time_s || !m->latest_time_s_clean)) {
                    metrics_skipped++;
                    continue;
                }

                struct rrdengine_instance *ctx_instance = (struct rrdengine_instance *)m->section;
                nd_uuid_t uuid_buf;
                uuidmap_uuid(m->uuid, uuid_buf);
                ND_UUID uuid = uuid2UUID(uuid_buf);

                if (!mrg_file_add_metric(
                        ctx,
                        ctx_instance->config.tier,
                        uuid,
                        m->first_time_s,
                        m->latest_time_s_clean,
                        m->latest_update_every_s)) {
                    mrg_index_read_unlock(mrg, i);
                    mrg_file_ctx_destroy(ctx);
                    return false;
                }

                metrics_added++;
            }
        }

        mrg_index_read_unlock(mrg, i);
    }

    // Process files
    uint32_t files_added = 0;
    for (size_t tier = 0; tier < RRD_STORAGE_TIERS; tier++) {
        if (!multidb_ctx[tier]) continue;

        uv_rwlock_rdlock(&multidb_ctx[tier]->datafiles.rwlock);

        for (struct rrdengine_datafile *d = multidb_ctx[tier]->datafiles.first; d; d = d->next) {
            char filepath[FILENAME_MAX + 1];
            generate_datafilepath(d, filepath, sizeof(filepath));

            struct stat st;
            if (stat(filepath, &st) != 0) {
                nd_log(NDLS_DAEMON, NDLP_WARNING,
                       "MRG DUMP: Failed to stat() %s: %s", filepath, strerror(errno));
                continue;
            }

            if (!mrg_file_add_file(
                    ctx,
                    tier,
                    d->fileno,
                    st.st_size,
                    STAT_GET_MTIME_SEC(st) * USEC_PER_SEC + STAT_GET_MTIME_NSEC(st) / 1000)) {
                uv_rwlock_rdunlock(&multidb_ctx[tier]->datafiles.rwlock);
                mrg_file_ctx_destroy(ctx);
                return false;
            }

            files_added++;
        }

        uv_rwlock_rdunlock(&multidb_ctx[tier]->datafiles.rwlock);
    }

    // Flush any remaining buffers
    bool success = true;

    if (!mrg_file_flush_metric_buffer(ctx))
        success = false;

    if (success && !mrg_file_flush_file_buffer(ctx))
        success = false;

    // Write the final header with updated offsets and counts
    if (success && !mrg_file_write_header(ctx))
        success = false;

    // Clean up
    close(ctx->fd);
    ctx->fd = -1;
    mrg_file_ctx_destroy(ctx);

    if (!success) {
        unlink(filename);
        return false;
    }

    // Rename the temporary file to the final name
    char final_filename[FILENAME_MAX + 1];
    snprintfz(final_filename, FILENAME_MAX, "%s/" MRG_FILE_NAME, netdata_configured_cache_dir);

    if (rename(filename, final_filename) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "MRG DUMP: Failed to rename %s to %s: %s",
               filename, final_filename, strerror(errno));
        unlink(filename);
        return false;
    }

    usec_t ended = now_monotonic_usec();
    char dt[32];
    duration_snprintf(dt, sizeof(dt), ended - started, "us", false);

    nd_log(NDLS_DAEMON, NDLP_INFO,
           "MRG DUMP: Saved %u metrics (%u skipped) and %u files in %s",
           metrics_added, metrics_skipped, files_added, dt);

    return true;
}
