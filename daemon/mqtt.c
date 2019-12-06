// SPDX-License-Identifier: GPL-3.0-or-later

#include "mqtt.h"
#include "common.h"

struct mosquitto *mosq = NULL;


int am_i_claimed()
{
    return 1;
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

    time_t now                = now_realtime_sec();
    time_t hibernation_delay  = config_get_number(CONFIG_SECTION_HEALTH, "postpone alarms during hibernation for seconds", 60);

    //unsigned int loop = 0;
    while(!netdata_exit) {
        int rc;
        //loop++;
        //debug(D_HEALTH, "Health monitoring iteration no %u started", loop);

        if (am_i_claimed() == 0) {
            usleep(60000 * 1000);
            continue;
        }

        if (unlikely(mosq == NULL)) {
            info("Initializing MQTT connection");
            mqtt_init();
            continue;
        }

        // Call the loop to handle inbound and outbound messages  // Timeout after 60 seconds
        rc = mosquitto_loop(mosq, 60 * 1000, 1);
        if (unlikely(rc == MOSQ_ERR_INVAL)) {
            error("Invalid MQTT connection; dying?");
            break;
        }

        if (unlikely(rc !=MOSQ_ERR_SUCCESS )) {
            error("MQTT loop error code %d (%s)", rc, mosquitto_strerror(rc));
        }

        //usleep(1000 * 1000);

    } // forever
    mqtt_shutdown();

    netdata_thread_cleanup_pop(1);
    return NULL;
}

//    rc = mosquitto_publish(mosq, NULL, "netdata/alarm", strlen(mqtt_message), mqtt_message, 0, 0);


int mqtt_send(char *topic, char *message)
{
    int rc;

    if (unlikely(netdata_exit)) {
        info("MQTT message not sent -- shutdown in progress");
        return 0;
    }

    rc = mosquitto_publish_v5(mosq, NULL, topic, strlen(message), message, 1, 0, NULL);

    return rc;
}


int 		password_callback (char *buf, int size, int rwflag, void *userdata) {

    return 8;

}


void mqtt_message_callback(struct mosquitto *moqs, void *obj, const struct mosquitto_message *msg, const mosquitto_property *props) {
    info("MQTT received message %d [%s]", msg->payloadlen, (char *) msg->payload);

    if (strcmp((char *) msg->payload, "reload")==0) {
        error_log_limit_unlimited();
        info("Reloading health configuration");
        health_reload();
        error_log_limit_reset();
    }
}


void connect_callback(struct mosquitto *mosq, void *obj, int rc, int flags, const mosquitto_property *props)
{
    info("Cloud connecting with MQTT estabilished");
}

void mqtt_shutdown()
{
    int rc;
    info("MQTT shutdown initiated");
    rc = mosquitto_disconnect(mosq);
    switch (rc) {
        case MOSQ_ERR_SUCCESS:
            info("MQTT disconnected from broker");
            break;
        default:
            info("MQTT invalid structure");
            break;
    };
    info("MQTT thread processing shutting down");


    mosquitto_destroy(mosq);
    info("MQTT shutdown complete");
    mosq = NULL;
}

void mqtt_init() {
    int rc;
    // initialize mosquitto library
    rc = mosquitto_lib_init();
    info("MQTT init -- %s",mosquitto_strerror(rc));

    errno = 0;
    mosq = mosquitto_new(NULL, true, NULL);
    info("MQTT new structure  -- %s",mosquitto_strerror(errno));


    if (mosq) {

        mosquitto_connect_v5_callback_set(mosq, connect_callback);

        //mosquitto_connect_callback_set(mosq, connect_callback);
        //info("MQTT connect callback set -- %s",mosquitto_strerror(rc));
        rc = mosquitto_threaded_set(mosq,1);
        error("MQTT thread model set  -- %s",mosquitto_strerror(rc));

        rc = mosquitto_int_option(mosq, MQTT_PROTOCOL_V5, 0);
        //rc = mosquitto_int_option(mosq, MQTT_PROTOCOL_V311, 0);
        info("MQTT protocol specification rc = %d (%s)", rc, mosquitto_strerror(rc));

        rc = mosquitto_int_option(mosq, MOSQ_OPT_SEND_MAXIMUM, 1);
        info("MQTT in flight messages set to 1  -- %s",mosquitto_strerror(rc));

        rc = mosquitto_reconnect_delay_set(mosq, MQTT_RECONNECT_DELAY, MQTT_MAX_RECONNECT_DELAY, 1);
        //mosquitto_tls_set(mosq, "/etc/netdata/mqtt/ca.crt", NULL, "/etc/netdata/mqtt/server.crt", "/etc/netdata/mqtt/server.key", NULL);
        //rc = mosquitto_connect_bind_async mosq, netdata_configured_hostname, 1883, 60, netdata_configured_hostname, NULL);
        rc = mosquitto_connect_async(mosq, netdata_configured_hostname, 1883, 60);

        info("Connect %s  MQTT status = %d (%s)", netdata_configured_hostname, rc, mosquitto_strerror(rc));


        mosquitto_message_v5_callback_set(mosq, mqtt_message_callback);
        rc = mosquitto_subscribe_v5(mosq, NULL, "netdata/command", 1, 0, NULL);


        // A separate thread will be started to handle the mosquitto connections

        //rc = mosquitto_loop_start(mosq);
        //mosquitto_loop(mosq, 0, 1);
        //info("MQTT thread started -- %s",mosquitto_strerror(rc));
    }
}

