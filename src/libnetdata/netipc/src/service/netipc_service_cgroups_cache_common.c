#include "netipc_service_cgroups_cache_common.h"

#include "netipc/netipc_protocol.h"

#include <stdlib.h>
#include <string.h>

typedef struct {
    uint32_t index; /* index into items[] and views[] */
    bool used;
} nipc_cgroups_cache_bucket_t;

struct nipc_cgroups_cache_snapshot {
    nipc_cgroups_cache_item_t *items;
    nipc_cgroups_cache_item_view_t *views;
    uint32_t item_count;
    uint32_t systemd_enabled;
    uint64_t generation;
    nipc_cgroups_cache_bucket_t *buckets;
    uint32_t bucket_count; /* always a power of 2 when non-zero */
};

static uint32_t cache_hash_name(const char *name)
{
    uint32_t h = 5381;
    for (const unsigned char *p = (const unsigned char *)name; *p; p++)
        h = ((h << 5) + h) + *p;
    return h;
}

static void cache_free_items(nipc_cgroups_cache_item_t *items, uint32_t count)
{
    if (!items)
        return;

    for (uint32_t i = 0; i < count; i++) {
        free(items[i].name);
        free(items[i].path);
    }
    free(items);
}

static void cache_snapshot_free(nipc_cgroups_cache_snapshot_t *snapshot)
{
    if (!snapshot)
        return;

    cache_free_items(snapshot->items, snapshot->item_count);
    free(snapshot->views);
    free(snapshot->buckets);
    free(snapshot);
}

static bool cache_build_hashtable(nipc_cgroups_cache_snapshot_t *snapshot,
                                  const nipc_service_common_cache_ops_t *ops)
{
    if (snapshot->item_count == 0)
        return true;

    if (snapshot->item_count > UINT32_MAX / 2)
        return false;

    uint32_t bcount =
        nipc_service_common_next_power_of_2_u32(snapshot->item_count * 2);
    if (bcount == 0)
        return false;

    nipc_cgroups_cache_bucket_t *buckets = ops->calloc_fn(
        bcount, sizeof(nipc_cgroups_cache_bucket_t),
        ops->cache_buckets_fault_site);
    if (!buckets)
        return false;

    uint32_t mask = bcount - 1;
    for (uint32_t i = 0; i < snapshot->item_count; i++) {
        uint32_t key = snapshot->items[i].hash ^
                       cache_hash_name(snapshot->items[i].name);
        uint32_t slot = key & mask;

        while (buckets[slot].used)
            slot = (slot + 1) & mask;

        buckets[slot].index = i;
        buckets[slot].used = true;
    }

    snapshot->buckets = buckets;
    snapshot->bucket_count = bcount;
    return true;
}

static bool cache_copy_str(char **dst,
                           const char *src,
                           size_t len,
                           const nipc_service_common_cache_ops_t *ops,
                           int fault_site)
{
    *dst = ops->malloc_fn(len + 1, fault_site);
    if (!*dst)
        return false;
    if (len > 0)
        memcpy(*dst, src, len);
    (*dst)[len] = '\0';
    return true;
}

static size_t cache_cstr_len(const char *src)
{
    if (!src)
        return 0;

    const char *p = src;
    while (*p)
        p++;
    return (size_t)(p - src);
}

