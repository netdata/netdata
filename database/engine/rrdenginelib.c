// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

#define BUFSIZE (512)

/* Caller must hold descriptor lock */
void print_page_cache_descr(struct rrdeng_page_descr *descr)
{
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;
    char uuid_str[UUID_STR_LEN];
    char str[BUFSIZE + 1];
    int pos = 0;

    uuid_unparse_lower(*descr->id, uuid_str);
    pos += snprintfz(str, BUFSIZE - pos, "page(%p) id=%s\n"
                                    "--->len:%"PRIu32" time:%"PRIu64"->%"PRIu64" xt_offset:",
                    pg_cache_descr->page, uuid_str,
                    descr->page_length,
                    (uint64_t)descr->start_time,
                    (uint64_t)descr->end_time);
    if (!descr->extent) {
        pos += snprintfz(str + pos, BUFSIZE - pos, "N/A");
    } else {
        pos += snprintfz(str + pos, BUFSIZE - pos, "%"PRIu64, descr->extent->offset);
    }

    snprintfz(str + pos, BUFSIZE - pos, " flags:0x%2.2lX refcnt:%u\n\n", pg_cache_descr->flags, pg_cache_descr->refcnt);
    debug(D_RRDENGINE, "%s", str);
}

void print_page_descr(struct rrdeng_page_descr *descr)
{
    char uuid_str[UUID_STR_LEN];
    char str[BUFSIZE + 1];
    int pos = 0;

    uuid_unparse_lower(*descr->id, uuid_str);
    pos += snprintfz(str, BUFSIZE - pos, "id=%s\n"
                                     "--->len:%"PRIu32" time:%"PRIu64"->%"PRIu64" xt_offset:",
                     uuid_str,
                     descr->page_length,
                     (uint64_t)descr->start_time,
                     (uint64_t)descr->end_time);
    if (!descr->extent) {
        pos += snprintfz(str + pos, BUFSIZE - pos, "N/A");
    } else {
        pos += snprintfz(str + pos, BUFSIZE - pos, "%"PRIu64, descr->extent->offset);
    }
    snprintfz(str + pos, BUFSIZE - pos, "\n\n");
    fputs(str, stderr);
}

int check_file_properties(uv_file file, uint64_t *file_size, size_t min_size)
{
    int ret;
    uv_fs_t req;
    uv_stat_t* s;

    ret = uv_fs_fstat(NULL, &req, file, NULL);
    if (ret < 0) {
        fatal("uv_fs_fstat: %s\n", uv_strerror(ret));
    }
    fatal_assert(req.result == 0);
    s = req.ptr;
    if (!(s->st_mode & S_IFREG)) {
        error("Not a regular file.\n");
        uv_fs_req_cleanup(&req);
        return UV_EINVAL;
    }
    if (s->st_size < min_size) {
        error("File length is too short.\n");
        uv_fs_req_cleanup(&req);
        return UV_EINVAL;
    }
    *file_size = s->st_size;
    uv_fs_req_cleanup(&req);

    return 0;
}

/**
 * Open file for I/O.
 *
 * @param path The full path of the file.
 * @param flags Same flags as the open() system call uses.
 * @param file On success sets (*file) to be the uv_file that was opened.
 * @param direct Tries to open a file in direct I/O mode when direct=1, falls back to buffered mode if not possible.
 * @return Returns UV error number that is < 0 on failure. 0 on success.
 */
