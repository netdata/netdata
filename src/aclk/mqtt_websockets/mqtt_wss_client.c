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

// If mqtt_wss_service() keeps being entered with poll() reporting readiness but makes no forward
// progress for this long, force a reconnect. This breaks a no-progress one-core 100% CPU spin
// whatever its cause (e.g. a runtime poll()/SSL readiness quirk). It bounds spins only, not a
// quiet hang: a clean poll() timeout counts as progress, so a peer that goes silent is not
// caught here. Before CONNACK that gap is closed by MQTT_WSS_CONNECT_BUDGET_SECS below; on an
// established connection the keepalive/ping path is what notices a silent peer.
//
// A clean poll() timeout always counts as progress. On top of that, before CONNACK wire bytes
// count too, read from the BIO counters, because the handshake gives SSL_read()/SSL_write()
// nothing but WANT_READ/WANT_WRITE while records are really flowing. Once established only
// plaintext counts, so a peer streaming records that yield no plaintext cannot refresh progress
// forever. See mqtt_wss_note_wire_progress().
#define MQTT_WSS_IO_WATCHDOG_SECS (2 * PING_TIMEOUT)

// Overall budget for one connection setup attempt, covering the TLS handshake, the WebSocket
// upgrade and the wait for CONNACK together. MQTT_WSS_IO_WATCHDOG_SECS cannot bound this phase:
// a clean poll() timeout counts as progress, so a peer that completes the TCP connection and then
// goes quiet would keep the setup loop servicing 60s at a time forever, with the ACLK thread stuck
// inside mqtt_wss_connect() and its caller unable to retry or notice a shutdown.
#define MQTT_WSS_CONNECT_BUDGET_SECS (2 * PING_TIMEOUT)

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

// monotonic time of the last forward progress (plaintext moved, a clean poll() timeout, or -
// before CONNACK only - wire bytes moved); drives the no-progress watchdog in
// mqtt_wss_service() (see MQTT_WSS_IO_WATCHDOG_SECS)
    usec_t last_io_progress_ut;

// last observed BIO byte counters, used to detect wire progress that produces no plaintext
// (TLS/WebSocket handshake records). Reset on every connect: a new SSL object restarts them.
    uint64_t last_bio_rx_bytes;
    uint64_t last_bio_tx_bytes;

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

// SSL_write() returned WANT_READ (TLS renegotiation or a post-handshake message): the write cannot
// proceed until the socket is readable, so queued output must NOT arm POLLOUT while this is set.
// Cleared when a write actually makes progress, or on the next connect attempt.
// Invariant: set implies buf_write is non-empty - it is only assigned inside the write block, and
// the only paths that empty buf_write also clear it.
    unsigned int write_wants_read:1;

// Application layer callback pointers
    void (*msg_callback)(const char *, const void *, size_t, int);
    void (*puback_callback)(uint16_t packet_id);

    SPINLOCK stat_lock;
    struct mqtt_wss_stats stats;

#ifdef MQTT_WSS_DEBUG
    void (*ssl_ctx_keylog_cb)(const SSL *ssl, const char *line);
#endif
};

static void mqtt_wss_close_sockfd(mqtt_wss_client client)
{
    if (client->sockfd >= 0)
        close(client->sockfd);

    client->sockfd = -1;
    client->poll_fds[POLLFD_SOCKET].fd = -1;
    client->poll_fds[POLLFD_SOCKET].events = 0;
    client->poll_fds[POLLFD_SOCKET].revents = 0;
}

static void mws_connack_callback_ng(void *user_ctx, int code)
{
    mqtt_wss_client client = user_ctx;
    switch(code) {
        case 0:
            client->mqtt_connected = 1;
            // (re)start the no-progress watchdog clock for this fresh connection
            client->last_io_progress_ut = now_monotonic_usec();
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
    client->sockfd = -1;
    client->poll_fds[POLLFD_SOCKET].fd = -1;

    // Defensive: mqtt_wss_connect() re-arms this per attempt and is the only path to a serviced
    // connection, so a 0 here is not reachable today. Seeded anyway because the watchdog check is
    // no longer gated on mqtt_connected, and a 0 would read as the machine's uptime.
    client->last_io_progress_ut = now_monotonic_usec();

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

    mqtt_wss_close_sockfd(client);

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
    bool *fallback_ipv4,
    int *service_rc)
{
    if (service_rc)
        *service_rc = 0;

    if (!mqtt_params) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "mqtt_params can't be null!");
        return -1;
    }

    // reset state in case this is reconnect
    client->mqtt_didnt_finish_write = 0;
    client->mqtt_connected = 0;
    client->mqtt_disconnecting = 0;
    client->write_wants_read = 0;
    ws_client_reset(client->ws_client);

    // A stale ping_timeout suppresses keepalive management and then fires a bogus PING timeout
    // on the new connection, so it must not outlive the connection it belonged to.
    ping_timeout = 0;

    // aclk.c consumes disconnect_req before tearing down, but the teardown keeps servicing the
    // link, so a PING timeout can re-arm it for a connection that no longer exists and tear the
    // next one down on its first iteration. That is the reachable path. ACLK_CLOUD_DISCONNECT is
    // covered too because it is equally connection-scoped, though during a normal teardown
    // msg_callback() drops inbound messages once mqtt_shutdown_msg_id is set, so it can only
    // re-arm if sending the app-layer disconnect failed. Clear only connection-scoped values -
    // the compare-exchange fails if another thread stored something else meanwhile, so a
    // concurrent reclaim is never lost.
    ACLK_DISCONNECT_ACTION stale = __atomic_load_n(&disconnect_req, __ATOMIC_RELAXED);
    if (aclk_disconnect_action_is_connection_scoped(stale))
        __atomic_compare_exchange_n(&disconnect_req, &stale, ACLK_NO_DISCONNECT,
                                    false, __ATOMIC_RELAXED, __ATOMIC_RELAXED);

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

    mqtt_wss_close_sockfd(client);

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
        mqtt_wss_close_sockfd(client);
        return -8;
    }

    if (client->proxy_type != MQTT_WSS_DIRECT) {
        if (aclk_proxy_negotiation_connect(client->sockfd, client->proxy_type, client->proxy_uname, client->proxy_passwd,
                                           client->target_host, client->target_port, 10000)) {
            mqtt_wss_close_sockfd(client);
            return -4;
        }

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
        mqtt_wss_close_sockfd(client);
        return -1;
    };
