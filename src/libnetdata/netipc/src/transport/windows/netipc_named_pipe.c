/*
 * netipc_named_pipe.c - L1 Windows Named Pipe transport.
 *
 * Implements connection lifecycle, handshake with profile/limit negotiation,
 * and send/receive with transparent chunking over Win32 Named Pipes in
 * message mode. Wire-compatible with all language implementations.
 */

#if defined(_WIN32) || defined(__MSYS__)

#include "netipc/netipc_named_pipe.h"
#include "netipc/netipc_protocol.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* ------------------------------------------------------------------ */
/*  Win32 error constants for disconnect detection                     */
/* ------------------------------------------------------------------ */

#ifndef ERROR_BROKEN_PIPE
#define ERROR_BROKEN_PIPE 109
#endif
#ifndef ERROR_NO_DATA
#define ERROR_NO_DATA 232
#endif
#ifndef ERROR_PIPE_NOT_CONNECTED
#define ERROR_PIPE_NOT_CONNECTED 233
#endif

/* ------------------------------------------------------------------ */
/*  Internal helpers                                                   */
/* ------------------------------------------------------------------ */

static inline uint32_t min_u32(uint32_t a, uint32_t b)
{
    return a < b ? a : b;
}

static inline uint32_t max_u32(uint32_t a, uint32_t b)
{
    return a > b ? a : b;
}

static inline uint32_t apply_default(uint32_t val, uint32_t def)
{
    return val == 0 ? def : val;
}

static inline uint32_t pipe_buffer_size(uint32_t packet_size)
{
    /* The protocol packet size controls logical framing and chunk size. The
     * underlying pipe quota must stay large enough for full-duplex pipelining
     * even when tests force a tiny protocol packet size. */
    return max_u32(apply_default(packet_size, NIPC_NP_DEFAULT_PIPE_BUF_SIZE),
                   NIPC_NP_DEFAULT_PIPE_BUF_SIZE);
}

static bool header_version_incompatible(const void *buf, size_t buf_len,
                                        uint16_t expected_code)
{
    if (buf_len < NIPC_HEADER_LEN)
        return false;

    nipc_header_t hdr;
    memcpy(&hdr, buf, sizeof(hdr));
    return hdr.magic == NIPC_MAGIC_MSG &&
           hdr.version != NIPC_VERSION &&
           hdr.header_len == NIPC_HEADER_LEN &&
           hdr.kind == NIPC_KIND_CONTROL &&
           hdr.code == expected_code;
}

static bool hello_layout_incompatible(const void *buf, size_t buf_len)
{
    if (buf_len < sizeof(uint16_t))
        return false;

    nipc_hello_t hello;
    memset(&hello, 0, sizeof(hello));
    memcpy(&hello, buf, sizeof(uint16_t));
    return hello.layout_version != 1;
}

static bool hello_ack_layout_incompatible(const void *buf, size_t buf_len)
{
    if (buf_len < sizeof(uint16_t))
        return false;

    nipc_hello_ack_t ack;
    memset(&ack, 0, sizeof(ack));
    memcpy(&ack, buf, sizeof(uint16_t));
    return ack.layout_version != 1;
}

static nipc_np_error_t raw_send(HANDLE pipe, const void *data, size_t len);

static void send_rejection_ack(HANDLE pipe, uint16_t status)
{
    nipc_hello_ack_t ack = { .layout_version = 1 };
    uint8_t ack_buf[48];
    uint8_t pkt[80];
    nipc_header_t ack_hdr = {
        .magic = NIPC_MAGIC_MSG,
        .version = NIPC_VERSION,
        .header_len = NIPC_HEADER_LEN,
        .kind = NIPC_KIND_CONTROL,
        .code = NIPC_CODE_HELLO_ACK,
        .transport_status = status,
        .payload_len = sizeof(ack_buf),
        .item_count = 1,
    };

    nipc_hello_ack_encode(&ack, ack_buf, sizeof(ack_buf));
    nipc_header_encode(&ack_hdr, pkt, sizeof(pkt));
    memcpy(pkt + NIPC_HEADER_LEN, ack_buf, sizeof(ack_buf));
    raw_send(pipe, pkt, NIPC_HEADER_LEN + sizeof(ack_buf));
}

/* Highest set bit in a bitmask (0 if empty). */
static uint32_t highest_bit(uint32_t mask)
{
    if (mask == 0)
        return 0;

    uint32_t bit = 1u << 31;
    while (!(mask & bit))
        bit >>= 1;
    return bit;
}

/* ------------------------------------------------------------------ */
/*  FNV-1a 64-bit hash                                                 */
/* ------------------------------------------------------------------ */

uint64_t nipc_fnv1a_64(const void *data, size_t len)
{
    uint64_t hash = NIPC_FNV1A_OFFSET_BASIS;
    const uint8_t *p = (const uint8_t *)data;

    for (size_t i = 0; i < len; i++) {
        hash ^= (uint64_t)p[i];
        hash *= NIPC_FNV1A_PRIME;
    }

    return hash;
}

/* ------------------------------------------------------------------ */
/*  Service name validation                                            */
/* ------------------------------------------------------------------ */

