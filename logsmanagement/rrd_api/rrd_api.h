/** @file rrd_api.h
 */

#ifndef RRD_API_H_
#define RRD_API_H_

#include "daemon/common.h"
#include "../circular_buffer.h"
#include "../helper.h"

struct Chart_meta;
struct Chart_str {
    const char *type;
    const char *id;    
    const char *title;
    const char *units; 
    const char *family;  
    const char *context;
    const char *chart_type; 
    long priority;
    int update_every;
};

#include "rrd_api_generic.h"
#include "rrd_api_web_log.h"
#include "rrd_api_kernel.h"
#include "rrd_api_systemd.h"
#include "rrd_api_docker_ev.h"
#include "rrd_api_mqtt.h"

#define CHART_TITLE_TOTAL_COLLECTED_LOGS    "Total collected log records"
#define CHART_TITLE_RATE_COLLECTED_LOGS     "Rate of collected log records"
#define NETDATA_CHART_PRIO_LOGS_INCR        100     /**< PRIO increment step from one log source to another **/

typedef struct Chart_data_cus {
    char *id;

    struct chart_data_cus_dim {
        char *name;
        collected_number val;
        unsigned long long *p_counter;
    } *dims;

    int dims_size;

    struct Chart_data_cus *next;

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
        chart_data_mqtt_t       *chart_data_mqtt;
    };

    Chart_data_cus_t *chart_data_cus_arr;

    void (*init)(struct File_info *p_file_info);
    void (*update)(struct File_info *p_file_info);

};

static inline struct Chart_str lgs_mng_create_chart(const char *type,  
                                                    const char *id,      
                                                    const char *title,
                                                    const char *units, 
                                                    const char *family,  
                                                    const char *context,
                                                    const char *chart_type, 
                                                    long priority,  
                                                    int update_every){

    struct Chart_str cs = {
        .type           = type,
        .id             = id,
        .title          = title,
        .units          = units,
        .family         = family ? family : "",
        .context        = context ? context : "",
        .chart_type     = chart_type ? chart_type : "",
        .priority       = priority,
        .update_every   = update_every
    };

    printf("CHART '%s.%s' '' '%s' '%s' '%s' '%s' '%s' %ld %d '' '" LOGS_MANAGEMENT_PLUGIN_STR "' ''\n",
        cs.type, 
        cs.id, 
        cs.title, 
        cs.units, 
        cs.family,
        cs.context,
        cs.chart_type,
        cs.priority, 
        cs.update_every
    );

    return cs;
}

static inline void lgs_mng_add_dim( const char *id, 
                            const char *algorithm,
                            collected_number multiplier, 
                            collected_number divisor){

    printf("DIMENSION '%s' '' '%s' %lld %lld\n", id, algorithm, multiplier, divisor);
}

static inline void lgs_mng_add_dim_post_init(   struct Chart_str *cs,
                                        const char *dim_id, 
                                        const char *algorithm,
                                        collected_number multiplier, 
                                        collected_number divisor){

    printf("CHART '%s.%s' '' '%s' '%s' '%s' '%s' '%s' %ld %d '' '" LOGS_MANAGEMENT_PLUGIN_STR "' ''\n",
        cs->type, 
        cs->id, 
        cs->title, 
        cs->units, 
        cs->family,
        cs->context,
        cs->chart_type,
        cs->priority, 
        cs->update_every
    );
    lgs_mng_add_dim(dim_id, algorithm, multiplier, divisor);
}

static inline void lgs_mng_update_chart_begin(const char *type, const char *id){

    printf("BEGIN '%s.%s'\n", type, id);
}

static inline void lgs_mng_update_chart_set(const char *id, collected_number val){
    printf("SET '%s' = %lld\n", id, val);
}

static inline void lgs_mng_update_chart_end(time_t sec){
    printf("END %" PRId64 " 0 1\n", sec);
}

#define lgs_mng_do_num_of_logs_charts_init(p_file_info, chart_prio){                            \
                                                                                                \
    /* Number of collected logs total - initialise */                                           \
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_TOTAL){                  \
        lgs_mng_create_chart(                                                                   \
            (char *) p_file_info->chartname     /* type         */                              \
            , "collected_logs_total"            /* id           */                              \
            , CHART_TITLE_TOTAL_COLLECTED_LOGS  /* title        */                              \
            , "log records"                     /* units        */                              \
            , "collected_logs"                  /* family       */                              \
            , NULL                              /* context      */                              \
            , RRDSET_TYPE_AREA_NAME             /* chart_type   */                              \
            , ++chart_prio                      /* priority     */                              \
            , p_file_info->update_every         /* update_every */                              \
        );                                                                                      \
        lgs_mng_add_dim("total records", RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1);                    \
    }                                                                                           \
                                                                                                \
    /* Number of collected logs rate - initialise */                                            \
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_RATE){                   \
        lgs_mng_create_chart(                                                                   \
            (char *) p_file_info->chartname     /* type         */                              \
            , "collected_logs_rate"             /* id           */                              \
            , CHART_TITLE_RATE_COLLECTED_LOGS   /* title        */                              \
            , "log records"                     /* units        */                              \
            , "collected_logs"                  /* family       */                              \
            , NULL                              /* context      */                              \
            , RRDSET_TYPE_LINE_NAME             /* chart_type   */                              \
            , ++chart_prio                      /* priority     */                              \
            , p_file_info->update_every         /* update_every */                              \
        );                                                                                      \
        lgs_mng_add_dim("records", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);                       \
    }                                                                                           \
                                                                                                \
}                                                                                               \

