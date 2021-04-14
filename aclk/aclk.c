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

#ifdef ACLK_LOG_CONVERSATION_DIR
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#endif

#define ACLK_STABLE_TIMEOUT 3 // Minimum delay to mark AGENT as stable

//TODO remove most (as in 99.999999999%) of this crap
int aclk_connected = 0;
int aclk_disable_runtime = 0;
int aclk_disable_single_updates = 0;
int aclk_kill_link = 0;

int aclk_pubacks_per_conn = 0; // How many PubAcks we got since MQTT conn est.

usec_t aclk_session_us = 0;         // Used by the mqtt layer
time_t aclk_session_sec = 0;        // Used by the mqtt layer

mqtt_wss_client mqttwss_client;

netdata_mutex_t aclk_shared_state_mutex = NETDATA_MUTEX_INITIALIZER;
#define ACLK_SHARED_STATE_LOCK netdata_mutex_lock(&aclk_shared_state_mutex)
#define ACLK_SHARED_STATE_UNLOCK netdata_mutex_unlock(&aclk_shared_state_mutex)

struct aclk_shared_state aclk_shared_state = {
    .agent_state = AGENT_INITIALIZING,
    .last_popcorn_interrupt = 0,
    .version_neg = 0,
    .version_neg_wait_till = 0,
    .mqtt_shutdown_msg_id = -1,
    .mqtt_shutdown_msg_rcvd = 0
};

void aclk_single_update_disable()
{
    aclk_disable_single_updates = 1;
}

void aclk_single_update_enable()
{
    aclk_disable_single_updates = 0;
}

//ENDTODO

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
 * @param ackl_port port to int where port will be saved
 * 
 * @return If non 0 returned irrecoverable error happened and ACLK should be terminated
 */
