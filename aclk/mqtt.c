// SPDX-License-Identifier: GPL-3.0-or-later

#include <libnetdata/json/json.h>
#include "../daemon/common.h"
#include "mqtt.h"
#include "aclk_lws_wss_client.h"
#include "aclk_stats.h"
#include "aclk_rx_msgs.h"

extern usec_t aclk_session_us;
extern time_t aclk_session_sec;

inline const char *_link_strerror(int rc)
{
    return mosquitto_strerror(rc);
}

#ifdef NETDATA_INTERNAL_CHECKS
static struct timeval sendTimes[1024];
#endif

static struct mosquitto *mosq = NULL;


void mqtt_message_callback(struct mosquitto *mosq, void *obj, const struct mosquitto_message *msg)
{
    UNUSED(mosq);
    UNUSED(obj);

    aclk_handle_cloud_message(msg->payload);
}

void publish_callback(struct mosquitto *mosq, void *obj, int rc)
{
    UNUSED(mosq);
    UNUSED(obj);
    UNUSED(rc);
#ifdef NETDATA_INTERNAL_CHECKS
    struct timeval now, *orig;
    now_realtime_timeval(&now);
    orig = &sendTimes[ rc & 0x3ff ];
    int64_t diff = (now.tv_sec - orig->tv_sec) * USEC_PER_SEC + (now.tv_usec - orig->tv_usec);
    diff /= 1000;

    info("Publish_callback: mid=%d latency=%" PRId64 "ms", rc, diff);

    if (aclk_stats_enabled) {
        ACLK_STATS_LOCK;
        if (aclk_metrics_per_sample.latency_max < diff)
            aclk_metrics_per_sample.latency_max = diff;

        aclk_metrics_per_sample.latency_total += diff;
        aclk_metrics_per_sample.latency_count++;
        ACLK_STATS_UNLOCK;
    }
#endif
    return;
}

void connect_callback(struct mosquitto *mosq, void *obj, int rc)
{
    UNUSED(mosq);
    UNUSED(obj);
    UNUSED(rc);

    info("Connection to cloud estabilished");
    aclk_connect();

    return;
}

void disconnect_callback(struct mosquitto *mosq, void *obj, int rc)
{
    UNUSED(mosq);
    UNUSED(obj);
    UNUSED(rc);

    if (netdata_exit)
        info("Connection to cloud terminated due to agent shutdown");
    else {
        errno = 0;
        error("Connection to cloud failed");
    }
    aclk_disconnect();

    aclk_lws_wss_mqtt_layer_disconect_notif();

    return;
}

void _show_mqtt_info()
{
    int libmosq_major, libmosq_minor, libmosq_revision, libmosq_version;
    libmosq_version = mosquitto_lib_version(&libmosq_major, &libmosq_minor, &libmosq_revision);

    info(
        "Detected libmosquitto library version %d, %d.%d.%d", libmosq_version, libmosq_major, libmosq_minor,
        libmosq_revision);
}

size_t _mqtt_external_write_hook(void *buf, size_t count)
{
    return aclk_lws_wss_client_write(buf, count);
}

size_t _mqtt_external_read_hook(void *buf, size_t count)
{
    return aclk_lws_wss_client_read(buf, count);
}

int _mqtt_lib_init()
{
    int rc;
    //int libmosq_major, libmosq_minor, libmosq_revision, libmosq_version;
    /* Commenting out now as it is unused - do not delete, this is needed for the on-prem version.
    char *ca_crt;
    char *server_crt;
    char *server_key;

    // show library info so can have it in the logfile
    //libmosq_version = mosquitto_lib_version(&libmosq_major, &libmosq_minor, &libmosq_revision);
    ca_crt = config_get(CONFIG_SECTION_CLOUD, "link cert", "*");
    server_crt = config_get(CONFIG_SECTION_CLOUD, "link server cert", "*");
    server_key = config_get(CONFIG_SECTION_CLOUD, "link server key", "*");

    if (ca_crt[0] == '*') {
        freez(ca_crt);
        ca_crt = NULL;
    }

    if (server_crt[0] == '*') {
        freez(server_crt);
        server_crt = NULL;
    }

    if (server_key[0] == '*') {
        freez(server_key);
        server_key = NULL;
    }
    */

    //    info(
    //        "Detected libmosquitto library version %d, %d.%d.%d", libmosq_version, libmosq_major, libmosq_minor,
    //        libmosq_revision);

    rc = mosquitto_lib_init();
    if (unlikely(rc != MOSQ_ERR_SUCCESS)) {
        error("Failed to initialize MQTT (libmosquitto library)");
        return 1;
    }
    return 0;
}

