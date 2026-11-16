/*
 * netipc_service_posix_client_connect.c - POSIX client connection management.
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

static bool client_prepare_session_buffers(nipc_client_ctx_t *ctx)
{
    size_t response_need;
    if (!nipc_service_common_header_payload_len(
            ctx->session.max_response_payload_bytes, &response_need))
        return false;
    if (response_need < NIPC_HEADER_LEN + NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES)
        response_need = NIPC_HEADER_LEN + NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES;

    if (!nipc_service_posix_ensure_buffer(&ctx->response_buf, &ctx->response_buf_size, response_need,
                       NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_RESPONSE_BUF_REALLOC_INTERNAL))
        return false;

    if (ctx->session.selected_profile == NIPC_PROFILE_SHM_HYBRID ||
        ctx->session.selected_profile == NIPC_PROFILE_SHM_FUTEX) {
        size_t send_need;
        if (!nipc_service_common_header_payload_len(
                ctx->session.max_request_payload_bytes, &send_need))
            return false;
        if (!nipc_service_posix_ensure_buffer(&ctx->send_buf, &ctx->send_buf_size, send_need,
                           NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_SEND_BUF_REALLOC_INTERNAL))
            return false;
    }

    return true;
}

/* ------------------------------------------------------------------ */
/*  Internal: client connection helpers                                */
/* ------------------------------------------------------------------ */

/* Tear down the current connection (UDS session + SHM if any). */
void nipc_service_posix_client_disconnect(nipc_client_ctx_t *ctx)
{
    if (ctx->shm) {
        nipc_shm_close(ctx->shm);
        free(ctx->shm);
        ctx->shm = NULL;
    }

    if (ctx->session_valid) {
        nipc_uds_close_session(&ctx->session);
        ctx->session_valid = false;
    }
}

static void client_disable_shm_profiles(nipc_client_ctx_t *ctx)
{
    ctx->transport_config.supported_profiles &=
        ~(NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX);
    ctx->transport_config.preferred_profiles &=
        ~(NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX);
}

/* Attempt a full connection: UDS connect + handshake, then SHM upgrade
 * if negotiated. Returns the new state. */
nipc_client_state_t nipc_service_posix_client_try_connect(nipc_client_ctx_t *ctx)
{
    nipc_uds_session_t session;
    memset(&session, 0, sizeof(session));
    session.fd = -1;

    uint32_t remaining = nipc_service_common_client_op_remaining_ms(ctx);
    if (remaining == 0 || nipc_service_common_client_abort_requested(ctx))
        return NIPC_CLIENT_DISCONNECTED;

    int abort_fd = ctx->abort_pipe_valid ? ctx->abort_pipe[0] : -1;
    nipc_uds_error_t err = nipc_uds_connect_timeout(
        ctx->run_dir, ctx->service_name,
        &ctx->transport_config, remaining, abort_fd, &session);

    switch (err) {
    case NIPC_UDS_OK:
        break;
    case NIPC_UDS_ERR_CONNECT:
        return NIPC_CLIENT_NOT_FOUND;
    case NIPC_UDS_ERR_AUTH_FAILED:
        return NIPC_CLIENT_AUTH_FAILED;
    case NIPC_UDS_ERR_NO_PROFILE:
    case NIPC_UDS_ERR_INCOMPATIBLE:
        return NIPC_CLIENT_INCOMPATIBLE;
    default:
        return NIPC_CLIENT_DISCONNECTED;
    }

    ctx->session = session;
    ctx->session_valid = true;

    if (!client_prepare_session_buffers(ctx)) {
        nipc_uds_close_session(&ctx->session);
        ctx->session_valid = false;
        return NIPC_CLIENT_DISCONNECTED;
    }

    /* SHM upgrade if negotiated */
    if (session.selected_profile == NIPC_PROFILE_SHM_HYBRID ||
        session.selected_profile == NIPC_PROFILE_SHM_FUTEX) {

        nipc_shm_ctx_t *shm = nipc_service_posix_calloc(
            1, sizeof(nipc_shm_ctx_t),
            NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_SHM_CTX_CALLOC_INTERNAL);
        if (!shm) {
            nipc_uds_close_session(&ctx->session);
            ctx->session_valid = false;
            return NIPC_CLIENT_DISCONNECTED;
        }
        {
            /* Retry attach: server creates the SHM region after
             * the UDS handshake, so it may not exist yet. */
            nipc_shm_error_t serr = NIPC_SHM_ERR_NOT_READY;
            bool keep_shm = false;
            uint32_t attach_ms = nipc_service_common_client_op_remaining_ms(ctx);
            /* When the call deadline ends the wait first, SHM is not at
             * fault and stays enabled for the next connect, unless that
             * keeps happening (CLIENT_SHM_CALL_BOUND_FAILURE_LIMIT). */
            /* Both deadlines are whole milliseconds on one clock, so a tie
             * ends both waits together and counts as call-bound. */
            bool call_bound = attach_ms <= ctx->shm_attach_timeout_ms;
            if (!call_bound)
                attach_ms = ctx->shm_attach_timeout_ms;
            uint64_t deadline_ms = nipc_service_platform_monotonic_ms() + attach_ms;
            for (;;) {
                serr = nipc_shm_client_attach(
                    ctx->run_dir, ctx->service_name,
                    session.session_id, shm);
                if (serr == NIPC_SHM_OK)
                    break;
                if (serr != NIPC_SHM_ERR_NOT_READY &&
                    serr != NIPC_SHM_ERR_OPEN &&
                    serr != NIPC_SHM_ERR_BAD_MAGIC)
                    break;
                if (nipc_service_common_client_abort_requested(ctx)) {
                    keep_shm = true;
                    break;
                }
                /* A server that refused or died closes the session socket;
                 * waiting for its SHM region would only burn the timeout.
                 * Checked before the deadline, so a peer gone at either
                 * deadline is never taken for an SHM failure. */
                if (nipc_service_posix_peer_disconnected(session.fd)) {
                    keep_shm = true;
                    break;
                }
                if (nipc_service_platform_monotonic_ms() >= deadline_ms) {
                    keep_shm = call_bound &&
                               ++ctx->shm_call_bound_failures <
                                   CLIENT_SHM_CALL_BOUND_FAILURE_LIMIT;
                    break;
                }
                nipc_service_posix_sleep_us(CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS * 1000u);
            }

            if (serr == NIPC_SHM_OK) {
                ctx->shm = shm;
                ctx->shm_call_bound_failures = 0;
            } else if (keep_shm) {
                /* Not an SHM problem: keep SHM enabled for the next connect. */
                free(shm);
                nipc_uds_close_session(&ctx->session);
                ctx->session_valid = false;
                return NIPC_CLIENT_DISCONNECTED;
            } else {
                /* SHM attach failed after negotiation. Close that session,
                 * blacklist SHM for this client context, and retry
                 * baseline via a new handshake. */
                free(shm);
                nipc_uds_close_session(&ctx->session);
                ctx->session_valid = false;
                client_disable_shm_profiles(ctx);
                if (ctx->transport_config.supported_profiles == 0)
                    return NIPC_CLIENT_DISCONNECTED;
                return nipc_service_posix_client_try_connect(ctx);
            }
        }
    }

    return NIPC_CLIENT_READY;
}

/* One reconnect per failed call, as the spec states; the call deadline
 * bounds it. */
bool nipc_service_posix_client_reconnect_for_call(nipc_client_ctx_t *ctx)
{
    ctx->state = nipc_service_posix_client_try_connect(ctx);
    return ctx->state == NIPC_CLIENT_READY;
}
