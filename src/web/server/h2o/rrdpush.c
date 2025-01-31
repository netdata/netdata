// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "streaming.h"
#include "connlist.h"
#include "h2o_utils.h"
#include "streaming/h2o-common.h"

static int pending_write_reqs = 0;

#define H2O2STREAM_BUF_SIZE (1024 * 1024)

// h2o_stream_conn_t related functions
void h2o_stream_conn_t_init(h2o_stream_conn_t *conn)
{
    memset(conn, 0, sizeof(*conn));
    conn->rx = rbuf_create(H2O2STREAM_BUF_SIZE);
    conn->tx = rbuf_create(H2O2STREAM_BUF_SIZE);

    pthread_mutex_init(&conn->rx_buf_lock, NULL);
    pthread_mutex_init(&conn->tx_buf_lock, NULL);
    pthread_cond_init(&conn->rx_buf_cond, NULL);
    // no need to check for NULL as rbuf_create uses mallocz internally
}

void h2o_stream_conn_t_destroy(h2o_stream_conn_t *conn)
{
    rbuf_free(conn->rx);
    rbuf_free(conn->tx);

    freez(conn->url);
    freez(conn->user_agent);

    pthread_mutex_destroy(&conn->rx_buf_lock);
    pthread_mutex_destroy(&conn->tx_buf_lock);
    pthread_cond_destroy(&conn->rx_buf_cond);
}

// streaming upgrade related functions
int is_streaming_handshake(h2o_req_t *req)
{
    /* method */
    if (!h2o_memis(req->input.method.base, req->input.method.len, H2O_STRLIT("GET")))
        return 1;

    if (!h2o_memis(req->path_normalized.base, req->path_normalized.len, H2O_STRLIT(NETDATA_STREAM_URL))) {
        return 1;
    }

    /* upgrade header */
    if (req->upgrade.base == NULL || !h2o_lcstris(req->upgrade.base, req->upgrade.len, H2O_STRLIT(NETDATA_STREAM_PROTO_NAME)))
        return 1;

    // TODO consider adding some key in form of random number
    // to prevent caching on route especially if TLS is not used
    // e.g. client sends random number
    // server replies with it xored

    return 0;
}

static void stream_on_close(h2o_stream_conn_t *conn);
void stream_process(h2o_stream_conn_t *conn, int initial);

void stream_on_complete(void *user_data, h2o_socket_t *sock, size_t reqsize)
{
    h2o_stream_conn_t *conn = user_data;

    /* close the connection on error */
    if (sock == NULL) {
        stream_on_close(conn);
        return;
    }

    conn->sock = sock;
    sock->data = conn;

    conn_list_insert(&conn_list, conn);

    h2o_buffer_consume(&sock->input, reqsize);
    stream_process(conn, 1);
}

// handling of active streams
static void stream_on_close(h2o_stream_conn_t *conn)
{
    if (conn->sock != NULL)
        h2o_socket_close(conn->sock);

    conn_list_remove_conn(&conn_list, conn);

    pthread_mutex_lock(&conn->rx_buf_lock);
    conn->shutdown = 1;
    pthread_cond_broadcast(&conn->rx_buf_cond);
    pthread_mutex_unlock(&conn->rx_buf_lock);

    h2o_stream_conn_t_destroy(conn);
    freez(conn);
}

static void on_write_complete(h2o_socket_t *sock, const char *err)
{
    h2o_stream_conn_t *conn = sock->data;

    if (err != NULL) {
        stream_on_close(conn);
        error_report("Streaming connection error \"%s\"", err);
        return;
    }

    pthread_mutex_lock(&conn->tx_buf_lock);

    rbuf_bump_tail(conn->tx, conn->tx_buf.len);

    conn->tx_buf.base = NULL;
    conn->tx_buf.len = 0;

    pthread_mutex_unlock(&conn->tx_buf_lock);

    stream_process(conn, 0);
}

static void stream_on_recv(h2o_socket_t *sock, const char *err)
{
    h2o_stream_conn_t *conn = sock->data;

    if (err != NULL) {
        stream_on_close(conn);
        error_report("Streaming connection error \"%s\"", err);
        return;
    }
    stream_process(conn, 0);
}

#define PARSE_DONE 1
#define PARSE_ERROR -1
#define GIMME_MORE_OF_DEM_SWEET_BYTEZ 0

#define STREAM_METHOD "STREAM "
#define USER_AGENT "User-Agent: "

#define NEED_MIN_BYTES(buf, bytes) do {      \
    if(rbuf_bytes_available(buf) < bytes)    \
        return GIMME_MORE_OF_DEM_SWEET_BYTEZ;\
} while(0)

// TODO check in streaming code this is probably defined somewhere already
#define MAX_LEN_STREAM_HELLO (1024*2)

