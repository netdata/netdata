// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk.h"

#include "mqtt_websockets/mqtt_wss_client.h"
#include "mqtt_websockets/aclk_mqtt_workers.h"
#include "aclk_otp.h"
#include "aclk_tx_msgs.h"
#include "aclk_query.h"
#include "aclk_query_queue.h"
#include "aclk_util.h"
#include "aclk_rx_msgs.h"
#include "https_client.h"
#include "schema-wrappers/schema_wrappers.h"
#include "aclk_capas.h"
#include "aclk_proxy.h"

#ifdef ACLK_LOG_CONVERSATION_DIR
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#endif

int aclk_pubacks_per_conn = 0; // How many PubAcks we got since MQTT conn est.
int aclk_rcvd_cloud_msgs = 0;
int aclk_connection_counter = 0;

mqtt_wss_client mqttwss_client;

static bool aclk_connected = false;
static inline void aclk_set_connected(void) {
    __atomic_store_n(&aclk_connected, true, __ATOMIC_RELAXED);

    daemon_status_file_update_status(DAEMON_STATUS_NONE);
}
static inline void aclk_set_disconnected(void) {
    __atomic_store_n(&aclk_connected, false, __ATOMIC_RELAXED);

    if(mqttwss_client)
        mqtt_wss_reset_stats(mqttwss_client);

    daemon_status_file_update_status(DAEMON_STATUS_NONE);
}

inline bool aclk_online(void) {
    return __atomic_load_n(&aclk_connected, __ATOMIC_RELAXED);
}

bool aclk_online_for_contexts(void) {
    return aclk_online() && aclk_query_scope_has(HTTP_ACL_METRICS);
}

bool aclk_online_for_alerts(void) {
    return aclk_online() && aclk_query_scope_has(HTTP_ACL_ALERTS);
}

bool aclk_online_for_nodes(void) {
    return aclk_online() && aclk_query_scope_has(HTTP_ACL_NODES);
}

int aclk_ctx_based = 0;
int aclk_disable_runtime = 0;

ACLK_DISCONNECT_ACTION disconnect_req = ACLK_NO_DISCONNECT;

usec_t aclk_session_us = 0;
time_t aclk_session_sec = 0;

time_t last_conn_time_mqtt = 0;
time_t last_conn_time_appl = 0;
time_t last_disconnect_time = 0;
time_t next_connection_attempt = 0;
float last_backoff_value = 0;

time_t aclk_block_until = 0;

struct mqtt_wss_stats aclk_statistics(void) {
    if(mqttwss_client)
        return mqtt_wss_get_stats(mqttwss_client);
    else
        return (struct mqtt_wss_stats) { 0 };
}

struct aclk_shared_state aclk_shared_state = {
    .mqtt_shutdown_msg_id = -1,
    .mqtt_shutdown_msg_rcvd = 0
};

#ifdef MQTT_WSS_DEBUG
#include <openssl/ssl.h>
#define DEFAULT_SSKEYLOGFILE_NAME "SSLKEYLOGFILE"
const char *ssl_log_filename = NULL;
FILE *ssl_log_file = NULL;
static void aclk_ssl_keylog_cb(const SSL *ssl, const char *line)
{
    (void)ssl;
    if (!ssl_log_file)
        ssl_log_file = fopen(ssl_log_filename, "a");
    if (!ssl_log_file) {
        netdata_log_error("ACLK: Couldn't open ssl_log file (%s) for append.", ssl_log_filename);
        return;
    }
    fputs(line, ssl_log_file);
    putc('\n', ssl_log_file);
    fflush(ssl_log_file);
}
#endif

#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_300
OSSL_DECODER_CTX *aclk_dctx = NULL;
EVP_PKEY *aclk_private_key = NULL;
#else
static RSA *aclk_private_key = NULL;
#endif
static int load_private_key()
{
    if (aclk_private_key != NULL) {
#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_300
        EVP_PKEY_free(aclk_private_key);
        if (aclk_dctx)
            OSSL_DECODER_CTX_free(aclk_dctx);

        aclk_dctx = NULL;
#else
        RSA_free(aclk_private_key);
#endif
    }
    aclk_private_key = NULL;
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/cloud.d/private.pem", netdata_configured_varlib_dir);

    long bytes_read;
    char *private_key = read_by_filename(filename, &bytes_read);
    if (!private_key) {
        netdata_log_error("ACLK: Claimed agent cannot establish ACLK - unable to load private key '%s' failed.", filename);
        return 1;
    }
    netdata_log_debug(D_ACLK, "Claimed agent loaded private key len=%ld bytes", bytes_read);

    BIO *key_bio = BIO_new_mem_buf(private_key, -1);
    if (key_bio==NULL) {
        netdata_log_error("ACLK: Claimed agent cannot establish ACLK - failed to create BIO for key");
        goto biofailed;
    }

#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_300
    aclk_dctx = OSSL_DECODER_CTX_new_for_pkey(&aclk_private_key, "PEM", NULL,
                                              "RSA",
                                              OSSL_KEYMGMT_SELECT_PRIVATE_KEY,
                                              NULL, NULL);

    if (!aclk_dctx) {
        netdata_log_error("ACLK: Loading private key (from claiming) failed - no OpenSSL Decoders found");
        goto biofailed;
    }

    // this is necesseary to avoid RSA key with wrong size
    if (!OSSL_DECODER_from_bio(aclk_dctx, key_bio)) {
        netdata_log_error("ACLK: Decoding private key (from claiming) failed - invalid format.");
        goto biofailed;
    }
#else
    aclk_private_key = PEM_read_bio_RSAPrivateKey(key_bio, NULL, NULL, NULL);
#endif
    BIO_free(key_bio);
    if (aclk_private_key!=NULL)
    {
        freez(private_key);
        return 0;
    }
    char err[512];
    ERR_error_string_n(ERR_get_error(), err, sizeof(err));
    netdata_log_error("ACLK: Claimed agent cannot establish ACLK - cannot create private key: %s", err);

biofailed:
    freez(private_key);
    return 1;
}

/**
 * Will block until agent is claimed. Returns only if agent claimed
 * or if agent needs to shutdown.
 * 
 * @return `0` if agent has been claimed, 
 * `1` if interrupted due to agent shutting down
 */
static int wait_till_agent_claimed(void)
{
    ND_UUID uuid = claim_id_get_uuid();
    while (likely(UUIDiszero(uuid))) {
        sleep_usec(USEC_PER_SEC * 1);
        if (!service_running(SERVICE_ACLK))
            return 1;
        uuid = claim_id_get_uuid();
    }
    return 0;
}

