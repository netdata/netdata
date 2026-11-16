#include "netipc_service_common.h"
#include "netipc_service_platform.h"

#include "netipc/netipc_protocol.h"

#include <stdlib.h>
#include <string.h>

#define NIPC_SERVICE_RESPONSE_BUF_DEFAULT 65536u

uint32_t nipc_service_common_next_power_of_2_u32(uint32_t n) {
  if (n < 16)
    return 16;
  n--;
  n |= n >> 1;
  n |= n >> 2;
  n |= n >> 4;
  n |= n >> 8;
  n |= n >> 16;
  return n + 1;
}

static uint32_t nipc_service_common_payload_growth_ceiling(
    uint32_t configured_payload_bytes) {
  return configured_payload_bytes > 0 ? configured_payload_bytes
                                      : NIPC_MAX_PAYLOAD_CAP;
}

bool nipc_service_common_header_payload_len(size_t payload_len,
                                            size_t *msg_len_out) {
#if SIZE_MAX <= UINT32_MAX
  if (payload_len > SIZE_MAX - NIPC_HEADER_LEN)
    return false;
#endif

  *msg_len_out = NIPC_HEADER_LEN + payload_len;
  return true;
}

bool nipc_service_common_header_payload_len_u32(uint32_t payload_len,
                                                uint32_t *msg_len_out) {
  if (payload_len > UINT32_MAX - NIPC_HEADER_LEN)
    return false;

  *msg_len_out = payload_len + NIPC_HEADER_LEN;
  return true;
}

bool nipc_service_common_mul_would_overflow(size_t count, size_t size) {
  return size != 0 && count > SIZE_MAX / size;
}

uint32_t nipc_service_common_request_payload_default(void) {
  return NIPC_MAX_PAYLOAD_DEFAULT;
}

uint32_t nipc_service_common_response_payload_default(void) {
  return NIPC_SERVICE_RESPONSE_BUF_DEFAULT;
}

uint32_t nipc_service_common_logical_lookup_items_default(void) {
  return NIPC_LOGICAL_LOOKUP_ITEMS_DEFAULT;
}

uint32_t nipc_service_common_logical_lookup_subcalls_default(void) {
  return NIPC_LOGICAL_LOOKUP_SUBCALLS_DEFAULT;
}

uint32_t nipc_service_common_logical_lookup_response_bytes_default(void) {
  return NIPC_LOGICAL_LOOKUP_RESPONSE_BYTES_DEFAULT;
}

/* Typed services expose one batch-count knob; level 1 keeps request/response
 * counts symmetric. */
uint32_t nipc_service_common_typed_response_batch_items(
    uint32_t max_request_batch_items) {
  return max_request_batch_items;
}

void nipc_service_common_copy_cstr_field(char *dst, size_t dst_size,
                                         const char *src) {
  if (!dst || dst_size == 0)
    return;

  dst[0] = '\0';
  if (!src)
    return;

  size_t len = 0;
  size_t max_copy = dst_size - 1;
  while (len < max_copy && src[len] != '\0')
    len++;

  memcpy(dst, src, len);
  dst[len] = '\0';
}

bool nipc_service_common_client_transport_fields(
    nipc_service_common_transport_fields_t *fields,
    const nipc_client_config_t *config) {
  memset(fields, 0, sizeof(*fields));
  if (!config)
    return false;

  fields->supported_profiles = config->supported_profiles;
  fields->preferred_profiles = config->preferred_profiles;
  fields->max_request_payload_bytes = config->max_request_payload_bytes;
  fields->max_request_batch_items = config->max_request_batch_items;
  fields->max_response_payload_bytes = config->max_response_payload_bytes;
  fields->max_response_batch_items =
      nipc_service_common_typed_response_batch_items(
          config->max_request_batch_items);
  fields->auth_token = config->auth_token;
  return true;
}

