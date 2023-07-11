#include "rrd_api_stats.h"

/* NETDATA_CHART_PRIO for Stats_chart_data */
#define NETDATA_CHART_PRIO_CIRC_BUFF_MEM_TOT    NETDATA_CHART_PRIO_LOGS_STATS_BASE + 1
#define NETDATA_CHART_PRIO_CIRC_BUFF_NUM_ITEMS  NETDATA_CHART_PRIO_LOGS_STATS_BASE + 2
#define NETDATA_CHART_PRIO_CIRC_BUFF_MEM_UNC    NETDATA_CHART_PRIO_LOGS_STATS_BASE + 3
#define NETDATA_CHART_PRIO_CIRC_BUFF_MEM_COM    NETDATA_CHART_PRIO_LOGS_STATS_BASE + 4
#define NETDATA_CHART_PRIO_COMPR_RATIO          NETDATA_CHART_PRIO_LOGS_STATS_BASE + 5
#define NETDATA_CHART_PRIO_DISK_USAGE           NETDATA_CHART_PRIO_LOGS_STATS_BASE + 6
#define NETDATA_CHART_PRIO_DB_TIMINGS           NETDATA_CHART_PRIO_LOGS_STATS_BASE + 7

struct Stats_chart_data{
    char *rrd_type;

    RRDSET *st_circ_buff_mem_total;
    RRDDIM **dim_circ_buff_mem_total_arr;
    collected_number *num_circ_buff_mem_total_arr;

    RRDSET *st_circ_buff_num_of_items;
    RRDDIM **dim_circ_buff_num_of_items_arr;
    collected_number *num_circ_buff_num_of_items_arr;

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

    RRDSET *st_db_timings;
    RRDDIM **dim_db_timings_write, **dim_db_timings_rotate;
    collected_number *num_db_timings_write, *num_db_timings_rotate;
};

static struct Stats_chart_data *stats_chart_data;
static struct Chart_meta **chart_data_arr;

