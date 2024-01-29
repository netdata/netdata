// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_CLIENT_H
#define NETDATA_WEB_CLIENT_H 1

#include "libnetdata/libnetdata.h"

struct web_client;

extern int web_enable_gzip, web_gzip_level, web_gzip_strategy;

#define HTTP_REQ_MAX_HEADER_FETCH_TRIES 100

extern int respect_web_browser_do_not_track_policy;
extern char *web_x_frame_options;

typedef enum __attribute__((packed)) {
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

typedef enum __attribute__((packed)) {
    WEB_CLIENT_FLAG_DEAD                    = (1 << 0), // this client is dead

    WEB_CLIENT_FLAG_KEEPALIVE               = (1 << 1), // the web client will be re-used

    // compression
    WEB_CLIENT_ENCODING_GZIP                = (1 << 2),
    WEB_CLIENT_ENCODING_DEFLATE             = (1 << 3),
    WEB_CLIENT_CHUNKED_TRANSFER             = (1 << 4), // chunked transfer (used with zlib compression)

    WEB_CLIENT_FLAG_WAIT_RECEIVE            = (1 << 5), // we are waiting more input data
    WEB_CLIENT_FLAG_WAIT_SEND               = (1 << 6), // we have data to send to the client
    WEB_CLIENT_FLAG_SSL_WAIT_RECEIVE        = (1 << 7), // we are waiting more input data from ssl connection
    WEB_CLIENT_FLAG_SSL_WAIT_SEND           = (1 << 8), // we have data to send to the client from ssl connection

    // DNT
    WEB_CLIENT_FLAG_DO_NOT_TRACK            = (1 << 9), // we should not set cookies on this client
    WEB_CLIENT_FLAG_TRACKING_REQUIRED       = (1 << 10), // we need to send cookies

    // connection type
    WEB_CLIENT_FLAG_CONN_TCP                = (1 << 11), // the client is using a TCP socket
    WEB_CLIENT_FLAG_CONN_UNIX               = (1 << 12), // the client is using a UNIX socket
    WEB_CLIENT_FLAG_CONN_CLOUD              = (1 << 13), // the client is using Netdata Cloud
    WEB_CLIENT_FLAG_CONN_WEBRTC             = (1 << 14), // the client is using WebRTC

    // streaming
    WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET       = (1 << 15), // don't close the socket when cleaning up

    // dashboard version
    WEB_CLIENT_FLAG_PATH_IS_V0              = (1 << 16), // v0 dashboard found on the path
    WEB_CLIENT_FLAG_PATH_IS_V1              = (1 << 17), // v1 dashboard found on the path
    WEB_CLIENT_FLAG_PATH_IS_V2              = (1 << 18), // v2 dashboard found on the path
    WEB_CLIENT_FLAG_PATH_HAS_TRAILING_SLASH = (1 << 19), // the path has a trailing hash
    WEB_CLIENT_FLAG_PATH_HAS_FILE_EXTENSION = (1 << 20), // the path ends with a filename extension

    // authorization
    WEB_CLIENT_FLAG_AUTH_CLOUD              = (1 << 21),
    WEB_CLIENT_FLAG_AUTH_BEARER             = (1 << 22),
    WEB_CLIENT_FLAG_AUTH_GOD                = (1 << 23),

    // transient settings
    WEB_CLIENT_FLAG_PROGRESS_TRACKING       = (1 << 24), // flag to avoid redoing progress work
} WEB_CLIENT_FLAGS;

#define WEB_CLIENT_FLAG_PATH_WITH_VERSION (WEB_CLIENT_FLAG_PATH_IS_V0|WEB_CLIENT_FLAG_PATH_IS_V1|WEB_CLIENT_FLAG_PATH_IS_V2)
#define web_client_reset_path_flags(w) (w)->flags &= ~(WEB_CLIENT_FLAG_PATH_WITH_VERSION|WEB_CLIENT_FLAG_PATH_HAS_TRAILING_SLASH|WEB_CLIENT_FLAG_PATH_HAS_FILE_EXTENSION)

#define web_client_flag_check(w, flag) ((w)->flags & (flag))
#define web_client_flag_set(w, flag) (w)->flags |= (flag)
#define web_client_flag_clear(w, flag) (w)->flags &= ~(flag)

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

#define web_client_check_conn_unix(w) web_client_flag_check(w, WEB_CLIENT_FLAG_CONN_UNIX)
#define web_client_check_conn_tcp(w) web_client_flag_check(w, WEB_CLIENT_FLAG_CONN_TCP)
#define web_client_check_conn_cloud(w) web_client_flag_check(w, WEB_CLIENT_FLAG_CONN_CLOUD)
#define web_client_check_conn_webrtc(w) web_client_flag_check(w, WEB_CLIENT_FLAG_CONN_WEBRTC)

#define WEB_CLIENT_FLAG_ALL_AUTHS (WEB_CLIENT_FLAG_AUTH_CLOUD | WEB_CLIENT_FLAG_AUTH_BEARER)
#define web_client_flags_clear_conn(w) web_client_flag_clear(w, WEB_CLIENT_FLAG_CONN_TCP | WEB_CLIENT_FLAG_CONN_UNIX | WEB_CLIENT_FLAG_CONN_CLOUD | WEB_CLIENT_FLAG_CONN_WEBRTC)
#define web_client_flags_check_auth(w) web_client_flag_check(w, WEB_CLIENT_FLAG_ALL_AUTHS)
#define web_client_flags_clear_auth(w) web_client_flag_clear(w, WEB_CLIENT_FLAG_ALL_AUTHS)

void web_client_reset_permissions(struct web_client *w);
void web_client_set_permissions(struct web_client *w, HTTP_ACCESS access, HTTP_USER_ROLE role, WEB_CLIENT_FLAGS auth);

void web_client_set_conn_tcp(struct web_client *w);
void web_client_set_conn_unix(struct web_client *w);
void web_client_set_conn_cloud(struct web_client *w);
void web_client_set_conn_webrtc(struct web_client *w);

#define NETDATA_WEB_REQUEST_URL_SIZE 65536              // static allocation

#define NETDATA_WEB_RESPONSE_ZLIB_CHUNK_SIZE 16384

#define NETDATA_WEB_RESPONSE_HEADER_INITIAL_SIZE 4096
#define NETDATA_WEB_RESPONSE_INITIAL_SIZE 8192
#define NETDATA_WEB_REQUEST_INITIAL_SIZE 8192
#define NETDATA_WEB_REQUEST_MAX_SIZE 65536
#define NETDATA_WEB_DECODED_URL_INITIAL_SIZE 512

#define CLOUD_USER_NAME_LENGTH 64

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

    uuid_t transaction;

    WEB_CLIENT_FLAGS flags;             // status flags for the client
    HTTP_REQUEST_MODE mode;             // the operational mode of the client
    HTTP_ACL acl;                       // the access list of the client
    HTTP_ACL port_acl;                  // the operations permitted on the port the client connected to
    HTTP_ACCESS access;                 // the access permissions of the client
    HTTP_USER_ROLE user_role;           // the user role of the client
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
    char *forwarded_host;               // the X-Forwarded-Host: header
    char *forwarded_for;                // the X-Forwarded-For: header
    char *origin;                       // the Origin: header
    char *user_agent;                   // the User-Agent: header

    BUFFER *payload;                    // when this request is a POST, this has the payload

    // STATIC-THREADED WEB SERVER MEMBERS
    size_t pollinfo_slot;               // POLLINFO slot of the web client
    size_t pollinfo_filecopy_slot;      // POLLINFO slot of the file read

#ifdef ENABLE_HTTPS
    NETDATA_SSL ssl;
#endif

    struct {
        uuid_t bearer_token;
        uuid_t cloud_account_id;
        char client_name[CLOUD_USER_NAME_LENGTH];
    } auth;

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
int web_client_permission_denied_acl(struct web_client *w);

int web_client_service_unavailable(struct web_client *w);

ssize_t web_client_send(struct web_client *w);
ssize_t web_client_receive(struct web_client *w);
ssize_t web_client_read_file(struct web_client *w);

void web_client_process_request_from_web_server(struct web_client *w);
void web_client_request_done(struct web_client *w);

void buffer_data_options2string(BUFFER *wb, uint32_t options);

void web_client_build_http_header(struct web_client *w);

void web_client_reuse_from_cache(struct web_client *w);
struct web_client *web_client_create(size_t *statistics_memory_accounting);
void web_client_free(struct web_client *w);

#include "web/api/web_api_v1.h"
#include "web/api/web_api_v2.h"
#include "daemon/common.h"

void web_client_decode_path_and_query_string(struct web_client *w, const char *path_and_query_string);
int web_client_api_request(RRDHOST *host, struct web_client *w, char *url_path_fragment);
int web_client_api_request_with_node_selection(RRDHOST *host, struct web_client *w, char *decoded_url_path);

void web_client_timeout_checkpoint_init(struct web_client *w);
void web_client_timeout_checkpoint_set(struct web_client *w, int timeout_ms);
usec_t web_client_timeout_checkpoint(struct web_client *w);
bool web_client_timeout_checkpoint_and_check(struct web_client *w, usec_t *usec_since_last_checkpoint);
usec_t web_client_timeout_checkpoint_response_ready(struct web_client *w, usec_t *usec_since_last_checkpoint);
void web_client_log_completed_request(struct web_client *w, bool update_web_stats);

HTTP_VALIDATION http_request_validate(struct web_client *w);

#endif
