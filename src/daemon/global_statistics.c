// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

#define GLOBAL_STATS_RESET_WEB_USEC_MAX 0x01

#define WORKER_JOB_GLOBAL             0
#define WORKER_JOB_REGISTRY           1
#define WORKER_JOB_WORKERS            2
#define WORKER_JOB_DBENGINE           3
#define WORKER_JOB_HEARTBEAT          4
#define WORKER_JOB_STRINGS            5
#define WORKER_JOB_DICTIONARIES       6
#define WORKER_JOB_MALLOC_TRACE       7
#define WORKER_JOB_SQLITE3            8

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 9
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 9
#endif

bool global_statistics_enabled = true;

struct netdata_buffers_statistics netdata_buffers_statistics = {};

static size_t dbengine_total_memory = 0;
size_t rrddim_db_memory_size = 0;

static struct global_statistics {
    uint16_t connected_clients;

    uint64_t web_requests;
    uint64_t web_usec;
    uint64_t web_usec_max;
    uint64_t bytes_received;
    uint64_t bytes_sent;
    uint64_t content_size;
    uint64_t compressed_content_size;

    uint64_t web_client_count;

    uint64_t api_data_queries_made;
    uint64_t api_data_db_points_read;
    uint64_t api_data_result_points_generated;

    uint64_t api_weights_queries_made;
    uint64_t api_weights_db_points_read;
    uint64_t api_weights_result_points_generated;

    uint64_t api_badges_queries_made;
    uint64_t api_badges_db_points_read;
    uint64_t api_badges_result_points_generated;

    uint64_t health_queries_made;
    uint64_t health_db_points_read;
    uint64_t health_result_points_generated;

    uint64_t ml_queries_made;
    uint64_t ml_db_points_read;
    uint64_t ml_result_points_generated;
    uint64_t ml_models_consulted;

    uint64_t exporters_queries_made;
    uint64_t exporters_db_points_read;

    uint64_t backfill_queries_made;
    uint64_t backfill_db_points_read;

    uint64_t tier0_hot_gorilla_buffers;

    uint64_t tier0_disk_compressed_bytes;
    uint64_t tier0_disk_uncompressed_bytes;

    uint64_t db_points_stored_per_tier[RRD_STORAGE_TIERS];

} global_statistics = {
        .connected_clients = 0,
        .web_requests = 0,
        .web_usec = 0,
        .bytes_received = 0,
        .bytes_sent = 0,
        .content_size = 0,
        .compressed_content_size = 0,
        .web_client_count = 1,

        .api_data_queries_made = 0,
        .api_data_db_points_read = 0,
        .api_data_result_points_generated = 0,

        .tier0_hot_gorilla_buffers = 0,
        .tier0_disk_compressed_bytes = 0,
        .tier0_disk_uncompressed_bytes = 0,
};

void global_statistics_rrdset_done_chart_collection_completed(size_t *points_read_per_tier_array) {
    for(size_t tier = 0; tier < storage_tiers ;tier++) {
        __atomic_fetch_add(&global_statistics.db_points_stored_per_tier[tier], points_read_per_tier_array[tier], __ATOMIC_RELAXED);
        points_read_per_tier_array[tier] = 0;
    }
}

void global_statistics_ml_query_completed(size_t points_read) {
    __atomic_fetch_add(&global_statistics.ml_queries_made, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&global_statistics.ml_db_points_read, points_read, __ATOMIC_RELAXED);
}

void global_statistics_ml_models_consulted(size_t models_consulted) {
    __atomic_fetch_add(&global_statistics.ml_models_consulted, models_consulted, __ATOMIC_RELAXED);
}

void global_statistics_exporters_query_completed(size_t points_read) {
    __atomic_fetch_add(&global_statistics.exporters_queries_made, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&global_statistics.exporters_db_points_read, points_read, __ATOMIC_RELAXED);
}

void global_statistics_backfill_query_completed(size_t points_read) {
    __atomic_fetch_add(&global_statistics.backfill_queries_made, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&global_statistics.backfill_db_points_read, points_read, __ATOMIC_RELAXED);
}

void global_statistics_gorilla_buffer_add_hot() {
    __atomic_fetch_add(&global_statistics.tier0_hot_gorilla_buffers, 1, __ATOMIC_RELAXED);
}

void global_statistics_tier0_disk_compressed_bytes(uint32_t size) {
    __atomic_fetch_add(&global_statistics.tier0_disk_compressed_bytes, size, __ATOMIC_RELAXED);
}

void global_statistics_tier0_disk_uncompressed_bytes(uint32_t size) {
    __atomic_fetch_add(&global_statistics.tier0_disk_uncompressed_bytes, size, __ATOMIC_RELAXED);
}

void global_statistics_rrdr_query_completed(size_t queries, uint64_t db_points_read, uint64_t result_points_generated, QUERY_SOURCE query_source) {
    switch(query_source) {
        case QUERY_SOURCE_API_DATA:
            __atomic_fetch_add(&global_statistics.api_data_queries_made, queries, __ATOMIC_RELAXED);
            __atomic_fetch_add(&global_statistics.api_data_db_points_read, db_points_read, __ATOMIC_RELAXED);
            __atomic_fetch_add(&global_statistics.api_data_result_points_generated, result_points_generated, __ATOMIC_RELAXED);
            break;

        case QUERY_SOURCE_ML:
            __atomic_fetch_add(&global_statistics.ml_queries_made, queries, __ATOMIC_RELAXED);
            __atomic_fetch_add(&global_statistics.ml_db_points_read, db_points_read, __ATOMIC_RELAXED);
            __atomic_fetch_add(&global_statistics.ml_result_points_generated, result_points_generated, __ATOMIC_RELAXED);
            break;

        case QUERY_SOURCE_API_WEIGHTS:
            __atomic_fetch_add(&global_statistics.api_weights_queries_made, queries, __ATOMIC_RELAXED);
            __atomic_fetch_add(&global_statistics.api_weights_db_points_read, db_points_read, __ATOMIC_RELAXED);
            __atomic_fetch_add(&global_statistics.api_weights_result_points_generated, result_points_generated, __ATOMIC_RELAXED);
            break;

        case QUERY_SOURCE_API_BADGE:
            __atomic_fetch_add(&global_statistics.api_badges_queries_made, queries, __ATOMIC_RELAXED);
            __atomic_fetch_add(&global_statistics.api_badges_db_points_read, db_points_read, __ATOMIC_RELAXED);
            __atomic_fetch_add(&global_statistics.api_badges_result_points_generated, result_points_generated, __ATOMIC_RELAXED);
            break;

        case QUERY_SOURCE_HEALTH:
            __atomic_fetch_add(&global_statistics.health_queries_made, queries, __ATOMIC_RELAXED);
            __atomic_fetch_add(&global_statistics.health_db_points_read, db_points_read, __ATOMIC_RELAXED);
            __atomic_fetch_add(&global_statistics.health_result_points_generated, result_points_generated, __ATOMIC_RELAXED);
            break;

        default:
        case QUERY_SOURCE_UNITTEST:
        case QUERY_SOURCE_UNKNOWN:
            break;
    }
}

void global_statistics_web_request_completed(uint64_t dt,
                                             uint64_t bytes_received,
                                             uint64_t bytes_sent,
                                             uint64_t content_size,
                                             uint64_t compressed_content_size) {
    uint64_t old_web_usec_max = global_statistics.web_usec_max;
    while(dt > old_web_usec_max)
        __atomic_compare_exchange(&global_statistics.web_usec_max, &old_web_usec_max, &dt, 1, __ATOMIC_RELAXED, __ATOMIC_RELAXED);

    __atomic_fetch_add(&global_statistics.web_requests, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&global_statistics.web_usec, dt, __ATOMIC_RELAXED);
    __atomic_fetch_add(&global_statistics.bytes_received, bytes_received, __ATOMIC_RELAXED);
    __atomic_fetch_add(&global_statistics.bytes_sent, bytes_sent, __ATOMIC_RELAXED);
    __atomic_fetch_add(&global_statistics.content_size, content_size, __ATOMIC_RELAXED);
    __atomic_fetch_add(&global_statistics.compressed_content_size, compressed_content_size, __ATOMIC_RELAXED);
}

uint64_t global_statistics_web_client_connected(void) {
    __atomic_fetch_add(&global_statistics.connected_clients, 1, __ATOMIC_RELAXED);
    return __atomic_fetch_add(&global_statistics.web_client_count, 1, __ATOMIC_RELAXED);
}

void global_statistics_web_client_disconnected(void) {
    __atomic_fetch_sub(&global_statistics.connected_clients, 1, __ATOMIC_RELAXED);
}

static inline void global_statistics_copy(struct global_statistics *gs, uint8_t options) {
    gs->connected_clients            = __atomic_load_n(&global_statistics.connected_clients, __ATOMIC_RELAXED);
    gs->web_requests                 = __atomic_load_n(&global_statistics.web_requests, __ATOMIC_RELAXED);
    gs->web_usec                     = __atomic_load_n(&global_statistics.web_usec, __ATOMIC_RELAXED);
    gs->web_usec_max                 = __atomic_load_n(&global_statistics.web_usec_max, __ATOMIC_RELAXED);
    gs->bytes_received               = __atomic_load_n(&global_statistics.bytes_received, __ATOMIC_RELAXED);
    gs->bytes_sent                   = __atomic_load_n(&global_statistics.bytes_sent, __ATOMIC_RELAXED);
    gs->content_size                 = __atomic_load_n(&global_statistics.content_size, __ATOMIC_RELAXED);
    gs->compressed_content_size      = __atomic_load_n(&global_statistics.compressed_content_size, __ATOMIC_RELAXED);
    gs->web_client_count             = __atomic_load_n(&global_statistics.web_client_count, __ATOMIC_RELAXED);

    gs->api_data_queries_made            = __atomic_load_n(&global_statistics.api_data_queries_made, __ATOMIC_RELAXED);
    gs->api_data_db_points_read          = __atomic_load_n(&global_statistics.api_data_db_points_read, __ATOMIC_RELAXED);
    gs->api_data_result_points_generated = __atomic_load_n(&global_statistics.api_data_result_points_generated, __ATOMIC_RELAXED);

    gs->api_weights_queries_made            = __atomic_load_n(&global_statistics.api_weights_queries_made, __ATOMIC_RELAXED);
    gs->api_weights_db_points_read          = __atomic_load_n(&global_statistics.api_weights_db_points_read, __ATOMIC_RELAXED);
    gs->api_weights_result_points_generated = __atomic_load_n(&global_statistics.api_weights_result_points_generated, __ATOMIC_RELAXED);

    gs->api_badges_queries_made            = __atomic_load_n(&global_statistics.api_badges_queries_made, __ATOMIC_RELAXED);
    gs->api_badges_db_points_read          = __atomic_load_n(&global_statistics.api_badges_db_points_read, __ATOMIC_RELAXED);
    gs->api_badges_result_points_generated = __atomic_load_n(&global_statistics.api_badges_result_points_generated, __ATOMIC_RELAXED);

    gs->health_queries_made            = __atomic_load_n(&global_statistics.health_queries_made, __ATOMIC_RELAXED);
    gs->health_db_points_read          = __atomic_load_n(&global_statistics.health_db_points_read, __ATOMIC_RELAXED);
    gs->health_result_points_generated = __atomic_load_n(&global_statistics.health_result_points_generated, __ATOMIC_RELAXED);

    gs->ml_queries_made              = __atomic_load_n(&global_statistics.ml_queries_made, __ATOMIC_RELAXED);
    gs->ml_db_points_read            = __atomic_load_n(&global_statistics.ml_db_points_read, __ATOMIC_RELAXED);
    gs->ml_result_points_generated   = __atomic_load_n(&global_statistics.ml_result_points_generated, __ATOMIC_RELAXED);
    gs->ml_models_consulted          = __atomic_load_n(&global_statistics.ml_models_consulted, __ATOMIC_RELAXED);

    gs->exporters_queries_made       = __atomic_load_n(&global_statistics.exporters_queries_made, __ATOMIC_RELAXED);
    gs->exporters_db_points_read     = __atomic_load_n(&global_statistics.exporters_db_points_read, __ATOMIC_RELAXED);
    gs->backfill_queries_made       = __atomic_load_n(&global_statistics.backfill_queries_made, __ATOMIC_RELAXED);
    gs->backfill_db_points_read     = __atomic_load_n(&global_statistics.backfill_db_points_read, __ATOMIC_RELAXED);

    gs->tier0_hot_gorilla_buffers     = __atomic_load_n(&global_statistics.tier0_hot_gorilla_buffers, __ATOMIC_RELAXED);

    gs->tier0_disk_compressed_bytes = __atomic_load_n(&global_statistics.tier0_disk_compressed_bytes, __ATOMIC_RELAXED);
    gs->tier0_disk_uncompressed_bytes = __atomic_load_n(&global_statistics.tier0_disk_uncompressed_bytes, __ATOMIC_RELAXED);

    for(size_t tier = 0; tier < storage_tiers ;tier++)
        gs->db_points_stored_per_tier[tier] = __atomic_load_n(&global_statistics.db_points_stored_per_tier[tier], __ATOMIC_RELAXED);

    if(options & GLOBAL_STATS_RESET_WEB_USEC_MAX) {
        uint64_t n = 0;
        __atomic_compare_exchange(&global_statistics.web_usec_max, (uint64_t *) &gs->web_usec_max, &n, 1, __ATOMIC_RELAXED, __ATOMIC_RELAXED);
    }
}

#define dictionary_stats_memory_total(stats) \
    ((stats).memory.dict + (stats).memory.values + (stats).memory.index)

