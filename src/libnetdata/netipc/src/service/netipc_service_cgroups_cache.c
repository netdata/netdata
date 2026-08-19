#include "netipc_service_cgroups_cache_common.h"
#include "netipc_service_platform.h"

static const nipc_service_common_cache_ops_t cgroups_cache_ops = {
    .malloc_fn = nipc_service_platform_malloc,
    .calloc_fn = nipc_service_platform_calloc,
    .monotonic_ms_fn = nipc_service_platform_monotonic_ms,
    .cache_buckets_fault_site =
        NIPC_SERVICE_PLATFORM_TEST_FAULT_CACHE_BUCKETS_CALLOC_INTERNAL,
    .cache_items_fault_site =
        NIPC_SERVICE_PLATFORM_TEST_FAULT_CACHE_ITEMS_CALLOC_INTERNAL,
    .cache_item_name_fault_site =
        NIPC_SERVICE_PLATFORM_TEST_FAULT_CACHE_ITEM_NAME_MALLOC_INTERNAL,
    .cache_item_path_fault_site =
        NIPC_SERVICE_PLATFORM_TEST_FAULT_CACHE_ITEM_PATH_MALLOC_INTERNAL,
};

void nipc_cgroups_cache_init(nipc_cgroups_cache_t *cache,
                             const char *run_dir,
                             const char *service_name,
                             const nipc_client_config_t *config)
{
    nipc_service_common_cgroups_cache_init(cache, run_dir, service_name, config);
}

bool nipc_cgroups_cache_refresh(nipc_cgroups_cache_t *cache)
{
    return nipc_service_common_cgroups_cache_refresh(cache, &cgroups_cache_ops);
}

bool nipc_cgroups_cache_ready(nipc_cgroups_cache_t *cache)
{
    return nipc_service_common_cgroups_cache_ready(cache);
}

bool nipc_cgroups_cache_read_lock(nipc_cgroups_cache_t *cache,
                                  nipc_cgroups_cache_read_guard_t *guard)
{
    return nipc_service_common_cgroups_cache_read_lock(cache, guard);
}

void nipc_cgroups_cache_read_unlock(nipc_cgroups_cache_read_guard_t *guard)
{
    nipc_service_common_cgroups_cache_read_unlock(guard);
}

const nipc_cgroups_cache_item_view_t *
nipc_cgroups_cache_get(const nipc_cgroups_cache_read_guard_t *guard,
                       uint32_t hash,
                       const char *name)
{
    return nipc_service_common_cgroups_cache_get(guard, hash, name);
}

nipc_cgroups_cache_item_t *
nipc_cgroups_cache_item_dup(const nipc_cgroups_cache_read_guard_t *guard,
                            const nipc_cgroups_cache_item_view_t *view)
{
    return nipc_service_common_cgroups_cache_item_dup(guard, view);
}

void nipc_cgroups_cache_item_free(nipc_cgroups_cache_item_t *item)
{
    nipc_service_common_cgroups_cache_item_free(item);
}

void nipc_cgroups_cache_status(nipc_cgroups_cache_t *cache,
                               nipc_cgroups_cache_status_t *out)
{
    nipc_service_common_cgroups_cache_status(cache, out);
}

bool nipc_cgroups_cache_seed_for_tests(nipc_cgroups_cache_t *cache,
                                       const nipc_cgroups_cache_item_t *items,
                                       uint32_t item_count,
                                       uint32_t systemd_enabled,
                                       uint64_t generation)
{
    return nipc_service_common_cgroups_cache_seed_for_tests(
        cache, items, item_count, systemd_enabled, generation,
        &cgroups_cache_ops);
}

void nipc_cgroups_cache_close(nipc_cgroups_cache_t *cache)
{
    nipc_service_common_cgroups_cache_close(cache);
}
