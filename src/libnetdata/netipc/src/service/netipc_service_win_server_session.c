/*
 * netipc_service_win_server_session.c - Windows managed server session loop.
 */

#if defined(_WIN32) || defined(__MSYS__)

#include "netipc/netipc_service.h"
#include "netipc/netipc_protocol.h"
#include "netipc/netipc_named_pipe.h"
#include "netipc/netipc_win_shm.h"
#include "netipc_service_common.h"
#include "netipc_service_win_internal.h"

#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <windows.h>

static void server_handle_session(nipc_managed_server_t *server,
                                   nipc_np_session_t *session,
                                   nipc_win_shm_ctx_t *shm,
                                   uint8_t *resp_buf,
                                   size_t resp_buf_size)
{
    /* Dynamically allocate recv buffer based on negotiated max */
    size_t recv_size;
    if (!nipc_service_common_header_payload_len(
            session->max_request_payload_bytes, &recv_size))
        return;
    if (recv_size < NIPC_HEADER_LEN + NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES)
        recv_size = NIPC_HEADER_LEN + NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES;
    uint8_t *recv_buf = nipc_service_win_malloc(
        recv_size, NIPC_WIN_SERVICE_TEST_FAULT_SERVER_RECV_BUF_MALLOC_INTERNAL);
    if (!recv_buf)
        return;

    while (InterlockedCompareExchange(&server->running, 0, 0)) {
        nipc_header_t hdr;
        const void *payload;
        size_t payload_len;

        /* Receive request via the active transport */
        if (shm) {
            size_t msg_len;
            nipc_win_shm_error_t serr = nipc_win_shm_receive(shm, recv_buf, recv_size,
                                                               &msg_len, SERVER_POLL_TIMEOUT_MS);
            if (serr == NIPC_WIN_SHM_ERR_TIMEOUT)
                continue;
            if (serr != NIPC_WIN_SHM_OK)
                break;
            if (msg_len < NIPC_HEADER_LEN)
                break;

            nipc_error_t perr = nipc_header_decode(recv_buf, msg_len, &hdr);
            if (perr != NIPC_OK)
                break;

            payload = recv_buf + NIPC_HEADER_LEN;
            payload_len = msg_len - NIPC_HEADER_LEN;
        } else {
            /* Named Pipe path: wait for readability first, then receive.
             * This mirrors the Go/Rust Windows server loops and avoids
             * relying on a blocking ReadFile wake-up for each ping-pong
             * request. */
            bool readable = false;
            nipc_np_error_t werr = nipc_np_wait_readable(
                session, SERVER_POLL_TIMEOUT_MS, &readable);
            if (werr == NIPC_NP_ERR_DISCONNECTED)
                break;
            if (werr != NIPC_NP_OK)
                break;
            if (!readable)
                continue;

            nipc_np_error_t uerr = nipc_np_receive(
                session, recv_buf, recv_size,
                &hdr, &payload, &payload_len);
            if (uerr == NIPC_NP_ERR_LIMIT_EXCEEDED) {
                if (hdr.kind == NIPC_KIND_REQUEST) {
                    if (hdr.payload_len > 0)
                        nipc_service_common_server_note_request_capacity(
                            server, hdr.payload_len);

                    nipc_header_t resp_hdr = {0};
                    resp_hdr.kind = NIPC_KIND_RESPONSE;
                    resp_hdr.code = hdr.code;
                    resp_hdr.message_id = hdr.message_id;
                    resp_hdr.transport_status = NIPC_STATUS_LIMIT_EXCEEDED;
                    resp_hdr.item_count = 1;
                    resp_hdr.flags = 0;

                    if (nipc_np_send(session, &resp_hdr, NULL, 0) != NIPC_NP_OK)
                        break;
                }
                break;
            }
            if (uerr != NIPC_NP_OK)
                break;
        }

        /* Protocol violation: unexpected message kind terminates session */
        if (hdr.kind != NIPC_KIND_REQUEST)
            break;

        if (hdr.code != server->expected_method_code) {
            nipc_header_t resp_hdr = {0};
            resp_hdr.kind             = NIPC_KIND_RESPONSE;
            resp_hdr.code             = hdr.code;
            resp_hdr.message_id       = hdr.message_id;
            resp_hdr.transport_status = NIPC_STATUS_UNSUPPORTED;
            resp_hdr.item_count       = 1;
            resp_hdr.flags            = 0;

            if (shm) {
                uint8_t msg[NIPC_HEADER_LEN];
                resp_hdr.magic = NIPC_MAGIC_MSG;
                resp_hdr.version = NIPC_VERSION;
                resp_hdr.header_len = NIPC_HEADER_LEN;
                resp_hdr.payload_len = 0;
                nipc_header_encode(&resp_hdr, msg, sizeof(msg));
                if (nipc_win_shm_send(shm, msg, sizeof(msg)) != NIPC_WIN_SHM_OK)
                    break;
            } else {
                if (nipc_np_send(session, &resp_hdr, NULL, 0) != NIPC_NP_OK)
                    break;
            }
            continue;
        }

        if (payload_len <= UINT32_MAX)
            nipc_service_common_server_note_request_capacity(
                server, (uint32_t)payload_len);

        /* Dispatch against the negotiated payload limit, not the larger
         * scratch allocation, so codec builders can return item-level
         * overflow statuses before transport-level resize fallback. */
        size_t dispatch_resp_size = (size_t)session->max_response_payload_bytes;
        if (dispatch_resp_size > resp_buf_size)
            dispatch_resp_size = resp_buf_size;

        /* Dispatch: one request kind per service endpoint. */
        size_t response_len = 0;
        nipc_error_t dispatch_err = server->handler(
            server->handler_user,
            &hdr,
            (const uint8_t *)payload, payload_len,
            resp_buf, dispatch_resp_size,
            &response_len);

        /* Build response header */
        nipc_header_t resp_hdr;
        bool close_after_response = false;
        nipc_service_common_prepare_response_header(&hdr, &resp_hdr);
        nipc_service_common_apply_dispatch_result(
            server, dispatch_err, dispatch_resp_size,
            session->max_response_payload_bytes, true,
            &resp_hdr, &response_len, &close_after_response);

        /* Send response via the active transport */
        if (shm) {
            size_t msg_len;
            if (!nipc_service_common_header_payload_len(response_len, &msg_len))
                break;

            resp_hdr.magic      = NIPC_MAGIC_MSG;
            resp_hdr.version    = NIPC_VERSION;
            resp_hdr.header_len = NIPC_HEADER_LEN;
            resp_hdr.payload_len = (uint32_t)response_len;

            uint8_t stack_msg[4096];
            uint8_t *msg = (msg_len <= sizeof(stack_msg)) ? stack_msg : malloc(msg_len);
            if (!msg)
                break;

            nipc_header_encode(&resp_hdr, msg, NIPC_HEADER_LEN);
            if (response_len > 0)
                memcpy(msg + NIPC_HEADER_LEN, resp_buf, response_len);

            nipc_win_shm_error_t serr = nipc_win_shm_send(shm, msg, msg_len);
            if (msg != stack_msg)
                free(msg);
            if (serr != NIPC_WIN_SHM_OK)
                break;
        } else {
            nipc_np_error_t uerr = nipc_np_send(
                session, &resp_hdr, resp_buf, response_len);
            if (uerr != NIPC_NP_OK)
                break;
        }

        if (close_after_response)
            break;
    }

    free(recv_buf);
}

