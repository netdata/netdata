// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#include "https_client.h"

#include "aclk_util.h"

#include "daemon/pulse/pulse.h"

ENUM_STR_MAP_DEFINE(https_client_resp_t) = {
    {
        .id = HTTPS_CLIENT_RESP_OK,
        .name = "ok",
    },
    {
        .id = HTTPS_CLIENT_RESP_UNKNOWN_ERROR,
        .name = "unknown error",
    },
    {
        .id = HTTPS_CLIENT_RESP_NO_MEM,
        .name = "not enough memory",
    },
    {
        .id = HTTPS_CLIENT_RESP_NONBLOCK_FAILED,
        .name = "cannot set socket to non-blocking mode",
    },
    {
        .id = HTTPS_CLIENT_RESP_PROXY_NOT_200,
        .name = "proxy did not return http/200",
    },
    {
        .id = HTTPS_CLIENT_RESP_NO_SSL_CTX,
        .name = "cannot create SSL ctx",
    },
    {
        .id = HTTPS_CLIENT_RESP_NO_SSL_VERIFY_PATHS,
        .name = "cannot set SSL verify paths",
    },
    {
        .id = HTTPS_CLIENT_RESP_NO_SSL_NEW,
        .name = "cannot create SSL",
    },
    {
        .id = HTTPS_CLIENT_RESP_NO_TLS_SNI,
        .name = "cannot set TLS SNI",
    },
    {
        .id = HTTPS_CLIENT_RESP_SSL_CONNECT_FAILED,
        .name = "SSL_connect() failed",
    },
    {
        .id = HTTPS_CLIENT_RESP_SSL_START_FAILED,
        .name = "cannot start SSL connection",
    },
    {
        .id = HTTPS_CLIENT_RESP_UNKNOWN_REQUEST_TYPE,
        .name = "unknown https client request type",
    },
    {
        .id = HTTPS_CLIENT_RESP_HEADER_WRITE_FAILED,
        .name = "https client failed to write http header",
    },
    {
        .id = HTTPS_CLIENT_RESP_PAYLOAD_WRITE_FAILED,
        .name = "https client failed to write http payload",
    },
    {
        .id = HTTPS_CLIENT_RESP_POLL_ERROR,
        .name = "https client poll() error",
    },
    {
        .id = HTTPS_CLIENT_RESP_TIMEOUT,
        .name = "https client timeout",
    },
    {
        .id = HTTPS_CLIENT_RESP_READ_ERROR,
        .name = "https client read error",
    },
    {
        .id = HTTPS_CLIENT_RESP_PARSE_ERROR,
        .name = "https client parsing of response failed",
    },
    {
        .id = HTTPS_CLIENT_RESP_ENV_AGENT_NOT_CLAIMED,
        .name = "agent is not claimed (during /env)",
    },
    {
        .id = HTTPS_CLIENT_RESP_ENV_NOT_200,
        .name = "/env response code is not 200",
    },
    {
        .id = HTTPS_CLIENT_RESP_ENV_EMPTY,
        .name = "/env response is empty",
    },
    {
        .id = HTTPS_CLIENT_RESP_ENV_NOT_JSON,
        .name = "/env response is not JSON",
    },
    {
        .id = HTTPS_CLIENT_RESP_OTP_CHALLENGE_NOT_200,
        .name = "otp challenge response is not http/200",
    },
    {
        .id = HTTPS_CLIENT_RESP_OTP_CHALLENGE_INVALID,
        .name = "otp challenge response is invalid",
    },
    {
        .id = HTTPS_CLIENT_RESP_OTP_PASSWORD_NOT_201,
        .name = "otp password response is not http/201",
    },
    {
        .id = HTTPS_CLIENT_RESP_OTP_PASSWORD_EMPTY,
        .name = "otp password response is empty",
    },
    {
        .id = HTTPS_CLIENT_RESP_OTP_PASSWORD_NOT_JSON,
        .name = "otp password response is not JSON",
    },
    {
        .id = HTTPS_CLIENT_RESP_OTP_AGENT_NOT_CLAIMED,
        .name = "agent is not claimed (during otp)",
    },
    {
        .id = HTTPS_CLIENT_RESP_OTP_CHALLENGE_DECRYPTION_FAILED,
        .name = "otp challenge decryption failed",
    },

    // terminator
    {.name = NULL, .id = 0}
};

