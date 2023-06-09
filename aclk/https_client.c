// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#include "https_client.h"

#include "mqtt_websockets/c-rbuf/include/ringbuffer.h"

#include "aclk_util.h"

#include "daemon/global_statistics.h"

enum http_parse_state {
    HTTP_PARSE_INITIAL = 0,
    HTTP_PARSE_HEADERS,
    HTTP_PARSE_CONTENT
};

static const char *http_req_type_to_str(http_req_type_t req) {
    switch (req) {
        case HTTP_REQ_GET:
            return "GET";
        case HTTP_REQ_POST:
            return "POST";
        case HTTP_REQ_CONNECT:
            return "CONNECT";
        default:
            return "unknown";
    }
}

typedef struct {
    enum http_parse_state state;
    int content_length;
    int http_code;
} http_parse_ctx;

#define HTTP_PARSE_CTX_INITIALIZER { .state = HTTP_PARSE_INITIAL, .content_length = -1, .http_code = 0 }
static inline void http_parse_ctx_clear(http_parse_ctx *ctx) {
    ctx->state = HTTP_PARSE_INITIAL;
    ctx->content_length = -1;
    ctx->http_code = 0;
}

#define POLL_TO_MS 100

#define NEED_MORE_DATA  0
#define PARSE_SUCCESS   1
#define PARSE_ERROR    -1
#define HTTP_LINE_TERM "\x0D\x0A"
#define RESP_PROTO "HTTP/1.1 "
#define HTTP_KEYVAL_SEPARATOR ": "
#define HTTP_HDR_BUFFER_SIZE 256
#define PORT_STR_MAX_BYTES 12

static void process_http_hdr(http_parse_ctx *parse_ctx, const char *key, const char *val)
{
    // currently we care only about content-length
    // but in future the way this is written
    // it can be extended
    if (!strcmp("content-length", key)) {
        parse_ctx->content_length = atoi(val);
    }
}

static int parse_http_hdr(rbuf_t buf, http_parse_ctx *parse_ctx)
{
    int idx, idx_end;
    char buf_key[HTTP_HDR_BUFFER_SIZE];
    char buf_val[HTTP_HDR_BUFFER_SIZE];
    char *ptr = buf_key;
    if (!rbuf_find_bytes(buf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM), &idx_end)) {
        error("CRLF expected");
        return 1;
    }

    char *separator = rbuf_find_bytes(buf, HTTP_KEYVAL_SEPARATOR, strlen(HTTP_KEYVAL_SEPARATOR), &idx);
    if (!separator) {
        error("Missing Key/Value separator");
        return 1;
    }
    if (idx >= HTTP_HDR_BUFFER_SIZE) {
        error("Key name is too long");
        return 1;
    }

    rbuf_pop(buf, buf_key, idx);
    buf_key[idx] = 0;

    rbuf_bump_tail(buf, strlen(HTTP_KEYVAL_SEPARATOR));
    idx_end -= strlen(HTTP_KEYVAL_SEPARATOR) + idx;
    if (idx_end >= HTTP_HDR_BUFFER_SIZE) {
        error("Value of key \"%s\" too long", buf_key);
        return 1;
    }

    rbuf_pop(buf, buf_val, idx_end);
    buf_val[idx_end] = 0;

    for (ptr = buf_key; *ptr; ptr++)
        *ptr = tolower(*ptr);

    process_http_hdr(parse_ctx, buf_key, buf_val);

    return 0;
}

