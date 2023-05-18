/** @file plugins_logsmanagement_kernel.h
 *  @brief Incudes the structure and function definitions to use kernel log charts.
 *
 *  @author Dimitris Pantazis
 */

#ifndef PLUGIN_LOGSMANAGEMENT_KERNEL_H_
#define PLUGIN_LOGSMANAGEMENT_KERNEL_H_

#include "daemon/common.h"
#include "logsmanagement/file_info.h"
#include "logsmanagement/circular_buffer.h"

typedef struct Chart_data_kernel chart_data_kernel_t;

#include "plugin_logsmanagement.h"

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

void kernel_chart_init(struct File_info *p_file_info, struct Chart_meta *chart_meta);
void kernel_chart_update(struct File_info *p_file_info, struct Chart_meta *chart_meta);

#endif // PLUGIN_LOGSMANAGEMENT_KERNEL_H_
