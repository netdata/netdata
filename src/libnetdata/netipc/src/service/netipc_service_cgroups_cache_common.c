#include "netipc_service_cgroups_cache_common.h"

#include "netipc/netipc_protocol.h"

#include <stdlib.h>
#include <string.h>

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

static uint32_t cache_hash_name(const char *name)
{
    uint32_t h = 5381;
    for (const unsigned char *p = (const unsigned char *)name; *p; p++)
        h = ((h << 5) + h) + *p;
    return h;
}

static bool cache_build_hashtable(nipc_cgroups_cache_t *cache,
                                  const nipc_service_common_cache_ops_t *ops)
{
    free(cache->buckets);
    cache->buckets = NULL;
    cache->bucket_count = 0;

    if (cache->item_count == 0)
        return true;

    uint32_t bcount = nipc_service_common_next_power_of_2_u32(cache->item_count * 2);
    nipc_cgroups_hash_bucket_t *buckets = ops->calloc_fn(
        bcount, sizeof(nipc_cgroups_hash_bucket_t),
        ops->cache_buckets_fault_site);
    if (!buckets)
        return false;

    uint32_t mask = bcount - 1;
    for (uint32_t i = 0; i < cache->item_count; i++) {
        uint32_t key = cache->items[i].hash ^ cache_hash_name(cache->items[i].name);
        uint32_t slot = key & mask;

        while (buckets[slot].used)
            slot = (slot + 1) & mask;

        buckets[slot].index = i;
        buckets[slot].used = true;
    }

    cache->buckets = buckets;
    cache->bucket_count = bcount;
    return true;
}

static nipc_cgroups_cache_item_t *cache_build_items(
    const nipc_cgroups_resp_view_t *view,
    const nipc_service_common_cache_ops_t *ops,
    uint32_t *count_out)
{
    uint32_t n = view->item_count;
    *count_out = 0;

    if (n == 0)
        return NULL;

    nipc_cgroups_cache_item_t *items = ops->calloc_fn(
        n, sizeof(nipc_cgroups_cache_item_t),
        ops->cache_items_fault_site);
    if (!items)
        return NULL;

    for (uint32_t i = 0; i < n; i++) {
        nipc_cgroups_item_view_t iv;
        nipc_error_t err = nipc_cgroups_resp_item(view, i, &iv);
        if (err != NIPC_OK) {
            cache_free_items(items, i);
            return NULL;
        }

        items[i].hash = iv.hash;
        items[i].options = iv.options;
        items[i].enabled = iv.enabled;

        items[i].name = ops->malloc_fn(
            iv.name.len + 1, ops->cache_item_name_fault_site);
        if (!items[i].name) {
            cache_free_items(items, i);
            return NULL;
        }
        if (iv.name.len > 0)
            memcpy(items[i].name, iv.name.ptr, iv.name.len);
        items[i].name[iv.name.len] = '\0';

        items[i].path = ops->malloc_fn(
            iv.path.len + 1, ops->cache_item_path_fault_site);
        if (!items[i].path) {
            free(items[i].name);
            cache_free_items(items, i);
            return NULL;
        }
        if (iv.path.len > 0)
            memcpy(items[i].path, iv.path.ptr, iv.path.len);
        items[i].path[iv.path.len] = '\0';
    }

    *count_out = n;
    return items;
}

void nipc_service_common_cgroups_cache_init(nipc_cgroups_cache_t *cache,
                                            const char *run_dir,
                                            const char *service_name,
                                            const nipc_client_config_t *config)
{
    memset(cache, 0, sizeof(*cache));
    nipc_client_init(&cache->client, run_dir, service_name, config);
}

bool nipc_service_common_cgroups_cache_refresh(
    nipc_cgroups_cache_t *cache,
    const nipc_service_common_cache_ops_t *ops)
{
    nipc_client_refresh(&cache->client);

    nipc_cgroups_resp_view_t view;
    nipc_error_t err = nipc_client_call_cgroups_snapshot(&cache->client, &view);
    if (err != NIPC_OK) {
        cache->refresh_failure_count++;
        return false;
    }

    uint32_t new_count = 0;
    nipc_cgroups_cache_item_t *new_items = NULL;
    if (view.item_count > 0) {
        new_items = cache_build_items(&view, ops, &new_count);
        if (!new_items) {
            cache->refresh_failure_count++;
            return false;
        }
    }

    cache_free_items(cache->items, cache->item_count);
    cache->items = new_items;
    cache->item_count = new_count;
    cache->systemd_enabled = view.systemd_enabled;
    cache->generation = view.generation;
    cache->populated = true;
    cache->refresh_success_count++;
    cache->last_refresh_ts = ops->monotonic_ms_fn();
    cache_build_hashtable(cache, ops);

    return true;
}

const nipc_cgroups_cache_item_t *nipc_service_common_cgroups_cache_lookup(
    const nipc_cgroups_cache_t *cache,
    uint32_t hash,
    const char *name)
{
    if (!cache->populated || !cache->items || !name)
        return NULL;

    if (cache->buckets && cache->bucket_count > 0) {
        uint32_t key = hash ^ cache_hash_name(name);
        uint32_t mask = cache->bucket_count - 1;
        uint32_t slot = key & mask;

        while (cache->buckets[slot].used) {
            uint32_t idx = cache->buckets[slot].index;
            if (cache->items[idx].hash == hash &&
                strcmp(cache->items[idx].name, name) == 0)
                return &cache->items[idx];
            slot = (slot + 1) & mask;
        }
        return NULL;
    }

    for (uint32_t i = 0; i < cache->item_count; i++) {
        if (cache->items[i].hash == hash &&
            strcmp(cache->items[i].name, name) == 0)
            return &cache->items[i];
    }

    return NULL;
}

void nipc_service_common_cgroups_cache_status(const nipc_cgroups_cache_t *cache,
                                              nipc_cgroups_cache_status_t *out)
{
    out->populated = cache->populated;
    out->item_count = cache->item_count;
    out->systemd_enabled = cache->systemd_enabled;
    out->generation = cache->generation;
    out->refresh_success_count = cache->refresh_success_count;
    out->refresh_failure_count = cache->refresh_failure_count;
    out->connection_state = cache->client.state;
    out->last_refresh_ts = cache->last_refresh_ts;
}

void nipc_service_common_cgroups_cache_close(nipc_cgroups_cache_t *cache)
{
    cache_free_items(cache->items, cache->item_count);
    cache->items = NULL;
    cache->item_count = 0;
    cache->populated = false;

    free(cache->buckets);
    cache->buckets = NULL;
    cache->bucket_count = 0;

    free(cache->response_buf);
    cache->response_buf = NULL;
    cache->response_buf_size = 0;

    nipc_client_close(&cache->client);
}
