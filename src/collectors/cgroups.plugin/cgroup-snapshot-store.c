// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-snapshot-store.h"

struct cgroup_snapshot_store {
    uint64_t generation;
    size_t count;
    CGROUP_SNAPSHOT_ENTRY *entries;     // discovery order, for snapshot iteration
    DICTIONARY *by_path;                // id -> CGROUP_SNAPSHOT_ENTRY* (borrowed)
};

struct cgroup_snapshot_builder {
    uint64_t generation;
    size_t count;
    size_t capacity;
    CGROUP_SNAPSHOT_ENTRY *entries;     // store owns these directly (no dup)
    DICTIONARY *seen;                   // id -> NULL, dedupe within a build
};

// the published store lives behind one rwlock: discovery swaps it under the
// write lock, handlers read under the read lock.
static netdata_rwlock_t cgroup_snapshot_rwlock;
static CGROUP_SNAPSHOT_STORE *cgroup_snapshot_current = NULL;

void cgroup_snapshot_store_init(void) {
    netdata_rwlock_init(&cgroup_snapshot_rwlock);
}

// ---------------------------------------------------------------------------
// entry lifetime

static void cgroup_snapshot_entry_free(CGROUP_SNAPSHOT_ENTRY *e) {
    string_freez(e->id);
    string_freez(e->name);
    string_freez(e->chart_name);
    string_freez(e->procs_path);
    for (uint16_t i = 0; i < e->label_count; i++) {
        string_freez(e->labels[i].key);
        string_freez(e->labels[i].value);
    }
    freez(e->labels);
}

static void cgroup_snapshot_store_free(CGROUP_SNAPSHOT_STORE *store) {
    if (!store)
        return;

    dictionary_destroy(store->by_path);
    for (size_t i = 0; i < store->count; i++)
        cgroup_snapshot_entry_free(&store->entries[i]);
    freez(store->entries);
    freez(store);
}

// ---------------------------------------------------------------------------
// build

CGROUP_SNAPSHOT_BUILDER *cgroup_snapshot_builder_create(uint64_t generation, size_t expected_entries) {
    CGROUP_SNAPSHOT_BUILDER *b = callocz(1, sizeof(*b));
    b->generation = generation;
    b->capacity = expected_entries ? expected_entries : 64;
    b->entries = callocz(b->capacity, sizeof(*b->entries));
    b->seen = dictionary_create_advanced(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_NAME_LINK_DONT_CLONE |
            DICT_OPTION_VALUE_LINK_DONT_CLONE,
        NULL, 0);
    return b;
}

CGROUP_SNAPSHOT_ENTRY *cgroup_snapshot_builder_add_entry(CGROUP_SNAPSHOT_BUILDER *builder, const char *id) {
    if (!builder || !id || !*id)
        return NULL;

    // first entry for an id wins, matching the legacy first-match linear scan.
    // The value is only a presence marker; store a non-NULL sentinel so the
    // linked (non-cloned) value is retrievable by dictionary_get.
    if (dictionary_get(builder->seen, id))
        return NULL;
    dictionary_set(builder->seen, id, builder, 0);

    if (builder->count == builder->capacity) {
        builder->capacity *= 2;
        builder->entries = reallocz(builder->entries, builder->capacity * sizeof(*builder->entries));
    }

    CGROUP_SNAPSHOT_ENTRY *e = &builder->entries[builder->count++];
    *e = (CGROUP_SNAPSHOT_ENTRY){ 0 };
    e->id = string_strdupz(id);
    return e;
}

void cgroup_snapshot_entry_add_label(CGROUP_SNAPSHOT_ENTRY *entry, const char *key, const char *value) {
    if (!entry || entry->label_count == UINT16_MAX)
        return;

    entry->labels = reallocz(entry->labels, (size_t)(entry->label_count + 1) * sizeof(*entry->labels));
    entry->labels[entry->label_count].key = string_strdupz(key ? key : "");
    entry->labels[entry->label_count].value = string_strdupz(value ? value : "");
    entry->label_count++;
}

void cgroup_snapshot_entry_set_dir_identity(CGROUP_SNAPSHOT_ENTRY *entry, const struct stat *st) {
    if (!entry || !st)
        return;

    entry->dir_identity_available = true;
    entry->dir_dev = st->st_dev;
    entry->dir_ino = st->st_ino;
}

