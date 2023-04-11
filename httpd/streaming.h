// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef HTTPD_STREAMING_H
#define HTTPD_STREAMING_H

#include "daemon/common.h"
#include "mqtt_websockets/c-rbuf/include/ringbuffer.h"
#include "h2o.h"

typedef enum {
    STREAM_X_HTTP_1_1 = 0,
    STREAM_X_HTTP_1_1_DONE,
    STREAM_ACTIVE,
    STREAM_CLOSE
} h2o_stream_state_t;

typedef enum {
    HTTP_STREAM = 0,
    HTTP_URL,
    HTTP_PROTO,
    HTTP_USER_AGENT_KEY,
    HTTP_USER_AGENT_VALUE,
    HTTP_HDR,
    HTTP_DONE
} http_stream_parse_state_t;

typedef struct {
    h2o_socket_t *sock;
    h2o_stream_state_t state;

    rbuf_t rx;
    pthread_cond_t  rx_buf_cond;
    pthread_mutex_t rx_buf_lock;

    rbuf_t tx;
    h2o_iovec_t tx_buf;
    pthread_mutex_t tx_buf_lock;

    http_stream_parse_state_t parse_state;
    char *url;
    char *user_agent;

    int shutdown;
} h2o_stream_conn_t;

#endif /* HTTPD_STREAMING_H */
