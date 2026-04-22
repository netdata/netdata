/*
 * netipc_service.c - L2 orchestration implementation.
 *
 * Pure composition of L1 (UDS/SHM) + Codec. No direct socket/mmap calls
 * for data framing. Uses poll() for timeout-based shutdown detection
 * in the managed server.
 *
 * Client context manages connection lifecycle with at-least-once retry.
 * Managed server handles accept, read, dispatch, respond.
 */

#include "netipc/netipc_service.h"
#include "netipc/netipc_protocol.h"
#include "netipc/netipc_uds.h"
#include "netipc/netipc_shm.h"

#include <errno.h>
#include <poll.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

/* Poll timeout for server loops: 100ms between shutdown checks */
#define SERVER_POLL_TIMEOUT_MS 100
#define NIPC_CLIENT_BUF_DEFAULT 65536u
#define CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS 5u
#define CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS 5000u

enum {
    NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_RESPONSE_BUF_REALLOC_INTERNAL = 1,
    NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_SEND_BUF_REALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_SHM_CTX_CALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SHM_CTX_CALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_RECV_BUF_MALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_RESP_BUF_MALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SESSIONS_CALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SESSION_CTX_CALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_THREAD_CREATE_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_BUCKETS_CALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_ITEMS_CALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_ITEM_NAME_MALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_ITEM_PATH_MALLOC_INTERNAL,
};

static uint64_t g_posix_service_test_fault_state = 0;

static uint64_t posix_service_fault_state_make(int site, uint32_t skip_matches)
{
    return ((uint64_t)skip_matches << 32) | (uint32_t)site;
}

void nipc_posix_service_test_fault_set(int site, uint32_t skip_matches)
{
    __atomic_store_n(&g_posix_service_test_fault_state,
                     posix_service_fault_state_make(site, skip_matches),
                     __ATOMIC_RELEASE);
}

void nipc_posix_service_test_fault_clear(void)
{
    __atomic_store_n(&g_posix_service_test_fault_state, 0, __ATOMIC_RELEASE);
}

static bool service_test_should_fail(int site)
{
    for (;;) {
        uint64_t state = __atomic_load_n(&g_posix_service_test_fault_state,
                                         __ATOMIC_ACQUIRE);
        uint32_t current_site = (uint32_t)state;
        uint32_t current_skip = (uint32_t)(state >> 32);
        uint64_t next_state;

        if (current_site != (uint32_t)site)
            return false;

        if (current_skip > 0)
            next_state = posix_service_fault_state_make(site, current_skip - 1u);
        else
            next_state = 0;

        if (__atomic_compare_exchange_n(&g_posix_service_test_fault_state,
                                        &state, next_state, false,
                                        __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE))
            return current_skip == 0;
    }
}

static void *service_malloc(size_t size, int fault_site)
{
    if (service_test_should_fail(fault_site))
        return NULL;
    return malloc(size);
}

static void *service_calloc(size_t count, size_t size, int fault_site)
{
    if (service_test_should_fail(fault_site))
        return NULL;
    return calloc(count, size);
}

static void *service_realloc(void *ptr, size_t size, int fault_site)
{
    if (service_test_should_fail(fault_site))
        return NULL;
    return realloc(ptr, size);
}

static int service_pthread_create(pthread_t *thread,
                                  const pthread_attr_t *attr,
                                  void *(*start_routine)(void *),
                                  void *arg)
{
    if (service_test_should_fail(NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_THREAD_CREATE_INTERNAL))
        return EAGAIN;
    return pthread_create(thread, attr, start_routine, arg);
}

static uint64_t monotonic_time_ms(void)
{
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (uint64_t)ts.tv_sec * 1000u + (uint64_t)ts.tv_nsec / 1000000u;
}

static uint32_t next_power_of_2_u32(uint32_t n)
{
    if (n < 16)
        return 16;
    n--;
    n |= n >> 1;
    n |= n >> 2;
    n |= n >> 4;
    n |= n >> 8;
    n |= n >> 16;
    return n + 1;
}

static bool ensure_buffer(uint8_t **buf, size_t *buf_size, size_t need, int fault_site)
{
    if (*buf && *buf_size >= need)
        return true;

    uint8_t *new_buf = service_realloc(*buf, need, fault_site);
    if (!new_buf)
        return false;

    *buf = new_buf;
    *buf_size = need;
    return true;
}

static void client_note_request_capacity(nipc_client_ctx_t *ctx, uint32_t payload_len)
{
    uint32_t grown = next_power_of_2_u32(payload_len);
    if (grown > NIPC_MAX_PAYLOAD_CAP)
        grown = NIPC_MAX_PAYLOAD_CAP;
    if (grown > ctx->transport_config.max_request_payload_bytes)
        ctx->transport_config.max_request_payload_bytes = grown;
}

static void client_note_response_capacity(nipc_client_ctx_t *ctx, uint32_t payload_len)
{
    uint32_t grown = next_power_of_2_u32(payload_len);
    if (grown > NIPC_MAX_PAYLOAD_CAP)
        grown = NIPC_MAX_PAYLOAD_CAP;
    if (grown > ctx->transport_config.max_response_payload_bytes)
        ctx->transport_config.max_response_payload_bytes = grown;
}

static bool client_prepare_session_buffers(nipc_client_ctx_t *ctx)
{
    size_t response_need = (size_t)ctx->session.max_response_payload_bytes + NIPC_HEADER_LEN;
    if (response_need < NIPC_HEADER_LEN + 1024u)
        response_need = NIPC_HEADER_LEN + 1024u;

    if (!ensure_buffer(&ctx->response_buf, &ctx->response_buf_size, response_need,
                       NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_RESPONSE_BUF_REALLOC_INTERNAL))
        return false;

    if (ctx->session.selected_profile == NIPC_PROFILE_SHM_HYBRID ||
        ctx->session.selected_profile == NIPC_PROFILE_SHM_FUTEX) {
        size_t send_need = (size_t)ctx->session.max_request_payload_bytes + NIPC_HEADER_LEN;
        if (!ensure_buffer(&ctx->send_buf, &ctx->send_buf_size, send_need,
                           NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_SEND_BUF_REALLOC_INTERNAL))
            return false;
    }

    return true;
}

static uint32_t cgroups_request_payload_default(void)
{
    return 16u;
}

static uint32_t cgroups_response_payload_default(void)
{
    return NIPC_CLIENT_BUF_DEFAULT;
}

