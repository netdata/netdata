// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MQTT_H
#define NETDATA_MQTT_H
#include "mosquitto/lib/mosquitto.h"


void _show_mqtt_info();
int _link_event_loop(int timeout);
void _link_shutdown();
int _link_lib_init(char *aclk_hostname, int aclk_port, void (*on_connect)(void *), void (*on_disconnect)(void *));
int _link_subscribe(char *topic, int qos);
int _link_send_message(char *topic, char *message);
const char *_link_strerror(int rc);

extern int aclk_connection_initialized;
int aclk_queue_query(char *token, char *data, char *query, int run_after, int repeat_every, int repeat_count);


#endif //NETDATA_MQTT_H
