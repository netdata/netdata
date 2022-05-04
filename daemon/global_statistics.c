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

#define WORKERS_THREADS_INCREASE_STEP 2

struct worker_utilization {
    const char *name;
    const char *family;
    size_t priority;

    size_t worker_count;
    usec_t worker_total_utilization;
    usec_t worker_total_duration;
    size_t worker_total_jobs_done;
    size_t worker_total_jobs_running;
    double worker_min_utilization;
    double worker_max_utilization;

#ifdef __linux__
    size_t workers_arrays_size;
    size_t workers_arrays_slot;
    kernel_uint_t *utime_new_array;
    kernel_uint_t *utime_old_array;
    kernel_uint_t *stime_new_array;
    kernel_uint_t *stime_old_array;
    usec_t *cpu_old_collected_time_array;
    usec_t *cpu_new_collected_time_array;
#endif

    RRDSET *st_util;
    RRDDIM *rd_util_avg;
    RRDDIM *rd_util_min;
    RRDDIM *rd_util_max;

    RRDSET *st_workers;
    RRDDIM *rd_workers_available;
    RRDDIM *rd_workers_busy;

    RRDSET *st_jobs;
    RRDDIM *rd_jobs_done;
    RRDDIM *rd_jobs_running;

    RRDSET *st_cpu;
    RRDDIM *rd_cpu_avg;
    RRDDIM *rd_cpu_min;
    RRDDIM *rd_cpu_max;
};

