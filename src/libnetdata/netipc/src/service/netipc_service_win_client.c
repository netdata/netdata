/*
 * netipc_service_win_client.c - WIN public service client API.
 */

#if defined(_WIN32) || defined(__MSYS__)

#include "netipc/netipc_service.h"
#include "netipc/netipc_protocol.h"
#include "netipc/netipc_named_pipe.h"
#include "netipc/netipc_win_shm.h"
#include "netipc_service_common.h"
#include "netipc_service_platform.h"
#include "netipc_service_win_internal.h"

#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <windows.h>

static nipc_np_client_config_t service_client_config_to_transport(
    const nipc_client_config_t *config)
{
    nipc_np_client_config_t transport = {0};

    if (!config)
        return transport;

    transport.supported_profiles = config->supported_profiles;
    transport.preferred_profiles = config->preferred_profiles;
    transport.max_request_batch_items = config->max_request_batch_items;
    transport.max_response_payload_bytes = config->max_response_payload_bytes;
    transport.max_response_batch_items =
        nipc_service_common_typed_response_batch_items(config->max_request_batch_items);
    transport.auth_token = config->auth_token;

    return transport;
}

void nipc_service_platform_server_config_from_service(
    nipc_service_platform_server_config_t *transport,
    const nipc_server_config_t *config)
{
    memset(transport, 0, sizeof(*transport));

    if (!config)
        return;

    transport->supported_profiles = config->supported_profiles;
    transport->preferred_profiles = config->preferred_profiles;
    transport->max_request_batch_items = config->max_request_batch_items;
    transport->max_response_payload_bytes = config->max_response_payload_bytes;
    transport->max_response_batch_items =
        nipc_service_common_typed_response_batch_items(config->max_request_batch_items);
    transport->auth_token = config->auth_token;
}

/* ------------------------------------------------------------------ */
/*  Public API: client lifecycle                                       */
/* ------------------------------------------------------------------ */

void nipc_client_init(nipc_client_ctx_t *ctx,
                      const char *run_dir,
                      const char *service_name,
                      const nipc_client_config_t *config)
{
    nipc_service_common_client_init(ctx, run_dir, service_name);
    ctx->session.pipe = INVALID_HANDLE_VALUE;

    ctx->transport_config = service_client_config_to_transport(config);
    if (ctx->transport_config.max_request_payload_bytes == 0)
        ctx->transport_config.max_request_payload_bytes =
            nipc_service_common_request_payload_default();
    if (ctx->transport_config.max_response_payload_bytes == 0)
        ctx->transport_config.max_response_payload_bytes =
            nipc_service_common_response_payload_default();
}

bool nipc_client_refresh(nipc_client_ctx_t *ctx)
{
    nipc_client_state_t old_state = ctx->state;

    switch (ctx->state) {
    case NIPC_CLIENT_DISCONNECTED:
    case NIPC_CLIENT_NOT_FOUND:
        ctx->state = NIPC_CLIENT_CONNECTING;
        ctx->state = nipc_service_win_client_try_connect(ctx);
        if (ctx->state == NIPC_CLIENT_READY)
            ctx->connect_count++;
        break;

    case NIPC_CLIENT_BROKEN:
        nipc_service_win_client_disconnect(ctx);
        ctx->state = NIPC_CLIENT_CONNECTING;
        ctx->state = nipc_service_win_client_try_connect(ctx);
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

void nipc_client_status(const nipc_client_ctx_t *ctx,
                        nipc_client_status_t *out)
{
    nipc_service_common_client_status(ctx, out);
}

void nipc_client_close(nipc_client_ctx_t *ctx)
{
    nipc_service_win_client_disconnect(ctx);
    nipc_service_common_client_close_buffers(ctx);
}

#endif /* _WIN32 || __MSYS__ */
