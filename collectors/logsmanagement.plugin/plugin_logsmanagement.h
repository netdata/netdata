/** @file plugin_logsmanagement.h
 *
 *  @author Dimitris Pantazis
 */

#ifndef PLUGIN_LOGSMANAGEMENT_H_
#define PLUGIN_LOGSMANAGEMENT_H_

#include "daemon/common.h"
#include "logsmanagement/file_info.h"
#include "logsmanagement/circular_buffer.h"

struct Chart_meta;

#include "plugin_logsmanagement_generic.h"
#include "plugin_logsmanagement_web_log.h"
#include "plugin_logsmanagement_kernel.h"
#include "plugin_logsmanagement_systemd.h"
#include "plugin_logsmanagement_docker_ev.h"

#define CHART_TITLE_TOTAL_COLLECTED_LOGS "Total collected log records"
#define CHART_TITLE_RATE_COLLECTED_LOGS "Rate of collected log records"

typedef struct Chart_data_cus {
    /* See Log_parser_cus_metrics_t in parser.h for other 
     * dimensions and collected numbers to add here */
    RRDSET *st_cus;
    int need_rrdset_done;
    RRDDIM *dim_cus_count;
    collected_number num_cus_count;
} Chart_data_cus_t ;

struct Chart_meta {
    enum log_src_type_t type;
    long base_prio;

    union {
        chart_data_generic_t    *chart_data_generic;
        chart_data_web_log_t    *chart_data_web_log;
        chart_data_kernel_t     *chart_data_kernel;
        chart_data_systemd_t    *chart_data_systemd;
        chart_data_docker_ev_t  *chart_data_docker_ev;
    };

    Chart_data_cus_t **chart_data_cus_arr;

    void (*init)(struct File_info *p_file_info, struct Chart_meta *chart_meta);
    void (*update)(struct File_info *p_file_info, struct Chart_meta *chart_meta);

};

#define do_num_of_logs_charts_update(p_file_info, chart_data, tv, lag_in_sec){\
    /* Number of collected logs total - update previous values */\
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_TOTAL){\
        if(likely(chart_data->st_lines_total->counter_done)){\
            while(tv.tv_sec < chart_data->tv.tv_sec){\
                rrddim_set_by_pointer(  chart_data->st_lines_total,\
                                        chart_data->dim_lines_total,\
                                        chart_data->num_lines);\
                rrdset_timed_done(      chart_data->st_lines_total, tv, true);\
                tv.tv_sec++;\
            }\
        }\
    }\
    /* Number of collected logs rate - update previous values */\
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_RATE){\
        if(likely(chart_data->st_lines_rate->counter_done)){\
            tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;\
            while(tv.tv_sec < chart_data->tv.tv_sec){\
                rrddim_set_by_pointer(  chart_data->st_lines_rate,\
                                        chart_data->dim_lines_rate,\
                                        chart_data->num_lines);\
                rrdset_timed_done(      chart_data->st_lines_rate, tv, true);\
                tv.tv_sec++;\
            }\
        }\
    }\
    chart_data->num_lines = p_file_info->parser_metrics->num_lines;\
    /* Number of collected logs total - update current value */\
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_TOTAL){\
        rrddim_set_by_pointer(  chart_data->st_lines_total,\
                                chart_data->dim_lines_total,\
                                chart_data->num_lines);\
        rrdset_timed_done(      chart_data->st_lines_total, chart_data->tv,\
                                chart_data->st_lines_total->counter_done != 0);\
    }\
    /* Number of collected logs rate - update current value */\
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_RATE){\
        rrddim_set_by_pointer(  chart_data->st_lines_rate,\
                                chart_data->dim_lines_rate,\
                                chart_data->num_lines);\
        rrdset_timed_done(      chart_data->st_lines_rate, chart_data->tv,\
                                chart_data->st_lines_rate->counter_done != 0);\
    }\
}

#define do_custom_charts_update(p_file_info, chart_data, tv, lag_in_sec){\
    tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;\
    while(tv.tv_sec && tv.tv_sec < chart_data->tv.tv_sec){\
        for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){\
            if(likely(chart_meta->chart_data_cus_arr[cus_off]->st_cus->counter_done)){\
                rrddim_set_by_pointer(  chart_meta->chart_data_cus_arr[cus_off]->st_cus,\
                                        chart_meta->chart_data_cus_arr[cus_off]->dim_cus_count,\
                                        chart_meta->chart_data_cus_arr[cus_off]->num_cus_count);\
            }\
        }\
        for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){\
            if(likely(chart_meta->chart_data_cus_arr[cus_off]->st_cus->counter_done)){\
                if(chart_meta->chart_data_cus_arr[cus_off]->need_rrdset_done)\
                    rrdset_timed_done(chart_meta->chart_data_cus_arr[cus_off]->st_cus, tv, true);\
            }\
        }\
        tv.tv_sec++;\
    }\
    for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){\
        chart_meta->chart_data_cus_arr[cus_off]->num_cus_count += \
                p_file_info->parser_metrics->parser_cus[cus_off]->count;\
        p_file_info->parser_metrics->parser_cus[cus_off]->count = 0;\
        rrddim_set_by_pointer(  chart_meta->chart_data_cus_arr[cus_off]->st_cus,\
                                chart_meta->chart_data_cus_arr[cus_off]->dim_cus_count,\
                                chart_meta->chart_data_cus_arr[cus_off]->num_cus_count);\
    }\
    for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){\
        if(chart_meta->chart_data_cus_arr[cus_off]->need_rrdset_done)\
            rrdset_timed_done(  chart_meta->chart_data_cus_arr[cus_off]->st_cus, chart_data->tv, \
                                chart_meta->chart_data_cus_arr[cus_off]->st_cus->counter_done != 0);\
    }\
}

#endif // PLUGIN_LOGSMANAGEMENT_H_
