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

    nipc_np_error_t err = nipc_np_connect(
        ctx->run_dir, ctx->service_name,
        &ctx->transport_config, &session);

    switch (err) {
    case NIPC_NP_OK:
        break;
    case NIPC_NP_ERR_CONNECT:
        return NIPC_CLIENT_NOT_FOUND;
    case NIPC_NP_ERR_AUTH_FAILED:
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
            ULONGLONG deadline_ms = GetTickCount64() + CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS;
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
                if (GetTickCount64() >= deadline_ms)
                    break;
                Sleep(CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS);
            }

            if (serr == NIPC_WIN_SHM_OK) {
                ctx->shm = shm;
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

bool nipc_service_win_client_reconnect_for_call(nipc_client_ctx_t *ctx)
{
    for (uint32_t i = 0; i < CLIENT_CALL_RECONNECT_RETRIES; i++) {
        ctx->state = nipc_service_win_client_try_connect(ctx);
        if (ctx->state == NIPC_CLIENT_READY)
            return true;
        if (ctx->state == NIPC_CLIENT_AUTH_FAILED ||
            ctx->state == NIPC_CLIENT_INCOMPATIBLE)
            return false;
        if (i + 1u < CLIENT_CALL_RECONNECT_RETRIES)
            Sleep(CLIENT_CALL_RECONNECT_RETRY_INTERVAL_MS);
    }

    return false;
}


#endif /* _WIN32 || __MSYS__ */
