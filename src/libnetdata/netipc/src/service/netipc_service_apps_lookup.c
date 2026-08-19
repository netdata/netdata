#include "netipc_service_platform.h"

#include "netipc/netipc_protocol.h"

#include <stdlib.h>
#include <string.h>

static bool apps_lookup_request_size(uint32_t pid_count, size_t *size_out) {
  if (nipc_service_common_mul_would_overflow((size_t)pid_count,
                                             NIPC_LOOKUP_DIR_ENTRY_SIZE) ||
      nipc_service_common_mul_would_overflow((size_t)pid_count,
                                             NIPC_APPS_LOOKUP_KEY_SIZE))
    return false;

  size_t dir = (size_t)pid_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  size_t keys = (size_t)pid_count * NIPC_APPS_LOOKUP_KEY_SIZE;
#if SIZE_MAX <= UINT32_MAX
  if (NIPC_APPS_LOOKUP_REQ_HDR_SIZE > SIZE_MAX - dir ||
      NIPC_APPS_LOOKUP_REQ_HDR_SIZE + dir > SIZE_MAX - keys)
    return false;
#endif
  *size_out = NIPC_APPS_LOOKUP_REQ_HDR_SIZE + dir + keys;
  return true;
}

static nipc_error_t
apps_lookup_dispatch(void *user, const nipc_header_t *request_hdr,
                     const uint8_t *request_payload, size_t request_len,
                     uint8_t *response_buf, size_t response_buf_size,
                     size_t *response_len_out) {
  nipc_apps_lookup_service_handler_t *service_handler =
      (nipc_apps_lookup_service_handler_t *)user;
  (void)request_hdr;

  if (!service_handler->handle)
    return NIPC_ERR_HANDLER_FAILED;

  return nipc_dispatch_apps_lookup(
      request_payload, request_len, response_buf, response_buf_size,
      response_len_out, service_handler->handle, service_handler->user);
}

typedef struct {
  const uint32_t *pids;
  uint32_t pid_count;
  nipc_apps_lookup_resp_view_t *view_out;
  uint64_t deadline_ms;
} apps_lookup_call_state_t;

typedef struct {
  const void *request_payload;
  size_t request_len;
  const void **response_payload_out;
  size_t *response_len_out;
  uint32_t timeout_ms;
} apps_lookup_raw_call_state_t;

static nipc_error_t apps_lookup_raw_call_attempt(nipc_client_ctx_t *ctx,
                                                 void *state) {
  apps_lookup_raw_call_state_t *s = (apps_lookup_raw_call_state_t *)state;
  return nipc_service_platform_do_raw_call(
      ctx, NIPC_METHOD_APPS_LOOKUP, s->request_payload, s->request_len,
      s->response_payload_out, s->response_len_out, s->timeout_ms);
}

static nipc_error_t apps_lookup_do_raw_call_with_retry(
    nipc_client_ctx_t *ctx, const void *request_payload, size_t request_len,
    const void **response_payload_out, size_t *response_len_out,
    uint32_t timeout_ms) {
  apps_lookup_raw_call_state_t state = {
      .request_payload = request_payload,
      .request_len = request_len,
      .response_payload_out = response_payload_out,
      .response_len_out = response_len_out,
      .timeout_ms = timeout_ms,
  };
  return nipc_service_platform_call_with_retry(ctx, apps_lookup_raw_call_attempt,
                                               &state);
}

static bool apps_lookup_request_capacity_can_reconnect(nipc_client_ctx_t *ctx,
                                                       size_t required) {
  return required <= UINT32_MAX &&
         ctx->transport_config.max_request_payload_bytes >= required;
}

static nipc_error_t apps_lookup_remaining_timeout(nipc_client_ctx_t *ctx,
                                                  uint64_t deadline_ms,
                                                  uint32_t *timeout_out) {
  if (nipc_service_common_client_abort_requested(ctx))
    return NIPC_ERR_ABORTED;

  uint64_t now = nipc_service_platform_monotonic_ms();
  if (now >= deadline_ms)
    return NIPC_ERR_TIMEOUT;

  uint64_t remaining = deadline_ms - now;
  *timeout_out =
      remaining > UINT32_MAX ? UINT32_MAX : (uint32_t)remaining;
  return NIPC_OK;
}

