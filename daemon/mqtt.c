// SPDX-License-Identifier: GPL-3.0-or-later

#include "mqtt.h"
#include "common.h"

struct mosquitto *mosq = NULL;

// Read from the config file -- new section [mqtt]
// Defaults are supplied
int mqtt_recv_maximum = 0;      // default 20
int mqtt_send_maximum = 0;      // default 20
int mqtt_broker_port = 0;       // default 1883
char *mqtt_broker_hostname = NULL;  //default localhost

// Set when we have connection up and running from the connection callback
int mqtt_connection_initialized = 0;

// TODO: integrate with the actual claiming process and get the claiming status from there
int am_i_claimed()
{
    return 1;
}


// This will give the base topic that the agent will publish messages.
// subtopics will be sent under the base topic e.g.  base_topic/subtopic
// This is called by mqtt_init(), to compute the base topic once and have
// it stored internally.
// Need to check if additional logic should be added to make sure that there
// is enough information to determine the base topic at init time


// TODO: Locking may be needed, depends on the calculation of the base topic and also if we need to switch
// that on the fly

char *get_publish_base_topic(PUBLISH_TOPIC_ACTION action)
{
    static char  *topic = NULL;

    if (unlikely(action == PUBLICH_TOPIC_FREE)) {
        // TODO: the following is not safe
        if (likely(topic)) {
            freez(topic);
            topic = NULL;
        }
        return NULL;
    }

    if (unlikely(action == PUBLICH_TOPIC_REBUILD)) {
        get_publish_base_topic(PUBLICH_TOPIC_FREE);
        return get_publish_base_topic(PUBLICH_TOPIC_GET);
    }

    if (unlikely(!topic))
        topic = strdupz("netdata");

    return topic;
}


// Thread cleanup
static void mqtt_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}


/**
 * MQTT Main
 *
 * This thread will simply call the internal libmosquitto loop to handle
 * pending requests - both inbound and outbound
 *
 * @param ptr is a pointer to the netdata_static_thread structure.
 *
 * @return It always returns NULL
 */
void *mqtt_main(void *ptr) {

    netdata_thread_cleanup_push(mqtt_main_cleanup, ptr);

    int libmosq_major, libmosq_minor, libmosq_revision, libmosq_version;

    libmosq_version =  mosquitto_lib_version(&libmosq_major, &libmosq_minor, &libmosq_revision);

    info("Detected libmosquitto library version %d, %d.%d.%d",libmosq_version, libmosq_major, libmosq_minor, libmosq_revision);

    while(!netdata_exit) {
        int rc;

        // TODO: This may change when we have enough info from the claiming itself to avoid wasting 60 seconds
        // TODO: Handle the unclaim command as well -- we may need to shutdown the connection
        if (am_i_claimed() == 0) {
            usleep(60000 * 1000);
            continue;
        }

        if (unlikely(!mosq)) {
            info("Initializing connection");
            if (unlikely(mqtt_init(MQTT_INIT))) {
                // TODO: TBD how to handle. We are claimed and we cant init the connection. For now keep trying.
                usleep(60000 * 1000);
                continue;
            }
            continue;
        }

        // Call the loop to handle inbound and outbound messages  // Timeout after 60 seconds
        rc = mosquitto_loop(mosq, NETDATA_MQTT_LOOP_TIMEOUT * 1000, 1);

        if (unlikely(rc !=MOSQ_ERR_SUCCESS )) {
            rc = mosquitto_reconnect(mosq);
            if (unlikely(rc != MOSQ_ERR_SUCCESS)) {
                error("Reconnect loop error code %d (%s) host=%s, port=%d", rc, mosquitto_strerror(rc),
                    mqtt_broker_hostname, mqtt_broker_port);

                // TBD: Using delay
                usleep(10000 * 1000);
            } else
                error("Loop error code %d (%s)", rc, mosquitto_strerror(rc));
            continue;
        }

//        if (unlikely(rc == MOSQ_ERR_INVAL)) {
//            // TODO: handle the edge case -- reset the connection and retry (for now sleep and loop)
//            error("Invalid MQTT connection; dying?");
//            break;
//        }

        // TODO: handle connection lost and REINIT gracefully -- for now handles internally
        // TODO: need to decide retry interval and total duration
//        if (rc == MOSQ_ERR_CONN_LOST) {
//            mqtt_init(MQTT_REINIT);
//        }

    } // forever
    mqtt_shutdown();

    netdata_thread_cleanup_pop(1);
    return NULL;
}

