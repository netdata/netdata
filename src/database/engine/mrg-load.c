// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-internals.h"

DEFINE_JUDYL_TYPED(METRIC, METRIC *);
METRIC_JudyLSet acquired_metrics = { 0 };
size_t acquired_metrics_counter = 0;
size_t acquired_metrics_deleted = 0;

// ----------------------------------------------------------------------------
// prepopulation index
//
// While mrg_load() prepopulates MRG with every (uuid, tier) known to the
// metadata database, it also builds this open-addressing hash
// (uuid -> METRIC* per tier), so that startup journal replay can find the
// prepopulated metrics with a single lock-free probe instead of paying the
// uuidmap intern/free, the MRG partition locks, the Judy walks and the
// refcount round-trips for every journal metric entry (billions of entries
// on busy parents, with each unique uuid repeated across hundreds of
// journal files).
//
// Lifecycle: built single-threaded inside mrg_load() BEFORE the tiers start
// journal replay, strictly read-only while replay runs (probes need no
// locks), and freed by mrg_metric_prepopulate_cleanup() right before the
// prepopulated references are released - so every METRIC* in the index is
// pinned by acquired_metrics for the whole life of the index.

#define PREP_IDX_INITIAL_CAPACITY (1 << 16)         // must be a power of 2

struct prepopulation_index {
    uint8_t *slots;         // capacity slots of stride bytes:
                            // 16 bytes raw uuid + tiers x METRIC*
                            // a nil uuid marks an empty slot
    size_t capacity;        // power of 2
    size_t stride;
    size_t tiers;
    size_t used;            // unique uuids stored
};

static struct prepopulation_index prep_idx = { 0 };
static size_t prep_idx_hits = 0;
static size_t prep_idx_misses = 0;

static inline uint8_t *prep_idx_slot(const struct prepopulation_index *idx, size_t i) {
    return idx->slots + i * idx->stride;
}

static inline METRIC **prep_idx_slot_metrics(uint8_t *slot) {
    return (METRIC **)(slot + sizeof(nd_uuid_t));
}

static inline size_t prep_idx_first_slot(const struct prepopulation_index *idx, const nd_uuid_t uuid) {
    return XXH3_64bits(uuid, sizeof(nd_uuid_t)) & (idx->capacity - 1);
}

void mrg_prepopulation_index_init(size_t tiers) {
    if(!tiers || prep_idx.slots)
        return;

    prep_idx.tiers = tiers;
    prep_idx.stride = sizeof(nd_uuid_t) + tiers * sizeof(METRIC *);
    prep_idx.capacity = PREP_IDX_INITIAL_CAPACITY;
    prep_idx.used = 0;
    prep_idx.slots = callocz(prep_idx.capacity, prep_idx.stride);
    prep_idx_hits = 0;
    prep_idx_misses = 0;
}

// find the slot of uuid, or the empty slot where it should be inserted;
// the load factor is kept below 3/4 so the linear probe always terminates
static uint8_t *prep_idx_find_slot(const struct prepopulation_index *idx, const nd_uuid_t uuid) {
    size_t mask = idx->capacity - 1;
    for(size_t i = prep_idx_first_slot(idx, uuid); ; i = (i + 1) & mask) {
        uint8_t *slot = prep_idx_slot(idx, i);
        if(uuid_is_null(*(const nd_uuid_t *)slot) || memcmp(slot, uuid, sizeof(nd_uuid_t)) == 0)
            return slot;
    }
}

static void prep_idx_grow(struct prepopulation_index *idx) {
    struct prepopulation_index old = *idx;

    idx->capacity = old.capacity << 1;
    idx->slots = callocz(idx->capacity, idx->stride);

    for(size_t i = 0; i < old.capacity ;i++) {
        uint8_t *slot = prep_idx_slot(&old, i);
        if(uuid_is_null(*(const nd_uuid_t *)slot))
            continue;

        uint8_t *new_slot = prep_idx_find_slot(idx, *(const nd_uuid_t *)slot);
        memcpy(new_slot, slot, idx->stride);
    }

    freez(old.slots);
}

