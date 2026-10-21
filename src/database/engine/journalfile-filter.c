// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"

bool dbengine_journal_time_filters = true;

// ----------------------------------------------------------------------------
// keys

typedef struct {
    uint32_t block;     // which block of the filter
    uint32_t a;         // first bit position in the block
    uint32_t b;         // step between bit positions (odd, so the k positions are distinct)
} NJTF_KEY;

static inline uint64_t njtf_bucket(time_t t) {
    return (uint64_t)t / NJTF_BUCKET_S;
}

static inline NJTF_KEY njtf_key(const uint8_t *uuid, uint64_t bucket, uint64_t seed, uint32_t blocks) {
    uint8_t buf[sizeof(nd_uuid_t) + 4];
    memcpy(buf, uuid, sizeof(nd_uuid_t));

    // little-endian, so a filter means the same on every CPU
    uint32_t b32 = (uint32_t)bucket;
    buf[16] = (uint8_t)(b32);
    buf[17] = (uint8_t)(b32 >> 8);
    buf[18] = (uint8_t)(b32 >> 16);
    buf[19] = (uint8_t)(b32 >> 24);

    XXH128_hash_t h = XXH3_128bits_withSeed(buf, sizeof(buf), seed);

    return (NJTF_KEY) {
        .block = (uint32_t)(((h.low64 >> 32) * (uint64_t)blocks) >> 32),
        .a = (uint32_t)(h.high64 % NJTF_BLOCK_BITS),
        .b = (uint32_t)((h.high64 >> 9) % NJTF_BLOCK_BITS) | 1,
    };
}

static inline void njtf_key_set(uint8_t *payload, NJTF_KEY key) {
    uint8_t *block = payload + (size_t)key.block * NJTF_BLOCK_BYTES;
    for(uint32_t i = 0; i < NJTF_K; i++) {
        uint32_t pos = (key.a + i * key.b) % NJTF_BLOCK_BITS;
        block[pos >> 3] |= (uint8_t)(1u << (pos & 7));
    }
}

static inline bool njtf_key_test(const uint8_t *payload, NJTF_KEY key) {
    const uint8_t *block = payload + (size_t)key.block * NJTF_BLOCK_BYTES;
    for(uint32_t i = 0; i < NJTF_K; i++) {
        uint32_t pos = (key.a + i * key.b) % NJTF_BLOCK_BITS;
        if(!(block[pos >> 3] & (uint8_t)(1u << (pos & 7))))
            return false;
    }
    return true;
}

// ----------------------------------------------------------------------------
// paths

void njtf_generate_path(struct rrdengine_datafile *datafile, char *str, size_t maxlen, bool tmp) {
    (void) snprintfz(str, maxlen, "%s/" WALFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL "%s",
                     datafile_ctx(datafile)->config.dbfiles_path, datafile->tier, datafile->fileno,
                     tmp ? NJTF_TMP_EXTENSION : NJTF_EXTENSION);
}

// Charge the tier with the size of the journal's filter file on disk (0 = no file): the tier's disk space includes
// its filters, so that retention keeps the tier within its configured size.
static void njtf_charge(struct rrdengine_journalfile *journalfile, uint64_t bytes) {
    uint64_t old = __atomic_exchange_n(&journalfile->njtf.disk_bytes, bytes, __ATOMIC_RELAXED);
    struct rrdengine_instance *ctx = datafile_ctx(journalfile->datafile);
    if(bytes > old)
        ctx_current_disk_space_increase(ctx, bytes - old);
    else if(old > bytes)
        ctx_current_disk_space_decrease(ctx, old - bytes);
}

void njtf_charge_existing(struct rrdengine_datafile *datafile) {
    char path[RRDENG_PATH_MAX];
    njtf_generate_path(datafile, path, sizeof(path), false);

    struct stat st;
    if(stat(path, &st) == 0 && S_ISREG(st.st_mode))
        njtf_charge(datafile->journalfile, (uint64_t)st.st_size);
}

static uint32_t njtf_journal_header_crc(const uint8_t *jv2, size_t jv2_size) {
    const struct journal_v2_block_trailer *trailer =
        (const struct journal_v2_block_trailer *)(jv2 + jv2_size - sizeof(struct journal_v2_block_trailer));
    uint32_t crc;
    memcpy(&crc, trailer->checksum, sizeof(crc));
    return crc;
}

static uint32_t njtf_header_crc(const struct njtf_header *h) {
    uLong crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, (const Bytef *)h, offsetof(struct njtf_header, header_crc));
    return (uint32_t)crc;
}

// ----------------------------------------------------------------------------
// building

// The hours (buckets) a metric's pages touch in a journal, as sorted, non-overlapping, non-adjacent ranges: every
// (uuid, bucket) key once, however many pages of the metric fall in the same hour.
typedef struct {
    uint64_t first;
    uint64_t last;
} NJTF_RANGE;

static int njtf_range_compare(const void *a, const void *b) {
    const NJTF_RANGE *ra = a, *rb = b;
    return (ra->first > rb->first) - (ra->first < rb->first);
}

// sort and merge ranges in place; returns how many remain
static size_t njtf_ranges_merge(NJTF_RANGE *ranges, size_t count) {
    bool sorted = true;
    for(size_t i = 1; i < count && sorted; i++)
        sorted = ranges[i - 1].first <= ranges[i].first;

    if(!sorted)
        qsort(ranges, count, sizeof(*ranges), njtf_range_compare);

    size_t merged = 0;
    for(size_t i = 0; i < count; i++) {
        if(merged && ranges[i].first <= ranges[merged - 1].last + 1)
            ranges[merged - 1].last = MAX(ranges[merged - 1].last, ranges[i].last);
        else
            ranges[merged++] = ranges[i];
    }
    return merged;
}

