// SPDX-License-Identifier: GPL-3.0-only

#include <unistd.h>
#include <stdio.h>
#include <string.h>
#include <stdlib.h>

#include "mqtt_wss_client.h"

int test_exit = 0;
int port = 0;

void mqtt_wss_log_cb(mqtt_wss_log_type_t log_type, const char* str)
{
    (void)log_type;
    puts(str);
}

#define TEST_MSGLEN_MAX 512
void msg_callback(const char *topic, const void *msg, size_t msglen, int qos)
{
    char cmsg[TEST_MSGLEN_MAX];
    size_t len = (msglen < TEST_MSGLEN_MAX - 1) ? msglen : (TEST_MSGLEN_MAX - 1);
    memcpy(cmsg,
           msg,
           len);
    cmsg[len] = 0;

    if (!strcmp(cmsg, "shutdown"))
        test_exit = 1;

    printf("Got Message From Broker Topic \"%s\" QOS %d MSG: \"%s\"\n", topic, qos, cmsg);
}

#define TESTMSG "Hello World!"
int client_handle(mqtt_wss_client client)
{
    struct mqtt_connect_params params = {
        .clientid = "test",
        .username = "anon",
        .password = "anon",
        .keep_alive = 10
    };

/*    struct mqtt_wss_proxy proxy = {
        .host = "127.0.0.1",
        .port = 3128,
        .type = MQTT_WSS_PROXY_HTTP
    };*/

    while (mqtt_wss_connect(client, "127.0.0.1", port, &params, MQTT_WSS_SSL_ALLOW_SELF_SIGNED, NULL /*&proxy*/)) {
        printf("Connect failed\n");
        sleep(1);
        printf("Attempting Reconnect\n");
    }
    printf("Connection succeeded\n");

    mqtt_wss_subscribe(client, "test", 1);

    while (!test_exit) {
        if(mqtt_wss_service(client, -1) < 0)
            return 1;
    }
    return 0;
}

int main(int argc, char **argv)
{
    if (argc >= 2)
        port = atoi(argv[1]);
    if (!port)
        port = 9002;
    printf("Using port %d\n", port);

    mqtt_wss_client client = mqtt_wss_new("main", mqtt_wss_log_cb, msg_callback, NULL);
    if (!client) {
        printf("Couldn't initialize mqtt_wss\n");
        return 1;
    }
    while (!test_exit) {
        printf("client_handle = %d\n", client_handle(client));
    }
    if (test_exit) {
        mqtt_wss_disconnect(client, 2000);
    }

    mqtt_wss_destroy(client);
    return 0;
}
