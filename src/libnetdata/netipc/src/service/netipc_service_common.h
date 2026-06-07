#ifndef NETIPC_SERVICE_COMMON_H
#define NETIPC_SERVICE_COMMON_H

#include "netipc/netipc_service.h"

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    void *(*malloc_fn)(size_t size, int fault_site);
    void *(*calloc_fn)(size_t count, size_t size, int fault_site);
    uint64_t (*monotonic_ms_fn)(void);
    int cache_buckets_fault_site;
    int cache_items_fault_site;
    int cache_item_name_fault_site;
    int cache_item_path_fault_site;
} nipc_service_common_cache_ops_t;

uint32_t nipc_service_common_next_power_of_2_u32(uint32_t n);
bool nipc_service_common_header_payload_len(size_t payload_len,
                                            size_t *msg_len_out);
bool nipc_service_common_header_payload_len_u32(uint32_t payload_len,
                                                uint32_t *msg_len_out);
bool nipc_service_common_mul_would_overflow(size_t count, size_t size);
uint32_t nipc_service_common_request_payload_default(void);
uint32_t nipc_service_common_response_payload_default(void);
uint32_t nipc_service_common_typed_response_batch_items(uint32_t max_request_batch_items);

void nipc_service_common_client_init(nipc_client_ctx_t *ctx,
                                     const char *run_dir,
                                     const char *service_name);
void nipc_service_common_client_status(const nipc_client_ctx_t *ctx,
                                       nipc_client_status_t *out);
void nipc_service_common_client_close_buffers(nipc_client_ctx_t *ctx);
void nipc_service_common_client_note_request_capacity(nipc_client_ctx_t *ctx,
                                                      uint32_t payload_len);
void nipc_service_common_client_note_response_capacity(nipc_client_ctx_t *ctx,
                                                       uint32_t payload_len);
nipc_error_t nipc_service_common_response_status_to_error(nipc_client_ctx_t *ctx,
                                                          const nipc_header_t *resp_hdr);

void nipc_service_common_server_note_request_capacity(nipc_managed_server_t *server,
                                                      uint32_t payload_len);
void nipc_service_common_server_note_response_capacity(nipc_managed_server_t *server,
                                                       uint32_t payload_len);
void nipc_service_common_prepare_response_header(const nipc_header_t *request_hdr,
                                                 nipc_header_t *resp_hdr);
void nipc_service_common_apply_dispatch_result(nipc_managed_server_t *server,
                                               nipc_error_t dispatch_err,
                                               size_t resp_buf_size,
                                               uint32_t max_response_payload_bytes,
                                               bool check_header_room,
                                               nipc_header_t *resp_hdr,
                                               size_t *response_len,
                                               bool *close_after_response);

#ifdef __cplusplus
}
#endif

#endif /* NETIPC_SERVICE_COMMON_H */
