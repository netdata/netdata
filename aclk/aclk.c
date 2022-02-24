// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk.h"

#include "aclk_stats.h"
#include "mqtt_wss_client.h"
#include "aclk_otp.h"
#include "aclk_tx_msgs.h"
#include "aclk_query.h"
#include "aclk_query_queue.h"
#include "aclk_util.h"
#include "aclk_rx_msgs.h"
#include "aclk_collector_list.h"
#include "https_client.h"

#include "aclk_proxy.h"

#ifdef ACLK_LOG_CONVERSATION_DIR
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#endif

#define ACLK_STABLE_TIMEOUT 3 // Minimum delay to mark AGENT as stable

int aclk_pubacks_per_conn = 0; // How many PubAcks we got since MQTT conn est.
int aclk_rcvd_cloud_msgs = 0;
int aclk_connection_counter = 0;
int disconnect_req = 0;

int aclk_alert_reloaded = 1; //1 on startup, and again on health_reload

time_t aclk_block_until = 0;

mqtt_wss_client mqttwss_client;

netdata_mutex_t aclk_shared_state_mutex = NETDATA_MUTEX_INITIALIZER;
#define ACLK_SHARED_STATE_LOCK netdata_mutex_lock(&aclk_shared_state_mutex)
#define ACLK_SHARED_STATE_UNLOCK netdata_mutex_unlock(&aclk_shared_state_mutex)

struct aclk_shared_state aclk_shared_state = {
    .agent_state = ACLK_HOST_INITIALIZING,
    .last_popcorn_interrupt = 0,
    .mqtt_shutdown_msg_id = -1,
    .mqtt_shutdown_msg_rcvd = 0
};

static RSA *aclk_private_key = NULL;
static int load_private_key()
{
    if (aclk_private_key != NULL)
        RSA_free(aclk_private_key);
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

    aclk_private_key = PEM_read_bio_RSAPrivateKey(key_bio, NULL, NULL, NULL);
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
    char *agent_id = is_agent_claimed();
    while (likely(!agent_id)) {
        sleep_usec(USEC_PER_SEC * 1);
        if (netdata_exit)
            return 1;
        agent_id = is_agent_claimed();
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
 * @return If non 0 returned irrecoverable error happened and ACLK should be terminated
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
            error("Agent is claimed but the configuration is invalid, please fix");
            url_t_destroy(&url);
            sleep(5);
            continue;
        }
        url_t_destroy(&url);

        if (!load_private_key()) {
            sleep(5);
            break;
        }
    }

    return 0;
}

