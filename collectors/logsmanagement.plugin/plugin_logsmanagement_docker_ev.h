/** @file plugins_logsmanagement_docker_ev.h
 *  @brief Incudes the structure and function definitions to use docker event log charts.
 *
 *  @author Dimitris Pantazis
 */

#ifndef PLUGIN_LOGSMANAGEMENT_DOCKER_EV_H_
#define PLUGIN_LOGSMANAGEMENT_DOCKER_EV_H_

#include "daemon/common.h"
#include "logsmanagement/file_info.h"
#include "logsmanagement/circular_buffer.h"

typedef struct Chart_data_docker_ev chart_data_docker_ev_t;

#include "plugin_logsmanagement.h"

struct Chart_data_docker_ev {
    char *rrd_type;

    /* Number of collected log records */
    RRDSET *st_lines_total, *st_lines_rate;
    RRDDIM *dim_lines_total, *dim_lines_rate;
    collected_number num_lines;

    /* Docker events metrics - event type */
    RRDSET *st_dock_ev_type;
    RRDDIM *dim_dock_ev_type[NUM_OF_DOCKER_EV_TYPES];
    collected_number num_dock_ev_type[NUM_OF_DOCKER_EV_TYPES];
};

void docker_ev_chart_init(struct File_info *p_file_info, struct Chart_meta *chart_meta);
void docker_ev_chart_collect(struct File_info *p_file_info, struct Chart_meta *chart_meta);
void docker_ev_chart_update(struct File_info *p_file_info, struct Chart_meta *chart_meta);

#endif // PLUGIN_LOGSMANAGEMENT_DOCKER_EV_H_