// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "database/rrddim-collection.h"
#include "daemon/unit_test_bridge.h"
#include "pagecache.h"
#include "page_test.h"

#ifdef ENABLE_DBENGINE

#define CHARTS 64
#define DIMS 16 // CHARTS * DIMS dimensions
#define REGIONS 11
#define POINTS_PER_REGION 16384
static const int REGION_UPDATE_EVERY[REGIONS] = {1, 15, 3, 20, 2, 6, 30, 12, 5, 4, 10};

#define START_TIMESTAMP MAX(2 * API_RELATIVE_TIME_MAX, 200000000)

static time_t region_start_time(time_t previous_region_end_time, time_t update_every) {
    // leave a small gap between regions
    // but keep them close together, so that cross-region queries will be fast

    time_t rc = previous_region_end_time + update_every;
    rc += update_every - (rc % update_every);
    rc += update_every;
    return rc;
}

static inline collected_number point_value_get(size_t region, size_t chart, size_t dim, size_t point) {
    // calculate the value to be stored for each point in the database

    collected_number r = (collected_number)region;
    collected_number c = (collected_number)chart;
    collected_number d = (collected_number)dim;
    collected_number p = (collected_number)point;

    return (r * CHARTS * DIMS * POINTS_PER_REGION +
            c * DIMS * POINTS_PER_REGION +
            d * POINTS_PER_REGION +
            p) % 10000000;
}

static inline void storage_point_check(size_t region, size_t chart, size_t dim, size_t point, time_t now, time_t update_every, STORAGE_POINT sp, size_t *value_errors, size_t *time_errors, size_t *update_every_errors) {
    // check the supplied STORAGE_POINT retrieved from the database
    // against the computed timestamp, update_every and expected value

    if(storage_point_is_gap(sp)) sp.min = sp.max = sp.sum = NAN;

    collected_number expected = point_value_get(region, chart, dim, point);

    if(roundndd(expected) != roundndd(sp.sum)) {
        if(*value_errors < DIMS * 2) {
            fprintf(stderr, " >>> DBENGINE: VALUE DOES NOT MATCH: "
                            "region %zu, chart %zu, dimension %zu, point %zu, time %ld: "
                            "expected %lld, found %f\n",
                    region, chart, dim, point, now, expected, sp.sum);
        }

        (*value_errors)++;
    }

    if(sp.start_time_s > now || sp.end_time_s < now) {
        if(*time_errors < DIMS * 2) {
            fprintf(stderr, " >>> DBENGINE: TIMESTAMP DOES NOT MATCH: "
                            "region %zu, chart %zu, dimension %zu, point %zu, timestamp %ld: "
                            "expected %ld, found %ld - %ld\n",
                    region, chart, dim, point, now, now, sp.start_time_s, sp.end_time_s);
        }

        (*time_errors)++;
    }

    if(update_every != sp.end_time_s - sp.start_time_s) {
        if(*update_every_errors < DIMS * 2) {
            fprintf(stderr, " >>> DBENGINE: UPDATE EVERY DOES NOT MATCH: "
                            "region %zu, chart %zu, dimension %zu, point %zu, timestamp %ld: "
                            "expected %ld, found %ld\n",
                    region, chart, dim, point, now, update_every, sp.end_time_s - sp.start_time_s);
        }

        (*update_every_errors)++;
    }
}

static inline void rrddim_set_by_pointer_fake_time(RRDDIM *rd, collected_number value, time_t now) {
    rd->collector.last_collected_time.tv_sec = now;
    rd->collector.last_collected_time.tv_usec = 0;
    rrddim_set_collected_int(rd, value);
    rrddim_set_updated(rd);

    rd->collector.counter++;

    rrddim_update_collected_max_from_int(rd, value);
}

static RRDHOST *dbengine_rrdhost_find_or_create(char *name) {
    /* We don't want to drop metrics when generating load,
     * we prefer to block data generation itself */

    SYSTEM_TZ tz = system_tz_get();
    RRDHOST *host = rrdhost_find_or_create(
        name,
        name,
        name,
        os_type,
        tz.timezone,
        tz.abbrev_timezone,
        tz.utc_offset,
        program_name,
        NETDATA_VERSION,
        nd_profile.update_every,
        default_rrd_history_entries,
        RRD_DB_MODE_DBENGINE,
        health_plugin_enabled(),
        stream_send.enabled,
        stream_send.parents.destination,
        stream_send.api_key,
        stream_send.send_charts_matching,
        stream_receive.replication.enabled,
        stream_receive.replication.period,
        stream_receive.replication.step,
        NULL,
        0
        );
    system_tz_free(&tz);
    return host;
}

