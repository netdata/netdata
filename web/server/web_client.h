// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_CLIENT_H
#define NETDATA_WEB_CLIENT_H 1

#include "libnetdata/libnetdata.h"

extern int web_enable_gzip, web_gzip_level, web_gzip_strategy;

#define HTTP_REQ_MAX_HEADER_FETCH_TRIES 100

extern int respect_web_browser_do_not_track_policy;
extern char *web_x_frame_options;

typedef enum web_client_mode {
    WEB_CLIENT_MODE_GET = 0,
    WEB_CLIENT_MODE_POST = 1,
    WEB_CLIENT_MODE_FILECOPY = 2,
    WEB_CLIENT_MODE_OPTIONS = 3,
    WEB_CLIENT_MODE_STREAM = 4,
} WEB_CLIENT_MODE;

typedef enum {
    HTTP_VALIDATION_OK,
    HTTP_VALIDATION_NOT_SUPPORTED,
    HTTP_VALIDATION_TOO_MANY_READ_RETRIES,
    HTTP_VALIDATION_EXCESS_REQUEST_DATA,
    HTTP_VALIDATION_MALFORMED_URL,
    HTTP_VALIDATION_INCOMPLETE,
#ifdef ENABLE_HTTPS
    HTTP_VALIDATION_REDIRECT
#endif
} HTTP_VALIDATION;

typedef enum web_client_flags {
    WEB_CLIENT_FLAG_DEAD = 1 << 1, // if set, this client is dead

    WEB_CLIENT_FLAG_KEEPALIVE = 1 << 2, // if set, the web client will be re-used

    WEB_CLIENT_FLAG_WAIT_RECEIVE = 1 << 3, // if set, we are waiting more input data
    WEB_CLIENT_FLAG_WAIT_SEND = 1 << 4,    // if set, we have data to send to the client

    WEB_CLIENT_FLAG_DO_NOT_TRACK = 1 << 5,      // if set, we should not set cookies on this client
    WEB_CLIENT_FLAG_TRACKING_REQUIRED = 1 << 6, // if set, we need to send cookies

    WEB_CLIENT_FLAG_TCP_CLIENT = 1 << 7,  // if set, the client is using a TCP socket
    WEB_CLIENT_FLAG_UNIX_CLIENT = 1 << 8, // if set, the client is using a UNIX socket

    WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET = 1 << 9, // don't close the socket when cleaning up (static-threaded web server)

    WEB_CLIENT_CHUNKED_TRANSFER = 1 << 10, // chunked transfer (used with zlib compression)

    WEB_CLIENT_FLAG_SSL_WAIT_RECEIVE = 1 << 11, // if set, we are waiting more input data from an ssl conn
    WEB_CLIENT_FLAG_SSL_WAIT_SEND = 1 << 12,    // if set, we have data to send to the client from an ssl conn

    WEB_CLIENT_FLAG_PROXY_HTTPS = 1 << 13, // if set, the client reaches us via an https proxy
} WEB_CLIENT_FLAGS;

#define web_client_flag_check(w, flag) ((w)->flags & (flag))
#define web_client_flag_set(w, flag) (w)->flags |= flag
#define web_client_flag_clear(w, flag) (w)->flags &= ~flag

#define WEB_CLIENT_IS_DEAD(w) web_client_flag_set(w, WEB_CLIENT_FLAG_DEAD)
#define web_client_check_dead(w) web_client_flag_check(w, WEB_CLIENT_FLAG_DEAD)

#define web_client_has_keepalive(w) web_client_flag_check(w, WEB_CLIENT_FLAG_KEEPALIVE)
#define web_client_enable_keepalive(w) web_client_flag_set(w, WEB_CLIENT_FLAG_KEEPALIVE)
#define web_client_disable_keepalive(w) web_client_flag_clear(w, WEB_CLIENT_FLAG_KEEPALIVE)

#define web_client_has_donottrack(w) web_client_flag_check(w, WEB_CLIENT_FLAG_DO_NOT_TRACK)
#define web_client_enable_donottrack(w) web_client_flag_set(w, WEB_CLIENT_FLAG_DO_NOT_TRACK)
#define web_client_disable_donottrack(w) web_client_flag_clear(w, WEB_CLIENT_FLAG_DO_NOT_TRACK)

