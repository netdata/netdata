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

    nipc_uds_error_t err = nipc_uds_connect(
        ctx->run_dir, ctx->service_name,
        &ctx->transport_config, &session);

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
            uint64_t deadline_ms = nipc_service_platform_monotonic_ms() + CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS;
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
                if (nipc_service_platform_monotonic_ms() >= deadline_ms)
                    break;
                nipc_service_posix_sleep_us(CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS * 1000u);
            }

            if (serr == NIPC_SHM_OK) {
                ctx->shm = shm;
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

bool nipc_service_posix_client_reconnect_for_call(nipc_client_ctx_t *ctx)
{
    for (uint32_t i = 0; i < CLIENT_CALL_RECONNECT_RETRIES; i++) {
        ctx->state = nipc_service_posix_client_try_connect(ctx);
        if (ctx->state == NIPC_CLIENT_READY)
            return true;
        if (ctx->state == NIPC_CLIENT_AUTH_FAILED ||
            ctx->state == NIPC_CLIENT_INCOMPATIBLE)
            return false;
        if (i + 1u < CLIENT_CALL_RECONNECT_RETRIES)
            nipc_service_posix_sleep_us(CLIENT_CALL_RECONNECT_RETRY_INTERVAL_MS * 1000u);
    }

    return false;
}
