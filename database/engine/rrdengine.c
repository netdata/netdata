// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

rrdeng_stats_t global_io_errors = 0;
rrdeng_stats_t global_fs_errors = 0;
rrdeng_stats_t rrdeng_reserved_file_descriptors = 0;
rrdeng_stats_t global_pg_cache_over_half_dirty_events = 0;
rrdeng_stats_t global_flushing_pressure_page_deletions = 0;

unsigned rrdeng_pages_per_extent = MAX_PAGES_PER_EXTENT;

#if WORKER_UTILIZATION_MAX_JOB_TYPES < (RRDENG_MAX_OPCODE + 2)
#error Please increase WORKER_UTILIZATION_MAX_JOB_TYPES to at least (RRDENG_MAX_OPCODE + 2)
#endif

void *dbengine_page_alloc() {
    void *page = NULL;
    if (unlikely(db_engine_use_malloc))
        page = mallocz(RRDENG_BLOCK_SIZE);
    else {
        page = netdata_mmap(NULL, RRDENG_BLOCK_SIZE, MAP_PRIVATE, enable_ksm, false);
        if(!page) fatal("Cannot allocate dbengine page cache page, with mmap()");
    }
    return page;
}

void dbengine_page_free(void *page) {
    if (unlikely(db_engine_use_malloc))
        freez(page);
    else
        netdata_munmap(page, RRDENG_BLOCK_SIZE);
}

static void sanity_check(void)
{
    BUILD_BUG_ON(WORKER_UTILIZATION_MAX_JOB_TYPES < (RRDENG_MAX_OPCODE + 2));

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

    /* extent cache count must fit in 32 bits */
    BUILD_BUG_ON(MAX_CACHED_EXTENTS > 32);

    /* page info scratch space must be able to hold 2 32-bit integers */
    BUILD_BUG_ON(sizeof(((struct rrdeng_page_info *)0)->scratch) < 2 * sizeof(uint32_t));
}

/* always inserts into tail */
static inline void xt_cache_replaceQ_insert(struct rrdengine_worker_config* wc,
                                            struct extent_cache_element *xt_cache_elem)
{
    struct extent_cache *xt_cache = &wc->xt_cache;

    xt_cache_elem->prev = NULL;
    xt_cache_elem->next = NULL;

    if (likely(NULL != xt_cache->replaceQ_tail)) {
        xt_cache_elem->prev = xt_cache->replaceQ_tail;
        xt_cache->replaceQ_tail->next = xt_cache_elem;
    }
    if (unlikely(NULL == xt_cache->replaceQ_head)) {
        xt_cache->replaceQ_head = xt_cache_elem;
    }
    xt_cache->replaceQ_tail = xt_cache_elem;
}

static inline void xt_cache_replaceQ_delete(struct rrdengine_worker_config* wc,
                                            struct extent_cache_element *xt_cache_elem)
{
    struct extent_cache *xt_cache = &wc->xt_cache;
    struct extent_cache_element *prev, *next;

    prev = xt_cache_elem->prev;
    next = xt_cache_elem->next;

    if (likely(NULL != prev)) {
        prev->next = next;
    }
    if (likely(NULL != next)) {
        next->prev = prev;
    }
    if (unlikely(xt_cache_elem == xt_cache->replaceQ_head)) {
        xt_cache->replaceQ_head = next;
    }
    if (unlikely(xt_cache_elem == xt_cache->replaceQ_tail)) {
        xt_cache->replaceQ_tail = prev;
    }
    xt_cache_elem->prev = xt_cache_elem->next = NULL;
}

static inline void xt_cache_replaceQ_set_hot(struct rrdengine_worker_config* wc,
                                             struct extent_cache_element *xt_cache_elem)
{
    xt_cache_replaceQ_delete(wc, xt_cache_elem);
    xt_cache_replaceQ_insert(wc, xt_cache_elem);
}

/* Returns the index of the cached extent if it was successfully inserted in the extent cache, otherwise -1 */
static int try_insert_into_xt_cache(struct rrdengine_worker_config* wc, struct extent_info *extent)
{
    struct extent_cache *xt_cache = &wc->xt_cache;
    struct extent_cache_element *xt_cache_elem;
    unsigned idx;
    int ret;

    if (unlikely(!extent))
        return -1;

    ret = find_first_zero(xt_cache->allocation_bitmap);
    if (-1 == ret || ret >= MAX_CACHED_EXTENTS) {
        for (xt_cache_elem = xt_cache->replaceQ_head ; NULL != xt_cache_elem ; xt_cache_elem = xt_cache_elem->next) {
            idx = xt_cache_elem - xt_cache->extent_array;
            if (!check_bit(xt_cache->inflight_bitmap, idx)) {
                xt_cache_replaceQ_delete(wc, xt_cache_elem);
                break;
            }
        }
        if (NULL == xt_cache_elem)
            return -1;
    } else {
        idx = (unsigned)ret;
        xt_cache_elem = &xt_cache->extent_array[idx];
    }
    xt_cache_elem->extent = extent;
    xt_cache_elem->fileno = extent->datafile->fileno;
    xt_cache_elem->inflight_io_descr = NULL;
    xt_cache_replaceQ_insert(wc, xt_cache_elem);
    modify_bit(&xt_cache->allocation_bitmap, idx, 1);

    return (int)idx;
}

/**
 * Returns 0 if the cached extent was found in the extent cache, 1 otherwise.
 * Sets *idx to point to the position of the extent inside the cache.
 **/
static uint8_t lookup_in_xt_cache(struct rrdengine_worker_config* wc, struct extent_info *extent, unsigned *idx)
{
    struct extent_cache *xt_cache = &wc->xt_cache;
    struct extent_cache_element *xt_cache_elem;
    unsigned i;

    if (unlikely(!extent))
        return 1;

    for (i = 0 ; i < MAX_CACHED_EXTENTS ; ++i) {
        xt_cache_elem = &xt_cache->extent_array[i];
        if (check_bit(xt_cache->allocation_bitmap, i) && xt_cache_elem->extent == extent &&
            xt_cache_elem->fileno == extent->datafile->fileno) {
            *idx = i;
            return 0;
        }
    }
    return 1;
}

#if 0 /* disabled code */
static void delete_from_xt_cache(struct rrdengine_worker_config* wc, unsigned idx)
{
    struct extent_cache *xt_cache = &wc->xt_cache;
    struct extent_cache_element *xt_cache_elem;

    xt_cache_elem = &xt_cache->extent_array[idx];
    xt_cache_replaceQ_delete(wc, xt_cache_elem);
    xt_cache_elem->extent = NULL;
    modify_bit(&wc->xt_cache.allocation_bitmap, idx, 0); /* invalidate it */
    modify_bit(&wc->xt_cache.inflight_bitmap, idx, 0); /* not in-flight anymore */
}
#endif

void enqueue_inflight_read_to_xt_cache(struct rrdengine_worker_config* wc, unsigned idx,
                                       struct extent_io_descriptor *xt_io_descr)
{
    struct extent_cache *xt_cache = &wc->xt_cache;
    struct extent_cache_element *xt_cache_elem;
    struct extent_io_descriptor *old_next;

    xt_cache_elem = &xt_cache->extent_array[idx];
    old_next = xt_cache_elem->inflight_io_descr->next;
    xt_cache_elem->inflight_io_descr->next = xt_io_descr;
    xt_io_descr->next = old_next;
}

void read_cached_extent_cb(struct rrdengine_worker_config* wc, unsigned idx, struct extent_io_descriptor *xt_io_descr)
{
    unsigned i, j, page_offset;
    struct rrdengine_instance *ctx = wc->ctx;
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr;
    void *page;
    struct extent_info *extent = xt_io_descr->descr_array[0]->extent;

    for (i = 0 ; i < xt_io_descr->descr_count; ++i) {
        page = dbengine_page_alloc();
        descr = xt_io_descr->descr_array[i];
        for (j = 0, page_offset = 0 ; j < extent->number_of_pages ; ++j) {
            /* care, we don't hold the descriptor mutex */
            if (!uuid_compare(*extent->pages[j]->id, *descr->id) &&
                extent->pages[j]->page_length == descr->page_length &&
                extent->pages[j]->start_time_ut == descr->start_time_ut &&
                extent->pages[j]->end_time_ut == descr->end_time_ut) {
                break;
            }
            page_offset += extent->pages[j]->page_length;

        }
        /* care, we don't hold the descriptor mutex */
       (void) memcpy(page, wc->xt_cache.extent_array[idx].pages + page_offset, descr->page_length);

        rrdeng_page_descr_mutex_lock(ctx, descr);
        pg_cache_descr = descr->pg_cache_descr;
        pg_cache_descr->page = page;
        pg_cache_descr->flags |= RRD_PAGE_POPULATED;
        pg_cache_descr->flags &= ~RRD_PAGE_READ_PENDING;
        rrdeng_page_descr_mutex_unlock(ctx, descr);
        pg_cache_replaceQ_insert(ctx, descr);
        if (xt_io_descr->release_descr) {
            pg_cache_put(ctx, descr);
        } else {
            debug(D_RRDENGINE, "%s: Waking up waiters.", __func__);
            pg_cache_wake_up_waiters(ctx, descr);
        }
    }
    if (xt_io_descr->completion)
        completion_mark_complete(xt_io_descr->completion);
    freez(xt_io_descr);
}

