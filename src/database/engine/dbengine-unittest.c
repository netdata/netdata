// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "database/rrddim-collection.h"
#include "daemon/unit_test_bridge.h"

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
                            "region %zu, chart %zu, dimension %zu, point %zu, time %" PRId64 ": "
                            "expected %lld, found %f\n",
                    region, chart, dim, point, (int64_t)now, expected, sp.sum);
        }

        (*value_errors)++;
    }

    if(sp.start_time_s > now || sp.end_time_s < now) {
        if(*time_errors < DIMS * 2) {
            fprintf(stderr, " >>> DBENGINE: TIMESTAMP DOES NOT MATCH: "
                            "region %zu, chart %zu, dimension %zu, point %zu, timestamp %" PRId64 ": "
                            "expected %" PRId64 ", found %" PRId64 " - %" PRId64 "\n",
                    region, chart, dim, point, (int64_t)now, (int64_t)now, (int64_t)sp.start_time_s, (int64_t)sp.end_time_s);
        }

        (*time_errors)++;
    }

    if(update_every != sp.end_time_s - sp.start_time_s) {
        if(*update_every_errors < DIMS * 2) {
            fprintf(stderr, " >>> DBENGINE: UPDATE EVERY DOES NOT MATCH: "
                            "region %zu, chart %zu, dimension %zu, point %zu, timestamp %" PRId64 ": "
                            "expected %" PRId64 ", found %" PRId64 "\n",
                    region, chart, dim, point, (int64_t)now, (int64_t)update_every, (int64_t)(sp.end_time_s - sp.start_time_s));
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

static RRDHOST *dbengine_rrdhost_find_or_create_mode(char *name, RRD_DB_MODE mode, int history) {
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
        history,
        mode,
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

    // nothing removes hosts while these tests run
    if (host)
        rrdhost_pin_release(host);

    return host;
}

static RRDHOST *dbengine_rrdhost_find_or_create(char *name) {
    return dbengine_rrdhost_find_or_create_mode(name, RRD_DB_MODE_DBENGINE, default_rrd_history_entries);
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
                    "region %zu, from %" PRId64 " to %" PRId64 ", with update every %" PRId64 "...\n",
            current_region, (int64_t)time_start, (int64_t)(time_start + POINTS_PER_REGION * update_every), (int64_t)update_every);

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
                    "region %zu, from %" PRId64 " to %" PRId64 ", with update every %" PRId64 "...\n",
            current_region, (int64_t)time_start, (int64_t)time_end, (int64_t)update_every);

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
                    "region %zu, start time %lld, end time %lld, update every %lld, on %d dimensions...\n",
            current_region, (long long)time_start, (long long)time_end, (long long)update_every, CHARTS * DIMS);

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
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' first_entry is %" PRId64 ", expected %" PRId64 "\n",
                id, (int64_t)first, (int64_t)expected_first);
        errors++;
    }

    if(last != expected_last) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' last_entry is %" PRId64 ", expected %" PRId64 "\n",
                id, (int64_t)last, (int64_t)expected_last);
        errors++;
    }

    // a second read must return the same retention (the first read must not
    // have persisted anything different)
    time_t first2 = 0, last2 = 0;
    rrdeng_metric_retention_by_id(host->db[0].si, rd->uuid, &first2, &last2);

    if(first2 != first || last2 != last) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' retention is not stable across reads: "
                        "%" PRId64 " - %" PRId64 ", then %" PRId64 " - %" PRId64 "\n",
                id, (int64_t)first, (int64_t)last, (int64_t)first2, (int64_t)last2);
        errors++;
    }

    return errors;
}

// a gap-only metric whose last point exactly fills its page: the page-full flush discards the page, and storing that
// point sets the hot time again with no page behind it, so the final flush finds no page and must still leave no
// retention behind
static size_t test_dbengine_burst_retention_page_full_case(RRDHOST *host, const char *id) {
    size_t errors = 0;

    fprintf(stderr, "DBENGINE Burst Retention Test: '%s', gap-only points exactly filling a page...\n", id);

    RRDSET *st = rrdset_create(host, "netdata", id, id, "netdata", NULL, "Unit Testing", "a value", "unittest",
                               NULL, 1, 1, RRDSET_TYPE_LINE);
    RRDDIM *rd = rrddim_add(st, "dim", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)rd->tiers[0].sch;

    time_t t0 = START_TIMESTAMP;
    rrddim_store_metric(rd, (usec_t)(t0 + 1) * USEC_PER_SEC, NAN, SN_EMPTY_SLOT);

    size_t capacity = handle->page_entries_max;
    for(size_t p = 1; p < capacity; p++)
        rrddim_store_metric(rd, (usec_t)(t0 + 1 + (time_t)p) * USEC_PER_SEC, NAN, SN_EMPTY_SLOT);

    if(!capacity || handle->pgc_page) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' the page did not fill (capacity %zu)\n", id, capacity);
        return 1;
    }

    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
        storage_engine_store_flush(rd->tiers[tier].sch);

    time_t first = 0, last = 0;
    rrdeng_metric_retention_by_id(host->db[0].si, rd->uuid, &first, &last);

    if(first || last) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' retention is %" PRId64 " - %" PRId64 ", expected none\n",
                id, (int64_t)first, (int64_t)last);
        errors++;
    }

    return errors;
}

// rotation's retention check of a metric whose only page is still hot: the collector may flush that page right
// after the check, so the check must leave the page's end as the clean time - a clean time of 0 would leave the
// metric without retention once the flush clears the hot time, with its page still dirty
static size_t test_dbengine_zero_retention_check_keeps_hot_page(RRDHOST *host, const char *id) {
    fprintf(stderr, "DBENGINE Burst Retention Test: '%s', retention check while the page is hot...\n", id);

    RRDSET *st = rrdset_create(host, "netdata", id, id, "netdata", NULL, "Unit Testing", "a value", "unittest",
                               NULL, 1, 1, RRDSET_TYPE_LINE);
    RRDDIM *rd = rrddim_add(st, "dim", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)rd->tiers[0].sch;

    time_t t0 = START_TIMESTAMP;
    for(size_t p = 0; p < 10; p++)
        rrddim_store_metric(rd, (usec_t)(t0 + 1 + (time_t)p) * USEC_PER_SEC, (NETDATA_DOUBLE)p, SN_DEFAULT_FLAGS);

    if(!handle->pgc_page) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' has no hot page\n", id);
        return 1;
    }

    mrg_metric_has_zero_disk_retention(main_mrg, handle->metric);

    time_t clean = mrg_metric_get_latest_clean_time_s(main_mrg, handle->metric);
    if(clean != t0 + 10) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' clean time is %" PRId64 ", expected %" PRId64 "\n",
                id, (int64_t)clean, (int64_t)(t0 + 10));
        return 1;
    }

    return 0;
}

