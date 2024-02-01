// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd_api_docker_ev.h"

void docker_ev_chart_init(struct File_info *p_file_info){
    p_file_info->chart_meta->chart_data_docker_ev = callocz(1, sizeof (struct Chart_data_docker_ev));
    p_file_info->chart_meta->chart_data_docker_ev->last_update = now_realtime_sec(); // initial value shouldn't be 0
    long chart_prio = p_file_info->chart_meta->base_prio;

    lgs_mng_do_num_of_logs_charts_init(p_file_info, chart_prio);

    /* Docker events type - initialise */
    if(p_file_info->parser_config->chart_config & CHART_DOCKER_EV_TYPE){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "events_type"                     // id
            , "Events type"                     // title
            , "events types"                    // units
            , "event_type"                      // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );  
        
        for(int idx = 0; idx < NUM_OF_DOCKER_EV_TYPES; idx++)
            lgs_mng_add_dim(docker_ev_type_string[idx], RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
    }

    /* Docker events actions - initialise */
    if(p_file_info->parser_config->chart_config & CHART_DOCKER_EV_ACTION){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "events_action"                   // id
            , "Events action"                   // title
            , "events actions"                  // units
            , "event_action"                    // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        for(int ev_off = 0; ev_off < NUM_OF_DOCKER_EV_TYPES; ev_off++){
            int act_off = -1;
            while(docker_ev_action_string[ev_off][++act_off] != NULL){

                char dim[50];
                snprintfz(dim, 50, "%s %s", 
                        docker_ev_type_string[ev_off], 
                        docker_ev_action_string[ev_off][act_off]);
                
                lgs_mng_add_dim(dim, RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
            }
        }
    }

    lgs_mng_do_custom_charts_init(p_file_info);
}

void docker_ev_chart_update(struct File_info *p_file_info){
    chart_data_docker_ev_t *chart_data = p_file_info->chart_meta->chart_data_docker_ev;

    if(chart_data->last_update != p_file_info->parser_metrics->last_update){

        time_t lag_in_sec = p_file_info->parser_metrics->last_update - chart_data->last_update - 1;

        lgs_mng_do_num_of_logs_charts_update(p_file_info, lag_in_sec, chart_data);

        /* Docker events type - update */
        if(p_file_info->parser_config->chart_config & CHART_DOCKER_EV_TYPE){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){

                lgs_mng_update_chart_begin(p_file_info->chartname, "events_type");
                for(int idx = 0; idx < NUM_OF_DOCKER_EV_TYPES; idx++)
                    lgs_mng_update_chart_set(docker_ev_type_string[idx], chart_data->num_dock_ev_type[idx]);
                lgs_mng_update_chart_end(sec);
            }

            lgs_mng_update_chart_begin(p_file_info->chartname, "events_type");
            for(int idx = 0; idx < NUM_OF_DOCKER_EV_TYPES; idx++){
                chart_data->num_dock_ev_type[idx] = p_file_info->parser_metrics->docker_ev->ev_type[idx];
                lgs_mng_update_chart_set(docker_ev_type_string[idx], chart_data->num_dock_ev_type[idx]);
            }
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update); 
        }

        /* Docker events action - update */
        if(p_file_info->parser_config->chart_config & CHART_DOCKER_EV_ACTION){
            char dim[50];

            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){

                lgs_mng_update_chart_begin(p_file_info->chartname, "events_action");
                for(int ev_off = 0; ev_off < NUM_OF_DOCKER_EV_TYPES; ev_off++){
                    int act_off = -1;
                    while(docker_ev_action_string[ev_off][++act_off] != NULL){
                        if(chart_data->num_dock_ev_action[ev_off][act_off]){
                            snprintfz(dim, 50, "%s %s", 
                                    docker_ev_type_string[ev_off], 
                                    docker_ev_action_string[ev_off][act_off]);
                            lgs_mng_update_chart_set(dim, chart_data->num_dock_ev_action[ev_off][act_off]);
                        }
                    }
                }
                lgs_mng_update_chart_end(sec);
            }

            lgs_mng_update_chart_begin(p_file_info->chartname, "events_action");
            for(int ev_off = 0; ev_off < NUM_OF_DOCKER_EV_TYPES; ev_off++){
                int act_off = -1;
                while(docker_ev_action_string[ev_off][++act_off] != NULL){
                    chart_data->num_dock_ev_action[ev_off][act_off] = 
                        p_file_info->parser_metrics->docker_ev->ev_action[ev_off][act_off];

                    if(chart_data->num_dock_ev_action[ev_off][act_off]){
                        snprintfz(dim, 50, "%s %s", 
                                docker_ev_type_string[ev_off], 
                                docker_ev_action_string[ev_off][act_off]);
                        lgs_mng_update_chart_set(dim, chart_data->num_dock_ev_action[ev_off][act_off]);
                    }
                }
            }
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);

        }

        lgs_mng_do_custom_charts_update(p_file_info, lag_in_sec);

        chart_data->last_update = p_file_info->parser_metrics->last_update;
    }

}
