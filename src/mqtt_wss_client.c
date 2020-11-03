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
#define _GNU_SOURCE

#include "mqtt_wss_client.h"
#include "mqtt.h"
#include "ws_client.h"

#include <stdlib.h>
#include <fcntl.h>
#include <unistd.h>
#include <poll.h>
#include <string.h>

#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <netinet/tcp.h> //TCP_NODELAY
#include <netdb.h>

#include <openssl/err.h>
#include <openssl/ssl.h>

#define PIPE_READ_END  0
#define PIPE_WRITE_END 1
#define POLLFD_SOCKET  0
#define POLLFD_PIPE    1

char *util_openssl_ret_err(int err)
{
    switch(err){
        case SSL_ERROR_WANT_READ:
            return "SSL_ERROR_WANT_READ";
        case SSL_ERROR_WANT_WRITE:
            return "SSL_ERROR_WANT_WRITE";
        case SSL_ERROR_NONE:
            return "SSL_ERROR_NONE";
        case SSL_ERROR_ZERO_RETURN:
            return "SSL_ERROR_ZERO_RETURN";
        case SSL_ERROR_WANT_CONNECT:
            return "SSL_ERROR_WANT_CONNECT";
        case SSL_ERROR_WANT_ACCEPT:
            return "SSL_ERROR_WANT_ACCEPT";
    }
    return "Unknown!!!";
}

struct mqtt_wss_client {
    ws_client *ws_client;

    mqtt_wss_log_ctx_t log;

    char *host;
    int port;

// nonblock IO related
    int sockfd;
    int write_notif_pipe[2];
    struct pollfd poll_fds[2];

    SSL_CTX *ssl_ctx;
    SSL *ssl;

    struct mqtt_client *mqtt_client;
    uint8_t *mqtt_send_buf;
    uint8_t *mqtt_recv_buf;

// signifies that we didn't write all MQTT wanted
// us to write during last cycle (e.g. due to buffer
// size) and thus we should arm POLLOUT
    int mqtt_didnt_finish_write:1;

    int mqtt_connected:1;

// Application layer callback pointers
    void (*msg_callback)(const char *, const void *, size_t, int);
    void (*puback_callback)(uint16_t packet_id);
};

static void mws_connack_callback(struct mqtt_client* client, enum MQTTConnackReturnCode code)
{
    switch(code) {
        case MQTT_CONNACK_ACCEPTED:
            mws_debug(client->socketfd->log, "MQTT Connection Accepted");
            client->socketfd->mqtt_connected = 1;
            return;
        case MQTT_CONNACK_REFUSED_PROTOCOL_VERSION:
            mws_error(client->socketfd->log, "MQTT Connection refused \"Unsuported Protocol Version\"");
            return;
        case MQTT_CONNACK_REFUSED_IDENTIFIER_REJECTED:
            mws_error(client->socketfd->log, "MQTT Connection refused \"The Client identifier is correct UTF-8 but not allowed by the Server\"");
            return;
        case MQTT_CONNACK_REFUSED_SERVER_UNAVAILABLE:
            mws_error(client->socketfd->log, "MQTT Connection refused \"The Network Connection has been made but the MQTT service is unavailable\"");
            return;
        case MQTT_CONNACK_REFUSED_BAD_USER_NAME_OR_PASSWORD:
            mws_error(client->socketfd->log, "MQTT Connection refused \"The data in the user name or password is malformed\"");
            return;
        case MQTT_CONNACK_REFUSED_NOT_AUTHORIZED:
            mws_error(client->socketfd->log, "MQTT Connection refused \"The Client is not authorized to connect\"");
            return;
        default:
            mws_fatal(client->socketfd->log, "MQTT Unknown CONNACK code");
    }
}

static void mws_puback_callback(struct mqtt_client* client, uint16_t packet_id)
{
#ifdef DEBUG_ULTRA_VERBOSE
    mws_debug(client->socketfd->log, "PUBACK Received for %d", packet_id);
#endif
    if (client->socketfd->puback_callback)
        client->socketfd->puback_callback(packet_id);
}