static int parse_http_response(rbuf_t buf, http_parse_ctx *parse_ctx)
{
    int idx;
    char rc[4];

    do {
        if (parse_ctx->state != HTTP_PARSE_CONTENT && !rbuf_find_bytes(buf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM), &idx))
            return NEED_MORE_DATA;
        switch (parse_ctx->state) {
            case HTTP_PARSE_INITIAL:
                if (rbuf_memcmp_n(buf, RESP_PROTO, strlen(RESP_PROTO))) {
                    error("Expected response to start with \"%s\"", RESP_PROTO);
                    return PARSE_ERROR;
                }
                rbuf_bump_tail(buf, strlen(RESP_PROTO));
                if (rbuf_pop(buf, rc, 4) != 4) {
                    error("Expected HTTP status code");
                    return PARSE_ERROR;
                }
                if (rc[3] != ' ') {
                    error("Expected space after HTTP return code");
                    return PARSE_ERROR;
                }
                rc[3] = 0;
                parse_ctx->http_code = atoi(rc);
                if (parse_ctx->http_code < 100 || parse_ctx->http_code >= 600) {
                    error("HTTP code not in range 100 to 599");
                    return PARSE_ERROR;
                }

                rbuf_find_bytes(buf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM), &idx);

                rbuf_bump_tail(buf, idx + strlen(HTTP_LINE_TERM));

                parse_ctx->state = HTTP_PARSE_HEADERS;
                break;
            case HTTP_PARSE_HEADERS:
                if (!idx) {
                    parse_ctx->state = HTTP_PARSE_CONTENT;
                    rbuf_bump_tail(buf, strlen(HTTP_LINE_TERM));
                    break;
                }
                if (parse_http_hdr(buf, parse_ctx))
                    return PARSE_ERROR;
                rbuf_find_bytes(buf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM), &idx);
                rbuf_bump_tail(buf, idx + strlen(HTTP_LINE_TERM));
                break;
            case HTTP_PARSE_CONTENT:
                // replies like CONNECT etc. do not have content
                if (parse_ctx->content_length < 0)
                    return PARSE_SUCCESS;

                if (rbuf_bytes_available(buf) >= (size_t)parse_ctx->content_length)
                    return PARSE_SUCCESS;
                return NEED_MORE_DATA;
        }
    } while(1);
}

typedef struct https_req_ctx {
    https_req_t *request;

    int sock;
    rbuf_t buf_rx;

    struct pollfd poll_fd;

    SSL_CTX *ssl_ctx;
    SSL *ssl;

    size_t written;

    int self_signed_allowed;

    http_parse_ctx parse_ctx;

    time_t req_start_time;
} https_req_ctx_t;

static int https_req_check_timedout(https_req_ctx_t *ctx) {
    if (now_realtime_sec() > ctx->req_start_time + ctx->request->timeout_s) {
        error("request timed out");
        return 1;
    }
    return 0;
}

static char *_ssl_err_tos(int err)
{
    switch(err){
        case SSL_ERROR_SSL:
            return "SSL_ERROR_SSL";
        case SSL_ERROR_WANT_READ:
            return "SSL_ERROR_WANT_READ";
        case SSL_ERROR_WANT_WRITE:
            return "SSL_ERROR_WANT_WRITE";
        case SSL_ERROR_NONE:
            return "SSL_ERROR_NONE";
        case SSL_ERROR_ZERO_RETURN:
            return "SSL_ERROR_ZERO_RETURN";
        case SSL_ERROR_WANT_CONNECT:
            return "SSL_ERROR_WANT_CONNECT";
        case SSL_ERROR_WANT_ACCEPT:
            return "SSL_ERROR_WANT_ACCEPT";
    }
    return "Unknown!!!";
}

static int socket_write_all(https_req_ctx_t *ctx, char *data, size_t data_len) {
    ctx->written = 0;
    ctx->poll_fd.events = POLLOUT;

    do {
        int ret = poll(&ctx->poll_fd, 1, POLL_TO_MS);
        if (ret < 0) {
            error("poll error");
            return 1;
        }
        if (ret == 0) {
            if (https_req_check_timedout(ctx)) {
                error("Poll timed out");
                return 2;
            }
            continue;
        }

        ret = write(ctx->sock, &data[ctx->written], data_len - ctx->written);
        if (ret > 0) {
            ctx->written += ret;
        } else if (errno != EAGAIN && errno != EWOULDBLOCK) {
            error("Error writing to socket");
            return 3;
        }
    } while (ctx->written < data_len);

    return 0;
}

