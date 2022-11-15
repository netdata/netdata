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

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 8
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 5
#endif

bool global_statistics_enabled = true;

static struct global_statistics {
    volatile uint16_t connected_clients;

    volatile uint64_t web_requests;
    volatile uint64_t web_usec;
    volatile uint64_t web_usec_max;
    volatile uint64_t bytes_received;
    volatile uint64_t bytes_sent;
    volatile uint64_t content_size;
    volatile uint64_t compressed_content_size;

    volatile uint64_t web_client_count;

    volatile uint64_t rrdr_queries_made;
    volatile uint64_t rrdr_db_points_read;
    volatile uint64_t rrdr_result_points_generated;

    volatile uint64_t sqlite3_queries_made;
    volatile uint64_t sqlite3_queries_ok;
    volatile uint64_t sqlite3_queries_failed;
    volatile uint64_t sqlite3_queries_failed_busy;
    volatile uint64_t sqlite3_queries_failed_locked;
    volatile uint64_t sqlite3_rows;
    volatile uint64_t sqlite3_metadata_cache_hit;
    volatile uint64_t sqlite3_context_cache_hit;
    volatile uint64_t sqlite3_metadata_cache_miss;
    volatile uint64_t sqlite3_context_cache_miss;
    volatile uint64_t sqlite3_metadata_cache_spill;
    volatile uint64_t sqlite3_context_cache_spill;
    volatile uint64_t sqlite3_metadata_cache_write;
    volatile uint64_t sqlite3_context_cache_write;

} global_statistics = {
        .connected_clients = 0,
        .web_requests = 0,
        .web_usec = 0,
        .bytes_received = 0,
        .bytes_sent = 0,
        .content_size = 0,
        .compressed_content_size = 0,
        .web_client_count = 1,

        .rrdr_queries_made = 0,
        .rrdr_db_points_read = 0,
        .rrdr_result_points_generated = 0,
};

void sqlite3_query_completed(bool success, bool busy, bool locked) {
    __atomic_fetch_add(&global_statistics.sqlite3_queries_made, 1, __ATOMIC_RELAXED);

    if(success) {
        __atomic_fetch_add(&global_statistics.sqlite3_queries_ok, 1, __ATOMIC_RELAXED);
    }
    else {
        __atomic_fetch_add(&global_statistics.sqlite3_queries_failed, 1, __ATOMIC_RELAXED);

        if(busy)
            __atomic_fetch_add(&global_statistics.sqlite3_queries_failed_busy, 1, __ATOMIC_RELAXED);

        if(locked)
            __atomic_fetch_add(&global_statistics.sqlite3_queries_failed_locked, 1, __ATOMIC_RELAXED);
    }
}

void sqlite3_row_completed(void) {
    __atomic_fetch_add(&global_statistics.sqlite3_rows, 1, __ATOMIC_RELAXED);
}

void rrdr_query_completed(uint64_t db_points_read, uint64_t result_points_generated) {
    __atomic_fetch_add(&global_statistics.rrdr_queries_made, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&global_statistics.rrdr_db_points_read, db_points_read, __ATOMIC_RELAXED);
    __atomic_fetch_add(&global_statistics.rrdr_result_points_generated, result_points_generated, __ATOMIC_RELAXED);
}