static void fill_page_with_nulls(void *page, uint32_t page_length, uint8_t type) {
    switch(type) {
        case PAGE_METRICS: {
            storage_number n = pack_storage_number(NAN, SN_FLAG_NONE);
            storage_number *array = (storage_number *)page;
            size_t slots = page_length / sizeof(n);
            for(size_t i = 0; i < slots ; i++)
                array[i] = n;
        }
        break;

        case PAGE_TIER: {
            storage_number_tier1_t n = {
                .min_value = NAN,
                .max_value = NAN,
                .sum_value = NAN,
                .count = 1,
                .anomaly_count = 0,
            };
            storage_number_tier1_t *array = (storage_number_tier1_t *)page;
            size_t slots = page_length / sizeof(n);
            for(size_t i = 0; i < slots ; i++)
                array[i] = n;
        }
        break;

        default: {
            static bool logged = false;
            if(!logged) {
                error("DBENGINE: cannot fill page with nulls on unknown page type id %d", type);
                logged = true;
            }
            memset(page, 0, page_length);
        }
    }
}

static void do_extent_processing (struct rrdengine_worker_config *wc, struct extent_io_descriptor *xt_io_descr, bool read_failed)
{
    struct rrdengine_instance *ctx = wc->ctx;
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr;
    int ret;
    unsigned i, j, count;
    void *page, *uncompressed_buf = NULL;
    uint32_t payload_length, payload_offset, page_offset, uncompressed_payload_length = 0;
    uint8_t have_read_error = 0;
    /* persistent structures */
    struct rrdeng_df_extent_header *header;
    struct rrdeng_df_extent_trailer *trailer;
    uLong crc;

    header = xt_io_descr->buf;
    payload_length = header->payload_length;
    count = header->number_of_pages;
    payload_offset = sizeof(*header) + sizeof(header->descr[0]) * count;
    trailer = xt_io_descr->buf + xt_io_descr->bytes - sizeof(*trailer);

    if (unlikely(read_failed)) {
        struct rrdengine_datafile *datafile = xt_io_descr->descr_array[0]->extent->datafile;

        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        have_read_error = 1;
        error("%s: uv_fs_read - extent at offset %"PRIu64"(%u) in datafile %u-%u.", __func__, xt_io_descr->pos,
              xt_io_descr->bytes, datafile->tier, datafile->fileno);
        goto after_crc_check;
    }
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, xt_io_descr->buf, xt_io_descr->bytes - sizeof(*trailer));
    ret = crc32cmp(trailer->checksum, crc);
#ifdef NETDATA_INTERNAL_CHECKS
    if(xt_io_descr->descr_array[0]->extent) {
        struct rrdengine_datafile *datafile = xt_io_descr->descr_array[0]->extent->datafile;
        debug(D_RRDENGINE, "%s: Extent at offset %"PRIu64"(%u) was read from datafile %u-%u. CRC32 check: %s", __func__,
              xt_io_descr->pos, xt_io_descr->bytes, datafile->tier, datafile->fileno, ret ? "FAILED" : "SUCCEEDED");
    }
#endif
    if (unlikely(ret)) {
        struct rrdengine_datafile *datafile = xt_io_descr->descr_array[0]->extent->datafile;

        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        have_read_error = 1;
        error("%s: Extent at offset %"PRIu64"(%u) was read from datafile %u-%u. CRC32 check: FAILED", __func__,
              xt_io_descr->pos, xt_io_descr->bytes, datafile->tier, datafile->fileno);
    }

after_crc_check:
    if (!have_read_error && RRD_NO_COMPRESSION != header->compression_algorithm) {
        uncompressed_payload_length = 0;
        for (i = 0 ; i < count ; ++i) {
            uncompressed_payload_length += header->descr[i].page_length;
        }
        uncompressed_buf = mallocz(uncompressed_payload_length);
        ret = LZ4_decompress_safe(xt_io_descr->buf + payload_offset, uncompressed_buf,
                                  payload_length, uncompressed_payload_length);
        ctx->stats.before_decompress_bytes += payload_length;
        ctx->stats.after_decompress_bytes += ret;
        debug(D_RRDENGINE, "LZ4 decompressed %u bytes to %d bytes.", payload_length, ret);
        /* care, we don't hold the descriptor mutex */
    }
    {
        uint8_t xt_is_cached = 0;
        unsigned xt_idx;
        struct extent_info *extent = xt_io_descr->descr_array[0]->extent;

        if (extent) {
            xt_is_cached = !lookup_in_xt_cache(wc, extent, &xt_idx);
            if (xt_is_cached && check_bit(wc->xt_cache.inflight_bitmap, xt_idx)) {
                struct extent_cache *xt_cache = &wc->xt_cache;
                struct extent_cache_element *xt_cache_elem = &xt_cache->extent_array[xt_idx];
                struct extent_io_descriptor *curr, *next;

                if (have_read_error) {
                    memset(xt_cache_elem->pages, 0, sizeof(xt_cache_elem->pages));
                } else if (RRD_NO_COMPRESSION == header->compression_algorithm) {
                    (void)memcpy(xt_cache_elem->pages, xt_io_descr->buf + payload_offset, payload_length);
                } else {
                    (void)memcpy(xt_cache_elem->pages, uncompressed_buf, uncompressed_payload_length);
                }
                /* complete all connected in-flight read requests */
                for (curr = xt_cache_elem->inflight_io_descr->next; curr; curr = next) {
                    next = curr->next;
                    read_cached_extent_cb(wc, xt_idx, curr);
                }
                xt_cache_elem->inflight_io_descr = NULL;
                modify_bit(&xt_cache->inflight_bitmap, xt_idx, 0); /* not in-flight anymore */
            }
        }
    }

    for (i = 0, page_offset = 0; i < count; page_offset += header->descr[i++].page_length) {
        uint8_t is_prefetched_page;
        descr = NULL;
        for (j = 0 ; j < xt_io_descr->descr_count; ++j) {
            struct rrdeng_page_descr *descrj;

            descrj = xt_io_descr->descr_array[j];
            /* care, we don't hold the descriptor mutex */
            if (!uuid_compare(*(uuid_t *) header->descr[i].uuid, *descrj->id) &&
                header->descr[i].page_length == descrj->page_length &&
                header->descr[i].start_time_ut == descrj->start_time_ut &&
                header->descr[i].end_time_ut == descrj->end_time_ut) {
                descr = descrj;
                bitmap256_set_bit(&xt_io_descr->descr_array_wakeup, j, 0);
                break;
            }
        }
        is_prefetched_page = 0;
        if (!descr) { /* This extent page has not been requested. Try populating it for locality (best effort). */
            struct page_cache *pg_cache = &ctx->pg_cache;
            if (pg_cache->populated_pages >= ctx->cache_pages_low_watermark)
                continue;

            descr = pg_cache_lookup_unpopulated_and_lock(ctx, (uuid_t *)header->descr[i].uuid,
                                                         header->descr[i].start_time_ut);
            if (!descr)
                continue; /* Failed to reserve a suitable page */
            is_prefetched_page = 1;
        }
        page = dbengine_page_alloc();

        /* care, we don't hold the descriptor mutex */
        if (have_read_error) {
            fill_page_with_nulls(page, descr->page_length, descr->type);
        } else if (RRD_NO_COMPRESSION == header->compression_algorithm) {
            (void) memcpy(page, xt_io_descr->buf + payload_offset + page_offset, descr->page_length);
        } else {
            (void) memcpy(page, uncompressed_buf + page_offset, descr->page_length);
        }
        rrdeng_page_descr_mutex_lock(ctx, descr);
        pg_cache_descr = descr->pg_cache_descr;
        pg_cache_descr->page = page;
        pg_cache_descr->flags |= RRD_PAGE_POPULATED;
        pg_cache_descr->flags &= ~RRD_PAGE_READ_PENDING;
        rrdeng_page_descr_mutex_unlock(ctx, descr);
        pg_cache_replaceQ_insert(ctx, descr);
        if (xt_io_descr->release_descr || is_prefetched_page) {
            pg_cache_put(ctx, descr);
        } else {
            debug(D_RRDENGINE, "%s: Waking up waiters.", __func__);
            pg_cache_wake_up_waiters(ctx, descr);
        }
    }
    for (j = 0 ; j < xt_io_descr->descr_count; ++j) {
        struct rrdeng_page_descr *descr =  xt_io_descr->descr_array[j];

        if (unlikely(bitmap256_get_bit(&xt_io_descr->descr_array_wakeup, j))) {
            if (!(descr->pg_cache_descr->flags & RRD_PAGE_POPULATED)) {
                descr->pg_cache_descr->flags &= ~RRD_PAGE_READ_PENDING;
                descr->pg_cache_descr->flags |= RRD_PAGE_INVALID;
            }
            pg_cache_wake_up_waiters(ctx, descr);
        }
    }
    if (!have_read_error && RRD_NO_COMPRESSION != header->compression_algorithm) {
        freez(uncompressed_buf);
    }
    if (xt_io_descr->completion)
        completion_mark_complete(xt_io_descr->completion);
}