bool nipc_service_common_server_transport_fields(
    nipc_service_common_transport_fields_t *fields,
    const nipc_server_config_t *config) {
  memset(fields, 0, sizeof(*fields));
  if (!config)
    return false;

  fields->supported_profiles = config->supported_profiles;
  fields->preferred_profiles = config->preferred_profiles;
  fields->max_request_payload_bytes = config->max_request_payload_bytes;
  fields->max_request_batch_items = config->max_request_batch_items;
  fields->max_response_payload_bytes = config->max_response_payload_bytes;
  fields->max_response_batch_items =
      nipc_service_common_typed_response_batch_items(
          config->max_request_batch_items);
  fields->auth_token = config->auth_token;
  return true;
}

void nipc_service_common_client_init(nipc_client_ctx_t *ctx,
                                     const char *run_dir,
                                     const char *service_name) {
  memset(ctx, 0, sizeof(*ctx));
  ctx->state = NIPC_CLIENT_DISCONNECTED;
  ctx->session_valid = false;
  ctx->shm = NULL;
  ctx->call_timeout_ms = NIPC_CLIENT_CALL_TIMEOUT_DEFAULT_MS;
  ctx->shm_attach_timeout_ms = NIPC_CLIENT_SHM_ATTACH_TIMEOUT_DEFAULT_MS;
  nipc_service_common_copy_cstr_field(ctx->run_dir, sizeof(ctx->run_dir),
                                      run_dir);
  nipc_service_common_copy_cstr_field(ctx->service_name,
                                      sizeof(ctx->service_name), service_name);
}

void nipc_service_common_client_status(const nipc_client_ctx_t *ctx,
                                       nipc_client_status_t *out) {
  out->state = ctx->state;
  out->connect_count = ctx->connect_count;
  out->reconnect_count = ctx->reconnect_count;
  out->call_count = ctx->call_count;
  out->error_count = ctx->error_count;
}

void nipc_service_common_client_close_buffers(nipc_client_ctx_t *ctx) {
  free(ctx->response_buf);
  free(ctx->send_buf);
  ctx->response_buf = NULL;
  ctx->send_buf = NULL;
  ctx->response_buf_size = 0;
  ctx->send_buf_size = 0;
  ctx->state = NIPC_CLIENT_DISCONNECTED;
}

void nipc_service_common_client_apply_logical_lookup_config(
    nipc_client_ctx_t *ctx, const nipc_client_config_t *config) {
  ctx->max_logical_lookup_items =
      (config && config->max_logical_lookup_items != 0)
          ? config->max_logical_lookup_items
          : nipc_service_common_logical_lookup_items_default();
  ctx->max_logical_lookup_subcalls =
      (config && config->max_logical_lookup_subcalls != 0)
          ? config->max_logical_lookup_subcalls
          : nipc_service_common_logical_lookup_subcalls_default();
  ctx->max_logical_lookup_response_bytes =
      (config && config->max_logical_lookup_response_bytes != 0)
          ? config->max_logical_lookup_response_bytes
          : nipc_service_common_logical_lookup_response_bytes_default();
}

uint32_t
nipc_service_common_client_call_timeout_ms(const nipc_client_ctx_t *ctx,
                                           uint32_t timeout_ms) {
  if (timeout_ms != 0)
    return timeout_ms;
  if (ctx->call_timeout_ms != 0)
    return ctx->call_timeout_ms;
  return NIPC_CLIENT_CALL_TIMEOUT_DEFAULT_MS;
}

bool nipc_service_common_client_abort_requested(const nipc_client_ctx_t *ctx) {
  return __atomic_load_n(&ctx->abort_requested, __ATOMIC_ACQUIRE) != 0;
}

void nipc_service_common_client_begin_op(nipc_client_ctx_t *ctx,
                                         uint32_t timeout_ms) {
  ctx->op_deadline_ms =
      nipc_service_platform_monotonic_ms() +
      nipc_service_common_client_call_timeout_ms(ctx, timeout_ms);
}

uint32_t
nipc_service_common_client_op_remaining_ms(const nipc_client_ctx_t *ctx) {
  uint64_t now = nipc_service_platform_monotonic_ms();
  if (now >= ctx->op_deadline_ms)
    return 0;
  uint64_t left = ctx->op_deadline_ms - now;
  return left > UINT32_MAX ? UINT32_MAX : (uint32_t)left;
}

