// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

#define TESTFILE "testfile.txt"

void rrdeng_enq_cmd(struct rrdengine_worker_config* wc, struct rrdeng_cmd *cmd);

static void sanity_check(void)
{
    /* Magic numbers must fit in the super-blocks */
    BUILD_BUG_ON(strlen(RRDENG_DF_MAGIC) > RRDENG_MAGIC_SZ);
    BUILD_BUG_ON(strlen(RRDENG_JF_MAGIC) > RRDENG_MAGIC_SZ);

    /* Version strings must fit in the super-blocks */
    BUILD_BUG_ON(strlen(RRDENG_DF_VER) > RRDENG_VER_SZ);
    BUILD_BUG_ON(strlen(RRDENG_JF_VER) > RRDENG_VER_SZ);

    /* Data file super-block cannot be larger than RRDENG_BLOCK_SIZE */
    BUILD_BUG_ON(RRDENG_DF_SB_PADDING_SZ < 0);

    BUILD_BUG_ON(sizeof(uuid_t) != UUID_SZ); /* check UUID size */

    /* page count must fit in 8 bits */
    BUILD_BUG_ON(MAX_PAGES_PER_EXTENT > 255);
}

/*
 * Global state of RRD Engine
 */
static volatile enum {
    RRDENGINE_STATUS_UNINITIALIZED = 0,
    RRDENGINE_STATUS_INITIALIZING,
    RRDENGINE_STATUS_INITIALIZED
} rrdengine_state;
static struct rrdengine_worker_config worker_config;
struct completion rrdengine_completion;
struct page_cache pg_cache;
struct rrdengine_datafile datafile;
uint8_t compression_algorithm = RRD_NO_COMPRESSION;
//uint8_t compression_algorithm = RRD_LZ4;

static void print_page_cache_descr(struct rrdeng_page_cache_descr *page_cache_descr)
{
    char uuid_str[37];
    char str[512];
    int pos = 0;

    uuid_unparse_lower(*page_cache_descr->metric, uuid_str);
    pos += snprintf(str, 512 - pos, "page(%p) metric=%s\n"
           "--->len:%"PRIu32" time:%"PRIu64"->%"PRIu64" xt_offset:",
           page_cache_descr->page, uuid_str,
           page_cache_descr->page_length,
           (uint64_t)page_cache_descr->start_time,
           (uint64_t)page_cache_descr->end_time);
    if (page_cache_descr->extent.offset == INVALID_EXTENT_OFFSET) {
        pos += snprintf(str + pos, 512 - pos, "N/A");
    } else {
        pos += snprintf(str + pos, 512 - pos, "%"PRIu64, page_cache_descr->extent.offset);
    }
    snprintf(str + pos, 512 - pos, " flags:0x%2.2lX refcnt:%u\n\n", page_cache_descr->flags, page_cache_descr->refcnt);
    fputs(str, stderr);
}

/* Returns page flags */
unsigned long pg_cache_wait_event(struct rrdeng_page_cache_descr *descr)
{
    unsigned long flags;

    uv_mutex_lock(&descr->mutex);
    uv_cond_wait(&descr->cond, &descr->mutex);
    flags = descr->flags;
    uv_mutex_unlock(&descr->mutex);

    return flags;
}

/*
 * The caller must hold the page cache and the page descriptor locks
 * TODO: implement
 * TODO: last waiter frees descriptor
 */
static void pg_cache_punch_hole_unsafe(struct rrdeng_page_cache_descr *descr)
{
    return;
}

/*
 * The caller must hold page descriptor lock.
 * Gets a reference to the page descriptor.
 * Returns 1 on success and 0 on failure.
 */
int pg_cache_try_get_unsafe(struct rrdeng_page_cache_descr *descr, int exclusive_access)
{
    if ((descr->flags & (RRD_PAGE_LOCKED | RRD_PAGE_READ_PENDING)) ||
        (exclusive_access && descr->refcnt)) {
        return 0;
    }
    if (exclusive_access)
        descr->flags |= RRD_PAGE_LOCKED;
    ++descr->refcnt;

    return 1;
}

/*
 * The caller must hold the page descriptor lock.
 * This function may block doing cleanup.
 */
void pg_cache_put_unsafe(struct rrdeng_page_cache_descr *descr)
{
    --descr->refcnt;
    descr->flags &= ~RRD_PAGE_LOCKED;
    /* TODO: perform cleanup */
}

/*
 * This function may block doing cleanup.
 */
void pg_cache_put(struct rrdeng_page_cache_descr *descr)
{
    uv_mutex_lock(&descr->mutex);
    pg_cache_put_unsafe(descr);
    uv_mutex_unlock(&descr->mutex);
}

/* The caller must hold the page cache and the page descriptor locks */
static void pg_cache_evict_unsafe(struct rrdeng_page_cache_descr *descr)
{
    free(descr->page);
    descr->page = NULL;
    descr->flags &= ~RRD_PAGE_POPULATED;
    --pg_cache.populated_pages;
}

/*
 * The caller must hold the page cache lock.
 * This function iterates all pages and tries to evict one.
 * If it fails it sets dirty_pages to the number of dirty pages iterated.
 * If it fails it sets in_flight_descr to the last descriptor that has write-back in progress,
 * or it sets it to NULL if no write-back is in progress.
 *
 * Returns 1 on success and 0 on failure.
 */