#define web_client_has_tracking_required(w) web_client_flag_check(w, WEB_CLIENT_FLAG_TRACKING_REQUIRED)
#define web_client_enable_tracking_required(w) web_client_flag_set(w, WEB_CLIENT_FLAG_TRACKING_REQUIRED)
#define web_client_disable_tracking_required(w) web_client_flag_clear(w, WEB_CLIENT_FLAG_TRACKING_REQUIRED)

#define web_client_has_wait_receive(w) web_client_flag_check(w, WEB_CLIENT_FLAG_WAIT_RECEIVE)
#define web_client_enable_wait_receive(w) web_client_flag_set(w, WEB_CLIENT_FLAG_WAIT_RECEIVE)
#define web_client_disable_wait_receive(w) web_client_flag_clear(w, WEB_CLIENT_FLAG_WAIT_RECEIVE)

#define web_client_has_wait_send(w) web_client_flag_check(w, WEB_CLIENT_FLAG_WAIT_SEND)
#define web_client_enable_wait_send(w) web_client_flag_set(w, WEB_CLIENT_FLAG_WAIT_SEND)
#define web_client_disable_wait_send(w) web_client_flag_clear(w, WEB_CLIENT_FLAG_WAIT_SEND)

#define web_client_has_ssl_wait_receive(w) web_client_flag_check(w, WEB_CLIENT_FLAG_SSL_WAIT_RECEIVE)
#define web_client_enable_ssl_wait_receive(w) web_client_flag_set(w, WEB_CLIENT_FLAG_SSL_WAIT_RECEIVE)
#define web_client_disable_ssl_wait_receive(w) web_client_flag_clear(w, WEB_CLIENT_FLAG_SSL_WAIT_RECEIVE)

#define web_client_has_ssl_wait_send(w) web_client_flag_check(w, WEB_CLIENT_FLAG_SSL_WAIT_SEND)
#define web_client_enable_ssl_wait_send(w) web_client_flag_set(w, WEB_CLIENT_FLAG_SSL_WAIT_SEND)
#define web_client_disable_ssl_wait_send(w) web_client_flag_clear(w, WEB_CLIENT_FLAG_SSL_WAIT_SEND)

#define web_client_set_tcp(w) web_client_flag_set(w, WEB_CLIENT_FLAG_TCP_CLIENT)
#define web_client_set_unix(w) web_client_flag_set(w, WEB_CLIENT_FLAG_UNIX_CLIENT)
#define web_client_check_unix(w) web_client_flag_check(w, WEB_CLIENT_FLAG_UNIX_CLIENT)
#define web_client_check_tcp(w) web_client_flag_check(w, WEB_CLIENT_FLAG_TCP_CLIENT)

#define web_client_is_corkable(w) web_client_flag_check(w, WEB_CLIENT_FLAG_TCP_CLIENT)

#define NETDATA_WEB_REQUEST_URL_SIZE 65536              // static allocation

#define NETDATA_WEB_RESPONSE_ZLIB_CHUNK_SIZE 16384

#define NETDATA_WEB_RESPONSE_HEADER_INITIAL_SIZE 4096
#define NETDATA_WEB_RESPONSE_INITIAL_SIZE 8192
#define NETDATA_WEB_REQUEST_INITIAL_SIZE 8192
#define NETDATA_WEB_REQUEST_MAX_SIZE 65536
#define NETDATA_WEB_DECODED_URL_INITIAL_SIZE 512

struct response {
    BUFFER *header;         // our response header
    BUFFER *header_output;  // internal use
    BUFFER *data;           // our response data buffer

    short int code;         // the HTTP response code
    bool has_cookies;

    size_t rlen; // if non-zero, the excepted size of ifd (input of firecopy)
    size_t sent; // current data length sent to output

    bool zoutput; // if set to 1, web_client_send() will send compressed data

    bool zinitialized;
    z_stream zstream;                                    // zlib stream for sending compressed output to client
    size_t zsent;                                        // the compressed bytes we have sent to the client
    size_t zhave;                                        // the compressed bytes that we have received from zlib
    Bytef zbuffer[NETDATA_WEB_RESPONSE_ZLIB_CHUNK_SIZE]; // temporary buffer for storing compressed output
};