/* ------------------------------------------------------------------ */
/*  Internal: per-session handler thread                                */
/* ------------------------------------------------------------------ */

/* Thread function: handles one client session from accept to disconnect. */
unsigned __stdcall nipc_service_win_session_handler_thread(void *arg)
{
    nipc_session_ctx_t *sctx = (nipc_session_ctx_t *)arg;
    nipc_managed_server_t *server = sctx->server;
    /* Allocate a per-session response buffer */
    size_t resp_size = (size_t)sctx->session.max_response_payload_bytes;
    if (resp_size < NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES)
        resp_size = NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES;
    uint8_t *resp_buf = nipc_service_win_malloc(
        resp_size, NIPC_WIN_SERVICE_TEST_FAULT_SERVER_RESP_BUF_MALLOC_INTERNAL);
    if (resp_buf) {
        server_handle_session(server, &sctx->session, sctx->shm,
                              resp_buf, resp_size);
        free(resp_buf);
    }

    /* Cleanup SHM and session */
    if (sctx->shm) {
        nipc_win_shm_destroy(sctx->shm);
        free(sctx->shm);
    }
    nipc_np_close_session(&sctx->session);

    /* Mark inactive; the reap/destroy path owns removal from the array */
    InterlockedExchange((volatile LONG *)&sctx->active, 0);
    return 0;
}

/* ------------------------------------------------------------------ */
/*  Internal: reap finished session threads                             */
/* ------------------------------------------------------------------ */

/* Reap all finished (inactive) session threads. Called with lock held. */
void nipc_service_win_server_reap_sessions_locked(nipc_managed_server_t *server)
{
    int i = 0;
    while (i < server->session_count) {
        nipc_session_ctx_t *s = server->sessions[i];
        if (!InterlockedCompareExchange((volatile LONG *)&s->active, 0, 0)) {
            WaitForSingleObject(s->thread, INFINITE);
            CloseHandle(s->thread);
            /* Swap with last, free */
            server->sessions[i] = server->sessions[server->session_count - 1];
            server->session_count--;
            free(s);
        } else {
            i++;
        }
    }

}


#endif /* _WIN32 || __MSYS__ */