static int pg_cache_try_evict_one_page_unsafe(struct rrdeng_page_cache_descr **in_flight_descr)
{
    int i;
    unsigned long old_flags;
    struct rrdeng_page_cache_descr *tmp, *failed_descr;

    failed_descr = NULL;

    for (i = 0; i < PAGE_CACHE_MAX_SIZE; ++i) {
        tmp = pg_cache.page_cache_array[i];
        if (NULL == tmp)
            continue;
        uv_mutex_lock(&tmp->mutex);
        old_flags = tmp->flags;
        if (old_flags & RRD_PAGE_POPULATED) {
            int locked_page = 0;
            /* must evict */
            if (pg_cache_try_get_unsafe(tmp, 1)) {
                locked_page = 1;
            }
            if (locked_page && !(old_flags & RRD_PAGE_DIRTY)) {
                pg_cache_evict_unsafe(tmp);
                pg_cache_put_unsafe(tmp);
                uv_mutex_unlock(&tmp->mutex);
                break;
            }
            if (old_flags & RRD_PAGE_WRITE_PENDING) {
                failed_descr = tmp;
            }
            if (locked_page)
                pg_cache_put_unsafe(tmp);
        }
        uv_mutex_unlock(&tmp->mutex);
    }
    if (i == PAGE_CACHE_MAX_SIZE) {
        /* failed to evict */
        *in_flight_descr = failed_descr;
        return 0;
    }
    return 1;
}

void pg_cache_insert(struct rrdeng_page_cache_descr *descr)
{
    int i;
    struct rrdeng_page_cache_descr *tmp;
    unsigned long old_flags;
    unsigned dirty_pages;
    struct rrdeng_page_cache_descr *in_flight_descr;

    uv_rwlock_wrlock(&pg_cache.pg_cache_rwlock);
    while (pg_cache.populated_pages == PAGE_CACHE_MAX_PAGES) {
        fprintf(stderr, "=================================\nPage cache full. Trying to evict.\n=================================\n");
        if (!pg_cache_try_evict_one_page_unsafe(&in_flight_descr)) {
            /* failed to evict */
            if (in_flight_descr) {
                uv_mutex_lock(&in_flight_descr->mutex);
                uv_rwlock_wrunlock(&pg_cache.pg_cache_rwlock);
                fprintf(stderr, "%s: waiting for page to be written to disk before evicting:\n", __func__);
                print_page_cache_descr(in_flight_descr);
                uv_cond_wait(&in_flight_descr->cond, &in_flight_descr->mutex);
                uv_mutex_unlock(&in_flight_descr->mutex);
            } else {
                struct completion compl;
                struct rrdeng_cmd cmd;

                uv_rwlock_wrunlock(&pg_cache.pg_cache_rwlock);

                init_completion(&compl);
                cmd.opcode = RRDENG_FLUSH_PAGES;
                cmd.completion = &compl;
                rrdeng_enq_cmd(&worker_config, &cmd);
                /* wait for some pages to be flushed */
                fprintf(stderr, "%s: waiting for pages to be written to disk before evicting.\n", __func__);
                wait_for_completion(&compl);
                destroy_completion(&compl);
            }
            uv_rwlock_wrlock(&pg_cache.pg_cache_rwlock);
        }
    }
    for (i = 0; i < PAGE_CACHE_MAX_SIZE ; ++i) {
        tmp = pg_cache.page_cache_array[i];
        if (NULL == tmp) {
            pg_cache.page_cache_array[i] = descr;
            if (descr->flags & RRD_PAGE_POPULATED) {
                ++pg_cache.populated_pages;
            }
            break;
        }
    }
    uv_rwlock_wrunlock(&pg_cache.pg_cache_rwlock);
    if (i == PAGE_CACHE_MAX_SIZE) {
        fprintf(stderr, "CRITICAL: Out of page cache descriptors. Cannot insert:\n");
        print_page_cache_descr(descr);
    }
}

/*
 * Searches for a page and gets a reference.
 * When point_in_time is INVALID_TIME get any page.
 */
struct rrdeng_page_cache_descr *pg_cache_lookup(uuid_t *metric, usec_t point_in_time)
{
    struct rrdeng_page_cache_descr *descr = NULL;
    int i, page_cache_locked;
    unsigned long flags;

    page_cache_locked = 1;
    uv_rwlock_rdlock(&pg_cache.pg_cache_rwlock);
    for (i = 0 ; i < PAGE_CACHE_MAX_SIZE ; ++i) {
        if ((NULL == (descr = pg_cache.page_cache_array[i])) || uuid_compare(*metric, *descr->metric)) {
            continue;
        }
        if (INVALID_TIME == point_in_time) {
            int got_ref = 0;

            uv_mutex_lock(&descr->mutex);
            flags = descr->flags;
            if (pg_cache_try_get_unsafe(descr, 0)) {
                if (flags & RRD_PAGE_POPULATED) {
                    /* success */
                    uv_mutex_unlock(&descr->mutex);
                    fprintf(stderr, "%s: Page was found in memory.\n", __func__);
                    break;
                }
                got_ref = 1;
            }
            if (got_ref)
                pg_cache_put_unsafe(descr);
            if (!(flags & RRD_PAGE_POPULATED) && pg_cache_try_get_unsafe(descr, 1)) {
                struct rrdeng_cmd cmd;

                if (flags & RRD_PAGE_POPULATED) {
                    /* success */
                    /* Downgrade exclusive reference to allow other readers */
                    descr->flags &= ~RRD_PAGE_LOCKED;
                    uv_mutex_unlock(&descr->mutex);
                    break;
                }
                cmd.opcode = RRDENG_READ_PAGE;
                cmd.page_cache_descr = descr;
                rrdeng_enq_cmd(&worker_config, &cmd);

                uv_rwlock_rdunlock(&pg_cache.pg_cache_rwlock);
                page_cache_locked = 0;
                fprintf(stderr, "%s: Waiting for page to be asynchronously read from disk:\n", __func__);
                print_page_cache_descr(descr);
                while (!(descr->flags & RRD_PAGE_POPULATED)) {
                    uv_cond_wait(&descr->cond, &descr->mutex);
                }
                /* success */
                /* Downgrade exclusive reference to allow other readers */
                descr->flags &= ~RRD_PAGE_LOCKED;
                uv_mutex_unlock(&descr->mutex);
                break;
            }
            uv_rwlock_rdunlock(&pg_cache.pg_cache_rwlock);
            fprintf(stderr, "%s: Waiting for page to be unlocked:", __func__);
            print_page_cache_descr(descr);
            uv_cond_wait(&descr->cond, &descr->mutex);
            uv_mutex_unlock(&descr->mutex);

            /* reset scan to find again */
            i = -1;
            uv_rwlock_rdlock(&pg_cache.pg_cache_rwlock);
        } else {
            fprintf(stderr, "NOT IMPLEMENTED YET.\n"); /* TODO: Implement */
        }
    }
    if (page_cache_locked)
        uv_rwlock_rdunlock(&pg_cache.pg_cache_rwlock);

