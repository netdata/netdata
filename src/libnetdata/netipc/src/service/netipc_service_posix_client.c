/*
 * netipc_service_posix_client.c - POSIX public service client API.
 */

#include "netipc/netipc_service.h"
#include "netipc/netipc_protocol.h"
#include "netipc/netipc_uds.h"
#include "netipc/netipc_shm.h"
#include "netipc_service_common.h"
#include "netipc_service_platform.h"
#include "netipc_service_posix_internal.h"

#include <stdint.h>
#include <stdlib.h>
#include <string.h>

static nipc_uds_client_config_t service_client_config_to_transport(
    const nipc_client_config_t *config)
{
    nipc_uds_client_config_t transport = {0};

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

static void client_sleep_ms(uint32_t ms)
{
    nipc_service_posix_sleep_us(ms * 1000u);
}

const nipc_service_common_client_ops_t *nipc_service_posix_client_ops(void)
{
    static const nipc_service_common_client_ops_t ops = {
        .disconnect = nipc_service_posix_client_disconnect,
        .try_connect = nipc_service_posix_client_try_connect,
        .reconnect_for_call = nipc_service_posix_client_reconnect_for_call,
        .sleep_ms = client_sleep_ms,
        .reconnect_drain_ms = CLIENT_CALL_RECONNECT_DRAIN_MS,
        .reconnect_retry_interval_ms = CLIENT_CALL_RECONNECT_RETRY_INTERVAL_MS,
    };
    return &ops;
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
    ctx->session.fd = -1;

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
    return nipc_service_common_client_refresh(ctx, nipc_service_posix_client_ops());
}

void nipc_client_status(const nipc_client_ctx_t *ctx,
                        nipc_client_status_t *out)
{
    nipc_service_common_client_status(ctx, out);
}

void nipc_client_close(nipc_client_ctx_t *ctx)
{
    nipc_service_posix_client_disconnect(ctx);
    nipc_service_common_client_close_buffers(ctx);
}