#define lgs_mng_do_num_of_logs_charts_update(p_file_info, lag_in_sec, chart_data){              \
                                                                                                \
    /* Number of collected logs total - update previous values */                               \
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_TOTAL){                  \
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;            \
                        sec < p_file_info->parser_metrics->last_update;                         \
                        sec++){                                                                 \
                lgs_mng_update_chart_begin(p_file_info->chartname, "collected_logs_total");     \
                lgs_mng_update_chart_set("total records", chart_data->num_lines);               \
                lgs_mng_update_chart_end(sec);                                                  \
            }                                                                                   \
    }                                                                                           \
                                                                                                \
    /* Number of collected logs rate - update previous values */                                \
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_RATE){                   \
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;            \
                        sec < p_file_info->parser_metrics->last_update;                         \
                        sec++){                                                                 \
                lgs_mng_update_chart_begin(p_file_info->chartname, "collected_logs_rate");      \
                lgs_mng_update_chart_set("records", chart_data->num_lines);                     \
                lgs_mng_update_chart_end(sec);                                                  \
            }                                                                                   \
    }                                                                                           \
                                                                                                \
    chart_data->num_lines = p_file_info->parser_metrics->num_lines;                             \
                                                                                                \
    /* Number of collected logs total - update */                                               \
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_TOTAL){                  \
        lgs_mng_update_chart_begin( (char *) p_file_info->chartname, "collected_logs_total");   \
        lgs_mng_update_chart_set("total records", chart_data->num_lines);                       \
        lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);                     \
    }                                                                                           \
                                                                                                \
    /* Number of collected logs rate - update */                                                \
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_RATE){                   \
        lgs_mng_update_chart_begin( (char *) p_file_info->chartname, "collected_logs_rate");    \
        lgs_mng_update_chart_set("records", chart_data->num_lines);                             \
        lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);                     \
    }                                                                                           \
}

#define lgs_mng_do_custom_charts_init(p_file_info) {                                            \
                                                                                                \
    for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){                   \
                                                                                                \
        Chart_data_cus_t *cus;                                                                  \
        Chart_data_cus_t **p_cus = &p_file_info->chart_meta->chart_data_cus_arr;                \
                                                                                                \
        for(cus = p_file_info->chart_meta->chart_data_cus_arr;                                  \
            cus;                                                                                \
            cus = cus->next){                                                                   \
                                                                                                \
            if(!strcmp(cus->id, p_file_info->parser_cus_config[cus_off]->chartname))            \
                break;                                                                          \
                                                                                                \
            p_cus = &(cus->next);                                                               \
        }                                                                                       \
                                                                                                \
        if(!cus){                                                                               \
            cus = callocz(1, sizeof(Chart_data_cus_t));                                         \
            *p_cus = cus;                                                                       \
                                                                                                \
            cus->id = p_file_info->parser_cus_config[cus_off]->chartname;                       \
                                                                                                \
            lgs_mng_create_chart(                                                               \
                (char *) p_file_info->chartname                         /* type         */      \
                , cus->id                                               /* id           */      \
                , cus->id                                               /* title        */      \
                , "matches"                                             /* units        */      \
                , "custom_charts"                                       /* family       */      \
                , NULL                                                  /* context      */      \
                , RRDSET_TYPE_AREA_NAME                                 /* chart_type   */      \
                , p_file_info->chart_meta->base_prio + 1000 + cus_off   /* priority     */      \
                , p_file_info->update_every                             /* update_every */      \
            );                                                                                  \
        }                                                                                       \
                                                                                                \
        cus->dims = reallocz(cus->dims, ++cus->dims_size * sizeof(struct chart_data_cus_dim));  \
        cus->dims[cus->dims_size - 1].name =                                                    \
            p_file_info->parser_cus_config[cus_off]->regex_name;                                \
        cus->dims[cus->dims_size - 1].val = 0;                                                  \
        cus->dims[cus->dims_size - 1].p_counter =                                               \
            &p_file_info->parser_metrics->parser_cus[cus_off]->count;                           \
                                                                                                \
        lgs_mng_add_dim(cus->dims[cus->dims_size - 1].name,                                     \
                        RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);                                  \
                                                                                                \
    }                                                                                           \
}

#define lgs_mng_do_custom_charts_update(p_file_info, lag_in_sec) {                              \
                                                                                                \
    for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;                    \
                sec < p_file_info->parser_metrics->last_update;                                 \
                sec++){                                                                         \
                                                                                                \
        for(Chart_data_cus_t *cus = p_file_info->chart_meta->chart_data_cus_arr;                \
                              cus;                                                              \
                              cus = cus->next){                                                 \
                                                                                                \
            lgs_mng_update_chart_begin(p_file_info->chartname, cus->id);                        \
                                                                                                \
            for(int d_idx = 0; d_idx < cus->dims_size; d_idx++)                                 \
                lgs_mng_update_chart_set(cus->dims[d_idx].name, cus->dims[d_idx].val);          \
                                                                                                \
            lgs_mng_update_chart_end(sec);                                                      \
        }                                                                                       \
                                                                                                \
    }                                                                                           \
                                                                                                \
    for(Chart_data_cus_t *cus = p_file_info->chart_meta->chart_data_cus_arr;                    \
                          cus;                                                                  \
                          cus = cus->next){                                                     \
                                                                                                \
        lgs_mng_update_chart_begin(p_file_info->chartname, cus->id);                            \
                                                                                                \
        for(int d_idx = 0; d_idx < cus->dims_size; d_idx++){                                    \
                                                                                                \
            cus->dims[d_idx].val += *(cus->dims[d_idx].p_counter);                              \
            *(cus->dims[d_idx].p_counter) = 0;                                                  \
                                                                                                \
            lgs_mng_update_chart_set(cus->dims[d_idx].name, cus->dims[d_idx].val);              \
        }                                                                                       \
                                                                                                \
        lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);                     \
    }                                                                                           \
}

#endif // RRD_API_H_
