/*
 * netipc_uds.c - L1 POSIX UDS SEQPACKET transport.
 *
 * Implements connection lifecycle, handshake with profile/limit negotiation,
 * and send/receive with transparent chunking over AF_UNIX SEQPACKET sockets.
 */

#include "netipc/netipc_uds.h"
#include "netipc/netipc_protocol.h"

#include <errno.h>
#include <fcntl.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/un.h>

/* ------------------------------------------------------------------ */
/*  Internal constants                                                 */
/* ------------------------------------------------------------------ */

#define UDS_DEFAULT_BACKLOG   16
#define UDS_DEFAULT_BATCH_ITEMS 1
#define UDS_INITIAL_RECV_BUF  4096

/* ------------------------------------------------------------------ */
/*  Internal helpers                                                   */
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

/* Build socket path into dst, return 0 on success, -1 if too long. */
static int build_socket_path(char *dst, size_t dst_len,
                             const char *run_dir, const char *service_name)
{
    if (validate_service_name(service_name) < 0)
        return -2; /* invalid service name */

    int n = snprintf(dst, dst_len, "%s/%s.sock", run_dir, service_name);
    if (n < 0 || (size_t)n >= dst_len)
        return -1; /* path too long */
    return 0;
}

/* Get the socket's send buffer size as the packet size. */
static uint32_t detect_packet_size(int fd)
{
    int val = 0;
    socklen_t len = sizeof(val);
    if (getsockopt(fd, SOL_SOCKET, SO_SNDBUF, &val, &len) < 0)
        return 65536; /* safe default */

    /* Linux doubles SO_SNDBUF internally; use the value as-is, it
     * represents the actual kernel buffer available. Clamp to u32. */
    if (val <= 0)
        return 65536;
    return (uint32_t)val;
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

static nipc_uds_error_t raw_send(int fd, const void *data, size_t len);

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

static void send_rejection_ack(int fd, uint16_t status)
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
    raw_send(fd, pkt, NIPC_HEADER_LEN + sizeof(ack_buf));
}

/* ------------------------------------------------------------------ */
/*  Low-level send/recv (one SEQPACKET datagram)                       */
/* ------------------------------------------------------------------ */

/* Send exactly len bytes as one SEQPACKET message. */
static nipc_uds_error_t raw_send(int fd, const void *data, size_t len)
{
    ssize_t n = send(fd, data, len, MSG_NOSIGNAL);
    if (n < 0 || (size_t)n != len)
        return NIPC_UDS_ERR_SEND;
    return NIPC_UDS_OK;
}

/* Send header + payload as one SEQPACKET message using sendmsg. */
static nipc_uds_error_t raw_send_iov(int fd, const void *hdr, size_t hdr_len,
                                      const void *payload, size_t payload_len)
{
    struct iovec iov[2];
    struct msghdr msg;
    int iovcnt = 0;

    memset(&msg, 0, sizeof(msg));

    iov[0].iov_base = (void *)hdr;
    iov[0].iov_len  = hdr_len;
    iovcnt = 1;

    if (payload && payload_len > 0) {
        iov[1].iov_base = (void *)payload;
        iov[1].iov_len  = payload_len;
        iovcnt = 2;
    }

    msg.msg_iov    = iov;
    msg.msg_iovlen = iovcnt;

    size_t total = hdr_len + payload_len;
    ssize_t n = sendmsg(fd, &msg, MSG_NOSIGNAL);
    if (n < 0 || (size_t)n != total)
        return NIPC_UDS_ERR_SEND;

    return NIPC_UDS_OK;
}

/* Receive one SEQPACKET message into buf. Returns bytes received, 0 on
 * disconnect, -1 on error. */
static ssize_t raw_recv(int fd, void *buf, size_t buf_len)
{
    ssize_t n = recv(fd, buf, buf_len, 0);
    return n;
}

/* ------------------------------------------------------------------ */
/*  Handshake: client side                                             */
/* ------------------------------------------------------------------ */