/**
 * Checks everything is ready for connection
 * agent claimed, cloud url set and private key available
 * 
 * @param aclk_hostname points to location where string pointer to hostname will be set
 * @param aclk_port port to int where port will be saved
 * 
 * @return If non 0 returned irrecoverable error happened (or exit_initiated) and ACLK should be terminated
 */
static int wait_till_agent_claim_ready()
{
    url_t url;
    while (service_running(SERVICE_ACLK)) {
        if (wait_till_agent_claimed())
            return 1;

        // The NULL return means the value was never initialised, but this value has been initialized in post_conf_load.
        // We trap the impossible NULL here to keep the linter happy without using a fatal() in the code.
        const char *cloud_base_url = cloud_config_url_get();
        if (cloud_base_url == NULL) {
            netdata_log_error("ACLK: Do not move the \"url\" out of netdata_conf_section_global_run_as_user!!");
            return 1;
        }

        // We just check configuration is valid here
        // TODO make it without malloc/free
        memset(&url, 0, sizeof(url_t));
        if (url_parse(cloud_base_url, &url)) {
            netdata_log_error("ACLK: Agent is claimed but the URL in configuration key \"url\" is invalid, please fix");
            url_t_destroy(&url);
            sleep(5);
            continue;
        }
        url_t_destroy(&url);

        if (!load_private_key())
            return 0;

        sleep(5);
    }

    return 1;
}

static void msg_callback(const char *topic, const void *msg, size_t msglen, int qos)
{
    UNUSED(qos);
    aclk_rcvd_cloud_msgs++;

    netdata_log_debug(D_ACLK, "Got Message From Broker Topic \"%s\" QOS %d", topic, qos);

    if (aclk_shared_state.mqtt_shutdown_msg_id > 0) {
        netdata_log_error("ACLK: Link is shutting down. Ignoring incoming message.");
        return;
    }

    const char *msgtype = strrchr(topic, '/');
    if (unlikely(!msgtype)) {
        error_report("Cannot get message type from topic. Ignoring message from topic \"%s\"", topic);
        return;
    }
    msgtype++;
    if (unlikely(!*msgtype)) {
        error_report("Message type empty. Ignoring message from topic \"%s\"", topic);
        return;
    }

#ifdef ACLK_LOG_CONVERSATION_DIR
#define FN_MAX_LEN 512
    char filename[FN_MAX_LEN];
    int logfd;
    snprintf(filename, FN_MAX_LEN, ACLK_LOG_CONVERSATION_DIR "/%010d-rx-%s.bin", ACLK_GET_CONV_LOG_NEXT(), msgtype);
    logfd = open(filename, O_CREAT | O_TRUNC | O_WRONLY, S_IRUSR | S_IWUSR );
    if(logfd < 0)
        netdata_log_error("ACLK: Error opening ACLK Conversation logfile \"%s\" for RX message.", filename);
    write(logfd, msg, msglen);
    close(logfd);
#endif

    aclk_handle_new_cloud_msg(msgtype, msg, msglen, topic);
}

static void puback_callback(uint16_t packet_id)
{
    if (++aclk_pubacks_per_conn == ACLK_PUBACKS_CONN_STABLE) {
        last_conn_time_appl = now_realtime_sec();
        aclk_tbeb_reset();
    }

//#ifdef NETDATA_INTERNAL_CHECKS
//    aclk_stats_msg_puback(packet_id);
//#endif

    if (aclk_shared_state.mqtt_shutdown_msg_id == (int)packet_id) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "Shutdown message has been acknowledged by the cloud. Exiting gracefully");

        aclk_shared_state.mqtt_shutdown_msg_rcvd = 1;
    }
}

void aclk_graceful_disconnect(mqtt_wss_client client);

bool schedule_node_update = false;
/* Keeps connection alive and handles all network communications.
 * Returns on error or when netdata is shutting down.
 * @param client instance of mqtt_wss_client
 * @returns  0 - Netdata Exits
 *          >0 - Error happened. Reconnect and start over.
 */
static int handle_connection(mqtt_wss_client client)
{
    while (service_running(SERVICE_ACLK)) {
        // timeout 1000 to check at least once a second
        // for exit_initiated
        int rc = mqtt_wss_service(client, 1000);
        if (rc < 0){
            worker_is_busy(WORKER_ACLK_DISCONNECTED);
            error_report("Connection Error or Dropped");

            if(rc == MQTT_WSS_ERR_REMOTE_CLOSED)
                aclk_status_set(ACLK_STATUS_OFFLINE_CLOSED_BY_REMOTE);
            else if(rc == MQTT_WSS_ERR_PROTO_MQTT)
                aclk_status_set(ACLK_STATUS_OFFLINE_MQTT_PROTOCOL_ERROR);
            else if(rc == MQTT_WSS_ERR_PROTO_WS)
                aclk_status_set(ACLK_STATUS_OFFLINE_WS_PROTOCOL_ERROR);
            else if(rc == MQTT_WSS_ERR_MSG_TOO_BIG)
                aclk_status_set(ACLK_STATUS_OFFLINE_MESSAGE_TOO_BIG);
            else if(rc == MQTT_WSS_ERR_POLL_FAILED)
                aclk_status_set(ACLK_STATUS_OFFLINE_POLL_ERROR);
            else /* if(rc == MQTT_WSS_ERR_CONN_DROP) */
                aclk_status_set(ACLK_STATUS_OFFLINE_SOCKET_ERROR);

            return 1;
        }

        if (disconnect_req != ACLK_NO_DISCONNECT) {
            const char *reason;
            switch (disconnect_req) {
                case ACLK_CLOUD_DISCONNECT:
                    worker_is_busy(WORKER_ACLK_CMD_DISCONNECT);
                    reason = "cloud request";
                    aclk_status_set(ACLK_STATUS_OFFLINE_CLOUD_REQUESTED_DISCONNECT);
                    break;
                case ACLK_PING_TIMEOUT:
                    worker_is_busy(WORKER_ACLK_CMD_TIMEOUT);
                    reason = "ping timeout";
                    schedule_node_update = true;
                    aclk_status_set(ACLK_STATUS_OFFLINE_PING_TIMEOUT);
                    break;
                case ACLK_RELOAD_CONF:
                    worker_is_busy(WORKER_ACLK_CMD_RELOAD_CONF);
                    reason = "reclaim";
                    aclk_status_set(ACLK_STATUS_OFFLINE_RELOADING_CONFIG);
                    break;
                default:
                    worker_is_busy(WORKER_ACLK_CMD_UNKNOWN);
                    reason = "unknown";
                    aclk_status_set(ACLK_STATUS_OFFLINE);
                    break;
            }

            nd_log(NDLS_DAEMON, NDLP_NOTICE, "Going to restart connection due to \"%s\"", reason);

            disconnect_req = ACLK_NO_DISCONNECT;
            aclk_graceful_disconnect(client);
            aclk_shared_state.mqtt_shutdown_msg_id = -1;
            aclk_shared_state.mqtt_shutdown_msg_rcvd = 0;
            return 1;
        }
    }
    return 0;
}

