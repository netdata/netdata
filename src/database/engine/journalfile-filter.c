// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"

bool dbengine_journal_time_filters = true;

// TEST-ONLY (SOW-20261008-jv2-tier0-time-filters): <cache dir>/njtf-test-enabled (0 or 1, missing = 1) switches the
// filters off and on at runtime for interleaved A/B measurements; re-read by the builder every run. Not for merge.
static bool njtf_runtime_enabled = true;

void njtf_test_knob_refresh(void) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/njtf-test-enabled", netdata_configured_cache_dir);
    unsigned long long value = 1;
    if(read_single_number_file(filename, &value) != 0)
        value = 1;
    __atomic_store_n(&njtf_runtime_enabled, value ? true : false, __ATOMIC_RELAXED);
}

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

// Walk every page of a journal v2 file and call cb(uuid, first_s, last_s) with the times the planner reads back:
// the journal start plus the stored deltas. Returns false if the journal is malformed.
typedef bool (*njtf_page_cb)(const uint8_t *uuid, time_t first_s, time_t last_s, void *data);

static bool njtf_walk_pages(const uint8_t *jv2, size_t jv2_size, njtf_page_cb cb, void *data) {
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

    for(size_t i = 0; i < metric_count; i++) {
        size_t page_offset = metrics[i].page_offset;
        if(page_offset > jv2_size || jv2_size - page_offset < sizeof(struct journal_page_header))
            return false;

        const struct journal_page_header *ph = (const struct journal_page_header *)(jv2 + page_offset);
        size_t entries = ph->entries;
        size_t room = jv2_size - page_offset - sizeof(struct journal_page_header);
        if(entries > room / sizeof(struct journal_page_list))
            return false;

        const struct journal_page_list *pages =
            (const struct journal_page_list *)((const uint8_t *)ph + sizeof(struct journal_page_header));

        for(size_t p = 0; p < entries; p++) {
            time_t first_s = journal_start_s + pages[p].delta_start_s;
            time_t last_s = journal_start_s + pages[p].delta_end_s;
            if(!cb(metrics[i].uuid, first_s, last_s, data))
                return false;
        }
    }

    return true;
}

struct njtf_build_state {
    uint64_t keys;
    uint8_t *payload;
    uint32_t blocks;
    uint64_t seed;
};

static bool njtf_count_page(const uint8_t *uuid __maybe_unused, time_t first_s, time_t last_s, void *data) {
    struct njtf_build_state *bs = data;
    time_t lo = MIN(first_s, last_s), hi = MAX(first_s, last_s);
    if(lo < 0)
        return false;

    uint64_t buckets = njtf_bucket(hi) - njtf_bucket(lo) + 1;
    if(buckets > NJTF_MAX_PAGE_BUCKETS)
        return false;

    bs->keys += buckets;
    return true;
}

static bool njtf_set_page(const uint8_t *uuid, time_t first_s, time_t last_s, void *data) {
    struct njtf_build_state *bs = data;
    time_t lo = MIN(first_s, last_s), hi = MAX(first_s, last_s);
    for(uint64_t b = njtf_bucket(lo); b <= njtf_bucket(hi); b++)
        njtf_key_set(bs->payload, njtf_key(uuid, b, bs->seed, bs->blocks));
    return true;
}

