// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_logsmanagement_kernel.h"

void kernel_chart_init(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_meta->chart_data_kernel = callocz(1, sizeof (struct Chart_data_kernel));
    chart_data_kernel_t *chart_data = chart_meta->chart_data_kernel;
    long chart_prio = chart_meta->base_prio;

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
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_lines_rate = rrddim_add(chart_data->st_lines_rate, "records", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    /* Syslog severity level (== Systemd priority) - initialise */
    if(p_file_info->parser_config->chart_config & CHART_SYSLOG_SEVER){
        chart_data->st_sever = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "severity_levels"
                , NULL
                , "severity"
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

    /* Subsystem - initialise */
    if(p_file_info->parser_config->chart_config & CHART_KMSG_SUBSYSTEM){
        chart_data->st_subsys = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "subsystems"
                , NULL
                , "subsystem"
                , NULL
                , "Subsystems"
                , "subsystems"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        ); 
    }

    /* Device - initialise */
    if(p_file_info->parser_config->chart_config & CHART_KMSG_DEVICE){
        chart_data->st_device = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "devices"
                , NULL
                , "device"
                , NULL
                , "Devices"
                , "devices"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        ); 
    }
}

void kernel_chart_collect(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_data_kernel_t *chart_data = chart_meta->chart_data_kernel;

    /* Number of collected logs - collect */
    chart_data->num_lines = p_file_info->parser_metrics->num_lines;

    /* Syslog severity level (== Systemd priority) - collect */
    if(p_file_info->parser_config->chart_config & CHART_SYSLOG_SEVER){
        for(int j = 0; j < SYSLOG_SEVER_ARR_SIZE; j++){
            chart_data->num_sever[j] += p_file_info->parser_metrics->kernel->sever[j];
            p_file_info->parser_metrics->kernel->sever[j] = 0;
        }
    }

    /* Subsystem - collect */
    /* No collection step for subsystem as dictionaries use r/w locks that 
     * allow update direct update of values. */

    /* Device - collect */
    /* No collection step for device as dictionaries use r/w locks that 
     * allow update direct update of values. */
}

void kernel_chart_update(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_data_kernel_t *chart_data = chart_meta->chart_data_kernel;

    /* Number of collected logs total - update chart */
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_TOTAL){
        rrddim_set_by_pointer(  chart_data->st_lines_total, 
                                chart_data->dim_lines_total, 
                                chart_data->num_lines);
        rrdset_done(chart_data->st_lines_total);
    }

    /* Number of collected logs rate - update chart */
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_RATE){
        rrddim_set_by_pointer(  chart_data->st_lines_rate, 
                                chart_data->dim_lines_rate, 
                                chart_data->num_lines);
        rrdset_done(chart_data->st_lines_rate);
    }

    /* Syslog severity level (== Systemd priority) - update chart */
    if(p_file_info->parser_config->chart_config & CHART_SYSLOG_SEVER){
        for(int j = 0; j < SYSLOG_SEVER_ARR_SIZE; j++){
            rrddim_set_by_pointer(  chart_data->st_sever, 
                                    chart_data->dim_sever[j], 
                                    chart_data->num_sever[j]);
        }
        rrdset_done(chart_data->st_sever);
    }

    /* Subsystem - update chart */
    if(p_file_info->parser_config->chart_config & CHART_KMSG_SUBSYSTEM){
        Kernel_metrics_dict_item_t *it;
        dfe_start_read(p_file_info->parser_metrics->kernel->subsystem, it){
            if(!it->dim) it->dim = rrddim_add(chart_data->st_subsys, it_dfe.name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrddim_set_by_pointer(chart_data->st_subsys, it->dim, (collected_number) it->num);
        }
        dfe_done(it);
    }
    rrdset_done(chart_data->st_subsys);

    /* Device - update chart */
    if(p_file_info->parser_config->chart_config & CHART_KMSG_DEVICE){
        Kernel_metrics_dict_item_t *it;
        dfe_start_read(p_file_info->parser_metrics->kernel->device, it){
            if(!it->dim) it->dim = rrddim_add(chart_data->st_device, it_dfe.name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrddim_set_by_pointer(chart_data->st_device, it->dim, (collected_number) it->num);
        }
        dfe_done(it);
    }
    rrdset_done(chart_data->st_device);

}