static nipc_uds_error_t client_handshake(int fd,
                                          const nipc_uds_client_config_t *cfg,
                                          nipc_uds_session_t *session)
{
    uint8_t buf[128]; /* enough for header(32) + hello(44) = 76, and ack */
    nipc_uds_error_t err;

    /* Detect packet size if not specified */
    uint32_t pkt_size = cfg->packet_size;
    if (pkt_size == 0)
        pkt_size = detect_packet_size(fd);

    /* Build HELLO payload */
    nipc_hello_t hello = {
        .layout_version             = 1,
        .flags                      = 0,
        .supported_profiles         = cfg->supported_profiles ? cfg->supported_profiles : NIPC_PROFILE_BASELINE,
        .preferred_profiles         = cfg->preferred_profiles,
        .max_request_payload_bytes  = apply_default(cfg->max_request_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT),
        .max_request_batch_items    = apply_default(cfg->max_request_batch_items, UDS_DEFAULT_BATCH_ITEMS),
        .max_response_payload_bytes = apply_default(cfg->max_response_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT),
        .max_response_batch_items   = apply_default(cfg->max_response_batch_items, UDS_DEFAULT_BATCH_ITEMS),
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
    err = raw_send(fd, buf, NIPC_HEADER_LEN + sizeof(hello_buf));
    if (err != NIPC_UDS_OK)
        return err;

    /* Receive HELLO_ACK */
    ssize_t n = raw_recv(fd, buf, sizeof(buf));
    if (n <= 0)
        return NIPC_UDS_ERR_RECV;

    /* Decode outer header */
    nipc_header_t ack_hdr;
    nipc_error_t perr = nipc_header_decode(buf, (size_t)n, &ack_hdr);
    if (perr == NIPC_ERR_BAD_VERSION)
        return NIPC_UDS_ERR_INCOMPATIBLE;
    if (perr != NIPC_OK)
        return NIPC_UDS_ERR_PROTOCOL;

    if (ack_hdr.kind != NIPC_KIND_CONTROL || ack_hdr.code != NIPC_CODE_HELLO_ACK)
        return NIPC_UDS_ERR_PROTOCOL;

    /* Check transport_status for rejection */
    if (ack_hdr.transport_status == NIPC_STATUS_AUTH_FAILED)
        return NIPC_UDS_ERR_AUTH_FAILED;
    if (ack_hdr.transport_status == NIPC_STATUS_UNSUPPORTED)
        return NIPC_UDS_ERR_NO_PROFILE;
    if (ack_hdr.transport_status == NIPC_STATUS_INCOMPATIBLE)
        return NIPC_UDS_ERR_INCOMPATIBLE;
    if (ack_hdr.transport_status == NIPC_STATUS_LIMIT_EXCEEDED)
        return NIPC_UDS_ERR_LIMIT_EXCEEDED;
    if (ack_hdr.transport_status != NIPC_STATUS_OK)
        return NIPC_UDS_ERR_HANDSHAKE;

    /* Decode hello-ack payload */
    nipc_hello_ack_t ack;
    perr = nipc_hello_ack_decode(buf + NIPC_HEADER_LEN,
                                  (size_t)n - NIPC_HEADER_LEN, &ack);
    if (perr == NIPC_ERR_BAD_LAYOUT &&
        hello_ack_layout_incompatible(buf + NIPC_HEADER_LEN,
                                      (size_t)n - NIPC_HEADER_LEN))
        return NIPC_UDS_ERR_INCOMPATIBLE;
    if (perr != NIPC_OK)
        return NIPC_UDS_ERR_PROTOCOL;

    /* Fill session */
    session->fd                        = fd;
    session->role                      = NIPC_UDS_ROLE_CLIENT;
    session->max_request_payload_bytes = ack.agreed_max_request_payload_bytes;
    session->max_request_batch_items   = ack.agreed_max_request_batch_items;
    session->max_response_payload_bytes = ack.agreed_max_response_payload_bytes;
    session->max_response_batch_items  = ack.agreed_max_response_batch_items;
    session->packet_size               = ack.agreed_packet_size;
    session->selected_profile          = ack.selected_profile;
    session->session_id                = ack.session_id;
    session->recv_buf                  = NULL;
    session->recv_buf_size             = 0;

    /* Sanity: reject a packet_size too small for chunking arithmetic */
    if (session->packet_size <= NIPC_HEADER_LEN)
        return NIPC_UDS_ERR_PROTOCOL;

    return NIPC_UDS_OK;
}

/* ------------------------------------------------------------------ */
/*  Handshake: server side                                             */
/* ------------------------------------------------------------------ */

static nipc_uds_error_t server_handshake(int fd,
                                          const nipc_uds_server_config_t *cfg,
                                          uint64_t session_id,
                                          nipc_uds_session_t *session)
{
    uint8_t buf[128];

    /* Detect server packet size */
    uint32_t server_pkt_size = cfg->packet_size;
    if (server_pkt_size == 0)
        server_pkt_size = detect_packet_size(fd);

    /* Server limits with defaults applied */
    uint32_t s_req_pay  = apply_default(cfg->max_request_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT);
    uint32_t s_req_bat  = apply_default(cfg->max_request_batch_items, UDS_DEFAULT_BATCH_ITEMS);
    uint32_t s_resp_pay = apply_default(cfg->max_response_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT);
    uint32_t s_resp_bat = apply_default(cfg->max_response_batch_items, UDS_DEFAULT_BATCH_ITEMS);
    uint32_t s_profiles = cfg->supported_profiles ? cfg->supported_profiles : NIPC_PROFILE_BASELINE;
    uint32_t s_preferred = cfg->preferred_profiles;

    /* Receive HELLO */
    ssize_t n = raw_recv(fd, buf, sizeof(buf));
    if (n <= 0)
        return NIPC_UDS_ERR_RECV;

    nipc_header_t hdr;
    nipc_error_t perr = nipc_header_decode(buf, (size_t)n, &hdr);
    if (perr == NIPC_ERR_BAD_VERSION &&
        header_version_incompatible(buf, (size_t)n, NIPC_CODE_HELLO)) {
        send_rejection_ack(fd, NIPC_STATUS_INCOMPATIBLE);
        return NIPC_UDS_ERR_INCOMPATIBLE;
    }
    if (perr != NIPC_OK)
        return NIPC_UDS_ERR_PROTOCOL;

    if (hdr.kind != NIPC_KIND_CONTROL || hdr.code != NIPC_CODE_HELLO)
        return NIPC_UDS_ERR_PROTOCOL;

    nipc_hello_t hello;
    perr = nipc_hello_decode(buf + NIPC_HEADER_LEN,
                              (size_t)n - NIPC_HEADER_LEN, &hello);
    if (perr == NIPC_ERR_BAD_LAYOUT &&
        hello_layout_incompatible(buf + NIPC_HEADER_LEN,
                                  (size_t)n - NIPC_HEADER_LEN)) {
        send_rejection_ack(fd, NIPC_STATUS_INCOMPATIBLE);
        return NIPC_UDS_ERR_INCOMPATIBLE;
    }
    if (perr != NIPC_OK)
        return NIPC_UDS_ERR_PROTOCOL;

    /* Compute intersection */
    uint32_t intersection = hello.supported_profiles & s_profiles;

    /* Check intersection */
    if (intersection == 0) {
        send_rejection_ack(fd, NIPC_STATUS_UNSUPPORTED);
        return NIPC_UDS_ERR_NO_PROFILE;
    }

    /* Check auth */
    if (hello.auth_token != cfg->auth_token) {
        send_rejection_ack(fd, NIPC_STATUS_AUTH_FAILED);
        return NIPC_UDS_ERR_AUTH_FAILED;
    }

    /* Select profile: prefer preferred_intersection, then intersection */
    uint32_t preferred_intersection = intersection &
                                       hello.preferred_profiles & s_preferred;
    uint32_t selected;
    if (preferred_intersection != 0)
        selected = highest_bit(preferred_intersection);
    else
        selected = highest_bit(intersection);

    if (hello.max_request_payload_bytes > NIPC_MAX_PAYLOAD_CAP) {
        send_rejection_ack(fd, NIPC_STATUS_LIMIT_EXCEEDED);
        return NIPC_UDS_ERR_LIMIT_EXCEEDED;
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

    /* packet_size must be large enough for a usable message packet */
    if (agreed_pkt <= NIPC_HEADER_LEN) {
        send_rejection_ack(fd, NIPC_STATUS_INCOMPATIBLE);
        return NIPC_UDS_ERR_INCOMPATIBLE;
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

    nipc_uds_error_t send_ack_err = raw_send(fd, pkt, NIPC_HEADER_LEN + sizeof(ack_buf));
    if (send_ack_err != NIPC_UDS_OK)
        return send_ack_err;

    /* Fill session */
    session->fd                        = fd;
    session->role                      = NIPC_UDS_ROLE_SERVER;
    session->max_request_payload_bytes = agreed_req_pay;
    session->max_request_batch_items   = agreed_req_bat;
    session->max_response_payload_bytes = agreed_resp_pay;
    session->max_response_batch_items  = agreed_resp_bat;
    session->packet_size               = agreed_pkt;
    session->selected_profile          = selected;
    session->session_id                = session_id;
    session->recv_buf                  = NULL;
    session->recv_buf_size             = 0;

    return NIPC_UDS_OK;
}

/* ------------------------------------------------------------------ */
/*  Stale endpoint recovery                                            */
/* ------------------------------------------------------------------ */

/* Returns: 0 = stale (unlinked), 1 = live server, -1 = doesn't exist */
static int check_and_recover_stale(const char *path)
{
    struct stat st;
    if (stat(path, &st) != 0)
        return -1; /* doesn't exist */

    /* Try connecting to check if a live server is there */
    int probe = socket(AF_UNIX, SOCK_SEQPACKET, 0);
    if (probe < 0)
        return -1;

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, path, sizeof(addr.sun_path) - 1);

    int ret;
    if (connect(probe, (struct sockaddr *)&addr, sizeof(addr)) == 0) {
        /* Connected -> live server */
        close(probe);
        ret = 1;
    } else {
        int saved_errno = errno;
        close(probe);
        /* Only unlink on ECONNREFUSED/ENOENT (stale socket).
         * Other errors (EACCES, etc.) should not remove the file. */
        if (saved_errno == ECONNREFUSED || saved_errno == ENOENT) {
            unlink(path);
            ret = 0;
        } else {
            /* Can't determine ownership — treat as live to prevent overwriting */
            ret = 1;
        }
    }
    return ret;
}

/* ------------------------------------------------------------------ */
/*  Public API: listen                                                 */
/* ------------------------------------------------------------------ */

nipc_uds_error_t nipc_uds_listen(const char *run_dir,
                                  const char *service_name,
                                  const nipc_uds_server_config_t *config,
                                  nipc_uds_listener_t *out)
{
    memset(out, 0, sizeof(*out));
    out->fd = -1;

    /* Build path */
    char path[sizeof(((struct sockaddr_un *)0)->sun_path)];
    int path_rc = build_socket_path(path, sizeof(path), run_dir, service_name);
    if (path_rc == -2)
        return NIPC_UDS_ERR_BAD_PARAM;
    if (path_rc < 0)
        return NIPC_UDS_ERR_PATH_TOO_LONG;

    /* Stale recovery */
    int stale = check_and_recover_stale(path);
    if (stale == 1)
        return NIPC_UDS_ERR_ADDR_IN_USE;

    /* Create socket */
    int fd = socket(AF_UNIX, SOCK_SEQPACKET, 0);
    if (fd < 0)
        return NIPC_UDS_ERR_SOCKET;

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, path, sizeof(addr.sun_path) - 1);

    if (bind(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        close(fd);
        return NIPC_UDS_ERR_SOCKET;
    }

    int backlog = config->backlog > 0 ? config->backlog : UDS_DEFAULT_BACKLOG;
    if (listen(fd, backlog) < 0) {
        close(fd);
        unlink(path);
        return NIPC_UDS_ERR_SOCKET;
    }

    out->fd = fd;
    out->config = *config;
    strncpy(out->path, path, sizeof(out->path) - 1);
    out->path[sizeof(out->path) - 1] = '\0';

    return NIPC_UDS_OK;
}

/* ------------------------------------------------------------------ */
/*  Public API: accept                                                 */
/* ------------------------------------------------------------------ */

nipc_uds_error_t nipc_uds_accept(nipc_uds_listener_t *listener,
                                  uint64_t session_id,
                                  nipc_uds_session_t *out)
{
    memset(out, 0, sizeof(*out));
    out->fd = -1;

    int client_fd = accept(listener->fd, NULL, NULL);
    if (client_fd < 0)
        return NIPC_UDS_ERR_ACCEPT;

    nipc_uds_error_t err = server_handshake(client_fd, &listener->config,
                                             session_id, out);
    if (err != NIPC_UDS_OK) {
        close(client_fd);
        out->fd = -1;
        return err;
    }

    return NIPC_UDS_OK;
}

/* ------------------------------------------------------------------ */
/*  Public API: connect                                                */
/* ------------------------------------------------------------------ */

nipc_uds_error_t nipc_uds_connect(const char *run_dir,
                                   const char *service_name,
                                   const nipc_uds_client_config_t *config,
                                   nipc_uds_session_t *out)
{
    memset(out, 0, sizeof(*out));
    out->fd = -1;

    /* Build path */
    char path[sizeof(((struct sockaddr_un *)0)->sun_path)];
    int path_rc2 = build_socket_path(path, sizeof(path), run_dir, service_name);
    if (path_rc2 == -2)
        return NIPC_UDS_ERR_BAD_PARAM;
    if (path_rc2 < 0)
        return NIPC_UDS_ERR_PATH_TOO_LONG;

    /* Create socket */
    int fd = socket(AF_UNIX, SOCK_SEQPACKET, 0);
    if (fd < 0)
        return NIPC_UDS_ERR_SOCKET;

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, path, sizeof(addr.sun_path) - 1);

    if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        close(fd);
        return NIPC_UDS_ERR_CONNECT;
    }

    nipc_uds_error_t err = client_handshake(fd, config, out);
    if (err != NIPC_UDS_OK) {
        close(fd);
        out->fd = -1;
        return err;
    }

    return NIPC_UDS_OK;
}

