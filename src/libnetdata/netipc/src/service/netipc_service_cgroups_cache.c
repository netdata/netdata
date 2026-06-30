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

const nipc_cgroups_cache_item_t *nipc_cgroups_cache_lookup(
    const nipc_cgroups_cache_t *cache,
    uint32_t hash,
    const char *name)
{
    return nipc_service_common_cgroups_cache_lookup(cache, hash, name);
}

void nipc_cgroups_cache_status(const nipc_cgroups_cache_t *cache,
                               nipc_cgroups_cache_status_t *out)
{
    nipc_service_common_cgroups_cache_status(cache, out);
}

void nipc_cgroups_cache_close(nipc_cgroups_cache_t *cache)
{
    nipc_service_common_cgroups_cache_close(cache);
}