    if (i == PAGE_CACHE_MAX_SIZE) {
        /* no such page */
        return NULL;
    }

    return descr;
}

static void init_page_cache(void)
{
    int i;

    for (i = 0 ; i < PAGE_CACHE_MAX_SIZE ; ++i) {
        pg_cache.page_cache_array[i] = NULL;
    }
    pg_cache.populated_pages = 0;
    assert(0 == uv_rwlock_init(&pg_cache.pg_cache_rwlock));

    for (i = 0 ; i < PAGE_CACHE_MAX_COMMITED_PAGES ; ++i) {
        pg_cache.commited_pages[i] = NULL;
    }
    pg_cache.nr_commited_pages = 0;
    assert(0 == uv_rwlock_init(&pg_cache.commited_pages_rwlock));
}

void read_page_cb(uv_fs_t* req)
{
    struct extent_io_descriptor *xt_io_descr;
    struct rrdeng_page_cache_descr *descr;
    int i, ret;
    unsigned count, pos;
    void *page, *payload, *uncompressed_buf;
    uint32_t payload_length, payload_offset, offset, page_offset, uncompressed_payload_length;
    /* persistent structures */
    struct rrdeng_df_extent_header *header;
    struct rrdeng_df_extent_trailer *trailer;
    uLong crc;

    if (req->result < 0) {
        fprintf(stderr, "%s: uv_fs_read: %s\n", __func__, uv_strerror((int)req->result));
        goto cleanup;
    }
    xt_io_descr = req->data;
    descr = xt_io_descr->descr_array[0];

    header = xt_io_descr->buf;
    payload_length = header->payload_length;
    count = header->number_of_pages;

    payload_offset = sizeof(*header) + sizeof(header->descr[0]) * count;

    for (i = 0, page_offset = 0; i < count ; ++i) {
        /* care, we don't hold the descriptor mutex */
        if (!uuid_compare(*(uuid_t *)header->descr[i].uuid, *descr->metric) &&
            header->descr[i].page_length == descr->page_length &&
            header->descr[i].start_time == descr->start_time &&
            header->descr[i].end_time == descr->end_time) {
            break;
        }
        page_offset += header->descr[i].page_length;
    }

    trailer = xt_io_descr->buf + xt_io_descr->bytes - sizeof(*trailer);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, xt_io_descr->buf, xt_io_descr->bytes - sizeof(*trailer));
    ret = memcmp(trailer->checksum, &crc, sizeof(crc));
    fprintf(stderr, "%s: Extent was read from disk. CRC32 check: %s\n", __func__, ret ? "FAILED" : "SUCCEEDED");
    if (unlikely(ret)) {
        /* TODO: handle errors */
        goto cleanup;
    }

    ret = posix_memalign(&page, RRDFILE_ALIGNMENT, RRDENG_BLOCK_SIZE);
    if (unlikely(ret)) {
        fprintf(stderr, "posix_memalign:%s\n", strerror(ret));
        goto cleanup;
    }
    if (RRD_NO_COMPRESSION == compression_algorithm) {
        /* care, we don't hold the descriptor mutex */
        (void) memcpy(page, xt_io_descr->buf + payload_offset + page_offset, descr->page_length);
    } else {
        uncompressed_payload_length = 0;
        for (i = 0 ; i < count ; ++i) {
            uncompressed_payload_length += header->descr[i].page_length;
        }
        uncompressed_buf = malloc(uncompressed_payload_length);
        if (!uncompressed_buf) {
            fprintf(stderr, "malloc failed.\n");
            exit(1);
        }
        ret = LZ4_decompress_safe(xt_io_descr->buf + payload_offset, uncompressed_buf,
                                  payload_length, uncompressed_payload_length);
        fprintf(stderr, "LZ4 decompressed %d bytes to %d bytes.\n", payload_length, ret);
        /* care, we don't hold the descriptor mutex */
        (void) memcpy(page, uncompressed_buf + page_offset, descr->page_length);
        free(uncompressed_buf);
    }
    uv_rwlock_wrlock(&pg_cache.pg_cache_rwlock);
    ++pg_cache.populated_pages;
    uv_mutex_lock(&descr->mutex);
    descr->page = page;
    descr->flags |= RRD_PAGE_POPULATED;
    descr->flags &= ~RRD_PAGE_READ_PENDING;
    uv_mutex_unlock(&descr->mutex);
    uv_rwlock_wrunlock(&pg_cache.pg_cache_rwlock);

    fprintf(stderr, "%s: Waking up waiters.\n", __func__);
    /* wake up waiters */
    uv_cond_broadcast(&descr->cond);

    if (xt_io_descr->completion)
        complete(xt_io_descr->completion);
cleanup:
    free(xt_io_descr->buf);
    free(xt_io_descr);
    uv_fs_req_cleanup(req);
}