static inline void mqtt_connected_actions(mqtt_wss_client client)
{
    char *topic = (char*)aclk_get_topic(ACLK_TOPICID_COMMAND);

    if (!topic)
        netdata_log_error("ACLK: Unable to fetch topic for COMMAND (to subscribe)");
    else
        mqtt_wss_subscribe(client, topic, 1);

    topic = (char*)aclk_get_topic(ACLK_TOPICID_CMD_NG_V1);
    if (!topic)
        netdata_log_error("ACLK: Unable to fetch topic for protobuf COMMAND (to subscribe)");
    else
        mqtt_wss_subscribe(client, topic, 1);

    aclk_set_connected();
    aclk_pubacks_per_conn = 0;
    aclk_rcvd_cloud_msgs = 0;
    aclk_connection_counter++;

    size_t iter = 0;
    while ((topic = (char*)aclk_topic_cache_iterate(&iter)) != NULL)
        mqtt_wss_set_topic_alias(client, topic);

    aclk_send_agent_connection_update(client, 1);
}

void aclk_graceful_disconnect(mqtt_wss_client client)
{
    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "Preparing to gracefully shutdown ACLK connection");

    aclk_shared_state.mqtt_shutdown_msg_id = aclk_send_agent_connection_update(client, 0);

    time_t t = now_monotonic_sec();
    while (!mqtt_wss_service(client, 100)) {
        if (now_monotonic_sec() - t >= 2) {
            netdata_log_error("ACLK: Wasn't able to gracefully shutdown ACLK in time!");
            break;
        }
        if (aclk_shared_state.mqtt_shutdown_msg_rcvd) {
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "MQTT App Layer `disconnect` message sent successfully");
            break;
        }
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "ACLK link is down");
    nd_log(NDLS_ACCESS, NDLP_WARNING, "ACLK DISCONNECTED");

    last_disconnect_time = now_realtime_sec();
    aclk_set_disconnected();

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "Attempting to gracefully shutdown the MQTT/WSS connection");

    mqtt_wss_disconnect(client, 1000);
}

static unsigned long aclk_reconnect_delay() {
    unsigned long recon_delay;
    time_t now;

    if (aclk_disable_runtime) {
        aclk_tbeb_reset();
        return 60 * MSEC_PER_SEC;
    }

    now = now_monotonic_sec();
    if (aclk_block_until) {
        if (now < aclk_block_until) {
            recon_delay = aclk_block_until - now;
            recon_delay *= MSEC_PER_SEC;
            aclk_block_until = 0;
            aclk_tbeb_reset();
            return recon_delay;
        }
        aclk_block_until = 0;
    }

    if (!aclk_env || !aclk_env->backoff.base)
        return aclk_tbeb_delay(0, 2, 0, 1024);

    return aclk_tbeb_delay(0, aclk_env->backoff.base, aclk_env->backoff.min_s, aclk_env->backoff.max_s);
}

/* Block till aclk_reconnect_delay is satisfied or exit_initiated is signalled
 * @return 0 - Go ahead and connect (delay expired)
 *         1 - exit_initiated
 */
#define NETDATA_EXIT_POLL_MS (MSEC_PER_SEC/4)
static int aclk_block_till_recon_allowed() {
    unsigned long recon_delay = aclk_reconnect_delay();

    next_connection_attempt = now_realtime_sec() + (recon_delay / MSEC_PER_SEC);
    last_backoff_value = (float)recon_delay / MSEC_PER_SEC;

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "Wait before attempting to reconnect in %.3f seconds", recon_delay / (float)MSEC_PER_SEC);

    // we want to wake up from time to time to check exit_initiated
    worker_is_busy(WORKER_ACLK_WAITING_TO_CONNECT);
    while (recon_delay)
    {
        if (!service_running(SERVICE_ACLK))
            return 1;
        if (recon_delay > NETDATA_EXIT_POLL_MS) {
            sleep_usec(NETDATA_EXIT_POLL_MS * USEC_PER_MS);
            recon_delay -= NETDATA_EXIT_POLL_MS;
            continue;
        }
        sleep_usec(recon_delay * USEC_PER_MS);
        recon_delay = 0;
    }

    worker_is_busy(WORKER_ACLK_CONNECT);
    return !service_running(SERVICE_ACLK);
}

#ifndef ACLK_DISABLE_CHALLENGE
/* Cloud returns transport list ordered with highest
 * priority first. This function selects highest prio
 * transport that we can actually use (support)
 */
static int aclk_get_transport_idx(aclk_env_t *env) {
    for (size_t i = 0; i < env->transport_count; i++) {
        // currently we support only MQTT 5
        // therefore select first transport that matches
        if (env->transports[i]->type == ACLK_TRP_MQTT_5) {
            return i;
        }
    }
    return -1;
}
#endif

ACLK_STATUS aclk_status = ACLK_STATUS_OFFLINE;

