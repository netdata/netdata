#ifndef PROTOCOL_MESSAGE_H
#define PROTOCOL_MESSAGE_H

#include "daemon/common.h"

typedef struct {
    RRDHOST *host;
    struct netdata_ssl *ssl;
    int sockfd;
    int flags;
    time_t timeout;
} connection_handle_t;

typedef struct {
    char *buf;
    uint32_t len;
} binary_message_t;

bool binary_message_send(connection_handle_t *connection_handle,
                         binary_message_t *binary_message);

bool binary_message_recv(connection_handle_t *connection_handle,
                         binary_message_t *binary_message);

#endif /* PROTOCOL_MESSAGE_H */