static void do_read_page(struct rrdengine_worker_config* wc, struct rrdeng_page_cache_descr *descr)
{
    int i, ret;
    unsigned count, size_bytes, pos;
    uint32_t payload_length;
    struct rrdeng_page_cache_descr *eligible_pages[MAX_PAGES_PER_EXTENT];
    struct extent_io_descriptor *xt_io_descr;

    uv_mutex_lock(&descr->mutex);
    descr->flags |= RRD_PAGE_READ_PENDING;
    payload_length = descr->page_length;
    pos = descr->extent.offset;
    size_bytes = descr->extent.size;
    uv_mutex_unlock(&descr->mutex);

    xt_io_descr = malloc(sizeof(*xt_io_descr));
    if (unlikely(NULL == xt_io_descr)) {
        fprintf(stderr, "%s: malloc failed.\n", __func__);
        return;
    }
    ret = posix_memalign((void *)&xt_io_descr->buf, RRDFILE_ALIGNMENT, ALIGN_BYTES_CEILING(size_bytes));
    if (unlikely(ret)) {
        fprintf(stderr, "posix_memalign:%s\n", strerror(ret));
        free(xt_io_descr);
        return;
    }
    xt_io_descr->descr_array[0] = descr;
    xt_io_descr->descr_count = 1;
    xt_io_descr->bytes = size_bytes;
    xt_io_descr->pos = pos;
    xt_io_descr->req.data = xt_io_descr;
    xt_io_descr->completion = NULL;
    /* xt_io_descr->descr_commit_idx_array[0] */

    xt_io_descr->iov = uv_buf_init((void *)xt_io_descr->buf, ALIGN_BYTES_CEILING(size_bytes));
    ret = uv_fs_read(wc->loop, &xt_io_descr->req, datafile.file, &xt_io_descr->iov, 1, pos, read_page_cb);
    assert (-1 != ret);

    return;
}

static void do_commit_page(struct rrdengine_worker_config* wc, struct rrdeng_page_cache_descr *descr)
{
    return;
}

void flush_pages_cb(uv_fs_t* req)
{
    struct extent_io_descriptor *xt_io_descr;
    struct rrdeng_page_cache_descr *descr;
    int i;
    unsigned count, commit_idx;

    fprintf(stderr, "%s: Extent was written to disk.\n", __func__);
    if (req->result < 0) {
        fprintf(stderr, "%s: uv_fs_write: %s\n", __func__, uv_strerror((int)req->result));
        goto cleanup;
    }
    xt_io_descr = req->data;

    count = xt_io_descr->descr_count;
    for (i = 0 ; i < count ; ++i) {
        /* care, we don't hold the descriptor mutex */
        descr = xt_io_descr->descr_array[i];

        uv_rwlock_wrlock(&pg_cache.commited_pages_rwlock);

        uv_mutex_lock(&descr->mutex);
        descr->flags &= ~(RRD_PAGE_DIRTY | RRD_PAGE_WRITE_PENDING);
        descr->extent.offset = xt_io_descr->pos;
        descr->extent.size = xt_io_descr->bytes;
        uv_mutex_unlock(&descr->mutex);

        commit_idx = xt_io_descr->descr_commit_idx_array[i];
        pg_cache.commited_pages[commit_idx] = NULL;
        --pg_cache.nr_commited_pages;
        uv_rwlock_wrunlock(&pg_cache.commited_pages_rwlock);

        fprintf(stderr, "%s: Waking up waiters of page %i.\n", __func__, i);
        /* wake up waiters */
        uv_cond_broadcast(&descr->cond);
    }
    if (xt_io_descr->completion)
        complete(xt_io_descr->completion);
cleanup:
    free(xt_io_descr->buf);
    free(xt_io_descr);
    uv_fs_req_cleanup(req);
}

/*
 * completion must be NULL or valid.
 * Returns 0 when no flushing can take place.
 * Returns 1 on successful flushing initiation.
 */
