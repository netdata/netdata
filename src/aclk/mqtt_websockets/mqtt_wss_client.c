// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include "libnetdata/libnetdata.h"
#include "aclk_mqtt_workers.h"
#include "mqtt_wss_client.h"
#include "mqtt_ng.h"
#include "ws_client.h"
#include "common_internal.h"
#include "../aclk.h"
#include "../aclk_util.h"

#define PIPE_READ_END  0
#define PIPE_WRITE_END 1
#define POLLFD_SOCKET  0
#define POLLFD_PIPE    1

#define PING_TIMEOUT    (60)  //Expect a ping response within this time (seconds)
time_t ping_timeout = 0;

#if (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110) && (SSLEAY_VERSION_NUMBER >= OPENSSL_VERSION_097)
#include <openssl/conf.h>
#endif

//TODO MQTT_PUBLISH_RETAIN should not be needed anymore
#define MQTT_PUBLISH_RETAIN 0x01
#define MQTT_CONNECT_CLEAN_SESSION 0x02
#define MQTT_CONNECT_WILL_RETAIN 0x20

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
        case SSL_ERROR_WANT_X509_LOOKUP:
            return "SSL_ERROR_WANT_X509_LOOKUP";
#ifdef SSL_ERROR_WANT_ASYNC
        case SSL_ERROR_WANT_ASYNC:
            return "SSL_ERROR_WANT_ASYNC";
#endif
#ifdef SSL_ERROR_WANT_ASYNC_JOB
        case SSL_ERROR_WANT_ASYNC_JOB:
            return "SSL_ERROR_WANT_ASYNC_JOB";
#endif
#ifdef SSL_ERROR_WANT_CLIENT_HELLO_CB
        case SSL_ERROR_WANT_CLIENT_HELLO_CB:
            return "SSL_ERROR_WANT_CLIENT_HELLO_CB";
#endif
        case SSL_ERROR_SYSCALL:
            return "SSL_ERROR_SYSCALL";
        case SSL_ERROR_SSL:
            return "SSL_ERROR_SSL";
        default:
            break;
    }
    return "UNKNOWN";
}

struct mqtt_wss_client_struct {
    ws_client *ws_client;

// immediate connection (e.g. proxy server)
    char *host; 
    int port;

// target of connection (e.g. where we want to connect to)
    char *target_host;
    int target_port;

    enum mqtt_wss_proxy_type proxy_type;
    char *proxy_uname;
    char *proxy_passwd;

// nonblock IO related
    int sockfd;
    int write_notif_pipe[2];
    struct pollfd poll_fds[2];

    SSL_CTX *ssl_ctx;
    SSL *ssl;
    int ssl_flags;

    struct mqtt_ng_client *mqtt;

    int mqtt_keepalive;

// signifies that we didn't write all MQTT wanted
// us to write during last cycle (e.g. due to buffer
// size) and thus we should arm POLLOUT
    unsigned int mqtt_didnt_finish_write:1;

    unsigned int mqtt_connected:1;
    unsigned int mqtt_disconnecting:1;

// Application layer callback pointers
    void (*msg_callback)(const char *, const void *, size_t, int);
    void (*puback_callback)(uint16_t packet_id);

    SPINLOCK stat_lock;
    struct mqtt_wss_stats stats;

#ifdef MQTT_WSS_DEBUG
    void (*ssl_ctx_keylog_cb)(const SSL *ssl, const char *line);
#endif
};

static void mws_connack_callback_ng(void *user_ctx, int code)
{
    mqtt_wss_client client = user_ctx;
    switch(code) {
        case 0:
            client->mqtt_connected = 1;
            break;
//TODO manual labor: all the CONNACK error codes with some nice error message
        default:
            nd_log(NDLS_DAEMON, NDLP_ERR, "MQTT CONNACK returned error %d", code);
            break;
    }
}

static ssize_t mqtt_send_cb(void *user_ctx, const void* buf, size_t len)
{
    mqtt_wss_client client = user_ctx;
    int ret = ws_client_send(client->ws_client, WS_OP_BINARY_FRAME, buf, len);
    if (ret >= 0 && (size_t)ret != len)
        client->mqtt_didnt_finish_write = 1;
    return ret;
}

