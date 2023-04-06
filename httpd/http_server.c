// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "streaming/common.h"
#include "http_server.h"
#include "h2o.h"
#include "h2o/http1.h"

#include "h2o_utils.h"

#include "mqtt_websockets/c-rbuf/include/ringbuffer.h"

static h2o_globalconf_t config;
static h2o_context_t ctx;
static h2o_accept_ctx_t accept_ctx;

#define CONTENT_JSON_UTF8 H2O_STRLIT("application/json; charset=utf-8")
#define CONTENT_TEXT_UTF8 H2O_STRLIT("text/plain; charset=utf-8")
#define NBUF_INITIAL_SIZE_RESP (4096)
#define API_V1_PREFIX "/api/v1/"
#define HOST_SELECT_PREFIX "/host/"

#define HTTPD_CONFIG_SECTION "httpd"
#define HTTPD_ENABLED_DEFAULT false

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

#define H2O2STREAM_BUF_SIZE (1024 * 1024)

static void h2o_stream_conn_t_init(h2o_stream_conn_t *conn)
{
    memset(conn, 0, sizeof(*conn));
    conn->rx = rbuf_create(H2O2STREAM_BUF_SIZE);
    conn->tx = rbuf_create(H2O2STREAM_BUF_SIZE);

    pthread_mutex_init(&conn->rx_buf_lock, NULL);
    pthread_mutex_init(&conn->tx_buf_lock, NULL);
    pthread_cond_init(&conn->rx_buf_cond, NULL);
    // no need to check for NULL as rbuf_create uses mallocz internally
}

static void h2o_stream_conn_t_destroy(h2o_stream_conn_t *conn)
{
    rbuf_free(conn->rx);
    rbuf_free(conn->tx);

    freez(conn->url);
    freez(conn->user_agent);

    pthread_mutex_destroy(&conn->rx_buf_lock);
    pthread_mutex_destroy(&conn->tx_buf_lock);
    pthread_cond_destroy(&conn->rx_buf_cond);
}

static void on_accept(h2o_socket_t *listener, const char *err)
{
    h2o_socket_t *sock;

    if (err != NULL) {
        return;
    }

    if ((sock = h2o_evloop_socket_accept(listener)) == NULL)
        return;
    h2o_accept(&accept_ctx, sock);
}

static int create_listener(const char *ip, int port)
{
    struct sockaddr_in addr;
    int fd, reuseaddr_flag = 1;
    h2o_socket_t *sock;

    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = inet_addr(ip);
    addr.sin_port = htons(port);

    if ((fd = socket(AF_INET, SOCK_STREAM, 0)) == -1 ||
        setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &reuseaddr_flag, sizeof(reuseaddr_flag)) != 0 ||
        bind(fd, (struct sockaddr *)&addr, sizeof(addr)) != 0 || listen(fd, SOMAXCONN) != 0) {
        return -1;
    }

    sock = h2o_evloop_socket_create(ctx.loop, fd, H2O_SOCKET_FLAG_DONT_READ);
    h2o_socket_read_start(sock, on_accept);

    return 0;
}

static int ssl_init()
{
    if (!config_get_boolean(HTTPD_CONFIG_SECTION, "ssl", false))
        return 0;

    char default_fn[FILENAME_MAX + 1];

    snprintfz(default_fn,  FILENAME_MAX, "%s/ssl/key.pem",  netdata_configured_user_config_dir);
    const char *key_fn  = config_get(HTTPD_CONFIG_SECTION, "ssl key", default_fn);

    snprintfz(default_fn, FILENAME_MAX, "%s/ssl/cert.pem", netdata_configured_user_config_dir);
    const char *cert_fn = config_get(HTTPD_CONFIG_SECTION, "ssl certificate",  default_fn);

#if OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
    accept_ctx.ssl_ctx = SSL_CTX_new(SSLv23_server_method());
#else
    accept_ctx.ssl_ctx = SSL_CTX_new(TLS_server_method());
#endif

    SSL_CTX_set_options(accept_ctx.ssl_ctx, SSL_OP_NO_SSLv2);

    /* load certificate and private key */
    if (SSL_CTX_use_PrivateKey_file(accept_ctx.ssl_ctx, key_fn, SSL_FILETYPE_PEM) != 1) {
        error("Could not load server key from \"%s\"", key_fn);
        return -1;
    }
    if (SSL_CTX_use_certificate_file(accept_ctx.ssl_ctx, cert_fn, SSL_FILETYPE_PEM) != 1) {
        error("Could not load certificate from \"%s\"", cert_fn);
        return -1;
    }

    h2o_ssl_register_alpn_protocols(accept_ctx.ssl_ctx, h2o_http2_alpn_protocols);

    info("SSL support enabled");

    return 0;
}

