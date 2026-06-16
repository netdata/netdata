/*
 * netipc_service_win_client_call.c - WIN raw client call flow.
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

/* ------------------------------------------------------------------ */
/*  Internal: send/receive via the active transport                    */
/* ------------------------------------------------------------------ */

static nipc_error_t transport_send(nipc_client_ctx_t *ctx,
                                    nipc_header_t *hdr,
                                    const void *payload,
                                    size_t payload_len)
{
    if (payload_len > UINT32_MAX)
        return NIPC_ERR_OVERFLOW;

    if (ctx->shm) {
        uint8_t *msg;
        size_t msg_len;
        nipc_error_t perr = nipc_service_common_client_prepare_shm_request(
            ctx, hdr, payload, payload_len, &msg, &msg_len);
        if (perr != NIPC_OK)
            return perr;

        nipc_win_shm_error_t serr = nipc_win_shm_send(ctx->shm, msg, msg_len);
        if (serr == NIPC_WIN_SHM_ERR_MSG_TOO_LARGE) {
            nipc_service_common_client_note_request_capacity(
                ctx, (uint32_t)payload_len);
            return NIPC_ERR_OVERFLOW;
        }
        return (serr == NIPC_WIN_SHM_OK) ? NIPC_OK : NIPC_ERR_NOT_READY;
    }

    /* Named Pipe path */
    nipc_np_error_t uerr = nipc_np_send(&ctx->session, hdr,
                                          payload, payload_len);
    if (uerr == NIPC_NP_ERR_LIMIT_EXCEEDED) {
        nipc_service_common_client_note_request_capacity(
            ctx, (uint32_t)payload_len);
        return NIPC_ERR_OVERFLOW;
    }
    return (uerr == NIPC_NP_OK) ? NIPC_OK : NIPC_ERR_NOT_READY;
}

static nipc_error_t transport_receive(nipc_client_ctx_t *ctx,
                                       void *buf, size_t buf_size,
                                       nipc_header_t *hdr_out,
                                       const void **payload_out,
                                       size_t *payload_len_out,
                                       uint32_t timeout_ms)
{
    if (ctx->shm) {
        size_t msg_len;
        uint64_t deadline = nipc_service_platform_monotonic_ms() + timeout_ms;

        for (;;) {
            if (nipc_service_common_client_abort_requested(ctx))
                return NIPC_ERR_ABORTED;

            uint64_t now = nipc_service_platform_monotonic_ms();
            if (timeout_ms != 0 && now >= deadline)
                return NIPC_ERR_TIMEOUT;

            uint32_t wait_ms = NIPC_CLIENT_ABORT_POLL_MS;
            if (timeout_ms != 0) {
                uint64_t remaining = deadline - now;
                if (remaining < wait_ms)
                    wait_ms = (uint32_t)remaining;
                if (wait_ms == 0)
                    wait_ms = 1;
            }

            nipc_win_shm_error_t serr = nipc_win_shm_receive(
                ctx->shm, buf, buf_size, &msg_len, wait_ms);
            if (serr == NIPC_WIN_SHM_OK)
                break;
            if (serr == NIPC_WIN_SHM_ERR_TIMEOUT)
                continue;
            return NIPC_ERR_TRUNCATED;
        }

        return nipc_service_common_client_parse_shm_response(
            buf, msg_len, hdr_out, payload_out, payload_len_out);
    }

    /* Named Pipe path */
    if (nipc_service_common_client_abort_requested(ctx))
        return NIPC_ERR_ABORTED;

    nipc_np_error_t uerr = nipc_np_receive_timeout(
        &ctx->session, buf, buf_size, hdr_out, payload_out, payload_len_out,
        timeout_ms, ctx->abort_event);
    switch (uerr) {
    case NIPC_NP_OK:
        return NIPC_OK;
    case NIPC_NP_ERR_TIMEOUT:
        return NIPC_ERR_TIMEOUT;
    case NIPC_NP_ERR_ABORTED:
        return NIPC_ERR_ABORTED;
    case NIPC_NP_ERR_LIMIT_EXCEEDED:
        return NIPC_ERR_OVERFLOW;
    default:
        return NIPC_ERR_TRUNCATED;
    }
}

/* ------------------------------------------------------------------ */
/*  Internal: generic raw call (send request, receive response)        */
/* ------------------------------------------------------------------ */

/*
 * Single-attempt raw call: build envelope, send, receive, validate
 * envelope. The caller handles encode before and decode after.
 *
 * On success, response_payload_out and response_len_out point into the
 * internal client response buffer (valid until next call on this context).
 */
nipc_error_t nipc_service_platform_do_raw_call(nipc_client_ctx_t *ctx,
                                               uint16_t method_code,
                                               const void *request_payload,
                                               size_t request_len,
                                               const void **response_payload_out,
                                               size_t *response_len_out,
                                               uint32_t timeout_ms)
{
    return nipc_service_common_do_raw_call(
        ctx, method_code, request_payload, request_len,
        response_payload_out, response_len_out,
        nipc_service_common_client_call_timeout_ms(ctx, timeout_ms),
        transport_send, transport_receive);
}

/*
 * Generic call-with-retry:
 *   - ordinary failures reconnect and retry once
 *   - overflow-driven resize recovery may reconnect repeatedly until
 *     negotiated capacities grow or recovery fails
 * The caller provides a function pointer for the single-attempt logic.
 */
nipc_error_t nipc_service_platform_call_with_retry(
    nipc_client_ctx_t *ctx,
    nipc_service_platform_attempt_fn attempt,
    void *state)
{
    return nipc_service_common_call_with_retry(
        ctx, attempt, state, nipc_service_win_client_ops());
}

nipc_error_t nipc_service_platform_ensure_request_capacity(
    nipc_client_ctx_t *ctx, size_t required_payload_bytes)
{
    return nipc_service_common_client_ensure_request_capacity(
        ctx, required_payload_bytes, nipc_service_win_client_ops());
}


#endif /* _WIN32 || __MSYS__ */