/* ------------------------------------------------------------------ */
/*  Public API: close                                                  */
/* ------------------------------------------------------------------ */

void nipc_uds_close_session(nipc_uds_session_t *session)
{
    if (!session)
        return;

    if (session->fd >= 0) {
        close(session->fd);
        session->fd = -1;
    }

    free(session->recv_buf);
    session->recv_buf = NULL;
    session->recv_buf_size = 0;

    free(session->inflight_ids);
    session->inflight_ids = NULL;
    session->inflight_count = 0;
    session->inflight_capacity = 0;
}

void nipc_uds_close_listener(nipc_uds_listener_t *listener)
{
    if (!listener)
        return;

    if (listener->fd >= 0) {
        close(listener->fd);
        listener->fd = -1;
    }

    if (listener->path[0]) {
        unlink(listener->path);
        listener->path[0] = '\0';
    }
}

/* ------------------------------------------------------------------ */
/*  In-flight message_id tracking (client side)                        */
/* ------------------------------------------------------------------ */

/* Add message_id to in-flight set. Returns 0 on success, -1 if duplicate,
 * -2 on allocation failure. */
static int inflight_add(nipc_uds_session_t *s, uint64_t id)
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

/* Remove message_id from in-flight set. Returns 0 on success, -1 if
 * not found. */