// TODO create permanent buffer in mqtt_wss_client
// to not need so much stack
#define TOPIC_MAX 512
static void mqtt_rx_msg_callback(void **state, struct mqtt_response_publish *publish)
{
    mqtt_wss_client client = *state;
    char topic[TOPIC_MAX];
    size_t len = (publish->topic_name_size < TOPIC_MAX - 1) ? publish->topic_name_size : (TOPIC_MAX - 1);
    memcpy(topic,
           publish->topic_name,
           len);
    topic[len] = 0;

#ifdef DEBUG_ULTRA_VERBOSE
    mws_debug(client->log, "Got message on topic \"%s\" size %d", topic, (int)publish->application_message_size);
#endif
    if (client->msg_callback)
        client->msg_callback(topic, publish->application_message, publish->application_message_size, publish->qos_level);
}

// TODO modify the MQTT to grow the buffer as needed (with min/max)
// of look for MQTT implementation that doesn't need to fit whole message into the buffer
// at once or write custom MQTT... MQTT is not that complex anyway
// TODO pretify the callbacks -> move them into cb struct so that mqtt_wss_new parameter list
// doesn't look like Windows nightmare API
#define MQTT_BUFFER_SIZE 1024*1024*3
mqtt_wss_client mqtt_wss_new(const char *log_prefix,
                             mqtt_wss_log_callback_t log_callback,
                             void (*msg_callback)(const char *topic, const void *msg, size_t msglen, int qos),
                             void (*puback_callback)(uint16_t packet_id))
{
    enum MQTTErrors ret;
    mqtt_wss_log_ctx_t log;

    log = mqtt_wss_log_ctx_create(log_prefix, log_callback);
    if(!log)
        return NULL;

    SSL_library_init();
    SSL_load_error_strings();

    mqtt_wss_client client = calloc(1, sizeof(struct mqtt_wss_client));
    if (!client) {
        mws_error(log, "OOM alocating mqtt_wss_client");
        goto fail;
    }
    client->msg_callback = msg_callback;
    client->puback_callback = puback_callback;

    client->ws_client = ws_client_new(0, &client->host, log);
    if (!client->ws_client) {
        mws_error(log, "Error creating ws_client");
        goto fail_1;
    }

    client->log = log;

    if (pipe2(client->write_notif_pipe, O_CLOEXEC /*| O_DIRECT*/)) {
        mws_error(log, "Couldn't create pipe");
        goto fail_2;
    }

    client->poll_fds[POLLFD_PIPE].fd = client->write_notif_pipe[PIPE_READ_END];
    client->poll_fds[POLLFD_PIPE].events = POLLIN;

    client->poll_fds[POLLFD_SOCKET].events = POLLIN;

    // MQTT related
    // TODO maybe move that to sub struct
    client->mqtt_client = calloc(1, sizeof(struct mqtt_client));
    if (!client->mqtt_client)
        goto fail_3;
    client->mqtt_send_buf = malloc(MQTT_BUFFER_SIZE);
    if (!client->mqtt_send_buf)
        goto fail_4;
    client->mqtt_recv_buf = malloc(MQTT_BUFFER_SIZE);
    if (!client->mqtt_send_buf)
        goto fail_5;

    ret = mqtt_init(client->mqtt_client,
                    client, client->mqtt_send_buf,
                    MQTT_BUFFER_SIZE,
                    client->mqtt_recv_buf,
                    MQTT_BUFFER_SIZE,
                    (msg_callback ? mqtt_rx_msg_callback : NULL)
          );
    if (ret != MQTT_OK) {
        mws_error(log, "Error initializing MQTT \"%s\"", mqtt_error_str(ret));
        goto fail_6;
    }

    client->mqtt_client->publish_response_callback_state = client;
    client->mqtt_client->connack_callback = mws_connack_callback;
    client->mqtt_client->puback_callback = mws_puback_callback;

    return client;

fail_6:
    free(client->mqtt_recv_buf);
fail_5:
    free(client->mqtt_send_buf);
fail_4:
    free(client->mqtt_client);
fail_3:
    close(client->write_notif_pipe[PIPE_WRITE_END]);
    close(client->write_notif_pipe[PIPE_READ_END]);
fail_2:
    ws_client_destroy(client->ws_client);
fail_1:
    free(client);
fail:
    mqtt_wss_log_ctx_destroy(log);
    return NULL;
}