// Walk every metric of a journal v2 file and call cb(uuid, ranges, count) with the hours its pages touch, from the
// times the planner reads back: the journal start plus the stored deltas. Returns false if the journal is malformed,
// or a page cannot be keyed (negative time, or spanning more than NJTF_MAX_PAGE_BUCKETS hours).
typedef bool (*njtf_metric_cb)(const uint8_t *uuid, const NJTF_RANGE *ranges, size_t count, void *data);

static bool njtf_walk_metrics(const uint8_t *jv2, size_t jv2_size, njtf_metric_cb cb, void *data) {
    if(jv2_size < sizeof(struct journal_v2_header) + sizeof(struct journal_v2_block_trailer))
        return false;

    const struct journal_v2_header *j2_header = (const struct journal_v2_header *)jv2;
    if(j2_header->magic != JOURVAL_V2_MAGIC || j2_header->journal_v2_file_size != jv2_size)
        return false;

    size_t metric_count = j2_header->metric_count;
    size_t metric_offset = j2_header->metric_offset;
    size_t metric_list_size;
    if(__builtin_mul_overflow(metric_count, sizeof(struct journal_metric_list), &metric_list_size) ||
       metric_offset > jv2_size || metric_list_size > jv2_size - metric_offset)
        return false;

    time_t journal_start_s = (time_t)(j2_header->start_time_ut / USEC_PER_SEC);
    const struct journal_metric_list *metrics = (const struct journal_metric_list *)(jv2 + metric_offset);

    NJTF_RANGE *ranges = NULL;
    size_t ranges_size = 0;
    bool ok = true;

    for(size_t i = 0; ok && i < metric_count; i++) {
        size_t page_offset = metrics[i].page_offset;
        if(page_offset > jv2_size || jv2_size - page_offset < sizeof(struct journal_page_header)) {
            ok = false;
            break;
        }

        const struct journal_page_header *ph = (const struct journal_page_header *)(jv2 + page_offset);
        size_t entries = ph->entries;
        size_t room = jv2_size - page_offset - sizeof(struct journal_page_header);
        if(entries > room / sizeof(struct journal_page_list)) {
            ok = false;
            break;
        }

        const struct journal_page_list *pages =
            (const struct journal_page_list *)((const uint8_t *)ph + sizeof(struct journal_page_header));

        if(entries > ranges_size) {
            ranges_size = MAX(entries, ranges_size * 2);
            ranges = reallocz(ranges, ranges_size * sizeof(*ranges));
        }

        for(size_t p = 0; p < entries; p++) {
            time_t first_s = journal_start_s + pages[p].delta_start_s;
            time_t last_s = journal_start_s + pages[p].delta_end_s;
            time_t lo = MIN(first_s, last_s), hi = MAX(first_s, last_s);
            if(lo < 0 || njtf_bucket(hi) - njtf_bucket(lo) + 1 > NJTF_MAX_PAGE_BUCKETS) {
                ok = false;
                break;
            }

            ranges[p] = (NJTF_RANGE){ .first = njtf_bucket(lo), .last = njtf_bucket(hi) };
        }

        if(ok && entries)
            ok = cb(metrics[i].uuid, ranges, njtf_ranges_merge(ranges, entries), data);
    }

    freez(ranges);
    return ok;
}

struct njtf_build_state {
    uint64_t keys;
    uint8_t *payload;
    uint32_t blocks;
    uint64_t seed;
};

static bool njtf_count_metric(const uint8_t *uuid __maybe_unused, const NJTF_RANGE *ranges, size_t count, void *data) {
    struct njtf_build_state *bs = data;
    for(size_t r = 0; r < count; r++)
        bs->keys += ranges[r].last - ranges[r].first + 1;
    return true;
}

static bool njtf_set_metric(const uint8_t *uuid, const NJTF_RANGE *ranges, size_t count, void *data) {
    struct njtf_build_state *bs = data;
    for(size_t r = 0; r < count; r++) {
        for(uint64_t b = ranges[r].first; b <= ranges[r].last; b++)
            njtf_key_set(bs->payload, njtf_key(uuid, b, bs->seed, bs->blocks));
    }
    return true;
}

// build the payload of a journal's filter; the caller frees *payload
static bool njtf_build_payload(const uint8_t *jv2, size_t jv2_size, uint64_t seed,
                               uint8_t **payload, uint32_t *blocks, uint64_t *keys) {
    struct njtf_build_state bs = { .seed = seed };

    // sized by distinct keys: NJTF_BITS_PER_KEY bits for each
    if(!njtf_walk_metrics(jv2, jv2_size, njtf_count_metric, &bs))
        return false;

    uint64_t bits = bs.keys * NJTF_BITS_PER_KEY;
    uint64_t nblocks = (bits + NJTF_BLOCK_BITS - 1) / NJTF_BLOCK_BITS;
    if(!nblocks)
        nblocks = 1;
    if(nblocks > UINT32_MAX / NJTF_BLOCK_BYTES)
        return false;

    bs.blocks = (uint32_t)nblocks;
    bs.payload = callocz(nblocks, NJTF_BLOCK_BYTES);

    if(!njtf_walk_metrics(jv2, jv2_size, njtf_set_metric, &bs)) {
        freez(bs.payload);
        return false;
    }

    *payload = bs.payload;
    *blocks = bs.blocks;
    *keys = bs.keys;
    return true;
}