static void read_extent_cb(uv_fs_t *req)
{
    struct rrdengine_worker_config *wc = req->loop->data;
    struct extent_io_descriptor *xt_io_descr;

    xt_io_descr = req->data;
    do_extent_processing(wc, xt_io_descr, req->result < 0);
    uv_fs_req_cleanup(req);
    posix_memfree(xt_io_descr->buf);
    freez(xt_io_descr);
}

static void read_mmap_extent_cb(uv_work_t *req, int status __maybe_unused)
{
    struct rrdengine_worker_config *wc = req->loop->data;
    struct rrdengine_instance *ctx = wc->ctx;
    struct extent_io_descriptor *xt_io_descr;
    xt_io_descr = req->data;

    if (likely(xt_io_descr->map_base)) {
        do_extent_processing(wc, xt_io_descr, false);
        munmap(xt_io_descr->map_base, xt_io_descr->map_length);
        freez(xt_io_descr);
        return;
    }

    // MMAP failed, so do uv_fs_read
    int ret = posix_memalign((void *)&xt_io_descr->buf, RRDFILE_ALIGNMENT, ALIGN_BYTES_CEILING(xt_io_descr->bytes));
    if (unlikely(ret)) {
        fatal("posix_memalign:%s", strerror(ret));
    }
    unsigned real_io_size = ALIGN_BYTES_CEILING( xt_io_descr->bytes);
    xt_io_descr->iov = uv_buf_init((void *)xt_io_descr->buf, real_io_size);
    xt_io_descr->req.data = xt_io_descr;
    ret = uv_fs_read(req->loop, &xt_io_descr->req, xt_io_descr->file, &xt_io_descr->iov, 1, (unsigned) xt_io_descr->pos, read_extent_cb);
    fatal_assert(-1 != ret);
    ctx->stats.io_read_bytes += real_io_size;
    ctx->stats.io_read_extent_bytes += real_io_size;
}

static void do_mmap_read_extent(uv_work_t *req)
{
    struct extent_io_descriptor *xt_io_descr = (struct extent_io_descriptor * )req->data;
    struct rrdengine_worker_config *wc = req->loop->data;
    struct rrdengine_instance *ctx = wc->ctx;

    off_t map_start =  ALIGN_BYTES_FLOOR(xt_io_descr->pos);
    size_t length = ALIGN_BYTES_CEILING(xt_io_descr->pos + xt_io_descr->bytes) - map_start;
    unsigned real_io_size = xt_io_descr->bytes;

    void *data = mmap(NULL, length, PROT_READ, MAP_SHARED, xt_io_descr->file, map_start);
    if (likely(data != MAP_FAILED)) {
        xt_io_descr->map_base = data;
        xt_io_descr->map_length = length;
        xt_io_descr->buf = data + (xt_io_descr->pos - map_start);
        ctx->stats.io_read_bytes += real_io_size;
        ctx->stats.io_read_extent_bytes += real_io_size;
    }
}

static void do_read_extent(struct rrdengine_worker_config* wc,
                           struct rrdeng_page_descr **descr,
                           unsigned count,
                           uint8_t release_descr)
{
    struct rrdengine_instance *ctx = wc->ctx;
    struct page_cache_descr *pg_cache_descr;
    int ret;
    unsigned i, size_bytes, pos;
    struct extent_io_descriptor *xt_io_descr;
    struct rrdengine_datafile *datafile;
    struct extent_info *extent = descr[0]->extent;
    uint8_t xt_is_cached = 0, xt_is_inflight = 0;
    unsigned xt_idx;
    uv_file file_to_use;

    if (extent) {
        datafile = extent->datafile;
        file_to_use = datafile->file;
        pos = extent->offset;
        size_bytes = extent->size;
    }
    else {
        struct journal_extent_list *extent_entry = (struct journal_extent_list *) descr[0]->extent_entry;
        file_to_use = descr[0]->datafile_fd;
        pos = extent_entry->datafile_offset;
        size_bytes = extent_entry->datafile_size;
    }

    xt_io_descr = callocz(1, sizeof(*xt_io_descr));
    for (i = 0 ; i < count; ++i) {
        rrdeng_page_descr_mutex_lock(ctx, descr[i]);
        pg_cache_descr = descr[i]->pg_cache_descr;
        pg_cache_descr->flags |= RRD_PAGE_READ_PENDING;
        rrdeng_page_descr_mutex_unlock(ctx, descr[i]);
        xt_io_descr->descr_array[i] = descr[i];
        bitmap256_set_bit(&xt_io_descr->descr_array_wakeup, i, 1);
    }
    xt_io_descr->descr_count = count;
    xt_io_descr->file = file_to_use;
    xt_io_descr->bytes = size_bytes;
    xt_io_descr->pos = pos;
    xt_io_descr->req_worker.data = xt_io_descr;
    xt_io_descr->completion = NULL;
    xt_io_descr->release_descr = release_descr;
    xt_io_descr->buf = NULL;

    if (extent) {
        xt_is_cached = !lookup_in_xt_cache(wc, extent, &xt_idx);
        if (xt_is_cached) {
            xt_cache_replaceQ_set_hot(wc, &wc->xt_cache.extent_array[xt_idx]);
            xt_is_inflight = check_bit(wc->xt_cache.inflight_bitmap, xt_idx);
            if (xt_is_inflight) {
                enqueue_inflight_read_to_xt_cache(wc, xt_idx, xt_io_descr);
                return;
            }
            return read_cached_extent_cb(wc, xt_idx, xt_io_descr);
        } else {
            ret = try_insert_into_xt_cache(wc, extent);
            if (-1 != ret) {
                xt_idx = (unsigned)ret;
                modify_bit(&wc->xt_cache.inflight_bitmap, xt_idx, 1);
                wc->xt_cache.extent_array[xt_idx].inflight_io_descr = xt_io_descr;
            }
        }
    }

    ret = uv_queue_work(wc->loop, &xt_io_descr->req_worker, do_mmap_read_extent, read_mmap_extent_cb);
    fatal_assert(-1 != ret);

    ++ctx->stats.io_read_requests;
    ++ctx->stats.io_read_extents;
    ctx->stats.pg_cache_backfills += count;
}

static void commit_data_extent(struct rrdengine_worker_config* wc, struct extent_io_descriptor *xt_io_descr)
{
    struct rrdengine_instance *ctx = wc->ctx;
    unsigned count, payload_length, descr_size, size_bytes;
    void *buf;
    /* persistent structures */
    struct rrdeng_df_extent_header *df_header;
    struct rrdeng_jf_transaction_header *jf_header;
    struct rrdeng_jf_store_data *jf_metric_data;
    struct rrdeng_jf_transaction_trailer *jf_trailer;
    uLong crc;

    df_header = xt_io_descr->buf;
    count = df_header->number_of_pages;
    descr_size = sizeof(*jf_metric_data->descr) * count;
    payload_length = sizeof(*jf_metric_data) + descr_size;
    size_bytes = sizeof(*jf_header) + payload_length + sizeof(*jf_trailer);

    buf = wal_get_transaction_buffer(wc, size_bytes);

    jf_header = buf;
    jf_header->type = STORE_DATA;
    jf_header->reserved = 0;
    jf_header->id = ctx->commit_log.transaction_id++;
    jf_header->payload_length = payload_length;

    jf_metric_data = buf + sizeof(*jf_header);
    jf_metric_data->extent_offset = xt_io_descr->pos;
    jf_metric_data->extent_size = xt_io_descr->bytes;
    jf_metric_data->number_of_pages = count;
    memcpy(jf_metric_data->descr, df_header->descr, descr_size);

    jf_trailer = buf + sizeof(*jf_header) + payload_length;
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, buf, sizeof(*jf_header) + payload_length);
    crc32set(jf_trailer->checksum, crc);
}

static void do_commit_transaction(struct rrdengine_worker_config* wc, uint8_t type, void *data)
{
    switch (type) {
    case STORE_DATA:
        commit_data_extent(wc, (struct extent_io_descriptor *)data);
        break;
    default:
        fatal_assert(type == STORE_DATA);
        break;
    }
}

