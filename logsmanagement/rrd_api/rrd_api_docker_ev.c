// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd_api_docker_ev.h"

void docker_ev_chart_init(struct File_info *p_file_info){
    p_file_info->chart_meta->chart_data_docker_ev = callocz(1, sizeof (struct Chart_data_docker_ev));
    chart_data_docker_ev_t *chart_data = p_file_info->chart_meta->chart_data_docker_ev;
    chart_data->tv.tv_sec = now_realtime_sec(); // initial value shouldn't be 0
    long chart_prio = p_file_info->chart_meta->base_prio;

    /* Number of collected logs total - initialise */
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_TOTAL){
        chart_data->st_lines_total = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "collected_logs_total"
                , NULL
                , "collected_logs"
                , "docker_events_logs.collected_logs"
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
                , "docker_events_logs.collected_logs"
                , CHART_TITLE_RATE_COLLECTED_LOGS
                , "log records"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_LINE
        );
        chart_data->dim_lines_rate = rrddim_add(chart_data->st_lines_rate, "records", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    /* Docker events type - initialise */
    if(p_file_info->parser_config->chart_config & CHART_DOCKER_EV_TYPE){
        chart_data->st_dock_ev_type = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "events_type"
                , "events_type"
                , "docker_events"
                , "docker_events_logs.events_type"
                , "Events type"
                , "events types"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        for(int idx = 0; idx < NUM_OF_DOCKER_EV_TYPES; idx++){
            chart_data->dim_dock_ev_type[idx] = rrddim_add( chart_data->st_dock_ev_type, 
                                                            docker_ev_type_string[idx], NULL, 
                                                            1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
    }

    do_custom_charts_init();
}

void docker_ev_chart_update(struct File_info *p_file_info){
    chart_data_docker_ev_t *chart_data = p_file_info->chart_meta->chart_data_docker_ev;

    if(chart_data->tv.tv_sec != p_file_info->parser_metrics->tv.tv_sec){

        time_t lag_in_sec = p_file_info->parser_metrics->tv.tv_sec - chart_data->tv.tv_sec - 1;

        chart_data->tv = p_file_info->parser_metrics->tv;

        struct timeval tv = {
            .tv_sec = chart_data->tv.tv_sec - lag_in_sec,
            .tv_usec = chart_data->tv.tv_usec
        };

        do_num_of_logs_charts_update(p_file_info, chart_data, tv, lag_in_sec);

        /* Docker events type - update */
        if(p_file_info->parser_config->chart_config & CHART_DOCKER_EV_TYPE){
            if(likely(chart_data->st_dock_ev_type->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;                
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    for(int j = 0; j < NUM_OF_DOCKER_EV_TYPES; j++)
                        rrddim_set_by_pointer(  chart_data->st_dock_ev_type, 
                                                chart_data->dim_dock_ev_type[j], 
                                                chart_data->num_dock_ev_type[j]);
                    rrdset_timed_done(  chart_data->st_dock_ev_type, tv, true);
                    tv.tv_sec++;
                }
            }

            for(int j = 0; j < NUM_OF_DOCKER_EV_TYPES; j++){
                chart_data->num_dock_ev_type[j] += p_file_info->parser_metrics->docker_ev->ev_type[j];
                p_file_info->parser_metrics->docker_ev->ev_type[j] = 0;

                rrddim_set_by_pointer(  chart_data->st_dock_ev_type, 
                                        chart_data->dim_dock_ev_type[j], 
                                        chart_data->num_dock_ev_type[j]);
            }
            rrdset_timed_done(  chart_data->st_dock_ev_type, chart_data->tv, 
                                chart_data->st_dock_ev_type->counter_done != 0);
        }

        do_custom_charts_update();
    }
}
