// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MQTT_H
#define NETDATA_MQTT_H

void _show_mqtt_info();
int _link_event_loop(int timeout);
void _link_shutdown();
int _link_lib_init(char *aclk_hostname, int aclk_port, void (*on_connect)(void *), void (*on_disconnect)(void *));
int _link_subscribe(char *topic);
int _link_send_message(char *topic, char *message);
const char *_link_strerror(int rc);

extern int aclk_connection_initialized;
extern int aclk_queue_query(char *token, char *query);


#endif //NETDATA_MQTT_H