static void test_dbengine_create_charts(RRDHOST *host, RRDSET *st[CHARTS], RRDDIM *rd[CHARTS][DIMS],
                                        int update_every) {
    fprintf(stderr, "DBENGINE Creating Test Charts...\n");

    int i, j;
    char name[101];

    for (i = 0 ; i < CHARTS ; ++i) {
        snprintfz(name, sizeof(name) - 1, "dbengine-chart-%d", i);

        // create the chart
        st[i] = rrdset_create(host, "netdata", name, name, "netdata", NULL, "Unit Testing", "a value", "unittest",
                              NULL, 1, update_every, RRDSET_TYPE_LINE);
        rrdset_flag_set(st[i], RRDSET_FLAG_DEBUG);
        rrdset_flag_set(st[i], RRDSET_FLAG_STORE_FIRST);
        for (j = 0 ; j < DIMS ; ++j) {
            snprintfz(name, sizeof(name) - 1, "dim-%d", j);

            rd[i][j] = rrddim_add(st[i], name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
    }

    // Initialize DB with the very first entries
    for (i = 0 ; i < CHARTS ; ++i) {
        for (j = 0 ; j < DIMS ; ++j) {
            rd[i][j]->collector.last_collected_time.tv_sec =
                st[i]->last_collected_time.tv_sec = st[i]->last_updated.tv_sec = START_TIMESTAMP - 1;
            rd[i][j]->collector.last_collected_time.tv_usec =
                st[i]->last_collected_time.tv_usec = st[i]->last_updated.tv_usec = 0;
        }
    }
    for (i = 0 ; i < CHARTS ; ++i) {
        st[i]->usec_since_last_update = USEC_PER_SEC;

        for (j = 0; j < DIMS; ++j) {
            rrddim_set_by_pointer_fake_time(rd[i][j], 69, START_TIMESTAMP); // set first value to 69
        }

        struct timeval now;
        now_realtime_timeval(&now);
        rrdset_timed_done(st[i], now, false);
    }
    // Flush pages for subsequent real values
    for (i = 0 ; i < CHARTS ; ++i) {
        for (j = 0; j < DIMS; ++j) {
            rrdeng_store_metric_flush_current_page((rd[i][j])->tiers[0].sch);
        }
    }
}

static time_t test_dbengine_create_metrics(
    RRDSET *st[CHARTS],
    RRDDIM *rd[CHARTS][DIMS],
    size_t current_region,
    time_t time_start) {

    time_t update_every = REGION_UPDATE_EVERY[current_region];
    fprintf(stderr, "DBENGINE Single Region Write  to "
                    "region %zu, from %ld to %ld, with update every %ld...\n",
            current_region, time_start, time_start + POINTS_PER_REGION * update_every, update_every);

    // for the database to save the metrics at the right time, we need to set
    // the last data collection time to be just before the first data collection.
    time_t time_now = time_start;
    for (size_t c = 0 ; c < CHARTS ; ++c) {
        for (size_t d = 0 ; d < DIMS ; ++d) {
            storage_engine_store_change_collection_frequency(rd[c][d]->tiers[0].sch, (int)update_every);

            // setting these timestamps, to the data collection time, prevents interpolation
            // during data collection, so that our value will be written as-is to the
            // database.

            rd[c][d]->collector.last_collected_time.tv_sec =
                st[c]->last_collected_time.tv_sec = st[c]->last_updated.tv_sec = time_now;

            rd[c][d]->collector.last_collected_time.tv_usec =
                st[c]->last_collected_time.tv_usec = st[c]->last_updated.tv_usec = 0;
        }
    }

    // set the samples to the database
    for (size_t p = 0; p < POINTS_PER_REGION ; ++p) {
        for (size_t c = 0 ; c < CHARTS ; ++c) {
            st[c]->usec_since_last_update = USEC_PER_SEC * update_every;

            for (size_t d = 0; d < DIMS; ++d)
                rrddim_set_by_pointer_fake_time(rd[c][d], point_value_get(current_region, c, d, p), time_now);

            rrdset_timed_done(st[c], (struct timeval){ .tv_sec = time_now, .tv_usec = 0 }, false);
        }

        time_now += update_every;
    }

    return time_now;
}

// Checks the metric data for the given region, returns number of errors
static size_t test_dbengine_check_metrics(
    RRDSET *st[CHARTS] __maybe_unused,
    RRDDIM *rd[CHARTS][DIMS],
    size_t current_region,
    time_t time_start,
    time_t time_end) {

    time_t update_every = REGION_UPDATE_EVERY[current_region];
    fprintf(stderr, "DBENGINE Single Region Read from "
                    "region %zu, from %ld to %ld, with update every %ld...\n",
            current_region, time_start, time_end, update_every);

    // initialize all queries
    struct storage_engine_query_handle handles[CHARTS * DIMS] = { 0 };
    for (size_t c = 0 ; c < CHARTS ; ++c) {
        for (size_t d = 0; d < DIMS; ++d) {
            storage_engine_query_init(rd[c][d]->tiers[0].seb,
                                      rd[c][d]->tiers[0].smh,
                                      &handles[c * DIMS + d],
                                      time_start,
                                      time_end,
                                      STORAGE_PRIORITY_NORMAL);
        }
    }

    // check the stored samples
    size_t value_errors = 0, time_errors = 0, update_every_errors = 0;
    time_t time_now = time_start;
    for(size_t p = 0; p < POINTS_PER_REGION ;p++) {
        for (size_t c = 0 ; c < CHARTS ; ++c) {
            for (size_t d = 0; d < DIMS; ++d) {
                STORAGE_POINT sp = storage_engine_query_next_metric(&handles[c * DIMS + d]);
                storage_point_check(current_region, c, d, p, time_now, update_every, sp,
                                    &value_errors, &time_errors, &update_every_errors);
            }
        }

        time_now += update_every;
    }

    // finalize the queries
    for (size_t c = 0 ; c < CHARTS ; ++c) {
        for (size_t d = 0; d < DIMS; ++d) {
            storage_engine_query_finalize(&handles[c * DIMS + d]);
        }
    }

    if(value_errors)
        fprintf(stderr, "%zu value errors encountered (out of %d checks)\n", value_errors, POINTS_PER_REGION * CHARTS * DIMS);

    if(time_errors)
        fprintf(stderr, "%zu time errors encountered (out of %d checks)\n", time_errors, POINTS_PER_REGION * CHARTS * DIMS);

    if(update_every_errors)
        fprintf(stderr, "%zu update every errors encountered (out of %d checks)\n", update_every_errors, POINTS_PER_REGION * CHARTS * DIMS);

    return value_errors + time_errors + update_every_errors;
}

static size_t dbengine_test_rrdr_single_region(
    RRDSET *st[CHARTS],
    RRDDIM *rd[CHARTS][DIMS],
    size_t current_region,
    time_t time_start,
    time_t time_end) {

    time_t update_every = REGION_UPDATE_EVERY[current_region];
    fprintf(stderr, "RRDR Single Region Test on "
                    "region %zu, start time %lld, end time %lld, update every %ld, on %d dimensions...\n",
            current_region, (long long)time_start, (long long)time_end, update_every, CHARTS * DIMS);

    size_t errors = 0, value_errors = 0, time_errors = 0, update_every_errors = 0;
    long points = (time_end - time_start) / update_every;
    for(size_t c = 0; c < CHARTS ;c++) {
        ONEWAYALLOC *owa = onewayalloc_create(0);
        RRDR *r = rrd2rrdr_legacy(owa, st[c], points, time_start, time_end,
                                  RRDR_GROUPING_AVERAGE, 0, RRDR_OPTION_NATURAL_POINTS,
                                  NULL, NULL, 0, 0,
                                  QUERY_SOURCE_UNITTEST, STORAGE_PRIORITY_NORMAL);
        if (!r) {
            fprintf(stderr, " >>> DBENGINE: %s: empty RRDR on region %zu\n", rrdset_name(st[c]), current_region);
            onewayalloc_destroy(owa);
            errors++;
            continue;
        }

        if(r->internal.qt->request.st != st[c])
            fatal("queried wrong chart");

        if(rrdr_rows(r) != POINTS_PER_REGION)
            fatal("query returned wrong number of points (expected %d, got %zu)", POINTS_PER_REGION, rrdr_rows(r));

        time_t time_now = time_start;
        for (size_t p = 0; p < rrdr_rows(r); p++) {
            size_t d = 0;
            RRDDIM *dim;
            rrddim_foreach_read(dim, r->internal.qt->request.st) {
                if(unlikely(d >= r->d))
                    fatal("got more dimensions (%zu) than expected (%zu)", d, r->d);

                if(rd[c][d] != dim)
                    fatal("queried wrong dimension");

                RRDR_VALUE_FLAGS *co = &r->o[ p * r->d ];
                NETDATA_DOUBLE *cn = &r->v[ p * r->d ];

                time_t end_time_s = r->t[p];
                time_t start_time_s = end_time_s - r->view.update_every;
                STORAGE_POINT sp = STORAGE_POINT_UNSET;
                if(co[d] & RRDR_VALUE_EMPTY)
                    storage_point_empty(sp, start_time_s, end_time_s);
                else {
                    sp.min = sp.max = sp.sum = cn[d];
                    sp.count = 1;
                    sp.start_time_s = start_time_s;
                    sp.end_time_s = end_time_s;
                }

                storage_point_check(current_region, c, d, p, time_now, update_every, sp, &value_errors, &time_errors, &update_every_errors);
                d++;
            }
            rrddim_foreach_done(dim);
            time_now += update_every;
        }

        rrdr_free(owa, r);
        onewayalloc_destroy(owa);
    }

    if(value_errors)
        fprintf(stderr, "%zu value errors encountered (out of %d checks)\n", value_errors, POINTS_PER_REGION * CHARTS * DIMS);

    if(time_errors)
        fprintf(stderr, "%zu time errors encountered (out of %d checks)\n", time_errors, POINTS_PER_REGION * CHARTS * DIMS);

    if(update_every_errors)
        fprintf(stderr, "%zu update every errors encountered (out of %d checks)\n", update_every_errors, POINTS_PER_REGION * CHARTS * DIMS);

    return errors + value_errors + time_errors + update_every_errors;
}

static size_t test_dbengine_burst_retention_case(
    RRDHOST *host, const char *id,
    size_t points, size_t leading_gaps, bool flush_before_check,
    time_t expected_first, time_t expected_last) {
    size_t errors = 0;

    fprintf(stderr, "DBENGINE Burst Retention Test: '%s', %zu points (%zu leading gaps)...\n",
            id, points, leading_gaps);

    RRDSET *st = rrdset_create(host, "netdata", id, id, "netdata", NULL, "Unit Testing", "a value", "unittest",
                               NULL, 1, 1, RRDSET_TYPE_LINE);
    RRDDIM *rd = rrddim_add(st, "dim", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    // burst-store points at fixed timestamps, the way the streaming receiver
    // stores replicated history of a newly-created chart: many points land in
    // the open hot page before any retention reader runs
    time_t t0 = START_TIMESTAMP;
    for(size_t p = 0; p < points; p++) {
        if(p < leading_gaps)
            rrddim_store_metric(rd, (usec_t)(t0 + 1 + (time_t)p) * USEC_PER_SEC, NAN, SN_EMPTY_SLOT);
        else
            rrddim_store_metric(rd, (usec_t)(t0 + 1 + (time_t)p) * USEC_PER_SEC, (NETDATA_DOUBLE)(p % 10), SN_DEFAULT_FLAGS);
    }

    // gap-only cases must flush before the first retention read: pages that
    // never received a real value are discarded at flush and must not leave
    // any retention behind
    if(flush_before_check) {
        for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
            storage_engine_store_flush(rd->tiers[tier].sch);
    }

    // this is the first retention reader; it used to stamp first_time_s from
    // the newest hot point (sub-page bursts) or the first page flush had
    // already stamped the page end time (multi-page bursts), hiding all
    // earlier points of the burst from queries until a restart
    time_t first = 0, last = 0;
    rrdeng_metric_retention_by_id(host->db[0].si, rd->uuid, &first, &last);

    if(first != expected_first) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' first_entry is %ld, expected %ld\n",
                id, first, expected_first);
        errors++;
    }

    if(last != expected_last) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' last_entry is %ld, expected %ld\n",
                id, last, expected_last);
        errors++;
    }

    // a second read must return the same retention (the first read must not
    // have persisted anything different)
    time_t first2 = 0, last2 = 0;
    rrdeng_metric_retention_by_id(host->db[0].si, rd->uuid, &first2, &last2);

    if(first2 != first || last2 != last) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' retention is not stable across reads: "
                        "%ld - %ld, then %ld - %ld\n",
                id, first, last, first2, last2);
        errors++;
    }

    return errors;
}

