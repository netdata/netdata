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

typedef nipc_error_t (*nipc_service_common_attempt_fn)(nipc_client_ctx_t *ctx,
                                                       void *state);
typedef nipc_error_t (*nipc_service_common_transport_send_fn)(
    nipc_client_ctx_t *ctx, nipc_header_t *hdr, const void *payload,
    size_t payload_len);
typedef nipc_error_t (*nipc_service_common_transport_receive_fn)(
    nipc_client_ctx_t *ctx, void *buf, size_t buf_size, nipc_header_t *hdr_out,
    const void **payload_out, size_t *payload_len_out, uint32_t timeout_ms);

typedef struct {
  void (*disconnect)(nipc_client_ctx_t *ctx);
  nipc_client_state_t (*try_connect)(nipc_client_ctx_t *ctx);
  bool (*reconnect_for_call)(nipc_client_ctx_t *ctx);
  void (*sleep_ms)(uint32_t ms);
  uint32_t reconnect_drain_ms;
  uint32_t reconnect_retry_interval_ms;
} nipc_service_common_client_ops_t;

typedef void *(*nipc_service_common_calloc_fn)(size_t count, size_t size,
                                               int fault_site);

typedef struct {
  uint32_t supported_profiles;
  uint32_t preferred_profiles;
  uint32_t max_request_payload_bytes;
  uint32_t max_request_batch_items;
  uint32_t max_response_payload_bytes;
  uint32_t max_response_batch_items;
  uint64_t auth_token;
} nipc_service_common_transport_fields_t;

#define NIPC_SERVICE_COMMON_APPLY_TRANSPORT_FIELDS(dst, fields)                \
  do {                                                                         \
    (dst)->supported_profiles = (fields)->supported_profiles;                  \
    (dst)->preferred_profiles = (fields)->preferred_profiles;                  \
    (dst)->max_request_payload_bytes = (fields)->max_request_payload_bytes;    \
    (dst)->max_request_batch_items = (fields)->max_request_batch_items;        \
    (dst)->max_response_payload_bytes = (fields)->max_response_payload_bytes;  \
    (dst)->max_response_batch_items = (fields)->max_response_batch_items;      \
    (dst)->auth_token = (fields)->auth_token;                                  \
  } while (0)

uint32_t nipc_service_common_next_power_of_2_u32(uint32_t n);
bool nipc_service_common_header_payload_len(size_t payload_len,
                                            size_t *msg_len_out);
bool nipc_service_common_header_payload_len_u32(uint32_t payload_len,
                                                uint32_t *msg_len_out);
bool nipc_service_common_mul_would_overflow(size_t count, size_t size);
uint32_t nipc_service_common_request_payload_default(void);
uint32_t nipc_service_common_response_payload_default(void);
uint32_t nipc_service_common_logical_lookup_items_default(void);
uint32_t nipc_service_common_logical_lookup_subcalls_default(void);
uint32_t nipc_service_common_logical_lookup_response_bytes_default(void);
uint32_t nipc_service_common_typed_response_batch_items(
    uint32_t max_request_batch_items);
void nipc_service_common_copy_cstr_field(char *dst, size_t dst_size,
                                         const char *src);
bool nipc_service_common_client_transport_fields(
    nipc_service_common_transport_fields_t *fields,
    const nipc_client_config_t *config);
bool nipc_service_common_server_transport_fields(
    nipc_service_common_transport_fields_t *fields,
    const nipc_server_config_t *config);

void nipc_service_common_client_init(nipc_client_ctx_t *ctx,
                                     const char *run_dir,
                                     const char *service_name);
void nipc_service_common_client_status(const nipc_client_ctx_t *ctx,
                                       nipc_client_status_t *out);
void nipc_service_common_client_close_buffers(nipc_client_ctx_t *ctx);
void nipc_service_common_client_apply_logical_lookup_config(
    nipc_client_ctx_t *ctx, const nipc_client_config_t *config);
uint32_t
nipc_service_common_client_call_timeout_ms(const nipc_client_ctx_t *ctx,
                                           uint32_t timeout_ms);
bool nipc_service_common_client_abort_requested(const nipc_client_ctx_t *ctx);
bool nipc_service_common_client_refresh(
    nipc_client_ctx_t *ctx, const nipc_service_common_client_ops_t *ops);
void nipc_service_common_client_note_request_capacity(nipc_client_ctx_t *ctx,
                                                      uint32_t payload_len);
void nipc_service_common_client_note_response_capacity(nipc_client_ctx_t *ctx,
                                                       uint32_t payload_len);
nipc_error_t nipc_service_common_client_prepare_shm_request(
    nipc_client_ctx_t *ctx, nipc_header_t *hdr, const void *payload,
    size_t payload_len, uint8_t **msg_out, size_t *msg_len_out);
nipc_error_t nipc_service_common_client_parse_shm_response(
    void *buf, size_t msg_len, nipc_header_t *hdr_out, const void **payload_out,
    size_t *payload_len_out);
nipc_error_t
nipc_service_common_response_status_to_error(nipc_client_ctx_t *ctx,
                                             const nipc_header_t *resp_hdr);
nipc_error_t nipc_service_common_do_raw_call(
    nipc_client_ctx_t *ctx, uint16_t method_code, const void *request_payload,
    size_t request_len, const void **response_payload_out,
    size_t *response_len_out, uint32_t timeout_ms,
    nipc_service_common_transport_send_fn send_fn,
    nipc_service_common_transport_receive_fn receive_fn);
nipc_error_t nipc_service_common_call_with_retry(
    nipc_client_ctx_t *ctx, nipc_service_common_attempt_fn attempt, void *state,
    const nipc_service_common_client_ops_t *ops);
nipc_error_t nipc_service_common_client_ensure_request_capacity(
    nipc_client_ctx_t *ctx, size_t required_payload_bytes,
    const nipc_service_common_client_ops_t *ops);

void nipc_service_common_server_note_request_capacity(
    nipc_managed_server_t *server, uint32_t payload_len);
void nipc_service_common_server_note_response_capacity(
    nipc_managed_server_t *server, uint32_t payload_len);
nipc_error_t nipc_service_common_server_init_base(
    nipc_managed_server_t *server, const char *run_dir,
    const char *service_name, int worker_count, uint16_t expected_method_code,
    nipc_server_handler_fn handler, void *user,
    uint32_t max_request_payload_bytes, uint32_t max_response_payload_bytes);
nipc_error_t nipc_service_common_server_alloc_sessions(
    nipc_managed_server_t *server, nipc_service_common_calloc_fn calloc_fn,
    int fault_site);
void nipc_service_common_prepare_response_header(
    const nipc_header_t *request_hdr, nipc_header_t *resp_hdr);
void nipc_service_common_apply_dispatch_result(
    nipc_managed_server_t *server, nipc_error_t dispatch_err,
    size_t resp_buf_size, uint32_t max_response_payload_bytes,
    bool check_header_room, nipc_header_t *resp_hdr, size_t *response_len,
    bool *close_after_response);

#ifdef __cplusplus
}
#endif

#endif /* NETIPC_SERVICE_COMMON_H */
