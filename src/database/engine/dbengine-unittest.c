// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "database/rrddim-collection.h"

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

                STORAGE_POINT sp = STORAGE_POINT_UNSET;
                sp.min = sp.max = sp.sum = (co[d] & RRDR_VALUE_EMPTY) ? NAN :cn[d];
                sp.count = 1;
                sp.end_time_s = r->t[p];
                sp.start_time_s = sp.end_time_s - r->view.update_every;

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

int test_dbengine(void) {
    size_t errors = 0, value_errors = 0, time_errors = 0;

    nd_log_limits_unlimited();
    fprintf(stderr, "\nRunning DB-engine test\n");

    default_rrd_memory_mode = RRD_DB_MODE_DBENGINE;
    fprintf(stderr, "Initializing localhost with hostname 'unittest-dbengine'");
    RRDHOST *host = dbengine_rrdhost_find_or_create("unittest-dbengine");
    if(!host)
        fatal("Failed to initialize host");

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
