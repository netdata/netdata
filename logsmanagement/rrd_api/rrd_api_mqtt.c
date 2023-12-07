// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd_api_mqtt.h"

void mqtt_chart_init(struct File_info *p_file_info){
    p_file_info->chart_meta->chart_data_mqtt = callocz(1, sizeof (struct Chart_data_mqtt));
    chart_data_mqtt_t *chart_data = p_file_info->chart_meta->chart_data_mqtt;
    chart_data->last_update = now_realtime_sec(); // initial value shouldn't be 0
    long chart_prio = p_file_info->chart_meta->base_prio;

    lgs_mng_do_num_of_logs_charts_init(p_file_info, chart_prio);

    /* Topic - initialise */
    if(p_file_info->parser_config->chart_config & CHART_MQTT_TOPIC){
        chart_data->cs_topic = lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "topics"                          // id
            , "Topics"                          // title
            , "topics"                          // units
            , "topic"                           // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        ); 
    }

    lgs_mng_do_custom_charts_init(p_file_info);
}

void mqtt_chart_update(struct File_info *p_file_info){
    chart_data_mqtt_t *chart_data = p_file_info->chart_meta->chart_data_mqtt;

    if(chart_data->last_update != p_file_info->parser_metrics->last_update){

        time_t lag_in_sec = p_file_info->parser_metrics->last_update - chart_data->last_update - 1;

        lgs_mng_do_num_of_logs_charts_update(p_file_info, lag_in_sec, chart_data);

        /* Topic - update */
        if(p_file_info->parser_config->chart_config & CHART_MQTT_TOPIC){
            metrics_dict_item_t *it;

            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){

                lgs_mng_update_chart_begin(p_file_info->chartname, "topics");
                dfe_start_read(p_file_info->parser_metrics->mqtt->topic, it){
                    if(it->dim_initialized)
                        lgs_mng_update_chart_set(it_dfe.name, (collected_number) it->num);
                } 
                dfe_done(it);
                lgs_mng_update_chart_end(sec);
            }

            dfe_start_write(p_file_info->parser_metrics->mqtt->topic, it){
                if(!it->dim_initialized){
                    it->dim_initialized = true;
                    lgs_mng_add_dim_post_init(  &chart_data->cs_topic, it_dfe.name, 
                                                RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
                }
            }
            dfe_done(it);

            lgs_mng_update_chart_begin(p_file_info->chartname, "topics");
            dfe_start_write(p_file_info->parser_metrics->mqtt->topic, it){
                it->num = it->num_new;
                lgs_mng_update_chart_set(it_dfe.name, (collected_number) it->num);
            }
            dfe_done(it);
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update); 
        }

        lgs_mng_do_custom_charts_update(p_file_info, lag_in_sec);

        chart_data->last_update = p_file_info->parser_metrics->last_update;
    }
}