static void after_invalidate_oldest_committed(struct rrdengine_worker_config* wc)
{
    int error;

    error = uv_thread_join(wc->now_invalidating_dirty_pages);
    if (error) {
        error("uv_thread_join(): %s", uv_strerror(error));
    }
    freez(wc->now_invalidating_dirty_pages);
    wc->now_invalidating_dirty_pages = NULL;
    wc->cleanup_thread_invalidating_dirty_pages = 0;
}

static void invalidate_oldest_committed(void *arg)
{
    struct rrdengine_instance *ctx = arg;
    struct rrdengine_worker_config *wc = &ctx->worker_config;
    struct page_cache *pg_cache = &ctx->pg_cache;
    int ret;
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr;
    Pvoid_t *PValue;
    Word_t Index;
    unsigned nr_committed_pages;

    do {
        uv_rwlock_wrlock(&pg_cache->committed_page_index.lock);
        for (Index = 0,
             PValue = JudyLFirst(pg_cache->committed_page_index.JudyL_array, &Index, PJE0),
             descr = unlikely(NULL == PValue) ? NULL : *PValue;

             descr != NULL;

             PValue = JudyLNext(pg_cache->committed_page_index.JudyL_array, &Index, PJE0),
                     descr = unlikely(NULL == PValue) ? NULL : *PValue) {
            fatal_assert(0 != descr->page_length);

            rrdeng_page_descr_mutex_lock(ctx, descr);
            pg_cache_descr = descr->pg_cache_descr;
            if (!(pg_cache_descr->flags & RRD_PAGE_WRITE_PENDING) && pg_cache_try_get_unsafe(descr, 1)) {
                rrdeng_page_descr_mutex_unlock(ctx, descr);

                ret = JudyLDel(&pg_cache->committed_page_index.JudyL_array, Index, PJE0);
                fatal_assert(1 == ret);
                break;
            }
            rrdeng_page_descr_mutex_unlock(ctx, descr);
        }
        uv_rwlock_wrunlock(&pg_cache->committed_page_index.lock);

        if (!descr) {
            info("Failed to invalidate any dirty pages to relieve page cache pressure.");

            goto out;
        }
        pg_cache_punch_hole(ctx, descr, 1, 1, NULL, true);

        uv_rwlock_wrlock(&pg_cache->committed_page_index.lock);
        nr_committed_pages = --pg_cache->committed_page_index.nr_committed_pages;
        uv_rwlock_wrunlock(&pg_cache->committed_page_index.lock);
        rrd_stat_atomic_add(&ctx->stats.flushing_pressure_page_deletions, 1);
        rrd_stat_atomic_add(&global_flushing_pressure_page_deletions, 1);

    } while (nr_committed_pages >= pg_cache_committed_hard_limit(ctx));
out:
    wc->cleanup_thread_invalidating_dirty_pages = 1;
    /* wake up event loop */
    fatal_assert(0 == uv_async_send(&wc->async));
}

void rrdeng_invalidate_oldest_committed(struct rrdengine_worker_config* wc)
{
    struct rrdengine_instance *ctx = wc->ctx;
    struct page_cache *pg_cache = &ctx->pg_cache;
    unsigned nr_committed_pages;
    int error;

    if (unlikely(ctx->quiesce != NO_QUIESCE)) /* Shutting down */
        return;

    uv_rwlock_rdlock(&pg_cache->committed_page_index.lock);
    nr_committed_pages = pg_cache->committed_page_index.nr_committed_pages;
    uv_rwlock_rdunlock(&pg_cache->committed_page_index.lock);

    if (nr_committed_pages >= pg_cache_committed_hard_limit(ctx)) {
        /* delete the oldest page in memory */
        if (wc->now_invalidating_dirty_pages) {
            /* already deleting a page */
            return;
        }
        errno = 0;
        error("Failed to flush dirty buffers quickly enough in dbengine instance \"%s\". "
              "Metric data are being deleted, please reduce disk load or use a faster disk.", ctx->dbfiles_path);

        wc->now_invalidating_dirty_pages = mallocz(sizeof(*wc->now_invalidating_dirty_pages));
        wc->cleanup_thread_invalidating_dirty_pages = 0;

        error = uv_thread_create(wc->now_invalidating_dirty_pages, invalidate_oldest_committed, ctx);
        if (error) {
            error("uv_thread_create(): %s", uv_strerror(error));
            freez(wc->now_invalidating_dirty_pages);
            wc->now_invalidating_dirty_pages = NULL;
        }
    }
}

void after_delete_descriptors(uv_work_t *req, int status __maybe_unused)
{
    struct rrdeng_work  *work_request = req->data;
    struct rrdengine_worker_config *wc = req->loop->data;

    wc->delete_descriptors = 0;
    freez(work_request);
}

static struct pg_cache_page_index *get_page_index(struct page_cache *pg_cache, uuid_t *uuid)
{
    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    Pvoid_t *PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, uuid, sizeof(uuid_t));
    struct pg_cache_page_index *page_index = (NULL == PValue) ? NULL : *PValue;
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
    return page_index;
}

static struct rrdeng_page_descr *get_descriptor(struct pg_cache_page_index *page_index, time_t start_time_s)
{
    uv_rwlock_rdlock(&page_index->lock);
    Pvoid_t *PValue = JudyLGet(page_index->JudyL_array, start_time_s, PJE0);
    struct rrdeng_page_descr *descr = unlikely(NULL == PValue) ? NULL : *PValue;
    uv_rwlock_rdunlock(&page_index->lock);
    return descr;
};

static bool try_to_remove_v2_descriptor( struct rrdengine_instance *ctx, struct pg_cache_page_index *page_index, time_t start_time_s, bool expired)
{
    struct rrdeng_page_descr *descr = get_descriptor(page_index, start_time_s);
    if (unlikely(!descr))
        return true;

    rrdeng_page_descr_mutex_lock(ctx, descr);
    unsigned flags = descr->pg_cache_descr->flags & RRD_PAGE_POPULATED;
    if ((!flags || expired) && pg_cache_try_get_unsafe(descr, 1)) {
        rrdeng_page_descr_mutex_unlock(ctx, descr);
        pg_cache_punch_hole(ctx, descr, 0, 1, NULL, false);
        return true;
    }
    rrdeng_page_descr_mutex_unlock(ctx, descr);

    return false;
}
#define JOURVAL_V2_DESCRIPTOR_EXPIRATION_TIME (600)

void check_journal_file(struct rrdengine_journalfile *journalfile)
{
    struct rrdengine_instance *ctx = journalfile->datafile->ctx;
    struct page_cache *pg_cache = &ctx->pg_cache;

    Pvoid_t *PValue;
    Word_t page_address;
    uint32_t Index;

    struct journal_v2_header *journal_header = (struct journal_v2_header *) journalfile->journal_data;
    time_t journal_start_time_s = (time_t) (journal_header->start_time_ut / USEC_PER_SEC);

    uv_rwlock_rdlock(&pg_cache->v2_lock);

    bool expired = ((now_realtime_sec() - journalfile->last_access) > JOURVAL_V2_DESCRIPTOR_EXPIRATION_TIME);

    for (page_address = 0,
        PValue = JudyLFirst(journalfile->JudyL_array, &page_address, PJE0),
        Index = unlikely(NULL == PValue) ? 0 : *(uint32_t *) PValue;
        Index ;
        PValue = JudyLNext(journalfile->JudyL_array, &page_address, PJE0),
        Index = unlikely(NULL == PValue) ? 0 : *(uint32_t *) PValue) {

        uv_rwlock_rdunlock(&pg_cache->v2_lock);

        // Assume we will evict everything
        bool all_evicted = true;
        // Get the page index will be working on
        struct journal_page_header *page_list_header = journalfile->journal_data + page_address;
        struct pg_cache_page_index *page_index = get_page_index(pg_cache, &page_list_header->uuid);

        if (likely(page_index)) {
            struct journal_page_list *page_list = (struct journal_page_list *) ((void *) page_list_header + sizeof(*page_list_header));
            uint32_t entries = page_list_header->entries;

            // First try to target the marked entry; Note marked entry is recorded +1
            struct journal_page_list *page_entry = &page_list[Index - 1];
            time_t index_time_s = journal_start_time_s + page_entry->delta_start_s;
            all_evicted = try_to_remove_v2_descriptor(ctx, page_index, index_time_s, expired);

            // Try to scan range ; all need to return evicted
            for (uint32_t x = 0; all_evicted && x < entries; x++) {
                index_time_s = journal_start_time_s + (&page_list[x])->delta_start_s;
                all_evicted = all_evicted && try_to_remove_v2_descriptor(ctx, page_index, index_time_s, expired);
            }
        }

        if (all_evicted) {
            uv_rwlock_wrlock(&pg_cache->v2_lock);
            (void) JudyLDel(&journalfile->JudyL_array, page_address, PJE0);
            uv_rwlock_wrunlock(&pg_cache->v2_lock);
        }

        uv_rwlock_rdlock(&pg_cache->v2_lock);
    }

    uv_rwlock_rdunlock(&pg_cache->v2_lock);
}

