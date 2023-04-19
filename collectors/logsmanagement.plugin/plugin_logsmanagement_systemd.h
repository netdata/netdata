/** @file plugins_logsmanagement_systemd.h
 *  @brief Incudes the structure and function definitions to use system log charts.
 *
 *  @author Dimitris Pantazis
 */

#ifndef PLUGIN_LOGSMANAGEMENT_SYSTEMD_H_
#define PLUGIN_LOGSMANAGEMENT_SYSTEMD_H_

#include "daemon/common.h"
#include "logsmanagement/file_info.h"
#include "logsmanagement/circular_buffer.h"

typedef struct Chart_data_systemd chart_data_systemd_t;

#include "plugin_logsmanagement.h"

struct Chart_data_systemd {

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

void systemd_chart_init(struct File_info *p_file_info, struct Chart_meta *chart_meta);
void systemd_chart_collect(struct File_info *p_file_info, struct Chart_meta *chart_meta);
void systemd_chart_update(struct File_info *p_file_info, struct Chart_meta *chart_meta);

#endif // PLUGIN_LOGSMANAGEMENT_SYSTEMD_H_