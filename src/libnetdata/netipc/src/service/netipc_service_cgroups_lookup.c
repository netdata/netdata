#include "netipc_service_platform.h"

#include "netipc/netipc_protocol.h"

#include <stdlib.h>
#include <string.h>

static bool cgroups_lookup_request_size(const nipc_str_view_t *paths,
                                        uint32_t path_count, size_t *size_out) {
  if (nipc_service_common_mul_would_overflow((size_t)path_count,
                                             NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return false;

  size_t data = NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE +
                (size_t)path_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  for (uint32_t i = 0; i < path_count; i++) {
    size_t aligned = nipc_align8(data);
    if (aligned < data)
      return false;
    if (!paths || paths[i].len > SIZE_MAX - aligned - 1u)
      return false;
    data = aligned + (size_t)paths[i].len + 1u;
  }
  *size_out = data;
  return true;
}

static nipc_error_t
cgroups_lookup_dispatch(void *user, const nipc_header_t *request_hdr,
                        const uint8_t *request_payload, size_t request_len,
                        uint8_t *response_buf, size_t response_buf_size,
                        size_t *response_len_out) {
  nipc_cgroups_lookup_service_handler_t *service_handler =
      (nipc_cgroups_lookup_service_handler_t *)user;
  (void)request_hdr;

  if (!service_handler->handle)
    return NIPC_ERR_HANDLER_FAILED;

  return nipc_dispatch_cgroups_lookup(
      request_payload, request_len, response_buf, response_buf_size,
      response_len_out, service_handler->handle, service_handler->user);
}

typedef struct {
  const nipc_str_view_t *paths;
  uint32_t path_count;
  nipc_cgroups_lookup_resp_view_t *view_out;
  uint64_t deadline_ms;
} cgroups_lookup_call_state_t;

typedef struct {
  const void *request_payload;
  size_t request_len;
  const void **response_payload_out;
  size_t *response_len_out;
  uint32_t timeout_ms;
} cgroups_lookup_raw_call_state_t;

static nipc_error_t cgroups_lookup_raw_call_attempt(nipc_client_ctx_t *ctx,
                                                    void *state) {
  cgroups_lookup_raw_call_state_t *s =
      (cgroups_lookup_raw_call_state_t *)state;
  return nipc_service_platform_do_raw_call(
      ctx, NIPC_METHOD_CGROUPS_LOOKUP, s->request_payload, s->request_len,
      s->response_payload_out, s->response_len_out, s->timeout_ms);
}

static nipc_error_t cgroups_lookup_do_raw_call_with_retry(
    nipc_client_ctx_t *ctx, const void *request_payload, size_t request_len,
    const void **response_payload_out, size_t *response_len_out,
    uint32_t timeout_ms) {
  cgroups_lookup_raw_call_state_t state = {
      .request_payload = request_payload,
      .request_len = request_len,
      .response_payload_out = response_payload_out,
      .response_len_out = response_len_out,
      .timeout_ms = timeout_ms,
  };
  return nipc_service_platform_call_with_retry(
      ctx, cgroups_lookup_raw_call_attempt, &state);
}

static bool
cgroups_lookup_request_capacity_can_reconnect(nipc_client_ctx_t *ctx,
                                              size_t required) {
  return required <= UINT32_MAX &&
         ctx->transport_config.max_request_payload_bytes >= required;
}

static nipc_error_t cgroups_lookup_remaining_timeout(nipc_client_ctx_t *ctx,
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

static nipc_error_t
cgroups_lookup_next_request_count(nipc_client_ctx_t *ctx,
                                  const nipc_str_view_t *paths,
                                  uint32_t remaining, uint32_t *count_out,
                                  size_t *size_out) {
  if (remaining == 0) {
    if (!cgroups_lookup_request_size(NULL, 0, size_out))
      return NIPC_ERR_OVERFLOW;
    *count_out = 0;
    return NIPC_OK;
  }

  uint32_t cap = ctx->session.max_request_payload_bytes;
  if (cap == 0)
    cap = ctx->transport_config.max_request_payload_bytes;
  if (cap == 0)
    cap = nipc_service_common_request_payload_default();

  uint32_t lo = 1;
  uint32_t hi = remaining;
  uint32_t best = 0;
  size_t best_size = 0;

  while (lo <= hi) {
    uint32_t mid = lo + (hi - lo) / 2u;
    size_t req_size;
    if (!cgroups_lookup_request_size(paths, mid, &req_size))
      return NIPC_ERR_OVERFLOW;

    if (req_size <= cap) {
      best = mid;
      best_size = req_size;
      lo = mid + 1u;
    } else {
      if (mid == 1)
        break;
      hi = mid - 1u;
    }
  }

  if (best == 0)
    return NIPC_ERR_OVERFLOW;

  *count_out = best;
  *size_out = best_size;
  return NIPC_OK;
}

static void cgroups_lookup_free_items(uint8_t **items, uint32_t count) {
  if (!items)
    return;
  for (uint32_t i = 0; i < count; i++)
    free(items[i]);
  free(items);
}

static nipc_error_t cgroups_lookup_raw_response_size(const uint32_t *item_lens,
                                                     uint32_t item_count,
                                                     size_t *size_out) {
  if (nipc_service_common_mul_would_overflow((size_t)item_count,
                                             NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return NIPC_ERR_OVERFLOW;

  size_t data = NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE +
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

static nipc_error_t
cgroups_lookup_clone_oversized_request_item(const nipc_str_view_t *path,
                                            uint8_t **item_out,
                                            uint32_t *item_len_out) {
  if (!path || !path->ptr || path->len == 0)
    return NIPC_ERR_BAD_LAYOUT;

  size_t data = NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE + NIPC_LOOKUP_DIR_ENTRY_SIZE;
  size_t item_start = nipc_align8(data);
  if (item_start < data)
    return NIPC_ERR_OVERFLOW;
  if ((size_t)path->len > SIZE_MAX - NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE - 2u)
    return NIPC_ERR_OVERFLOW;

  size_t item_size =
      NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE + (size_t)path->len + 2u;
  if (item_size > SIZE_MAX - item_start)
    return NIPC_ERR_OVERFLOW;

  size_t resp_size = item_start + item_size;
  uint8_t *buf = malloc(resp_size);
  if (!buf)
    return NIPC_ERR_OVERFLOW;

  nipc_cgroups_lookup_builder_t builder;
  nipc_cgroups_lookup_builder_init(&builder, buf, resp_size, 1, 0);
  nipc_error_t err = nipc_cgroups_lookup_builder_add(
      &builder, NIPC_CGROUP_LOOKUP_OVERSIZED_ITEM, 0, path->ptr, path->len, "",
      0, NULL, 0);
  if (err != NIPC_OK)
    goto cleanup;

  size_t encoded_len = nipc_cgroups_lookup_builder_finish(&builder);
  if (encoded_len == 0) {
    err = NIPC_ERR_OVERFLOW;
    goto cleanup;
  }

  nipc_cgroups_lookup_resp_view_t view;
  err = nipc_cgroups_lookup_resp_decode(buf, encoded_len, &view);
  if (err != NIPC_OK)
    goto cleanup;

  const uint8_t *raw_item;
  uint32_t raw_len;
  err = nipc_cgroups_lookup_resp_raw_item(&view, 0, &raw_item, &raw_len);
  if (err != NIPC_OK)
    goto cleanup;

  uint8_t *item = malloc(raw_len);
  if (!item) {
    err = NIPC_ERR_OVERFLOW;
    goto cleanup;
  }
  memcpy(item, raw_item, raw_len);
  *item_out = item;
  *item_len_out = raw_len;

cleanup:
  free(buf);
  return err;
}

static nipc_error_t do_cgroups_lookup_attempt(nipc_client_ctx_t *ctx,
                                              void *state) {
  cgroups_lookup_call_state_t *s = (cgroups_lookup_call_state_t *)state;
  uint8_t **items = NULL;
  const uint8_t **item_ptrs = NULL;
  uint32_t *item_lens = NULL;
  uint32_t accepted = 0;
  uint32_t start = 0;
  uint32_t subcalls = 0;
  uint64_t generation = 0;
  bool have_generation = false;
  nipc_error_t err = NIPC_OK;

  if (s->path_count > 0 && !s->paths)
    return NIPC_ERR_BAD_LAYOUT;
  if (s->path_count > ctx->max_logical_lookup_items)
    return NIPC_ERR_OVERFLOW;

  if (s->path_count > 0) {
    items = calloc(s->path_count, sizeof(*items));
    item_ptrs = calloc(s->path_count, sizeof(*item_ptrs));
    item_lens = calloc(s->path_count, sizeof(*item_lens));
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
    const nipc_str_view_t *req_paths = s->paths ? s->paths + start : NULL;
    size_t req_size;
    err = cgroups_lookup_next_request_count(ctx, req_paths,
                                            s->path_count - start, &req_count,
                                            &req_size);
    if (err != NIPC_OK) {
      if (err == NIPC_ERR_OVERFLOW && start < s->path_count) {
        size_t one_item_size = 0;
        if (cgroups_lookup_request_size(&s->paths[start], 1,
                                        &one_item_size) &&
            cgroups_lookup_request_capacity_can_reconnect(ctx,
                                                          one_item_size)) {
          err = nipc_service_platform_ensure_request_capacity(ctx,
                                                              one_item_size);
          if (err == NIPC_OK)
            continue;
          goto cleanup;
        }
        err = cgroups_lookup_clone_oversized_request_item(
            &s->paths[start], &items[accepted], &item_lens[accepted]);
        if (err != NIPC_OK)
          goto cleanup;
        accepted++;
        start++;
        continue;
      }
      goto cleanup;
    }
    if (req_count == 0) {
      if (start != s->path_count) {
        err = NIPC_ERR_BAD_ITEM_COUNT;
        goto cleanup;
      }
    } else if (!req_paths || !items || !item_ptrs || !item_lens) {
      err = NIPC_ERR_BAD_LAYOUT;
      goto cleanup;
    }
    /* next_request_count returns only sizes bounded by the uint32 transport cap. */
    if (!nipc_service_platform_ensure_client_send_buffer(ctx, req_size)) {
      err = NIPC_ERR_OVERFLOW;
      goto cleanup;
    }

    size_t req_len = nipc_cgroups_lookup_req_encode(
        req_paths, req_count, ctx->send_buf, ctx->send_buf_size);
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
    err = cgroups_lookup_remaining_timeout(ctx, s->deadline_ms, &timeout_ms);
    if (err != NIPC_OK)
      goto cleanup;
    err = cgroups_lookup_do_raw_call_with_retry(ctx, ctx->send_buf, req_len,
                                                &payload, &payload_len,
                                                timeout_ms);
    if (err != NIPC_OK)
      goto cleanup;

    err = nipc_cgroups_lookup_resp_decode(payload, payload_len, s->view_out);
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
      nipc_cgroups_lookup_item_view_t item;
      err = nipc_cgroups_lookup_resp_item(s->view_out, i, &item);
      if (err != NIPC_OK)
        goto cleanup;
      if (item.path.len != req_paths[i].len ||
          memcmp(item.path.ptr, req_paths[i].ptr, item.path.len) != 0) {
        err = NIPC_ERR_BAD_LAYOUT;
        goto cleanup;
      }
      if (item.status == NIPC_CGROUP_LOOKUP_PAYLOAD_EXCEEDED) {
        payload_exceeded_at = i;
        break;
      }

      const uint8_t *raw_item;
      uint32_t raw_len;
      err = nipc_cgroups_lookup_resp_raw_item(s->view_out, i, &raw_item,
                                              &raw_len);
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
      if (start >= s->path_count)
        break;
      continue;
    }

    for (uint32_t i = payload_exceeded_at; i < req_count; i++) {
      nipc_cgroups_lookup_item_view_t item;
      err = nipc_cgroups_lookup_resp_item(s->view_out, i, &item);
      if (err != NIPC_OK)
        goto cleanup;
      if (item.path.len != req_paths[i].len ||
          memcmp(item.path.ptr, req_paths[i].ptr, item.path.len) != 0 ||
          item.status != NIPC_CGROUP_LOOKUP_PAYLOAD_EXCEEDED) {
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

  err = (accepted == s->path_count) ? NIPC_OK : NIPC_ERR_BAD_ITEM_COUNT;
  if (err != NIPC_OK)
    goto cleanup;

  size_t final_size;
  err = cgroups_lookup_raw_response_size(item_lens, accepted, &final_size);
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
  err = nipc_cgroups_lookup_raw_resp_encode(
      item_ptrs, item_lens, accepted, generation, ctx->response_buf,
      ctx->response_buf_size, &encoded_len);
  if (err != NIPC_OK)
    goto cleanup;
  err = nipc_cgroups_lookup_resp_decode(ctx->response_buf, encoded_len,
                                        s->view_out);

cleanup:
  if (err != NIPC_OK && s->view_out)
    memset(s->view_out, 0, sizeof(*s->view_out));
  cgroups_lookup_free_items(items, accepted);
  free(item_ptrs);
  free(item_lens);
  return err;
}

nipc_error_t nipc_client_call_cgroups_lookup(
    nipc_client_ctx_t *ctx, const nipc_str_view_t *paths, uint32_t path_count,
    nipc_cgroups_lookup_resp_view_t *view_out) {
  return nipc_client_call_cgroups_lookup_timeout(ctx, paths, path_count,
                                                 view_out, 0);
}

nipc_error_t nipc_client_call_cgroups_lookup_timeout(
    nipc_client_ctx_t *ctx, const nipc_str_view_t *paths, uint32_t path_count,
    nipc_cgroups_lookup_resp_view_t *view_out, uint32_t timeout_ms) {
  uint32_t resolved_timeout =
      nipc_service_common_client_call_timeout_ms(ctx, timeout_ms);
  cgroups_lookup_call_state_t state = {
      .paths = paths,
      .path_count = path_count,
      .view_out = view_out,
      .deadline_ms = nipc_service_platform_monotonic_ms() + resolved_timeout,
  };
  if (ctx->state != NIPC_CLIENT_READY) {
    ctx->error_count++;
    return NIPC_ERR_NOT_READY;
  }
  return do_cgroups_lookup_attempt(ctx, &state);
}

nipc_error_t nipc_server_init_cgroups_lookup(
    nipc_managed_server_t *server, const char *run_dir,
    const char *service_name, const nipc_server_config_t *config,
    int worker_count,
    const nipc_cgroups_lookup_service_handler_t *service_handler) {
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
      NIPC_METHOD_CGROUPS_LOOKUP, cgroups_lookup_dispatch,
      &server->typed_handler.cgroups_lookup);
  if (err != NIPC_OK)
    return err;

  server->typed_handler.cgroups_lookup = *service_handler;
  return NIPC_OK;
}
