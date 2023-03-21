// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_logsmanagement_generic.h"

void generic_chart_init(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_meta->chart_data_generic = callocz(1, sizeof (struct Chart_data_generic));
    chart_data_generic_t *chart_data = chart_meta->chart_data_generic;
    long chart_prio = chart_meta->base_prio;

    /* Number of collected logs - initialise */
    chart_data->st_lines = rrdset_create_localhost(
            (char *) p_file_info->chart_name
            , "collected_logs"
            , NULL
            , "collected_logs"
            , NULL
            , "Collected log records"
            , "records"
            , "logsmanagement.plugin"
            , NULL
            , ++chart_prio
            , p_file_info->update_every
            , RRDSET_TYPE_AREA
    );
    chart_data->dim_lines_total = rrddim_add(chart_data->st_lines, "Total records", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    chart_data->dim_lines_rate = rrddim_add(chart_data->st_lines, "New records", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

}

void generic_chart_collect(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_data_generic_t *chart_data = chart_meta->chart_data_generic;

    /* Number of lines - collect */
    chart_data->num_lines_total = p_file_info->parser_metrics->num_lines_total;
    chart_data->num_lines_rate += p_file_info->parser_metrics->num_lines_rate;
    p_file_info->parser_metrics->num_lines_rate = 0;

}

void generic_chart_update(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    UNUSED(p_file_info);
    chart_data_generic_t *chart_data = chart_meta->chart_data_generic;

    /* Number of lines - update chart */
    rrddim_set_by_pointer(  chart_data->st_lines, 
                            chart_data->dim_lines_total, 
                            chart_data->num_lines_total);
    rrddim_set_by_pointer(  chart_data->st_lines, 
                            chart_data->dim_lines_rate, 
                            chart_data->num_lines_rate);
    rrdset_done(chart_data->st_lines);

}