ENUM_STR_DEFINE_FUNCTIONS(https_client_resp_t, HTTPS_CLIENT_RESP_UNKNOWN_ERROR, "unknown error");

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

#define TRANSFER_ENCODING_CHUNKED (-2)

void http_parse_ctx_create(http_parse_ctx *ctx, enum http_parse_state parse_state)
{
    ctx->state = parse_state;
    ctx->content_length = -1;
    ctx->http_code = 0;
    ctx->headers = c_rhash_new(0);
    ctx->flags = HTTP_PARSE_FLAGS_DEFAULT;
}

void http_parse_ctx_destroy(http_parse_ctx *ctx)
{
    if(!ctx->headers)
        return;

    c_rhash_iter_t iter;
    const char *key;

    c_rhash_iter_t_initialize(&iter);
    while ( !c_rhash_iter_str_keys(ctx->headers, &iter, &key) ) {
        void *val;
        c_rhash_get_ptr_by_str(ctx->headers, key, &val);
        freez(val);
    }

    c_rhash_destroy(ctx->headers);
    ctx->headers = NULL;
}

#define POLL_TO_MS 100

#define HTTP_LINE_TERM "\x0D\x0A"
#define RESP_PROTO "HTTP/1.1 "
#define RESP_PROTO10 "HTTP/1.0 "
#define HTTP_KEYVAL_SEPARATOR ": "
#define HTTP_HDR_BUFFER_SIZE 1024
#define PORT_STR_MAX_BYTES 12

static int process_http_hdr(http_parse_ctx *parse_ctx, const char *key, const char *val)
{
    // currently we care only about specific headers
    // we can skip the rest
    if (parse_ctx->content_length < 0 && !strcmp("content-length", key)) {
        if (parse_ctx->content_length == TRANSFER_ENCODING_CHUNKED) {
            netdata_log_error("ACLK: Content-length and transfer-encoding: chunked headers are mutually exclusive");
            return 1;
        }
        if (parse_ctx->content_length != -1) {
            netdata_log_error("ACLK: Duplicate content-length header");
            return 1;
        }
        parse_ctx->content_length = str2u(val);
        if (parse_ctx->content_length < 0) {
            netdata_log_error("ACLK: Invalid content-length %d", parse_ctx->content_length);
            return 1;
        }
        return 0;
    }
    if (!strcmp("transfer-encoding", key)) {
        if (!strcmp("chunked", val)) {
            if (parse_ctx->content_length != -1) {
                netdata_log_error("ACLK: Content-length and transfer-encoding: chunked headers are mutually exclusive");
                return 1;
            }
            parse_ctx->content_length = TRANSFER_ENCODING_CHUNKED;
        }
        return 0;
    }
    char *val_cpy = strdupz(val);
    c_rhash_insert_str_ptr(parse_ctx->headers, key, val_cpy);
    return 0;
}

const char *get_http_header_by_name(http_parse_ctx *ctx, const char *name)
{
    const char *ret;
    if (c_rhash_get_ptr_by_str(ctx->headers, name, (void**)&ret))
        return NULL;

    return ret;
}