mqtt_wss_client mqtt_wss_new(
    msg_callback_fnc_t msg_callback,
    void (*puback_callback)(uint16_t packet_id))
{
    SSL_library_init();
    SSL_load_error_strings();

    mqtt_wss_client client = callocz(1, sizeof(struct mqtt_wss_client_struct));

    spinlock_init(&client->stat_lock);

    client->msg_callback = msg_callback;
    client->puback_callback = puback_callback;

    client->ws_client = ws_client_new(0, &client->target_host);
    if (!client->ws_client) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Error creating ws_client");
        goto fail_1;
    }

#ifdef __APPLE__
    if (pipe(client->write_notif_pipe)) {
#else
    if (pipe2(client->write_notif_pipe, O_CLOEXEC /*| O_DIRECT*/)) {
#endif
        nd_log(NDLS_DAEMON, NDLP_ERR, "Couldn't create pipe");
        goto fail_2;
    }

    client->poll_fds[POLLFD_PIPE].fd = client->write_notif_pipe[PIPE_READ_END];
    client->poll_fds[POLLFD_PIPE].events = POLLIN;

    client->poll_fds[POLLFD_SOCKET].events = POLLIN;

    struct mqtt_ng_init settings = {
        .data_in = client->ws_client->buf_to_mqtt,
        .data_out_fnc = &mqtt_send_cb,
        .user_ctx = client,
        .connack_callback = &mws_connack_callback_ng,
        .puback_callback = puback_callback,
        .msg_callback = msg_callback
    };
    client->mqtt = mqtt_ng_init(&settings);

    return client;

fail_2:
    ws_client_destroy(client->ws_client);
fail_1:
    freez(client);
    return NULL;
}

void mqtt_wss_set_max_buf_size(mqtt_wss_client client, size_t size)
{
    mqtt_ng_set_max_mem(client->mqtt, size);
}

void mqtt_wss_destroy(mqtt_wss_client client)
{
    mqtt_ng_destroy(client->mqtt);

    close(client->write_notif_pipe[PIPE_WRITE_END]);
    close(client->write_notif_pipe[PIPE_READ_END]);

    ws_client_destroy(client->ws_client);

    // deleted after client->ws_client
    // as it "borrows" this pointer and might use it
    if (client->target_host == client->host)
        client->target_host = NULL;

    if (client->target_host)
        freez(client->target_host);

    if (client->host)
        freez(client->host);

    aclk_sensitive_free(&client->proxy_passwd);
    freez(client->proxy_uname);

    if (client->ssl)
        SSL_free(client->ssl);
    
    if (client->ssl_ctx)
        SSL_CTX_free(client->ssl_ctx);

    if (client->sockfd > 0)
        close(client->sockfd);

    freez(client);
}

static int cert_verify_callback(int preverify_ok, X509_STORE_CTX *ctx)
{
    int err = 0;

    SSL* ssl = X509_STORE_CTX_get_ex_data(ctx, SSL_get_ex_data_X509_STORE_CTX_idx());
    mqtt_wss_client client = SSL_get_ex_data(ssl, 0);

    if (!preverify_ok) {
        err = X509_STORE_CTX_get_error(ctx);
        netdata_ssl_log_verify_error(ctx);
    }

    if (!preverify_ok && (client->ssl_flags & MQTT_WSS_SSL_ALLOW_SELF_SIGNED)) {
        // MQTT_WSS_SSL_ALLOW_SELF_SIGNED means "this connection accepts a
        // certificate that wouldn't pass full validation". Cover the errors
        // that on-prem deployments routinely hit:
        //  - leaf is self-signed (no CA at all)
        //  - cert subject does not match the configured hostname/IP
        //    (DNS aliases, IP-only access, certs without proper SAN)
        switch (err) {
            case X509_V_ERR_DEPTH_ZERO_SELF_SIGNED_CERT:
                preverify_ok = 1;
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "Self Signed Certificate Accepted as the connection was "
                       "requested with MQTT_WSS_SSL_ALLOW_SELF_SIGNED");
                break;
            case X509_V_ERR_HOSTNAME_MISMATCH:
                preverify_ok = 1;
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "Certificate hostname mismatch accepted as the connection "
                       "was requested with MQTT_WSS_SSL_ALLOW_SELF_SIGNED");
                break;
            case X509_V_ERR_IP_ADDRESS_MISMATCH:
                preverify_ok = 1;
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "Certificate IP address mismatch accepted as the connection "
                       "was requested with MQTT_WSS_SSL_ALLOW_SELF_SIGNED");
                break;
            default:
                break;
        }
    }

    return preverify_ok;
}