bool nipc_service_common_client_refresh(
    nipc_client_ctx_t *ctx, const nipc_service_common_client_ops_t *ops) {
  nipc_client_state_t old_state = ctx->state;

  /* A pending abort keeps the context offline until it is cleared. */
  if (nipc_service_common_client_abort_requested(ctx))
    return false;
  nipc_service_common_client_begin_op(ctx, 0);

  switch (ctx->state) {
  case NIPC_CLIENT_DISCONNECTED:
  case NIPC_CLIENT_NOT_FOUND:
    ctx->state = NIPC_CLIENT_CONNECTING;
    ctx->state = ops->try_connect(ctx);
    if (ctx->state == NIPC_CLIENT_READY)
      ctx->connect_count++;
    break;

  case NIPC_CLIENT_BROKEN:
    ops->disconnect(ctx);
    ctx->state = NIPC_CLIENT_CONNECTING;
    ctx->state = ops->try_connect(ctx);
    if (ctx->state == NIPC_CLIENT_READY)
      ctx->reconnect_count++;
    break;

  case NIPC_CLIENT_READY:
  case NIPC_CLIENT_CONNECTING:
  case NIPC_CLIENT_AUTH_FAILED:
  case NIPC_CLIENT_INCOMPATIBLE:
    break;
  }

  return ctx->state != old_state;
}

void nipc_service_common_client_note_request_capacity(nipc_client_ctx_t *ctx,
                                                      uint32_t payload_len) {
  uint32_t grown = nipc_service_common_next_power_of_2_u32(payload_len);
  if (grown > NIPC_MAX_PAYLOAD_CAP)
    grown = NIPC_MAX_PAYLOAD_CAP;
  if (grown > ctx->transport_config.max_request_payload_bytes)
    ctx->transport_config.max_request_payload_bytes = grown;
}

void nipc_service_common_client_note_response_capacity(nipc_client_ctx_t *ctx,
                                                       uint32_t payload_len) {
  uint32_t grown = nipc_service_common_next_power_of_2_u32(payload_len);
  if (grown > NIPC_MAX_PAYLOAD_CAP)
    grown = NIPC_MAX_PAYLOAD_CAP;
  if (grown > ctx->transport_config.max_response_payload_bytes)
    ctx->transport_config.max_response_payload_bytes = grown;
}

nipc_error_t nipc_service_common_client_prepare_shm_request(
    nipc_client_ctx_t *ctx, nipc_header_t *hdr, const void *payload,
    size_t payload_len, uint8_t **msg_out, size_t *msg_len_out) {
  if (payload_len > UINT32_MAX)
    return NIPC_ERR_OVERFLOW;

  if (payload_len > ctx->session.max_request_payload_bytes) {
    nipc_service_common_client_note_request_capacity(ctx,
                                                     (uint32_t)payload_len);
    return NIPC_ERR_OVERFLOW;
  }

  size_t msg_len;
  if (!nipc_service_common_header_payload_len(payload_len, &msg_len))
    return NIPC_ERR_OVERFLOW;

  uint8_t *msg = ctx->send_buf;
  if (!msg || msg_len > ctx->send_buf_size)
    return NIPC_ERR_OVERFLOW;

  if (payload_len > 0)
    memmove(msg + NIPC_HEADER_LEN, payload, payload_len);

  hdr->magic = NIPC_MAGIC_MSG;
  hdr->version = NIPC_VERSION;
  hdr->header_len = NIPC_HEADER_LEN;
  hdr->payload_len = (uint32_t)payload_len;

  nipc_header_encode(hdr, msg, NIPC_HEADER_LEN);

  *msg_out = msg;
  *msg_len_out = msg_len;
  return NIPC_OK;
}

nipc_error_t nipc_service_common_client_parse_shm_response(
    void *buf, size_t msg_len, nipc_header_t *hdr_out, const void **payload_out,
    size_t *payload_len_out) {
  if (msg_len < NIPC_HEADER_LEN)
    return NIPC_ERR_TRUNCATED;

  nipc_error_t perr = nipc_header_decode(buf, msg_len, hdr_out);
  if (perr != NIPC_OK)
    return perr;

  *payload_out = (const uint8_t *)buf + NIPC_HEADER_LEN;
  *payload_len_out = msg_len - NIPC_HEADER_LEN;
  return NIPC_OK;
}