static int do_flush_pages(struct rrdengine_worker_config* wc, int force, struct completion *completion)
{
    int i, ret, compressed_size, max_compressed_size;
    unsigned count, size_bytes, pos;
    uint32_t uncompressed_payload_length, payload_offset;
    struct rrdeng_page_cache_descr *descr, *eligible_pages[MAX_PAGES_PER_EXTENT];
    struct extent_io_descriptor *xt_io_descr;
    void *compressed_buf;
    /* persistent structures */
    struct rrdeng_df_extent_header *header;
    struct rrdeng_df_extent_trailer *trailer;
    unsigned descr_commit_idx_array[MAX_PAGES_PER_EXTENT];
    uLong crc;

    if (force)
        fprintf(stderr, "Asynchronous flushing of extent has been forced by page pressure.\n");

    uv_rwlock_rdlock(&pg_cache.commited_pages_rwlock);
    for (i = 0, count = 0, uncompressed_payload_length = 0 ;
         i < PAGE_CACHE_MAX_COMMITED_PAGES && count != MAX_PAGES_PER_EXTENT ;
         ++i) {
        if (NULL == (descr = pg_cache.commited_pages[i]))
            continue;
        uv_mutex_lock(&descr->mutex);
        if (!(descr->flags & RRD_PAGE_WRITE_PENDING)) {
            descr->flags |= RRD_PAGE_WRITE_PENDING;
            uncompressed_payload_length += descr->page_length;
            descr_commit_idx_array[count] = i;
            eligible_pages[count++] = descr;
        }
        uv_mutex_unlock(&descr->mutex);
    }
    uv_rwlock_rdunlock(&pg_cache.commited_pages_rwlock);

    if (!count) {
        fprintf(stderr, "%s: no pages eligible for flushing.\n", __func__);
        return 0;
    }
    xt_io_descr = malloc(sizeof(*xt_io_descr));
    if (unlikely(NULL == xt_io_descr)) {
        fprintf(stderr, "%s: malloc failed.\n", __func__);
        return 0;
    }
    payload_offset = sizeof(*header) + count * sizeof(header->descr[0]);
    if (RRD_NO_COMPRESSION != compression_algorithm) {
        assert(uncompressed_payload_length < LZ4_MAX_INPUT_SIZE);
        max_compressed_size = LZ4_compressBound(uncompressed_payload_length);
        compressed_buf = malloc(max_compressed_size);
        if (!compressed_buf) {
            printf("malloc failed.\n");
            exit(1);
        }
        size_bytes = payload_offset + MAX(uncompressed_payload_length, max_compressed_size) + sizeof(*trailer);
    } else {
        size_bytes = payload_offset + uncompressed_payload_length + sizeof(*trailer);
    }
    ret = posix_memalign((void *)&xt_io_descr->buf, RRDFILE_ALIGNMENT, ALIGN_BYTES_CEILING(size_bytes));
    if (unlikely(ret)) {
        fprintf(stderr, "posix_memalign:%s\n", strerror(ret));
        free(xt_io_descr);
        return 0;
    }
    (void) memcpy(xt_io_descr->descr_array, eligible_pages, sizeof(struct rrdeng_page_cache_descr *) * count);
    xt_io_descr->descr_count = count;

    pos = 0;
    header = xt_io_descr->buf;
    header->compression_algorithm = compression_algorithm;
    header->number_of_pages = count;
    pos += sizeof(*header);

    for (i = 0 ; i < count ; ++i) {
        /* This is here for performance reasons */
        xt_io_descr->descr_commit_idx_array[i] = descr_commit_idx_array[i];

        descr = xt_io_descr->descr_array[i];
        uuid_copy(*(uuid_t *)header->descr[i].uuid, *descr->metric);
        header->descr[i].page_length = descr->page_length;
        header->descr[i].start_time = descr->start_time;
        header->descr[i].end_time = descr->end_time;
        pos += sizeof(header->descr[i]);
    }
    for (i = 0 ; i < count ; ++i) {
        descr = xt_io_descr->descr_array[i];
        /* care, we don't hold the descriptor mutex */
        (void) memcpy(xt_io_descr->buf + pos, descr->page, descr->page_length);
        pos += descr->page_length;
    }
    if (RRD_NO_COMPRESSION != compression_algorithm) {
        compressed_size = LZ4_compress_default(xt_io_descr->buf + payload_offset, compressed_buf,
                                               uncompressed_payload_length, max_compressed_size);
        fprintf(stderr, "LZ4 compressed %"PRIu32" bytes to %d bytes.\n", uncompressed_payload_length, compressed_size);
        (void) memcpy(xt_io_descr->buf + payload_offset, compressed_buf, compressed_size);
        free(compressed_buf);
        size_bytes = payload_offset + compressed_size + sizeof(*trailer);
    }
    if (RRD_NO_COMPRESSION == compression_algorithm) {
        header->payload_length = uncompressed_payload_length;
    } else {
        header->payload_length = compressed_size;
    }
    xt_io_descr->bytes = size_bytes;
    xt_io_descr->pos = datafile.pos;
    xt_io_descr->req.data = xt_io_descr;
    xt_io_descr->completion = completion;

    trailer = xt_io_descr->buf + size_bytes - sizeof(*trailer);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, xt_io_descr->buf, size_bytes - sizeof(*trailer));
    memcpy(trailer->checksum, &crc, sizeof(crc));

    xt_io_descr->iov = uv_buf_init((void *)xt_io_descr->buf, ALIGN_BYTES_CEILING(size_bytes));
    ret = uv_fs_write(wc->loop, &xt_io_descr->req, datafile.file, &xt_io_descr->iov, 1, datafile.pos, flush_pages_cb);
    assert (-1 != ret);
    datafile.pos += ALIGN_BYTES_CEILING(size_bytes);

    return 1;
}

static int init_rrd_files(void)
{
    uv_fs_t req;
    uv_file file;
    int i, ret, fd;
    struct rrdeng_df_sb *superblock;
    uv_buf_t iov;

    fd = uv_fs_open(NULL, &req, TESTFILE, UV_FS_O_DIRECT | UV_FS_O_CREAT | UV_FS_O_RDWR | UV_FS_O_TRUNC, S_IRUSR | S_IWUSR, NULL);
    if (fd == -1) {
        perror("uv_fs_open");
        return req.result;
    }
    assert(req.result >= 0);
    file = req.result;
    uv_fs_req_cleanup(&req);

    ret = posix_memalign((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    if (unlikely(ret)) {
        fprintf(stderr, "posix_memalign:%s\n", strerror(ret));
        return UV_ENOMEM;
    }
    (void) strncpy(superblock->magic_number, RRDENG_DF_MAGIC, RRDENG_MAGIC_SZ);
    (void) strncpy(superblock->version, RRDENG_DF_VER, RRDENG_VER_SZ);
    superblock->tier = 1;

    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_write(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret == -1) {
        perror("uv_fs_write");
        return req.result;
    }
    if (req.result < 0) {
        printf("uv_fs_write: %s\n", uv_strerror((int)req.result));
        exit(ret);
    }
    uv_fs_req_cleanup(&req);

    datafile.file = file;
    datafile.fd = fd;
    datafile.pos = sizeof(*superblock);

    return 0;
}

void rrdeng_init_cmd_queue(struct rrdengine_worker_config* wc)
{
    wc->cmd_queue.head = wc->cmd_queue.tail = 0;
    wc->queue_size = 0;
    assert(0 == uv_cond_init(&wc->cmd_cond));
    assert(0 == uv_mutex_init(&wc->cmd_mutex));
}

void rrdeng_enq_cmd(struct rrdengine_worker_config* wc, struct rrdeng_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&wc->cmd_mutex);
    while ((queue_size = wc->queue_size) == RRDENG_CMD_Q_MAX_SIZE) {
        uv_cond_wait(&wc->cmd_cond, &wc->cmd_mutex);
    }
    assert(queue_size < RRDENG_CMD_Q_MAX_SIZE);
    /* enqueue command */
    wc->cmd_queue.cmd_array[wc->cmd_queue.tail] = *cmd;
    wc->cmd_queue.tail = wc->cmd_queue.tail != RRDENG_CMD_Q_MAX_SIZE - 1 ?
                         wc->cmd_queue.tail + 1 : 0;
    wc->queue_size = queue_size + 1;
    uv_mutex_unlock(&wc->cmd_mutex);

    /* wake up event loop */
    assert(0 == uv_async_send(&wc->async));
}

struct rrdeng_cmd rrdeng_deq_cmd(struct rrdengine_worker_config* wc)
{
    struct rrdeng_cmd ret;
    unsigned queue_size;

    uv_mutex_lock(&wc->cmd_mutex);
    queue_size = wc->queue_size;
    if (queue_size == 0) {
        ret.opcode = RRDENG_NOOP;
    } else {
        /* dequeue command */
        ret = wc->cmd_queue.cmd_array[wc->cmd_queue.head];
        if (queue_size == 1) {
            wc->cmd_queue.head = wc->cmd_queue.tail = 0;
        } else {
            wc->cmd_queue.head = wc->cmd_queue.head != RRDENG_CMD_Q_MAX_SIZE - 1 ?
                                 wc->cmd_queue.head + 1 : 0;
        }
        wc->queue_size = queue_size - 1;

        /* wake up producers */
        uv_cond_signal(&wc->cmd_cond);
    }
    uv_mutex_unlock(&wc->cmd_mutex);

    return ret;
}

void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    fprintf(stderr, "%s called, active=%d.\n", __func__, uv_is_active((uv_handle_t *)handle));
}