int mqtt_wss_connect(
    mqtt_wss_client client,
    char *host,
    int port,
    struct mqtt_connect_params *mqtt_params,
    int ssl_flags,
    const struct mqtt_wss_proxy *proxy,
    bool *fallback_ipv4)
{
    if (!mqtt_params) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "mqtt_params can't be null!");
        return -1;
    }

    // reset state in case this is reconnect
    client->mqtt_didnt_finish_write = 0;
    client->mqtt_connected = 0;
    client->mqtt_disconnecting = 0;
    ws_client_reset(client->ws_client);

    if (client->target_host == client->host)
        client->target_host = NULL;

    if (client->target_host)
        freez(client->target_host);

    if (client->host)
        freez(client->host);

    if (client->proxy_uname) {
        freez(client->proxy_uname);
        client->proxy_uname = NULL;
    }

    if (client->proxy_passwd) {
        aclk_sensitive_free(&client->proxy_passwd);
    }

    if (proxy && proxy->type != MQTT_WSS_DIRECT) {
        client->host = strdupz(proxy->host);
        client->port = proxy->port;
        client->target_host = strdupz(host);
        client->target_port = port;
        client->proxy_type = proxy->type;
        if (proxy->username)
            client->proxy_uname = strdupz(proxy->username);
        if (proxy->password)
            client->proxy_passwd = strdupz(proxy->password);
    } else {
        client->host = strdupz(host);
        client->port = port;
        client->target_host = client->host;
        client->target_port = port;
        client->proxy_type = MQTT_WSS_DIRECT;
    }

    client->ssl_flags = ssl_flags;

    if (client->sockfd > 0)
        close(client->sockfd);

    char port_str[16];
    snprintf(port_str, sizeof(port_str) -1, "%d", client->port);

    if (proxy && proxy->type != MQTT_WSS_DIRECT) {
        const char *proxy_proto = aclk_mqtt_proxy_type_to_scheme(proxy->type);
        nd_log_daemon(NDLP_INFO, "ACLK: connecting to %s:%d via proxy %s%s:%d%s",
                      client->target_host, client->target_port,
                      proxy_proto, client->host, client->port,
                      client->proxy_uname ? " (with credentials)" : " (without credentials)");
    }
    else
        nd_log_daemon(NDLP_INFO, "ACLK: connecting to %s:%d (no proxy)",
                      client->target_host, client->target_port);

    struct timeval timeout = { .tv_sec = 10, .tv_usec = 0 };
    int fd = connect_to_this_ip46(IPPROTO_TCP, SOCK_STREAM, client->host, 0, port_str, &timeout, fallback_ipv4);
    if (fd < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Could not connect to remote endpoint \"%s\", port %d.\n", client->host, port);
        return -3;
    }

    client->sockfd = fd;

#ifndef SOCK_CLOEXEC
    int flags = fcntl(client->sockfd, F_GETFD);
    if (flags != -1)
        (void) fcntl(client->sockfd, F_SETFD, flags| FD_CLOEXEC);
#endif

    int flag = 1;
    int result = setsockopt(client->sockfd, IPPROTO_TCP, TCP_NODELAY, &flag, sizeof(int));
    if (result < 0)
       nd_log(NDLS_DAEMON, NDLP_ERR, "Could not dissable NAGLE");

    client->poll_fds[POLLFD_SOCKET].fd = client->sockfd;

    if (fcntl(client->sockfd, F_SETFL, fcntl(client->sockfd, F_GETFL, 0) | O_NONBLOCK) == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Error setting O_NONBLOCK to TCP socket. \"%s\"", strerror(errno));
        return -8;
    }

    if (client->proxy_type != MQTT_WSS_DIRECT) {
        if (aclk_proxy_negotiation_connect(client->sockfd, client->proxy_type, client->proxy_uname, client->proxy_passwd,
                                           client->target_host, client->target_port, 10000))
            return -4;

        // Credentials are only needed for proxy negotiation; wipe them now.
        aclk_sensitive_free(&client->proxy_passwd);
    }