static size_t test_dbengine_burst_retention(RRDHOST *host) {
    size_t errors = 0;
    time_t t0 = START_TIMESTAMP;

    // sub-page burst: no page flush happens while storing
    errors += test_dbengine_burst_retention_case(host, "dbengine-burst-subpage",
                                                 100, 0, false, t0 + 1, t0 + 100);

    // multi-page burst: the first pages fill and are flushed while storing
    errors += test_dbengine_burst_retention_case(host, "dbengine-burst-multipage",
                                                 4000, 0, false, t0 + 1, t0 + 4000);

    // leading gaps: retention starts at the page start (like journal replay
    // registers pages with leading gaps), stamped when the first real value
    // arrives
    errors += test_dbengine_burst_retention_case(host, "dbengine-burst-leading-gaps",
                                                 100, 5, false, t0 + 1, t0 + 100);

    // gap-only pages: discarded at flush, so the page start must NOT be
    // recorded as retention; what remains is the pre-existing reader
    // fallback from latest_time_s_hot (which survives the flush - the
    // flush's clear is filtered out by mrg_metric_set_hot_latest_time_s
    // ignoring zero), giving first == last == the last gap time
    errors += test_dbengine_burst_retention_case(host, "dbengine-burst-all-gaps",
                                                 100, 100, true, t0 + 100, t0 + 100);

    // gap-only pages discarded at page-full rotation: same, at capacity
    errors += test_dbengine_burst_retention_case(host, "dbengine-burst-all-gaps-capacity",
                                                 1100, 1100, true, t0 + 1100, t0 + 1100);

    return errors;
}

typedef struct dbengine_expected_point {
    time_t start_time_s;
    time_t end_time_s;
    NETDATA_DOUBLE value;
    bool is_gap;
} DBENGINE_EXPECTED_POINT;

