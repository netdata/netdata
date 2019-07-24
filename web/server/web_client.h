// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_CLIENT_H
#define NETDATA_WEB_CLIENT_H 1

#include "libnetdata/libnetdata.h"

#ifdef NETDATA_WITH_ZLIB
extern int web_enable_gzip,
        web_gzip_level,
        web_gzip_strategy;
#endif /* NETDATA_WITH_ZLIB */

//HTTP_CODES 4XX
#define HTTP_RESPONSE_BAD_REQUEST   400

extern int respect_web_browser_do_not_track_policy;
extern char *web_x_frame_options;

typedef enum web_client_mode {
    WEB_CLIENT_MODE_NORMAL      = 0,
    WEB_CLIENT_MODE_FILECOPY    = 1,
    WEB_CLIENT_MODE_OPTIONS     = 2,
    WEB_CLIENT_MODE_STREAM      = 3
} WEB_CLIENT_MODE;

typedef enum {
    HTTP_VALIDATION_OK,
    HTTP_VALIDATION_NOT_SUPPORTED,
    HTTP_VALIDATION_MALFORMED_URL,
#ifdef ENABLE_HTTPS
    HTTP_VALIDATION_INCOMPLETE,
    HTTP_VALIDATION_REDIRECT
#else
    HTTP_VALIDATION_INCOMPLETE
#endif
} HTTP_VALIDATION;

typedef enum web_client_flags {
    WEB_CLIENT_FLAG_DEAD              = 1 << 1, // if set, this client is dead

    WEB_CLIENT_FLAG_KEEPALIVE         = 1 << 2, // if set, the web client will be re-used

    WEB_CLIENT_FLAG_WAIT_RECEIVE      = 1 << 3, // if set, we are waiting more input data
    WEB_CLIENT_FLAG_WAIT_SEND         = 1 << 4, // if set, we have data to send to the client

    WEB_CLIENT_FLAG_DO_NOT_TRACK      = 1 << 5, // if set, we should not set cookies on this client
    WEB_CLIENT_FLAG_TRACKING_REQUIRED = 1 << 6, // if set, we need to send cookies

    WEB_CLIENT_FLAG_TCP_CLIENT        = 1 << 7, // if set, the client is using a TCP socket
    WEB_CLIENT_FLAG_UNIX_CLIENT       = 1 << 8, // if set, the client is using a UNIX socket

    WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET = 1 << 9,  // don't close the socket when cleaning up (static-threaded web server)
} WEB_CLIENT_FLAGS;

//#ifdef HAVE_C___ATOMIC
//#define web_client_flag_check(w, flag) (__atomic_load_n(&((w)->flags), __ATOMIC_SEQ_CST) & flag)
//#define web_client_flag_set(w, flag)   __atomic_or_fetch(&((w)->flags), flag, __ATOMIC_SEQ_CST)
//#define web_client_flag_clear(w, flag) __atomic_and_fetch(&((w)->flags), ~flag, __ATOMIC_SEQ_CST)
//#else
#define web_client_flag_check(w, flag) ((w)->flags & (flag))
#define web_client_flag_set(w, flag)   (w)->flags |= flag
#define web_client_flag_clear(w, flag) (w)->flags &= ~flag
//#endif

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

#define web_client_set_tcp(w) web_client_flag_set(w, WEB_CLIENT_FLAG_TCP_CLIENT)
#define web_client_set_unix(w) web_client_flag_set(w, WEB_CLIENT_FLAG_UNIX_CLIENT)
#define web_client_check_unix(w) web_client_flag_check(w, WEB_CLIENT_FLAG_UNIX_CLIENT)
#define web_client_check_tcp(w) web_client_flag_check(w, WEB_CLIENT_FLAG_TCP_CLIENT)

#define web_client_is_corkable(w) web_client_flag_check(w, WEB_CLIENT_FLAG_TCP_CLIENT)

