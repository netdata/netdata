// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-queries.h"
#include "streaming/stream-replication-sender.h"

static struct query_statistics {
    PAD64(uint64_t) api_data_queries_made;
    PAD64(uint64_t) api_data_db_points_read;
    PAD64(uint64_t) api_data_result_points_generated;

    PAD64(uint64_t) api_weights_queries_made;
    PAD64(uint64_t) api_weights_db_points_read;
    PAD64(uint64_t) api_weights_result_points_generated;

    PAD64(uint64_t) api_badges_queries_made;
    PAD64(uint64_t) api_badges_db_points_read;
    PAD64(uint64_t) api_badges_result_points_generated;

    PAD64(uint64_t) health_queries_made;
    PAD64(uint64_t) health_db_points_read;
    PAD64(uint64_t) health_result_points_generated;

    PAD64(uint64_t) ml_queries_made;
    PAD64(uint64_t) ml_db_points_read;
    PAD64(uint64_t) ml_result_points_generated;

    PAD64(uint64_t) backfill_queries_made;
    PAD64(uint64_t) backfill_db_points_read;

    PAD64(uint64_t) exporters_queries_made;
    PAD64(uint64_t) exporters_db_points_read;
} query_statistics = { 0 };

ALWAYS_INLINE void pulse_queries_ml_query_completed(size_t points_read) {
    __atomic_fetch_add(&query_statistics.ml_queries_made, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&query_statistics.ml_db_points_read, points_read, __ATOMIC_RELAXED);
}

ALWAYS_INLINE void pulse_queries_exporters_query_completed(size_t points_read) {
    __atomic_fetch_add(&query_statistics.exporters_queries_made, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&query_statistics.exporters_db_points_read, points_read, __ATOMIC_RELAXED);
}

ALWAYS_INLINE void pulse_queries_backfill_query_completed(size_t points_read) {
    __atomic_fetch_add(&query_statistics.backfill_queries_made, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&query_statistics.backfill_db_points_read, points_read, __ATOMIC_RELAXED);
}

ALWAYS_INLINE void pulse_queries_rrdr_query_completed(size_t queries, uint64_t db_points_read, uint64_t result_points_generated, QUERY_SOURCE query_source) {
    switch(query_source) {
        case QUERY_SOURCE_API_DATA:
            __atomic_fetch_add(&query_statistics.api_data_queries_made, queries, __ATOMIC_RELAXED);
            __atomic_fetch_add(&query_statistics.api_data_db_points_read, db_points_read, __ATOMIC_RELAXED);
            __atomic_fetch_add(&query_statistics.api_data_result_points_generated, result_points_generated, __ATOMIC_RELAXED);
            break;

        case QUERY_SOURCE_ML:
            __atomic_fetch_add(&query_statistics.ml_queries_made, queries, __ATOMIC_RELAXED);
            __atomic_fetch_add(&query_statistics.ml_db_points_read, db_points_read, __ATOMIC_RELAXED);
            __atomic_fetch_add(&query_statistics.ml_result_points_generated, result_points_generated, __ATOMIC_RELAXED);
            break;

        case QUERY_SOURCE_API_WEIGHTS:
            __atomic_fetch_add(&query_statistics.api_weights_queries_made, queries, __ATOMIC_RELAXED);
            __atomic_fetch_add(&query_statistics.api_weights_db_points_read, db_points_read, __ATOMIC_RELAXED);
            __atomic_fetch_add(&query_statistics.api_weights_result_points_generated, result_points_generated, __ATOMIC_RELAXED);
            break;

        case QUERY_SOURCE_API_BADGE:
            __atomic_fetch_add(&query_statistics.api_badges_queries_made, queries, __ATOMIC_RELAXED);
            __atomic_fetch_add(&query_statistics.api_badges_db_points_read, db_points_read, __ATOMIC_RELAXED);
            __atomic_fetch_add(&query_statistics.api_badges_result_points_generated, result_points_generated, __ATOMIC_RELAXED);
            break;

        case QUERY_SOURCE_HEALTH:
            __atomic_fetch_add(&query_statistics.health_queries_made, queries, __ATOMIC_RELAXED);
            __atomic_fetch_add(&query_statistics.health_db_points_read, db_points_read, __ATOMIC_RELAXED);
            __atomic_fetch_add(&query_statistics.health_result_points_generated, result_points_generated, __ATOMIC_RELAXED);
            break;

        default:
        case QUERY_SOURCE_UNITTEST:
        case QUERY_SOURCE_UNKNOWN:
            break;
    }
}