#if OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
#if (SSLEAY_VERSION_NUMBER >= OPENSSL_VERSION_097)
    OPENSSL_config(NULL);
#endif
    SSL_load_error_strings();
    SSL_library_init();
#else
    if (OPENSSL_init_ssl(OPENSSL_INIT_LOAD_CONFIG, NULL) != 1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to initialize SSL");
        return -1;
    };
#endif

    // free SSL structs from possible previous connections
    if (client->ssl)
        SSL_free(client->ssl);

    if (client->ssl_ctx)
        SSL_CTX_free(client->ssl_ctx);

    client->ssl_ctx = SSL_CTX_new(SSLv23_client_method());
    if (!(client->ssl_flags & MQTT_WSS_SSL_DONT_CHECK_CERTS)) {
        SSL_CTX_set_default_verify_paths(client->ssl_ctx);
        SSL_CTX_set_verify(client->ssl_ctx, SSL_VERIFY_PEER | SSL_VERIFY_CLIENT_ONCE, cert_verify_callback);
    } else
        nd_log(NDLS_DAEMON, NDLP_ERR, "SSL Certificate checking completely disabled!!!");

#ifdef MQTT_WSS_DEBUG
    if(client->ssl_ctx_keylog_cb)
        SSL_CTX_set_keylog_callback(client->ssl_ctx, client->ssl_ctx_keylog_cb);
#endif

    client->ssl = SSL_new(client->ssl_ctx);
    if (!(client->ssl_flags & MQTT_WSS_SSL_DONT_CHECK_CERTS)) {
        if (!SSL_set_ex_data(client->ssl, 0, client)) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "Could not SSL_set_ex_data");
            return -4;
        }
    }
    SSL_set_fd(client->ssl, client->sockfd);
    SSL_set_connect_state(client->ssl);

    if (!SSL_set_tlsext_host_name(client->ssl, client->target_host)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Error setting TLS SNI host");
        return -7;
    }

    if (!(client->ssl_flags & MQTT_WSS_SSL_DONT_CHECK_CERTS)) {
        // target_host may be either a DNS hostname or an IP literal.
        // X509_VERIFY_PARAM_set1_ip_asc() parses the string as an IP and
        // matches against the cert's iPAddress SAN; it returns 0 if the
        // string is not a valid IP. X509_VERIFY_PARAM_set1_host() matches
        // against the dNSName SAN. Try the IP path first; if the input is
        // not an IP literal, fall back to hostname matching.
        X509_VERIFY_PARAM *param = SSL_get0_param(client->ssl);
        if (!X509_VERIFY_PARAM_set1_ip_asc(param, client->target_host) &&
            !X509_VERIFY_PARAM_set1_host(param, client->target_host, 0)) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "Error setting TLS hostname verification host");
            return -7;
        }
    }

    result = SSL_connect(client->ssl);
    if (result != -1 && result != 1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "SSL could not connect");
        return -5;
    }

    if (result == -1) {
        int ec = SSL_get_error(client->ssl, result);
        if (ec != SSL_ERROR_WANT_READ && ec != SSL_ERROR_WANT_WRITE) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to start SSL connection");
            return -6;
        }
    }

    client->mqtt_keepalive = (mqtt_params->keep_alive ? mqtt_params->keep_alive : 400);

    struct mqtt_auth_properties auth;
    auth.client_id = (char*)mqtt_params->clientid;
    auth.client_id_free = NULL;
    auth.username = (char*)mqtt_params->username;
    auth.username_free = NULL;
    auth.password = (char*)mqtt_params->password;
    auth.password_free = NULL;

    struct mqtt_lwt_properties lwt;
    lwt.will_topic = (char*)mqtt_params->will_topic;
    lwt.will_topic_free = NULL;
    lwt.will_message = (void*)mqtt_params->will_msg;
    lwt.will_message_free = NULL; // TODO expose no copy version to API
    lwt.will_message_size = mqtt_params->will_msg_len;
    lwt.will_qos = (int) (mqtt_params->will_flags & MQTT_WSS_PUB_QOSMASK);
    lwt.will_retain = (int) mqtt_params->will_flags & MQTT_WSS_PUB_RETAIN;

    int ret = mqtt_ng_connect(client->mqtt, &auth, mqtt_params->will_msg ? &lwt : NULL, client->mqtt_keepalive);
    if (ret) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Error generating MQTT connect");
        return 1;
    }

    client->poll_fds[POLLFD_PIPE].events = POLLIN;
    client->poll_fds[POLLFD_SOCKET].events = POLLIN;
    // wait till MQTT connection is established
    while (!client->mqtt_connected) {
        int rc = mqtt_wss_service(client, 60 * MSEC_PER_SEC);
        if(rc) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "Error connecting to MQTT WSS server \"%s\", port %d. Code: %d", host, port, rc);
            return 2;
        }
    }

    return 0;
}