//    rc = mosquitto_publish(mosq, NULL, "netdata/alarm", strlen(mqtt_message), mqtt_message, 0, 0);

/*
 * Send a message to the cloud, using a base topic and sib_topic
 * The final topic will be in the form <base_topic>/<sub_topic>
 * If base_topic is missing then the global_base_topic will be used (if available)
 *
 */
int mqtt_send(char *base_topic, char *sub_topic, char *message)
{
    int rc;
    static int skip_due_to_shutdown = 0;
    static char *global_base_topic = NULL;
    char topic[NETDATA_MQTT_MAX_TOPIC + 1];
    char *final_topic;

    if (!am_i_claimed())
        return 0;

    if (unlikely(netdata_exit)) {

        if (unlikely(!mqtt_connection_initialized))
            return 0;

        ++skip_due_to_shutdown;
        if (unlikely(!(skip_due_to_shutdown % 100)))
            info("%d messages not sent -- shutdown in progress", skip_due_to_shutdown);
        return 0;
    }

    if (unlikely(!message))
        return 0;

    // Just wait to be initialized
    if (unlikely(!mqtt_connection_initialized)) {
        time_t now = now_realtime_sec();

        while (!mqtt_connection_initialized && (now_realtime_sec() - now) < NETDATA_MQTT_INITIALIZATION_WAIT) {
            usleep(NETDATA_MQTT_INITIALIZATION_SLEEP_WAIT * 1000);
        }

        if (unlikely(!mqtt_connection_initialized)) {
            error("MQTT connection not active");
            return 0;
        }
    }

    if (unlikely(!global_base_topic))
        global_base_topic = GET_PUBLISH_BASE_TOPIC;

    if (unlikely(!base_topic)) {
        if (unlikely(!global_base_topic))
            final_topic = sub_topic;
        else {
            snprintfz(topic, NETDATA_MQTT_MAX_TOPIC, "%s/%s", global_base_topic, sub_topic);
            final_topic = topic;
        }
    } else {
        snprintfz(topic, NETDATA_MQTT_MAX_TOPIC, "%s/%s", base_topic, sub_topic);
        final_topic = topic;
    }

    rc = mosquitto_pub_topic_check(final_topic);
    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        return rc;

    int msg_len = strlen(message);

    // TODO: handle encoding validation -- the message should be UFT8 encoded by the sender
    //rc = mosquitto_validate_utf8(message, msg_len);
    //if (unlikely(rc != MOSQ_ERR_SUCCESS))
    //    return rc;

    rc = mosquitto_publish_v5(mosq, NULL, final_topic, msg_len, message, NETDATA_MQTT_QOS , 0, NULL);

    // TODO: Add better handling -- error will flood the logfile here
    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        error("MQTT message failed : %s",mosquitto_strerror(rc));


    return rc;
}

//TODO: placeholder for password check if we need it
//int password_callback(char *buf, int size, int rwflag, void *userdata)
//{
//    return 8;
//}

void mqtt_message_callback(
    struct mosquitto *moqs, void *obj, const struct mosquitto_message *msg, const mosquitto_property *props)
{
    info("MQTT received message %d [%s]", msg->payloadlen, (char *)msg->payload);

    // TODO: handle commands in a more efficient way, if we have many
    if (strcmp((char *)msg->payload, "reload") == 0) {
        error_log_limit_unlimited();
        info("Reloading health configuration");
        health_reload();
        error_log_limit_reset();
    }
}

void connect_callback(struct mosquitto *mosq, void *obj, int rc, int flags, const mosquitto_property *props)
{
    info("Connection to cloud estabilished");
    mqtt_connection_initialized = 1;
}


void disconnect_callback(struct mosquitto *mosq, void *obj, int rc, int flags, const mosquitto_property *props)
{
    info("Connection to cloud failed");
    // TODO: Keep the connection "alive" for now. The library will reconnect.
    //mqtt_connection_initialized = 0;
}


void mqtt_shutdown()
{
    int rc;

    info("MQTT Shutdown initiated");

    mqtt_connection_initialized = 0;

    rc = mosquitto_disconnect(mosq);
    switch (rc) {
        case MOSQ_ERR_SUCCESS:
            info("MQTT disconnected from broker");
            break;
        default:
            info("MQTT invalid structure");
            break;
    };
    info("Thread processing shutting down");

    mosquitto_destroy(mosq);
    info("MQTT shutdown complete");
    mosq = NULL;
}

