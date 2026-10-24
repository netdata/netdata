#ifndef NETIPC_SERVICE_CGROUPS_CACHE_COMMON_H
#define NETIPC_SERVICE_CGROUPS_CACHE_COMMON_H

#include "netipc_service_common.h"

#ifdef __cplusplus
extern "C" {
#endif

void nipc_service_common_cgroups_cache_init(nipc_cgroups_cache_t *cache,
                                            const char *run_dir,
                                            const char *service_name,
                                            const nipc_client_config_t *config);
bool nipc_service_common_cgroups_cache_refresh(
    nipc_cgroups_cache_t *cache,
    const nipc_service_common_cache_ops_t *ops);
bool nipc_service_common_cgroups_cache_ready(nipc_cgroups_cache_t *cache);
bool nipc_service_common_cgroups_cache_read_lock(
    nipc_cgroups_cache_t *cache,
    nipc_cgroups_cache_read_guard_t *guard);
void nipc_service_common_cgroups_cache_read_unlock(
    nipc_cgroups_cache_read_guard_t *guard);
const nipc_cgroups_cache_item_view_t *nipc_service_common_cgroups_cache_get(
    const nipc_cgroups_cache_read_guard_t *guard,
    uint32_t hash,
    const char *name);
nipc_cgroups_cache_item_t *nipc_service_common_cgroups_cache_item_dup(
    const nipc_cgroups_cache_read_guard_t *guard,
    const nipc_cgroups_cache_item_view_t *view);
void nipc_service_common_cgroups_cache_item_free(nipc_cgroups_cache_item_t *item);
void nipc_service_common_cgroups_cache_status(nipc_cgroups_cache_t *cache,
                                              nipc_cgroups_cache_status_t *out);
bool nipc_service_common_cgroups_cache_seed_for_tests(
    nipc_cgroups_cache_t *cache,
    const nipc_cgroups_cache_item_t *items,
    uint32_t item_count,
    uint32_t systemd_enabled,
    uint64_t generation,
    const nipc_service_common_cache_ops_t *ops);
void nipc_service_common_cgroups_cache_close(nipc_cgroups_cache_t *cache);

#ifdef __cplusplus
}
#endif

#endif /* NETIPC_SERVICE_CGROUPS_CACHE_COMMON_H */