static int ssl_write_all(https_req_ctx_t *ctx, char *data, size_t data_len) {
    ctx->written = 0;
    ctx->poll_fd.events |= POLLOUT;

    do {
        int ret = poll(&ctx->poll_fd, 1, POLL_TO_MS);
        if (ret < 0) {
            error("poll error");
            return 1;
        }
        if (ret == 0) {
            if (https_req_check_timedout(ctx)) {
                error("Poll timed out");
                return 2;
            }
            continue;
        }
        ctx->poll_fd.events = 0;

        ret = SSL_write(ctx->ssl, &data[ctx->written], data_len - ctx->written);
        if (ret > 0) {
            ctx->written += ret;
        } else {
            ret = SSL_get_error(ctx->ssl, ret);
            switch (ret) {
                case SSL_ERROR_WANT_READ:
                    ctx->poll_fd.events |= POLLIN;
                    break;
                case SSL_ERROR_WANT_WRITE:
                    ctx->poll_fd.events |= POLLOUT;
                    break;
                default:
                    error("SSL_write Err: %s", _ssl_err_tos(ret));
                    return 3;
            }
        }
    } while (ctx->written < data_len);

    return 0;
}

static inline int https_client_write_all(https_req_ctx_t *ctx, char *data, size_t data_len) {
    if (ctx->ssl_ctx)
        return ssl_write_all(ctx, data, data_len);
    return socket_write_all(ctx, data, data_len);
}

static int read_parse_response(https_req_ctx_t *ctx) {
    int ret;
    char *ptr;
    size_t size;

    ctx->poll_fd.events = POLLIN;
    do {
        ret = poll(&ctx->poll_fd, 1, POLL_TO_MS);
        if (ret < 0) {
            error("poll error");
            return 1;
        }
        if (ret == 0) {
            if (https_req_check_timedout(ctx)) {
                error("Poll timed out");
                return 2;
            }
            if (!ctx->ssl_ctx)
                continue;
        }
        ctx->poll_fd.events = 0;

        ptr = rbuf_get_linear_insert_range(ctx->buf_rx, &size);

        if (ctx->ssl_ctx)
            ret = SSL_read(ctx->ssl, ptr, size);
        else
            ret = read(ctx->sock, ptr, size);

        if (ret > 0) {
            rbuf_bump_head(ctx->buf_rx, ret);
        } else {
            if (ctx->ssl_ctx) {
                ret = SSL_get_error(ctx->ssl, ret);
                switch (ret) {
                    case SSL_ERROR_WANT_READ:
                        ctx->poll_fd.events |= POLLIN;
                        break;
                    case SSL_ERROR_WANT_WRITE:
                        ctx->poll_fd.events |= POLLOUT;
                        break;
                    default:
                        error("SSL_read Err: %s", _ssl_err_tos(ret));
                        return 3;
                }
            } else {
                if (errno != EAGAIN && errno != EWOULDBLOCK) {
                    error("write error");
                    return 3;
                }
                ctx->poll_fd.events |= POLLIN;
            }
        }
    } while (!(ret = parse_http_response(ctx->buf_rx, &ctx->parse_ctx)));

    if (ret != PARSE_SUCCESS) {
        error("Error parsing HTTP response");
        return 1;
    }

    return 0;
}

