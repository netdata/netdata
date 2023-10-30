// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd_api_systemd.h"

const char *dim_sever_str[SYSLOG_SEVER_ARR_SIZE] = {
    "0:Emergency",
    "1:Alert",
    "2:Critical",
    "3:Error",
    "4:Warning",
    "5:Notice",
    "6:Informational",
    "7:Debug",
    "uknown"
};

static const char *dim_facil_str[SYSLOG_FACIL_ARR_SIZE] = {
        "0:kernel",
        "1:user-level",
        "2:mail",
        "3:system",
        "4:sec/auth",
        "5:syslog",
        "6:lpd/printer",                                                 
        "7:news/nntp",     
        "8:uucp",  
        "9:time",
        "10:sec/auth",
        "11:ftp",
        "12:ntp",
        "13:logaudit",
        "14:logalert",
        "15:clock",                                                 
        "16:local0",     
        "17:local1", 
        "18:local2", 
        "19:local3", 
        "20:local4", 
        "21:local5", 
        "22:local6",
        "23:local7", 
        "uknown"
};

void systemd_chart_init(struct File_info *p_file_info){
    p_file_info->chart_meta->chart_data_systemd = callocz(1, sizeof (struct Chart_data_systemd));
    chart_data_systemd_t *chart_data = p_file_info->chart_meta->chart_data_systemd;
    chart_data->last_update = now_realtime_sec(); // initial value shouldn't be 0
    long chart_prio = p_file_info->chart_meta->base_prio;

    lgs_mng_do_num_of_logs_charts_init(p_file_info, chart_prio);

    /* Syslog priority value - initialise */
    if(p_file_info->parser_config->chart_config & CHART_SYSLOG_PRIOR){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "priority_values"                 // id
            , "Priority Values"                 // title
            , "priority values"                 // units
            , "priority"                        // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        ); 

        for(int i = 0; i < SYSLOG_PRIOR_ARR_SIZE - 1; i++){
            char dim_id[4];
            snprintfz(dim_id, 4, "%d", i);
            chart_data->dim_prior[i] = strdupz(dim_id);
            lgs_mng_add_dim(chart_data->dim_prior[i], RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        }
        chart_data->dim_prior[SYSLOG_PRIOR_ARR_SIZE - 1] = "uknown";
        lgs_mng_add_dim(chart_data->dim_prior[SYSLOG_PRIOR_ARR_SIZE - 1], 
                        RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);

    }

    /* Syslog severity level (== Systemd priority) - initialise */
    if(p_file_info->parser_config->chart_config & CHART_SYSLOG_SEVER){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "severity_levels"                 // id
            , "Severity Levels"                 // title
            , "severity levels"                 // units
            , "priority"                        // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        for(int i = 0; i < SYSLOG_SEVER_ARR_SIZE; i++)
            lgs_mng_add_dim(dim_sever_str[i], RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
    }
    
    /* Syslog facility level - initialise */
    if(p_file_info->parser_config->chart_config & CHART_SYSLOG_FACIL){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname    // type
            , "facility_levels"                 // id
            , "Facility Levels"                 // title
            , "facility levels"                 // units
            , "priority"                        // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        for(int i = 0; i < SYSLOG_FACIL_ARR_SIZE; i++)
            lgs_mng_add_dim(dim_facil_str[i], RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
    }

    lgs_mng_do_custom_charts_init(p_file_info);
}

void systemd_chart_update(struct File_info *p_file_info){
    chart_data_systemd_t *chart_data = p_file_info->chart_meta->chart_data_systemd;

    if(chart_data->last_update != p_file_info->parser_metrics->last_update){

        time_t lag_in_sec = p_file_info->parser_metrics->last_update - chart_data->last_update - 1;

        lgs_mng_do_num_of_logs_charts_update(p_file_info, lag_in_sec, chart_data);

        /* Syslog priority value - update */
        if(p_file_info->parser_config->chart_config & CHART_SYSLOG_PRIOR){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
            
                lgs_mng_update_chart_begin(p_file_info->chartname, "priority_values");
                for(int idx = 0; idx < SYSLOG_PRIOR_ARR_SIZE; idx++){
                    if(chart_data->num_prior[idx])
                        lgs_mng_update_chart_set(chart_data->dim_prior[idx], chart_data->num_prior[idx]);
                }
                lgs_mng_update_chart_end(sec);
            }

            lgs_mng_update_chart_begin(p_file_info->chartname, "priority_values");
            for(int idx = 0; idx < SYSLOG_PRIOR_ARR_SIZE; idx++){
                if(p_file_info->parser_metrics->systemd->prior[idx]){
                    chart_data->num_prior[idx] = p_file_info->parser_metrics->systemd->prior[idx];
                    lgs_mng_update_chart_set(chart_data->dim_prior[idx], chart_data->num_prior[idx]);
                }
            }
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);
        
        }

        /* Syslog severity level (== Systemd priority) - update chart */
        if(p_file_info->parser_config->chart_config & CHART_SYSLOG_SEVER){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
            
                lgs_mng_update_chart_begin(p_file_info->chartname, "severity_levels");
                for(int idx = 0; idx < SYSLOG_SEVER_ARR_SIZE; idx++){
                    if(chart_data->num_sever[idx])
                        lgs_mng_update_chart_set(dim_sever_str[idx], chart_data->num_sever[idx]);
                }
                lgs_mng_update_chart_end(sec);
            }

            lgs_mng_update_chart_begin(p_file_info->chartname, "severity_levels");
            for(int idx = 0; idx < SYSLOG_SEVER_ARR_SIZE; idx++){
                if(p_file_info->parser_metrics->systemd->sever[idx]){
                    chart_data->num_sever[idx] = p_file_info->parser_metrics->systemd->sever[idx];
                    lgs_mng_update_chart_set(dim_sever_str[idx], chart_data->num_sever[idx]);
                }
            }
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);

        }

        /* Syslog facility value - update chart */
        if(p_file_info->parser_config->chart_config & CHART_SYSLOG_FACIL){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
            
                lgs_mng_update_chart_begin(p_file_info->chartname, "facility_levels");
                for(int idx = 0; idx < SYSLOG_FACIL_ARR_SIZE; idx++){
                    if(chart_data->num_facil[idx])
                        lgs_mng_update_chart_set(dim_facil_str[idx], chart_data->num_facil[idx]);
                }
                lgs_mng_update_chart_end(sec);
            }

            lgs_mng_update_chart_begin(p_file_info->chartname, "facility_levels");
            for(int idx = 0; idx < SYSLOG_FACIL_ARR_SIZE; idx++){
                if(p_file_info->parser_metrics->systemd->facil[idx]){
                    chart_data->num_facil[idx] = p_file_info->parser_metrics->systemd->facil[idx];
                    lgs_mng_update_chart_set(dim_facil_str[idx], chart_data->num_facil[idx]);
                }
            }
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);
        
        }

        lgs_mng_do_custom_charts_update(p_file_info, lag_in_sec);

        chart_data->last_update = p_file_info->parser_metrics->last_update;
    }
}