static bool njtf_write_all(int fd, const void *buf, size_t len) {
    const uint8_t *p = buf;
    while(len) {
        ssize_t w = write(fd, p, len);
        if(w < 0) {
            if(errno == EINTR)
                continue;
            return false;
        }
        p += w;
        len -= (size_t)w;
    }
    return true;
}

bool njtf_build(struct rrdengine_datafile *datafile, const uint8_t *jv2, size_t jv2_size) {
    uint64_t seed = 0x9E3779B97F4A7C15ULL ^ datafile->fileno;
    uint8_t *payload = NULL;
    uint32_t blocks = 0;
    uint64_t keys = 0;

    if(!njtf_build_payload(jv2, jv2_size, seed, &payload, &blocks, &keys)) {
        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_NOTICE,
                     "DBENGINE: tier %d: journal v2 of " DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL
                     " cannot have a time filter (malformed, or a page spans too many hours); it will be searched",
                     datafile_ctx(datafile)->config.tier, datafile->tier, datafile->fileno);
        return false;
    }

    size_t payload_size = (size_t)blocks * NJTF_BLOCK_BYTES;

    uint8_t header_block[NJTF_HEADER_SIZE] = { 0 };
    struct njtf_header h = {
        .magic = NJTF_MAGIC,
        .version = NJTF_VERSION,
        .tier = datafile->tier,
        .fileno = datafile->fileno,
        .journal_v2_file_size = (uint32_t)jv2_size,
        .journal_v2_header_crc = njtf_journal_header_crc(jv2, jv2_size),
        .bucket_s = NJTF_BUCKET_S,
        .bits_per_key = NJTF_BITS_PER_KEY,
        .k = NJTF_K,
        .blocks = blocks,
        .seed = seed,
        .keys = keys,
    };
    uLong crc = crc32(0L, Z_NULL, 0);
    h.payload_crc = (uint32_t)crc32(crc, payload, payload_size);
    h.header_crc = njtf_header_crc(&h);
    memcpy(header_block, &h, sizeof(h));

    char path[RRDENG_PATH_MAX], path_tmp[RRDENG_PATH_MAX];
    njtf_generate_path(datafile, path, sizeof(path), false);
    njtf_generate_path(datafile, path_tmp, sizeof(path_tmp), true);

    bool ok = false;
    int fd = open(path_tmp, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 0664);
    if(fd != -1) {
        ok = njtf_write_all(fd, header_block, sizeof(header_block)) &&
             njtf_write_all(fd, payload, payload_size) &&
             fsync(fd) == 0;
        close(fd);

        if(ok && rename(path_tmp, path) != 0)
            ok = false;

        if(ok)
            njtf_charge(datafile->journalfile, sizeof(header_block) + payload_size);

        if(!ok)
            unlink(path_tmp);
    }

    if(!ok)
        nd_log(NDLS_DAEMON, NDLP_ERR, "DBENGINE: cannot write journal time filter \"%s\"", path);

    freez(payload);
    return ok;
}

// ----------------------------------------------------------------------------
// reading

// Does a filter header (of a file of file_size bytes) belong to this journal, with the geometry this code reads?
static bool njtf_header_matches(const struct njtf_header *h, size_t file_size, unsigned tier, unsigned fileno,
                                const uint8_t *jv2, size_t jv2_size) {
    return file_size >= NJTF_HEADER_SIZE &&
           h->magic == NJTF_MAGIC && h->version == NJTF_VERSION && h->header_crc == njtf_header_crc(h) &&
           h->tier == tier && h->fileno == fileno &&
           h->journal_v2_file_size == jv2_size &&
           h->journal_v2_header_crc == njtf_journal_header_crc(jv2, jv2_size) &&
           h->bucket_s == NJTF_BUCKET_S && h->k == NJTF_K && h->blocks &&
           file_size == NJTF_HEADER_SIZE + (size_t)h->blocks * NJTF_BLOCK_BYTES;
}

// Open a journal's filter file and read its header. Returns the open fd (and the header) when it belongs to the
// journal, -1 otherwise.
static int njtf_open_matching(struct rrdengine_datafile *datafile, const uint8_t *jv2, size_t jv2_size,
                              struct njtf_header *h, size_t *file_size) {
    char path[RRDENG_PATH_MAX];
    njtf_generate_path(datafile, path, sizeof(path), false);

    int fd = open(path, O_RDONLY | O_CLOEXEC);
    if(fd == -1)
        return -1;

    struct stat st;
    if(fstat(fd, &st) != 0 || (size_t)st.st_size < NJTF_HEADER_SIZE ||
       pread(fd, h, sizeof(*h), 0) != (ssize_t)sizeof(*h) ||
       !njtf_header_matches(h, (size_t)st.st_size, datafile->tier, datafile->fileno, jv2, jv2_size)) {
        close(fd);
        return -1;
    }

    *file_size = (size_t)st.st_size;
    return fd;
}

