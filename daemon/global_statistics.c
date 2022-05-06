// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

#define GLOBAL_STATS_RESET_WEB_USEC_MAX 0x01

#define CONFIG_SECTION_GLOBAL_STATISTICS "global statistics"

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

void rrdr_query_completed(uint64_t db_points_read, uint64_t result_points_generated) {
    __atomic_fetch_add(&global_statistics.rrdr_queries_made, 1, __ATOMIC_SEQ_CST);
    __atomic_fetch_add(&global_statistics.rrdr_db_points_read, db_points_read, __ATOMIC_SEQ_CST);
    __atomic_fetch_add(&global_statistics.rrdr_result_points_generated, result_points_generated, __ATOMIC_SEQ_CST);
}

void finished_web_request_statistics(uint64_t dt,
                                     uint64_t bytes_received,
                                     uint64_t bytes_sent,
                                     uint64_t content_size,
                                     uint64_t compressed_content_size) {
    uint64_t old_web_usec_max = global_statistics.web_usec_max;
    while(dt > old_web_usec_max)
        __atomic_compare_exchange(&global_statistics.web_usec_max, &old_web_usec_max, &dt, 1, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST);

    __atomic_fetch_add(&global_statistics.web_requests, 1, __ATOMIC_SEQ_CST);
    __atomic_fetch_add(&global_statistics.web_usec, dt, __ATOMIC_SEQ_CST);
    __atomic_fetch_add(&global_statistics.bytes_received, bytes_received, __ATOMIC_SEQ_CST);
    __atomic_fetch_add(&global_statistics.bytes_sent, bytes_sent, __ATOMIC_SEQ_CST);
    __atomic_fetch_add(&global_statistics.content_size, content_size, __ATOMIC_SEQ_CST);
    __atomic_fetch_add(&global_statistics.compressed_content_size, compressed_content_size, __ATOMIC_SEQ_CST);
}

uint64_t web_client_connected(void) {
    __atomic_fetch_add(&global_statistics.connected_clients, 1, __ATOMIC_SEQ_CST);
    return __atomic_fetch_add(&global_statistics.web_client_count, 1, __ATOMIC_SEQ_CST);
}

void web_client_disconnected(void) {
    __atomic_fetch_sub(&global_statistics.connected_clients, 1, __ATOMIC_SEQ_CST);
}


static inline void global_statistics_copy(struct global_statistics *gs, uint8_t options) {
    gs->connected_clients            = __atomic_fetch_add(&global_statistics.connected_clients, 0, __ATOMIC_SEQ_CST);
    gs->web_requests                 = __atomic_fetch_add(&global_statistics.web_requests, 0, __ATOMIC_SEQ_CST);
    gs->web_usec                     = __atomic_fetch_add(&global_statistics.web_usec, 0, __ATOMIC_SEQ_CST);
    gs->web_usec_max                 = __atomic_fetch_add(&global_statistics.web_usec_max, 0, __ATOMIC_SEQ_CST);
    gs->bytes_received               = __atomic_fetch_add(&global_statistics.bytes_received, 0, __ATOMIC_SEQ_CST);
    gs->bytes_sent                   = __atomic_fetch_add(&global_statistics.bytes_sent, 0, __ATOMIC_SEQ_CST);
    gs->content_size                 = __atomic_fetch_add(&global_statistics.content_size, 0, __ATOMIC_SEQ_CST);
    gs->compressed_content_size      = __atomic_fetch_add(&global_statistics.compressed_content_size, 0, __ATOMIC_SEQ_CST);
    gs->web_client_count             = __atomic_fetch_add(&global_statistics.web_client_count, 0, __ATOMIC_SEQ_CST);

    gs->rrdr_queries_made            = __atomic_fetch_add(&global_statistics.rrdr_queries_made, 0, __ATOMIC_SEQ_CST);
    gs->rrdr_db_points_read          = __atomic_fetch_add(&global_statistics.rrdr_db_points_read, 0, __ATOMIC_SEQ_CST);
    gs->rrdr_result_points_generated = __atomic_fetch_add(&global_statistics.rrdr_result_points_generated, 0, __ATOMIC_SEQ_CST);

    if(options & GLOBAL_STATS_RESET_WEB_USEC_MAX) {
        uint64_t n = 0;
        __atomic_compare_exchange(&global_statistics.web_usec_max, (uint64_t *) &gs->web_usec_max, &n, 1, __ATOMIC_SEQ_CST,
                                  __ATOMIC_SEQ_CST);
    }
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
                    , "netdata"
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
                    , "netdata"
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
                    , "netdata"
                    , NULL
                    , "Netdata Network Traffic"
                    , "kilobits/s"
                    , "netdata"
                    , "stats"
                    , 130000
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
                    , "netdata"
                    , NULL
                    , "Netdata API Response Time"
                    , "milliseconds/request"
                    , "netdata"
                    , "stats"
                    , 130400
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
                    , "netdata"
                    , NULL
                    , "Netdata API Responses Compression Savings Ratio"
                    , "percentage"
                    , "netdata"
                    , "stats"
                    , 130500
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
                    , 130500
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
                    , 130501
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
}