#define MWS_TIMED_OUT 1
#define MWS_ERROR 2
#define MWS_OK 0
static const char *mqtt_wss_error_tos(int ec)
{
    switch(ec) {
        case MWS_TIMED_OUT:
            return "Error: Operation was not able to finish in time";
        case MWS_ERROR:
            return "Unspecified Error";
        default:
            return "Unknown Error Code!";
    }

}

static int mqtt_wss_service_all(mqtt_wss_client client, int timeout_ms)
{
    uint64_t exit_by_us = now_boottime_usec() + (timeout_ms * NSEC_PER_MSEC);
    client->poll_fds[POLLFD_SOCKET].events |= POLLOUT; // TODO when entering mwtt_wss_service use out buffer size to arm POLLOUT
    while (rbuf_bytes_available(client->ws_client->buf_write)) {
        const uint64_t now_us = now_boottime_usec();
        if (now_us >= exit_by_us)
            return MWS_TIMED_OUT;
        if (mqtt_wss_service(client, (exit_by_us - now_us) / USEC_PER_SEC))
            return MWS_ERROR;
    }
    return MWS_OK;
}

void mqtt_wss_disconnect(mqtt_wss_client client, int timeout_ms)
{
    // block application from sending more MQTT messages
    client->mqtt_disconnecting = 1;

    // send whatever was left at the time of calling this function
    int ret = mqtt_wss_service_all(client, timeout_ms / 4);
    if(ret)
        nd_log(NDLS_DAEMON, NDLP_ERR,
                  "Error while trying to send all remaining data in an attempt "
                  "to gracefully disconnect! EC=%d Desc:\"%s\"",
                  ret,
                  mqtt_wss_error_tos(ret));

    // schedule and send MQTT disconnect
    mqtt_ng_disconnect(client->mqtt, 0);
    mqtt_ng_sync(client->mqtt);

    ret = mqtt_wss_service_all(client, timeout_ms / 4);
    if(ret)
        nd_log(NDLS_DAEMON, NDLP_ERR,
                  "Error while trying to send MQTT disconnect message in an attempt "
                  "to gracefully disconnect! EC=%d Desc:\"%s\"",
                  ret,
                  mqtt_wss_error_tos(ret));

    // send WebSockets close message
    uint16_t ws_rc = htobe16(1000);
    ws_client_send(client->ws_client, WS_OP_CONNECTION_CLOSE, (const char*)&ws_rc, sizeof(ws_rc));
    ret = mqtt_wss_service_all(client, timeout_ms / 4);
    if(ret) {
        // Some MQTT/WSS servers will close socket on receipt of MQTT disconnect and
        // do not wait for WebSocket to be closed properly
        nd_log(NDLS_DAEMON, NDLP_WARNING,
                 "Error while trying to send WebSocket disconnect message in an attempt "
                 "to gracefully disconnect! EC=%d Desc:\"%s\".",
                 ret,
                 mqtt_wss_error_tos(ret));
    }

    // Service WSS connection until remote closes connection (usual)
    // or timeout happens (unusual) in which case we close
    mqtt_wss_service_all(client, timeout_ms / 4);

    close(client->sockfd);
    client->sockfd = -1;
}

