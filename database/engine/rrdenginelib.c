// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

void print_page_cache_descr(struct rrdeng_page_cache_descr *page_cache_descr)
{
    char uuid_str[37];
    char str[512];
    int pos = 0;

    uuid_unparse_lower(*page_cache_descr->id, uuid_str);
    pos += snprintf(str, 512 - pos, "page(%p) id=%s\n"
                                    "--->len:%"PRIu32" time:%"PRIu64"->%"PRIu64" xt_offset:",
                    page_cache_descr->page, uuid_str,
                    page_cache_descr->page_length,
                    (uint64_t)page_cache_descr->start_time,
                    (uint64_t)page_cache_descr->end_time);
    if (!page_cache_descr->extent) {
        pos += snprintf(str + pos, 512 - pos, "N/A");
    } else {
        pos += snprintf(str + pos, 512 - pos, "%"PRIu64, page_cache_descr->extent->offset);
    }
    snprintf(str + pos, 512 - pos, " flags:0x%2.2lX refcnt:%u\n\n", page_cache_descr->flags, page_cache_descr->refcnt);
    fputs(str, stderr);
}

int check_file_properties(uv_file file, uint64_t *file_size, size_t min_size)
{
    int ret;
    uv_fs_t req;
    uv_stat_t* s;

    ret = uv_fs_fstat(NULL, &req, file, NULL);
    if (ret < 0) {
        fprintf(stderr, "uv_fs_fstat: %s\n", uv_strerror(ret));
        exit(ret);
    }
    assert(req.result == 0);
    s = req.ptr;
    if (!(s->st_mode & S_IFREG)) {
        fprintf(stderr, "Not a regular file.\n");
        uv_fs_req_cleanup(&req);
        return UV_EINVAL;
    }
    if (s->st_size < min_size) {
        fprintf(stderr, "File length is too short.\n");
        uv_fs_req_cleanup(&req);
        return UV_EINVAL;
    }
    *file_size = s->st_size;
    uv_fs_req_cleanup(&req);

    return 0;
}