nipc_error_t nipc_apps_lookup_remaining_timeout_for_tests(
    nipc_client_ctx_t *ctx, uint64_t deadline_ms, uint32_t *timeout_out) {
  return apps_lookup_remaining_timeout(ctx, deadline_ms, timeout_out);
}

static nipc_error_t
apps_lookup_next_request_count(nipc_client_ctx_t *ctx, uint32_t remaining,
                               uint32_t *count_out, size_t *size_out) {
  if (remaining == 0) {
    *size_out = NIPC_APPS_LOOKUP_REQ_HDR_SIZE;
    *count_out = 0;
    return NIPC_OK;
  }

  uint32_t cap = ctx->session.max_request_payload_bytes;
  if (cap == 0)
    cap = ctx->transport_config.max_request_payload_bytes;
  if (cap == 0)
    cap = nipc_service_common_request_payload_default();

  size_t per_item = NIPC_LOOKUP_DIR_ENTRY_SIZE + NIPC_APPS_LOOKUP_KEY_SIZE;
  if (cap < NIPC_APPS_LOOKUP_REQ_HDR_SIZE ||
      (size_t)(cap - NIPC_APPS_LOOKUP_REQ_HDR_SIZE) < per_item)
    return NIPC_ERR_OVERFLOW;

  size_t max_by_cap = ((size_t)cap - NIPC_APPS_LOOKUP_REQ_HDR_SIZE) / per_item;
  if (max_by_cap == 0)
    return NIPC_ERR_OVERFLOW;

  uint32_t count = remaining;
  if ((size_t)count > max_by_cap)
    count = (uint32_t)max_by_cap;
  if (!apps_lookup_request_size(count, size_out))
    return NIPC_ERR_OVERFLOW;
  *count_out = count;
  return NIPC_OK;
}

static void apps_lookup_free_items(uint8_t **items, uint32_t count) {
  if (!items)
    return;
  for (uint32_t i = 0; i < count; i++)
    free(items[i]);
  free(items);
}