static RRDDIM *dbengine_test_create_metric(RRDHOST *host, const char *id_prefix, int update_every) {
    char id[128];
    snprintfz(id, sizeof(id) - 1, "%s-%d", id_prefix, getpid());

    RRDSET *st = rrdset_create(
        host, "netdata", id, id, "netdata", NULL, "Unit Testing", "a value", "unittest",
        NULL, 1, update_every, RRDSET_TYPE_LINE);
    RRDDIM *rd = rrddim_add(st, "dim", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    if(!rd || rd->tiers[0].seb != STORAGE_ENGINE_BACKEND_DBENGINE ||
       !rd->tiers[0].smh || !rd->tiers[0].sch) {
        fprintf(stderr, " >>> DBENGINE: %s metric initialization failed\n", id);
        return NULL;
    }

    return rd;
}

static void dbengine_test_store_point(RRDDIM *rd, time_t end_time_s, NETDATA_DOUBLE value) {
    unittest_storage_engine_store_metric(
        rd->tiers[0].sch, (usec_t)end_time_s * USEC_PER_SEC,
        value, value, value, 1, 0, SN_DEFAULT_FLAGS);
}

static Word_t dbengine_test_metric_id(RRDDIM *rd) {
    return mrg_metric_id(main_mrg, (METRIC *)rd->tiers[0].smh);
}

static bool dbengine_test_evict_page(RRDHOST *host, Word_t metric_id, time_t start_time_s, const char *id) {
    PGC_PAGE *page = pgc_page_get_and_acquire(
        main_cache, (Word_t)host->db[0].si, metric_id, start_time_s, PGC_SEARCH_EXACT);
    if(!page) {
        fprintf(stderr, " >>> DBENGINE: %s page is not cached\n", id);
        return false;
    }

    if(!pgc_page_to_clean_evict_or_release(main_cache, page)) {
        fprintf(stderr, " >>> DBENGINE: %s page could not be evicted\n", id);
        return false;
    }

    return true;
}

static bool dbengine_test_add_empty_page(
    RRDHOST *host,
    Word_t metric_id,
    time_t start_time_s,
    time_t end_time_s,
    uint32_t update_every_s,
    const char *id) {
    PGC_ENTRY entry = {
        .section = (Word_t)host->db[0].si,
        .metric_id = metric_id,
        .start_time_s = start_time_s,
        .end_time_s = end_time_s,
        .size = 0,
        .data = PGD_EMPTY,
        .update_every_s = update_every_s,
        .hot = false,
    };
    bool added = false;
    PGC_PAGE *page = pgc_page_add_and_acquire(main_cache, entry, &added);
    if(!page || !added) {
        fprintf(stderr, " >>> DBENGINE: failed to insert %s page\n", id);
        if(page)
            pgc_page_release(main_cache, page);
        return false;
    }

    pgc_page_release(main_cache, page);
    return true;
}

typedef struct dbengine_test_cache_stats {
    size_t open_lookups;
    size_t journal_lookups;
    size_t planned_gaps;
} DBENGINE_TEST_CACHE_STATS;

static DBENGINE_TEST_CACHE_STATS dbengine_test_cache_stats(void) {
    struct rrdeng_cache_efficiency_stats stats = rrdeng_get_cache_efficiency_stats();
    return (DBENGINE_TEST_CACHE_STATS){
        .open_lookups = stats.prep_time_in_open_cache_lookup.count,
        .journal_lookups = stats.prep_time_in_journal_v2_lookup.count,
        .planned_gaps = stats.queries_planned_with_gaps,
    };
}

#define DBENGINE_TEST_STAT_UNCHECKED SIZE_MAX

static size_t dbengine_test_expect_stat_delta(
    const char *id,
    const char *stat,
    size_t before,
    size_t after,
    size_t expected) {
    if(expected == DBENGINE_TEST_STAT_UNCHECKED || after == before + expected)
        return 0;

    fprintf(stderr,
            " >>> DBENGINE: %s used %zu %s, expected %zu\n",
            id, after - before, stat, expected);
    return 1;
}

static size_t dbengine_test_expect_cache_deltas(
    const char *id,
    DBENGINE_TEST_CACHE_STATS before,
    DBENGINE_TEST_CACHE_STATS after,
    size_t expected_open,
    size_t expected_journal,
    size_t expected_planned_gaps) {
    size_t errors = 0;
    errors += dbengine_test_expect_stat_delta(
        id, "open-cache lookups", before.open_lookups, after.open_lookups, expected_open);
    errors += dbengine_test_expect_stat_delta(
        id, "journal lookups", before.journal_lookups, after.journal_lookups, expected_journal);
    errors += dbengine_test_expect_stat_delta(
        id, "planner gaps", before.planned_gaps, after.planned_gaps, expected_planned_gaps);
    return errors;
}

static size_t dbengine_test_query_points(
    RRDDIM *rd,
    time_t start_time_s,
    time_t end_time_s,
    const DBENGINE_EXPECTED_POINT *expected,
    size_t expected_points,
    const char *id) {
    size_t errors = 0;
    struct storage_engine_query_handle seqh = { 0 };
    unittest_storage_engine_query_init(
        rd->tiers[0].seb, rd->tiers[0].smh, &seqh,
        start_time_s, end_time_s, STORAGE_PRIORITY_SYNCHRONOUS);

    size_t points = 0;
    bool finished = false;
    for(size_t safety = 0; safety < 32; safety++) {
        if(unittest_storage_engine_query_is_finished(&seqh)) {
            finished = true;
            break;
        }

        STORAGE_POINT sp = unittest_storage_engine_query_next_metric(&seqh);
        if(points >= expected_points) {
            fprintf(stderr,
                    " >>> DBENGINE: %s returned unexpected point %zu: %ld-%ld, value %f\n",
                    id, points, sp.start_time_s, sp.end_time_s, sp.sum);
            errors++;
            points++;
            continue;
        }

        const DBENGINE_EXPECTED_POINT *ep = &expected[points];
        if(sp.start_time_s != ep->start_time_s || sp.end_time_s != ep->end_time_s) {
            fprintf(stderr,
                    " >>> DBENGINE: %s point %zu timestamps are %ld-%ld, expected %ld-%ld\n",
                    id, points, sp.start_time_s, sp.end_time_s, ep->start_time_s, ep->end_time_s);
            errors++;
        }

        if(ep->is_gap) {
            if(!storage_point_is_gap(sp) || isfinite(sp.min) || isfinite(sp.max) || isfinite(sp.sum) ||
               sp.count != 0 || sp.gap_count != 1 || sp.anomaly_count != 0 || sp.flags != SN_FLAG_NONE) {
                fprintf(stderr,
                        " >>> DBENGINE: %s point %zu is not the expected gap: "
                        "min=%f max=%f sum=%f count=%u gaps=%u anomalies=%u flags=0x%x\n",
                        id, points, sp.min, sp.max, sp.sum, sp.count, sp.gap_count,
                        sp.anomaly_count, (unsigned)sp.flags);
                errors++;
            }
        }
        else {
            if(sp.min != ep->value || sp.max != ep->value || sp.sum != ep->value) {
                fprintf(stderr,
                        " >>> DBENGINE: %s point %zu value is min=%f max=%f sum=%f, expected %f\n",
                        id, points, sp.min, sp.max, sp.sum, ep->value);
                errors++;
            }

            if(!storage_point_is_complete(sp) || sp.count != 1 || sp.gap_count != 0 ||
               sp.anomaly_count != 0 || sp.flags != SN_DEFAULT_FLAGS) {
                fprintf(stderr,
                        " >>> DBENGINE: %s point %zu metadata is count=%u gaps=%u anomalies=%u flags=0x%x\n",
                        id, points, sp.count, sp.gap_count, sp.anomaly_count, (unsigned)sp.flags);
                errors++;
            }
        }

        points++;
    }

    if(!finished && unittest_storage_engine_query_is_finished(&seqh))
        finished = true;

    if(!finished) {
        fprintf(stderr, " >>> DBENGINE: %s query did not finish within 32 points\n", id);
        errors++;
    }

    if(points != expected_points) {
        fprintf(stderr,
                " >>> DBENGINE: %s returned %zu points, expected %zu\n",
                id, points, expected_points);
        errors++;
    }

    unittest_storage_engine_query_finalize(&seqh);
    return errors;
}

static size_t dbengine_test_query_points_with_cache_deltas(
    RRDDIM *rd,
    time_t start_time_s,
    time_t end_time_s,
    const DBENGINE_EXPECTED_POINT *expected,
    size_t expected_points,
    const char *id,
    size_t expected_open,
    size_t expected_journal,
    size_t expected_planned_gaps) {
    DBENGINE_TEST_CACHE_STATS before = dbengine_test_cache_stats();
    size_t errors = dbengine_test_query_points(
        rd, start_time_s, end_time_s, expected, expected_points, id);
    DBENGINE_TEST_CACHE_STATS after = dbengine_test_cache_stats();
    errors += dbengine_test_expect_cache_deltas(
        id, before, after, expected_open, expected_journal, expected_planned_gaps);
    return errors;
}

static size_t test_dbengine_cadence_page_transitions(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 1000000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-cadence-page-transitions", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 10);
    for(size_t i = 0; i < 4; i++)
        dbengine_test_store_point(rd, t + 170 + (time_t)i * 10, 3 + (NETDATA_DOUBLE)i);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    for(size_t i = 0; i < 8; i++)
        dbengine_test_store_point(rd, t + 230 + (time_t)i * 10, 7 + (NETDATA_DOUBLE)i);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    const DBENGINE_EXPECTED_POINT narrow[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 160, t + 170, 3 },
        { t + 170, t + 180, 4 },
        { t + 180, t + 190, 5 },
        { t + 190, t + 200, 6 },
    };

    const DBENGINE_EXPECTED_POINT wide[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 160, t + 170, 3 },
        { t + 170, t + 180, 4 },
        { t + 180, t + 190, 5 },
        { t + 190, t + 200, 6 },
        { t + 220, t + 230, 7 },
        { t + 230, t + 240, 8 },
        { t + 240, t + 250, 9 },
        { t + 250, t + 260, 10 },
        { t + 260, t + 270, 11 },
        { t + 270, t + 280, 12 },
        { t + 280, t + 290, 13 },
        { t + 290, t + 300, 14 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 200, narrow, _countof(narrow),
        "cadence-page-narrow", DBENGINE_TEST_STAT_UNCHECKED, 0, DBENGINE_TEST_STAT_UNCHECKED);

    struct storage_engine_query_handle seqh = { 0 };
    unittest_storage_engine_query_init(
        rd->tiers[0].seb, rd->tiers[0].smh, &seqh,
        t + 100, t + 175, STORAGE_PRIORITY_SYNCHRONOUS);
    time_t original_start_time_s = seqh.start_time_s;
    time_t aligned_end_time_s = unittest_storage_engine_align_to_optimal_before(&seqh);
    if(original_start_time_s != t + 100 || seqh.start_time_s != original_start_time_s ||
       aligned_end_time_s != t + 200 || seqh.end_time_s != aligned_end_time_s) {
        fprintf(stderr,
                " >>> DBENGINE: cadence-page optimal range is %ld-%ld, expected %ld-%ld\n",
                seqh.start_time_s, seqh.end_time_s, t + 100, t + 200);
        errors++;
    }
    unittest_storage_engine_query_finalize(&seqh);

    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 300, wide, _countof(wide),
        "cadence-page-wide", DBENGINE_TEST_STAT_UNCHECKED, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 300, wide, _countof(wide),
        "cadence-page-wide-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_negative_page_reuse_across_cadence(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2000000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-negative-page-cadence", 1);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 101, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 10);
    for(size_t i = 0; i < 8; i++)
        dbengine_test_store_point(rd, t + 131 + (time_t)i * 10, 3 + (NETDATA_DOUBLE)i);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 99,  t + 100, 1 },
        { t + 100, t + 101, 2 },
        { t + 121, t + 131, 3 },
        { t + 131, t + 141, 4 },
        { t + 141, t + 151, 5 },
        { t + 151, t + 161, 6 },
        { t + 161, t + 171, 7 },
        { t + 171, t + 181, 8 },
        { t + 181, t + 191, 9 },
        { t + 191, t + 201, 10 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 201, expected, _countof(expected),
        "negative-page-first", DBENGINE_TEST_STAT_UNCHECKED, 1, 1);

    const DBENGINE_EXPECTED_POINT inside_gap[] = {
        { t + 121, t + 131, 3 },
        { t + 131, t + 141, 4 },
    };
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 125, t + 141, inside_gap, _countof(inside_gap),
        "negative-page-inside-gap", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 201, expected, _countof(expected),
        "negative-page-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_continuous_slowdown(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2500000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-continuous-slowdown", 1);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 101, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 10);
    dbengine_test_store_point(rd, t + 111, 3);
    dbengine_test_store_point(rd, t + 121, 4);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 99,  t + 100, 1 },
        { t + 100, t + 101, 2 },
        { t + 101, t + 111, 3 },
        { t + 111, t + 121, 4 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 121, expected, _countof(expected),
        "continuous-slowdown", DBENGINE_TEST_STAT_UNCHECKED, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 121, expected, _countof(expected),
        "continuous-slowdown-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_candidate_coverage_finds_real_gap(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2600000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-candidate-real-gap", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 10);
    dbengine_test_store_point(rd, t + 200, 3);
    dbengine_test_store_point(rd, t + 210, 4);
    dbengine_test_store_point(rd, t + 220, 5);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 190, t + 200, 3 },
        { t + 200, t + 210, 4 },
        { t + 210, t + 220, 5 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 220, expected, _countof(expected),
        "candidate-real-gap", DBENGINE_TEST_STAT_UNCHECKED, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 220, expected, _countof(expected),
        "candidate-real-gap-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_candidate_tail_requires_authoritative_lookup(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2606250;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-candidate-tail-gap", 100);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 200, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 10);
    dbengine_test_store_point(rd, t + 210, 3);
    dbengine_test_store_point(rd, t + 220, 4);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 100);
    dbengine_test_store_point(rd, t + 300, 5);
    dbengine_test_store_point(rd, t + 400, 6);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    pgc_flush_dirty_pages(main_cache, (Word_t)host->db[0].si);

    Word_t metric_id = dbengine_test_metric_id(rd);
    if(!dbengine_test_evict_page(host, metric_id, t + 210, "candidate tail"))
        return 1;

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t,       t + 100, 1 },
        { t + 100, t + 200, 2 },
        { t + 200, t + 210, 3 },
        { t + 210, t + 220, 4 },
        { t + 240, t + 250, NAN, true },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 250, expected, _countof(expected),
        "candidate-tail-gap", 1, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 250, expected, _countof(expected),
        "candidate-tail-gap-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_candidate_terminal_equality_is_gap(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2607812;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-candidate-terminal-equality", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 10);
    dbengine_test_store_point(rd, t + 170, 3);
    dbengine_test_store_point(rd, t + 180, 4);
    dbengine_test_store_point(rd, t + 190, 5);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 100);
    dbengine_test_store_point(rd, t + 260, 6);
    dbengine_test_store_point(rd, t + 360, 7);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);
    pgc_flush_dirty_pages(main_cache, (Word_t)host->db[0].si);

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 160, t + 170, 3 },
        { t + 170, t + 180, 4 },
        { t + 180, t + 190, 5 },
        { t + 190, t + 200, NAN, true },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 200, expected, _countof(expected),
        "candidate-terminal-equality", DBENGINE_TEST_STAT_UNCHECKED, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 200, expected, _countof(expected),
        "candidate-terminal-equality-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_tail_before_next_sample_is_not_gap(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2609375;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-tail-before-next-sample", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    dbengine_test_store_point(rd, t + 260, 3);
    dbengine_test_store_point(rd, t + 320, 4);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 200, expected, _countof(expected),
        "tail-before-next-sample", DBENGINE_TEST_STAT_UNCHECKED, 1, 0);

    return errors;
}