const char *aclk_status_to_string(void) {
    if(aclk_status == ACLK_STATUS_CONNECTED)
        return "connected";

    if((int)aclk_status < (int)ND_SOCK_ERR_MAX)
        return ND_SOCK_ERROR_2str((ND_SOCK_ERROR)aclk_status);

    if((int)aclk_status < (int)HTTPS_CLIENT_RESP_MAX)
        return https_client_resp_t_2str((https_client_resp_t)aclk_status);

    switch(aclk_status) {
        case ACLK_STATUS_CONNECTED:
            return "connected";

        case ACLK_STATUS_OFFLINE:
            return "offline";

        case ACLK_STATUS_DISABLED:
            return "disabled";

        case ACLK_STATUS_CANT_CONNECT_NO_CLOUD_URL:
            return "configuration error, no cloud url";

        case ACLK_STATUS_CANT_CONNECT_INVALID_CLOUD_URL:
            return "configuration error, invalid cloud url";

        case ACLK_STATUS_BLOCKED:
            return "agent is blocked";

        case ACLK_STATUS_NO_OLD_PROTOCOL:
            return "can't connect, old protocol not supported";

        case ACLK_STATUS_NO_PROTOCOL_CAPABILITY:
            return "can't connect, protocol capability not supported";

        case ACLK_STATUS_INVALID_ENV_AUTH_URL:
            return "can't connect, invalid /env auth url";

        case ACLK_STATUS_INVALID_ENV_TRANSPORT_IDX:
            return "can't connect, invalid /env transport idx";

        case ACLK_STATUS_INVALID_ENV_TRANSPORT_URL:
            return "can't connect, invalid /env transport URL";

        case ACLK_STATUS_NO_LWT_TOPIC:
            return "can't connect, no LWT topic";

        case ACLK_STATUS_OFFLINE_CLOUD_REQUESTED_DISCONNECT:
            return "disconnected, due to remote request";

        case ACLK_STATUS_OFFLINE_PING_TIMEOUT:
            return "disconnected, ping timed out";

        case ACLK_STATUS_OFFLINE_RELOADING_CONFIG:
            return "disconnected, to reload config";

        case ACLK_STATUS_OFFLINE_POLL_ERROR:
            return "disconnected, poll() failed";

        case ACLK_STATUS_OFFLINE_CLOSED_BY_REMOTE:
            return "disconnected, closed by remote end";

        case ACLK_STATUS_OFFLINE_SOCKET_ERROR:
            return "disconnected, socket error";

        case ACLK_STATUS_OFFLINE_MQTT_PROTOCOL_ERROR:
            return "disconnected, MQTT protocol error";

        case ACLK_STATUS_OFFLINE_WS_PROTOCOL_ERROR:
            return "disconnected, WebSockets protocol error";

        case ACLK_STATUS_OFFLINE_MESSAGE_TOO_BIG:
            return "disconnected, message too big";

        default:
            return "unknown status";
    }
}

void aclk_status_set(ACLK_STATUS status) {
    aclk_status = status;

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &aclk_connection_msgid),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, status == ACLK_STATUS_CONNECTED ? NDLP_INFO : NDLP_ERR,
           "Netdata Cloud, ACLK connection status: %s", aclk_status_to_string());
}

const char *aclk_cloud_base_url = NULL;

/* Attempts to make a connection to MQTT broker over WSS
 * @param client instance of mqtt_wss_client
 * @return  0 - Successful Connection,
 *          <0 - Irrecoverable Error -> Kill ACLK,
 *          >0 - exit_initiated
 */
#define CLOUD_BASE_URL_READ_RETRY 30
static int aclk_attempt_to_connect(mqtt_wss_client client)
{
    https_client_resp_t rc;

    url_t base_url;

#ifndef ACLK_DISABLE_CHALLENGE
    url_t auth_url;
    url_t mqtt_url;
#endif

    bool fallback_ipv4 = false;
    while (service_running(SERVICE_ACLK)) {
        aclk_cloud_base_url = cloud_config_url_get();
        if (aclk_cloud_base_url == NULL) {
            error_report("ACLK: cloud base URL is empty.");
            aclk_status_set(ACLK_STATUS_CANT_CONNECT_NO_CLOUD_URL);
            return -1;
        }

        if (aclk_block_till_recon_allowed()) {
            aclk_status_set(ACLK_STATUS_BLOCKED);
            return 1;
        }

        nd_log(NDLS_DAEMON, NDLP_DEBUG, "ACLK: attempting to connect now");

        memset(&base_url, 0, sizeof(url_t));
        if (url_parse(aclk_cloud_base_url, &base_url)) {
            aclk_status_set(ACLK_STATUS_CANT_CONNECT_INVALID_CLOUD_URL);
            error_report("ACLK: base URL '%s' cannot be parsed.", aclk_cloud_base_url);
            sleep(CLOUD_BASE_URL_READ_RETRY);
            url_t_destroy(&base_url);
            continue;
        }

        struct mqtt_wss_proxy proxy_conf = { .host = NULL, .port = 0, .username = NULL, .password = NULL, .type = MQTT_WSS_DIRECT };
        aclk_set_proxy((char**)&proxy_conf.host, &proxy_conf.port, (char**)&proxy_conf.username, (char**)&proxy_conf.password, &proxy_conf.type);

        struct mqtt_connect_params mqtt_conn_params = {
            .clientid   = "anon",
            .username   = "anon",
            .password   = "anon",
            .will_topic = "lwt",
            .will_msg   = NULL,
            .will_flags = MQTT_WSS_PUB_QOS2,
            .keep_alive = 60,
            .drop_on_publish_fail = 1
        };

#ifndef ACLK_DISABLE_CHALLENGE
        if (aclk_env) {
            aclk_env_t_destroy(aclk_env);
            freez(aclk_env);
        }
        aclk_env = callocz(1, sizeof(aclk_env_t));

        rc = aclk_get_env(aclk_env, base_url.host, base_url.port, &fallback_ipv4);
        url_t_destroy(&base_url);
        if (rc != HTTPS_CLIENT_RESP_OK) {
            aclk_status_set((ACLK_STATUS)rc);
            aclk_env_t_destroy(aclk_env);
            freez(aclk_env);
            aclk_env = NULL;
            continue;
        }

        if (!service_running(SERVICE_ACLK)) {
            aclk_status_set(ACLK_STATUS_DISABLED);
            return 1;
        }

        if (aclk_env->encoding != ACLK_ENC_PROTO) {
            aclk_status_set(ACLK_STATUS_NO_OLD_PROTOCOL);
            error_report("ACLK: this agent can only use the new cloud protocol but cloud requested old one.");
            continue;
        }

        if (!aclk_env_has_capa("proto")) {
            aclk_status_set(ACLK_STATUS_NO_PROTOCOL_CAPABILITY);
            error_report("ACLK: can't use encoding=proto without at least \"proto\" capability.");
            continue;
        }

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "ACLK: new ACLK protobuf protocol negotiated successfully (/env response).");

        memset(&auth_url, 0, sizeof(url_t));
        if (url_parse(aclk_env->auth_endpoint, &auth_url)) {
            aclk_status_set(ACLK_STATUS_INVALID_ENV_AUTH_URL);
            error_report("ACLK: parsing URL returned by env endpoint for authentication failed. \"%s\"", aclk_env->auth_endpoint);
            url_t_destroy(&auth_url);
            continue;
        }

        rc = aclk_get_mqtt_otp(aclk_private_key, (char **)&mqtt_conn_params.clientid, (char **)&mqtt_conn_params.username, (char **)&mqtt_conn_params.password, &auth_url, &fallback_ipv4);
        url_t_destroy(&auth_url);
        if (rc != HTTPS_CLIENT_RESP_OK) {
            aclk_status_set((ACLK_STATUS)rc);
            error_report("ACLK: error passing Challenge/Response to get OTP");
            continue;
        }

        // aclk_get_topic moved here as during OTP we
        // generate the topic cache
        mqtt_conn_params.will_topic = aclk_get_topic(ACLK_TOPICID_AGENT_CONN);

        if (!mqtt_conn_params.will_topic) {
            aclk_status_set(ACLK_STATUS_NO_LWT_TOPIC);
            error_report("ACLK: couldn't get LWT topic. Will not send LWT.");
            continue;
        }

        // Do the MQTT connection
        int trp = aclk_get_transport_idx(aclk_env);
        if (trp < 0) {
            aclk_status_set(ACLK_STATUS_INVALID_ENV_TRANSPORT_IDX);
            error_report("ACLK: cloud /env endpoint didn't return any transport usable by this agent.");
            continue;
        }

        memset(&mqtt_url, 0, sizeof(url_t));
        if (url_parse(aclk_env->transports[rc]->endpoint, &mqtt_url)){
            aclk_status_set(ACLK_STATUS_INVALID_ENV_TRANSPORT_URL);
            error_report("ACLK: failed to parse target URL for /env trp idx %d \"%s\"", trp, aclk_env->transports[rc]->endpoint);
            url_t_destroy(&mqtt_url);
            continue;
        }
#endif

        aclk_session_newarch = now_realtime_usec();
        aclk_session_sec = aclk_session_newarch / USEC_PER_SEC;
        aclk_session_us = aclk_session_newarch % USEC_PER_SEC;

        mqtt_conn_params.will_msg = aclk_generate_lwt(&mqtt_conn_params.will_msg_len);

        int ssl_flags = cloud_config_insecure_get() ? MQTT_WSS_SSL_ALLOW_SELF_SIGNED : MQTT_WSS_SSL_CERT_CHECK_FULL;

#ifdef ACLK_DISABLE_CHALLENGE
        int mqtt_rc = mqtt_wss_connect(client, base_url.host, base_url.port, &mqtt_conn_params, ssl_flags, &proxy_conf);
        url_t_destroy(&base_url);
#else
        int mqtt_rc = mqtt_wss_connect(client, mqtt_url.host, mqtt_url.port, &mqtt_conn_params, ssl_flags, &proxy_conf, &fallback_ipv4);
        url_t_destroy(&mqtt_url);

        freez((char*)mqtt_conn_params.clientid);
        freez((char*)mqtt_conn_params.password);
        freez((char*)mqtt_conn_params.username);
#endif

        freez((char*)mqtt_conn_params.will_msg);
        freez((char*)proxy_conf.host);
        freez((char*)proxy_conf.username);
        freez((char*)proxy_conf.password);

        if (!mqtt_rc) {
            last_conn_time_mqtt = now_realtime_sec();
            nd_log(NDLS_DAEMON, NDLP_INFO, "ACLK: connection successfully established");
            aclk_status_set(ACLK_STATUS_CONNECTED);
            nd_log(NDLS_ACCESS, NDLP_INFO, "ACLK CONNECTED");
            mqtt_connected_actions(client);
            fallback_ipv4 = false;
            return 0;
        }

        error_report("ACLK: connection failed");
    }

    aclk_status_set(ACLK_STATUS_DISABLED);
    return 1;
}