void delete_descriptors(uv_work_t *req)
{
    struct rrdengine_worker_config *wc = req->loop->data;
    struct rrdengine_instance *ctx = wc->ctx;
    struct page_cache *pg_cache = &ctx->pg_cache;

    if (unlikely(ctx->quiesce != NO_QUIESCE)) /* Shutting down */
        return;

    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    struct rrdengine_datafile *datafile = ctx->datafiles.first;
    struct rrdengine_journalfile *journalfile;
    while (datafile) {
        journalfile = datafile->journalfile;
        if (!journalfile->journal_data) {
            datafile = datafile->next;
            continue;
        }

        uv_rwlock_rdlock(&pg_cache->v2_lock);
        Word_t count = JudyLCount(journalfile->JudyL_array, 0, -1, PJE0);
        uv_rwlock_rdunlock(&pg_cache->v2_lock);

        if (unlikely(count))
            check_journal_file(journalfile);

        datafile = datafile->next;
    }

    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);
}

void flush_pages_cb(uv_fs_t* req)
{
    struct rrdengine_worker_config* wc = req->loop->data;
    struct rrdengine_instance *ctx = wc->ctx;
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct extent_io_descriptor *xt_io_descr;
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr;
    unsigned i, count;

    xt_io_descr = req->data;
    if (req->result < 0) {
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        error("%s: uv_fs_write: %s", __func__, uv_strerror((int)req->result));
    }
#ifdef NETDATA_INTERNAL_CHECKS
    {
        struct rrdengine_datafile *datafile = xt_io_descr->descr_array[0]->extent->datafile;
        debug(D_RRDENGINE, "%s: Extent at offset %"PRIu64"(%u) was written to datafile %u-%u. Waking up waiters.",
              __func__, xt_io_descr->pos, xt_io_descr->bytes, datafile->tier, datafile->fileno);
    }
#endif
    count = xt_io_descr->descr_count;
    for (i = 0 ; i < count ; ++i) {
        /* care, we don't hold the descriptor mutex */
        descr = xt_io_descr->descr_array[i];

        pg_cache_replaceQ_insert(ctx, descr);

        rrdeng_page_descr_mutex_lock(ctx, descr);
        pg_cache_descr = descr->pg_cache_descr;
        pg_cache_descr->flags &= ~(RRD_PAGE_DIRTY | RRD_PAGE_WRITE_PENDING);
        /* wake up waiters, care no reference being held */
        pg_cache_wake_up_waiters_unsafe(descr);
        rrdeng_page_descr_mutex_unlock(ctx, descr);
    }
    if (xt_io_descr->completion)
        completion_mark_complete(xt_io_descr->completion);
    uv_fs_req_cleanup(req);
    posix_memfree(xt_io_descr->buf);
    freez(xt_io_descr);

    uv_rwlock_wrlock(&pg_cache->committed_page_index.lock);
    pg_cache->committed_page_index.nr_committed_pages -= count;
    uv_rwlock_wrunlock(&pg_cache->committed_page_index.lock);
    wc->inflight_dirty_pages -= count;
}

/*
 * completion must be NULL or valid.
 * Returns 0 when no flushing can take place.
 * Returns datafile bytes to be written on successful flushing initiation.
 */
static int do_flush_pages(struct rrdengine_worker_config* wc, int force, struct completion *completion)
{
    struct rrdengine_instance *ctx = wc->ctx;
    struct page_cache *pg_cache = &ctx->pg_cache;
    int ret;
    int compressed_size, max_compressed_size = 0;
    unsigned i, count, size_bytes, pos, real_io_size;
    uint32_t uncompressed_payload_length, payload_offset;
    struct rrdeng_page_descr *descr, *eligible_pages[MAX_PAGES_PER_EXTENT];
    struct page_cache_descr *pg_cache_descr;
    struct extent_io_descriptor *xt_io_descr;
    void *compressed_buf = NULL;
    Word_t descr_commit_idx_array[MAX_PAGES_PER_EXTENT];
    Pvoid_t *PValue;
    Word_t Index;
    uint8_t compression_algorithm = ctx->global_compress_alg;
    struct extent_info *extent;
    struct rrdengine_datafile *datafile;
    /* persistent structures */
    struct rrdeng_df_extent_header *header;
    struct rrdeng_df_extent_trailer *trailer;
    uLong crc;

    if (force) {
        debug(D_RRDENGINE, "Asynchronous flushing of extent has been forced by page pressure.");
    }
    uv_rwlock_wrlock(&pg_cache->committed_page_index.lock);
    for (Index = 0, count = 0, uncompressed_payload_length = 0,
         PValue = JudyLFirst(pg_cache->committed_page_index.JudyL_array, &Index, PJE0),
         descr = unlikely(NULL == PValue) ? NULL : *PValue ;

         descr != NULL && count != rrdeng_pages_per_extent;

         PValue = JudyLNext(pg_cache->committed_page_index.JudyL_array, &Index, PJE0),
         descr = unlikely(NULL == PValue) ? NULL : *PValue) {
        uint8_t page_write_pending;

        fatal_assert(0 != descr->page_length);
        page_write_pending = 0;

        rrdeng_page_descr_mutex_lock(ctx, descr);
        pg_cache_descr = descr->pg_cache_descr;
        if (!(pg_cache_descr->flags & RRD_PAGE_WRITE_PENDING)) {
            page_write_pending = 1;
            /* care, no reference being held */
            pg_cache_descr->flags |= RRD_PAGE_WRITE_PENDING;
            uncompressed_payload_length += descr->page_length;
            descr_commit_idx_array[count] = Index;
            eligible_pages[count++] = descr;
        }
        rrdeng_page_descr_mutex_unlock(ctx, descr);

        if (page_write_pending) {
            ret = JudyLDel(&pg_cache->committed_page_index.JudyL_array, Index, PJE0);
            fatal_assert(1 == ret);
        }
    }
    uv_rwlock_wrunlock(&pg_cache->committed_page_index.lock);

    if (!count) {
        debug(D_RRDENGINE, "%s: no pages eligible for flushing.", __func__);
        if (completion)
            completion_mark_complete(completion);
        return 0;
    }
    wc->inflight_dirty_pages += count;

    xt_io_descr = mallocz(sizeof(*xt_io_descr));
    payload_offset = sizeof(*header) + count * sizeof(header->descr[0]);
    switch (compression_algorithm) {
    case RRD_NO_COMPRESSION:
        size_bytes = payload_offset + uncompressed_payload_length + sizeof(*trailer);
        break;
    default: /* Compress */
        fatal_assert(uncompressed_payload_length < LZ4_MAX_INPUT_SIZE);
        max_compressed_size = LZ4_compressBound(uncompressed_payload_length);
        compressed_buf = mallocz(max_compressed_size);
        size_bytes = payload_offset + MAX(uncompressed_payload_length, (unsigned)max_compressed_size) + sizeof(*trailer);
        break;
    }
    ret = posix_memalign((void *)&xt_io_descr->buf, RRDFILE_ALIGNMENT, ALIGN_BYTES_CEILING(size_bytes));
    if (unlikely(ret)) {
        fatal("posix_memalign:%s", strerror(ret));
        /* freez(xt_io_descr);*/
    }
    memset(xt_io_descr->buf, 0, ALIGN_BYTES_CEILING(size_bytes));
    (void) memcpy(xt_io_descr->descr_array, eligible_pages, sizeof(struct rrdeng_page_descr *) * count);
    xt_io_descr->descr_count = count;

    pos = 0;
    header = xt_io_descr->buf;
    header->compression_algorithm = compression_algorithm;
    header->number_of_pages = count;
    pos += sizeof(*header);

    extent = mallocz(sizeof(*extent) + count * sizeof(extent->pages[0]));
    datafile = ctx->datafiles.last;  /* TODO: check for exceeded size quota */
    extent->offset = datafile->pos;
    extent->number_of_pages = count;
    extent->datafile = datafile;
    extent->next = NULL;

    for (i = 0 ; i < count ; ++i) {
        /* This is here for performance reasons */
        xt_io_descr->descr_commit_idx_array[i] = descr_commit_idx_array[i];

        descr = xt_io_descr->descr_array[i];
        header->descr[i].type = descr->type;
        uuid_copy(*(uuid_t *)header->descr[i].uuid, *descr->id);
        header->descr[i].page_length = descr->page_length;
        header->descr[i].start_time_ut = descr->start_time_ut;
        header->descr[i].end_time_ut = descr->end_time_ut;
        pos += sizeof(header->descr[i]);
    }
    for (i = 0 ; i < count ; ++i) {
        descr = xt_io_descr->descr_array[i];
        /* care, we don't hold the descriptor mutex */
        (void) memcpy(xt_io_descr->buf + pos, descr->pg_cache_descr->page, descr->page_length);
        descr->extent = extent;
        extent->pages[i] = descr;

        pos += descr->page_length;
    }
    df_extent_insert(extent);

    switch (compression_algorithm) {
    case RRD_NO_COMPRESSION:
        header->payload_length = uncompressed_payload_length;
        break;
    default: /* Compress */
        compressed_size = LZ4_compress_default(xt_io_descr->buf + payload_offset, compressed_buf,
                                               uncompressed_payload_length, max_compressed_size);
        ctx->stats.before_compress_bytes += uncompressed_payload_length;
        ctx->stats.after_compress_bytes += compressed_size;
        debug(D_RRDENGINE, "LZ4 compressed %"PRIu32" bytes to %d bytes.", uncompressed_payload_length, compressed_size);
        (void) memcpy(xt_io_descr->buf + payload_offset, compressed_buf, compressed_size);
        freez(compressed_buf);
        size_bytes = payload_offset + compressed_size + sizeof(*trailer);
        header->payload_length = compressed_size;
        break;
    }
    extent->size = size_bytes;
    xt_io_descr->bytes = size_bytes;
    xt_io_descr->pos = datafile->pos;
    xt_io_descr->req.data = xt_io_descr;
    xt_io_descr->completion = completion;

    trailer = xt_io_descr->buf + size_bytes - sizeof(*trailer);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, xt_io_descr->buf, size_bytes - sizeof(*trailer));
    crc32set(trailer->checksum, crc);

    real_io_size = ALIGN_BYTES_CEILING(size_bytes);
    xt_io_descr->iov = uv_buf_init((void *)xt_io_descr->buf, real_io_size);
    ret = uv_fs_write(wc->loop, &xt_io_descr->req, datafile->file, &xt_io_descr->iov, 1, datafile->pos, flush_pages_cb);
    fatal_assert(-1 != ret);
    ctx->stats.io_write_bytes += real_io_size;
    ++ctx->stats.io_write_requests;
    ctx->stats.io_write_extent_bytes += real_io_size;
    ++ctx->stats.io_write_extents;
    do_commit_transaction(wc, STORE_DATA, xt_io_descr);
    datafile->pos += ALIGN_BYTES_CEILING(size_bytes);
    ctx->disk_space += ALIGN_BYTES_CEILING(size_bytes);
    rrdeng_test_quota(wc);

    return ALIGN_BYTES_CEILING(size_bytes);
}