static int wait_till_agent_claim_ready()
{
    int port;
    char *hostname = NULL;
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
        if (aclk_decode_base_url(cloud_base_url, &hostname, &port)) {
            error("Agent is claimed but the configuration is invalid, please fix");
            freez(hostname);
            hostname = NULL;
            sleep(5);
            continue;
        }
        freez(hostname);
        hostname = NULL;

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
static void msg_callback(const char *topic, const void *msg, size_t msglen, int qos)
{
    char cmsg[RX_MSGLEN_MAX];
    size_t len = (msglen < RX_MSGLEN_MAX - 1) ? msglen : (RX_MSGLEN_MAX - 1);

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

    debug(D_ACLK, "Got Message From Broker Topic \"%s\" QOS %d MSG: \"%s\"", topic, qos, cmsg);

    if (strcmp(aclk_get_topic(ACLK_TOPICID_COMMAND), topic))
        error("Received message on unexpected topic %s", topic);

    if (aclk_shared_state.mqtt_shutdown_msg_id > 0) {
        error("Link is shutting down. Ignoring message.");
        return;
    }

    aclk_handle_cloud_message(cmsg);
}

static void puback_callback(uint16_t packet_id)
{
    if (++aclk_pubacks_per_conn == ACLK_PUBACKS_CONN_STABLE)
        aclk_reconnect_delay(0);

#ifdef NETDATA_INTERNAL_CHECKS
    aclk_stats_msg_puback(packet_id);
#endif

    if (aclk_shared_state.mqtt_shutdown_msg_id == (int)packet_id) {
        error("Got PUBACK for shutdown message. Can exit gracefully.");
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

/* Keeps connection alive and handles all network comms.
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
            error("Connection Error or Dropped");
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

inline static int aclk_popcorn_check_bump()
{
    ACLK_SHARED_STATE_LOCK;
    if (unlikely(aclk_shared_state.agent_state == AGENT_INITIALIZING)) {
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
    // TODO global vars?
    usec_t now = now_realtime_usec();
    aclk_session_sec = now / USEC_PER_SEC;
    aclk_session_us = now % USEC_PER_SEC;

    mqtt_wss_subscribe(client, aclk_get_topic(ACLK_TOPICID_COMMAND), 1);

    aclk_stats_upd_online(1);
    aclk_connected = 1;
    aclk_pubacks_per_conn = 0;
    aclk_hello_msg(client);
    ACLK_SHARED_STATE_LOCK;
    if (aclk_shared_state.agent_state != AGENT_INITIALIZING) {
        error("Sending `connect` payload immediatelly as popcorning was finished already.");
        queue_connect_payloads();
    }
    ACLK_SHARED_STATE_UNLOCK;
}

/* Waits until agent is ready or needs to exit
 * @param client instance of mqtt_wss_client
 * @param query_threads pointer to aclk_query_threads
 *        structure where to store data about started query threads
 * @return  0 - Popcorning Finished - Agent STABLE,
 *         !0 - netdata_exit
 */
static int wait_popcorning_finishes(mqtt_wss_client client, struct aclk_query_threads *query_threads)
{
    time_t elapsed;
    int need_wait;
    while (!netdata_exit) {
        ACLK_SHARED_STATE_LOCK;
        if (likely(aclk_shared_state.agent_state != AGENT_INITIALIZING)) {
            ACLK_SHARED_STATE_UNLOCK;
            return 0;
        }
        elapsed = now_realtime_sec() - aclk_shared_state.last_popcorn_interrupt;
        if (elapsed >= ACLK_STABLE_TIMEOUT) {
            aclk_shared_state.agent_state = AGENT_STABLE;
            ACLK_SHARED_STATE_UNLOCK;
            error("ACLK localhost popocorn finished");
            if (unlikely(!query_threads->thread_list))
                aclk_query_threads_start(query_threads, client);
            queue_connect_payloads();
            return 0;
        }
        ACLK_SHARED_STATE_UNLOCK;
        need_wait = ACLK_STABLE_TIMEOUT - elapsed;
        error("ACLK localhost popocorn wait %d seconds longer", need_wait);
        sleep(need_wait);
    }
    return 1;
}

void aclk_graceful_disconnect(mqtt_wss_client client)
{
    error("Preparing to Gracefully Shutdown the ACLK");
    aclk_queue_lock();
    aclk_queue_flush();
    aclk_shared_state.mqtt_shutdown_msg_id = aclk_send_app_layer_disconnect(client, "graceful");
    time_t t = now_monotonic_sec();
    while (!mqtt_wss_service(client, 100)) {
        if (now_monotonic_sec() - t >= 2) {
            error("Wasn't able to gracefully shutdown ACLK in time!");
            break;
        }
        if (aclk_shared_state.mqtt_shutdown_msg_rcvd) {
            error("MQTT App Layer `disconnect` message sent successfully");
            break;
        }
    }
    aclk_stats_upd_online(0);
    aclk_connected = 0;

    error("Attempting to Gracefully Shutdown MQTT/WSS connection");
    mqtt_wss_disconnect(client, 1000);
}

/* Block till aclk_reconnect_delay is satisifed or netdata_exit is signalled
 * @return 0 - Go ahead and connect (delay expired)
 *         1 - netdata_exit
 */
#define NETDATA_EXIT_POLL_MS (MSEC_PER_SEC/4)
static int aclk_block_till_recon_allowed() {
    // Handle reconnect exponential backoff
    // fnc aclk_reconnect_delay comes from ACLK Legacy @amoss
    // but has been modifed slightly (more randomness)
    unsigned long recon_delay = aclk_reconnect_delay(1);
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
    return 0;
}

#define HTTP_PROXY_PREFIX "http://"
static void set_proxy(struct mqtt_wss_proxy *out)
{
    ACLK_PROXY_TYPE pt;
    const char *ptr = aclk_get_proxy(&pt);
    char *tmp;
    char *host;
    if (pt != PROXY_TYPE_HTTP)
        return;

    out->port = 0;

    if (!strncmp(ptr, HTTP_PROXY_PREFIX, strlen(HTTP_PROXY_PREFIX)))
        ptr += strlen(HTTP_PROXY_PREFIX);

    if ((tmp = strchr(ptr, '@')))
        ptr = tmp;

    if ((tmp = strchr(ptr, '/'))) {
        host = mallocz((tmp - ptr) + 1);
        memcpy(host, ptr, (tmp - ptr));
        host[tmp - ptr] = 0;
    } else
        host = strdupz(ptr);

    if ((tmp = strchr(host, ':'))) {
        *tmp = 0;
        tmp++;
        out->port = atoi(tmp);
    }

    if (out->port <= 0 || out->port > 65535)
        out->port = 8080;

    out->host = host;

    out->type = MQTT_WSS_PROXY_HTTP;
}

/* Attempts to make a connection to MQTT broker over WSS
 * @param client instance of mqtt_wss_client
 * @return  0 - Successfull Connection,
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
    char *aclk_hostname = NULL;
    int aclk_port;

#ifndef ACLK_DISABLE_CHALLENGE
    char *mqtt_otp_user = NULL;
    char *mqtt_otp_pass = NULL;
#endif

    json_object *lwt;

    while (!netdata_exit) {
        char *cloud_base_url = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", NULL);
        if (cloud_base_url == NULL) {
            error("Do not move the cloud base url out of post_conf_load!!");
            return -1;
        }

        if (aclk_block_till_recon_allowed())
            return 1;

        info("Attempting connection now");
        if (aclk_decode_base_url(cloud_base_url, &aclk_hostname, &aclk_port)) {
            error("ACLK base URL configuration key could not be parsed. Will retry in %d seconds.", CLOUD_BASE_URL_READ_RETRY);
            sleep(CLOUD_BASE_URL_READ_RETRY);
            continue;
        }

        struct mqtt_wss_proxy proxy_conf;
        proxy_conf.type = MQTT_WSS_DIRECT;
        set_proxy(&proxy_conf);

        struct mqtt_connect_params mqtt_conn_params = {
            .clientid   = "anon",
            .username   = "anon",
            .password   = "anon",
            .will_topic = aclk_get_topic(ACLK_TOPICID_METADATA),
            .will_msg   = NULL,
            .will_flags = MQTT_WSS_PUB_QOS2,
            .keep_alive = 60
        };
#ifndef ACLK_DISABLE_CHALLENGE
        aclk_get_mqtt_otp(aclk_private_key, aclk_hostname, aclk_port, &mqtt_otp_user, &mqtt_otp_pass);
        mqtt_conn_params.clientid = mqtt_otp_user;
        mqtt_conn_params.username = mqtt_otp_user;
        mqtt_conn_params.password = mqtt_otp_pass;
#endif

        lwt = aclk_generate_disconnect(NULL);
        mqtt_conn_params.will_msg = json_object_to_json_string_ext(lwt, JSON_C_TO_STRING_PLAIN);

        mqtt_conn_params.will_msg_len = strlen(mqtt_conn_params.will_msg);
        if (!mqtt_wss_connect(client, aclk_hostname, aclk_port, &mqtt_conn_params, ACLK_SSL_FLAGS, &proxy_conf)) {
            json_object_put(lwt);
            freez(aclk_hostname);
            aclk_hostname = NULL;
            info("MQTTWSS connection succeeded");
            mqtt_connected_actions(client);
            return 0;
        }

        freez(aclk_hostname);
        aclk_hostname = NULL;
        json_object_put(lwt);
        error("Connect failed\n");
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

    if (!(mqttwss_client = mqtt_wss_new("mqtt_wss", aclk_mqtt_wss_log_cb, msg_callback, puback_callback))) {
        error("Couldn't initialize MQTT_WSS network library");
        goto exit;
    }

    aclk_stats_enabled = config_get_boolean(CONFIG_SECTION_CLOUD, "statistics", CONFIG_BOOLEAN_YES);
    if (aclk_stats_enabled) {
        stats_thread = callocz(1, sizeof(struct aclk_stats_thread));
        stats_thread->thread = mallocz(sizeof(netdata_thread_t));
        stats_thread->query_thread_count = query_threads.count;
        netdata_thread_create(
            stats_thread->thread, ACLK_STATS_THREAD_NAME, NETDATA_THREAD_OPTION_JOINABLE, aclk_stats_main_thread,
            stats_thread);
    }

    // Keep reconnecting and talking until our time has come
    // and the Grim Reaper (netdata_exit) calls
    do {
        if (aclk_attempt_to_connect(mqttwss_client))
            goto exit_full;

        // warning this assumes the popcorning is relative short (3s)
        // if that changes call mqtt_wss_service from within
        // to keep OpenSSL, WSS and MQTT connection alive
        if (wait_popcorning_finishes(mqttwss_client, &query_threads))
            goto exit_full;

        if (!handle_connection(mqttwss_client)) {
            aclk_stats_upd_online(0);
            aclk_connected = 0;
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
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
    return NULL;
}

// TODO this is taken over as workaround from old ACLK
// fix this in both old and new ACLK
extern void health_alarm_entry2json_nolock(BUFFER *wb, ALARM_ENTRY *ae, RRDHOST *host);

void aclk_alarm_reload(void)
{
    ACLK_SHARED_STATE_LOCK;
    if (unlikely(aclk_shared_state.agent_state == AGENT_INITIALIZING)) {
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
    if (unlikely(aclk_shared_state.agent_state == AGENT_INITIALIZING)) {
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

    if (aclk_popcorn_check_bump())
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
    if (unlikely(!netdata_ready)) {
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
    if (unlikely(!netdata_ready)) {
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

struct label *add_aclk_host_labels(struct label *label) {
#ifdef ENABLE_ACLK
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
    label = add_label_to_list(label, "_aclk_impl", "Next Generation", LABEL_SOURCE_AUTO);
    return add_label_to_list(label, "_aclk_proxy", proxy_str, LABEL_SOURCE_AUTO);
#else
    return label;
#endif
}