static void mqtt_wss_wakeup(mqtt_wss_client client)
{
    if(write(client->write_notif_pipe[PIPE_WRITE_END], " ", 1) <= 0) { ; }
}

#define THROWAWAY_BUF_SIZE 32
char throwaway[THROWAWAY_BUF_SIZE];
static void util_clear_pipe(int fd)
{
    if(read(fd, throwaway, THROWAWAY_BUF_SIZE) <= 0)  { ; }
}

static void set_socket_pollfds(mqtt_wss_client client, int ssl_ret) {
    if (ssl_ret == SSL_ERROR_WANT_WRITE)
        client->poll_fds[POLLFD_SOCKET].events |= POLLOUT;
    if (ssl_ret == SSL_ERROR_WANT_READ)
        client->poll_fds[POLLFD_SOCKET].events |= POLLIN;
}

static int handle_mqtt_internal(mqtt_wss_client client)
{
    int rc = mqtt_ng_sync(client->mqtt);
    if (rc) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "mqtt_ng_sync returned %d != 0", rc);
        client->mqtt_connected = 0;
        return 1;
    }
    return 0;
}

static int t_till_next_keepalive_ms(mqtt_wss_client client)
{
    time_t last_send_ts = mqtt_ng_last_send_time(client->mqtt);
    time_t next_mqtt_keep_alive_ts = last_send_ts + client->mqtt_keepalive * 0.75;

    time_t now_ts = now_realtime_sec();

    if(now_ts >= next_mqtt_keep_alive_ts)
        return 0;

    int timeout_ms = (int)((next_mqtt_keep_alive_ts - now_ts) * MSEC_PER_SEC);

    if(timeout_ms < 1)
        timeout_ms = 1;

    if(timeout_ms > (int)(45 * MSEC_PER_SEC))
        timeout_ms = (int)(45 * MSEC_PER_SEC);

    return timeout_ms;
}