static size_t test_dbengine_legacy_cursor_includes_equal_page_end(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2612500;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-legacy-equal-end", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 100);
    dbengine_test_store_point(rd, t + 220, 3);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    dbengine_test_store_point(rd, t + 320, 4);
    dbengine_test_store_point(rd, t + 420, 5);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 120, t + 220, 3 },
        { t + 220, t + 320, 4 },
        { t + 320, t + 420, 5 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 420, expected, _countof(expected),
        "legacy-equal-end", 0, 0, DBENGINE_TEST_STAT_UNCHECKED);

    return errors;
}

static size_t test_dbengine_legacy_completion_equality_requires_authority(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2618750;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-legacy-completion-equality", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 100);
    dbengine_test_store_point(rd, t + 170, 3);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 30);
    dbengine_test_store_point(rd, t + 180, 4);
    dbengine_test_store_point(rd, t + 210, 5);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 100);
    dbengine_test_store_point(rd, t + 250, 6);
    dbengine_test_store_point(rd, t + 350, 7);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);
    pgc_flush_dirty_pages(main_cache, (Word_t)host->db[0].si);

    if(!dbengine_test_evict_page(
           host, dbengine_test_metric_id(rd), t + 180, "legacy completion-equality middle"))
        return 1;

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 70,  t + 170, 3 },
        { t + 150, t + 180, 4 },
        { t + 180, t + 210, 5 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 220, expected, _countof(expected),
        "legacy-completion-equality", 1, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 220, expected, _countof(expected),
        "legacy-completion-equality-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_cadence_overlap_does_not_hide_intermediate_page(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2625000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-cadence-overlap", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 10);
    for(size_t i = 0; i < 4; i++)
        dbengine_test_store_point(rd, t + 170 + (time_t)i * 10, 3 + (NETDATA_DOUBLE)i);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 100);
    dbengine_test_store_point(rd, t + 260, 7);
    dbengine_test_store_point(rd, t + 360, 8);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);
    pgc_flush_dirty_pages(main_cache, (Word_t)host->db[0].si);

    if(!dbengine_test_evict_page(host, dbengine_test_metric_id(rd), t + 170, "cadence-overlap middle"))
        return 1;

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 160, t + 170, 3 },
        { t + 170, t + 180, 4 },
        { t + 180, t + 190, 5 },
        { t + 190, t + 200, 6 },
        { t + 160, t + 260, 7 },
        { t + 260, t + 360, 8 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 360, expected, _countof(expected),
        "cadence-overlap-hidden-page", 1, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 360, expected, _countof(expected),
        "cadence-overlap-hidden-page-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_singleton_bridge_does_not_hide_intermediate_page(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2650000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-singleton-bridge", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 100);
    dbengine_test_store_point(rd, t + 170, 3);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 10);
    for(size_t i = 0; i < 3; i++)
        dbengine_test_store_point(rd, t + 180 + (time_t)i * 10, 4 + (NETDATA_DOUBLE)i);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 100);
    dbengine_test_store_point(rd, t + 250, 7);
    dbengine_test_store_point(rd, t + 350, 8);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);
    pgc_flush_dirty_pages(main_cache, (Word_t)host->db[0].si);

    if(!dbengine_test_evict_page(
           host, dbengine_test_metric_id(rd), t + 180, "singleton-bridge intermediate"))
        return 1;

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 70,  t + 170, 3 },
        { t + 170, t + 180, 4 },
        { t + 180, t + 190, 5 },
        { t + 190, t + 200, 6 },
        { t + 150, t + 250, 7 },
        { t + 250, t + 350, 8 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 350, expected, _countof(expected),
        "singleton-bridge-hidden-page", 1, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 350, expected, _countof(expected),
        "singleton-bridge-hidden-page-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_empty_suffix_does_not_hide_intermediate_page(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2675000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-empty-suffix", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 10);
    for(size_t i = 0; i < 4; i++)
        dbengine_test_store_point(rd, t + 210 + (time_t)i * 10, 3 + (NETDATA_DOUBLE)i);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 100);
    dbengine_test_store_point(rd, t + 260, 7);
    dbengine_test_store_point(rd, t + 360, 8);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);
    pgc_flush_dirty_pages(main_cache, (Word_t)host->db[0].si);

    Word_t metric_id = dbengine_test_metric_id(rd);
    if(!dbengine_test_evict_page(host, metric_id, t + 210, "EMPTY-suffix intermediate"))
        return 1;

    if(!dbengine_test_add_empty_page(host, metric_id, t + 161, t + 200, 0, "EMPTY-suffix"))
        return 1;

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 200, t + 210, 3 },
        { t + 210, t + 220, 4 },
        { t + 220, t + 230, 5 },
        { t + 230, t + 240, 6 },
        { t + 160, t + 260, 7 },
        { t + 260, t + 360, 8 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 360, expected, _countof(expected),
        "empty-suffix-hidden-page", 1, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 360, expected, _countof(expected),
        "empty-suffix-hidden-page-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_empty_cadence_advances_legacy_cursor(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2700000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-empty-cadence-shadow", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 20);
    dbengine_test_store_point(rd, t + 250, 3);
    dbengine_test_store_point(rd, t + 270, 4);
    dbengine_test_store_point(rd, t + 290, 5);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    dbengine_test_store_point(rd, t + 310, 6);
    dbengine_test_store_point(rd, t + 330, 7);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);
    pgc_flush_dirty_pages(main_cache, (Word_t)host->db[0].si);

    if(!dbengine_test_add_empty_page(
           host, dbengine_test_metric_id(rd), t + 161, t + 240, 10, "EMPTY-cadence shadow"))
        return 1;

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 230, t + 250, 3 },
        { t + 250, t + 270, 4 },
        { t + 270, t + 290, 5 },
        { t + 290, t + 310, 6 },
        { t + 310, t + 330, 7 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 330, expected, _countof(expected),
        "empty-cadence-shadow", 0, 0, DBENGINE_TEST_STAT_UNCHECKED);

    return errors;
}