// I did not find a way to do wildcard paths to make common handler for urls like:
// /api/v1/info
// /host/child/api/v1/info
// /host/uuid/api/v1/info
// ideally we could do something like "/*/api/v1/info" subscription
// so we do it "manually" here with uberhandler
static inline int _netdata_uberhandler(h2o_req_t *req, RRDHOST **host)
{
    if (!h2o_memis(req->method.base, req->method.len, H2O_STRLIT("GET")))
        return -1;

    static h2o_generator_t generator = { NULL, NULL };

    h2o_iovec_t norm_path = req->path_normalized;

    if (norm_path.len > strlen(HOST_SELECT_PREFIX) && !memcmp(norm_path.base, HOST_SELECT_PREFIX, strlen(HOST_SELECT_PREFIX))) {
        h2o_iovec_t host_id; // host_id can be either and UUID or a hostname of the child

        norm_path.base += strlen(HOST_SELECT_PREFIX);
        norm_path.len -= strlen(HOST_SELECT_PREFIX);

        host_id = norm_path;

        size_t end_loc = h2o_strstr(host_id.base, host_id.len, "/", 1);
        if (end_loc != SIZE_MAX) {
            host_id.len = end_loc;
            norm_path.base += end_loc;
            norm_path.len -= end_loc;
        }

        char *c_host_id = iovec_to_cstr(&host_id);
        *host = rrdhost_find_by_hostname(c_host_id);
        if (!*host)
            *host = rrdhost_find_by_guid(c_host_id);
        if (!*host) {
            req->res.status = HTTP_RESP_BAD_REQUEST;
            req->res.reason = "Wrong host id";
            h2o_send_inline(req, H2O_STRLIT("Host id provided was not found!\n"));
            freez(c_host_id);
            return 0;
        }
        freez(c_host_id);

        // we have to rewrite URL here in case this is not an api call
        // so that the subsequent file upload handler can send the correct
        // files to the client
        // if this is not an API call we will abort this handler later
        // and let the internal serve file handler of h2o care for things

        if (end_loc == SIZE_MAX) {
            req->path.len = 1;
            req->path_normalized.len = 1;
        } else {
            size_t offset = norm_path.base - req->path_normalized.base;
            req->path.len -= offset;
            req->path.base += offset;
            req->query_at -= offset;
            req->path_normalized.len -= offset;
            req->path_normalized.base += offset;
        }
    }

    // workaround for a dashboard bug which causes sometimes urls like
    // "//api/v1/info" to be caled instead of "/api/v1/info"
    if (norm_path.len > 2 &&
        norm_path.base[0] == '/' &&
        norm_path.base[1] == '/' ) {
            norm_path.base++;
            norm_path.len--;
    }

    size_t api_loc = h2o_strstr(norm_path.base, norm_path.len, H2O_STRLIT(API_V1_PREFIX));
    if (api_loc == SIZE_MAX)
        return 1;

    h2o_iovec_t api_command = norm_path;
    api_command.base += api_loc + strlen(API_V1_PREFIX);
    api_command.len -= api_loc + strlen(API_V1_PREFIX);

    if (!api_command.len)
        return 1;

    // this (emulating struct web_client) is a hack and will be removed
    // in future PRs but needs bigger changes in old http_api_v1
    // we need to make the web_client_api_request_v1 to be web server
    // agnostic and remove the old webservers dependency creep into the
    // individual response generators and thus remove the need to "emulate"
    // the old webserver calling this function here and in ACLK
    struct web_client w;
    w.response.data = buffer_create(NBUF_INITIAL_SIZE_RESP, NULL);
    w.response.header = buffer_create(NBUF_INITIAL_SIZE_RESP, NULL);
    w.decoded_query_string[0] = 0;
    w.acl = WEB_CLIENT_ACL_DASHBOARD;

    char *path_c_str = iovec_to_cstr(&api_command);
    char *path_unescaped = url_unescape(path_c_str);
    freez(path_c_str);

    IF_HAS_URL_PARAMS(req) {
        h2o_iovec_t query_params = URL_PARAMS_IOVEC_INIT_WITH_QUESTIONMARK(req);
        char *query_c_str = iovec_to_cstr(&query_params);
        char *query_unescaped = url_unescape(query_c_str);
        freez(query_c_str);
        strcpy(w.decoded_query_string, query_unescaped);
        freez(query_unescaped);
    }

    web_client_api_request_v1(*host, &w, path_unescaped);
    freez(path_unescaped);

    h2o_iovec_t body = buffer_to_h2o_iovec(w.response.data);

    // we move msg body to req->pool managed memory as it has to
    // live until whole response has been encrypted and sent
    // when req is finished memory will be freed with the pool
    void *managed = h2o_mem_alloc_shared(&req->pool, body.len, NULL);
    memcpy(managed, body.base, body.len);
    body.base = managed;

    req->res.status = HTTP_RESP_OK;
    req->res.reason = "OK";
    if (w.response.data->content_type == CT_APPLICATION_JSON)
        h2o_add_header(&req->pool, &req->res.headers, H2O_TOKEN_CONTENT_TYPE, NULL, CONTENT_JSON_UTF8);
    else
        h2o_add_header(&req->pool, &req->res.headers, H2O_TOKEN_CONTENT_TYPE, NULL, CONTENT_TEXT_UTF8);
    h2o_start_response(req, &generator);
    h2o_send(req, &body, 1, H2O_SEND_STATE_FINAL);

    buffer_free(w.response.data);
    buffer_free(w.response.header);

    return 0;
}

