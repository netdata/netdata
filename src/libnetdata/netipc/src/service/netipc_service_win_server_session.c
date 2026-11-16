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
                                                               &msg_len, server->poll_ms);
            if (serr == NIPC_WIN_SHM_ERR_TIMEOUT) {
                /* A crashed client never sets the close flag; its broken
                 * pipe is the only sign it left. */
                if (nipc_service_win_peer_disconnected(session))
                    break;
                continue;
            }
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
                session, server->poll_ms, &readable);
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

static void server_destroy_win_shm(nipc_win_shm_ctx_t **shm) {
    if (!shm || !*shm)
        return;
    nipc_win_shm_destroy(*shm);
    free(*shm);
    *shm = NULL;
}

static bool server_prepare_accept_config(nipc_managed_server_t *server,
                                         nipc_np_server_config_t *cfg_out) {
    *cfg_out = server->base_config;
    /* Workers grow these concurrently (InterlockedCompareExchange); read
     * them with the same interlocked access. */
    cfg_out->max_request_payload_bytes = (uint32_t)InterlockedCompareExchange(
        (volatile LONG *)&server->learned_request_payload_bytes, 0, 0);
    cfg_out->max_response_payload_bytes = (uint32_t)InterlockedCompareExchange(
        (volatile LONG *)&server->learned_response_payload_bytes, 0, 0);

    uint32_t shm_profiles =
            cfg_out->supported_profiles &
            (NIPC_WIN_SHM_PROFILE_HYBRID | NIPC_WIN_SHM_PROFILE_BUSYWAIT);
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
    nipc_win_shm_ctx_t *shm;
} server_shm_prepare_t;

/* Handshake prepare step: create the session's kernel objects for the
 * selected SHM profile only, sized for the agreed limits. */
static bool server_prepare_profile(void *user, uint32_t profile,
                                   uint32_t max_request_payload_bytes,
                                   uint32_t max_response_payload_bytes) {
    server_shm_prepare_t *prep = (server_shm_prepare_t *)user;
    if (profile != NIPC_WIN_SHM_PROFILE_HYBRID &&
        profile != NIPC_WIN_SHM_PROFILE_BUSYWAIT)
        return true;

    uint32_t request_capacity;
    uint32_t response_capacity;
    if (!nipc_service_common_header_payload_len_u32(max_request_payload_bytes,
                                                    &request_capacity) ||
        !nipc_service_common_header_payload_len_u32(max_response_payload_bytes,
                                                    &response_capacity))
        return false;

    nipc_win_shm_ctx_t *ctx = nipc_service_win_calloc(
            1, sizeof(nipc_win_shm_ctx_t),
            NIPC_WIN_SERVICE_TEST_FAULT_SERVER_SHM_CTX_CALLOC_INTERNAL);
    if (!ctx)
        return false;

    nipc_managed_server_t *server = prep->server;
    if (nipc_win_shm_server_create(server->run_dir, server->service_name,
                                   server->auth_token, prep->session_id,
                                   profile, request_capacity,
                                   response_capacity, ctx) != NIPC_WIN_SHM_OK) {
        free(ctx);
        return false;
    }

    prep->shm = ctx;
    if (server->test_event_hook)
        server->test_event_hook(server->test_event_user,
                                NIPC_SERVER_TEST_EVENT_SHM_CREATED);
    return true;
}

/* Wait for HELLO without outliving a server stop or the handshake
 * deadline. */
static bool server_wait_hello(nipc_managed_server_t *server, HANDLE pipe)
{
    ULONGLONG deadline = GetTickCount64() + server->handshake_ms;
    for (;;) {
        DWORD available = 0;
        if (!PeekNamedPipe(pipe, NULL, 0, NULL, &available, NULL))
            return false;
        if (available > 0)
            return true;
        if (!InterlockedCompareExchange(&server->running, 0, 0) ||
            GetTickCount64() >= deadline)
            return false;
        Sleep(1);
    }
}

/* Run the handshake for an accepted pipe and publish the session.
 * Takes ownership of pipe. Returns false if the session must not start. */
