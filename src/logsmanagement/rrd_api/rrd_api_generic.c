// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd_api_generic.h"

void generic_chart_init(struct File_info *p_file_info){
    p_file_info->chart_meta->chart_data_generic = callocz(1, sizeof (struct Chart_data_generic));
    p_file_info->chart_meta->chart_data_generic->last_update = now_realtime_sec(); // initial value shouldn't be 0
    long chart_prio = p_file_info->chart_meta->base_prio;

    lgs_mng_do_num_of_logs_charts_init(p_file_info, chart_prio);

    lgs_mng_do_custom_charts_init(p_file_info);
}

void generic_chart_update(struct File_info *p_file_info){
    chart_data_generic_t *chart_data = p_file_info->chart_meta->chart_data_generic;

    if(chart_data->last_update != p_file_info->parser_metrics->last_update){

        time_t lag_in_sec = p_file_info->parser_metrics->last_update - chart_data->last_update - 1;

        lgs_mng_do_num_of_logs_charts_update(p_file_info, lag_in_sec, chart_data);

        lgs_mng_do_custom_charts_update(p_file_info, lag_in_sec);
                                                       
        chart_data->last_update = p_file_info->parser_metrics->last_update;
    }
}