static nipc_error_t apps_lookup_raw_response_size(const uint32_t *item_lens,
                                                  uint32_t item_count,
                                                  size_t *size_out) {
  if (nipc_service_common_mul_would_overflow((size_t)item_count,
                                             NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return NIPC_ERR_OVERFLOW;

  size_t data = NIPC_APPS_LOOKUP_RESP_HDR_SIZE +
                (size_t)item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  for (uint32_t i = 0; i < item_count; i++) {
    size_t aligned = nipc_align8(data);
    if (aligned < data || item_lens[i] > SIZE_MAX - aligned)
      return NIPC_ERR_OVERFLOW;
    data = aligned + item_lens[i];
  }
  *size_out = data;
  return NIPC_OK;
}

static nipc_error_t do_apps_lookup_attempt(nipc_client_ctx_t *ctx,
                                           void *state) {
  apps_lookup_call_state_t *s = (apps_lookup_call_state_t *)state;
  uint8_t **items = NULL;
  const uint8_t **item_ptrs = NULL;
  uint32_t *item_lens = NULL;
  uint32_t accepted = 0;
  uint32_t start = 0;
  uint32_t subcalls = 0;
  uint64_t generation = 0;
  bool have_generation = false;
  nipc_error_t err = NIPC_OK;

  if (s->pid_count > 0 && !s->pids)
    return NIPC_ERR_BAD_LAYOUT;
  if (s->pid_count > ctx->max_logical_lookup_items)
    return NIPC_ERR_OVERFLOW;

  if (s->pid_count > 0) {
    items = calloc(s->pid_count, sizeof(*items));
    item_ptrs = calloc(s->pid_count, sizeof(*item_ptrs));
    item_lens = calloc(s->pid_count, sizeof(*item_lens));
    if (!items || !item_ptrs || !item_lens) {
      err = NIPC_ERR_OVERFLOW;
      goto cleanup;
    }
  }

  for (;;) {
    if (nipc_service_common_client_abort_requested(ctx)) {
      err = NIPC_ERR_ABORTED;
      goto cleanup;
    }

    uint32_t req_count;
    const uint32_t *req_pids = s->pids ? s->pids + start : NULL;
    size_t req_size;
    err = apps_lookup_next_request_count(ctx, s->pid_count - start, &req_count,
                                         &req_size);
    if (err != NIPC_OK) {
      size_t one_item_size = 0;
      if (err == NIPC_ERR_OVERFLOW && start < s->pid_count &&
          apps_lookup_request_size(1, &one_item_size) &&
          apps_lookup_request_capacity_can_reconnect(ctx, one_item_size)) {
        err = nipc_service_platform_ensure_request_capacity(ctx,
                                                            one_item_size);
        if (err == NIPC_OK)
          continue;
      }
      goto cleanup;
    }
    if (req_count == 0) {
      if (start != s->pid_count) {
        err = NIPC_ERR_BAD_ITEM_COUNT;
        goto cleanup;
      }
    } else if (!req_pids || !items || !item_ptrs || !item_lens) {
      err = NIPC_ERR_BAD_LAYOUT;
      goto cleanup;
    }
    /* next_request_count returns only sizes bounded by the uint32 transport cap. */
    if (!nipc_service_platform_ensure_client_send_buffer(ctx, req_size)) {
      err = NIPC_ERR_OVERFLOW;
      goto cleanup;
    }

    size_t req_len = nipc_apps_lookup_req_encode(
        req_pids, req_count, ctx->send_buf, ctx->send_buf_size);
    if (req_len == 0) {
      err = NIPC_ERR_BAD_LAYOUT;
      goto cleanup;
    }

    const void *payload;
    size_t payload_len;
    uint32_t timeout_ms;
    if (++subcalls > ctx->max_logical_lookup_subcalls) {
      err = NIPC_ERR_OVERFLOW;
      goto cleanup;
    }
    err = apps_lookup_remaining_timeout(ctx, s->deadline_ms, &timeout_ms);
    if (err != NIPC_OK)
      goto cleanup;
    err = apps_lookup_do_raw_call_with_retry(ctx, ctx->send_buf, req_len,
                                             &payload, &payload_len,
                                             timeout_ms);
    if (err != NIPC_OK)
      goto cleanup;

    err = nipc_apps_lookup_resp_decode(payload, payload_len, s->view_out);
    if (err != NIPC_OK)
      goto cleanup;
    if (have_generation && s->view_out->generation != generation) {
      err = NIPC_ERR_BAD_LAYOUT;
      goto cleanup;
    }
    generation = s->view_out->generation;
    have_generation = true;
    if (s->view_out->item_count != req_count) {
      err = NIPC_ERR_BAD_ITEM_COUNT;
      goto cleanup;
    }

    uint32_t payload_exceeded_at = UINT32_MAX;
    for (uint32_t i = 0; i < req_count; i++) {
      nipc_apps_lookup_item_view_t item;
      err = nipc_apps_lookup_resp_item(s->view_out, i, &item);
      if (err != NIPC_OK)
        goto cleanup;
      if (item.pid != req_pids[i]) {
        err = NIPC_ERR_BAD_LAYOUT;
        goto cleanup;
      }
      if (item.status == NIPC_PID_LOOKUP_PAYLOAD_EXCEEDED) {
        payload_exceeded_at = i;
        break;
      }

      const uint8_t *raw_item;
      uint32_t raw_len;
      err = nipc_apps_lookup_resp_raw_item(s->view_out, i, &raw_item, &raw_len);
      if (err != NIPC_OK)
        goto cleanup;
      items[accepted] = malloc(raw_len);
      if (!items[accepted]) {
        err = NIPC_ERR_OVERFLOW;
        goto cleanup;
      }
      memcpy(items[accepted], raw_item, raw_len);
      item_lens[accepted] = raw_len;
      accepted++;
    }

    if (payload_exceeded_at == UINT32_MAX) {
      start += req_count;
      if (start >= s->pid_count)
        break;
      continue;
    }

    for (uint32_t i = payload_exceeded_at; i < req_count; i++) {
      nipc_apps_lookup_item_view_t item;
      err = nipc_apps_lookup_resp_item(s->view_out, i, &item);
      if (err != NIPC_OK)
        goto cleanup;
      if (item.pid != req_pids[i] ||
          item.status != NIPC_PID_LOOKUP_PAYLOAD_EXCEEDED) {
        err = NIPC_ERR_BAD_LAYOUT;
        goto cleanup;
      }
    }
    if (payload_exceeded_at == 0) {
      err = NIPC_ERR_OVERFLOW;
      goto cleanup;
    }
    start += payload_exceeded_at;
  }

  err = (accepted == s->pid_count) ? NIPC_OK : NIPC_ERR_BAD_ITEM_COUNT;
  if (err != NIPC_OK)
    goto cleanup;

  size_t final_size;
  err = apps_lookup_raw_response_size(item_lens, accepted, &final_size);
  if (err != NIPC_OK)
    goto cleanup;
  if (final_size > ctx->max_logical_lookup_response_bytes) {
    err = NIPC_ERR_OVERFLOW;
    goto cleanup;
  }
  if (!nipc_service_platform_ensure_client_response_buffer(ctx, final_size)) {
    err = NIPC_ERR_OVERFLOW;
    goto cleanup;
  }
  for (uint32_t i = 0; i < accepted; i++)
    item_ptrs[i] = items[i];
  size_t encoded_len;
  err = nipc_apps_lookup_raw_resp_encode(item_ptrs, item_lens, accepted,
                                         generation, ctx->response_buf,
                                         ctx->response_buf_size, &encoded_len);
  if (err != NIPC_OK)
    goto cleanup;
  err =
      nipc_apps_lookup_resp_decode(ctx->response_buf, encoded_len, s->view_out);

cleanup:
  if (err != NIPC_OK && s->view_out)
    memset(s->view_out, 0, sizeof(*s->view_out));
  apps_lookup_free_items(items, accepted);
  free(item_ptrs);
  free(item_lens);
  return err;
}

nipc_error_t
nipc_client_call_apps_lookup(nipc_client_ctx_t *ctx, const uint32_t *pids,
                             uint32_t pid_count,
                             nipc_apps_lookup_resp_view_t *view_out) {
  return nipc_client_call_apps_lookup_timeout(ctx, pids, pid_count, view_out,
                                              0);
}

nipc_error_t nipc_client_call_apps_lookup_timeout(
    nipc_client_ctx_t *ctx, const uint32_t *pids, uint32_t pid_count,
    nipc_apps_lookup_resp_view_t *view_out, uint32_t timeout_ms) {
  uint32_t resolved_timeout =
      nipc_service_common_client_call_timeout_ms(ctx, timeout_ms);
  apps_lookup_call_state_t state = {
      .pids = pids,
      .pid_count = pid_count,
      .view_out = view_out,
      .deadline_ms = nipc_service_platform_monotonic_ms() + resolved_timeout,
  };
  if (ctx->state != NIPC_CLIENT_READY) {
    ctx->error_count++;
    return NIPC_ERR_NOT_READY;
  }
  return do_apps_lookup_attempt(ctx, &state);
}

nipc_error_t nipc_server_init_apps_lookup(
    nipc_managed_server_t *server, const char *run_dir,
    const char *service_name, const nipc_server_config_t *config,
    int worker_count,
    const nipc_apps_lookup_service_handler_t *service_handler) {
  if (!service_handler)
    return NIPC_ERR_BAD_LAYOUT;

  nipc_service_platform_server_config_t typed_cfg;
  nipc_service_platform_server_config_from_service(&typed_cfg, config);
  if (typed_cfg.max_request_payload_bytes == 0)
    typed_cfg.max_request_payload_bytes =
        nipc_service_common_request_payload_default();
  if (typed_cfg.max_response_payload_bytes == 0)
    typed_cfg.max_response_payload_bytes =
        nipc_service_common_response_payload_default();

  nipc_error_t err = nipc_service_platform_server_init_raw(
      server, run_dir, service_name, &typed_cfg, worker_count,
      NIPC_METHOD_APPS_LOOKUP, apps_lookup_dispatch,
      &server->typed_handler.apps_lookup);
  if (err != NIPC_OK)
    return err;

  server->typed_handler.apps_lookup = *service_handler;
  return NIPC_OK;
}