int mqtt_init(MQTT_INIT_ACTION action)
{
    static  init = 0;
    int rc;

    // Check if we should do reinit
    if (unlikely(action == MQTT_REINIT)) {
        if (unlikely(!init))
            return 0;

        info("MQTT reinit requested");
        mqtt_shutdown();
        info("Cleanup mosquitto library");
        mosquitto_lib_cleanup();

    }

    if (unlikely(!init)) {
        mqtt_send_maximum  = config_get_number(CONFIG_SECTION_MQTT, "mqtt send maximum", 20);
        mqtt_recv_maximum  = config_get_number(CONFIG_SECTION_MQTT, "mqtt receive maximum", 20);

        mqtt_broker_hostname = config_get(CONFIG_SECTION_MQTT, "mqtt broker hostname",  "localhost");
        mqtt_broker_port     = config_get_number(CONFIG_SECTION_MQTT, "mqtt broker port",  1883);

        info("Maximum parallel outgoing messages %d", mqtt_send_maximum);
        info("Maximum parallel incoming messages %d", mqtt_recv_maximum);

        // This will setup the base publish topic internally
        get_publish_base_topic(PUBLICH_TOPIC_GET);
        init = 1;
    }

    // initialize mosquitto library
    rc = mosquitto_lib_init();
    if (unlikely(rc != MOSQ_ERR_SUCCESS)) {
        error("Failed to initialize MQTT (libmosquitto library)");
        return 1;
    }

    mosq = mosquitto_new(NULL, true, NULL);
    if (unlikely(!mosq)) {
        mosquitto_lib_cleanup();
        error("MQTT new structure  -- %s", mosquitto_strerror(errno));
        return 1;
    }

    mosquitto_connect_v5_callback_set(mosq, connect_callback);

    mosquitto_disconnect_v5_callback_set(mosq, disconnect_callback);

    rc = mosquitto_threaded_set(mosq,1);
    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        error("Failed to tune the thread model for libmoquitto (%s)",mosquitto_strerror(rc));

    // attempt V5 protocol for now, no doc on the return status for now
    rc = mosquitto_int_option(mosq, MQTT_PROTOCOL_V5, 0);
    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        error("MQTT protocol specification rc = %d (%s)", rc, mosquitto_strerror(rc));


    // These added for the V5 -- may need to be removed if we go to 3.x
    rc = mosquitto_int_option(mosq, MOSQ_OPT_RECEIVE_MAXIMUM, mqtt_recv_maximum);
    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        error("MQTT receive maximum queue set failed rc = %d (%s)", rc, mosquitto_strerror(rc));

    rc = mosquitto_int_option(mosq, MOSQ_OPT_SEND_MAXIMUM, mqtt_send_maximum);
    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        error("MQTT send maximum queue set failed rc = %d (%s)", rc, mosquitto_strerror(rc));

    rc = mosquitto_int_option(mosq, MOSQ_OPT_SEND_MAXIMUM, 1);
    info("MQTT in flight messages set to 1  -- %s",mosquitto_strerror(rc));

    rc = mosquitto_reconnect_delay_set(mosq, NETDATA_MQTT_RECONNECT_DELAY, NETDATA_MQTT_MAX_RECONNECT_DELAY, 1);

    //mosquitto_tls_set(mosq, "/etc/netdata/mqtt/ca.crt", NULL, "/etc/netdata/mqtt/server.crt", "/etc/netdata/mqtt/server.key", NULL);

    rc = mosquitto_connect_async(mosq, mqtt_broker_hostname, mqtt_broker_port, NETDATA_MQTT_PING_INTERVAL);
    if (unlikely(rc != MOSQ_ERR_SUCCESS))
        error("Connect %s MQTT status = %d (%s)", netdata_configured_hostname, rc, mosquitto_strerror(rc));
    else
        info("Establishing MQTT link to %s", netdata_configured_hostname);

    // TODO: This may need to be elsewhere -- also check return code
    mqtt_subscribe("netdata/command");

    return 0;
}

/* Subscribe to a topic
 *
 *  Return 0 - success
 *         1 - Failure
 */

int mqtt_subscribe(char  *topic)
{
    int rc;

    if (unlikely(!mosq))
        return 1;

    mosquitto_message_v5_callback_set(mosq, mqtt_message_callback);

    // TODO: remove hard coded params
    rc = mosquitto_subscribe_v5(mosq, NULL, "netdata/command", 1, 0, NULL);
    if (unlikely(rc)) {
        errno = 0;
        error("Failed to register subscription %d (%s)", rc, mosquitto_strerror(rc));
        return 1;
    }

    return 0;
}
