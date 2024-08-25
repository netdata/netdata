// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef HTTPD_STREAMING_H
#define HTTPD_STREAMING_H

#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wunused-parameter"
#pragma GCC diagnostic ignored "-Wunused-but-set-variable"
#pragma GCC diagnostic ignored "-Wtype-limits"
#include "h2o.h"
#pragma GCC diagnostic pop

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

// h2o_stream_conn_t related functions
void h2o_stream_conn_t_init(h2o_stream_conn_t *conn);
void h2o_stream_conn_t_destroy(h2o_stream_conn_t *conn);

// streaming upgrade related functions
int is_streaming_handshake(h2o_req_t *req);
void stream_on_complete(void *user_data, h2o_socket_t *sock, size_t reqsize);

// read and write functions to be used by streaming parser
int h2o_stream_write(void *ctx, const char *data, size_t data_len);
size_t h2o_stream_read(void *ctx, char *buf, size_t read_bytes);

// call this periodically to check if there are any pending write requests
void h2o_stream_check_pending_write_reqs(void);

#endif /* HTTPD_STREAMING_H */
