// SPDX-License-Identifier: GPL-3.0-or-later

/** @file  rrd_api_mqtt.h
 *  @brief Incudes the structure and function definitions 
 *         for the mqtt log charts.
 */

#ifndef RRD_API_MQTT_H_
#define RRD_API_MQTT_H_

#include "daemon/common.h"

struct File_info;

typedef struct Chart_data_mqtt chart_data_mqtt_t;

#include "../file_info.h"
#include "../circular_buffer.h"

#include "rrd_api.h"

struct Chart_data_mqtt {

    time_t last_update;

    /* Number of collected log records */
    collected_number num_lines;

    /* MQTT metrics - Topic */
    struct Chart_str cs_topic;
    // Special case: Topic dimension and number are part of Mqtt_metrics_t
};

void mqtt_chart_init(struct File_info *p_file_info);
void mqtt_chart_update(struct File_info *p_file_info);

#endif // RRD_API_MQTT_H_
