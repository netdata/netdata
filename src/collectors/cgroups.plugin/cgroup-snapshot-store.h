// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_SNAPSHOT_STORE_H
#define NETDATA_CGROUP_SNAPSHOT_STORE_H 1

#include "libnetdata/libnetdata.h"

// A discovery-owned, hash-indexed, immutable-per-generation snapshot of the
// cgroup metadata the netipc handlers serve. Discovery builds a new store off
// any contended lock and publishes it with a pointer swap under a dedicated
// rwlock; the lookup and snapshot handlers read it under a read lock and never
// touch cgroup_root or cgroup_root_mutex. This keeps the collection loop and
// the IPC handlers from contending, makes lookups O(1) instead of a linear
// scan, and moves the per-cgroup cgroup.procs stat() to discovery build time
// instead of the snapshot handler's hot path.

typedef struct cgroup_snapshot_label {
    STRING *key;
    STRING *value;
} CGROUP_SNAPSHOT_LABEL;

typedef struct cgroup_snapshot_entry {
    STRING *id;             // cgroup path; the lookup/snapshot key

    // lookup-handler fields
    bool known;             // discovery finished processing and resolved the name
    uint16_t orchestrator;
    STRING *name;
    CGROUP_SNAPSHOT_LABEL *labels;
    uint16_t label_count;

    // snapshot-handler fields (consumed by ebpf.plugin)
    STRING *chart_name;     // prefix + chart_id, precomputed
    uint32_t hash;          // simple_hash(chart_name)
    uint32_t options;
    uint32_t enabled;
    STRING *procs_path;     // precomputed cgroup.procs path; empty when missing
    bool dir_identity_available;
    dev_t dir_dev;
    ino_t dir_ino;
} CGROUP_SNAPSHOT_ENTRY;

typedef struct cgroup_snapshot_store CGROUP_SNAPSHOT_STORE;

// Initialize the store rwlock; call once before the netipc servers start.
void cgroup_snapshot_store_init(void);

// ---------------------------------------------------------------------------
// Build (discovery thread, off any contended lock)

typedef struct cgroup_snapshot_builder CGROUP_SNAPSHOT_BUILDER;

CGROUP_SNAPSHOT_BUILDER *cgroup_snapshot_builder_create(uint64_t generation, size_t expected_entries);

// Reserves an entry for id and returns it for the caller to fill in place; the
// store owns whatever STRINGs the caller assigns (use string_strdupz / the
// label helper below). Returns NULL if id was already added (first wins, like
// the legacy first-match scan). The pointer is valid until the next
// _add_entry() call on the same builder.
CGROUP_SNAPSHOT_ENTRY *cgroup_snapshot_builder_add_entry(CGROUP_SNAPSHOT_BUILDER *builder, const char *id);

// Appends a label to an entry returned by _add_entry(); copies key and value.
void cgroup_snapshot_entry_add_label(CGROUP_SNAPSHOT_ENTRY *entry, const char *key, const char *value);
void cgroup_snapshot_entry_set_dir_identity(CGROUP_SNAPSHOT_ENTRY *entry, const struct stat *st);

// Finalizes into an immutable store. The builder is consumed.
CGROUP_SNAPSHOT_STORE *cgroup_snapshot_builder_finalize(CGROUP_SNAPSHOT_BUILDER *builder);

// Publishes store as the current one under the rwlock and frees the previous
// store once no reader holds it. Takes ownership of store.
void cgroup_snapshot_store_publish(CGROUP_SNAPSHOT_STORE *store);

void cgroup_snapshot_store_shutdown(void);

// ---------------------------------------------------------------------------
// Read (netipc handler threads)

// Acquires the read lock and returns the current store (NULL before the first
// publish). Must be paired with cgroup_snapshot_store_release().
const CGROUP_SNAPSHOT_STORE *cgroup_snapshot_store_acquire(void);
void cgroup_snapshot_store_release(void);

uint64_t cgroup_snapshot_store_generation(const CGROUP_SNAPSHOT_STORE *store);
size_t cgroup_snapshot_store_count(const CGROUP_SNAPSHOT_STORE *store);
const CGROUP_SNAPSHOT_ENTRY *cgroup_snapshot_store_at(const CGROUP_SNAPSHOT_STORE *store, size_t index);
const CGROUP_SNAPSHOT_ENTRY *cgroup_snapshot_store_find(
    const CGROUP_SNAPSHOT_STORE *store, const char *path, size_t path_len);
const CGROUP_SNAPSHOT_ENTRY *cgroup_snapshot_store_find_unique_identity(
    const CGROUP_SNAPSHOT_STORE *store, dev_t dev, ino_t ino);
const CGROUP_SNAPSHOT_ENTRY *cgroup_snapshot_store_find_unique_suffix(
    const CGROUP_SNAPSHOT_STORE *store, const char *suffix, size_t suffix_len, bool *duplicate);

#endif // NETDATA_CGROUP_SNAPSHOT_STORE_H
