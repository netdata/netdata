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
#include <time.h>
#include <unistd.h>

int nipc_service_posix_poll_with_shutdown(int fd, bool *running,
                                          uint32_t poll_ms)
{
    while (__atomic_load_n(running, __ATOMIC_RELAXED)) {
        struct pollfd pfd = { .fd = fd, .events = POLLIN };
        int ret = poll(&pfd, 1, (int)poll_ms);

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

bool nipc_service_posix_peer_disconnected(int fd)
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
                                                       &msg_len, server->poll_ms);
            if (serr == NIPC_SHM_ERR_TIMEOUT) {
                if (nipc_service_posix_peer_disconnected(session->fd))
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
            int pr = nipc_service_posix_poll_with_shutdown(session->fd, &server->running,
                                                           server->poll_ms);
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

static void server_destroy_precreated_shm(nipc_shm_ctx_t **shm) {
    if (!shm || !*shm)
        return;
    nipc_shm_destroy(*shm);
    free(*shm);
    *shm = NULL;
}

static bool server_prepare_accept_config(nipc_managed_server_t *server,
                                         nipc_uds_server_config_t *cfg_out) {
    *cfg_out = server->base_config;
    cfg_out->max_request_payload_bytes =
            __atomic_load_n(&server->learned_request_payload_bytes, __ATOMIC_ACQUIRE);
    cfg_out->max_response_payload_bytes = __atomic_load_n(
            &server->learned_response_payload_bytes, __ATOMIC_ACQUIRE);

    uint32_t shm_profiles = cfg_out->supported_profiles &
                                                    (NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX);
    uint32_t request_capacity;
    uint32_t response_capacity;
    if (shm_profiles != 0 &&
            (!nipc_service_common_header_payload_len_u32(
                    cfg_out->max_request_payload_bytes, &request_capacity) ||
             !nipc_service_common_header_payload_len_u32(
                    cfg_out->max_response_payload_bytes, &response_capacity))) {
        cfg_out->supported_profiles &= ~shm_profiles;
        cfg_out->preferred_profiles &= ~shm_profiles;
    }
    return cfg_out->supported_profiles != 0;
}

typedef struct {
    nipc_managed_server_t *server;
    uint64_t session_id;
    nipc_shm_ctx_t *shm;
} server_shm_prepare_t;

/* Handshake prepare step: create the session's SHM region once the
 * negotiation selected SHM, sized for the agreed limits. */
static bool server_prepare_profile(void *user, uint32_t profile,
                                   uint32_t max_request_payload_bytes,
                                   uint32_t max_response_payload_bytes) {
    server_shm_prepare_t *prep = (server_shm_prepare_t *)user;
    if (profile != NIPC_PROFILE_SHM_HYBRID && profile != NIPC_PROFILE_SHM_FUTEX)
        return true;

    uint32_t request_capacity;
    uint32_t response_capacity;
    if (!nipc_service_common_header_payload_len_u32(max_request_payload_bytes,
                                                    &request_capacity) ||
        !nipc_service_common_header_payload_len_u32(max_response_payload_bytes,
                                                    &response_capacity))
        return false;

    nipc_shm_ctx_t *shm = nipc_service_posix_calloc(
            1, sizeof(nipc_shm_ctx_t),
            NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SHM_CTX_CALLOC_INTERNAL);
    if (!shm)
        return false;

    nipc_managed_server_t *server = prep->server;
    if (nipc_shm_server_create(server->run_dir, server->service_name,
                               prep->session_id, request_capacity,
                               response_capacity, shm) != NIPC_SHM_OK) {
        free(shm);
        return false;
    }

    prep->shm = shm;
    if (server->test_event_hook)
        server->test_event_hook(server->test_event_user,
                                NIPC_SERVER_TEST_EVENT_SHM_CREATED);
    return true;
}

/* Wait for HELLO without outliving a server stop or the handshake
 * deadline. The clock bounds the wait, so time lost to EINTR counts and the
 * last slice ends at the deadline. */
static bool server_wait_hello(nipc_managed_server_t *server, int fd,
                              uint64_t deadline_ms)
{
    while (__atomic_load_n(&server->running, __ATOMIC_RELAXED)) {
        uint64_t now = nipc_service_platform_monotonic_ms();
        if (now >= deadline_ms)
            return false;
        uint64_t left = deadline_ms - now;
        int slice = left < server->poll_ms ? (int)left : (int)server->poll_ms;
        struct pollfd pfd = { .fd = fd, .events = POLLIN };
        int ret = poll(&pfd, 1, slice);
        if (ret < 0) {
            if (errno == EINTR)
                continue;
            return false;
        }
        if (ret > 0)
            return true;
    }
    return false;
}

/* Run the handshake for an accepted connection and publish the session.
 * Takes ownership of fd. Returns false if the session must not start. */
static bool server_session_handshake(nipc_managed_server_t *server,
                                     nipc_session_ctx_t *sctx, int fd)
{
    nipc_uds_server_config_t cfg;
    if (!server_prepare_accept_config(server, &cfg)) {
        close(fd);
        return false;
    }

    if (server->test_event_hook)
        server->test_event_hook(server->test_event_user,
                                NIPC_SERVER_TEST_EVENT_HANDSHAKE_STARTED);
    uint64_t deadline_ms =
        nipc_service_platform_monotonic_ms() + server->handshake_ms;
    if (!server_wait_hello(server, fd, deadline_ms)) {
        close(fd);
        return false;
    }

    /* HELLO has started to arrive; a stream peer may still be sending the
     * rest, which the remaining handshake time bounds. */
    uint64_t now = nipc_service_platform_monotonic_ms();
    uint64_t left = now < deadline_ms ? deadline_ms - now : 1;
    server_shm_prepare_t prep = { .server = server, .session_id = sctx->id };
    nipc_uds_session_t session;
    if (nipc_uds_accept_handshake_prepared(fd, &cfg, sctx->id,
                                           left > UINT32_MAX ? UINT32_MAX
                                                             : (uint32_t)left,
                                           server_prepare_profile, &prep,
                                           &session) != NIPC_UDS_OK) {
        server_destroy_precreated_shm(&prep.shm);
        return false;
    }
    nipc_shm_ctx_t *shm = prep.shm;

    nipc_service_common_server_note_request_capacity(
        server, session.max_request_payload_bytes);
    nipc_service_common_server_note_response_capacity(
        server, session.max_response_payload_bytes);

    /* SHM profile guarantee: the prepare step created the region before the
     * ACK selected SHM. */
    if (session.selected_profile == NIPC_PROFILE_SHM_HYBRID ||
        session.selected_profile == NIPC_PROFILE_SHM_FUTEX) {
        if (!shm) {
            nipc_uds_close_session(&session);
            return false;
        }
    } else {
        server_destroy_precreated_shm(&shm);
    }

    /* Publish under the lock: shutdown paths read session.fd under it. */
    pthread_mutex_lock(&server->sessions_lock);
    sctx->session = session;
    sctx->shm = shm;
    pthread_mutex_unlock(&server->sessions_lock);
    return true;
}

/* Thread function: handles one client session from accept to disconnect. */
void *nipc_service_posix_session_handler_thread(void *arg)
{
    nipc_posix_session_start_arg_t *start = (nipc_posix_session_start_arg_t *)arg;
    pthread_mutex_lock(&start->lock);
    nipc_session_ctx_t *sctx = start->session;
    nipc_managed_server_t *server = start->server;
    int fd = start->fd;
    start->copied = true;
    pthread_cond_signal(&start->copied_cond);
    pthread_mutex_unlock(&start->lock);

    if (server_session_handshake(server, sctx, fd)) {
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
    }

    /* Cleanup SHM and session */
    if (sctx->shm) {
        nipc_shm_destroy(sctx->shm);
        free(sctx->shm);
        sctx->shm = NULL;
    }
    /* Close under the lock so a concurrent shutdown never targets a
     * reused descriptor. */
    pthread_mutex_lock(&server->sessions_lock);
    nipc_uds_close_session(&sctx->session);
    pthread_mutex_unlock(&server->sessions_lock);

    /* Mark inactive so the acceptor's reap loop (or server destroy)
     * can join this thread and free sctx.  Do NOT remove from the
     * tracking array here — the reap/destroy path owns that. */
    __atomic_store_n(&sctx->active, false, __ATOMIC_RELEASE);
    /* sctx may be freed from here on; report through the server only. */
    if (server->test_event_hook)
        server->test_event_hook(server->test_event_user,
                                NIPC_SERVER_TEST_EVENT_SESSION_EXITED);
    return NULL;
}

/* Wait up to grace_ms for session threads to finish on their own, then shut
 * down the sockets of the rest so threads blocked in send/recv return.
 * Returns true if all finished within the grace. */
bool nipc_service_posix_server_wait_sessions(nipc_managed_server_t *server,
                                             uint32_t grace_ms)
{
    struct timespec deadline;
    clock_gettime(CLOCK_MONOTONIC, &deadline);
    deadline.tv_sec += grace_ms / 1000;
    deadline.tv_nsec += (grace_ms % 1000) * 1000000L;
    if (deadline.tv_nsec >= 1000000000L) {
        deadline.tv_sec++;
        deadline.tv_nsec -= 1000000000L;
    }

    for (;;) {
        pthread_mutex_lock(&server->sessions_lock);
        int active_count = 0;
        for (int i = 0; i < server->session_count; i++) {
            if (__atomic_load_n(&server->sessions[i]->active, __ATOMIC_ACQUIRE))
                active_count++;
        }
        pthread_mutex_unlock(&server->sessions_lock);

        if (active_count == 0)
            return true;

        struct timespec now;
        clock_gettime(CLOCK_MONOTONIC, &now);
        if (now.tv_sec > deadline.tv_sec ||
            (now.tv_sec == deadline.tv_sec && now.tv_nsec >= deadline.tv_nsec))
            break;

        nipc_service_posix_sleep_us(5000); /* 5ms poll interval */
    }

    pthread_mutex_lock(&server->sessions_lock);
    for (int i = 0; i < server->session_count; i++) {
        nipc_session_ctx_t *s = server->sessions[i];
        if (__atomic_load_n(&s->active, __ATOMIC_ACQUIRE) && s->session.fd >= 0)
            shutdown(s->session.fd, SHUT_RDWR);
    }
    pthread_mutex_unlock(&server->sessions_lock);
    return false;
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
