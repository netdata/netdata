// SPDX-License-Identifier: GPL-3.0-or-later

/** @file  rrd_api_generic.h
 *  @brief Incudes the structure and function definitions for
 *         generic log charts.
 */

#ifndef RRD_API_GENERIC_H_
#define RRD_API_GENERIC_H_

#include "daemon/common.h"

struct File_info;

typedef struct Chart_data_generic chart_data_generic_t;

#include "../file_info.h"
#include "../circular_buffer.h"

#include "rrd_api.h"

struct Chart_data_generic {

    time_t last_update;

    /* Number of collected log records */
    collected_number num_lines;

};

void generic_chart_init(struct File_info *p_file_info);
void generic_chart_update(struct File_info *p_file_info);

#endif // RRD_API_GENERIC_H_
