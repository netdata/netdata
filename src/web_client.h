#ifndef NETDATA_WEB_CLIENT_H
#define NETDATA_WEB_CLIENT_H 1

/**
 * @file web_client.h
 * @brief API of the web client.
 */

#define DEFAULT_DISCONNECT_IDLE_WEB_CLIENTS_AFTER_SECONDS 60 ///< default timeout to disconnect idle web clients.
extern int web_client_timeout;                               ///< seconds to disconnect timeouts.

#ifdef NETDATA_WITH_ZLIB
extern int web_enable_gzip;      ///< enable gzip compressed web traffic
extern int web_gzip_level;       ///< gzip level to use
extern int web_gzip_strategy;    ///< gzip strategy to use
#endif /* NETDATA_WITH_ZLIB */

extern int respect_web_browser_do_not_track_policy;
extern char *web_x_frame_options;

#define WEB_CLIENT_MODE_NORMAL      0 ///< Standard web client mode.
#define WEB_CLIENT_MODE_FILECOPY    1 ///< Deliver a static file to the web client
#define WEB_CLIENT_MODE_OPTIONS     2 ///< HTTP OPTIONS request. Used in CORS.

#define URL_MAX 8192                   ///< Maximum URL length
#define ZLIB_CHUNK  16384              ///< zlib chunk size
#define HTTP_RESPONSE_HEADER_SIZE 4096 ///< http response header size
#define COOKIE_MAX 1024                ///< maximum cookie size
#define ORIGIN_MAX 1024                ///< maximum origin size

/** Web client response message. */
struct response {
    BUFFER *header;                 ///< our response header
    BUFFER *header_output;          ///< internal use
    BUFFER *data;                   ///< our response data buffer

    int code;                       ///< the HTTP response code

    size_t rlen;                    ///< if non-zero, the excepted size of ifd (input of firecopy)
    size_t sent;                    ///< current data length sent to output

    int zoutput;                    ///< if set to 1, web_client_send() will send compressed data
#ifdef NETDATA_WITH_ZLIB
    z_stream zstream;               ///< zlib stream for sending compressed output to client
    Bytef zbuffer[ZLIB_CHUNK];      ///< temporary buffer for storing compressed output
    size_t zsent;                   ///< the compressed bytes we have sent to the client
    size_t zhave;                   ///< the compressed bytes that we have received from zlib
    int zinitialized:1;             ///< boolean. Compression Library initialzied.
#endif /* NETDATA_WITH_ZLIB */

};

/** Doubly linked list of web clients */
struct web_client {
    unsigned long long id;              ///< web client id

    uint8_t obsolete:1;                 ///< if set to 1, the listener will remove this client
                                        ///< after setting this to 1, you should not touch
                                        ///< this web_client

    uint8_t dead:1;                     ///< if set to 1, this client is dead

    uint8_t keepalive:1;                ///< if set to 1, the web client will be re-used

    uint8_t mode:3;                     ///< the operational mode of the client

    uint8_t wait_receive:1;             ///< 1 = we are waiting more input data
    uint8_t wait_send:1;                ///< 1 = we have data to send to the client

    uint8_t donottrack:1;               ///< 1 = we should not set cookies on this client
    uint8_t tracking_required:1;        ///< 1 = if the request requires cookies

    int tcp_cork;                       ///< 1 = we have a cork on the socket

    int ifd; ///< Input socket
    int ofd; ///< Output socket

    char client_ip[NI_MAXHOST+1];   ///< IP address of the client
    char client_port[NI_MAXSERV+1]; ///< Port of the client

    char decoded_url[URL_MAX + 1];  ///< we decode the URL in this buffer
    char last_url[URL_MAX+1];       ///< we keep a copy of the decoded URL here

    struct timeval tv_in;    ///< Start time processing the request.
    struct timeval tv_ready; ///< Response ready. Time finshed processing the request.

    char cookie1[COOKIE_MAX+1]; ///< Host cookie used by the registry to track web browsers.
    char cookie2[COOKIE_MAX+1]; ///< Domain cookie used by the registry to track web browsers.
    char origin[ORIGIN_MAX+1];  ///< The origin HTTP header. Enables CORS for this specific origin instead of *.

    struct sockaddr_storage clientaddr; ///< address of the client
    struct response response;           ///< response message

    size_t stats_received_bytes; ///< recived bytes
    size_t stats_sent_bytes;     ///< sent bytes

    pthread_t thread;               ///< the thread servicing this client

    struct web_client *prev; ///< previous item in list
    struct web_client *next; ///< next intem in list
};

/**
 * Test if web client is dead.
 *
 * @param w Web client to test.
 * @return boolean
 */
#define WEB_CLIENT_IS_DEAD(w) (w)->dead=1

/** Doubly linked list of web clients */
extern struct web_client *web_clients;

/** 
 * Get the real user id of the web file owner.
 *
 * @return user id
 */
extern uid_t web_files_uid(void);
/** 
 * Get the real group id of the web file owner.
 *
 * @return user id
 */
extern uid_t web_files_gid(void);

/**
 * Create a new web client
 *
 * @param listener File descriptor.
 * @return the web client
 */
extern struct web_client *web_client_create(int listener);
/**
 * Free web client created with web_client_create().
 *
 * @param w The web client.
 * @return the next web client in the list.
 */
extern struct web_client *web_client_free(struct web_client *w);
/**
 * Perform a send operation.
 *
 * @param w Web client.
 * @return bytes sent.
 */
extern ssize_t web_client_send(struct web_client *w);
/**
 * Perform a receive operation.
 *
 * @param w Web client.
 * @return bytes received.
 */
extern ssize_t web_client_receive(struct web_client *w);
/**
 * Process a query.
 *
 * @param w Web client.
 */
extern void web_client_process(struct web_client *w);
/**
 * Reset a web client.
 *
 * @param w Web client.
 */
extern void web_client_reset(struct web_client *w);

/**
 * The thread of a single client.
 *
 * 1. waits for input and output, using async I/O
 * 2. it processes HTTP requests
 * 3. it generates HTTP responses
 * 4. it copies data from input to output if mode is FILECOPY
 *
 * @param ptr struct web_client
 */
extern void *web_client_main(void *ptr);

/**
 * Get data group for name.
 *
 * If not found, return `def`
 *
 * @param name of data group
 * @param def Default.
 * @return GROUP_*
 *
 * @see rrd2json.h
 */
extern int web_client_api_request_v1_data_group(char *name, int def);
/**
 * Get data group name for group.
 *
 * @param group GROUP_
 * @return a string representing `group`
 *
 * @see rrd2json.h
 */
extern const char *group_method2string(int group);

/**
 * Convert option into string and store it in buffer.
 *
 * @param wb BUFFER to write to.
 * @param options RRDR_OPTION_*
 *
 * @see rrd2json.h
 */
extern void buffer_data_options2string(BUFFER *wb, uint32_t options);
#endif