#endif

    // Free SSL structs from possible previous connections. Clear the pointers as we go: the
    // allocations below can fail and return early, and a stale non-NULL pointer here would be
    // freed again on the next attempt or by mqtt_wss_destroy().
    if (client->ssl) {
        SSL_free(client->ssl);
        client->ssl = NULL;
    }

    if (client->ssl_ctx) {
        SSL_CTX_free(client->ssl_ctx);
        client->ssl_ctx = NULL;
    }

    client->ssl_ctx = SSL_CTX_new(SSLv23_client_method());
    if (!client->ssl_ctx) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Couldn't allocate SSL context");
        mqtt_wss_close_sockfd(client);
        return -1;
    }

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
    if (!client->ssl) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Couldn't allocate SSL object");
        mqtt_wss_close_sockfd(client);
        return -1;
    }

    // paired with the SSL object above: its BIO counters start at 0, so a surviving snapshot
    // would read as wire progress on the first service call
    client->last_bio_rx_bytes = 0;
    client->last_bio_tx_bytes = 0;

    if (!(client->ssl_flags & MQTT_WSS_SSL_DONT_CHECK_CERTS)) {
        if (!SSL_set_ex_data(client->ssl, 0, client)) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "Could not SSL_set_ex_data");
            mqtt_wss_close_sockfd(client);
            return -4;
        }
    }
    SSL_set_fd(client->ssl, client->sockfd);
    SSL_set_connect_state(client->ssl);

    if (!SSL_set_tlsext_host_name(client->ssl, client->target_host)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Error setting TLS SNI host");
        mqtt_wss_close_sockfd(client);
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
            mqtt_wss_close_sockfd(client);
            return -7;
        }
    }

    result = SSL_connect(client->ssl);
    if (result != -1 && result != 1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "SSL could not connect");
        mqtt_wss_close_sockfd(client);
        return -5;
    }

    int ssl_connect_ec = SSL_ERROR_NONE;
    if (result == -1) {
        ssl_connect_ec = SSL_get_error(client->ssl, result);
        if (ssl_connect_ec != SSL_ERROR_WANT_READ && ssl_connect_ec != SSL_ERROR_WANT_WRITE) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to start SSL connection");
            mqtt_wss_close_sockfd(client);
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
        mqtt_wss_close_sockfd(client);
        return 1;
    }

    client->poll_fds[POLLFD_PIPE].events = POLLIN;
    client->poll_fds[POLLFD_SOCKET].events = POLLIN;

    // Honour the direction SSL_connect() asked for. A handshake blocked on WANT_WRITE needs
    // POLLOUT, and nothing else arms it before CONNACK: buf_write is still empty (the WebSocket
    // upgrade request is only generated later, inside ws_client_process()).
    if (ssl_connect_ec == SSL_ERROR_WANT_WRITE)
        client->poll_fds[POLLFD_SOCKET].events |= POLLOUT;

    client->last_io_progress_ut = now_monotonic_usec();
    const usec_t connect_start_ut = client->last_io_progress_ut;

    // wait till MQTT connection is established
    while (!client->mqtt_connected) {
        // The outer retry loop in aclk_attempt_to_connect() can only test service_running()
        // between attempts, so a shutdown while the peer is quiet would otherwise wait out the
        // whole budget below. Not a failure to report: leave service_rc at 0 and let the caller
        // see the same generic setup failure it gets from the paths above.
        if (unlikely(!service_running(SERVICE_ACLK))) {
            mqtt_wss_close_sockfd(client);
            return 1;
        }

        // One call gives both the expiry check and the poll cap, so the budget cannot be
        // overshot by a full service timeout on the last iteration.
        int budget_ms = aclk_timeout_remaining_ms(connect_start_ut,
                                                  MQTT_WSS_CONNECT_BUDGET_SECS * (int)MSEC_PER_SEC);
        if (unlikely(!budget_ms)) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "Timed out after %d seconds waiting for CONNACK from MQTT WSS server \"%s\", port %d",
                   MQTT_WSS_CONNECT_BUDGET_SECS, host, port);
            mqtt_wss_close_sockfd(client);
            if (service_rc)
                *service_rc = MQTT_WSS_ERR_CONNECT_TIMEOUT;
            return 2;
        }

        int rc = mqtt_wss_service(client, MIN(budget_ms, 60 * (int)MSEC_PER_SEC));
        if(rc) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "Error connecting to MQTT WSS server \"%s\", port %d. Code: %d", host, port, rc);
            mqtt_wss_close_sockfd(client);
            // Report the service code separately rather than as our return value: our own error
            // codes above overlap the MQTT_WSS_ERR_* space (-1, -3, -6 and -8 all collide), so
            // the caller could not tell a TCP failure from a WebSocket protocol error.
            if (service_rc)
                *service_rc = rc;
            return 2;
        }
    }

    return 0;
}

