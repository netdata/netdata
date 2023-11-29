// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd_api_stats.h"

static const char *const rrd_type = "netdata";

static char **dim_db_timings_write, **dim_db_timings_rotate;

extern bool logsmanagement_should_exit;

static void stats_charts_update(void){

    /* Circular buffer total memory stats - update */
    lgs_mng_update_chart_begin(rrd_type, "circular_buffers_mem_total_cached");
    for(int i = 0; i < p_file_infos_arr->count; i++){
        struct File_info *p_file_info = p_file_infos_arr->data[i];
        if(!p_file_info->parser_config) 
            continue; 

        lgs_mng_update_chart_set(p_file_info->chartname, 
            __atomic_load_n(&p_file_info->circ_buff->total_cached_mem, __ATOMIC_RELAXED));
    }
    lgs_mng_update_chart_end(0);

    /* Circular buffer number of items - update */
    lgs_mng_update_chart_begin(rrd_type, "circular_buffers_num_of_items");
    for(int i = 0; i < p_file_infos_arr->count; i++){
        struct File_info *p_file_info = p_file_infos_arr->data[i];
        if(!p_file_info->parser_config) 
            continue;
        
        lgs_mng_update_chart_set(p_file_info->chartname, p_file_info->circ_buff->num_of_items);
    }
    lgs_mng_update_chart_end(0);

    /* Circular buffer uncompressed buffered items memory stats - update */
    lgs_mng_update_chart_begin(rrd_type, "circular_buffers_mem_uncompressed_used");
    for(int i = 0; i < p_file_infos_arr->count; i++){
        struct File_info *p_file_info = p_file_infos_arr->data[i];
        if(!p_file_info->parser_config) 
            continue;
        
        lgs_mng_update_chart_set(p_file_info->chartname, 
            __atomic_load_n(&p_file_info->circ_buff->text_size_total, __ATOMIC_RELAXED));
    }
    lgs_mng_update_chart_end(0);

    /* Circular buffer compressed buffered items memory stats - update */
    lgs_mng_update_chart_begin(rrd_type, "circular_buffers_mem_compressed_used");
    for(int i = 0; i < p_file_infos_arr->count; i++){
        struct File_info *p_file_info = p_file_infos_arr->data[i];
        if(!p_file_info->parser_config) 
            continue;
        
        lgs_mng_update_chart_set(p_file_info->chartname, 
            __atomic_load_n(&p_file_info->circ_buff->text_compressed_size_total, __ATOMIC_RELAXED));
    }
    lgs_mng_update_chart_end(0);

    /* Compression stats - update */
    lgs_mng_update_chart_begin(rrd_type, "average_compression_ratio");
    for(int i = 0; i < p_file_infos_arr->count; i++){
        struct File_info *p_file_info = p_file_infos_arr->data[i];
        if(!p_file_info->parser_config) 
            continue;
        
        lgs_mng_update_chart_set(p_file_info->chartname, 
            __atomic_load_n(&p_file_info->circ_buff->compression_ratio, __ATOMIC_RELAXED));
    }
    lgs_mng_update_chart_end(0);

    /* DB disk usage stats - update */
    lgs_mng_update_chart_begin(rrd_type, "database_disk_usage");
    for(int i = 0; i < p_file_infos_arr->count; i++){
        struct File_info *p_file_info = p_file_infos_arr->data[i];
        if(!p_file_info->parser_config) 
            continue;
        
        lgs_mng_update_chart_set(p_file_info->chartname, 
            __atomic_load_n(&p_file_info->blob_total_size, __ATOMIC_RELAXED));
    }
    lgs_mng_update_chart_end(0);

    /* DB timings - update */
    lgs_mng_update_chart_begin(rrd_type, "database_timings");
    for(int i = 0; i < p_file_infos_arr->count; i++){
        struct File_info *p_file_info = p_file_infos_arr->data[i];
        if(!p_file_info->parser_config) 
            continue;
        
        lgs_mng_update_chart_set(dim_db_timings_write[i], 
            __atomic_exchange_n(&p_file_info->db_write_duration, 0, __ATOMIC_RELAXED));
        
        lgs_mng_update_chart_set(dim_db_timings_rotate[i], 
            __atomic_exchange_n(&p_file_info->db_rotate_duration, 0, __ATOMIC_RELAXED));
    }
    lgs_mng_update_chart_end(0);

    /* Query CPU time per byte (user) - update */
    lgs_mng_update_chart_begin(rrd_type, "query_cpu_time_per_MiB_user");
    for(int i = 0; i < p_file_infos_arr->count; i++){
        struct File_info *p_file_info = p_file_infos_arr->data[i];
        if(!p_file_info->parser_config) 
            continue;
        
        lgs_mng_update_chart_set(p_file_info->chartname, 
            __atomic_load_n(&p_file_info->cpu_time_per_mib.user, __ATOMIC_RELAXED));
    }
    lgs_mng_update_chart_end(0);

    /* Query CPU time per byte (user) - update */
    lgs_mng_update_chart_begin(rrd_type, "query_cpu_time_per_MiB_sys");
    for(int i = 0; i < p_file_infos_arr->count; i++){
        struct File_info *p_file_info = p_file_infos_arr->data[i];
        if(!p_file_info->parser_config) 
            continue;
        
        lgs_mng_update_chart_set(p_file_info->chartname, 
            __atomic_load_n(&p_file_info->cpu_time_per_mib.sys, __ATOMIC_RELAXED));
    }
    lgs_mng_update_chart_end(0);

}

