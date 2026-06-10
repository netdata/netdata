#include "netipc_service_platform.h"

#include "netipc/netipc_protocol.h"

static uint32_t snapshot_max_items(
    size_t response_buf_size,
    const nipc_cgroups_service_handler_t *service_handler)
{
    if (service_handler->snapshot_max_items != 0)
        return service_handler->snapshot_max_items;
    return nipc_cgroups_builder_estimate_max_items(response_buf_size);
}

static nipc_error_t cgroups_snapshot_dispatch(void *user,
                                              const nipc_header_t *request_hdr,
                                              const uint8_t *request_payload,
                                              size_t request_len,
    uint8_t *response_buf,
    size_t response_buf_size,
    size_t *response_len_out)
{
    nipc_cgroups_service_handler_t *service_handler =
        (nipc_cgroups_service_handler_t *)user;
    (void)request_hdr;

    if (!service_handler->handle)
        return NIPC_ERR_HANDLER_FAILED;

    return nipc_dispatch_cgroups_snapshot(
        request_payload, request_len,
        response_buf, response_buf_size, response_len_out,
        snapshot_max_items(response_buf_size, service_handler),
        service_handler->handle, service_handler->user);
}

typedef struct {
    nipc_cgroups_resp_view_t *view_out;
} cgroups_snapshot_call_state_t;

static nipc_error_t do_cgroups_snapshot_attempt(nipc_client_ctx_t *ctx,
                                                void *state)
{
    cgroups_snapshot_call_state_t *s = (cgroups_snapshot_call_state_t *)state;

    nipc_cgroups_req_t req = { .layout_version = 1, .flags = 0 };
    uint8_t req_buf[4];
    size_t req_len = nipc_cgroups_req_encode(&req, req_buf, sizeof(req_buf));
    if (req_len == 0)
        return NIPC_ERR_TRUNCATED;

    const void *payload;
    size_t payload_len;
    nipc_error_t err = nipc_service_platform_do_raw_call(
        ctx, NIPC_METHOD_CGROUPS_SNAPSHOT, req_buf, req_len,
        &payload, &payload_len);
    if (err != NIPC_OK)
        return err;

    return nipc_cgroups_resp_decode(payload, payload_len, s->view_out);
}

nipc_error_t nipc_client_call_cgroups_snapshot(
    nipc_client_ctx_t *ctx,
    nipc_cgroups_resp_view_t *view_out)
{
    cgroups_snapshot_call_state_t state = {
        .view_out = view_out,
    };
    return nipc_service_platform_call_with_retry(
        ctx, do_cgroups_snapshot_attempt, &state);
}

nipc_error_t nipc_server_init_typed(
    nipc_managed_server_t *server,
    const char *run_dir,
    const char *service_name,
    const nipc_server_config_t *config,
    int worker_count,
    const nipc_cgroups_service_handler_t *service_handler)
{
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
        NIPC_METHOD_CGROUPS_SNAPSHOT, cgroups_snapshot_dispatch,
        &server->typed_handler.cgroups_snapshot);
    if (err != NIPC_OK)
        return err;

    server->typed_handler.cgroups_snapshot = *service_handler;
    return NIPC_OK;
}
