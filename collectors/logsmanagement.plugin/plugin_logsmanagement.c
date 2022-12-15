// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_logsmanagement.h"

/* NETDATA_CHART_PRIO for Stats_chart_data */
#define NETDATA_CHART_PRIO_CIRC_BUFF_MEM_TOT    NETDATA_CHART_PRIO_LOGS_STATS_BASE + 1
#define NETDATA_CHART_PRIO_CIRC_BUFF_MEM_UNC    NETDATA_CHART_PRIO_LOGS_STATS_BASE + 2
#define NETDATA_CHART_PRIO_CIRC_BUFF_MEM_COM    NETDATA_CHART_PRIO_LOGS_STATS_BASE + 3
#define NETDATA_CHART_PRIO_COMPR_RATIO          NETDATA_CHART_PRIO_LOGS_STATS_BASE + 4
#define NETDATA_CHART_PRIO_DISK_USAGE           NETDATA_CHART_PRIO_LOGS_STATS_BASE + 5

#define NETDATA_CHART_PRIO_LOGS_INCR            100  /**< PRIO increment step from one log source to another **/

#define WORKER_JOB_COLLECT 0
#define WORKER_JOB_UPDATE  1

static struct Chart_meta chart_types[] = {
    {.type = GENERIC,       .init = generic_chart_init,   .collect = generic_chart_collect,   .update = generic_chart_update},
    {.type = FLB_GENERIC,   .init = generic_chart_init,   .collect = generic_chart_collect,   .update = generic_chart_update},
    {.type = WEB_LOG,       .init = web_log_chart_init,   .collect = web_log_chart_collect,   .update = web_log_chart_update},
    {.type = FLB_WEB_LOG,   .init = web_log_chart_init,   .collect = web_log_chart_collect,   .update = web_log_chart_update},
    {.type = FLB_SYSTEMD,   .init = systemd_chart_init,   .collect = systemd_chart_collect,   .update = systemd_chart_update},
    {.type = FLB_DOCKER_EV, .init = docker_ev_chart_init, .collect = docker_ev_chart_collect, .update = docker_ev_chart_update},
    {.type = FLB_SYSLOG,   .init = systemd_chart_init,   .collect = systemd_chart_collect,   .update = systemd_chart_update}
};

struct Stats_chart_data{
    char *rrd_type;

    RRDSET *st_circ_buff_mem_total;
    RRDDIM **dim_circ_buff_mem_total_arr;
    collected_number *num_circ_buff_mem_total_arr;

    RRDSET *st_circ_buff_mem_uncompressed;
    RRDDIM **dim_circ_buff_mem_uncompressed_arr;
    collected_number *num_circ_buff_mem_uncompressed_arr;

    RRDSET *st_circ_buff_mem_compressed;
    RRDDIM **dim_circ_buff_mem_compressed_arr;
    collected_number *num_circ_buff_mem_compressed_arr;

    RRDSET *st_compression_ratio;
    RRDDIM **dim_compression_ratio;
    collected_number *num_compression_ratio_arr;

    RRDSET *st_disk_usage;
    RRDDIM **dim_disk_usage;
    collected_number *num_disk_usage_arr;
};

static struct Stats_chart_data *stats_chart_data;
static struct Chart_meta **chart_data_arr;

static void logsmanagement_plugin_main_cleanup(void *ptr) {
    worker_unregister();
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}