int mqtt_wss_connect(mqtt_wss_client client, char *host, int port, struct mqtt_connect_params *mqtt_params)
{
    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);

    struct hostent *he;
    struct in_addr **addr_list;

    if (!mqtt_params) {
        mws_error(client->log, "mqtt_params can't be null!");
        return -1;
    }

    if(client->host)
        free(client->host);
    client->host = strdup(host);
    client->port = port;

    //TODO gethostbyname -> getaddinfo
    //     hstrerror -> gai_strerror
    if ((he = gethostbyname(host)) == NULL) {
        mws_error(client->log, "gethostbyname() error \"%s\"", hstrerror(h_errno));
        return -1;
    }

    addr_list = (struct in_addr **)he->h_addr_list;
    if(!addr_list[0]) {
        mws_error(client->log, "No IP addr resolved");
        return -1;
    }
    mws_debug(client->log, "Resolved IP: %s", inet_ntoa(*addr_list[0]));
    addr.sin_addr = *addr_list[0];

    client->sockfd = socket(AF_INET, SOCK_STREAM, 0);
    if (client->sockfd < 0) {
        mws_error(client->log, "Couldn't create socket()");
        return -1;
    }

    int flag = 1;
    int result = setsockopt(client->sockfd,
                            IPPROTO_TCP,
                            TCP_NODELAY,
                            &flag,
                            sizeof(int));
    if (result < 0)
       mws_error(client->log, "Could not dissable NAGLE");

    if (connect(client->sockfd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        mws_error(client->log, "Could not connect to remote endpoint \"%s\", port %d.\n", host, port);
        return -3;
    }

    client->poll_fds[POLLFD_SOCKET].fd = client->sockfd;

    fcntl(client->sockfd, F_SETFL, fcntl(client->sockfd, F_GETFL, 0) | O_NONBLOCK);

    OPENSSL_init_ssl(0, 0);

    client->ssl_ctx = SSL_CTX_new(SSLv23_client_method());
    client->ssl = SSL_new(client->ssl_ctx);
    SSL_set_fd(client->ssl, client->sockfd);
    SSL_set_connect_state(client->ssl);
    SSL_connect(client->ssl);

    enum MQTTErrors ret = mqtt_connect(client->mqtt_client,
                                       mqtt_params->clientid,
                                       mqtt_params->will_topic,
                                       mqtt_params->will_msg,
                                       (mqtt_params->will_msg ? mqtt_params->will_msg_len : 0),
                                       mqtt_params->username,
                                       mqtt_params->password,
                                       MQTT_CONNECT_CLEAN_SESSION,
                                       400);
    if (ret != MQTT_OK) {
        mws_error(client->log, "Error with MQTT connect \"%s\"", mqtt_error_str(ret));
        return 1;
    }

    // wait till MQTT connection is established
    while (!client->mqtt_connected) {
        if(mqtt_wss_service(client, -1)) {
            mws_error(client->log, "Error connecting to MQTT WSS server \"%s\", port &d.", host, port);
            return 2;
        }
    }

    return 0;
}

static inline void mqtt_wss_wakeup(mqtt_wss_client client)
{
#ifdef DEBUG_ULTRA_VERBOSE
    mws_debug(client->log, "mqtt_wss_wakup - forcing wake up of main loop");
#endif
    write(client->write_notif_pipe[PIPE_WRITE_END], " ", 1);
}

#define THROWAWAY_BUF_SIZE 32
char throwaway[THROWAWAY_BUF_SIZE];
static inline void util_clear_pipe(int fd)
{
    read(fd, throwaway, THROWAWAY_BUF_SIZE);
}

static inline void set_socket_pollfds(mqtt_wss_client client, int ssl_ret) {
    if (ssl_ret == SSL_ERROR_WANT_WRITE)
        client->poll_fds[POLLFD_SOCKET].events |= POLLOUT;
    if (ssl_ret == SSL_ERROR_WANT_READ)
        client->poll_fds[POLLFD_SOCKET].events |= POLLIN;
}

