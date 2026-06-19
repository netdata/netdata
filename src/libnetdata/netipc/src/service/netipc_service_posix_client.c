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
#include <errno.h>
#include <fcntl.h>
#include <unistd.h>

static nipc_uds_client_config_t service_client_config_to_transport(
    const nipc_client_config_t *config)
{
    nipc_uds_client_config_t transport = {0};
    nipc_service_common_transport_fields_t fields;

    if (!nipc_service_common_client_transport_fields(&fields, config))
        return transport;

    NIPC_SERVICE_COMMON_APPLY_TRANSPORT_FIELDS(&transport, &fields);
    return transport;
}

static void client_sleep_ms(uint32_t ms)
{
    nipc_service_posix_sleep_us(ms * 1000u);
}

static void close_abort_pipe(nipc_client_ctx_t *ctx)
{
    if (!ctx->abort_pipe_valid)
        return;

    if (ctx->abort_pipe[0] >= 0) {
        close(ctx->abort_pipe[0]);
        ctx->abort_pipe[0] = -1;
    }
    if (ctx->abort_pipe[1] >= 0) {
        close(ctx->abort_pipe[1]);
        ctx->abort_pipe[1] = -1;
    }
    ctx->abort_pipe_valid = false;
}

static void drain_abort_pipe(nipc_client_ctx_t *ctx)
{
    if (!ctx->abort_pipe_valid || ctx->abort_pipe[0] < 0)
        return;

    char buf[64];
    for (;;) {
        ssize_t n = read(ctx->abort_pipe[0], buf, sizeof(buf));
        if (n > 0)
            continue;
        if (n < 0 && errno == EINTR)
            continue;
        break;
    }
}

static void init_abort_pipe(nipc_client_ctx_t *ctx)
{
    ctx->abort_pipe[0] = -1;
    ctx->abort_pipe[1] = -1;
    ctx->abort_pipe_valid = false;

    int fds[2];
    if (pipe(fds) != 0)
        return;

    for (int i = 0; i < 2; i++) {
        int flags = fcntl(fds[i], F_GETFL, 0);
        if (flags >= 0)
            (void)fcntl(fds[i], F_SETFL, flags | O_NONBLOCK);
    }

    ctx->abort_pipe[0] = fds[0];
    ctx->abort_pipe[1] = fds[1];
    ctx->abort_pipe_valid = true;
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
    ctx->session.fd = -1;
    init_abort_pipe(ctx);

    ctx->transport_config = service_client_config_to_transport(config);
    ctx->call_timeout_ms = (config && config->call_timeout_ms != 0)
        ? config->call_timeout_ms
        : NIPC_CLIENT_CALL_TIMEOUT_DEFAULT_MS;
    nipc_service_common_client_apply_logical_lookup_config(ctx, config);
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
    if (ctx->abort_pipe_valid && ctx->abort_pipe[1] >= 0) {
        const char b = 1;
        ssize_t n;
        do {
            n = write(ctx->abort_pipe[1], &b, 1);
        } while (n < 0 && errno == EINTR);
    }
}

void nipc_client_clear_abort(nipc_client_ctx_t *ctx)
{
    if (!ctx)
        return;

    drain_abort_pipe(ctx);
    __atomic_store_n(&ctx->abort_requested, 0u, __ATOMIC_RELEASE);
}

void nipc_client_close(nipc_client_ctx_t *ctx)
{
    nipc_service_posix_client_disconnect(ctx);
    close_abort_pipe(ctx);
    __atomic_store_n(&ctx->abort_requested, 0u, __ATOMIC_RELEASE);
    nipc_service_common_client_close_buffers(ctx);
}