static void after_delete_old_data(struct rrdengine_worker_config* wc)
{
    struct rrdengine_instance *ctx = wc->ctx;
    struct rrdengine_datafile *datafile;
    struct rrdengine_journalfile *journalfile;
    unsigned deleted_bytes, journalfile_bytes, datafile_bytes;
    int ret, error;
    char path[RRDENG_PATH_MAX];

    datafile = ctx->datafiles.first;
    journalfile = datafile->journalfile;
    datafile_bytes = datafile->pos;
    journalfile_bytes = journalfile->pos;
    deleted_bytes = journalfile->journal_data_size;

    info("Deleting data and journal file pair.");
    datafile_list_delete(ctx, datafile);
    ret = destroy_journal_file(journalfile, datafile);
    if (!ret) {
        generate_journalfilepath(datafile, path, sizeof(path));
        info("Deleted journal file \"%s\".", path);
        deleted_bytes += journalfile_bytes;
    }
    ret = destroy_data_file(datafile);
    if (!ret) {
        generate_datafilepath(datafile, path, sizeof(path));
        info("Deleted data file \"%s\".", path);
        deleted_bytes += datafile_bytes;
    }
    freez(journalfile);
    freez(datafile);

    ctx->disk_space -= deleted_bytes;
    info("Reclaimed %u bytes of disk space.", deleted_bytes);

    error = uv_thread_join(wc->now_deleting_files);
    if (error) {
        error("uv_thread_join(): %s", uv_strerror(error));
    }
    freez(wc->now_deleting_files);
    /* unfreeze command processing */
    wc->now_deleting_files = NULL;

    wc->cleanup_thread_deleting_files = 0;
    rrdcontext_db_rotation();

    /* interrupt event loop */
    uv_stop(wc->loop);
}

static void delete_old_data(void *arg)
{
    struct rrdengine_instance *ctx = arg;
    struct rrdengine_worker_config* wc = &ctx->worker_config;
    struct rrdengine_datafile *datafile;
    struct extent_info *extent, *next;
    struct rrdeng_page_descr *descr;
    unsigned count, i;
    uint8_t can_delete_metric;
    uuid_t metric_id;

    /* Safe to use since it will be deleted after we are done */
    datafile = ctx->datafiles.first;

    for (extent = datafile->extents.first ; extent != NULL ; extent = next) {
        count = extent->number_of_pages;
        for (i = 0 ; i < count ; ++i) {
            descr = extent->pages[i];
            can_delete_metric = pg_cache_punch_hole(ctx, descr, 0, 0, &metric_id, true);
            if (unlikely(can_delete_metric)) {
                /*
                 * If the metric is empty, has no active writers and if the metadata log has been initialized then
                 * attempt to delete the corresponding netdata dimension.
                 */
                metaqueue_delete_dimension_uuid(&metric_id);
            }
        }
        next = extent->next;
        freez(extent);
    }
    wc->cleanup_thread_deleting_files = 1;
    /* wake up event loop */
    fatal_assert(0 == uv_async_send(&wc->async));
}

void rrdeng_test_quota(struct rrdengine_worker_config* wc)
{
    struct rrdengine_instance *ctx = wc->ctx;
    struct rrdengine_datafile *datafile;
    unsigned current_size, target_size;
    uint8_t out_of_space, only_one_datafile;
    int ret, error;

    out_of_space = 0;
    /* Do not allow the pinned pages to exceed the disk space quota to avoid deadlocks */
    if (unlikely(ctx->disk_space > MAX(ctx->max_disk_space, 2 * ctx->metric_API_max_producers * RRDENG_BLOCK_SIZE))) {
        out_of_space = 1;
    }
    datafile = ctx->datafiles.last;
    current_size = datafile->pos;
    target_size = ctx->max_disk_space / TARGET_DATAFILES;
    target_size = MIN(target_size, MAX_DATAFILE_SIZE);
    target_size = MAX(target_size, MIN_DATAFILE_SIZE);
    only_one_datafile = (datafile == ctx->datafiles.first) ? 1 : 0;
    if (unlikely(current_size >= target_size || (out_of_space && only_one_datafile))) {
        /* Finalize data and journal file and create a new pair */
        wal_flush_transaction_buffer(wc);
        ret = create_new_datafile_pair(ctx, 1, ctx->last_fileno + 1);
        if (likely(!ret)) {
            ++ctx->last_fileno;
        }
    }
    if (unlikely(out_of_space && NO_QUIESCE == ctx->quiesce)) {
        /* delete old data */
        if (wc->now_deleting_files) {
            /* already deleting data */
            return;
        }
        if (NULL == ctx->datafiles.first->next) {
            error("Cannot delete data file \"%s/"DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION"\""
                 " to reclaim space, there are no other file pairs left.",
                 ctx->dbfiles_path, ctx->datafiles.first->tier, ctx->datafiles.first->fileno);
            return;
        }
        info("Deleting data file \"%s/"DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION"\".",
             ctx->dbfiles_path, ctx->datafiles.first->tier, ctx->datafiles.first->fileno);
        wc->now_deleting_files = mallocz(sizeof(*wc->now_deleting_files));
        wc->cleanup_thread_deleting_files = 0;

        error = uv_thread_create(wc->now_deleting_files, delete_old_data, ctx);
        if (error) {
            error("uv_thread_create(): %s", uv_strerror(error));
            freez(wc->now_deleting_files);
            wc->now_deleting_files = NULL;
        }
    }
}

static inline int rrdeng_threads_alive(struct rrdengine_worker_config* wc)
{
    if (wc->now_invalidating_dirty_pages || wc->now_deleting_files) {
        return 1;
    }
    return 0;
}

static void rrdeng_cleanup_finished_threads(struct rrdengine_worker_config* wc)
{
    struct rrdengine_instance *ctx = wc->ctx;

    if (unlikely(wc->cleanup_thread_invalidating_dirty_pages)) {
        after_invalidate_oldest_committed(wc);
    }
    if (unlikely(wc->cleanup_thread_deleting_files)) {
        after_delete_old_data(wc);
    }
    if (unlikely(SET_QUIESCE == ctx->quiesce && !rrdeng_threads_alive(wc))) {
        ctx->quiesce = QUIESCED;
        completion_mark_complete(&ctx->rrdengine_completion);
    }
}