void aclk_mqtt_wss_log_cb(mqtt_wss_log_type_t log_type, const char* str)
{
    switch(log_type) {
        case MQTT_WSS_LOG_ERROR:
        case MQTT_WSS_LOG_FATAL:
        case MQTT_WSS_LOG_WARN:
            error("%s", str);
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
static void msg_callback_old_protocol(const char *topic, const void *msg, size_t msglen, int qos)
{
    UNUSED(qos);
    char cmsg[RX_MSGLEN_MAX];
    size_t len = (msglen < RX_MSGLEN_MAX - 1) ? msglen : (RX_MSGLEN_MAX - 1);
    const char *cmd_topic = aclk_get_topic(ACLK_TOPICID_COMMAND);
    if (!cmd_topic) {
        error("Error retrieving command topic");
        return;
    }

    if (msglen > RX_MSGLEN_MAX - 1)
        error("Incoming ACLK message was bigger than MAX of %d and got truncated.", RX_MSGLEN_MAX);

    memcpy(cmsg,
           msg,
           len);
    cmsg[len] = 0;

#ifdef ACLK_LOG_CONVERSATION_DIR
#define FN_MAX_LEN 512
    char filename[FN_MAX_LEN];
    int logfd;
    snprintf(filename, FN_MAX_LEN, ACLK_LOG_CONVERSATION_DIR "/%010d-rx.json", ACLK_GET_CONV_LOG_NEXT());
    logfd = open(filename, O_CREAT | O_TRUNC | O_WRONLY, S_IRUSR | S_IWUSR );
    if(logfd < 0)
        error("Error opening ACLK Conversation logfile \"%s\" for RX message.", filename);
    write(logfd, msg, msglen);
    close(logfd);
#endif

    debug(D_ACLK, "Got Message From Broker Topic \"%s\" QoS %d MSG: \"%s\"", topic, qos, cmsg);

    if (strcmp(cmd_topic, topic))
        error("Received message on unexpected topic %s", topic);

    if (aclk_shared_state.mqtt_shutdown_msg_id > 0) {
        error("Link is shutting down. Ignoring incoming message.");
        return;
    }

    aclk_handle_cloud_cmd_message(cmsg);
}

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
static void msg_callback_new_protocol(const char *topic, const void *msg, size_t msglen, int qos)
{
    UNUSED(qos);
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

    aclk_handle_new_cloud_msg(msgtype, msg, msglen);
}

static inline void msg_callback(const char *topic, const void *msg, size_t msglen, int qos) {
    aclk_rcvd_cloud_msgs++;
    if (aclk_use_new_cloud_arch)
        msg_callback_new_protocol(topic, msg, msglen, qos);
    else
        msg_callback_old_protocol(topic, msg, msglen, qos);
}
#endif /* ENABLE_NEW_CLOUD_PROTOCOL */

static void puback_callback(uint16_t packet_id)
{
    if (++aclk_pubacks_per_conn == ACLK_PUBACKS_CONN_STABLE)
        aclk_tbeb_reset();

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

inline static int aclk_popcorn_check()
{
    ACLK_SHARED_STATE_LOCK;
    if (unlikely(aclk_shared_state.agent_state == ACLK_HOST_INITIALIZING)) {
        ACLK_SHARED_STATE_UNLOCK;
        return 1;
    }
    ACLK_SHARED_STATE_UNLOCK;
    return 0;
}

inline static int aclk_popcorn_check_bump()
{
    ACLK_SHARED_STATE_LOCK;
    if (unlikely(aclk_shared_state.agent_state == ACLK_HOST_INITIALIZING)) {
        aclk_shared_state.last_popcorn_interrupt = now_realtime_sec();
        ACLK_SHARED_STATE_UNLOCK;
        return 1;
    }
    ACLK_SHARED_STATE_UNLOCK;
    return 0;
}

static inline void queue_connect_payloads(void)
{
    aclk_query_t query = aclk_query_new(METADATA_INFO);
    query->data.metadata_info.host = localhost;
    query->data.metadata_info.initial_on_connect = 1;
    aclk_queue_query(query);
    query = aclk_query_new(METADATA_ALARMS);
    query->data.metadata_alarms.initial_on_connect = 1;
    aclk_queue_query(query);
}

static inline void mqtt_connected_actions(mqtt_wss_client client)
{
    const char *topic = aclk_get_topic(ACLK_TOPICID_COMMAND);

    if (!topic)
        error("Unable to fetch topic for COMMAND (to subscribe)");
    else
        mqtt_wss_subscribe(client, topic, 1);

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    if (aclk_use_new_cloud_arch) {
        topic = aclk_get_topic(ACLK_TOPICID_CMD_NG_V1);
        if (!topic)
            error("Unable to fetch topic for protobuf COMMAND (to subscribe)");
        else
            mqtt_wss_subscribe(client, topic, 1);
    }
#endif

    aclk_stats_upd_online(1);
    aclk_connected = 1;
    aclk_pubacks_per_conn = 0;
    aclk_rcvd_cloud_msgs = 0;
    aclk_connection_counter++;

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    if (!aclk_use_new_cloud_arch) {
#endif
        ACLK_SHARED_STATE_LOCK;
        if (aclk_shared_state.agent_state != ACLK_HOST_INITIALIZING) {
            error("Sending `connect` payload immediately as popcorning was finished already.");
            queue_connect_payloads();
        }
        ACLK_SHARED_STATE_UNLOCK;
#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    } else {
        aclk_send_agent_connection_update(client, 1);
    }
#endif
}

/* Waits until agent is ready or needs to exit
 * @param client instance of mqtt_wss_client
 * @param query_threads pointer to aclk_query_threads
 *        structure where to store data about started query threads
 * @return  0 - Popcorning Finished - Agent STABLE,
 *         !0 - netdata_exit
 */
static int wait_popcorning_finishes()
{
    time_t elapsed;
    int need_wait;
    if (aclk_use_new_cloud_arch)
        return 0;

    while (!netdata_exit) {
        ACLK_SHARED_STATE_LOCK;
        if (likely(aclk_shared_state.agent_state != ACLK_HOST_INITIALIZING)) {
            ACLK_SHARED_STATE_UNLOCK;
            return 0;
        }
        elapsed = now_realtime_sec() - aclk_shared_state.last_popcorn_interrupt;
        if (elapsed >= ACLK_STABLE_TIMEOUT) {
            aclk_shared_state.agent_state = ACLK_HOST_STABLE;
            ACLK_SHARED_STATE_UNLOCK;
            error("ACLK localhost popcorn timer finished");
            return 0;
        }
        ACLK_SHARED_STATE_UNLOCK;
        need_wait = ACLK_STABLE_TIMEOUT - elapsed;
        error("ACLK localhost popcorn timer - wait %d seconds longer", need_wait);
        sleep(need_wait);
    }
    return 1;
}

void aclk_graceful_disconnect(mqtt_wss_client client)
{
    info("Preparing to gracefully shutdown ACLK connection");
    aclk_queue_lock();
    aclk_queue_flush();
#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    if (aclk_use_new_cloud_arch)
        aclk_shared_state.mqtt_shutdown_msg_id = aclk_send_agent_connection_update(client, 0);
    else
#endif
        aclk_shared_state.mqtt_shutdown_msg_id = aclk_send_app_layer_disconnect(client, "graceful");

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

    json_object *lwt = NULL;

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

        aclk_use_new_cloud_arch = 0;

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

        if (aclk_env->encoding == ACLK_ENC_PROTO) {
#ifndef ENABLE_NEW_CLOUD_PROTOCOL
            error("Cloud requested New Cloud Protocol to be used but this agent cannot support it!");
            continue;
#else
            if (!aclk_env_has_capa("proto")) {
                error ("Can't encoding=proto without at least \"proto\" capability.");
                continue;
            }
            info("Switching ACLK to new protobuf protocol. Due to /env response.");
            aclk_use_new_cloud_arch = 1;
#endif
        }

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
        if (aclk_use_new_cloud_arch)
            mqtt_conn_params.will_topic = aclk_get_topic(ACLK_TOPICID_AGENT_CONN);
        else
            mqtt_conn_params.will_topic = aclk_get_topic(ACLK_TOPICID_METADATA);

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

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
        if (aclk_use_new_cloud_arch) {
            mqtt_conn_params.will_msg = aclk_generate_lwt(&mqtt_conn_params.will_msg_len);
        } else {
#endif
            lwt = aclk_generate_disconnect(NULL);
            mqtt_conn_params.will_msg = json_object_to_json_string_ext(lwt, JSON_C_TO_STRING_PLAIN);
            mqtt_conn_params.will_msg_len = strlen(mqtt_conn_params.will_msg);
#ifdef ENABLE_NEW_CLOUD_PROTOCOL
        }
#endif

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

        if (aclk_use_new_cloud_arch)
            freez((char *)mqtt_conn_params.will_msg);
        else
            json_object_put(lwt);

        if (!ret) {
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

    unsigned int proto_hdl_cnt;
#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    proto_hdl_cnt = aclk_init_rx_msg_handlers();
#endif

    // This thread is unusual in that it cannot be cancelled by cancel_main_threads()
    // as it must notify the far end that it shutdown gracefully and avoid the LWT.
    netdata_thread_disable_cancelability();

#if defined( DISABLE_CLOUD ) || !defined( ENABLE_ACLK )
    info("Killing ACLK thread -> cloud functionality has been disabled");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
    return NULL;
#endif
    aclk_popcorn_check_bump(); // start localhost popcorn timer
    query_threads.count = read_query_thread_count();

    if (wait_till_cloud_enabled())
        goto exit;

    if (wait_till_agent_claim_ready())
        goto exit;

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    if (!(mqttwss_client = mqtt_wss_new("mqtt_wss", aclk_mqtt_wss_log_cb, msg_callback, puback_callback))) {
#else
    if (!(mqttwss_client = mqtt_wss_new("mqtt_wss", aclk_mqtt_wss_log_cb, msg_callback_old_protocol, puback_callback))) {
#endif
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

#if defined(ENABLE_ACLK) && !defined(ENABLE_NEW_CLOUD_PROTOCOL)
        error_report("############################  WARNING  ###############################");
        error_report("#       Your agent is configured to connect to cloud but has         #");
        error_report("#      no protobuf protocol support (uses legacy JSON protocol)      #");
        error_report("#  Legacy protocol will be deprecated soon (planned 1st March 2022)  #");
        error_report("#  Visit following link for more info and instructions how to solve  #");
        error_report("#   https://www.netdata.cloud/blog/netdata-clouds-new-architecture   #");
        error_report("######################################################################");
#endif

        // warning this assumes the popcorning is relative short (3s)
        // if that changes call mqtt_wss_service from within
        // to keep OpenSSL, WSS and MQTT connection alive
        if (wait_popcorning_finishes())
            goto exit_full;
        
        if (unlikely(!query_threads.thread_list))
            aclk_query_threads_start(&query_threads, mqttwss_client);

        if (!aclk_use_new_cloud_arch)
            queue_connect_payloads();

        if (handle_connection(mqttwss_client)) {
            aclk_stats_upd_online(0);
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

// TODO this is taken over as workaround from old ACLK
// fix this in both old and new ACLK
extern void health_alarm_entry2json_nolock(BUFFER *wb, ALARM_ENTRY *ae, RRDHOST *host);

void aclk_alarm_reload(void)
{
    ACLK_SHARED_STATE_LOCK;
    if (unlikely(aclk_shared_state.agent_state == ACLK_HOST_INITIALIZING)) {
        ACLK_SHARED_STATE_UNLOCK;
        return;
    }
    ACLK_SHARED_STATE_UNLOCK;

    aclk_queue_query(aclk_query_new(METADATA_ALARMS));
}

int aclk_update_alarm(RRDHOST *host, ALARM_ENTRY *ae)
{
    BUFFER *local_buffer;
    json_object *msg;

    if (host != localhost)
        return 0;

    ACLK_SHARED_STATE_LOCK;
    if (unlikely(aclk_shared_state.agent_state == ACLK_HOST_INITIALIZING)) {
        ACLK_SHARED_STATE_UNLOCK;
        return 0;
    }
    ACLK_SHARED_STATE_UNLOCK;

    local_buffer = buffer_create(NETDATA_WEB_RESPONSE_HEADER_SIZE);

    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);
    health_alarm_entry2json_nolock(local_buffer, ae, host);
    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);

    msg = json_tokener_parse(local_buffer->buffer);

    struct aclk_query *query = aclk_query_new(ALARM_STATE_UPDATE);
    query->data.alarm_update = msg;
    aclk_queue_query(query);

    buffer_free(local_buffer);
    return 0;
}

int aclk_update_chart(RRDHOST *host, char *chart_name, int create)
{
    struct aclk_query *query;

    if (host == localhost ? aclk_popcorn_check_bump() : aclk_popcorn_check())
        return 0;

    query = aclk_query_new(create ? CHART_NEW : CHART_DEL);
    if(create) {
        query->data.chart_add_del.host = host;
        query->data.chart_add_del.chart_name = strdupz(chart_name);
    } else {
        query->data.metadata_info.host = host;
        query->data.metadata_info.initial_on_connect = 0;
    }

    aclk_queue_query(query);
    return 0;
}

/*
 * Add a new collector to the list
 * If it exists, update the chart count
 */
void aclk_add_collector(RRDHOST *host, const char *plugin_name, const char *module_name)
{
    struct aclk_query *query;
    struct _collector *tmp_collector;
    if (unlikely(!netdata_ready || aclk_use_new_cloud_arch)) {
        return;
    }

    COLLECTOR_LOCK;

    tmp_collector = _add_collector(host->machine_guid, plugin_name, module_name);

    if (unlikely(tmp_collector->count != 1)) {
        COLLECTOR_UNLOCK;
        return;
    }

    COLLECTOR_UNLOCK;

    if (aclk_popcorn_check_bump())
        return;

    if (host != localhost)
        return;

    query = aclk_query_new(METADATA_INFO);
    query->data.metadata_info.host = localhost; //TODO
    query->data.metadata_info.initial_on_connect = 0;
    aclk_queue_query(query);

    query = aclk_query_new(METADATA_ALARMS);
    query->data.metadata_alarms.initial_on_connect = 0;
    aclk_queue_query(query);
}

/*
 * Delete a collector from the list
 * If the chart count reaches zero the collector will be removed
 * from the list by calling del_collector.
 *
 * This function will release the memory used and schedule
 * a cloud update
 */
void aclk_del_collector(RRDHOST *host, const char *plugin_name, const char *module_name)
{
    struct aclk_query *query;
    struct _collector *tmp_collector;
    if (unlikely(!netdata_ready || aclk_use_new_cloud_arch)) {
        return;
    }

    COLLECTOR_LOCK;

    tmp_collector = _del_collector(host->machine_guid, plugin_name, module_name);

    if (unlikely(!tmp_collector || tmp_collector->count)) {
        COLLECTOR_UNLOCK;
        return;
    }

    debug(
        D_ACLK, "DEL COLLECTOR [%s:%s] -- charts %u", plugin_name ? plugin_name : "*", module_name ? module_name : "*",
        tmp_collector->count);

    COLLECTOR_UNLOCK;

    _free_collector(tmp_collector);

    if (aclk_popcorn_check_bump())
        return;
    
    if (host != localhost)
        return;

    query = aclk_query_new(METADATA_INFO);
    query->data.metadata_info.host = localhost; //TODO
    query->data.metadata_info.initial_on_connect = 0;
    aclk_queue_query(query);

    query = aclk_query_new(METADATA_ALARMS);
    query->data.metadata_alarms.initial_on_connect = 0;
    aclk_queue_query(query);
}

void aclk_host_state_update(RRDHOST *host, int cmd)
{
    uuid_t node_id;
    int ret;

    if (!aclk_connected || !aclk_use_new_cloud_arch)
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
        create_query->data.node_creation.claim_id = strdupz(localhost->aclk_state.claimed_id);
        rrdhost_aclk_state_unlock(localhost);
        create_query->data.node_creation.hops = (uint32_t) host->system_info->hops;
        create_query->data.node_creation.hostname = strdupz(host->hostname);
        create_query->data.node_creation.machine_guid = strdupz(host->machine_guid);
        info("Registering host=%s, hops=%u",host->machine_guid, host->system_info->hops);
        aclk_queue_query(create_query);
        return;
    }

    aclk_query_t query = aclk_query_new(NODE_STATE_UPDATE);
    query->data.node_update.hops = (uint32_t) host->system_info->hops;
    rrdhost_aclk_state_lock(localhost);
    query->data.node_update.claim_id = strdupz(localhost->aclk_state.claimed_id);
    rrdhost_aclk_state_unlock(localhost);
    query->data.node_update.live = cmd;
    query->data.node_update.node_id = mallocz(UUID_STR_LEN);
    uuid_unparse_lower(node_id, (char*)query->data.node_update.node_id);
    query->data.node_update.queryable = 1;
    query->data.node_update.session_id = aclk_session_newarch;
    info("Queuing status update for node=%s, live=%d, hops=%u",(char*)query->data.node_update.node_id, cmd,
         host->system_info->hops);
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
            rrdhost_aclk_state_lock(localhost);
            query->data.node_update.claim_id = strdupz(localhost->aclk_state.claimed_id);
            rrdhost_aclk_state_unlock(localhost);
            query->data.node_update.live = list->live;
            query->data.node_update.hops = list->hops;
            query->data.node_update.node_id = mallocz(UUID_STR_LEN);
            uuid_unparse_lower(list->node_id, (char*)query->data.node_update.node_id);
            query->data.node_update.queryable = 1;
            query->data.node_update.session_id = aclk_session_newarch;
            info("Queuing status update for node=%s, live=%d, hops=%d",(char*)query->data.node_update.node_id,
                 list->live,
                 list->hops);
            aclk_queue_query(query);
        } else {
            aclk_query_t create_query;
            create_query = aclk_query_new(REGISTER_NODE);
            rrdhost_aclk_state_lock(localhost);
            create_query->data.node_creation.claim_id = strdupz(localhost->aclk_state.claimed_id);
            rrdhost_aclk_state_unlock(localhost);
            create_query->data.node_creation.hops = list->hops;
            create_query->data.node_creation.hostname = list->hostname;
            create_query->data.node_creation.machine_guid  = mallocz(UUID_STR_LEN);
            uuid_unparse_lower(list->host_id, (char*)create_query->data.node_creation.machine_guid);
            info("Queuing registration for host=%s, hops=%d",(char*)create_query->data.node_creation.machine_guid,
                 list->hops);
            aclk_queue_query(create_query);
        }

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
    if (!get_proto_alert_status(host, &status)) {
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

char *ng_aclk_state(void)
{
    BUFFER *wb = buffer_create(1024);
    char *ret;

    buffer_strcat(wb,
        "ACLK Available: Yes\n"
        "ACLK Version: 2\n"
#ifdef ENABLE_NEW_CLOUD_PROTOCOL
        "Protocols Supported: Legacy, Protobuf\n"
#else
        "Protocols Supported: Legacy\n"
#endif
    );
    buffer_sprintf(wb, "Protocol Used: %s\nClaimed: ", aclk_use_new_cloud_arch ? "Protobuf" : "Legacy");

    char *agent_id = is_agent_claimed();
    if (agent_id == NULL)
        buffer_strcat(wb, "No\n");
    else {
        char *cloud_base_url = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", NULL);
        buffer_sprintf(wb, "Yes\nClaimed Id: %s\nCloud URL: %s\n", agent_id, cloud_base_url ? cloud_base_url : "null");
        freez(agent_id);
    }

    buffer_sprintf(wb, "Online: %s\nReconnect count: %d\n", aclk_connected ? "Yes" : "No", aclk_connection_counter > 0 ? (aclk_connection_counter - 1) : 0);

    if (aclk_connected) {
        buffer_sprintf(wb, "Received Cloud MQTT Messages: %d\nMQTT Messages Confirmed by Remote Broker (PUBACKs): %d", aclk_rcvd_cloud_msgs, aclk_pubacks_per_conn);

        RRDHOST *host;
        rrd_rdlock();
        rrdhost_foreach_read(host) {
            buffer_sprintf(wb, "\n\n> Node Instance for mGUID: \"%s\" hostname \"%s\"\n", host->machine_guid, host->hostname);

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
        }
        rrd_unlock();
    }

    ret = strdupz(buffer_tostring(wb));
    buffer_free(wb);
    return ret;
}

static void fill_alert_status_for_host_json(json_object *obj, RRDHOST *host)
{
    struct proto_alert_status status;
    if (!get_proto_alert_status(host, &status))
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

char *ng_aclk_state_json(void)
{
    json_object *tmp, *grp, *msg = json_object_new_object();

    tmp = json_object_new_boolean(1);
    json_object_object_add(msg, "aclk-available", tmp);

    tmp = json_object_new_int(2);
    json_object_object_add(msg, "aclk-version", tmp);

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    grp = json_object_new_array_ext(2);
    tmp = json_object_new_string("Legacy");
    json_object_array_add(grp, tmp);
    tmp = json_object_new_string("Protobuf");
    json_object_array_add(grp, tmp);
#else
    grp = json_object_new_array_ext(1);
    tmp = json_object_new_string("Legacy");
    json_object_array_add(grp, tmp);
#endif
    json_object_object_add(msg, "protocols-supported", grp);

    char *agent_id = is_agent_claimed();
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

    tmp = json_object_new_string(aclk_use_new_cloud_arch ? "Protobuf" : "Legacy");
    json_object_object_add(msg, "used-cloud-protocol", tmp);

    tmp = json_object_new_int(aclk_rcvd_cloud_msgs);
    json_object_object_add(msg, "received-app-layer-msgs", tmp);

    tmp = json_object_new_int(aclk_pubacks_per_conn);
    json_object_object_add(msg, "received-mqtt-pubacks", tmp);

    tmp = json_object_new_int(aclk_connection_counter > 0 ? (aclk_connection_counter - 1) : 0);
    json_object_object_add(msg, "reconnect-count", tmp);

    grp = json_object_new_array();

    RRDHOST *host;
    rrd_rdlock();
    rrdhost_foreach_read(host) {
        json_object *nodeinstance = json_object_new_object();

        tmp = json_object_new_string(host->hostname);
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

        tmp =json_object_new_object();
        fill_alert_status_for_host_json(tmp, host);
        json_object_object_add(nodeinstance, "alert-sync-status", tmp);

        json_object_array_add(grp, nodeinstance);
    }
    rrd_unlock();
    json_object_object_add(msg, "node-instances", grp);

    char *str = strdupz(json_object_to_json_string_ext(msg, JSON_C_TO_STRING_PLAIN));
    json_object_put(msg);
    return str;
}
