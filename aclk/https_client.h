#ifndef NETDATA_HTTPS_CLIENT_H
#define NETDATA_HTTPS_CLIENT_H

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

    int timeout_s; //timeout in seconds for the network operation (send/recv)

    void *payload;
    size_t payload_size;

    char *proxy_host;
    int proxy_port;
} https_req_t;

typedef struct https_req_hdr https_req_hdr_t;
struct https_req_hdr {
    char *key;
    char *val;

    https_req_hdr_t *next;
};

typedef struct {
    int http_code;

    void *payload;
    size_t payload_size;
} https_req_response_t;

void https_req_response_free(https_req_response_t *res);

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

#endif /* NETDATA_HTTPS_CLIENT_H */