static void worker_utilization_update_chart(struct worker_utilization *wu) {
    if(!wu->worker_count) return;

    //fprintf(stderr, "%-12s WORKER UTILIZATION: %-3.2f%%, %zu jobs done, %zu running, on %zu workers, min %-3.02f%%, max %-3.02f%%.\n",
    //        wu->name,
    //        (double)wu->worker_total_utilization * 100.0 / (double)wu->worker_total_duration,
    //        wu->worker_total_jobs_done, wu->worker_total_jobs_running, wu->worker_count,
    //        wu->worker_min_utilization, wu->worker_max_utilization);

    // ----------------------------------------------------------------------

    if(unlikely(!wu->st_util)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_utilization_%s", wu->name);

        wu->st_util = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , "netdata.workers.utilization"
            , "Netdata Workers Utilization"
            , "%"
            , "netdata"
            , "stats"
            , wu->priority
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA
        );

        wu->rd_util_avg = rrddim_add(wu->st_util, "average", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
        wu->rd_util_min = rrddim_add(wu->st_util, "min", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
        wu->rd_util_max = rrddim_add(wu->st_util, "max", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
    }
    else
        rrdset_next(wu->st_util);

    rrddim_set_by_pointer(wu->st_util, wu->rd_util_avg, (collected_number)((double)wu->worker_total_utilization * 100.0 * 10000.0 / (double)wu->worker_total_duration));
    rrddim_set_by_pointer(wu->st_util, wu->rd_util_min, (collected_number)((double)wu->worker_min_utilization * 10000.0));
    rrddim_set_by_pointer(wu->st_util, wu->rd_util_max, (collected_number)((double)wu->worker_max_utilization * 10000.0));
    rrdset_done(wu->st_util);

    // ----------------------------------------------------------------------

    if(unlikely(!wu->st_jobs)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_jobs_%s", wu->name);

        wu->st_jobs = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , "netdata.workers.jobs"
            , "Netdata Active Workers"
            , "jobs"
            , "netdata"
            , "stats"
            , wu->priority + 1
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA
        );

        wu->rd_jobs_done    = rrddim_add(wu->st_jobs, "finished", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        wu->rd_jobs_running = rrddim_add(wu->st_jobs, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    else
        rrdset_next(wu->st_jobs);

    rrddim_set_by_pointer(wu->st_jobs, wu->rd_jobs_done, (collected_number)(wu->worker_total_jobs_done));
    rrddim_set_by_pointer(wu->st_jobs, wu->rd_jobs_running, (collected_number)(wu->worker_total_jobs_running));
    rrdset_done(wu->st_jobs);

    // ----------------------------------------------------------------------

    if(unlikely(!wu->st_workers)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "worker_threads_%s", wu->name);

        wu->st_workers = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , "netdata.workers.threads"
            , "Netdata Active Workers"
            , "threads"
            , "netdata"
            , "stats"
            , wu->priority + 2
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
        );

        wu->rd_workers_available = rrddim_add(wu->st_workers, "available", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        wu->rd_workers_busy      = rrddim_add(wu->st_workers, "busy", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    else
        rrdset_next(wu->st_workers);

    rrddim_set_by_pointer(wu->st_workers, wu->rd_workers_available, (collected_number)(wu->worker_count - wu->worker_total_jobs_running));
    rrddim_set_by_pointer(wu->st_workers, wu->rd_workers_busy, (collected_number)(wu->worker_total_jobs_running));
    rrdset_done(wu->st_workers);

    // ----------------------------------------------------------------------
#ifdef __linux__
    if(unlikely(!wu->st_cpu)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_cpu_utilization_%s", wu->name);

        wu->st_cpu = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , "netdata.workers.cpu.utilization"
            , "Netdata Workers CPU Utilization"
            , "%"
            , "netdata"
            , "stats"
            , wu->priority + 3
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA
        );

        wu->rd_cpu_avg = rrddim_add(wu->st_cpu, "average", NULL, 1, 10000ULL, RRD_ALGORITHM_ABSOLUTE);
        wu->rd_cpu_min = rrddim_add(wu->st_cpu, "min", NULL, 1, 10000ULL, RRD_ALGORITHM_ABSOLUTE);
        wu->rd_cpu_max = rrddim_add(wu->st_cpu, "max", NULL, 1, 10000ULL, RRD_ALGORITHM_ABSOLUTE);
    }
    else
        rrdset_next(wu->st_cpu);

    size_t i;
    calculated_number min = 100.0, max = 0.0, total = 0.0;
    for(i = 0; i < wu->workers_arrays_slot; i++) {
        kernel_uint_t utime_old = wu->utime_old_array[i];
        kernel_uint_t stime_old = wu->utime_old_array[i];
        kernel_uint_t utime_new = wu->utime_new_array[i];
        kernel_uint_t stime_new = wu->utime_new_array[i];
        usec_t collected_old = wu->cpu_old_collected_time_array[i];
        usec_t collected_new = wu->cpu_new_collected_time_array[i];

        usec_t delta = collected_new - collected_old;
        calculated_number utime = (calculated_number)(utime_new - utime_old) * 100.0 / system_hz * (calculated_number)USEC_PER_SEC / (calculated_number)delta;
        calculated_number stime = (calculated_number)(stime_new - stime_old) * 100.0 / system_hz * (calculated_number)USEC_PER_SEC / (calculated_number)delta;
        calculated_number cpu_util = utime + stime;

        // parsing files from /proc is not that accurate
        // mainly because the time we collect the values
        // vs the time we think they reflect are not right
        // so, let's stop this from being visible to users
        if(cpu_util > 100.0) cpu_util = 100.0;

        total += cpu_util;
        if(cpu_util < min) min = cpu_util;
        if(cpu_util > max) max = cpu_util;
    }
    rrddim_set_by_pointer(wu->st_cpu, wu->rd_cpu_avg, (collected_number)( total * 10000ULL / (calculated_number)i ));
    rrddim_set_by_pointer(wu->st_cpu, wu->rd_cpu_min, (collected_number)( min * 10000ULL));
    rrddim_set_by_pointer(wu->st_cpu, wu->rd_cpu_max, (collected_number)( max * 10000ULL));
    rrdset_done(wu->st_cpu);
#endif
}

#ifdef __linux__
static void resize_workers_arrays_if_required(struct worker_utilization *wu) {
    if(wu->workers_arrays_slot >= wu->workers_arrays_size) {
        size_t newsize = wu->workers_arrays_size + WORKERS_THREADS_INCREASE_STEP;

        wu->utime_new_array = reallocz(wu->utime_new_array, newsize * sizeof(kernel_uint_t));
        wu->utime_old_array = reallocz(wu->utime_old_array, newsize * sizeof(kernel_uint_t));
        wu->stime_new_array = reallocz(wu->stime_new_array, newsize * sizeof(kernel_uint_t));
        wu->stime_old_array = reallocz(wu->stime_old_array, newsize * sizeof(kernel_uint_t));
        wu->cpu_old_collected_time_array = reallocz(wu->cpu_old_collected_time_array, newsize * sizeof(usec_t));
        wu->cpu_new_collected_time_array = reallocz(wu->cpu_new_collected_time_array, newsize * sizeof(usec_t));

        wu->workers_arrays_size = newsize;
    }
}
#endif

static void worker_utilization_reset_statistics(struct worker_utilization *wu) {
    wu->worker_count = 0;
    wu->worker_total_utilization = 0;
    wu->worker_total_duration = 0;
    wu->worker_total_jobs_done = 0;
    wu->worker_total_jobs_running = 0;
    wu->worker_min_utilization = 100.0;
    wu->worker_max_utilization = 0;

#ifdef __linux__
    wu->workers_arrays_slot = 0;
    if(wu->cpu_old_collected_time_array && wu->cpu_new_collected_time_array) {
        memcpy(wu->utime_old_array, wu->utime_new_array,wu->workers_arrays_size * sizeof(kernel_uint_t));
        memcpy(wu->stime_old_array, wu->stime_new_array,wu->workers_arrays_size * sizeof(kernel_uint_t));
        memcpy(wu->cpu_old_collected_time_array, wu->cpu_new_collected_time_array, wu->workers_arrays_size * sizeof(usec_t));
    }
#endif
}


#ifdef __linux__
static int read_thread_cpu_time_from_proc_stat(pid_t pid, kernel_uint_t *utime, kernel_uint_t *stime) {
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
}
#endif

static void worker_utilization_charts_callback(void *ptr, pid_t pid __maybe_unused, const char *thread_tag __maybe_unused, size_t utilization_usec __maybe_unused, size_t duration_usec __maybe_unused, size_t jobs_done __maybe_unused, size_t jobs_running __maybe_unused) {
    struct worker_utilization *wu = (struct worker_utilization *)ptr;

    wu->worker_total_utilization += utilization_usec;
    wu->worker_total_duration += duration_usec;
    wu->worker_total_jobs_done += jobs_done;
    wu->worker_total_jobs_running += jobs_running;
    wu->worker_count++;

    double util = (double)utilization_usec * 100.0 / (double)duration_usec;
    if(util > wu->worker_max_utilization)
        wu->worker_max_utilization = util;

    if(util < wu->worker_min_utilization)
        wu->worker_min_utilization = util;

#ifdef __linux__
    resize_workers_arrays_if_required(wu);
    if(!read_thread_cpu_time_from_proc_stat(pid, &wu->utime_new_array[wu->workers_arrays_slot], &wu->stime_new_array[wu->workers_arrays_slot])) {
        wu->cpu_new_collected_time_array[wu->workers_arrays_slot] = now_realtime_usec();

        // kernel_uint_t utime = wu->utime_new_array[wu->workers_arrays_slot];
        // kernel_uint_t stime = wu->stime_new_array[wu->workers_arrays_slot];
        // fprintf(stderr, "WORKER UTILIZATION NAME %s, pid %d/%zu, SIZE %zu, utime %zu, stime %zu\n", wu->name, pid, wu->workers_arrays_slot, wu->workers_arrays_size, (size_t)utime, (size_t)stime);

        wu->workers_arrays_slot++;
    }
#endif
}

static struct worker_utilization all_workers_utilization[] = {
    { .name = "WEB",       .family = "web",      .priority = 131990 },
    { .name = "DBENGINE",  .family = "dbengine", .priority = 130501 },
    { .name = "ACLKQUERY", .family = "aclk",     .priority = 200000 },
    { .name = "ACLKSYNC",  .family = "aclk",     .priority = 200003 },

    // has to be terminated with a NULL
    { .name = NULL,        .family = NULL       }
};

void worker_utilization_charts(void) {
    int i;
    for(i = 0; all_workers_utilization[i].name ;i++) {
        worker_utilization_reset_statistics(&all_workers_utilization[i]);
        workers_foreach(all_workers_utilization[i].name, worker_utilization_charts_callback, &all_workers_utilization[i]);
        worker_utilization_update_chart(&all_workers_utilization[i]);
    }
}

// ---------------------------------------------------------------------------------------------------------------------

static void global_statistics_cleanup(void *ptr)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *global_statistics_main(void *ptr)
{
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

        worker_utilization_charts();
        global_statistics_charts();
        registry_statistics();
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