void finished_web_request_statistics(uint64_t dt,
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

uint64_t web_client_connected(void) {
    __atomic_fetch_add(&global_statistics.connected_clients, 1, __ATOMIC_RELAXED);
    return __atomic_fetch_add(&global_statistics.web_client_count, 1, __ATOMIC_RELAXED);
}

void web_client_disconnected(void) {
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

    gs->rrdr_queries_made            = __atomic_load_n(&global_statistics.rrdr_queries_made, __ATOMIC_RELAXED);
    gs->rrdr_db_points_read          = __atomic_load_n(&global_statistics.rrdr_db_points_read, __ATOMIC_RELAXED);
    gs->rrdr_result_points_generated = __atomic_load_n(&global_statistics.rrdr_result_points_generated, __ATOMIC_RELAXED);

    if(options & GLOBAL_STATS_RESET_WEB_USEC_MAX) {
        uint64_t n = 0;
        __atomic_compare_exchange(&global_statistics.web_usec_max, (uint64_t *) &gs->web_usec_max, &n, 1, __ATOMIC_RELAXED, __ATOMIC_RELAXED);
    }

    gs->sqlite3_queries_made          = __atomic_load_n(&global_statistics.sqlite3_queries_made, __ATOMIC_RELAXED);
    gs->sqlite3_queries_ok            = __atomic_load_n(&global_statistics.sqlite3_queries_ok, __ATOMIC_RELAXED);
    gs->sqlite3_queries_failed        = __atomic_load_n(&global_statistics.sqlite3_queries_failed, __ATOMIC_RELAXED);
    gs->sqlite3_queries_failed_busy   = __atomic_load_n(&global_statistics.sqlite3_queries_failed_busy, __ATOMIC_RELAXED);
    gs->sqlite3_queries_failed_locked = __atomic_load_n(&global_statistics.sqlite3_queries_failed_locked, __ATOMIC_RELAXED);
    gs->sqlite3_rows                  = __atomic_load_n(&global_statistics.sqlite3_rows, __ATOMIC_RELAXED);

    gs->sqlite3_metadata_cache_hit  = (uint64_t) sql_metadata_cache_stats(SQLITE_DBSTATUS_CACHE_HIT);
    gs->sqlite3_context_cache_hit   = (uint64_t) sql_context_cache_stats(SQLITE_DBSTATUS_CACHE_HIT);

    gs->sqlite3_metadata_cache_miss = (uint64_t) sql_metadata_cache_stats(SQLITE_DBSTATUS_CACHE_MISS);
    gs->sqlite3_context_cache_miss  = (uint64_t) sql_context_cache_stats(SQLITE_DBSTATUS_CACHE_MISS);

    gs->sqlite3_metadata_cache_spill = (uint64_t) sql_metadata_cache_stats(SQLITE_DBSTATUS_CACHE_SPILL);
    gs->sqlite3_context_cache_spill  = (uint64_t) sql_context_cache_stats(SQLITE_DBSTATUS_CACHE_SPILL);

    gs->sqlite3_metadata_cache_write = (uint64_t) sql_metadata_cache_stats(SQLITE_DBSTATUS_CACHE_WRITE);
    gs->sqlite3_context_cache_write  = (uint64_t) sql_context_cache_stats(SQLITE_DBSTATUS_CACHE_WRITE);
}

static void global_statistics_charts(void) {
    static unsigned long long old_web_requests = 0,
                              old_web_usec = 0,
                              old_content_size = 0,
                              old_compressed_content_size = 0;

    static collected_number compression_ratio = -1,
                            average_response_time = -1;

    static time_t netdata_start_time = 0;
    if (!netdata_start_time)
        netdata_start_time = now_boottime_sec();
    time_t netdata_uptime = now_boottime_sec() - netdata_start_time;

    struct global_statistics gs;
    struct rusage me;

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
        else
            rrdset_next(st_cpu);

        rrddim_set_by_pointer(st_cpu, rd_cpu_user,   me.ru_utime.tv_sec * 1000000ULL + me.ru_utime.tv_usec);
        rrddim_set_by_pointer(st_cpu, rd_cpu_system, me.ru_stime.tv_sec * 1000000ULL + me.ru_stime.tv_usec);
        rrdset_done(st_cpu);
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
                130100,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rd_uptime = rrddim_add(st_uptime, "uptime", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        } else
            rrdset_next(st_uptime);

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
        else
            rrdset_next(st_clients);

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
        else
            rrdset_next(st_reqs);

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
        else
            rrdset_next(st_bytes);

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
        else
            rrdset_next(st_duration);

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
        else
            rrdset_next(st_compression);

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

    if(gs.rrdr_queries_made) {
        static RRDSET *st_rrdr_queries = NULL;
        static RRDDIM *rd_queries = NULL;

        if (unlikely(!st_rrdr_queries)) {
            st_rrdr_queries = rrdset_create_localhost(
                    "netdata"
                    , "queries"
                    , NULL
                    , "queries"
                    , NULL
                    , "Netdata API Queries"
                    , "queries/s"
                    , "netdata"
                    , "stats"
                    , 131000
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
            );

            rd_queries = rrddim_add(st_rrdr_queries, "queries", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else
            rrdset_next(st_rrdr_queries);

        rrddim_set_by_pointer(st_rrdr_queries, rd_queries, (collected_number)gs.rrdr_queries_made);

        rrdset_done(st_rrdr_queries);
    }

    // ----------------------------------------------------------------

    if(gs.rrdr_db_points_read || gs.rrdr_result_points_generated) {
        static RRDSET *st_rrdr_points = NULL;
        static RRDDIM *rd_points_read = NULL;
        static RRDDIM *rd_points_generated = NULL;

        if (unlikely(!st_rrdr_points)) {
            st_rrdr_points = rrdset_create_localhost(
                    "netdata"
                    , "db_points"
                    , NULL
                    , "queries"
                    , NULL
                    , "Netdata API Points"
                    , "points/s"
                    , "netdata"
                    , "stats"
                    , 131001
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_AREA
            );

            rd_points_read = rrddim_add(st_rrdr_points, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_points_generated = rrddim_add(st_rrdr_points, "generated", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else
            rrdset_next(st_rrdr_points);

        rrddim_set_by_pointer(st_rrdr_points, rd_points_read, (collected_number)gs.rrdr_db_points_read);
        rrddim_set_by_pointer(st_rrdr_points, rd_points_generated, (collected_number)gs.rrdr_result_points_generated);

        rrdset_done(st_rrdr_points);
    }

    // ----------------------------------------------------------------

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
        else
            rrdset_next(st_sqlite3_queries);

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
        else
            rrdset_next(st_sqlite3_queries_by_status);

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
        else
            rrdset_next(st_sqlite3_rows);

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
        else
            rrdset_next(st_sqlite3_cache);

        rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_hit, (collected_number)gs.sqlite3_metadata_cache_hit);
        rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_miss, (collected_number)gs.sqlite3_metadata_cache_miss);
        rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_spill, (collected_number)gs.sqlite3_metadata_cache_spill);
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
        else
            rrdset_next(st_sqlite3_cache);

        rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_hit, (collected_number)gs.sqlite3_context_cache_hit);
        rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_miss, (collected_number)gs.sqlite3_context_cache_miss);
        rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_spill, (collected_number)gs.sqlite3_context_cache_spill);
        rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_write, (collected_number)gs.sqlite3_context_cache_write);

        rrdset_done(st_sqlite3_cache);
    }

    // ----------------------------------------------------------------
}