// map the filter and verify its payload; on success the journalfile holds the mapping
bool njtf_verify_and_map(struct rrdengine_journalfile *journalfile, const uint8_t *jv2, size_t jv2_size) {
    struct njtf_header h;
    size_t file_size = 0;
    int fd = njtf_open_matching(journalfile->datafile, jv2, jv2_size, &h, &file_size);
    if(fd == -1)
        return false;

    uint8_t *map = nd_mmap(NULL, file_size, PROT_READ, MAP_SHARED, fd, 0);
    close(fd);
    if(!map)
        return false;

    madvise_sequential(map, file_size);
    volatile uint32_t crc = 0;
    {
        char path[RRDENG_PATH_MAX];
        njtf_generate_path(journalfile->datafile, path, sizeof(path), false);
        PROTECTED_ACCESS_SETUP(map, file_size, path, "verify");
        if(no_signal_received)
            crc = (uint32_t)crc32(crc32(0L, Z_NULL, 0), map + NJTF_HEADER_SIZE, (uInt)(file_size - NJTF_HEADER_SIZE));
        else
            crc = ~h.payload_crc;   // unreadable: treat it as corrupted
    }
    if(crc != h.payload_crc) {
        nd_munmap(map, file_size);
        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                     "DBENGINE: tier %d: the journal time filter of " DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL
                     " failed its payload check; it will be rebuilt",
                     datafile_ctx(journalfile->datafile)->config.tier, journalfile->datafile->tier,
                     journalfile->datafile->fileno);
        return false;
    }
    madvise_random(map, file_size);

    journalfile->njtf.map = map;
    journalfile->njtf.map_size = file_size;
    njtf_charge(journalfile, file_size);
    return true;
}

NJTF_CHECK njtf_check(struct rrdengine_journalfile *journalfile, const nd_uuid_t *uuid, time_t after_s, time_t before_s) {
    if(!dbengine_journal_time_filters || after_s < 0 || before_s < after_s)
        return NJTF_CHECK_BYPASS;

    uint64_t first_bucket = njtf_bucket(after_s), last_bucket = njtf_bucket(before_s);
    if(last_bucket - first_bucket + 1 > NJTF_MAX_WINDOW_BUCKETS)
        return NJTF_CHECK_BYPASS;

    // queries never read or verify a filter: until the builder has verified and mapped it, search as today
    if(__atomic_load_n(&journalfile->njtf.state, __ATOMIC_ACQUIRE) != NJTF_STATE_VALID)
        return NJTF_CHECK_BYPASS;

    const uint8_t *map = journalfile->njtf.map;
    volatile NJTF_CHECK rc = NJTF_CHECK_SKIP;

    // an I/O error under the mapping (SIGBUS) must not crash the agent: search, and stop using this filter
    PROTECTED_ACCESS_SETUP((void *)map, journalfile->njtf.map_size, "journal time filter", "check");
    if(no_signal_received) {
        const struct njtf_header *h = (const struct njtf_header *)map;
        const uint8_t *payload = map + NJTF_HEADER_SIZE;

        for(uint64_t b = first_bucket; b <= last_bucket; b++) {
            if(njtf_key_test(payload, njtf_key(uuid[0], b, h->seed, h->blocks))) {
                rc = NJTF_CHECK_SEARCH;
                break;
            }
        }
    }
    else {
        // the mapping stays until the journal is closed: other queries may still be reading it
        __atomic_store_n(&journalfile->njtf.state, NJTF_STATE_FAILED, __ATOMIC_RELEASE);
        rc = NJTF_CHECK_BYPASS;
    }

    return rc;
}

void njtf_release(struct rrdengine_journalfile *journalfile) {
    if(journalfile->njtf.map) {
        nd_munmap(journalfile->njtf.map, journalfile->njtf.map_size);
        journalfile->njtf.map = NULL;
        journalfile->njtf.map_size = 0;
    }
    __atomic_store_n(&journalfile->njtf.state, NJTF_STATE_UNKNOWN, __ATOMIC_RELEASE);
}

void njtf_unlink(struct rrdengine_datafile *datafile) {
    char path[RRDENG_PATH_MAX];

    njtf_generate_path(datafile, path, sizeof(path), false);
    if(unlink(path) != 0 && errno != ENOENT)
        nd_log(NDLS_DAEMON, NDLP_ERR, "DBENGINE: cannot delete journal time filter \"%s\"", path);

    // uncharged even when it could not be deleted: the next startup removes it as an orphan
    njtf_charge(datafile->journalfile, 0);

    njtf_generate_path(datafile, path, sizeof(path), true);
    (void)unlink(path);
}

// ----------------------------------------------------------------------------
// background builder

static void njtf_remove_orphans(struct rrdengine_instance *ctx) {
    DIR *dir = opendir(ctx->config.dbfiles_path);
    if(!dir)
        return;

    struct dirent *de;
    while((de = readdir(dir))) {
        unsigned tier, fileno;
        char ext[32] = "";
        if(sscanf(de->d_name, WALFILE_PREFIX RRDENG_FILE_NUMBER_SCAN_TMPL "%31s", &tier, &fileno, ext) != 3)
            continue;

        bool tmp = strcmp(ext, NJTF_TMP_EXTENSION) == 0;
        if(!tmp && strcmp(ext, NJTF_EXTENSION) != 0)
            continue;

        // a .tmp file of an existing datafile may be in the middle of a build: the next build of it truncates it
        netdata_rwlock_rdlock(&ctx->datafiles.rwlock);
        bool orphan = JudyLGet(ctx->datafiles.JudyL, (Word_t)fileno, PJE0) == NULL;
        netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

        if(orphan) {
            char path[RRDENG_PATH_MAX];
            snprintfz(path, sizeof(path), "%s/%s", ctx->config.dbfiles_path, de->d_name);
            if(unlink(path) == 0)
                nd_log(NDLS_DAEMON, NDLP_INFO, "DBENGINE: tier %d: deleted orphan journal time filter \"%s\"",
                       ctx->config.tier, path);
        }
    }

    closedir(dir);
}

