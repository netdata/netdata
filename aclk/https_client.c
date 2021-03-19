#include "libnetdata/libnetdata.h"

#include "https_client.h"

#include "../mqtt_websockets/c-rbuf/include/ringbuffer.h"

enum http_parse_state {
    HTTP_PARSE_INITIAL = 0,
    HTTP_PARSE_HEADERS,
    HTTP_PARSE_CONTENT
};

typedef struct {
    enum http_parse_state state;
    int content_length;
    int http_code;
} http_parse_ctx;

#define HTTP_PARSE_CTX_INITIALIZER { .state = HTTP_PARSE_INITIAL, .content_length = -1, .http_code = 0 }

#define NEED_MORE_DATA  0
#define PARSE_SUCCESS   1
#define PARSE_ERROR    -1
#define HTTP_LINE_TERM "\x0D\x0A"
#define RESP_PROTO "HTTP/1.1 "
#define HTTP_KEYVAL_SEPARATOR ": "
#define HTTP_HDR_BUFFER_SIZE 256
#define PORT_STR_MAX_BYTES 7

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

    rbuf_bump_tail(buf, strlen(HTTP_KEYVAL_SEPARATOR));

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
                if (parse_ctx->content_length < 0) {
                    error("content-length missing and http headers ended");
                    return PARSE_ERROR;
                }
                if (rbuf_bytes_available(buf) >= (size_t)parse_ctx->content_length)
                    return PARSE_SUCCESS;
                return NEED_MORE_DATA;
        }
    } while(1);
}

int https_request(http_req_type_t method, char *host, int port, char *url, char *b, size_t b_size, char *payload)
{
    struct timeval timeout = { .tv_sec = 30, .tv_usec = 0 };
    char sport[PORT_STR_MAX_BYTES];
    size_t len = 0;
    int rc = 1;
    int ret;
    char *ptr;
    http_parse_ctx parse_ctx = HTTP_PARSE_CTX_INITIALIZER;

    rbuf_t buffer = rbuf_create(b_size);
    if (!buffer)
        return 1;

    snprintf(sport, PORT_STR_MAX_BYTES, "%d", port);

    if (payload != NULL)
        len = strlen(payload);

    snprintf(
        b,
        b_size,
        "%s %s HTTP/1.1\r\nHost: %s\r\nAccept: application/json\r\nContent-length: %zu\r\nAccept-Language: en-us\r\n"
        "User-Agent: Netdata/rocks\r\n\r\n",
        (method == HTTP_REQ_GET ? "GET" : "POST"), url, host, len);

    if (payload != NULL)
        strncat(b, payload, b_size - len);

    len = strlen(b);

    debug(D_ACLK, "Sending HTTPS req (%zu bytes): '%s'", len, b);
    int sock = connect_to_this_ip46(IPPROTO_TCP, SOCK_STREAM, host, 0, sport, &timeout);

    if (unlikely(sock == -1)) {
        error("Handshake failed");
        goto exit_buf;
    }

    SSL_CTX *ctx = security_initialize_openssl_client();
    if (ctx==NULL) {
        error("Cannot allocate SSL context");
        goto exit_sock;
    }
    // Certificate chain: not updating the stores - do we need private CA roots?
    // Calls to SSL_CTX_load_verify_locations would go here.
    SSL *ssl = SSL_new(ctx);
    if (ssl==NULL) {
        error("Cannot allocate SSL");
        goto exit_CTX;
    }
    SSL_set_fd(ssl, sock);
    ret = SSL_connect(ssl);
    if (ret != 1) {
        error("SSL_connect() failed with err=%d", ret);
        goto exit_SSL;
    }

    ret = SSL_write(ssl, b, len);
    if (ret <= 0)
    {
        error("SSL_write() failed with err=%d", ret);
        goto exit_SSL;
    }

    b[0] = 0;

    do {
        ptr = rbuf_get_linear_insert_range(buffer, &len);
        ret = SSL_read(ssl, ptr, len - 1);
        if (ret)
            rbuf_bump_head(buffer, ret);
        if (ret <= 0)
        {
            error("No response available - SSL_read()=%d", ret);
            goto exit_FULL;
        }
    } while (!(ret = parse_http_response(buffer, &parse_ctx)));

    if (ret != PARSE_SUCCESS) {
        error("Error parsing HTTP response");
        goto exit_FULL;
    }

    if (parse_ctx.http_code < 200 || parse_ctx.http_code >= 300) {
        error("HTTP Response not Success (got %d)", parse_ctx.http_code);
        goto exit_FULL;
    }

    len = rbuf_pop(buffer, b, b_size);
    b[MIN(len, b_size-1)] = 0;

    rc = 0;
exit_FULL:
exit_SSL:
    SSL_free(ssl);
exit_CTX:
    SSL_CTX_free(ctx);
exit_sock:
    close(sock);
exit_buf:
    rbuf_free(buffer);
    return rc;
}