static int process_STREAM_X_HTTP_1_1(http_stream_parse_state_t *parser_state, rbuf_t buf, char **url, char **user_agent)
{
    int idx;
    switch(*parser_state) {
        case HTTP_STREAM:
            NEED_MIN_BYTES(buf, strlen(STREAM_METHOD));
            if (rbuf_memcmp_n(buf, H2O_STRLIT(STREAM_METHOD))) {
                error_report("Expected \"%s\"", STREAM_METHOD);
                return PARSE_ERROR;
            }
            rbuf_bump_tail(buf, strlen(STREAM_METHOD));
            *parser_state = HTTP_URL;
            /* FALLTHROUGH */
        case HTTP_URL:
            if (!rbuf_find_bytes(buf, " ", 1, &idx)) {
                if (rbuf_bytes_available(buf) >= MAX_LEN_STREAM_HELLO) {
                    error_report("The initial \"STREAM [URL]" HTTP_1_1 "\" over max of %d", MAX_LEN_STREAM_HELLO);
                    return PARSE_ERROR;
                }
            }
            *url = mallocz(idx + 1);
            rbuf_pop(buf, *url, idx);
            (*url)[idx] = 0;

            *parser_state = HTTP_PROTO;
            /* FALLTHROUGH */
        case HTTP_PROTO:
            NEED_MIN_BYTES(buf, strlen(HTTP_1_1));
            if (rbuf_memcmp_n(buf, H2O_STRLIT(HTTP_1_1))) {
                error_report("Expected \"%s\"", HTTP_1_1);
                return PARSE_ERROR;
            }
            rbuf_bump_tail(buf, strlen(HTTP_1_1));
            *parser_state = HTTP_USER_AGENT_KEY;
            /* FALLTHROUGH */
        case HTTP_USER_AGENT_KEY:
            // and OF COURSE EVERYTHING is passed in URL except
            // for user agent which we need and is passed as HTTP header
            // not worth writing a parser for this so we manually extract
            // just the single header we need and skip everything else
            if (!rbuf_find_bytes(buf, USER_AGENT, strlen(USER_AGENT), &idx)) {
                if (rbuf_bytes_available(buf) >= (size_t)(rbuf_get_capacity(buf) * 0.9)) {
                    error_report("The initial \"STREAM [URL]" HTTP_1_1 "\" over max of %d", MAX_LEN_STREAM_HELLO);
                    return PARSE_ERROR;
                }
                return GIMME_MORE_OF_DEM_SWEET_BYTEZ;
            }
            rbuf_bump_tail(buf, idx + strlen(USER_AGENT));
            *parser_state = HTTP_USER_AGENT_VALUE;
            /* FALLTHROUGH */
        case HTTP_USER_AGENT_VALUE:
            if (!rbuf_find_bytes(buf, "\r\n", 2, &idx)) {
                if (rbuf_bytes_available(buf) >= (size_t)(rbuf_get_capacity(buf) * 0.9)) {
                    error_report("The initial \"STREAM [URL]" HTTP_1_1 "\" over max of %d", MAX_LEN_STREAM_HELLO);
                    return PARSE_ERROR;
                }
                return GIMME_MORE_OF_DEM_SWEET_BYTEZ;
            }

            *user_agent = mallocz(idx + 1);
            rbuf_pop(buf, *user_agent, idx);
            (*user_agent)[idx] = 0;

            *parser_state = HTTP_HDR;
            /* FALLTHROUGH */
        case HTTP_HDR:
            if (!rbuf_find_bytes(buf, HTTP_HDR_END, strlen(HTTP_HDR_END), &idx)) {
                if (rbuf_bytes_available(buf) >= (size_t)(rbuf_get_capacity(buf) * 0.9)) {
                    error_report("The initial \"STREAM [URL]" HTTP_1_1 "\" over max of %d", MAX_LEN_STREAM_HELLO);
                    return PARSE_ERROR;
                }
                return GIMME_MORE_OF_DEM_SWEET_BYTEZ;
            }
            rbuf_bump_tail(buf, idx + strlen(HTTP_HDR_END));

            *parser_state = HTTP_DONE;
            return PARSE_DONE;
        case HTTP_DONE:
            error_report("Parsing is done. No need to call again.");
            return PARSE_DONE;
        default:
            error_report("Unknown parser state %d", (int)*parser_state);
            return PARSE_ERROR;
    }
}

#define SINGLE_WRITE_MAX (1024)

