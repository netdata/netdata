// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HTTPS_CLIENT_H
#define NETDATA_HTTPS_CLIENT_H

#include "libnetdata/libnetdata.h"

typedef enum https_client_resp {
    HTTPS_CLIENT_RESP_OK = 0,

    // all the ND_SOCK_ERR_XXX are place here

    HTTPS_CLIENT_RESP_UNKNOWN_ERROR = ND_SOCK_ERR_MAX,
    HTTPS_CLIENT_RESP_NO_MEM,
    HTTPS_CLIENT_RESP_NONBLOCK_FAILED,
    HTTPS_CLIENT_RESP_PROXY_NOT_200,
    HTTPS_CLIENT_RESP_NO_SSL_CTX,
    HTTPS_CLIENT_RESP_NO_SSL_VERIFY_PATHS,
    HTTPS_CLIENT_RESP_NO_SSL_NEW,
    HTTPS_CLIENT_RESP_NO_TLS_SNI,
    HTTPS_CLIENT_RESP_SSL_CONNECT_FAILED,
    HTTPS_CLIENT_RESP_SSL_START_FAILED,
    HTTPS_CLIENT_RESP_UNKNOWN_REQUEST_TYPE,
    HTTPS_CLIENT_RESP_HEADER_WRITE_FAILED,
    HTTPS_CLIENT_RESP_PAYLOAD_WRITE_FAILED,
    HTTPS_CLIENT_RESP_POLL_ERROR,
    HTTPS_CLIENT_RESP_TIMEOUT,
    HTTPS_CLIENT_RESP_READ_ERROR,
    HTTPS_CLIENT_RESP_PARSE_ERROR,
    HTTPS_CLIENT_RESP_ENV_AGENT_NOT_CLAIMED,
    HTTPS_CLIENT_RESP_ENV_NOT_200,
    HTTPS_CLIENT_RESP_ENV_EMPTY,
    HTTPS_CLIENT_RESP_ENV_NOT_JSON,
    HTTPS_CLIENT_RESP_OTP_CHALLENGE_NOT_200,
    HTTPS_CLIENT_RESP_OTP_CHALLENGE_INVALID,
    HTTPS_CLIENT_RESP_OTP_PASSWORD_NOT_201,
    HTTPS_CLIENT_RESP_OTP_PASSWORD_EMPTY,
    HTTPS_CLIENT_RESP_OTP_PASSWORD_NOT_JSON,
    HTTPS_CLIENT_RESP_OTP_AGENT_NOT_CLAIMED,
    HTTPS_CLIENT_RESP_OTP_CHALLENGE_DECRYPTION_FAILED,

    // terminator
    HTTPS_CLIENT_RESP_MAX,
} https_client_resp_t;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(https_client_resp_t);

typedef enum http_req_type {
    HTTP_REQ_GET = 0,
    HTTP_REQ_POST,
    HTTP_REQ_CONNECT,
    HTTP_REQ_INVALID
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

https_client_resp_t https_request(https_req_t *request, https_req_response_t *response, bool *fallback_ipv4);

// we expose previously internal parser as this is usefull also from
// other parts of the code
enum http_parse_state {
    HTTP_PARSE_PROXY_CONNECT = 0,
    HTTP_PARSE_INITIAL,
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

void http_parse_ctx_create(http_parse_ctx *ctx, enum http_parse_state parse_state);
void http_parse_ctx_destroy(http_parse_ctx *ctx);

typedef enum {
    HTTP_PARSE_ERROR = -1,
    HTTP_PARSE_NEED_MORE_DATA = 0,
    HTTP_PARSE_SUCCESS = 1
} http_parse_rc;

http_parse_rc parse_http_response(rbuf_t buf, http_parse_ctx *parse_ctx);

const char *get_http_header_by_name(http_parse_ctx *ctx, const char *name);

#endif /* NETDATA_HTTPS_CLIENT_H */
