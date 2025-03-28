// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-dump.h"

// Context for file reading
typedef struct {
    int fd;                          // File descriptor
    mrg_file_header_t header;        // File header
    void *compressed_buffer;         // Buffer for compressed data
    void *uncompressed_buffer;       // Buffer for uncompressed data
} mrg_file_load_ctx_t;

// Initialize the file context for loading
static mrg_file_load_ctx_t *mrg_file_load_ctx_create(void) {
    mrg_file_load_ctx_t *ctx = callocz(1, sizeof(mrg_file_load_ctx_t));

    // Allocate buffers
    ctx->uncompressed_buffer = mallocz(MRG_FILE_PAGE_SIZE);
    size_t max_compressed_size = ZSTD_compressBound(MRG_FILE_PAGE_SIZE);
    ctx->compressed_buffer = mallocz(max_compressed_size);

    return ctx;
}

// Clean up the file context
static void mrg_file_load_ctx_destroy(mrg_file_load_ctx_t *ctx) {
    if (!ctx) return;

    freez(ctx->uncompressed_buffer);
    freez(ctx->compressed_buffer);

    if (ctx->fd >= 0)
        close(ctx->fd);

    freez(ctx);
}

// Read the file header
static bool mrg_file_read_header(mrg_file_load_ctx_t *ctx) {
    if (lseek(ctx->fd, 0, SEEK_SET) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to seek to beginning of file: %s", strerror(errno));
        return false;
    }

    if (read(ctx->fd, &ctx->header, sizeof(mrg_file_header_t)) != sizeof(mrg_file_header_t)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to read file header: %s", strerror(errno));
        return false;
    }

    // Verify magic
    if (memcmp(ctx->header.magic, "NETDMRG", 7) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Invalid magic in file header");
        return false;
    }

    // Verify version
    if (ctx->header.version != 1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Unsupported version %u", ctx->header.version);
        return false;
    }

    return true;
}

// Read a page from the file
static ssize_t mrg_file_read_page(mrg_file_load_ctx_t *ctx, uint64_t offset, mrg_page_header_t *header, void *data) {
    // Seek to the specified offset in the file
    if (lseek(ctx->fd, offset, SEEK_SET) != (off_t)offset) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to seek to offset %"PRIu64": %s",
               offset, strerror(errno));
        return -1;
    }

    // Read page header
    ssize_t bytes_read = read(ctx->fd, header, sizeof(mrg_page_header_t));
    if (bytes_read != sizeof(mrg_page_header_t)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to read page header: %s", strerror(errno));
        return -1;
    }

    // Verify page magic
    if (memcmp(header->magic, "MRGP", 4) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Invalid magic in page header at offset %"PRIu64" (got: %02x %02x %02x %02x)",
               offset,
               (unsigned char)header->magic[0],
               (unsigned char)header->magic[1],
               (unsigned char)header->magic[2],
               (unsigned char)header->magic[3]);
        return -1;
    }

    // Validate compressed size
    if (header->compressed_size == 0 || header->compressed_size > ZSTD_compressBound(MRG_FILE_PAGE_SIZE)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Invalid compressed size %u at offset %"PRIu64,
               header->compressed_size, offset);
        return -1;
    }

    // Read compressed data
    bytes_read = read(ctx->fd, ctx->compressed_buffer, header->compressed_size);
    if (bytes_read != (ssize_t)header->compressed_size) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to read compressed data (%zd of %u bytes): %s",
               bytes_read, header->compressed_size, strerror(errno));
        return -1;
    }

    // Decompress the data
    size_t result = ZSTD_decompress(
        data,
        header->uncompressed_size,
        ctx->compressed_buffer,
        header->compressed_size
    );

    if (ZSTD_isError(result)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: ZSTD decompression failed: %s",
               ZSTD_getErrorName(result));
        return -1;
    }

    if (result != header->uncompressed_size) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Decompressed size mismatch: expected %u, got %zu",
               header->uncompressed_size, result);
        return -1;
    }

    return header->uncompressed_size;
}

DEFINE_JUDYL_TYPED(METRIC, METRIC *);
METRIC_JudyLSet acquired_metrics = { 0 };
size_t acquired_metrics_counter = 0;
size_t acquired_metrics_deleted = 0;