static nipc_cgroups_cache_snapshot_t *cache_snapshot_build(
    const nipc_cgroups_resp_view_t *view,
    const nipc_service_common_cache_ops_t *ops)
{
    nipc_cgroups_cache_snapshot_t *snapshot =
        calloc(1, sizeof(nipc_cgroups_cache_snapshot_t));
    if (!snapshot)
        return NULL;

    snapshot->item_count = view->item_count;
    snapshot->systemd_enabled = view->systemd_enabled;
    snapshot->generation = view->generation;

    if (view->item_count == 0)
        return snapshot;

    snapshot->items = ops->calloc_fn(view->item_count,
                                     sizeof(nipc_cgroups_cache_item_t),
                                     ops->cache_items_fault_site);
    if (!snapshot->items) {
        cache_snapshot_free(snapshot);
        return NULL;
    }

    snapshot->views = calloc(view->item_count,
                             sizeof(nipc_cgroups_cache_item_view_t));
    if (!snapshot->views) {
        cache_snapshot_free(snapshot);
        return NULL;
    }

    for (uint32_t i = 0; i < view->item_count; i++) {
        nipc_cgroups_item_view_t iv;
        nipc_error_t err = nipc_cgroups_resp_item(view, i, &iv);
        if (err != NIPC_OK) {
            cache_snapshot_free(snapshot);
            return NULL;
        }

        snapshot->items[i].hash = iv.hash;
        snapshot->items[i].options = iv.options;
        snapshot->items[i].enabled = iv.enabled;

        if (!cache_copy_str(&snapshot->items[i].name, iv.name.ptr, iv.name.len,
                            ops, ops->cache_item_name_fault_site) ||
            !cache_copy_str(&snapshot->items[i].path, iv.path.ptr, iv.path.len,
                            ops, ops->cache_item_path_fault_site)) {
            cache_snapshot_free(snapshot);
            return NULL;
        }

        snapshot->views[i].hash = snapshot->items[i].hash;
        snapshot->views[i].options = snapshot->items[i].options;
        snapshot->views[i].enabled = snapshot->items[i].enabled;
        snapshot->views[i].name = snapshot->items[i].name;
        snapshot->views[i].path = snapshot->items[i].path;
    }

    if (!cache_build_hashtable(snapshot, ops)) {
        /*
         * Hashtable construction is an optimization. Keep the snapshot usable
         * through linear lookup if the bucket allocation fails.
         */
        free(snapshot->buckets);
        snapshot->buckets = NULL;
        snapshot->bucket_count = 0;
    }

    return snapshot;
}

static nipc_cgroups_cache_snapshot_t *cache_snapshot_build_from_items(
    const nipc_cgroups_cache_item_t *items,
    uint32_t item_count,
    uint32_t systemd_enabled,
    uint64_t generation,
    const nipc_service_common_cache_ops_t *ops)
{
    nipc_cgroups_cache_snapshot_t *snapshot =
        calloc(1, sizeof(nipc_cgroups_cache_snapshot_t));
    if (!snapshot)
        return NULL;

    snapshot->item_count = item_count;
    snapshot->systemd_enabled = systemd_enabled;
    snapshot->generation = generation;

    if (item_count == 0)
        return snapshot;

    snapshot->items = ops->calloc_fn(item_count,
                                     sizeof(nipc_cgroups_cache_item_t),
                                     ops->cache_items_fault_site);
    if (!snapshot->items) {
        cache_snapshot_free(snapshot);
        return NULL;
    }

    snapshot->views = calloc(item_count,
                             sizeof(nipc_cgroups_cache_item_view_t));
    if (!snapshot->views) {
        cache_snapshot_free(snapshot);
        return NULL;
    }

    for (uint32_t i = 0; i < item_count; i++) {
        const char *name = items[i].name ? items[i].name : "";
        const char *path = items[i].path ? items[i].path : "";

        snapshot->items[i].hash = items[i].hash;
        snapshot->items[i].options = items[i].options;
        snapshot->items[i].enabled = items[i].enabled;

        size_t name_len = cache_cstr_len(name);
        size_t path_len = cache_cstr_len(path);

        if (!cache_copy_str(&snapshot->items[i].name, name, name_len,
                            ops, ops->cache_item_name_fault_site) ||
            !cache_copy_str(&snapshot->items[i].path, path, path_len,
                            ops, ops->cache_item_path_fault_site)) {
            cache_snapshot_free(snapshot);
            return NULL;
        }

        snapshot->views[i].hash = snapshot->items[i].hash;
        snapshot->views[i].options = snapshot->items[i].options;
        snapshot->views[i].enabled = snapshot->items[i].enabled;
        snapshot->views[i].name = snapshot->items[i].name;
        snapshot->views[i].path = snapshot->items[i].path;
    }

    if (!cache_build_hashtable(snapshot, ops)) {
        free(snapshot->buckets);
        snapshot->buckets = NULL;
        snapshot->bucket_count = 0;
    }

    return snapshot;
}

