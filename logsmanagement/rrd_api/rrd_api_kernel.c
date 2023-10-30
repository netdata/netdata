// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd_api_kernel.h"

void kernel_chart_init(struct File_info *p_file_info){
    p_file_info->chart_meta->chart_data_kernel = callocz(1, sizeof (struct Chart_data_kernel));
    chart_data_kernel_t *chart_data = p_file_info->chart_meta->chart_data_kernel;
    chart_data->last_update = now_realtime_sec(); // initial value shouldn't be 0
    long chart_prio = p_file_info->chart_meta->base_prio;

    lgs_mng_do_num_of_logs_charts_init(p_file_info, chart_prio);

    /* Syslog severity level (== Systemd priority) - initialise */
    if(p_file_info->parser_config->chart_config & CHART_SYSLOG_SEVER){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "severity_levels"                 // id
            , "Severity Levels"                 // title
            , "severity levels"                 // units
            , "severity"                        // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        ); 

        for(int i = 0; i < SYSLOG_SEVER_ARR_SIZE; i++)
            lgs_mng_add_dim(dim_sever_str[i], RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
  
    }

    /* Subsystem - initialise */
    if(p_file_info->parser_config->chart_config & CHART_KMSG_SUBSYSTEM){
        chart_data->cs_subsys = lgs_mng_create_chart(
            (char *) p_file_info->chartname             // type
            , "subsystems"                              // id
            , "Subsystems"                              // title
            , "subsystems"                              // units
            , "subsystem"                               // family
            , NULL                                      // context
            , RRDSET_TYPE_AREA_NAME                     // chart_type
            , ++chart_prio                              // priority
            , p_file_info->update_every                 // update_every
        );
    }

    /* Device - initialise */
    if(p_file_info->parser_config->chart_config & CHART_KMSG_DEVICE){
        chart_data->cs_device = lgs_mng_create_chart(
            (char *) p_file_info->chartname             // type
            , "devices"                                 // id
            , "Devices"                                 // title
            , "devices"                                 // units
            , "device"                                  // family
            , NULL                                      // context
            , RRDSET_TYPE_AREA_NAME                     // chart_type
            , ++chart_prio                              // priority
            , p_file_info->update_every                 // update_every
        ); 
    }

    lgs_mng_do_custom_charts_init(p_file_info);
}

void kernel_chart_update(struct File_info *p_file_info){
    chart_data_kernel_t *chart_data = p_file_info->chart_meta->chart_data_kernel;

    if(chart_data->last_update != p_file_info->parser_metrics->last_update){

        time_t lag_in_sec = p_file_info->parser_metrics->last_update - chart_data->last_update - 1;

        lgs_mng_do_num_of_logs_charts_update(p_file_info, lag_in_sec, chart_data);

        /* Syslog severity level (== Systemd priority) - update */
        if(p_file_info->parser_config->chart_config & CHART_SYSLOG_SEVER){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
            
                lgs_mng_update_chart_begin(p_file_info->chartname, "severity_levels");
                for(int idx = 0; idx < SYSLOG_SEVER_ARR_SIZE; idx++)
                    lgs_mng_update_chart_set(dim_sever_str[idx], chart_data->num_sever[idx]);
                lgs_mng_update_chart_end(sec);
            }

            lgs_mng_update_chart_begin(p_file_info->chartname, "severity_levels");
            for(int idx = 0; idx < SYSLOG_SEVER_ARR_SIZE; idx++){
                chart_data->num_sever[idx] = p_file_info->parser_metrics->kernel->sever[idx];
                lgs_mng_update_chart_set(dim_sever_str[idx], chart_data->num_sever[idx]);
            }
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update); 
        }

        /* Subsystem - update */
        if(p_file_info->parser_config->chart_config & CHART_KMSG_SUBSYSTEM){
            metrics_dict_item_t *it;

            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){

                lgs_mng_update_chart_begin(p_file_info->chartname, "subsystems");
                dfe_start_read(p_file_info->parser_metrics->kernel->subsystem, it){
                    if(it->dim_initialized)
                        lgs_mng_update_chart_set(it_dfe.name, (collected_number) it->num);
                } 
                dfe_done(it);
                lgs_mng_update_chart_end(sec);
            }

            dfe_start_write(p_file_info->parser_metrics->kernel->subsystem, it){
                if(!it->dim_initialized){
                    it->dim_initialized = true;
                    lgs_mng_add_dim_post_init(  &chart_data->cs_subsys, it_dfe.name, 
                                                RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
                }
            }
            dfe_done(it);

            lgs_mng_update_chart_begin(p_file_info->chartname, "subsystems");
            dfe_start_write(p_file_info->parser_metrics->kernel->subsystem, it){
                it->num = it->num_new;
                lgs_mng_update_chart_set(it_dfe.name, (collected_number) it->num);
            }
            dfe_done(it);
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update); 
        }

        /* Device - update */
        if(p_file_info->parser_config->chart_config & CHART_KMSG_DEVICE){
            metrics_dict_item_t *it;

            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){

                lgs_mng_update_chart_begin(p_file_info->chartname, "devices");
                dfe_start_read(p_file_info->parser_metrics->kernel->device, it){
                    if(it->dim_initialized)
                        lgs_mng_update_chart_set(it_dfe.name, (collected_number) it->num);
                } 
                dfe_done(it);
                lgs_mng_update_chart_end(sec);
            }

            dfe_start_write(p_file_info->parser_metrics->kernel->device, it){
                if(!it->dim_initialized){
                    it->dim_initialized = true;
                    lgs_mng_add_dim_post_init(  &chart_data->cs_device, it_dfe.name,
                                                RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
                }
            }
            dfe_done(it);

            lgs_mng_update_chart_begin(p_file_info->chartname, "devices");
            dfe_start_write(p_file_info->parser_metrics->kernel->device, it){
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