void stats_charts_init(void *arg){

    netdata_mutex_t *p_stdout_mut = (netdata_mutex_t *) arg;

    netdata_mutex_lock(p_stdout_mut);

    int chart_prio = NETDATA_CHART_PRIO_LOGS_STATS_BASE;

    /* Circular buffer total memory stats - initialise */
    lgs_mng_create_chart(
        rrd_type                                    // type
        , "circular_buffers_mem_total_cached"       // id
        , "Circular buffers total cached memory"    // title
        , "bytes"                                   // units
        , "logsmanagement"                          // family
        , NULL                                      // context
        , RRDSET_TYPE_STACKED_NAME                  // chart_type
        , ++chart_prio                              // priority
        , g_logs_manag_config.update_every          // update_every
    );
    for(int i = 0; i < p_file_infos_arr->count; i++)
        lgs_mng_add_dim(p_file_infos_arr->data[i]->chartname, RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1);

    /* Circular buffer number of items - initialise */
    lgs_mng_create_chart(
        rrd_type                                // type
        , "circular_buffers_num_of_items"       // id
        , "Circular buffers number of items"    // title
        , "items"                               // units
        , "logsmanagement"                      // family
        , NULL                                  // context
        , RRDSET_TYPE_LINE_NAME                 // chart_type
        , ++chart_prio                          // priority
        , g_logs_manag_config.update_every      // update_every
    );
    for(int i = 0; i < p_file_infos_arr->count; i++)
        lgs_mng_add_dim(p_file_infos_arr->data[i]->chartname, RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1);
        
    /* Circular buffer uncompressed buffered items memory stats - initialise */
    lgs_mng_create_chart(
        rrd_type                                                // type
        , "circular_buffers_mem_uncompressed_used"              // id
        , "Circular buffers used memory for uncompressed logs"  // title
        , "bytes"                                               // units
        , "logsmanagement"                                      // family
        , NULL                                                  // context
        , RRDSET_TYPE_STACKED_NAME                              // chart_type
        , ++chart_prio                                          // priority
        , g_logs_manag_config.update_every                      // update_every
    );
    for(int i = 0; i < p_file_infos_arr->count; i++)
        lgs_mng_add_dim(p_file_infos_arr->data[i]->chartname, RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1);

    /* Circular buffer compressed buffered items memory stats - initialise */
    lgs_mng_create_chart(
        rrd_type                                                // type
        , "circular_buffers_mem_compressed_used"                // id
        , "Circular buffers used memory for compressed logs"    // title
        , "bytes"                                               // units
        , "logsmanagement"                                      // family
        , NULL                                                  // context
        , RRDSET_TYPE_STACKED_NAME                              // chart_type
        , ++chart_prio                                          // priority
        , g_logs_manag_config.update_every                      // update_every
    );
    for(int i = 0; i < p_file_infos_arr->count; i++)
        lgs_mng_add_dim(p_file_infos_arr->data[i]->chartname, RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1);

    /* Compression stats - initialise */
    lgs_mng_create_chart(
        rrd_type                                // type
        , "average_compression_ratio"           // id
        , "Average compression ratio"           // title
        , "uncompressed / compressed ratio"     // units
        , "logsmanagement"                      // family
        , NULL                                  // context
        , RRDSET_TYPE_LINE_NAME                 // chart_type
        , ++chart_prio                          // priority
        , g_logs_manag_config.update_every      // update_every
    );
    for(int i = 0; i < p_file_infos_arr->count; i++)
        lgs_mng_add_dim(p_file_infos_arr->data[i]->chartname, RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1);

    /* DB disk usage stats - initialise */
    lgs_mng_create_chart(
        rrd_type                            // type
        , "database_disk_usage"             // id
        , "Database disk usage"             // title
        , "bytes"                           // units
        , "logsmanagement"                  // family
        , NULL                              // context
        , RRDSET_TYPE_STACKED_NAME          // chart_type
        , ++chart_prio                      // priority
        , g_logs_manag_config.update_every  // update_every
    );
    for(int i = 0; i < p_file_infos_arr->count; i++)
        lgs_mng_add_dim(p_file_infos_arr->data[i]->chartname, RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1);

    /* DB timings - initialise */
    lgs_mng_create_chart(
        rrd_type                            // type
        , "database_timings"                // id
        , "Database timings"                // title
        , "ns"                              // units
        , "logsmanagement"                  // family
        , NULL                              // context
        , RRDSET_TYPE_STACKED_NAME          // chart_type
        , ++chart_prio                      // priority
        , g_logs_manag_config.update_every  // update_every
    );
    for(int i = 0; i < p_file_infos_arr->count; i++){
        struct File_info *p_file_info = p_file_infos_arr->data[i];

        dim_db_timings_write = reallocz(dim_db_timings_write, (i + 1) * sizeof(char *));
        dim_db_timings_rotate = reallocz(dim_db_timings_rotate, (i + 1) * sizeof(char *));

        dim_db_timings_write[i] = mallocz(snprintf(NULL, 0, "%s_write", p_file_info->chartname) + 1);
        sprintf(dim_db_timings_write[i], "%s_write", p_file_info->chartname);
        lgs_mng_add_dim(dim_db_timings_write[i], RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1);

        dim_db_timings_rotate[i] = mallocz(snprintf(NULL, 0, "%s_rotate", p_file_info->chartname) + 1);
        sprintf(dim_db_timings_rotate[i], "%s_rotate", p_file_info->chartname);
        lgs_mng_add_dim(dim_db_timings_rotate[i], RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1);
    }

    /* Query CPU time per byte (user) - initialise */
    lgs_mng_create_chart(
        rrd_type                                    // type
        , "query_cpu_time_per_MiB_user"             // id
        , "CPU user time per MiB of query results"  // title
        , "usec/MiB"                                // units
        , "logsmanagement"                          // family
        , NULL                                      // context
        , RRDSET_TYPE_STACKED_NAME                  // chart_type
        , ++chart_prio                              // priority
        , g_logs_manag_config.update_every          // update_every
    );
    for(int i = 0; i < p_file_infos_arr->count; i++)
        lgs_mng_add_dim(p_file_infos_arr->data[i]->chartname, RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);

    /* Query CPU time per byte (system) - initialise */
    lgs_mng_create_chart(
        rrd_type                                        // type
        , "query_cpu_time_per_MiB_sys"                  // id
        , "CPU system time per MiB of query results"    // title
        , "usec/MiB"                                    // units
        , "logsmanagement"                              // family
        , NULL                                          // context
        , RRDSET_TYPE_STACKED_NAME                      // chart_type
        , ++chart_prio                                  // priority
        , g_logs_manag_config.update_every              // update_every
    );
    for(int i = 0; i < p_file_infos_arr->count; i++)
        lgs_mng_add_dim(p_file_infos_arr->data[i]->chartname, RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);

    netdata_mutex_unlock(p_stdout_mut);


    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step_ut = g_logs_manag_config.update_every * USEC_PER_SEC;

    while (0 == __atomic_load_n(&logsmanagement_should_exit, __ATOMIC_RELAXED)) {
        heartbeat_next(&hb, step_ut);

        netdata_mutex_lock(p_stdout_mut);
        stats_charts_update();
        fflush(stdout);
        netdata_mutex_unlock(p_stdout_mut);
    }

    collector_info("[stats charts]: thread exiting...");
}