#define MWS_TIMED_OUT 2
#define MWS_OK 0
static const char *mqtt_wss_error_tos(int ec)
{
    switch(ec) {
        // mqtt_wss_service_all()'s own codes
        case MWS_TIMED_OUT:
            return "the flush budget expired with data still queued";
        // propagated from mqtt_wss_service(), so a teardown failure names its real cause
        case MQTT_WSS_ERR_CONN_DROP:
            return "connection dropped";
        case MQTT_WSS_ERR_PROTO_MQTT:
            return "MQTT protocol error";
        case MQTT_WSS_ERR_PROTO_WS:
            return "WebSocket protocol error";
        case MQTT_WSS_ERR_MSG_TOO_BIG:
            return "message too big";
        case MQTT_WSS_ERR_CANT_DO:
            return "unsupported operation";
        case MQTT_WSS_ERR_POLL_FAILED:
            return "poll() failed";
        case MQTT_WSS_ERR_REMOTE_CLOSED:
            return "closed by remote end";
        case MQTT_WSS_ERR_NO_IO_PROGRESS:
            return "no I/O progress on a ready socket";
        case MQTT_WSS_ERR_CONNECT_TIMEOUT:
            return "the connection setup budget expired before CONNACK";

        default:
            return "unknown error code";
    }
}

#define MQTT_WSS_IO_WATCHDOG_UT ((usec_t)MQTT_WSS_IO_WATCHDOG_SECS * USEC_PER_SEC)

typedef enum {
    MQTT_WSS_DROP_NONE = 0,
    MQTT_WSS_DROP_POLL_ERROR,
    MQTT_WSS_DROP_NO_IO_PROGRESS,
} MQTT_WSS_DROP_REASON;

// POLLERR/POLLNVAL are unrecoverable -> drop now. POLLHUP is intentionally NOT a drop: it can
// accompany still-readable data (a graceful close carrying a final frame), so the caller lets
// it fall through to SSL_read(), which drains the remaining bytes and then reports the close
// cleanly (SSL_ERROR_ZERO_RETURN). A dead socket that keeps signalling readiness without
// progress is caught by the watchdog instead.
static MQTT_WSS_DROP_REASON mqtt_wss_drop_reason(short revents, usec_t last_io_progress_ut, usec_t now_ut) {
    if (unlikely(revents & (POLLERR | POLLNVAL)))
        return MQTT_WSS_DROP_POLL_ERROR;

    if (unlikely(aclk_usec_budget_spent(last_io_progress_ut, MQTT_WSS_IO_WATCHDOG_UT, now_ut)))
        return MQTT_WSS_DROP_NO_IO_PROGRESS;

    return MQTT_WSS_DROP_NONE;
}

// Separate from the logging switch so the drop -> error-code mapping is pinned by a test: this is
// what makes a watchdog drop distinguishable from a socket error in status and logs. A poll error
// keeps reporting as MQTT_WSS_ERR_CONN_DROP (and therefore ACLK_STATUS_OFFLINE_SOCKET_ERROR);
// MQTT_WSS_ERR_POLL_FAILED is reserved for the poll() syscall itself failing.
static int mqtt_wss_err_from_drop_reason(MQTT_WSS_DROP_REASON reason) {
    switch (reason) {
        case MQTT_WSS_DROP_POLL_ERROR:
            return MQTT_WSS_ERR_CONN_DROP;

        case MQTT_WSS_DROP_NO_IO_PROGRESS:
            return MQTT_WSS_ERR_NO_IO_PROGRESS;

        case MQTT_WSS_DROP_NONE:
            return MQTT_WSS_OK;
    }

    return MQTT_WSS_OK;
}

// Queued output wants writability, unless OpenSSL is waiting on readability to let the write
// proceed - arming POLLOUT then would make poll() return instantly on every retry.
static bool mqtt_wss_should_arm_pollout(bool bytes_queued, bool write_wants_read) {
    return bytes_queued && !write_wants_read;
}