#define TX_BUFFER_SIZE 8192
#define RX_BUFFER_SIZE (TX_BUFFER_SIZE*2)
static int handle_http_request(https_req_ctx_t *ctx) {
    BUFFER *hdr = buffer_create(TX_BUFFER_SIZE, &netdata_buffers_statistics.buffers_aclk);
    int rc = 0;

    http_parse_ctx_clear(&ctx->parse_ctx);

    // Prepare data to send
    switch (ctx->request->request_type) {
        case HTTP_REQ_CONNECT:
            buffer_strcat(hdr, "CONNECT ");
            break;
        case HTTP_REQ_GET:
            buffer_strcat(hdr, "GET ");
            break;
        case HTTP_REQ_POST:
            buffer_strcat(hdr, "POST ");
            break;
        default:
            error("Unknown HTTPS request type!");
            rc = 1;
            goto err_exit;
    }

    if (ctx->request->request_type == HTTP_REQ_CONNECT) {
        buffer_strcat(hdr, ctx->request->host);
        buffer_sprintf(hdr, ":%d", ctx->request->port);
    } else {
        buffer_strcat(hdr, ctx->request->url);
    }

    buffer_strcat(hdr, " HTTP/1.1\x0D\x0A");

    //TODO Headers!
    if (ctx->request->request_type != HTTP_REQ_CONNECT) {
        buffer_sprintf(hdr, "Host: %s\x0D\x0A", ctx->request->host);
    }
    buffer_strcat(hdr, "User-Agent: Netdata/rocks newhttpclient\x0D\x0A");

    if (ctx->request->request_type == HTTP_REQ_POST && ctx->request->payload && ctx->request->payload_size) {
        buffer_sprintf(hdr, "Content-Length: %zu\x0D\x0A", ctx->request->payload_size);
    }
    if (ctx->request->proxy_username) {
        size_t creds_plain_len = strlen(ctx->request->proxy_username) + strlen(ctx->request->proxy_password) + 1 /* ':' */;
        char *creds_plain = callocz(1, creds_plain_len + 1);
        char *ptr = creds_plain;
        strcpy(ptr, ctx->request->proxy_username);
        ptr += strlen(ctx->request->proxy_username);
        *ptr++ = ':';
        strcpy(ptr, ctx->request->proxy_password);

        int creds_base64_len = (((4 * creds_plain_len / 3) + 3) & ~3);
        // OpenSSL encoder puts newline every 64 output bytes
        // we remove those but during encoding we need that space in the buffer
        creds_base64_len += (1+(creds_base64_len/64)) * strlen("\n");
        char *creds_base64 = callocz(1, creds_base64_len + 1);
        base64_encode_helper((unsigned char*)creds_base64, &creds_base64_len, (unsigned char*)creds_plain, creds_plain_len);
        buffer_sprintf(hdr, "Proxy-Authorization: Basic %s\x0D\x0A", creds_base64);
        freez(creds_plain);
    }

    buffer_strcat(hdr, "\x0D\x0A");

    // Send the request
    if (https_client_write_all(ctx, hdr->buffer, hdr->len)) {
        error("Couldn't write HTTP request header into SSL connection");
        rc = 2;
        goto err_exit;
    }

    if (ctx->request->request_type == HTTP_REQ_POST && ctx->request->payload && ctx->request->payload_size) {
        if (https_client_write_all(ctx, ctx->request->payload, ctx->request->payload_size)) {
            error("Couldn't write payload into SSL connection");
            rc = 3;
            goto err_exit;
        }
    }

    // Read The Response
    if (read_parse_response(ctx)) {
        error("Error reading or parsing response from server");
        rc = 4;
        goto err_exit;
    }

err_exit:
    buffer_free(hdr);
    return rc;
}

static int cert_verify_callback(int preverify_ok, X509_STORE_CTX *ctx)
{
    X509 *err_cert;
    int err, depth;
    char *err_str;

    if (!preverify_ok) {
        err = X509_STORE_CTX_get_error(ctx);
        depth = X509_STORE_CTX_get_error_depth(ctx);
        err_cert = X509_STORE_CTX_get_current_cert(ctx);
        err_str = X509_NAME_oneline(X509_get_subject_name(err_cert), NULL, 0);

        error("Cert Chain verify error:num=%d:%s:depth=%d:%s", err,
                 X509_verify_cert_error_string(err), depth, err_str);

        free(err_str);
    }

#ifdef ACLK_SSL_ALLOW_SELF_SIGNED
    if (!preverify_ok && err == X509_V_ERR_DEPTH_ZERO_SELF_SIGNED_CERT)
    {
        preverify_ok = 1;
        error("Self Signed Certificate Accepted as the agent was built with ACLK_SSL_ALLOW_SELF_SIGNED");
    }
#endif

    return preverify_ok;
}