// rotation's retention check of a metric whose only page is hot and holds gaps only: the flush will discard that
// page, so the check must not record it as retention, or the stopped metric is never deleted
static size_t test_dbengine_zero_retention_check_ignores_gap_only_page(RRDHOST *host, const char *id) {
    fprintf(stderr, "DBENGINE Burst Retention Test: '%s', retention check while a gap-only page is hot...\n", id);

    RRDSET *st = rrdset_create(host, "netdata", id, id, "netdata", NULL, "Unit Testing", "a value", "unittest",
                               NULL, 1, 1, RRDSET_TYPE_LINE);
    RRDDIM *rd = rrddim_add(st, "dim", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)rd->tiers[0].sch;

    time_t t0 = START_TIMESTAMP;
    for(size_t p = 0; p < 10; p++)
        rrddim_store_metric(rd, (usec_t)(t0 + 1 + (time_t)p) * USEC_PER_SEC, NAN, SN_EMPTY_SLOT);

    if(!handle->pgc_page) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' has no hot page\n", id);
        return 1;
    }

    mrg_metric_has_zero_disk_retention(main_mrg, handle->metric);

    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
        storage_engine_store_flush(rd->tiers[tier].sch);

    // the end of retention decides deletion (the first time may keep the hot time the retention reader stamped
    // while the page was hot)
    time_t first = 0, last = 0;
    rrdeng_metric_retention_by_id(host->db[0].si, rd->uuid, &first, &last);
    if(last) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' retention is %" PRId64 " - %" PRId64 ", expected no end\n",
                id, (int64_t)first, (int64_t)last);
        return 1;
    }

    return 0;
}

// two collectors writing the same metric (a duplicate writer): one of them flushing must not clear the hot time the
// other one stored, whether it stored nothing itself or stored before the other, or the metric reports no current
// data until the next store
static size_t test_dbengine_flush_keeps_other_writer_hot_time(RRDHOST *host, const char *id) {
    fprintf(stderr, "DBENGINE Burst Retention Test: '%s', a flush with a duplicate writer...\n", id);

    RRDSET *st = rrdset_create(host, "netdata", id, id, "netdata", NULL, "Unit Testing", "a value", "unittest",
                               NULL, 1, 1, RRDSET_TYPE_LINE);
    RRDDIM *rd = rrddim_add(st, "dim", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    time_t t0 = START_TIMESTAMP;
    for(size_t p = 0; p < 5; p++)
        rrddim_store_metric(rd, (usec_t)(t0 + 1 + (time_t)p) * USEC_PER_SEC, (NETDATA_DOUBLE)p, SN_DEFAULT_FLAGS);

    size_t errors = 0;
    STORAGE_COLLECT_HANDLE *other = rrdeng_store_metric_init(rd->tiers[0].smh, 1, st->smg[0]);
    if(!other) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' failed to create the duplicate writer\n", id);
        return 1;
    }
    METRIC *metric = ((struct rrdeng_collect_handle *)other)->metric;

    // the new writer has stored nothing: its flush must leave the first writer's hot time
    rrdeng_store_metric_flush_current_page(other);
    time_t latest = mrg_metric_get_latest_time_s(main_mrg, metric);
    if(latest != t0 + 5) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' after an idle writer's flush the latest time is %" PRId64
                        ", expected %" PRId64 "\n", id, (int64_t)latest, (int64_t)(t0 + 5));
        errors++;
    }

    // the new writer stores a newer point: the first writer's flush must leave it
    rrdeng_store_metric_next(other, (usec_t)(t0 + 6) * USEC_PER_SEC, 6, 6, 6, 1, 0, SN_DEFAULT_FLAGS);
    rrdeng_store_metric_flush_current_page(rd->tiers[0].sch);
    latest = mrg_metric_get_latest_time_s(main_mrg, metric);
    if(latest != t0 + 6) {
        fprintf(stderr, " >>> DBENGINE: BURST RETENTION: '%s' after the older writer's flush the latest time is %" PRId64
                        ", expected %" PRId64 "\n", id, (int64_t)latest, (int64_t)(t0 + 6));
        errors++;
    }

    rrdeng_store_metric_finalize(other);
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

    // gap-only pages: discarded at flush, and the flush clears the hot
    // time, so nothing is left as retention
    errors += test_dbengine_burst_retention_case(host, "dbengine-burst-all-gaps",
                                                 100, 100, true, 0, 0);

    // gap-only pages discarded at page-full rotation: same, at capacity
    errors += test_dbengine_burst_retention_case(host, "dbengine-burst-all-gaps-capacity",
                                                 1100, 1100, true, 0, 0);

    // the last gap fills the page exactly, so the final flush finds no page
    errors += test_dbengine_burst_retention_page_full_case(host, "dbengine-burst-all-gaps-page-full");

    errors += test_dbengine_zero_retention_check_keeps_hot_page(host, "dbengine-zero-retention-hot-page");
    errors += test_dbengine_zero_retention_check_ignores_gap_only_page(host, "dbengine-zero-retention-gap-only-page");
    errors += test_dbengine_flush_keeps_other_writer_hot_time(host, "dbengine-duplicate-writer-hot-time");

    return errors;
}

typedef struct dbengine_cadence_expected_point {
    time_t start_time_s;
    time_t end_time_s;
    NETDATA_DOUBLE value;
} DBENGINE_CADENCE_EXPECTED_POINT;