void timer_cb(uv_timer_t* handle)
{
    struct rrdengine_worker_config* wc = handle->data;

    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    fprintf(stderr, "%s: timeout reached, flushing pages to disk.\n", __func__);
    (void) do_flush_pages(wc, 0, NULL);
}

/* Flushes dirty pages when timer expires */
#define TIMER_PERIOD_MS (10000)

#define CMD_BATCH_SIZE (256)

static void rrdeng_worker(void* arg)
{
    struct rrdengine_worker_config* wc = arg;
    uv_loop_t* loop;
    int shutdown, fd, error;
    enum rrdeng_opcode opcode;
    uv_timer_t timer_req;
    struct rrdeng_cmd cmd;
    unsigned current_cmd_batch_size;

    error = init_rrd_files();
    if (error) /*TODO: error? */
        return;

    rrdeng_init_cmd_queue(wc);

    loop = wc->loop = uv_default_loop();
    loop->data = wc;

    uv_async_init(wc->loop, &wc->async, async_cb);
    wc->async.data = wc;

    /* dirty page flushing timer */
    uv_timer_init(loop, &timer_req);
    timer_req.data = wc;

    /* wake up initialization thread */
    complete(&rrdengine_completion);

    uv_timer_start(&timer_req, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS);
    shutdown = 0;
    while (shutdown == 0 || uv_loop_alive(loop)) {
        uv_run(loop, UV_RUN_DEFAULT);
        current_cmd_batch_size = 0;
        /* wait for commands */
        do {
            cmd = rrdeng_deq_cmd(wc);
            opcode = cmd.opcode;

            switch (opcode) {
            case RRDENG_NOOP:
                /* the command queue was empty, do nothing */
                break;
            case RRDENG_SHUTDOWN:
                shutdown = 1;
                /*
                 * uv_async_send after uv_close does not seem to crash in linux at the moment,
                 * it is however undocumented behaviour and we need to be aware if this becomes
                 * an issue in the future.
                 */
                uv_close((uv_handle_t *)&wc->async, NULL);
                assert(0 == uv_timer_stop(&timer_req));
                uv_close((uv_handle_t *)&timer_req, NULL);
                fprintf(stderr, "Shutting down RRD engine event loop.\n");
                while (do_flush_pages(wc, 1, NULL)) {
                    ; /* Force flushing of all commited pages. */
                }
                break;
            case RRDENG_READ_PAGE:
                do_read_page(wc, cmd.page_cache_descr);
                break;
            case RRDENG_COMMIT_PAGE:
                do_commit_page(wc, cmd.page_cache_descr);
                break;
            case RRDENG_FLUSH_PAGES:
                (void) do_flush_pages(wc, 1, cmd.completion);
                break;
            default:
                fprintf(stderr, "default.\n");
                break;
            }
        } while ((opcode != RRDENG_NOOP) && (current_cmd_batch_size  < CMD_BATCH_SIZE));
    }

    /* TODO: don't let the API block by waiting to enqueue commands */
    uv_cond_destroy(&wc->cmd_cond);
/*  uv_mutex_destroy(&wc->cmd_mutex); */
}

/* Also gets a reference for the page */
void *rrdeng_create_page(uuid_t *metric, void **handle)
{
    struct rrdeng_page_cache_descr *descr;
    void *page;
    int ret;

    /* TODO: check maximum number of pages in page cache limit */

    ret = posix_memalign(&page, RRDFILE_ALIGNMENT, RRDENG_BLOCK_SIZE);
    if (unlikely(ret)) {
        fprintf(stderr, "posix_memalign:%s\n", strerror(ret));
        return NULL;
    }
    descr = malloc(sizeof(*descr));
    if (unlikely(descr == NULL)) {
        fprintf(stderr, "malloc failed.\n");
        free(page);
        *handle = NULL;
        return NULL;
    }
    descr->page = page;
    descr->page_length = 0;
    descr->start_time = now_boottime_usec();
    descr->end_time = INVALID_TIME;
    descr->metric = metric;
    descr->extent.offset = INVALID_EXTENT_OFFSET;
    descr->flags = RRD_PAGE_DIRTY | RRD_PAGE_LOCKED | RRD_PAGE_POPULATED;
    descr->refcnt = 1;
    assert(0 == uv_cond_init(&descr->cond));
    assert(0 == uv_mutex_init(&descr->mutex));

    fprintf(stderr, "-----------------\nCreated new page:\n-----------------\n");
    print_page_cache_descr(descr);
    pg_cache_insert(descr);
    *handle = descr;
    return page;
}

