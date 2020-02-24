// SPDX-License-Identifier: GPL-3.0-or-later

#include <libnetdata/json/json.h>
#include "../daemon/common.h"
#include "mqtt.h"
#include "aclk_lws_wss_client.h"

void (*_on_connect)(void *ptr) = NULL;
void (*_on_disconnect)(void *ptr) = NULL;

inline const char *_link_strerror(int rc)
{
    return mosquitto_strerror(rc);
}

static struct mosquitto *mosq = NULL;

void mqtt_message_callback(struct mosquitto *mosq, void *obj, const struct mosquitto_message *msg)
{
    UNUSED(mosq);
    UNUSED(obj);

    aclk_handle_cloud_request(msg->payload);
}

struct aclk_lws_wss_engine_instance *lws_engine_instance = NULL;

void publish_callback(struct mosquitto *mosq, void *obj, int rc)
{
    UNUSED(mosq);
    UNUSED(obj);
    UNUSED(rc);

    // TODO: link this with a msg_id so it can be traced
    return;
}

void connect_callback(struct mosquitto *mosq, void *obj, int rc)
{
    UNUSED(obj);
    UNUSED(rc);

    info("Connection to cloud estabilished");

    aclk_mqtt_connected = 1;
    _on_connect((void *)mosq);

    return;
}

void disconnect_callback(struct mosquitto *mosq, void *obj, int rc)
{
    UNUSED(obj);
    UNUSED(rc);

    info("Connection to cloud failed");

    aclk_mqtt_connected = 0;
    _on_disconnect((void *)mosq);

    if (lws_engine_instance)
        aclk_lws_wss_mqtt_layer_disconect_notif(lws_engine_instance);

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
    return aclk_lws_wss_client_write(lws_engine_instance, buf, count);
}

size_t _mqtt_external_read_hook(void *buf, size_t count)
{
    return aclk_lws_wss_client_read(lws_engine_instance, buf, count);
}

int _mqtt_lib_init(void (*on_connect)(void *), void (*on_disconnect)(void *))
{
    int rc;
    int libmosq_major, libmosq_minor, libmosq_revision, libmosq_version;
    char *ca_crt;
    char *server_crt;
    char *server_key;

    // show library info so can have it in the logfile
    libmosq_version = mosquitto_lib_version(&libmosq_major, &libmosq_minor, &libmosq_revision);
    ca_crt = config_get(CONFIG_SECTION_ACLK, "agent cloud link cert", "*");
    server_crt = config_get(CONFIG_SECTION_ACLK, "agent cloud link server cert", "*");
    server_key = config_get(CONFIG_SECTION_ACLK, "agent cloud link server key", "*");

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

    //    info(
    //        "Detected libmosquitto library version %d, %d.%d.%d", libmosq_version, libmosq_major, libmosq_minor,
    //        libmosq_revision);

    rc = mosquitto_lib_init();
    if (unlikely(rc != MOSQ_ERR_SUCCESS)) {
        error("Failed to initialize MQTT (libmosquitto library)");
        return 1;
    }

    mosq = mosquitto_new("anon", true, NULL);
    if (unlikely(!mosq)) {
        mosquitto_lib_cleanup();
        error("MQTT new structure  -- %s", mosquitto_strerror(errno));
        return 1;
    }

    _on_connect = on_connect;
    _on_disconnect = on_disconnect;

    mosquitto_connect_callback_set(mosq, connect_callback);
    mosquitto_disconnect_callback_set(mosq, disconnect_callback);
    mosquitto_publish_callback_set(mosq, publish_callback);

    mosquitto_username_pw_set(mosq, NULL, NULL);

    rc = mosquitto_threaded_set(mosq, 1);
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

int _link_mqtt_connect(char *aclk_hostname, int aclk_port)
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

    rc = mosquitto_loop_misc(mosq);
    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        debug(D_ACLK, "ACLK: failure during mosquitto_loop_misc %s", mosquitto_strerror(rc));

    if (likely(mosquitto_want_write(mosq))) {
        rc = mosquitto_loop_write(mosq, 1);
        if (rc != MOSQ_ERR_SUCCESS)
            debug(D_ACLK, "ACLK: failure during mosquitto_loop_write %s", mosquitto_strerror(rc));
    }
}

void aclk_lws_connect_notif_callback()
{
    //the connection is done by LWS so this parameters dont matter
    //ig MQTT over LWS is used
    _link_mqtt_connect(aclk_hostname, aclk_port);
    _link_mosquitto_write();
}

void aclk_lws_data_received_callback()
{
    int rc = mosquitto_loop_read(mosq, 1);
    if (rc != MOSQ_ERR_SUCCESS)
        debug(D_ACLK, "ACLK: failure during mosquitto_loop_read %s", mosquitto_strerror(rc));
}

void aclk_lws_connection_closed()
{
    aclk_disconnect(NULL);
}

static const struct aclk_lws_wss_engine_callbacks aclk_lws_engine_callbacks = {
    .connection_established_callback = aclk_lws_connect_notif_callback,
    .data_rcvd_callback = aclk_lws_data_received_callback,
    .data_writable_callback = NULL,
    .connection_closed = aclk_lws_connection_closed
};

int _link_lib_init(char *aclk_hostname, int aclk_port, void (*on_connect)(void *), void (*on_disconnect)(void *))
{
    int rc;

    // we will connect when WebSocket connection is up
    // based on callback
    if (!lws_engine_instance)
        lws_engine_instance = aclk_lws_wss_client_init(&aclk_lws_engine_callbacks, aclk_hostname, aclk_port);
    else
        aclk_lws_wss_connect(lws_engine_instance);

    aclk_lws_wss_service_loop(lws_engine_instance);

    rc = _mqtt_lib_init(on_connect, on_disconnect);
    if (rc != MOSQ_ERR_SUCCESS)
        return rc;

    mosquitto_external_callbacks_set(mosq, _mqtt_external_write_hook, _mqtt_external_read_hook);
    if (!lws_engine_instance)
        return 1;
    else
        return MOSQ_ERR_SUCCESS;
}

inline int _link_event_loop()
{
    if (unlikely(!lws_engine_instance)) {
        return MOSQ_ERR_SUCCESS;
    }

    if (lws_engine_instance && lws_engine_instance->websocket_connection_up)
        _link_mosquitto_write();

    aclk_lws_wss_service_loop(lws_engine_instance);
    // this is because if use LWS we don't want
    // mqtt to reconnect by itself
    return MOSQ_ERR_SUCCESS;
}

void _link_shutdown()
{
    int rc;

    rc = mosquitto_disconnect(mosq);
    switch (rc) {
        case MOSQ_ERR_SUCCESS:
            info("MQTT disconnected from broker");
            break;
        default:
            info("MQTT invalid structure");
            break;
    };

    mosquitto_destroy(mosq);
    mosq = NULL;

    if (lws_engine_instance) {
        aclk_lws_wss_client_destroy(lws_engine_instance);
        lws_engine_instance = NULL;
    }
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

int _link_send_message(char *topic, char *message, int *mid)
{
    int rc;

    rc = mosquitto_pub_topic_check(topic);

    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        return rc;

    int msg_len = strlen(message);

    rc = mosquitto_publish(mosq, mid, topic, msg_len, message, ACLK_QOS, 0);

    // TODO: Add better handling -- error will flood the logfile here
    if (unlikely(rc != MOSQ_ERR_SUCCESS)) {
        errno = 0;
        error("MQTT message failed : %s", mosquitto_strerror(rc));
    }
    _link_mosquitto_write();
    return rc;
}
