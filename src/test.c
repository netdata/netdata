// Copyright (C) 2020 Timotej Šiškovič
// SPDX-License-Identifier: GPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free Software Foundation, version 3.
//
// This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY;
// without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with this program.
// If not, see <https://www.gnu.org/licenses/>.

#include <unistd.h>
#include <stdio.h>
#include <string.h>

#include "mqtt_wss_client.h"

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

    printf("Got Message From Broker Topic \"%s\" QOS %d MSG: \"%s\"\n", topic, qos, cmsg);
}

#define TESTMSG "Hello World!"
int main()
{
    int exit = 0;

    mqtt_wss_client client = mqtt_wss_new("main", mqtt_wss_log_cb, msg_callback, NULL);
    struct mqtt_connect_params params = {
        .clientid = "test",
        .username = "anon",
        .password = "anon"
    };

    while (mqtt_wss_connect(client, "127.0.0.1", 9002, &params)) {
        printf("Connect failed\n");
        sleep(1);
        printf("Attempting Reconnect\n");
    }
    printf("Connection succeeded\n");

    mqtt_wss_subscribe(client, "test", 1);
    mqtt_wss_publish(client, "test", TESTMSG, strlen(TESTMSG), MQTT_WSS_PUB_QOS1);

    while (!exit) {
        if(mqtt_wss_service(client, -1))
            break;
    }

    return 0;
}