CGROUP_SNAPSHOT_STORE *cgroup_snapshot_builder_finalize(CGROUP_SNAPSHOT_BUILDER *builder) {
    if (!builder)
        return NULL;

    CGROUP_SNAPSHOT_STORE *store = callocz(1, sizeof(*store));
    store->generation = builder->generation;
    store->count = builder->count;
    store->entries = builder->entries;          // ownership transferred
    store->by_path = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE,
        NULL, 0);

    for (size_t i = 0; i < store->count; i++)
        dictionary_set(store->by_path, string2str(store->entries[i].id), &store->entries[i], 0);

    dictionary_destroy(builder->seen);
    freez(builder);
    return store;
}

void cgroup_snapshot_store_publish(CGROUP_SNAPSHOT_STORE *store) {
    netdata_rwlock_wrlock(&cgroup_snapshot_rwlock);
    CGROUP_SNAPSHOT_STORE *old = cgroup_snapshot_current;
    cgroup_snapshot_current = store;
    netdata_rwlock_wrunlock(&cgroup_snapshot_rwlock);

    // safe: the write lock above guaranteed no reader still holds old
    cgroup_snapshot_store_free(old);
}

// ---------------------------------------------------------------------------
// read

const CGROUP_SNAPSHOT_STORE *cgroup_snapshot_store_acquire(void) {
    netdata_rwlock_rdlock(&cgroup_snapshot_rwlock);
    return cgroup_snapshot_current;
}

void cgroup_snapshot_store_release(void) {
    netdata_rwlock_rdunlock(&cgroup_snapshot_rwlock);
}

uint64_t cgroup_snapshot_store_generation(const CGROUP_SNAPSHOT_STORE *store) {
    return store ? store->generation : 0;
}

size_t cgroup_snapshot_store_count(const CGROUP_SNAPSHOT_STORE *store) {
    return store ? store->count : 0;
}

const CGROUP_SNAPSHOT_ENTRY *cgroup_snapshot_store_at(const CGROUP_SNAPSHOT_STORE *store, size_t index) {
    if (!store || index >= store->count)
        return NULL;
    return &store->entries[index];
}

const CGROUP_SNAPSHOT_ENTRY *cgroup_snapshot_store_find(
    const CGROUP_SNAPSHOT_STORE *store, const char *path, size_t path_len) {
    if (!store || !path)
        return NULL;
    return dictionary_get_advanced(store->by_path, path, (ssize_t)path_len);
}

const CGROUP_SNAPSHOT_ENTRY *cgroup_snapshot_store_find_unique_identity(
    const CGROUP_SNAPSHOT_STORE *store, dev_t dev, ino_t ino) {
    if (!store)
        return NULL;

    const CGROUP_SNAPSHOT_ENTRY *match = NULL;
    for (size_t i = 0; i < store->count; i++) {
        const CGROUP_SNAPSHOT_ENTRY *entry = &store->entries[i];
        if (!entry->dir_identity_available || entry->dir_dev != dev || entry->dir_ino != ino)
            continue;

        if (match)
            return NULL;

        match = entry;
    }

    return match;
}

static bool cgroup_snapshot_entry_has_suffix(const CGROUP_SNAPSHOT_ENTRY *entry, const char *suffix, size_t suffix_len) {
    if (!entry || !entry->id || !suffix || !suffix_len)
        return false;

    const char *id = string2str(entry->id);
    size_t id_len = string_strlen(entry->id);
    if (id_len < suffix_len)
        return false;

    const char *tail = id + id_len - suffix_len;
    if (memcmp(tail, suffix, suffix_len) != 0)
        return false;

    return id_len == suffix_len || tail == id || tail[-1] == '/';
}

const CGROUP_SNAPSHOT_ENTRY *cgroup_snapshot_store_find_unique_suffix(
    const CGROUP_SNAPSHOT_STORE *store, const char *suffix, size_t suffix_len, bool *duplicate) {
    if (duplicate)
        *duplicate = false;

    if (!store || !suffix || !suffix_len)
        return NULL;

    const CGROUP_SNAPSHOT_ENTRY *match = NULL;
    for (size_t i = 0; i < store->count; i++) {
        const CGROUP_SNAPSHOT_ENTRY *entry = &store->entries[i];
        if (!cgroup_snapshot_entry_has_suffix(entry, suffix, suffix_len))
            continue;

        if (match) {
            if (duplicate)
                *duplicate = true;
            return NULL;
        }

        match = entry;
    }

    return match;
}

// ---------------------------------------------------------------------------
// shutdown

void cgroup_snapshot_store_shutdown(void) {
    netdata_rwlock_wrlock(&cgroup_snapshot_rwlock);

    CGROUP_SNAPSHOT_STORE *store = cgroup_snapshot_current;
    cgroup_snapshot_current = NULL;

    netdata_rwlock_wrunlock(&cgroup_snapshot_rwlock);

    cgroup_snapshot_store_free(store);
}