static nipc_uds_client_config_t service_client_config_to_transport(
    const nipc_client_config_t *config)
{
    nipc_uds_client_config_t transport = {0};

    if (!config)
        return transport;

    transport.supported_profiles = config->supported_profiles;
    transport.preferred_profiles = config->preferred_profiles;
    transport.max_request_batch_items = config->max_request_batch_items;
    transport.max_response_payload_bytes = config->max_response_payload_bytes;
    transport.max_response_batch_items = config->max_request_batch_items;
    transport.auth_token = config->auth_token;

    return transport;
}

static nipc_uds_server_config_t service_server_config_to_transport(
    const nipc_server_config_t *config)
{
    nipc_uds_server_config_t transport = {0};

    if (!config)
        return transport;

    transport.supported_profiles = config->supported_profiles;
    transport.preferred_profiles = config->preferred_profiles;
    transport.max_request_batch_items = config->max_request_batch_items;
    transport.max_response_payload_bytes = config->max_response_payload_bytes;
    transport.max_response_batch_items = config->max_request_batch_items;
    transport.auth_token = config->auth_token;

    return transport;
}

/* ------------------------------------------------------------------ */
/*  Internal: client connection helpers                                */
/* ------------------------------------------------------------------ */

/* Tear down the current connection (UDS session + SHM if any). */
static void client_disconnect(nipc_client_ctx_t *ctx)
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
static nipc_client_state_t client_try_connect(nipc_client_ctx_t *ctx)
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

        nipc_shm_ctx_t *shm = service_calloc(
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
            uint64_t deadline_ms = monotonic_time_ms() + CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS;
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
                if (monotonic_time_ms() >= deadline_ms)
                    break;
                usleep(CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS * 1000u);
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
                return client_try_connect(ctx);
            }
        }
    }

    return NIPC_CLIENT_READY;
}

/* ------------------------------------------------------------------ */
/*  Internal: send/receive via the active transport                    */
/* ------------------------------------------------------------------ */

/*
 * Send a complete message (header + payload) using whichever transport
 * is active: SHM if negotiated, UDS otherwise.
 */
static nipc_error_t transport_send(nipc_client_ctx_t *ctx,
                                    nipc_header_t *hdr,
                                    const void *payload,
                                    size_t payload_len)
{
    if (payload_len > UINT32_MAX)
        return NIPC_ERR_OVERFLOW;

    if (ctx->shm) {
        if (payload_len > ctx->session.max_request_payload_bytes) {
            client_note_request_capacity(ctx, (uint32_t)payload_len);
            return NIPC_ERR_OVERFLOW;
        }

        size_t msg_len = NIPC_HEADER_LEN + payload_len;
        uint8_t *msg = ctx->send_buf;
        if (!msg || msg_len > ctx->send_buf_size)
            return NIPC_ERR_OVERFLOW;

        hdr->magic      = NIPC_MAGIC_MSG;
        hdr->version    = NIPC_VERSION;
        hdr->header_len = NIPC_HEADER_LEN;
        hdr->payload_len = (uint32_t)payload_len;

        nipc_header_encode(hdr, msg, NIPC_HEADER_LEN);
        if (payload_len > 0)
            memcpy(msg + NIPC_HEADER_LEN, payload, payload_len);

        nipc_shm_error_t serr = nipc_shm_send(ctx->shm, msg, msg_len);
        if (serr == NIPC_SHM_ERR_MSG_TOO_LARGE) {
            client_note_request_capacity(ctx, (uint32_t)payload_len);
            return NIPC_ERR_OVERFLOW;
        }
        return (serr == NIPC_SHM_OK) ? NIPC_OK : NIPC_ERR_NOT_READY;
    }

    /* UDS path */
    nipc_uds_error_t uerr = nipc_uds_send(&ctx->session, hdr,
                                            payload, payload_len);
    if (uerr == NIPC_UDS_ERR_LIMIT_EXCEEDED) {
        client_note_request_capacity(ctx, (uint32_t)payload_len);
        return NIPC_ERR_OVERFLOW;
    }
    return (uerr == NIPC_UDS_OK) ? NIPC_OK : NIPC_ERR_NOT_READY;
}

/*
 * Receive a complete message. For SHM, reads from the SHM region.
 * For UDS, reads from the socket into the caller's buffer.
 *
 * On success, hdr_out is filled, and payload_out + payload_len_out
 * point to the payload bytes (valid until next receive).
 */
