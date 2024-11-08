// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

ENUM_STR_MAP_DEFINE(ND_SOCK_ERROR) = {
    { .id = ND_SOCK_ERR_NONE,                               .name = "NONE", },
    { .id = ND_SOCK_ERR_CONNECTION_REFUSED,                 .name = "CONNECTION REFUSED", },
    { .id = ND_SOCK_ERR_CANNOT_RESOLVE_HOSTNAME,            .name = "CANNOT RESOLVE HOSTNAME", },
    { .id = ND_SOCK_ERR_FAILED_TO_CREATE_SOCKET,            .name = "FAILED TO CREATE SOCKET", },
    { .id = ND_SOCK_ERR_NO_HOST_IN_DEFINITION,              .name = "NO HOST IN DEFINITION", },
    { .id = ND_SOCK_ERR_POLL_ERROR,                         .name = "POLL ERROR", },
    { .id = ND_SOCK_ERR_TIMEOUT,                            .name = "TIMEOUT", },
    { .id = ND_SOCK_ERR_SSL_CANT_ESTABLISH_SSL_CONNECTION,  .name = "CANT ESTABLISH SSL CONNECTION", },
    { .id = ND_SOCK_ERR_SSL_INVALID_CERTIFICATE,            .name = "INVALID SSL CERTIFICATE", },
    { .id = ND_SOCK_ERR_SSL_FAILED_TO_OPEN,                 .name = "FAILED TO OPEN SSL", },
    { .id = ND_SOCK_ERR_THREAD_CANCELLED,                   .name = "THREAD CANCELLED", },
    { .id = ND_SOCK_ERR_NO_PARENT_AVAILABLE,                .name = "NO PARENT AVAILABLE", },
    { .id = ND_SOCK_ERR_UNKNOWN_ERROR,                      .name = "UNKNOWN ERROR", },

    // terminator
    { .name = NULL, .id = 0 }
};

ENUM_STR_DEFINE_FUNCTIONS(ND_SOCK_ERROR, ND_SOCK_ERR_NONE, "");

// --------------------------------------------------------------------------------------------------------------------

static const unsigned char alpn_proto_list[] = {
    18, 'n', 'e', 't', 'd', 'a', 't', 'a', '_', 's', 't', 'r', 'e', 'a', 'm', '/', '2', '.', '0',
    8, 'h', 't', 't', 'p', '/', '1', '.', '1'
};

static bool nd_sock_open_ssl(ND_SOCK *s) {
    if(!s) return false;

    if (netdata_ssl_open_ext(&s->ssl, s->ctx, s->fd, alpn_proto_list, sizeof(alpn_proto_list))) {
        if(!netdata_ssl_connect(&s->ssl)) {
            // couldn't connect
            s->error = ND_SOCK_ERR_SSL_CANT_ESTABLISH_SSL_CONNECTION;
            return false;
        }

        if (s->verify_certificate && security_test_certificate(s->ssl.conn)) {
            // certificate is not valid
            s->error = ND_SOCK_ERR_SSL_INVALID_CERTIFICATE;
            return false;
        }

        return true;
    }

    s->error = ND_SOCK_ERR_SSL_FAILED_TO_OPEN;
    return false;
}

bool nd_sock_connect_to_this(ND_SOCK *s, const char *definition, int default_port, time_t timeout, bool ssl) {
    nd_sock_close(s);

    struct timeval tv = {
        .tv_sec = timeout,
        .tv_usec = 0
    };

    s->fd = connect_to_this(definition, default_port, &tv);
    if(s->fd < 0) {
        s->error = -s->fd;
        return false;
    }

    if(ssl && s->ctx) {
        if (!nd_sock_open_ssl(s)) {
            close(s->fd);
            s->fd = -1;
            return false;
        }
    }
    else
        s->ssl = NETDATA_SSL_UNSET_CONNECTION;

    return true;
}

ssize_t nd_sock_send_timeout(ND_SOCK *s, void *buf, size_t len, int flags, time_t timeout) {
    switch(wait_on_socket_or_cancel_with_timeout(&s->ssl, s->fd, (int)(timeout * 1000), POLLOUT, NULL)) {
        case 0: // data are waiting
            break;

        case 1: // timeout
            s->error = ND_SOCK_ERR_TIMEOUT;
            return 0;

        case -1: // thread cancelled
            s->error = ND_SOCK_ERR_THREAD_CANCELLED;
            return -1;

        case 2:  // poll() error
            s->error = ND_SOCK_ERR_POLL_ERROR;
            return -1;

        default:
            s->error = ND_SOCK_ERR_UNKNOWN_ERROR;
            return -1;
    }

    if(s->ssl.conn) {
        if (nd_sock_is_ssl(s))
            return netdata_ssl_write(&s->ssl, buf, len);
        else
            return -1;
    }

    return send(s->fd, buf, len, flags);
}

ssize_t nd_sock_recv_timeout(ND_SOCK *s, void *buf, size_t len, int flags, time_t timeout) {
    switch(wait_on_socket_or_cancel_with_timeout(&s->ssl, s->fd, (int)(timeout * 1000), POLLIN, NULL)) {
        case 0: // data are waiting
            break;

        case 1: // timeout
            s->error = ND_SOCK_ERR_TIMEOUT;
            return 0;

        case -1: // thread cancelled
            s->error = ND_SOCK_ERR_THREAD_CANCELLED;
            return -1;

        case 2:  // poll() error
            s->error = ND_SOCK_ERR_POLL_ERROR;
            return -1;

        default:
            s->error = ND_SOCK_ERR_UNKNOWN_ERROR;
            return -1;
    }

    if (nd_sock_is_ssl(s))
        return netdata_ssl_read(&s->ssl, buf, len);

    return recv(s->fd, buf, len, flags);
}