/**
 * Main agent cloud link thread
 *
 * This thread will simply call the main event loop that handles
 * pending requests - both inbound and outbound
 *
 * @param ptr is a pointer to the netdata_static_thread structure.
 *
 * @return It always returns NULL
 */
void *aclk_main(void *ptr)
{
    struct netdata_static_thread *static_thread = ptr;

    worker_register("ACLK");
    worker_register_job_name(WORKER_ACLK_WAIT_CLAIMING, "wait claim");
    worker_register_job_name(WORKER_ACLK_CONNECT, "connect");
    worker_register_job_name(WORKER_ACLK_NODE_UPDATE, "node update");
    worker_register_job_name(WORKER_ACLK_HANDLE_CONNECTION, "handle connection");
    worker_register_job_name(WORKER_ACLK_DISCONNECTED, "disconnected");
    worker_register_job_name(WORKER_ACLK_CMD_DISCONNECT, "cmd disconnect");
    worker_register_job_name(WORKER_ACLK_CMD_TIMEOUT, "cmd timeout");
    worker_register_job_name(WORKER_ACLK_CMD_RELOAD_CONF, "cmd reload");
    worker_register_job_name(WORKER_ACLK_CMD_UNKNOWN, "cmd unknown");
    worker_register_job_name(WORKER_ACLK_SENT_PING, "sent ping");
    worker_register_job_name(WORKER_ACLK_POLL_ERROR, "poll error");
    worker_register_job_name(WORKER_ACLK_POLL_OK, "poll ok");
    worker_register_job_name(WORKER_ACLK_RX, "rx");
    worker_register_job_name(WORKER_ACLK_RX_ERROR, "rx error");
    worker_register_job_name(WORKER_ACLK_PROCESS_RAW, "p-raw");
    worker_register_job_name(WORKER_ACLK_PROCESS_HANDSHAKE, "p-handshake");
    worker_register_job_name(WORKER_ACLK_PROCESS_ESTABLISHED, "p-established");
    worker_register_job_name(WORKER_ACLK_PROCESS_ERROR, "p-error");
    worker_register_job_name(WORKER_ACLK_PROCESS_CLOSED_GRACEFULLY, "p-closed");
    worker_register_job_name(WORKER_ACLK_PROCESS_UNKNOWN, "p-unknown");
    worker_register_job_name(WORKER_ACLK_HANDLE_MQTT_INTERNAL, "mqtt internal");
    worker_register_job_name(WORKER_ACLK_TX, "tx");
    worker_register_job_name(WORKER_ACLK_TX_ERROR, "tx error");
    worker_register_job_name(WORKER_ACLK_TRY_SEND_ALL, "try send all");
    worker_register_job_name(WORKER_ACLK_HANDLE_INCOMING, "handle incoming");
    worker_register_job_name(WORKER_ACLK_CPT_CONNACK, "cpt connack");
    worker_register_job_name(WORKER_ACLK_CPT_PUBACK, "cpt puback");
    worker_register_job_name(WORKER_ACLK_CPT_PINGRESP, "cpt pingresp");
    worker_register_job_name(WORKER_ACLK_CPT_SUBACK, "cpt suback");
    worker_register_job_name(WORKER_ACLK_CPT_PUBLISH, "cpt publish");
    worker_register_job_name(WORKER_ACLK_CPT_DISCONNECT, "cpt disconnect");
    worker_register_job_name(WORKER_ACLK_CPT_UNKNOWN, "cpt unknown");
    worker_register_job_name(WORKER_ACLK_SEND_FRAGMENT, "send fragment");
    worker_register_job_name(WORKER_ACLK_MSG_CALLBACK, "msg callback");
    worker_register_job_name(WORKER_ACLK_WAITING_TO_CONNECT, "conn wait");

    ACLK_PROXY_TYPE proxy_type;
    aclk_get_proxy(&proxy_type);
    if (proxy_type == PROXY_TYPE_SOCKS5) {
        netdata_log_error("ACLK: SOCKS5 proxy is not supported by ACLK-NG yet.");
        static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
        return NULL;
    }

    aclk_init_rx_msg_handlers();

    worker_is_busy(WORKER_ACLK_WAIT_CLAIMING);
    if (wait_till_agent_claim_ready())
        goto exit;

    if (!((mqttwss_client = mqtt_wss_new(msg_callback, puback_callback)))) {
        netdata_log_error("ACLK: Couldn't initialize MQTT_WSS network library");
        goto exit;
    }

#ifdef MQTT_WSS_DEBUG
    size_t default_ssl_log_filename_size = strlen(netdata_configured_log_dir) + strlen(DEFAULT_SSKEYLOGFILE_NAME) + 2;
    char *default_ssl_log_filename = mallocz(default_ssl_log_filename_size);
    snprintfz(default_ssl_log_filename, default_ssl_log_filename_size, "%s/%s", netdata_configured_log_dir, DEFAULT_SSKEYLOGFILE_NAME);
    ssl_log_filename = inicfg_get(&netdata_config, CONFIG_SECTION_CLOUD, "aclk ssl keylog file", default_ssl_log_filename);
    freez(default_ssl_log_filename);
    if (ssl_log_filename) {
        error_report("SSLKEYLOGFILE active (path:\"%s\")!", ssl_log_filename);
        mqtt_wss_set_SSL_CTX_keylog_cb(mqttwss_client, aclk_ssl_keylog_cb);
    }
#endif

    // Enable MQTT buffer growth if necessary
    // e.g. old cloud architecture clients with huge nodes
    // that send JSON payloads of 10 MB as single messages
    mqtt_wss_set_max_buf_size(mqttwss_client, 25*1024*1024);

    // Keep reconnecting and talking until our time has come
    // and the Grim Reaper (exit_initiated) calls
    netdata_log_info("ACLK: Starting ACLK query event loop");
    aclk_mqtt_client_set(mqttwss_client);
    bool client_to_reset = true;
    do {
        worker_is_busy(WORKER_ACLK_CONNECT);
        if (aclk_attempt_to_connect(mqttwss_client))
            goto exit_full;

        if (schedule_node_update) {
            worker_is_busy(WORKER_ACLK_NODE_UPDATE);
            schedule_node_state_update(localhost, 10);
            schedule_node_update = false;
        }

        worker_is_busy(WORKER_ACLK_HANDLE_CONNECTION);
        if (handle_connection(mqttwss_client)) {
            worker_is_busy(WORKER_ACLK_DISCONNECTED);
            last_disconnect_time = now_realtime_sec();
            aclk_set_disconnected();
            nd_log(NDLS_ACCESS, NDLP_WARNING, "ACLK DISCONNECTED");
        }
    } while (service_running(SERVICE_ACLK));
    aclk_mqtt_client_reset();
    // No need to reset the client again when exiting
    client_to_reset = false;
    worker_is_busy(WORKER_ACLK_DISCONNECTED);
    aclk_graceful_disconnect(mqttwss_client);

#ifdef MQTT_WSS_DEBUG
    if (ssl_log_file)
        fclose(ssl_log_file);
#endif

exit_full:
    free_topic_cache();
    if (client_to_reset)
        aclk_mqtt_client_reset();
    mqtt_wss_destroy(mqttwss_client);
exit:
    if (aclk_env) {
        aclk_env_t_destroy(aclk_env);
        freez(aclk_env);
    }
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
    return NULL;
}