static bool cache_read_lock(nipc_cgroups_cache_t *cache)
{
    if (!cache || !cache->cache_lock_initialized)
        return false;

#if defined(_WIN32) || defined(__MSYS__)
    AcquireSRWLockShared(&cache->cache_lock);
    return true;
#else
    return pthread_rwlock_rdlock(&cache->cache_lock) == 0;
#endif
}

static void cache_read_unlock(nipc_cgroups_cache_t *cache)
{
    if (!cache || !cache->cache_lock_initialized)
        return;

#if defined(_WIN32) || defined(__MSYS__)
    ReleaseSRWLockShared(&cache->cache_lock);
#else
    pthread_rwlock_unlock(&cache->cache_lock);
#endif
}

static bool cache_write_lock(nipc_cgroups_cache_t *cache)
{
    if (!cache || !cache->cache_lock_initialized)
        return false;

#if defined(_WIN32) || defined(__MSYS__)
    AcquireSRWLockExclusive(&cache->cache_lock);
    return true;
#else
    return pthread_rwlock_wrlock(&cache->cache_lock) == 0;
#endif
}

static void cache_write_unlock(nipc_cgroups_cache_t *cache)
{
    if (!cache || !cache->cache_lock_initialized)
        return;

#if defined(_WIN32) || defined(__MSYS__)
    ReleaseSRWLockExclusive(&cache->cache_lock);
#else
    pthread_rwlock_unlock(&cache->cache_lock);
#endif
}

static bool cache_writer_lock(nipc_cgroups_cache_t *cache)
{
    if (!cache || !cache->writer_lock_initialized)
        return false;

#if defined(_WIN32) || defined(__MSYS__)
    EnterCriticalSection(&cache->writer_lock);
    return true;
#else
    return pthread_mutex_lock(&cache->writer_lock) == 0;
#endif
}

static void cache_writer_unlock(nipc_cgroups_cache_t *cache)
{
    if (!cache || !cache->writer_lock_initialized)
        return;

#if defined(_WIN32) || defined(__MSYS__)
    LeaveCriticalSection(&cache->writer_lock);
#else
    pthread_mutex_unlock(&cache->writer_lock);
#endif
}

static bool cache_init_locks(nipc_cgroups_cache_t *cache)
{
#if defined(_WIN32) || defined(__MSYS__)
    InitializeSRWLock(&cache->cache_lock);
    cache->cache_lock_initialized = true;
    InitializeCriticalSection(&cache->writer_lock);
    cache->writer_lock_initialized = true;
    return true;
#else
    if (pthread_rwlock_init(&cache->cache_lock, NULL) != 0)
        return false;
    cache->cache_lock_initialized = true;

    if (pthread_mutex_init(&cache->writer_lock, NULL) != 0) {
        pthread_rwlock_destroy(&cache->cache_lock);
        cache->cache_lock_initialized = false;
        return false;
    }
    cache->writer_lock_initialized = true;
    return true;
#endif
}

static void cache_destroy_locks(nipc_cgroups_cache_t *cache)
{
#if defined(_WIN32) || defined(__MSYS__)
    if (cache->writer_lock_initialized) {
        DeleteCriticalSection(&cache->writer_lock);
        cache->writer_lock_initialized = false;
    }
    cache->cache_lock_initialized = false;
#else
    if (cache->writer_lock_initialized) {
        pthread_mutex_destroy(&cache->writer_lock);
        cache->writer_lock_initialized = false;
    }
    if (cache->cache_lock_initialized) {
        pthread_rwlock_destroy(&cache->cache_lock);
        cache->cache_lock_initialized = false;
    }
#endif
}

void nipc_service_common_cgroups_cache_init(nipc_cgroups_cache_t *cache,
                                            const char *run_dir,
                                            const char *service_name,
                                            const nipc_client_config_t *config)
{
    memset(cache, 0, sizeof(*cache));
    cache_init_locks(cache);
    nipc_client_init(&cache->client, run_dir, service_name, config);
}

