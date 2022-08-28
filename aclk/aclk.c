// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk.h"

#ifdef ENABLE_ACLK
#include "aclk_stats.h"
#include "mqtt_wss_client.h"
#include "aclk_otp.h"
#include "aclk_tx_msgs.h"
#include "aclk_query.h"
#include "aclk_query_queue.h"
#include "aclk_util.h"
#include "aclk_rx_msgs.h"
#include "https_client.h"
#include "schema-wrappers/schema_wrappers.h"

#include "aclk_proxy.h"

#ifdef ACLK_LOG_CONVERSATION_DIR
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#endif

#define ACLK_STABLE_TIMEOUT 3 // Minimum delay to mark AGENT as stable

#endif /* ENABLE_ACLK */

int aclk_pubacks_per_conn = 0; // How many PubAcks we got since MQTT conn est.
int aclk_rcvd_cloud_msgs = 0;
int aclk_connection_counter = 0;
int disconnect_req = 0;

int aclk_connected = 0;
int use_mqtt_5 = 0;
int aclk_ctx_based = 0;
int aclk_disable_runtime = 0;
int aclk_stats_enabled;
int aclk_kill_link = 0;

usec_t aclk_session_us = 0;
time_t aclk_session_sec = 0;

time_t last_conn_time_mqtt = 0;
time_t last_conn_time_appl = 0;
time_t last_disconnect_time = 0;
time_t next_connection_attempt = 0;
float last_backoff_value = 0;

int aclk_alert_reloaded = 0; //1 on health log exchange, and again on health_reload

time_t aclk_block_until = 0;

#ifdef ENABLE_ACLK
mqtt_wss_client mqttwss_client;

netdata_mutex_t aclk_shared_state_mutex = NETDATA_MUTEX_INITIALIZER;
#define ACLK_SHARED_STATE_LOCK netdata_mutex_lock(&aclk_shared_state_mutex)
#define ACLK_SHARED_STATE_UNLOCK netdata_mutex_unlock(&aclk_shared_state_mutex)

struct aclk_shared_state aclk_shared_state = {
    .mqtt_shutdown_msg_id = -1,
    .mqtt_shutdown_msg_rcvd = 0
};

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
        error("Claimed agent cannot establish ACLK - unable to load private key '%s' failed.", filename);
        return 1;
    }
    debug(D_ACLK, "Claimed agent loaded private key len=%ld bytes", bytes_read);

    BIO *key_bio = BIO_new_mem_buf(private_key, -1);
    if (key_bio==NULL) {
        error("Claimed agent cannot establish ACLK - failed to create BIO for key");
        goto biofailed;
    }

#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_300
    aclk_dctx = OSSL_DECODER_CTX_new_for_pkey(&aclk_private_key, "PEM", NULL,
                                              "RSA",
                                              OSSL_KEYMGMT_SELECT_PRIVATE_KEY,
                                              NULL, NULL);

    if (!aclk_dctx) {
        error("Loading private key (from claiming) failed - no OpenSSL Decoders found");
        goto biofailed;
    }

    // this is necesseary to avoid RSA key with wrong size
    if (!OSSL_DECODER_from_bio(aclk_dctx, key_bio)) {
        error("Decoding private key (from claiming) failed - invalid format.");
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
    error("Claimed agent cannot establish ACLK - cannot create private key: %s", err);

biofailed:
    freez(private_key);
    return 1;
}

static int wait_till_cloud_enabled()
{
    info("Waiting for Cloud to be enabled");
    while (!netdata_cloud_setting) {
        sleep_usec(USEC_PER_SEC * 1);
        if (netdata_exit)
            return 1;
    }
    return 0;
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
    //TODO prevent malloc and freez
    char *agent_id = get_agent_claimid();
    while (likely(!agent_id)) {
        sleep_usec(USEC_PER_SEC * 1);
        if (netdata_exit)
            return 1;
        agent_id = get_agent_claimid();
    }
    freez(agent_id);
    return 0;
}

/**
 * Checks everything is ready for connection
 * agent claimed, cloud url set and private key available
 * 
 * @param aclk_hostname points to location where string pointer to hostname will be set
 * @param aclk_port port to int where port will be saved
 * 
 * @return If non 0 returned irrecoverable error happened (or netdata_exit) and ACLK should be terminated
 */
