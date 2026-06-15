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
const nipc_cgroups_cache_item_t *nipc_service_common_cgroups_cache_lookup(
    const nipc_cgroups_cache_t *cache,
    uint32_t hash,
    const char *name);
void nipc_service_common_cgroups_cache_status(const nipc_cgroups_cache_t *cache,
                                              nipc_cgroups_cache_status_t *out);
void nipc_service_common_cgroups_cache_close(nipc_cgroups_cache_t *cache);

#ifdef __cplusplus
}
#endif

#endif /* NETIPC_SERVICE_CGROUPS_CACHE_COMMON_H */
