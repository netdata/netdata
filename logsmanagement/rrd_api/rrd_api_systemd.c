// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd_api_systemd.h"

void systemd_chart_init(struct File_info *p_file_info){
    p_file_info->chart_meta->chart_data_systemd = callocz(1, sizeof (struct Chart_data_systemd));
    chart_data_systemd_t *chart_data = p_file_info->chart_meta->chart_data_systemd;
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

    /* Number of collected logs total - initialise */
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

    /* Syslog priority value - initialise */
    if(p_file_info->parser_config->chart_config & CHART_SYSLOG_PRIOR){
        chart_data->st_prior = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "priority_value"
                , NULL
                , "priority"
                , NULL
                , "Priority Value"
                , "priority values"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_prior[SYSLOG_PRIOR_ARR_SIZE - 1] = rrddim_add(chart_data->st_prior, "Unknown", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    /* Syslog severity level (== Systemd priority) - initialise */
    if(p_file_info->parser_config->chart_config & CHART_SYSLOG_SEVER){
        chart_data->st_sever = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "severity_levels"
                , NULL
                , "priority"
                , NULL
                , "Severity Levels"
                , "severity levels"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_sever[0] = rrddim_add(chart_data->st_sever, "0:Emergency", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_sever[1] = rrddim_add(chart_data->st_sever, "1:Alert",     NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_sever[2] = rrddim_add(chart_data->st_sever, "2:Critical",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_sever[3] = rrddim_add(chart_data->st_sever, "3:Error",     NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_sever[4] = rrddim_add(chart_data->st_sever, "4:Warning",   NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_sever[5] = rrddim_add(chart_data->st_sever, "5:Notice",    NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_sever[6] = rrddim_add(chart_data->st_sever, "6:Informational", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);                                                 
        chart_data->dim_sever[7] = rrddim_add(chart_data->st_sever, "7:Debug",     NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);     
        chart_data->dim_sever[8] = rrddim_add(chart_data->st_sever, "Unknown",     NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);  
    }
    
    /* Syslog facility level - initialise */
    if(p_file_info->parser_config->chart_config & CHART_SYSLOG_FACIL){
        chart_data->st_facil = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "facility_levels"
                , NULL
                , "priority"
                , NULL
                , "Facility Levels"
                , "facility levels"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_facil[0] = rrddim_add(chart_data->st_facil, "0:kernel", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[1] = rrddim_add(chart_data->st_facil, "1:user-level", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[2] = rrddim_add(chart_data->st_facil, "2:mail", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[3] = rrddim_add(chart_data->st_facil, "3:system", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[4] = rrddim_add(chart_data->st_facil, "4:sec/auth", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[5] = rrddim_add(chart_data->st_facil, "5:syslog", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[6] = rrddim_add(chart_data->st_facil, "6:lpd/printer", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);                                                 
        chart_data->dim_facil[7] = rrddim_add(chart_data->st_facil, "7:news/nntp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);     
        chart_data->dim_facil[8] = rrddim_add(chart_data->st_facil, "8:uucp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);  
        chart_data->dim_facil[9] = rrddim_add(chart_data->st_facil, "9:time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[10] = rrddim_add(chart_data->st_facil, "10:sec/auth", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[11] = rrddim_add(chart_data->st_facil, "11:ftp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[12] = rrddim_add(chart_data->st_facil, "12:ntp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[13] = rrddim_add(chart_data->st_facil, "13:logaudit", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[14] = rrddim_add(chart_data->st_facil, "14:logalert", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[15] = rrddim_add(chart_data->st_facil, "15:clock", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);                                                 
        chart_data->dim_facil[16] = rrddim_add(chart_data->st_facil, "16:local0", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);     
        chart_data->dim_facil[17] = rrddim_add(chart_data->st_facil, "17:local1", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL); 
        chart_data->dim_facil[18] = rrddim_add(chart_data->st_facil, "18:local2", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL); 
        chart_data->dim_facil[19] = rrddim_add(chart_data->st_facil, "19:local3", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL); 
        chart_data->dim_facil[20] = rrddim_add(chart_data->st_facil, "20:local4", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL); 
        chart_data->dim_facil[21] = rrddim_add(chart_data->st_facil, "21:local5", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL); 
        chart_data->dim_facil[22] = rrddim_add(chart_data->st_facil, "22:local6", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_facil[23] = rrddim_add(chart_data->st_facil, "23:local7", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL); 
        chart_data->dim_facil[24] = rrddim_add(chart_data->st_facil, "uknown", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);   
    }

    do_custom_charts_init();
}

void systemd_chart_update(struct File_info *p_file_info){
    chart_data_systemd_t *chart_data = p_file_info->chart_meta->chart_data_systemd;

    if(chart_data->tv.tv_sec != p_file_info->parser_metrics->tv.tv_sec){

        time_t lag_in_sec = p_file_info->parser_metrics->tv.tv_sec - chart_data->tv.tv_sec - 1;

        chart_data->tv = p_file_info->parser_metrics->tv;

        struct timeval tv = {
            .tv_sec = chart_data->tv.tv_sec - lag_in_sec,
            .tv_usec = chart_data->tv.tv_usec
        };

        do_num_of_logs_charts_update(p_file_info, chart_data, tv, lag_in_sec);

        /* Syslog priority value - update */
        if(p_file_info->parser_config->chart_config & CHART_SYSLOG_PRIOR){
            if(likely(chart_data->st_prior->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    for(int j = 0; j < SYSLOG_PRIOR_ARR_SIZE - 1; j++){
                        if(chart_data->dim_prior[j])
                            rrddim_set_by_pointer(  chart_data->st_prior, 
                                                    chart_data->dim_prior[j], 
                                                    chart_data->num_prior[j]);
                    }
                    rrddim_set_by_pointer(  chart_data->st_prior, 
                                            chart_data->dim_prior[SYSLOG_PRIOR_ARR_SIZE - 1], // "Unknown"
                                            chart_data->num_prior[SYSLOG_PRIOR_ARR_SIZE - 1]);
                    rrdset_timed_done(  chart_data->st_prior, tv, true);
                    tv.tv_sec++;
                }
            }

            for(int j = 0; j < SYSLOG_PRIOR_ARR_SIZE; j++){
                chart_data->num_prior[j] += p_file_info->parser_metrics->systemd->prior[j];
                p_file_info->parser_metrics->systemd->prior[j] = 0;

                if(unlikely(!chart_data->dim_prior[j] && chart_data->num_prior[j])){
                    char dim_prior_name[4];
                    snprintfz(dim_prior_name, 4, "%d", j);
                    chart_data->dim_prior[j] = rrddim_add(  chart_data->st_prior, 
                                                            dim_prior_name, NULL, 1, 1, 
                                                            RRD_ALGORITHM_INCREMENTAL);
                }
                if(chart_data->dim_prior[j]) rrddim_set_by_pointer( chart_data->st_prior, 
                                                                    chart_data->dim_prior[j], 
                                                                    chart_data->num_prior[j]);
            }
            rrdset_timed_done(  chart_data->st_prior, chart_data->tv, 
                                chart_data->st_prior->counter_done != 0);
        
        }

        /* Syslog severity level (== Systemd priority) - update chart */
        if(p_file_info->parser_config->chart_config & CHART_SYSLOG_SEVER){
            if(likely(chart_data->st_sever->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;

                while(tv.tv_sec < chart_data->tv.tv_sec){
                    for(int j = 0; j < SYSLOG_SEVER_ARR_SIZE; j++)
                        rrddim_set_by_pointer(  chart_data->st_sever, 
                                                chart_data->dim_sever[j], 
                                                chart_data->num_sever[j]);
                    rrdset_timed_done(  chart_data->st_sever, tv, true);
                    tv.tv_sec++;
                }
            }

            for(int j = 0; j < SYSLOG_SEVER_ARR_SIZE; j++){
                chart_data->num_sever[j] += p_file_info->parser_metrics->systemd->sever[j];
                p_file_info->parser_metrics->systemd->sever[j] = 0;

                rrddim_set_by_pointer(  chart_data->st_sever, 
                                        chart_data->dim_sever[j], 
                                        chart_data->num_sever[j]);
            }
            rrdset_timed_done(  chart_data->st_sever, chart_data->tv, 
                                chart_data->st_sever->counter_done != 0);
        }

        /* Syslog facility value - update chart */
        if(p_file_info->parser_config->chart_config & CHART_SYSLOG_FACIL){
            if(likely(chart_data->st_facil->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;

                while(tv.tv_sec < chart_data->tv.tv_sec){
                    for(int j = 0; j < SYSLOG_FACIL_ARR_SIZE; j++)
                        rrddim_set_by_pointer(  chart_data->st_facil, 
                                                chart_data->dim_facil[j], 
                                                chart_data->num_facil[j]);
                    rrdset_timed_done(  chart_data->st_facil, tv, true);
                    tv.tv_sec++;
                }
            }

            for(int j = 0; j < SYSLOG_FACIL_ARR_SIZE; j++){
                chart_data->num_facil[j] += p_file_info->parser_metrics->systemd->facil[j];
                p_file_info->parser_metrics->systemd->facil[j] = 0;

                rrddim_set_by_pointer(  chart_data->st_facil, 
                                        chart_data->dim_facil[j], 
                                        chart_data->num_facil[j]);
            }
            rrdset_timed_done(  chart_data->st_facil, chart_data->tv, 
                                chart_data->st_facil->counter_done != 0);
        }

        do_custom_charts_update();
    }
}