static int parse_http_hdr(rbuf_t buf, http_parse_ctx *parse_ctx)
{
    int idx, idx_end;
    char buf_key[HTTP_HDR_BUFFER_SIZE];
    char buf_val[HTTP_HDR_BUFFER_SIZE];
    char *ptr;

    if (!rbuf_find_bytes(buf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM), &idx_end)) {
        netdata_log_error("ACLK: CRLF expected");
        return 1;
    }

    char *separator = rbuf_find_bytes(buf, HTTP_KEYVAL_SEPARATOR, strlen(HTTP_KEYVAL_SEPARATOR), &idx);
    if (!separator) {
        netdata_log_error("ACLK: Missing Key/Value separator");
        return 1;
    }
    if (idx >= HTTP_HDR_BUFFER_SIZE) {
        netdata_log_error("ACLK: Key name is too long");
        return 1;
    }

    rbuf_pop(buf, buf_key, idx);
    buf_key[idx] = 0;

    rbuf_bump_tail(buf, strlen(HTTP_KEYVAL_SEPARATOR));
    idx_end -= strlen(HTTP_KEYVAL_SEPARATOR) + idx;
    if (idx_end >= HTTP_HDR_BUFFER_SIZE) {
        netdata_log_error("ACLK: Value of key \"%s\" too long", buf_key);
        return 1;
    }

    rbuf_pop(buf, buf_val, idx_end);
    buf_val[idx_end] = 0;

    for (ptr = buf_key; *ptr; ptr++)
        *ptr = tolower(*ptr);

    if (process_http_hdr(parse_ctx, buf_key, buf_val))
        return 1;

    return 0;
}

static inline void chunked_response_buffer_grow_by(http_parse_ctx *parse_ctx, size_t size)
{
    if (unlikely(parse_ctx->chunked_response_size == 0)) {
        parse_ctx->chunked_response = mallocz(size);
        parse_ctx->chunked_response_size = size;
        return;
    }
    parse_ctx->chunked_response = reallocz((void *)parse_ctx->chunked_response, parse_ctx->chunked_response_size + size);
    parse_ctx->chunked_response_size += size;
}

static int process_chunked_content(rbuf_t buf, http_parse_ctx *parse_ctx)
{
    int idx;
    size_t bytes_to_copy;

    do {
        switch (parse_ctx->chunked_content_state) {
            case CHUNKED_CONTENT_CHUNK_SIZE:
                if (!rbuf_find_bytes(buf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM), &idx)) {
                    if (rbuf_bytes_available(buf) >= rbuf_get_capacity(buf))
                        return HTTP_PARSE_ERROR;
                    return HTTP_PARSE_NEED_MORE_DATA;
                }
                if (idx == 0) {
                    parse_ctx->chunked_content_state = CHUNKED_CONTENT_FINAL_CRLF;
                    continue;
                }
                if (idx >= HTTP_HDR_BUFFER_SIZE) {
                    netdata_log_error("ACLK: Chunk size is too long");
                    return HTTP_PARSE_ERROR;
                }
                char buf_size[HTTP_HDR_BUFFER_SIZE];
                rbuf_pop(buf, buf_size, idx);
                buf_size[idx] = 0;
                long chunk_size = strtol(buf_size, NULL, 16);
                if (chunk_size < 0 || chunk_size == LONG_MAX) {
                    netdata_log_error("ACLK: Chunk size out of range");
                    return HTTP_PARSE_ERROR;
                }
                parse_ctx->chunk_size = chunk_size;
                if (parse_ctx->chunk_size == 0) {
                    if (errno == EINVAL) {
                        netdata_log_error("ACLK: Invalid chunk size");
                        return HTTP_PARSE_ERROR;
                    }
                    parse_ctx->chunked_content_state = CHUNKED_CONTENT_CHUNK_END_CRLF;
                    continue;
                }
                parse_ctx->chunk_got = 0;
                chunked_response_buffer_grow_by(parse_ctx, parse_ctx->chunk_size);
                rbuf_bump_tail(buf, strlen(HTTP_LINE_TERM));
                parse_ctx->chunked_content_state = CHUNKED_CONTENT_CHUNK_DATA;
                // fallthrough
            case CHUNKED_CONTENT_CHUNK_DATA:
                if (!(bytes_to_copy = rbuf_bytes_available(buf)))
                    return HTTP_PARSE_NEED_MORE_DATA;
                if (bytes_to_copy > parse_ctx->chunk_size - parse_ctx->chunk_got)
                    bytes_to_copy = parse_ctx->chunk_size - parse_ctx->chunk_got;
                rbuf_pop(buf, parse_ctx->chunked_response + parse_ctx->chunked_response_written, bytes_to_copy);
                parse_ctx->chunk_got += bytes_to_copy;
                parse_ctx->chunked_response_written += bytes_to_copy;
                if (parse_ctx->chunk_got != parse_ctx->chunk_size)
                    continue;
                parse_ctx->chunked_content_state = CHUNKED_CONTENT_CHUNK_END_CRLF;
                // fallthrough
            case CHUNKED_CONTENT_FINAL_CRLF:
            case CHUNKED_CONTENT_CHUNK_END_CRLF:
                if (rbuf_bytes_available(buf) < strlen(HTTP_LINE_TERM))
                    return HTTP_PARSE_NEED_MORE_DATA;
                char buf_crlf[strlen(HTTP_LINE_TERM)];
                rbuf_pop(buf, buf_crlf, strlen(HTTP_LINE_TERM));
                if (memcmp(buf_crlf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM))) {
                    netdata_log_error("ACLK: CRLF expected");
                    return HTTP_PARSE_ERROR;
                }
                if (parse_ctx->chunked_content_state == CHUNKED_CONTENT_FINAL_CRLF) {
                    if (parse_ctx->chunked_response_size != parse_ctx->chunked_response_written)
                        netdata_log_error("ACLK: Chunked response size mismatch");
                    chunked_response_buffer_grow_by(parse_ctx, 1);
                    parse_ctx->chunked_response[parse_ctx->chunked_response_written] = 0;
                    return HTTP_PARSE_SUCCESS;
                }
                if (parse_ctx->chunk_size == 0) {
                    parse_ctx->chunked_content_state = CHUNKED_CONTENT_FINAL_CRLF;
                    continue;
                }
                parse_ctx->chunked_content_state = CHUNKED_CONTENT_CHUNK_SIZE;
                continue;
        }
    } while(1);
}

