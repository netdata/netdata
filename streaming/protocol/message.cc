// SPDX-License-Identifier: GPL-3.0-or-later

#include "message.h"

static bool send_uint32(struct netdata_ssl *ssl, int sockfd,
                        int flags, time_t timeout, uint32_t value)
{
    uint32_t network_value = htonl(value);
    char *buf = (char*) &network_value;
    size_t n = sizeof(uint32_t);

    size_t remaining = send_exact(ssl, sockfd, buf, n, flags, timeout);
    return remaining == 0;
}

static bool recv_uint32(struct netdata_ssl *ssl, int sockfd,
                        int flags, time_t timeout, uint32_t *value)
{
    const size_t n = sizeof(uint32_t);
    char buf[n];

    size_t remaining = recv_exact(ssl, sockfd, buf, n, flags, timeout);
    if (remaining)
        return false;

    memcpy(value, buf, n);
    *value = ntohl(*value);
    return true;
}

static bool send_binary_message(connection_handle_t *conn, binary_message_t *msg)
{
    struct netdata_ssl *ssl = conn->ssl;
    int sockfd = conn->sockfd;
    int flags = conn->flags;
    time_t timeout = conn->timeout;

    if (!send_uint32(ssl, sockfd, flags, timeout, msg->len))
        return false;

    if (!msg->len)
        return true;

    size_t remaining = send_exact(ssl, sockfd, msg->buf, msg->len, flags, timeout);
    if (remaining)
        return false;

    return true;
}

static bool recv_binary_message(connection_handle_t *conn,
                                binary_message_t *msg)
{
    struct netdata_ssl *ssl = conn->ssl;
    int sockfd = conn->sockfd;
    int flags = conn->flags;
    time_t timeout = conn->timeout;

    if (!recv_uint32(ssl, sockfd, flags, timeout, &msg->len))
        return false;

    if (!msg->len)
        return true;

    msg->buf = static_cast<char *>(callocz(sizeof(char), msg->len));

    size_t remaining = recv_exact(ssl, sockfd, msg->buf, msg->len, flags, timeout);

    if (remaining) {
        freez(msg->buf);
        return false;
    }

    return true;
}

bool binary_message_send(connection_handle_t *connection_handle,
                         binary_message_t *binary_message)
{
    return send_binary_message(connection_handle, binary_message);
}

bool binary_message_recv(connection_handle_t *connection_handle,
                         binary_message_t *binary_message)
{
    return recv_binary_message(connection_handle, binary_message);
}