// single-threaded, during mrg_load() only - the index is immutable once
// journal replay starts
void mrg_prepopulation_index_insert(const nd_uuid_t uuid, size_t tier, METRIC *metric) {
    struct prepopulation_index *idx = &prep_idx;

    if(unlikely(!idx->slots || tier >= idx->tiers || uuid_is_null(uuid)))
        return;

    if(unlikely(idx->used >= idx->capacity / 4 * 3))
        prep_idx_grow(idx);

    uint8_t *slot = prep_idx_find_slot(idx, uuid);
    if(uuid_is_null(*(const nd_uuid_t *)slot)) {
        memcpy(slot, uuid, sizeof(nd_uuid_t));
        idx->used++;
    }

    prep_idx_slot_metrics(slot)[tier] = metric;
}

// lock-free; safe at any time - returns NULL when there is no index, the
// tier is out of range, or the uuid is not in the metadata database
METRIC *mrg_prepopulation_index_get(const nd_uuid_t uuid, size_t tier) {
    const struct prepopulation_index *idx = &prep_idx;

    if(unlikely(!idx->slots || tier >= idx->tiers))
        return NULL;

    size_t mask = idx->capacity - 1;
    for(size_t i = prep_idx_first_slot(idx, uuid); ; i = (i + 1) & mask) {
        uint8_t *slot = prep_idx_slot(idx, i);

        if(uuid_is_null(*(const nd_uuid_t *)slot))
            return NULL;

        if(memcmp(slot, uuid, sizeof(nd_uuid_t)) == 0)
            return prep_idx_slot_metrics(slot)[tier];
    }
}

// hit/miss accounting is batched per journal file by the callers, so the
// hot per-entry path never touches these shared counters
void mrg_prepopulation_index_account(size_t hits, size_t misses) {
    if(hits)
        __atomic_add_fetch(&prep_idx_hits, hits, __ATOMIC_RELAXED);
    if(misses)
        __atomic_add_fetch(&prep_idx_misses, misses, __ATOMIC_RELAXED);
}

void mrg_prepopulation_index_free(void) {
    if(!prep_idx.slots)
        return;

    nd_log(NDLS_DAEMON, NDLP_INFO,
           "MRG: prepopulation index released: %zu unique metrics, %zu slots, %.1f MiB, "
           "journal replay used it for %zu entries and fell back for %zu",
           prep_idx.used, prep_idx.capacity,
           (double)(prep_idx.capacity * prep_idx.stride) / (1024.0 * 1024.0),
           __atomic_load_n(&prep_idx_hits, __ATOMIC_RELAXED),
           __atomic_load_n(&prep_idx_misses, __ATOMIC_RELAXED));

    freez(prep_idx.slots);
    memset(&prep_idx, 0, sizeof(prep_idx));
}

// ----------------------------------------------------------------------------

ALWAYS_INLINE
static void mrg_metric_prepopulate(void *mrg_ptr, Word_t section, size_t tier, nd_uuid_t *uuid) {
    MRG *mrg = mrg_ptr;
    MRG_ENTRY entry = {
        .uuid = uuid,
        .section = section,
        .first_time_s = 0,
        .last_time_s = 0,
        .latest_update_every_s = 0,
    };
    bool added = false;
    METRIC *metric = metric_add_and_acquire(mrg, &entry, &added);

    // whether added now or already there, the metric is pinned by
    // acquired_metrics until mrg_metric_prepopulate_cleanup(), so the index
    // can hold the raw pointer
    mrg_prepopulation_index_insert(*uuid, tier, metric);

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
    // free the index BEFORE releasing the references that pin its metrics
    mrg_prepopulation_index_free();

    acquired_metrics_deleted = 0;
    METRIC_FREE(&acquired_metrics, mrg_release_cb, mrg);

    if(acquired_metrics_counter || acquired_metrics_deleted)
        nd_log(NDLS_DAEMON, NDLP_INFO, "MRG DUMP: Prepopulated %zu metrics, released %zu, deleted %zu",
               acquired_metrics_counter, acquired_metrics_counter - acquired_metrics_deleted, acquired_metrics_deleted);

    acquired_metrics_counter = 0;
}

// Main function to load metrics from the database
bool mrg_load(MRG *mrg) {
    mrg_prepopulation_index_init(nd_profile.storage_tiers);
    size_t processed_metrics = populate_metrics_from_database(mrg, mrg_metric_prepopulate);
    return processed_metrics > 0;
}