void stats_charts_init(void){
    stats_chart_data = callocz(1, sizeof(struct Stats_chart_data));
    stats_chart_data->rrd_type = "netdata";

    /* Circular buffer total memory stats - initialise */
    stats_chart_data->st_circ_buff_mem_total = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "circular_buffers_mem_total_cached"
            , NULL
            , "logsmanagement"
            , NULL
            , "Circular buffers total cached memory"
            , "bytes"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_CIRC_BUFF_MEM_TOT 
            , g_logs_manag_config.update_every
            , RRDSET_TYPE_STACKED
    );
    stats_chart_data->dim_circ_buff_mem_total_arr = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_circ_buff_mem_total_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

     /* Circular buffer number of items - initialise */
    stats_chart_data->st_circ_buff_num_of_items = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "circular_buffers_num_of_items"
            , NULL
            , "logsmanagement"
            , NULL
            , "Circular buffers number of items"
            , "items"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_CIRC_BUFF_NUM_ITEMS
            , g_logs_manag_config.update_every
            , RRDSET_TYPE_LINE
    );
    stats_chart_data->dim_circ_buff_num_of_items_arr = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_circ_buff_num_of_items_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* Circular buffer uncompressed buffered items memory stats - initialise */
    stats_chart_data->st_circ_buff_mem_uncompressed = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "circular_buffers_mem_uncompressed_used"
            , NULL
            , "logsmanagement"
            , NULL
            , "Circular buffers used memory for uncompressed logs"
            , "bytes"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_CIRC_BUFF_MEM_UNC 
            , g_logs_manag_config.update_every
            , RRDSET_TYPE_STACKED
    );
    stats_chart_data->dim_circ_buff_mem_uncompressed_arr = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_circ_buff_mem_uncompressed_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* Circular buffer compressed buffered items memory stats - initialise */
    stats_chart_data->st_circ_buff_mem_compressed = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "circular_buffers_mem_compressed_used"
            , NULL
            , "logsmanagement"
            , NULL
            , "Circular buffers used memory for compressed logs"
            , "bytes"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_CIRC_BUFF_MEM_COM 
            , g_logs_manag_config.update_every
            , RRDSET_TYPE_STACKED
    );
    stats_chart_data->dim_circ_buff_mem_compressed_arr = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_circ_buff_mem_compressed_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* Compression stats - initialise */
    stats_chart_data->st_compression_ratio = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "average_compression_ratio"
            , NULL
            , "logsmanagement"
            , NULL
            , "Average compression ratio"
            , "uncompressed / compressed ratio"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_COMPR_RATIO 
            , g_logs_manag_config.update_every
            , RRDSET_TYPE_LINE
    );
    stats_chart_data->dim_compression_ratio = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_compression_ratio_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* DB disk usage stats - initialise */
    stats_chart_data->st_disk_usage = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "database_disk_usage"
            , NULL
            , "logsmanagement"
            , NULL
            , "Database disk usage"
            , "bytes"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_DISK_USAGE 
            , g_logs_manag_config.update_every
            , RRDSET_TYPE_STACKED
    );
    stats_chart_data->dim_disk_usage = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_disk_usage_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* DB timings - initialise */
    stats_chart_data->st_db_timings = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "database_timings"
            , NULL
            , "logsmanagement"
            , NULL
            , "Database timings"
            , "ns"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_DB_TIMINGS 
            , g_logs_manag_config.update_every
            , RRDSET_TYPE_STACKED
    );
    stats_chart_data->dim_db_timings_write = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->dim_db_timings_rotate = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_db_timings_write = callocz(p_file_infos_arr->count, sizeof(collected_number));
    stats_chart_data->num_db_timings_rotate = callocz(p_file_infos_arr->count, sizeof(collected_number));

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

        /* Circular buffer number of items - add dimensions */
        stats_chart_data->dim_circ_buff_num_of_items_arr[i] = 
            rrddim_add( stats_chart_data->st_circ_buff_num_of_items, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        /* Compression stats - add dimensions */
        stats_chart_data->dim_compression_ratio[i] = 
            rrddim_add( stats_chart_data->st_compression_ratio, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        /* DB disk usage stats - add dimensions */
        stats_chart_data->dim_disk_usage[i] = 
            rrddim_add( stats_chart_data->st_disk_usage, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        /* DB timings - add dimensions */
        char *dim_db_timings_name = mallocz(snprintf(NULL, 0, "%s_rotate", p_file_info->chart_name) + 1);
        sprintf(dim_db_timings_name, "%s_write", p_file_info->chart_name);
        stats_chart_data->dim_db_timings_write[i] = 
            rrddim_add( stats_chart_data->st_db_timings, dim_db_timings_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        sprintf(dim_db_timings_name, "%s_rotate", p_file_info->chart_name);
        stats_chart_data->dim_db_timings_rotate[i] = 
            rrddim_add( stats_chart_data->st_db_timings, dim_db_timings_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        freez(dim_db_timings_name);
    }
}

void stats_charts_update(uv_timer_t *handle){
    UNUSED(handle);
    
    for(int i = 0; i < p_file_infos_arr->count; i++){
        struct File_info *p_file_info = p_file_infos_arr->data[i];
        
        // Check if there is parser configuration to be used for chart generation
        if(!p_file_info->parser_config) continue; 

        /* Circular buffer total memory stats - update */
        stats_chart_data->num_circ_buff_mem_total_arr[i] = 
            __atomic_load_n(&p_file_info->circ_buff->total_cached_mem, __ATOMIC_RELAXED);
        rrddim_set_by_pointer(stats_chart_data->st_circ_buff_mem_total, 
                                stats_chart_data->dim_circ_buff_mem_total_arr[i], 
                                stats_chart_data->num_circ_buff_mem_total_arr[i]);

        /* Circular buffer number of items - update */
        stats_chart_data->num_circ_buff_num_of_items_arr[i] = p_file_info->circ_buff->num_of_items;
        rrddim_set_by_pointer(stats_chart_data->st_circ_buff_num_of_items, 
                                stats_chart_data->dim_circ_buff_num_of_items_arr[i], 
                                stats_chart_data->num_circ_buff_num_of_items_arr[i]);

        /* Circular buffer buffered uncompressed & compressed memory stats - update */
        stats_chart_data->num_circ_buff_mem_uncompressed_arr[i] = 
            __atomic_load_n(&p_file_info->circ_buff->text_size_total, __ATOMIC_RELAXED);
        stats_chart_data->num_circ_buff_mem_compressed_arr[i] = 
            __atomic_load_n(&p_file_info->circ_buff->text_compressed_size_total, __ATOMIC_RELAXED);
        rrddim_set_by_pointer(stats_chart_data->st_circ_buff_mem_uncompressed, 
                                stats_chart_data->dim_circ_buff_mem_uncompressed_arr[i], 
                                stats_chart_data->num_circ_buff_mem_uncompressed_arr[i]);
        rrddim_set_by_pointer(stats_chart_data->st_circ_buff_mem_compressed, 
                                stats_chart_data->dim_circ_buff_mem_compressed_arr[i], 
                                stats_chart_data->num_circ_buff_mem_compressed_arr[i]);

        /* Compression stats - update */
        stats_chart_data->num_compression_ratio_arr[i] = 
            __atomic_load_n(&p_file_info->circ_buff->compression_ratio, __ATOMIC_RELAXED);
        rrddim_set_by_pointer(stats_chart_data->st_compression_ratio, 
                                stats_chart_data->dim_compression_ratio[i], 
                                stats_chart_data->num_compression_ratio_arr[i]);

        /* DB disk usage stats - update */
        stats_chart_data->num_disk_usage_arr[i] = 
            __atomic_load_n(&p_file_info->blob_total_size, __ATOMIC_RELAXED);
        rrddim_set_by_pointer(stats_chart_data->st_disk_usage, 
                                stats_chart_data->dim_disk_usage[i], 
                                stats_chart_data->num_disk_usage_arr[i]);

        /* DB write duration stats - update*/
        stats_chart_data->num_db_timings_write[i] = 
            __atomic_exchange_n(&p_file_info->db_write_duration, 0, __ATOMIC_RELAXED);
        stats_chart_data->num_db_timings_rotate[i] = 
            __atomic_exchange_n(&p_file_info->db_rotate_duration, 0, __ATOMIC_RELAXED);
        rrddim_set_by_pointer(stats_chart_data->st_db_timings, 
                                stats_chart_data->dim_db_timings_write[i], 
                                stats_chart_data->num_db_timings_write[i]);
        rrddim_set_by_pointer(stats_chart_data->st_db_timings, 
                                stats_chart_data->dim_db_timings_rotate[i], 
                                stats_chart_data->num_db_timings_rotate[i]);

    }

    // outside for loop as dimensions updated across different loop iterations, unlike chart_data_arr metrics.
    rrdset_done(stats_chart_data->st_circ_buff_mem_total); 
    rrdset_done(stats_chart_data->st_circ_buff_num_of_items); 
    rrdset_done(stats_chart_data->st_circ_buff_mem_uncompressed);
    rrdset_done(stats_chart_data->st_circ_buff_mem_compressed);
    rrdset_done(stats_chart_data->st_compression_ratio);
    rrdset_done(stats_chart_data->st_disk_usage);
    rrdset_done(stats_chart_data->st_db_timings);
}