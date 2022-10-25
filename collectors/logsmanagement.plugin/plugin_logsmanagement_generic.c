// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_logsmanagement_generic.h"

void generic_chart_init(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_meta->chart_data_generic = callocz(1, sizeof (struct Chart_data_generic));
    chart_data_generic_t *chart_data = chart_meta->chart_data_generic;

    /* Number of lines - initialise */
    chart_data->st_lines = rrdset_create_localhost(
            (char *) p_file_info->chart_name
            , "lines parsed"
            , NULL
            , "lines parsed"
            , NULL
            , "Log lines parsed"
            , "lines/s"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_LINES
            , p_file_info->update_every
            , RRDSET_TYPE_AREA
    );
    // TODO: Change dim_lines_total to RRD_ALGORITHM_INCREMENTAL
    chart_data->dim_lines_total = rrddim_add(chart_data->st_lines, "Total lines", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    chart_data->dim_lines_rate = rrddim_add(chart_data->st_lines, "New lines", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

}

void generic_chart_collect(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_data_generic_t *chart_data = chart_meta->chart_data_generic;

    /* Number of lines - collect */
    chart_data->num_lines_total = p_file_info->parser_metrics->num_lines_total;
    chart_data->num_lines_rate += p_file_info->parser_metrics->num_lines_rate;
    p_file_info->parser_metrics->num_lines_rate = 0;

}

void generic_chart_update(struct File_info *p_file_info, struct Chart_meta *chart_meta, int first_update){
    UNUSED(p_file_info);
    chart_data_generic_t *chart_data = chart_meta->chart_data_generic;

    /* Number of lines - update chart first time */
    if(likely(!first_update)) rrdset_next(chart_data->st_lines);
    rrddim_set_by_pointer(  chart_data->st_lines, 
                            chart_data->dim_lines_total, 
                            chart_data->num_lines_total);
    rrddim_set_by_pointer(  chart_data->st_lines, 
                            chart_data->dim_lines_rate, 
                            chart_data->num_lines_rate);
    rrdset_done(chart_data->st_lines);

}