bool nipc_service_common_cgroups_cache_refresh(
    nipc_cgroups_cache_t *cache,
    const nipc_service_common_cache_ops_t *ops)
{
    if (!cache_writer_lock(cache))
        return false;

    nipc_client_refresh(&cache->client);

    nipc_cgroups_resp_view_t view;
    nipc_error_t err = nipc_client_call_cgroups_snapshot(&cache->client, &view);
    if (err != NIPC_OK) {
        cache->refresh_failure_count++;
        cache_writer_unlock(cache);
        return false;
    }

    nipc_cgroups_cache_snapshot_t *new_snapshot =
        cache_snapshot_build(&view, ops);
    if (!new_snapshot) {
        cache->refresh_failure_count++;
        cache_writer_unlock(cache);
        return false;
    }

    nipc_cgroups_cache_snapshot_t *old_snapshot = NULL;
    if (!cache_write_lock(cache)) {
        cache_snapshot_free(new_snapshot);
        cache->refresh_failure_count++;
        cache_writer_unlock(cache);
        return false;
    }

    old_snapshot = cache->snapshot;
    cache->snapshot = new_snapshot;
    cache->refresh_success_count++;
    cache->last_refresh_ts = ops->monotonic_ms_fn();

    cache_write_unlock(cache);
    cache_snapshot_free(old_snapshot);
    cache_writer_unlock(cache);

    return true;
}

bool nipc_service_common_cgroups_cache_ready(nipc_cgroups_cache_t *cache)
{
    if (!cache_read_lock(cache))
        return false;
    bool ready = cache->snapshot != NULL;
    cache_read_unlock(cache);
    return ready;
}

bool nipc_service_common_cgroups_cache_read_lock(
    nipc_cgroups_cache_t *cache,
    nipc_cgroups_cache_read_guard_t *guard)
{
    if (!guard)
        return false;

    memset(guard, 0, sizeof(*guard));

    if (!cache_read_lock(cache))
        return false;

    guard->cache = cache;
    guard->snapshot = cache->snapshot;
    guard->locked = true;
    return true;
}

void nipc_service_common_cgroups_cache_read_unlock(
    nipc_cgroups_cache_read_guard_t *guard)
{
    if (!guard || !guard->locked)
        return;

    nipc_cgroups_cache_t *cache = guard->cache;
    guard->cache = NULL;
    guard->snapshot = NULL;
    guard->locked = false;
    cache_read_unlock(cache);
}

const nipc_cgroups_cache_item_view_t *nipc_service_common_cgroups_cache_get(
    const nipc_cgroups_cache_read_guard_t *guard,
    uint32_t hash,
    const char *name)
{
    if (!guard || !guard->locked || !guard->snapshot || !name)
        return NULL;

    const nipc_cgroups_cache_snapshot_t *snapshot = guard->snapshot;
    if (snapshot->item_count == 0 || !snapshot->views)
        return NULL;

    if (snapshot->buckets && snapshot->bucket_count > 0) {
        uint32_t key = hash ^ cache_hash_name(name);
        uint32_t mask = snapshot->bucket_count - 1;
        uint32_t slot = key & mask;

        while (snapshot->buckets[slot].used) {
            uint32_t idx = snapshot->buckets[slot].index;
            if (snapshot->views[idx].hash == hash &&
                strcmp(snapshot->views[idx].name, name) == 0)
                return &snapshot->views[idx];
            slot = (slot + 1) & mask;
        }
        return NULL;
    }

    for (uint32_t i = 0; i < snapshot->item_count; i++) {
        if (snapshot->views[i].hash == hash &&
            strcmp(snapshot->views[i].name, name) == 0)
            return &snapshot->views[i];
    }

    return NULL;
}

static char *cache_strdup_owned(const char *src)
{
    if (!src)
        src = "";
    size_t len = cache_cstr_len(src);
    char *dst = malloc(len + 1);
    if (!dst)
        return NULL;
    memcpy(dst, src, len + 1);
    return dst;
}