static nipc_error_t transport_receive(nipc_client_ctx_t *ctx,
                                       void *buf, size_t buf_size,
                                       nipc_header_t *hdr_out,
                                       const void **payload_out,
                                       size_t *payload_len_out)
{
    if (ctx->shm) {
        size_t msg_len;
        nipc_shm_error_t serr = nipc_shm_receive(ctx->shm, buf, buf_size,
                                                    &msg_len, 30000);
        if (serr != NIPC_SHM_OK)
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

    /* UDS path */
    nipc_uds_error_t uerr = nipc_uds_receive(&ctx->session, buf, buf_size,
                                               hdr_out, payload_out,
                                               payload_len_out);
    return (uerr == NIPC_UDS_OK) ? NIPC_OK : NIPC_ERR_TRUNCATED;
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
static nipc_error_t do_raw_call(nipc_client_ctx_t *ctx,
                                 uint16_t method_code,
                                 const void *request_payload,
                                 size_t request_len,
                                 const void **response_payload_out,
                                 size_t *response_len_out)
{
    nipc_header_t hdr = {0};
    hdr.kind             = NIPC_KIND_REQUEST;
    hdr.code             = method_code;
    hdr.flags            = 0;
    hdr.item_count       = 1;
    hdr.message_id       = (uint64_t)(ctx->call_count + 1);
    hdr.transport_status = NIPC_STATUS_OK;

    nipc_error_t err = transport_send(ctx, &hdr, request_payload, request_len);
    if (err != NIPC_OK) {
        return err;
    }

    nipc_header_t resp_hdr;
    err = transport_receive(ctx, ctx->response_buf, ctx->response_buf_size,
                            &resp_hdr, response_payload_out, response_len_out);
    if (err != NIPC_OK) {
        return err;
    }

    if (resp_hdr.kind != NIPC_KIND_RESPONSE)
        return NIPC_ERR_BAD_KIND;
    if (resp_hdr.code != method_code)
        return NIPC_ERR_BAD_LAYOUT;
    if (resp_hdr.message_id != hdr.message_id)
        return NIPC_ERR_BAD_LAYOUT;

    switch (resp_hdr.transport_status) {
    case NIPC_STATUS_OK:
        break;
    case NIPC_STATUS_LIMIT_EXCEEDED:
        if (ctx->session.max_response_payload_bytes > 0) {
            uint32_t current = ctx->session.max_response_payload_bytes;
            client_note_response_capacity(
                ctx, current >= UINT32_MAX / 2u ? UINT32_MAX : current * 2u);
        }
        return NIPC_ERR_OVERFLOW;
    case NIPC_STATUS_UNSUPPORTED:
        return NIPC_ERR_BAD_LAYOUT;
    case NIPC_STATUS_BAD_ENVELOPE:
    case NIPC_STATUS_INTERNAL_ERROR:
    default:
        return NIPC_ERR_BAD_LAYOUT;
    }

    return NIPC_OK;
}

/*
 * Generic call-with-retry:
 *   - ordinary failures reconnect and retry once
 *   - overflow-driven resize recovery may reconnect repeatedly until
 *     negotiated capacities grow or recovery fails
 * The caller provides a function pointer for the single-attempt logic.
 */
typedef nipc_error_t (*nipc_attempt_fn)(nipc_client_ctx_t *ctx, void *state);

static nipc_error_t call_with_retry(nipc_client_ctx_t *ctx,
                                     nipc_attempt_fn attempt,
                                     void *state)
{
    if (ctx->state != NIPC_CLIENT_READY) {
        ctx->error_count++;
        return NIPC_ERR_NOT_READY;
    }

    /* Cap overflow-driven retries: payloads grow by powers of 2, so 8
     * retries allows ~256x growth from the initial negotiated size. */
    int overflow_retries = 0;
    for (;;) {
        uint32_t prev_req = ctx->session.max_request_payload_bytes;
        uint32_t prev_resp = ctx->session.max_response_payload_bytes;
        uint32_t prev_cfg_req = ctx->transport_config.max_request_payload_bytes;
        uint32_t prev_cfg_resp = ctx->transport_config.max_response_payload_bytes;

        nipc_error_t err = attempt(ctx, state);
        if (err == NIPC_OK) {
            ctx->call_count++;
            return NIPC_OK;
        }

        if (err != NIPC_ERR_OVERFLOW) {
            client_disconnect(ctx);
            ctx->state = NIPC_CLIENT_BROKEN;
            ctx->state = client_try_connect(ctx);
            if (ctx->state != NIPC_CLIENT_READY) {
                ctx->error_count++;
                return err;
            }

            ctx->reconnect_count++;
            err = attempt(ctx, state);
            if (err == NIPC_OK) {
                ctx->call_count++;
                return NIPC_OK;
            }

            client_disconnect(ctx);
            ctx->state = NIPC_CLIENT_BROKEN;
            ctx->error_count++;
            return err;
        }

        client_disconnect(ctx);
        ctx->state = NIPC_CLIENT_BROKEN;
        ctx->state = client_try_connect(ctx);
        if (ctx->state != NIPC_CLIENT_READY) {
            ctx->error_count++;
            return err;
        }
        ctx->reconnect_count++;

        if (ctx->session.max_request_payload_bytes <= prev_req &&
            ctx->session.max_response_payload_bytes <= prev_resp &&
            ctx->transport_config.max_request_payload_bytes <= prev_cfg_req &&
            ctx->transport_config.max_response_payload_bytes <= prev_cfg_resp) {
            client_disconnect(ctx);
            ctx->state = NIPC_CLIENT_BROKEN;
            ctx->error_count++;
            return err;
        }

        if (++overflow_retries >= 8) {
            client_disconnect(ctx);
            ctx->state = NIPC_CLIENT_BROKEN;
            ctx->error_count++;
            return err;
        }
    }
}

/* ------------------------------------------------------------------ */
/*  Internal: single attempt at a cgroups snapshot call                */
/* ------------------------------------------------------------------ */

typedef struct {
    nipc_cgroups_resp_view_t *view_out;
} cgroups_call_state_t;

static nipc_error_t do_cgroups_attempt(nipc_client_ctx_t *ctx, void *state)
{
    cgroups_call_state_t *s = (cgroups_call_state_t *)state;

    nipc_cgroups_req_t req = { .layout_version = 1, .flags = 0 };
    uint8_t req_buf[4];
    size_t req_len = nipc_cgroups_req_encode(&req, req_buf, sizeof(req_buf));
    if (req_len == 0)
        return NIPC_ERR_TRUNCATED;

    const void *payload;
    size_t payload_len;
    nipc_error_t err = do_raw_call(ctx, NIPC_METHOD_CGROUPS_SNAPSHOT,
                                     req_buf, req_len,
                                     &payload, &payload_len);
    if (err != NIPC_OK)
        return err;

    return nipc_cgroups_resp_decode(payload, payload_len, s->view_out);
}

/* ------------------------------------------------------------------ */
/*  Public API: client lifecycle                                       */
/* ------------------------------------------------------------------ */

void nipc_client_init(nipc_client_ctx_t *ctx,
                      const char *run_dir,
                      const char *service_name,
                      const nipc_client_config_t *config)
{
    memset(ctx, 0, sizeof(*ctx));
    ctx->state = NIPC_CLIENT_DISCONNECTED;
    ctx->session.fd = -1;
    ctx->session_valid = false;
    ctx->shm = NULL;

    if (run_dir) {
        size_t len = strlen(run_dir);
        if (len >= sizeof(ctx->run_dir))
            len = sizeof(ctx->run_dir) - 1;
        memcpy(ctx->run_dir, run_dir, len);
        ctx->run_dir[len] = '\0';
    }

    if (service_name) {
        size_t len = strlen(service_name);
        if (len >= sizeof(ctx->service_name))
            len = sizeof(ctx->service_name) - 1;
        memcpy(ctx->service_name, service_name, len);
        ctx->service_name[len] = '\0';
    }

    ctx->transport_config = service_client_config_to_transport(config);
    if (ctx->transport_config.max_request_payload_bytes == 0)
        ctx->transport_config.max_request_payload_bytes = cgroups_request_payload_default();
    if (ctx->transport_config.max_response_payload_bytes == 0)
        ctx->transport_config.max_response_payload_bytes = cgroups_response_payload_default();
}

bool nipc_client_refresh(nipc_client_ctx_t *ctx)
{
    nipc_client_state_t old_state = ctx->state;

    switch (ctx->state) {
    case NIPC_CLIENT_DISCONNECTED:
    case NIPC_CLIENT_NOT_FOUND:
        /* Attempt to connect */
        ctx->state = NIPC_CLIENT_CONNECTING;
        ctx->state = client_try_connect(ctx);
        if (ctx->state == NIPC_CLIENT_READY)
            ctx->connect_count++;
        break;

    case NIPC_CLIENT_BROKEN:
        /* Reconnect: tear down old connection first */
        client_disconnect(ctx);
        ctx->state = NIPC_CLIENT_CONNECTING;
        ctx->state = client_try_connect(ctx);
        if (ctx->state == NIPC_CLIENT_READY)
            ctx->reconnect_count++;
        break;

    case NIPC_CLIENT_READY:
    case NIPC_CLIENT_CONNECTING:
    case NIPC_CLIENT_AUTH_FAILED:
    case NIPC_CLIENT_INCOMPATIBLE:
        /* No action needed */
        break;
    }

    return ctx->state != old_state;
}

void nipc_client_status(const nipc_client_ctx_t *ctx,
                        nipc_client_status_t *out)
{
    out->state           = ctx->state;
    out->connect_count   = ctx->connect_count;
    out->reconnect_count = ctx->reconnect_count;
    out->call_count      = ctx->call_count;
    out->error_count     = ctx->error_count;
}

void nipc_client_close(nipc_client_ctx_t *ctx)
{
    client_disconnect(ctx);
    free(ctx->response_buf);
    free(ctx->send_buf);
    ctx->response_buf = NULL;
    ctx->send_buf = NULL;
    ctx->response_buf_size = 0;
    ctx->send_buf_size = 0;
    ctx->state = NIPC_CLIENT_DISCONNECTED;
}

/* ------------------------------------------------------------------ */
/*  Public API: typed cgroups snapshot call                            */
/* ------------------------------------------------------------------ */

nipc_error_t nipc_client_call_cgroups_snapshot(
    nipc_client_ctx_t *ctx,
    nipc_cgroups_resp_view_t *view_out)
{
    cgroups_call_state_t state = {
        .view_out          = view_out,
    };
    return call_with_retry(ctx, do_cgroups_attempt, &state);
}

/* ------------------------------------------------------------------ */
/*  Internal: managed server session handler                           */
/* ------------------------------------------------------------------ */

/*
 * Wait for data on a file descriptor with periodic shutdown checks.
 * Returns: 1 = data ready, 0 = server stopping, -1 = error/hangup.
 */
static int poll_with_shutdown(int fd, bool *running)
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

static uint32_t server_snapshot_max_items(size_t response_buf_size,
                                          const nipc_cgroups_service_handler_t *service_handler)
{
    if (service_handler->snapshot_max_items != 0)
        return service_handler->snapshot_max_items;
    return nipc_cgroups_builder_estimate_max_items(response_buf_size);
}

static void server_note_request_capacity(nipc_managed_server_t *server,
                                         uint32_t payload_len)
{
    uint32_t grown = next_power_of_2_u32(payload_len);
    uint32_t current = __atomic_load_n(&server->learned_request_payload_bytes,
                                       __ATOMIC_RELAXED);
    while (grown > current &&
           !__atomic_compare_exchange_n(&server->learned_request_payload_bytes,
                                        &current, grown, false,
                                        __ATOMIC_RELEASE, __ATOMIC_RELAXED)) {
    }
}

static void server_note_response_capacity(nipc_managed_server_t *server,
                                          uint32_t payload_len)
{
    uint32_t grown = next_power_of_2_u32(payload_len);
    uint32_t current = __atomic_load_n(&server->learned_response_payload_bytes,
                                       __ATOMIC_RELAXED);
    while (grown > current &&
           !__atomic_compare_exchange_n(&server->learned_response_payload_bytes,
                                        &current, grown, false,
                                        __ATOMIC_RELEASE, __ATOMIC_RELAXED)) {
    }
}

static nipc_error_t server_typed_dispatch(void *user,
                                          const nipc_header_t *request_hdr,
                                          const uint8_t *request_payload,
                                          size_t request_len,
                                          uint8_t *response_buf,
                                          size_t response_buf_size,
                                          size_t *response_len_out)
{
    nipc_managed_server_t *server = (nipc_managed_server_t *)user;
    nipc_cgroups_service_handler_t *service_handler = &server->service_handler;
    (void)request_hdr;

    if (!service_handler->handle)
        return NIPC_ERR_HANDLER_FAILED;

    return nipc_dispatch_cgroups_snapshot(
        request_payload, request_len,
        response_buf, response_buf_size, response_len_out,
        server_snapshot_max_items(response_buf_size, service_handler),
        service_handler->handle, service_handler->user);
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
    size_t recv_size = NIPC_HEADER_LEN + session->max_request_payload_bytes;
    if (recv_size < NIPC_HEADER_LEN + 1024u)
        recv_size = NIPC_HEADER_LEN + 1024u;
    uint8_t *recv_buf = service_malloc(
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
            if (serr == NIPC_SHM_ERR_TIMEOUT)
                continue; /* check running flag */
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
            int pr = poll_with_shutdown(session->fd, &server->running);
            if (pr <= 0)
                break; /* shutdown or error */

            nipc_uds_error_t uerr = nipc_uds_receive(
                session, recv_buf, recv_size,
                &hdr, &payload, &payload_len);
            if (uerr == NIPC_UDS_ERR_LIMIT_EXCEEDED) {
                if (hdr.kind == NIPC_KIND_REQUEST) {
                    if (hdr.payload_len > 0)
                        server_note_request_capacity(server, hdr.payload_len);

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
            server_note_request_capacity(server, (uint32_t)payload_len);

        /* Dispatch: one request kind per service endpoint. */
        size_t response_len = 0;
        nipc_error_t dispatch_err = server->handler(
            server->handler_user,
            &hdr,
            (const uint8_t *)payload, payload_len,
            resp_buf, resp_buf_size,
            &response_len);

        /* Build response header */
        nipc_header_t resp_hdr = {0};
        resp_hdr.kind       = NIPC_KIND_RESPONSE;
        resp_hdr.code       = hdr.code;
        resp_hdr.message_id = hdr.message_id;
        if ((hdr.flags & NIPC_FLAG_BATCH) && hdr.item_count >= 1) {
            resp_hdr.item_count = hdr.item_count;
            resp_hdr.flags = NIPC_FLAG_BATCH;
        } else {
            resp_hdr.item_count = 1;
            resp_hdr.flags = 0;
        }

        switch (dispatch_err) {
        case NIPC_OK:
            if (response_len > session->max_response_payload_bytes) {
                server_note_response_capacity(
                    server,
                    response_len >= UINT32_MAX ? UINT32_MAX : (uint32_t)response_len);
                resp_hdr.transport_status = NIPC_STATUS_LIMIT_EXCEEDED;
                response_len = 0;
            } else {
                if (response_len <= UINT32_MAX)
                    server_note_response_capacity(server, (uint32_t)response_len);
                resp_hdr.transport_status = NIPC_STATUS_OK;
            }
            break;
        case NIPC_ERR_OVERFLOW:
            if (session->max_response_payload_bytes >= UINT32_MAX / 2u)
                server_note_response_capacity(server, UINT32_MAX);
            else
                server_note_response_capacity(server,
                                             session->max_response_payload_bytes * 2u);
            resp_hdr.transport_status = NIPC_STATUS_LIMIT_EXCEEDED;
            response_len = 0;
            break;
        case NIPC_ERR_TRUNCATED:
        case NIPC_ERR_BAD_LAYOUT:
        case NIPC_ERR_OUT_OF_BOUNDS:
        case NIPC_ERR_MISSING_NUL:
        case NIPC_ERR_BAD_ALIGNMENT:
        case NIPC_ERR_BAD_ITEM_COUNT:
            resp_hdr.transport_status = NIPC_STATUS_BAD_ENVELOPE;
            response_len = 0;
            break;
        case NIPC_ERR_HANDLER_FAILED:
        default:
            resp_hdr.transport_status = NIPC_STATUS_INTERNAL_ERROR;
            response_len = 0;
            break;
        }

        /* Send response via the active transport */
        if (shm) {
            size_t msg_len = NIPC_HEADER_LEN + response_len;

            resp_hdr.magic      = NIPC_MAGIC_MSG;
            resp_hdr.version    = NIPC_VERSION;
            resp_hdr.header_len = NIPC_HEADER_LEN;
            resp_hdr.payload_len = (uint32_t)response_len;

            /* Use a stack buffer for small responses, heap for large ones */
            uint8_t stack_msg[4096];
            uint8_t *msg = (msg_len <= sizeof(stack_msg)) ? stack_msg :
                service_malloc(msg_len, NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_RESP_BUF_MALLOC_INTERNAL);
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

        if (dispatch_err == NIPC_ERR_OVERFLOW)
            break;
    }

    free(recv_buf);
}

/* ------------------------------------------------------------------ */
/*  Internal: per-session handler thread                                */
/* ------------------------------------------------------------------ */

/* Thread function: handles one client session from accept to disconnect. */
static void *session_handler_thread(void *arg)
{
    nipc_session_ctx_t *sctx = (nipc_session_ctx_t *)arg;
    nipc_managed_server_t *server = sctx->server;

    /* Allocate a per-session response buffer */
    size_t resp_size = (size_t)sctx->session.max_response_payload_bytes;
    if (resp_size < 1024u)
        resp_size = 1024u;
    uint8_t *resp_buf = service_malloc(
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
static void server_reap_sessions_locked(nipc_managed_server_t *server)
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

static void server_destroy_precreated_shm(nipc_shm_ctx_t **shm)
{
    if (!shm || !*shm)
        return;
    nipc_shm_destroy(*shm);
    free(*shm);
    *shm = NULL;
}

static bool server_prepare_accept_config(nipc_managed_server_t *server,
                                         uint64_t sid,
                                         nipc_uds_server_config_t *cfg_out,
                                         nipc_shm_ctx_t **shm_out)
{
    *cfg_out = server->base_config;
    cfg_out->max_request_payload_bytes =
        __atomic_load_n(&server->learned_request_payload_bytes, __ATOMIC_ACQUIRE);
    cfg_out->max_response_payload_bytes =
        __atomic_load_n(&server->learned_response_payload_bytes, __ATOMIC_ACQUIRE);
    *shm_out = NULL;

    uint32_t shm_profiles = cfg_out->supported_profiles &
                            (NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX);
    if (shm_profiles == 0)
        return true;

    nipc_shm_ctx_t *shm = service_calloc(
        1, sizeof(nipc_shm_ctx_t),
        NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SHM_CTX_CALLOC_INTERNAL);
    if (!shm)
        return false;

    /* HELLO has not been read yet, so the request segment must cover any
     * client proposal the handshake may legally echo back. */
    nipc_shm_error_t serr = nipc_shm_server_create(
        server->run_dir, server->service_name,
        sid,
        NIPC_MAX_PAYLOAD_CAP + NIPC_HEADER_LEN,
        cfg_out->max_response_payload_bytes + NIPC_HEADER_LEN,
        shm);
    if (serr == NIPC_SHM_OK) {
        *shm_out = shm;
        return true;
    }

    free(shm);
    cfg_out->supported_profiles &= ~(NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX);
    cfg_out->preferred_profiles &= ~(NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX);
    return cfg_out->supported_profiles != 0;
}

/* ------------------------------------------------------------------ */
/*  Public API: managed server                                         */
/* ------------------------------------------------------------------ */

static nipc_error_t server_init_raw(nipc_managed_server_t *server,
                                    const char *run_dir,
                                    const char *service_name,
                                    const nipc_uds_server_config_t *config,
                                    int worker_count,
                                    uint16_t expected_method_code,
                                    nipc_server_handler_fn handler,
                                    void *user)
{
    memset(server, 0, sizeof(*server));
    server->listener.fd = -1;
    __atomic_store_n(&server->running, false, __ATOMIC_RELAXED);
    server->acceptor_started = false;

    if (!run_dir || !service_name || !handler)
        return NIPC_ERR_BAD_LAYOUT;

    if (worker_count < 1)
        worker_count = 1;

    /* Store config */
    {
        size_t len = strlen(run_dir);
        if (len >= sizeof(server->run_dir))
            len = sizeof(server->run_dir) - 1;
        memcpy(server->run_dir, run_dir, len);
        server->run_dir[len] = '\0';
    }
    {
        size_t len = strlen(service_name);
        if (len >= sizeof(server->service_name))
            len = sizeof(server->service_name) - 1;
        memcpy(server->service_name, service_name, len);
        server->service_name[len] = '\0';
    }

    server->handler = handler;
    server->handler_user = user;
    server->worker_count = worker_count;
    server->expected_method_code = expected_method_code;
    server->base_config = *config;
    server->learned_request_payload_bytes =
        (config && config->max_request_payload_bytes > 0)
            ? config->max_request_payload_bytes
            : NIPC_MAX_PAYLOAD_DEFAULT;
    server->learned_response_payload_bytes =
        (config && config->max_response_payload_bytes > 0)
            ? config->max_response_payload_bytes
            : NIPC_MAX_PAYLOAD_DEFAULT;

    /* Initialize session tracking */
    server->session_capacity = worker_count * 2; /* room for slots being reaped */
    if (server->session_capacity < 16)
        server->session_capacity = 16;
    server->sessions = service_calloc((size_t)server->session_capacity,
                              sizeof(nipc_session_ctx_t *),
                              NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SESSIONS_CALLOC_INTERNAL);
    if (!server->sessions)
        return NIPC_ERR_OVERFLOW;
    server->session_count = 0;
    server->next_session_id = 1; /* spec: monotonic counter starting at 1 */
    pthread_mutex_init(&server->sessions_lock, NULL);

    /* Clean up stale SHM regions from previous crashes (spec requirement:
     * runs once at server startup, before the listener begins accepting). */
    nipc_shm_cleanup_stale(run_dir, service_name);

    /* Start listening via L1 */
    nipc_uds_error_t uerr = nipc_uds_listen(
        run_dir, service_name, config, &server->listener);
    if (uerr != NIPC_UDS_OK) {
        free(server->sessions);
        server->sessions = NULL;
        pthread_mutex_destroy(&server->sessions_lock);
        return NIPC_ERR_BAD_LAYOUT;
    }

    return NIPC_OK;
}

nipc_error_t nipc_server_init_typed(nipc_managed_server_t *server,
                                    const char *run_dir,
                                    const char *service_name,
                                    const nipc_server_config_t *config,
                                    int worker_count,
                                    const nipc_cgroups_service_handler_t *service_handler)
{
    if (!service_handler)
        return NIPC_ERR_BAD_LAYOUT;

    nipc_uds_server_config_t typed_cfg = service_server_config_to_transport(config);
    if (typed_cfg.max_request_payload_bytes == 0)
        typed_cfg.max_request_payload_bytes = cgroups_request_payload_default();
    if (typed_cfg.max_response_payload_bytes == 0)
        typed_cfg.max_response_payload_bytes = cgroups_response_payload_default();

    nipc_error_t err = server_init_raw(server, run_dir, service_name,
                                       &typed_cfg, worker_count,
                                       NIPC_METHOD_CGROUPS_SNAPSHOT,
                                       server_typed_dispatch, server);
    if (err != NIPC_OK)
        return err;

    server->service_handler = *service_handler;
    return NIPC_OK;
}

nipc_error_t nipc_server_init_raw_for_tests(nipc_managed_server_t *server,
                                            const char *run_dir,
                                            const char *service_name,
                                            const nipc_uds_server_config_t *config,
                                            int worker_count,
                                            uint16_t expected_method_code,
                                            nipc_server_handler_fn handler,
                                            void *user)
{
    return server_init_raw(server, run_dir, service_name, config,
                           worker_count, expected_method_code, handler, user);
}

void nipc_server_run(nipc_managed_server_t *server)
{
    __atomic_store_n(&server->running, true, __ATOMIC_RELEASE);

    while (__atomic_load_n(&server->running, __ATOMIC_RELAXED)) {
        /* Poll the listener fd before blocking on accept */
        int pr = poll_with_shutdown(server->listener.fd, &server->running);
        if (pr <= 0)
            break; /* shutdown or error */

        /* Accept one client via L1 */
        nipc_uds_session_t session;
        memset(&session, 0, sizeof(session));
        session.fd = -1;

        uint64_t sid = server->next_session_id++;
        nipc_uds_server_config_t accept_cfg;
        nipc_shm_ctx_t *prepared_shm = NULL;
        if (!server_prepare_accept_config(server, sid, &accept_cfg, &prepared_shm)) {
            usleep(10000);
            continue;
        }

        server->listener.config = accept_cfg;
        nipc_uds_error_t uerr = nipc_uds_accept(
            &server->listener, sid, &session);
        if (uerr != NIPC_UDS_OK) {
            server_destroy_precreated_shm(&prepared_shm);
            if (!__atomic_load_n(&server->running, __ATOMIC_RELAXED))
                break;
            usleep(10000);
            continue;
        }

        server_note_request_capacity(server, session.max_request_payload_bytes);
        server_note_response_capacity(server, session.max_response_payload_bytes);

        /* Enforce worker_count limit: reap finished sessions, check count */
        pthread_mutex_lock(&server->sessions_lock);
        server_reap_sessions_locked(server);

        if (server->session_count >= server->worker_count) {
            /* At capacity: reject this client by closing the session */
            pthread_mutex_unlock(&server->sessions_lock);
            server_destroy_precreated_shm(&prepared_shm);
            nipc_uds_close_session(&session);
            continue;
        }

        /* SHM profile guarantee: only negotiate SHM for sessions that already
         * have a prepared per-session SHM region. */
        nipc_shm_ctx_t *shm = prepared_shm;
        if (session.selected_profile == NIPC_PROFILE_SHM_HYBRID ||
            session.selected_profile == NIPC_PROFILE_SHM_FUTEX) {
            if (!shm) {
                pthread_mutex_unlock(&server->sessions_lock);
                nipc_uds_close_session(&session);
                continue;
            }
        } else {
            server_destroy_precreated_shm(&prepared_shm);
            shm = NULL;
        }

        /* Create session context */
        nipc_session_ctx_t *sctx = service_calloc(
            1, sizeof(nipc_session_ctx_t),
            NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SESSION_CTX_CALLOC_INTERNAL);
        if (!sctx) {
            if (shm) { nipc_shm_destroy(shm); free(shm); }
            pthread_mutex_unlock(&server->sessions_lock);
            nipc_uds_close_session(&session);
            continue;
        }

        sctx->server = server;
        sctx->session = session;
        sctx->shm = shm;
        sctx->id = sid;
        __atomic_store_n(&sctx->active, true, __ATOMIC_RELAXED);

        server->sessions[server->session_count++] = sctx;
        pthread_mutex_unlock(&server->sessions_lock);

        /* Spawn handler thread for this session */
        int rc = service_pthread_create(&sctx->thread, NULL,
                                        session_handler_thread, sctx);
        if (rc != 0) {
            /* Thread creation failed: clean up */
            pthread_mutex_lock(&server->sessions_lock);
            /* Remove the sctx we just added */
            for (int i = 0; i < server->session_count; i++) {
                if (server->sessions[i] == sctx) {
                    server->sessions[i] = server->sessions[server->session_count - 1];
                    server->session_count--;
                    break;
                }
            }
            pthread_mutex_unlock(&server->sessions_lock);

            if (shm) { nipc_shm_destroy(shm); free(shm); }
            nipc_uds_close_session(&session);
            free(sctx);
        }
    }
}

void nipc_server_stop(nipc_managed_server_t *server)
{
    __atomic_store_n(&server->running, false, __ATOMIC_RELEASE);
}

bool nipc_server_drain(nipc_managed_server_t *server, uint32_t timeout_ms)
{
    /* 1. Stop accepting new clients.
     * Do NOT close the listener here — the run loop may still be
     * polling on listener.fd. Setting the flag is enough; the run
     * loop will exit on its next poll timeout (100ms). The listener
     * is closed later by nipc_server_destroy(). */
    __atomic_store_n(&server->running, false, __ATOMIC_RELEASE);

    /* 2. Wait for in-flight sessions to complete */
    bool all_drained = true;
    if (server->sessions) {
        struct timespec deadline;
        clock_gettime(CLOCK_MONOTONIC, &deadline);
        deadline.tv_sec  += timeout_ms / 1000;
        deadline.tv_nsec += (timeout_ms % 1000) * 1000000L;
        if (deadline.tv_nsec >= 1000000000L) {
            deadline.tv_sec++;
            deadline.tv_nsec -= 1000000000L;
        }

        /* Poll until all sessions are inactive or timeout */
        while (1) {
            pthread_mutex_lock(&server->sessions_lock);
            int active_count = 0;
            for (int i = 0; i < server->session_count; i++) {
                if (__atomic_load_n(&server->sessions[i]->active,
                                    __ATOMIC_ACQUIRE))
                    active_count++;
            }
            pthread_mutex_unlock(&server->sessions_lock);

            if (active_count == 0)
                break;

            struct timespec now;
            clock_gettime(CLOCK_MONOTONIC, &now);
            if (now.tv_sec > deadline.tv_sec ||
                (now.tv_sec == deadline.tv_sec &&
                 now.tv_nsec >= deadline.tv_nsec)) {
                /* Timeout: force-close session fds to unblock recv.
                 * Closing the fd causes poll/recv to return error,
                 * which terminates the session handler loop. */
                pthread_mutex_lock(&server->sessions_lock);
                for (int i = 0; i < server->session_count; i++) {
                    nipc_session_ctx_t *s = server->sessions[i];
                    if (__atomic_load_n(&s->active, __ATOMIC_ACQUIRE)) {
                        if (s->session.fd >= 0) {
                            shutdown(s->session.fd, SHUT_RDWR);
                        }
                    }
                }
                pthread_mutex_unlock(&server->sessions_lock);
                all_drained = false;
                break;
            }

            usleep(5000); /* 5ms poll interval */
        }

        /* 3. Join all session threads (finished or not) */
        pthread_mutex_lock(&server->sessions_lock);
        for (int i = 0; i < server->session_count; i++) {
            nipc_session_ctx_t *s = server->sessions[i];
            pthread_mutex_unlock(&server->sessions_lock);
            pthread_join(s->thread, NULL);
            free(s);
            pthread_mutex_lock(&server->sessions_lock);
        }
        server->session_count = 0;
        pthread_mutex_unlock(&server->sessions_lock);

        free(server->sessions);
        server->sessions = NULL;
        server->session_capacity = 0;
        pthread_mutex_destroy(&server->sessions_lock);
    }

    server->worker_count = 0;
    return all_drained;
}

void nipc_server_destroy(nipc_managed_server_t *server)
{
    __atomic_store_n(&server->running, false, __ATOMIC_RELEASE);
    nipc_uds_close_listener(&server->listener);

    /* Join all active session threads */
    if (server->sessions) {
        pthread_mutex_lock(&server->sessions_lock);
        for (int i = 0; i < server->session_count; i++) {
            nipc_session_ctx_t *s = server->sessions[i];
            pthread_mutex_unlock(&server->sessions_lock);
            pthread_join(s->thread, NULL);
            free(s);
            pthread_mutex_lock(&server->sessions_lock);
        }
        server->session_count = 0;
        pthread_mutex_unlock(&server->sessions_lock);

        free(server->sessions);
        server->sessions = NULL;
        server->session_capacity = 0;
        pthread_mutex_destroy(&server->sessions_lock);
    }

    server->worker_count = 0;
}

/* ------------------------------------------------------------------ */
/*  L3: Client-side cgroups snapshot cache                             */
/* ------------------------------------------------------------------ */

/* Free all owned strings in cache items and the items array itself. */
static void cache_free_items(nipc_cgroups_cache_item_t *items, uint32_t count)
{
    if (!items)
        return;

    for (uint32_t i = 0; i < count; i++) {
        free(items[i].name);
        free(items[i].path);
    }
    free(items);
}

/* Hash a name string (djb2). Combined with item hash for bucket index. */
static uint32_t cache_hash_name(const char *name)
{
    uint32_t h = 5381;
    for (const unsigned char *p = (const unsigned char *)name; *p; p++)
        h = ((h << 5) + h) + *p;
    return h;
}

/*
 * Build the open-addressing hash table from the items array.
 * Uses (item.hash ^ name_hash) as the probe key.
 * Load factor <= 0.5 (bucket_count >= 2 * item_count).
 */
static bool cache_build_hashtable(nipc_cgroups_cache_t *cache)
{
    free(cache->buckets);
    cache->buckets = NULL;
    cache->bucket_count = 0;

    if (cache->item_count == 0)
        return true;

    uint32_t bcount = next_power_of_2_u32(cache->item_count * 2);
    nipc_cgroups_hash_bucket_t *buckets = service_calloc(bcount,
        sizeof(nipc_cgroups_hash_bucket_t),
        NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_BUCKETS_CALLOC_INTERNAL);
    if (!buckets)
        return false;

    uint32_t mask = bcount - 1;
    for (uint32_t i = 0; i < cache->item_count; i++) {
        uint32_t key = cache->items[i].hash ^ cache_hash_name(cache->items[i].name);
        uint32_t slot = key & mask;

        /* Linear probe for an empty bucket */
        while (buckets[slot].used)
            slot = (slot + 1) & mask;

        buckets[slot].index = i;
        buckets[slot].used = true;
    }

    cache->buckets = buckets;
    cache->bucket_count = bcount;
    return true;
}

/*
 * Build a new cache from a decoded snapshot view. Copies all strings
 * from the ephemeral view into owned heap allocations.
 *
 * Returns the new items array and sets *count_out. Returns NULL on
 * allocation failure.
 */
static nipc_cgroups_cache_item_t *cache_build_items(
    const nipc_cgroups_resp_view_t *view,
    uint32_t *count_out)
{
    uint32_t n = view->item_count;
    *count_out = 0;

    if (n == 0)
        return NULL; /* empty snapshot is valid */

    nipc_cgroups_cache_item_t *items = service_calloc(
        n, sizeof(nipc_cgroups_cache_item_t),
        NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_ITEMS_CALLOC_INTERNAL);
    if (!items)
        return NULL;

    for (uint32_t i = 0; i < n; i++) {
        nipc_cgroups_item_view_t iv;
        nipc_error_t err = nipc_cgroups_resp_item(view, i, &iv);
        if (err != NIPC_OK) {
            /* Malformed item: abort build, free partial */
            cache_free_items(items, i);
            return NULL;
        }

        items[i].hash    = iv.hash;
        items[i].options = iv.options;
        items[i].enabled = iv.enabled;

        /* Copy name (add NUL terminator) */
        items[i].name = service_malloc(
            iv.name.len + 1, NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_ITEM_NAME_MALLOC_INTERNAL);
        if (!items[i].name) {
            cache_free_items(items, i);
            return NULL;
        }
        if (iv.name.len > 0)
            memcpy(items[i].name, iv.name.ptr, iv.name.len);
        items[i].name[iv.name.len] = '\0';

        /* Copy path (add NUL terminator) */
        items[i].path = service_malloc(
            iv.path.len + 1, NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_ITEM_PATH_MALLOC_INTERNAL);
        if (!items[i].path) {
            free(items[i].name);
            cache_free_items(items, i);
            return NULL;
        }
        if (iv.path.len > 0)
            memcpy(items[i].path, iv.path.ptr, iv.path.len);
        items[i].path[iv.path.len] = '\0';
    }

    *count_out = n;
    return items;
}

void nipc_cgroups_cache_init(nipc_cgroups_cache_t *cache,
                              const char *run_dir,
                              const char *service_name,
                              const nipc_client_config_t *config)
{
    memset(cache, 0, sizeof(*cache));

    nipc_client_init(&cache->client, run_dir, service_name, config);

    cache->items = NULL;
    cache->item_count = 0;
    cache->systemd_enabled = 0;
    cache->generation = 0;
    cache->populated = false;
    cache->buckets = NULL;
    cache->bucket_count = 0;
    cache->refresh_success_count = 0;
    cache->refresh_failure_count = 0;

    cache->response_buf = NULL;
    cache->response_buf_size = 0;
}

bool nipc_cgroups_cache_refresh(nipc_cgroups_cache_t *cache)
{
    /* Drive L2 connection lifecycle */
    nipc_client_refresh(&cache->client);

    /* Attempt snapshot call */
    nipc_cgroups_resp_view_t view;
    nipc_error_t err = nipc_client_call_cgroups_snapshot(&cache->client, &view);

    if (err != NIPC_OK) {
        /* Refresh failed -- preserve previous cache */
        cache->refresh_failure_count++;
        return false;
    }

    /* Build new cache from the snapshot view */
    uint32_t new_count = 0;
    nipc_cgroups_cache_item_t *new_items = NULL;

    if (view.item_count > 0) {
        new_items = cache_build_items(&view, &new_count);
        if (!new_items && view.item_count > 0) {
            /* Build failed (allocation error) -- preserve old cache */
            cache->refresh_failure_count++;
            return false;
        }
    }

    /* Replace old cache with new one */
    cache_free_items(cache->items, cache->item_count);
    cache->items = new_items;
    cache->item_count = new_count;
    cache->systemd_enabled = view.systemd_enabled;
    cache->generation = view.generation;
    cache->populated = true;
    cache->refresh_success_count++;

    /* Record monotonic timestamp */
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    cache->last_refresh_ts = (uint64_t)ts.tv_sec * 1000 + (uint64_t)ts.tv_nsec / 1000000;

    /* Rebuild hash table for O(1) lookup */
    cache_build_hashtable(cache);

    return true;
}

const nipc_cgroups_cache_item_t *nipc_cgroups_cache_lookup(
    const nipc_cgroups_cache_t *cache,
    uint32_t hash,
    const char *name)
{
    if (!cache->populated || !cache->items || !name)
        return NULL;

    /* Use hash table if available, fall back to linear scan */
    if (cache->buckets && cache->bucket_count > 0) {
        uint32_t key = hash ^ cache_hash_name(name);
        uint32_t mask = cache->bucket_count - 1;
        uint32_t slot = key & mask;

        while (cache->buckets[slot].used) {
            uint32_t idx = cache->buckets[slot].index;
            if (cache->items[idx].hash == hash &&
                strcmp(cache->items[idx].name, name) == 0) {
                return &cache->items[idx];
            }
            slot = (slot + 1) & mask;
        }
        return NULL;
    }

    /* Fallback linear scan (hash table allocation failed) */
    for (uint32_t i = 0; i < cache->item_count; i++) {
        if (cache->items[i].hash == hash &&
            strcmp(cache->items[i].name, name) == 0) {
            return &cache->items[i];
        }
    }

    return NULL;
}

void nipc_cgroups_cache_status(const nipc_cgroups_cache_t *cache,
                                nipc_cgroups_cache_status_t *out)
{
    out->populated             = cache->populated;
    out->item_count            = cache->item_count;
    out->systemd_enabled       = cache->systemd_enabled;
    out->generation            = cache->generation;
    out->refresh_success_count = cache->refresh_success_count;
    out->refresh_failure_count = cache->refresh_failure_count;
    out->connection_state      = cache->client.state;
    out->last_refresh_ts       = cache->last_refresh_ts;
}

void nipc_cgroups_cache_close(nipc_cgroups_cache_t *cache)
{
    cache_free_items(cache->items, cache->item_count);
    cache->items = NULL;
    cache->item_count = 0;
    cache->populated = false;

    free(cache->buckets);
    cache->buckets = NULL;
    cache->bucket_count = 0;

    free(cache->response_buf);
    cache->response_buf = NULL;
    cache->response_buf_size = 0;

    nipc_client_close(&cache->client);
}