void *logsmanagement_plugin_main(void *ptr){
    worker_register("LOGSMANAGPLG");
    worker_register_job_name(WORKER_JOB_COLLECT, "collection");
    worker_register_job_name(WORKER_JOB_UPDATE, "update");

	netdata_thread_cleanup_push(logsmanagement_plugin_main_cleanup, ptr);

    /* wait for p_file_infos_arr initialisation */
    for(int retries = 20; !p_file_infos_arr_ready; retries--){
        sleep_usec(500 * USEC_PER_MS);
        if(retries == 0) goto cleanup;
    }

    stats_chart_data = callocz(1, sizeof(struct Stats_chart_data));
    stats_chart_data->rrd_type = "netdata";

    /* Circular buffer total memory stats - initialise */
    stats_chart_data->st_circ_buff_mem_total = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "circular buffer memory total"
            , NULL
            , "logsmanagement.plugin"
            , NULL
            , "Circular buffers total memory"
            , "bytes"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_CIRC_BUFF_MEM_TOT 
            , g_logs_manag_update_every
            , RRDSET_TYPE_AREA
    );
    stats_chart_data->dim_circ_buff_mem_total_arr = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_circ_buff_mem_total_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* Circular buffer uncompressed buffered items memory stats - initialise */
    stats_chart_data->st_circ_buff_mem_uncompressed = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "circular buffer uncompressed buffered total"
            , NULL
            , "logsmanagement.plugin"
            , NULL
            , "Circular buffers uncompressed buffered total"
            , "bytes"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_CIRC_BUFF_MEM_UNC 
            , g_logs_manag_update_every
            , RRDSET_TYPE_AREA
    );
    stats_chart_data->dim_circ_buff_mem_uncompressed_arr = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_circ_buff_mem_uncompressed_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* Circular buffer compressed buffered items memory stats - initialise */
    stats_chart_data->st_circ_buff_mem_compressed = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "circular buffer compressed buffered total"
            , NULL
            , "logsmanagement.plugin"
            , NULL
            , "Circular buffers compressed buffered total"
            , "bytes"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_CIRC_BUFF_MEM_COM 
            , g_logs_manag_update_every
            , RRDSET_TYPE_AREA
    );
    stats_chart_data->dim_circ_buff_mem_compressed_arr = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_circ_buff_mem_compressed_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* Compression stats - initialise */
    stats_chart_data->st_compression_ratio = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "average compression ratio"
            , NULL
            , "logsmanagement.plugin"
            , NULL
            , "Average compression ratio"
            , "uncompressed / compressed ratio"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_COMPR_RATIO 
            , g_logs_manag_update_every
            , RRDSET_TYPE_AREA
    );
    stats_chart_data->dim_compression_ratio = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_compression_ratio_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* DB disk usage stats - initialise */
    stats_chart_data->st_disk_usage = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "database disk usage"
            , NULL
            , "logsmanagement.plugin"
            , NULL
            , "Database disk usage"
            , "bytes"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_DISK_USAGE 
            , g_logs_manag_update_every
            , RRDSET_TYPE_AREA
    );
    stats_chart_data->dim_disk_usage = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_disk_usage_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));


    chart_data_arr = callocz(p_file_infos_arr->count, sizeof(struct Chart_meta *));

    for(int i = 0; i < p_file_infos_arr->count; i++){

        struct File_info *p_file_info = p_file_infos_arr->data[i];
        if(!p_file_info->parser_config){ // Check if there is parser configuration to be used for chart generation
            chart_data_arr[i] = NULL;
            continue; 
        } 

        /* Circular buffer memory stats - add dimensions */
        stats_chart_data->dim_circ_buff_mem_total_arr[i] = 
            rrddim_add( stats_chart_data->st_circ_buff_mem_total, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        stats_chart_data->dim_circ_buff_mem_uncompressed_arr[i] = 
            rrddim_add( stats_chart_data->st_circ_buff_mem_uncompressed, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        stats_chart_data->dim_circ_buff_mem_compressed_arr[i] = 
            rrddim_add( stats_chart_data->st_circ_buff_mem_compressed, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        /* Compression stats - add dimensions */
        stats_chart_data->dim_compression_ratio[i] = 
            rrddim_add( stats_chart_data->st_compression_ratio, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        /* DB disk usage stats - add dimensions */
        stats_chart_data->dim_disk_usage[i] = 
            rrddim_add( stats_chart_data->st_disk_usage, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        chart_data_arr[i] = callocz(1, sizeof(struct Chart_meta));
        memcpy(chart_data_arr[i], &chart_types[p_file_info->log_type], sizeof(struct Chart_meta));
        chart_data_arr[i]->base_prio = NETDATA_CHART_PRIO_LOGS_BASE + (i + 1) * NETDATA_CHART_PRIO_LOGS_INCR;
        chart_data_arr[i]->init(p_file_info, chart_data_arr[i]);

        /* Custom charts - initialise */
        for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){
            chart_data_arr[i]->chart_data_cus_arr = reallocz( chart_data_arr[i]->chart_data_cus_arr, 
                                                              (cus_off + 1) * sizeof(Chart_data_cus_t *));
            chart_data_arr[i]->chart_data_cus_arr[cus_off] = callocz(1, sizeof(Chart_data_cus_t));

            RRDSET *st_cus = rrdset_find_active_bytype_localhost( p_file_info->chart_name, 
                                                                  p_file_info->parser_cus_config[cus_off]->chart_name);
            if(st_cus) chart_data_arr[i]->chart_data_cus_arr[cus_off]->st_cus = st_cus;
            else {
                chart_data_arr[i]->chart_data_cus_arr[cus_off]->st_cus = rrdset_create_localhost(
                        (char *) p_file_info->chart_name
                        , p_file_info->parser_cus_config[cus_off]->chart_name
                        , NULL
                        , "regex charts"
                        , NULL
                        , p_file_info->parser_cus_config[cus_off]->chart_name
                        , "matches/s"
                        , "logsmanagement.plugin"
                        , NULL
                        , chart_data_arr[i]->base_prio + 1000 + cus_off
                        , p_file_info->update_every
                        , RRDSET_TYPE_AREA
                );
                // rrdset_done() need to be run for this
                chart_data_arr[i]->chart_data_cus_arr[cus_off]->need_rrdset_done = 1; 
            }  
            chart_data_arr[i]->chart_data_cus_arr[cus_off]->dim_cus_count = 
                rrddim_add( chart_data_arr[i]->chart_data_cus_arr[cus_off]->st_cus, 
                            p_file_info->parser_cus_config[cus_off]->regex_name, NULL, 
                            1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

    }


    usec_t step = g_logs_manag_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);

	while(!netdata_exit){

        worker_is_idle();
        (void) heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;

        for(int i = 0; i < p_file_infos_arr->count; i++){
            struct File_info *p_file_info = p_file_infos_arr->data[i];
            
            // Check if there is parser configuration to be used for chart generation
            if(!p_file_info->parser_config) continue; 

            worker_is_busy(WORKER_JOB_COLLECT);

            /* Circular buffer total memory stats - collect (no need to be within p_file_info->parser_metrics_mut lock) */
            stats_chart_data->num_circ_buff_mem_total_arr[i] = __atomic_load_n(&p_file_info->circ_buff->total_cached_mem, __ATOMIC_RELAXED);

            /* Circular buffer buffered uncompressed & compressed memory stats - collect */
            stats_chart_data->num_circ_buff_mem_uncompressed_arr[i] = __atomic_load_n(&p_file_info->circ_buff->text_size_total, __ATOMIC_RELAXED);
            stats_chart_data->num_circ_buff_mem_compressed_arr[i] = __atomic_load_n(&p_file_info->circ_buff->text_compressed_size_total, __ATOMIC_RELAXED);

            /* Compression stats - collect */
            stats_chart_data->num_compression_ratio_arr[i] = __atomic_load_n(&p_file_info->circ_buff->compression_ratio, __ATOMIC_RELAXED);

            /* DB disk usage stats - collect */
            stats_chart_data->num_disk_usage_arr[i] = __atomic_load_n(&p_file_info->blob_total_size, __ATOMIC_RELAXED);


            uv_mutex_lock(p_file_info->parser_metrics_mut);

            chart_data_arr[i]->collect(p_file_info, chart_data_arr[i]);

            /* Custom charts - collect */
            for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){
                chart_data_arr[i]->chart_data_cus_arr[cus_off]->num_cus_count += 
                    p_file_info->parser_metrics->parser_cus[cus_off]->count;
                p_file_info->parser_metrics->parser_cus[cus_off]->count = 0;
            }

            uv_mutex_unlock(p_file_info->parser_metrics_mut);


            if(unlikely(netdata_exit)) break;

            worker_is_busy(WORKER_JOB_UPDATE);

            /* Circular buffer total memory stats - update chart */
            rrddim_set_by_pointer(stats_chart_data->st_circ_buff_mem_total, 
                                  stats_chart_data->dim_circ_buff_mem_total_arr[i], 
                                  stats_chart_data->num_circ_buff_mem_total_arr[i]);
            
            /* Circular buffer buffered compressed & uncompressed memory stats - update chart */
            rrddim_set_by_pointer(stats_chart_data->st_circ_buff_mem_uncompressed, 
                                  stats_chart_data->dim_circ_buff_mem_uncompressed_arr[i], 
                                  stats_chart_data->num_circ_buff_mem_uncompressed_arr[i]);
            rrddim_set_by_pointer(stats_chart_data->st_circ_buff_mem_compressed, 
                                  stats_chart_data->dim_circ_buff_mem_compressed_arr[i], 
                                  stats_chart_data->num_circ_buff_mem_compressed_arr[i]);

            /* Compression stats - update chart */
            rrddim_set_by_pointer(stats_chart_data->st_compression_ratio, 
                                  stats_chart_data->dim_compression_ratio[i], 
                                  stats_chart_data->num_compression_ratio_arr[i]);

            /* DB disk usage stats - update chart */
            rrddim_set_by_pointer(stats_chart_data->st_disk_usage, 
                                  stats_chart_data->dim_disk_usage[i], 
                                  stats_chart_data->num_disk_usage_arr[i]);

            chart_data_arr[i]->update(p_file_info, chart_data_arr[i]);

            /* Custom charts - update chart */
            for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){
                rrddim_set_by_pointer(  chart_data_arr[i]->chart_data_cus_arr[cus_off]->st_cus,
                                        chart_data_arr[i]->chart_data_cus_arr[cus_off]->dim_cus_count,
                                        chart_data_arr[i]->chart_data_cus_arr[cus_off]->num_cus_count);
            }
            for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){
                if(chart_data_arr[i]->chart_data_cus_arr[cus_off]->need_rrdset_done){
                    rrdset_done(chart_data_arr[i]->chart_data_cus_arr[cus_off]->st_cus);
                }
            }

        }

        // outside for loop as dimensions updated across different loop iterations, unlike chart_data_arr metrics.
        rrdset_done(stats_chart_data->st_circ_buff_mem_total); 
        rrdset_done(stats_chart_data->st_circ_buff_mem_uncompressed);
        rrdset_done(stats_chart_data->st_circ_buff_mem_compressed);
        rrdset_done(stats_chart_data->st_compression_ratio);
        rrdset_done(stats_chart_data->st_disk_usage);
    }

cleanup:
    netdata_thread_cleanup_pop(1);
    return NULL;
}