int https_request(https_req_t *request, https_req_response_t *response) {
    int rc = 1, ret;
    char connect_port_str[PORT_STR_MAX_BYTES];

    const char *connect_host = request->proxy_host ? request->proxy_host : request->host;
    int connect_port = request->proxy_host ? request->proxy_port : request->port;
    struct timeval timeout = { .tv_sec = request->timeout_s, .tv_usec = 0 };

    https_req_ctx_t *ctx = callocz(1, sizeof(https_req_ctx_t));
    ctx->req_start_time = now_realtime_sec();

    ctx->buf_rx = rbuf_create(RX_BUFFER_SIZE);
    if (!ctx->buf_rx) {
        error("Couldn't allocate buffer for RX data");
        goto exit_req_ctx;
    }

    snprintfz(connect_port_str, PORT_STR_MAX_BYTES, "%d", connect_port);

    ctx->sock = connect_to_this_ip46(IPPROTO_TCP, SOCK_STREAM, connect_host, 0, connect_port_str, &timeout);
    if (ctx->sock < 0) {
        error("Error connecting TCP socket to \"%s\"", connect_host);
        goto exit_buf_rx;
    }

    if (fcntl(ctx->sock, F_SETFL, fcntl(ctx->sock, F_GETFL, 0) | O_NONBLOCK) == -1) {
        error("Error setting O_NONBLOCK to TCP socket.");
        goto exit_sock;
    }

    ctx->poll_fd.fd = ctx->sock;

    // Do the CONNECT if proxy is used
    if (request->proxy_host) {
        https_req_t req = HTTPS_REQ_T_INITIALIZER;
        req.request_type = HTTP_REQ_CONNECT;
        req.timeout_s = request->timeout_s;
        req.host = request->host;
        req.port = request->port;
        req.url = request->url;
        req.proxy_username = request->proxy_username;
        req.proxy_password = request->proxy_password;
        ctx->request = &req;
        if (handle_http_request(ctx)) {
            error("Failed to CONNECT with proxy");
            goto exit_sock;
        }
        if (ctx->parse_ctx.http_code != 200) {
            error("Proxy didn't return 200 OK (got %d)", ctx->parse_ctx.http_code);
            goto exit_sock;
        }
        info("Proxy accepted CONNECT upgrade");
    }
    ctx->request = request;

    ctx->ssl_ctx = netdata_ssl_create_client_ctx(0);
    if (ctx->ssl_ctx==NULL) {
        error("Cannot allocate SSL context");
        goto exit_sock;
    }

    if (!SSL_CTX_set_default_verify_paths(ctx->ssl_ctx)) {
        error("Error setting default verify paths");
        goto exit_CTX;
    }
    SSL_CTX_set_verify(ctx->ssl_ctx, SSL_VERIFY_PEER | SSL_VERIFY_CLIENT_ONCE, cert_verify_callback);

    ctx->ssl = SSL_new(ctx->ssl_ctx);
    if (ctx->ssl==NULL) {
        error("Cannot allocate SSL");
        goto exit_CTX;
    }

    SSL_set_fd(ctx->ssl, ctx->sock);
    ret = SSL_connect(ctx->ssl);
    if (ret != -1 && ret != 1) {
        error("SSL could not connect");
        goto exit_SSL;
    }
    if (ret == -1) {
        // expected as underlying socket is non blocking!
        // consult SSL_connect documentation for details
        int ec = SSL_get_error(ctx->ssl, ret);
        if (ec != SSL_ERROR_WANT_READ && ec != SSL_ERROR_WANT_WRITE) {
            error("Failed to start SSL connection");
            goto exit_SSL;
        }
    }

    // The actual request here
    if (handle_http_request(ctx)) {
        error("Couldn't process request");
        goto exit_SSL;
    }
    response->http_code = ctx->parse_ctx.http_code;
    if (ctx->parse_ctx.content_length > 0) {
        response->payload_size = ctx->parse_ctx.content_length;
        response->payload = mallocz(response->payload_size + 1);
        ret = rbuf_pop(ctx->buf_rx, response->payload, response->payload_size);
        if (ret != (int)response->payload_size) {
            error("Payload size doesn't match remaining data on the buffer!");
            response->payload_size = ret;
        }
        // normally we take payload as it is and copy it
        // but for convenience in cases where payload is sth. like
        // json we add terminating zero so that user of the data
        // doesn't have to convert to C string (0 terminated)
        // other uses still have correct payload_size and can copy
        // only exact data without affixed 0x00
        ((char*)response->payload)[response->payload_size] = 0; // mallocz(response->payload_size + 1);
    }
    info("HTTPS \"%s\" request to \"%s\" finished with HTTP code: %d", http_req_type_to_str(ctx->request->request_type), ctx->request->host, response->http_code);

    rc = 0;

exit_SSL:
    SSL_free(ctx->ssl);
exit_CTX:
    SSL_CTX_free(ctx->ssl_ctx);
exit_sock:
    close(ctx->sock);
exit_buf_rx:
    rbuf_free(ctx->buf_rx);
exit_req_ctx:
    freez(ctx);
    return rc;
}