static int inflight_remove(nipc_uds_session_t *s, uint64_t id)
{
    for (uint32_t i = 0; i < s->inflight_count; i++) {
        if (s->inflight_ids[i] == id) {
            /* Swap with last */
            s->inflight_ids[i] = s->inflight_ids[s->inflight_count - 1];
            s->inflight_count--;
            return 0;
        }
    }
    return -1; /* not found */
}

static void inflight_fail_all(nipc_uds_session_t *s)
{
    if (!s || s->role != NIPC_UDS_ROLE_CLIENT)
        return;

    /* A broken session invalidates every in-flight request on it. */
    s->inflight_count = 0;
}

/* ------------------------------------------------------------------ */
/*  Public API: send                                                   */
/* ------------------------------------------------------------------ */

nipc_uds_error_t nipc_uds_send(nipc_uds_session_t *session,
                                nipc_header_t *hdr,
                                const void *payload,
                                size_t payload_len)
{
    if (!session || session->fd < 0)
        return NIPC_UDS_ERR_BAD_PARAM;

    int tracked = (session->role == NIPC_UDS_ROLE_CLIENT &&
                   hdr->kind == NIPC_KIND_REQUEST);

    /* Client-side: track in-flight message_ids for requests */
    if (tracked) {
        int rc = inflight_add(session, hdr->message_id);
        if (rc == -1)
            return NIPC_UDS_ERR_DUPLICATE_MSG_ID;
        if (rc == -2)
            return NIPC_UDS_ERR_LIMIT_EXCEEDED;
    }

    uint32_t max_payload = 0;
    uint32_t max_batch = 0;
    if (session->role == NIPC_UDS_ROLE_CLIENT &&
        hdr->kind == NIPC_KIND_REQUEST) {
        max_payload = session->max_request_payload_bytes;
        max_batch = session->max_request_batch_items;
    } else if (session->role == NIPC_UDS_ROLE_SERVER &&
               hdr->kind == NIPC_KIND_RESPONSE) {
        max_payload = session->max_response_payload_bytes;
        max_batch = session->max_response_batch_items;
    }
    if (payload_len > UINT32_MAX ||
        (max_payload > 0 && payload_len > max_payload) ||
        (max_batch > 0 && hdr->item_count > max_batch)) {
        if (tracked)
            inflight_remove(session, hdr->message_id);
        return NIPC_UDS_ERR_LIMIT_EXCEEDED;
    }

    /* Fill envelope fields the caller shouldn't set */
    hdr->magic      = NIPC_MAGIC_MSG;
    hdr->version    = NIPC_VERSION;
    hdr->header_len = NIPC_HEADER_LEN;
    hdr->payload_len = (uint32_t)payload_len;

    size_t total_msg = NIPC_HEADER_LEN + payload_len;

    /* Does it fit in one packet? */
    if (total_msg <= session->packet_size) {
        /* Single packet: encode header then send header+payload together */
        uint8_t hdr_buf[NIPC_HEADER_LEN];
        nipc_header_encode(hdr, hdr_buf, sizeof(hdr_buf));
        nipc_uds_error_t send_err = raw_send_iov(session->fd, hdr_buf, NIPC_HEADER_LEN,
                            payload, payload_len);
        if (send_err != NIPC_UDS_OK) {
            if (session->role == NIPC_UDS_ROLE_CLIENT &&
                hdr->kind == NIPC_KIND_REQUEST) {
                if (send_err == NIPC_UDS_ERR_SEND)
                    inflight_fail_all(session);
                else
                    inflight_remove(session, hdr->message_id);
            }
        }
        return send_err;
    }

    /* Chunked send */
    size_t chunk_payload_budget = session->packet_size - NIPC_HEADER_LEN;
    if (chunk_payload_budget == 0)
        return NIPC_UDS_ERR_BAD_PARAM;

    /* Calculate chunk count:
     * First chunk: header(32) + up to chunk_payload_budget payload bytes.
     * But the first chunk carries the outer header, so payload in first
     * chunk = chunk_payload_budget. */
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

    /* Send first chunk: outer header + first part of payload */
    uint8_t hdr_buf[NIPC_HEADER_LEN];
    nipc_header_encode(hdr, hdr_buf, sizeof(hdr_buf));

    nipc_uds_error_t err = raw_send_iov(session->fd, hdr_buf, NIPC_HEADER_LEN,
                                         payload, first_chunk_payload);
    if (err != NIPC_UDS_OK) {
        if (session->role == NIPC_UDS_ROLE_CLIENT &&
            hdr->kind == NIPC_KIND_REQUEST) {
            if (err == NIPC_UDS_ERR_SEND)
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

        err = raw_send_iov(session->fd, chk_buf, NIPC_HEADER_LEN,
                           src, this_chunk);
        if (err != NIPC_UDS_OK) {
            if (session->role == NIPC_UDS_ROLE_CLIENT &&
                hdr->kind == NIPC_KIND_REQUEST) {
                if (err == NIPC_UDS_ERR_SEND)
                    inflight_fail_all(session);
                else
                    inflight_remove(session, hdr->message_id);
            }
            return err;
        }

        src += this_chunk;
        remaining -= this_chunk;
    }

    return NIPC_UDS_OK;
}

/* ------------------------------------------------------------------ */
/*  Public API: receive                                                */
/* ------------------------------------------------------------------ */

/* Ensure session recv_buf can hold at least `needed` bytes. */
static nipc_uds_error_t ensure_recv_buf(nipc_uds_session_t *session,
                                         size_t needed)
{
    if (session->recv_buf_size >= needed)
        return NIPC_UDS_OK;

    uint8_t *p = realloc(session->recv_buf, needed);
    if (!p)
        return NIPC_UDS_ERR_ALLOC;

    session->recv_buf = p;
    session->recv_buf_size = needed;
    return NIPC_UDS_OK;
}

/* Validate batch directory if the message has BATCH flag and item_count > 1.
 * Called after the full payload is assembled, before returning to caller. */
static nipc_uds_error_t validate_batch(const nipc_header_t *hdr,
                                        const void *payload, size_t payload_len)
{
    if (!(hdr->flags & NIPC_FLAG_BATCH) || hdr->item_count <= 1)
        return NIPC_UDS_OK;

    uint32_t dir_bytes = hdr->item_count * 8;
    uint32_t dir_aligned = (uint32_t)nipc_align8(dir_bytes);
    if (payload_len < dir_aligned)
        return NIPC_UDS_ERR_PROTOCOL;

    uint32_t packed_area_len = (uint32_t)(payload_len - dir_aligned);
    nipc_error_t perr = nipc_batch_dir_validate(payload, dir_bytes,
                                                  hdr->item_count,
                                                  packed_area_len);
    return (perr == NIPC_OK) ? NIPC_UDS_OK : NIPC_UDS_ERR_PROTOCOL;
}

nipc_uds_error_t nipc_uds_receive(nipc_uds_session_t *session,
                                   void *buf, size_t buf_size,
                                   nipc_header_t *hdr_out,
                                   const void **payload_out,
                                   size_t *payload_len_out)
{
    if (!session || session->fd < 0)
        return NIPC_UDS_ERR_BAD_PARAM;

    /* Read first packet into the caller's buffer */
    ssize_t n = raw_recv(session->fd, buf, buf_size);
    if (n <= 0) {
        inflight_fail_all(session);
        return NIPC_UDS_ERR_RECV;
    }

    if ((size_t)n < NIPC_HEADER_LEN)
        return NIPC_UDS_ERR_PROTOCOL;

    /* Decode outer header */
    nipc_error_t perr = nipc_header_decode(buf, (size_t)n, hdr_out);
    if (perr != NIPC_OK)
        return NIPC_UDS_ERR_PROTOCOL;

    /* Validate payload_len against negotiated directional limit.
     * Server receives requests; client receives responses. */
    uint32_t max_payload = (session->role == NIPC_UDS_ROLE_SERVER)
        ? session->max_request_payload_bytes
        : session->max_response_payload_bytes;
    if (hdr_out->payload_len > max_payload)
        return NIPC_UDS_ERR_LIMIT_EXCEEDED;

    /* Validate item_count against negotiated directional batch limit. */
    uint32_t max_batch = (session->role == NIPC_UDS_ROLE_SERVER)
        ? session->max_request_batch_items
        : session->max_response_batch_items;
    if (hdr_out->item_count > max_batch)
        return NIPC_UDS_ERR_LIMIT_EXCEEDED;

    /* Client-side: validate response message_id is in-flight */
    if (session->role == NIPC_UDS_ROLE_CLIENT &&
        hdr_out->kind == NIPC_KIND_RESPONSE) {
        if (inflight_remove(session, hdr_out->message_id) < 0)
            return NIPC_UDS_ERR_UNKNOWN_MSG_ID;
    }

    size_t total_msg = NIPC_HEADER_LEN + hdr_out->payload_len;

    /* Non-chunked: entire message arrived in one packet */
    if ((size_t)n >= total_msg) {
        *payload_out = (const uint8_t *)buf + NIPC_HEADER_LEN;
        *payload_len_out = hdr_out->payload_len;

        nipc_uds_error_t berr = validate_batch(hdr_out, *payload_out, *payload_len_out);
        if (berr != NIPC_UDS_OK)
            return berr;

        return NIPC_UDS_OK;
    }

    /* Chunked: first packet has partial payload. The total message
     * size is NIPC_HEADER_LEN + payload_len from the header. */
    size_t first_payload_bytes = (size_t)n - NIPC_HEADER_LEN;

    /* We need a buffer for the full payload. Use the session recv_buf. */
    nipc_uds_error_t err = ensure_recv_buf(session, hdr_out->payload_len);
    if (err != NIPC_UDS_OK)
        return err;

    /* Copy first chunk's payload into recv_buf */
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

    /* A temporary buffer for reading continuation packets */
    size_t pkt_buf_size = session->packet_size;
    uint8_t *pkt_buf = malloc(pkt_buf_size);
    if (!pkt_buf)
        return NIPC_UDS_ERR_ALLOC;

    for (uint32_t ci = 1; assembled < hdr_out->payload_len; ci++) {
        ssize_t cn = raw_recv(session->fd, pkt_buf, pkt_buf_size);
        if (cn <= 0) {
            free(pkt_buf);
            inflight_fail_all(session);
            return NIPC_UDS_ERR_RECV;
        }

        if ((size_t)cn < NIPC_HEADER_LEN) {
            free(pkt_buf);
            return NIPC_UDS_ERR_CHUNK;
        }

        nipc_chunk_header_t chk;
        perr = nipc_chunk_header_decode(pkt_buf, (size_t)cn, &chk);
        if (perr != NIPC_OK) {
            free(pkt_buf);
            return NIPC_UDS_ERR_CHUNK;
        }

        /* Validate chunk header */
        if (chk.message_id != hdr_out->message_id ||
            chk.chunk_index != ci ||
            chk.chunk_count != expected_chunk_count ||
            chk.total_message_len != (uint32_t)total_msg) {
            free(pkt_buf);
            return NIPC_UDS_ERR_CHUNK;
        }

        size_t chunk_data = (size_t)cn - NIPC_HEADER_LEN;
        if (chunk_data != chk.chunk_payload_len) {
            free(pkt_buf);
            return NIPC_UDS_ERR_CHUNK;
        }

        if (assembled + chunk_data > hdr_out->payload_len) {
            free(pkt_buf);
            return NIPC_UDS_ERR_CHUNK;
        }

        memcpy(session->recv_buf + assembled,
               pkt_buf + NIPC_HEADER_LEN, chunk_data);
        assembled += chunk_data;
    }

    free(pkt_buf);

    *payload_out = session->recv_buf;
    *payload_len_out = hdr_out->payload_len;

    nipc_uds_error_t berr = validate_batch(hdr_out, *payload_out, *payload_len_out);
    if (berr != NIPC_UDS_OK)
        return berr;

    return NIPC_UDS_OK;
}