static int netdata_uberhandler(h2o_handler_t *self, h2o_req_t *req)
{
    UNUSED(self);
    RRDHOST *host = localhost;

    int ret = _netdata_uberhandler(req, &host);

    char host_uuid_str[UUID_STR_LEN];
    uuid_unparse_lower(host->host_uuid, host_uuid_str);

    if (!ret) {
        log_access("HTTPD OK method: " PRINTF_H2O_IOVEC_FMT
                   ", path: " PRINTF_H2O_IOVEC_FMT
                   ", as host: %s"
                   ", response: %d",
                   PRINTF_H2O_IOVEC(&req->method),
                   PRINTF_H2O_IOVEC(&req->input.path),
                   host == localhost ? "localhost" : host_uuid_str,
                   req->res.status);
    } else {
        log_access("HTTPD %d"
                   " method: " PRINTF_H2O_IOVEC_FMT
                   ", path: " PRINTF_H2O_IOVEC_FMT
                   ", forwarding to file handler as path: " PRINTF_H2O_IOVEC_FMT,
                   ret,
                   PRINTF_H2O_IOVEC(&req->method),
                   PRINTF_H2O_IOVEC(&req->input.path),
                   PRINTF_H2O_IOVEC(&req->path));
    }

    return ret;
}

static int hdl_netdata_conf(h2o_handler_t *self, h2o_req_t *req)
{
    UNUSED(self);
    if (!h2o_memis(req->method.base, req->method.len, H2O_STRLIT("GET")))
        return -1;

    BUFFER *buf = buffer_create(NBUF_INITIAL_SIZE_RESP, NULL);
    config_generate(buf, 0);

    void *managed = h2o_mem_alloc_shared(&req->pool, buf->len, NULL);
    memcpy(managed, buf->buffer, buf->len);

    req->res.status = HTTP_RESP_OK;
    req->res.reason = "OK";
    h2o_add_header(&req->pool, &req->res.headers, H2O_TOKEN_CONTENT_TYPE, NULL, CONTENT_TEXT_UTF8);
    h2o_send_inline(req, managed, buf->len);
    buffer_free(buf);

    return 0;
}

void stream_process(h2o_stream_conn_t *conn, int initial);

