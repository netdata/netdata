/** @file plugins_logsmanagement_generic.h
 *  @brief Incudes the structure and function definitions to use generic log charts.
 *
 *  @author Dimitris Pantazis
 */

#ifndef PLUGIN_LOGSMANAGEMENT_GENERIC_H_
#define PLUGIN_LOGSMANAGEMENT_GENERIC_H_

#include "daemon/common.h"
#include "logsmanagement/file_info.h"
#include "logsmanagement/circular_buffer.h"

typedef struct Chart_data_generic chart_data_generic_t;

#include "plugin_logsmanagement.h"

struct Chart_data_generic {

    /* Number of lines */
    RRDSET *st_lines;
    RRDDIM *dim_lines_total;
    RRDDIM *dim_lines_rate;
    collected_number num_lines_total, num_lines_rate;

};

void generic_chart_init(struct File_info *p_file_info, struct Chart_meta *chart_meta);
void generic_chart_collect(struct File_info *p_file_info, struct Chart_meta *chart_meta);
void generic_chart_update(struct File_info *p_file_info, struct Chart_meta *chart_meta);

#endif // PLUGIN_LOGSMANAGEMENT_GENERIC_H_