static void dbengine_statistics_charts(void) {
#ifdef ENABLE_DBENGINE
    if(netdata_rwlock_tryrdlock(&rrd_rwlock) == 0) {
        RRDHOST *host;
        unsigned long long stats_array[RRDENG_NR_STATS] = {0};
        unsigned long long local_stats_array[RRDENG_NR_STATS];
        unsigned dbengine_contexts = 0, counted_multihost_db[RRD_STORAGE_TIERS] = { 0 }, i;

        rrdhost_foreach_read(host) {
            if (!rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {

                /* get localhost's DB engine's statistics for each tier */
                for(size_t tier = 0; tier < storage_tiers ;tier++) {
                    if(host->db[tier].mode != RRD_MEMORY_MODE_DBENGINE) continue;
                    if(!host->db[tier].instance) continue;

                    if(is_storage_engine_shared(host->db[tier].instance)) {
                        if(counted_multihost_db[tier])
                            continue;
                        else
                            counted_multihost_db[tier] = 1;
                    }

                    ++dbengine_contexts;
                    rrdeng_get_37_statistics((struct rrdengine_instance *)host->db[tier].instance, local_stats_array);
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
                        "dbengine",
                        NULL,
                        "Netdata DB engine data extents' compression savings ratio",
                        "percentage",
                        "netdata",
                        "stats",
                        132000,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_savings = rrddim_add(st_compression, "savings", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                } else
                    rrdset_next(st_compression);

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
                static RRDSET *st_pg_cache_hit_ratio = NULL;
                static RRDDIM *rd_hit_ratio = NULL;

                if (unlikely(!st_pg_cache_hit_ratio)) {
                    st_pg_cache_hit_ratio = rrdset_create_localhost(
                        "netdata",
                        "page_cache_hit_ratio",
                        NULL,
                        "dbengine",
                        NULL,
                        "Netdata DB engine page cache hit ratio",
                        "percentage",
                        "netdata",
                        "stats",
                        132003,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_hit_ratio = rrddim_add(st_pg_cache_hit_ratio, "ratio", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                } else
                    rrdset_next(st_pg_cache_hit_ratio);

                static unsigned long long old_hits = 0;
                static unsigned long long old_misses = 0;
                unsigned long long hits = stats_array[7];
                unsigned long long misses = stats_array[8];
                unsigned long long hits_delta;
                unsigned long long misses_delta;
                unsigned long long ratio;

                hits_delta = hits - old_hits;
                misses_delta = misses - old_misses;
                old_hits = hits;
                old_misses = misses;

                if (hits_delta + misses_delta) {
                    ratio = (hits_delta * 100 * 1000) / (hits_delta + misses_delta);
                } else {
                    ratio = 0;
                }
                rrddim_set_by_pointer(st_pg_cache_hit_ratio, rd_hit_ratio, ratio);

                rrdset_done(st_pg_cache_hit_ratio);
            }

            // ----------------------------------------------------------------

            {
                static RRDSET *st_pg_cache_pages = NULL;
                static RRDDIM *rd_descriptors = NULL;
                static RRDDIM *rd_populated = NULL;
                static RRDDIM *rd_dirty = NULL;
                static RRDDIM *rd_backfills = NULL;
                static RRDDIM *rd_evictions = NULL;
                static RRDDIM *rd_used_by_collectors = NULL;

                if (unlikely(!st_pg_cache_pages)) {
                    st_pg_cache_pages = rrdset_create_localhost(
                        "netdata",
                        "page_cache_stats",
                        NULL,
                        "dbengine",
                        NULL,
                        "Netdata dbengine page cache statistics",
                        "pages",
                        "netdata",
                        "stats",
                        132004,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_descriptors = rrddim_add(st_pg_cache_pages, "descriptors", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    rd_populated = rrddim_add(st_pg_cache_pages, "populated", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    rd_dirty = rrddim_add(st_pg_cache_pages, "dirty", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    rd_backfills = rrddim_add(st_pg_cache_pages, "backfills", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_evictions = rrddim_add(st_pg_cache_pages, "evictions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_used_by_collectors =
                        rrddim_add(st_pg_cache_pages, "used_by_collectors", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                } else
                    rrdset_next(st_pg_cache_pages);

                rrddim_set_by_pointer(st_pg_cache_pages, rd_descriptors, (collected_number)stats_array[27]);
                rrddim_set_by_pointer(st_pg_cache_pages, rd_populated, (collected_number)stats_array[3]);
                rrddim_set_by_pointer(st_pg_cache_pages, rd_dirty, (collected_number)stats_array[0] + stats_array[4]);
                rrddim_set_by_pointer(st_pg_cache_pages, rd_backfills, (collected_number)stats_array[9]);
                rrddim_set_by_pointer(st_pg_cache_pages, rd_evictions, (collected_number)stats_array[10]);
                rrddim_set_by_pointer(st_pg_cache_pages, rd_used_by_collectors, (collected_number)stats_array[0]);
                rrdset_done(st_pg_cache_pages);
            }

            // ----------------------------------------------------------------

            {
                static RRDSET *st_long_term_pages = NULL;
                static RRDDIM *rd_memory = NULL;
                static RRDDIM *rd_total = NULL;
                static RRDDIM *rd_insertions = NULL;
                static RRDDIM *rd_deletions = NULL;
                static RRDDIM *rd_flushing_pressure_deletions = NULL;

                if (unlikely(!st_long_term_pages)) {
                    st_long_term_pages = rrdset_create_localhost(
                        "netdata",
                        "dbengine_long_term_page_stats",
                        NULL,
                        "dbengine",
                        NULL,
                        "Netdata dbengine long-term page statistics",
                        "pages",
                        "netdata",
                        "stats",
                        132005,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_memory = rrddim_add(st_long_term_pages, "journal v2 descriptors", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    rd_total = rrddim_add(st_long_term_pages, "total", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    rd_insertions = rrddim_add(st_long_term_pages, "insertions", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_deletions = rrddim_add(st_long_term_pages, "deletions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_flushing_pressure_deletions = rrddim_add(
                        st_long_term_pages, "flushing_pressure_deletions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st_long_term_pages);

                rrddim_set_by_pointer(st_long_term_pages, rd_memory, (collected_number)stats_array[37]);
                rrddim_set_by_pointer(st_long_term_pages, rd_total, (collected_number)stats_array[2]);
                rrddim_set_by_pointer(st_long_term_pages, rd_insertions, (collected_number)stats_array[5]);
                rrddim_set_by_pointer(st_long_term_pages, rd_deletions, (collected_number)stats_array[6]);
                rrddim_set_by_pointer(
                    st_long_term_pages, rd_flushing_pressure_deletions, (collected_number)stats_array[36]);
                rrdset_done(st_long_term_pages);
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
                        "dbengine",
                        NULL,
                        "Netdata DB engine I/O throughput",
                        "MiB/s",
                        "netdata",
                        "stats",
                        132006,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_reads = rrddim_add(st_io_stats, "reads", NULL, 1, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                    rd_writes = rrddim_add(st_io_stats, "writes", NULL, -1, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st_io_stats);

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
                        "dbengine",
                        NULL,
                        "Netdata DB engine I/O operations",
                        "operations/s",
                        "netdata",
                        "stats",
                        132007,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_reads = rrddim_add(st_io_stats, "reads", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_writes = rrddim_add(st_io_stats, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st_io_stats);

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
                        "dbengine",
                        NULL,
                        "Netdata DB engine errors",
                        "errors/s",
                        "netdata",
                        "stats",
                        132008,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_io_errors = rrddim_add(st_errors, "io_errors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_fs_errors = rrddim_add(st_errors, "fs_errors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    pg_cache_over_half_dirty_events =
                        rrddim_add(st_errors, "pg_cache_over_half_dirty_events", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st_errors);

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
                        "dbengine",
                        NULL,
                        "Netdata DB engine File Descriptors",
                        "descriptors",
                        "netdata",
                        "stats",
                        132009,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_fd_current = rrddim_add(st_fd, "current", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    rd_fd_max = rrddim_add(st_fd, "max", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                } else
                    rrdset_next(st_fd);

                rrddim_set_by_pointer(st_fd, rd_fd_current, (collected_number)stats_array[32]);
                /* Careful here, modify this accordingly if the File-Descriptor budget ever changes */
                rrddim_set_by_pointer(st_fd, rd_fd_max, (collected_number)rlimit_nofile.rlim_cur / 4);
                rrdset_done(st_fd);
            }

            // ----------------------------------------------------------------

            {
                static RRDSET *st_ram_usage = NULL;
                static RRDDIM *rd_cached = NULL;
                static RRDDIM *rd_pinned = NULL;
                static RRDDIM *rd_cache_metadata = NULL;
                static RRDDIM *rd_index_metadata = NULL;
                static RRDDIM *rd_pages_metadata = NULL;

                collected_number API_producers, populated_pages, cache_metadata, pages_on_disk,
                    page_cache_descriptors, index_metadata, pages_metadata;

                if (unlikely(!st_ram_usage)) {
                    st_ram_usage = rrdset_create_localhost(
                        "netdata",
                        "dbengine_ram",
                        NULL,
                        "dbengine",
                        NULL,
                        "Netdata DB engine RAM usage",
                        "MiB",
                        "netdata",
                        "stats",
                        132010,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_STACKED);

                    rd_cached = rrddim_add(st_ram_usage, "cache", NULL, RRDENG_BLOCK_SIZE, 1024*1024, RRD_ALGORITHM_ABSOLUTE);
                    rd_pinned = rrddim_add(st_ram_usage, "collectors", NULL, RRDENG_BLOCK_SIZE, 1024*1024, RRD_ALGORITHM_ABSOLUTE);
                    rd_cache_metadata = rrddim_add(st_ram_usage, "cache metadata", NULL, 1, 1024*1024, RRD_ALGORITHM_ABSOLUTE);
                    rd_pages_metadata = rrddim_add(st_ram_usage, "pages metadata", NULL, 1, 1024*1024, RRD_ALGORITHM_ABSOLUTE);
                    rd_index_metadata = rrddim_add(st_ram_usage, "index metadata", NULL, 1, 1024*1024, RRD_ALGORITHM_ABSOLUTE);
                } else
                    rrdset_next(st_ram_usage);

                API_producers = (collected_number)stats_array[0];
                pages_on_disk = (collected_number)stats_array[2];
                populated_pages = (collected_number)stats_array[3];
                page_cache_descriptors = (collected_number)stats_array[27];

                cache_metadata = page_cache_descriptors * sizeof(struct page_cache_descr);

                pages_metadata = pages_on_disk * sizeof(struct rrdeng_page_descr);

                /* This is an empirical estimation for Judy array indexing and extent structures */
                index_metadata = pages_on_disk * 58;

                rrddim_set_by_pointer(st_ram_usage, rd_cached, populated_pages - API_producers);
                rrddim_set_by_pointer(st_ram_usage, rd_pinned, API_producers);
                rrddim_set_by_pointer(st_ram_usage, rd_cache_metadata, cache_metadata);
                rrddim_set_by_pointer(st_ram_usage, rd_pages_metadata, pages_metadata);
                rrddim_set_by_pointer(st_ram_usage, rd_index_metadata, index_metadata);
                rrdset_done(st_ram_usage);
            }
        }
    }
#endif
}

static void update_strings_charts() {
    static RRDSET *st_ops = NULL, *st_entries = NULL, *st_mem = NULL;
    static RRDDIM *rd_ops_inserts = NULL, *rd_ops_deletes = NULL, *rd_ops_searches = NULL, *rd_ops_duplications = NULL, *rd_ops_releases = NULL;
    static RRDDIM *rd_entries_entries = NULL, *rd_entries_refs = NULL;
    static RRDDIM *rd_mem = NULL;

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
        rd_ops_searches     = rrddim_add(st_ops, "searches",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_ops_duplications = rrddim_add(st_ops, "duplications", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_ops_releases     = rrddim_add(st_ops, "releases",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
    } else
        rrdset_next(st_ops);

    rrddim_set_by_pointer(st_ops, rd_ops_inserts,      (collected_number)inserts);
    rrddim_set_by_pointer(st_ops, rd_ops_deletes,      (collected_number)deletes);
    rrddim_set_by_pointer(st_ops, rd_ops_searches,     (collected_number)searches);
    rrddim_set_by_pointer(st_ops, rd_ops_duplications, (collected_number)duplications);
    rrddim_set_by_pointer(st_ops, rd_ops_releases,     (collected_number)releases);
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
        rd_entries_refs  = rrddim_add(st_entries, "references", NULL, 1, -1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(st_entries);

    rrddim_set_by_pointer(st_entries, rd_entries_entries, (collected_number)entries);
    rrddim_set_by_pointer(st_entries, rd_entries_refs, (collected_number)references);
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
    } else
        rrdset_next(st_mem);

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
    } else
        rrdset_next(st_heartbeat);

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

} dictionary_categories[] = {
    { .stats = &dictionary_stats_category_other, "dictionaries", "dictionaries", 900000 },

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
        else
            rrdset_next(c->st_dicts);

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
        else
            rrdset_next(c->st_items);

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
        else
            rrdset_next(c->st_ops);

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
        else
            rrdset_next(c->st_callbacks);

        rrddim_set_by_pointer(c->st_callbacks, c->rd_callbacks_inserts, (collected_number)stats.callbacks.inserts);
        rrddim_set_by_pointer(c->st_callbacks, c->rd_callbacks_conflicts, (collected_number)stats.callbacks.conflicts);
        rrddim_set_by_pointer(c->st_callbacks, c->rd_callbacks_reacts, (collected_number)stats.callbacks.reacts);
        rrddim_set_by_pointer(c->st_callbacks, c->rd_callbacks_deletes, (collected_number)stats.callbacks.deletes);

        rrdset_done(c->st_callbacks);
    }

    // ------------------------------------------------------------------------

    total = 0;
    load_dictionary_stats_entry(memory.indexed);
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
        else
            rrdset_next(c->st_memory);

        rrddim_set_by_pointer(c->st_memory, c->rd_memory_indexed, (collected_number)stats.memory.indexed);
        rrddim_set_by_pointer(c->st_memory, c->rd_memory_values, (collected_number)stats.memory.values);
        rrddim_set_by_pointer(c->st_memory, c->rd_memory_dict, (collected_number)stats.memory.dict);

        rrdset_done(c->st_memory);
    }

    // ------------------------------------------------------------------------

    total = 0;
    load_dictionary_stats_entry(spin_locks.use);
    load_dictionary_stats_entry(spin_locks.search);
    load_dictionary_stats_entry(spin_locks.insert);

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

            rrdlabels_add(c->st_spins->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }
        else
            rrdset_next(c->st_spins);

        rrddim_set_by_pointer(c->st_spins, c->rd_spins_use, (collected_number)stats.spin_locks.use);
        rrddim_set_by_pointer(c->st_spins, c->rd_spins_search, (collected_number)stats.spin_locks.search);
        rrddim_set_by_pointer(c->st_spins, c->rd_spins_insert, (collected_number)stats.spin_locks.insert);

        rrdset_done(c->st_spins);
    }
}

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
    else
        rrdset_next(tmp.st_memory);

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
    else
        rrdset_next(tmp.st_ops);

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
    else
        rrdset_next(tmp.st_allocations);

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
    else
        rrdset_next(tmp.st_avg_alloc);

    malloc_trace_walkthrough(do_memory_trace_item, &tmp);

    rrdset_done(tmp.st_memory);
    rrdset_done(tmp.st_ops);
    rrdset_done(tmp.st_allocations);
    rrdset_done(tmp.st_avg_alloc);
}
#endif

static void dictionary_statistics(void) {
    for(int i = 0; dictionary_categories[i].stats ;i++) {
        update_dictionary_category_charts(&dictionary_categories[i]);
    }
}

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
    int enabled;

    int cpu_enabled;
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
};

struct worker_utilization {
    const char *name;
    const char *family;
    size_t priority;
    uint32_t flags;

    char *name_lowercase;

    struct worker_job_type_gs per_job_type[WORKER_UTILIZATION_MAX_JOB_TYPES];

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
    { .name = "RRDCONTEXT",  .family = "workers contexts",                .priority = 1000000 },
    { .name = "SERVICE",     .family = "workers service",                 .priority = 1000000 },

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

    rrdset_next(st);

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

    rrdset_next(wu->st_workers_time);

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

        rrdset_next(wu->st_workers_cpu);

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

    rrdset_next(wu->st_workers_jobs_per_job_type);

    {
        size_t i;
        for(i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES ;i++) {
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

    rrdset_next(wu->st_workers_busy_per_job_type);

    {
        size_t i;
        for(i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES ;i++) {
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
        else
            rrdset_next(wu->st_workers_threads);

        rrddim_set_by_pointer(wu->st_workers_threads, wu->rd_workers_threads_free, (collected_number)(wu->workers_registered - wu->workers_busy));
        rrddim_set_by_pointer(wu->st_workers_threads, wu->rd_workers_threads_busy, (collected_number)(wu->workers_busy));
        rrdset_done(wu->st_workers_threads);
    }

    // ----------------------------------------------------------------------
    // custom metric types WORKER_METRIC_ABSOLUTE

    {
        size_t i;
        for (i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES; i++) {
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
                snprintf(title, 1000, "Netdata Workers %s Value of %s", wu->name_lowercase, string2str(wu->per_job_type[i].name));

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
                    , wu->priority + 5
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
                    );

                wu->per_job_type[i].rd_min = rrddim_add(wu->per_job_type[i].st, "min", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
                wu->per_job_type[i].rd_max = rrddim_add(wu->per_job_type[i].st, "max", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
                wu->per_job_type[i].rd_avg = rrddim_add(wu->per_job_type[i].st, "average", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
            }
            else
                rrdset_next(wu->per_job_type[i].st);

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
        for (i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES; i++) {
            if(wu->per_job_type[i].type != WORKER_METRIC_INCREMENTAL)
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
                snprintf(title, 1000, "Netdata Workers %s Rate of %s", wu->name_lowercase, string2str(wu->per_job_type[i].name));

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
                    , wu->priority + 5
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
                );

                wu->per_job_type[i].rd_min = rrddim_add(wu->per_job_type[i].st, "min", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
                wu->per_job_type[i].rd_max = rrddim_add(wu->per_job_type[i].st, "max", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
                wu->per_job_type[i].rd_avg = rrddim_add(wu->per_job_type[i].st, "average", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
            }
            else
                rrdset_next(wu->per_job_type[i].st);

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
        wt->enabled = 0;
        wt->cpu_enabled = 0;
    }
}

static int read_thread_cpu_time_from_proc_stat(pid_t pid __maybe_unused, kernel_uint_t *utime __maybe_unused, kernel_uint_t *stime __maybe_unused) {
#ifdef __linux__
    char filename[200 + 1];
    snprintfz(filename, 200, "/proc/self/task/%d/stat", pid);

    procfile *ff = procfile_open(filename, " ", PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(!ff) return -1;

    ff = procfile_readall(ff);
    if(!ff) return -1;

    *utime = str2kernel_uint_t(procfile_lineword(ff, 0, 13));
    *stime = str2kernel_uint_t(procfile_lineword(ff, 0, 14));

    procfile_close(ff);
    return 0;
#else
    // TODO: add here cpu time detection per thread, for FreeBSD and MacOS
    *utime = 0;
    *stime = 0;
    return 1;
#endif
}

static void workers_threads_cleanup(struct worker_utilization *wu) {
    struct worker_thread *t;

    // free threads at the beginning of the linked list
    while(wu->threads && !wu->threads->enabled) {
        t = wu->threads;
        wu->threads = t->next;
        t->next = NULL;
        freez(t);
    }

    // free threads in the middle of the linked list
    for(t = wu->threads; t && t->next ; t = t->next) {
        if(t->next->enabled) continue;

        struct worker_thread *to_remove = t->next;
        t->next = to_remove->next;
        to_remove->next = NULL;
        freez(to_remove);
    }
}

static struct worker_thread *worker_thread_find(struct worker_utilization *wu, pid_t pid) {
    struct worker_thread *wt;
    for(wt = wu->threads; wt && wt->pid != pid ; wt = wt->next) ;
    return wt;
}

static struct worker_thread *worker_thread_create(struct worker_utilization *wu, pid_t pid) {
    struct worker_thread *wt;

    wt = (struct worker_thread *)callocz(1, sizeof(struct worker_thread));
    wt->pid = pid;

    // link it
    wt->next = wu->threads;
    wu->threads = wt;

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

    wt->enabled = 1;
    wt->busy_time = utilization_usec;
    wt->jobs_started = jobs_started;

    wt->utime_old = wt->utime;
    wt->stime_old = wt->stime;
    wt->collected_time_old = wt->collected_time;

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
    for(i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES ;i++) {
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
        wt->cpu_enabled = 1;

        wu->workers_cpu_total += cpu;
        if(cpu < wu->workers_cpu_min) wu->workers_cpu_min = cpu;
        if(cpu > wu->workers_cpu_max) wu->workers_cpu_max = cpu;
    }
    wu->workers_cpu_registered += wt->cpu_enabled;
}

static void worker_utilization_charts(void) {
    static size_t iterations = 0;
    iterations++;

    int i;
    for(i = 0; all_workers_utilization[i].name ;i++) {
        workers_utilization_reset_statistics(&all_workers_utilization[i]);
        workers_foreach(all_workers_utilization[i].name, worker_utilization_charts_callback, &all_workers_utilization[i]);

        // skip the first iteration, so that we don't accumulate startup utilization to our charts
        if(likely(iterations > 1))
            workers_utilization_update_chart(&all_workers_utilization[i]);

        workers_threads_cleanup(&all_workers_utilization[i]);
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
        for(t = wu->threads; t ; t = t->next) t->enabled = 0;

        // let the cleanup job free them
        workers_threads_cleanup(wu);
    }
}

// ---------------------------------------------------------------------------------------------------------------------

static void global_statistics_cleanup(void *ptr)
{
    worker_unregister();

    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    worker_utilization_finish();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *global_statistics_main(void *ptr)
{
    worker_register("STATS");
    worker_register_job_name(WORKER_JOB_GLOBAL, "global");
    worker_register_job_name(WORKER_JOB_REGISTRY, "registry");
    worker_register_job_name(WORKER_JOB_WORKERS, "workers");
    worker_register_job_name(WORKER_JOB_DBENGINE, "dbengine");
    worker_register_job_name(WORKER_JOB_STRINGS, "strings");
    worker_register_job_name(WORKER_JOB_DICTIONARIES, "dictionaries");
    worker_register_job_name(WORKER_JOB_MALLOC_TRACE, "malloc_trace");

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

    while (!netdata_exit) {
        worker_is_idle();
        heartbeat_next(&hb, step);

        worker_is_busy(WORKER_JOB_WORKERS);
        worker_utilization_charts();

        worker_is_busy(WORKER_JOB_GLOBAL);
        global_statistics_charts();

        worker_is_busy(WORKER_JOB_REGISTRY);
        registry_statistics();

        if(dbengine_enabled) {
            worker_is_busy(WORKER_JOB_DBENGINE);
            dbengine_statistics_charts();
        }

        worker_is_busy(WORKER_JOB_HEARTBEAT);
        update_heartbeat_charts();
        
        worker_is_busy(WORKER_JOB_STRINGS);
        update_strings_charts();

        worker_is_busy(WORKER_JOB_DICTIONARIES);
        dictionary_statistics();

#ifdef NETDATA_TRACE_ALLOCATIONS
        worker_is_busy(WORKER_JOB_MALLOC_TRACE);
        malloc_trace_statistics();
#endif
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
