/*
 * netipc_service_posix_server_session.c - POSIX managed server session loop.
 */

#include "netipc/netipc_service.h"
#include "netipc/netipc_protocol.h"
#include "netipc/netipc_uds.h"
#include "netipc/netipc_shm.h"
#include "netipc_service_common.h"
#include "netipc_service_posix_internal.h"

#include <errno.h>
#include <poll.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>

int nipc_service_posix_poll_with_shutdown(int fd, bool *running)
{
    while (__atomic_load_n(running, __ATOMIC_RELAXED)) {
        struct pollfd pfd = { .fd = fd, .events = POLLIN };
        int ret = poll(&pfd, 1, SERVER_POLL_TIMEOUT_MS);

        if (ret < 0) {
            if (errno == EINTR)
                continue;
            return -1;
        }

        if (ret == 0)
            continue; /* timeout, check running flag */

        if (pfd.revents & (POLLERR | POLLHUP | POLLNVAL))
            return -1;

        if (pfd.revents & POLLIN)
            return 1;
    }
    return 0;
}

static bool session_peer_disconnected(int fd)
{
    if (fd < 0)
        return true;

    struct pollfd pfd = {
        .fd = fd,
        .events = POLLIN | POLLHUP | POLLERR,
    };
#ifdef POLLRDHUP
    pfd.events |= POLLRDHUP;
#endif

    int ret;
    do {
        ret = poll(&pfd, 1, 0);
    } while (ret < 0 && errno == EINTR);

    if (ret < 0)
        return true;
    if (ret == 0)
        return false;

    short disconnected = POLLHUP | POLLERR | POLLNVAL;
#ifdef POLLRDHUP
    disconnected |= POLLRDHUP;
#endif
    if (pfd.revents & disconnected)
        return true;

    if (pfd.revents & POLLIN) {
        char ch;
        ssize_t n;
        do {
            n = recv(fd, &ch, sizeof(ch), MSG_PEEK | MSG_DONTWAIT);
        } while (n < 0 && errno == EINTR);

        if (n == 0)
            return true;
        if (n < 0 && errno != EAGAIN && errno != EWOULDBLOCK)
            return true;
    }

    return false;
}

/*
 * Handle one client session: read requests, dispatch to handler,
 * send responses. Each session gets its own response buffer.
 * Runs until the client disconnects or server stops.
 */
