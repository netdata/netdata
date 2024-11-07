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
    ND_SOCK_ERR_NO_PARENT_AVAILABLE,
    ND_SOCK_ERR_UNKNOWN_ERROR,
} ND_SOCK_ERROR;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(ND_SOCK_ERROR);

typedef struct nd {
    bool verify_certificate;
    ND_SOCK_ERROR error;
    int fd;
    NETDATA_SSL ssl;
    SSL_CTX *ctx;
} ND_SOCK;

static inline void nd_sock_init(ND_SOCK *s, SSL_CTX *ctx) {
    memset(s, 0, sizeof(*s));
    s->ssl = NETDATA_SSL_UNSET_CONNECTION;
    s->fd = -1;
    s->ctx = ctx;
}

static inline bool nd_sock_is_ssl(ND_SOCK *s) {
    return SSL_connection(&(s)->ssl);
}

static inline SOCKET_PEERS nd_sock_socket_peers(ND_SOCK *s) {
    return socket_peers(s->fd);
}

static inline void nd_sock_close(ND_SOCK *s) {
    netdata_ssl_close(&s->ssl);

    if(s->fd != -1) {
        close(s->fd);
        s->fd = -1;
    }

    s->error = ND_SOCK_ERR_NONE;
}

static inline ssize_t nd_sock_read_nowait(ND_SOCK *s, void *buf, size_t num) {
    if (nd_sock_is_ssl(s))
        return netdata_ssl_read(&s->ssl, buf, num);
    else
        return read(s->fd, buf, num);
}

static inline ssize_t nd_sock_revc_nowait(ND_SOCK *s, void *buf, size_t num) {
    if (nd_sock_is_ssl(s))
        return netdata_ssl_read(&s->ssl, buf, num);
    else
        return recv(s->fd, buf, num, MSG_DONTWAIT);
}

static inline ssize_t nd_sock_send_nowait(ND_SOCK *s, void *buf, size_t num) {
    if (nd_sock_is_ssl(s))
        return netdata_ssl_write(&s->ssl, buf, num);
    else
        return send(s->fd, buf, num, MSG_DONTWAIT);
}

ssize_t nd_sock_send_timeout(ND_SOCK *s, void *buf, size_t len, int flags, time_t timeout);
ssize_t nd_sock_recv_timeout(ND_SOCK *s, void *buf, size_t len, int flags, time_t timeout);

bool nd_sock_connect_to_this(ND_SOCK *s, const char *definition, int default_port, time_t timeout, bool ssl);

#endif //NETDATA_ND_SOCK_H