int mqtt_wss_service(mqtt_wss_client client, int timeout_ms)
{
    char *ptr;
    size_t size;
    int ret;
    int send_keepalive = 0;

#ifdef MQTT_WSS_CPUSTATS
    uint64_t t2;
    uint64_t t1 = now_monotonic_usec();
#endif

    // Check user requested TO doesn't interfere with MQTT keep alives
    if (!ping_timeout) {
        int till_next_keep_alive = t_till_next_keepalive_ms(client);
        if (client->mqtt_connected && (timeout_ms < 0 || timeout_ms >= till_next_keep_alive)) {
            timeout_ms = till_next_keep_alive;
            send_keepalive = 1;
        }
    }

#ifdef MQTT_WSS_CPUSTATS
    t2 = now_monotonic_usec();
    client->stats.time_keepalive += t2 - t1;
#endif

    worker_is_idle();
    if ((ret = poll(client->poll_fds, 2, timeout_ms >= 0 ? timeout_ms : -1)) < 0) {
        worker_is_busy(WORKER_ACLK_POLL_ERROR);

        if (errno == EINTR) {
            nd_log(NDLS_DAEMON, NDLP_WARNING, "poll interrupted by EINTR");
            return MQTT_WSS_OK;
        }
        nd_log(NDLS_DAEMON, NDLP_ERR, "poll error \"%s\"", strerror(errno));
        return MQTT_WSS_ERR_POLL_FAILED;
    }
    worker_is_busy(WORKER_ACLK_POLL_OK);

#ifdef MQTT_WSS_CPUSTATS
    t1 = now_monotonic_usec();
#endif

    if (ret == 0) {
        time_t now = now_realtime_sec();
        if (send_keepalive) {
            // otherwise we shortened the timeout ourselves to take care of
            // MQTT keep alives
            mqtt_ng_ping(client->mqtt);
            ping_timeout = now + PING_TIMEOUT;
            worker_is_busy(WORKER_ACLK_SENT_PING);
        } else {
            if (ping_timeout && ping_timeout < now) {
                disconnect_req = ACLK_PING_TIMEOUT;
                ping_timeout = 0;
            }
            // if poll timed out and user requested timeout was being used
            // return here let user do his work and he will call us back soon
            return MQTT_WSS_OK;
        }
    }

#ifdef MQTT_WSS_CPUSTATS
    t2 = now_monotonic_usec();
    client->stats.time_keepalive += t2 - t1;
#endif

    client->poll_fds[POLLFD_SOCKET].events = 0;

    if ((ptr = rbuf_get_linear_insert_range(client->ws_client->buf_read, &size))) {
        worker_is_busy(WORKER_ACLK_RX);

        if((ret = SSL_read(client->ssl, ptr, size)) > 0) {
            spinlock_lock(&client->stat_lock);
            client->stats.bytes_rx += ret;
            spinlock_unlock(&client->stat_lock);
            rbuf_bump_head(client->ws_client->buf_read, ret);
        } else {
            int errnobkp = errno;
            ret = SSL_get_error(client->ssl, ret);
            set_socket_pollfds(client, ret);

            if (ret != SSL_ERROR_WANT_READ &&
                ret != SSL_ERROR_WANT_WRITE) {
                worker_is_busy(WORKER_ACLK_RX_ERROR);
                nd_log(NDLS_DAEMON, NDLP_ERR, "SSL_read error: %d %s", ret, util_openssl_ret_err(ret));

                if (ret == SSL_ERROR_ZERO_RETURN) {
                    nd_log(NDLS_DAEMON, NDLP_ERR, "SSL_read connection closed by remote end");
                    return MQTT_WSS_ERR_REMOTE_CLOSED;
                }

                if (ret == SSL_ERROR_SYSCALL)
                    nd_log(NDLS_DAEMON, NDLP_ERR, "SSL_read SYSCALL errno: %d %s", errnobkp, strerror(errnobkp));

                return MQTT_WSS_ERR_CONN_DROP;
            }
        }
    }

#ifdef MQTT_WSS_CPUSTATS
    t1 = now_monotonic_usec();
    client->stats.time_read_socket += t1 - t2;
#endif

    ret = ws_client_process(client->ws_client);
    switch(ret) {
        case WS_CLIENT_PROTOCOL_ERROR:
            return MQTT_WSS_ERR_PROTO_WS;

        case WS_CLIENT_NEED_MORE_BYTES:
            client->poll_fds[POLLFD_SOCKET].events |= POLLIN;
            break;

        case WS_CLIENT_CONNECTION_REMOTE_CLOSED:
            return MQTT_WSS_ERR_REMOTE_CLOSED;

        case WS_CLIENT_CONNECTION_CLOSED:
            return MQTT_WSS_ERR_CONN_DROP;

        default:
            return MQTT_WSS_ERR_PROTO_WS;
    }

#ifdef MQTT_WSS_CPUSTATS
    t2 = now_monotonic_usec();
    client->stats.time_process_websocket += t2 - t1;
#endif

    // process MQTT stuff
    if(client->ws_client->state == WS_ESTABLISHED) {
        worker_is_busy(WORKER_ACLK_HANDLE_MQTT_INTERNAL);
        if (handle_mqtt_internal(client))
            return MQTT_WSS_ERR_PROTO_MQTT;
    }

    if (client->mqtt_didnt_finish_write) {
        client->mqtt_didnt_finish_write = 0;
        client->poll_fds[POLLFD_SOCKET].events |= POLLOUT;
    }

#ifdef MQTT_WSS_CPUSTATS
    t1 = now_monotonic_usec();
    client->stats.time_process_mqtt += t1 - t2;
#endif

    if ((ptr = rbuf_get_linear_read_range(client->ws_client->buf_write, &size))) {
        worker_is_busy(WORKER_ACLK_TX);

        if ((ret = SSL_write(client->ssl, ptr, size)) > 0) {
            spinlock_lock(&client->stat_lock);
            client->stats.bytes_tx += ret;
            spinlock_unlock(&client->stat_lock);
            rbuf_bump_tail(client->ws_client->buf_write, ret);
        } else {
            int errnobkp = errno;
            ret = SSL_get_error(client->ssl, ret);
            set_socket_pollfds(client, ret);
            if (ret != SSL_ERROR_WANT_READ &&
                ret != SSL_ERROR_WANT_WRITE) {
                worker_is_busy(WORKER_ACLK_TX_ERROR);
                nd_log(NDLS_DAEMON, NDLP_ERR, "SSL_write error: %d %s", ret, util_openssl_ret_err(ret));

                if (ret == SSL_ERROR_ZERO_RETURN) {
                    nd_log(NDLS_DAEMON, NDLP_ERR, "SSL_write connection closed by remote end");
                    return MQTT_WSS_ERR_REMOTE_CLOSED;
                }

                if (ret == SSL_ERROR_SYSCALL)
                    nd_log(NDLS_DAEMON, NDLP_ERR, "SSL_write SYSCALL errno: %d %s", errnobkp, strerror(errnobkp));

                return MQTT_WSS_ERR_CONN_DROP;
            }
        }
    }

    if(client->poll_fds[POLLFD_PIPE].revents & POLLIN)
        util_clear_pipe(client->write_notif_pipe[PIPE_READ_END]);

#ifdef MQTT_WSS_CPUSTATS
    t2 = now_monotonic_usec();
    client->stats.time_write_socket += t2 - t1;
#endif

    return MQTT_WSS_OK;
}