static int mqtt_wss_service_all(mqtt_wss_client client, int timeout_ms)
{
    const usec_t start_ut = now_monotonic_usec();
    while (rbuf_bytes_available(client->ws_client->buf_write)) {
        // shared with the other ACLK deadlines, so a sub-millisecond remainder still yields 1ms
        // rather than degenerating into a non-blocking poll() spin
        const int remaining_ms = aclk_timeout_remaining_ms(start_ut, timeout_ms);
        if (remaining_ms <= 0)
            return MWS_TIMED_OUT;

        // Cap each attempt so a congested socket gets many tries within the phase instead of
        // spending the whole budget inside one poll(). mqtt_wss_service() arms POLLOUT itself
        // from the write buffer, so nothing needs re-arming here.
        // Return the underlying cause rather than a generic error: every teardown failure used to
        // log as "Unspecified Error". MQTT_WSS_ERR_* are negative and MWS_* are 0/2, so the caller
        // can stringify either. MWS_TIMED_OUT deliberately avoids 1, which mqtt_wss_client.h
        // assigns to MQTT_WSS_OK_TO - a documented (currently unused) success return of
        // mqtt_wss_service() that would otherwise be misread here as a failure.
        const int rc = mqtt_wss_service(client, MIN(remaining_ms, 100));
        if (rc)
            return rc;
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
        nd_log(NDLS_DAEMON, ret == MWS_TIMED_OUT ? NDLP_WARNING : NDLP_ERR,
                  "Could not send all remaining data while gracefully disconnecting: %s",
                  mqtt_wss_error_tos(ret));

    // schedule and send MQTT disconnect
    mqtt_ng_disconnect(client->mqtt, 0);
    mqtt_ng_sync(client->mqtt);

    ret = mqtt_wss_service_all(client, timeout_ms / 4);
    if(ret)
        nd_log(NDLS_DAEMON, ret == MWS_TIMED_OUT ? NDLP_WARNING : NDLP_ERR,
                  "Could not send the MQTT disconnect message while gracefully disconnecting: %s",
                  mqtt_wss_error_tos(ret));

    // send WebSockets close message
    uint16_t ws_rc = htobe16(1000);
    ws_client_send(client->ws_client, WS_OP_CONNECTION_CLOSE, (const char*)&ws_rc, sizeof(ws_rc));
    ret = mqtt_wss_service_all(client, timeout_ms / 4);
    if(ret) {
        // Some MQTT/WSS servers will close socket on receipt of MQTT disconnect and
        // do not wait for WebSocket to be closed properly
        nd_log(NDLS_DAEMON, NDLP_WARNING,
                 "Could not send the WebSocket close message while gracefully disconnecting: %s",
                 mqtt_wss_error_tos(ret));
    }

    // Final flush of anything the close frame left queued. This does not wait for the remote to
    // close: mqtt_wss_service_all() is gated on the write buffer, so it returns immediately once
    // that is empty.
    mqtt_wss_service_all(client, timeout_ms / 4);

    mqtt_wss_close_sockfd(client);
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

// Did either wire counter move? Split out so the comparison is unit-testable.
static bool mqtt_wss_bio_advanced(uint64_t rx, uint64_t tx, uint64_t last_rx, uint64_t last_tx) {
    return rx != last_rx || tx != last_tx;
}

// Credit wire-level progress to the watchdog, but only before CONNACK: the handshake produces no
// plaintext for SSL_read()/SSL_write() to report, so without this a healthy slow setup could be
// killed. Once established the watchdog must demand plaintext or a clean poll() timeout again -
// crediting raw bytes there would let a peer streaming records that yield no plaintext (a
// KeyUpdate/NewSessionTicket flood) refresh progress forever and spin a core undetected.
typedef enum {
    MQTT_WSS_WIRE_NO_CHANGE = 0,   // counters did not move
    MQTT_WSS_WIRE_SNAPSHOT_ONLY,   // record the new counters, but do not credit the watchdog
    MQTT_WSS_WIRE_CREDIT,          // record and credit the watchdog
} MQTT_WSS_WIRE_ACTION;

// Pure so the phase rule is testable. The snapshot has to track in both phases or it goes stale
// behind a distant reset and a later comparison lies, but only the pre-CONNACK phase may credit
// the watchdog - see the phase note at the top of this file.
static MQTT_WSS_WIRE_ACTION mqtt_wss_wire_progress_action(bool advanced, bool mqtt_connected) {
    if (!advanced)
        return MQTT_WSS_WIRE_NO_CHANGE;

    return mqtt_connected ? MQTT_WSS_WIRE_SNAPSHOT_ONLY : MQTT_WSS_WIRE_CREDIT;
}

static void mqtt_wss_note_wire_progress(mqtt_wss_client client) {
    if (unlikely(!client->ssl))
        return;

    BIO *rbio = SSL_get_rbio(client->ssl);
    BIO *wbio = SSL_get_wbio(client->ssl);

    // Both exist from SSL_set_fd() onwards. If either is missing we have no usable reading:
    // substituting 0 for the absent side would compare against a non-zero snapshot and fake
    // movement, while treating it as progress outright would silently disable the watchdog.
    if (unlikely(!rbio || !wbio))
        return;

    const uint64_t rx = (uint64_t)BIO_number_read(rbio);
    const uint64_t tx = (uint64_t)BIO_number_written(wbio);

    const bool advanced =
        mqtt_wss_bio_advanced(rx, tx, client->last_bio_rx_bytes, client->last_bio_tx_bytes);

    const MQTT_WSS_WIRE_ACTION action =
        mqtt_wss_wire_progress_action(advanced, client->mqtt_connected);

    if (action == MQTT_WSS_WIRE_NO_CHANGE)
        return;

    if (action == MQTT_WSS_WIRE_CREDIT)
        client->last_io_progress_ut = now_monotonic_usec();

    client->last_bio_rx_bytes = rx;
    client->last_bio_tx_bytes = tx;
}

static void set_socket_pollfds(mqtt_wss_client client, int ssl_ret) {
    if (ssl_ret == SSL_ERROR_WANT_WRITE)
        client->poll_fds[POLLFD_SOCKET].events |= POLLOUT;
    if (ssl_ret == SSL_ERROR_WANT_READ)
        client->poll_fds[POLLFD_SOCKET].events |= POLLIN;
}

#define MQTT_WSS_TEST(condition, msg) do {                                       \
        if(!(condition)) {                                                       \
            fprintf(stderr, "mqtt wss timeout unittest FAILED: %s (%s:%d)\n",    \
                    (msg), __FUNCTION__, __LINE__);                              \
            errors++;                                                            \
        }                                                                        \
    } while(0)

int mqtt_wss_client_timeout_unittest(void) {
    int errors = 0;
    const usec_t watchdog_ut = (usec_t)MQTT_WSS_IO_WATCHDOG_SECS * USEC_PER_SEC;
    const usec_t progress_ut = 100 * USEC_PER_SEC;

    fprintf(stderr, "\nrunning mqtt wss timeout unittest\n");

    // Pin the derivation and non-degeneracy, not the literal: the boundary assertions below hold
    // for any value including 0, so something must reject a degenerate window - but hardcoding
    // 120s would fail this test for a legitimate PING_TIMEOUT retune rather than for a defect.
    MQTT_WSS_TEST(watchdog_ut == (usec_t)(2 * PING_TIMEOUT) * USEC_PER_SEC,
                  "watchdog window is no longer derived from PING_TIMEOUT");
    MQTT_WSS_TEST(watchdog_ut > 0, "watchdog window is degenerate");

    // the boundary is inclusive, the same convention as every other ACLK budget: the window is
    // spent once exactly that much has elapsed
    MQTT_WSS_TEST(!aclk_usec_budget_spent(progress_ut, watchdog_ut, progress_ut + watchdog_ut - 1),
                  "expired 1us short of the watchdog window");
    MQTT_WSS_TEST(aclk_usec_budget_spent(progress_ut, watchdog_ut, progress_ut + watchdog_ut),
                  "did not expire exactly at the watchdog window boundary");

    // a backward monotonic reading must never be treated as elapsed time
    MQTT_WSS_TEST(!aclk_usec_budget_spent(progress_ut, watchdog_ut, progress_ut - 1),
                  "backward clock reading expired the watchdog");

    // The drop decision must use the watchdog window itself, not just any budget: these two pin
    // MQTT_WSS_IO_WATCHDOG_UT, which the generic aclk_usec_budget_spent() assertions above cannot.
    MQTT_WSS_TEST(mqtt_wss_drop_reason(POLLIN, progress_ut, progress_ut + watchdog_ut - 1) ==
                      MQTT_WSS_DROP_NONE,
                  "dropped 1us short of the watchdog window");
    MQTT_WSS_TEST(mqtt_wss_drop_reason(POLLIN, progress_ut, progress_ut + watchdog_ut) ==
                      MQTT_WSS_DROP_NO_IO_PROGRESS,
                  "did not drop exactly at the watchdog window boundary");

    // POLLOUT arming rule, all four combinations.
    MQTT_WSS_TEST(mqtt_wss_should_arm_pollout(true, false),
                  "queued output did not arm POLLOUT");
    MQTT_WSS_TEST(!mqtt_wss_should_arm_pollout(true, true),
                  "queued output armed POLLOUT while the write was blocked on readability");
    MQTT_WSS_TEST(!mqtt_wss_should_arm_pollout(false, false),
                  "an empty write buffer armed POLLOUT");
    MQTT_WSS_TEST(!mqtt_wss_should_arm_pollout(false, true),
                  "an empty write buffer armed POLLOUT while blocked on readability");

    // Drop decision: dispatch and precedence.
    MQTT_WSS_TEST(mqtt_wss_drop_reason(POLLIN, progress_ut, progress_ut) == MQTT_WSS_DROP_NONE,
                  "healthy readable socket was dropped");
    MQTT_WSS_TEST(mqtt_wss_drop_reason(POLLERR, progress_ut, progress_ut) == MQTT_WSS_DROP_POLL_ERROR,
                  "POLLERR did not drop");
    MQTT_WSS_TEST(mqtt_wss_drop_reason(POLLNVAL, progress_ut, progress_ut) == MQTT_WSS_DROP_POLL_ERROR,
                  "POLLNVAL did not drop");
    MQTT_WSS_TEST(mqtt_wss_drop_reason(POLLHUP, progress_ut, progress_ut) == MQTT_WSS_DROP_NONE,
                  "POLLHUP dropped instead of falling through to SSL_read()");
    MQTT_WSS_TEST(mqtt_wss_drop_reason(POLLIN, progress_ut, progress_ut + watchdog_ut + 1) ==
                      MQTT_WSS_DROP_NO_IO_PROGRESS,
                  "ready-without-progress spin did not trip the watchdog");

    // an unrecoverable socket error outranks the watchdog, so the log names the real cause
    MQTT_WSS_TEST(mqtt_wss_drop_reason(POLLERR, progress_ut, progress_ut + watchdog_ut + 1) ==
                      MQTT_WSS_DROP_POLL_ERROR,
                  "watchdog outranked POLLERR when both conditions held");

    // wire progress: either counter moving counts, and neither moving does not. A stalled
    // handshake must not be credited just because one direction was already non-zero.
    MQTT_WSS_TEST(!mqtt_wss_bio_advanced(500, 300, 500, 300), "idle wire counters reported progress");
    MQTT_WSS_TEST(mqtt_wss_bio_advanced(501, 300, 500, 300), "inbound wire progress was missed");
    MQTT_WSS_TEST(mqtt_wss_bio_advanced(500, 301, 500, 300), "outbound wire progress was missed");
    MQTT_WSS_TEST(mqtt_wss_bio_advanced(0, 0, 0, 1), "a counter reset was not treated as movement");

    // The phase rule. Deleting the !mqtt_connected gate would let a peer streaming records that
    // yield no plaintext refresh progress forever and spin a core undetected; dropping the
    // snapshot update in the established phase would leave it stale behind a distant reset.
    MQTT_WSS_TEST(mqtt_wss_wire_progress_action(false, false) == MQTT_WSS_WIRE_NO_CHANGE,
                  "idle wire counters produced an action before CONNACK");
    MQTT_WSS_TEST(mqtt_wss_wire_progress_action(false, true) == MQTT_WSS_WIRE_NO_CHANGE,
                  "idle wire counters produced an action after CONNACK");
    MQTT_WSS_TEST(mqtt_wss_wire_progress_action(true, false) == MQTT_WSS_WIRE_CREDIT,
                  "wire progress during the handshake was not credited to the watchdog");
    MQTT_WSS_TEST(mqtt_wss_wire_progress_action(true, true) == MQTT_WSS_WIRE_SNAPSHOT_ONLY,
                  "wire progress after CONNACK was credited instead of only snapshotted");

    // Classification only. This does NOT cover the reset in mqtt_wss_connect(): deleting that
    // block leaves every assertion here passing. It pins which values the reset is allowed to
    // clear, which is where the round-three regression was (ACLK_RELOAD_CONF must be preserved).
    MQTT_WSS_TEST(aclk_disconnect_action_is_connection_scoped(ACLK_PING_TIMEOUT),
                  "ACLK_PING_TIMEOUT not classified as connection-scoped");
    MQTT_WSS_TEST(aclk_disconnect_action_is_connection_scoped(ACLK_CLOUD_DISCONNECT),
                  "ACLK_CLOUD_DISCONNECT not classified as connection-scoped");
    MQTT_WSS_TEST(!aclk_disconnect_action_is_connection_scoped(ACLK_RELOAD_CONF),
                  "ACLK_RELOAD_CONF classified as connection-scoped - a reclaim would be dropped");
    MQTT_WSS_TEST(!aclk_disconnect_action_is_connection_scoped(ACLK_NO_DISCONNECT),
                  "ACLK_NO_DISCONNECT classified as a pending request");

    // The drop -> error-code -> status chain. This is the deliverable: without it a watchdog drop
    // is indistinguishable from a socket error to an operator. Swapping either mapping's arms
    // used to leave the whole suite green.
    MQTT_WSS_TEST(mqtt_wss_err_from_drop_reason(MQTT_WSS_DROP_NO_IO_PROGRESS) ==
                      MQTT_WSS_ERR_NO_IO_PROGRESS,
                  "a watchdog drop did not map to MQTT_WSS_ERR_NO_IO_PROGRESS");
    MQTT_WSS_TEST(mqtt_wss_err_from_drop_reason(MQTT_WSS_DROP_POLL_ERROR) == MQTT_WSS_ERR_CONN_DROP,
                  "a poll-error drop did not map to MQTT_WSS_ERR_CONN_DROP");
    MQTT_WSS_TEST(mqtt_wss_err_from_drop_reason(MQTT_WSS_DROP_NONE) == MQTT_WSS_OK,
                  "no drop did not map to MQTT_WSS_OK");

    MQTT_WSS_TEST(aclk_status_from_mqtt_wss_rc(MQTT_WSS_ERR_NO_IO_PROGRESS) ==
                      ACLK_STATUS_OFFLINE_NO_IO_PROGRESS,
                  "a watchdog drop is not reported as its own status");
    MQTT_WSS_TEST(aclk_status_from_mqtt_wss_rc(MQTT_WSS_ERR_REMOTE_CLOSED) ==
                      ACLK_STATUS_OFFLINE_CLOSED_BY_REMOTE,
                  "remote close is not reported as closed-by-remote");
    MQTT_WSS_TEST(aclk_status_from_mqtt_wss_rc(MQTT_WSS_ERR_PROTO_MQTT) ==
                      ACLK_STATUS_OFFLINE_MQTT_PROTOCOL_ERROR,
                  "an MQTT protocol error is not reported as such");
    MQTT_WSS_TEST(aclk_status_from_mqtt_wss_rc(MQTT_WSS_ERR_PROTO_WS) ==
                      ACLK_STATUS_OFFLINE_WS_PROTOCOL_ERROR,
                  "a WebSocket protocol error is not reported as such");
    MQTT_WSS_TEST(aclk_status_from_mqtt_wss_rc(MQTT_WSS_ERR_MSG_TOO_BIG) ==
                      ACLK_STATUS_OFFLINE_MESSAGE_TOO_BIG,
                  "an oversized message is not reported as such");
    MQTT_WSS_TEST(aclk_status_from_mqtt_wss_rc(MQTT_WSS_ERR_POLL_FAILED) ==
                      ACLK_STATUS_OFFLINE_POLL_ERROR,
                  "a failed poll() syscall is not reported as a poll error");
    MQTT_WSS_TEST(aclk_status_from_mqtt_wss_rc(MQTT_WSS_ERR_CONN_DROP) ==
                      ACLK_STATUS_OFFLINE_SOCKET_ERROR,
                  "a connection drop is not reported as a socket error");
    MQTT_WSS_TEST(aclk_status_from_mqtt_wss_rc(MQTT_WSS_ERR_CONNECT_TIMEOUT) ==
                      ACLK_STATUS_OFFLINE_CONNECT_TIMEOUT,
                  "a CONNACK timeout is not reported as its own status");

    // The setup budget is a separate window from the watchdog: the watchdog cannot bound the
    // pre-CONNACK phase at all, because a clean poll() timeout refreshes progress. Pin the
    // derivation and non-degeneracy the same way, and pin the poll cap - the remaining budget is
    // what stops the last mqtt_wss_service() call from overshooting it.
    const int connect_budget_ms = MQTT_WSS_CONNECT_BUDGET_SECS * (int)MSEC_PER_SEC;
    MQTT_WSS_TEST(connect_budget_ms == (2 * PING_TIMEOUT) * (int)MSEC_PER_SEC,
                  "connect budget is no longer derived from PING_TIMEOUT");
    MQTT_WSS_TEST(connect_budget_ms > 0, "connect budget is degenerate");
    MQTT_WSS_TEST(aclk_timeout_remaining_ms(now_monotonic_usec(), connect_budget_ms) > 0,
                  "a fresh connect budget reports no time remaining");

    if (errors)
        fprintf(stderr, "mqtt wss timeout unittest: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "mqtt wss timeout unittest: OK\n");


    return errors;
}

#undef MQTT_WSS_TEST

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

    // Arm POLLOUT from the write buffer, not from the last SSL error. SSL_write() can report a
    // short count while still returning success - it completes a previously pending fragment, and
    // buf_write (128KiB) is larger than OpenSSL's 16KiB max_send_fragment - and that leaves no
    // WANT_WRITE for set_socket_pollfds() to re-arm from. Keying off the error would strand those
    // bytes until the poll timeout. (This is not SSL_MODE_ENABLE_PARTIAL_WRITE, which is not set.)
    //
    // Except while the write is blocked on readability: OpenSSL requires the retry to wait for
    // POLLIN there, and arming POLLOUT on an already-writable socket would make poll() return
    // instantly on every retry - a busy loop that credits no progress and ends in a watchdog drop.
    if (mqtt_wss_should_arm_pollout(rbuf_bytes_available(client->ws_client->buf_write) > 0,
                                    client->write_wants_read))
        client->poll_fds[POLLFD_SOCKET].events |= POLLOUT;

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
        // A clean poll() timeout means the loop blocked rather than spun: that is forward progress
        // for the watchdog (a healthy idle link reaches here at the caller cadence).
        //
        // Except while a write is blocked on readability with bytes still queued. That is a
        // stalled write, not an idle link: crediting it would hide the stall from the watchdog and
        // leave recovery to the much slower keepalive path, reported as a ping timeout rather than
        // as the lack of I/O progress it actually is.
        if (!(client->write_wants_read && rbuf_bytes_available(client->ws_client->buf_write)))
            client->last_io_progress_ut = now_monotonic_usec();
        time_t now = now_realtime_sec();
        if (send_keepalive) {
            // otherwise we shortened the timeout ourselves to take care of
            // MQTT keep alives
            mqtt_ng_ping(client->mqtt);
            ping_timeout = now + PING_TIMEOUT;
            worker_is_busy(WORKER_ACLK_SENT_PING);
        } else {
            if (ping_timeout && ping_timeout < now) {
                __atomic_store_n(&disconnect_req, ACLK_PING_TIMEOUT, __ATOMIC_RELAXED);
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

    // Tear the connection down if the socket reports an unrecoverable error, or if we keep
    // being re-entered with poll() reporting readiness but making no forward progress (a
    // spin). In either case drop so the outer loop reconnects, rather than burning a core
    // indefinitely. This applies during TLS/WebSocket/MQTT setup too: the connect loop uses
    // this same service function before CONNACK, and is equally vulnerable to ready-without-
    // progress spins. Which is why progress has to be accounted at the wire level first: the
    // previous iteration's SSL calls may have moved handshake records without ever producing
    // plaintext, and that is genuine progress the watchdog must not mistake for a spin.
    mqtt_wss_note_wire_progress(client);

    const MQTT_WSS_DROP_REASON drop =
        mqtt_wss_drop_reason(client->poll_fds[POLLFD_SOCKET].revents,
                             client->last_io_progress_ut, now_monotonic_usec());

    switch (drop) {
        case MQTT_WSS_DROP_POLL_ERROR:
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "ACLK: socket poll() reported error (revents=0x%x); dropping connection",
                   (unsigned)client->poll_fds[POLLFD_SOCKET].revents);
            return mqtt_wss_err_from_drop_reason(drop);

        case MQTT_WSS_DROP_NO_IO_PROGRESS:
            // report both revents: this branch is reached whenever poll() returned for *either*
            // fd, so the socket may show 0 here and the wakeup pipe alone be responsible
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "ACLK: no I/O progress for %d seconds while poll() kept returning "
                   "(socket revents=0x%x, pipe revents=0x%x); dropping connection to break a "
                   "CPU spin", MQTT_WSS_IO_WATCHDOG_SECS,
                   (unsigned)client->poll_fds[POLLFD_SOCKET].revents,
                   (unsigned)client->poll_fds[POLLFD_PIPE].revents);
            return mqtt_wss_err_from_drop_reason(drop);

        case MQTT_WSS_DROP_NONE:
            break;
    }

    client->poll_fds[POLLFD_SOCKET].events = 0;

    if ((ptr = rbuf_get_linear_insert_range(client->ws_client->buf_read, &size))) {
        worker_is_busy(WORKER_ACLK_RX);

        if((ret = SSL_read(client->ssl, ptr, size)) > 0) {
            spinlock_lock(&client->stat_lock);
            client->stats.bytes_rx += ret;
            spinlock_unlock(&client->stat_lock);
            rbuf_bump_head(client->ws_client->buf_read, ret);
            client->last_io_progress_ut = now_monotonic_usec();
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

        case WS_CLIENT_BUFFER_FULL:
            return MQTT_WSS_ERR_MSG_TOO_BIG;

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
            client->last_io_progress_ut = now_monotonic_usec();
            client->write_wants_read = 0;
        } else {
            int errnobkp = errno;
            ret = SSL_get_error(client->ssl, ret);
            set_socket_pollfds(client, ret);

            client->write_wants_read = (ret == SSL_ERROR_WANT_READ);
            if (client->write_wants_read) {
                // Drop any POLLOUT armed earlier in this same iteration - by
                // mqtt_didnt_finish_write above, or by a WANT_WRITE from SSL_read(). events is not
                // cleared again until after the next poll(), so a stale POLLOUT would make it
                // return instantly on writability and spin. Such a POLLOUT is provably stale:
                // SSL_write() retries any pending flush first, so it cannot report WANT_READ
                // until writability is no longer what OpenSSL is waiting for.
                client->poll_fds[POLLFD_SOCKET].events &= ~POLLOUT;
            }
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
    // topic_free is not yet supported: the rollback path inside mqtt_ng_publish
    // can free topic asymmetrically across failure modes (see contract notes in
    // mqtt_ng.h and the long comment below). Enforce NULL until that is fixed
    // so callers don't silently leak a borrowed/allocated topic on failure.
    internal_fatal(topic_free != NULL, "mqtt_wss_publish5: topic_free must be NULL until rollback ownership is made symmetric");

    const char *fail_reason = NULL;
    if (client->mqtt_disconnecting)
        fail_reason = "mqtt_wss is disconnecting can't publish";
    else if (!client->mqtt_connected)
        fail_reason = "MQTT is offline. Can't send message.";

    if (fail_reason) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "%s", fail_reason);
        if (packet_id)
            *packet_id = 0;
        if (msg_free)
            msg_free(msg);
        return 1;
    }

    uint8_t mqtt_flags = (publish_flags & MQTT_WSS_PUB_QOSMASK) << 1;
    if (publish_flags & MQTT_WSS_PUB_RETAIN)
        mqtt_flags |= MQTT_PUBLISH_RETAIN;

    // Failure-path ownership contract with mqtt_ng_publish:
    //  - On MQTT_NG_MSGGEN_OK, msg is attached to a buffer fragment and the
    //    transaction buffer will call msg_free after the message is ack'd.
    //  - On any non-OK return, msg is never attached, so ownership stays with
    //    us and we must call msg_free here.
    //
    //    Single-free invariant: msg is attached to a fragment only at the
    //    final frag_set_external_data() inside mqtt_ng_generate_publish(),
    //    after which the function commits unconditionally -- there is no
    //    `goto fail_rollback` between attachment and commit. Every reachable
    //    fail_rollback site therefore runs with msg unattached, so the
    //    rollback walks no msg-bearing fragment and msg_free() is only ever
    //    called by us. If a future change inserts a failure exit after
    //    attaching msg but before commit, the rollback would also invoke
    //    msg_free and this branch would double-free; preserve the invariant
    //    or move responsibility entirely into mqtt_ng_publish().
    //
    //  - topic_free is intentionally NOT handled here. mqtt_ng_publish may
    //    attach topic to a fragment via optimized_add() before failing, in
    //    which case the rollback already invokes topic_free; if it fails
    //    earlier, topic_free is never invoked at all. The current callers all
    //    pass topic_free=NULL, so this asymmetry is harmless today.
    int rc = mqtt_ng_publish(client->mqtt, topic, topic_free, msg, msg_free, msg_len, mqtt_flags, packet_id);
    if (rc != MQTT_NG_MSGGEN_OK) {
        if (packet_id)
            *packet_id = 0;
        if (msg_free)
            msg_free(msg);
        if (rc == MQTT_NG_MSGGEN_MSG_TOO_BIG)
            return MQTT_WSS_ERR_MSG_TOO_BIG;
        return rc;
    }

    mqtt_wss_wakeup(client);
    return MQTT_WSS_OK;
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