int open_file_for_io(char *path, int flags, uv_file *file, int direct)
{
    uv_fs_t req;
    int fd = -1, current_flags;

    fatal_assert(0 == direct || 1 == direct);
    for ( ; direct >= 0 ; --direct) {
#ifdef __APPLE__
        /* Apple OS does not support O_DIRECT */
        direct = 0;
#endif
        current_flags = flags;
        if (direct) {
            current_flags |= O_DIRECT;
        }
        fd = uv_fs_open(NULL, &req, path, current_flags, S_IRUSR | S_IWUSR, NULL);
        if (fd < 0) {
            if ((direct) && (UV_EINVAL == fd)) {
                error("File \"%s\" does not support direct I/O, falling back to buffered I/O.", path);
            } else {
                error("Failed to open file \"%s\".", path);
                --direct; /* break the loop */
            }
        } else {
            fatal_assert(req.result >= 0);
            *file = req.result;
#ifdef __APPLE__
            info("Disabling OS X caching for file \"%s\".", path);
            fcntl(fd, F_NOCACHE, 1);
#endif
            --direct; /* break the loop */
        }
        uv_fs_req_cleanup(&req);
    }

    return fd;
}

char *get_rrdeng_statistics(struct rrdengine_instance *ctx, char *str, size_t size)
{
    struct page_cache *pg_cache;

    pg_cache = &ctx->pg_cache;
    snprintfz(str, size,
              "metric_API_producers: %ld\n"
              "metric_API_consumers: %ld\n"
              "page_cache_total_pages: %ld\n"
              "page_cache_descriptors: %ld\n"
              "page_cache_populated_pages: %ld\n"
              "page_cache_committed_pages: %ld\n"
              "page_cache_insertions: %ld\n"
              "page_cache_deletions: %ld\n"
              "page_cache_hits: %ld\n"
              "page_cache_misses: %ld\n"
              "page_cache_backfills: %ld\n"
              "page_cache_evictions: %ld\n"
              "compress_before_bytes: %ld\n"
              "compress_after_bytes: %ld\n"
              "decompress_before_bytes: %ld\n"
              "decompress_after_bytes: %ld\n"
              "io_write_bytes: %ld\n"
              "io_write_requests: %ld\n"
              "io_read_bytes: %ld\n"
              "io_read_requests: %ld\n"
              "io_write_extent_bytes: %ld\n"
              "io_write_extents: %ld\n"
              "io_read_extent_bytes: %ld\n"
              "io_read_extents: %ld\n"
              "datafile_creations: %ld\n"
              "datafile_deletions: %ld\n"
              "journalfile_creations: %ld\n"
              "journalfile_deletions: %ld\n"
              "io_errors: %ld\n"
              "fs_errors: %ld\n"
              "global_io_errors: %ld\n"
              "global_fs_errors: %ld\n"
              "rrdeng_reserved_file_descriptors: %ld\n"
              "pg_cache_over_half_dirty_events: %ld\n"
              "global_pg_cache_over_half_dirty_events: %ld\n"
              "flushing_pressure_page_deletions: %ld\n"
              "global_flushing_pressure_page_deletions: %ld\n",
              (long)ctx->stats.metric_API_producers,
              (long)ctx->stats.metric_API_consumers,
              (long)pg_cache->page_descriptors,
              (long)ctx->stats.page_cache_descriptors,
              (long)pg_cache->populated_pages,
              (long)pg_cache->committed_page_index.nr_committed_pages,
              (long)ctx->stats.pg_cache_insertions,
              (long)ctx->stats.pg_cache_deletions,
              (long)ctx->stats.pg_cache_hits,
              (long)ctx->stats.pg_cache_misses,
              (long)ctx->stats.pg_cache_backfills,
              (long)ctx->stats.pg_cache_evictions,
              (long)ctx->stats.before_compress_bytes,
              (long)ctx->stats.after_compress_bytes,
              (long)ctx->stats.before_decompress_bytes,
              (long)ctx->stats.after_decompress_bytes,
              (long)ctx->stats.io_write_bytes,
              (long)ctx->stats.io_write_requests,
              (long)ctx->stats.io_read_bytes,
              (long)ctx->stats.io_read_requests,
              (long)ctx->stats.io_write_extent_bytes,
              (long)ctx->stats.io_write_extents,
              (long)ctx->stats.io_read_extent_bytes,
              (long)ctx->stats.io_read_extents,
              (long)ctx->stats.datafile_creations,
              (long)ctx->stats.datafile_deletions,
              (long)ctx->stats.journalfile_creations,
              (long)ctx->stats.journalfile_deletions,
              (long)ctx->stats.io_errors,
              (long)ctx->stats.fs_errors,
              (long)global_io_errors,
              (long)global_fs_errors,
              (long)rrdeng_reserved_file_descriptors,
              (long)ctx->stats.pg_cache_over_half_dirty_events,
              (long)global_pg_cache_over_half_dirty_events,
              (long)ctx->stats.flushing_pressure_page_deletions,
              (long)global_flushing_pressure_page_deletions
    );
    return str;
}