bool njtf_backfill_one(struct rrdengine_instance *ctx) {
    static bool orphans_removed = false;
    if(!orphans_removed) {
        njtf_remove_orphans(ctx);
        orphans_removed = true;
    }

    // find a journal whose filter is not verified yet, newest first: recent data is queried the most
    struct rrdengine_datafile *datafile = NULL;
    NJTF_STATE state = NJTF_STATE_UNKNOWN;

    netdata_rwlock_rdlock(&ctx->datafiles.rwlock);
    bool first = true;
    Word_t idx = (Word_t)-1;
    Pvoid_t *PValue;
    while((PValue = JudyLLastThenPrev(ctx->datafiles.JudyL, &idx, &first))) {
        struct rrdengine_datafile *df = *PValue;
        if(!df || !df->journalfile || !journalfile_v2_data_available(df->journalfile))
            continue;

        state = __atomic_load_n(&df->journalfile->njtf.state, __ATOMIC_ACQUIRE);
        if(state != NJTF_STATE_UNKNOWN && state != NJTF_STATE_MISSING)
            continue;

        if(!__atomic_compare_exchange_n(&df->journalfile->njtf.state, &state, NJTF_STATE_BUSY,
                                        false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED))
            continue;

        if(!datafile_acquire(df, DATAFILE_ACQUIRE_PAGE_DETAILS)) {
            __atomic_store_n(&df->journalfile->njtf.state, state, __ATOMIC_RELEASE);
            continue;
        }

        datafile = df;
        break;
    }
    netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if(!datafile)
        return false;

    struct rrdengine_journalfile *journalfile = datafile->journalfile;
    volatile NJTF_STATE result = state;     // the journal is not available right now: try again later

    char path[RRDENG_PATH_MAX];
    journalfile_v2_generate_path(datafile, path, sizeof(path));

    // an existing file that matches and verifies is used as is; verifying it reads only the journal's trailer,
    // so the journal is acquired for random access (no read-ahead)
    size_t jv2_size = 0;
    struct journal_v2_header *j2_header =
        journalfile_v2_data_acquire_with_hint(journalfile, &jv2_size, 0, 0, JOURNALFILE_V2_ACCESS_RANDOM);
    if(j2_header) {
        PROTECTED_ACCESS_SETUP(journalfile->mmap.data, journalfile->mmap.size, path, "verify filter");
        if(!no_signal_received)
            result = NJTF_STATE_FAILED;
        else if(njtf_verify_and_map(journalfile, (const uint8_t *)j2_header, jv2_size))
            result = NJTF_STATE_VALID;
        else
            result = NJTF_STATE_MISSING;
        journalfile_v2_data_release(journalfile);
    }

    if(result == NJTF_STATE_MISSING) {
        // building walks the metric list and every page list in order
        j2_header = journalfile_v2_data_acquire_with_hint(journalfile, &jv2_size, 0, 0,
                                                          JOURNALFILE_V2_ACCESS_SEQUENTIAL_DIRECTORY);
        if(j2_header) {
            PROTECTED_ACCESS_SETUP(journalfile->mmap.data, journalfile->mmap.size, path, "build filter");
            if(no_signal_received &&
               njtf_build(datafile, (const uint8_t *)j2_header, jv2_size) &&
               njtf_verify_and_map(journalfile, (const uint8_t *)j2_header, jv2_size))
                result = NJTF_STATE_VALID;
            else
                result = NJTF_STATE_FAILED;
            journalfile_v2_data_release(journalfile);

            if(result == NJTF_STATE_FAILED)
                njtf_unlink(datafile);  // a stale or broken file would only take disk space
        }
        else
            result = state;
    }

    __atomic_store_n(&journalfile->njtf.state, result, __ATOMIC_RELEASE);
    datafile_release(datafile, DATAFILE_ACQUIRE_PAGE_DETAILS);
    return true;
}

// ----------------------------------------------------------------------------
// verifier: every page of every journal v2 file in a directory must test positive for every hour it touches.
// netdata -W njtfverify, with NETDATA_NJTF_VERIFY_DIR=<tier-0 dbengine directory> (and optionally
// NETDATA_NJTF_VERIFY_MAX=<max journals>). Read-only.

struct njtf_verify_state {
    const uint8_t *payload;
    uint32_t blocks;
    uint64_t seed;
    uint64_t checked;
    uint64_t false_negatives;
};

static bool njtf_verify_metric(const uint8_t *uuid, const NJTF_RANGE *ranges, size_t count, void *data) {
    struct njtf_verify_state *vs = data;
    for(size_t r = 0; r < count; r++) {
        for(uint64_t b = ranges[r].first; b <= ranges[r].last; b++) {
            vs->checked++;
            if(!njtf_key_test(vs->payload, njtf_key(uuid, b, vs->seed, vs->blocks)))
                vs->false_negatives++;
        }
    }
    return true;
}

