// SPDX-License-Identifier: GPL-3.0-or-later

/** @file  rrd_api_kernel.h
 *  @brief Incudes the structure and function definitions 
 *         for the kernel log charts.
 */

#ifndef RRD_API_KERNEL_H_
#define RRD_API_KERNEL_H_

#include "daemon/common.h"

struct File_info;

typedef struct Chart_data_kernel chart_data_kernel_t;

#include "../file_info.h"
#include "../circular_buffer.h"

#include "rrd_api.h"

struct Chart_data_kernel {

    struct timeval tv;

    /* Number of collected log records */
    RRDSET *st_lines_total, *st_lines_rate;
    RRDDIM *dim_lines_total, *dim_lines_rate;
    collected_number num_lines;

    /* Kernel metrics - Syslog Severity value */
    RRDSET *st_sever;
    RRDDIM *dim_sever[9];
    collected_number num_sever[9];

    /* Kernel metrics - Subsystem */
    RRDSET *st_subsys;
    // Special case: Subsystem dimension and number are part of Kernel_metrics_t

    /* Kernel metrics - Device */
    RRDSET *st_device;
    // Special case: Device dimension and number are part of Kernel_metrics_t
};

void kernel_chart_init(struct File_info *p_file_info);
void kernel_chart_update(struct File_info *p_file_info);

#endif // RRD_API_KERNEL_H_