// TODO handle case if timeout_ms > MQTT keep alive!
// shoud be easy but keeping it as it is for now
// as we will use timeout_ms of 1s for now
int mqtt_wss_service(mqtt_wss_client client, int timeout_ms)
{
    char *ptr;
    size_t size;
    int ret;

#ifdef DEBUG_ULTRA_VERBOSE
    mws_debug(client->log, ">>>>> mqtt_wss_service <<<<<");
    mws_debug(client->log, "Waiting for events: %s%s%s",
        (client->poll_fds[POLLFD_SOCKET].events & POLLIN)  ? "SOCKET_POLLIN " : "",
        (client->poll_fds[POLLFD_SOCKET].events & POLLOUT) ? "SOCKET_POLLOUT " : "",
        (client->poll_fds[POLLFD_PIPE].events & POLLIN) ? "PIPE_POLLIN" : "" );
#endif

    if ((ret = poll(client->poll_fds, 2, timeout_ms > 0 ? timeout_ms : -1)) < 0) {
        mws_error(client->log, "poll error \"%s\"", strerror(errno));
        return -2;
    }
    // if poll TO expires we don't want to do anything differently
    if (ret == 0)
        return 0;

    client->poll_fds[POLLFD_SOCKET].events = 0;

#ifdef DEBUG_ULTRA_VERBOSE
    mws_debug(client->log, "Poll events happened: %s%s%s",
        (client->poll_fds[POLLFD_SOCKET].revents & POLLIN)  ? "SOCKET_POLLIN " : "",
        (client->poll_fds[POLLFD_SOCKET].revents & POLLOUT) ? "SOCKET_POLLOUT " : "",
        (client->poll_fds[POLLFD_PIPE].revents & POLLIN) ? "PIPE_POLLIN" : "" );
#endif

    if ((ptr = rbuf_get_linear_insert_range(client->ws_client->buf_read, &size))) {
        if((ret = SSL_read(client->ssl, ptr, size)) > 0) {
#ifdef DEBUG_ULTRA_VERBOSE
            mws_debug(client->log, "SSL_Read: Read %d.", ret);
#endif
            rbuf_bump_head(client->ws_client->buf_read, ret);
        } else {
            ret = SSL_get_error(client->ssl, ret);
#ifdef DEBUG_ULTRA_VERBOSE
            mws_debug(client->log, "Read Err: %s", util_openssl_ret_err(ret));
#endif
            set_socket_pollfds(client, ret);
            if (ret == SSL_ERROR_ZERO_RETURN)
                return -1;
        }
    }

    ret = ws_client_process(client->ws_client);
    if (ret == WS_CLIENT_PROTOCOL_ERROR)
        return 1;
    if (ret == WS_CLIENT_NEED_MORE_BYTES) {
#ifdef DEBUG_ULTRA_VERBOSE
        mws_debug(client->log, "WSCLIENT WANT READ");
#endif
        client->poll_fds[POLLFD_SOCKET].events |= POLLIN;
    }

    if(client->ws_client->state == WS_ESTABLISHED) {
        // we need to call this only if there has been some movements
        // - read side is handled by POLLIN and ws_client_process
        // - write side is handled by PIPE write every time we send MQTT
        // message ensuring we wake up from poll
        enum MQTTErrors mqtt_ret = mqtt_sync(client->mqtt_client);
        if (mqtt_ret != MQTT_OK) {
            mws_error(client->log, "Error mqtt_sync MQTT \"%s\"", mqtt_error_str(mqtt_ret));
            client->mqtt_connected = 0;
            return 1;
        }
        if (client->mqtt_didnt_finish_write) {
            client->mqtt_didnt_finish_write = 0;
            client->poll_fds[POLLFD_SOCKET].events |= POLLOUT;
        }
    }

    if ((ptr = rbuf_get_linear_read_range(client->ws_client->buf_write, &size))) {
#ifdef DEBUG_ULTRA_VERBOSE
        mws_debug(client->log, "Have data to write to SSL");
#endif
        if ((ret = SSL_write(client->ssl, ptr, size)) > 0) {
#ifdef DEBUG_ULTRA_VERBOSE
            mws_debug(client->log, "SSL_Write: Written %d.", ret);
#endif
            rbuf_bump_tail(client->ws_client->buf_write, ret);
        } else {
            ret = SSL_get_error(client->ssl, ret);
#ifdef DEBUG_ULTRA_VERBOSE
            mws_debug(client->log, "Write Err: %s", util_openssl_ret_err(ret));
#endif
            set_socket_pollfds(client, ret);
        }
    }

    if(client->poll_fds[POLLFD_PIPE].revents & POLLIN)
        util_clear_pipe(client->write_notif_pipe[PIPE_READ_END]);

    return 0;
}