int mqtt_wss_publish5(mqtt_wss_client client,
                      char *topic,
                      free_fnc_t topic_free,
                      void *msg,
                      free_fnc_t msg_free,
                      size_t msg_len,
                      uint8_t publish_flags,
                      uint16_t *packet_id)
{
    if (client->mqtt_disconnecting) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "mqtt_wss is disconnecting can't publish");
        if (msg_free)
            msg_free(msg);
        return 1;
    }

    if (!client->mqtt_connected) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MQTT is offline. Can't send message.");
        if (msg_free)
            msg_free(msg);
        return 1;
    }
    uint8_t mqtt_flags = 0;

    mqtt_flags = (publish_flags & MQTT_WSS_PUB_QOSMASK) << 1;
    if (publish_flags & MQTT_WSS_PUB_RETAIN)
        mqtt_flags |= MQTT_PUBLISH_RETAIN;

    int rc = mqtt_ng_publish(client->mqtt, topic, topic_free, msg, msg_free, msg_len, mqtt_flags, packet_id);
    if (rc == MQTT_NG_MSGGEN_MSG_TOO_BIG)
        return MQTT_WSS_ERR_MSG_TOO_BIG;

    mqtt_wss_wakeup(client);

    return rc;
}

int mqtt_wss_subscribe(mqtt_wss_client client, char *topic, int max_qos_level)
{
    (void)max_qos_level; //TODO now hardcoded
    if (!client->mqtt_connected) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MQTT is offline. Can't subscribe.");
        return 1;
    }

    if (client->mqtt_disconnecting) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "mqtt_wss is disconnecting can't subscribe");
        return 1;
    }

    struct mqtt_sub sub = {
        .topic = topic,
        .topic_free = NULL,
        .options = /* max_qos_level & 0x3 TODO when QOS > 1 implemented */ 0x01 | (0x01 << 3)
    };
    mqtt_ng_subscribe(client->mqtt, &sub, 1);

    mqtt_wss_wakeup(client);
    return 0;
}

struct mqtt_wss_stats mqtt_wss_get_stats(mqtt_wss_client client)
{
    struct mqtt_wss_stats current;
    spinlock_lock(&client->stat_lock);
    current = client->stats;
    spinlock_unlock(&client->stat_lock);
    mqtt_ng_get_stats(client->mqtt, &current.mqtt);
    return current;
}

void mqtt_wss_reset_stats(mqtt_wss_client client)
{
    spinlock_lock(&client->stat_lock);
    memset(&client->stats, 0, sizeof(client->stats));
    spinlock_unlock(&client->stat_lock);
}

int mqtt_wss_set_topic_alias(mqtt_wss_client client, const char *topic)
{
    return mqtt_ng_set_topic_alias(client->mqtt, topic);
}

#ifdef MQTT_WSS_DEBUG
void mqtt_wss_set_SSL_CTX_keylog_cb(mqtt_wss_client client, void (*ssl_ctx_keylog_cb)(const SSL *ssl, const char *line))
{
    client->ssl_ctx_keylog_cb = ssl_ctx_keylog_cb;
}
#endif