void https_req_response_free(https_req_response_t *res) {
    freez(res->payload);
}

void https_req_response_init(https_req_response_t *res) {
    res->http_code = 0;
    res->payload = NULL;
    res->payload_size = 0;
}

static inline char *UNUSED_FUNCTION(min_non_null)(char *a, char *b) {
    if (!a)
        return b;
    if (!b)
        return a;
    return (a < b ? a : b);
}

#define URI_PROTO_SEPARATOR "://"
#define URL_PARSER_LOG_PREFIX "url_parser "

static int parse_host_port(url_t *url) {
    char *ptr = strrchr(url->host, ':');
    if (ptr) {
        size_t port_len = strlen(ptr + 1);
        if (!port_len) {
            error(URL_PARSER_LOG_PREFIX ": specified but no port number");
            return 1;
        }
        if (port_len > 5 /* MAX port length is 5digit long in decimal */) {
            error(URL_PARSER_LOG_PREFIX "port # is too long");
            return 1;
        }
        *ptr = 0;
        if (!strlen(url->host)) {
            error(URL_PARSER_LOG_PREFIX "host empty after removing port");
            return 1;
        }
        url->port = atoi (ptr + 1);
    }
    return 0;
}

static inline void port_by_proto(url_t *url) {
    if (url->port)
        return;
    if (!url->proto)
        return;
    if (!strcmp(url->proto, "http")) {
        url->port = 80;
        return;
    }
    if (!strcmp(url->proto, "https")) {
        url->port = 443;
        return;
    }
}

#define STRDUPZ_2PTR(dest, start, end)                                                                                 \
    {                                                                                                                  \
        dest = mallocz(1 + end - start);                                                                               \
        memcpy(dest, start, end - start);                                                                              \
        dest[end - start] = 0;                                                                                         \
    }

int url_parse(const char *url, url_t *parsed) {
    const char *start = url;
    const char *end = strstr(url, URI_PROTO_SEPARATOR);

    if (end) {
        if (end == start) {
            error (URL_PARSER_LOG_PREFIX "found " URI_PROTO_SEPARATOR " without protocol specified");
            return 1;
        }

        STRDUPZ_2PTR(parsed->proto, start, end)
        start = end + strlen(URI_PROTO_SEPARATOR);
    }

    end = strchr(start, '/');
    if (!end)
        end = start + strlen(start);
    
    if (start == end) {
        error(URL_PARSER_LOG_PREFIX "Host empty");
        return 1;
    }

    STRDUPZ_2PTR(parsed->host, start, end);

    if (parse_host_port(parsed))
        return 1;

    if (!*end) {
        parsed->path = strdupz("/");
        port_by_proto(parsed);
        return 0;
    }

    parsed->path = strdupz(end);
    port_by_proto(parsed);
    return 0;
}

void url_t_destroy(url_t *url) {
    freez(url->host);
    freez(url->path);
    freez(url->proto);
}