nipc_cgroups_cache_item_t *nipc_service_common_cgroups_cache_item_dup(
    const nipc_cgroups_cache_read_guard_t *guard,
    const nipc_cgroups_cache_item_view_t *view)
{
    if (!guard || !guard->locked || !view)
        return NULL;

    nipc_cgroups_cache_item_t *item = calloc(1, sizeof(*item));
    if (!item)
        return NULL;

    item->hash = view->hash;
    item->options = view->options;
    item->enabled = view->enabled;
    item->name = cache_strdup_owned(view->name);
    item->path = cache_strdup_owned(view->path);
    if (!item->name || !item->path) {
        nipc_service_common_cgroups_cache_item_free(item);
        return NULL;
    }

    return item;
}

void nipc_service_common_cgroups_cache_item_free(nipc_cgroups_cache_item_t *item)
{
    if (!item)
        return;

    free(item->name);
    free(item->path);
    free(item);
}

void nipc_service_common_cgroups_cache_status(nipc_cgroups_cache_t *cache,
                                              nipc_cgroups_cache_status_t *out)
{
    if (!out)
        return;

    memset(out, 0, sizeof(*out));

    if (!cache_writer_lock(cache))
        return;
    if (!cache_read_lock(cache)) {
        cache_writer_unlock(cache);
        return;
    }

    const nipc_cgroups_cache_snapshot_t *snapshot = cache->snapshot;
    out->populated = snapshot != NULL;
    if (snapshot) {
        out->item_count = snapshot->item_count;
        out->systemd_enabled = snapshot->systemd_enabled;
        out->generation = snapshot->generation;
    }
    out->refresh_success_count = cache->refresh_success_count;
    out->refresh_failure_count = cache->refresh_failure_count;
    out->connection_state = cache->client.state;
    out->last_refresh_ts = cache->last_refresh_ts;

    cache_read_unlock(cache);
    cache_writer_unlock(cache);
}

bool nipc_service_common_cgroups_cache_seed_for_tests(
    nipc_cgroups_cache_t *cache,
    const nipc_cgroups_cache_item_t *items,
    uint32_t item_count,
    uint32_t systemd_enabled,
    uint64_t generation,
    const nipc_service_common_cache_ops_t *ops)
{
    if (!cache || !ops || (!items && item_count > 0))
        return false;

    nipc_cgroups_cache_snapshot_t *new_snapshot =
        cache_snapshot_build_from_items(items, item_count,
                                        systemd_enabled, generation, ops);
    if (!new_snapshot)
        return false;

    if (!cache_writer_lock(cache)) {
        cache_snapshot_free(new_snapshot);
        return false;
    }

    nipc_cgroups_cache_snapshot_t *old_snapshot = NULL;
    if (!cache_write_lock(cache)) {
        cache_writer_unlock(cache);
        cache_snapshot_free(new_snapshot);
        return false;
    }

    old_snapshot = cache->snapshot;
    cache->snapshot = new_snapshot;
    cache->refresh_success_count++;
    cache->last_refresh_ts = ops->monotonic_ms_fn();

    cache_write_unlock(cache);
    cache_writer_unlock(cache);
    cache_snapshot_free(old_snapshot);

    return true;
}

void nipc_service_common_cgroups_cache_close(nipc_cgroups_cache_t *cache)
{
    if (!cache)
        return;

    if (cache_writer_lock(cache)) {
        nipc_cgroups_cache_snapshot_t *old_snapshot = NULL;
        if (cache_write_lock(cache)) {
            old_snapshot = cache->snapshot;
            cache->snapshot = NULL;
            cache_write_unlock(cache);
        }
        cache_snapshot_free(old_snapshot);

        free(cache->response_buf);
        cache->response_buf = NULL;
        cache->response_buf_size = 0;

        nipc_client_close(&cache->client);
        cache_writer_unlock(cache);
    }

    cache_destroy_locks(cache);
}
