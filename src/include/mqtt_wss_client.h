// Copyright (C) 2020 Timotej Šiškovič
// SPDX-License-Identifier: GPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free Software Foundation, version 3.
//
// This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY;
// without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with this program.
// If not, see <https://www.gnu.org/licenses/>.

#ifndef MQTT_WSS_CLIENT_H
#define MQTT_WSS_CLIENT_H

#include <stdint.h>
#include <stddef.h> //size_t

#include "mqtt_wss_log.h"
#include "common_public.h"

// All OK call me at your earliest convinience
#define MQTT_WSS_OK              0
/* All OK, poll timeout you requested when calling mqtt_wss_service expired - you might want to know if timeout
 * happened or we got some data or handle same as MQTT_WSS_OK
 */
#define MQTT_WSS_OK_TO           1
// Connection was closed by remote
#define MQTT_WSS_ERR_CONN_DROP  -1
// Error in MQTT protocol (e.g. malformed packet)
#define MQTT_WSS_ERR_PROTO_MQTT -2
// Error in WebSocket protocol (e.g. malformed packet)
#define MQTT_WSS_ERR_PROTO_WS   -3

#define MQTT_WSS_ERR_TX_BUF_TOO_SMALL  -4
#define MQTT_WSS_ERR_RX_BUF_TOO_SMALL  -5

#define MQTT_WSS_ERR_CANT_SEND_NOW -6
#define MQTT_WSS_ERR_BLOCK_TIMEOUT -7
// if client was initialized with MQTT 3 but MQTT 5 feature
// was requested by user of library
#define MQTT_WSS_ERR_CANT_DO       -8

typedef struct mqtt_wss_client_struct *mqtt_wss_client;

typedef void (*msg_callback_fnc_t)(const char *topic, const void *msg, size_t msglen, int qos);
/* Creates new instance of MQTT over WSS. Doesn't start connection.
 * @param log_prefix this is prefix to be used when logging to discern between multiple
 *        mqtt_wss instances. Can be NULL.
 * @param log_callback is function pointer to fnc to be called when mqtt_wss wants
 *        to log. This allows plugging this library into your own logging system/solution.
 *        If NULL STDOUT/STDERR will be used.
 * @param msg_callback is function pointer to function which will be called
 *        when application level message arrives from broker (for subscribed topics).
 *        Can be NULL if you are not interested about incoming messages.
 * @param puback_callback is function pointer to function to be called when QOS1 Publish
 *        is acknowledged by server
 */
mqtt_wss_client mqtt_wss_new(const char *log_prefix,
                             mqtt_wss_log_callback_t log_callback,
                             msg_callback_fnc_t msg_callback,
                             void (*puback_callback)(uint16_t packet_id),
                             int mqtt5);

void mqtt_wss_set_max_buf_size(mqtt_wss_client client, size_t size);

int mqtt_wss_able_to_send(mqtt_wss_client client, size_t bytes);

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
int mqtt_wss_connect(mqtt_wss_client client, char *host, int port, struct mqtt_connect_params *mqtt_params, int ssl_flags, struct mqtt_wss_proxy *proxy);
int mqtt_wss_service(mqtt_wss_client client, int timeout_ms);
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

/* Publishes MQTT message
 * @param client mqtt_wss_client which should transfer the message
 * @param topic MQTT topic to publish message to (0 terminated C string)
 * @param msg Message to be published (no need for 0 termination)
 * @param msg_len Length of the message to be published
 * @param publish_flags see enum mqtt_wss_publish_flags e.g. (MQTT_WSS_PUB_QOS1 | MQTT_WSS_PUB_RETAIN)
 * @return Returns 0 on success
 */
int mqtt_wss_publish(mqtt_wss_client client, const char *topic, const void *msg, int msg_len, uint8_t publish_flags);

/* Publishes MQTT message and gets message id
 * @param client mqtt_wss_client which should transfer the message
 * @param topic MQTT topic to publish message to (0 terminated C string)
 * @param msg Message to be published (no need for 0 termination)
 * @param msg_len Length of the message to be published
 * @param publish_flags see enum mqtt_wss_publish_flags e.g. (MQTT_WSS_PUB_QOS1 | MQTT_WSS_PUB_RETAIN)
 * @param packet_id is 16 bit unsigned int representing ID that can be used to pair with PUBACK callback
 *        for usages where application layer wants to know which messages are delivered when
 * @return Returns 0 on success
 */
int mqtt_wss_publish_pid(mqtt_wss_client client, const char *topic, const void *msg, int msg_len, uint8_t publish_flags, uint16_t *packet_id);

int mqtt_wss_publish_pid_block(mqtt_wss_client client, const char *topic, const void *msg, int msg_len, uint8_t publish_flags, uint16_t *packet_id, int timeout_ms);

/* Publishes MQTT 5 message
 */
int mqtt_wss_publish5(mqtt_wss_client client,
                      char *topic,
                      free_fnc_t topic_free,
                      void *msg,
                      free_fnc_t msg_free,
                      size_t msg_len,
                      uint8_t publish_flags,
                      uint16_t *packet_id);

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
};

struct mqtt_wss_stats mqtt_wss_get_stats(mqtt_wss_client client);

#ifdef MQTT_WSS_DEBUG
#include <openssl/ssl.h>
void mqtt_wss_set_SSL_CTX_keylog_cb(mqtt_wss_client client, void (*ssl_ctx_keylog_cb)(const SSL *ssl, const char *line));
#endif

#endif /* MQTT_WSS_CLIENT_H */