int njtf_verify_directory(void) {
    const char *dir_path = getenv("NETDATA_NJTF_VERIFY_DIR");
    const char *max_s = getenv("NETDATA_NJTF_VERIFY_MAX");
    size_t max = max_s ? strtoull(max_s, NULL, 10) : 0;
    if(!dir_path) {
        fprintf(stderr, "NJTF: set NETDATA_NJTF_VERIFY_DIR to a tier-0 dbengine directory\n");
        return 1;
    }

    DIR *dir = opendir(dir_path);
    if(!dir) {
        fprintf(stderr, "NJTF: cannot open %s\n", dir_path);
        return 1;
    }

    size_t journals = 0, with_filter = 0, identity_mismatch = 0, bad_payload = 0;
    uint64_t checked = 0, false_negatives = 0;
    struct dirent *de;
    while((de = readdir(dir)) && (!max || journals < max)) {
        size_t len = strlen(de->d_name);
        if(len < strlen(WALFILE_EXTENSION_V2) || strcmp(de->d_name + len - strlen(WALFILE_EXTENSION_V2), WALFILE_EXTENSION_V2) != 0)
            continue;

        char jpath[FILENAME_MAX + 1], fpath[FILENAME_MAX + 1];
        snprintfz(jpath, FILENAME_MAX, "%s/%s", dir_path, de->d_name);
        snprintfz(fpath, FILENAME_MAX, "%.*s%s", (int)(strlen(jpath) - strlen(WALFILE_EXTENSION_V2)), jpath, NJTF_EXTENSION);

        int jfd = open(jpath, O_RDONLY | O_CLOEXEC);
        if(jfd == -1)
            continue;
        struct stat jst;
        if(fstat(jfd, &jst) != 0 || jst.st_size < (off_t)NJTF_HEADER_SIZE) {
            close(jfd);
            continue;
        }
        size_t jsize = (size_t)jst.st_size;
        uint8_t *jv2 = nd_mmap(NULL, jsize, PROT_READ, MAP_SHARED, jfd, 0);
        close(jfd);
        if(!jv2)
            continue;
        journals++;

        unsigned tier = 0, fileno = 0;
        if(sscanf(de->d_name, WALFILE_PREFIX RRDENG_FILE_NUMBER_SCAN_TMPL, &tier, &fileno) != 2)
            tier = fileno = 0;

        int ffd = open(fpath, O_RDONLY | O_CLOEXEC);
        if(ffd != -1) {
            struct stat fst;
            struct njtf_header h;
            if(fstat(ffd, &fst) == 0 && (size_t)fst.st_size >= NJTF_HEADER_SIZE &&
               pread(ffd, &h, sizeof(h), 0) == (ssize_t)sizeof(h) && h.magic == NJTF_MAGIC &&
               h.header_crc == njtf_header_crc(&h)) {
                with_filter++;
                if(!njtf_header_matches(&h, (size_t)fst.st_size, tier, fileno, jv2, jsize))
                    identity_mismatch++;
                else {
                    uint8_t *map = nd_mmap(NULL, (size_t)fst.st_size, PROT_READ, MAP_SHARED, ffd, 0);
                    if(map) {
                        uLong crc = crc32(0L, Z_NULL, 0);
                        crc = crc32(crc, map + NJTF_HEADER_SIZE, (uInt)((size_t)fst.st_size - NJTF_HEADER_SIZE));
                        if((uint32_t)crc != h.payload_crc)
                            bad_payload++;
                        else {
                            struct njtf_verify_state vs = { .payload = map + NJTF_HEADER_SIZE, .blocks = h.blocks, .seed = h.seed };
                            njtf_walk_metrics(jv2, jsize, njtf_verify_metric, &vs);
                            checked += vs.checked;
                            false_negatives += vs.false_negatives;
                            if(vs.false_negatives)
                                fprintf(stderr, "NJTF: %s: %" PRIu64 " false negatives of %" PRIu64 "\n",
                                        fpath, vs.false_negatives, vs.checked);
                        }
                        nd_munmap(map, (size_t)fst.st_size);
                    }
                }
            }
            close(ffd);
        }

        nd_munmap(jv2, jsize);
    }
    closedir(dir);

    fprintf(stderr, "NJTF: %zu journals, %zu with a filter, %zu identity mismatches, %zu bad payloads, "
                    "%" PRIu64 " metric-hours checked, %" PRIu64 " false negatives: %s\n",
            journals, with_filter, identity_mismatch, bad_payload, checked, false_negatives,
            (false_negatives || bad_payload) ? "FAILED" : "OK");
    return (false_negatives || bad_payload) ? 1 : 0;
}

// ----------------------------------------------------------------------------
// unit test: a synthetic journal v2, every page positive for every hour it touches, false positives ~1%

// -W njtftest runs before the agent installs its signal handlers: route SIGBUS to protected access, like they do
static void njtf_unittest_sigbus(int sig, siginfo_t *si, void *context) {
    signal_protected_access_check(sig, si, context);
    abort();    // not a protected access: a real crash
}

