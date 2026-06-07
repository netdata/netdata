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
        if (payload_len > ctx->session.max_request_payload_bytes) {
            nipc_service_common_client_note_request_capacity(
                ctx, (uint32_t)payload_len);
            return NIPC_ERR_OVERFLOW;
        }

        size_t msg_len;
        if (!nipc_service_common_header_payload_len(payload_len, &msg_len))
            return NIPC_ERR_OVERFLOW;

        uint8_t *msg = ctx->send_buf;
        if (!msg || msg_len > ctx->send_buf_size)
            return NIPC_ERR_OVERFLOW;

        if (payload_len > 0)
            memmove(msg + NIPC_HEADER_LEN, payload, payload_len);

        hdr->magic      = NIPC_MAGIC_MSG;
        hdr->version    = NIPC_VERSION;
        hdr->header_len = NIPC_HEADER_LEN;
        hdr->payload_len = (uint32_t)payload_len;

        nipc_header_encode(hdr, msg, NIPC_HEADER_LEN);

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
                                       size_t *payload_len_out)
{
    if (ctx->shm) {
        size_t msg_len;
        nipc_win_shm_error_t serr = nipc_win_shm_receive(ctx->shm, buf, buf_size,
                                                            &msg_len, 30000);
        if (serr != NIPC_WIN_SHM_OK)
            return NIPC_ERR_TRUNCATED;

        if (msg_len < NIPC_HEADER_LEN)
            return NIPC_ERR_TRUNCATED;

        nipc_error_t perr = nipc_header_decode(buf, msg_len, hdr_out);
        if (perr != NIPC_OK)
            return perr;

        *payload_out = (const uint8_t *)buf + NIPC_HEADER_LEN;
        *payload_len_out = msg_len - NIPC_HEADER_LEN;
        return NIPC_OK;
    }

    /* Named Pipe path */
    nipc_np_error_t uerr = nipc_np_receive(&ctx->session, buf, buf_size,
                                             hdr_out, payload_out,
                                             payload_len_out);
    return (uerr == NIPC_NP_OK) ? NIPC_OK : NIPC_ERR_TRUNCATED;
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
                                               size_t *response_len_out)
{
    return nipc_service_common_do_raw_call(
        ctx, method_code, request_payload, request_len,
        response_payload_out, response_len_out,
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


#endif /* _WIN32 || __MSYS__ */
