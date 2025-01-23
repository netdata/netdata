// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef MQTT_WSS_CLIENT_H
#define MQTT_WSS_CLIENT_H

#include "common_public.h"

#define MQTT_WSS_OK                 0       // All OK call me at your earliest convinience
#define MQTT_WSS_OK_TO              1       // All OK, poll timeout you requested when calling mqtt_wss_service expired
                                            //you might want to know if timeout
                                            //happened or we got some data or handle same as MQTT_WSS_OK
#define MQTT_WSS_ERR_CONN_DROP      -1      // Connection was closed by remote
#define MQTT_WSS_ERR_PROTO_MQTT     -2      // Error in MQTT protocol (e.g. malformed packet)
#define MQTT_WSS_ERR_PROTO_WS       -3      // Error in WebSocket protocol (e.g. malformed packet)
#define MQTT_WSS_ERR_MSG_TOO_BIG    -6      // Message size too big for server
#define MQTT_WSS_ERR_CANT_DO        -8      // if client was initialized with MQTT 3 but MQTT 5 feature
                                            // was requested by user of library
#define MQTT_WSS_ERR_POLL_FAILED    -9
#define MQTT_WSS_ERR_REMOTE_CLOSED  -10

typedef struct mqtt_wss_client_struct *mqtt_wss_client;

typedef void (*msg_callback_fnc_t)(const char *topic, const void *msg, size_t msglen, int qos);

/* Creates new instance of MQTT over WSS. Doesn't start connection.
 * @param msg_callback is function pointer to function which will be called
 *        when application level message arrives from broker (for subscribed topics).
 *        Can be NULL if you are not interested about incoming messages.
 * @param puback_callback is function pointer to function to be called when QOS1 Publish
 *        is acknowledged by server
 */
mqtt_wss_client mqtt_wss_new(
    msg_callback_fnc_t msg_callback,
    void (*puback_callback)(uint16_t packet_id));

void mqtt_wss_set_max_buf_size(mqtt_wss_client client, size_t size);

void mqtt_wss_destroy(mqtt_wss_client client);

struct mqtt_connect_params;
struct mqtt_wss_proxy;

#define MQTT_WSS_SSL_CERT_CHECK_FULL   0x00
#define MQTT_WSS_SSL_ALLOW_SELF_SIGNED 0x01
#define MQTT_WSS_SSL_DONT_CHECK_CERTS  0x08

/* Will block until the MQTT over WSS connection is established or return error
 * @param client mqtt_wss_client which should connect
 * @param host to connect to (where MQTT over WSS server is listening)
 * @param port to connect to (where MQTT over WSS server is listening)
 * @param mqtt_params pointer to mqtt_connect_params structure which contains MQTT credentials and settings
 * @param ssl_flags parameters for OpenSSL, 0=MQTT_WSS_SSL_CERT_CHECK_FULL
 */
int mqtt_wss_connect(
    mqtt_wss_client client,
    char *host,
    int port,
    struct mqtt_connect_params *mqtt_params,
    int ssl_flags,
    const struct mqtt_wss_proxy *proxy,
    bool *fallback_ipv4);
int mqtt_wss_service(mqtt_wss_client client, int t_ms);
void mqtt_wss_disconnect(mqtt_wss_client client, int timeout_ms);

// we redefine this instead of using MQTT-C flags as in future
// we want to support different MQTT implementations if needed
enum mqtt_wss_publish_flags {
    MQTT_WSS_PUB_QOS0    = 0x0,
    MQTT_WSS_PUB_QOS1    = 0x1,
    MQTT_WSS_PUB_QOS2    = 0x2,
    MQTT_WSS_PUB_QOSMASK = 0x3,
    MQTT_WSS_PUB_RETAIN  = 0x4
};

struct mqtt_connect_params {
    const char *clientid;
    const char *username;
    const char *password;
    const char *will_topic;
    const void *will_msg;
    enum mqtt_wss_publish_flags will_flags;
    size_t will_msg_len;
    int keep_alive;
    int drop_on_publish_fail;
};

enum mqtt_wss_proxy_type {
    MQTT_WSS_DIRECT = 0,
    MQTT_WSS_PROXY_HTTP
};

struct mqtt_wss_proxy {
    enum mqtt_wss_proxy_type type;
    const char *host;
    int port;
    const char *username;
    const char *password;
};

/* TODO!!! update the description
 * Publishes MQTT message and gets message id
 * @param client mqtt_wss_client which should transfer the message
 * @param topic MQTT topic to publish message to (0 terminated C string)
 * @param msg Message to be published (no need for 0 termination)
 * @param msg_len Length of the message to be published
 * @param publish_flags see enum mqtt_wss_publish_flags e.g. (MQTT_WSS_PUB_QOS1 | MQTT_WSS_PUB_RETAIN)
 * @param packet_id is 16 bit unsigned int representing ID that can be used to pair with PUBACK callback
 *        for usages where application layer wants to know which messages are delivered when
 * @return Returns 0 on success
 */
int mqtt_wss_publish5(mqtt_wss_client client,
                      char *topic,
                      free_fnc_t topic_free,
                      void *msg,
                      free_fnc_t msg_free,
                      size_t msg_len,
                      uint8_t publish_flags,
                      uint16_t *packet_id);

int mqtt_wss_set_topic_alias(mqtt_wss_client client, const char *topic);

/* Subscribes to MQTT topic
 * @param client mqtt_wss_client which should do the subscription
 * @param topic MQTT topic to subscribe to
 * @param max_qos_level maximum QOS level that broker can send to us on this subscription
 * @return Returns 0 on success
 */
int mqtt_wss_subscribe(mqtt_wss_client client, char *topic, int max_qos_level);


struct mqtt_wss_stats {
    uint64_t bytes_tx;
    uint64_t bytes_rx;
#ifdef MQTT_WSS_CPUSTATS
    uint64_t time_keepalive;
    uint64_t time_read_socket;
    uint64_t time_write_socket;
    uint64_t time_process_websocket;
    uint64_t time_process_mqtt;
#endif
    struct mqtt_ng_stats mqtt;
};

struct mqtt_wss_stats mqtt_wss_get_stats(mqtt_wss_client client);
void mqtt_wss_reset_stats(mqtt_wss_client client);

#ifdef MQTT_WSS_DEBUG
#include <openssl/ssl.h>
void mqtt_wss_set_SSL_CTX_keylog_cb(mqtt_wss_client client, void (*ssl_ctx_keylog_cb)(const SSL *ssl, const char *line));
#endif

#endif /* MQTT_WSS_CLIENT_H */