bool aclk_host_state_update_auto(RRDHOST *host) {
    int live;
    switch(rrdhost_ingestion_status(host)) {
        default:
        case RRDHOST_INGEST_STATUS_ARCHIVED:
        case RRDHOST_INGEST_STATUS_INITIALIZING:
        case RRDHOST_INGEST_STATUS_OFFLINE:
            live = 0;
            break;

        case RRDHOST_INGEST_STATUS_REPLICATING:
            // receiving replication
            // no need to send this to NC
            return false;

        case RRDHOST_INGEST_STATUS_ONLINE:
            // currently collecting data
            live = 1;
            break;
    }
    aclk_host_state_update(host, live, 1);
    return true;
}

void aclk_create_node_instance_job(RRDHOST *host)
{
    if (unlikely(!host))
        return;

    CLAIM_ID claim_id = claim_id_get();
    if (!claim_id_is_set(claim_id))
        return;

    aclk_query_t *query = aclk_query_new(REGISTER_NODE);
    int32_t hops =  rrdhost_ingestion_hops(host);
    node_instance_creation_t node_instance_creation = {
        .hops = hops,
        .hostname = rrdhost_hostname(host),
        .machine_guid = host->machine_guid,
        .claim_id = claim_id.str
    };

    query->data.bin_payload.topic = ACLK_TOPICID_CREATE_NODE;
    query->data.bin_payload.msg_name = "CreateNodeInstance";
    query->data.bin_payload.payload = generate_node_instance_creation(&query->data.bin_payload.size, &node_instance_creation);

    nd_log_daemon(NDLP_DEBUG, "Queuing registration for host=%s, hops=%d", host->machine_guid, hops);

    aclk_add_job(query);
}

void aclk_update_node_instance_job(RRDHOST *host, int live, int queryable)
{
    if (unlikely(!host))
        return;

    CLAIM_ID claim_id = claim_id_get();
    if (!claim_id_is_set(claim_id))
        return;

    aclk_query_t *query = aclk_query_new(NODE_STATE_UPDATE);

    int32_t hops = rrdhost_ingestion_hops(host);
    node_instance_connection_t node_state_update = {
        .claim_id = claim_id.str,
        .hops = hops,
        .live = live,
        .queryable = queryable,
        .session_id = aclk_session_newarch};

    char node_id[UUID_STR_LEN];
    uuid_unparse_lower(host->node_id.uuid, node_id);

    node_state_update.node_id = node_id;
    node_state_update.capabilities = aclk_get_node_instance_capas(host);

    query->data.bin_payload.topic = ACLK_TOPICID_NODE_CONN;
    query->data.bin_payload.msg_name = "UpdateNodeInstanceConnection";
    query->data.bin_payload.payload = generate_node_instance_connection(&query->data.bin_payload.size, &node_state_update);

    nd_log_daemon(
        NDLP_DEBUG,
        "Queuing status update for node=%s, live=%d, hops=%d, queryable=%d",
        (char *)node_state_update.node_id,
        live,
        hops,
        queryable);

    freez((void *)node_state_update.capabilities);
    aclk_add_job(query);
}

