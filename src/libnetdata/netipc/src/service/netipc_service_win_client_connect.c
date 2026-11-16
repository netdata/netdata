/*
 * netipc_service_win_client_connect.c - WIN client connection management.
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

static bool client_prepare_session_buffers(nipc_client_ctx_t *ctx)
{
    size_t response_need;
    if (!nipc_service_common_header_payload_len(
            ctx->session.max_response_payload_bytes, &response_need))
        return false;
    if (response_need < NIPC_HEADER_LEN + NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES)
        response_need = NIPC_HEADER_LEN + NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES;

    if (!nipc_service_win_ensure_buffer(&ctx->response_buf, &ctx->response_buf_size, response_need,
                       NIPC_WIN_SERVICE_TEST_FAULT_CLIENT_RESPONSE_BUF_REALLOC_INTERNAL))
        return false;

    if (ctx->session.selected_profile == NIPC_WIN_SHM_PROFILE_HYBRID ||
        ctx->session.selected_profile == NIPC_WIN_SHM_PROFILE_BUSYWAIT) {
        size_t send_need;
        if (!nipc_service_common_header_payload_len(
                ctx->session.max_request_payload_bytes, &send_need))
            return false;
        if (!nipc_service_win_ensure_buffer(&ctx->send_buf, &ctx->send_buf_size, send_need,
                           NIPC_WIN_SERVICE_TEST_FAULT_CLIENT_SEND_BUF_REALLOC_INTERNAL))
            return false;
    }

    return true;
}

/* ------------------------------------------------------------------ */
/*  Internal: client connection helpers                                */
/* ------------------------------------------------------------------ */

/* Tear down the current connection (Named Pipe session + Win SHM). */
void nipc_service_win_client_disconnect(nipc_client_ctx_t *ctx)
{
    if (ctx->shm) {
        nipc_win_shm_close(ctx->shm);
        free(ctx->shm);
        ctx->shm = NULL;
    }

    if (ctx->session_valid) {
        nipc_np_close_session(&ctx->session);
        ctx->session_valid = false;
    }
}

static void client_disable_shm_profiles(nipc_client_ctx_t *ctx)
{
    ctx->transport_config.supported_profiles &=
        ~(NIPC_WIN_SHM_PROFILE_HYBRID | NIPC_WIN_SHM_PROFILE_BUSYWAIT);
    ctx->transport_config.preferred_profiles &=
        ~(NIPC_WIN_SHM_PROFILE_HYBRID | NIPC_WIN_SHM_PROFILE_BUSYWAIT);
}

/* Attempt a full connection: Named Pipe connect + handshake, then
 * Win SHM upgrade if negotiated. Returns the new state. */