static int _mqtt_create_connection(char *username, char *password)
{
    if (mosq != NULL)
        mosquitto_destroy(mosq);
    mosq = mosquitto_new(username, true, NULL);
    if (unlikely(!mosq)) {
        mosquitto_lib_cleanup();
        error("MQTT new structure  -- %s", mosquitto_strerror(errno));
        return MOSQ_ERR_UNKNOWN;
    }

    // Record the session start time to allow a nominal LWT timestamp
    usec_t now = now_realtime_usec();
    aclk_session_sec = now / USEC_PER_SEC;
    aclk_session_us = now % USEC_PER_SEC;  

    _link_set_lwt("outbound/meta", 2);

    mosquitto_connect_callback_set(mosq, connect_callback);
    mosquitto_disconnect_callback_set(mosq, disconnect_callback);
    mosquitto_publish_callback_set(mosq, publish_callback);

    info("Using challenge-response: %s / %s", username, password);
    mosquitto_username_pw_set(mosq, username, password);

    int rc = mosquitto_threaded_set(mosq, 1);
    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        error("Failed to tune the thread model for libmoquitto (%s)", mosquitto_strerror(rc));

#if defined(LIBMOSQUITTO_VERSION_NUMBER) >= 1006000
    rc = mosquitto_int_option(mosq, MQTT_PROTOCOL_V311, 0);
    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        error("MQTT protocol specification rc = %d (%s)", rc, mosquitto_strerror(rc));

    rc = mosquitto_int_option(mosq, MOSQ_OPT_SEND_MAXIMUM, 1);
    info("MQTT in flight messages set to 1  -- %s", mosquitto_strerror(rc));
#endif

    return rc;
}

static int _link_mqtt_connect(char *aclk_hostname, int aclk_port)
{
    int rc;

    rc = mosquitto_connect_async(mosq, aclk_hostname, aclk_port, ACLK_PING_INTERVAL);

    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        error(
            "Failed to establish link to [%s:%d] MQTT status = %d (%s)", aclk_hostname, aclk_port, rc,
            mosquitto_strerror(rc));
    else
        info("Establishing MQTT link to [%s:%d]", aclk_hostname, aclk_port);

    return rc;
}

static inline void _link_mosquitto_write()
{
    int rc;

    if (unlikely(!mosq)) {
        return;
    }

    rc = mosquitto_loop_misc(mosq);
    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        debug(D_ACLK, "ACLK: failure during mosquitto_loop_misc %s", mosquitto_strerror(rc));

    if (likely(mosquitto_want_write(mosq))) {
        rc = mosquitto_loop_write(mosq, 1);
        if (rc != MOSQ_ERR_SUCCESS)
            debug(D_ACLK, "ACLK: failure during mosquitto_loop_write %s", mosquitto_strerror(rc));
    }
}

void aclk_lws_connection_established(char *hostname, int port)
{
    _link_mqtt_connect(hostname, port);  // Parameters only used for logging, lower layer connected.
    _link_mosquitto_write();
}

void aclk_lws_connection_data_received()
{
    int rc = mosquitto_loop_read(mosq, 1);
    if (rc != MOSQ_ERR_SUCCESS)
        debug(D_ACLK, "ACLK: failure during mosquitto_loop_read %s", mosquitto_strerror(rc));
}

void aclk_lws_connection_closed()
{
    aclk_disconnect();

}


