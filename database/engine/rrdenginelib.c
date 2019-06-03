// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

#define BUFSIZE (512)

/* Caller must hold descriptor lock */
void print_page_cache_descr(struct rrdeng_page_descr *descr)
{
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;
    char uuid_str[UUID_STR_LEN];
    char str[BUFSIZE];
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
    char str[BUFSIZE];
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
    assert(req.result == 0);
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
              "page_cache_commited_pages: %ld\n"
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
              "journalfile_deletions: %ld\n",
              (long)ctx->stats.metric_API_producers,
              (long)ctx->stats.metric_API_consumers,
              (long)pg_cache->page_descriptors,
              (long)ctx->stats.page_cache_descriptors,
              (long)pg_cache->populated_pages,
              (long)pg_cache->commited_page_index.nr_commited_pages,
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
              (long)ctx->stats.journalfile_deletions
    );
    return str;
}