ssize_t mqtt_pal_sendall(mqtt_pal_socket_handle mqtt_wss_client, const void* buf, size_t len, int flags)
{
    (void)flags;
#ifdef DEBUG_ULTRA_VERBOSE
    mws_debug(mqtt_wss_client->log, "mqtt_pal_sendall(len=%d)", len);
#endif
    int ret = ws_client_send(mqtt_wss_client->ws_client, buf, len);
    if (ret >= 0 && (size_t)ret != len) {
#ifdef DEBUG_ULTRA_VERBOSE
        mws_debug(mqtt_wss_client->log, "Not complete message sent (Msg=%d,Sent=%d). Need to arm POLLOUT!", len, ret);
#endif
        mqtt_wss_client->mqtt_didnt_finish_write = 1;
    }
    return ret;
}

ssize_t mqtt_pal_recvall(mqtt_pal_socket_handle mqtt_wss_client, void* buf, size_t bufsz, int flags)
{
    (void)flags;
#ifdef DEBUG_ULTRA_VERBOSE
    mws_debug(mqtt_wss_client->log, "mqtt_pal_rcvall()");
    size_t size;
    if ((size = rbuf_pop(mqtt_wss_client->ws_client->buf_to_mqtt, buf, bufsz)))
        mws_debug(mqtt_wss_client->log, "Passing data to MQTT: %d bytes", (int)size);
    return size;
#else
    return rbuf_pop(mqtt_wss_client->ws_client->buf_to_mqtt, buf, bufsz);
#endif
}

int mqtt_wss_publish_pid(mqtt_wss_client client, const char *topic, const void *msg, int msg_len, uint8_t publish_flags, uint16_t *packet_id)
{
    int ret;
    uint8_t mqtt_flags = 0;

    if (!client->mqtt_connected) {
        mws_error(client->log, "MQTT is offline. Can't send message.");
        return 1;
    }

    mqtt_flags = (publish_flags & MQTT_WSS_PUB_QOSMASK) << 1;
    if (publish_flags & MQTT_WSS_PUB_RETAIN)
        mqtt_flags |= MQTT_PUBLISH_RETAIN;

    ret = mqtt_publish_pid(client->mqtt_client, topic, msg, msg_len, mqtt_flags, packet_id);
    if (ret != MQTT_OK) {
        mws_error(client->log, "Error Publishing MQTT msg. Desc: \"%s\"", mqtt_error_str(ret));
        return 1;
    }

#ifdef DEBUG_ULTRA_VERBOSE
    mws_debug(client->log, "Publishing Message to topic \"%s\" with size %d as packet_id=%d", topic, msg_len, *packet_id);
#endif

    mqtt_wss_wakeup(client);
    return 0;
}

int mqtt_wss_publish(mqtt_wss_client client, const char *topic, const void *msg, int msg_len, uint8_t publish_flags)
{
    uint16_t pid;
    return mqtt_wss_publish_pid(client, topic, msg, msg_len, publish_flags, &pid);
}

int mqtt_wss_subscribe(mqtt_wss_client client, const char *topic, int max_qos_level)
{
    int ret;

    if (!client->mqtt_connected) {
        mws_error(client->log, "MQTT is offline. Can't subscribe.");
        return 1;
    }

    ret = mqtt_subscribe(client->mqtt_client, topic, max_qos_level);
    if (ret != MQTT_OK) {
        mws_error(client->log, "Error Subscribing. Desc: \"%s\"", mqtt_error_str(ret));
        return 1;
    }

    mqtt_wss_wakeup(client);
    return 0;
}