#define NETDATA_WEB_REQUEST_URL_SIZE 8192
#define NETDATA_WEB_RESPONSE_ZLIB_CHUNK_SIZE 16384
#define NETDATA_WEB_RESPONSE_HEADER_SIZE 4096
#define NETDATA_WEB_REQUEST_COOKIE_SIZE 1024
#define NETDATA_WEB_REQUEST_ORIGIN_HEADER_SIZE 1024
#define NETDATA_WEB_RESPONSE_INITIAL_SIZE 16384
#define NETDATA_WEB_REQUEST_RECEIVE_SIZE 16384
#define NETDATA_WEB_REQUEST_MAX_SIZE 16384

struct response {
    BUFFER *header;                 // our response header
    BUFFER *header_output;          // internal use
    BUFFER *data;                   // our response data buffer

    int code;                       // the HTTP response code

    size_t rlen;                    // if non-zero, the excepted size of ifd (input of firecopy)
    size_t sent;                    // current data length sent to output

    int zoutput;                    // if set to 1, web_client_send() will send compressed data
#ifdef NETDATA_WITH_ZLIB
    z_stream zstream;               // zlib stream for sending compressed output to client
    Bytef zbuffer[NETDATA_WEB_RESPONSE_ZLIB_CHUNK_SIZE]; // temporary buffer for storing compressed output
    size_t zsent;                   // the compressed bytes we have sent to the client
    size_t zhave;                   // the compressed bytes that we have received from zlib
    unsigned int zinitialized:1;
#endif /* NETDATA_WITH_ZLIB */

};

struct web_client {
    unsigned long long id;

    WEB_CLIENT_FLAGS flags;         // status flags for the client
    WEB_CLIENT_MODE mode;           // the operational mode of the client
    WEB_CLIENT_ACL acl;             // the access list of the client
    int port_acl;                   // the operations permitted on the port the client connected to
    char *auth_bearer_token;        // the Bearer auth token (if sent)
    size_t header_parse_tries;
    size_t header_parse_last_size;

    int tcp_cork;                   // 1 = we have a cork on the socket

    int ifd;
    int ofd;

    char client_ip[NI_MAXHOST+1];
    char client_port[NI_MAXSERV+1];

    char decoded_url[NETDATA_WEB_REQUEST_URL_SIZE + 1];  // we decode the URL in this buffer
    char decoded_query_string[NETDATA_WEB_REQUEST_URL_SIZE + 1];  // we decode the Query String in this buffer
    char last_url[NETDATA_WEB_REQUEST_URL_SIZE+1];       // we keep a copy of the decoded URL here
    char host[256];
    size_t url_path_length;
    char separator; // This value can be either '?' or 'f'
    char *url_search_path; //A pointer to the search path sent by the client

    struct timeval tv_in, tv_ready;

    char cookie1[NETDATA_WEB_REQUEST_COOKIE_SIZE+1];
    char cookie2[NETDATA_WEB_REQUEST_COOKIE_SIZE+1];
    char origin[NETDATA_WEB_REQUEST_ORIGIN_HEADER_SIZE+1];
    char *user_agent;

    struct response response;

    size_t stats_received_bytes;
    size_t stats_sent_bytes;

    // cache of web_client allocations
    struct web_client *prev;        // maintain a linked list of web clients
    struct web_client *next;        // for the web servers that need it

    // MULTI-THREADED WEB SERVER MEMBERS
    netdata_thread_t thread;        // the thread servicing this client
    volatile int running;           // 1 when the thread runs, 0 otherwise

    // STATIC-THREADED WEB SERVER MEMBERS
    size_t pollinfo_slot;           // POLLINFO slot of the web client
    size_t pollinfo_filecopy_slot;  // POLLINFO slot of the file read
#ifdef ENABLE_HTTPS
    struct netdata_ssl ssl;
#endif
};


extern uid_t web_files_uid(void);
extern uid_t web_files_gid(void);

extern int web_client_permission_denied(struct web_client *w);

extern ssize_t web_client_send(struct web_client *w);
extern ssize_t web_client_receive(struct web_client *w);
extern ssize_t web_client_read_file(struct web_client *w);

extern void web_client_process_request(struct web_client *w);
extern void web_client_request_done(struct web_client *w);

extern void buffer_data_options2string(BUFFER *wb, uint32_t options);

extern int mysendfile(struct web_client *w, char *filename);

#include "daemon/common.h"

#endif