/* Validate service_name: only [a-zA-Z0-9._-], non-empty, not "." or "..". */
static int validate_service_name(const char *name)
{
    if (!name || name[0] == '\0')
        return -1;

    /* Reject "." and ".." */
    if (name[0] == '.' && (name[1] == '\0' || (name[1] == '.' && name[2] == '\0')))
        return -1;

    for (const char *p = name; *p; p++) {
        char c = *p;
        if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
            (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-')
            continue;
        return -1;
    }
    return 0;
}

/* ------------------------------------------------------------------ */
/*  Pipe name derivation                                               */
/* ------------------------------------------------------------------ */

int nipc_np_build_pipe_name(wchar_t *dst, size_t dst_chars,
                             const char *run_dir,
                             const char *service_name)
{
    if (!dst || dst_chars == 0 || !run_dir || !service_name)
        return -1;

    if (validate_service_name(service_name) < 0)
        return -1;

    /* Hash the run_dir */
    uint64_t hash = nipc_fnv1a_64(run_dir, strlen(run_dir));

    /* Build the pipe name as narrow first, then widen */
    char narrow[NIPC_NP_MAX_PIPE_NAME];
    int n = snprintf(narrow, sizeof(narrow),
                     "\\\\.\\pipe\\netipc-%016llx-%s",
                     (unsigned long long)hash, service_name);
    if (n < 0 || (size_t)n >= sizeof(narrow))
        return -1;

    /* Convert to wide string */
    if ((size_t)(n + 1) > dst_chars)
        return -1;

    for (int i = 0; i <= n; i++)
        dst[i] = (wchar_t)(unsigned char)narrow[i];

    return 0;
}

/* ------------------------------------------------------------------ */
/*  Create a new pipe instance                                         */
/* ------------------------------------------------------------------ */

static HANDLE create_pipe_instance(const wchar_t *pipe_name,
                                    uint32_t buf_size,
                                    BOOL first_instance)
{
    DWORD open_mode = PIPE_ACCESS_DUPLEX;
    if (first_instance)
        open_mode |= FILE_FLAG_FIRST_PIPE_INSTANCE;

    HANDLE h = CreateNamedPipeW(
        pipe_name,
        open_mode,
        PIPE_TYPE_MESSAGE | PIPE_READMODE_MESSAGE | PIPE_WAIT,
        NIPC_NP_MAX_INSTANCES,
        buf_size,
        buf_size,
        0,    /* default timeout */
        NULL  /* default security */
    );

    return h;
}

/* ------------------------------------------------------------------ */
/*  Low-level send / recv                                              */
/* ------------------------------------------------------------------ */

/* Check if a Win32 error indicates peer disconnect (graceful). */
static int is_disconnect_error(DWORD err)
{
    return err == ERROR_BROKEN_PIPE ||
           err == ERROR_NO_DATA ||
           err == ERROR_PIPE_NOT_CONNECTED;
}

/* Write exactly len bytes as one pipe message. */
static nipc_np_error_t raw_send(HANDLE pipe, const void *data, size_t len)
{
    DWORD written = 0;
    BOOL ok = WriteFile(pipe, data, (DWORD)len, &written, NULL);
    if (!ok) {
        if (is_disconnect_error(GetLastError()))
            return NIPC_NP_ERR_DISCONNECTED;
        return NIPC_NP_ERR_SEND;
    }
    if (written != (DWORD)len)
        return NIPC_NP_ERR_SEND;
    return NIPC_NP_OK;
}

/* Send header + payload as one pipe message (concatenated). */
static nipc_np_error_t raw_send_msg(HANDLE pipe,
                                     const void *hdr, size_t hdr_len,
                                     const void *payload, size_t payload_len)
{
    /* Named Pipes don't support scatter-gather like sendmsg.
     * Concatenate into one buffer for a single WriteFile call. */
    size_t total = hdr_len + payload_len;
    uint8_t stack_buf[256];
    uint8_t *buf;
    int allocated = 0;

    if (total <= sizeof(stack_buf)) {
        buf = stack_buf;
    } else {
        buf = (uint8_t *)malloc(total);
        if (!buf)
            return NIPC_NP_ERR_ALLOC;
        allocated = 1;
    }

    memcpy(buf, hdr, hdr_len);
    if (payload && payload_len > 0)
        memcpy(buf + hdr_len, payload, payload_len);

    nipc_np_error_t err = raw_send(pipe, buf, total);

    if (allocated)
        free(buf);

    return err;
}

/* Read one pipe message. Returns bytes read, 0 on disconnect, -1 on error. */
static int raw_recv(HANDLE pipe, void *buf, size_t buf_len, DWORD *bytes_read)
{
    DWORD read = 0;
    BOOL ok = ReadFile(pipe, buf, (DWORD)buf_len, &read, NULL);
    if (!ok) {
        DWORD err = GetLastError();
        if (is_disconnect_error(err)) {
            *bytes_read = 0;
            return 0; /* disconnect */
        }
        return -1; /* error */
    }
    if (read == 0) {
        *bytes_read = 0;
        return 0; /* disconnect */
    }
    *bytes_read = read;
    return 1; /* success */
}

/* Poll a synchronous pipe handle until bytes are available. Go/Rust already
 * use PeekNamedPipe here; waiting on the pipe HANDLE itself is not reliable
 * for request readability. */
static nipc_np_error_t wait_readable(HANDLE pipe,
                                      uint32_t timeout_ms,
                                      bool *readable_out)
{
    ULONGLONG deadline = GetTickCount64() + timeout_ms;
    int yielded = 0;

    if (readable_out)
        *readable_out = false;

    for (;;) {
        DWORD available = 0;
        BOOL ok = PeekNamedPipe(pipe, NULL, 0, NULL, &available, NULL);
        if (!ok) {
            DWORD err = GetLastError();
            if (is_disconnect_error(err))
                return NIPC_NP_ERR_DISCONNECTED;
            return NIPC_NP_ERR_RECV;
        }

        if (available > 0) {
            if (readable_out)
                *readable_out = true;
            return NIPC_NP_OK;
        }

        if (GetTickCount64() >= deadline)
            return NIPC_NP_OK;

        if (!yielded) {
            yielded = 1;
            for (int i = 0; i < 256; i++) {
                SwitchToThread();

                available = 0;
                ok = PeekNamedPipe(pipe, NULL, 0, NULL, &available, NULL);
                if (!ok) {
                    DWORD err = GetLastError();
                    if (is_disconnect_error(err))
                        return NIPC_NP_ERR_DISCONNECTED;
                    return NIPC_NP_ERR_RECV;
                }

                if (available > 0) {
                    if (readable_out)
                        *readable_out = true;
                    return NIPC_NP_OK;
                }

                if (GetTickCount64() >= deadline)
                    return NIPC_NP_OK;
            }
            continue;
        }

        Sleep(1);
    }
}

/* ------------------------------------------------------------------ */
/*  In-flight message_id tracking (client side)                        */
/* ------------------------------------------------------------------ */

static int inflight_add(nipc_np_session_t *s, uint64_t id)
{
    for (uint32_t i = 0; i < s->inflight_count; i++) {
        if (s->inflight_ids[i] == id)
            return -1; /* duplicate */
    }
    if (s->inflight_count >= s->inflight_capacity) {
        uint32_t new_cap = s->inflight_capacity ? s->inflight_capacity * 2 : 16;
        uint64_t *new_ids = realloc(s->inflight_ids, (size_t)new_cap * sizeof(uint64_t));
        if (!new_ids)
            return -2; /* allocation failure */
        s->inflight_ids = new_ids;
        s->inflight_capacity = new_cap;
    }
    s->inflight_ids[s->inflight_count++] = id;
    return 0;
}

static int inflight_remove(nipc_np_session_t *s, uint64_t id)
{
    for (uint32_t i = 0; i < s->inflight_count; i++) {
        if (s->inflight_ids[i] == id) {
            s->inflight_ids[i] = s->inflight_ids[s->inflight_count - 1];
            s->inflight_count--;
            return 0;
        }
    }
    return -1; /* not found */
}

static void inflight_fail_all(nipc_np_session_t *s)
{
    if (!s || s->role != NIPC_NP_ROLE_CLIENT)
        return;

    /* A broken session invalidates every in-flight request on it. */
    s->inflight_count = 0;
}

/* ------------------------------------------------------------------ */
/*  Handshake: client side                                             */
/* ------------------------------------------------------------------ */

static nipc_np_error_t client_handshake(HANDLE pipe,
                                         const nipc_np_client_config_t *cfg,
                                         nipc_np_session_t *session)
{
    uint8_t buf[128];

    uint32_t pkt_size = apply_default(cfg->packet_size, NIPC_NP_DEFAULT_PACKET_SIZE);

    /* Build HELLO payload */
    nipc_hello_t hello = {
        .layout_version             = 1,
        .flags                      = 0,
        .supported_profiles         = cfg->supported_profiles ? cfg->supported_profiles : NIPC_PROFILE_BASELINE,
        .preferred_profiles         = cfg->preferred_profiles,
        .max_request_payload_bytes  = apply_default(cfg->max_request_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT),
        .max_request_batch_items    = apply_default(cfg->max_request_batch_items, NIPC_NP_DEFAULT_BATCH_ITEMS),
        .max_response_payload_bytes = apply_default(cfg->max_response_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT),
        .max_response_batch_items   = apply_default(cfg->max_response_batch_items, NIPC_NP_DEFAULT_BATCH_ITEMS),
        .auth_token                 = cfg->auth_token,
        .packet_size                = pkt_size,
    };

    uint8_t hello_buf[44];
    nipc_hello_encode(&hello, hello_buf, sizeof(hello_buf));

    /* Build outer CONTROL header */
    nipc_header_t hdr = {
        .magic            = NIPC_MAGIC_MSG,
        .version          = NIPC_VERSION,
        .header_len       = NIPC_HEADER_LEN,
        .kind             = NIPC_KIND_CONTROL,
        .flags            = 0,
        .code             = NIPC_CODE_HELLO,
        .transport_status = NIPC_STATUS_OK,
        .payload_len      = sizeof(hello_buf),
        .item_count       = 1,
        .message_id       = 0,
    };

    nipc_header_encode(&hdr, buf, sizeof(buf));
    memcpy(buf + NIPC_HEADER_LEN, hello_buf, sizeof(hello_buf));

    /* Send HELLO */
    nipc_np_error_t err = raw_send(pipe, buf, NIPC_HEADER_LEN + sizeof(hello_buf));
    if (err != NIPC_NP_OK)
        return err;

    /* Receive HELLO_ACK */
    DWORD bytes_read = 0;
    int rc = raw_recv(pipe, buf, sizeof(buf), &bytes_read);
    if (rc <= 0)
        return NIPC_NP_ERR_RECV;

    /* Decode outer header */
    nipc_header_t ack_hdr;
    nipc_error_t perr = nipc_header_decode(buf, (size_t)bytes_read, &ack_hdr);
    if (perr == NIPC_ERR_BAD_VERSION)
        return NIPC_NP_ERR_INCOMPATIBLE;
    if (perr != NIPC_OK)
        return NIPC_NP_ERR_PROTOCOL;

    if (ack_hdr.kind != NIPC_KIND_CONTROL || ack_hdr.code != NIPC_CODE_HELLO_ACK)
        return NIPC_NP_ERR_PROTOCOL;

    /* Check transport_status for rejection */
    if (ack_hdr.transport_status == NIPC_STATUS_AUTH_FAILED)
        return NIPC_NP_ERR_AUTH_FAILED;
    if (ack_hdr.transport_status == NIPC_STATUS_UNSUPPORTED)
        return NIPC_NP_ERR_NO_PROFILE;
    if (ack_hdr.transport_status == NIPC_STATUS_INCOMPATIBLE)
        return NIPC_NP_ERR_INCOMPATIBLE;
    if (ack_hdr.transport_status == NIPC_STATUS_LIMIT_EXCEEDED)
        return NIPC_NP_ERR_LIMIT_EXCEEDED;
    if (ack_hdr.transport_status != NIPC_STATUS_OK)
        return NIPC_NP_ERR_HANDSHAKE;

    /* Decode hello-ack payload */
    nipc_hello_ack_t ack;
    perr = nipc_hello_ack_decode(buf + NIPC_HEADER_LEN,
                                  (size_t)bytes_read - NIPC_HEADER_LEN, &ack);
    if (perr == NIPC_ERR_BAD_LAYOUT &&
        hello_ack_layout_incompatible(buf + NIPC_HEADER_LEN,
                                      (size_t)bytes_read - NIPC_HEADER_LEN))
        return NIPC_NP_ERR_INCOMPATIBLE;
    if (perr != NIPC_OK)
        return NIPC_NP_ERR_PROTOCOL;

    /* Fill session */
    session->pipe                       = pipe;
    session->role                       = NIPC_NP_ROLE_CLIENT;
    session->max_request_payload_bytes  = ack.agreed_max_request_payload_bytes;
    session->max_request_batch_items    = ack.agreed_max_request_batch_items;
    session->max_response_payload_bytes = ack.agreed_max_response_payload_bytes;
    session->max_response_batch_items   = ack.agreed_max_response_batch_items;
    session->packet_size                = ack.agreed_packet_size;
    session->selected_profile           = ack.selected_profile;
    session->session_id                 = ack.session_id;
    session->recv_buf                   = NULL;
    session->recv_buf_size              = 0;
    session->inflight_count             = 0;

    return NIPC_NP_OK;
}

/* ------------------------------------------------------------------ */
/*  Handshake: server side                                             */
/* ------------------------------------------------------------------ */

static nipc_np_error_t server_handshake(HANDLE pipe,
                                         const nipc_np_server_config_t *cfg,
                                         uint64_t session_id,
                                         nipc_np_session_t *session)
{
    uint8_t buf[128];

    uint32_t server_pkt_size = apply_default(cfg->packet_size, NIPC_NP_DEFAULT_PACKET_SIZE);
    uint32_t s_req_pay  = apply_default(cfg->max_request_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT);
    uint32_t s_req_bat  = apply_default(cfg->max_request_batch_items, NIPC_NP_DEFAULT_BATCH_ITEMS);
    uint32_t s_resp_pay = apply_default(cfg->max_response_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT);
    uint32_t s_resp_bat = apply_default(cfg->max_response_batch_items, NIPC_NP_DEFAULT_BATCH_ITEMS);
    uint32_t s_profiles = cfg->supported_profiles ? cfg->supported_profiles : NIPC_PROFILE_BASELINE;
    uint32_t s_preferred = cfg->preferred_profiles;

    /* Receive HELLO */
    DWORD bytes_read = 0;
    int rc = raw_recv(pipe, buf, sizeof(buf), &bytes_read);
    if (rc <= 0)
        return NIPC_NP_ERR_RECV;

    nipc_header_t hdr;
    nipc_error_t perr = nipc_header_decode(buf, (size_t)bytes_read, &hdr);
    if (perr == NIPC_ERR_BAD_VERSION &&
        header_version_incompatible(buf, (size_t)bytes_read, NIPC_CODE_HELLO)) {
        send_rejection_ack(pipe, NIPC_STATUS_INCOMPATIBLE);
        return NIPC_NP_ERR_INCOMPATIBLE;
    }
    if (perr != NIPC_OK)
        return NIPC_NP_ERR_PROTOCOL;

    if (hdr.kind != NIPC_KIND_CONTROL || hdr.code != NIPC_CODE_HELLO)
        return NIPC_NP_ERR_PROTOCOL;

    nipc_hello_t hello;
    perr = nipc_hello_decode(buf + NIPC_HEADER_LEN,
                              (size_t)bytes_read - NIPC_HEADER_LEN, &hello);
    if (perr == NIPC_ERR_BAD_LAYOUT &&
        hello_layout_incompatible(buf + NIPC_HEADER_LEN,
                                  (size_t)bytes_read - NIPC_HEADER_LEN)) {
        send_rejection_ack(pipe, NIPC_STATUS_INCOMPATIBLE);
        return NIPC_NP_ERR_INCOMPATIBLE;
    }
    if (perr != NIPC_OK)
        return NIPC_NP_ERR_PROTOCOL;

    /* Compute intersection */
    uint32_t intersection = hello.supported_profiles & s_profiles;

    /* Check intersection */
    if (intersection == 0) {
        send_rejection_ack(pipe, NIPC_STATUS_UNSUPPORTED);
        return NIPC_NP_ERR_NO_PROFILE;
    }

    /* Check auth */
    if (hello.auth_token != cfg->auth_token) {
        send_rejection_ack(pipe, NIPC_STATUS_AUTH_FAILED);
        return NIPC_NP_ERR_AUTH_FAILED;
    }

    /* Select profile */
    uint32_t preferred_intersection = intersection &
                                       hello.preferred_profiles & s_preferred;
    uint32_t selected;
    if (preferred_intersection != 0)
        selected = highest_bit(preferred_intersection);
    else
        selected = highest_bit(intersection);

    if (hello.max_request_payload_bytes > NIPC_MAX_PAYLOAD_CAP) {
        send_rejection_ack(pipe, NIPC_STATUS_LIMIT_EXCEEDED);
        return NIPC_NP_ERR_LIMIT_EXCEEDED;
    }

    /* Negotiate limits:
     * - request payload and batch size are client-proposed and echoed
     * - response payload is server-authoritative
     * - response batch size is symmetric with request batch size */
    uint32_t agreed_req_pay  = hello.max_request_payload_bytes;
    uint32_t agreed_req_bat  = hello.max_request_batch_items;
    uint32_t agreed_resp_pay = s_resp_pay;
    uint32_t agreed_resp_bat = agreed_req_bat;
    uint32_t agreed_pkt      = min_u32(hello.packet_size, server_pkt_size);

    /* Reject packet sizes too small for a usable message packet */
    if (agreed_pkt <= NIPC_HEADER_LEN) {
        send_rejection_ack(pipe, NIPC_STATUS_INCOMPATIBLE);
        return NIPC_NP_ERR_INCOMPATIBLE;
    }

    /* Send HELLO_ACK (success) */
    nipc_hello_ack_t ack = {
        .layout_version                    = 1,
        .flags                             = 0,
        .server_supported_profiles         = s_profiles,
        .intersection_profiles             = intersection,
        .selected_profile                  = selected,
        .agreed_max_request_payload_bytes  = agreed_req_pay,
        .agreed_max_request_batch_items    = agreed_req_bat,
        .agreed_max_response_payload_bytes = agreed_resp_pay,
        .agreed_max_response_batch_items   = agreed_resp_bat,
        .agreed_packet_size                = agreed_pkt,
        .session_id                        = session_id,
    };

    uint8_t ack_buf[48];
    nipc_hello_ack_encode(&ack, ack_buf, sizeof(ack_buf));

    nipc_header_t ack_hdr = {
        .magic            = NIPC_MAGIC_MSG,
        .version          = NIPC_VERSION,
        .header_len       = NIPC_HEADER_LEN,
        .kind             = NIPC_KIND_CONTROL,
        .flags            = 0,
        .code             = NIPC_CODE_HELLO_ACK,
        .transport_status = NIPC_STATUS_OK,
        .payload_len      = sizeof(ack_buf),
        .item_count       = 1,
        .message_id       = 0,
    };

    uint8_t pkt[80];
    nipc_header_encode(&ack_hdr, pkt, sizeof(pkt));
    memcpy(pkt + NIPC_HEADER_LEN, ack_buf, sizeof(ack_buf));

    nipc_np_error_t send_err = raw_send(pipe, pkt, NIPC_HEADER_LEN + sizeof(ack_buf));
    if (send_err != NIPC_NP_OK)
        return send_err;

    /* Fill session */
    session->pipe                       = pipe;
    session->role                       = NIPC_NP_ROLE_SERVER;
    session->max_request_payload_bytes  = agreed_req_pay;
    session->max_request_batch_items    = agreed_req_bat;
    session->max_response_payload_bytes = agreed_resp_pay;
    session->max_response_batch_items   = agreed_resp_bat;
    session->packet_size                = agreed_pkt;
    session->selected_profile           = selected;
    session->session_id                 = session_id;
    session->recv_buf                   = NULL;
    session->recv_buf_size              = 0;
    session->inflight_count             = 0;

    return NIPC_NP_OK;
}

/* ------------------------------------------------------------------ */
/*  Public API: listen                                                 */
/* ------------------------------------------------------------------ */

nipc_np_error_t nipc_np_listen(const char *run_dir,
                                const char *service_name,
                                const nipc_np_server_config_t *config,
                                nipc_np_listener_t *out)
{
    if (!config || !out)
        return NIPC_NP_ERR_BAD_PARAM;

    memset(out, 0, sizeof(*out));
    out->pipe = INVALID_HANDLE_VALUE;

    /* Build pipe name */
    if (nipc_np_build_pipe_name(out->pipe_name, NIPC_NP_MAX_PIPE_NAME,
                                 run_dir, service_name) < 0)
        return NIPC_NP_ERR_BAD_PARAM;

    /* Determine buffer size */
    uint32_t buf_size = pipe_buffer_size(config->packet_size);

    /* Create first pipe instance with FILE_FLAG_FIRST_PIPE_INSTANCE
     * to detect if another server already owns this pipe name. */
    HANDLE h = create_pipe_instance(out->pipe_name, buf_size, TRUE);
    if (h == INVALID_HANDLE_VALUE) {
        DWORD err = GetLastError();
        if (err == ERROR_ACCESS_DENIED || err == ERROR_PIPE_BUSY)
            return NIPC_NP_ERR_ADDR_IN_USE;
        return NIPC_NP_ERR_CREATE_PIPE;
    }

    out->pipe = h;
    out->config = *config;

    return NIPC_NP_OK;
}

/* ------------------------------------------------------------------ */
/*  Public API: accept                                                 */
/* ------------------------------------------------------------------ */

nipc_np_error_t nipc_np_accept(nipc_np_listener_t *listener,
                                uint64_t session_id,
                                nipc_np_session_t *out)
{
    if (!listener || !out)
        return NIPC_NP_ERR_BAD_PARAM;
    if (listener->pipe == INVALID_HANDLE_VALUE || listener->pipe == NULL)
        return NIPC_NP_ERR_ACCEPT;

    memset(out, 0, sizeof(*out));
    out->pipe = INVALID_HANDLE_VALUE;

    /* Wait for client to connect to the current pipe instance */
    BOOL connected = ConnectNamedPipe(listener->pipe, NULL);
    if (!connected) {
        DWORD err = GetLastError();
        /* ERROR_PIPE_CONNECTED means a client connected between
         * CreateNamedPipe and ConnectNamedPipe — that's fine. */
        if (err != ERROR_PIPE_CONNECTED) {
            return NIPC_NP_ERR_ACCEPT;
        }
    }

    /* The listener's current pipe instance is now the session pipe. */
    HANDLE session_pipe = listener->pipe;

    /* Create a new pipe instance for the next client. */
    uint32_t buf_size = pipe_buffer_size(listener->config.packet_size);
    HANDLE next = create_pipe_instance(listener->pipe_name, buf_size, FALSE);
    if (next == INVALID_HANDLE_VALUE) {
        /* Cannot create next instance — close the session pipe too. */
        DisconnectNamedPipe(session_pipe);
        CloseHandle(session_pipe);
        return NIPC_NP_ERR_CREATE_PIPE;
    }
    listener->pipe = next;

    /* Perform handshake */
    nipc_np_error_t herr = server_handshake(session_pipe, &listener->config,
                                             session_id, out);
    if (herr != NIPC_NP_OK) {
        DisconnectNamedPipe(session_pipe);
        CloseHandle(session_pipe);
        out->pipe = INVALID_HANDLE_VALUE;
        return herr;
    }

    return NIPC_NP_OK;
}

/* ------------------------------------------------------------------ */
/*  Public API: connect                                                */
/* ------------------------------------------------------------------ */

nipc_np_error_t nipc_np_connect(const char *run_dir,
                                 const char *service_name,
                                 const nipc_np_client_config_t *config,
                                 nipc_np_session_t *out)
{
    if (!config || !out)
        return NIPC_NP_ERR_BAD_PARAM;

    memset(out, 0, sizeof(*out));
    out->pipe = INVALID_HANDLE_VALUE;

    /* Build pipe name */
    wchar_t pipe_name[NIPC_NP_MAX_PIPE_NAME];
    if (nipc_np_build_pipe_name(pipe_name, NIPC_NP_MAX_PIPE_NAME,
                                 run_dir, service_name) < 0)
        return NIPC_NP_ERR_BAD_PARAM;

    /* Connect to the server pipe */
    HANDLE h = CreateFileW(
        pipe_name,
        GENERIC_READ | GENERIC_WRITE,
        0,     /* no sharing */
        NULL,  /* default security */
        OPEN_EXISTING,
        0,     /* default attributes */
        NULL   /* no template */
    );

    if (h == INVALID_HANDLE_VALUE)
        return NIPC_NP_ERR_CONNECT;

    /* Set read mode to message mode */
    DWORD mode = PIPE_READMODE_MESSAGE;
    if (!SetNamedPipeHandleState(h, &mode, NULL, NULL)) {
        CloseHandle(h);
        return NIPC_NP_ERR_CONNECT;
    }

    /* Perform handshake */
    nipc_np_error_t err = client_handshake(h, config, out);
    if (err != NIPC_NP_OK) {
        CloseHandle(h);
        out->pipe = INVALID_HANDLE_VALUE;
        return err;
    }

    return NIPC_NP_OK;
}

/* ------------------------------------------------------------------ */
/*  Public API: close                                                  */
/* ------------------------------------------------------------------ */

void nipc_np_close_session(nipc_np_session_t *session)
{
    if (!session)
        return;

    if (session->pipe != INVALID_HANDLE_VALUE && session->pipe != NULL) {
        /* Only the server side should flush before disconnecting.
         * Client-side flush on a broken session can block shutdown. */
        if (session->role == NIPC_NP_ROLE_SERVER) {
            FlushFileBuffers(session->pipe);
            DisconnectNamedPipe(session->pipe);
        }
        CloseHandle(session->pipe);
    }
    session->pipe = INVALID_HANDLE_VALUE;

    free(session->recv_buf);
    session->recv_buf = NULL;
    session->recv_buf_size = 0;

    free(session->inflight_ids);
    session->inflight_ids = NULL;
    session->inflight_count = 0;
    session->inflight_capacity = 0;
}

void nipc_np_close_listener(nipc_np_listener_t *listener)
{
    if (!listener)
        return;

    if (listener->pipe != INVALID_HANDLE_VALUE && listener->pipe != NULL) {
        /* A loopback connect reliably wakes a blocking ConnectNamedPipe()
         * so the accept thread can observe shutdown and exit. */
        if (listener->pipe_name[0] != L'\0') {
            HANDLE wake = CreateFileW(
                listener->pipe_name,
                GENERIC_READ | GENERIC_WRITE,
                0,
                NULL,
                OPEN_EXISTING,
                0,
                NULL);
            if (wake != INVALID_HANDLE_VALUE)
                CloseHandle(wake);
        }
        CloseHandle(listener->pipe);
    }
    listener->pipe = INVALID_HANDLE_VALUE;
}

/* ------------------------------------------------------------------ */
/*  Public API: send                                                   */
/* ------------------------------------------------------------------ */

nipc_np_error_t nipc_np_send(nipc_np_session_t *session,
                              nipc_header_t *hdr,
                              const void *payload,
                              size_t payload_len)
{
    if (!session || session->pipe == INVALID_HANDLE_VALUE)
        return NIPC_NP_ERR_BAD_PARAM;

    int tracked = (session->role == NIPC_NP_ROLE_CLIENT &&
                   hdr->kind == NIPC_KIND_REQUEST);

    /* Client-side: track in-flight message_ids for requests */
    if (tracked) {
        int rc = inflight_add(session, hdr->message_id);
        if (rc == -1)
            return NIPC_NP_ERR_DUPLICATE_MSG_ID;
        if (rc == -2)
            return NIPC_NP_ERR_LIMIT_EXCEEDED;
    }

    uint32_t max_payload = 0;
    uint32_t max_batch = 0;
    if (session->role == NIPC_NP_ROLE_CLIENT &&
        hdr->kind == NIPC_KIND_REQUEST) {
        max_payload = session->max_request_payload_bytes;
        max_batch = session->max_request_batch_items;
    } else if (session->role == NIPC_NP_ROLE_SERVER &&
               hdr->kind == NIPC_KIND_RESPONSE) {
        max_payload = session->max_response_payload_bytes;
        max_batch = session->max_response_batch_items;
    }
    if (payload_len > UINT32_MAX ||
        (max_payload > 0 && payload_len > max_payload) ||
        (max_batch > 0 && hdr->item_count > max_batch)) {
        if (tracked)
            inflight_remove(session, hdr->message_id);
        return NIPC_NP_ERR_LIMIT_EXCEEDED;
    }

    /* Fill envelope fields */
    hdr->magic      = NIPC_MAGIC_MSG;
    hdr->version    = NIPC_VERSION;
    hdr->header_len = NIPC_HEADER_LEN;
    hdr->payload_len = (uint32_t)payload_len;

    size_t total_msg = NIPC_HEADER_LEN + payload_len;

    /* Single packet? */
    if (total_msg <= session->packet_size) {
        uint8_t hdr_buf[NIPC_HEADER_LEN];
        nipc_header_encode(hdr, hdr_buf, sizeof(hdr_buf));
        nipc_np_error_t err = raw_send_msg(session->pipe, hdr_buf,
                                            NIPC_HEADER_LEN, payload, payload_len);
        if (err != NIPC_NP_OK && tracked) {
            if (err == NIPC_NP_ERR_SEND || err == NIPC_NP_ERR_DISCONNECTED)
                inflight_fail_all(session);
            else
                inflight_remove(session, hdr->message_id);
        }
        return err;
    }

    /* Chunked send */
    size_t chunk_payload_budget = session->packet_size - NIPC_HEADER_LEN;
    if (chunk_payload_budget == 0) {
        if (tracked) inflight_remove(session, hdr->message_id);
        return NIPC_NP_ERR_BAD_PARAM;
    }

    size_t remaining = payload_len;
    size_t first_chunk_payload = remaining < chunk_payload_budget
                                     ? remaining : chunk_payload_budget;

    remaining -= first_chunk_payload;
    uint32_t continuation_chunks = 0;
    if (remaining > 0) {
        continuation_chunks = (uint32_t)((remaining + chunk_payload_budget - 1)
                                          / chunk_payload_budget);
    }
    uint32_t chunk_count = 1 + continuation_chunks;

    /* Send first chunk */
    uint8_t hdr_buf[NIPC_HEADER_LEN];
    nipc_header_encode(hdr, hdr_buf, sizeof(hdr_buf));

    nipc_np_error_t err = raw_send_msg(session->pipe, hdr_buf, NIPC_HEADER_LEN,
                                        payload, first_chunk_payload);
    if (err != NIPC_NP_OK) {
        if (tracked) {
            if (err == NIPC_NP_ERR_SEND || err == NIPC_NP_ERR_DISCONNECTED)
                inflight_fail_all(session);
            else
                inflight_remove(session, hdr->message_id);
        }
        return err;
    }

    /* Send continuation chunks */
    const uint8_t *src = (const uint8_t *)payload + first_chunk_payload;
    remaining = payload_len - first_chunk_payload;

    for (uint32_t ci = 1; ci < chunk_count; ci++) {
        size_t this_chunk = remaining < chunk_payload_budget
                                ? remaining : chunk_payload_budget;

        nipc_chunk_header_t chk = {
            .magic             = NIPC_MAGIC_CHUNK,
            .version           = NIPC_VERSION,
            .flags             = 0,
            .message_id        = hdr->message_id,
            .total_message_len = (uint32_t)total_msg,
            .chunk_index       = ci,
            .chunk_count       = chunk_count,
            .chunk_payload_len = (uint32_t)this_chunk,
        };

        uint8_t chk_buf[NIPC_HEADER_LEN];
        nipc_chunk_header_encode(&chk, chk_buf, sizeof(chk_buf));

        err = raw_send_msg(session->pipe, chk_buf, NIPC_HEADER_LEN,
                           src, this_chunk);
        if (err != NIPC_NP_OK) {
            if (tracked) {
                if (err == NIPC_NP_ERR_SEND || err == NIPC_NP_ERR_DISCONNECTED)
                    inflight_fail_all(session);
                else
                    inflight_remove(session, hdr->message_id);
            }
            return err;
        }

        src += this_chunk;
        remaining -= this_chunk;
    }

    return NIPC_NP_OK;
}

/* ------------------------------------------------------------------ */
/*  Public API: receive                                                */
/* ------------------------------------------------------------------ */

static nipc_np_error_t ensure_recv_buf(nipc_np_session_t *session,
                                        size_t needed)
{
    if (session->recv_buf_size >= needed)
        return NIPC_NP_OK;

    uint8_t *p = (uint8_t *)realloc(session->recv_buf, needed);
    if (!p)
        return NIPC_NP_ERR_ALLOC;

    session->recv_buf = p;
    session->recv_buf_size = needed;
    return NIPC_NP_OK;
}

/* Validate batch directory if the message has BATCH flag and item_count > 1.
 * Called after the full payload is assembled, before returning to caller. */
static nipc_np_error_t validate_batch(const nipc_header_t *hdr,
                                       const void *payload, size_t payload_len)
{
    if (!(hdr->flags & NIPC_FLAG_BATCH) || hdr->item_count <= 1)
        return NIPC_NP_OK;

    uint32_t dir_bytes = hdr->item_count * 8;
    uint32_t dir_aligned = (uint32_t)nipc_align8(dir_bytes);
    if (payload_len < dir_aligned)
        return NIPC_NP_ERR_PROTOCOL;

    uint32_t packed_area_len = (uint32_t)(payload_len - dir_aligned);
    nipc_error_t perr = nipc_batch_dir_validate(payload, dir_bytes,
                                                  hdr->item_count,
                                                  packed_area_len);
    return (perr == NIPC_OK) ? NIPC_NP_OK : NIPC_NP_ERR_PROTOCOL;
}

nipc_np_error_t nipc_np_receive(nipc_np_session_t *session,
                                 void *buf, size_t buf_size,
                                 nipc_header_t *hdr_out,
                                 const void **payload_out,
                                 size_t *payload_len_out)
{
    if (!session || session->pipe == INVALID_HANDLE_VALUE)
        return NIPC_NP_ERR_BAD_PARAM;

    /* Read first message */
    DWORD bytes_read = 0;
    int rc = raw_recv(session->pipe, buf, buf_size, &bytes_read);
    if (rc <= 0) {
        inflight_fail_all(session);
        return NIPC_NP_ERR_RECV;
    }

    size_t n = (size_t)bytes_read;
    if (n < NIPC_HEADER_LEN)
        return NIPC_NP_ERR_PROTOCOL;

    /* Decode outer header */
    nipc_error_t perr = nipc_header_decode(buf, n, hdr_out);
    if (perr != NIPC_OK)
        return NIPC_NP_ERR_PROTOCOL;

    /* Validate payload_len against negotiated directional limit */
    uint32_t max_payload = (session->role == NIPC_NP_ROLE_SERVER)
        ? session->max_request_payload_bytes
        : session->max_response_payload_bytes;
    if (hdr_out->payload_len > max_payload)
        return NIPC_NP_ERR_LIMIT_EXCEEDED;

    /* Validate item_count */
    uint32_t max_batch = (session->role == NIPC_NP_ROLE_SERVER)
        ? session->max_request_batch_items
        : session->max_response_batch_items;
    if (hdr_out->item_count > max_batch)
        return NIPC_NP_ERR_LIMIT_EXCEEDED;

    /* Client-side: validate response message_id is in-flight */
    if (session->role == NIPC_NP_ROLE_CLIENT &&
        hdr_out->kind == NIPC_KIND_RESPONSE) {
        if (inflight_remove(session, hdr_out->message_id) < 0)
            return NIPC_NP_ERR_UNKNOWN_MSG_ID;
    }

    size_t total_msg = NIPC_HEADER_LEN + hdr_out->payload_len;

    /* Non-chunked: entire message in one read */
    if (n >= total_msg) {
        *payload_out = (const uint8_t *)buf + NIPC_HEADER_LEN;
        *payload_len_out = hdr_out->payload_len;

        nipc_np_error_t berr = validate_batch(hdr_out, *payload_out, *payload_len_out);
        if (berr != NIPC_NP_OK)
            return berr;

        return NIPC_NP_OK;
    }

    /* Chunked: first message has partial payload */
    size_t first_payload_bytes = n - NIPC_HEADER_LEN;

    nipc_np_error_t err = ensure_recv_buf(session, hdr_out->payload_len);
    if (err != NIPC_NP_OK)
        return err;

    memcpy(session->recv_buf, (uint8_t *)buf + NIPC_HEADER_LEN,
           first_payload_bytes);

    size_t assembled = first_payload_bytes;
    size_t chunk_payload_budget = session->packet_size - NIPC_HEADER_LEN;

    /* Calculate expected chunk count */
    size_t remaining_after_first = hdr_out->payload_len - first_payload_bytes;
    uint32_t expected_continuations = 0;
    if (remaining_after_first > 0 && chunk_payload_budget > 0) {
        expected_continuations = (uint32_t)((remaining_after_first +
                                              chunk_payload_budget - 1)
                                             / chunk_payload_budget);
    }
    uint32_t expected_chunk_count = 1 + expected_continuations;

    /* Temporary buffer for continuation messages */
    size_t pkt_buf_size = session->packet_size;
    uint8_t *pkt_buf = (uint8_t *)malloc(pkt_buf_size);
    if (!pkt_buf)
        return NIPC_NP_ERR_ALLOC;

    for (uint32_t ci = 1; assembled < hdr_out->payload_len; ci++) {
        DWORD cn = 0;
        int crc = raw_recv(session->pipe, pkt_buf, pkt_buf_size, &cn);
        if (crc <= 0) {
            free(pkt_buf);
            inflight_fail_all(session);
            return NIPC_NP_ERR_RECV;
        }

        if ((size_t)cn < NIPC_HEADER_LEN) {
            free(pkt_buf);
            return NIPC_NP_ERR_CHUNK;
        }

        nipc_chunk_header_t chk;
        perr = nipc_chunk_header_decode(pkt_buf, (size_t)cn, &chk);
        if (perr != NIPC_OK) {
            free(pkt_buf);
            return NIPC_NP_ERR_CHUNK;
        }

        /* Validate chunk header */
        if (chk.message_id != hdr_out->message_id ||
            chk.chunk_index != ci ||
            chk.chunk_count != expected_chunk_count ||
            chk.total_message_len != (uint32_t)total_msg) {
            free(pkt_buf);
            return NIPC_NP_ERR_CHUNK;
        }

        size_t chunk_data = (size_t)cn - NIPC_HEADER_LEN;
        if (chunk_data != chk.chunk_payload_len) {
            free(pkt_buf);
            return NIPC_NP_ERR_CHUNK;
        }

        if (assembled + chunk_data > hdr_out->payload_len) {
            free(pkt_buf);
            return NIPC_NP_ERR_CHUNK;
        }

        memcpy(session->recv_buf + assembled,
               pkt_buf + NIPC_HEADER_LEN, chunk_data);
        assembled += chunk_data;
    }

    free(pkt_buf);

    *payload_out = session->recv_buf;
    *payload_len_out = hdr_out->payload_len;

    nipc_np_error_t berr = validate_batch(hdr_out, *payload_out, *payload_len_out);
    if (berr != NIPC_NP_OK)
        return berr;

    return NIPC_NP_OK;
}

nipc_np_error_t nipc_np_wait_readable(nipc_np_session_t *session,
                                       uint32_t timeout_ms,
                                       bool *readable_out)
{
    if (!session || !readable_out || session->pipe == INVALID_HANDLE_VALUE)
        return NIPC_NP_ERR_BAD_PARAM;
    nipc_np_error_t err = wait_readable(session->pipe, timeout_ms, readable_out);
    if (err == NIPC_NP_ERR_DISCONNECTED || err == NIPC_NP_ERR_RECV)
        inflight_fail_all(session);
    return err;
}

#endif /* _WIN32 || __MSYS__ */
