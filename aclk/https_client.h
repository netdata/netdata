// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HTTPS_CLIENT_H
#define NETDATA_HTTPS_CLIENT_H

#include "libnetdata/libnetdata.h"

#include "mqtt_websockets/c-rbuf/include/ringbuffer.h"

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
void https_req_response_init(https_req_response_t *res);

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

typedef struct {
    enum http_parse_state state;
    int content_length;
    int http_code;
} http_parse_ctx;

#define HTTP_PARSE_NEED_MORE_DATA  0
#define HTTP_PARSE_SUCCESS   1
#define HTTP_PARSE_ERROR    -1
int parse_http_response(rbuf_t buf, http_parse_ctx *parse_ctx);

#endif /* NETDATA_HTTPS_CLIENT_H */