void stream_process(h2o_stream_conn_t *conn, int initial)
{
    int rc;
    struct web_client w;

    pthread_mutex_lock(&conn->tx_buf_lock);
    if (h2o_socket_is_writing(conn->sock) || rbuf_bytes_available(conn->tx)) {
        if (rbuf_bytes_available(conn->tx) && !conn->tx_buf.base) {
            conn->tx_buf.base = rbuf_get_linear_read_range(conn->tx, &conn->tx_buf.len);
            if (conn->tx_buf.base) {
                conn->tx_buf.len = MIN(conn->tx_buf.len, SINGLE_WRITE_MAX);
                h2o_socket_write(conn->sock, &conn->tx_buf, 1, on_write_complete);
            }
        }
    }
    pthread_mutex_unlock(&conn->tx_buf_lock);

    if (initial)
        h2o_socket_read_start(conn->sock, stream_on_recv);

    if (conn->sock->input->size) {
        size_t insert_max;
        pthread_mutex_lock(&conn->rx_buf_lock);
        char *insert_loc = rbuf_get_linear_insert_range(conn->rx, &insert_max);
        if (insert_loc == NULL) {
            pthread_cond_broadcast(&conn->rx_buf_cond);
            pthread_mutex_unlock(&conn->rx_buf_lock);
            return;
        }
        insert_max = MIN(insert_max, conn->sock->input->size);
        memcpy(insert_loc, conn->sock->input->bytes, insert_max);
        rbuf_bump_head(conn->rx, insert_max);

        h2o_buffer_consume(&conn->sock->input, insert_max);

        pthread_cond_broadcast(&conn->rx_buf_cond);
        pthread_mutex_unlock(&conn->rx_buf_lock);
    }

    switch (conn->state) {
        case STREAM_X_HTTP_1_1:
            // no conn->rx lock here as at this point we are still single threaded
            // until we call rrdpush_receiver_thread_spawn() later down
            rc = process_STREAM_X_HTTP_1_1(&conn->parse_state, conn->rx, &conn->url, &conn->user_agent);
            if (rc == PARSE_ERROR) {
                error_report("error parsing the STREAM hello");
                break;
            }
            if (rc != PARSE_DONE)
                break;
            conn->state = STREAM_X_HTTP_1_1_DONE;
            /* FALLTHROUGH */
        case STREAM_X_HTTP_1_1_DONE:
            memset(&w, 0, sizeof(w));
            w.response.data = buffer_create(1024, NULL);

            // get client ip from the conn->sock
            struct sockaddr client;
            socklen_t len = h2o_socket_getpeername(conn->sock, &client);
            char peername[NI_MAXHOST];
            size_t peername_len = h2o_socket_getnumerichost(&client, len, peername);
            size_t cpy_len = sizeof(w.client_ip) < peername_len ? sizeof(w.client_ip) : peername_len;
            memcpy(w.client_ip, peername, cpy_len);
            w.client_ip[cpy_len - 1] = 0;
            w.user_agent = conn->user_agent;

            rc = stream_receiver_accept_connection(&w, conn->url, conn);
            if (rc != HTTP_RESP_OK) {
                error_report("HTTPD Failed to spawn the receiver thread %d", rc);
                conn->state = STREAM_CLOSE;
                stream_on_close(conn);
            } else {
                conn->state = STREAM_ACTIVE;
            }
            buffer_free(w.response.data);
            /* FALLTHROUGH */
        case STREAM_ACTIVE:
            break;
        default:
            error_report("Unknown conn->state");
    }
}

// read and write functions to be used by streaming parser
int h2o_stream_write(void *ctx, const char *data, size_t data_len)
{
    h2o_stream_conn_t *conn = (h2o_stream_conn_t *)ctx;

    pthread_mutex_lock(&conn->tx_buf_lock);
    size_t avail = rbuf_bytes_free(conn->tx);
    avail = MIN(avail, data_len);
    rbuf_push(conn->tx, data, avail);
    pthread_mutex_unlock(&conn->tx_buf_lock);
    __atomic_add_fetch(&pending_write_reqs, 1, __ATOMIC_SEQ_CST);
    return avail;
}

size_t h2o_stream_read(void *ctx, char *buf, size_t read_bytes)
{
    int ret;
    h2o_stream_conn_t *conn = (h2o_stream_conn_t *)ctx;

    pthread_mutex_lock(&conn->rx_buf_lock);
    size_t avail = rbuf_bytes_available(conn->rx);

    if (!avail) {
        if (conn->shutdown) {
            pthread_mutex_unlock(&conn->rx_buf_lock);
            return -1;
        }
        pthread_cond_wait(&conn->rx_buf_cond, &conn->rx_buf_lock);
        if (conn->shutdown) {
            pthread_mutex_unlock(&conn->rx_buf_lock);
            return -1;
        }
        avail = rbuf_bytes_available(conn->rx);
        if (!avail) {
            pthread_mutex_unlock(&conn->rx_buf_lock);
            return 0;
        }
    }

    avail = MIN(avail, read_bytes);

    ret = rbuf_pop(conn->rx, buf, avail);
    pthread_mutex_unlock(&conn->rx_buf_lock);

    return ret;
}

// periodic check for pending write requests
void check_tx_buf(h2o_stream_conn_t *conn)
{
    pthread_mutex_lock(&conn->tx_buf_lock);
    if (rbuf_bytes_available(conn->tx)) {
        pthread_mutex_unlock(&conn->tx_buf_lock);
        stream_process(conn, 0);
    } else
        pthread_mutex_unlock(&conn->tx_buf_lock);
}

void h2o_stream_check_pending_write_reqs(void)
{
    int _write_reqs = __atomic_exchange_n(&pending_write_reqs, 0, __ATOMIC_SEQ_CST);
    if (_write_reqs > 0)
        conn_list_iter_all(&conn_list, check_tx_buf);
}