nipc_client_state_t nipc_service_win_client_try_connect(nipc_client_ctx_t *ctx)
{
    nipc_np_session_t session;
    memset(&session, 0, sizeof(session));
    session.pipe = INVALID_HANDLE_VALUE;

    uint32_t remaining = nipc_service_common_client_op_remaining_ms(ctx);
    if (remaining == 0 || nipc_service_common_client_abort_requested(ctx))
        return NIPC_CLIENT_DISCONNECTED;

    nipc_np_error_t err = nipc_np_connect_timeout(
        ctx->run_dir, ctx->service_name,
        &ctx->transport_config, remaining, ctx->abort_event, &session);

    switch (err) {
    case NIPC_NP_OK:
        break;
    case NIPC_NP_ERR_CONNECT:
        return NIPC_CLIENT_NOT_FOUND;
    case NIPC_NP_ERR_AUTH_FAILED:
    case NIPC_NP_ERR_UNTRUSTED_SERVER:
        return NIPC_CLIENT_AUTH_FAILED;
    case NIPC_NP_ERR_NO_PROFILE:
    case NIPC_NP_ERR_INCOMPATIBLE:
        return NIPC_CLIENT_INCOMPATIBLE;
    default:
        return NIPC_CLIENT_DISCONNECTED;
    }

    ctx->session = session;
    ctx->session_valid = true;

    if (!client_prepare_session_buffers(ctx)) {
        nipc_np_close_session(&ctx->session);
        ctx->session_valid = false;
        return NIPC_CLIENT_DISCONNECTED;
    }

    /* Win SHM upgrade if negotiated */
    if (session.selected_profile == NIPC_WIN_SHM_PROFILE_HYBRID ||
        session.selected_profile == NIPC_WIN_SHM_PROFILE_BUSYWAIT) {

        nipc_win_shm_ctx_t *shm = nipc_service_win_calloc(
            1, sizeof(nipc_win_shm_ctx_t),
            NIPC_WIN_SERVICE_TEST_FAULT_CLIENT_SHM_CTX_CALLOC_INTERNAL);
        if (!shm) {
            nipc_np_close_session(&ctx->session);
            ctx->session_valid = false;
            return NIPC_CLIENT_DISCONNECTED;
        }
        {
            /* Retry attach: the server prepares SHM before accepting the
             * handshake, but filesystem/object visibility may lag briefly. */
            nipc_win_shm_error_t serr = NIPC_WIN_SHM_ERR_OPEN_MAPPING;
            bool keep_shm = false;
            uint32_t attach_ms = nipc_service_common_client_op_remaining_ms(ctx);
            /* When the call deadline ends the wait first, WinSHM is not at
             * fault and stays enabled for the next connect, unless that
             * keeps happening (CLIENT_SHM_CALL_BOUND_FAILURE_LIMIT). */
            /* Both deadlines are whole milliseconds on one clock, so a tie
             * ends both waits together and counts as call-bound. */
            bool call_bound = attach_ms <= ctx->shm_attach_timeout_ms;
            if (!call_bound)
                attach_ms = ctx->shm_attach_timeout_ms;
            ULONGLONG deadline_ms = GetTickCount64() + attach_ms;
            for (;;) {
                serr = nipc_win_shm_client_attach(
                    ctx->run_dir, ctx->service_name,
                    ctx->transport_config.auth_token,
                    session.session_id,
                    session.selected_profile,
                    shm);
                if (serr == NIPC_WIN_SHM_OK)
                    break;
                if (serr != NIPC_WIN_SHM_ERR_OPEN_MAPPING &&
                    serr != NIPC_WIN_SHM_ERR_OPEN_EVENT &&
                    serr != NIPC_WIN_SHM_ERR_BAD_MAGIC)
                    break;
                if (nipc_service_common_client_abort_requested(ctx)) {
                    keep_shm = true;
                    break;
                }
                /* A server that refused or died breaks the session pipe;
                 * waiting for its SHM objects would only burn the timeout.
                 * Checked before the deadline, so a peer gone at either
                 * deadline is never taken for a WinSHM failure. */
                if (nipc_service_win_peer_disconnected(&session)) {
                    keep_shm = true;
                    break;
                }
                if (GetTickCount64() >= deadline_ms) {
                    keep_shm = call_bound &&
                               ++ctx->shm_call_bound_failures <
                                   CLIENT_SHM_CALL_BOUND_FAILURE_LIMIT;
                    break;
                }
                Sleep(CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS);
            }

            if (serr == NIPC_WIN_SHM_OK) {
                ctx->shm = shm;
                ctx->shm_call_bound_failures = 0;
            } else if (keep_shm) {
                /* Not a WinSHM problem: keep WinSHM enabled for the next connect. */
                free(shm);
                nipc_np_close_session(&ctx->session);
                ctx->session_valid = false;
                return NIPC_CLIENT_DISCONNECTED;
            } else {
                /* WinSHM attach failed after negotiation. Close that
                 * session, blacklist WinSHM for this client context, and
                 * retry baseline via a new handshake. */
                free(shm);
                nipc_np_close_session(&ctx->session);
                ctx->session_valid = false;
                client_disable_shm_profiles(ctx);
                if (ctx->transport_config.supported_profiles == 0)
                    return NIPC_CLIENT_DISCONNECTED;
                return nipc_service_win_client_try_connect(ctx);
            }
        }
    }

    return NIPC_CLIENT_READY;
}

/* One reconnect per failed call, as the spec states; the call deadline
 * bounds it. */
bool nipc_service_win_client_reconnect_for_call(nipc_client_ctx_t *ctx)
{
    ctx->state = nipc_service_win_client_try_connect(ctx);
    return ctx->state == NIPC_CLIENT_READY;
}

#endif /* _WIN32 || __MSYS__ */