http_parse_rc parse_http_response(rbuf_t buf, http_parse_ctx *parse_ctx)
{
    int idx;
    char rc[4];

    do {
        if (parse_ctx->state != HTTP_PARSE_CONTENT && !rbuf_find_bytes(buf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM), &idx))
            return HTTP_PARSE_NEED_MORE_DATA;
        switch (parse_ctx->state) {
            case HTTP_PARSE_PROXY_CONNECT:
            case HTTP_PARSE_INITIAL:
                if (rbuf_memcmp_n(buf, RESP_PROTO, strlen(RESP_PROTO))) {
                    if (parse_ctx->state == HTTP_PARSE_PROXY_CONNECT) {
                        if (rbuf_memcmp_n(buf, RESP_PROTO10, strlen(RESP_PROTO10))) {
                            netdata_log_error(
                                "ACLK: Expected response to start with \"%s\" or \"%s\"", RESP_PROTO, RESP_PROTO10);
                            return HTTP_PARSE_ERROR;
                        }
                    }
                    else {
                        netdata_log_error("ACLK: Expected response to start with \"%s\"", RESP_PROTO);
                        return HTTP_PARSE_ERROR;
                    }
                }
                rbuf_bump_tail(buf, strlen(RESP_PROTO));
                if (rbuf_pop(buf, rc, 4) != 4) {
                    netdata_log_error("ACLK: Expected HTTP status code");
                    return HTTP_PARSE_ERROR;
                }
                if (rc[3] != ' ') {
                    netdata_log_error("ACLK: Expected space after HTTP return code");
                    return HTTP_PARSE_ERROR;
                }
                rc[3] = 0;
                parse_ctx->http_code = atoi(rc);
                if (parse_ctx->http_code < 100 || parse_ctx->http_code >= 600) {
                    netdata_log_error("ACLK: HTTP code not in range 100 to 599");
                    return HTTP_PARSE_ERROR;
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
                    return HTTP_PARSE_ERROR;
                rbuf_find_bytes(buf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM), &idx);
                rbuf_bump_tail(buf, idx + strlen(HTTP_LINE_TERM));
                break;
            case HTTP_PARSE_CONTENT:
                // replies like CONNECT etc. do not have content
                if (parse_ctx->content_length == TRANSFER_ENCODING_CHUNKED)
                    return process_chunked_content(buf, parse_ctx);

                if (parse_ctx->content_length < 0)
                    return HTTP_PARSE_SUCCESS;

                if (parse_ctx->flags & HTTP_PARSE_FLAG_DONT_WAIT_FOR_CONTENT)
                    return HTTP_PARSE_SUCCESS;

                if (rbuf_bytes_available(buf) >= (size_t)parse_ctx->content_length)
                    return HTTP_PARSE_SUCCESS;
                return HTTP_PARSE_NEED_MORE_DATA;
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

    http_parse_ctx parse_ctx;

    time_t req_start_time;
} https_req_ctx_t;

static int https_req_check_timedout(https_req_ctx_t *ctx) {
    if (now_realtime_sec() > ctx->req_start_time + ctx->request->timeout_s) {
        netdata_log_error("ACLK: request timed out");
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
            netdata_log_error("ACLK: poll error");
            return 1;
        }
        if (ret == 0) {
            if (https_req_check_timedout(ctx)) {
                netdata_log_error("ACLK: Poll timed out");
                return 2;
            }
            continue;
        }

        ret = write(ctx->sock, &data[ctx->written], data_len - ctx->written);
        if (ret > 0) {
            ctx->written += ret;
        } else if (errno != EAGAIN && errno != EWOULDBLOCK) {
            netdata_log_error("ACLK: Error writing to socket");
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
            netdata_log_error("ACLK: poll error");
            return 1;
        }
        if (ret == 0) {
            if (https_req_check_timedout(ctx)) {
                netdata_log_error("ACLK: Poll timed out");
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
                    netdata_log_error("ACLK: SSL_write Err: %s", _ssl_err_tos(ret));
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

static https_client_resp_t read_parse_response(https_req_ctx_t *ctx) {
    int ret;
    char *ptr;
    size_t size;

    ctx->poll_fd.events = POLLIN;
    do {
        ret = poll(&ctx->poll_fd, 1, POLL_TO_MS);
        if (ret < 0) {
            netdata_log_error("ACLK: poll error");
            return HTTPS_CLIENT_RESP_POLL_ERROR;
        }
        if (ret == 0) {
            if (https_req_check_timedout(ctx)) {
                netdata_log_error("ACLK: poll() timed out");
                return HTTPS_CLIENT_RESP_TIMEOUT;
            }
            if (!ctx->ssl_ctx)
                continue;
        }
        ctx->poll_fd.events = 0;

        do {
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
                            netdata_log_error("ACLK: SSL_read() Err: %s", _ssl_err_tos(ret));
                            return HTTPS_CLIENT_RESP_READ_ERROR;
                    }
                }
                else {
                    if (errno != EAGAIN && errno != EWOULDBLOCK) {
                        netdata_log_error("ACLK: read error");
                        return HTTPS_CLIENT_RESP_READ_ERROR;
                    }
                    ctx->poll_fd.events |= POLLIN;
                }
            }
        } while (ctx->poll_fd.events == 0 && rbuf_bytes_free(ctx->buf_rx) > 0);
    } while (!(ret = parse_http_response(ctx->buf_rx, &ctx->parse_ctx)));

    if (ret != HTTP_PARSE_SUCCESS) {
        netdata_log_error("ACLK: error parsing HTTP response");
        return HTTPS_CLIENT_RESP_PARSE_ERROR;
    }

    return HTTPS_CLIENT_RESP_OK;
}

static const char *http_methods[] = {
    [HTTP_REQ_GET] = "GET ",
    [HTTP_REQ_POST] = "POST ",
    [HTTP_REQ_CONNECT] = "CONNECT ",
};


#define TX_BUFFER_SIZE 8192
#define RX_BUFFER_SIZE (TX_BUFFER_SIZE*2)
static https_client_resp_t handle_http_request(https_req_ctx_t *ctx) {
    BUFFER *hdr = buffer_create(TX_BUFFER_SIZE, &netdata_buffers_statistics.buffers_aclk);
    https_client_resp_t rc = HTTPS_CLIENT_RESP_OK;

    http_req_type_t req_type = ctx->request->request_type;

    if (req_type >= HTTP_REQ_INVALID) {
        netdata_log_error("ACLK: unknown HTTPS request type!");
        rc = HTTPS_CLIENT_RESP_UNKNOWN_REQUEST_TYPE;
        goto err_exit;
    }
    buffer_strcat(hdr, http_methods[req_type]);

    if (req_type == HTTP_REQ_CONNECT) {
        buffer_strcat(hdr, ctx->request->host);
        buffer_sprintf(hdr, ":%d", ctx->request->port);
        http_parse_ctx_create(&ctx->parse_ctx, HTTP_PARSE_PROXY_CONNECT);
    }
    else {
        buffer_strcat(hdr, ctx->request->url);
        http_parse_ctx_create(&ctx->parse_ctx, HTTP_PARSE_INITIAL);
    }

    buffer_strcat(hdr, HTTP_1_1 HTTP_ENDL);

    //TODO Headers!
    buffer_sprintf(hdr, "Host: %s\x0D\x0A", ctx->request->host);
    buffer_strcat(hdr, "User-Agent: Netdata/rocks newhttpclient\x0D\x0A");

    if (req_type == HTTP_REQ_POST && ctx->request->payload && ctx->request->payload_size) {
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
        (void) netdata_base64_encode((unsigned char *)creds_base64, (unsigned char *)creds_plain, creds_plain_len);
        buffer_sprintf(hdr, "Proxy-Authorization: Basic %s\x0D\x0A", creds_base64);
        freez(creds_plain);
    }

    buffer_strcat(hdr, "\x0D\x0A");

    // Send the request
    if (https_client_write_all(ctx, hdr->buffer, hdr->len)) {
        netdata_log_error("ACLK: couldn't write HTTP request header into SSL connection");
        rc = HTTPS_CLIENT_RESP_HEADER_WRITE_FAILED;
        goto err_exit;
    }

    if (req_type == HTTP_REQ_POST && ctx->request->payload && ctx->request->payload_size) {
        if (https_client_write_all(ctx, ctx->request->payload, ctx->request->payload_size)) {
            netdata_log_error("ACLK: couldn't write payload into SSL connection");
            rc = HTTPS_CLIENT_RESP_PAYLOAD_WRITE_FAILED;
            goto err_exit;
        }
    }

    // Read The Response
    rc = read_parse_response(ctx);
    if (rc != HTTPS_CLIENT_RESP_OK) {
        netdata_log_error("ACLK: error reading or parsing response from server");
        if (ctx->parse_ctx.chunked_response)
            freez(ctx->parse_ctx.chunked_response);
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

        netdata_log_error("Cert Chain verify error:num=%d:%s:depth=%d:%s", err,
                 X509_verify_cert_error_string(err), depth, err_str);

        free(err_str);
    }

#ifdef ACLK_SSL_ALLOW_SELF_SIGNED
    if (!preverify_ok && err == X509_V_ERR_DEPTH_ZERO_SELF_SIGNED_CERT)
    {
        preverify_ok = 1;
        netdata_log_error("ACLK: Self Signed Certificate Accepted as the agent was built with ACLK_SSL_ALLOW_SELF_SIGNED");
    }
#endif

    return preverify_ok;
}

https_client_resp_t https_request(https_req_t *request, https_req_response_t *response, bool *fallback_ipv4)
{
    https_client_resp_t rc;
    int ret;
    char connect_port_str[PORT_STR_MAX_BYTES];

    const char *connect_host = request->proxy_host ? request->proxy_host : request->host;
    int connect_port = request->proxy_host ? request->proxy_port : request->port;
    struct timeval timeout = { .tv_sec = 10, .tv_usec = 0 };

    https_req_ctx_t *ctx = callocz(1, sizeof(https_req_ctx_t));
    ctx->req_start_time = now_realtime_sec();

    ctx->buf_rx = rbuf_create(RX_BUFFER_SIZE);
    if (!ctx->buf_rx) {
        rc = HTTPS_CLIENT_RESP_NO_MEM;
        netdata_log_error("ACLK: couldn't allocate buffer for RX data");
        goto exit_req_ctx;
    }

    snprintfz(connect_port_str, PORT_STR_MAX_BYTES, "%d", connect_port);

    ctx->sock = connect_to_this_ip46(IPPROTO_TCP, SOCK_STREAM, connect_host, 0, connect_port_str, &timeout, fallback_ipv4);
    if (ctx->sock < 0) {
        rc = -ctx->sock;
        netdata_log_error("ACLK: error connecting TCP socket to \"%s\"", connect_host);
        goto exit_buf_rx;
    }

    if (fcntl(ctx->sock, F_SETFL, fcntl(ctx->sock, F_GETFL, 0) | O_NONBLOCK) == -1) {
        rc = HTTPS_CLIENT_RESP_NONBLOCK_FAILED;
        netdata_log_error("ACLK: error setting O_NONBLOCK to TCP socket.");
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
        rc = handle_http_request(ctx);
        if (rc != HTTPS_CLIENT_RESP_OK) {
            netdata_log_error("ACLK: failed to CONNECT with proxy");
            http_parse_ctx_destroy(&ctx->parse_ctx);
            goto exit_sock;
        }
        if (ctx->parse_ctx.http_code != 200) {
            rc = HTTPS_CLIENT_RESP_PROXY_NOT_200;
            netdata_log_error("ACLK: proxy didn't return 200 OK (got %d)", ctx->parse_ctx.http_code);
            http_parse_ctx_destroy(&ctx->parse_ctx);
            goto exit_sock;
        }
        http_parse_ctx_destroy(&ctx->parse_ctx);
    }
    ctx->request = request;

    ctx->ssl_ctx = netdata_ssl_create_client_ctx(0);
    if (ctx->ssl_ctx==NULL) {
        rc = HTTPS_CLIENT_RESP_NO_SSL_CTX;
        netdata_log_error("ACLK: cannot allocate SSL context");
        goto exit_sock;
    }

    if (!SSL_CTX_set_default_verify_paths(ctx->ssl_ctx)) {
        rc = HTTPS_CLIENT_RESP_NO_SSL_VERIFY_PATHS;
        netdata_log_error("ACLK: error setting default verify paths");
        goto exit_CTX;
    }
    SSL_CTX_set_verify(ctx->ssl_ctx, SSL_VERIFY_PEER | SSL_VERIFY_CLIENT_ONCE, cert_verify_callback);

    ctx->ssl = SSL_new(ctx->ssl_ctx);
    if (ctx->ssl==NULL) {
        rc = HTTPS_CLIENT_RESP_NO_SSL_NEW;
        netdata_log_error("ACLK: cannot allocate SSL");
        goto exit_CTX;
    }

    if (!SSL_set_tlsext_host_name(ctx->ssl, request->host)) {
        rc = HTTPS_CLIENT_RESP_NO_TLS_SNI;
        netdata_log_error("ACLK: error setting TLS SNI host");
        goto exit_CTX;
    }

    SSL_set_fd(ctx->ssl, ctx->sock);
    ret = SSL_connect(ctx->ssl);
    if (ret != -1 && ret != 1) {
        rc = HTTPS_CLIENT_RESP_SSL_CONNECT_FAILED;
        netdata_log_error("ACLK: SSL failed to connect");
        goto exit_SSL;
    }
    if (ret == -1) {
        // expected as underlying socket is non blocking!
        // consult SSL_connect documentation for details
        int ec = SSL_get_error(ctx->ssl, ret);
        if (ec != SSL_ERROR_WANT_READ && ec != SSL_ERROR_WANT_WRITE) {
            rc = HTTPS_CLIENT_RESP_SSL_START_FAILED;
            netdata_log_error("ACLK: failed to start SSL connection");
            goto exit_SSL;
        }
    }

    // The actual request here
    rc = handle_http_request(ctx);
    if (rc != HTTPS_CLIENT_RESP_OK) {
        netdata_log_error("ACLK: couldn't process request");
        http_parse_ctx_destroy(&ctx->parse_ctx);
        goto exit_SSL;
    }
    http_parse_ctx_destroy(&ctx->parse_ctx);
    response->http_code = ctx->parse_ctx.http_code;
    if (ctx->parse_ctx.content_length == TRANSFER_ENCODING_CHUNKED) {
        response->payload_size = ctx->parse_ctx.chunked_response_size;
        response->payload = ctx->parse_ctx.chunked_response;
    }
    if (ctx->parse_ctx.content_length > 0) {
        response->payload_size = ctx->parse_ctx.content_length;
        response->payload = mallocz(response->payload_size + 1);
        ret = rbuf_pop(ctx->buf_rx, response->payload, response->payload_size);
        if (ret != (int)response->payload_size) {
            netdata_log_error("ACLK: payload size doesn't match remaining data on the buffer!");
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
    netdata_log_info("ACLK: HTTPS \"%s\" request to \"%s\" finished with HTTP code: %d", http_req_type_to_str(ctx->request->request_type), ctx->request->host, response->http_code);

    rc = HTTPS_CLIENT_RESP_OK;

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

static inline char *UNUSED_FUNCTION(min_non_null)(char *a, char *b) {
    if (!a)
        return b;
    if (!b)
        return a;
    return (a < b ? a : b);
}

#define URI_PROTO_SEPARATOR "://"
#define URL_PARSER_LOG_PREFIX "ACLK: url_parser "

static int parse_host_port(url_t *url) {
    char *ptr = strrchr(url->host, ':');
    if (ptr) {
        size_t port_len = strlen(ptr + 1);
        if (!port_len) {
            netdata_log_error(URL_PARSER_LOG_PREFIX ": specified but no port number");
            return 1;
        }
        if (port_len > 5 /* MAX port length is 5digit long in decimal */) {
            netdata_log_error(URL_PARSER_LOG_PREFIX "port # is too long");
            return 1;
        }
        *ptr = 0;
        if (!strlen(url->host)) {
            netdata_log_error(URL_PARSER_LOG_PREFIX "host empty after removing port");
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

#define STRDUPZ_2PTR(dest, start, end) do {                                                                            \
        dest = mallocz(1 + end - start);                                                                               \
        memcpy(dest, start, end - start);                                                                              \
        dest[end - start] = 0;                                                                                         \
    } while(0)

int url_parse(const char *url, url_t *parsed) {
    const char *start = url;
    const char *end = strstr(url, URI_PROTO_SEPARATOR);

    if (end) {
        if (end == start) {
            netdata_log_error(URL_PARSER_LOG_PREFIX "found " URI_PROTO_SEPARATOR " without protocol specified");
            return 1;
        }

        STRDUPZ_2PTR(parsed->proto, start, end);
        start = end + strlen(URI_PROTO_SEPARATOR);
    }

    end = strchr(start, '/');
    if (!end)
        end = start + strlen(start);
    
    if (start == end) {
        netdata_log_error(URL_PARSER_LOG_PREFIX "Host empty");
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