static void stream_on_close(h2o_stream_conn_t *conn)
{
    if (conn->sock != NULL)
        h2o_socket_close(conn->sock);

    pthread_mutex_lock(&conn->rx_buf_lock);
    conn->shutdown = 1;
    pthread_cond_broadcast(&conn->rx_buf_cond);
    pthread_mutex_unlock(&conn->rx_buf_lock);

    h2o_stream_conn_t_destroy(conn);
    freez(conn);
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

void on_write_complete(h2o_socket_t *sock, const char *err)
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

#define PARSE_DONE 1
#define PARSE_ERROR -1
#define GIMME_MORE_OF_DEM_SWEET_BYTEZ 0

#define STREAM_METHOD "STREAM "
#define HTTP_1_1 " HTTP/1.1"
#define HTTP_HDR_END "\r\n\r\n"
#define USER_AGENT "User-Agent: "

#define NEED_MIN_BYTES(buf, bytes)       \
if (rbuf_bytes_available(buf) < bytes)   \
    return GIMME_MORE_OF_DEM_SWEET_BYTEZ;

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
                    error_report("The initial \"STREAM [URL] HTTP/1.1\" over max of %d", MAX_LEN_STREAM_HELLO);
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
                    error_report("The initial \"STREAM [URL] HTTP/1.1\" over max of %d", MAX_LEN_STREAM_HELLO);
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
                    error_report("The initial \"STREAM [URL] HTTP/1.1\" over max of %d", MAX_LEN_STREAM_HELLO);
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
                    error_report("The initial \"STREAM [URL] HTTP/1.1\" over max of %d", MAX_LEN_STREAM_HELLO);
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
            info("RX buffer full, temporarily stopping the reading until consumer (streaming thread) reads some data");
            pthread_cond_broadcast(&conn->rx_buf_cond);
            pthread_mutex_unlock(&conn->rx_buf_lock);
            h2o_socket_read_stop(conn->sock);
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
            int rc = process_STREAM_X_HTTP_1_1(&conn->parse_state, conn->rx, &conn->url, &conn->user_agent);
            if (rc == PARSE_ERROR) {
                error_report("error parsing the STREAM hello");
                break;
            }
            if (rc != PARSE_DONE)
                break;
            conn->state = STREAM_X_HTTP_1_1_DONE;
            /* FALLTHROUGH */
        case STREAM_X_HTTP_1_1_DONE:
            struct web_client w;
            memset(&w, 0, sizeof(w));
            w.response.data = buffer_create(1024, NULL);

            // get client ip from the conn->sock
            struct sockaddr client;
            socklen_t len = h2o_socket_getpeername(conn->sock, &client);
            char peername[NI_MAXHOST];
            size_t peername_len = h2o_socket_getnumerichost(&client, len, peername);
            memcpy(w.client_ip, peername, peername_len);
            w.client_ip[peername_len] = 0;
            w.user_agent = conn->user_agent;

            rrdpush_receiver_thread_spawn(&w, conn->url, conn);
            // http_code returned is ignored as there is nobody to get it after HTTP upgrade
            // so it lost any sense with h2o streaming mode
            freez(conn->url);
            buffer_free(w.response.data);
            conn->state = STREAM_ACTIVE;
            /* FALLTHROUGH */
        case STREAM_ACTIVE:
            break;
        default:
            error_report("Unknown conn->state");
    }
}

static void stream_on_complete(void *user_data, h2o_socket_t *sock, size_t reqsize)
{
    h2o_stream_conn_t *conn = user_data;

    /* close the connection on error */
    if (sock == NULL) {
// can call connection close callback here  (*conn->cb)(conn, NULL);
        return;
    }

    conn->sock = sock;
    sock->data = conn;

    h2o_buffer_consume(&sock->input, reqsize);
    stream_process(conn, 1);
}

static inline int is_streaming_handshake(h2o_req_t *req)
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

int h2o_stream_write(void *ctx, const char *data, size_t data_len)
{
    h2o_stream_conn_t *conn = (h2o_stream_conn_t *)ctx;

    pthread_mutex_lock(&conn->tx_buf_lock);
    size_t avail = rbuf_bytes_free(conn->tx);
    avail = MIN(avail, data_len);
    rbuf_push(conn->tx, data, avail);
    pthread_mutex_unlock(&conn->tx_buf_lock);
    return avail;
}