nipc_error_t
nipc_service_common_response_status_to_error(nipc_client_ctx_t *ctx,
                                             const nipc_header_t *resp_hdr) {
  switch (resp_hdr->transport_status) {
  case NIPC_STATUS_OK:
    return NIPC_OK;
  case NIPC_STATUS_LIMIT_EXCEEDED:
    if (ctx->session.max_response_payload_bytes > 0) {
      uint32_t current = ctx->session.max_response_payload_bytes;
      nipc_service_common_client_note_response_capacity(
          ctx, current >= UINT32_MAX / 2u ? UINT32_MAX : current * 2u);
    }
    return NIPC_ERR_OVERFLOW;
  case NIPC_STATUS_UNSUPPORTED:
    return NIPC_ERR_BAD_LAYOUT;
  case NIPC_STATUS_BAD_ENVELOPE:
  case NIPC_STATUS_INTERNAL_ERROR:
  default:
    return NIPC_ERR_BAD_LAYOUT;
  }
}

nipc_error_t nipc_service_common_do_raw_call(
    nipc_client_ctx_t *ctx, uint16_t method_code, const void *request_payload,
    size_t request_len, const void **response_payload_out,
    size_t *response_len_out, uint32_t timeout_ms,
    nipc_service_common_transport_send_fn send_fn,
    nipc_service_common_transport_receive_fn receive_fn) {
  if (nipc_service_common_client_abort_requested(ctx))
    return NIPC_ERR_ABORTED;

  uint32_t remaining = nipc_service_common_client_op_remaining_ms(ctx);
  if (remaining == 0)
    return NIPC_ERR_TIMEOUT;
  if (timeout_ms == 0 || timeout_ms > remaining)
    timeout_ms = remaining;

  nipc_header_t hdr = {0};
  hdr.kind = NIPC_KIND_REQUEST;
  hdr.code = method_code;
  hdr.flags = 0;
  hdr.item_count = 1;
  hdr.message_id = (uint64_t)(ctx->call_count + 1);
  hdr.transport_status = NIPC_STATUS_OK;

  uint64_t attempt_deadline = nipc_service_platform_monotonic_ms() + timeout_ms;
  nipc_error_t err =
      send_fn(ctx, &hdr, request_payload, request_len, timeout_ms);
  if (err != NIPC_OK)
    return err;

  /* The send may have waited for buffer space: the receive gets what is
   * left of this attempt's budget. */
  uint64_t now = nipc_service_platform_monotonic_ms();
  if (now >= attempt_deadline)
    return NIPC_ERR_TIMEOUT;
  timeout_ms = (uint32_t)(attempt_deadline - now);

  nipc_header_t resp_hdr;
  err = receive_fn(ctx, ctx->response_buf, ctx->response_buf_size, &resp_hdr,
                   response_payload_out, response_len_out, timeout_ms);
  if (err != NIPC_OK)
    return err;

  if (resp_hdr.kind != NIPC_KIND_RESPONSE)
    return NIPC_ERR_BAD_KIND;
  if (resp_hdr.code != method_code)
    return NIPC_ERR_BAD_LAYOUT;
  if (resp_hdr.message_id != hdr.message_id)
    return NIPC_ERR_BAD_LAYOUT;
  return nipc_service_common_response_status_to_error(ctx, &resp_hdr);
}

/* A reconnect inside a call failed: report why the call ended. */
static nipc_error_t call_reconnect_failure(const nipc_client_ctx_t *ctx,
                                           nipc_error_t err) {
  if (nipc_service_common_client_abort_requested(ctx))
    return NIPC_ERR_ABORTED;
  if (nipc_service_common_client_op_remaining_ms(ctx) == 0)
    return NIPC_ERR_TIMEOUT;
  return err;
}

static void call_mark_broken(nipc_client_ctx_t *ctx,
                             const nipc_service_common_client_ops_t *ops) {
  ops->disconnect(ctx);
  ctx->state = NIPC_CLIENT_BROKEN;
}

