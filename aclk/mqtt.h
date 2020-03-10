// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MQTT_H
#define NETDATA_MQTT_H

#ifdef ENABLE_ACLK
#include "externaldeps/mosquitto/mosquitto.h"
#endif

void _show_mqtt_info();
int _link_event_loop();
void _link_shutdown();
int mqtt_attempt_connection(char *aclk_hostname, int aclk_port, char *username, char *password);
//int _link_lib_init();
int _mqtt_lib_init();
int _link_subscribe(char *topic, int qos);
int _link_send_message(char *topic, char *message, int *mid);
const char *_link_strerror(int rc);

int aclk_handle_cloud_request(char *);

#endif //NETDATA_MQTT_H