int mqtt_attempt_connection(char *aclk_hostname, int aclk_port, char *username, char *password)
{
    if(aclk_lws_wss_connect(aclk_hostname, aclk_port))
        return MOSQ_ERR_UNKNOWN;
    aclk_lws_wss_service_loop();

    int rc = _mqtt_create_connection(username, password);
    if (rc!= MOSQ_ERR_SUCCESS)
        return rc;

    mosquitto_external_callbacks_set(mosq, _mqtt_external_write_hook, _mqtt_external_read_hook);
    return rc;
}

inline int _link_event_loop()
{

    // TODO: Check if we need to flush undelivered messages from libmosquitto on new connection attempts (QoS=1).
    _link_mosquitto_write();
    aclk_lws_wss_service_loop();

    // this is because if use LWS we don't want
    // mqtt to reconnect by itself
    return MOSQ_ERR_SUCCESS;
}

void _link_shutdown()
{
    int rc;

    if (likely(!mosq))
        return;

    rc = mosquitto_disconnect(mosq);
    switch (rc) {
        case MOSQ_ERR_SUCCESS:
            info("MQTT disconnected from broker");
            break;
        default:
            info("MQTT invalid structure");
            break;
    };
}


int _link_set_lwt(char *sub_topic, int qos)
{
    int rc;
    char topic[ACLK_MAX_TOPIC + 1];
    char *final_topic;

    final_topic = get_topic(sub_topic, topic, ACLK_MAX_TOPIC);
    if (unlikely(!final_topic)) {
        errno = 0;
        error("Unable to build outgoing topic; truncated?");
        return 1;
    }

    usec_t lwt_time = aclk_session_sec * USEC_PER_SEC + aclk_session_us + 1;
    BUFFER *b = buffer_create(512);
    aclk_create_header(b, "disconnect", NULL, lwt_time / USEC_PER_SEC, lwt_time % USEC_PER_SEC, ACLK_VERSION_NEG_VERSION);
    buffer_strcat(b, ", \"payload\": \"unexpected\" }");
    rc = mosquitto_will_set(mosq, topic, buffer_strlen(b), buffer_tostring(b), qos, 0);
    buffer_free(b);

    return rc;
}

int _link_subscribe(char *topic, int qos)
{
    int rc;

    if (unlikely(!mosq))
        return 1;

    mosquitto_message_callback_set(mosq, mqtt_message_callback);

    rc = mosquitto_subscribe(mosq, NULL, topic, qos);
    if (unlikely(rc)) {
        errno = 0;
        error("Failed to register subscription %d (%s)", rc, mosquitto_strerror(rc));
        return 1;
    }

    _link_mosquitto_write();
    return 0;
}

/*
 * Send a message to the cloud to specific topic
 *
 */

int _link_send_message(char *topic, const void *message, size_t len, int *mid)
{
    int rc;
    size_t write_q, write_q_bytes, read_q;

    rc = mosquitto_pub_topic_check(topic);

    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        return rc;

    lws_wss_check_queues(&write_q, &write_q_bytes, &read_q);
    rc = mosquitto_publish(mosq, mid, topic, len, message, ACLK_QOS, 0);

#ifdef NETDATA_INTERNAL_CHECKS
    char msg_head[64];
    memset(msg_head, 0, sizeof(msg_head));
    strncpy(msg_head, (char*)message, 60);
    for (size_t i = 0; i < sizeof(msg_head); i++)
        if(msg_head[i] == '\n') msg_head[i] = ' ';
    info("Sending MQTT len=%d mid=%d wq=%zu (%zu-bytes) readq=%zu: %s", (int)len,
         *mid, write_q, write_q_bytes, read_q, msg_head);
    now_realtime_timeval(&sendTimes[ *mid & 0x3ff ]);
#endif

    // TODO: Add better handling -- error will flood the logfile here
    if (unlikely(rc != MOSQ_ERR_SUCCESS)) {
        errno = 0;
        error("MQTT message failed : %s", mosquitto_strerror(rc));
    }
    _link_mosquitto_write();
    return rc;
}