static size_t test_dbengine_single_missing_sample_reuse(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2750000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-single-missing-sample", 1);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 101, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    dbengine_test_store_point(rd, t + 103, 3);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 99,  t + 100, 1 },
        { t + 100, t + 101, 2 },
        { t + 102, t + 103, 3 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 103, expected, _countof(expected),
        "single-missing-sample-first", DBENGINE_TEST_STAT_UNCHECKED, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 103, expected, _countof(expected),
        "single-missing-sample-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_existing_negative_page_continuity(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 3000000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-existing-negative-page", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    dbengine_test_store_point(rd, t + 360, 3);
    dbengine_test_store_point(rd, t + 420, 4);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    if(!dbengine_test_add_empty_page(
           host, dbengine_test_metric_id(rd), t + 220, t + 300, 0, "existing negative"))
        return 1;

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 300, t + 360, 3 },
        { t + 360, t + 420, 4 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 420, expected, _countof(expected),
        "existing-negative-page-first", DBENGINE_TEST_STAT_UNCHECKED, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 420, expected, _countof(expected),
        "existing-negative-page-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_negative_page_does_not_hide_fine_page(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 3125000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-negative-before-fine", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 10);
    for(size_t i = 0; i < 4; i++)
        dbengine_test_store_point(rd, t + 170 + (time_t)i * 10, 3 + (NETDATA_DOUBLE)i);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    for(size_t i = 0; i < 4; i++)
        dbengine_test_store_point(rd, t + 230 + (time_t)i * 10, 7 + (NETDATA_DOUBLE)i);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);
    pgc_flush_dirty_pages(main_cache, (Word_t)host->db[0].si);

    Word_t metric_id = dbengine_test_metric_id(rd);
    if(!dbengine_test_evict_page(host, metric_id, t + 170, "negative-before-fine"))
        return 1;

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 160, t + 170, 3 },
        { t + 170, t + 180, 4 },
        { t + 180, t + 190, 5 },
        { t + 190, t + 200, 6 },
        { t + 220, t + 230, 7 },
        { t + 230, t + 240, 8 },
        { t + 240, t + 250, 9 },
        { t + 250, t + 260, 10 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 260, expected, _countof(expected),
        "negative-before-fine-first", 1, 1, 1);

    if(!dbengine_test_evict_page(host, metric_id, t + 170, "recovered negative-before-fine"))
        return errors + 1;

    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 260, expected, _countof(expected),
        "negative-before-fine-repeat", 1, 0, 0);

    return errors;
}