static int wait_till_agent_claim_ready()
{
    url_t url;
    while (!netdata_exit) {
        if (wait_till_agent_claimed())
            return 1;

        // The NULL return means the value was never initialised, but this value has been initialized in post_conf_load.
        // We trap the impossible NULL here to keep the linter happy without using a fatal() in the code.
        char *cloud_base_url = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", NULL);
        if (cloud_base_url == NULL) {
            error("Do not move the cloud base url out of post_conf_load!!");
            return 1;
        }

        // We just check configuration is valid here
        // TODO make it without malloc/free
        memset(&url, 0, sizeof(url_t));
        if (url_parse(cloud_base_url, &url)) {
            error("Agent is claimed but the URL in configuration key \"cloud base url\" is invalid, please fix");
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

void aclk_mqtt_wss_log_cb(mqtt_wss_log_type_t log_type, const char* str)
{
    switch(log_type) {
        case MQTT_WSS_LOG_ERROR:
        case MQTT_WSS_LOG_FATAL:
        case MQTT_WSS_LOG_WARN:
            error_report("%s", str);
            return;
        case MQTT_WSS_LOG_INFO:
            info("%s", str);
            return;
        case MQTT_WSS_LOG_DEBUG:
            debug(D_ACLK, "%s", str);
            return;
        default:
            error("Unknown log type from mqtt_wss");
    }
}

//TODO prevent big buffer on stack
#define RX_MSGLEN_MAX 4096
static void msg_callback(const char *topic, const void *msg, size_t msglen, int qos)
{
    UNUSED(qos);
    aclk_rcvd_cloud_msgs++;
    if (msglen > RX_MSGLEN_MAX)
        error("Incoming ACLK message was bigger than MAX of %d and got truncated.", RX_MSGLEN_MAX);

    debug(D_ACLK, "Got Message From Broker Topic \"%s\" QOS %d", topic, qos);

    if (aclk_shared_state.mqtt_shutdown_msg_id > 0) {
        error("Link is shutting down. Ignoring incoming message.");
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
        error("Error opening ACLK Conversation logfile \"%s\" for RX message.", filename);
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

#ifdef NETDATA_INTERNAL_CHECKS
    aclk_stats_msg_puback(packet_id);
#endif

    if (aclk_shared_state.mqtt_shutdown_msg_id == (int)packet_id) {
        info("Shutdown message has been acknowledged by the cloud. Exiting gracefully");
        aclk_shared_state.mqtt_shutdown_msg_rcvd = 1;
    }
}

static int read_query_thread_count()
{
    int threads = MIN(processors/2, 6);
    threads = MAX(threads, 2);
    threads = config_get_number(CONFIG_SECTION_CLOUD, "query thread count", threads);
    if(threads < 1) {
        error("You need at least one query thread. Overriding configured setting of \"%d\"", threads);
        threads = 1;
        config_set_number(CONFIG_SECTION_CLOUD, "query thread count", threads);
    }
    return threads;
}

void aclk_graceful_disconnect(mqtt_wss_client client);

/* Keeps connection alive and handles all network communications.
 * Returns on error or when netdata is shutting down.
 * @param client instance of mqtt_wss_client
 * @returns  0 - Netdata Exits
 *          >0 - Error happened. Reconnect and start over.
 */
static int handle_connection(mqtt_wss_client client)
{
    time_t last_periodic_query_wakeup = now_monotonic_sec();
    while (!netdata_exit) {
        // timeout 1000 to check at least once a second
        // for netdata_exit
        if (mqtt_wss_service(client, 1000) < 0){
            error_report("Connection Error or Dropped");
            return 1;
        }

        if (disconnect_req || aclk_kill_link) {
            info("Going to restart connection due to disconnect_req=%s (cloud req), aclk_kill_link=%s (reclaim)",
                disconnect_req ? "true" : "false",
                aclk_kill_link ? "true" : "false");
            disconnect_req = 0;
            aclk_kill_link = 0;
            aclk_graceful_disconnect(client);
            aclk_queue_unlock();
            aclk_shared_state.mqtt_shutdown_msg_id = -1;
            aclk_shared_state.mqtt_shutdown_msg_rcvd = 0;
            return 1;
        }

        // mqtt_wss_service will return faster than in one second
        // if there is enough work to do
        time_t now = now_monotonic_sec();
        if (last_periodic_query_wakeup < now) {
            // wake up at least one Query Thread at least
            // once per second
            last_periodic_query_wakeup = now;
            QUERY_THREAD_WAKEUP;
        }
    }
    return 0;
}

static inline void mqtt_connected_actions(mqtt_wss_client client)
{
    char *topic = (char*)aclk_get_topic(ACLK_TOPICID_COMMAND);

    if (!topic)
        error("Unable to fetch topic for COMMAND (to subscribe)");
    else
        mqtt_wss_subscribe(client, topic, 1);

    topic = (char*)aclk_get_topic(ACLK_TOPICID_CMD_NG_V1);
    if (!topic)
        error("Unable to fetch topic for protobuf COMMAND (to subscribe)");
    else
        mqtt_wss_subscribe(client, topic, 1);

    aclk_stats_upd_online(1);
    aclk_connected = 1;
    aclk_pubacks_per_conn = 0;
    aclk_rcvd_cloud_msgs = 0;
    aclk_connection_counter++;

    aclk_send_agent_connection_update(client, 1);
}

void aclk_graceful_disconnect(mqtt_wss_client client)
{
    info("Preparing to gracefully shutdown ACLK connection");
    aclk_queue_lock();
    aclk_queue_flush();

    aclk_shared_state.mqtt_shutdown_msg_id = aclk_send_agent_connection_update(client, 0);

    time_t t = now_monotonic_sec();
    while (!mqtt_wss_service(client, 100)) {
        if (now_monotonic_sec() - t >= 2) {
            error("Wasn't able to gracefully shutdown ACLK in time!");
            break;
        }
        if (aclk_shared_state.mqtt_shutdown_msg_rcvd) {
            info("MQTT App Layer `disconnect` message sent successfully");
            break;
        }
    }
    info("ACLK link is down");
    log_access("ACLK DISCONNECTED");
    aclk_stats_upd_online(0);
    last_disconnect_time = now_realtime_sec();
    aclk_connected = 0;

    info("Attempting to gracefully shutdown the MQTT/WSS connection");
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

/* Block till aclk_reconnect_delay is satisfied or netdata_exit is signalled
 * @return 0 - Go ahead and connect (delay expired)
 *         1 - netdata_exit
 */
#define NETDATA_EXIT_POLL_MS (MSEC_PER_SEC/4)
static int aclk_block_till_recon_allowed() {
    unsigned long recon_delay = aclk_reconnect_delay();

    next_connection_attempt = now_realtime_sec() + (recon_delay / MSEC_PER_SEC);
    last_backoff_value = (float)recon_delay / MSEC_PER_SEC;

    info("Wait before attempting to reconnect in %.3f seconds\n", recon_delay / (float)MSEC_PER_SEC);
    // we want to wake up from time to time to check netdata_exit
    while (recon_delay)
    {
        if (netdata_exit)
            return 1;
        if (recon_delay > NETDATA_EXIT_POLL_MS) {
            sleep_usec(NETDATA_EXIT_POLL_MS * USEC_PER_MS);
            recon_delay -= NETDATA_EXIT_POLL_MS;
            continue;
        }
        sleep_usec(recon_delay * USEC_PER_MS);
        recon_delay = 0;
    }
    return netdata_exit;
}

#ifndef ACLK_DISABLE_CHALLENGE
/* Cloud returns transport list ordered with highest
 * priority first. This function selects highest prio
 * transport that we can actually use (support)
 */
static int aclk_get_transport_idx(aclk_env_t *env) {
    for (size_t i = 0; i < env->transport_count; i++) {
        // currently we support only MQTT 3
        // therefore select first transport that matches
        if (env->transports[i]->type == ACLK_TRP_MQTT_3_1_1) {
            return i;
        }
    }
    return -1;
}
#endif

/* Attempts to make a connection to MQTT broker over WSS
 * @param client instance of mqtt_wss_client
 * @return  0 - Successful Connection,
 *          <0 - Irrecoverable Error -> Kill ACLK,
 *          >0 - netdata_exit
 */
#define CLOUD_BASE_URL_READ_RETRY 30
#ifdef ACLK_SSL_ALLOW_SELF_SIGNED
#define ACLK_SSL_FLAGS MQTT_WSS_SSL_ALLOW_SELF_SIGNED
#else
#define ACLK_SSL_FLAGS MQTT_WSS_SSL_CERT_CHECK_FULL
#endif
static int aclk_attempt_to_connect(mqtt_wss_client client)
{
    int ret;

    url_t base_url;

#ifndef ACLK_DISABLE_CHALLENGE
    url_t auth_url;
    url_t mqtt_url;
#endif

    while (!netdata_exit) {
        char *cloud_base_url = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", NULL);
        if (cloud_base_url == NULL) {
            error("Do not move the cloud base url out of post_conf_load!!");
            return -1;
        }

        if (aclk_block_till_recon_allowed())
            return 1;

        info("Attempting connection now");
        memset(&base_url, 0, sizeof(url_t));
        if (url_parse(cloud_base_url, &base_url)) {
            error("ACLK base URL configuration key could not be parsed. Will retry in %d seconds.", CLOUD_BASE_URL_READ_RETRY);
            sleep(CLOUD_BASE_URL_READ_RETRY);
            url_t_destroy(&base_url);
            continue;
        }

        struct mqtt_wss_proxy proxy_conf = { .host = NULL, .port = 0, .type = MQTT_WSS_DIRECT };
        aclk_set_proxy((char**)&proxy_conf.host, &proxy_conf.port, &proxy_conf.type);

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

        ret = aclk_get_env(aclk_env, base_url.host, base_url.port);
        url_t_destroy(&base_url);
        if (ret) {
            error("Failed to Get ACLK environment");
            // delay handled by aclk_block_till_recon_allowed
            continue;
        }

        if (netdata_exit)
            return 1;

        if (aclk_env->encoding != ACLK_ENC_PROTO) {
            error_report("This agent can only use the new cloud protocol but cloud requested old one.");
            continue;
        }

        if (!aclk_env_has_capa("proto")) {
            error ("Can't use encoding=proto without at least \"proto\" capability.");
            continue;
        }
        info("New ACLK protobuf protocol negotiated successfully (/env response).");

        memset(&auth_url, 0, sizeof(url_t));
        if (url_parse(aclk_env->auth_endpoint, &auth_url)) {
            error("Parsing URL returned by env endpoint for authentication failed. \"%s\"", aclk_env->auth_endpoint);
            url_t_destroy(&auth_url);
            continue;
        }

        ret = aclk_get_mqtt_otp(aclk_private_key, (char **)&mqtt_conn_params.clientid, (char **)&mqtt_conn_params.username, (char **)&mqtt_conn_params.password, &auth_url);
        url_t_destroy(&auth_url);
        if (ret) {
            error("Error passing Challenge/Response to get OTP");
            continue;
        }

        // aclk_get_topic moved here as during OTP we
        // generate the topic cache
        mqtt_conn_params.will_topic = aclk_get_topic(ACLK_TOPICID_AGENT_CONN);

        if (!mqtt_conn_params.will_topic) {
            error("Couldn't get LWT topic. Will not send LWT.");
            continue;
        }

        // Do the MQTT connection
        ret = aclk_get_transport_idx(aclk_env);
        if (ret < 0) {
            error("Cloud /env endpoint didn't return any transport usable by this Agent.");
            continue;
        }

        memset(&mqtt_url, 0, sizeof(url_t));
        if (url_parse(aclk_env->transports[ret]->endpoint, &mqtt_url)){
            error("Failed to parse target URL for /env trp idx %d \"%s\"", ret, aclk_env->transports[ret]->endpoint);
            url_t_destroy(&mqtt_url);
            continue;
        }
#endif

        aclk_session_newarch = now_realtime_usec();
        aclk_session_sec = aclk_session_newarch / USEC_PER_SEC;
        aclk_session_us = aclk_session_newarch % USEC_PER_SEC;

        mqtt_conn_params.will_msg = aclk_generate_lwt(&mqtt_conn_params.will_msg_len);

#ifdef ACLK_DISABLE_CHALLENGE
        ret = mqtt_wss_connect(client, base_url.host, base_url.port, &mqtt_conn_params, ACLK_SSL_FLAGS, &proxy_conf);
        url_t_destroy(&base_url);
#else
        ret = mqtt_wss_connect(client, mqtt_url.host, mqtt_url.port, &mqtt_conn_params, ACLK_SSL_FLAGS, &proxy_conf);
        url_t_destroy(&mqtt_url);

        freez((char*)mqtt_conn_params.clientid);
        freez((char*)mqtt_conn_params.password);
        freez((char*)mqtt_conn_params.username);
#endif

        freez((char *)mqtt_conn_params.will_msg);

        if (!ret) {
            last_conn_time_mqtt = now_realtime_sec();
            info("ACLK connection successfully established");
            log_access("ACLK CONNECTED");
            mqtt_connected_actions(client);
            return 0;
        }

        error_report("Connect failed");
    }

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
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    struct aclk_stats_thread *stats_thread = NULL;

    struct aclk_query_threads query_threads;
    query_threads.thread_list = NULL;

    ACLK_PROXY_TYPE proxy_type;
    aclk_get_proxy(&proxy_type);
    if (proxy_type == PROXY_TYPE_SOCKS5) {
        error("SOCKS5 proxy is not supported by ACLK-NG yet.");
        static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
        return NULL;
    }

    unsigned int proto_hdl_cnt = aclk_init_rx_msg_handlers();

    // This thread is unusual in that it cannot be cancelled by cancel_main_threads()
    // as it must notify the far end that it shutdown gracefully and avoid the LWT.
    netdata_thread_disable_cancelability();

#if defined( DISABLE_CLOUD ) || !defined( ENABLE_ACLK )
    info("Killing ACLK thread -> cloud functionality has been disabled");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
    return NULL;
#endif
    query_threads.count = read_query_thread_count();

    if (wait_till_cloud_enabled())
        goto exit;

    if (wait_till_agent_claim_ready())
        goto exit;

    use_mqtt_5 = config_get_boolean(CONFIG_SECTION_CLOUD, "mqtt5", CONFIG_BOOLEAN_YES);

    if (!(mqttwss_client = mqtt_wss_new("mqtt_wss", aclk_mqtt_wss_log_cb, msg_callback, puback_callback, use_mqtt_5))) {
        error("Couldn't initialize MQTT_WSS network library");
        goto exit;
    }

    // Enable MQTT buffer growth if necessary
    // e.g. old cloud architecture clients with huge nodes
    // that send JSON payloads of 10 MB as single messages
    mqtt_wss_set_max_buf_size(mqttwss_client, 25*1024*1024);

    aclk_stats_enabled = config_get_boolean(CONFIG_SECTION_CLOUD, "statistics", CONFIG_BOOLEAN_YES);
    if (aclk_stats_enabled) {
        stats_thread = callocz(1, sizeof(struct aclk_stats_thread));
        stats_thread->thread = mallocz(sizeof(netdata_thread_t));
        stats_thread->query_thread_count = query_threads.count;
        stats_thread->client = mqttwss_client;
        aclk_stats_thread_prepare(query_threads.count, proto_hdl_cnt);
        netdata_thread_create(
            stats_thread->thread, ACLK_STATS_THREAD_NAME, NETDATA_THREAD_OPTION_JOINABLE, aclk_stats_main_thread,
            stats_thread);
    }

    // Keep reconnecting and talking until our time has come
    // and the Grim Reaper (netdata_exit) calls
    do {
        if (aclk_attempt_to_connect(mqttwss_client))
            goto exit_full;

        if (unlikely(!query_threads.thread_list))
            aclk_query_threads_start(&query_threads, mqttwss_client);

        if (handle_connection(mqttwss_client)) {
            aclk_stats_upd_online(0);
            last_disconnect_time = now_realtime_sec();
            aclk_connected = 0;
            log_access("ACLK DISCONNECTED");
        }
    } while (!netdata_exit);

    aclk_graceful_disconnect(mqttwss_client);

exit_full:
// Tear Down
    QUERY_THREAD_WAKEUP_ALL;

    aclk_query_threads_cleanup(&query_threads);

    if (aclk_stats_enabled) {
        netdata_thread_join(*stats_thread->thread, NULL);
        aclk_stats_thread_cleanup();
        freez(stats_thread->thread);
        freez(stats_thread);
    }
    free_topic_cache();
    mqtt_wss_destroy(mqttwss_client);
exit:
    if (aclk_env) {
        aclk_env_t_destroy(aclk_env);
        freez(aclk_env);
    }
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
    return NULL;
}

void aclk_host_state_update(RRDHOST *host, int cmd)
{
    uuid_t node_id;
    int ret;

    if (!aclk_connected)
        return;

    ret = get_node_id(&host->host_uuid, &node_id);
    if (ret > 0) {
        // this means we were not able to check if node_id already present
        error("Unable to check for node_id. Ignoring the host state update.");
        return;
    }
    if (ret < 0) {
        // node_id not found
        aclk_query_t create_query;
        create_query = aclk_query_new(REGISTER_NODE);
        rrdhost_aclk_state_lock(localhost);
        node_instance_creation_t node_instance_creation = {
            .claim_id = localhost->aclk_state.claimed_id,
            .hops = host->system_info->hops,
            .hostname = rrdhost_hostname(host),
            .machine_guid = host->machine_guid
        };
        create_query->data.bin_payload.payload = generate_node_instance_creation(&create_query->data.bin_payload.size, &node_instance_creation);
        rrdhost_aclk_state_unlock(localhost);
        create_query->data.bin_payload.topic = ACLK_TOPICID_CREATE_NODE;
        create_query->data.bin_payload.msg_name = "CreateNodeInstance";
        info("Registering host=%s, hops=%u",host->machine_guid, host->system_info->hops);
        aclk_queue_query(create_query);
        return;
    }

    aclk_query_t query = aclk_query_new(NODE_STATE_UPDATE);
    node_instance_connection_t node_state_update = {
        .hops = host->system_info->hops,
        .live = cmd,
        .queryable = 1,
        .session_id = aclk_session_newarch
    };
    node_state_update.node_id = mallocz(UUID_STR_LEN);
    uuid_unparse_lower(node_id, (char*)node_state_update.node_id);

    struct capability caps[] = {
        { .name = "proto", .version = 1,                     .enabled = 1 },
        { .name = "ml",    .version = ml_capable(localhost), .enabled = ml_enabled(host) },
        { .name = "mc",    .version = enable_metric_correlations ? metric_correlations_version : 0, .enabled = enable_metric_correlations },
        { .name = "ctx",   .version = 1,                     .enabled = rrdcontext_enabled },
        { .name = NULL,    .version = 0,                     .enabled = 0 }
    };
    node_state_update.capabilities = caps;

    rrdhost_aclk_state_lock(localhost);
    node_state_update.claim_id = localhost->aclk_state.claimed_id;
    query->data.bin_payload.payload = generate_node_instance_connection(&query->data.bin_payload.size, &node_state_update);
    rrdhost_aclk_state_unlock(localhost);

    info("Queuing status update for node=%s, live=%d, hops=%u",(char*)node_state_update.node_id, cmd,
         host->system_info->hops);
    freez((void*)node_state_update.node_id);
    query->data.bin_payload.msg_name = "UpdateNodeInstanceConnection";
    query->data.bin_payload.topic = ACLK_TOPICID_NODE_CONN;
    aclk_queue_query(query);
}

void aclk_send_node_instances()
{
    struct node_instance_list *list_head = get_node_list();
    struct node_instance_list *list = list_head;
    if (unlikely(!list)) {
        error_report("Failure to get_node_list from DB!");
        return;
    }
    while (!uuid_is_null(list->host_id)) {
        if (!uuid_is_null(list->node_id)) {
            aclk_query_t query = aclk_query_new(NODE_STATE_UPDATE);
            node_instance_connection_t node_state_update = {
                .live = list->live,
                .hops = list->hops,
                .queryable = 1,
                .session_id = aclk_session_newarch
            };
            node_state_update.node_id = mallocz(UUID_STR_LEN);
            uuid_unparse_lower(list->node_id, (char*)node_state_update.node_id);

            char host_id[UUID_STR_LEN];
            uuid_unparse_lower(list->host_id, host_id);

            RRDHOST *host = rrdhost_find_by_guid(host_id);
            struct capability caps[] = {
                { .name = "proto", .version = 1,                     .enabled = 1 },
                { .name = "ml",    .version = ml_capable(localhost), .enabled = host ? ml_enabled(host) : 0 },
                { .name = "mc",    .version = enable_metric_correlations ? metric_correlations_version : 0, .enabled = enable_metric_correlations },
                { .name = "ctx",   .version = 1,                     .enabled = rrdcontext_enabled },
                { .name = NULL,    .version = 0,                     .enabled = 0 }
            };
            node_state_update.capabilities = caps;

            rrdhost_aclk_state_lock(localhost);
            node_state_update.claim_id = localhost->aclk_state.claimed_id;
            query->data.bin_payload.payload = generate_node_instance_connection(&query->data.bin_payload.size, &node_state_update);
            rrdhost_aclk_state_unlock(localhost);
            info("Queuing status update for node=%s, live=%d, hops=%d",(char*)node_state_update.node_id,
                 list->live,
                 list->hops);
            freez((void*)node_state_update.node_id);
            query->data.bin_payload.msg_name = "UpdateNodeInstanceConnection";
            query->data.bin_payload.topic = ACLK_TOPICID_NODE_CONN;
            aclk_queue_query(query);
        } else {
            aclk_query_t create_query;
            create_query = aclk_query_new(REGISTER_NODE);
            node_instance_creation_t node_instance_creation = {
                .hops = list->hops,
                .hostname = list->hostname,
            };
            node_instance_creation.machine_guid = mallocz(UUID_STR_LEN);
            uuid_unparse_lower(list->host_id, (char*)node_instance_creation.machine_guid);
            create_query->data.bin_payload.topic = ACLK_TOPICID_CREATE_NODE;
            create_query->data.bin_payload.msg_name = "CreateNodeInstance";
            rrdhost_aclk_state_lock(localhost);
            node_instance_creation.claim_id = localhost->aclk_state.claimed_id,
            create_query->data.bin_payload.payload = generate_node_instance_creation(&create_query->data.bin_payload.size, &node_instance_creation);
            rrdhost_aclk_state_unlock(localhost);
            info("Queuing registration for host=%s, hops=%d",(char*)node_instance_creation.machine_guid,
                 list->hops);
            freez((void *)node_instance_creation.machine_guid);
            aclk_queue_query(create_query);
        }
        freez(list->hostname);

        list++;
    }
    freez(list_head);
}

void aclk_send_bin_msg(char *msg, size_t msg_len, enum aclk_topics subtopic, const char *msgname)
{
    aclk_send_bin_message_subtopic_pid(mqttwss_client, msg, msg_len, subtopic, msgname);
}

static void fill_alert_status_for_host(BUFFER *wb, RRDHOST *host)
{
    struct proto_alert_status status;
    memset(&status, 0, sizeof(status));
    if (get_proto_alert_status(host, &status)) {
        buffer_strcat(wb, "\nFailed to get alert streaming status for this host");
        return;
    }
    buffer_sprintf(wb,
        "\n\t\tUpdates: %d"
        "\n\t\tBatch ID: %"PRIu64
        "\n\t\tLast Acked Seq ID: %"PRIu64
        "\n\t\tPending Min Seq ID: %"PRIu64
        "\n\t\tPending Max Seq ID: %"PRIu64
        "\n\t\tLast Submitted Seq ID: %"PRIu64,
        status.alert_updates,
        status.alerts_batch_id,
        status.last_acked_sequence_id,
        status.pending_min_sequence_id,
        status.pending_max_sequence_id,
        status.last_submitted_sequence_id
    );
}

static void fill_chart_status_for_host(BUFFER *wb, RRDHOST *host)
{
    struct aclk_chart_sync_stats *stats = aclk_get_chart_sync_stats(host);
    if (!stats) {
        buffer_strcat(wb, "\n\t\tFailed to get alert streaming status for this host");
        return;
    }
    buffer_sprintf(wb,
        "\n\t\tUpdates: %d"
        "\n\t\tBatch ID: %"PRIu64
        "\n\t\tMin Seq ID: %"PRIu64
        "\n\t\tMax Seq ID: %"PRIu64
        "\n\t\tPending Min Seq ID: %"PRIu64
        "\n\t\tPending Max Seq ID: %"PRIu64
        "\n\t\tSent Min Seq ID: %"PRIu64
        "\n\t\tSent Max Seq ID: %"PRIu64
        "\n\t\tAcked Min Seq ID: %"PRIu64
        "\n\t\tAcked Max Seq ID: %"PRIu64,
        stats->updates,
        stats->batch_id,
        stats->min_seqid,
        stats->max_seqid,
        stats->min_seqid_pend,
        stats->max_seqid_pend,
        stats->min_seqid_sent,
        stats->max_seqid_sent,
        stats->min_seqid_ack,
        stats->max_seqid_ack
    );
    freez(stats);
}
#endif /* ENABLE_ACLK */

char *aclk_state(void)
{
#ifndef ENABLE_ACLK
    return strdupz("ACLK Available: No");
#else
    BUFFER *wb = buffer_create(1024);
    struct tm *tmptr, tmbuf;
    char *ret;

    buffer_strcat(wb,
        "ACLK Available: Yes\n"
        "ACLK Version: 2\n"
        "Protocols Supported: Protobuf\n"
    );
    buffer_sprintf(wb, "Protocol Used: Protobuf\nMQTT Version: %d\nClaimed: ", use_mqtt_5 ? 5 : 3);

    char *agent_id = get_agent_claimid();
    if (agent_id == NULL)
        buffer_strcat(wb, "No\n");
    else {
        char *cloud_base_url = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", NULL);
        buffer_sprintf(wb, "Yes\nClaimed Id: %s\nCloud URL: %s\n", agent_id, cloud_base_url ? cloud_base_url : "null");
        freez(agent_id);
    }

    buffer_sprintf(wb, "Online: %s\nReconnect count: %d\nBanned By Cloud: %s\n", aclk_connected ? "Yes" : "No", aclk_connection_counter > 0 ? (aclk_connection_counter - 1) : 0, aclk_disable_runtime ? "Yes" : "No");
    if (last_conn_time_mqtt && (tmptr = localtime_r(&last_conn_time_mqtt, &tmbuf)) ) {
        char timebuf[26];
        strftime(timebuf, 26, "%Y-%m-%d %H:%M:%S", tmptr);
        buffer_sprintf(wb, "Last Connection Time: %s\n", timebuf);
    }
    if (last_conn_time_appl && (tmptr = localtime_r(&last_conn_time_appl, &tmbuf)) ) {
        char timebuf[26];
        strftime(timebuf, 26, "%Y-%m-%d %H:%M:%S", tmptr);
        buffer_sprintf(wb, "Last Connection Time + %d PUBACKs received: %s\n", ACLK_PUBACKS_CONN_STABLE, timebuf);
    }
    if (last_disconnect_time && (tmptr = localtime_r(&last_disconnect_time, &tmbuf)) ) {
        char timebuf[26];
        strftime(timebuf, 26, "%Y-%m-%d %H:%M:%S", tmptr);
        buffer_sprintf(wb, "Last Disconnect Time: %s\n", timebuf);
    }
    if (!aclk_connected && next_connection_attempt && (tmptr = localtime_r(&next_connection_attempt, &tmbuf)) ) {
        char timebuf[26];
        strftime(timebuf, 26, "%Y-%m-%d %H:%M:%S", tmptr);
        buffer_sprintf(wb, "Next Connection Attempt At: %s\nLast Backoff: %.3f", timebuf, last_backoff_value);
    }

    if (aclk_connected) {
        buffer_sprintf(wb, "Received Cloud MQTT Messages: %d\nMQTT Messages Confirmed by Remote Broker (PUBACKs): %d", aclk_rcvd_cloud_msgs, aclk_pubacks_per_conn);

        RRDHOST *host;
        rrd_rdlock();
        rrdhost_foreach_read(host) {
            buffer_sprintf(wb, "\n\n> Node Instance for mGUID: \"%s\" hostname \"%s\"\n", host->machine_guid, rrdhost_hostname(host));

            buffer_strcat(wb, "\tClaimed ID: ");
            rrdhost_aclk_state_lock(host);
            if (host->aclk_state.claimed_id)
                buffer_strcat(wb, host->aclk_state.claimed_id);
            else
                buffer_strcat(wb, "null");
            rrdhost_aclk_state_unlock(host);


            if (host->node_id == NULL || uuid_is_null(*host->node_id)) {
                buffer_strcat(wb, "\n\tNode ID: null\n");
            } else {
                char node_id[GUID_LEN + 1];
                uuid_unparse_lower(*host->node_id, node_id);
                buffer_sprintf(wb, "\n\tNode ID: %s\n", node_id);
            }

            buffer_sprintf(wb, "\tStreaming Hops: %d\n\tRelationship: %s", host->system_info->hops, host == localhost ? "self" : "child");

            if (host != localhost)
                buffer_sprintf(wb, "\n\tStreaming Connection Live: %s", host->receiver ? "true" : "false");

            buffer_strcat(wb, "\n\tAlert Streaming Status:");
            fill_alert_status_for_host(wb, host);

            buffer_strcat(wb, "\n\tChart Streaming Status:");
            fill_chart_status_for_host(wb, host);
        }
        rrd_unlock();
    }

    ret = strdupz(buffer_tostring(wb));
    buffer_free(wb);
    return ret;
#endif /* ENABLE_ACLK */
}

#ifdef ENABLE_ACLK
static void fill_alert_status_for_host_json(json_object *obj, RRDHOST *host)
{
    struct proto_alert_status status;
    memset(&status, 0, sizeof(status));
    if (get_proto_alert_status(host, &status))
        return;

    json_object *tmp = json_object_new_int(status.alert_updates);
    json_object_object_add(obj, "updates", tmp);

    tmp = json_object_new_int(status.alerts_batch_id);
    json_object_object_add(obj, "batch-id", tmp);

    tmp = json_object_new_int(status.last_acked_sequence_id);
    json_object_object_add(obj, "last-acked-seq-id", tmp);

    tmp = json_object_new_int(status.pending_min_sequence_id);
    json_object_object_add(obj, "pending-min-seq-id", tmp);

    tmp = json_object_new_int(status.pending_max_sequence_id);
    json_object_object_add(obj, "pending-max-seq-id", tmp);

    tmp = json_object_new_int(status.last_submitted_sequence_id);
    json_object_object_add(obj, "last-submitted-seq-id", tmp);
}

static void fill_chart_status_for_host_json(json_object *obj, RRDHOST *host)
{
    struct aclk_chart_sync_stats *stats = aclk_get_chart_sync_stats(host);
    if (!stats)
        return;

    json_object *tmp = json_object_new_int(stats->updates);
    json_object_object_add(obj, "updates", tmp);

    tmp = json_object_new_int(stats->batch_id);
    json_object_object_add(obj, "batch-id", tmp);

    tmp = json_object_new_int(stats->min_seqid);
    json_object_object_add(obj, "min-seq-id", tmp);

    tmp = json_object_new_int(stats->max_seqid);
    json_object_object_add(obj, "max-seq-id", tmp);

    tmp = json_object_new_int(stats->min_seqid_pend);
    json_object_object_add(obj, "pending-min-seq-id", tmp);

    tmp = json_object_new_int(stats->max_seqid_pend);
    json_object_object_add(obj, "pending-max-seq-id", tmp);

    tmp = json_object_new_int(stats->min_seqid_sent);
    json_object_object_add(obj, "sent-min-seq-id", tmp);

    tmp = json_object_new_int(stats->max_seqid_sent);
    json_object_object_add(obj, "sent-max-seq-id", tmp);

    tmp = json_object_new_int(stats->min_seqid_ack);
    json_object_object_add(obj, "acked-min-seq-id", tmp);

    tmp = json_object_new_int(stats->max_seqid_ack);
    json_object_object_add(obj, "acked-max-seq-id", tmp);

    freez(stats);
}

static json_object *timestamp_to_json(const time_t *t)
{
    struct tm *tmptr, tmbuf;
    if (*t && (tmptr = gmtime_r(t, &tmbuf)) ) {
        char timebuf[26];
        strftime(timebuf, 26, "%Y-%m-%d %H:%M:%S", tmptr);
        return json_object_new_string(timebuf);
    }
    return NULL;
}
#endif /* ENABLE_ACLK */

char *aclk_state_json(void)
{
#ifndef ENABLE_ACLK
    return strdupz("{\"aclk-available\":false}");
#else
    json_object *tmp, *grp, *msg = json_object_new_object();

    tmp = json_object_new_boolean(1);
    json_object_object_add(msg, "aclk-available", tmp);

    tmp = json_object_new_int(2);
    json_object_object_add(msg, "aclk-version", tmp);

    grp = json_object_new_array();
    tmp = json_object_new_string("Protobuf");
    json_object_array_add(grp, tmp);
    json_object_object_add(msg, "protocols-supported", grp);

    char *agent_id = get_agent_claimid();
    tmp = json_object_new_boolean(agent_id != NULL);
    json_object_object_add(msg, "agent-claimed", tmp);

    if (agent_id) {
        tmp = json_object_new_string(agent_id);
        freez(agent_id);
    } else
        tmp = NULL;
    json_object_object_add(msg, "claimed-id", tmp);

    char *cloud_base_url = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", NULL);
    tmp = cloud_base_url ? json_object_new_string(cloud_base_url) : NULL;
    json_object_object_add(msg, "cloud-url", tmp);

    tmp = json_object_new_boolean(aclk_connected);
    json_object_object_add(msg, "online", tmp);

    tmp = json_object_new_string("Protobuf");
    json_object_object_add(msg, "used-cloud-protocol", tmp);

    tmp = json_object_new_int(use_mqtt_5 ? 5 : 3);
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
    json_object_object_add(msg, "next-connection-attempt-utc", !aclk_connected ? timestamp_to_json(&next_connection_attempt) : NULL);
    tmp = NULL;
    if (!aclk_connected && last_backoff_value)
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

        rrdhost_aclk_state_lock(host);
        if (host->aclk_state.claimed_id) {
            tmp = json_object_new_string(host->aclk_state.claimed_id);
            json_object_object_add(nodeinstance, "claimed_id", tmp);
        } else
            json_object_object_add(nodeinstance, "claimed_id", NULL);
        rrdhost_aclk_state_unlock(host);

        if (host->node_id == NULL || uuid_is_null(*host->node_id)) {
            json_object_object_add(nodeinstance, "node-id", NULL);
        } else {
            char node_id[GUID_LEN + 1];
            uuid_unparse_lower(*host->node_id, node_id);
            tmp = json_object_new_string(node_id);
            json_object_object_add(nodeinstance, "node-id", tmp);
        }

        tmp = json_object_new_int(host->system_info->hops);
        json_object_object_add(nodeinstance, "streaming-hops", tmp);

        tmp = json_object_new_string(host == localhost ? "self" : "child");
        json_object_object_add(nodeinstance, "relationship", tmp);

        tmp = json_object_new_boolean((host->receiver || host == localhost));
        json_object_object_add(nodeinstance, "streaming-online", tmp);

        tmp = json_object_new_object();
        fill_alert_status_for_host_json(tmp, host);
        json_object_object_add(nodeinstance, "alert-sync-status", tmp);

        tmp = json_object_new_object();
        fill_chart_status_for_host_json(tmp, host);
        json_object_object_add(nodeinstance, "chart-sync-status", tmp);

        json_object_array_add(grp, nodeinstance);
    }
    rrd_unlock();
    json_object_object_add(msg, "node-instances", grp);

    char *str = strdupz(json_object_to_json_string_ext(msg, JSON_C_TO_STRING_PLAIN));
    json_object_put(msg);
    return str;
#endif /* ENABLE_ACLK */
}

void add_aclk_host_labels(void) {
    DICTIONARY *labels = localhost->host_labels;

#ifdef ENABLE_ACLK
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

    int mqtt5 = config_get_boolean(CONFIG_SECTION_CLOUD, "mqtt5", CONFIG_BOOLEAN_YES);

    rrdlabels_add(labels, "_mqtt_version", mqtt5 ? "5" : "3", RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_aclk_proxy", proxy_str, RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_aclk_ng_new_cloud_protocol", "true", RRDLABEL_SRC_AUTO|RRDLABEL_SRC_ACLK);
#else
    rrdlabels_add(labels, "_aclk_available", "false", RRDLABEL_SRC_AUTO|RRDLABEL_SRC_ACLK);
#endif
}