static void global_statistics_charts(void) {
    static unsigned long long old_web_requests = 0,
                              old_web_usec = 0,
                              old_content_size = 0,
                              old_compressed_content_size = 0;

    static collected_number compression_ratio = -1,
                            average_response_time = -1;

    static time_t netdata_boottime_time = 0;
    if (!netdata_boottime_time)
        netdata_boottime_time = now_boottime_sec();
    time_t netdata_uptime = now_boottime_sec() - netdata_boottime_time;

    struct global_statistics gs;
    struct rusage me;

    struct replication_query_statistics replication = replication_get_query_statistics();
    global_statistics_copy(&gs, GLOBAL_STATS_RESET_WEB_USEC_MAX);
    getrusage(RUSAGE_SELF, &me);

    // ----------------------------------------------------------------

    {
        static RRDSET *st_cpu = NULL;
        static RRDDIM *rd_cpu_user   = NULL,
                      *rd_cpu_system = NULL;

        if (unlikely(!st_cpu)) {
            st_cpu = rrdset_create_localhost(
                    "netdata"
                    , "server_cpu"
                    , NULL
                    , "netdata"
                    , NULL
                    , "Netdata CPU usage"
                    , "milliseconds/s"
                    , "netdata"
                    , "stats"
                    , 130000
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_cpu_user   = rrddim_add(st_cpu, "user",   NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
            rd_cpu_system = rrddim_add(st_cpu, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_cpu, rd_cpu_user,   me.ru_utime.tv_sec * 1000000ULL + me.ru_utime.tv_usec);
        rrddim_set_by_pointer(st_cpu, rd_cpu_system, me.ru_stime.tv_sec * 1000000ULL + me.ru_stime.tv_usec);
        rrdset_done(st_cpu);
    }

    // ----------------------------------------------------------------

    {
        static RRDSET *st_memory = NULL;
        static RRDDIM *rd_database = NULL;
        static RRDDIM *rd_collectors = NULL;
        static RRDDIM *rd_hosts = NULL;
        static RRDDIM *rd_rrd = NULL;
        static RRDDIM *rd_contexts = NULL;
        static RRDDIM *rd_health = NULL;
        static RRDDIM *rd_functions = NULL;
        static RRDDIM *rd_labels = NULL;
        static RRDDIM *rd_strings = NULL;
        static RRDDIM *rd_streaming = NULL;
        static RRDDIM *rd_replication = NULL;
        static RRDDIM *rd_buffers = NULL;
        static RRDDIM *rd_workers = NULL;
        static RRDDIM *rd_aral = NULL;
        static RRDDIM *rd_judy = NULL;
        static RRDDIM *rd_other = NULL;

        if (unlikely(!st_memory)) {
            st_memory = rrdset_create_localhost(
                    "netdata",
                    "memory",
                    NULL,
                    "netdata",
                    NULL,
                    "Netdata Memory",
                    "bytes",
                    "netdata",
                    "stats",
                    130100,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

            rd_database = rrddim_add(st_memory, "db", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_collectors = rrddim_add(st_memory, "collectors", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_hosts = rrddim_add(st_memory, "hosts", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_rrd = rrddim_add(st_memory, "rrdset rrddim", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_contexts = rrddim_add(st_memory, "contexts", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_health = rrddim_add(st_memory, "health", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_functions = rrddim_add(st_memory, "functions", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_labels = rrddim_add(st_memory, "labels", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_strings = rrddim_add(st_memory, "strings", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_streaming = rrddim_add(st_memory, "streaming", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_replication = rrddim_add(st_memory, "replication", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers = rrddim_add(st_memory, "buffers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_workers = rrddim_add(st_memory, "workers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_aral = rrddim_add(st_memory, "aral", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_judy = rrddim_add(st_memory, "judy", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_other = rrddim_add(st_memory, "other", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        size_t buffers =
            netdata_buffers_statistics.query_targets_size +
            netdata_buffers_statistics.rrdset_done_rda_size +
            netdata_buffers_statistics.buffers_aclk +
            netdata_buffers_statistics.buffers_api +
            netdata_buffers_statistics.buffers_functions +
            netdata_buffers_statistics.buffers_sqlite +
            netdata_buffers_statistics.buffers_exporters +
            netdata_buffers_statistics.buffers_health +
            netdata_buffers_statistics.buffers_streaming +
            netdata_buffers_statistics.cbuffers_streaming +
            netdata_buffers_statistics.buffers_web +
            replication_allocated_buffers() +
            aral_by_size_overhead() +
            judy_aral_overhead();

        size_t strings = 0;
        string_statistics(NULL, NULL, NULL, NULL, NULL, &strings, NULL, NULL);

        rrddim_set_by_pointer(st_memory, rd_database, (collected_number)dbengine_total_memory + (collected_number)rrddim_db_memory_size);
        rrddim_set_by_pointer(st_memory, rd_collectors, (collected_number)dictionary_stats_memory_total(dictionary_stats_category_collectors));
        rrddim_set_by_pointer(st_memory, rd_hosts, (collected_number)dictionary_stats_memory_total(dictionary_stats_category_rrdhost) + (collected_number)netdata_buffers_statistics.rrdhost_allocations_size);
        rrddim_set_by_pointer(st_memory, rd_rrd, (collected_number)dictionary_stats_memory_total(dictionary_stats_category_rrdset_rrddim));
        rrddim_set_by_pointer(st_memory, rd_contexts, (collected_number)dictionary_stats_memory_total(dictionary_stats_category_rrdcontext));
        rrddim_set_by_pointer(st_memory, rd_health, (collected_number)dictionary_stats_memory_total(dictionary_stats_category_rrdhealth));
        rrddim_set_by_pointer(st_memory, rd_functions, (collected_number)dictionary_stats_memory_total(dictionary_stats_category_functions));
        rrddim_set_by_pointer(st_memory, rd_labels, (collected_number)dictionary_stats_memory_total(dictionary_stats_category_rrdlabels));
        rrddim_set_by_pointer(st_memory, rd_strings, (collected_number)strings);
        rrddim_set_by_pointer(st_memory, rd_streaming, (collected_number)netdata_buffers_statistics.rrdhost_senders + (collected_number)netdata_buffers_statistics.rrdhost_receivers);
        rrddim_set_by_pointer(st_memory, rd_replication, (collected_number)dictionary_stats_memory_total(dictionary_stats_category_replication) + (collected_number)replication_allocated_memory());
        rrddim_set_by_pointer(st_memory, rd_buffers, (collected_number)buffers);
        rrddim_set_by_pointer(st_memory, rd_workers, (collected_number) workers_allocated_memory());
        rrddim_set_by_pointer(st_memory, rd_aral, (collected_number) aral_by_size_structures());
        rrddim_set_by_pointer(st_memory, rd_judy, (collected_number) judy_aral_structures());
        rrddim_set_by_pointer(st_memory, rd_other, (collected_number)dictionary_stats_memory_total(dictionary_stats_category_other));

        rrdset_done(st_memory);
    }

    {
        static RRDSET *st_memory_buffers = NULL;
        static RRDDIM *rd_queries = NULL;
        static RRDDIM *rd_collectors = NULL;
        static RRDDIM *rd_buffers_aclk = NULL;
        static RRDDIM *rd_buffers_api = NULL;
        static RRDDIM *rd_buffers_functions = NULL;
        static RRDDIM *rd_buffers_sqlite = NULL;
        static RRDDIM *rd_buffers_exporters = NULL;
        static RRDDIM *rd_buffers_health = NULL;
        static RRDDIM *rd_buffers_streaming = NULL;
        static RRDDIM *rd_cbuffers_streaming = NULL;
        static RRDDIM *rd_buffers_replication = NULL;
        static RRDDIM *rd_buffers_web = NULL;
        static RRDDIM *rd_buffers_aral = NULL;
        static RRDDIM *rd_buffers_judy = NULL;

        if (unlikely(!st_memory_buffers)) {
            st_memory_buffers = rrdset_create_localhost(
                "netdata",
                "memory_buffers",
                NULL,
                "netdata",
                NULL,
                "Netdata Memory Buffers",
                "bytes",
                "netdata",
                "stats",
                130101,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_queries = rrddim_add(st_memory_buffers, "queries", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_collectors = rrddim_add(st_memory_buffers, "collection", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_aclk = rrddim_add(st_memory_buffers, "aclk", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_api = rrddim_add(st_memory_buffers, "api", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_functions = rrddim_add(st_memory_buffers, "functions", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_sqlite = rrddim_add(st_memory_buffers, "sqlite", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_exporters = rrddim_add(st_memory_buffers, "exporters", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_health = rrddim_add(st_memory_buffers, "health", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_streaming = rrddim_add(st_memory_buffers, "streaming", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_cbuffers_streaming = rrddim_add(st_memory_buffers, "streaming cbuf", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_replication = rrddim_add(st_memory_buffers, "replication", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_web = rrddim_add(st_memory_buffers, "web", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_aral = rrddim_add(st_memory_buffers, "aral", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_judy = rrddim_add(st_memory_buffers, "judy", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_memory_buffers, rd_queries, (collected_number)netdata_buffers_statistics.query_targets_size + (collected_number) onewayalloc_allocated_memory());
        rrddim_set_by_pointer(st_memory_buffers, rd_collectors, (collected_number)netdata_buffers_statistics.rrdset_done_rda_size);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_aclk, (collected_number)netdata_buffers_statistics.buffers_aclk);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_api, (collected_number)netdata_buffers_statistics.buffers_api);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_functions, (collected_number)netdata_buffers_statistics.buffers_functions);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_sqlite, (collected_number)netdata_buffers_statistics.buffers_sqlite);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_exporters, (collected_number)netdata_buffers_statistics.buffers_exporters);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_health, (collected_number)netdata_buffers_statistics.buffers_health);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_streaming, (collected_number)netdata_buffers_statistics.buffers_streaming);
        rrddim_set_by_pointer(st_memory_buffers, rd_cbuffers_streaming, (collected_number)netdata_buffers_statistics.cbuffers_streaming);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_replication, (collected_number)replication_allocated_buffers());
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_web, (collected_number)netdata_buffers_statistics.buffers_web);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_aral, (collected_number)aral_by_size_overhead());
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_judy, (collected_number)judy_aral_overhead());

        rrdset_done(st_memory_buffers);
    }

    // ----------------------------------------------------------------

    {
        static RRDSET *st_uptime = NULL;
        static RRDDIM *rd_uptime = NULL;

        if (unlikely(!st_uptime)) {
            st_uptime = rrdset_create_localhost(
                    "netdata",
                    "uptime",
                    NULL,
                    "netdata",
                    NULL,
                    "Netdata uptime",
                    "seconds",
                    "netdata",
                    "stats",
                    130150,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            rd_uptime = rrddim_add(st_uptime, "uptime", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_uptime, rd_uptime, netdata_uptime);
        rrdset_done(st_uptime);
    }

    // ----------------------------------------------------------------

    {
        static RRDSET *st_clients = NULL;
        static RRDDIM *rd_clients = NULL;

        if (unlikely(!st_clients)) {
            st_clients = rrdset_create_localhost(
                    "netdata"
                    , "clients"
                    , NULL
                    , "api"
                    , NULL
                    , "Netdata Web Clients"
                    , "connected clients"
                    , "netdata"
                    , "stats"
                    , 130200
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
            );

            rd_clients = rrddim_add(st_clients, "clients", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_clients, rd_clients, gs.connected_clients);
        rrdset_done(st_clients);
    }

    // ----------------------------------------------------------------

    {
        static RRDSET *st_reqs = NULL;
        static RRDDIM *rd_requests = NULL;

        if (unlikely(!st_reqs)) {
            st_reqs = rrdset_create_localhost(
                    "netdata"
                    , "requests"
                    , NULL
                    , "api"
                    , NULL
                    , "Netdata Web Requests"
                    , "requests/s"
                    , "netdata"
                    , "stats"
                    , 130300
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
            );

            rd_requests = rrddim_add(st_reqs, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_reqs, rd_requests, (collected_number) gs.web_requests);
        rrdset_done(st_reqs);
    }

    // ----------------------------------------------------------------

    {
        static RRDSET *st_bytes = NULL;
        static RRDDIM *rd_in = NULL,
                      *rd_out = NULL;

        if (unlikely(!st_bytes)) {
            st_bytes = rrdset_create_localhost(
                    "netdata"
                    , "net"
                    , NULL
                    , "api"
                    , NULL
                    , "Netdata Network Traffic"
                    , "kilobits/s"
                    , "netdata"
                    , "stats"
                    , 130400
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_AREA
            );

            rd_in  = rrddim_add(st_bytes, "in",  NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_bytes, "out", NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_bytes, rd_in, (collected_number) gs.bytes_received);
        rrddim_set_by_pointer(st_bytes, rd_out, (collected_number) gs.bytes_sent);
        rrdset_done(st_bytes);
    }

    // ----------------------------------------------------------------

    {
        static RRDSET *st_duration = NULL;
        static RRDDIM *rd_average = NULL,
                      *rd_max     = NULL;

        if (unlikely(!st_duration)) {
            st_duration = rrdset_create_localhost(
                    "netdata"
                    , "response_time"
                    , NULL
                    , "api"
                    , NULL
                    , "Netdata API Response Time"
                    , "milliseconds/request"
                    , "netdata"
                    , "stats"
                    , 130500
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
            );

            rd_average = rrddim_add(st_duration, "average", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
            rd_max = rrddim_add(st_duration, "max", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }

        uint64_t gweb_usec = gs.web_usec;
        uint64_t gweb_requests = gs.web_requests;

        uint64_t web_usec = (gweb_usec >= old_web_usec) ? gweb_usec - old_web_usec : 0;
        uint64_t web_requests = (gweb_requests >= old_web_requests) ? gweb_requests - old_web_requests : 0;

        old_web_usec = gweb_usec;
        old_web_requests = gweb_requests;

        if (web_requests)
            average_response_time = (collected_number) (web_usec / web_requests);

        if (unlikely(average_response_time != -1))
            rrddim_set_by_pointer(st_duration, rd_average, average_response_time);
        else
            rrddim_set_by_pointer(st_duration, rd_average, 0);

        rrddim_set_by_pointer(st_duration, rd_max, ((gs.web_usec_max)?(collected_number)gs.web_usec_max:average_response_time));
        rrdset_done(st_duration);
    }

    // ----------------------------------------------------------------

    {
        static RRDSET *st_compression = NULL;
        static RRDDIM *rd_savings = NULL;

        if (unlikely(!st_compression)) {
            st_compression = rrdset_create_localhost(
                    "netdata"
                    , "compression_ratio"
                    , NULL
                    , "api"
                    , NULL
                    , "Netdata API Responses Compression Savings Ratio"
                    , "percentage"
                    , "netdata"
                    , "stats"
                    , 130600
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
            );

            rd_savings = rrddim_add(st_compression, "savings", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }

        // since we don't lock here to read the global statistics
        // read the smaller value first
        unsigned long long gcompressed_content_size = gs.compressed_content_size;
        unsigned long long gcontent_size = gs.content_size;

        unsigned long long compressed_content_size = gcompressed_content_size - old_compressed_content_size;
        unsigned long long content_size = gcontent_size - old_content_size;

        old_compressed_content_size = gcompressed_content_size;
        old_content_size = gcontent_size;

        if (content_size && content_size >= compressed_content_size)
            compression_ratio = ((content_size - compressed_content_size) * 100 * 1000) / content_size;

        if (compression_ratio != -1)
            rrddim_set_by_pointer(st_compression, rd_savings, compression_ratio);

        rrdset_done(st_compression);
    }

    // ----------------------------------------------------------------

    {
        static RRDSET *st_queries = NULL;
        static RRDDIM *rd_api_data_queries = NULL;
        static RRDDIM *rd_api_weights_queries = NULL;
        static RRDDIM *rd_api_badges_queries = NULL;
        static RRDDIM *rd_health_queries = NULL;
        static RRDDIM *rd_ml_queries = NULL;
        static RRDDIM *rd_exporters_queries = NULL;
        static RRDDIM *rd_backfill_queries = NULL;
        static RRDDIM *rd_replication_queries = NULL;

        if (unlikely(!st_queries)) {
            st_queries = rrdset_create_localhost(
                    "netdata"
                    , "queries"
                    , NULL
                    , "queries"
                    , NULL
                    , "Netdata DB Queries"
                    , "queries/s"
                    , "netdata"
                    , "stats"
                    , 131000
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_api_data_queries = rrddim_add(st_queries, "/api/v1/data", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_api_weights_queries = rrddim_add(st_queries, "/api/v1/weights", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_api_badges_queries = rrddim_add(st_queries, "/api/v1/badge", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_health_queries = rrddim_add(st_queries, "health", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_ml_queries = rrddim_add(st_queries, "ml", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_exporters_queries = rrddim_add(st_queries, "exporters", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_backfill_queries = rrddim_add(st_queries, "backfill", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_replication_queries = rrddim_add(st_queries, "replication", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_queries, rd_api_data_queries, (collected_number)gs.api_data_queries_made);
        rrddim_set_by_pointer(st_queries, rd_api_weights_queries, (collected_number)gs.api_weights_queries_made);
        rrddim_set_by_pointer(st_queries, rd_api_badges_queries, (collected_number)gs.api_badges_queries_made);
        rrddim_set_by_pointer(st_queries, rd_health_queries, (collected_number)gs.health_queries_made);
        rrddim_set_by_pointer(st_queries, rd_ml_queries, (collected_number)gs.ml_queries_made);
        rrddim_set_by_pointer(st_queries, rd_exporters_queries, (collected_number)gs.exporters_queries_made);
        rrddim_set_by_pointer(st_queries, rd_backfill_queries, (collected_number)gs.backfill_queries_made);
        rrddim_set_by_pointer(st_queries, rd_replication_queries, (collected_number)replication.queries_finished);

        rrdset_done(st_queries);
    }

    // ----------------------------------------------------------------

    {
        static RRDSET *st_points_read = NULL;
        static RRDDIM *rd_api_data_points_read = NULL;
        static RRDDIM *rd_api_weights_points_read = NULL;
        static RRDDIM *rd_api_badges_points_read = NULL;
        static RRDDIM *rd_health_points_read = NULL;
        static RRDDIM *rd_ml_points_read = NULL;
        static RRDDIM *rd_exporters_points_read = NULL;
        static RRDDIM *rd_backfill_points_read = NULL;
        static RRDDIM *rd_replication_points_read = NULL;

        if (unlikely(!st_points_read)) {
            st_points_read = rrdset_create_localhost(
                    "netdata"
                    , "db_points_read"
                    , NULL
                    , "queries"
                    , NULL
                    , "Netdata DB Points Query Read"
                    , "points/s"
                    , "netdata"
                    , "stats"
                    , 131001
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_api_data_points_read = rrddim_add(st_points_read, "/api/v1/data", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_api_weights_points_read = rrddim_add(st_points_read, "/api/v1/weights", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_api_badges_points_read = rrddim_add(st_points_read, "/api/v1/badge", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_health_points_read = rrddim_add(st_points_read, "health", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_ml_points_read = rrddim_add(st_points_read, "ml", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_exporters_points_read = rrddim_add(st_points_read, "exporters", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_backfill_points_read = rrddim_add(st_points_read, "backfill", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_replication_points_read = rrddim_add(st_points_read, "replication", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_points_read, rd_api_data_points_read, (collected_number)gs.api_data_db_points_read);
        rrddim_set_by_pointer(st_points_read, rd_api_weights_points_read, (collected_number)gs.api_weights_db_points_read);
        rrddim_set_by_pointer(st_points_read, rd_api_badges_points_read, (collected_number)gs.api_badges_db_points_read);
        rrddim_set_by_pointer(st_points_read, rd_health_points_read, (collected_number)gs.health_db_points_read);
        rrddim_set_by_pointer(st_points_read, rd_ml_points_read, (collected_number)gs.ml_db_points_read);
        rrddim_set_by_pointer(st_points_read, rd_exporters_points_read, (collected_number)gs.exporters_db_points_read);
        rrddim_set_by_pointer(st_points_read, rd_backfill_points_read, (collected_number)gs.backfill_db_points_read);
        rrddim_set_by_pointer(st_points_read, rd_replication_points_read, (collected_number)replication.points_read);

        rrdset_done(st_points_read);
    }

    // ----------------------------------------------------------------

    if(gs.api_data_result_points_generated || replication.points_generated) {
        static RRDSET *st_points_generated = NULL;
        static RRDDIM *rd_api_data_points_generated = NULL;
        static RRDDIM *rd_api_weights_points_generated = NULL;
        static RRDDIM *rd_api_badges_points_generated = NULL;
        static RRDDIM *rd_health_points_generated = NULL;
        static RRDDIM *rd_ml_points_generated = NULL;
        static RRDDIM *rd_replication_points_generated = NULL;

        if (unlikely(!st_points_generated)) {
            st_points_generated = rrdset_create_localhost(
                    "netdata"
                    , "db_points_results"
                    , NULL
                    , "queries"
                    , NULL
                    , "Netdata Points in Query Results"
                    , "points/s"
                    , "netdata"
                    , "stats"
                    , 131002
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_api_data_points_generated = rrddim_add(st_points_generated, "/api/v1/data", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_api_weights_points_generated = rrddim_add(st_points_generated, "/api/v1/weights", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_api_badges_points_generated = rrddim_add(st_points_generated, "/api/v1/badge", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_health_points_generated = rrddim_add(st_points_generated, "health", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_ml_points_generated = rrddim_add(st_points_generated, "ml", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_replication_points_generated = rrddim_add(st_points_generated, "replication", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_points_generated, rd_api_data_points_generated, (collected_number)gs.api_data_result_points_generated);
        rrddim_set_by_pointer(st_points_generated, rd_api_weights_points_generated, (collected_number)gs.api_weights_result_points_generated);
        rrddim_set_by_pointer(st_points_generated, rd_api_badges_points_generated, (collected_number)gs.api_badges_result_points_generated);
        rrddim_set_by_pointer(st_points_generated, rd_health_points_generated, (collected_number)gs.health_result_points_generated);
        rrddim_set_by_pointer(st_points_generated, rd_ml_points_generated, (collected_number)gs.ml_result_points_generated);
        rrddim_set_by_pointer(st_points_generated, rd_replication_points_generated, (collected_number)replication.points_generated);

        rrdset_done(st_points_generated);
    }

    // ----------------------------------------------------------------

    {
        static RRDSET *st_points_stored = NULL;
        static RRDDIM *rds[RRD_STORAGE_TIERS] = {};

        if (unlikely(!st_points_stored)) {
            st_points_stored = rrdset_create_localhost(
                    "netdata"
                    , "db_points_stored"
                    , NULL
                    , "queries"
                    , NULL
                    , "Netdata DB Points Stored"
                    , "points/s"
                    , "netdata"
                    , "stats"
                    , 131003
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_STACKED
            );

            for(size_t tier = 0; tier < storage_tiers ;tier++) {
                char buf[30 + 1];
                snprintfz(buf, sizeof(buf) - 1, "tier%zu", tier);
                rds[tier] = rrddim_add(st_points_stored, buf, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
        }

        for(size_t tier = 0; tier < storage_tiers ;tier++)
            rrddim_set_by_pointer(st_points_stored, rds[tier], (collected_number)gs.db_points_stored_per_tier[tier]);

        rrdset_done(st_points_stored);
    }

    ml_update_global_statistics_charts(gs.ml_models_consulted);

    // ----------------------------------------------------------------

#ifdef ENABLE_DBENGINE
    if (tier_page_type[0] == PAGE_GORILLA_METRICS)
    {
        static RRDSET *st_tier0_gorilla_pages = NULL;
        static RRDDIM *rd_num_gorilla_pages = NULL;

        if (unlikely(!st_tier0_gorilla_pages)) {
            st_tier0_gorilla_pages = rrdset_create_localhost(
                    "netdata"
                    , "tier0_gorilla_pages"
                    , NULL
                    , "tier0_gorilla_pages"
                    , NULL
                    , "Number of gorilla_pages"
                    , "count"
                    , "netdata"
                    , "stats"
                    , 131004
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
            );

            rd_num_gorilla_pages = rrddim_add(st_tier0_gorilla_pages, "count", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_tier0_gorilla_pages, rd_num_gorilla_pages, (collected_number)gs.tier0_hot_gorilla_buffers);

        rrdset_done(st_tier0_gorilla_pages);
    }

    if (tier_page_type[0] == PAGE_GORILLA_METRICS)
    {
        static RRDSET *st_tier0_compression_info = NULL;

        static RRDDIM *rd_compressed_bytes = NULL;
        static RRDDIM *rd_uncompressed_bytes = NULL;

        if (unlikely(!st_tier0_compression_info)) {
            st_tier0_compression_info = rrdset_create_localhost(
                    "netdata"
                    , "tier0_compression_info"
                    , NULL
                    , "tier0_compression_info"
                    , NULL
                    , "Tier 0 compression info"
                    , "bytes"
                    , "netdata"
                    , "stats"
                    , 131005
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
            );

            rd_compressed_bytes = rrddim_add(st_tier0_compression_info, "compressed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_uncompressed_bytes = rrddim_add(st_tier0_compression_info, "uncompressed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_tier0_compression_info, rd_compressed_bytes, (collected_number)gs.tier0_disk_compressed_bytes);
        rrddim_set_by_pointer(st_tier0_compression_info, rd_uncompressed_bytes, (collected_number)gs.tier0_disk_uncompressed_bytes);

        rrdset_done(st_tier0_compression_info);
    }
#endif
}

// ----------------------------------------------------------------------------
// sqlite3 statistics

struct sqlite3_statistics {
    uint64_t sqlite3_queries_made;
    uint64_t sqlite3_queries_ok;
    uint64_t sqlite3_queries_failed;
    uint64_t sqlite3_queries_failed_busy;
    uint64_t sqlite3_queries_failed_locked;
    uint64_t sqlite3_rows;
    uint64_t sqlite3_metadata_cache_hit;
    uint64_t sqlite3_context_cache_hit;
    uint64_t sqlite3_metadata_cache_miss;
    uint64_t sqlite3_context_cache_miss;
    uint64_t sqlite3_metadata_cache_spill;
    uint64_t sqlite3_context_cache_spill;
    uint64_t sqlite3_metadata_cache_write;
    uint64_t sqlite3_context_cache_write;

} sqlite3_statistics = { };

void global_statistics_sqlite3_query_completed(bool success, bool busy, bool locked) {
    __atomic_fetch_add(&sqlite3_statistics.sqlite3_queries_made, 1, __ATOMIC_RELAXED);

    if(success) {
        __atomic_fetch_add(&sqlite3_statistics.sqlite3_queries_ok, 1, __ATOMIC_RELAXED);
    }
    else {
        __atomic_fetch_add(&sqlite3_statistics.sqlite3_queries_failed, 1, __ATOMIC_RELAXED);

        if(busy)
            __atomic_fetch_add(&sqlite3_statistics.sqlite3_queries_failed_busy, 1, __ATOMIC_RELAXED);

        if(locked)
            __atomic_fetch_add(&sqlite3_statistics.sqlite3_queries_failed_locked, 1, __ATOMIC_RELAXED);
    }
}

void global_statistics_sqlite3_row_completed(void) {
    __atomic_fetch_add(&sqlite3_statistics.sqlite3_rows, 1, __ATOMIC_RELAXED);
}

static inline void sqlite3_statistics_copy(struct sqlite3_statistics *gs) {
    static usec_t last_run = 0;

    gs->sqlite3_queries_made          = __atomic_load_n(&sqlite3_statistics.sqlite3_queries_made, __ATOMIC_RELAXED);
    gs->sqlite3_queries_ok            = __atomic_load_n(&sqlite3_statistics.sqlite3_queries_ok, __ATOMIC_RELAXED);
    gs->sqlite3_queries_failed        = __atomic_load_n(&sqlite3_statistics.sqlite3_queries_failed, __ATOMIC_RELAXED);
    gs->sqlite3_queries_failed_busy   = __atomic_load_n(&sqlite3_statistics.sqlite3_queries_failed_busy, __ATOMIC_RELAXED);
    gs->sqlite3_queries_failed_locked = __atomic_load_n(&sqlite3_statistics.sqlite3_queries_failed_locked, __ATOMIC_RELAXED);
    gs->sqlite3_rows                  = __atomic_load_n(&sqlite3_statistics.sqlite3_rows, __ATOMIC_RELAXED);

    usec_t timeout = default_rrd_update_every * USEC_PER_SEC + default_rrd_update_every * USEC_PER_SEC / 3;
    usec_t now = now_monotonic_usec();
    if(!last_run)
        last_run = now;
    usec_t delta = now - last_run;
    bool query_sqlite3 = delta < timeout;

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_metadata_cache_hit = (uint64_t) sql_metadata_cache_stats(SQLITE_DBSTATUS_CACHE_HIT);
    else {
        gs->sqlite3_metadata_cache_hit = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_context_cache_hit = (uint64_t) sql_context_cache_stats(SQLITE_DBSTATUS_CACHE_HIT);
    else {
        gs->sqlite3_context_cache_hit = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_metadata_cache_miss = (uint64_t) sql_metadata_cache_stats(SQLITE_DBSTATUS_CACHE_MISS);
    else {
        gs->sqlite3_metadata_cache_miss = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_context_cache_miss = (uint64_t) sql_context_cache_stats(SQLITE_DBSTATUS_CACHE_MISS);
    else {
        gs->sqlite3_context_cache_miss = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_metadata_cache_spill = (uint64_t) sql_metadata_cache_stats(SQLITE_DBSTATUS_CACHE_SPILL);
    else {
        gs->sqlite3_metadata_cache_spill = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_context_cache_spill = (uint64_t) sql_context_cache_stats(SQLITE_DBSTATUS_CACHE_SPILL);
    else {
        gs->sqlite3_context_cache_spill = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_metadata_cache_write = (uint64_t) sql_metadata_cache_stats(SQLITE_DBSTATUS_CACHE_WRITE);
    else {
        gs->sqlite3_metadata_cache_write = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_context_cache_write = (uint64_t) sql_context_cache_stats(SQLITE_DBSTATUS_CACHE_WRITE);
    else {
        gs->sqlite3_context_cache_write = UINT64_MAX;
        query_sqlite3 = false;
    }

    last_run = now_monotonic_usec();
}

static void sqlite3_statistics_charts(void) {
    struct sqlite3_statistics gs;
    sqlite3_statistics_copy(&gs);

    if(gs.sqlite3_queries_made) {
        static RRDSET *st_sqlite3_queries = NULL;
        static RRDDIM *rd_queries = NULL;

        if (unlikely(!st_sqlite3_queries)) {
            st_sqlite3_queries = rrdset_create_localhost(
                "netdata"
                , "sqlite3_queries"
                , NULL
                , "sqlite3"
                , NULL
                , "Netdata SQLite3 Queries"
                , "queries/s"
                , "netdata"
                , "stats"
                , 131100
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_queries = rrddim_add(st_sqlite3_queries, "queries", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_sqlite3_queries, rd_queries, (collected_number)gs.sqlite3_queries_made);

        rrdset_done(st_sqlite3_queries);
    }

    // ----------------------------------------------------------------

    if(gs.sqlite3_queries_ok || gs.sqlite3_queries_failed) {
        static RRDSET *st_sqlite3_queries_by_status = NULL;
        static RRDDIM *rd_ok = NULL, *rd_failed = NULL, *rd_busy = NULL, *rd_locked = NULL;

        if (unlikely(!st_sqlite3_queries_by_status)) {
            st_sqlite3_queries_by_status = rrdset_create_localhost(
                "netdata"
                , "sqlite3_queries_by_status"
                , NULL
                , "sqlite3"
                , NULL
                , "Netdata SQLite3 Queries by status"
                , "queries/s"
                , "netdata"
                , "stats"
                , 131101
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_ok     = rrddim_add(st_sqlite3_queries_by_status, "ok",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_failed = rrddim_add(st_sqlite3_queries_by_status, "failed", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_busy   = rrddim_add(st_sqlite3_queries_by_status, "busy",   NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_locked = rrddim_add(st_sqlite3_queries_by_status, "locked", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_sqlite3_queries_by_status, rd_ok,     (collected_number)gs.sqlite3_queries_made);
        rrddim_set_by_pointer(st_sqlite3_queries_by_status, rd_failed, (collected_number)gs.sqlite3_queries_failed);
        rrddim_set_by_pointer(st_sqlite3_queries_by_status, rd_busy,   (collected_number)gs.sqlite3_queries_failed_busy);
        rrddim_set_by_pointer(st_sqlite3_queries_by_status, rd_locked, (collected_number)gs.sqlite3_queries_failed_locked);

        rrdset_done(st_sqlite3_queries_by_status);
    }

    // ----------------------------------------------------------------

    if(gs.sqlite3_rows) {
        static RRDSET *st_sqlite3_rows = NULL;
        static RRDDIM *rd_rows = NULL;

        if (unlikely(!st_sqlite3_rows)) {
            st_sqlite3_rows = rrdset_create_localhost(
                "netdata"
                , "sqlite3_rows"
                , NULL
                , "sqlite3"
                , NULL
                , "Netdata SQLite3 Rows"
                , "rows/s"
                , "netdata"
                , "stats"
                , 131102
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_rows = rrddim_add(st_sqlite3_rows, "ok", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_sqlite3_rows, rd_rows, (collected_number)gs.sqlite3_rows);

        rrdset_done(st_sqlite3_rows);
    }

    if(gs.sqlite3_metadata_cache_hit) {
        static RRDSET *st_sqlite3_cache = NULL;
        static RRDDIM *rd_cache_hit = NULL;
        static RRDDIM *rd_cache_miss= NULL;
        static RRDDIM *rd_cache_spill= NULL;
        static RRDDIM *rd_cache_write= NULL;

        if (unlikely(!st_sqlite3_cache)) {
            st_sqlite3_cache = rrdset_create_localhost(
                "netdata"
                , "sqlite3_metatada_cache"
                , NULL
                , "sqlite3"
                , NULL
                , "Netdata SQLite3 metadata cache"
                , "ops/s"
                , "netdata"
                , "stats"
                , 131103
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_cache_hit = rrddim_add(st_sqlite3_cache, "cache_hit", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cache_miss = rrddim_add(st_sqlite3_cache, "cache_miss", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cache_spill = rrddim_add(st_sqlite3_cache, "cache_spill", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cache_write = rrddim_add(st_sqlite3_cache, "cache_write", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        if(gs.sqlite3_metadata_cache_hit != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_hit, (collected_number)gs.sqlite3_metadata_cache_hit);

        if(gs.sqlite3_metadata_cache_miss != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_miss, (collected_number)gs.sqlite3_metadata_cache_miss);

        if(gs.sqlite3_metadata_cache_spill != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_spill, (collected_number)gs.sqlite3_metadata_cache_spill);

        if(gs.sqlite3_metadata_cache_write != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_write, (collected_number)gs.sqlite3_metadata_cache_write);

        rrdset_done(st_sqlite3_cache);
    }

    if(gs.sqlite3_context_cache_hit) {
        static RRDSET *st_sqlite3_cache = NULL;
        static RRDDIM *rd_cache_hit = NULL;
        static RRDDIM *rd_cache_miss= NULL;
        static RRDDIM *rd_cache_spill= NULL;
        static RRDDIM *rd_cache_write= NULL;

        if (unlikely(!st_sqlite3_cache)) {
            st_sqlite3_cache = rrdset_create_localhost(
                "netdata"
                , "sqlite3_context_cache"
                , NULL
                , "sqlite3"
                , NULL
                , "Netdata SQLite3 context cache"
                , "ops/s"
                , "netdata"
                , "stats"
                , 131104
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_cache_hit = rrddim_add(st_sqlite3_cache, "cache_hit", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cache_miss = rrddim_add(st_sqlite3_cache, "cache_miss", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cache_spill = rrddim_add(st_sqlite3_cache, "cache_spill", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cache_write = rrddim_add(st_sqlite3_cache, "cache_write", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        if(gs.sqlite3_context_cache_hit != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_hit, (collected_number)gs.sqlite3_context_cache_hit);

        if(gs.sqlite3_context_cache_miss != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_miss, (collected_number)gs.sqlite3_context_cache_miss);

        if(gs.sqlite3_context_cache_spill != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_spill, (collected_number)gs.sqlite3_context_cache_spill);

        if(gs.sqlite3_context_cache_write != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_write, (collected_number)gs.sqlite3_context_cache_write);

        rrdset_done(st_sqlite3_cache);
    }

    // ----------------------------------------------------------------
}

#ifdef ENABLE_DBENGINE

struct dbengine2_cache_pointers {
    RRDSET *st_cache_hit_ratio;
    RRDDIM *rd_hit_ratio_closest;
    RRDDIM *rd_hit_ratio_exact;

    RRDSET *st_operations;
    RRDDIM *rd_searches_closest;
    RRDDIM *rd_searches_exact;
    RRDDIM *rd_add_hot;
    RRDDIM *rd_add_clean;
    RRDDIM *rd_evictions;
    RRDDIM *rd_flushes;
    RRDDIM *rd_acquires;
    RRDDIM *rd_releases;
    RRDDIM *rd_acquires_for_deletion;

    RRDSET *st_pgc_memory;
    RRDDIM *rd_pgc_memory_free;
    RRDDIM *rd_pgc_memory_clean;
    RRDDIM *rd_pgc_memory_hot;
    RRDDIM *rd_pgc_memory_dirty;
    RRDDIM *rd_pgc_memory_index;
    RRDDIM *rd_pgc_memory_evicting;
    RRDDIM *rd_pgc_memory_flushing;

    RRDSET *st_pgc_tm;
    RRDDIM *rd_pgc_tm_current;
    RRDDIM *rd_pgc_tm_wanted;
    RRDDIM *rd_pgc_tm_hot_max;
    RRDDIM *rd_pgc_tm_dirty_max;
    RRDDIM *rd_pgc_tm_hot;
    RRDDIM *rd_pgc_tm_dirty;
    RRDDIM *rd_pgc_tm_referenced;

    RRDSET *st_pgc_pages;
    RRDDIM *rd_pgc_pages_clean;
    RRDDIM *rd_pgc_pages_hot;
    RRDDIM *rd_pgc_pages_dirty;
    RRDDIM *rd_pgc_pages_referenced;

    RRDSET *st_pgc_memory_changes;
    RRDDIM *rd_pgc_memory_new_hot;
    RRDDIM *rd_pgc_memory_new_clean;
    RRDDIM *rd_pgc_memory_clean_evictions;

    RRDSET *st_pgc_memory_migrations;
    RRDDIM *rd_pgc_memory_hot_to_dirty;
    RRDDIM *rd_pgc_memory_dirty_to_clean;

    RRDSET *st_pgc_workers;
    RRDDIM *rd_pgc_workers_evictors;
    RRDDIM *rd_pgc_workers_flushers;
    RRDDIM *rd_pgc_workers_adders;
    RRDDIM *rd_pgc_workers_searchers;
    RRDDIM *rd_pgc_workers_jv2_flushers;
    RRDDIM *rd_pgc_workers_hot2dirty;

    RRDSET *st_pgc_memory_events;
    RRDDIM *rd_pgc_memory_evictions_critical;
    RRDDIM *rd_pgc_memory_evictions_aggressive;
    RRDDIM *rd_pgc_memory_flushes_critical;

    RRDSET *st_pgc_waste;
    RRDDIM *rd_pgc_waste_evictions_skipped;
    RRDDIM *rd_pgc_waste_flushes_cancelled;
    RRDDIM *rd_pgc_waste_insert_spins;
    RRDDIM *rd_pgc_waste_evict_spins;
    RRDDIM *rd_pgc_waste_release_spins;
    RRDDIM *rd_pgc_waste_acquire_spins;
    RRDDIM *rd_pgc_waste_delete_spins;
    RRDDIM *rd_pgc_waste_flush_spins;

};

static void dbengine2_cache_statistics_charts(struct dbengine2_cache_pointers *ptrs, struct pgc_statistics *pgc_stats, struct pgc_statistics *pgc_stats_old __maybe_unused, const char *name, int priority) {

    {
        if (unlikely(!ptrs->st_cache_hit_ratio)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_hit_ratio", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Hit Ratio", name);

            ptrs->st_cache_hit_ratio = rrdset_create_localhost(
                    "netdata",
                    buffer_tostring(id),
                    NULL,
                    buffer_tostring(family),
                    NULL,
                    buffer_tostring(title),
                    "%",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            ptrs->rd_hit_ratio_closest = rrddim_add(ptrs->st_cache_hit_ratio, "closest", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_hit_ratio_exact = rrddim_add(ptrs->st_cache_hit_ratio, "exact", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        size_t closest_percent = 100 * 10000;
        if(pgc_stats->searches_closest > pgc_stats_old->searches_closest)
            closest_percent = (pgc_stats->searches_closest_hits - pgc_stats_old->searches_closest_hits) * 100 * 10000 / (pgc_stats->searches_closest - pgc_stats_old->searches_closest);

        size_t exact_percent = 100 * 10000;
        if(pgc_stats->searches_exact > pgc_stats_old->searches_exact)
            exact_percent = (pgc_stats->searches_exact_hits - pgc_stats_old->searches_exact_hits) * 100 * 10000 / (pgc_stats->searches_exact - pgc_stats_old->searches_exact);

        rrddim_set_by_pointer(ptrs->st_cache_hit_ratio, ptrs->rd_hit_ratio_closest, (collected_number)closest_percent);
        rrddim_set_by_pointer(ptrs->st_cache_hit_ratio, ptrs->rd_hit_ratio_exact, (collected_number)exact_percent);

        rrdset_done(ptrs->st_cache_hit_ratio);
    }

    {
        if (unlikely(!ptrs->st_operations)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_operations", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Operations", name);

            ptrs->st_operations = rrdset_create_localhost(
                    "netdata",
                    buffer_tostring(id),
                    NULL,
                    buffer_tostring(family),
                    NULL,
                    buffer_tostring(title),
                    "ops/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            ptrs->rd_searches_closest   = rrddim_add(ptrs->st_operations, "search closest", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_searches_exact     = rrddim_add(ptrs->st_operations, "search exact", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_add_hot            = rrddim_add(ptrs->st_operations, "add hot", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_add_clean          = rrddim_add(ptrs->st_operations, "add clean", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_evictions          = rrddim_add(ptrs->st_operations, "evictions", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_flushes            = rrddim_add(ptrs->st_operations, "flushes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_acquires           = rrddim_add(ptrs->st_operations, "acquires", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_releases           = rrddim_add(ptrs->st_operations, "releases", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_acquires_for_deletion = rrddim_add(ptrs->st_operations, "del acquires", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_searches_closest, (collected_number)pgc_stats->searches_closest);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_searches_exact, (collected_number)pgc_stats->searches_exact);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_add_hot, (collected_number)pgc_stats->queues.hot.added_entries);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_add_clean, (collected_number)(pgc_stats->added_entries - pgc_stats->queues.hot.added_entries));
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_evictions, (collected_number)pgc_stats->queues.clean.removed_entries);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_flushes, (collected_number)pgc_stats->flushes_completed);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_acquires, (collected_number)pgc_stats->acquires);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_releases, (collected_number)pgc_stats->releases);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_acquires_for_deletion, (collected_number)pgc_stats->acquires_for_deletion);

        rrdset_done(ptrs->st_operations);
    }

    {
        if (unlikely(!ptrs->st_pgc_memory)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_memory", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Memory", name);

            ptrs->st_pgc_memory = rrdset_create_localhost(
                    "netdata",
                    buffer_tostring(id),
                    NULL,
                    buffer_tostring(family),
                    NULL,
                    buffer_tostring(title),
                    "bytes",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

            ptrs->rd_pgc_memory_free     = rrddim_add(ptrs->st_pgc_memory, "free",     NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_memory_hot      = rrddim_add(ptrs->st_pgc_memory, "hot",      NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_memory_dirty    = rrddim_add(ptrs->st_pgc_memory, "dirty",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_memory_clean    = rrddim_add(ptrs->st_pgc_memory, "clean",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_memory_index    = rrddim_add(ptrs->st_pgc_memory, "index",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_memory_evicting = rrddim_add(ptrs->st_pgc_memory, "evicting", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_memory_flushing = rrddim_add(ptrs->st_pgc_memory, "flushing", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        collected_number free = (pgc_stats->current_cache_size > pgc_stats->wanted_cache_size) ? 0 :
                                (collected_number)(pgc_stats->wanted_cache_size - pgc_stats->current_cache_size);

        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_free, free);
        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_hot, (collected_number)pgc_stats->queues.hot.size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_dirty, (collected_number)pgc_stats->queues.dirty.size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_clean, (collected_number)pgc_stats->queues.clean.size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_evicting, (collected_number)pgc_stats->evicting_size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_flushing, (collected_number)pgc_stats->flushing_size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_index,
                              (collected_number)(pgc_stats->size - pgc_stats->queues.clean.size - pgc_stats->queues.hot.size - pgc_stats->queues.dirty.size - pgc_stats->evicting_size - pgc_stats->flushing_size));

        rrdset_done(ptrs->st_pgc_memory);
    }

    {
        if (unlikely(!ptrs->st_pgc_tm)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_target_memory", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Target Cache Memory", name);

            ptrs->st_pgc_tm = rrdset_create_localhost(
                    "netdata",
                    buffer_tostring(id),
                    NULL,
                    buffer_tostring(family),
                    NULL,
                    buffer_tostring(title),
                    "bytes",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            ptrs->rd_pgc_tm_current    = rrddim_add(ptrs->st_pgc_tm, "current",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_tm_wanted     = rrddim_add(ptrs->st_pgc_tm, "wanted",     NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_tm_referenced = rrddim_add(ptrs->st_pgc_tm, "referenced", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_tm_hot_max    = rrddim_add(ptrs->st_pgc_tm, "hot max",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_tm_dirty_max  = rrddim_add(ptrs->st_pgc_tm, "dirty max",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_tm_hot        = rrddim_add(ptrs->st_pgc_tm, "hot",        NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_tm_dirty      = rrddim_add(ptrs->st_pgc_tm, "dirty",      NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_current, (collected_number)pgc_stats->current_cache_size);
        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_wanted, (collected_number)pgc_stats->wanted_cache_size);
        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_referenced, (collected_number)pgc_stats->referenced_size);
        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_hot_max, (collected_number)pgc_stats->queues.hot.max_size);
        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_dirty_max, (collected_number)pgc_stats->queues.dirty.max_size);
        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_hot, (collected_number)pgc_stats->queues.hot.size);
        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_dirty, (collected_number)pgc_stats->queues.dirty.size);

        rrdset_done(ptrs->st_pgc_tm);
    }

    {
        if (unlikely(!ptrs->st_pgc_pages)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_pages", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Pages", name);

            ptrs->st_pgc_pages = rrdset_create_localhost(
                    "netdata",
                    buffer_tostring(id),
                    NULL,
                    buffer_tostring(family),
                    NULL,
                    buffer_tostring(title),
                    "pages",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            ptrs->rd_pgc_pages_clean   = rrddim_add(ptrs->st_pgc_pages, "clean", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_pages_hot     = rrddim_add(ptrs->st_pgc_pages, "hot", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_pages_dirty   = rrddim_add(ptrs->st_pgc_pages, "dirty", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_pages_referenced = rrddim_add(ptrs->st_pgc_pages, "referenced", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_pages, ptrs->rd_pgc_pages_clean, (collected_number)pgc_stats->queues.clean.entries);
        rrddim_set_by_pointer(ptrs->st_pgc_pages, ptrs->rd_pgc_pages_hot, (collected_number)pgc_stats->queues.hot.entries);
        rrddim_set_by_pointer(ptrs->st_pgc_pages, ptrs->rd_pgc_pages_dirty, (collected_number)pgc_stats->queues.dirty.entries);
        rrddim_set_by_pointer(ptrs->st_pgc_pages, ptrs->rd_pgc_pages_referenced, (collected_number)pgc_stats->referenced_entries);

        rrdset_done(ptrs->st_pgc_pages);
    }

    {
        if (unlikely(!ptrs->st_pgc_memory_changes)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_memory_changes", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Memory Changes", name);

            ptrs->st_pgc_memory_changes = rrdset_create_localhost(
                    "netdata",
                    buffer_tostring(id),
                    NULL,
                    buffer_tostring(family),
                    NULL,
                    buffer_tostring(title),
                    "bytes/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_AREA);

            ptrs->rd_pgc_memory_new_clean         = rrddim_add(ptrs->st_pgc_memory_changes, "new clean", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_memory_clean_evictions   = rrddim_add(ptrs->st_pgc_memory_changes, "evictions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_memory_new_hot           = rrddim_add(ptrs->st_pgc_memory_changes, "new hot", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_memory_changes, ptrs->rd_pgc_memory_new_clean, (collected_number)(pgc_stats->added_size - pgc_stats->queues.hot.added_size));
        rrddim_set_by_pointer(ptrs->st_pgc_memory_changes, ptrs->rd_pgc_memory_clean_evictions, (collected_number)pgc_stats->queues.clean.removed_size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory_changes, ptrs->rd_pgc_memory_new_hot, (collected_number)pgc_stats->queues.hot.added_size);

        rrdset_done(ptrs->st_pgc_memory_changes);
    }

    {
        if (unlikely(!ptrs->st_pgc_memory_migrations)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_memory_migrations", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Memory Migrations", name);

            ptrs->st_pgc_memory_migrations = rrdset_create_localhost(
                    "netdata",
                    buffer_tostring(id),
                    NULL,
                    buffer_tostring(family),
                    NULL,
                    buffer_tostring(title),
                    "bytes/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_AREA);

            ptrs->rd_pgc_memory_dirty_to_clean    = rrddim_add(ptrs->st_pgc_memory_migrations, "dirty to clean", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_memory_hot_to_dirty      = rrddim_add(ptrs->st_pgc_memory_migrations, "hot to dirty", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_memory_migrations, ptrs->rd_pgc_memory_dirty_to_clean, (collected_number)pgc_stats->queues.dirty.removed_size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory_migrations, ptrs->rd_pgc_memory_hot_to_dirty, (collected_number)pgc_stats->queues.dirty.added_size);

        rrdset_done(ptrs->st_pgc_memory_migrations);
    }

    {
        if (unlikely(!ptrs->st_pgc_memory_events)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_events", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Events", name);

            ptrs->st_pgc_memory_events = rrdset_create_localhost(
                    "netdata",
                    buffer_tostring(id),
                    NULL,
                    buffer_tostring(family),
                    NULL,
                    buffer_tostring(title),
                    "events/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_AREA);

            ptrs->rd_pgc_memory_evictions_aggressive = rrddim_add(ptrs->st_pgc_memory_events, "evictions aggressive", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_memory_evictions_critical   = rrddim_add(ptrs->st_pgc_memory_events, "evictions critical", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_memory_flushes_critical     = rrddim_add(ptrs->st_pgc_memory_events, "flushes critical", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_memory_events, ptrs->rd_pgc_memory_evictions_aggressive, (collected_number)pgc_stats->events_cache_needs_space_aggressively);
        rrddim_set_by_pointer(ptrs->st_pgc_memory_events, ptrs->rd_pgc_memory_evictions_critical, (collected_number)pgc_stats->events_cache_under_severe_pressure);
        rrddim_set_by_pointer(ptrs->st_pgc_memory_events, ptrs->rd_pgc_memory_flushes_critical, (collected_number)pgc_stats->events_flush_critical);

        rrdset_done(ptrs->st_pgc_memory_events);
    }

    {
        if (unlikely(!ptrs->st_pgc_waste)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_waste_events", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Waste Events", name);

            ptrs->st_pgc_waste = rrdset_create_localhost(
                    "netdata",
                    buffer_tostring(id),
                    NULL,
                    buffer_tostring(family),
                    NULL,
                    buffer_tostring(title),
                    "events/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            ptrs->rd_pgc_waste_evictions_skipped = rrddim_add(ptrs->st_pgc_waste, "evictions skipped", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_flushes_cancelled = rrddim_add(ptrs->st_pgc_waste, "flushes cancelled", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_acquire_spins     = rrddim_add(ptrs->st_pgc_waste, "acquire spins", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_release_spins     = rrddim_add(ptrs->st_pgc_waste, "release spins", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_insert_spins      = rrddim_add(ptrs->st_pgc_waste, "insert spins", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_delete_spins      = rrddim_add(ptrs->st_pgc_waste, "delete spins", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_evict_spins       = rrddim_add(ptrs->st_pgc_waste, "evict spins", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_flush_spins       = rrddim_add(ptrs->st_pgc_waste, "flush spins", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_evictions_skipped, (collected_number)pgc_stats->evict_skipped);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_flushes_cancelled, (collected_number)pgc_stats->flushes_cancelled);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_acquire_spins, (collected_number)pgc_stats->acquire_spins);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_release_spins, (collected_number)pgc_stats->release_spins);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_insert_spins, (collected_number)pgc_stats->insert_spins);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_delete_spins, (collected_number)pgc_stats->delete_spins);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_evict_spins, (collected_number)pgc_stats->evict_spins);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_flush_spins, (collected_number)pgc_stats->flush_spins);

        rrdset_done(ptrs->st_pgc_waste);
    }

    {
        if (unlikely(!ptrs->st_pgc_workers)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_workers", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Workers", name);

            ptrs->st_pgc_workers = rrdset_create_localhost(
                    "netdata",
                    buffer_tostring(id),
                    NULL,
                    buffer_tostring(family),
                    NULL,
                    buffer_tostring(title),
                    "workers",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            ptrs->rd_pgc_workers_searchers = rrddim_add(ptrs->st_pgc_workers, "searchers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_workers_adders    = rrddim_add(ptrs->st_pgc_workers, "adders",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_workers_evictors  = rrddim_add(ptrs->st_pgc_workers, "evictors",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_workers_flushers  = rrddim_add(ptrs->st_pgc_workers, "flushers",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_workers_hot2dirty = rrddim_add(ptrs->st_pgc_workers, "hot2dirty",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_workers_jv2_flushers = rrddim_add(ptrs->st_pgc_workers, "jv2 flushers",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_workers, ptrs->rd_pgc_workers_searchers, (collected_number)pgc_stats->workers_search);
        rrddim_set_by_pointer(ptrs->st_pgc_workers, ptrs->rd_pgc_workers_adders, (collected_number)pgc_stats->workers_add);
        rrddim_set_by_pointer(ptrs->st_pgc_workers, ptrs->rd_pgc_workers_evictors, (collected_number)pgc_stats->workers_evict);
        rrddim_set_by_pointer(ptrs->st_pgc_workers, ptrs->rd_pgc_workers_flushers, (collected_number)pgc_stats->workers_flush);
        rrddim_set_by_pointer(ptrs->st_pgc_workers, ptrs->rd_pgc_workers_hot2dirty, (collected_number)pgc_stats->workers_hot2dirty);
        rrddim_set_by_pointer(ptrs->st_pgc_workers, ptrs->rd_pgc_workers_jv2_flushers, (collected_number)pgc_stats->workers_jv2_flush);

        rrdset_done(ptrs->st_pgc_workers);
    }
}


static void dbengine2_statistics_charts(void) {
    if(!main_cache || !main_mrg)
        return;

    static struct dbengine2_cache_pointers main_cache_ptrs = {}, open_cache_ptrs = {}, extent_cache_ptrs = {};
    static struct rrdeng_cache_efficiency_stats cache_efficiency_stats = {}, cache_efficiency_stats_old = {};
    static struct pgc_statistics pgc_main_stats = {}, pgc_main_stats_old = {}; (void)pgc_main_stats_old;
    static struct pgc_statistics pgc_open_stats = {}, pgc_open_stats_old = {}; (void)pgc_open_stats_old;
    static struct pgc_statistics pgc_extent_stats = {}, pgc_extent_stats_old = {}; (void)pgc_extent_stats_old;
    static struct mrg_statistics mrg_stats = {}, mrg_stats_old = {}; (void)mrg_stats_old;

    pgc_main_stats_old = pgc_main_stats;
    pgc_main_stats = pgc_get_statistics(main_cache);
    dbengine2_cache_statistics_charts(&main_cache_ptrs, &pgc_main_stats, &pgc_main_stats_old, "main", 135100);

    pgc_open_stats_old = pgc_open_stats;
    pgc_open_stats = pgc_get_statistics(open_cache);
    dbengine2_cache_statistics_charts(&open_cache_ptrs, &pgc_open_stats, &pgc_open_stats_old, "open", 135200);

    pgc_extent_stats_old = pgc_extent_stats;
    pgc_extent_stats = pgc_get_statistics(extent_cache);
    dbengine2_cache_statistics_charts(&extent_cache_ptrs, &pgc_extent_stats, &pgc_extent_stats_old, "extent", 135300);

    cache_efficiency_stats_old = cache_efficiency_stats;
    cache_efficiency_stats = rrdeng_get_cache_efficiency_stats();

    mrg_stats_old = mrg_stats;
    mrg_get_statistics(main_mrg, &mrg_stats);

    struct rrdeng_buffer_sizes buffers = rrdeng_get_buffer_sizes();
    size_t buffers_total_size = buffers.handles + buffers.xt_buf + buffers.xt_io + buffers.pdc + buffers.descriptors +
            buffers.opcodes + buffers.wal + buffers.workers + buffers.epdl + buffers.deol + buffers.pd + buffers.pgc + buffers.mrg;

#ifdef PDC_USE_JULYL
    buffers_total_size += buffers.julyl;
#endif

    dbengine_total_memory = pgc_main_stats.size + pgc_open_stats.size + pgc_extent_stats.size + mrg_stats.size + buffers_total_size;

    size_t priority = 135000;

    {
        static RRDSET *st_pgc_memory = NULL;
        static RRDDIM *rd_pgc_memory_main = NULL;
        static RRDDIM *rd_pgc_memory_open = NULL;  // open journal memory
        static RRDDIM *rd_pgc_memory_extent = NULL;  // extent compresses cache memory
        static RRDDIM *rd_pgc_memory_metrics = NULL;  // metric registry memory
        static RRDDIM *rd_pgc_memory_buffers = NULL;

        if (unlikely(!st_pgc_memory)) {
            st_pgc_memory = rrdset_create_localhost(
                    "netdata",
                    "dbengine_memory",
                    NULL,
                    "dbengine memory",
                    NULL,
                    "Netdata DB Memory",
                    "bytes",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

            rd_pgc_memory_main    = rrddim_add(st_pgc_memory, "main cache", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_memory_open    = rrddim_add(st_pgc_memory, "open cache",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_memory_extent  = rrddim_add(st_pgc_memory, "extent cache",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_memory_metrics = rrddim_add(st_pgc_memory, "metrics registry", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_memory_buffers = rrddim_add(st_pgc_memory, "buffers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;


        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_main, (collected_number)pgc_main_stats.size);
        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_open, (collected_number)pgc_open_stats.size);
        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_extent, (collected_number)pgc_extent_stats.size);
        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_metrics, (collected_number)mrg_stats.size);
        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_buffers, (collected_number)buffers_total_size);

        rrdset_done(st_pgc_memory);
    }

    {
        static RRDSET *st_pgc_buffers = NULL;
        static RRDDIM *rd_pgc_buffers_pgc = NULL;
        static RRDDIM *rd_pgc_buffers_mrg = NULL;
        static RRDDIM *rd_pgc_buffers_opcodes = NULL;
        static RRDDIM *rd_pgc_buffers_handles = NULL;
        static RRDDIM *rd_pgc_buffers_descriptors = NULL;
        static RRDDIM *rd_pgc_buffers_wal = NULL;
        static RRDDIM *rd_pgc_buffers_workers = NULL;
        static RRDDIM *rd_pgc_buffers_pdc = NULL;
        static RRDDIM *rd_pgc_buffers_xt_io = NULL;
        static RRDDIM *rd_pgc_buffers_xt_buf = NULL;
        static RRDDIM *rd_pgc_buffers_epdl = NULL;
        static RRDDIM *rd_pgc_buffers_deol = NULL;
        static RRDDIM *rd_pgc_buffers_pd = NULL;
#ifdef PDC_USE_JULYL
        static RRDDIM *rd_pgc_buffers_julyl = NULL;
#endif

        if (unlikely(!st_pgc_buffers)) {
            st_pgc_buffers = rrdset_create_localhost(
                    "netdata",
                    "dbengine_buffers",
                    NULL,
                    "dbengine memory",
                    NULL,
                    "Netdata DB Buffers",
                    "bytes",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

            rd_pgc_buffers_pgc         = rrddim_add(st_pgc_buffers, "pgc", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_mrg         = rrddim_add(st_pgc_buffers, "mrg", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_opcodes     = rrddim_add(st_pgc_buffers, "opcodes",        NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_handles     = rrddim_add(st_pgc_buffers, "query handles",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_descriptors = rrddim_add(st_pgc_buffers, "descriptors",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_wal         = rrddim_add(st_pgc_buffers, "wal",            NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_workers     = rrddim_add(st_pgc_buffers, "workers",        NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_pdc         = rrddim_add(st_pgc_buffers, "pdc",            NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_pd          = rrddim_add(st_pgc_buffers, "pd",             NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_xt_io       = rrddim_add(st_pgc_buffers, "extent io",      NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_xt_buf      = rrddim_add(st_pgc_buffers, "extent buffers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_epdl        = rrddim_add(st_pgc_buffers, "epdl",           NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_deol        = rrddim_add(st_pgc_buffers, "deol",           NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
#ifdef PDC_USE_JULYL
            rd_pgc_buffers_julyl       = rrddim_add(st_pgc_buffers, "julyl",          NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
#endif
        }
        priority++;

        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_pgc, (collected_number)buffers.pgc);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_mrg, (collected_number)buffers.mrg);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_opcodes, (collected_number)buffers.opcodes);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_handles, (collected_number)buffers.handles);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_descriptors, (collected_number)buffers.descriptors);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_wal, (collected_number)buffers.wal);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_workers, (collected_number)buffers.workers);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_pdc, (collected_number)buffers.pdc);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_pd, (collected_number)buffers.pd);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_xt_io, (collected_number)buffers.xt_io);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_xt_buf, (collected_number)buffers.xt_buf);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_epdl, (collected_number)buffers.epdl);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_deol, (collected_number)buffers.deol);
#ifdef PDC_USE_JULYL
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_julyl, (collected_number)buffers.julyl);
#endif

        rrdset_done(st_pgc_buffers);
    }

#ifdef PDC_USE_JULYL
    {
        static RRDSET *st_julyl_moved = NULL;
        static RRDDIM *rd_julyl_moved = NULL;

        if (unlikely(!st_julyl_moved)) {
            st_julyl_moved = rrdset_create_localhost(
                    "netdata",
                    "dbengine_julyl_moved",
                    NULL,
                    "dbengine memory",
                    NULL,
                    "Netdata JulyL Memory Moved",
                    "bytes/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_AREA);

            rd_julyl_moved     = rrddim_add(st_julyl_moved, "moved", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_julyl_moved, rd_julyl_moved, (collected_number)julyl_bytes_moved());

        rrdset_done(st_julyl_moved);
    }
#endif

    {
        static RRDSET *st_mrg_metrics = NULL;
        static RRDDIM *rd_mrg_metrics = NULL;
        static RRDDIM *rd_mrg_acquired = NULL;
        static RRDDIM *rd_mrg_collected = NULL;
        static RRDDIM *rd_mrg_multiple_writers = NULL;

        if (unlikely(!st_mrg_metrics)) {
            st_mrg_metrics = rrdset_create_localhost(
                    "netdata",
                    "dbengine_metrics",
                    NULL,
                    "dbengine metrics",
                    NULL,
                    "Netdata Metrics in Metrics Registry",
                    "metrics",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            rd_mrg_metrics = rrddim_add(st_mrg_metrics, "all", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_mrg_acquired = rrddim_add(st_mrg_metrics, "acquired", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_mrg_collected = rrddim_add(st_mrg_metrics, "collected", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_mrg_multiple_writers = rrddim_add(st_mrg_metrics, "multi-collected", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;

        rrddim_set_by_pointer(st_mrg_metrics, rd_mrg_metrics, (collected_number)mrg_stats.entries);
        rrddim_set_by_pointer(st_mrg_metrics, rd_mrg_acquired, (collected_number)mrg_stats.entries_referenced);
        rrddim_set_by_pointer(st_mrg_metrics, rd_mrg_collected, (collected_number)mrg_stats.writers);
        rrddim_set_by_pointer(st_mrg_metrics, rd_mrg_multiple_writers, (collected_number)mrg_stats.writers_conflicts);

        rrdset_done(st_mrg_metrics);
    }

    {
        static RRDSET *st_mrg_ops = NULL;
        static RRDDIM *rd_mrg_add = NULL;
        static RRDDIM *rd_mrg_del = NULL;
        static RRDDIM *rd_mrg_search = NULL;

        if (unlikely(!st_mrg_ops)) {
            st_mrg_ops = rrdset_create_localhost(
                    "netdata",
                    "dbengine_metrics_registry_operations",
                    NULL,
                    "dbengine metrics",
                    NULL,
                    "Netdata Metrics Registry Operations",
                    "metrics",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            rd_mrg_add = rrddim_add(st_mrg_ops, "add", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mrg_del = rrddim_add(st_mrg_ops, "delete", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mrg_search = rrddim_add(st_mrg_ops, "search", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_mrg_ops, rd_mrg_add, (collected_number)mrg_stats.additions);
        rrddim_set_by_pointer(st_mrg_ops, rd_mrg_del, (collected_number)mrg_stats.deletions);
        rrddim_set_by_pointer(st_mrg_ops, rd_mrg_search, (collected_number)mrg_stats.search_hits + (collected_number)mrg_stats.search_misses);

        rrdset_done(st_mrg_ops);
    }

    {
        static RRDSET *st_mrg_references = NULL;
        static RRDDIM *rd_mrg_references = NULL;

        if (unlikely(!st_mrg_references)) {
            st_mrg_references = rrdset_create_localhost(
                    "netdata",
                    "dbengine_metrics_registry_references",
                    NULL,
                    "dbengine metrics",
                    NULL,
                    "Netdata Metrics Registry References",
                    "references",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            rd_mrg_references = rrddim_add(st_mrg_references, "references", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;

        rrddim_set_by_pointer(st_mrg_references, rd_mrg_references, (collected_number)mrg_stats.current_references);

        rrdset_done(st_mrg_references);
    }

    {
        static RRDSET *st_cache_hit_ratio = NULL;
        static RRDDIM *rd_hit_ratio = NULL;
        static RRDDIM *rd_main_cache_hit_ratio = NULL;
        static RRDDIM *rd_extent_cache_hit_ratio = NULL;
        static RRDDIM *rd_extent_merge_hit_ratio = NULL;

        if (unlikely(!st_cache_hit_ratio)) {
            st_cache_hit_ratio = rrdset_create_localhost(
                    "netdata",
                    "dbengine_cache_hit_ratio",
                    NULL,
                    "dbengine query router",
                    NULL,
                    "Netdata Queries Cache Hit Ratio",
                    "%",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            rd_hit_ratio = rrddim_add(st_cache_hit_ratio, "overall", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
            rd_main_cache_hit_ratio = rrddim_add(st_cache_hit_ratio, "main cache", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
            rd_extent_cache_hit_ratio = rrddim_add(st_cache_hit_ratio, "extent cache", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
            rd_extent_merge_hit_ratio = rrddim_add(st_cache_hit_ratio, "extent merge", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;

        size_t delta_pages_total = cache_efficiency_stats.pages_total - cache_efficiency_stats_old.pages_total;
        size_t delta_pages_to_load_from_disk = cache_efficiency_stats.pages_to_load_from_disk - cache_efficiency_stats_old.pages_to_load_from_disk;
        size_t delta_extents_loaded_from_disk = cache_efficiency_stats.extents_loaded_from_disk - cache_efficiency_stats_old.extents_loaded_from_disk;

        size_t delta_pages_data_source_main_cache = cache_efficiency_stats.pages_data_source_main_cache - cache_efficiency_stats_old.pages_data_source_main_cache;
        size_t delta_pages_pending_found_in_cache_at_pass4 = cache_efficiency_stats.pages_data_source_main_cache_at_pass4 - cache_efficiency_stats_old.pages_data_source_main_cache_at_pass4;

        size_t delta_pages_data_source_extent_cache = cache_efficiency_stats.pages_data_source_extent_cache - cache_efficiency_stats_old.pages_data_source_extent_cache;
        size_t delta_pages_load_extent_merged = cache_efficiency_stats.pages_load_extent_merged - cache_efficiency_stats_old.pages_load_extent_merged;

        size_t pages_total_hit = delta_pages_total - delta_extents_loaded_from_disk;

        static size_t overall_hit_ratio = 100;
        size_t main_cache_hit_ratio = 0, extent_cache_hit_ratio = 0, extent_merge_hit_ratio = 0;
        if(delta_pages_total) {
            if(pages_total_hit > delta_pages_total)
                pages_total_hit = delta_pages_total;

            overall_hit_ratio = pages_total_hit * 100 * 10000 / delta_pages_total;

            size_t delta_pages_main_cache = delta_pages_data_source_main_cache + delta_pages_pending_found_in_cache_at_pass4;
            if(delta_pages_main_cache > delta_pages_total)
                delta_pages_main_cache = delta_pages_total;

            main_cache_hit_ratio = delta_pages_main_cache * 100 * 10000 / delta_pages_total;
        }

        if(delta_pages_to_load_from_disk) {
            if(delta_pages_data_source_extent_cache > delta_pages_to_load_from_disk)
                delta_pages_data_source_extent_cache = delta_pages_to_load_from_disk;

            extent_cache_hit_ratio = delta_pages_data_source_extent_cache * 100 * 10000 / delta_pages_to_load_from_disk;

            if(delta_pages_load_extent_merged > delta_pages_to_load_from_disk)
                delta_pages_load_extent_merged = delta_pages_to_load_from_disk;

            extent_merge_hit_ratio = delta_pages_load_extent_merged * 100 * 10000 / delta_pages_to_load_from_disk;
        }

        rrddim_set_by_pointer(st_cache_hit_ratio, rd_hit_ratio, (collected_number)overall_hit_ratio);
        rrddim_set_by_pointer(st_cache_hit_ratio, rd_main_cache_hit_ratio, (collected_number)main_cache_hit_ratio);
        rrddim_set_by_pointer(st_cache_hit_ratio, rd_extent_cache_hit_ratio, (collected_number)extent_cache_hit_ratio);
        rrddim_set_by_pointer(st_cache_hit_ratio, rd_extent_merge_hit_ratio, (collected_number)extent_merge_hit_ratio);

        rrdset_done(st_cache_hit_ratio);
    }

    {
        static RRDSET *st_queries = NULL;
        static RRDDIM *rd_total = NULL;
        static RRDDIM *rd_open = NULL;
        static RRDDIM *rd_jv2 = NULL;
        static RRDDIM *rd_planned_with_gaps = NULL;
        static RRDDIM *rd_executed_with_gaps = NULL;

        if (unlikely(!st_queries)) {
            st_queries = rrdset_create_localhost(
                    "netdata",
                    "dbengine_queries",
                    NULL,
                    "dbengine query router",
                    NULL,
                    "Netdata Queries",
                    "queries/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            rd_total = rrddim_add(st_queries, "total", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_open = rrddim_add(st_queries, "open cache", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_jv2 = rrddim_add(st_queries, "journal v2", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_planned_with_gaps = rrddim_add(st_queries, "planned with gaps", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_executed_with_gaps = rrddim_add(st_queries, "executed with gaps", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_queries, rd_total, (collected_number)cache_efficiency_stats.queries);
        rrddim_set_by_pointer(st_queries, rd_open, (collected_number)cache_efficiency_stats.queries_open);
        rrddim_set_by_pointer(st_queries, rd_jv2, (collected_number)cache_efficiency_stats.queries_journal_v2);
        rrddim_set_by_pointer(st_queries, rd_planned_with_gaps, (collected_number)cache_efficiency_stats.queries_planned_with_gaps);
        rrddim_set_by_pointer(st_queries, rd_executed_with_gaps, (collected_number)cache_efficiency_stats.queries_executed_with_gaps);

        rrdset_done(st_queries);
    }

    {
        static RRDSET *st_queries_running = NULL;
        static RRDDIM *rd_queries = NULL;

        if (unlikely(!st_queries_running)) {
            st_queries_running = rrdset_create_localhost(
                    "netdata",
                    "dbengine_queries_running",
                    NULL,
                    "dbengine query router",
                    NULL,
                    "Netdata Queries Running",
                    "queries",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            rd_queries = rrddim_add(st_queries_running, "queries", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;

        rrddim_set_by_pointer(st_queries_running, rd_queries, (collected_number)cache_efficiency_stats.currently_running_queries);

        rrdset_done(st_queries_running);
    }

    {
        static RRDSET *st_query_pages_metadata_source = NULL;
        static RRDDIM *rd_cache = NULL;
        static RRDDIM *rd_open = NULL;
        static RRDDIM *rd_jv2 = NULL;

        if (unlikely(!st_query_pages_metadata_source)) {
            st_query_pages_metadata_source = rrdset_create_localhost(
                    "netdata",
                    "dbengine_query_pages_metadata_source",
                    NULL,
                    "dbengine query router",
                    NULL,
                    "Netdata Query Pages Metadata Source",
                    "pages/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

            rd_cache = rrddim_add(st_query_pages_metadata_source, "cache hit", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_jv2   = rrddim_add(st_query_pages_metadata_source, "journal v2 scan", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_open  = rrddim_add(st_query_pages_metadata_source, "open journal", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_query_pages_metadata_source, rd_cache, (collected_number)cache_efficiency_stats.pages_meta_source_main_cache);
        rrddim_set_by_pointer(st_query_pages_metadata_source, rd_jv2, (collected_number)cache_efficiency_stats.pages_meta_source_journal_v2);
        rrddim_set_by_pointer(st_query_pages_metadata_source, rd_open, (collected_number)cache_efficiency_stats.pages_meta_source_open_cache);

        rrdset_done(st_query_pages_metadata_source);
    }

    {
        static RRDSET *st_query_pages_data_source = NULL;
        static RRDDIM *rd_pages_main_cache = NULL;
        static RRDDIM *rd_pages_disk = NULL;
        static RRDDIM *rd_pages_extent_cache = NULL;

        if (unlikely(!st_query_pages_data_source)) {
            st_query_pages_data_source = rrdset_create_localhost(
                    "netdata",
                    "dbengine_query_pages_data_source",
                    NULL,
                    "dbengine query router",
                    NULL,
                    "Netdata Query Pages to Data Source",
                    "pages/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

            rd_pages_main_cache = rrddim_add(st_query_pages_data_source, "main cache", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_disk = rrddim_add(st_query_pages_data_source, "disk", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_extent_cache = rrddim_add(st_query_pages_data_source, "extent cache", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_query_pages_data_source, rd_pages_main_cache, (collected_number)cache_efficiency_stats.pages_data_source_main_cache + (collected_number)cache_efficiency_stats.pages_data_source_main_cache_at_pass4);
        rrddim_set_by_pointer(st_query_pages_data_source, rd_pages_disk, (collected_number)cache_efficiency_stats.pages_to_load_from_disk);
        rrddim_set_by_pointer(st_query_pages_data_source, rd_pages_extent_cache, (collected_number)cache_efficiency_stats.pages_data_source_extent_cache);

        rrdset_done(st_query_pages_data_source);
    }

    {
        static RRDSET *st_query_next_page = NULL;
        static RRDDIM *rd_pass4 = NULL;
        static RRDDIM *rd_nowait_failed = NULL;
        static RRDDIM *rd_wait_failed = NULL;
        static RRDDIM *rd_wait_loaded = NULL;
        static RRDDIM *rd_nowait_loaded = NULL;

        if (unlikely(!st_query_next_page)) {
            st_query_next_page = rrdset_create_localhost(
                    "netdata",
                    "dbengine_query_next_page",
                    NULL,
                    "dbengine query router",
                    NULL,
                    "Netdata Query Next Page",
                    "pages/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

            rd_pass4 = rrddim_add(st_query_next_page, "pass4", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_wait_failed = rrddim_add(st_query_next_page, "failed slow", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_nowait_failed = rrddim_add(st_query_next_page, "failed fast", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_wait_loaded = rrddim_add(st_query_next_page, "loaded slow", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_nowait_loaded = rrddim_add(st_query_next_page, "loaded fast", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_query_next_page, rd_pass4, (collected_number)cache_efficiency_stats.pages_data_source_main_cache_at_pass4);
        rrddim_set_by_pointer(st_query_next_page, rd_wait_failed, (collected_number)cache_efficiency_stats.page_next_wait_failed);
        rrddim_set_by_pointer(st_query_next_page, rd_nowait_failed, (collected_number)cache_efficiency_stats.page_next_nowait_failed);
        rrddim_set_by_pointer(st_query_next_page, rd_wait_loaded, (collected_number)cache_efficiency_stats.page_next_wait_loaded);
        rrddim_set_by_pointer(st_query_next_page, rd_nowait_loaded, (collected_number)cache_efficiency_stats.page_next_nowait_loaded);

        rrdset_done(st_query_next_page);
    }

    {
        static RRDSET *st_query_page_issues = NULL;
        static RRDDIM *rd_pages_zero_time = NULL;
        static RRDDIM *rd_pages_past_time = NULL;
        static RRDDIM *rd_pages_invalid_size = NULL;
        static RRDDIM *rd_pages_fixed_update_every = NULL;
        static RRDDIM *rd_pages_fixed_entries = NULL;
        static RRDDIM *rd_pages_overlapping = NULL;

        if (unlikely(!st_query_page_issues)) {
            st_query_page_issues = rrdset_create_localhost(
                    "netdata",
                    "dbengine_query_next_page_issues",
                    NULL,
                    "dbengine query router",
                    NULL,
                    "Netdata Query Next Page Issues",
                    "pages/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

            rd_pages_zero_time = rrddim_add(st_query_page_issues, "zero timestamp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_invalid_size = rrddim_add(st_query_page_issues, "invalid size", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_past_time = rrddim_add(st_query_page_issues, "past time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_overlapping = rrddim_add(st_query_page_issues, "overlapping", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_fixed_update_every = rrddim_add(st_query_page_issues, "update every fixed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_fixed_entries = rrddim_add(st_query_page_issues, "entries fixed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_query_page_issues, rd_pages_zero_time, (collected_number)cache_efficiency_stats.pages_zero_time_skipped);
        rrddim_set_by_pointer(st_query_page_issues, rd_pages_invalid_size, (collected_number)cache_efficiency_stats.pages_invalid_size_skipped);
        rrddim_set_by_pointer(st_query_page_issues, rd_pages_past_time, (collected_number)cache_efficiency_stats.pages_past_time_skipped);
        rrddim_set_by_pointer(st_query_page_issues, rd_pages_overlapping, (collected_number)cache_efficiency_stats.pages_overlapping_skipped);
        rrddim_set_by_pointer(st_query_page_issues, rd_pages_fixed_update_every, (collected_number)cache_efficiency_stats.pages_invalid_update_every_fixed);
        rrddim_set_by_pointer(st_query_page_issues, rd_pages_fixed_entries, (collected_number)cache_efficiency_stats.pages_invalid_entries_fixed);

        rrdset_done(st_query_page_issues);
    }

    {
        static RRDSET *st_query_pages_from_disk = NULL;
        static RRDDIM *rd_compressed = NULL;
        static RRDDIM *rd_invalid = NULL;
        static RRDDIM *rd_uncompressed = NULL;
        static RRDDIM *rd_mmap_failed = NULL;
        static RRDDIM *rd_unavailable = NULL;
        static RRDDIM *rd_unroutable = NULL;
        static RRDDIM *rd_not_found = NULL;
        static RRDDIM *rd_cancelled = NULL;
        static RRDDIM *rd_invalid_extent = NULL;
        static RRDDIM *rd_extent_merged = NULL;

        if (unlikely(!st_query_pages_from_disk)) {
            st_query_pages_from_disk = rrdset_create_localhost(
                    "netdata",
                    "dbengine_query_pages_disk_load",
                    NULL,
                    "dbengine query router",
                    NULL,
                    "Netdata Query Pages Loaded from Disk",
                    "pages/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            rd_compressed = rrddim_add(st_query_pages_from_disk, "ok compressed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_invalid = rrddim_add(st_query_pages_from_disk, "fail invalid page", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_uncompressed = rrddim_add(st_query_pages_from_disk, "ok uncompressed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mmap_failed = rrddim_add(st_query_pages_from_disk, "fail cant mmap", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_unavailable = rrddim_add(st_query_pages_from_disk, "fail unavailable", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_unroutable = rrddim_add(st_query_pages_from_disk, "fail unroutable", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_not_found = rrddim_add(st_query_pages_from_disk, "fail not found", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_invalid_extent = rrddim_add(st_query_pages_from_disk, "fail invalid extent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_extent_merged = rrddim_add(st_query_pages_from_disk, "extent merged", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cancelled = rrddim_add(st_query_pages_from_disk, "cancelled", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_query_pages_from_disk, rd_compressed, (collected_number)cache_efficiency_stats.pages_load_ok_compressed);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_invalid, (collected_number)cache_efficiency_stats.pages_load_fail_invalid_page_in_extent);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_uncompressed, (collected_number)cache_efficiency_stats.pages_load_ok_uncompressed);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_mmap_failed, (collected_number)cache_efficiency_stats.pages_load_fail_cant_mmap_extent);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_unavailable, (collected_number)cache_efficiency_stats.pages_load_fail_datafile_not_available);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_unroutable, (collected_number)cache_efficiency_stats.pages_load_fail_unroutable);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_not_found, (collected_number)cache_efficiency_stats.pages_load_fail_not_found);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_cancelled, (collected_number)cache_efficiency_stats.pages_load_fail_cancelled);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_invalid_extent, (collected_number)cache_efficiency_stats.pages_load_fail_invalid_extent);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_extent_merged, (collected_number)cache_efficiency_stats.pages_load_extent_merged);

        rrdset_done(st_query_pages_from_disk);
    }

    {
        static RRDSET *st_events = NULL;
        static RRDDIM *rd_journal_v2_mapped = NULL;
        static RRDDIM *rd_journal_v2_unmapped = NULL;
        static RRDDIM *rd_datafile_creation = NULL;
        static RRDDIM *rd_datafile_deletion = NULL;
        static RRDDIM *rd_datafile_deletion_spin = NULL;
        static RRDDIM *rd_jv2_indexing = NULL;
        static RRDDIM *rd_retention = NULL;

        if (unlikely(!st_events)) {
            st_events = rrdset_create_localhost(
                    "netdata",
                    "dbengine_events",
                    NULL,
                    "dbengine query router",
                    NULL,
                    "Netdata Database Events",
                    "events/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

            rd_journal_v2_mapped = rrddim_add(st_events, "journal v2 mapped", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_journal_v2_unmapped = rrddim_add(st_events, "journal v2 unmapped", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_datafile_creation = rrddim_add(st_events, "datafile creation", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_datafile_deletion = rrddim_add(st_events, "datafile deletion", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_datafile_deletion_spin = rrddim_add(st_events, "datafile deletion spin", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_jv2_indexing = rrddim_add(st_events, "journal v2 indexing", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_retention = rrddim_add(st_events, "retention", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_events, rd_journal_v2_mapped, (collected_number)cache_efficiency_stats.journal_v2_mapped);
        rrddim_set_by_pointer(st_events, rd_journal_v2_unmapped, (collected_number)cache_efficiency_stats.journal_v2_unmapped);
        rrddim_set_by_pointer(st_events, rd_datafile_creation, (collected_number)cache_efficiency_stats.datafile_creation_started);
        rrddim_set_by_pointer(st_events, rd_datafile_deletion, (collected_number)cache_efficiency_stats.datafile_deletion_started);
        rrddim_set_by_pointer(st_events, rd_datafile_deletion_spin, (collected_number)cache_efficiency_stats.datafile_deletion_spin);
        rrddim_set_by_pointer(st_events, rd_jv2_indexing, (collected_number)cache_efficiency_stats.journal_v2_indexing_started);
        rrddim_set_by_pointer(st_events, rd_retention, (collected_number)cache_efficiency_stats.metrics_retention_started);

        rrdset_done(st_events);
    }

    {
        static RRDSET *st_prep_timings = NULL;
        static RRDDIM *rd_routing = NULL;
        static RRDDIM *rd_main_cache = NULL;
        static RRDDIM *rd_open_cache = NULL;
        static RRDDIM *rd_journal_v2 = NULL;
        static RRDDIM *rd_pass4 = NULL;

        if (unlikely(!st_prep_timings)) {
            st_prep_timings = rrdset_create_localhost(
                    "netdata",
                    "dbengine_prep_timings",
                    NULL,
                    "dbengine query router",
                    NULL,
                    "Netdata Query Preparation Timings",
                    "usec/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

            rd_routing = rrddim_add(st_prep_timings, "routing", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_main_cache = rrddim_add(st_prep_timings, "main cache", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_open_cache = rrddim_add(st_prep_timings, "open cache", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_journal_v2 = rrddim_add(st_prep_timings, "journal v2", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pass4 = rrddim_add(st_prep_timings, "pass4", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_prep_timings, rd_routing, (collected_number)cache_efficiency_stats.prep_time_to_route);
        rrddim_set_by_pointer(st_prep_timings, rd_main_cache, (collected_number)cache_efficiency_stats.prep_time_in_main_cache_lookup);
        rrddim_set_by_pointer(st_prep_timings, rd_open_cache, (collected_number)cache_efficiency_stats.prep_time_in_open_cache_lookup);
        rrddim_set_by_pointer(st_prep_timings, rd_journal_v2, (collected_number)cache_efficiency_stats.prep_time_in_journal_v2_lookup);
        rrddim_set_by_pointer(st_prep_timings, rd_pass4, (collected_number)cache_efficiency_stats.prep_time_in_pass4_lookup);

        rrdset_done(st_prep_timings);
    }

    {
        static RRDSET *st_query_timings = NULL;
        static RRDDIM *rd_init = NULL;
        static RRDDIM *rd_prep_wait = NULL;
        static RRDDIM *rd_next_page_disk_fast = NULL;
        static RRDDIM *rd_next_page_disk_slow = NULL;
        static RRDDIM *rd_next_page_preload_fast = NULL;
        static RRDDIM *rd_next_page_preload_slow = NULL;

        if (unlikely(!st_query_timings)) {
            st_query_timings = rrdset_create_localhost(
                    "netdata",
                    "dbengine_query_timings",
                    NULL,
                    "dbengine query router",
                    NULL,
                    "Netdata Query Timings",
                    "usec/s",
                    "netdata",
                    "stats",
                    priority,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

            rd_init = rrddim_add(st_query_timings, "init", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_prep_wait = rrddim_add(st_query_timings, "prep wait", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_next_page_disk_fast = rrddim_add(st_query_timings, "next page disk fast", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_next_page_disk_slow = rrddim_add(st_query_timings, "next page disk slow", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_next_page_preload_fast = rrddim_add(st_query_timings, "next page preload fast", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_next_page_preload_slow = rrddim_add(st_query_timings, "next page preload slow", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_query_timings, rd_init, (collected_number)cache_efficiency_stats.query_time_init);
        rrddim_set_by_pointer(st_query_timings, rd_prep_wait, (collected_number)cache_efficiency_stats.query_time_wait_for_prep);
        rrddim_set_by_pointer(st_query_timings, rd_next_page_disk_fast, (collected_number)cache_efficiency_stats.query_time_to_fast_disk_next_page);
        rrddim_set_by_pointer(st_query_timings, rd_next_page_disk_slow, (collected_number)cache_efficiency_stats.query_time_to_slow_disk_next_page);
        rrddim_set_by_pointer(st_query_timings, rd_next_page_preload_fast, (collected_number)cache_efficiency_stats.query_time_to_fast_preload_next_page);
        rrddim_set_by_pointer(st_query_timings, rd_next_page_preload_slow, (collected_number)cache_efficiency_stats.query_time_to_slow_preload_next_page);

        rrdset_done(st_query_timings);
    }

    if(netdata_rwlock_tryrdlock(&rrd_rwlock) == 0) {
        priority = 135400;

        RRDHOST *host;
        unsigned long long stats_array[RRDENG_NR_STATS] = {0};
        unsigned long long local_stats_array[RRDENG_NR_STATS];
        unsigned dbengine_contexts = 0, counted_multihost_db[RRD_STORAGE_TIERS] = { 0 }, i;

        rrdhost_foreach_read(host) {
            if (!rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {

                /* get localhost's DB engine's statistics for each tier */
                for(size_t tier = 0; tier < storage_tiers ;tier++) {
                    if(host->db[tier].mode != RRD_MEMORY_MODE_DBENGINE) continue;
                    if(!host->db[tier].si) continue;

                    if(is_storage_engine_shared(host->db[tier].si)) {
                        if(counted_multihost_db[tier])
                            continue;
                        else
                            counted_multihost_db[tier] = 1;
                    }

                    ++dbengine_contexts;
                    rrdeng_get_37_statistics((struct rrdengine_instance *)host->db[tier].si, local_stats_array);
                    for (i = 0; i < RRDENG_NR_STATS; ++i) {
                        /* aggregate statistics across hosts */
                        stats_array[i] += local_stats_array[i];
                    }
                }
            }
        }
        rrd_unlock();

        if (dbengine_contexts) {
            /* deduplicate global statistics by getting the ones from the last context */
            stats_array[30] = local_stats_array[30];
            stats_array[31] = local_stats_array[31];
            stats_array[32] = local_stats_array[32];
            stats_array[34] = local_stats_array[34];
            stats_array[36] = local_stats_array[36];

            // ----------------------------------------------------------------

            {
                static RRDSET *st_compression = NULL;
                static RRDDIM *rd_savings = NULL;

                if (unlikely(!st_compression)) {
                    st_compression = rrdset_create_localhost(
                            "netdata",
                            "dbengine_compression_ratio",
                            NULL,
                            "dbengine io",
                            NULL,
                            "Netdata DB engine data extents' compression savings ratio",
                            "percentage",
                            "netdata",
                            "stats",
                            priority,
                            localhost->rrd_update_every,
                            RRDSET_TYPE_LINE);

                    rd_savings = rrddim_add(st_compression, "savings", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                }
                priority++;

                unsigned long long ratio;
                unsigned long long compressed_content_size = stats_array[12];
                unsigned long long content_size = stats_array[11];

                if (content_size) {
                    // allow negative savings
                    ratio = ((content_size - compressed_content_size) * 100 * 1000) / content_size;
                } else {
                    ratio = 0;
                }
                rrddim_set_by_pointer(st_compression, rd_savings, ratio);

                rrdset_done(st_compression);
            }

            // ----------------------------------------------------------------

            {
                static RRDSET *st_io_stats = NULL;
                static RRDDIM *rd_reads = NULL;
                static RRDDIM *rd_writes = NULL;

                if (unlikely(!st_io_stats)) {
                    st_io_stats = rrdset_create_localhost(
                            "netdata",
                            "dbengine_io_throughput",
                            NULL,
                            "dbengine io",
                            NULL,
                            "Netdata DB engine I/O throughput",
                            "MiB/s",
                            "netdata",
                            "stats",
                            priority,
                            localhost->rrd_update_every,
                            RRDSET_TYPE_LINE);

                    rd_reads = rrddim_add(st_io_stats, "reads", NULL, 1, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                    rd_writes = rrddim_add(st_io_stats, "writes", NULL, -1, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                }
                priority++;

                rrddim_set_by_pointer(st_io_stats, rd_reads, (collected_number)stats_array[17]);
                rrddim_set_by_pointer(st_io_stats, rd_writes, (collected_number)stats_array[15]);
                rrdset_done(st_io_stats);
            }

            // ----------------------------------------------------------------

            {
                static RRDSET *st_io_stats = NULL;
                static RRDDIM *rd_reads = NULL;
                static RRDDIM *rd_writes = NULL;

                if (unlikely(!st_io_stats)) {
                    st_io_stats = rrdset_create_localhost(
                            "netdata",
                            "dbengine_io_operations",
                            NULL,
                            "dbengine io",
                            NULL,
                            "Netdata DB engine I/O operations",
                            "operations/s",
                            "netdata",
                            "stats",
                            priority,
                            localhost->rrd_update_every,
                            RRDSET_TYPE_LINE);

                    rd_reads = rrddim_add(st_io_stats, "reads", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_writes = rrddim_add(st_io_stats, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                priority++;

                rrddim_set_by_pointer(st_io_stats, rd_reads, (collected_number)stats_array[18]);
                rrddim_set_by_pointer(st_io_stats, rd_writes, (collected_number)stats_array[16]);
                rrdset_done(st_io_stats);
            }

            // ----------------------------------------------------------------

            {
                static RRDSET *st_errors = NULL;
                static RRDDIM *rd_fs_errors = NULL;
                static RRDDIM *rd_io_errors = NULL;
                static RRDDIM *pg_cache_over_half_dirty_events = NULL;

                if (unlikely(!st_errors)) {
                    st_errors = rrdset_create_localhost(
                            "netdata",
                            "dbengine_global_errors",
                            NULL,
                            "dbengine io",
                            NULL,
                            "Netdata DB engine errors",
                            "errors/s",
                            "netdata",
                            "stats",
                            priority,
                            localhost->rrd_update_every,
                            RRDSET_TYPE_LINE);

                    rd_io_errors = rrddim_add(st_errors, "io_errors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_fs_errors = rrddim_add(st_errors, "fs_errors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    pg_cache_over_half_dirty_events =
                            rrddim_add(st_errors, "pg_cache_over_half_dirty_events", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                priority++;

                rrddim_set_by_pointer(st_errors, rd_io_errors, (collected_number)stats_array[30]);
                rrddim_set_by_pointer(st_errors, rd_fs_errors, (collected_number)stats_array[31]);
                rrddim_set_by_pointer(st_errors, pg_cache_over_half_dirty_events, (collected_number)stats_array[34]);
                rrdset_done(st_errors);
            }

            // ----------------------------------------------------------------

            {
                static RRDSET *st_fd = NULL;
                static RRDDIM *rd_fd_current = NULL;
                static RRDDIM *rd_fd_max = NULL;

                if (unlikely(!st_fd)) {
                    st_fd = rrdset_create_localhost(
                            "netdata",
                            "dbengine_global_file_descriptors",
                            NULL,
                            "dbengine io",
                            NULL,
                            "Netdata DB engine File Descriptors",
                            "descriptors",
                            "netdata",
                            "stats",
                            priority,
                            localhost->rrd_update_every,
                            RRDSET_TYPE_LINE);

                    rd_fd_current = rrddim_add(st_fd, "current", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    rd_fd_max = rrddim_add(st_fd, "max", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                priority++;

                rrddim_set_by_pointer(st_fd, rd_fd_current, (collected_number)stats_array[32]);
                /* Careful here, modify this accordingly if the File-Descriptor budget ever changes */
                rrddim_set_by_pointer(st_fd, rd_fd_max, (collected_number)rlimit_nofile.rlim_cur / 4);
                rrdset_done(st_fd);
            }
        }
    }
}
#endif // ENABLE_DBENGINE

static void update_strings_charts() {
    static RRDSET *st_ops = NULL, *st_entries = NULL, *st_mem = NULL;
    static RRDDIM *rd_ops_inserts = NULL, *rd_ops_deletes = NULL;
    static RRDDIM *rd_entries_entries = NULL;
    static RRDDIM *rd_mem = NULL;
#ifdef NETDATA_INTERNAL_CHECKS
    static RRDDIM *rd_entries_refs = NULL, *rd_ops_releases = NULL,  *rd_ops_duplications = NULL, *rd_ops_searches = NULL;
#endif

    size_t inserts, deletes, searches, entries, references, memory, duplications, releases;

    string_statistics(&inserts, &deletes, &searches, &entries, &references, &memory, &duplications, &releases);

    if (unlikely(!st_ops)) {
        st_ops = rrdset_create_localhost(
            "netdata"
            , "strings_ops"
            , NULL
            , "strings"
            , NULL
            , "Strings operations"
            , "ops/s"
            , "netdata"
            , "stats"
            , 910000
            , localhost->rrd_update_every
            , RRDSET_TYPE_LINE);

        rd_ops_inserts      = rrddim_add(st_ops, "inserts",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_ops_deletes      = rrddim_add(st_ops, "deletes",      NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
#ifdef NETDATA_INTERNAL_CHECKS
        rd_ops_searches     = rrddim_add(st_ops, "searches",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_ops_duplications = rrddim_add(st_ops, "duplications", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_ops_releases     = rrddim_add(st_ops, "releases",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
#endif
    }

    rrddim_set_by_pointer(st_ops, rd_ops_inserts,      (collected_number)inserts);
    rrddim_set_by_pointer(st_ops, rd_ops_deletes,      (collected_number)deletes);
#ifdef NETDATA_INTERNAL_CHECKS
    rrddim_set_by_pointer(st_ops, rd_ops_searches,     (collected_number)searches);
    rrddim_set_by_pointer(st_ops, rd_ops_duplications, (collected_number)duplications);
    rrddim_set_by_pointer(st_ops, rd_ops_releases,     (collected_number)releases);
#endif
    rrdset_done(st_ops);

    if (unlikely(!st_entries)) {
        st_entries = rrdset_create_localhost(
            "netdata"
            , "strings_entries"
            , NULL
            , "strings"
            , NULL
            , "Strings entries"
            , "entries"
            , "netdata"
            , "stats"
            , 910001
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA);

        rd_entries_entries  = rrddim_add(st_entries, "entries", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
#ifdef NETDATA_INTERNAL_CHECKS
        rd_entries_refs  = rrddim_add(st_entries, "references", NULL, 1, -1, RRD_ALGORITHM_ABSOLUTE);
#endif
    }

    rrddim_set_by_pointer(st_entries, rd_entries_entries, (collected_number)entries);
#ifdef NETDATA_INTERNAL_CHECKS
    rrddim_set_by_pointer(st_entries, rd_entries_refs, (collected_number)references);
#endif
    rrdset_done(st_entries);

    if (unlikely(!st_mem)) {
        st_mem = rrdset_create_localhost(
            "netdata"
            , "strings_memory"
            , NULL
            , "strings"
            , NULL
            , "Strings memory"
            , "bytes"
            , "netdata"
            , "stats"
            , 910001
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA);

        rd_mem  = rrddim_add(st_mem, "memory", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_mem, rd_mem, (collected_number)memory);
    rrdset_done(st_mem);
}

static void update_heartbeat_charts() {
    static RRDSET *st_heartbeat = NULL;
    static RRDDIM *rd_heartbeat_min = NULL;
    static RRDDIM *rd_heartbeat_max = NULL;
    static RRDDIM *rd_heartbeat_avg = NULL;

    if (unlikely(!st_heartbeat)) {
        st_heartbeat = rrdset_create_localhost(
            "netdata"
            , "heartbeat"
            , NULL
            , "heartbeat"
            , NULL
            , "System clock jitter"
            , "microseconds"
            , "netdata"
            , "stats"
            , 900000
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA);

        rd_heartbeat_min = rrddim_add(st_heartbeat, "min", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_heartbeat_max = rrddim_add(st_heartbeat, "max", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_heartbeat_avg = rrddim_add(st_heartbeat, "average", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    usec_t min, max, average;
    size_t count;

    heartbeat_statistics(&min, &max, &average, &count);

    rrddim_set_by_pointer(st_heartbeat, rd_heartbeat_min, (collected_number)min);
    rrddim_set_by_pointer(st_heartbeat, rd_heartbeat_max, (collected_number)max);
    rrddim_set_by_pointer(st_heartbeat, rd_heartbeat_avg, (collected_number)average);

    rrdset_done(st_heartbeat);
}

// ---------------------------------------------------------------------------------------------------------------------
// dictionary statistics

struct dictionary_stats dictionary_stats_category_collectors = { .name = "collectors" };
struct dictionary_stats dictionary_stats_category_rrdhost = { .name = "rrdhost" };
struct dictionary_stats dictionary_stats_category_rrdset_rrddim = { .name = "rrdset_rrddim" };
struct dictionary_stats dictionary_stats_category_rrdcontext = { .name = "context" };
struct dictionary_stats dictionary_stats_category_rrdlabels = { .name = "labels" };
struct dictionary_stats dictionary_stats_category_rrdhealth = { .name = "health" };
struct dictionary_stats dictionary_stats_category_functions = { .name = "functions" };
struct dictionary_stats dictionary_stats_category_replication = { .name = "replication" };

#ifdef DICT_WITH_STATS
struct dictionary_categories {
    struct dictionary_stats *stats;
    const char *family;
    const char *context_prefix;
    int priority;

    RRDSET *st_dicts;
    RRDDIM *rd_dicts_active;
    RRDDIM *rd_dicts_deleted;

    RRDSET *st_items;
    RRDDIM *rd_items_entries;
    RRDDIM *rd_items_referenced;
    RRDDIM *rd_items_pending_deletion;

    RRDSET *st_ops;
    RRDDIM *rd_ops_creations;
    RRDDIM *rd_ops_destructions;
    RRDDIM *rd_ops_flushes;
    RRDDIM *rd_ops_traversals;
    RRDDIM *rd_ops_walkthroughs;
    RRDDIM *rd_ops_garbage_collections;
    RRDDIM *rd_ops_searches;
    RRDDIM *rd_ops_inserts;
    RRDDIM *rd_ops_resets;
    RRDDIM *rd_ops_deletes;

    RRDSET *st_callbacks;
    RRDDIM *rd_callbacks_inserts;
    RRDDIM *rd_callbacks_conflicts;
    RRDDIM *rd_callbacks_reacts;
    RRDDIM *rd_callbacks_deletes;

    RRDSET *st_memory;
    RRDDIM *rd_memory_indexed;
    RRDDIM *rd_memory_values;
    RRDDIM *rd_memory_dict;

    RRDSET *st_spins;
    RRDDIM *rd_spins_use;
    RRDDIM *rd_spins_search;
    RRDDIM *rd_spins_insert;
    RRDDIM *rd_spins_delete;

} dictionary_categories[] = {
    { .stats = &dictionary_stats_category_collectors, "dictionaries collectors", "dictionaries", 900000 },
    { .stats = &dictionary_stats_category_rrdhost, "dictionaries hosts", "dictionaries", 900000 },
    { .stats = &dictionary_stats_category_rrdset_rrddim, "dictionaries rrd", "dictionaries", 900000 },
    { .stats = &dictionary_stats_category_rrdcontext, "dictionaries contexts", "dictionaries", 900000 },
    { .stats = &dictionary_stats_category_rrdlabels, "dictionaries labels", "dictionaries", 900000 },
    { .stats = &dictionary_stats_category_rrdhealth, "dictionaries health", "dictionaries", 900000 },
    { .stats = &dictionary_stats_category_functions, "dictionaries functions", "dictionaries", 900000 },
    { .stats = &dictionary_stats_category_replication, "dictionaries replication", "dictionaries", 900000 },
    { .stats = &dictionary_stats_category_other, "dictionaries other", "dictionaries", 900000 },

    // terminator
    { .stats = NULL, NULL, NULL, 0 },
};

#define load_dictionary_stats_entry(x) total += (size_t)(stats.x = __atomic_load_n(&c->stats->x, __ATOMIC_RELAXED))

static void update_dictionary_category_charts(struct dictionary_categories *c) {
    struct dictionary_stats stats;
    stats.name = c->stats->name;

    // ------------------------------------------------------------------------

    size_t total = 0;
    load_dictionary_stats_entry(dictionaries.active);
    load_dictionary_stats_entry(dictionaries.deleted);

    if(c->st_dicts || total != 0) {
        if (unlikely(!c->st_dicts)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s.%s.dictionaries", c->context_prefix, stats.name);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintfz(context, RRD_ID_LENGTH_MAX, "netdata.%s.category.dictionaries", c->context_prefix);

            c->st_dicts = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , c->family
                , context
                , "Dictionaries"
                , "dictionaries"
                , "netdata"
                , "stats"
                , c->priority + 0
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            c->rd_dicts_active  = rrddim_add(c->st_dicts, "active",   NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            c->rd_dicts_deleted = rrddim_add(c->st_dicts, "deleted",   NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(c->st_dicts->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(c->st_dicts, c->rd_dicts_active,  (collected_number)stats.dictionaries.active);
        rrddim_set_by_pointer(c->st_dicts, c->rd_dicts_deleted, (collected_number)stats.dictionaries.deleted);
        rrdset_done(c->st_dicts);
    }

    // ------------------------------------------------------------------------

    total = 0;
    load_dictionary_stats_entry(items.entries);
    load_dictionary_stats_entry(items.referenced);
    load_dictionary_stats_entry(items.pending_deletion);

    if(c->st_items || total != 0) {
        if (unlikely(!c->st_items)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s.%s.items", c->context_prefix, stats.name);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintfz(context, RRD_ID_LENGTH_MAX, "netdata.%s.category.items", c->context_prefix);

            c->st_items = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , c->family
                , context
                , "Dictionary Items"
                , "items"
                , "netdata"
                , "stats"
                , c->priority + 1
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            c->rd_items_entries             = rrddim_add(c->st_items, "active",   NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            c->rd_items_pending_deletion    = rrddim_add(c->st_items, "deleted",   NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
            c->rd_items_referenced          = rrddim_add(c->st_items, "referenced",   NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(c->st_items->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(c->st_items, c->rd_items_entries,             stats.items.entries);
        rrddim_set_by_pointer(c->st_items, c->rd_items_pending_deletion,    stats.items.pending_deletion);
        rrddim_set_by_pointer(c->st_items, c->rd_items_referenced,          stats.items.referenced);
        rrdset_done(c->st_items);
    }

    // ------------------------------------------------------------------------

    total = 0;
    load_dictionary_stats_entry(ops.creations);
    load_dictionary_stats_entry(ops.destructions);
    load_dictionary_stats_entry(ops.flushes);
    load_dictionary_stats_entry(ops.traversals);
    load_dictionary_stats_entry(ops.walkthroughs);
    load_dictionary_stats_entry(ops.garbage_collections);
    load_dictionary_stats_entry(ops.searches);
    load_dictionary_stats_entry(ops.inserts);
    load_dictionary_stats_entry(ops.resets);
    load_dictionary_stats_entry(ops.deletes);

    if(c->st_ops || total != 0) {
        if (unlikely(!c->st_ops)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s.%s.ops", c->context_prefix, stats.name);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintfz(context, RRD_ID_LENGTH_MAX, "netdata.%s.category.ops", c->context_prefix);

            c->st_ops = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , c->family
                , context
                , "Dictionary Operations"
                , "ops/s"
                , "netdata"
                , "stats"
                , c->priority + 2
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            c->rd_ops_creations = rrddim_add(c->st_ops, "creations", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_destructions = rrddim_add(c->st_ops, "destructions", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_flushes = rrddim_add(c->st_ops, "flushes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_traversals = rrddim_add(c->st_ops, "traversals", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_walkthroughs = rrddim_add(c->st_ops, "walkthroughs", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_garbage_collections = rrddim_add(c->st_ops, "garbage_collections", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_searches = rrddim_add(c->st_ops, "searches", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_inserts = rrddim_add(c->st_ops, "inserts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_resets = rrddim_add(c->st_ops, "resets", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_deletes = rrddim_add(c->st_ops, "deletes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(c->st_ops->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(c->st_ops, c->rd_ops_creations,           (collected_number)stats.ops.creations);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_destructions,        (collected_number)stats.ops.destructions);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_flushes,             (collected_number)stats.ops.flushes);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_traversals,          (collected_number)stats.ops.traversals);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_walkthroughs,        (collected_number)stats.ops.walkthroughs);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_garbage_collections, (collected_number)stats.ops.garbage_collections);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_searches,            (collected_number)stats.ops.searches);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_inserts,             (collected_number)stats.ops.inserts);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_resets,              (collected_number)stats.ops.resets);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_deletes,             (collected_number)stats.ops.deletes);

        rrdset_done(c->st_ops);
    }

    // ------------------------------------------------------------------------

    total = 0;
    load_dictionary_stats_entry(callbacks.inserts);
    load_dictionary_stats_entry(callbacks.conflicts);
    load_dictionary_stats_entry(callbacks.reacts);
    load_dictionary_stats_entry(callbacks.deletes);

    if(c->st_callbacks || total != 0) {
        if (unlikely(!c->st_callbacks)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s.%s.callbacks", c->context_prefix, stats.name);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintfz(context, RRD_ID_LENGTH_MAX, "netdata.%s.category.callbacks", c->context_prefix);

            c->st_callbacks = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , c->family
                , context
                , "Dictionary Callbacks"
                , "callbacks/s"
                , "netdata"
                , "stats"
                , c->priority + 3
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            c->rd_callbacks_inserts = rrddim_add(c->st_callbacks, "inserts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_callbacks_deletes = rrddim_add(c->st_callbacks, "deletes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_callbacks_conflicts = rrddim_add(c->st_callbacks, "conflicts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_callbacks_reacts = rrddim_add(c->st_callbacks, "reacts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(c->st_callbacks->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(c->st_callbacks, c->rd_callbacks_inserts, (collected_number)stats.callbacks.inserts);
        rrddim_set_by_pointer(c->st_callbacks, c->rd_callbacks_conflicts, (collected_number)stats.callbacks.conflicts);
        rrddim_set_by_pointer(c->st_callbacks, c->rd_callbacks_reacts, (collected_number)stats.callbacks.reacts);
        rrddim_set_by_pointer(c->st_callbacks, c->rd_callbacks_deletes, (collected_number)stats.callbacks.deletes);

        rrdset_done(c->st_callbacks);
    }

    // ------------------------------------------------------------------------

    total = 0;
    load_dictionary_stats_entry(memory.index);
    load_dictionary_stats_entry(memory.values);
    load_dictionary_stats_entry(memory.dict);

    if(c->st_memory || total != 0) {
        if (unlikely(!c->st_memory)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s.%s.memory", c->context_prefix, stats.name);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintfz(context, RRD_ID_LENGTH_MAX, "netdata.%s.category.memory", c->context_prefix);

            c->st_memory = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , c->family
                , context
                , "Dictionary Memory"
                , "bytes"
                , "netdata"
                , "stats"
                , c->priority + 4
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
            );

            c->rd_memory_indexed = rrddim_add(c->st_memory, "index", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            c->rd_memory_values = rrddim_add(c->st_memory, "data", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            c->rd_memory_dict = rrddim_add(c->st_memory, "structures", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(c->st_memory->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(c->st_memory, c->rd_memory_indexed, (collected_number)stats.memory.index);
        rrddim_set_by_pointer(c->st_memory, c->rd_memory_values, (collected_number)stats.memory.values);
        rrddim_set_by_pointer(c->st_memory, c->rd_memory_dict, (collected_number)stats.memory.dict);

        rrdset_done(c->st_memory);
    }

    // ------------------------------------------------------------------------

    total = 0;
    load_dictionary_stats_entry(spin_locks.use_spins);
    load_dictionary_stats_entry(spin_locks.search_spins);
    load_dictionary_stats_entry(spin_locks.insert_spins);
    load_dictionary_stats_entry(spin_locks.delete_spins);

    if(c->st_spins || total != 0) {
        if (unlikely(!c->st_spins)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s.%s.spins", c->context_prefix, stats.name);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintfz(context, RRD_ID_LENGTH_MAX, "netdata.%s.category.spins", c->context_prefix);

            c->st_spins = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , c->family
                , context
                , "Dictionary Spins"
                , "count"
                , "netdata"
                , "stats"
                , c->priority + 5
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            c->rd_spins_use = rrddim_add(c->st_spins, "use", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_spins_search = rrddim_add(c->st_spins, "search", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_spins_insert = rrddim_add(c->st_spins, "insert", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_spins_delete = rrddim_add(c->st_spins, "delete", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(c->st_spins->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(c->st_spins, c->rd_spins_use, (collected_number)stats.spin_locks.use_spins);
        rrddim_set_by_pointer(c->st_spins, c->rd_spins_search, (collected_number)stats.spin_locks.search_spins);
        rrddim_set_by_pointer(c->st_spins, c->rd_spins_insert, (collected_number)stats.spin_locks.insert_spins);
        rrddim_set_by_pointer(c->st_spins, c->rd_spins_delete, (collected_number)stats.spin_locks.delete_spins);

        rrdset_done(c->st_spins);
    }
}

static void dictionary_statistics(void) {
    for(int i = 0; dictionary_categories[i].stats ;i++) {
        update_dictionary_category_charts(&dictionary_categories[i]);
    }
}
#endif // DICT_WITH_STATS

#ifdef NETDATA_TRACE_ALLOCATIONS

struct memory_trace_data {
    RRDSET *st_memory;
    RRDSET *st_allocations;
    RRDSET *st_avg_alloc;
    RRDSET *st_ops;
};

static int do_memory_trace_item(void *item, void *data) {
    struct memory_trace_data *tmp = data;
    struct malloc_trace *p = item;

    // ------------------------------------------------------------------------

    if(!p->rd_bytes)
        p->rd_bytes = rrddim_add(tmp->st_memory, p->function, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    collected_number bytes = (collected_number)__atomic_load_n(&p->bytes, __ATOMIC_RELAXED);
    rrddim_set_by_pointer(tmp->st_memory, p->rd_bytes, bytes);

    // ------------------------------------------------------------------------

    if(!p->rd_allocations)
        p->rd_allocations = rrddim_add(tmp->st_allocations, p->function, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    collected_number allocs = (collected_number)__atomic_load_n(&p->allocations, __ATOMIC_RELAXED);
    rrddim_set_by_pointer(tmp->st_allocations, p->rd_allocations, allocs);

    // ------------------------------------------------------------------------

    if(!p->rd_avg_alloc)
        p->rd_avg_alloc = rrddim_add(tmp->st_avg_alloc, p->function, NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);

    collected_number avg_alloc = (allocs)?(bytes * 100 / allocs):0;
    rrddim_set_by_pointer(tmp->st_avg_alloc, p->rd_avg_alloc, avg_alloc);

    // ------------------------------------------------------------------------

    if(!p->rd_ops)
        p->rd_ops = rrddim_add(tmp->st_ops, p->function, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    collected_number ops = 0;
    ops += (collected_number)__atomic_load_n(&p->malloc_calls, __ATOMIC_RELAXED);
    ops += (collected_number)__atomic_load_n(&p->calloc_calls, __ATOMIC_RELAXED);
    ops += (collected_number)__atomic_load_n(&p->realloc_calls, __ATOMIC_RELAXED);
    ops += (collected_number)__atomic_load_n(&p->strdup_calls, __ATOMIC_RELAXED);
    ops += (collected_number)__atomic_load_n(&p->free_calls, __ATOMIC_RELAXED);
    rrddim_set_by_pointer(tmp->st_ops, p->rd_ops, ops);

    // ------------------------------------------------------------------------

    return 1;
}
static void malloc_trace_statistics(void) {
    static struct memory_trace_data tmp = {
        .st_memory = NULL,
        .st_allocations = NULL,
        .st_avg_alloc = NULL,
        .st_ops = NULL,
    };

    if(!tmp.st_memory) {
        tmp.st_memory = rrdset_create_localhost(
            "netdata"
            , "memory_size"
            , NULL
            , "memory"
            , "netdata.memory.size"
            , "Netdata Memory Used by Function"
            , "bytes"
            , "netdata"
            , "stats"
            , 900000
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
        );
    }

    if(!tmp.st_ops) {
        tmp.st_ops = rrdset_create_localhost(
            "netdata"
            , "memory_operations"
            , NULL
            , "memory"
            , "netdata.memory.operations"
            , "Netdata Memory Operations by Function"
            , "ops/s"
            , "netdata"
            , "stats"
            , 900001
            , localhost->rrd_update_every
            , RRDSET_TYPE_LINE
        );
    }

    if(!tmp.st_allocations) {
        tmp.st_allocations = rrdset_create_localhost(
            "netdata"
            , "memory_allocations"
            , NULL
            , "memory"
            , "netdata.memory.allocations"
            , "Netdata Memory Allocations by Function"
            , "allocations"
            , "netdata"
            , "stats"
            , 900002
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
        );
    }

    if(!tmp.st_avg_alloc) {
        tmp.st_avg_alloc = rrdset_create_localhost(
            "netdata"
            , "memory_avg_alloc"
            , NULL
            , "memory"
            , "netdata.memory.avg_alloc"
            , "Netdata Average Allocation Size by Function"
            , "bytes"
            , "netdata"
            , "stats"
            , 900003
            , localhost->rrd_update_every
            , RRDSET_TYPE_LINE
        );
    }

    malloc_trace_walkthrough(do_memory_trace_item, &tmp);

    rrdset_done(tmp.st_memory);
    rrdset_done(tmp.st_ops);
    rrdset_done(tmp.st_allocations);
    rrdset_done(tmp.st_avg_alloc);
}
#endif

// ---------------------------------------------------------------------------------------------------------------------
// worker utilization

#define WORKERS_MIN_PERCENT_DEFAULT 10000.0

struct worker_job_type_gs {
    STRING *name;
    STRING *units;

    size_t jobs_started;
    usec_t busy_time;

    RRDDIM *rd_jobs_started;
    RRDDIM *rd_busy_time;

    WORKER_METRIC_TYPE type;
    NETDATA_DOUBLE min_value;
    NETDATA_DOUBLE max_value;
    NETDATA_DOUBLE sum_value;
    size_t count_value;

    RRDSET *st;
    RRDDIM *rd_min;
    RRDDIM *rd_max;
    RRDDIM *rd_avg;
};

struct worker_thread {
    pid_t pid;
    bool enabled;

    bool cpu_enabled;
    double cpu;

    kernel_uint_t utime;
    kernel_uint_t stime;

    kernel_uint_t utime_old;
    kernel_uint_t stime_old;

    usec_t collected_time;
    usec_t collected_time_old;

    size_t jobs_started;
    usec_t busy_time;

    struct worker_thread *next;
    struct worker_thread *prev;
};

struct worker_utilization {
    const char *name;
    const char *family;
    size_t priority;
    uint32_t flags;

    char *name_lowercase;

    struct worker_job_type_gs per_job_type[WORKER_UTILIZATION_MAX_JOB_TYPES];

    size_t workers_max_job_id;
    size_t workers_registered;
    size_t workers_busy;
    usec_t workers_total_busy_time;
    usec_t workers_total_duration;
    size_t workers_total_jobs_started;
    double workers_min_busy_time;
    double workers_max_busy_time;

    size_t workers_cpu_registered;
    double workers_cpu_min;
    double workers_cpu_max;
    double workers_cpu_total;

    struct worker_thread *threads;

    RRDSET *st_workers_time;
    RRDDIM *rd_workers_time_avg;
    RRDDIM *rd_workers_time_min;
    RRDDIM *rd_workers_time_max;

    RRDSET *st_workers_cpu;
    RRDDIM *rd_workers_cpu_avg;
    RRDDIM *rd_workers_cpu_min;
    RRDDIM *rd_workers_cpu_max;

    RRDSET *st_workers_threads;
    RRDDIM *rd_workers_threads_free;
    RRDDIM *rd_workers_threads_busy;

    RRDSET *st_workers_jobs_per_job_type;
    RRDSET *st_workers_busy_per_job_type;

    RRDDIM *rd_total_cpu_utilizaton;
};

static struct worker_utilization all_workers_utilization[] = {
    { .name = "STATS",       .family = "workers global statistics",       .priority = 1000000 },
    { .name = "HEALTH",      .family = "workers health alarms",           .priority = 1000000 },
    { .name = "MLTRAIN",     .family = "workers ML training",             .priority = 1000000 },
    { .name = "MLDETECT",    .family = "workers ML detection",            .priority = 1000000 },
    { .name = "STREAMRCV",   .family = "workers streaming receive",       .priority = 1000000 },
    { .name = "STREAMSND",   .family = "workers streaming send",          .priority = 1000000 },
    { .name = "DBENGINE",    .family = "workers dbengine instances",      .priority = 1000000 },
    { .name = "LIBUV",       .family = "workers libuv threadpool",        .priority = 1000000 },
    { .name = "WEB",         .family = "workers web server",              .priority = 1000000 },
    { .name = "ACLKQUERY",   .family = "workers aclk query",              .priority = 1000000 },
    { .name = "ACLKSYNC",    .family = "workers aclk host sync",          .priority = 1000000 },
    { .name = "METASYNC",    .family = "workers metadata sync",           .priority = 1000000 },
    { .name = "PLUGINSD",    .family = "workers plugins.d",               .priority = 1000000 },
    { .name = "STATSD",      .family = "workers plugin statsd",           .priority = 1000000 },
    { .name = "STATSDFLUSH", .family = "workers plugin statsd flush",     .priority = 1000000 },
    { .name = "PROC",        .family = "workers plugin proc",             .priority = 1000000 },
    { .name = "NETDEV",      .family = "workers plugin proc netdev",      .priority = 1000000 },
    { .name = "FREEBSD",     .family = "workers plugin freebsd",          .priority = 1000000 },
    { .name = "MACOS",       .family = "workers plugin macos",            .priority = 1000000 },
    { .name = "CGROUPS",     .family = "workers plugin cgroups",          .priority = 1000000 },
    { .name = "CGROUPSDISC", .family = "workers plugin cgroups find",     .priority = 1000000 },
    { .name = "DISKSPACE",   .family = "workers plugin diskspace",        .priority = 1000000 },
    { .name = "TC",          .family = "workers plugin tc",               .priority = 1000000 },
    { .name = "TIMEX",       .family = "workers plugin timex",            .priority = 1000000 },
    { .name = "IDLEJITTER",  .family = "workers plugin idlejitter",       .priority = 1000000 },
    { .name = "LOGSMANAGPLG",.family = "workers plugin logs management",  .priority = 1000000 },
    { .name = "RRDCONTEXT",  .family = "workers contexts",                .priority = 1000000 },
    { .name = "REPLICATION", .family = "workers replication sender",      .priority = 1000000 },
    { .name = "SERVICE",     .family = "workers service",                 .priority = 1000000 },
    { .name = "PROFILER",    .family = "workers profile",                 .priority = 1000000 },

    // has to be terminated with a NULL
    { .name = NULL,          .family = NULL       }
};

static void workers_total_cpu_utilization_chart(void) {
    size_t i, cpu_enabled = 0;
    for(i = 0; all_workers_utilization[i].name ;i++)
        if(all_workers_utilization[i].workers_cpu_registered) cpu_enabled++;

    if(!cpu_enabled) return;

    static RRDSET *st = NULL;

    if(!st) {
        st = rrdset_create_localhost(
            "netdata",
            "workers_cpu",
            NULL,
            "workers",
            "netdata.workers.cpu_total",
            "Netdata Workers CPU Utilization (100% = 1 core)",
            "%",
            "netdata",
            "stats",
            999000,
            localhost->rrd_update_every,
            RRDSET_TYPE_STACKED);
    }

    for(i = 0; all_workers_utilization[i].name ;i++) {
        struct worker_utilization *wu = &all_workers_utilization[i];
        if(!wu->workers_cpu_registered) continue;

        if(!wu->rd_total_cpu_utilizaton)
            wu->rd_total_cpu_utilizaton = rrddim_add(st, wu->name_lowercase, NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);

        rrddim_set_by_pointer(st, wu->rd_total_cpu_utilizaton, (collected_number)((double)wu->workers_cpu_total * 100.0));
    }

    rrdset_done(st);
}

#define WORKER_CHART_DECIMAL_PRECISION 100

static void workers_utilization_update_chart(struct worker_utilization *wu) {
    if(!wu->workers_registered) return;

    //fprintf(stderr, "%-12s WORKER UTILIZATION: %-3.2f%%, %zu jobs done, %zu running, on %zu workers, min %-3.02f%%, max %-3.02f%%.\n",
    //        wu->name,
    //        (double)wu->workers_total_busy_time * 100.0 / (double)wu->workers_total_duration,
    //        wu->workers_total_jobs_started, wu->workers_busy, wu->workers_registered,
    //        wu->workers_min_busy_time, wu->workers_max_busy_time);

    // ----------------------------------------------------------------------

    if(unlikely(!wu->st_workers_time)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_time_%s", wu->name_lowercase);

        char context[RRD_ID_LENGTH_MAX + 1];
        snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.time", wu->name_lowercase);

        wu->st_workers_time = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , context
            , "Netdata Workers Busy Time (100% = all workers busy)"
            , "%"
            , "netdata"
            , "stats"
            , wu->priority
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA
        );
    }

    // we add the min and max dimensions only when we have multiple workers

    if(unlikely(!wu->rd_workers_time_min && wu->workers_registered > 1))
        wu->rd_workers_time_min = rrddim_add(wu->st_workers_time, "min", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);

    if(unlikely(!wu->rd_workers_time_max && wu->workers_registered > 1))
        wu->rd_workers_time_max = rrddim_add(wu->st_workers_time, "max", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);

    if(unlikely(!wu->rd_workers_time_avg))
        wu->rd_workers_time_avg = rrddim_add(wu->st_workers_time, "average", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);

    if(unlikely(wu->workers_min_busy_time == WORKERS_MIN_PERCENT_DEFAULT)) wu->workers_min_busy_time = 0.0;

    if(wu->rd_workers_time_min)
        rrddim_set_by_pointer(wu->st_workers_time, wu->rd_workers_time_min, (collected_number)((double)wu->workers_min_busy_time * WORKER_CHART_DECIMAL_PRECISION));

    if(wu->rd_workers_time_max)
        rrddim_set_by_pointer(wu->st_workers_time, wu->rd_workers_time_max, (collected_number)((double)wu->workers_max_busy_time * WORKER_CHART_DECIMAL_PRECISION));

    if(wu->workers_total_duration == 0)
        rrddim_set_by_pointer(wu->st_workers_time, wu->rd_workers_time_avg, 0);
    else
        rrddim_set_by_pointer(wu->st_workers_time, wu->rd_workers_time_avg, (collected_number)((double)wu->workers_total_busy_time * 100.0 * WORKER_CHART_DECIMAL_PRECISION / (double)wu->workers_total_duration));

    rrdset_done(wu->st_workers_time);

    // ----------------------------------------------------------------------

#ifdef __linux__
    if(wu->workers_cpu_registered || wu->st_workers_cpu) {
        if(unlikely(!wu->st_workers_cpu)) {
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(name, RRD_ID_LENGTH_MAX, "workers_cpu_%s", wu->name_lowercase);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.cpu", wu->name_lowercase);

            wu->st_workers_cpu = rrdset_create_localhost(
                "netdata"
                , name
                , NULL
                , wu->family
                , context
                , "Netdata Workers CPU Utilization (100% = all workers busy)"
                , "%"
                , "netdata"
                , "stats"
                , wu->priority + 1
                , localhost->rrd_update_every
                , RRDSET_TYPE_AREA
            );
        }

        if (unlikely(!wu->rd_workers_cpu_min && wu->workers_registered > 1))
            wu->rd_workers_cpu_min = rrddim_add(wu->st_workers_cpu, "min", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);

        if (unlikely(!wu->rd_workers_cpu_max && wu->workers_registered > 1))
            wu->rd_workers_cpu_max = rrddim_add(wu->st_workers_cpu, "max", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);

        if(unlikely(!wu->rd_workers_cpu_avg))
            wu->rd_workers_cpu_avg = rrddim_add(wu->st_workers_cpu, "average", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);

        if(unlikely(wu->workers_cpu_min == WORKERS_MIN_PERCENT_DEFAULT)) wu->workers_cpu_min = 0.0;

        if(wu->rd_workers_cpu_min)
            rrddim_set_by_pointer(wu->st_workers_cpu, wu->rd_workers_cpu_min, (collected_number)(wu->workers_cpu_min * WORKER_CHART_DECIMAL_PRECISION));

        if(wu->rd_workers_cpu_max)
            rrddim_set_by_pointer(wu->st_workers_cpu, wu->rd_workers_cpu_max, (collected_number)(wu->workers_cpu_max * WORKER_CHART_DECIMAL_PRECISION));

        if(wu->workers_cpu_registered == 0)
            rrddim_set_by_pointer(wu->st_workers_cpu, wu->rd_workers_cpu_avg, 0);
        else
            rrddim_set_by_pointer(wu->st_workers_cpu, wu->rd_workers_cpu_avg, (collected_number)( wu->workers_cpu_total * WORKER_CHART_DECIMAL_PRECISION / (NETDATA_DOUBLE)wu->workers_cpu_registered ));

        rrdset_done(wu->st_workers_cpu);
    }
#endif

    // ----------------------------------------------------------------------

    if(unlikely(!wu->st_workers_jobs_per_job_type)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_jobs_by_type_%s", wu->name_lowercase);

        char context[RRD_ID_LENGTH_MAX + 1];
        snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.jobs_started_by_type", wu->name_lowercase);

        wu->st_workers_jobs_per_job_type = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , context
            , "Netdata Workers Jobs Started by Type"
            , "jobs"
            , "netdata"
            , "stats"
            , wu->priority + 2
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
        );
    }

    {
        size_t i;
        for(i = 0; i <= wu->workers_max_job_id ;i++) {
            if(unlikely(wu->per_job_type[i].type != WORKER_METRIC_IDLE_BUSY))
                continue;

            if (wu->per_job_type[i].name) {

                if(unlikely(!wu->per_job_type[i].rd_jobs_started))
                    wu->per_job_type[i].rd_jobs_started = rrddim_add(wu->st_workers_jobs_per_job_type, string2str(wu->per_job_type[i].name), NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrddim_set_by_pointer(wu->st_workers_jobs_per_job_type, wu->per_job_type[i].rd_jobs_started, (collected_number)(wu->per_job_type[i].jobs_started));
            }
        }
    }

    rrdset_done(wu->st_workers_jobs_per_job_type);

    // ----------------------------------------------------------------------

    if(unlikely(!wu->st_workers_busy_per_job_type)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_busy_time_by_type_%s", wu->name_lowercase);

        char context[RRD_ID_LENGTH_MAX + 1];
        snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.time_by_type", wu->name_lowercase);

        wu->st_workers_busy_per_job_type = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , context
            , "Netdata Workers Busy Time by Type"
            , "ms"
            , "netdata"
            , "stats"
            , wu->priority + 3
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
        );
    }

    {
        size_t i;
        for(i = 0; i <= wu->workers_max_job_id ;i++) {
            if(unlikely(wu->per_job_type[i].type != WORKER_METRIC_IDLE_BUSY))
                continue;

            if (wu->per_job_type[i].name) {

                if(unlikely(!wu->per_job_type[i].rd_busy_time))
                    wu->per_job_type[i].rd_busy_time = rrddim_add(wu->st_workers_busy_per_job_type, string2str(wu->per_job_type[i].name), NULL, 1, USEC_PER_MS, RRD_ALGORITHM_ABSOLUTE);

                rrddim_set_by_pointer(wu->st_workers_busy_per_job_type, wu->per_job_type[i].rd_busy_time, (collected_number)(wu->per_job_type[i].busy_time));
            }
        }
    }

    rrdset_done(wu->st_workers_busy_per_job_type);

    // ----------------------------------------------------------------------

    if(wu->st_workers_threads || wu->workers_registered > 1) {
        if(unlikely(!wu->st_workers_threads)) {
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(name, RRD_ID_LENGTH_MAX, "workers_threads_%s", wu->name_lowercase);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.threads", wu->name_lowercase);

            wu->st_workers_threads = rrdset_create_localhost(
                "netdata"
                , name
                , NULL
                , wu->family
                , context
                , "Netdata Workers Threads"
                , "threads"
                , "netdata"
                , "stats"
                , wu->priority + 4
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
            );

            wu->rd_workers_threads_free = rrddim_add(wu->st_workers_threads, "free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            wu->rd_workers_threads_busy = rrddim_add(wu->st_workers_threads, "busy", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(wu->st_workers_threads, wu->rd_workers_threads_free, (collected_number)(wu->workers_registered - wu->workers_busy));
        rrddim_set_by_pointer(wu->st_workers_threads, wu->rd_workers_threads_busy, (collected_number)(wu->workers_busy));
        rrdset_done(wu->st_workers_threads);
    }

    // ----------------------------------------------------------------------
    // custom metric types WORKER_METRIC_ABSOLUTE

    {
        size_t i;
        for (i = 0; i <= wu->workers_max_job_id; i++) {
            if(wu->per_job_type[i].type != WORKER_METRIC_ABSOLUTE)
                continue;

            if(!wu->per_job_type[i].count_value)
                continue;

            if(!wu->per_job_type[i].st) {
                size_t job_name_len = string_strlen(wu->per_job_type[i].name);
                if(job_name_len > RRD_ID_LENGTH_MAX) job_name_len = RRD_ID_LENGTH_MAX;

                char job_name_sanitized[job_name_len + 1];
                rrdset_strncpyz_name(job_name_sanitized, string2str(wu->per_job_type[i].name), job_name_len);

                char name[RRD_ID_LENGTH_MAX + 1];
                snprintfz(name, RRD_ID_LENGTH_MAX, "workers_%s_value_%s", wu->name_lowercase, job_name_sanitized);

                char context[RRD_ID_LENGTH_MAX + 1];
                snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.value.%s", wu->name_lowercase, job_name_sanitized);

                char title[1000 + 1];
                snprintf(title, 1000, "Netdata Workers %s value of %s", wu->name_lowercase, string2str(wu->per_job_type[i].name));

                wu->per_job_type[i].st = rrdset_create_localhost(
                    "netdata"
                    , name
                    , NULL
                    , wu->family
                    , context
                    , title
                    , (wu->per_job_type[i].units)?string2str(wu->per_job_type[i].units):"value"
                    , "netdata"
                    , "stats"
                    , wu->priority + 5 + i
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
                    );

                wu->per_job_type[i].rd_min = rrddim_add(wu->per_job_type[i].st, "min", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
                wu->per_job_type[i].rd_max = rrddim_add(wu->per_job_type[i].st, "max", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
                wu->per_job_type[i].rd_avg = rrddim_add(wu->per_job_type[i].st, "average", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(wu->per_job_type[i].st, wu->per_job_type[i].rd_min, (collected_number)(wu->per_job_type[i].min_value * WORKER_CHART_DECIMAL_PRECISION));
            rrddim_set_by_pointer(wu->per_job_type[i].st, wu->per_job_type[i].rd_max, (collected_number)(wu->per_job_type[i].max_value * WORKER_CHART_DECIMAL_PRECISION));
            rrddim_set_by_pointer(wu->per_job_type[i].st, wu->per_job_type[i].rd_avg, (collected_number)(wu->per_job_type[i].sum_value / wu->per_job_type[i].count_value * WORKER_CHART_DECIMAL_PRECISION));

            rrdset_done(wu->per_job_type[i].st);
        }
    }

    // ----------------------------------------------------------------------
    // custom metric types WORKER_METRIC_INCREMENTAL

    {
        size_t i;
        for (i = 0; i <= wu->workers_max_job_id ; i++) {
            if(wu->per_job_type[i].type != WORKER_METRIC_INCREMENT && wu->per_job_type[i].type != WORKER_METRIC_INCREMENTAL_TOTAL)
                continue;

            if(!wu->per_job_type[i].count_value)
                continue;

            if(!wu->per_job_type[i].st) {
                size_t job_name_len = string_strlen(wu->per_job_type[i].name);
                if(job_name_len > RRD_ID_LENGTH_MAX) job_name_len = RRD_ID_LENGTH_MAX;

                char job_name_sanitized[job_name_len + 1];
                rrdset_strncpyz_name(job_name_sanitized, string2str(wu->per_job_type[i].name), job_name_len);

                char name[RRD_ID_LENGTH_MAX + 1];
                snprintfz(name, RRD_ID_LENGTH_MAX, "workers_%s_rate_%s", wu->name_lowercase, job_name_sanitized);

                char context[RRD_ID_LENGTH_MAX + 1];
                snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.rate.%s", wu->name_lowercase, job_name_sanitized);

                char title[1000 + 1];
                snprintf(title, 1000, "Netdata Workers %s rate of %s", wu->name_lowercase, string2str(wu->per_job_type[i].name));

                wu->per_job_type[i].st = rrdset_create_localhost(
                    "netdata"
                    , name
                    , NULL
                    , wu->family
                    , context
                    , title
                    , (wu->per_job_type[i].units)?string2str(wu->per_job_type[i].units):"rate"
                    , "netdata"
                    , "stats"
                    , wu->priority + 5 + i
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
                );

                wu->per_job_type[i].rd_min = rrddim_add(wu->per_job_type[i].st, "min", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
                wu->per_job_type[i].rd_max = rrddim_add(wu->per_job_type[i].st, "max", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
                wu->per_job_type[i].rd_avg = rrddim_add(wu->per_job_type[i].st, "average", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(wu->per_job_type[i].st, wu->per_job_type[i].rd_min, (collected_number)(wu->per_job_type[i].min_value * WORKER_CHART_DECIMAL_PRECISION));
            rrddim_set_by_pointer(wu->per_job_type[i].st, wu->per_job_type[i].rd_max, (collected_number)(wu->per_job_type[i].max_value * WORKER_CHART_DECIMAL_PRECISION));
            rrddim_set_by_pointer(wu->per_job_type[i].st, wu->per_job_type[i].rd_avg, (collected_number)(wu->per_job_type[i].sum_value / wu->per_job_type[i].count_value * WORKER_CHART_DECIMAL_PRECISION));

            rrdset_done(wu->per_job_type[i].st);
        }
    }
}

static void workers_utilization_reset_statistics(struct worker_utilization *wu) {
    wu->workers_registered = 0;
    wu->workers_busy = 0;
    wu->workers_total_busy_time = 0;
    wu->workers_total_duration = 0;
    wu->workers_total_jobs_started = 0;
    wu->workers_min_busy_time = WORKERS_MIN_PERCENT_DEFAULT;
    wu->workers_max_busy_time = 0;

    wu->workers_cpu_registered = 0;
    wu->workers_cpu_min = WORKERS_MIN_PERCENT_DEFAULT;
    wu->workers_cpu_max = 0;
    wu->workers_cpu_total = 0;

    size_t i;
    for(i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES ;i++) {
        if(unlikely(!wu->name_lowercase)) {
            wu->name_lowercase = strdupz(wu->name);
            char *s = wu->name_lowercase;
            for( ; *s ; s++) *s = tolower(*s);
        }

        wu->per_job_type[i].jobs_started = 0;
        wu->per_job_type[i].busy_time = 0;

        wu->per_job_type[i].min_value = NAN;
        wu->per_job_type[i].max_value = NAN;
        wu->per_job_type[i].sum_value = NAN;
        wu->per_job_type[i].count_value = 0;
    }

    struct worker_thread *wt;
    for(wt = wu->threads; wt ; wt = wt->next) {
        wt->enabled = false;
        wt->cpu_enabled = false;
    }
}

#define TASK_STAT_PREFIX "/proc/self/task/"
#define TASK_STAT_SUFFIX "/stat"

static int read_thread_cpu_time_from_proc_stat(pid_t pid __maybe_unused, kernel_uint_t *utime __maybe_unused, kernel_uint_t *stime __maybe_unused) {
#ifdef __linux__
    static char filename[sizeof(TASK_STAT_PREFIX) + sizeof(TASK_STAT_SUFFIX) + 20] = TASK_STAT_PREFIX;
    static size_t start_pos = sizeof(TASK_STAT_PREFIX) - 1;
    static procfile *ff = NULL;

    // construct the filename
    size_t end_pos = snprintfz(&filename[start_pos], 20, "%d", pid);
    strcpy(&filename[start_pos + end_pos], TASK_STAT_SUFFIX);

    // (re)open the procfile to the new filename
    bool set_quotes = (ff == NULL) ? true : false;
    ff = procfile_reopen(ff, filename, NULL, PROCFILE_FLAG_ERROR_ON_ERROR_LOG);
    if(unlikely(!ff)) return -1;

    if(set_quotes)
        procfile_set_open_close(ff, "(", ")");

    // read the entire file and split it to lines and words
    ff = procfile_readall(ff);
    if(unlikely(!ff)) return -1;

    // parse the numbers we are interested
    *utime = str2kernel_uint_t(procfile_lineword(ff, 0, 13));
    *stime = str2kernel_uint_t(procfile_lineword(ff, 0, 14));

    // leave the file open for the next iteration

    return 0;
#else
    // TODO: add here cpu time detection per thread, for FreeBSD and MacOS
    *utime = 0;
    *stime = 0;
    return 1;
#endif
}

static Pvoid_t workers_by_pid_JudyL_array = NULL;

static void workers_threads_cleanup(struct worker_utilization *wu) {
    struct worker_thread *t = wu->threads;
    while(t) {
        struct worker_thread *next = t->next;

        if(!t->enabled) {
            JudyLDel(&workers_by_pid_JudyL_array, t->pid, PJE0);
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wu->threads, t, prev, next);
            freez(t);
        }
        t = next;
    }
 }

static struct worker_thread *worker_thread_find(struct worker_utilization *wu __maybe_unused, pid_t pid) {
    struct worker_thread *wt = NULL;

    Pvoid_t *PValue = JudyLGet(workers_by_pid_JudyL_array, pid, PJE0);
    if(PValue)
        wt = *PValue;

    return wt;
}

static struct worker_thread *worker_thread_create(struct worker_utilization *wu, pid_t pid) {
    struct worker_thread *wt;

    wt = (struct worker_thread *)callocz(1, sizeof(struct worker_thread));
    wt->pid = pid;

    Pvoid_t *PValue = JudyLIns(&workers_by_pid_JudyL_array, pid, PJE0);
    *PValue = wt;

    // link it
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(wu->threads, wt, prev, next);

    return wt;
}

static struct worker_thread *worker_thread_find_or_create(struct worker_utilization *wu, pid_t pid) {
    struct worker_thread *wt;
    wt = worker_thread_find(wu, pid);
    if(!wt) wt = worker_thread_create(wu, pid);

    return wt;
}

static void worker_utilization_charts_callback(void *ptr
                                               , pid_t pid __maybe_unused
                                               , const char *thread_tag __maybe_unused
                                               , size_t max_job_id __maybe_unused
                                               , size_t utilization_usec __maybe_unused
                                               , size_t duration_usec __maybe_unused
                                               , size_t jobs_started __maybe_unused
                                               , size_t is_running __maybe_unused
                                               , STRING **job_types_names __maybe_unused
                                               , STRING **job_types_units __maybe_unused
                                               , WORKER_METRIC_TYPE *job_types_metric_types __maybe_unused
                                               , size_t *job_types_jobs_started __maybe_unused
                                               , usec_t *job_types_busy_time __maybe_unused
                                               , NETDATA_DOUBLE *job_types_custom_metrics __maybe_unused
                                               ) {
    struct worker_utilization *wu = (struct worker_utilization *)ptr;

    // find the worker_thread in the list
    struct worker_thread *wt = worker_thread_find_or_create(wu, pid);

    if(utilization_usec > duration_usec)
        utilization_usec = duration_usec;

    wt->enabled = true;
    wt->busy_time = utilization_usec;
    wt->jobs_started = jobs_started;

    wt->utime_old = wt->utime;
    wt->stime_old = wt->stime;
    wt->collected_time_old = wt->collected_time;

    if(max_job_id > wu->workers_max_job_id)
        wu->workers_max_job_id = max_job_id;

    wu->workers_total_busy_time += utilization_usec;
    wu->workers_total_duration += duration_usec;
    wu->workers_total_jobs_started += jobs_started;
    wu->workers_busy += is_running;
    wu->workers_registered++;

    double util = (double)utilization_usec * 100.0 / (double)duration_usec;
    if(util > wu->workers_max_busy_time)
        wu->workers_max_busy_time = util;

    if(util < wu->workers_min_busy_time)
        wu->workers_min_busy_time = util;

    // accumulate per job type statistics
    size_t i;
    for(i = 0; i <= max_job_id ;i++) {
        if(!wu->per_job_type[i].name && job_types_names[i])
            wu->per_job_type[i].name = string_dup(job_types_names[i]);

        if(!wu->per_job_type[i].units && job_types_units[i])
            wu->per_job_type[i].units = string_dup(job_types_units[i]);

        wu->per_job_type[i].type = job_types_metric_types[i];

        wu->per_job_type[i].jobs_started += job_types_jobs_started[i];
        wu->per_job_type[i].busy_time += job_types_busy_time[i];

        NETDATA_DOUBLE value = job_types_custom_metrics[i];
        if(netdata_double_isnumber(value)) {
            if(!wu->per_job_type[i].count_value) {
                wu->per_job_type[i].count_value = 1;
                wu->per_job_type[i].min_value = value;
                wu->per_job_type[i].max_value = value;
                wu->per_job_type[i].sum_value = value;
            }
            else {
                wu->per_job_type[i].count_value++;
                wu->per_job_type[i].sum_value += value;
                if(value < wu->per_job_type[i].min_value) wu->per_job_type[i].min_value = value;
                if(value > wu->per_job_type[i].max_value) wu->per_job_type[i].max_value = value;
            }
        }
    }

    // find its CPU utilization
    if((!read_thread_cpu_time_from_proc_stat(pid, &wt->utime, &wt->stime))) {
        wt->collected_time = now_realtime_usec();
        usec_t delta = wt->collected_time - wt->collected_time_old;

        double utime = (double)(wt->utime - wt->utime_old) / (double)system_hz * 100.0 * (double)USEC_PER_SEC / (double)delta;
        double stime = (double)(wt->stime - wt->stime_old) / (double)system_hz * 100.0 * (double)USEC_PER_SEC / (double)delta;
        double cpu = utime + stime;
        wt->cpu = cpu;
        wt->cpu_enabled = true;

        wu->workers_cpu_total += cpu;
        if(cpu < wu->workers_cpu_min) wu->workers_cpu_min = cpu;
        if(cpu > wu->workers_cpu_max) wu->workers_cpu_max = cpu;
    }
    wu->workers_cpu_registered += (wt->cpu_enabled) ? 1 : 0;
}

static void worker_utilization_charts(void) {
    static size_t iterations = 0;
    iterations++;

    for(int i = 0; all_workers_utilization[i].name ;i++) {
        workers_utilization_reset_statistics(&all_workers_utilization[i]);

        netdata_thread_disable_cancelability();
        workers_foreach(all_workers_utilization[i].name, worker_utilization_charts_callback, &all_workers_utilization[i]);
        netdata_thread_enable_cancelability();

        // skip the first iteration, so that we don't accumulate startup utilization to our charts
        if(likely(iterations > 1))
            workers_utilization_update_chart(&all_workers_utilization[i]);

        netdata_thread_disable_cancelability();
        workers_threads_cleanup(&all_workers_utilization[i]);
        netdata_thread_enable_cancelability();
    }

    workers_total_cpu_utilization_chart();
}

static void worker_utilization_finish(void) {
    int i, j;
    for(i = 0; all_workers_utilization[i].name ;i++) {
        struct worker_utilization *wu = &all_workers_utilization[i];

        if(wu->name_lowercase) {
            freez(wu->name_lowercase);
            wu->name_lowercase = NULL;
        }

        for(j = 0; j < WORKER_UTILIZATION_MAX_JOB_TYPES ;j++) {
            string_freez(wu->per_job_type[j].name);
            wu->per_job_type[j].name = NULL;

            string_freez(wu->per_job_type[j].units);
            wu->per_job_type[j].units = NULL;
        }

        // mark all threads as not enabled
        struct worker_thread *t;
        for(t = wu->threads; t ; t = t->next)
            t->enabled = false;

        // let the cleanup job free them
        workers_threads_cleanup(wu);
    }
}

// ---------------------------------------------------------------------------------------------------------------------
// global statistics thread


static void global_statistics_register_workers(void) {
    worker_register("STATS");
    worker_register_job_name(WORKER_JOB_GLOBAL, "global");
    worker_register_job_name(WORKER_JOB_REGISTRY, "registry");
    worker_register_job_name(WORKER_JOB_DBENGINE, "dbengine");
    worker_register_job_name(WORKER_JOB_STRINGS, "strings");
    worker_register_job_name(WORKER_JOB_DICTIONARIES, "dictionaries");
    worker_register_job_name(WORKER_JOB_MALLOC_TRACE, "malloc_trace");
    worker_register_job_name(WORKER_JOB_WORKERS, "workers");
    worker_register_job_name(WORKER_JOB_SQLITE3, "sqlite3");
}

static void global_statistics_cleanup(void *ptr)
{
    worker_unregister();

    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    netdata_log_info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *global_statistics_main(void *ptr)
{
    global_statistics_register_workers();

    netdata_thread_cleanup_push(global_statistics_cleanup, ptr);

    int update_every =
        (int)config_get_number(CONFIG_SECTION_GLOBAL_STATISTICS, "update every", localhost->rrd_update_every);
    if (update_every < localhost->rrd_update_every)
        update_every = localhost->rrd_update_every;

    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);

    // keep the randomness at zero
    // to make sure we are not close to any other thread
    hb.randomness = 0;

    while (service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        heartbeat_next(&hb, step);

        worker_is_busy(WORKER_JOB_GLOBAL);
        global_statistics_charts();

        worker_is_busy(WORKER_JOB_REGISTRY);
        registry_statistics();

#ifdef ENABLE_DBENGINE
        if(dbengine_enabled) {
            worker_is_busy(WORKER_JOB_DBENGINE);
            dbengine2_statistics_charts();
        }
#endif

        worker_is_busy(WORKER_JOB_HEARTBEAT);
        update_heartbeat_charts();
        
        worker_is_busy(WORKER_JOB_STRINGS);
        update_strings_charts();

#ifdef DICT_WITH_STATS
        worker_is_busy(WORKER_JOB_DICTIONARIES);
        dictionary_statistics();
#endif

#ifdef NETDATA_TRACE_ALLOCATIONS
        worker_is_busy(WORKER_JOB_MALLOC_TRACE);
        malloc_trace_statistics();
#endif
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}


// ---------------------------------------------------------------------------------------------------------------------
// workers thread

static void global_statistics_workers_cleanup(void *ptr)
{
    worker_unregister();

    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    netdata_log_info("cleaning up...");

    worker_utilization_finish();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *global_statistics_workers_main(void *ptr)
{
    global_statistics_register_workers();

    netdata_thread_cleanup_push(global_statistics_workers_cleanup, ptr)
    {
        int update_every =
                (int)config_get_number(CONFIG_SECTION_GLOBAL_STATISTICS, "update every", localhost->rrd_update_every);
        if (update_every < localhost->rrd_update_every)
            update_every = localhost->rrd_update_every;

        usec_t step = update_every * USEC_PER_SEC;
        heartbeat_t hb;
        heartbeat_init(&hb);

        while (service_running(SERVICE_COLLECTORS)) {
            worker_is_idle();
            heartbeat_next(&hb, step);

            worker_is_busy(WORKER_JOB_WORKERS);
            worker_utilization_charts();
        }
    }
    netdata_thread_cleanup_pop(1);
    return NULL;
}

// ---------------------------------------------------------------------------------------------------------------------
// sqlite3 thread

static void global_statistics_sqlite3_cleanup(void *ptr)
{
    worker_unregister();

    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    netdata_log_info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *global_statistics_sqlite3_main(void *ptr)
{
    global_statistics_register_workers();

    netdata_thread_cleanup_push(global_statistics_sqlite3_cleanup, ptr)
    {

        int update_every =
                (int)config_get_number(CONFIG_SECTION_GLOBAL_STATISTICS, "update every", localhost->rrd_update_every);
        if (update_every < localhost->rrd_update_every)
            update_every = localhost->rrd_update_every;

        usec_t step = update_every * USEC_PER_SEC;
        heartbeat_t hb;
        heartbeat_init(&hb);

        while (service_running(SERVICE_COLLECTORS)) {
            worker_is_idle();
            heartbeat_next(&hb, step);

            worker_is_busy(WORKER_JOB_SQLITE3);
            sqlite3_statistics_charts();
        }
    }
    netdata_thread_cleanup_pop(1);
    return NULL;
}