nipc_error_t nipc_service_common_call_with_retry(
    nipc_client_ctx_t *ctx, nipc_service_common_attempt_fn attempt, void *state,
    uint32_t timeout_ms, const nipc_service_common_client_ops_t *ops) {
  if (ctx->state != NIPC_CLIENT_READY) {
    ctx->error_count++;
    return NIPC_ERR_NOT_READY;
  }
  nipc_service_common_client_begin_op(ctx, timeout_ms);

  int overflow_retries = 0;
  for (;;) {
    uint32_t prev_req = ctx->session.max_request_payload_bytes;
    uint32_t prev_resp = ctx->session.max_response_payload_bytes;
    uint32_t prev_cfg_req = ctx->transport_config.max_request_payload_bytes;
    uint32_t prev_cfg_resp = ctx->transport_config.max_response_payload_bytes;

    nipc_error_t err = attempt(ctx, state);
    if (err == NIPC_OK) {
      ctx->call_count++;
      return NIPC_OK;
    }

    if (err == NIPC_ERR_TIMEOUT || err == NIPC_ERR_ABORTED) {
      call_mark_broken(ctx, ops);
      ctx->error_count++;
      return err;
    }

    if (err != NIPC_ERR_OVERFLOW) {
      call_mark_broken(ctx, ops);
      if (!ops->reconnect_for_call(ctx)) {
        ctx->error_count++;
        return call_reconnect_failure(ctx, err);
      }

      ctx->reconnect_count++;
      err = attempt(ctx, state);
      if (err == NIPC_OK) {
        ctx->call_count++;
        return NIPC_OK;
      }

      call_mark_broken(ctx, ops);
      ctx->error_count++;
      return err;
    }

    call_mark_broken(ctx, ops);
    if (!ops->reconnect_for_call(ctx)) {
      ctx->error_count++;
      return call_reconnect_failure(ctx, err);
    }
    ctx->reconnect_count++;

    if (ctx->session.max_request_payload_bytes <= prev_req &&
        ctx->session.max_response_payload_bytes <= prev_resp &&
        ctx->transport_config.max_request_payload_bytes <= prev_cfg_req &&
        ctx->transport_config.max_response_payload_bytes <= prev_cfg_resp) {
      call_mark_broken(ctx, ops);
      ctx->error_count++;
      return err;
    }

    if (++overflow_retries >= 8) {
      call_mark_broken(ctx, ops);
      ctx->error_count++;
      return err;
    }
  }
}

nipc_error_t nipc_service_common_client_ensure_request_capacity(
    nipc_client_ctx_t *ctx, size_t required_payload_bytes, uint32_t timeout_ms,
    const nipc_service_common_client_ops_t *ops) {
  if (ctx->state != NIPC_CLIENT_READY) {
    ctx->error_count++;
    return NIPC_ERR_NOT_READY;
  }
  if (nipc_service_common_client_abort_requested(ctx)) {
    call_mark_broken(ctx, ops);
    ctx->error_count++;
    return NIPC_ERR_ABORTED;
  }
  if (required_payload_bytes > UINT32_MAX)
    return NIPC_ERR_OVERFLOW;

  uint32_t required = (uint32_t)required_payload_bytes;
  if (ctx->session.max_request_payload_bytes >= required)
    return NIPC_OK;
  if (ctx->transport_config.max_request_payload_bytes < required)
    return NIPC_ERR_OVERFLOW;

  nipc_service_common_client_begin_op(ctx, timeout_ms);
  uint32_t prev_req = ctx->session.max_request_payload_bytes;
  call_mark_broken(ctx, ops);
  if (!ops->reconnect_for_call(ctx)) {
    ctx->error_count++;
    return call_reconnect_failure(ctx, NIPC_ERR_OVERFLOW);
  }
  ctx->reconnect_count++;

  if (ctx->session.max_request_payload_bytes <= prev_req ||
      ctx->session.max_request_payload_bytes < required) {
    call_mark_broken(ctx, ops);
    ctx->error_count++;
    return NIPC_ERR_OVERFLOW;
  }

  return NIPC_OK;
}