int is_legacy_child(const char *machine_guid)
{
    uuid_t uuid;
    char  dbengine_file[FILENAME_MAX+1];

    if (unlikely(!strcmp(machine_guid, "unittest-dbengine") || !strcmp(machine_guid, "dbengine-dataset") ||
                 !strcmp(machine_guid, "dbengine-stress-test"))) {
        return 1;
    }
    if (!uuid_parse(machine_guid, uuid)) {
        uv_fs_t stat_req;
        snprintfz(dbengine_file, FILENAME_MAX, "%s/%s/dbengine", netdata_configured_cache_dir, machine_guid);
        int rc = uv_fs_stat(NULL, &stat_req, dbengine_file, NULL);
        if (likely(rc == 0 && ((stat_req.statbuf.st_mode & S_IFMT) == S_IFDIR))) {
            //info("Found legacy engine folder \"%s\"", dbengine_file);
            return 1;
        }
    }
    return 0;
}

int count_legacy_children(char *dbfiles_path)
{
    int ret;
    uv_fs_t req;
    uv_dirent_t dent;
    int legacy_engines = 0;

    ret = uv_fs_scandir(NULL, &req, dbfiles_path, 0, NULL);
    if (ret < 0) {
        uv_fs_req_cleanup(&req);
        error("uv_fs_scandir(%s): %s", dbfiles_path, uv_strerror(ret));
        return ret;
    }

    while(UV_EOF != uv_fs_scandir_next(&req, &dent)) {
        if (dent.type == UV_DIRENT_DIR) {
            if (is_legacy_child(dent.name))
                legacy_engines++;
        }
    }
    uv_fs_req_cleanup(&req);
    return legacy_engines;
}

int compute_multidb_diskspace()
{
    char multidb_disk_space_file[FILENAME_MAX + 1];
    FILE *fp;
    int computed_multidb_disk_quota_mb = -1;

    snprintfz(multidb_disk_space_file, FILENAME_MAX, "%s/dbengine_multihost_size", netdata_configured_varlib_dir);
    fp = fopen(multidb_disk_space_file, "r");
    if (likely(fp)) {
        int rc = fscanf(fp, "%d", &computed_multidb_disk_quota_mb);
        fclose(fp);
        if (unlikely(rc != 1 || computed_multidb_disk_quota_mb < RRDENG_MIN_DISK_SPACE_MB)) {
            errno = 0;
            error("File '%s' contains invalid input, it will be rebuild", multidb_disk_space_file);
            computed_multidb_disk_quota_mb = -1;
        }
    }

    if (computed_multidb_disk_quota_mb == -1) {
        int rc = count_legacy_children(netdata_configured_cache_dir);
        if (likely(rc >= 0)) {
            computed_multidb_disk_quota_mb = (rc + 1) * default_rrdeng_disk_quota_mb;
            info("Found %d legacy dbengines, setting multidb diskspace to %dMB", rc, computed_multidb_disk_quota_mb);

            fp = fopen(multidb_disk_space_file, "w");
            if (likely(fp)) {
                fprintf(fp, "%d", computed_multidb_disk_quota_mb);
                info("Created file '%s' to store the computed value", multidb_disk_space_file);
                fclose(fp);
            } else
                error("Failed to store the default multidb disk quota size on '%s'", multidb_disk_space_file);
        }
        else
            computed_multidb_disk_quota_mb = default_rrdeng_disk_quota_mb;
    }

    return computed_multidb_disk_quota_mb;
}