void aclk_host_state_update(RRDHOST *host, int live, int queryable)
{
    if (!aclk_online())
        return;

    if (uuid_is_null(host->node_id.uuid))
        aclk_create_node_instance_job(host);
    else
        aclk_update_node_instance_job(host, live, queryable);
}

void aclk_send_node_instances()
{
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host)
    {
        int live = rrdhost_ingestion_status(host) == RRDHOST_INGEST_STATUS_ONLINE ? 1 : 0;
        aclk_host_state_update(host, live, 1);
    }
    dfe_done(host);
}

void aclk_send_bin_msg(char *msg, size_t msg_len, enum aclk_topics subtopic, const char *msgname)
{
    aclk_send_bin_message_subtopic_pid(mqttwss_client, msg, msg_len, subtopic, msgname);
}

static void fill_alert_status_for_host(BUFFER *wb, RRDHOST *host)
{
    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_RELAXED);
    if (!aclk_host_config)
        return;

    buffer_sprintf(
        wb,
        "\n\t\tUpdates: %d"
        "\n\t\tCheckpoints: %d"
        "\n\t\tAlert count: %d"
        "\n\t\tAlert snapshot count: %d",
        aclk_host_config->stream_alerts,
        aclk_host_config->checkpoint_count,
        aclk_host_config->alert_count,
        aclk_host_config->snapshot_count);
}

char *aclk_state(void)
{
    BUFFER *wb = buffer_create(1024, &netdata_buffers_statistics.buffers_aclk);
    struct tm *tmptr, tmbuf;
    char *ret;

    buffer_strcat(wb,
        "ACLK Available: Yes\n"
        "ACLK Version: 2\n"
        "Protocols Supported: Protobuf\n"
    );
    buffer_sprintf(wb, "Protocol Used: Protobuf\nMQTT Version: %d\nClaimed: ", 5);

    CLAIM_ID claim_id = claim_id_get();
    if (!claim_id_is_set(claim_id))
        buffer_strcat(wb, "No\n");
    else {
        const char *cloud_base_url = cloud_config_url_get();
        buffer_sprintf(wb, "Yes\nClaimed Id: %s\nCloud URL: %s\n", claim_id.str, cloud_base_url ? cloud_base_url : "null");
    }

    buffer_sprintf(wb, "Online: %s\nReconnect count: %d\nBanned By Cloud: %s\n", aclk_online() ? "Yes" : "No", aclk_connection_counter > 0 ? (aclk_connection_counter - 1) : 0, aclk_disable_runtime ? "Yes" : "No");
    if (last_conn_time_mqtt && ((tmptr = localtime_r(&last_conn_time_mqtt, &tmbuf))) ) {
        char timebuf[26];
        strftime(timebuf, 26, "%Y-%m-%d %H:%M:%S", tmptr);
        buffer_sprintf(wb, "Last Connection Time: %s\n", timebuf);
    }
    if (last_conn_time_appl && ((tmptr = localtime_r(&last_conn_time_appl, &tmbuf))) ) {
        char timebuf[26];
        strftime(timebuf, 26, "%Y-%m-%d %H:%M:%S", tmptr);
        buffer_sprintf(wb, "Last Connection Time + %d PUBACKs received: %s\n", ACLK_PUBACKS_CONN_STABLE, timebuf);
    }
    if (last_disconnect_time && ((tmptr = localtime_r(&last_disconnect_time, &tmbuf))) ) {
        char timebuf[26];
        strftime(timebuf, 26, "%Y-%m-%d %H:%M:%S", tmptr);
        buffer_sprintf(wb, "Last Disconnect Time: %s\n", timebuf);
    }
    if (!aclk_connected && next_connection_attempt && ((tmptr = localtime_r(&next_connection_attempt, &tmbuf))) ) {
        char timebuf[26];
        strftime(timebuf, 26, "%Y-%m-%d %H:%M:%S", tmptr);
        buffer_sprintf(wb, "Next Connection Attempt At: %s\nLast Backoff: %.3f", timebuf, last_backoff_value);
    }

    if (aclk_online()) {
        buffer_sprintf(wb, "Received Cloud MQTT Messages: %d\nMQTT Messages Confirmed by Remote Broker (PUBACKs): %d", aclk_rcvd_cloud_msgs, aclk_pubacks_per_conn);

        RRDHOST *host;
        rrd_rdlock();
        rrdhost_foreach_read(host) {
            buffer_sprintf(wb, "\n\n> Node Instance for mGUID: \"%s\" hostname \"%s\"\n", host->machine_guid, rrdhost_hostname(host));

            buffer_strcat(wb, "\tClaimed ID: ");
            claim_id = rrdhost_claim_id_get(host);
            if(claim_id_is_set(claim_id))
                buffer_strcat(wb, claim_id.str);
            else
                buffer_strcat(wb, "null");

            if (UUIDiszero(host->node_id))
                buffer_strcat(wb, "\n\tNode ID: null\n");
            else {
                char node_id_str[UUID_STR_LEN];
                uuid_unparse_lower(host->node_id.uuid, node_id_str);
                buffer_sprintf(wb, "\n\tNode ID: %s\n", node_id_str);
            }

            buffer_sprintf(wb, "\tStreaming Hops: %d\n\tRelationship: %s",
                           rrdhost_ingestion_hops(host),
                           host == localhost ? "self" : "child");

            if (host != localhost)
                buffer_sprintf(wb, "\n\tStreaming Connection Live: %s", host->receiver ? "true" : "false");

            buffer_strcat(wb, "\n\tAlert Streaming Status:");
            fill_alert_status_for_host(wb, host);
        }
        rrd_rdunlock();
    }

    ret = strdupz(buffer_tostring(wb));
    buffer_free(wb);
    return ret;
}