static size_t test_dbengine_cadence_bearing_empty_pages(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 3250000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-cadence-bearing-empty", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 10);
    dbengine_test_store_point(rd, t + 210, 3);
    dbengine_test_store_point(rd, t + 220, 4);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    dbengine_test_store_point(rd, t + 270, 5);
    dbengine_test_store_point(rd, t + 280, 6);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);
    pgc_flush_dirty_pages(main_cache, (Word_t)host->db[0].si);

    Word_t metric_id = dbengine_test_metric_id(rd);
    if(!dbengine_test_evict_page(host, metric_id, t + 210, "cadence-bearing EMPTY middle"))
        return 1;

    const struct {
        time_t first;
        time_t last;
    } empty_ranges[] = {
        { t + 170, t + 200 },
        { t + 230, t + 260 },
    };

    for(size_t i = 0; i < _countof(empty_ranges); i++) {
        if(!dbengine_test_add_empty_page(
               host, metric_id, empty_ranges[i].first, empty_ranges[i].last, 10,
               "cadence-bearing EMPTY"))
            return 1;
    }

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
        { t + 200, t + 210, 3 },
        { t + 210, t + 220, 4 },
        { t + 260, t + 270, 5 },
        { t + 270, t + 280, 6 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 280, expected, _countof(expected),
        "cadence-bearing-empty-pages", 1, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 280, expected, _countof(expected),
        "cadence-bearing-empty-pages-repeat", DBENGINE_TEST_STAT_UNCHECKED, 0, 0);

    return errors;
}

static size_t test_dbengine_inclusive_cache_frontier(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 3500000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-inclusive-cache-frontier", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 10);
    for(size_t i = 0; i < 4; i++)
        dbengine_test_store_point(rd, t + 170 + (time_t)i * 10, 3 + (NETDATA_DOUBLE)i);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);
    pgc_flush_dirty_pages(main_cache, (Word_t)host->db[0].si);

    if(!dbengine_test_evict_page(
           host, dbengine_test_metric_id(rd), t + 100, "inclusive cache-frontier first"))
        return 1;

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 100, t + 160, 2 },
        { t + 160, t + 170, 3 },
        { t + 170, t + 180, 4 },
        { t + 180, t + 190, 5 },
        { t + 190, t + 200, 6 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 160, t + 200, expected, _countof(expected),
        "inclusive-cache-frontier", 1, 0, DBENGINE_TEST_STAT_UNCHECKED);

    return errors;
}

static size_t test_dbengine_long_collection_cadence(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 4000000;
    const uint32_t update_every = 90000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-long-cadence", (int)update_every);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + update_every, 1);
    dbengine_test_store_point(rd, t + 2 * update_every, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t,                t + update_every,     1 },
        { t + update_every, t + 2 * update_every, 2 },
    };

    size_t invalid_before =
        rrdeng_get_cache_efficiency_stats().pages_invalid_update_every_fixed;
    size_t errors = dbengine_test_query_points(
        rd, t + update_every, t + 2 * update_every,
        expected, _countof(expected), "long-cadence");
    size_t invalid_after =
        rrdeng_get_cache_efficiency_stats().pages_invalid_update_every_fixed;

    if(invalid_after != invalid_before) {
        fprintf(stderr,
                " >>> DBENGINE: valid long-cadence query reported %zu invalid page durations, expected 0\n",
                invalid_after - invalid_before);
        errors++;
    }

    return errors;
}

static size_t test_dbengine_zero_page_cadence_is_repaired(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 4250000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-zero-page-cadence", 10);
    if(!rd)
        return 1;

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 0);
    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 110, 2);

    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)rd->tiers[0].sch;
    if(!handle->pgc_page || pgc_page_data(handle->pgc_page) == PGD_EMPTY ||
       pgd_slots_used(pgc_page_data(handle->pgc_page)) != 2 ||
       pgc_page_start_time_s(handle->pgc_page) != t + 100 ||
       pgc_page_end_time_s(handle->pgc_page) != t + 110 ||
       pgc_page_update_every_s(handle->pgc_page) != 0) {
        fprintf(stderr,
                " >>> DBENGINE: zero-page-cadence fixture did not retain a nonempty 0-second page\n");
        unittest_storage_engine_store_flush(rd->tiers[0].sch);
        return 1;
    }

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 90,  t + 100, 1 },
        { t + 100, t + 110, 2 },
    };

    size_t invalid_before =
        rrdeng_get_cache_efficiency_stats().pages_invalid_update_every_fixed;
    size_t errors = dbengine_test_query_points(
        rd, t + 100, t + 110, expected, _countof(expected), "zero-page-cadence-first");
    size_t invalid_after_first =
        rrdeng_get_cache_efficiency_stats().pages_invalid_update_every_fixed;

    if(invalid_after_first != invalid_before + 1 ||
       pgc_page_update_every_s(handle->pgc_page) != 10) {
        fprintf(stderr,
                " >>> DBENGINE: zero page cadence repairs=%zu cadence=%u, expected 1 and 10\n",
                invalid_after_first - invalid_before,
                pgc_page_update_every_s(handle->pgc_page));
        errors++;
    }

    errors += dbengine_test_query_points(
        rd, t + 100, t + 110, expected, _countof(expected), "zero-page-cadence-repeat");
    size_t invalid_after_repeat =
        rrdeng_get_cache_efficiency_stats().pages_invalid_update_every_fixed;

    if(invalid_after_repeat != invalid_after_first) {
        fprintf(stderr,
                " >>> DBENGINE: repeated zero-page-cadence query reported %zu repairs, expected 0\n",
                invalid_after_repeat - invalid_after_first);
        errors++;
    }

    unittest_storage_engine_store_flush(rd->tiers[0].sch);
    return errors;
}