ALWAYS_INLINE
static void mrg_metric_prepopulate(MRG *mrg, Word_t section, nd_uuid_t *uuid) {
    MRG_ENTRY entry = {
        .uuid = uuid,
        .section = section,
        .first_time_s = 0,
        .last_time_s = 0,
        .latest_update_every_s = 0,
    };
    bool added = false;
    METRIC *metric = metric_add_and_acquire(mrg, &entry, &added);
    if(likely(added)) {
        METRIC_SET(&acquired_metrics, acquired_metrics_counter++, metric);
        return;
    }
    mrg_metric_release(mrg, metric);
}

static void mrg_release_cb(Word_t idx __maybe_unused, METRIC *m, void *data) {
    MRG *mrg = data;
    if(mrg_metric_release(mrg, m))
        acquired_metrics_deleted++;
}

void mrg_metric_prepopulate_cleanup(MRG *mrg) {
    acquired_metrics_deleted = 0;
    METRIC_FREE(&acquired_metrics, mrg_release_cb, mrg);

    if(acquired_metrics_counter || acquired_metrics_deleted)
        nd_log(NDLS_DAEMON, NDLP_INFO, "MRG DUMP: Prepopulated %zu metrics, released %zu, deleted %zu",
               acquired_metrics_counter, acquired_metrics_counter - acquired_metrics_deleted, acquired_metrics_deleted);

    acquired_metrics_counter = 0;
}

// Process a metric page
static void mrg_file_process_metric_page(MRG *mrg, mrg_file_load_ctx_t *ctx __maybe_unused,
                                         mrg_page_header_t *header,
                                         void *data, size_t data_size,
                                         uint32_t *processed_metrics) {
    // Calculate the number of metrics in the page
    size_t metrics_count = data_size / sizeof(mrg_file_metric_t);
    if (metrics_count != header->entries_count) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "MRG DUMP: Metrics count mismatch: expected %u, calculated %zu",
               header->entries_count, metrics_count);
    }

    // Process each metric
    mrg_file_metric_t *metrics = (mrg_file_metric_t *)data;
    for (size_t i = 0; i < metrics_count; i++) {
        size_t tier = metrics[i].tier;

        // Check if the tier is valid
        if (tier >= nd_profile.storage_tiers) {
            nd_log(NDLS_DAEMON, NDLP_WARNING, "MRG DUMP: Skipping metric with invalid tier %zu", tier);
            continue;
        }

        // Check if the context exists
        if (!multidb_ctx[tier]) {
            nd_log(NDLS_DAEMON, NDLP_WARNING, "MRG DUMP: Tier %zu context is not initialized", tier);
            continue;
        }

        mrg_metric_prepopulate(mrg, (Word_t)multidb_ctx[tier], &metrics[i].uuid.uuid);
        (*processed_metrics)++;
    }
}

// Process a file entry page
static void mrg_file_process_file_page(mrg_file_load_ctx_t *ctx __maybe_unused,
                                       mrg_page_header_t *header,
                                       void *data, size_t data_size,
                                       uint32_t *processed_files) {
    // Calculate the number of file entries in the page
    size_t files_count = data_size / sizeof(mrg_file_entry_t);
    if (files_count != header->entries_count) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "MRG DUMP: File entries count mismatch: expected %u, calculated %zu",
               header->entries_count, files_count);
    }

    // Process each file entry
    mrg_file_entry_t *files = (mrg_file_entry_t *)data;
    for (size_t i = 0; i < files_count; i++) {
        size_t tier = files[i].tier;

        // Check if the tier is valid
        if (tier >= nd_profile.storage_tiers) {
            nd_log(NDLS_DAEMON, NDLP_WARNING, "MRG DUMP: File entry has invalid tier %zu", tier);
            continue;
        }

        // Check if file exists
        char filepath[FILENAME_MAX + 1];
        if(tier == 0)
            snprintfz(filepath, FILENAME_MAX, "%s/dbengine/" DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION,
                      netdata_configured_cache_dir, (unsigned)1, (unsigned)files[i].fileno);
        else
            snprintfz(filepath, FILENAME_MAX, "%s/dbengine-tier%u/" DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION,
                      netdata_configured_cache_dir, (unsigned)tier, (unsigned)1, (unsigned)files[i].fileno);

        struct stat st;
        if (stat(filepath, &st) != 0) {
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "MRG DUMP: Data file %s not found", filepath);
            continue;
        }

        // Verify file size
        if (st.st_size != (off_t)files[i].size) {
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "MRG DUMP: Data file %s size mismatch: expected %"PRIu64", found %ld",
                   filepath, files[i].size, (long)st.st_size);
            continue;
        }

        // Verify modification time
        usec_t file_mtime = STAT_GET_MTIME_SEC(st) * USEC_PER_SEC + STAT_GET_MTIME_NSEC(st) / 1000;
        if (file_mtime != files[i].mtime) {
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "MRG DUMP: Data file %s modification time mismatch", filepath);
            continue;
        }

        (*processed_files)++;
    }
}