void rrdeng_commit_page(void *handle, uint32_t page_length)
{
    int i;
    struct rrdeng_cmd cmd;
    struct rrdeng_page_cache_descr *descr, *tmp;

    descr = handle;
    uv_mutex_lock(&descr->mutex);
    descr->page_length = page_length;
    descr->end_time = now_boottime_usec();
    uv_mutex_unlock(&descr->mutex);

    uv_rwlock_wrlock(&pg_cache.commited_pages_rwlock);
    while (PAGE_CACHE_MAX_COMMITED_PAGES == pg_cache.nr_commited_pages) {
        int found_pending;

        found_pending = 0;
        for (i = 0; i < PAGE_CACHE_MAX_COMMITED_PAGES; ++i) {
            tmp = pg_cache.commited_pages[i];
            assert(tmp);
            uv_mutex_lock(&tmp->mutex);
            if (tmp->flags & RRD_PAGE_WRITE_PENDING) {
                /* wait for the page to be flushed */
                uv_rwlock_wrunlock(&pg_cache.commited_pages_rwlock);
                found_pending = 1;
                fprintf(stderr, "%s: waiting for in-flight page to be written to disk:\n", __func__);
                print_page_cache_descr(tmp);
                uv_cond_wait(&tmp->cond, &tmp->mutex);
                uv_mutex_unlock(&tmp->mutex);
                break;
            }
            uv_mutex_unlock(&tmp->mutex);
        }
        if (!found_pending) {
            struct completion compl;

            uv_rwlock_wrunlock(&pg_cache.commited_pages_rwlock);
            init_completion(&compl);
            cmd.opcode = RRDENG_FLUSH_PAGES;
            cmd.completion = &compl;
            rrdeng_enq_cmd(&worker_config, &cmd);
            /* wait for some pages to be flushed */
            fprintf(stderr, "%s: forcing asynchronous flush of extent. Waiting for completion.\n", __func__);
            wait_for_completion(&compl);
            destroy_completion(&compl);
        }
        uv_rwlock_wrlock(&pg_cache.commited_pages_rwlock);
    }
    for (i = 0 ; i < PAGE_CACHE_MAX_COMMITED_PAGES ; ++i) {
        if (NULL == pg_cache.commited_pages[i]) {
            pg_cache.commited_pages[i] = descr;
            ++pg_cache.nr_commited_pages;
            break;
        }
    }
    assert (i != PAGE_CACHE_MAX_COMMITED_PAGES);
    uv_rwlock_wrunlock(&pg_cache.commited_pages_rwlock);

    pg_cache_put(descr);
}

/* Gets a reference for the page */
void *rrdeng_get_latest_page(uuid_t *metric, void **handle)
{
    struct rrdeng_page_cache_descr *descr;
    void *page;
    int ret;

    fprintf(stderr, "----------------------\nReading existing page:\n----------------------\n");
    descr = pg_cache_lookup(metric, INVALID_TIME);
    if (NULL == descr) {
        *handle = NULL;

        return NULL;
    }
    *handle = descr;

    return descr->page;
}

/* Releases reference to page */
void rrdeng_put_page(void *handle)
{
    pg_cache_put((struct rrdeng_page_cache_descr *)handle);
}

/*
 * Returns 0 on success, 1 on error
 */
int rrdeng_init(void)
{
    int error;

    if (rrdengine_state != RRDENGINE_STATUS_UNINITIALIZED) {
        return 1;
    }
    rrdengine_state = RRDENGINE_STATUS_INITIALIZING;
    sanity_check();

    error = 0;
    memset(&worker_config, 0, sizeof(worker_config));
    init_page_cache();

    init_completion(&rrdengine_completion);
    assert(0 == uv_thread_create(&worker_config.thread, rrdeng_worker, &worker_config));
    /* wait for worker thread to initialize */
    wait_for_completion(&rrdengine_completion);
    destroy_completion(&rrdengine_completion);

    if (error) {
        rrdengine_state = RRDENGINE_STATUS_UNINITIALIZED;
        return 1;
    }

    rrdengine_state = RRDENGINE_STATUS_INITIALIZED;
    return 0;
}

/*
 * Returns 0 on success, 1 on error
 */
int rrdeng_exit(void)
{
    struct rrdeng_cmd cmd;

    if (rrdengine_state != RRDENGINE_STATUS_INITIALIZED) {
        return 1;
    }

    /* TODO: add page to page cache */
    cmd.opcode = RRDENG_SHUTDOWN;
    rrdeng_enq_cmd(&worker_config, &cmd);

    assert(0 == uv_thread_join(&worker_config.thread));

    return 0;
}

#define NR_PAGES (32)
static void basic_functional_test(void)
{
    int i, j, failed_validations;
    usec_t now_usec;
    uuid_t uuid[NR_PAGES];
    void *buf, *handle[NR_PAGES];
    char uuid_str[37];
    char backup[NR_PAGES][37 * 100]; /* backup storage for page data verification */

    for (i = 0 ; i < NR_PAGES ; ++i) {
        uuid_generate(uuid[i]);
        uuid_unparse_lower(uuid[i], uuid_str);
//      fprintf(stderr, "Generated uuid[%d]=%s\n", i, uuid_str);
        buf = rrdeng_create_page(&uuid[i], &handle[i]);
        /* Each page contains 10 times its own UUID stringified */
        for (j = 0 ; j < 100 ; ++j) {
            strcpy(buf + 37 * j, uuid_str);
            strcpy(backup[i] + 37 * j, uuid_str);
        }
        rrdeng_commit_page(handle[i], 37 * 100);
    }
    fprintf(stderr, "\n********** CREATED %d METRIC PAGES ***********\n\n", NR_PAGES);
    failed_validations = 0;
    for (i = 0 ; i < NR_PAGES ; ++i) {
        buf = rrdeng_get_latest_page(&uuid[i], &handle[i]);
        if (NULL == buf) {
            ++failed_validations;
            fprintf(stderr, "Page %d was LOST.\n", i);
        }
        if (memcmp(backup[i], buf, 37 * 100)) {
            ++failed_validations;
            fprintf(stderr, "Page %d data comparison with backup FAILED validation.\n", i);
        }
        rrdeng_put_page(handle[i]);
    }
    fprintf(stderr, "\n********** CORRECTLY VALIDATED %d/%d METRIC PAGES ***********\n\n", NR_PAGES - failed_validations, NR_PAGES);

}
/* C entry point for development purposes
 * make "LDFLAGS=-errdengine_main"
 */