static void server_handle_session(nipc_managed_server_t *server,
                                   nipc_uds_session_t *session,
                                   nipc_shm_ctx_t *shm,
                                   uint8_t *resp_buf,
                                   size_t resp_buf_size)
{
    /* Allocate recv buffer based on negotiated max request size */
    size_t recv_size;
    if (!nipc_service_common_header_payload_len(
            session->max_request_payload_bytes, &recv_size))
        return;
    if (recv_size < NIPC_HEADER_LEN + NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES)
        recv_size = NIPC_HEADER_LEN + NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES;
    uint8_t *recv_buf = nipc_service_posix_malloc(
        recv_size, NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_RECV_BUF_MALLOC_INTERNAL);
    if (!recv_buf)
        return;

    while (__atomic_load_n(&server->running, __ATOMIC_RELAXED)) {
        nipc_header_t hdr;
        const void *payload;
        size_t payload_len;

        /* Receive request via the active transport */
        if (shm) {
            size_t msg_len;
            nipc_shm_error_t serr = nipc_shm_receive(shm, recv_buf, recv_size,
                                                       &msg_len, SERVER_POLL_TIMEOUT_MS);
            if (serr == NIPC_SHM_ERR_TIMEOUT) {
                if (session_peer_disconnected(session->fd))
                    break;
                continue; /* check running flag */
            }
            if (serr != NIPC_SHM_OK)
                break;
            if (msg_len < NIPC_HEADER_LEN)
                break;

            nipc_error_t perr = nipc_header_decode(recv_buf, msg_len, &hdr);
            if (perr != NIPC_OK)
                break;

            payload = recv_buf + NIPC_HEADER_LEN;
            payload_len = msg_len - NIPC_HEADER_LEN;
        } else {
            /* Poll the session fd before blocking on receive */
            int pr = nipc_service_posix_poll_with_shutdown(session->fd, &server->running);
            if (pr <= 0)
                break; /* shutdown or error */

            nipc_uds_error_t uerr = nipc_uds_receive(
                session, recv_buf, recv_size,
                &hdr, &payload, &payload_len);
            if (uerr == NIPC_UDS_ERR_LIMIT_EXCEEDED) {
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

                    if (nipc_uds_send(session, &resp_hdr, NULL, 0) != NIPC_UDS_OK)
                        break;
                }
                break;
            }
            if (uerr != NIPC_UDS_OK)
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
                if (nipc_shm_send(shm, msg, sizeof(msg)) != NIPC_SHM_OK)
                    break;
            } else {
                if (nipc_uds_send(session, &resp_hdr, NULL, 0) != NIPC_UDS_OK)
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
            session->max_response_payload_bytes, false,
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

            /* Use a stack buffer for small responses, heap for large ones */
            uint8_t stack_msg[4096];
            uint8_t *msg = (msg_len <= sizeof(stack_msg)) ? stack_msg :
                nipc_service_posix_malloc(msg_len, NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_RESP_BUF_MALLOC_INTERNAL);
            if (!msg)
                break;

            nipc_header_encode(&resp_hdr, msg, NIPC_HEADER_LEN);
            if (response_len > 0)
                memcpy(msg + NIPC_HEADER_LEN, resp_buf, response_len);

            nipc_shm_error_t serr = nipc_shm_send(shm, msg, msg_len);
            if (msg != stack_msg)
                free(msg);
            if (serr != NIPC_SHM_OK)
                break;
        } else {
            nipc_uds_error_t uerr = nipc_uds_send(
                session, &resp_hdr, resp_buf, response_len);
            if (uerr != NIPC_UDS_OK)
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
void *nipc_service_posix_session_handler_thread(void *arg)
{
    nipc_posix_session_start_arg_t *start = (nipc_posix_session_start_arg_t *)arg;
    pthread_mutex_lock(&start->lock);
    nipc_session_ctx_t *sctx = start->session;
    nipc_managed_server_t *server = start->server;
    start->copied = true;
    pthread_cond_signal(&start->copied_cond);
    pthread_mutex_unlock(&start->lock);

    /* Allocate a per-session response buffer */
    size_t resp_size = (size_t)sctx->session.max_response_payload_bytes;
    if (resp_size < NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES)
        resp_size = NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES;
    uint8_t *resp_buf = nipc_service_posix_malloc(
        resp_size, NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_RESP_BUF_MALLOC_INTERNAL);
    if (resp_buf) {
        server_handle_session(server, &sctx->session, sctx->shm,
                              resp_buf, resp_size);
        free(resp_buf);
    }

    /* Cleanup SHM and session */
    if (sctx->shm) {
        nipc_shm_destroy(sctx->shm);
        free(sctx->shm);
    }
    nipc_uds_close_session(&sctx->session);

    /* Mark inactive so the acceptor's reap loop (or server destroy)
     * can join this thread and free sctx.  Do NOT remove from the
     * tracking array here — the reap/destroy path owns that. */
    __atomic_store_n(&sctx->active, false, __ATOMIC_RELEASE);
    return NULL;
}

/* ------------------------------------------------------------------ */
/*  Internal: reap finished session threads                             */
/* ------------------------------------------------------------------ */

/* Reap all finished (inactive) session threads. Called with lock held. */
void nipc_service_posix_server_reap_sessions_locked(nipc_managed_server_t *server)
{
    int i = 0;
    while (i < server->session_count) {
        nipc_session_ctx_t *s = server->sessions[i];
        if (!__atomic_load_n(&s->active, __ATOMIC_ACQUIRE)) {
            pthread_join(s->thread, NULL);
            /* Swap with last, free */
            server->sessions[i] = server->sessions[server->session_count - 1];
            server->session_count--;
            free(s);
        } else {
            i++;
        }
    }

}
