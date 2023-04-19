// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_logsmanagement_generic.h"

void generic_chart_init(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_meta->chart_data_generic = callocz(1, sizeof (struct Chart_data_generic));
    chart_data_generic_t *chart_data = chart_meta->chart_data_generic;
    long chart_prio = chart_meta->base_prio;

    /* Number of collected logs total - initialise */
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_TOTAL){
        chart_data->st_lines_total = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "collected_logs_total"
                , NULL
                , "collected_logs"
                , NULL
                , CHART_TITLE_TOTAL_COLLECTED_LOGS
                , "log records"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_lines_total = rrddim_add(chart_data->st_lines_total, "total records", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    /* Number of collected logs rate - initialise */
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_RATE){
        chart_data->st_lines_rate = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "collected_logs_rate"
                , NULL
                , "collected_logs"
                , NULL
                , CHART_TITLE_RATE_COLLECTED_LOGS
                , "log records"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_lines_rate = rrddim_add(chart_data->st_lines_rate, "records", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

}

void generic_chart_collect(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_data_generic_t *chart_data = chart_meta->chart_data_generic;

    /* Number of collected logs - collect */
    chart_data->num_lines = p_file_info->parser_metrics->num_lines;

}

void generic_chart_update(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    UNUSED(p_file_info);
    chart_data_generic_t *chart_data = chart_meta->chart_data_generic;

    /* Number of collected logs total - update chart */
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_TOTAL){
        rrddim_set_by_pointer(  chart_data->st_lines_total, 
                                chart_data->dim_lines_total, 
                                chart_data->num_lines);
        rrdset_done(chart_data->st_lines_total);
    }

    /* Number of collected logs rate - update chart */
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_RATE){
        rrddim_set_by_pointer(  chart_data->st_lines_rate, 
                                chart_data->dim_lines_rate, 
                                chart_data->num_lines);
        rrdset_done(chart_data->st_lines_rate);
    }

}
