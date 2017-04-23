#ifndef NETDATA_WEB_CLIENT_H
#define NETDATA_WEB_CLIENT_H 1

#define DEFAULT_DISCONNECT_IDLE_WEB_CLIENTS_AFTER_SECONDS 60
extern int web_client_timeout;

#ifdef NETDATA_WITH_ZLIB
extern int web_enable_gzip,
        web_gzip_level,
        web_gzip_strategy;
#endif /* NETDATA_WITH_ZLIB */

extern int respect_web_browser_do_not_track_policy;
extern char *web_x_frame_options;

typedef enum web_client_mode {
    WEB_CLIENT_MODE_NORMAL      = 0,
    WEB_CLIENT_MODE_FILECOPY    = 1,
    WEB_CLIENT_MODE_OPTIONS     = 2,
    WEB_CLIENT_MODE_STREAM      = 3
} WEB_CLIENT_MODE;

#define URL_MAX 8192
#define ZLIB_CHUNK  16384
#define HTTP_RESPONSE_HEADER_SIZE 4096
#define COOKIE_MAX 1024
#define ORIGIN_MAX 1024

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
    Bytef zbuffer[ZLIB_CHUNK];      // temporary buffer for storing compressed output
    size_t zsent;                   // the compressed bytes we have sent to the client
    size_t zhave;                   // the compressed bytes that we have received from zlib
    int zinitialized:1;
#endif /* NETDATA_WITH_ZLIB */

};

struct web_client {
    unsigned long long id;

    uint8_t obsolete:1;                 // if set to 1, the listener will remove this client
                                        // after setting this to 1, you should not touch
                                        // this web_client

    uint8_t dead:1;                     // if set to 1, this client is dead

    uint8_t keepalive:1;                // if set to 1, the web client will be re-used

    uint8_t wait_receive:1;             // 1 = we are waiting more input data
    uint8_t wait_send:1;                // 1 = we have data to send to the client

    uint8_t donottrack:1;               // 1 = we should not set cookies on this client
    uint8_t tracking_required:1;        // 1 = if the request requires cookies

    WEB_CLIENT_MODE mode;               // the operational mode of the client

    int tcp_cork;                       // 1 = we have a cork on the socket

    int ifd;
    int ofd;

    char client_ip[NI_MAXHOST+1];
    char client_port[NI_MAXSERV+1];

    char decoded_url[URL_MAX + 1];  // we decode the URL in this buffer
    char last_url[URL_MAX+1];       // we keep a copy of the decoded URL here

    struct timeval tv_in, tv_ready;

    char cookie1[COOKIE_MAX+1];
    char cookie2[COOKIE_MAX+1];
    char origin[ORIGIN_MAX+1];

    struct response response;

    size_t stats_received_bytes;
    size_t stats_sent_bytes;

    pthread_t thread;               // the thread servicing this client

    struct web_client *prev;
    struct web_client *next;
};

#define WEB_CLIENT_IS_DEAD(w) (w)->dead=1

extern struct web_client *web_clients;

extern uid_t web_files_uid(void);
extern uid_t web_files_gid(void);

extern struct web_client *web_client_create(int listener);
extern struct web_client *web_client_free(struct web_client *w);
extern ssize_t web_client_send(struct web_client *w);
extern ssize_t web_client_receive(struct web_client *w);
extern void web_client_process_request(struct web_client *w);
extern void web_client_reset(struct web_client *w);

extern void *web_client_main(void *ptr);

extern int web_client_api_request_v1_data_group(char *name, int def);
extern const char *group_method2string(int group);

extern void buffer_data_options2string(BUFFER *wb, uint32_t options);

extern int mysendfile(struct web_client *w, char *filename);

#endif