// build the payload of a journal's filter; the caller frees *payload
static bool njtf_build_payload(const uint8_t *jv2, size_t jv2_size, uint64_t seed,
                               uint8_t **payload, uint32_t *blocks, uint64_t *keys) {
    struct njtf_build_state bs = { .seed = seed };

    if(!njtf_walk_pages(jv2, jv2_size, njtf_count_page, &bs))
        return false;

    uint64_t bits = bs.keys * NJTF_BITS_PER_KEY;
    uint64_t nblocks = (bits + NJTF_BLOCK_BITS - 1) / NJTF_BLOCK_BITS;
    if(!nblocks)
        nblocks = 1;
    if(nblocks > UINT32_MAX / NJTF_BLOCK_BYTES)
        return false;

    bs.blocks = (uint32_t)nblocks;
    bs.payload = callocz(nblocks, NJTF_BLOCK_BYTES);

    if(!njtf_walk_pages(jv2, jv2_size, njtf_set_page, &bs)) {
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

// Read and check the header of a journal's filter file against its journal. Returns the open fd (and the header)
// when the identity and the geometry match, -1 otherwise.
static int njtf_open_matching(struct rrdengine_datafile *datafile, const uint8_t *jv2, size_t jv2_size,
                              struct njtf_header *h, size_t *file_size) {
    char path[RRDENG_PATH_MAX];
    njtf_generate_path(datafile, path, sizeof(path), false);

    int fd = open(path, O_RDONLY | O_CLOEXEC);
    if(fd == -1)
        return -1;

    struct stat st;
    if(fstat(fd, &st) != 0 || (size_t)st.st_size < NJTF_HEADER_SIZE ||
       pread(fd, h, sizeof(*h), 0) != (ssize_t)sizeof(*h))
        goto bad;

    if(h->magic != NJTF_MAGIC || h->version != NJTF_VERSION || h->header_crc != njtf_header_crc(h) ||
       h->tier != datafile->tier || h->fileno != datafile->fileno ||
       h->journal_v2_file_size != jv2_size ||
       h->journal_v2_header_crc != njtf_journal_header_crc(jv2, jv2_size) ||
       h->bucket_s != NJTF_BUCKET_S || h->k != NJTF_K || !h->blocks ||
       (size_t)st.st_size != NJTF_HEADER_SIZE + (size_t)h->blocks * NJTF_BLOCK_BYTES)
        goto bad;

    *file_size = (size_t)st.st_size;
    return fd;

bad:
    close(fd);
    return -1;
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
    uLong crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, map + NJTF_HEADER_SIZE, (uInt)(file_size - NJTF_HEADER_SIZE));
    if((uint32_t)crc != h.payload_crc) {
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
    return true;
}

NJTF_CHECK njtf_check(struct rrdengine_journalfile *journalfile, const struct journal_v2_header *j2_header __maybe_unused,
                      size_t jv2_size __maybe_unused, const nd_uuid_t *uuid, time_t after_s, time_t before_s) {
    if(!dbengine_journal_time_filters || !__atomic_load_n(&njtf_runtime_enabled, __ATOMIC_RELAXED) ||
       after_s < 0 || before_s < after_s)
        return NJTF_CHECK_BYPASS;

    uint64_t first_bucket = njtf_bucket(after_s), last_bucket = njtf_bucket(before_s);
    if(last_bucket - first_bucket + 1 > NJTF_MAX_WINDOW_BUCKETS)
        return NJTF_CHECK_BYPASS;

    // queries never read or verify a filter: until the builder has verified and mapped it, search as today
    if(__atomic_load_n(&journalfile->njtf.state, __ATOMIC_ACQUIRE) != NJTF_STATE_VALID)
        return NJTF_CHECK_BYPASS;

    const uint8_t *map = journalfile->njtf.map;
    const struct njtf_header *h = (const struct njtf_header *)map;
    const uint8_t *payload = map + NJTF_HEADER_SIZE;

    for(uint64_t b = first_bucket; b <= last_bucket; b++) {
        if(njtf_key_test(payload, njtf_key(uuid[0], b, h->seed, h->blocks)))
            return NJTF_CHECK_SEARCH;
    }

    return NJTF_CHECK_SKIP;
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

        bool orphan = tmp;
        if(!tmp) {
            netdata_rwlock_rdlock(&ctx->datafiles.rwlock);
            orphan = JudyLGet(ctx->datafiles.JudyL, (Word_t)fileno, PJE0) == NULL;
            netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);
        }

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
    NJTF_STATE result = NJTF_STATE_FAILED;

    size_t jv2_size = 0;
    struct journal_v2_header *j2_header = journalfile_v2_data_acquire(journalfile, &jv2_size, 0, 0);
    if(j2_header) {
        char path[RRDENG_PATH_MAX];
        journalfile_v2_generate_path(datafile, path, sizeof(path));
        PROTECTED_ACCESS_SETUP(journalfile->mmap.data, journalfile->mmap.size, path, "filter");
        if(no_signal_received) {
            // an existing file that matches and verifies is used as is; otherwise (re)build it and verify that
            if(njtf_verify_and_map(journalfile, (const uint8_t *)j2_header, jv2_size))
                result = NJTF_STATE_VALID;
            else if(njtf_build(datafile, (const uint8_t *)j2_header, jv2_size) &&
                    njtf_verify_and_map(journalfile, (const uint8_t *)j2_header, jv2_size))
                result = NJTF_STATE_VALID;
        }
        journalfile_v2_data_release(journalfile);
    }
    else
        result = state;     // not available right now; try again later

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

static bool njtf_verify_page(const uint8_t *uuid, time_t first_s, time_t last_s, void *data) {
    struct njtf_verify_state *vs = data;
    time_t lo = MIN(first_s, last_s), hi = MAX(first_s, last_s);
    for(uint64_t b = njtf_bucket(lo); b <= njtf_bucket(hi); b++) {
        vs->checked++;
        if(!njtf_key_test(vs->payload, njtf_key(uuid, b, vs->seed, vs->blocks)))
            vs->false_negatives++;
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

        int ffd = open(fpath, O_RDONLY | O_CLOEXEC);
        if(ffd != -1) {
            struct stat fst;
            struct njtf_header h;
            if(fstat(ffd, &fst) == 0 && (size_t)fst.st_size >= NJTF_HEADER_SIZE &&
               pread(ffd, &h, sizeof(h), 0) == (ssize_t)sizeof(h) && h.magic == NJTF_MAGIC &&
               h.header_crc == njtf_header_crc(&h)) {
                with_filter++;
                if(h.journal_v2_file_size != jsize || h.journal_v2_header_crc != njtf_journal_header_crc(jv2, jsize) ||
                   (size_t)fst.st_size != NJTF_HEADER_SIZE + (size_t)h.blocks * NJTF_BLOCK_BYTES)
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
                            njtf_walk_pages(jv2, jsize, njtf_verify_page, &vs);
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
                    "%" PRIu64 " page-hours checked, %" PRIu64 " false negatives: %s\n",
            journals, with_filter, identity_mismatch, bad_payload, checked, false_negatives,
            (false_negatives || bad_payload) ? "FAILED" : "OK");
    return (false_negatives || bad_payload) ? 1 : 0;
}

// ----------------------------------------------------------------------------
// unit test: a synthetic journal v2, every page positive for every hour it touches, false positives ~1%

int njtf_unittest(void) {
    const size_t metrics = 20000;
    const size_t pages_per_metric = 3;
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
        for(size_t p = 0; p < pages_per_metric; p++) {
            // pages of ~17 minutes at varying offsets, some crossing hour boundaries
            uint32_t s = (uint32_t)((i * 37 + p * 4000) % (6 * 3600));
            pl[p].delta_start_s = s;
            pl[p].delta_end_s = s + 1023;
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

    // no false negatives: every hour of every page is positive
    size_t positives = 0;
    for(size_t i = 0; i < metrics; i++) {
        struct journal_page_header *ph = (struct journal_page_header *)(jv2 + ml[i].page_offset);
        struct journal_page_list *pl = (struct journal_page_list *)((uint8_t *)ph + sizeof(*ph));
        for(size_t p = 0; p < pages_per_metric; p++) {
            time_t first_s = start_s + pl[p].delta_start_s, last_s = start_s + pl[p].delta_end_s;
            for(uint64_t b = njtf_bucket(first_s); b <= njtf_bucket(last_s); b++) {
                positives++;
                if(!njtf_key_test(payload, njtf_key(ml[i].uuid, b, 12345, blocks))) {
                    if(errors++ < 10)
                        fprintf(stderr, "NJTF: false negative: metric %zu page %zu bucket %" PRIu64 "\n", i, p, b);
                }
            }
        }
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
    fprintf(stderr, "NJTF: %" PRIu64 " keys, %u blocks (%zu KiB), %zu positives checked, %d false negatives, "
                    "false positives %.2f%% (%zu of %zu)\n",
            keys, blocks, (size_t)blocks * NJTF_BLOCK_BYTES / 1024, positives, errors, fp_rate, fp, tests);

    if(keys != positives) {
        fprintf(stderr, "NJTF: key count %" PRIu64 " != positives checked %zu\n", keys, positives);
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
    freez(jv2);

    fprintf(stderr, "NJTF: %s\n", errors ? "FAILED" : "OK");
    return errors ? 1 : 0;
}
