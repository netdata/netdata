// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_logsmanagement_docker_ev.h"

void docker_ev_chart_init(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_meta->chart_data_docker_ev = callocz(1, sizeof (struct Chart_data_docker_ev));
    chart_data_docker_ev_t *chart_data = chart_meta->chart_data_docker_ev;

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

    /* Docker events type - initialise */
    if(p_file_info->parser_config->chart_config & CHART_DOCKER_EV_TYPE){
        chart_data->st_dock_ev_type = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "events type"
                , NULL
                , "docker events"
                , NULL
                , "Events type"
                , "events types/s"
                , "logsmanagement.plugin"
                , NULL
                , NETDATA_CHART_PRIO_DOCKER_EV_TYPE
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        for(int idx = 0; idx < NUM_OF_DOCKER_EV_TYPES; idx++){
            chart_data->dim_dock_ev_type[idx] = rrddim_add( chart_data->st_dock_ev_type, 
                                                            docker_ev_type_string[idx], NULL, 
                                                            1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
    }

}

void docker_ev_chart_collect(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_data_docker_ev_t *chart_data = chart_meta->chart_data_docker_ev;

    /* Number of lines - collect */
    chart_data->num_lines_total = p_file_info->parser_metrics->num_lines_total;
    chart_data->num_lines_rate += p_file_info->parser_metrics->num_lines_rate;
    p_file_info->parser_metrics->num_lines_rate = 0;

    /* Docker events type - collect */
    if(p_file_info->parser_config->chart_config & CHART_DOCKER_EV_TYPE){
        for(int j = 0; j < NUM_OF_DOCKER_EV_TYPES; j++){
            chart_data->num_dock_ev_type[j] += p_file_info->parser_metrics->docker_ev->ev_type[j];
            p_file_info->parser_metrics->docker_ev->ev_type[j] = 0;
        }
    }
}

void docker_ev_chart_update(struct File_info *p_file_info, struct Chart_meta *chart_meta, int first_update){
    chart_data_docker_ev_t *chart_data = chart_meta->chart_data_docker_ev;

    /* Number of lines - update chart first time */
    if(likely(!first_update)) rrdset_next(chart_data->st_lines);
    rrddim_set_by_pointer(  chart_data->st_lines, 
                            chart_data->dim_lines_total, 
                            chart_data->num_lines_total);
    rrddim_set_by_pointer(  chart_data->st_lines, 
                            chart_data->dim_lines_rate, 
                            chart_data->num_lines_rate);
    rrdset_done(chart_data->st_lines);

    /* Docker events type - update chart */
    if(p_file_info->parser_config->chart_config & CHART_DOCKER_EV_TYPE){
        if(likely(!first_update)) rrdset_next(chart_data->st_dock_ev_type);
        for(int j = 0; j < NUM_OF_DOCKER_EV_TYPES; j++){
            rrddim_set_by_pointer(  chart_data->st_dock_ev_type, 
                                    chart_data->dim_dock_ev_type[j], 
                                    chart_data->num_dock_ev_type[j]);
        }
        rrdset_done(chart_data->st_dock_ev_type);
    }
}