static size_t test_dbengine_finish_skips_negative_page(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 4500000;
    RRDDIM *rd = dbengine_test_create_metric(host, "dbengine-finish-skips-negative", 60);
    if(!rd)
        return 1;

    dbengine_test_store_point(rd, t + 100, 1);
    dbengine_test_store_point(rd, t + 160, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    dbengine_test_store_point(rd, t + 260, 3);
    dbengine_test_store_point(rd, t + 320, 4);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    if(!dbengine_test_add_empty_page(
           host, dbengine_test_metric_id(rd), t + 161, t + 200, 0, "terminal negative"))
        return 1;

    const DBENGINE_EXPECTED_POINT expected[] = {
        { t + 40,  t + 100, 1 },
        { t + 100, t + 160, 2 },
    };

    size_t errors = dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 200,
        expected, _countof(expected), "finish-skips-negative-page",
        0, 0, DBENGINE_TEST_STAT_UNCHECKED);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 202,
        expected, _countof(expected), "finish-after-negative-page", 1, 1, 1);
    errors += dbengine_test_query_points_with_cache_deltas(
        rd, t + 100, t + 202,
        expected, _countof(expected), "finish-after-negative-page-repeat", 0, 0, 0);

    return errors;
}

static size_t test_dbengine_query_page_transitions(RRDHOST *host) {
    size_t errors = 0;
    errors += test_dbengine_cadence_page_transitions(host);
    errors += test_dbengine_negative_page_reuse_across_cadence(host);
    errors += test_dbengine_continuous_slowdown(host);
    errors += test_dbengine_candidate_coverage_finds_real_gap(host);
    errors += test_dbengine_candidate_tail_requires_authoritative_lookup(host);
    errors += test_dbengine_candidate_terminal_equality_is_gap(host);
    errors += test_dbengine_tail_before_next_sample_is_not_gap(host);
    errors += test_dbengine_legacy_cursor_includes_equal_page_end(host);
    errors += test_dbengine_legacy_completion_equality_requires_authority(host);
    errors += test_dbengine_cadence_overlap_does_not_hide_intermediate_page(host);
    errors += test_dbengine_singleton_bridge_does_not_hide_intermediate_page(host);
    errors += test_dbengine_empty_suffix_does_not_hide_intermediate_page(host);
    errors += test_dbengine_empty_cadence_advances_legacy_cursor(host);
    errors += test_dbengine_single_missing_sample_reuse(host);
    errors += test_dbengine_existing_negative_page_continuity(host);
    errors += test_dbengine_negative_page_does_not_hide_fine_page(host);
    errors += test_dbengine_cadence_bearing_empty_pages(host);
    errors += test_dbengine_inclusive_cache_frontier(host);
    errors += test_dbengine_long_collection_cadence(host);
    errors += test_dbengine_zero_page_cadence_is_repaired(host);
    errors += test_dbengine_finish_skips_negative_page(host);
    return errors;
}

int test_dbengine(void) {
    // provide enough threads to dbengine
    setenv("UV_THREADPOOL_SIZE", "48", 1);

    size_t errors = 0, value_errors = 0, time_errors = 0;

    nd_log_limits_unlimited();
    fprintf(stderr, "\nRunning DB-engine test\n");

    default_rrd_memory_mode = RRD_DB_MODE_DBENGINE;
    fprintf(stderr, "Initializing localhost with hostname 'unittest-dbengine'");
    RRDHOST *host = dbengine_rrdhost_find_or_create("unittest-dbengine");
    if(!host)
        fatal("Failed to initialize host");

    errors += (size_t)pgd_storage_point_unittest();
    errors += test_dbengine_burst_retention(host);

    RRDSET *st[CHARTS] = { 0 };
    RRDDIM *rd[CHARTS][DIMS] = { 0 };
    time_t time_start[REGIONS] = { 0 }, time_end[REGIONS] = { 0 };

    // create the charts and dimensions we need
    test_dbengine_create_charts(host, st, rd, REGION_UPDATE_EVERY[0]);

    time_t now = START_TIMESTAMP;
    time_t update_every_old = REGION_UPDATE_EVERY[0];
    for(size_t current_region = 0; current_region < REGIONS ;current_region++) {
        time_t update_every = REGION_UPDATE_EVERY[current_region];

        if(update_every != update_every_old) {
            for (size_t c = 0 ; c < CHARTS ; ++c)
                rrdset_set_update_every_s(st[c], update_every);
        }

        time_start[current_region] = region_start_time(now, update_every);
        now = time_end[current_region] = test_dbengine_create_metrics(st,rd, current_region, time_start[current_region]);

        errors += test_dbengine_check_metrics(st, rd, current_region, time_start[current_region], time_end[current_region]);
    }

    // check everything again
    for(size_t current_region = 0; current_region < REGIONS ;current_region++)
        errors += test_dbengine_check_metrics(st, rd, current_region, time_start[current_region], time_end[current_region]);

    // check again in reverse order
    for(size_t current_region = 0; current_region < REGIONS ;current_region++) {
        size_t region = REGIONS - 1 - current_region;
        errors += test_dbengine_check_metrics(st, rd, region, time_start[region], time_end[region]);
    }

    // check all the regions using RRDR
    // this also checks the query planner and the query engine of Netdata
    for (size_t current_region = 0 ; current_region < REGIONS ; current_region++) {
        errors += dbengine_test_rrdr_single_region(st, rd, current_region, time_start[current_region], time_end[current_region]);
    }

    errors += test_dbengine_query_page_transitions(host);

    // prevent closing the database before the test is finished
    sleep(5);

    rrd_wrlock();
    rrdeng_quiesce((struct rrdengine_instance *)host->db[0].si);
    rrdeng_flush_all((struct rrdengine_instance *)host->db[0].si);
    rrdeng_exit((struct rrdengine_instance *)host->db[0].si);
    rrdeng_enq_cmd(NULL, RRDENG_OPCODE_SHUTDOWN_EVLOOP, NULL, NULL, STORAGE_PRIORITY_BEST_EFFORT, NULL, NULL);
    rrd_wrunlock();

    return (int)(errors + value_errors + time_errors);
}

#endif
