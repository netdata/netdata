// SPDX-License-Identifier: GPL-3.0-or-later

/** @file plugins_logsmanagement_systemd.h
 *  @brief Incudes the structure and function definitions
 *         for the systemd log charts.
 */

#ifndef RRD_API_SYSTEMD_H_
#define RRD_API_SYSTEMD_H_

#include "daemon/common.h"

struct File_info;

typedef struct Chart_data_systemd chart_data_systemd_t;

#include "../file_info.h"
#include "../circular_buffer.h"

#include "rrd_api.h"

struct Chart_data_systemd {

    struct timeval tv;

    /* Number of collected log records */
    RRDSET *st_lines_total, *st_lines_rate;
    RRDDIM *dim_lines_total, *dim_lines_rate;
    collected_number num_lines;
    
    /* Systemd metrics - Syslog Priority value */
    RRDSET *st_prior;
    RRDDIM *dim_prior[193];
    collected_number num_prior[193];

    /* Systemd metrics - Syslog Severity value */
    RRDSET *st_sever;
    RRDDIM *dim_sever[9];
    collected_number num_sever[9];

    /* Systemd metrics - Syslog Facility value */
    RRDSET *st_facil;
    RRDDIM *dim_facil[25];
    collected_number num_facil[25];
};

void systemd_chart_init(struct File_info *p_file_info);
void systemd_chart_update(struct File_info *p_file_info);

#endif // RRD_API_SYSTEMD_H_
