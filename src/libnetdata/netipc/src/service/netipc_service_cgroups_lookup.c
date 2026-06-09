#include "netipc_service_platform.h"

#include "netipc/netipc_protocol.h"

#include <string.h>

static bool cgroups_lookup_request_size(const nipc_str_view_t *paths,
                                        uint32_t path_count,
                                        size_t *size_out)
{
    if (nipc_service_common_mul_would_overflow(
            (size_t)path_count, NIPC_LOOKUP_DIR_ENTRY_SIZE))
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

static nipc_error_t cgroups_lookup_dispatch(void *user,
                                            const nipc_header_t *request_hdr,
                                            const uint8_t *request_payload,
                                            size_t request_len,
    uint8_t *response_buf,
    size_t response_buf_size,
    size_t *response_len_out)
{
    nipc_cgroups_lookup_service_handler_t *service_handler =
        (nipc_cgroups_lookup_service_handler_t *)user;
    (void)request_hdr;

    if (!service_handler->handle)
        return NIPC_ERR_HANDLER_FAILED;

    return nipc_dispatch_cgroups_lookup(
        request_payload, request_len,
        response_buf, response_buf_size, response_len_out,
        service_handler->handle, service_handler->user);
}

typedef struct {
    const nipc_str_view_t *paths;
    uint32_t path_count;
    nipc_cgroups_lookup_resp_view_t *view_out;
} cgroups_lookup_call_state_t;

static nipc_error_t do_cgroups_lookup_attempt(nipc_client_ctx_t *ctx,
                                              void *state)
{
    cgroups_lookup_call_state_t *s = (cgroups_lookup_call_state_t *)state;

    size_t req_size;
    if (!cgroups_lookup_request_size(s->paths, s->path_count, &req_size))
        return NIPC_ERR_OVERFLOW;
    if (req_size > UINT32_MAX)
        return NIPC_ERR_OVERFLOW;
    if (ctx->session.max_request_payload_bytes > 0 &&
        req_size > ctx->session.max_request_payload_bytes) {
        nipc_service_common_client_note_request_capacity(ctx, (uint32_t)req_size);
        return NIPC_ERR_OVERFLOW;
    }
    if (!nipc_service_platform_ensure_client_send_buffer(ctx, req_size))
        return NIPC_ERR_OVERFLOW;

    size_t req_len = nipc_cgroups_lookup_req_encode(
        s->paths, s->path_count, ctx->send_buf, ctx->send_buf_size);
    if (req_len == 0)
        return NIPC_ERR_BAD_LAYOUT;

    const void *payload;
    size_t payload_len;
    nipc_error_t err = nipc_service_platform_do_raw_call(
        ctx, NIPC_METHOD_CGROUPS_LOOKUP, ctx->send_buf, req_len,
        &payload, &payload_len);
    if (err != NIPC_OK)
        return err;

    err = nipc_cgroups_lookup_resp_decode(payload, payload_len, s->view_out);
    if (err != NIPC_OK)
        return err;
    if (s->view_out->item_count != s->path_count)
        return NIPC_ERR_BAD_ITEM_COUNT;
    for (uint32_t i = 0; i < s->path_count; i++) {
        nipc_cgroups_lookup_item_view_t item;
        err = nipc_cgroups_lookup_resp_item(s->view_out, i, &item);
        if (err != NIPC_OK)
            return err;
        if (item.path.len != s->paths[i].len ||
            memcmp(item.path.ptr, s->paths[i].ptr, item.path.len) != 0)
            return NIPC_ERR_BAD_LAYOUT;
    }
    return NIPC_OK;
}

nipc_error_t nipc_client_call_cgroups_lookup(
    nipc_client_ctx_t *ctx,
    const nipc_str_view_t *paths,
    uint32_t path_count,
    nipc_cgroups_lookup_resp_view_t *view_out)
{
    cgroups_lookup_call_state_t state = {
        .paths = paths,
        .path_count = path_count,
        .view_out = view_out,
    };
    return nipc_service_platform_call_with_retry(
        ctx, do_cgroups_lookup_attempt, &state);
}

nipc_error_t nipc_server_init_cgroups_lookup(
    nipc_managed_server_t *server,
    const char *run_dir,
    const char *service_name,
    const nipc_server_config_t *config,
    int worker_count,
    const nipc_cgroups_lookup_service_handler_t *service_handler)
{
    if (!service_handler)
        return NIPC_ERR_BAD_LAYOUT;

    nipc_service_platform_server_config_t typed_cfg;
    nipc_service_platform_server_config_from_service(&typed_cfg, config);
    if (typed_cfg.max_request_payload_bytes == 0)
        typed_cfg.max_request_payload_bytes =
            nipc_service_common_response_payload_default();
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
