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

#include "rrd_api_systemd.h" // required for dim_sever_str[]

struct Chart_data_kernel {

    time_t last_update;

    /* Number of collected log records */
    collected_number num_lines;

    /* Kernel metrics - Syslog Severity value */
    collected_number num_sever[SYSLOG_SEVER_ARR_SIZE];

    /* Kernel metrics - Subsystem */
    struct Chart_str cs_subsys;
    // Special case: Subsystem dimension and number are part of Kernel_metrics_t

    /* Kernel metrics - Device */
    struct Chart_str cs_device;
    // Special case: Device dimension and number are part of Kernel_metrics_t
};

void kernel_chart_init(struct File_info *p_file_info);
void kernel_chart_update(struct File_info *p_file_info);

#endif // RRD_API_KERNEL_H_