static RRDDIM *dbengine_cadence_test_create_metric(
    RRDHOST *host, const char *id_prefix, int update_every) {
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

static void dbengine_cadence_test_store_point(
    RRDDIM *rd, time_t end_time_s, NETDATA_DOUBLE value) {
    unittest_storage_engine_store_metric(
        rd->tiers[0].sch, (usec_t)end_time_s * USEC_PER_SEC,
        value, value, value, 1, 0, SN_DEFAULT_FLAGS);
}

static size_t dbengine_cadence_test_query_points(
    RRDDIM *rd,
    time_t start_time_s,
    time_t end_time_s,
    const DBENGINE_CADENCE_EXPECTED_POINT *expected,
    size_t expected_points,
    const char *id) {
    size_t errors = 0;
    struct storage_engine_query_handle seqh = { 0 };
    unittest_storage_engine_query_init(
        rd->tiers[0].seb, rd->tiers[0].smh, &seqh,
        start_time_s, end_time_s, STORAGE_PRIORITY_SYNCHRONOUS);

    size_t points = 0;
    bool finished = false;
    for(size_t safety = 0; safety < 8; safety++) {
        if(unittest_storage_engine_query_is_finished(&seqh)) {
            finished = true;
            break;
        }

        STORAGE_POINT sp = unittest_storage_engine_query_next_metric(&seqh);
        if(points >= expected_points) {
            fprintf(stderr,
                    " >>> DBENGINE: %s returned unexpected point %zu: %jd-%jd, value %f\n",
                    id, points, (intmax_t)sp.start_time_s, (intmax_t)sp.end_time_s, sp.sum);
            errors++;
            points++;
            continue;
        }

        const DBENGINE_CADENCE_EXPECTED_POINT *ep = &expected[points];
        if(sp.start_time_s != ep->start_time_s || sp.end_time_s != ep->end_time_s ||
           sp.min != ep->value || sp.max != ep->value || sp.sum != ep->value ||
           storage_point_is_gap(sp) || sp.count != 1 || sp.anomaly_count != 0 ||
           sp.flags != SN_DEFAULT_FLAGS) {
            fprintf(stderr,
                    " >>> DBENGINE: %s point %zu is %jd-%jd min=%f max=%f sum=%f count=%u "
                    "anomalies=%u flags=0x%x; expected %jd-%jd value=%f\n",
                    id, points, (intmax_t)sp.start_time_s, (intmax_t)sp.end_time_s,
                    sp.min, sp.max, sp.sum, sp.count, sp.anomaly_count, (unsigned)sp.flags,
                    (intmax_t)ep->start_time_s, (intmax_t)ep->end_time_s, ep->value);
            errors++;
        }

        points++;
    }

    if(!finished && unittest_storage_engine_query_is_finished(&seqh))
        finished = true;

    if(!finished) {
        fprintf(stderr, " >>> DBENGINE: %s query did not finish within 8 points\n", id);
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

// db = ram / alloc: tier 0 is an in-memory dbengine instance. Closed pages are packed and must read back the same;
// a newer page lost to the out of memory protection leaves a gap without hiding the older data; the retention evicts
// the pages that ended before it and moves the metric's first time past them.
#define IN_MEMORY_TEST_POINTS 1000          // the retention of the test host, in points
#define IN_MEMORY_TEST_PAGE_POINTS 50       // points per page (the test closes the pages)

struct in_memory_test_page_filter {
    Word_t section;
    time_t start_time_s;
};

static bool in_memory_test_page_match(PGC_PAGE *page, void *data) {
    struct in_memory_test_page_filter *f = data;
    return pgc_page_section(page) == f->section && pgc_page_start_time_s(page) == f->start_time_s;
}

static size_t in_memory_test_query(RRDDIM *rd, time_t start_s, time_t end_s, time_t base_s, const char *what) {
    size_t errors = 0, points = 0;
    struct storage_engine_query_handle seqh = { 0 };
    unittest_storage_engine_query_init(rd->tiers[0].seb, rd->tiers[0].smh, &seqh, start_s, end_s, STORAGE_PRIORITY_SYNCHRONOUS);

    while(!unittest_storage_engine_query_is_finished(&seqh)) {
        STORAGE_POINT sp = unittest_storage_engine_query_next_metric(&seqh);
        if(storage_point_is_gap(sp))
            continue;

        NETDATA_DOUBLE expected = unpack_storage_number(pack_storage_number((NETDATA_DOUBLE)(sp.end_time_s - base_s + 1), SN_DEFAULT_FLAGS));
        if(sp.count != 1 || sp.sum != expected) {
            fprintf(stderr, " >>> DBENGINE: in memory, %s: point at %jd is %f (count %u), expected %f\n",
                    what, (intmax_t)sp.end_time_s, sp.sum, sp.count, expected);
            errors++;
        }
        points++;
    }
    unittest_storage_engine_query_finalize(&seqh);

    size_t expected_points = (size_t)(end_s - start_s + 1);
    if(points != expected_points) {
        fprintf(stderr, " >>> DBENGINE: in memory, %s: %zu points, expected %zu\n", what, points, expected_points);
        errors++;
    }

    return errors;
}

static size_t test_dbengine_in_memory(void) {
    fprintf(stderr, "\nDBENGINE: testing the in-memory instances of db = ram / alloc...\n");
    size_t errors = 0;

    RRDHOST *host = dbengine_rrdhost_find_or_create_mode("unittest-dbengine-in-memory", RRD_DB_MODE_RAM, IN_MEMORY_TEST_POINTS);
    if(!host)
        return 1;

    struct rrdengine_instance *ctx = (struct rrdengine_instance *)host->db[0].si;
    if(!ctx || !ctx->config.in_memory || ctx->config.in_memory_points != IN_MEMORY_TEST_POINTS || !ram_cache ||
       host->db[0].mode != RRD_DB_MODE_RAM) {
        fprintf(stderr, " >>> DBENGINE: in memory: the host did not get an in-memory instance\n");
        return 1;
    }

    if(rrdeng_in_memory_instance(IN_MEMORY_TEST_POINTS) != ctx) {
        fprintf(stderr, " >>> DBENGINE: in memory: the same retention did not share the instance\n");
        errors++;
    }

    RRDDIM *rd = dbengine_cadence_test_create_metric(host, "unittest-in-memory", 1);
    if(!rd)
        return errors + 1;

    // three pages of recent data (inside the retention by the wall clock, which the out of memory protection of
    // step 2 judges by): points have value = seconds since base + 1
    const time_t base_s = now_realtime_sec() - 4 * IN_MEMORY_TEST_PAGE_POINTS;
    for(size_t page = 0; page < 3; page++) {
        for(size_t i = 0; i < IN_MEMORY_TEST_PAGE_POINTS; i++) {
            time_t t = base_s + (time_t)(page * IN_MEMORY_TEST_PAGE_POINTS + i);
            dbengine_cadence_test_store_point(rd, t, (NETDATA_DOUBLE)(t - base_s + 1));
        }
        unittest_storage_engine_store_flush(rd->tiers[0].sch);
    }

    const time_t last_s = base_s + 3 * IN_MEMORY_TEST_PAGE_POINTS - 1;

    // 1. the closed (packed) pages read back what was stored
    errors += in_memory_test_query(rd, base_s, last_s, base_s, "after packing");

    // 2. the middle page is lost (as the out of memory protection would do): the oldest page stays queryable and
    //    the metric's retention still starts at it
    struct in_memory_test_page_filter f = {
        .section = (Word_t)ctx,
        .start_time_s = base_s + IN_MEMORY_TEST_PAGE_POINTS,
    };
    pgc_evict_clean_pages_matching(ram_cache, in_memory_test_page_match, &f);

    time_t first_s = 0, newest_s = 0;
    if(!rrdeng_metric_retention_by_id((STORAGE_INSTANCE *)ctx, rd->uuid, &first_s, &newest_s) || first_s != base_s) {
        fprintf(stderr, " >>> DBENGINE: in memory: losing a newer page moved the first time to %jd (expected %jd)\n",
                (intmax_t)first_s, (intmax_t)base_s);
        errors++;
    }
    errors += in_memory_test_query(rd, base_s, base_s + IN_MEMORY_TEST_PAGE_POINTS - 1, base_s, "oldest page after a newer page was lost");
    errors += in_memory_test_query(rd, base_s + 2 * IN_MEMORY_TEST_PAGE_POINTS, last_s, base_s, "newest page after a middle page was lost");

    // 3. the retention: at last_s + 1000 points, the oldest page is out of it and the newest is not
    rrdeng_in_memory_enforce_retention(base_s + IN_MEMORY_TEST_PAGE_POINTS - 1 + IN_MEMORY_TEST_POINTS + 1);

    first_s = newest_s = 0;
    rrdeng_metric_retention_by_id((STORAGE_INSTANCE *)ctx, rd->uuid, &first_s, &newest_s);
    if(first_s != base_s + IN_MEMORY_TEST_PAGE_POINTS) {
        fprintf(stderr, " >>> DBENGINE: in memory: after the retention the first time is %jd (expected %jd)\n",
                (intmax_t)first_s, (intmax_t)(base_s + IN_MEMORY_TEST_PAGE_POINTS));
        errors++;
    }
    errors += in_memory_test_query(rd, base_s + 2 * IN_MEMORY_TEST_PAGE_POINTS, last_s, base_s, "newest page after the retention");

    // 4. everything out of the retention: the metric has no data left
    rrdeng_in_memory_enforce_retention(last_s + IN_MEMORY_TEST_POINTS + 1);
    first_s = newest_s = 0;
    if(rrdeng_metric_retention_by_id((STORAGE_INSTANCE *)ctx, rd->uuid, &first_s, &newest_s) && first_s && first_s <= newest_s) {
        fprintf(stderr, " >>> DBENGINE: in memory: retention %jd - %jd left after all pages were evicted\n",
                (intmax_t)first_s, (intmax_t)newest_s);
        errors++;
    }

    rrdset_free(rd->rrdset);

    fprintf(stderr, "DBENGINE: in-memory instances: %s\n", errors ? "FAILED" : "OK");
    return errors;
}

static size_t test_dbengine_long_collection_cadence(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 4000000;
    const uint32_t update_every = 90000;
    RRDDIM *rd = dbengine_cadence_test_create_metric(
        host, "dbengine-long-cadence", (int)update_every);
    if(!rd)
        return 1;

    dbengine_cadence_test_store_point(rd, t + update_every, 1);
    dbengine_cadence_test_store_point(rd, t + 2 * update_every, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    const DBENGINE_CADENCE_EXPECTED_POINT expected[] = {
        { t,                t + update_every,     1 },
        { t + update_every, t + 2 * update_every, 2 },
    };

    size_t invalid_before =
        rrdeng_get_cache_efficiency_stats().pages_invalid_update_every_fixed;
    size_t errors = dbengine_cadence_test_query_points(
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
    RRDDIM *rd = dbengine_cadence_test_create_metric(host, "dbengine-zero-page-cadence", 10);
    if(!rd)
        return 1;

    unittest_storage_engine_store_change_collection_frequency(rd->tiers[0].sch, 0);
    dbengine_cadence_test_store_point(rd, t + 100, 1);
    dbengine_cadence_test_store_point(rd, t + 110, 2);

    size_t errors = 0;
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)rd->tiers[0].sch;
    if(!handle->pgc_page || pgc_page_data(handle->pgc_page) == PGD_EMPTY ||
       pgd_slots_used(pgc_page_data(handle->pgc_page)) != 2 ||
       pgc_page_start_time_s(handle->pgc_page) != t + 100 ||
       pgc_page_end_time_s(handle->pgc_page) != t + 110 ||
       pgc_page_update_every_s(handle->pgc_page) != 0) {
        fprintf(stderr,
                " >>> DBENGINE: zero-page-cadence fixture did not retain a nonempty 0-second page\n");
        errors++;
        goto cleanup;
    }

    const DBENGINE_CADENCE_EXPECTED_POINT expected[] = {
        { t + 90,  t + 100, 1 },
        { t + 100, t + 110, 2 },
    };

    size_t invalid_before =
        rrdeng_get_cache_efficiency_stats().pages_invalid_update_every_fixed;
    errors += dbengine_cadence_test_query_points(
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

    errors += dbengine_cadence_test_query_points(
        rd, t + 100, t + 110, expected, _countof(expected), "zero-page-cadence-repeat");
    size_t invalid_after_repeat =
        rrdeng_get_cache_efficiency_stats().pages_invalid_update_every_fixed;

    if(invalid_after_repeat != invalid_after_first) {
        fprintf(stderr,
                " >>> DBENGINE: repeated zero-page-cadence query reported %zu repairs, expected 0\n",
                invalid_after_repeat - invalid_after_first);
        errors++;
    }

cleanup:
    unittest_storage_engine_store_flush(rd->tiers[0].sch);
    return errors;
}


// The ML training scheduler orders dimensions by rrdeng_store_metric_page_time_to_close_s(): how many seconds of data
// the current page still needs before the point that fills it and makes it flush. Verify it against the engine's own
// page sequence, for a cadence of 1 s and one of 5 s (the first page is shortened to the chart's slot, down to the
// max_slots / 3 floor), by storing points one by one and watching when the page actually flushes. The data is dated months before the local
// clock, so this also checks that the value depends only on the dimension's own timestamps (a lagging child).
static time_t test_dbengine_page_close_of(STORAGE_COLLECT_HANDLE *sch, time_t last_point_s) {
    time_t to_close = rrdeng_store_metric_page_time_to_close_s(sch);
    return to_close < 0 ? 0 : last_point_s + to_close;
}

static size_t test_dbengine_page_close_time(RRDHOST *host) {
    size_t errors = 0;
    const int cadences[] = { 1, 5 };

    for(size_t c = 0; c < _countof(cadences); c++) {
        const int ue = cadences[c];
        char prefix[64];
        snprintfz(prefix, sizeof(prefix) - 1, "dbengine-page-close-%d", ue);

        RRDDIM *rd = dbengine_cadence_test_create_metric(host, prefix, ue);
        if(!rd)
            return errors + 1;

        STORAGE_COLLECT_HANDLE *sch = rd->tiers[0].sch;

        if(rrdeng_store_metric_page_time_to_close_s(sch) != -1) {
            fprintf(stderr, " >>> DBENGINE: page-close ue=%d: a handle without a page must report -1\n", ue);
            errors++;
        }

        // deliberately not aligned to anything, so the first page is alignment-shortened. The data is written
        // BEFORE the region test's time range: pages dated after it, sharing the first datafile with the first
        // region charts, made those charts' region-0 tier-0 pages unreachable in the second verification pass
        // (queries fell back to tier 1). Recorded as a separate observation; this test does not exercise it.
        time_t t = START_TIMESTAMP - 10000000 + 17;
        time_t predicted = 0;       // the close reported while the current page was open (0: no page)
        size_t pages = 0, points = 0;

        while(pages < 4 && points < 20000) {
            dbengine_cadence_test_store_point(rd, t, (NETDATA_DOUBLE)points);
            points++;

            time_t close = test_dbengine_page_close_of(sch, t);

            if(!predicted) {
                // the point just stored opened a page
                if(!close) {
                    fprintf(stderr, " >>> DBENGINE: page-close ue=%d: a stored point must open a page (t=%ld)\n", ue, (long)t);
                    errors++;
                    break;
                }
                predicted = close;
            }
            else if(!close) {
                // the page just flushed: the point that flushed it must be the predicted close
                if(t != predicted) {
                    fprintf(stderr, " >>> DBENGINE: page-close ue=%d: page %zu flushed at %ld, predicted %ld\n", ue, pages, (long)t, (long)predicted);
                    errors++;
                }
                pages++;
                predicted = 0;
            }
            else if(close != predicted) {
                fprintf(stderr, " >>> DBENGINE: page-close ue=%d: close moved from %ld to %ld while the page was open\n", ue, (long)predicted, (long)close);
                errors++;
                predicted = close;
            }

            t += ue;
        }

        if(pages < 4) {
            fprintf(stderr, " >>> DBENGINE: page-close ue=%d: only %zu pages flushed in %zu points\n", ue, pages, points);
            errors++;
        }

        // an explicit flush (chart reset after a gap) leaves no page
        dbengine_cadence_test_store_point(rd, t, 0);
        unittest_storage_engine_store_flush(sch);
        if(rrdeng_store_metric_page_time_to_close_s(sch) != -1) {
            fprintf(stderr, " >>> DBENGINE: page-close ue=%d: a flushed handle must report -1\n", ue);
            errors++;
        }

        // a collection frequency change flushes too, and the next page closes on the new cadence
        t += ue;
        dbengine_cadence_test_store_point(rd, t, 0);
        unittest_storage_engine_store_change_collection_frequency(sch, ue * 2);
        if(rrdeng_store_metric_page_time_to_close_s(sch) != -1) {
            fprintf(stderr, " >>> DBENGINE: page-close ue=%d: a frequency change must flush the page\n", ue);
            errors++;
        }
        t += ue * 2;
        dbengine_cadence_test_store_point(rd, t, 0);
        time_t to_close = rrdeng_store_metric_page_time_to_close_s(sch);
        if(to_close <= 0 || (to_close % (ue * 2)) != 0) {
            fprintf(stderr, " >>> DBENGINE: page-close ue=%d: %ld seconds to close after a frequency change to %d is not on the new cadence\n", ue, (long)to_close, ue * 2);
            errors++;
        }
        unittest_storage_engine_store_flush(sch);
    }

    return errors;
}

// The page phase of a chart (the slot of the page cycle its pages close at) must depend only on the chart's identity:
// a restart creates new metrics groups, and their pages must close exactly where the previous process closed them,
// or data written before the restart is packed into extents in an order nothing reads in after it. For each cadence,
// writers on one metric with the chart's own group, a fresh group of the same identity and a group of another
// identity each open a page at three positions a third of a page cycle apart, every point in its own page cycle so
// the pages never collide. A page must close at the same phase of the cycle (max_slots points) wherever it started,
// except where the max_slots / 3 floor stretches a first page (at most one of the three positions).
#define PAGE_PHASE_POSITIONS 3

static void test_dbengine_page_phase_closes(RRDDIM *rd, STORAGE_METRICS_GROUP *smg, size_t max_slots, int ue,
                                            time_t base, size_t writer, time_t phases[PAGE_PHASE_POSITIONS]) {
    const time_t cycle_s = (time_t)max_slots * ue;
    for(size_t k = 0; k < PAGE_PHASE_POSITIONS; k++) {
        time_t t = base + (time_t)((writer * PAGE_PHASE_POSITIONS + k) * max_slots + k * (max_slots / 3)) * ue;
        STORAGE_COLLECT_HANDLE *sch = rrdeng_store_metric_init(rd->tiers[0].smh, ue, smg);
        if(!sch) {
            phases[k] = -1;
            continue;
        }
        rrdeng_store_metric_next(sch, (usec_t)t * USEC_PER_SEC, 1, 1, 1, 1, 0, SN_DEFAULT_FLAGS);
        time_t to_close = rrdeng_store_metric_page_time_to_close_s(sch);
        phases[k] = to_close < 0 ? -1 : (t + to_close) % cycle_s;
        rrdeng_store_metric_finalize(sch);
    }
}

static size_t test_dbengine_page_phase_seed(RRDHOST *host) {
    size_t errors = 0;
    const int cadences[] = { 1, 5, 60 };

    STORAGE_INSTANCE *si = host->db[0].si;
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)si;
    const size_t max_slots = tier_page_size[ctx->config.tier] / CTX_POINT_SIZE_BYTES(ctx);
    const time_t base = START_TIMESTAMP - 20000000;

    for(size_t c = 0; c < _countof(cadences); c++) {
        const int ue = cadences[c];
        char prefix[64];
        snprintfz(prefix, sizeof(prefix) - 1, "dbengine-page-phase-%d", ue);

        RRDDIM *rd = dbengine_cadence_test_create_metric(host, prefix, ue);
        if(!rd)
            return errors + 1;

        RRDSET *st = rd->rrdset;
        uint64_t seed = rrdset_storage_phase_seed(&host->host_id, rrdset_id(st));
        if(((struct pg_alignment *)st->smg[0])->seed != seed) {
            fprintf(stderr, " >>> DBENGINE: page-phase ue=%d: the chart's metrics group is not seeded from its identity\n", ue);
            errors++;
        }

        // another chart identity on a different slot
        uint64_t other_seed = seed;
        for(size_t i = 0; other_seed % max_slots == seed % max_slots; i++) {
            char id[64];
            snprintfz(id, sizeof(id) - 1, "netdata.dbengine-page-phase-other-%zu", i);
            other_seed = rrdset_storage_phase_seed(&host->host_id, id);
        }

        STORAGE_METRICS_GROUP *same = rrdeng_metrics_group_get(si, seed);
        STORAGE_METRICS_GROUP *other = rrdeng_metrics_group_get(si, other_seed);

        time_t own[PAGE_PHASE_POSITIONS], fresh[PAGE_PHASE_POSITIONS], another[PAGE_PHASE_POSITIONS];
        test_dbengine_page_phase_closes(rd, st->smg[0], max_slots, ue, base, 0, own);
        test_dbengine_page_phase_closes(rd, same, max_slots, ue, base, 1, fresh);
        test_dbengine_page_phase_closes(rd, other, max_slots, ue, base, 2, another);

        rrdeng_metrics_group_release(si, same);
        rrdeng_metrics_group_release(si, other);

        bool other_differs = false;
        size_t agreeing = 0;
        for(size_t k = 0; k < PAGE_PHASE_POSITIONS; k++) {
            if(own[k] < 0 || fresh[k] != own[k]) {
                fprintf(stderr, " >>> DBENGINE: page-phase ue=%d: at position %zu a new metrics group of the same chart "
                                "closes its page at phase %ld s, the chart's own writer at %ld s\n",
                        ue, k, (long)fresh[k], (long)own[k]);
                errors++;
            }
            if(another[k] != own[k])
                other_differs = true;

            size_t same_phase = 0;
            for(size_t j = 0; j < PAGE_PHASE_POSITIONS; j++)
                same_phase += own[j] == own[k];
            if(same_phase > agreeing)
                agreeing = same_phase;
        }
        if(agreeing < PAGE_PHASE_POSITIONS - 1) {
            fprintf(stderr, " >>> DBENGINE: page-phase ue=%d: pages close at phases %ld, %ld, %ld s, depending on where "
                            "they started\n", ue, (long)own[0], (long)own[1], (long)own[2]);
            errors++;
        }
        if(!other_differs) {
            fprintf(stderr, " >>> DBENGINE: page-phase ue=%d: another chart closes its pages where this chart does\n", ue);
            errors++;
        }
    }

    // charts spread over the slots: no flush bursts
    ND_UUID host_id = { 0 };
    host_id.uuid[0] = 1;

    // the same chart id on another host is another identity
    ND_UUID other_host_id = host_id;
    other_host_id.uuid[15] = 1;
    if(rrdset_storage_phase_seed(&host_id, "system.cpu") == rrdset_storage_phase_seed(&other_host_id, "system.cpu")) {
        fprintf(stderr, " >>> DBENGINE: page-phase: the seed ignores the host\n");
        errors++;
    }
    size_t per_slot[max_slots];
    memset(per_slot, 0, sizeof(per_slot));
    size_t used = 0, worst = 0;
    for(size_t i = 0; i < max_slots; i++) {
        char id[64];
        snprintfz(id, sizeof(id) - 1, "system.chart%zu", i);
        size_t slot = rrdset_storage_phase_seed(&host_id, id) % max_slots;
        if(!per_slot[slot]++) used++;
        if(per_slot[slot] > worst) worst = per_slot[slot];
    }
    if(worst > 8 || used < max_slots / 2) {
        fprintf(stderr, " >>> DBENGINE: page-phase: %zu charts use %zu slots, up to %zu in one\n", max_slots, used, worst);
        errors++;
    }

    return errors;
}

static size_t test_dbengine_single_missing_sample_reuse(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 2750000;
    RRDDIM *rd = dbengine_cadence_test_create_metric(host, "dbengine-single-missing-sample", 1);
    if(!rd)
        return 1;

    dbengine_cadence_test_store_point(rd, t + 100, 1);
    dbengine_cadence_test_store_point(rd, t + 101, 2);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);
    dbengine_cadence_test_store_point(rd, t + 103, 3);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    const DBENGINE_CADENCE_EXPECTED_POINT expected[] = {
        { t + 99, t + 100, 1 },
        { t + 100, t + 101, 2 },
        { t + 102, t + 103, 3 },
    };
    struct rrdeng_cache_efficiency_stats before = rrdeng_get_cache_efficiency_stats();
    size_t errors = dbengine_cadence_test_query_points(
        rd, t + 100, t + 103, expected, _countof(expected), "single-missing-sample-first");
    struct rrdeng_cache_efficiency_stats after_first = rrdeng_get_cache_efficiency_stats();

    if(after_first.prep_time_in_journal_v2_lookup.count !=
           before.prep_time_in_journal_v2_lookup.count + 1 ||
       after_first.queries_planned_with_gaps != before.queries_planned_with_gaps + 1) {
        fprintf(stderr,
                " >>> DBENGINE: first single-missing-sample query used %zu journal lookups and %zu planner gaps, "
                "expected 1 and 1\n",
                after_first.prep_time_in_journal_v2_lookup.count - before.prep_time_in_journal_v2_lookup.count,
                after_first.queries_planned_with_gaps - before.queries_planned_with_gaps);
        errors++;
    }

    errors += dbengine_cadence_test_query_points(
        rd, t + 100, t + 103, expected, _countof(expected), "single-missing-sample-repeat");
    struct rrdeng_cache_efficiency_stats after_repeat = rrdeng_get_cache_efficiency_stats();

    if(after_repeat.prep_time_in_journal_v2_lookup.count !=
           after_first.prep_time_in_journal_v2_lookup.count ||
       after_repeat.queries_planned_with_gaps != after_first.queries_planned_with_gaps) {
        fprintf(stderr,
                " >>> DBENGINE: repeated single-missing-sample query used %zu journal lookups and %zu planner gaps, "
                "expected 0 and 0\n",
                after_repeat.prep_time_in_journal_v2_lookup.count -
                    after_first.prep_time_in_journal_v2_lookup.count,
                after_repeat.queries_planned_with_gaps - after_first.queries_planned_with_gaps);
        errors++;
    }

    return errors;
}

static size_t dbengine_crc_test_valid_points(RRDDIM *rd, time_t start_time_s, time_t end_time_s) {
    struct storage_engine_query_handle seqh = { 0 };
    unittest_storage_engine_query_init(
        rd->tiers[0].seb, rd->tiers[0].smh, &seqh, start_time_s, end_time_s, STORAGE_PRIORITY_SYNCHRONOUS);

    size_t points = 0;
    for(size_t safety = 0; safety < 16 && !unittest_storage_engine_query_is_finished(&seqh); safety++) {
        STORAGE_POINT sp = unittest_storage_engine_query_next_metric(&seqh);
        if(!storage_point_is_gap(sp))
            points++;
    }

    unittest_storage_engine_query_finalize(&seqh);
    return points;
}

// a buffered write is visible to the engine's O_DIRECT reads (the kernel writes the range back first), so no fsync
static bool dbengine_crc_test_flip_byte(const char *path, uint64_t offset) {
    int fd = open(path, O_RDWR | O_CLOEXEC);
    if(fd == -1)
        return false;

    uint8_t byte;
    bool ok = pread(fd, &byte, 1, (off_t)offset) == 1;
    if(ok) {
        byte ^= 0xFF;
        ok = pwrite(fd, &byte, 1, (off_t)offset) == 1;
    }
    close(fd);
    return ok;
}

// A CRC-failing extent read from disk must not be cached: neither the extent (extent cache) nor empty pages for it
// (main cache). Otherwise a transient bad read stays sticky and the good bytes on disk are never read again.
// Flip a byte of a flushed extent on disk, query it (it must fail, once), restore the byte and query again: the
// second query must read the extent from disk again and return the original samples.
static size_t test_dbengine_corrupt_extent_is_not_cached(RRDHOST *host) {
    const time_t t = START_TIMESTAMP + 4500000;
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)host->db[0].si;
    RRDDIM *rd = dbengine_cadence_test_create_metric(host, "dbengine-corrupt-extent", 1);
    if(!rd)
        return 1;

    const DBENGINE_CADENCE_EXPECTED_POINT expected[] = {
        { t + 0, t + 1, 1 },
        { t + 1, t + 2, 2 },
        { t + 2, t + 3, 3 },
        { t + 3, t + 4, 4 },
        { t + 4, t + 5, 5 },
    };
    for(size_t i = 0; i < _countof(expected); i++)
        dbengine_cadence_test_store_point(rd, expected[i].end_time_s, expected[i].value);
    unittest_storage_engine_store_flush(rd->tiers[0].sch);

    // write the page to disk; the extent is on disk once the page is in the open cache (the background flusher may
    // have claimed the page first, so the flush below can return before that write completes)
    Word_t metric_id = mrg_metric_id(main_mrg, (METRIC *)rd->tiers[0].smh);
    PGC_PAGE *open_page = NULL;
    for(size_t i = 0; i < 100 && !open_page; i++) {
        if(i)
            sleep_usec(100 * USEC_PER_MS);

        pgc_flush_dirty_pages(main_cache, (Word_t)ctx);
        open_page = pgc_page_get_and_acquire(open_cache, (Word_t)ctx, metric_id, t + 1, PGC_SEARCH_CLOSEST);
    }
    if(!open_page) {
        fprintf(stderr, " >>> DBENGINE: corrupt-extent page did not reach the open cache\n");
        return 1;
    }

    char path[RRDENG_PATH_MAX];
    generate_datafilepath(pgc_page_data(open_page), path, sizeof(path));
    struct extent_io_data *xio = (struct extent_io_data *)pgc_page_custom_data(open_cache, open_page);
    unsigned fileno = xio->fileno;
    uint32_t block = xio->block;
    // the last payload byte: covered by the CRC, and the header stays valid
    uint64_t offset = BLOCK_TO_OFFSET(block) + xio->bytes - sizeof(struct rrdeng_df_extent_trailer) - 1;
    pgc_page_release(open_cache, open_page);

    // drop the clean page from the main cache (unless already evicted), so that the query reads the extent; a
    // background flusher drops its reference to the page just after the open cache entry appears, so retry
    bool evicted = false;
    for(size_t i = 0; i < 100 && !evicted; i++) {
        if(i)
            sleep_usec(10 * USEC_PER_MS);

        PGC_PAGE *page = pgc_page_get_and_acquire(main_cache, (Word_t)ctx, metric_id, t + 1, PGC_SEARCH_EXACT);
        evicted = !page || pgc_page_to_clean_evict_or_release(main_cache, page);
    }
    if(!evicted) {
        fprintf(stderr, " >>> DBENGINE: corrupt-extent page could not be evicted from the main cache\n");
        return 1;
    }

    size_t errors = 0;
    if(!dbengine_crc_test_flip_byte(path, offset)) {
        fprintf(stderr, " >>> DBENGINE: corrupt-extent cannot modify '%s'\n", path);
        return 1;
    }

    rrdeng_stats_t io_errors_before = __atomic_load_n(&ctx->stats.io_errors, __ATOMIC_RELAXED);
    size_t points = dbengine_crc_test_valid_points(rd, t + 1, t + 5);
    rrdeng_stats_t io_errors_after = __atomic_load_n(&ctx->stats.io_errors, __ATOMIC_RELAXED);
    if(points || io_errors_after != io_errors_before + 1) {
        fprintf(stderr, " >>> DBENGINE: corrupt extent returned %zu points and %zu io errors, expected 0 and 1\n",
                points, (size_t)(io_errors_after - io_errors_before));
        errors++;
    }

    PGC_PAGE *extent_page = pgc_page_get_and_acquire(extent_cache, (Word_t)ctx, fileno, (time_t)block, PGC_SEARCH_EXACT);
    if(extent_page) {
        pgc_page_release(extent_cache, extent_page);
        fprintf(stderr, " >>> DBENGINE: a CRC-failing extent was added to the extent cache\n");
        errors++;
    }

    if(!dbengine_crc_test_flip_byte(path, offset)) {
        fprintf(stderr, " >>> DBENGINE: corrupt-extent cannot restore '%s'\n", path);
        return errors + 1;
    }

    errors += dbengine_cadence_test_query_points(
        rd, t + 1, t + 5, expected, _countof(expected), "corrupt-extent-restored");

    if(__atomic_load_n(&ctx->stats.io_errors, __ATOMIC_RELAXED) != io_errors_after) {
        fprintf(stderr, " >>> DBENGINE: restored extent reported io errors\n");
        errors++;
    }

    return errors;
}

static size_t test_dbengine_datafile_names(void) {
    static const struct {
        const char *name;
        bool valid;
        unsigned fileno;
    } cases[] = {
        { "datafile-1-0000000001.ndf", true, 1 },
        { "datafile-1-4294967295.ndf", true, 4294967295U },
        { "datafile-1-0000000001.ndf.bak", false, 0 },
        { "datafile-1-0000000001.ndf~", false, 0 },
        { "datafile-1-0000000001.ndfx", false, 0 },
        { "datafile-1-0000000001.xyz", false, 0 },
        { "datafile-1-0000000001", false, 0 },
        { "datafile-1-1.ndf", false, 0 },
        { "datafile-2-0000000001.ndf", false, 0 },
        { "journalfile-1-0000000001.njf", false, 0 },
    };

    size_t errors = 0;
    for (size_t i = 0; i < _countof(cases); i++) {
        unsigned fileno = 0;
        bool valid = datafile_name_parse(cases[i].name, &fileno);
        if (valid != cases[i].valid || (valid && fileno != cases[i].fileno)) {
            fprintf(stderr, " >>> DBENGINE: datafile name '%s' parsed as %s (fileno %u), expected %s (fileno %u)\n",
                    cases[i].name, valid ? "valid" : "invalid", fileno,
                    cases[i].valid ? "valid" : "invalid", cases[i].fileno);
            errors++;
        }
    }

    return errors;
}

int test_dbengine(void) {
    // provide enough threads to dbengine
    setenv("UV_THREADPOOL_SIZE", "48", 1);

    size_t errors = 0, value_errors = 0, time_errors = 0;

    nd_log_limits_unlimited();
    fprintf(stderr, "\nRunning DB-engine test\n");

    // the host is created in dbengine mode explicitly; do not change the global default, the engine is shut down
    // below and later suites create hosts with the default mode
    fprintf(stderr, "Initializing localhost with hostname 'unittest-dbengine'");
    RRDHOST *host = dbengine_rrdhost_find_or_create("unittest-dbengine");
    if(!host)
        fatal("Failed to initialize host");

    errors += test_dbengine_datafile_names();
    errors += test_dbengine_in_memory();
    errors += test_dbengine_burst_retention(host);
    errors += test_dbengine_page_close_time(host);
    errors += test_dbengine_page_phase_seed(host);

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

    errors += test_dbengine_long_collection_cadence(host);
    errors += test_dbengine_zero_page_cadence_is_repaired(host);

#ifdef ENABLE_ML
    // the ML training scheduler's page-close ordering, driven against this host's dbengine-backed charts;
    // it takes the RRD read lock and stores points, so it must run before the quiesce/exit sequence below
    errors += (size_t)ml_queue_host_order_unittest(host);
#endif

    errors += test_dbengine_single_missing_sample_reuse(host);
    errors += test_dbengine_corrupt_extent_is_not_cached(host);

    // prevent closing the database before the test is finished
    sleep(5);

    struct rrdengine_instance *ctx = (struct rrdengine_instance *)host->db[0].si;
    rrd_wrlock();
    rrdeng_quiesce(ctx);
    rrdeng_flush_all(ctx);
    rrdeng_exit(ctx);
    host->db[0].si = NULL;
    rrd_wrunlock();

    // free the ctx only after the event loop finished its work: after-work callbacks use it
    if(dbengine_shutdown())
        rrdeng_ctx_free(ctx);
    else
        errors++;

    return (int)(errors + value_errors + time_errors);
}

#endif
