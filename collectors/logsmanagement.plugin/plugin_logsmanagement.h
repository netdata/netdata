/** @file plugin_logsmanagement.h
 *
 *  @author Dimitris Pantazis
 */

#ifndef PLUGIN_LOGSMANAGEMENT_H_
#define PLUGIN_LOGSMANAGEMENT_H_

#include "../../daemon/common.h"
#include "../../logsmanagement/file_info.h"
#include "../../logsmanagement/circular_buffer.h"

struct Chart_meta;

#include "plugin_logsmanagement_generic.h"
#include "plugin_logsmanagement_web_log.h"
#include "plugin_logsmanagement_systemd.h"
#include "plugin_logsmanagement_docker_ev.h"

typedef struct Chart_data_cus {
    /* See Log_parser_cus_metrics_t in parser.h for other 
     * dimensions and collected numbers to add here */
    RRDSET *st_cus;
    int need_rrdset_done;
    RRDDIM *dim_cus_count;
    collected_number num_cus_count;
} Chart_data_cus_t ;

struct Chart_meta {
    enum log_source_t type;
    long base_prio;

    union {
        chart_data_generic_t *chart_data_generic;
        chart_data_web_log_t *chart_data_web_log;
        chart_data_systemd_t *chart_data_systemd;
        chart_data_docker_ev_t *chart_data_docker_ev;
    };

    Chart_data_cus_t **chart_data_cus_arr;

    void (*init)(struct File_info *p_file_info, struct Chart_meta *chart_meta);
    void (*collect)(struct File_info *p_file_info, struct Chart_meta *chart_meta);
    void (*update)(struct File_info *p_file_info, struct Chart_meta *chart_meta);

};

#endif // PLUGIN_LOGSMANAGEMENT_H_