nipc_error_t nipc_service_common_server_init_base(
    nipc_managed_server_t *server, const char *run_dir,
    const char *service_name, int worker_count, uint16_t expected_method_code,
    nipc_server_handler_fn handler, void *user,
    uint32_t max_request_payload_bytes, uint32_t max_response_payload_bytes) {
  if (!run_dir || !service_name || !handler)
    return NIPC_ERR_BAD_LAYOUT;

  if (worker_count < 1)
    worker_count = 1;

  nipc_service_common_copy_cstr_field(server->run_dir, sizeof(server->run_dir),
                                      run_dir);
  nipc_service_common_copy_cstr_field(
      server->service_name, sizeof(server->service_name), service_name);

  server->handler = handler;
  server->handler_user = user;
  server->worker_count = worker_count;
  server->poll_ms = NIPC_SERVER_POLL_DEFAULT_MS;
  server->stop_grace_ms = NIPC_SERVER_STOP_GRACE_DEFAULT_MS;
  server->handshake_ms = NIPC_SERVER_HANDSHAKE_TIMEOUT_MS;
  server->expected_method_code = expected_method_code;
  server->learned_request_payload_bytes = max_request_payload_bytes > 0
                                              ? max_request_payload_bytes
                                              : NIPC_MAX_PAYLOAD_DEFAULT;
  server->learned_response_payload_bytes = max_response_payload_bytes > 0
                                               ? max_response_payload_bytes
                                               : NIPC_MAX_PAYLOAD_DEFAULT;
  server->request_payload_growth_ceiling =
      nipc_service_common_payload_growth_ceiling(max_request_payload_bytes);
  server->response_payload_growth_ceiling =
      nipc_service_common_payload_growth_ceiling(max_response_payload_bytes);
  server->session_capacity = worker_count * 2;
  if (server->session_capacity < 16)
    server->session_capacity = 16;
  server->session_count = 0;
  server->next_session_id = 1;
  return NIPC_OK;
}

nipc_error_t nipc_service_common_server_alloc_sessions(
    nipc_managed_server_t *server, nipc_service_common_calloc_fn calloc_fn,
    int fault_site) {
  server->sessions = calloc_fn((size_t)server->session_capacity,
                               sizeof(nipc_session_ctx_t *), fault_site);
  return server->sessions ? NIPC_OK : NIPC_ERR_OVERFLOW;
}

void nipc_service_common_server_note_request_capacity(
    nipc_managed_server_t *server, uint32_t payload_len) {
  uint32_t grown = nipc_service_common_next_power_of_2_u32(payload_len);
  if (grown > server->request_payload_growth_ceiling)
    grown = server->request_payload_growth_ceiling;
#if defined(_WIN32) || defined(__MSYS__)
  uint32_t current = (uint32_t)InterlockedCompareExchange(
      (volatile LONG *)&server->learned_request_payload_bytes, 0, 0);
  while (grown > current) {
    uint32_t previous = (uint32_t)InterlockedCompareExchange(
        (volatile LONG *)&server->learned_request_payload_bytes, (LONG)grown,
        (LONG)current);
    if (previous == current)
      break;
    current = previous;
  }
#else
  uint32_t current =
      __atomic_load_n(&server->learned_request_payload_bytes, __ATOMIC_RELAXED);
  while (grown > current &&
         !__atomic_compare_exchange_n(&server->learned_request_payload_bytes,
                                      &current, grown, false, __ATOMIC_RELEASE,
                                      __ATOMIC_RELAXED)) {
  }
#endif
}

void nipc_service_common_server_note_response_capacity(
    nipc_managed_server_t *server, uint32_t payload_len) {
  uint32_t grown = nipc_service_common_next_power_of_2_u32(payload_len);
  if (grown > server->response_payload_growth_ceiling)
    grown = server->response_payload_growth_ceiling;
#if defined(_WIN32) || defined(__MSYS__)
  uint32_t current = (uint32_t)InterlockedCompareExchange(
      (volatile LONG *)&server->learned_response_payload_bytes, 0, 0);
  while (grown > current) {
    uint32_t previous = (uint32_t)InterlockedCompareExchange(
        (volatile LONG *)&server->learned_response_payload_bytes, (LONG)grown,
        (LONG)current);
    if (previous == current)
      break;
    current = previous;
  }
#else
  uint32_t current = __atomic_load_n(&server->learned_response_payload_bytes,
                                     __ATOMIC_RELAXED);
  while (grown > current &&
         !__atomic_compare_exchange_n(&server->learned_response_payload_bytes,
                                      &current, grown, false, __ATOMIC_RELEASE,
                                      __ATOMIC_RELAXED)) {
  }
#endif
}