/* return 0 on success */
int init_rrd_files(struct rrdengine_instance *ctx)
{
    int ret = init_data_files(ctx);

    BUFFER *wb = buffer_create(1000);
    size_t all_errors = 0;
    usec_t now = now_realtime_usec();

    if(ctx->load_errors[LOAD_ERRORS_PAGE_FLIPPED_TIME].counter) {
        buffer_sprintf(wb, "%s%zu pages had start time > end time (latest: %llu secs ago)"
                       , (all_errors)?", ":""
                       , ctx->load_errors[LOAD_ERRORS_PAGE_FLIPPED_TIME].counter
                       , (now - ctx->load_errors[LOAD_ERRORS_PAGE_FLIPPED_TIME].latest_end_time_ut) / USEC_PER_SEC
                       );
        all_errors += ctx->load_errors[LOAD_ERRORS_PAGE_FLIPPED_TIME].counter;
    }

    if(ctx->load_errors[LOAD_ERRORS_PAGE_EQUAL_TIME].counter) {
        buffer_sprintf(wb, "%s%zu pages had start time = end time with more than 1 entries (latest: %llu secs ago)"
                       , (all_errors)?", ":""
                       , ctx->load_errors[LOAD_ERRORS_PAGE_EQUAL_TIME].counter
                       , (now - ctx->load_errors[LOAD_ERRORS_PAGE_EQUAL_TIME].latest_end_time_ut) / USEC_PER_SEC
        );
        all_errors += ctx->load_errors[LOAD_ERRORS_PAGE_EQUAL_TIME].counter;
    }

    if(ctx->load_errors[LOAD_ERRORS_PAGE_ZERO_ENTRIES].counter) {
        buffer_sprintf(wb, "%s%zu pages had zero points (latest: %llu secs ago)"
                       , (all_errors)?", ":""
                       , ctx->load_errors[LOAD_ERRORS_PAGE_ZERO_ENTRIES].counter
                       , (now - ctx->load_errors[LOAD_ERRORS_PAGE_ZERO_ENTRIES].latest_end_time_ut) / USEC_PER_SEC
        );
        all_errors += ctx->load_errors[LOAD_ERRORS_PAGE_ZERO_ENTRIES].counter;
    }

    if(ctx->load_errors[LOAD_ERRORS_PAGE_UPDATE_ZERO].counter) {
        buffer_sprintf(wb, "%s%zu pages had update every == 0 with entries > 1 (latest: %llu secs ago)"
                       , (all_errors)?", ":""
                       , ctx->load_errors[LOAD_ERRORS_PAGE_UPDATE_ZERO].counter
                       , (now - ctx->load_errors[LOAD_ERRORS_PAGE_UPDATE_ZERO].latest_end_time_ut) / USEC_PER_SEC
        );
        all_errors += ctx->load_errors[LOAD_ERRORS_PAGE_UPDATE_ZERO].counter;
    }

    if(ctx->load_errors[LOAD_ERRORS_PAGE_FLEXY_TIME].counter) {
        buffer_sprintf(wb, "%s%zu pages had a different number of points compared to their timestamps (latest: %llu secs ago; these page have been loaded)"
                       , (all_errors)?", ":""
                       , ctx->load_errors[LOAD_ERRORS_PAGE_FLEXY_TIME].counter
                       , (now - ctx->load_errors[LOAD_ERRORS_PAGE_FLEXY_TIME].latest_end_time_ut) / USEC_PER_SEC
        );
        all_errors += ctx->load_errors[LOAD_ERRORS_PAGE_FLEXY_TIME].counter;
    }

    if(ctx->load_errors[LOAD_ERRORS_DROPPED_EXTENT].counter) {
        buffer_sprintf(wb, "%s%zu extents have been dropped because they didn't have any valid pages"
                       , (all_errors)?", ":""
                       , ctx->load_errors[LOAD_ERRORS_DROPPED_EXTENT].counter
        );
        all_errors += ctx->load_errors[LOAD_ERRORS_DROPPED_EXTENT].counter;
    }

    if(all_errors)
        info("DBENGINE: tier %d: %s", ctx->tier, buffer_tostring(wb));

    buffer_free(wb);
    return ret;
}

void finalize_rrd_files(struct rrdengine_instance *ctx)
{
    return finalize_data_files(ctx);
}

void rrdeng_init_cmd_queue(struct rrdengine_worker_config* wc)
{
    wc->cmd_queue.head = wc->cmd_queue.tail = 0;
    wc->queue_size = 0;
    fatal_assert(0 == uv_cond_init(&wc->cmd_cond));
    fatal_assert(0 == uv_mutex_init(&wc->cmd_mutex));
}

void rrdeng_enq_cmd(struct rrdengine_worker_config* wc, struct rrdeng_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&wc->cmd_mutex);
    while ((queue_size = wc->queue_size) == RRDENG_CMD_Q_MAX_SIZE) {
        uv_cond_wait(&wc->cmd_cond, &wc->cmd_mutex);
    }
    fatal_assert(queue_size < RRDENG_CMD_Q_MAX_SIZE);
    /* enqueue command */
    wc->cmd_queue.cmd_array[wc->cmd_queue.tail] = *cmd;
    wc->cmd_queue.tail = wc->cmd_queue.tail != RRDENG_CMD_Q_MAX_SIZE - 1 ?
                         wc->cmd_queue.tail + 1 : 0;
    wc->queue_size = queue_size + 1;
    uv_mutex_unlock(&wc->cmd_mutex);

    /* wake up event loop */
    fatal_assert(0 == uv_async_send(&wc->async));
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

static void load_configuration_dynamic(void)
{
    unsigned read_num = (unsigned)config_get_number(CONFIG_SECTION_DB, "dbengine pages per extent", MAX_PAGES_PER_EXTENT);
    if (read_num > 0 && read_num <= MAX_PAGES_PER_EXTENT)
        rrdeng_pages_per_extent = read_num;
    else {
        error("Invalid dbengine pages per extent %u given. Using %u.", read_num, rrdeng_pages_per_extent);
        config_set_number(CONFIG_SECTION_DB, "dbengine pages per extent", rrdeng_pages_per_extent);
    }
}

void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    debug(D_RRDENGINE, "%s called, active=%d.", __func__, uv_is_active((uv_handle_t *)handle));
}

/* Flushes dirty pages when timer expires */
#define TIMER_PERIOD_MS (1000)

void timer_cb(uv_timer_t* handle)
{
//    static time_t next_journal_indexing_check =0;
    static time_t next_descriptor_cleanup =0;

    worker_is_busy(RRDENG_MAX_OPCODE + 1);

    struct rrdengine_worker_config* wc = handle->data;
    struct rrdengine_instance *ctx = wc->ctx;

    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    rrdeng_test_quota(wc);
    debug(D_RRDENGINE, "%s: timeout reached.", __func__);
    if (likely(!wc->now_deleting_files && !wc->now_invalidating_dirty_pages)) {
        /* There is free space so we can write to disk and we are not actively deleting dirty buffers */
        struct page_cache *pg_cache = &ctx->pg_cache;
        unsigned long total_bytes, bytes_written, nr_committed_pages, bytes_to_write = 0, producers, low_watermark,
                      high_watermark;

        uv_rwlock_rdlock(&pg_cache->committed_page_index.lock);
        nr_committed_pages = pg_cache->committed_page_index.nr_committed_pages;
        uv_rwlock_rdunlock(&pg_cache->committed_page_index.lock);
        producers = ctx->metric_API_max_producers;
        /* are flushable pages more than 25% of the maximum page cache size */
        high_watermark = (ctx->max_cache_pages * 25LLU) / 100;
        low_watermark = (ctx->max_cache_pages * 5LLU) / 100; /* 5%, must be smaller than high_watermark */

        /* Flush more pages only if disk can keep up */
        if (wc->inflight_dirty_pages < high_watermark + producers) {
            if (nr_committed_pages > producers &&
                /* committed to be written pages are more than the produced number */
                nr_committed_pages - producers > high_watermark) {
                /* Flushing speed must increase to stop page cache from filling with dirty pages */
                bytes_to_write = (nr_committed_pages - producers - low_watermark) * RRDENG_BLOCK_SIZE;
            }
            bytes_to_write = MAX(DATAFILE_IDEAL_IO_SIZE, bytes_to_write);

            debug(D_RRDENGINE, "Flushing pages to disk.");
            for (total_bytes = bytes_written = do_flush_pages(wc, 0, NULL);
                 bytes_written && (total_bytes < bytes_to_write);
                 total_bytes += bytes_written) {
                bytes_written = do_flush_pages(wc, 0, NULL);
            }
        }
    }
    load_configuration_dynamic();
#ifdef NETDATA_INTERNAL_CHECKS
    {
        char buf[4096];
        debug(D_RRDENGINE, "%s", get_rrdeng_statistics(wc->ctx, buf, sizeof(buf)));
    }
#endif

//    if (unlikely(!next_journal_indexing_check))
//        next_journal_indexing_check = now_realtime_sec() + 60;
//
//    if (next_journal_indexing_check < now_realtime_sec()) {
//        next_journal_indexing_check = now_realtime_sec() + 60;
//        if (!wc->running_journal_migration && ctx->quiesce == NO_QUIESCE) {
//            internal_error(true, "Checking for journal files that need indexing");
//
//            struct rrdeng_work *work_request;
//            work_request = mallocz(sizeof(*work_request));
//            work_request->req.data = work_request;
//            work_request->wc = wc;
//            wc->running_journal_migration = 1;
//            if (unlikely(
//                    uv_queue_work(wc->loop, &work_request->req, start_journal_indexing, after_journal_indexing))) {
//                freez(work_request);
//                wc->running_journal_migration = 0;
//            }
//        }
//    }

    if (next_descriptor_cleanup < now_realtime_sec()) {
        next_descriptor_cleanup = now_realtime_sec() + 30;

        struct rrdeng_cmd cmd;
        cmd.opcode = RRDENG_DELETE_DESCRIPTORS;
        rrdeng_enq_cmd(&ctx->worker_config, &cmd);
    }
    worker_is_idle();
}

