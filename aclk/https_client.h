#ifndef NETDATA_HTTPS_CLIENT_H
#define NETDATA_HTTPS_CLIENT_H

typedef enum http_req_type {
    HTTP_REQ_GET,
    HTTP_REQ_POST
} http_req_type_t;

int https_request(http_req_type_t method, char *host, int port, char *url, char *b, size_t b_size, char *payload);

#endif /* NETDATA_HTTPS_CLIENT_H */