void nipc_service_common_prepare_response_header(
    const nipc_header_t *request_hdr, nipc_header_t *resp_hdr) {
  memset(resp_hdr, 0, sizeof(*resp_hdr));
  resp_hdr->kind = NIPC_KIND_RESPONSE;
  resp_hdr->code = request_hdr->code;
  resp_hdr->message_id = request_hdr->message_id;
  if ((request_hdr->flags & NIPC_FLAG_BATCH) && request_hdr->item_count >= 1) {
    resp_hdr->item_count = request_hdr->item_count;
    resp_hdr->flags = NIPC_FLAG_BATCH;
  } else {
    resp_hdr->item_count = 1;
    resp_hdr->flags = 0;
  }
}

void nipc_service_common_apply_dispatch_result(
    nipc_managed_server_t *server, nipc_error_t dispatch_err,
    size_t resp_buf_size, uint32_t max_response_payload_bytes,
    bool check_header_room, nipc_header_t *resp_hdr, size_t *response_len,
    bool *close_after_response) {
  *close_after_response = false;

  switch (dispatch_err) {
  case NIPC_OK:
    if (*response_len > resp_buf_size ||
        *response_len > max_response_payload_bytes ||
        (check_header_room && *response_len > SIZE_MAX - NIPC_HEADER_LEN)) {
      nipc_service_common_server_note_response_capacity(
          server,
          *response_len >= UINT32_MAX ? UINT32_MAX : (uint32_t)*response_len);
      resp_hdr->transport_status = NIPC_STATUS_LIMIT_EXCEEDED;
      *close_after_response = true;
      *response_len = 0;
    } else {
      if (*response_len <= UINT32_MAX)
        nipc_service_common_server_note_response_capacity(
            server, (uint32_t)*response_len);
      resp_hdr->transport_status = NIPC_STATUS_OK;
    }
    break;
  case NIPC_ERR_OVERFLOW:
    if (max_response_payload_bytes >= UINT32_MAX / 2u)
      nipc_service_common_server_note_response_capacity(server, UINT32_MAX);
    else
      nipc_service_common_server_note_response_capacity(
          server, max_response_payload_bytes * 2u);
    resp_hdr->transport_status = NIPC_STATUS_LIMIT_EXCEEDED;
    *close_after_response = true;
    *response_len = 0;
    break;
  case NIPC_ERR_TRUNCATED:
  case NIPC_ERR_BAD_LAYOUT:
  case NIPC_ERR_OUT_OF_BOUNDS:
  case NIPC_ERR_MISSING_NUL:
  case NIPC_ERR_BAD_ALIGNMENT:
  case NIPC_ERR_BAD_ITEM_COUNT:
    resp_hdr->transport_status = NIPC_STATUS_BAD_ENVELOPE;
    *close_after_response = true;
    *response_len = 0;
    break;
  case NIPC_ERR_HANDLER_FAILED:
  default:
    resp_hdr->transport_status = NIPC_STATUS_INTERNAL_ERROR;
    *close_after_response = true;
    *response_len = 0;
    break;
  }
}

void nipc_server_set_timings_for_tests(nipc_managed_server_t *server,
                                       uint32_t poll_ms, uint32_t stop_grace_ms,
                                       uint32_t handshake_ms) {
  server->poll_ms = poll_ms;
  server->stop_grace_ms = stop_grace_ms;
  server->handshake_ms = handshake_ms;
}

void nipc_server_set_event_hook_for_tests(nipc_managed_server_t *server,
                                          void (*fn)(void *user, int event),
                                          void *user) {
  server->test_event_hook = fn;
  server->test_event_user = user;
}

void nipc_client_set_shm_attach_timeout_for_tests(nipc_client_ctx_t *ctx,
                                                  uint32_t timeout_ms) {
  ctx->shm_attach_timeout_ms = timeout_ms;
}