static inline void pulse_queries_copy(struct query_statistics *gs) {
    gs->api_data_queries_made            = __atomic_load_n(&query_statistics.api_data_queries_made, __ATOMIC_RELAXED);
    gs->api_data_db_points_read          = __atomic_load_n(&query_statistics.api_data_db_points_read, __ATOMIC_RELAXED);
    gs->api_data_result_points_generated = __atomic_load_n(&query_statistics.api_data_result_points_generated, __ATOMIC_RELAXED);

    gs->api_weights_queries_made            = __atomic_load_n(&query_statistics.api_weights_queries_made, __ATOMIC_RELAXED);
    gs->api_weights_db_points_read          = __atomic_load_n(&query_statistics.api_weights_db_points_read, __ATOMIC_RELAXED);
    gs->api_weights_result_points_generated = __atomic_load_n(&query_statistics.api_weights_result_points_generated, __ATOMIC_RELAXED);

    gs->api_badges_queries_made            = __atomic_load_n(&query_statistics.api_badges_queries_made, __ATOMIC_RELAXED);
    gs->api_badges_db_points_read          = __atomic_load_n(&query_statistics.api_badges_db_points_read, __ATOMIC_RELAXED);
    gs->api_badges_result_points_generated = __atomic_load_n(&query_statistics.api_badges_result_points_generated, __ATOMIC_RELAXED);

    gs->health_queries_made            = __atomic_load_n(&query_statistics.health_queries_made, __ATOMIC_RELAXED);
    gs->health_db_points_read          = __atomic_load_n(&query_statistics.health_db_points_read, __ATOMIC_RELAXED);
    gs->health_result_points_generated = __atomic_load_n(&query_statistics.health_result_points_generated, __ATOMIC_RELAXED);

    gs->ml_queries_made              = __atomic_load_n(&query_statistics.ml_queries_made, __ATOMIC_RELAXED);
    gs->ml_db_points_read            = __atomic_load_n(&query_statistics.ml_db_points_read, __ATOMIC_RELAXED);
    gs->ml_result_points_generated   = __atomic_load_n(&query_statistics.ml_result_points_generated, __ATOMIC_RELAXED);

    gs->exporters_queries_made       = __atomic_load_n(&query_statistics.exporters_queries_made, __ATOMIC_RELAXED);
    gs->exporters_db_points_read     = __atomic_load_n(&query_statistics.exporters_db_points_read, __ATOMIC_RELAXED);
    gs->backfill_queries_made       = __atomic_load_n(&query_statistics.backfill_queries_made, __ATOMIC_RELAXED);
    gs->backfill_db_points_read     = __atomic_load_n(&query_statistics.backfill_db_points_read, __ATOMIC_RELAXED);
}

void pulse_queries_do(bool extended __maybe_unused) {
    static struct query_statistics gs;
    pulse_queries_copy(&gs);

    struct replication_query_statistics replication = replication_get_query_statistics();

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
                , "Time-Series Queries"
                , "netdata.db_queries"
                , "Netdata Time-Series DB Queries"
                , "queries/s"
                , "netdata"
                , "pulse"
                , 131000
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
            );

            rd_api_data_queries = rrddim_add(st_queries, "/api/vX/data", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_api_weights_queries = rrddim_add(st_queries, "/api/vX/weights", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_api_badges_queries = rrddim_add(st_queries, "/api/vX/badge", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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
                , "db_samples_read"
                , NULL
                , "Time-Series Queries"
                , NULL
                , "Netdata Time-Series DB Samples Read"
                , "samples/s"
                , "netdata"
                , "pulse"
                , 131001
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
            );

            rd_api_data_points_read = rrddim_add(st_points_read, "/api/vX/data", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_api_weights_points_read = rrddim_add(st_points_read, "/api/vX/weights", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_api_badges_points_read = rrddim_add(st_points_read, "/api/vX/badge", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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
                , "Time-Series Queries"
                , NULL
                , "Netdata Time-Series Points Generated"
                , "points/s"
                , "netdata"
                , "pulse"
                , 131002
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
            );

            rd_api_data_points_generated = rrddim_add(st_points_generated, "/api/vX/data", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_api_weights_points_generated = rrddim_add(st_points_generated, "/api/vX/weights", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_api_badges_points_generated = rrddim_add(st_points_generated, "/api/vX/badge", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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
}