// Traverse the chain of pages starting from the last one
static bool mrg_file_traverse_pages(MRG *mrg, mrg_file_load_ctx_t *ctx,
                                    mrg_page_type_t type,
                                    uint64_t last_offset,
                                    uint32_t *processed_metrics,
                                    uint32_t *processed_files) {
    uint64_t offset = last_offset;
    while (offset > 0) {
        // Debug logging for page traversal
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "MRG DUMP: Processing page at offset %"PRIu64" of type %u",
               offset, (unsigned)type);

        mrg_page_header_t header;
        ssize_t size = mrg_file_read_page(ctx, offset, &header, ctx->uncompressed_buffer);
        if (size < 0) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to read page at offset %"PRIu64, offset);
            return false;
        }

        // Verify page type
        if (header.type != type) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "MRG DUMP: Page type mismatch at offset %"PRIu64": expected %u, got %u",
                   offset, type, header.type);
            return false;
        }

        // Process page based on type
        switch(type) {
            case MRG_PAGE_TYPE_METRIC:
                mrg_file_process_metric_page(mrg, ctx, &header, ctx->uncompressed_buffer, size, processed_metrics);
                break;

            case MRG_PAGE_TYPE_FILE:
                mrg_file_process_file_page(ctx, &header, ctx->uncompressed_buffer, size, processed_files);
                break;

            default:
                nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Invalid page type %u", type);
                return false;
        }

        // Move to previous page
        offset = header.prev_offset;
    }

    return true;
}

// Main function to load metrics from file
bool mrg_dump_load(MRG *mrg) {
    usec_t started = now_monotonic_usec();

    // Check if the file exists
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/" MRG_FILE_NAME, netdata_configured_cache_dir);

    if (access(filename, F_OK) != 0) {
        nd_log(NDLS_DAEMON, NDLP_INFO, "MRG DUMP: File %s does not exist", filename);
        return false;
    }

    // Open the file
    int fd = open(filename, O_RDONLY);
    if (fd < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "MRG DUMP: Failed to open file %s: %s", filename, strerror(errno));
        return false;
    }

    // Initialize context
    mrg_file_load_ctx_t *ctx = mrg_file_load_ctx_create();
    ctx->fd = fd;

    // Read the header
    if (!mrg_file_read_header(ctx)) {
        mrg_file_load_ctx_destroy(ctx);
        return false;
    }

    // Verify number of tiers
    if (ctx->header.tiers_count != nd_profile.storage_tiers) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "MRG DUMP: Wrong number of tiers (%u in file, %zu expected)",
               ctx->header.tiers_count, nd_profile.storage_tiers);
        mrg_file_load_ctx_destroy(ctx);
        return false;
    }

    // Load metrics and files
    uint32_t processed_metrics = 0;
    uint32_t processed_files = 0;
    bool success = true;

    // Process metrics pages
    if (ctx->header.metric_pages.last_offset > 0) {
        if (!mrg_file_traverse_pages(mrg, ctx, MRG_PAGE_TYPE_METRIC,
                                     ctx->header.metric_pages.last_offset,
                                     &processed_metrics, NULL)) {
            success = false;
        }
    }

    // Process file entries
    if (success && ctx->header.file_pages.last_offset > 0) {
        if (!mrg_file_traverse_pages(mrg, ctx, MRG_PAGE_TYPE_FILE,
                                     ctx->header.file_pages.last_offset,
                                     NULL, &processed_files)) {
            success = false;
        }
    }

    // Clean up
    mrg_file_load_ctx_destroy(ctx);

    if (!success) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG DUMP: Failed to load metrics from file");
        return false;
    }

    usec_t ended = now_monotonic_usec();
    char dt[32];
    duration_snprintf(dt, sizeof(dt), ended - started, "us", false);

    nd_log(NDLS_DAEMON, NDLP_INFO,
           "MRG DUMP: Loaded %u metrics and verified %u files in %s",
           processed_metrics, processed_files, dt);

    return processed_metrics > 0;
}