size_t h2o_stream_read(void *ctx, char *buf, size_t read_bytes)
{
    int ret;
    h2o_stream_conn_t *conn = (h2o_stream_conn_t *)ctx;

    pthread_mutex_lock(&conn->rx_buf_lock);
    size_t avail = rbuf_bytes_available(conn->rx);

    if (!avail) {
        if (conn->shutdown)
            return -1;
        pthread_cond_wait(&conn->rx_buf_cond, &conn->rx_buf_lock);
        if (conn->shutdown)
            return -1;
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

static int hdl_stream(h2o_handler_t *self, h2o_req_t *req)
{
    UNUSED(self);
    h2o_stream_conn_t *conn = mallocz(sizeof(*conn));
    h2o_stream_conn_t_init(conn);

    if (is_streaming_handshake(req))
        return 1;

    /* build response */
    req->res.status = 101;
    req->res.reason = "Switching Protocols";
    h2o_add_header(&req->pool, &req->res.headers, H2O_TOKEN_UPGRADE, NULL, H2O_STRLIT(NETDATA_STREAM_PROTO_NAME));

//  TODO we should consider adding some nonce header here
//    h2o_add_header_by_str(&req->pool, &req->res.headers, H2O_STRLIT("whatever reply"), 0, NULL, accept_key,
//                          strlen(accept_key));

    h2o_http1_upgrade(req, NULL, 0, stream_on_complete, conn);

    return 0;
}

#define POLL_INTERVAL 100

void *httpd_main(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    h2o_pathconf_t *pathconf;
    h2o_hostconf_t *hostconf;

    netdata_thread_disable_cancelability();

    const char *bind_addr = config_get(HTTPD_CONFIG_SECTION, "bind to", "127.0.0.1");
    int bind_port = config_get_number(HTTPD_CONFIG_SECTION, "port", 19998);

    h2o_config_init(&config);
    hostconf = h2o_config_register_host(&config, h2o_iovec_init(H2O_STRLIT("default")), bind_port);

    pathconf = h2o_config_register_path(hostconf, "/netdata.conf", 0);
    h2o_handler_t *handler = h2o_create_handler(pathconf, sizeof(*handler));
    handler->on_req = hdl_netdata_conf;

    pathconf = h2o_config_register_path(hostconf, NETDATA_STREAM_URL, 0);
    handler = h2o_create_handler(pathconf, sizeof(*handler));
    handler->on_req = hdl_stream;

    pathconf = h2o_config_register_path(hostconf, "/", 0);
    handler = h2o_create_handler(pathconf, sizeof(*handler));
    handler->on_req = netdata_uberhandler;
    h2o_file_register(pathconf, netdata_configured_web_dir, NULL, NULL, H2O_FILE_FLAG_SEND_COMPRESSED);

    h2o_context_init(&ctx, h2o_evloop_create(), &config);

    if(ssl_init()) {
        error_report("SSL was requested but could not be properly initialized. Aborting.");
        return NULL;
    }

    accept_ctx.ctx = &ctx;
    accept_ctx.hosts = config.hosts;

    if (create_listener(bind_addr, bind_port) != 0) {
        error("failed to create listener %s:%d", bind_addr, bind_port);
        return NULL;
    }

    usec_t last_wpoll = now_monotonic_usec();
    while (service_running(SERVICE_HTTPD)) {
        int rc = h2o_evloop_run(ctx.loop, POLL_INTERVAL);
        if (rc < 0 && errno != EINTR) {
            error("h2o_evloop_run returned (%d) with errno other than EINTR. Aborting", rc);
            break;
        }
        usec_t now = now_monotonic_usec();
        if (now - last_wpoll > POLL_INTERVAL * 1000) {
            last_wpoll = now;
            //h2o_context_request_wakeup(&ctx);
        }
    } 

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
    return NULL;
}

int httpd_is_enabled() {
    return config_get_boolean(HTTPD_CONFIG_SECTION, "enabled", HTTPD_ENABLED_DEFAULT);
}