void rrdengine_main(void)
{
    int ret, max_size, compressed_size;
    int fd, i, j;
    long alignment;
    void *block, *buf;
    struct aiocb aio_desc, *aio_descp;
    uv_file file;
    uv_fs_t req;
    uv_buf_t iov;
    static uv_loop_t* loop;
    uv_async_t async;
    char *data = "LIBUV overwrite file contents. LIBUV overwrite file contents. LIBUV overwrite file contents. LIBUV overwrite file contents. LIBUV overwrite file contents. LIBUV overwrite file contents. LIBUV overwrite file contents. LIBUV overwrite file contents. LIBUV overwrite file contents. LIBUV overwrite file contents.\n";
    uv_work_t work_req;
    const volatile int* flag;

    rrdeng_init();

    basic_functional_test();

    rrdeng_exit();
    printf("Hello world!\n");
    exit(0);

    fd = open(TESTFILE, O_DIRECT | O_CREAT | O_RDWR, S_IRUSR | S_IWUSR);
    if (fd == -1) {
        perror("open");
        exit(fd);
    }

    alignment = fpathconf(fd, _PC_REC_XFER_ALIGN);
    if (alignment == -1) {
        perror("fpathconf");
        exit(alignment);
    }
    printf("Alignment = %ld\n", alignment);
    if (alignment > RRDENG_BLOCK_SIZE || (RRDENG_BLOCK_SIZE % alignment != 0)) {
        printf("File alignment incompatible with RRD engine block size.\n");
    }

    ret = posix_memalign(&block, alignment, RRDENG_BLOCK_SIZE);
    if (ret) {
        printf("posix_memalign:%s\n", strerror(ret));
        exit(ret);
    }
    strcpy((char *) block, "Test file contents.\n");

    memset(&aio_desc, 0, sizeof(aio_desc));
    aio_desc.aio_fildes = fd;
    aio_desc.aio_offset = 0;
    aio_desc.aio_buf = block;
    aio_desc.aio_nbytes = RRDENG_BLOCK_SIZE;

    ret = aio_write(&aio_desc);
    if (ret == -1) {
        perror("aio_write");
        exit(ret);
    }
    ret = aio_error(&aio_desc);
    switch (ret) {
    case 0:
        printf("aio_error: request completed\n");
        break;
    case EINPROGRESS:
        printf("aio_error:%s\n", strerror(ret));
        aio_descp = &aio_desc;
        ret = aio_suspend((const struct aiocb *const *)&aio_descp, 1, NULL);
        if (ret == -1) {
            /* should handle catching signals here */
            perror("aio_suspend");
            exit(ret);
        }
        break;
    case ECANCELED:
        printf("aio_error:%s\n", strerror(ret));
        break;
    default:
        printf("aio_error:%s\n", strerror(ret));
        exit(ret);
        break;
    }
    ret = aio_return(&aio_desc);
    if (ret == -1) {
        perror("aio_return");
        exit(ret);
    }

    fd = close(fd);
    if (fd == -1) {
        perror("close");
        exit(fd);
    }

    loop = uv_default_loop();

    fd = uv_fs_open(loop, &req, TESTFILE, UV_FS_O_DIRECT | UV_FS_O_CREAT | UV_FS_O_RDWR, 0, NULL);
    if (fd == -1) {
        perror("uv_fs_open");
        exit(fd);
    }
    uv_run(loop, UV_RUN_DEFAULT);
    assert(req.result >= 0);
    file = req.result;
    uv_fs_req_cleanup(&req);

    assert(strlen(data) + 1 < LZ4_MAX_INPUT_SIZE);
    max_size = LZ4_compressBound(strlen(data) + 1);
    assert(max_size < RRDENG_BLOCK_SIZE);
    buf = malloc(max_size);
    if (!buf) {
        printf("malloc failed.\n");
        exit(1);
    }
    strcpy((char *) buf, data);

    compressed_size = LZ4_compress_default(data, buf, strlen(data) + 1, max_size);
    printf("LZ4 compressed %ld bytes to %d bytes.\n", strlen(data) + 1, compressed_size);

    ret = LZ4_decompress_safe(buf, block, compressed_size, RRDENG_BLOCK_SIZE);
    printf("LZ4 decompressed %d bytes to %d bytes.\n", compressed_size, ret);

    free(buf);

    iov = uv_buf_init(block, 32); /* this must fail with direct IO */

    ret = uv_fs_write(loop, &req, file, &iov, 1, 0, NULL);
    if (ret == -1) {
        perror("uv_fs_write");
        exit(ret);
    }
    uv_run(loop, UV_RUN_DEFAULT);
    if (req.result < 0) {
        /* This should fail */
        printf("uv_fs_write failed as expected: %s\n", uv_strerror((int)req.result));
    }
    uv_fs_req_cleanup(&req);

    strcpy((char *) block, "LIBUV overwrite file contents second try.\n");
    iov = uv_buf_init(block, RRDENG_BLOCK_SIZE);

    ret = uv_fs_write(loop, &req, file, &iov, 1, 0, NULL);
    if (ret == -1) {
        perror("uv_fs_write");
        exit(ret);
    }
    uv_run(loop, UV_RUN_DEFAULT);
    if (req.result < 0) {
        printf("uv_fs_write: %s\n", uv_strerror((int)req.result));
        exit(ret);
    }
    uv_run(loop, UV_RUN_DEFAULT);
    uv_fs_req_cleanup(&req);

    free(block);

    uv_loop_close(loop);

    exit(0);
}