static void fill_alert_status_for_host_json(json_object *obj, RRDHOST *host)
{
    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_RELAXED);
    if (!aclk_host_config)
        return;

    json_object *tmp = json_object_new_int(aclk_host_config->stream_alerts);
    json_object_object_add(obj, "updates", tmp);

    tmp = json_object_new_int(aclk_host_config->checkpoint_count);
    json_object_object_add(obj, "checkpoint-count", tmp);

    tmp = json_object_new_int(aclk_host_config->alert_count);
    json_object_object_add(obj, "alert-count", tmp);

    tmp = json_object_new_int(aclk_host_config->snapshot_count);
    json_object_object_add(obj, "alert-snapshot-count", tmp);
    tmp = json_object_new_int64(calculate_node_alert_version(aclk_host_config->host));
    json_object_object_add(obj, "alert-version", tmp);
}

static json_object *timestamp_to_json(const time_t *t)
{
    struct tm *tmptr, tmbuf;
    if (*t && ((tmptr = gmtime_r(t, &tmbuf))) ) {
        char timebuf[26];
        strftime(timebuf, 26, "%Y-%m-%d %H:%M:%S", tmptr);
        return json_object_new_string(timebuf);
    }
    return NULL;
}

char *aclk_state_json(void)
{
    json_object *tmp, *grp, *msg = json_object_new_object();

    tmp = json_object_new_boolean(1);
    json_object_object_add(msg, "aclk-available", tmp);

    tmp = json_object_new_int(2);
    json_object_object_add(msg, "aclk-version", tmp);

    grp = json_object_new_array();
    tmp = json_object_new_string("Protobuf");
    json_object_array_add(grp, tmp);
    json_object_object_add(msg, "protocols-supported", grp);

    CLAIM_ID claim_id = claim_id_get();
    tmp = json_object_new_boolean(claim_id_is_set(claim_id));
    json_object_object_add(msg, "agent-claimed", tmp);

    if (claim_id_is_set(claim_id))
        tmp = json_object_new_string(claim_id.str);
    else
        tmp = NULL;
    json_object_object_add(msg, "claimed-id", tmp);

    const char *cloud_base_url = cloud_config_url_get();
    tmp = cloud_base_url ? json_object_new_string(cloud_base_url) : NULL;
    json_object_object_add(msg, "cloud-url", tmp);

    tmp = json_object_new_boolean(aclk_online());
    json_object_object_add(msg, "online", tmp);

    tmp = json_object_new_string("Protobuf");
    json_object_object_add(msg, "used-cloud-protocol", tmp);

    tmp = json_object_new_int(5);
    json_object_object_add(msg, "mqtt-version", tmp);

    tmp = json_object_new_int(aclk_rcvd_cloud_msgs);
    json_object_object_add(msg, "received-app-layer-msgs", tmp);

    tmp = json_object_new_int(aclk_pubacks_per_conn);
    json_object_object_add(msg, "received-mqtt-pubacks", tmp);

    tmp = json_object_new_int(aclk_connection_counter > 0 ? (aclk_connection_counter - 1) : 0);
    json_object_object_add(msg, "reconnect-count", tmp);

    json_object_object_add(msg, "last-connect-time-utc", timestamp_to_json(&last_conn_time_mqtt));
    json_object_object_add(msg, "last-connect-time-puback-utc", timestamp_to_json(&last_conn_time_appl));
    json_object_object_add(msg, "last-disconnect-time-utc", timestamp_to_json(&last_disconnect_time));
    json_object_object_add(msg, "next-connection-attempt-utc", !aclk_online() ? timestamp_to_json(&next_connection_attempt) : NULL);
    tmp = NULL;
    if (!aclk_online() && last_backoff_value)
        tmp = json_object_new_double(last_backoff_value);
    json_object_object_add(msg, "last-backoff-value", tmp);

    tmp = json_object_new_boolean(aclk_disable_runtime);
    json_object_object_add(msg, "banned-by-cloud", tmp);

    grp = json_object_new_array();

    RRDHOST *host;
    rrd_rdlock();
    rrdhost_foreach_read(host) {
        json_object *nodeinstance = json_object_new_object();

        tmp = json_object_new_string(rrdhost_hostname(host));
        json_object_object_add(nodeinstance, "hostname", tmp);

        tmp = json_object_new_string(host->machine_guid);
        json_object_object_add(nodeinstance, "mguid", tmp);

        claim_id = rrdhost_claim_id_get(host);
        if(claim_id_is_set(claim_id)) {
            tmp = json_object_new_string(claim_id.str);
            json_object_object_add(nodeinstance, "claimed_id", tmp);
        } else
            json_object_object_add(nodeinstance, "claimed_id", NULL);

        if (UUIDiszero(host->node_id)) {
            json_object_object_add(nodeinstance, "node-id", NULL);
        } else {
            char node_id_str[UUID_STR_LEN];
            uuid_unparse_lower(host->node_id.uuid, node_id_str);
            tmp = json_object_new_string(node_id_str);
            json_object_object_add(nodeinstance, "node-id", tmp);
        }

        tmp = json_object_new_int(rrdhost_ingestion_hops(host));
        json_object_object_add(nodeinstance, "streaming-hops", tmp);

        tmp = json_object_new_string(host == localhost ? "self" : "child");
        json_object_object_add(nodeinstance, "relationship", tmp);

        tmp = json_object_new_boolean((host->receiver || host == localhost));
        json_object_object_add(nodeinstance, "streaming-online", tmp);

        tmp = json_object_new_object();
        fill_alert_status_for_host_json(tmp, host);
        json_object_object_add(nodeinstance, "alert-sync-status", tmp);

        json_object_array_add(grp, nodeinstance);
    }
    rrd_rdunlock();
    json_object_object_add(msg, "node-instances", grp);

    char *str = strdupz(json_object_to_json_string_ext(msg, JSON_C_TO_STRING_PLAIN));
    json_object_put(msg);
    return str;
}

void add_aclk_host_labels(void) {
    RRDLABELS *labels = localhost->rrdlabels;

    rrdlabels_add(labels, "_aclk_available", "true", RRDLABEL_SRC_AUTO|RRDLABEL_SRC_ACLK);
    ACLK_PROXY_TYPE aclk_proxy;
    char *proxy_str;
    aclk_get_proxy(&aclk_proxy);

    switch(aclk_proxy) {
        case PROXY_TYPE_SOCKS5:
            proxy_str = "SOCKS5";
            break;
        case PROXY_TYPE_HTTP:
            proxy_str = "HTTP";
            break;
        default:
            proxy_str = "none";
            break;
    }

    rrdlabels_add(labels, "_mqtt_version", "5", RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_aclk_proxy", proxy_str, RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_aclk_ng_new_cloud_protocol", "true", RRDLABEL_SRC_AUTO|RRDLABEL_SRC_ACLK);
}