static bool server_session_handshake(nipc_managed_server_t *server,
                                     nipc_session_ctx_t *sctx, HANDLE pipe)
{
    nipc_np_server_config_t cfg;
    if (!server_prepare_accept_config(server, &cfg)) {
        DisconnectNamedPipe(pipe);
        CloseHandle(pipe);
        return false;
    }
    if (server->test_event_hook)
        server->test_event_hook(server->test_event_user,
                                NIPC_SERVER_TEST_EVENT_HANDSHAKE_STARTED);
    if (!server_wait_hello(server, pipe)) {
        DisconnectNamedPipe(pipe);
        CloseHandle(pipe);
        return false;
    }

    /* HELLO is readable, so this does not block on the client. */
    server_shm_prepare_t prep = { .server = server, .session_id = sctx->id };
    nipc_np_session_t session;
    if (nipc_np_accept_handshake_prepared(pipe, &cfg, sctx->id, server->poll_ms,
                                          server_prepare_profile, &prep,
                                          &session) != NIPC_NP_OK) {
        server_destroy_win_shm(&prep.shm);
        return false;
    }

    nipc_service_common_server_note_request_capacity(
        server, session.max_request_payload_bytes);
    nipc_service_common_server_note_response_capacity(
        server, session.max_response_payload_bytes);

    /* SHM profile guarantee: the prepare step created the selected
     * profile's kernel objects before the ACK selected it. */
    nipc_win_shm_ctx_t *shm = prep.shm;
    if (session.selected_profile == NIPC_WIN_SHM_PROFILE_HYBRID ||
        session.selected_profile == NIPC_WIN_SHM_PROFILE_BUSYWAIT) {
        if (!shm) {
            nipc_np_close_session(&session);
            return false;
        }
    } else {
        server_destroy_win_shm(&shm);
    }

    EnterCriticalSection(&server->sessions_lock);
    sctx->session = session;
    sctx->shm = shm;
    LeaveCriticalSection(&server->sessions_lock);
    return true;
}

unsigned __stdcall nipc_service_win_session_handler_thread(void *arg)
{
    nipc_session_ctx_t *sctx = (nipc_session_ctx_t *)arg;
    nipc_managed_server_t *server = sctx->server;
    HANDLE pipe = sctx->session.pipe;
    sctx->session.pipe = INVALID_HANDLE_VALUE;

    if (server_session_handshake(server, sctx, pipe)) {
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
    }

    /* Cleanup SHM and session */
    if (sctx->shm) {
        nipc_win_shm_destroy(sctx->shm);
        free(sctx->shm);
        sctx->shm = NULL;
    }
    nipc_np_close_session(&sctx->session);

    /* Mark inactive; the reap/destroy path owns removal from the array */
    InterlockedExchange(&sctx->active, 0);
    /* sctx may be freed from here on; report through the server only. */
    if (server->test_event_hook)
        server->test_event_hook(server->test_event_user,
                                NIPC_SERVER_TEST_EVENT_SESSION_EXITED);
    return 0;
}

/* Ask active session threads to leave synchronous ReadFile/WriteFile
 * waits. Targets thread handles, which stay valid until the
 * owner joins them, never pipe handles. Returns the number still active. */
static int server_cancel_active_session_io_locked(nipc_managed_server_t *server)
{
    int active = 0;
    for (int i = 0; i < server->session_count; i++) {
        nipc_session_ctx_t *s = server->sessions[i];
        if (!s || !InterlockedCompareExchange(&s->active, 0, 0))
            continue;
        active++;
        if (s->thread != NULL && s->thread != INVALID_HANDLE_VALUE)
            CancelSynchronousIo(s->thread);
    }
    return active;
}

/* Wait up to grace_ms for session threads to finish on their own, then
 * cancel their pipe I/O until they exit. A single cancel can land before a
 * thread enters its next blocking call, so it repeats. Returns true if all
 * finished within the grace. */
bool nipc_service_win_server_wait_sessions(nipc_managed_server_t *server,
                                           uint32_t grace_ms)
{
    ULONGLONG deadline = GetTickCount64() + grace_ms;
    for (;;) {
        EnterCriticalSection(&server->sessions_lock);
        int active_count = 0;
        for (int i = 0; i < server->session_count; i++) {
            if (InterlockedCompareExchange(&server->sessions[i]->active, 0, 0))
                active_count++;
        }
        LeaveCriticalSection(&server->sessions_lock);

        if (active_count == 0)
            return true;
        if (GetTickCount64() >= deadline)
            break;
        Sleep(5); /* 5ms poll interval */
    }

    for (;;) {
        EnterCriticalSection(&server->sessions_lock);
        int active = server_cancel_active_session_io_locked(server);
        LeaveCriticalSection(&server->sessions_lock);
        if (active == 0)
            return false;
        Sleep(10);
    }
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
        if (!InterlockedCompareExchange(&s->active, 0, 0)) {
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