// The filter file on disk: built, charged to the tier, verified; refused when missing, of another journal,
// corrupted or truncated; bypassed when it becomes unreadable under a query; deleted and uncharged.
static int njtf_files_unittest(const uint8_t *jv2, size_t size) {
    char dbpath[FILENAME_MAX + 1];
    if(!jv2_make_tmpdir(dbpath, sizeof(dbpath))) {
        fprintf(stderr, "NJTF: SKIPPED the filter file test: no usable temporary directory\n");
        return 0;
    }

    struct rrdengine_instance *ctx = callocz(1, sizeof(*ctx));
    ctx->config.tier = 0;
    strncpyz(ctx->config.dbfiles_path, dbpath, sizeof(ctx->config.dbfiles_path) - 1);

    struct rrdengine_datafile *datafile = callocz(1, sizeof(*datafile));
    datafile->tier = 1;         // the datafile file-name tier, which is always 1
    datafile->fileno = 7;
    datafile->ctx = ctx;
    datafile->magic1 = datafile->magic2 = DATAFILE_MAGIC;
    struct rrdengine_journalfile *journalfile = journalfile_alloc_and_init(datafile);

    char path[RRDENG_PATH_MAX];
    njtf_generate_path(datafile, path, sizeof(path), false);

    int errors = 0;
#define NJTF_EXPECT(cond, what) do { if(!(cond)) { fprintf(stderr, "NJTF: files: %s\n", what); errors++; } } while(0)

    NJTF_EXPECT(!njtf_verify_and_map(journalfile, jv2, size), "a missing filter was accepted");
    NJTF_EXPECT(njtf_build(datafile, jv2, size), "building the filter file failed");

    struct stat st = { 0 };
    NJTF_EXPECT(stat(path, &st) == 0, "the filter file was not written");
    uint64_t file_size = (uint64_t)st.st_size;
    NJTF_EXPECT(file_size && ctx_current_disk_space_get(ctx) == file_size, "the tier was not charged the file size");

    NJTF_EXPECT(njtf_verify_and_map(journalfile, jv2, size), "a fresh filter failed verification");
    njtf_release(journalfile);
    njtf_charge_existing(datafile);
    NJTF_EXPECT(ctx_current_disk_space_get(ctx) == file_size, "verifying or loading charged the file twice");

    // the filter of another journal (its header CRC differs)
    uint8_t *other = mallocz(size);
    memcpy(other, jv2, size);
    other[size - 1] ^= 0xFF;
    NJTF_EXPECT(!njtf_verify_and_map(journalfile, other, size), "the filter of another journal was accepted");
    freez(other);

    // a flipped payload bit, then a truncated file
    int fd = open(path, O_RDWR | O_CLOEXEC);
    uint8_t byte = 0;
    if(fd == -1 || pread(fd, &byte, 1, NJTF_HEADER_SIZE + 5) != 1) {
        NJTF_EXPECT(false, "cannot read the filter file back");
    }
    else {
        byte ^= 0x10;
        NJTF_EXPECT(pwrite(fd, &byte, 1, NJTF_HEADER_SIZE + 5) == 1, "cannot corrupt the filter file");
        NJTF_EXPECT(!njtf_verify_and_map(journalfile, jv2, size), "a corrupted filter was accepted");
        byte ^= 0x10;
        NJTF_EXPECT(pwrite(fd, &byte, 1, NJTF_HEADER_SIZE + 5) == 1, "cannot restore the filter file");
        NJTF_EXPECT(ftruncate(fd, (off_t)(file_size - NJTF_BLOCK_BYTES)) == 0, "cannot truncate the filter file");
        NJTF_EXPECT(!njtf_verify_and_map(journalfile, jv2, size), "a truncated filter was accepted");
    }
    if(fd != -1)
        close(fd);

    // a filter that becomes unreadable under a query (SIGBUS past a truncated end) is bypassed, not a crash
    struct sigaction sa = { 0 }, saved_sa;
    sa.sa_sigaction = njtf_unittest_sigbus;
    sa.sa_flags = SA_SIGINFO;
    sigemptyset(&sa.sa_mask);
    if(sigaction(SIGBUS, &sa, &saved_sa) != 0) {
        NJTF_EXPECT(false, "cannot install a SIGBUS handler");
    }
    else {
        const struct journal_v2_header *j2 = (const struct journal_v2_header *)jv2;
        const struct journal_metric_list *ml = (const struct journal_metric_list *)(jv2 + j2->metric_offset);
        time_t after_s = (time_t)(j2->start_time_ut / USEC_PER_SEC);

        if(!njtf_build(datafile, jv2, size) || !njtf_verify_and_map(journalfile, jv2, size)) {
            NJTF_EXPECT(false, "cannot rebuild the filter file");
        }
        else {
            __atomic_store_n(&journalfile->njtf.state, NJTF_STATE_VALID, __ATOMIC_RELEASE);
            NJTF_EXPECT(truncate(path, NJTF_HEADER_SIZE) == 0, "cannot truncate the mapped filter file");
            NJTF_EXPECT(njtf_check(journalfile, &ml[0].uuid, after_s, after_s + 600) == NJTF_CHECK_BYPASS,
                        "an unreadable filter was not bypassed");
            NJTF_EXPECT(__atomic_load_n(&journalfile->njtf.state, __ATOMIC_ACQUIRE) == NJTF_STATE_FAILED,
                        "an unreadable filter was not marked failed");
            njtf_release(journalfile);
        }
        sigaction(SIGBUS, &saved_sa, NULL);
    }

    njtf_unlink(datafile);
    NJTF_EXPECT(stat(path, &st) != 0 && errno == ENOENT, "the filter file was not deleted");
    NJTF_EXPECT(ctx_current_disk_space_get(ctx) == 0, "deleting the filter did not uncharge the tier");

#undef NJTF_EXPECT

    fprintf(stderr, "NJTF: filter file of %" PRIu64 " bytes: written, charged, verified, rejected when stale, corrupted "
                    "or truncated, bypassed when unreadable, deleted and uncharged: %s\n", file_size, errors ? "FAILED" : "OK");

    rmdir(dbpath);
    freez(journalfile);
    freez(datafile);
    freez(ctx);
    return errors;
}