#define MAX_CMD_BATCH_SIZE (256)

void rrdeng_worker(void* arg)
{
    worker_register("DBENGINE");
    worker_register_job_name(RRDENG_NOOP,                          "noop");
    worker_register_job_name(RRDENG_READ_PAGE,                     "page read");
    worker_register_job_name(RRDENG_READ_EXTENT,                   "extent read");
    worker_register_job_name(RRDENG_COMMIT_PAGE,                   "commit");
    worker_register_job_name(RRDENG_FLUSH_PAGES,                   "flush");
    worker_register_job_name(RRDENG_SHUTDOWN,                      "shutdown");
    worker_register_job_name(RRDENG_INVALIDATE_OLDEST_MEMORY_PAGE, "page lru");
    worker_register_job_name(RRDENG_QUIESCE,                       "quiesce");
    worker_register_job_name(RRDENG_MAX_OPCODE,                    "cleanup");
    worker_register_job_name(RRDENG_MAX_OPCODE + 1,                "timer");

    struct rrdengine_worker_config* wc = arg;
    struct rrdengine_instance *ctx = wc->ctx;
    uv_loop_t* loop;
    int shutdown, ret;
    enum rrdeng_opcode opcode;
    uv_timer_t timer_req;
    struct rrdeng_cmd cmd;
    unsigned cmd_batch_size;

    rrdeng_init_cmd_queue(wc);

    loop = wc->loop = mallocz(sizeof(uv_loop_t));
    ret = uv_loop_init(loop);
    if (ret) {
        error("uv_loop_init(): %s", uv_strerror(ret));
        goto error_after_loop_init;
    }
    loop->data = wc;

    ret = uv_async_init(wc->loop, &wc->async, async_cb);
    if (ret) {
        error("uv_async_init(): %s", uv_strerror(ret));
        goto error_after_async_init;
    }
    wc->async.data = wc;

    wc->now_deleting_files = NULL;
    wc->cleanup_thread_deleting_files = 0;

    wc->now_invalidating_dirty_pages = NULL;
    wc->cleanup_thread_invalidating_dirty_pages = 0;
    wc->inflight_dirty_pages = 0;

    /* dirty page flushing timer */
    ret = uv_timer_init(loop, &timer_req);
    if (ret) {
        error("uv_timer_init(): %s", uv_strerror(ret));
        goto error_after_timer_init;
    }
    timer_req.data = wc;

    wc->error = 0;
    /* wake up initialization thread */
    completion_mark_complete(&ctx->rrdengine_completion);

    fatal_assert(0 == uv_timer_start(&timer_req, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));
    shutdown = 0;
    int set_name = 0;
    while (likely(shutdown == 0 || rrdeng_threads_alive(wc))) {
        worker_is_idle();
        uv_run(loop, UV_RUN_DEFAULT);
        worker_is_busy(RRDENG_MAX_OPCODE);
        rrdeng_cleanup_finished_threads(wc);

        /* wait for commands */
        cmd_batch_size = 0;
        do {
            /*
             * Avoid starving the loop when there are too many commands coming in.
             * timer_cb will interrupt the loop again to allow serving more commands.
             */
            if (unlikely(cmd_batch_size >= MAX_CMD_BATCH_SIZE))
                break;

            cmd = rrdeng_deq_cmd(wc);
            opcode = cmd.opcode;
            ++cmd_batch_size;

            if(likely(opcode != RRDENG_NOOP))
                worker_is_busy(opcode);

            switch (opcode) {
            case RRDENG_NOOP:
                /* the command queue was empty, do nothing */
                break;
            case RRDENG_SHUTDOWN:
                shutdown = 1;
                break;
            case RRDENG_QUIESCE:
                ctx->drop_metrics_under_page_cache_pressure = 0;
                ctx->quiesce = SET_QUIESCE;
                fatal_assert(0 == uv_timer_stop(&timer_req));
                uv_close((uv_handle_t *)&timer_req, NULL);
                while (do_flush_pages(wc, 1, NULL)) {
                    ; /* Force flushing of all committed pages. */
                }
                wal_flush_transaction_buffer(wc);
                if (!rrdeng_threads_alive(wc)) {
                    ctx->quiesce = QUIESCED;
                    completion_mark_complete(&ctx->rrdengine_completion);
                }
                break;
            case RRDENG_READ_PAGE:
                do_read_extent(wc, &cmd.read_page.page_cache_descr, 1, 0);
                break;
            case RRDENG_READ_EXTENT:
                do_read_extent(wc, cmd.read_extent.page_cache_descr, cmd.read_extent.page_count, 1);
                if (unlikely(!set_name)) {
                    set_name = 1;
                    uv_thread_set_name_np(ctx->worker_config.thread, "DBENGINE");
                }
                break;
            case RRDENG_COMMIT_PAGE:
                do_commit_transaction(wc, STORE_DATA, NULL);
                break;
            case RRDENG_FLUSH_PAGES: {
                if (wc->now_invalidating_dirty_pages) {
                    /* Do not flush if the disk cannot keep up */
                    completion_mark_complete(cmd.completion);
                } else {
                    (void)do_flush_pages(wc, 1, cmd.completion);
                }
                break;
            case RRDENG_INVALIDATE_OLDEST_MEMORY_PAGE:
                rrdeng_invalidate_oldest_committed(wc);
                break;
            }
            case RRDENG_DELETE_DESCRIPTORS:
                ;

                if (wc->delete_descriptors == 0) {
                    wc->delete_descriptors = 1;
                    struct rrdeng_work  *work_request;
                    work_request = mallocz(sizeof(*work_request));
                    work_request->req.data = work_request;
                    if (unlikely(
                            uv_queue_work(loop, &work_request->req,
                                          delete_descriptors, after_delete_descriptors))) {
                        freez(work_request);
                        wc->delete_descriptors = 0;
                    }
                }

                break;
            default:
                debug(D_RRDENGINE, "%s: default.", __func__);
                break;
            }
        } while (opcode != RRDENG_NOOP);
    }

    /* cleanup operations of the event loop */
    info("Shutting down RRD engine event loop for tier %d", ctx->tier);

    /*
     * uv_async_send after uv_close does not seem to crash in linux at the moment,
     * it is however undocumented behaviour and we need to be aware if this becomes
     * an issue in the future.
     */
    uv_close((uv_handle_t *)&wc->async, NULL);

    while (do_flush_pages(wc, 1, NULL)) {
        ; /* Force flushing of all committed pages. */
    }
    wal_flush_transaction_buffer(wc);
    uv_run(loop, UV_RUN_DEFAULT);

    info("Shutting down RRD engine event loop for tier %d complete", ctx->tier);
    /* TODO: don't let the API block by waiting to enqueue commands */
    uv_cond_destroy(&wc->cmd_cond);
/*  uv_mutex_destroy(&wc->cmd_mutex); */
    fatal_assert(0 == uv_loop_close(loop));
    freez(loop);

    worker_unregister();
    return;

error_after_timer_init:
    uv_close((uv_handle_t *)&wc->async, NULL);
error_after_async_init:
    fatal_assert(0 == uv_loop_close(loop));
error_after_loop_init:
    freez(loop);

    wc->error = UV_EAGAIN;
    /* wake up initialization thread */
    completion_mark_complete(&ctx->rrdengine_completion);
    worker_unregister();
}

/* C entry point for development purposes
 * make "LDFLAGS=-errdengine_main"
 */
void rrdengine_main(void)
{
    int ret;
    struct rrdengine_instance *ctx;

    sanity_check();
    ret = rrdeng_init(NULL, &ctx, "/tmp", RRDENG_MIN_PAGE_CACHE_SIZE_MB, RRDENG_MIN_DISK_SPACE_MB, 0);
    if (ret) {
        exit(ret);
    }
    rrdeng_exit(ctx);
    fprintf(stderr, "Hello world!");
    exit(0);
}
