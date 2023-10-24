// SPDX-License-Identifier: GPL-3.0-or-later

/** @file plugins_logsmanagement_docker_ev.h
 *  @brief Incudes the structure and function definitions 
 *         for the docker event log charts.
 */

#ifndef RRD_API_DOCKER_EV_H_
#define RRD_API_DOCKER_EV_H_

#include "daemon/common.h"

struct File_info;

typedef struct Chart_data_docker_ev chart_data_docker_ev_t;

#include "../file_info.h"
#include "../circular_buffer.h"

#include "rrd_api.h"

struct Chart_data_docker_ev {

    time_t last_update;

    /* Number of collected log records */
    collected_number num_lines;

    /* Docker events metrics - event type */
    collected_number num_dock_ev_type[NUM_OF_DOCKER_EV_TYPES];

    /* Docker events metrics - action type */
    collected_number num_dock_ev_action[NUM_OF_DOCKER_EV_TYPES][NUM_OF_CONTAINER_ACTIONS];
};

void docker_ev_chart_init(struct File_info *p_file_info);
void docker_ev_chart_update(struct File_info *p_file_info);

#endif // RRD_API_DOCKER_EV_H_
