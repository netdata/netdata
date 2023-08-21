// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd_api_mqtt.h"

void mqtt_chart_init(struct File_info *p_file_info){
    p_file_info->chart_meta->chart_data_mqtt = callocz(1, sizeof (struct Chart_data_mqtt));
    chart_data_mqtt_t *chart_data = p_file_info->chart_meta->chart_data_mqtt;
    chart_data->tv.tv_sec = now_realtime_sec(); // initial value shouldn't be 0
    long chart_prio = p_file_info->chart_meta->base_prio;

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
                , RRDSET_TYPE_LINE
        );
        chart_data->dim_lines_rate = rrddim_add(chart_data->st_lines_rate, "records", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    /* Topic - initialise */
    if(p_file_info->parser_config->chart_config & CHART_MQTT_TOPIC){
        chart_data->st_topic = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "topics"
                , NULL
                , "topic"
                , NULL
                , "Topics"
                , "topics"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        ); 
    }

    do_custom_charts_init();
}

void mqtt_chart_update(struct File_info *p_file_info){
    chart_data_mqtt_t *chart_data = p_file_info->chart_meta->chart_data_mqtt;

    if(chart_data->tv.tv_sec != p_file_info->parser_metrics->tv.tv_sec){

        time_t lag_in_sec = p_file_info->parser_metrics->tv.tv_sec - chart_data->tv.tv_sec - 1;

        chart_data->tv = p_file_info->parser_metrics->tv;

        struct timeval tv = {
            .tv_sec = chart_data->tv.tv_sec - lag_in_sec,
            .tv_usec = chart_data->tv.tv_usec
        };

        do_num_of_logs_charts_update(p_file_info, chart_data, tv, lag_in_sec);

        /* Topic - update */
        if(p_file_info->parser_config->chart_config & CHART_MQTT_TOPIC){
            metrics_dict_item_t *it;
            if(likely(chart_data->st_topic->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    dfe_start_read(p_file_info->parser_metrics->mqtt->topic, it){
                        if(it->dim)
                            rrddim_set_by_pointer(  chart_data->st_topic, 
                                                    it->dim, 
                                                    (collected_number) it->num);
                    }
                    dfe_done(it);
                    rrdset_timed_done(chart_data->st_topic, tv, true);
                    tv.tv_sec++;
                }
            }

            dfe_start_read(p_file_info->parser_metrics->mqtt->topic, it){
                if(!it->dim) it->dim = rrddim_add(  chart_data->st_topic, 
                                                    it_dfe.name, NULL, 1, 1, 
                                                    RRD_ALGORITHM_ABSOLUTE);
                rrddim_set_by_pointer(chart_data->st_topic, it->dim, (collected_number) it->num);
            }
            dfe_done(it);
            rrdset_timed_done(  chart_data->st_topic, chart_data->tv, 
                                chart_data->st_topic->counter_done != 0);
        }

        do_custom_charts_update();
    }
}
