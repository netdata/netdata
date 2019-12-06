// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MQTT_H

#define NETDATA_MQTT_H

#include <mosquitto.h>

#define MQTT_RECONNECT_DELAY 1
#define MQTT_MAX_RECONNECT_DELAY 120

//#include "../daemon/common.h"

void *mqtt_main(void *ptr);

#define NETDATA_MQTT_HOOK \
    { \
        .name = "MQTT", \
        .config_section = NULL, \
        .config_name = NULL, \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = mqtt_main \
    },

extern mqtt_send(char *topic, char *message);

extern void mqtt_init();
//extern struct mosquitto *mosq;


#endif //NETDATA_MQTT_H