int njtf_unittest(void) {
    const size_t metrics = 20000;
    const size_t pages_per_metric = 4;
    const time_t start_s = 1790000000;

    size_t metric_offset = NJTF_HEADER_SIZE;
    size_t pages_offset = metric_offset + metrics * sizeof(struct journal_metric_list) + sizeof(struct journal_v2_block_trailer);
    size_t per_metric = sizeof(struct journal_page_header) + pages_per_metric * sizeof(struct journal_page_list) +
                        sizeof(struct journal_v2_block_trailer);
    size_t size = pages_offset + metrics * per_metric + sizeof(struct journal_v2_block_trailer);

    uint8_t *jv2 = callocz(1, size);
    struct journal_v2_header *h = (struct journal_v2_header *)jv2;
    h->magic = JOURVAL_V2_MAGIC;
    h->start_time_ut = (usec_t)start_s * USEC_PER_SEC;
    h->metric_count = (uint32_t)metrics;
    h->metric_offset = (uint32_t)metric_offset;
    h->journal_v2_file_size = (uint32_t)size;

    struct journal_metric_list *ml = (struct journal_metric_list *)(jv2 + metric_offset);
    for(size_t i = 0; i < metrics; i++) {
        uuid_generate_random(ml[i].uuid);
        ml[i].entries = (uint32_t)pages_per_metric;
        ml[i].page_offset = (uint32_t)(pages_offset + i * per_metric);
        struct journal_page_header *ph = (struct journal_page_header *)(jv2 + ml[i].page_offset);
        ph->entries = (uint32_t)pages_per_metric;
        memcpy(ph->uuid, ml[i].uuid, sizeof(nd_uuid_t));
        struct journal_page_list *pl = (struct journal_page_list *)((uint8_t *)ph + sizeof(*ph));
        // pages of ~17 minutes, some crossing hour boundaries, out of time order: a page, another hours later, the
        // page right after the first (often the same hour) and one overlapping the first (always the same hours)
        uint32_t s = (uint32_t)((i * 37) % (6 * 3600));
        uint32_t starts[] = { s, (uint32_t)((i * 37 + 9000) % (6 * 3600)), s + 1024, s + 200 };
        for(size_t p = 0; p < pages_per_metric; p++) {
            pl[p].delta_start_s = starts[p];
            pl[p].delta_end_s = starts[p] + 1023;
            pl[p].update_every_s = 1;
        }
    }

    uint8_t *payload = NULL;
    uint32_t blocks = 0;
    uint64_t keys = 0;
    int errors = 0;

    if(!njtf_build_payload(jv2, size, 12345, &payload, &blocks, &keys)) {
        fprintf(stderr, "NJTF: building the payload failed\n");
        freez(jv2);
        return 1;
    }

    // no false negatives: every hour of every page is positive; and one key per distinct (metric, hour)
    size_t positives = 0, distinct = 0;
    for(size_t i = 0; i < metrics; i++) {
        struct journal_page_header *ph = (struct journal_page_header *)(jv2 + ml[i].page_offset);
        struct journal_page_list *pl = (struct journal_page_list *)((uint8_t *)ph + sizeof(*ph));
        uint64_t seen[16];
        size_t seen_count = 0;
        for(size_t p = 0; p < pages_per_metric; p++) {
            time_t first_s = start_s + pl[p].delta_start_s, last_s = start_s + pl[p].delta_end_s;
            for(uint64_t b = njtf_bucket(first_s); b <= njtf_bucket(last_s); b++) {
                bool dup = false;
                for(size_t k = 0; k < seen_count && !dup; k++)
                    dup = seen[k] == b;
                if(!dup && seen_count < _countof(seen))
                    seen[seen_count++] = b;

                positives++;
                if(!njtf_key_test(payload, njtf_key(ml[i].uuid, b, 12345, blocks))) {
                    if(errors++ < 10)
                        fprintf(stderr, "NJTF: false negative: metric %zu page %zu bucket %" PRIu64 "\n", i, p, b);
                }
            }
        }
        distinct += seen_count;
    }

    // false positives: hours no page touches, and metrics not in the journal
    size_t fp = 0, tests = 0;
    for(size_t i = 0; i < metrics; i++) {
        nd_uuid_t other;
        uuid_generate_random(other);
        for(uint64_t b = njtf_bucket(start_s + 30 * 3600); b < njtf_bucket(start_s + 30 * 3600) + 5; b++) {
            tests += 2;
            fp += njtf_key_test(payload, njtf_key(ml[i].uuid, b, 12345, blocks));
            fp += njtf_key_test(payload, njtf_key(other, b, 12345, blocks));
        }
    }

    double fp_rate = (double)fp * 100.0 / (double)tests;
    fprintf(stderr, "NJTF: %" PRIu64 " keys (%zu page-hours), %u blocks (%zu KiB), %zu positives checked, "
                    "%d false negatives, false positives %.2f%% (%zu of %zu)\n",
            keys, positives, blocks, (size_t)blocks * NJTF_BLOCK_BYTES / 1024, positives, errors, fp_rate, fp, tests);

    if(keys != distinct) {
        fprintf(stderr, "NJTF: key count %" PRIu64 " != distinct (metric, hour) pairs %zu\n", keys, distinct);
        errors++;
    }

    if(fp_rate > 2.5) {
        fprintf(stderr, "NJTF: false positive rate %.2f%% is above 2.5%%\n", fp_rate);
        errors++;
    }

    // a malformed journal is refused
    h->journal_v2_file_size = (uint32_t)size + 1;
    uint8_t *p2 = NULL;
    if(njtf_build_payload(jv2, size, 12345, &p2, &blocks, &keys)) {
        fprintf(stderr, "NJTF: a journal with the wrong size was accepted\n");
        freez(p2);
        errors++;
    }

    freez(payload);

    h->journal_v2_file_size = (uint32_t)size;
    errors += njtf_files_unittest(jv2, size);
    freez(jv2);

    fprintf(stderr, "NJTF: %s\n", errors ? "FAILED" : "OK");
    return errors ? 1 : 0;
}
