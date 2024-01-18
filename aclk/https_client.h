// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HTTPS_CLIENT_H
#define NETDATA_HTTPS_CLIENT_H

#include "libnetdata/libnetdata.h"

#include "mqtt_websockets/c-rbuf/cringbuffer.h"
#include "mqtt_websockets/c_rhash/c_rhash.h"

typedef enum http_req_type {
    HTTP_REQ_GET = 0,
    HTTP_REQ_POST,
    HTTP_REQ_CONNECT
} http_req_type_t;

typedef struct {
    http_req_type_t request_type;

    char *host;
    int port;
    char *url;

    time_t timeout_s; //timeout in seconds for the network operation (send/recv)

    void *payload;
    size_t payload_size;

    char *proxy_host;
    int proxy_port;
    const char *proxy_username;
    const char *proxy_password;
} https_req_t;

typedef struct {
    int http_code;

    void *payload;
    size_t payload_size;
} https_req_response_t;


// Non feature complete URL parser
// feel free to extend when needed
// currently implements only what ACLK
// needs
// proto://host[:port]/path
typedef struct {
    char *proto;
    char *host;
    int port;
    char* path;
} url_t;

int url_parse(const char *url, url_t *parsed);
void url_t_destroy(url_t *url);

void https_req_response_free(https_req_response_t *res);

#define HTTPS_REQ_RESPONSE_T_INITIALIZER            \
    {                                               \
        .http_code = 0,                             \
        .payload = NULL,                            \
        .payload_size = 0                           \
    }

#define HTTPS_REQ_T_INITIALIZER                     \
    {                                               \
        .request_type = HTTP_REQ_GET,               \
        .host = NULL,                               \
        .port = 443,                                \
        .url = NULL,                                \
        .timeout_s = 30,                            \
        .payload = NULL,                            \
        .payload_size = 0,                          \
        .proxy_host = NULL,                         \
        .proxy_port = 8080                          \
    }

int https_request(https_req_t *request, https_req_response_t *response);

// we expose previously internal parser as this is usefull also from
// other parts of the code
enum http_parse_state {
    HTTP_PARSE_INITIAL = 0,
    HTTP_PARSE_HEADERS,
    HTTP_PARSE_CONTENT
};

typedef uint32_t parse_ctx_flags_t;

#define HTTP_PARSE_FLAG_DONT_WAIT_FOR_CONTENT ((parse_ctx_flags_t)0x01)

#define HTTP_PARSE_FLAGS_DEFAULT ((parse_ctx_flags_t)0)

typedef struct {
    parse_ctx_flags_t flags;

    enum http_parse_state state;
    int content_length;
    int http_code;

    c_rhash headers;

    // for chunked data only
    char *chunked_response;
    size_t chunked_response_size;
    size_t chunked_response_written;

    enum chunked_content_state {
        CHUNKED_CONTENT_CHUNK_SIZE = 0,
        CHUNKED_CONTENT_CHUNK_DATA,
        CHUNKED_CONTENT_CHUNK_END_CRLF,
        CHUNKED_CONTENT_FINAL_CRLF
    } chunked_content_state;

    size_t chunk_size;
    size_t chunk_got;
} http_parse_ctx;

void http_parse_ctx_create(http_parse_ctx *ctx);
void http_parse_ctx_destroy(http_parse_ctx *ctx);

typedef enum {
    HTTP_PARSE_ERROR = -1,
    HTTP_PARSE_NEED_MORE_DATA = 0,
    HTTP_PARSE_SUCCESS = 1
} http_parse_rc;

http_parse_rc parse_http_response(rbuf_t buf, http_parse_ctx *parse_ctx);

const char *get_http_header_by_name(http_parse_ctx *ctx, const char *name);

#endif /* NETDATA_HTTPS_CLIENT_H */
