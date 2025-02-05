// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_SOCK_H
#define NETDATA_ND_SOCK_H

#include "socket-peers.h"

typedef enum __attribute__((packed)) {
    ND_SOCK_ERR_NONE = 0,
    ND_SOCK_ERR_CONNECTION_REFUSED,
    ND_SOCK_ERR_CANNOT_RESOLVE_HOSTNAME,
    ND_SOCK_ERR_FAILED_TO_CREATE_SOCKET,
    ND_SOCK_ERR_NO_HOST_IN_DEFINITION,
    ND_SOCK_ERR_POLL_ERROR,
    ND_SOCK_ERR_TIMEOUT,
    ND_SOCK_ERR_SSL_CANT_ESTABLISH_SSL_CONNECTION,
    ND_SOCK_ERR_SSL_INVALID_CERTIFICATE,
    ND_SOCK_ERR_SSL_FAILED_TO_OPEN,
    ND_SOCK_ERR_THREAD_CANCELLED,
    ND_SOCK_ERR_NO_DESTINATION_AVAILABLE,
    ND_SOCK_ERR_UNKNOWN_ERROR,

    // terminator
    ND_SOCK_ERR_MAX,
} ND_SOCK_ERROR;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(ND_SOCK_ERROR);

typedef struct nd_sock {
    bool verify_certificate;
    ND_SOCK_ERROR error;
    int fd;
    NETDATA_SSL ssl;
    SSL_CTX *ctx;
} ND_SOCK;

#define ND_SOCK_INIT(ssl_ctx, ssl_verify) (ND_SOCK){ \
        .verify_certificate = ssl_verify,            \
        .error = ND_SOCK_ERR_NONE,                   \
        .fd = -1,                                    \
        .ssl = NETDATA_SSL_UNSET_CONNECTION,         \
        .ctx = ssl_ctx,                              \
}

static inline void nd_sock_init(ND_SOCK *s, SSL_CTX *ctx, bool verify_certificate) {
    s->verify_certificate = verify_certificate;
    s->error = ND_SOCK_ERR_NONE;
    s->fd = -1;
    s->ssl = NETDATA_SSL_UNSET_CONNECTION;
    s->ctx = ctx;
}

ALWAYS_INLINE
static bool nd_sock_is_ssl(ND_SOCK *s) {
    return SSL_connection(&s->ssl);
}

ALWAYS_INLINE
static SOCKET_PEERS nd_sock_socket_peers(ND_SOCK *s) {
    return socket_peers(s->fd);
}

ALWAYS_INLINE
static void nd_sock_close(ND_SOCK *s) {
    netdata_ssl_close(&s->ssl);

    if(s->fd != -1) {
        close(s->fd);
        s->fd = -1;
    }

    s->error = ND_SOCK_ERR_NONE;
}

ALWAYS_INLINE
static ssize_t nd_sock_read(ND_SOCK *s, void *buf, size_t num, size_t retries) {
    ssize_t rc;
    do {
        if (nd_sock_is_ssl(s))
            rc = netdata_ssl_read(&s->ssl, buf, num);
        else
            rc = read(s->fd, buf, num);
    }
    while(rc <= 0 && (errno == EWOULDBLOCK || errno == EAGAIN || errno == EINTR) && retries--);

    return rc;
}

ALWAYS_INLINE
static ssize_t nd_sock_write(ND_SOCK *s, const void *buf, size_t num, size_t retries) {
    ssize_t rc;

    do {
        if (nd_sock_is_ssl(s))
            rc = netdata_ssl_write(&s->ssl, buf, num);
        else
            rc = write(s->fd, buf, num);
    }
    while(rc <= 0 && (errno == EWOULDBLOCK || errno == EAGAIN || errno == EINTR) && retries--);

    return rc;
}

ALWAYS_INLINE
static ssize_t nd_sock_write_persist(ND_SOCK *s, const void *buf, const size_t num, size_t retries) {
    const uint8_t *src = (const uint8_t *)buf;
    ssize_t bytes = 0;

    do {
        ssize_t sent = nd_sock_write(s, &src[bytes], (ssize_t)num - bytes, retries);
        if(sent <= 0) return sent;
        bytes += sent;
    }
    while(bytes < (ssize_t)num && retries--);

    return bytes;
}

ALWAYS_INLINE
static ssize_t nd_sock_revc_nowait(ND_SOCK *s, void *buf, size_t num) {
    if (nd_sock_is_ssl(s))
        return netdata_ssl_read(&s->ssl, buf, num);
    else
        return recv(s->fd, buf, num, MSG_DONTWAIT);
}

ALWAYS_INLINE
static ssize_t nd_sock_send_nowait(ND_SOCK *s, void *buf, size_t num) {
    if (nd_sock_is_ssl(s))
        return netdata_ssl_write(&s->ssl, buf, num);
    else
        return send(s->fd, buf, num, MSG_DONTWAIT);
}

ssize_t nd_sock_send_timeout(ND_SOCK *s, void *buf, size_t len, int flags, time_t timeout);
ssize_t nd_sock_recv_timeout(ND_SOCK *s, void *buf, size_t len, int flags, time_t timeout);

bool nd_sock_connect_to_this(ND_SOCK *s, const char *definition, int default_port, time_t timeout, bool ssl);

static inline void cleanup_nd_sock_p(ND_SOCK *s) {
    if(s) nd_sock_close(s);
}
#define CLEAN_ND_SOCK _cleanup_(cleanup_nd_sock_p) ND_SOCK

#endif //NETDATA_ND_SOCK_H