struct web_client;
typedef bool (*web_client_interrupt_t)(struct web_client *, void *data);

struct web_client {
    unsigned long long id;
    size_t use_count;

    WEB_CLIENT_FLAGS flags;             // status flags for the client
    WEB_CLIENT_MODE mode;               // the operational mode of the client
    WEB_CLIENT_ACL acl;                 // the access list of the client
    int port_acl;                       // the operations permitted on the port the client connected to
    size_t header_parse_tries;
    size_t header_parse_last_size;

    bool tcp_cork;
    int ifd;
    int ofd;

    char client_ip[INET6_ADDRSTRLEN];   // Defined buffer sizes include null-terminators
    char client_port[NI_MAXSERV];
    char client_host[NI_MAXHOST];

    BUFFER *url_as_received;            // the entire URL as received, used for logging - DO NOT MODIFY
    BUFFER *url_path_decoded;           // the path, decoded - it is incrementally parsed and altered
    BUFFER *url_query_string_decoded;   // the query string, decoded - it is incrementally parsed and altered

    // THESE NEED TO BE FREED
    char *auth_bearer_token;            // the Bearer auth token (if sent)
    char *server_host;                  // the Host: header
    char *forwarded_host;               // the X-Forwarded-For: header
    char *origin;                       // the Origin: header
    char *user_agent;                   // the User-Agent: header

    char *post_payload;                 // when this request is a POST, this has the payload
    size_t post_payload_size;           // the size of the buffer allocated for the payload
                                        // the actual contents may be less than the size

    // STATIC-THREADED WEB SERVER MEMBERS
    size_t pollinfo_slot;               // POLLINFO slot of the web client
    size_t pollinfo_filecopy_slot;      // POLLINFO slot of the file read

#ifdef ENABLE_HTTPS
    NETDATA_SSL ssl;
#endif

    struct {                            // A callback to check if the query should be interrupted / stopped
        web_client_interrupt_t callback;
        void *callback_data;
    } interrupt;

    struct {
        size_t received_bytes;
        size_t sent_bytes;
        size_t *memory_accounting;      // temporary pointer for constructor to use
    } statistics;

    struct {
        usec_t timeout_ut;              // timeout if set, or zero
        struct timeval tv_in;           // request received
        struct timeval tv_ready;        // request processed - response ready
        struct timeval tv_timeout_last_checkpoint; // last checkpoint
    } timings;

    struct {
        struct web_client *prev;
        struct web_client *next;
    } cache;

    struct response response;
};

int web_client_permission_denied(struct web_client *w);

ssize_t web_client_send(struct web_client *w);
ssize_t web_client_receive(struct web_client *w);
ssize_t web_client_read_file(struct web_client *w);

void web_client_process_request(struct web_client *w);
void web_client_request_done(struct web_client *w);

void buffer_data_options2string(BUFFER *wb, uint32_t options);

int mysendfile(struct web_client *w, char *filename);

void web_client_build_http_header(struct web_client *w);
char *strip_control_characters(char *url);

void web_client_zero(struct web_client *w);
struct web_client *web_client_create(size_t *statistics_memory_accounting);
void web_client_free(struct web_client *w);

#include "web/api/web_api_v1.h"
#include "web/api/web_api_v2.h"
#include "daemon/common.h"

void web_client_decode_path_and_query_string(struct web_client *w, const char *path_and_query_string);
int web_client_api_request(RRDHOST *host, struct web_client *w, char *url_path_fragment);
const char *web_content_type_to_string(HTTP_CONTENT_TYPE content_type);
void web_client_enable_deflate(struct web_client *w, int gzip);
int web_client_api_request_with_node_selection(RRDHOST *host, struct web_client *w, char *decoded_url_path);

void web_client_timeout_checkpoint_init(struct web_client *w);
void web_client_timeout_checkpoint_set(struct web_client *w, int timeout_ms);
usec_t web_client_timeout_checkpoint(struct web_client *w);
bool web_client_timeout_checkpoint_and_check(struct web_client *w, usec_t *usec_since_last_checkpoint);
usec_t web_client_timeout_checkpoint_response_ready(struct web_client *w, usec_t *usec_since_last_checkpoint);

#endif
