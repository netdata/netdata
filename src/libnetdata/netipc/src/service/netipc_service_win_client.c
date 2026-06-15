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
    nipc_service_common_transport_fields_t fields;

    if (!nipc_service_common_client_transport_fields(&fields, config))
        return transport;

    NIPC_SERVICE_COMMON_APPLY_TRANSPORT_FIELDS(&transport, &fields);
    return transport;
}

static void client_sleep_ms(uint32_t ms)
{
    Sleep(ms);
}

static void close_abort_event(nipc_client_ctx_t *ctx)
{
    if (ctx->abort_event) {
        CloseHandle(ctx->abort_event);
        ctx->abort_event = NULL;
    }
}

const nipc_service_common_client_ops_t *nipc_service_win_client_ops(void)
{
    static const nipc_service_common_client_ops_t ops = {
        .disconnect = nipc_service_win_client_disconnect,
        .try_connect = nipc_service_win_client_try_connect,
        .reconnect_for_call = nipc_service_win_client_reconnect_for_call,
        .sleep_ms = client_sleep_ms,
        .reconnect_drain_ms = 0,
        .reconnect_retry_interval_ms = CLIENT_CALL_RECONNECT_RETRY_INTERVAL_MS,
    };
    return &ops;
}

void nipc_service_platform_server_config_from_service(
    nipc_service_platform_server_config_t *transport,
    const nipc_server_config_t *config)
{
    memset(transport, 0, sizeof(*transport));
    nipc_service_common_transport_fields_t fields;

    if (!nipc_service_common_server_transport_fields(&fields, config))
        return;

    NIPC_SERVICE_COMMON_APPLY_TRANSPORT_FIELDS(transport, &fields);
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
    ctx->abort_event = CreateEventW(NULL, TRUE, FALSE, NULL);

    ctx->transport_config = service_client_config_to_transport(config);
    ctx->call_timeout_ms = (config && config->call_timeout_ms != 0)
        ? config->call_timeout_ms
        : NIPC_CLIENT_CALL_TIMEOUT_DEFAULT_MS;
    if (ctx->transport_config.max_request_payload_bytes == 0)
        ctx->transport_config.max_request_payload_bytes =
            nipc_service_common_request_payload_default();
    if (ctx->transport_config.max_response_payload_bytes == 0)
        ctx->transport_config.max_response_payload_bytes =
            nipc_service_common_response_payload_default();
}

bool nipc_client_refresh(nipc_client_ctx_t *ctx)
{
    return nipc_service_common_client_refresh(ctx, nipc_service_win_client_ops());
}

void nipc_client_status(const nipc_client_ctx_t *ctx,
                        nipc_client_status_t *out)
{
    nipc_service_common_client_status(ctx, out);
}

void nipc_client_set_call_timeout(nipc_client_ctx_t *ctx, uint32_t timeout_ms)
{
    if (!ctx)
        return;
    ctx->call_timeout_ms = (timeout_ms != 0)
        ? timeout_ms
        : NIPC_CLIENT_CALL_TIMEOUT_DEFAULT_MS;
}

void nipc_client_abort(nipc_client_ctx_t *ctx)
{
    if (!ctx)
        return;

    __atomic_store_n(&ctx->abort_requested, 1u, __ATOMIC_RELEASE);
    if (ctx->abort_event)
        SetEvent(ctx->abort_event);
}

void nipc_client_clear_abort(nipc_client_ctx_t *ctx)
{
    if (!ctx)
        return;

    if (ctx->abort_event)
        ResetEvent(ctx->abort_event);
    __atomic_store_n(&ctx->abort_requested, 0u, __ATOMIC_RELEASE);
}

void nipc_client_close(nipc_client_ctx_t *ctx)
{
    nipc_service_win_client_disconnect(ctx);
    close_abort_event(ctx);
    __atomic_store_n(&ctx->abort_requested, 0u, __ATOMIC_RELEASE);
    nipc_service_common_client_close_buffers(ctx);
}

#endif /* _WIN32 || __MSYS__ */
