#ifndef NETIPC_SERVICE_PLATFORM_H
#define NETIPC_SERVICE_PLATFORM_H

#include "netipc_service_common.h"

#ifdef __cplusplus
extern "C" {
#endif

#if defined(_WIN32) || defined(__MSYS__)
typedef nipc_np_server_config_t nipc_service_platform_server_config_t;
#else
typedef nipc_uds_server_config_t nipc_service_platform_server_config_t;
#endif

typedef nipc_service_common_attempt_fn nipc_service_platform_attempt_fn;

enum {
  NIPC_SERVICE_PLATFORM_TEST_FAULT_CACHE_BUCKETS_CALLOC_INTERNAL = 10,
  NIPC_SERVICE_PLATFORM_TEST_FAULT_CACHE_ITEMS_CALLOC_INTERNAL,
  NIPC_SERVICE_PLATFORM_TEST_FAULT_CACHE_ITEM_NAME_MALLOC_INTERNAL,
  NIPC_SERVICE_PLATFORM_TEST_FAULT_CACHE_ITEM_PATH_MALLOC_INTERNAL,
};

void *nipc_service_platform_malloc(size_t size, int fault_site);
void *nipc_service_platform_calloc(size_t count, size_t size, int fault_site);
uint64_t nipc_service_platform_monotonic_ms(void);

bool nipc_service_platform_ensure_client_send_buffer(nipc_client_ctx_t *ctx,
                                                     size_t need);
bool nipc_service_platform_ensure_client_response_buffer(nipc_client_ctx_t *ctx,
                                                         size_t need);

nipc_error_t nipc_service_platform_do_raw_call(
    nipc_client_ctx_t *ctx, uint16_t method_code, const void *request_payload,
    size_t request_len, const void **response_payload_out,
    size_t *response_len_out, uint32_t timeout_ms);

nipc_error_t
nipc_service_platform_call_with_retry(nipc_client_ctx_t *ctx,
                                      nipc_service_platform_attempt_fn attempt,
                                      void *state);
nipc_error_t nipc_service_platform_ensure_request_capacity(
    nipc_client_ctx_t *ctx, size_t required_payload_bytes);

void nipc_service_platform_server_config_from_service(
    nipc_service_platform_server_config_t *transport,
    const nipc_server_config_t *config);

nipc_error_t nipc_service_platform_server_init_raw(
    nipc_managed_server_t *server, const char *run_dir,
    const char *service_name,
    const nipc_service_platform_server_config_t *config, int worker_count,
    uint16_t expected_method_code, nipc_server_handler_fn handler, void *user);

#ifdef __cplusplus
}
#endif

#endif /* NETIPC_SERVICE_PLATFORM_H */