static void dbengine_statistics_charts(void) {
#ifdef ENABLE_DBENGINE
    if(netdata_rwlock_tryrdlock(&rrd_rwlock) == 0) {
        RRDHOST *host;
        unsigned long long stats_array[RRDENG_NR_STATS] = {0};
        unsigned long long local_stats_array[RRDENG_NR_STATS];
        unsigned dbengine_contexts = 0, counted_multihost_db = 0, i;

        rrdhost_foreach_read(host) {
            if (host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE && !rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {
                if (&multidb_ctx == host->rrdeng_ctx) {
                    if (counted_multihost_db)
                        continue; /* Only count multi-host DB once */
                    counted_multihost_db = 1;
                }
                ++dbengine_contexts;
                /* get localhost's DB engine's statistics */
                rrdeng_get_37_statistics(host->rrdeng_ctx, local_stats_array);
                for (i = 0; i < RRDENG_NR_STATS; ++i) {
                    /* aggregate statistics across hosts */
                    stats_array[i] += local_stats_array[i];
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
                        130502,
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
                        130503,
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
                        130504,
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
                        130505,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_total = rrddim_add(st_long_term_pages, "total", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    rd_insertions = rrddim_add(st_long_term_pages, "insertions", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_deletions = rrddim_add(st_long_term_pages, "deletions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_flushing_pressure_deletions = rrddim_add(
                        st_long_term_pages, "flushing_pressure_deletions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st_long_term_pages);

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
                        130506,
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
                        130507,
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
                        130508,
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
                        130509,
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
                static RRDDIM *rd_metadata = NULL;

                collected_number cached_pages, pinned_pages, API_producers, populated_pages, metadata, pages_on_disk,
                    page_cache_descriptors;

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
                        130510,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_STACKED);

                    rd_cached = rrddim_add(st_ram_usage, "cache", NULL, 1, 256, RRD_ALGORITHM_ABSOLUTE);
                    rd_pinned = rrddim_add(st_ram_usage, "collectors", NULL, 1, 256, RRD_ALGORITHM_ABSOLUTE);
                    rd_metadata = rrddim_add(st_ram_usage, "metadata", NULL, 1, 1048576, RRD_ALGORITHM_ABSOLUTE);
                } else
                    rrdset_next(st_ram_usage);

                API_producers = (collected_number)stats_array[0];
                pages_on_disk = (collected_number)stats_array[2];
                populated_pages = (collected_number)stats_array[3];
                page_cache_descriptors = (collected_number)stats_array[27];

                if (API_producers * 2 > populated_pages) {
                    pinned_pages = API_producers;
                } else {
                    pinned_pages = API_producers * 2;
                }
                cached_pages = populated_pages - pinned_pages;

                metadata = page_cache_descriptors * sizeof(struct page_cache_descr);
                metadata += pages_on_disk * sizeof(struct rrdeng_page_descr);
                /* This is an empirical estimation for Judy array indexing and extent structures */
                metadata += pages_on_disk * 58;

                rrddim_set_by_pointer(st_ram_usage, rd_cached, cached_pages);
                rrddim_set_by_pointer(st_ram_usage, rd_pinned, pinned_pages);
                rrddim_set_by_pointer(st_ram_usage, rd_metadata, metadata);
                rrdset_done(st_ram_usage);
            }
        }
    }
#endif

}

// ---------------------------------------------------------------------------------------------------------------------
// worker utilization

struct worker_job_type {
    char name[WORKER_UTILIZATION_MAX_JOB_NAME_LENGTH + 1];
    size_t jobs_started;
    usec_t busy_time;
    RRDDIM *rd_jobs_started;
    RRDDIM *rd_busy_time;
};

struct worker_thread {
    pid_t pid;
    int enabled;

    int cpu_enabled;

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

#define WORKER_FLAG_ALWAYS_ONE 0x00000001

struct worker_utilization {
    const char *name;
    const char *family;
    size_t priority;
    uint32_t flags;

    struct worker_job_type per_job_type[WORKER_UTILIZATION_MAX_JOB_TYPES];

    size_t workers_registered;
    size_t workers_busy;
    usec_t workers_total_busy_time;
    usec_t workers_total_duration;
    size_t workers_total_jobs_started;
    double workers_min_busy_time;
    double workers_max_busy_time;

    size_t worker_threads_size;
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
};

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
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_time_%s", wu->name);

        wu->st_workers_time = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , "netdata.workers.time"
            , "Netdata Workers Busy Time (100% = all workers busy)"
            , "%"
            , "netdata"
            , "stats"
            , wu->priority
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA
        );

        if(!(wu->flags & WORKER_FLAG_ALWAYS_ONE)) {
            wu->rd_workers_time_min = rrddim_add(wu->st_workers_time, "min", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
            wu->rd_workers_time_max = rrddim_add(wu->st_workers_time, "max", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
        }
        wu->rd_workers_time_avg = rrddim_add(wu->st_workers_time, "average", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
    }
    else
        rrdset_next(wu->st_workers_time);

    if(!(wu->flags & WORKER_FLAG_ALWAYS_ONE)) {
        rrddim_set_by_pointer(wu->st_workers_time, wu->rd_workers_time_min, (collected_number)((double)wu->workers_min_busy_time * 10000.0));
        rrddim_set_by_pointer(wu->st_workers_time, wu->rd_workers_time_max, (collected_number)((double)wu->workers_max_busy_time * 10000.0));
    }
    rrddim_set_by_pointer(wu->st_workers_time, wu->rd_workers_time_avg, (collected_number)((double)wu->workers_total_busy_time * 100.0 * 10000.0 / (double)wu->workers_total_duration));
    rrdset_done(wu->st_workers_time);

    // ----------------------------------------------------------------------

#ifdef __linux__
    if(unlikely(!wu->st_workers_cpu)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_cpu_%s", wu->name);

        wu->st_workers_cpu = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , "netdata.workers.cpu"
            , "Netdata Workers CPU Utilization (100% = all workers busy)"
            , "%"
            , "netdata"
            , "stats"
            , wu->priority + 1
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA
        );

        if(!(wu->flags & WORKER_FLAG_ALWAYS_ONE)) {
            wu->rd_workers_cpu_min = rrddim_add(wu->st_workers_cpu, "min", NULL, 1, 10000ULL, RRD_ALGORITHM_ABSOLUTE);
            wu->rd_workers_cpu_max = rrddim_add(wu->st_workers_cpu, "max", NULL, 1, 10000ULL, RRD_ALGORITHM_ABSOLUTE);
        }
        wu->rd_workers_cpu_avg = rrddim_add(wu->st_workers_cpu, "average", NULL, 1, 10000ULL, RRD_ALGORITHM_ABSOLUTE);
    }
    else
        rrdset_next(wu->st_workers_cpu);

    size_t i, count = 0;
    calculated_number min = 1000.0, max = 0.0, total = 0.0;
    struct worker_thread *wt;
    for(wt = wu->threads; wt ; wt = wt->next) {
        if(!wt->cpu_enabled) continue;
        count++;

        usec_t delta = wt->collected_time - wt->collected_time_old;
        calculated_number utime = (calculated_number)(wt->utime - wt->utime_old) / (calculated_number)system_hz * 100.0 * (calculated_number)USEC_PER_SEC / (calculated_number)delta;
        calculated_number stime = (calculated_number)(wt->stime - wt->stime_old) / (calculated_number)system_hz * 100.0 * (calculated_number)USEC_PER_SEC / (calculated_number)delta;
        calculated_number cpu_util = utime + stime;

        total += cpu_util;
        if(cpu_util < min) min = cpu_util;
        if(cpu_util > max) max = cpu_util;
    }
    if(unlikely(min == 1000.0)) min = 0.0;

    if(!(wu->flags & WORKER_FLAG_ALWAYS_ONE)) {
        rrddim_set_by_pointer(wu->st_workers_cpu, wu->rd_workers_cpu_min, (collected_number)(min * 10000ULL));
        rrddim_set_by_pointer(wu->st_workers_cpu, wu->rd_workers_cpu_max, (collected_number)(max * 10000ULL));
    }
    rrddim_set_by_pointer(wu->st_workers_cpu, wu->rd_workers_cpu_avg, (collected_number)( total * 10000ULL / (calculated_number)count ));
    rrdset_done(wu->st_workers_cpu);
#endif

    // ----------------------------------------------------------------------

    if(unlikely(!wu->st_workers_jobs_per_job_type)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_jobs_by_type_%s", wu->name);

        wu->st_workers_jobs_per_job_type = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , "netdata.workers.by_type.jobs_started"
            , "Netdata Workers Jobs Started by Type"
            , "jobs"
            , "netdata"
            , "stats"
            , wu->priority + 2
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
        );

        for(i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES ;i++) {
            if(wu->per_job_type[i].name[0])
                wu->per_job_type[i].rd_jobs_started = rrddim_add(wu->st_workers_jobs_per_job_type, wu->per_job_type[i].name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
    }
    else
        rrdset_next(wu->st_workers_jobs_per_job_type);

    for(i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES ;i++) {
        if (wu->per_job_type[i].name[0])
            rrddim_set_by_pointer(wu->st_workers_jobs_per_job_type, wu->per_job_type[i].rd_jobs_started, (collected_number)(wu->per_job_type[i].jobs_started));
    }

    rrdset_done(wu->st_workers_jobs_per_job_type);

    // ----------------------------------------------------------------------

    if(unlikely(!wu->st_workers_busy_per_job_type)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_busy_time_by_type_%s", wu->name);

        wu->st_workers_busy_per_job_type = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , "netdata.workers.by_type.busy_time"
            , "Netdata Workers Busy Time by Type"
            , "ms"
            , "netdata"
            , "stats"
            , wu->priority + 3
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
        );

        for(i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES ;i++) {
            if(wu->per_job_type[i].name[0])
                wu->per_job_type[i].rd_busy_time = rrddim_add(wu->st_workers_busy_per_job_type, wu->per_job_type[i].name, NULL, 1, USEC_PER_MS, RRD_ALGORITHM_ABSOLUTE);
        }
    }
    else
        rrdset_next(wu->st_workers_busy_per_job_type);

    for(i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES ;i++) {
        if (wu->per_job_type[i].name[0])
            rrddim_set_by_pointer(wu->st_workers_busy_per_job_type, wu->per_job_type[i].rd_busy_time, (collected_number)(wu->per_job_type[i].busy_time));
    }

    rrdset_done(wu->st_workers_busy_per_job_type);

    // ----------------------------------------------------------------------

    if(unlikely(!wu->st_workers_threads)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_threads_%s", wu->name);

        wu->st_workers_threads = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , "netdata.workers.threads"
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

static void workers_utilization_reset_statistics(struct worker_utilization *wu) {
    wu->workers_registered = 0;
    wu->workers_busy = 0;
    wu->workers_total_busy_time = 0;
    wu->workers_total_duration = 0;
    wu->workers_total_jobs_started = 0;
    wu->workers_min_busy_time = 100.0;
    wu->workers_max_busy_time = 0;

    size_t i;
    for(i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES ;i++) {
        wu->per_job_type[i].jobs_started = 0;
        wu->per_job_type[i].busy_time = 0;
    }

    struct worker_thread *wt;
    for(wt = wu->threads; wt ; wt = wt->next) {
        wt->enabled = 0;
        wt->cpu_enabled = 0;
    }
}

static int read_thread_cpu_time_from_proc_stat(pid_t pid, kernel_uint_t *utime, kernel_uint_t *stime) {
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
    return 1;
#endif
}

static void workers_threads_cleanup(struct worker_utilization *wu) {
    struct worker_thread *t;

    while(wu->threads && !wu->threads->enabled) {
        t = wu->threads;
        wu->threads = t->next;
        t->next = NULL;
        freez(t);
    }

    if(!wu->threads) return;

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

static void worker_utilization_charts_callback(void *ptr, pid_t pid __maybe_unused, const char *thread_tag __maybe_unused, size_t utilization_usec __maybe_unused, size_t duration_usec __maybe_unused, size_t jobs_started __maybe_unused, size_t is_running __maybe_unused, const char **job_types_names __maybe_unused, size_t *job_types_jobs_started __maybe_unused, usec_t *job_types_busy_time __maybe_unused) {
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
        wu->per_job_type[i].jobs_started += job_types_jobs_started[i];
        wu->per_job_type[i].busy_time += job_types_busy_time[i];

        // new job type found
        if(unlikely(!wu->per_job_type[i].name[0] && job_types_names[i]))
            strncpyz(wu->per_job_type[i].name, job_types_names[i], WORKER_UTILIZATION_MAX_JOB_NAME_LENGTH);
    }

    // find its CPU utilization
    if((wt->cpu_enabled = !read_thread_cpu_time_from_proc_stat(pid, &wt->utime, &wt->stime)))
        wt->collected_time = now_realtime_usec();
}

static struct worker_utilization all_workers_utilization[] = {
    { .name = "WEB",         .family = "web server threads",            .priority = 1000000 },
    { .name = "DBENGINE",    .family = "dbengine main threads",         .priority = 1000000, .flags = WORKER_FLAG_ALWAYS_ONE },
    { .name = "ACLKQUERY",   .family = "aclk query threads",            .priority = 1000000 },
    { .name = "ACLKSYNC",    .family = "aclk host sync threads",        .priority = 1000000 },
    { .name = "STATSD",      .family = "statsd collect threads",        .priority = 1000000, .flags = WORKER_FLAG_ALWAYS_ONE },
    { .name = "STATSDFLUSH", .family = "statsd flush threads",          .priority = 1000000, .flags = WORKER_FLAG_ALWAYS_ONE },
    { .name = "STATS",       .family = "global statistics threads",     .priority = 1000000, .flags = WORKER_FLAG_ALWAYS_ONE },
    { .name = "PROC",        .family = "proc threads",                  .priority = 1000000, .flags = WORKER_FLAG_ALWAYS_ONE },
    { .name = "CGROUPS",     .family = "cgroups collect threads",       .priority = 1000000, .flags = WORKER_FLAG_ALWAYS_ONE },
    { .name = "CGROUPSDISC", .family = "cgroups discovery threads",     .priority = 1000000, .flags = WORKER_FLAG_ALWAYS_ONE },

    // has to be terminated with a NULL
    { .name = NULL,        .family = NULL       }
};

void worker_utilization_charts(void) {
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
}

// ---------------------------------------------------------------------------------------------------------------------

static void global_statistics_cleanup(void *ptr)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    worker_unregister();
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

#define WORKER_JOB_GLOBAL   0
#define WORKER_JOB_REGISTRY 1
#define WORKER_JOB_WORKERS  2
#define WORKER_JOB_DBENGINE 3

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 4
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 4
#endif

void *global_statistics_main(void *ptr)
{
    worker_register("STATS");
    worker_register_job_name(WORKER_JOB_GLOBAL, "global");
    worker_register_job_name(WORKER_JOB_REGISTRY, "registry");
    worker_register_job_name(WORKER_JOB_WORKERS, "workers");
    worker_register_job_name(WORKER_JOB_DBENGINE, "dbengine");

    netdata_thread_cleanup_push(global_statistics_cleanup, ptr);

    int update_every =
        (int)config_get_number("CONFIG_SECTION_GLOBAL_STATISTICS", "update every", localhost->rrd_update_every);
    if (update_every < localhost->rrd_update_every)
        update_every = localhost->rrd_update_every;

    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    while (!netdata_exit) {
        heartbeat_next(&hb, step);

        worker_is_busy(WORKER_JOB_WORKERS);
        worker_utilization_charts();
        worker_is_idle();

        worker_is_busy(WORKER_JOB_GLOBAL);
        global_statistics_charts();
        worker_is_idle();

        worker_is_busy(WORKER_JOB_REGISTRY);
        registry_statistics();
        worker_is_idle();

        worker_is_busy(WORKER_JOB_DBENGINE);
        dbengine_statistics_charts();
        worker_is_idle();
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
