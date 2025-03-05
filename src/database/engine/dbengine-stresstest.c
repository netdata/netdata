// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"

#ifdef ENABLE_DBENGINE

static RRDHOST *dbengine_rrdhost_find_or_create(char *name) {
    /* We don't want to drop metrics when generating load,
     * we prefer to block data generation itself */

    return rrdhost_find_or_create(
        name,
        name,
        name,
        os_type,
        netdata_configured_timezone,
        netdata_configured_abbrev_timezone,
        netdata_configured_utc_offset,
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
}

static inline void rrddim_set_by_pointer_fake_time(RRDDIM *rd, collected_number value, time_t now) {
    rd->collector.last_collected_time.tv_sec = now;
    rd->collector.last_collected_time.tv_usec = 0;
    rd->collector.collected_value = value;
    rrddim_set_updated(rd);

    rd->collector.counter++;

    collected_number v = (value >= 0) ? value : -value;
    if(unlikely(v > rd->collector.collected_value_max)) rd->collector.collected_value_max = v;
}

struct dbengine_chart_thread {
    uv_thread_t thread;
    RRDHOST *host;
    char *chartname; /* Will be prefixed by type, e.g. "example_local1.", "example_local2." etc */
    unsigned dset_charts; /* number of charts */
    unsigned dset_dims; /* dimensions per chart */
    unsigned chart_i; /* current chart offset */
    time_t time_present; /* current virtual time of the benchmark */
    volatile time_t time_max; /* latest timestamp of stored values */
    unsigned history_seconds; /* how far back in the past to go */

    volatile long done; /* initialize to 0, set to 1 to stop thread */
    struct completion charts_initialized;
    unsigned long errors, stored_metrics_nr; /* statistics */

    RRDSET *st;
    RRDDIM *rd[]; /* dset_dims elements */
};

collected_number generate_dbengine_chart_value(int chart_i, int dim_i, time_t time_current)
{
    collected_number value;

    value = ((collected_number)time_current) * (chart_i + 1);
    value += ((collected_number)time_current) * (dim_i + 1);
    value %= 1024LLU;

    return value;
}

static void generate_dbengine_chart(void *arg)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    struct dbengine_chart_thread *thread_info = (struct dbengine_chart_thread *)arg;
    RRDHOST *host = thread_info->host;
    char *chartname = thread_info->chartname;
    const unsigned DSET_DIMS = thread_info->dset_dims;
    unsigned history_seconds = thread_info->history_seconds;
    time_t time_present = thread_info->time_present;

    unsigned j, update_every = 1;
    RRDSET *st;
    RRDDIM *rd[DSET_DIMS];
    char name[RRD_ID_LENGTH_MAX + 1];
    time_t time_current;

    // create the chart
    snprintfz(name, RRD_ID_LENGTH_MAX, "example_local%u", thread_info->chart_i + 1);
    thread_info->st = st = rrdset_create(host, name, chartname, chartname, "example", NULL, chartname, chartname,
                                         chartname, NULL, 1, update_every, RRDSET_TYPE_LINE);
    for (j = 0 ; j < DSET_DIMS ; ++j) {
        snprintfz(name, RRD_ID_LENGTH_MAX, "%s%u", chartname, j + 1);

        thread_info->rd[j] = rd[j] = rrddim_add(st, name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    completion_mark_complete(&thread_info->charts_initialized);

    // feed it with the test data
    time_current = time_present - history_seconds;
    for (j = 0 ; j < DSET_DIMS ; ++j) {
        rd[j]->collector.last_collected_time.tv_sec =
            st->last_collected_time.tv_sec = st->last_updated.tv_sec = time_current - update_every;
        rd[j]->collector.last_collected_time.tv_usec =
            st->last_collected_time.tv_usec = st->last_updated.tv_usec = 0;
    }
    for( ; !thread_info->done && time_current < time_present ; time_current += update_every) {
        st->usec_since_last_update = USEC_PER_SEC * update_every;

        for (j = 0; j < DSET_DIMS; ++j) {
            collected_number value;

            value = generate_dbengine_chart_value(thread_info->chart_i, j, time_current);
            rrddim_set_by_pointer_fake_time(rd[j], value, time_current);
            ++thread_info->stored_metrics_nr;
        }
        rrdset_done(st);
        thread_info->time_max = time_current;
    }
    for (j = 0; j < DSET_DIMS; ++j) {
        rrdeng_store_metric_finalize((rd[j])->tiers[0].sch);
    }
}

void generate_dbengine_dataset(unsigned history_seconds)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    const int DSET_CHARTS = 16;
    const int DSET_DIMS = 128;
    const uint64_t EXPECTED_COMPRESSION_RATIO = 20;
    RRDHOST *host = NULL;
    struct dbengine_chart_thread **thread_info;
    int i;
    time_t time_present;

    default_rrd_memory_mode = RRD_DB_MODE_DBENGINE;
    default_rrdeng_page_cache_mb = 128;
    // Worst case for uncompressible data
    default_rrdeng_disk_quota_mb = (((uint64_t)DSET_DIMS * DSET_CHARTS) * sizeof(storage_number) * history_seconds) /
                                   (1024 * 1024);
    default_rrdeng_disk_quota_mb -= default_rrdeng_disk_quota_mb * EXPECTED_COMPRESSION_RATIO / 100;

    nd_log_limits_unlimited();
    fprintf(stderr, "Initializing localhost with hostname 'dbengine-dataset'");

    host = dbengine_rrdhost_find_or_create("dbengine-dataset");
    if (NULL == host)
        return;

    thread_info = mallocz(sizeof(*thread_info) * DSET_CHARTS);
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        thread_info[i] = mallocz(sizeof(*thread_info[i]) + sizeof(RRDDIM *) * DSET_DIMS);
    }
    fprintf(stderr, "\nRunning DB-engine workload generator\n");

    time_present = now_realtime_sec();
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        thread_info[i]->host = host;
        thread_info[i]->chartname = "random";
        thread_info[i]->dset_charts = DSET_CHARTS;
        thread_info[i]->chart_i = i;
        thread_info[i]->dset_dims = DSET_DIMS;
        thread_info[i]->history_seconds = history_seconds;
        thread_info[i]->time_present = time_present;
        thread_info[i]->time_max = 0;
        thread_info[i]->done = 0;
        completion_init(&thread_info[i]->charts_initialized);
        fatal_assert(0 == uv_thread_create(&thread_info[i]->thread, generate_dbengine_chart, thread_info[i]));
        completion_wait_for(&thread_info[i]->charts_initialized);
        completion_destroy(&thread_info[i]->charts_initialized);
    }
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        fatal_assert(0 == uv_thread_join(&thread_info[i]->thread));
    }

    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        freez(thread_info[i]);
    }
    freez(thread_info);
    rrd_wrlock();
    rrdhost_free___while_having_rrd_wrlock(localhost);
    rrd_wrunlock();
}

struct dbengine_query_thread {
    uv_thread_t thread;
    RRDHOST *host;
    char *chartname; /* Will be prefixed by type, e.g. "example_local1.", "example_local2." etc */
    unsigned dset_charts; /* number of charts */
    unsigned dset_dims; /* dimensions per chart */
    time_t time_present; /* current virtual time of the benchmark */
    unsigned history_seconds; /* how far back in the past to go */
    volatile long done; /* initialize to 0, set to 1 to stop thread */
    unsigned long errors, queries_nr, queried_metrics_nr; /* statistics */
    uint8_t delete_old_data; /* if non zero then data are deleted when disk space is exhausted */

    struct dbengine_chart_thread *chart_threads[]; /* dset_charts elements */
};

static void query_dbengine_chart(void *arg)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    struct dbengine_query_thread *thread_info = (struct dbengine_query_thread *)arg;
    const int DSET_CHARTS = thread_info->dset_charts;
    const int DSET_DIMS = thread_info->dset_dims;
    time_t time_after, time_before, time_min, time_approx_min, time_max, duration;
    int i, j, update_every = 1;
    RRDSET *st;
    RRDDIM *rd;
    uint8_t same;
    time_t time_now, time_retrieved, end_time;
    collected_number generatedv;
    NETDATA_DOUBLE value, expected;
    struct storage_engine_query_handle seqh;
    size_t value_errors = 0, time_errors = 0;

    do {
        // pick a chart and dimension
        i = random() % DSET_CHARTS;
        st = thread_info->chart_threads[i]->st;
        j = random() % DSET_DIMS;
        rd = thread_info->chart_threads[i]->rd[j];

        time_min = thread_info->time_present - thread_info->history_seconds + 1;
        time_max = thread_info->chart_threads[i]->time_max;

        if (thread_info->delete_old_data) {
            /* A time window of twice the disk space is sufficient for compression space savings of up to 50% */
            time_approx_min = time_max - (default_rrdeng_disk_quota_mb * 2 * 1024 * 1024) /
                                             (((uint64_t) DSET_DIMS * DSET_CHARTS) * sizeof(storage_number));
            time_min = MAX(time_min, time_approx_min);
        }
        if (!time_max) {
            time_before = time_after = time_min;
        } else {
            time_after = time_min + random() % (MAX(time_max - time_min, 1));
            duration = random() % 3600;
            time_before = MIN(time_after + duration, time_max); /* up to 1 hour queries */
        }

        storage_engine_query_init(rd->tiers[0].seb, rd->tiers[0].smh, &seqh, time_after, time_before, STORAGE_PRIORITY_NORMAL);
        ++thread_info->queries_nr;
        for (time_now = time_after ; time_now <= time_before ; time_now += update_every) {
            generatedv = generate_dbengine_chart_value(i, j, time_now);
            expected = unpack_storage_number(pack_storage_number((NETDATA_DOUBLE) generatedv, SN_DEFAULT_FLAGS));

            if (unlikely(storage_engine_query_is_finished(&seqh))) {
                if (!thread_info->delete_old_data) { /* data validation only when we don't delete */
                    fprintf(stderr, "    DB-engine stresstest %s/%s: at %lu secs, expecting value " NETDATA_DOUBLE_FORMAT
                                    ", found data gap, ### ERROR 12 ###\n",
                            rrdset_name(st), rrddim_name(rd), (unsigned long) time_now, expected);
                    ++thread_info->errors;
                }
                break;
            }

            STORAGE_POINT sp = storage_engine_query_next_metric(&seqh);
            value = sp.sum;
            time_retrieved = sp.start_time_s;
            end_time = sp.end_time_s;

            if (!netdata_double_isnumber(value)) {
                if (!thread_info->delete_old_data) { /* data validation only when we don't delete */
                    fprintf(stderr, "    DB-engine stresstest %s/%s: at %lu secs, expecting value " NETDATA_DOUBLE_FORMAT
                                    ", found data gap, ### ERROR 13 ###\n",
                            rrdset_name(st), rrddim_name(rd), (unsigned long) time_now, expected);
                    ++thread_info->errors;
                }
                break;
            }
            ++thread_info->queried_metrics_nr;

            same = (roundndd(value) == roundndd(expected)) ? 1 : 0;
            if (!same) {
                if (!thread_info->delete_old_data) { /* data validation only when we don't delete */
                    if(!value_errors)
                        fprintf(stderr, "    DB-engine stresstest %s/%s: at %lu secs, expecting value " NETDATA_DOUBLE_FORMAT
                                        ", found " NETDATA_DOUBLE_FORMAT ", ### ERROR 14 ###\n",
                                rrdset_name(st), rrddim_name(rd), (unsigned long) time_now, expected, value);
                    value_errors++;
                    thread_info->errors++;
                }
            }
            if (end_time != time_now) {
                if (!thread_info->delete_old_data) { /* data validation only when we don't delete */
                    if(!time_errors)
                        fprintf(stderr,
                                "    DB-engine stresstest %s/%s: at %lu secs, found timestamp %lu ### ERROR 15 ###\n",
                                rrdset_name(st), rrddim_name(rd), (unsigned long) time_now, (unsigned long) time_retrieved);
                    time_errors++;
                    thread_info->errors++;
                }
            }
        }
        storage_engine_query_finalize(&seqh);
    } while(!thread_info->done);

    if(value_errors)
        fprintf(stderr, "%zu value errors encountered\n", value_errors);

    if(time_errors)
        fprintf(stderr, "%zu time errors encountered\n", time_errors);
}

void dbengine_stress_test(unsigned TEST_DURATION_SEC, unsigned DSET_CHARTS, unsigned QUERY_THREADS,
                          unsigned RAMP_UP_SECONDS, unsigned PAGE_CACHE_MB, unsigned DISK_SPACE_MB)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    const unsigned DSET_DIMS = 128;
    const uint64_t EXPECTED_COMPRESSION_RATIO = 20;
    const unsigned HISTORY_SECONDS = 3600 * 24 * 365 * 50; /* 50 year of history */
    RRDHOST *host = NULL;
    struct dbengine_chart_thread **chart_threads;
    struct dbengine_query_thread **query_threads;
    unsigned i, j;
    time_t time_start, test_duration;

    nd_log_limits_unlimited();

    if (!TEST_DURATION_SEC)
        TEST_DURATION_SEC = 10;
    if (!DSET_CHARTS)
        DSET_CHARTS = 1;
    if (!QUERY_THREADS)
        QUERY_THREADS = 1;
    if (PAGE_CACHE_MB < RRDENG_MIN_PAGE_CACHE_SIZE_MB)
        PAGE_CACHE_MB = RRDENG_MIN_PAGE_CACHE_SIZE_MB;

    default_rrd_memory_mode = RRD_DB_MODE_DBENGINE;
    default_rrdeng_page_cache_mb = PAGE_CACHE_MB;
    if (DISK_SPACE_MB) {
        fprintf(stderr, "By setting disk space limit data are allowed to be deleted. "
                        "Data validation is turned off for this run.\n");
        default_rrdeng_disk_quota_mb = DISK_SPACE_MB;
    } else {
        // Worst case for uncompressible data
        default_rrdeng_disk_quota_mb =
            (((uint64_t) DSET_DIMS * DSET_CHARTS) * sizeof(storage_number) * HISTORY_SECONDS) / (1024 * 1024);
        default_rrdeng_disk_quota_mb -= default_rrdeng_disk_quota_mb * EXPECTED_COMPRESSION_RATIO / 100;
    }

    fprintf(stderr, "Initializing localhost with hostname 'dbengine-stress-test'\n");

    (void)sql_init_meta_database(DB_CHECK_NONE, 1);
    host = dbengine_rrdhost_find_or_create("dbengine-stress-test");
    if (NULL == host)
        return;

    chart_threads = mallocz(sizeof(*chart_threads) * DSET_CHARTS);
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        chart_threads[i] = mallocz(sizeof(*chart_threads[i]) + sizeof(RRDDIM *) * DSET_DIMS);
    }
    query_threads = mallocz(sizeof(*query_threads) * QUERY_THREADS);
    for (i = 0 ; i < QUERY_THREADS ; ++i) {
        query_threads[i] = mallocz(sizeof(*query_threads[i]) + sizeof(struct dbengine_chart_thread *) * DSET_CHARTS);
    }
    fprintf(stderr, "\nRunning DB-engine stress test, %u seconds writers ramp-up time,\n"
                    "%u seconds of concurrent readers and writers, %u writer threads, %u reader threads,\n"
                    "%u MiB of page cache.\n",
            RAMP_UP_SECONDS, TEST_DURATION_SEC, DSET_CHARTS, QUERY_THREADS, PAGE_CACHE_MB);

    time_start = now_realtime_sec() + HISTORY_SECONDS; /* move history to the future */
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        chart_threads[i]->host = host;
        chart_threads[i]->chartname = "random";
        chart_threads[i]->dset_charts = DSET_CHARTS;
        chart_threads[i]->chart_i = i;
        chart_threads[i]->dset_dims = DSET_DIMS;
        chart_threads[i]->history_seconds = HISTORY_SECONDS;
        chart_threads[i]->time_present = time_start;
        chart_threads[i]->time_max = 0;
        chart_threads[i]->done = 0;
        chart_threads[i]->errors = chart_threads[i]->stored_metrics_nr = 0;
        completion_init(&chart_threads[i]->charts_initialized);
        fatal_assert(0 == uv_thread_create(&chart_threads[i]->thread, generate_dbengine_chart, chart_threads[i]));
    }
    /* barrier so that subsequent queries can access valid chart data */
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        completion_wait_for(&chart_threads[i]->charts_initialized);
        completion_destroy(&chart_threads[i]->charts_initialized);
    }
    sleep(RAMP_UP_SECONDS);
    /* at this point data have already began being written to the database */
    for (i = 0 ; i < QUERY_THREADS ; ++i) {
        query_threads[i]->host = host;
        query_threads[i]->chartname = "random";
        query_threads[i]->dset_charts = DSET_CHARTS;
        query_threads[i]->dset_dims = DSET_DIMS;
        query_threads[i]->history_seconds = HISTORY_SECONDS;
        query_threads[i]->time_present = time_start;
        query_threads[i]->done = 0;
        query_threads[i]->errors = query_threads[i]->queries_nr = query_threads[i]->queried_metrics_nr = 0;
        for (j = 0 ; j < DSET_CHARTS ; ++j) {
            query_threads[i]->chart_threads[j] = chart_threads[j];
        }
        query_threads[i]->delete_old_data = DISK_SPACE_MB ? 1 : 0;
        fatal_assert(0 == uv_thread_create(&query_threads[i]->thread, query_dbengine_chart, query_threads[i]));
    }
    sleep(TEST_DURATION_SEC);
    /* stop workload */
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        chart_threads[i]->done = 1;
    }
    for (i = 0 ; i < QUERY_THREADS ; ++i) {
        query_threads[i]->done = 1;
    }
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        assert(0 == uv_thread_join(&chart_threads[i]->thread));
    }
    for (i = 0 ; i < QUERY_THREADS ; ++i) {
        assert(0 == uv_thread_join(&query_threads[i]->thread));
    }
    test_duration = now_realtime_sec() - (time_start - HISTORY_SECONDS);
    if (!test_duration)
        test_duration = 1;
    fprintf(stderr, "\nDB-engine stress test finished in %lld seconds.\n", (long long)test_duration);
    unsigned long stored_metrics_nr = 0;
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        stored_metrics_nr += chart_threads[i]->stored_metrics_nr;
    }
    unsigned long queried_metrics_nr = 0;
    for (i = 0 ; i < QUERY_THREADS ; ++i) {
        queried_metrics_nr += query_threads[i]->queried_metrics_nr;
    }
    fprintf(stderr, "%u metrics were stored (dataset size of %lu MiB) in %u charts by 1 writer thread per chart.\n",
            DSET_CHARTS * DSET_DIMS, stored_metrics_nr * sizeof(storage_number) / (1024 * 1024), DSET_CHARTS);
    fprintf(stderr, "Metrics were being generated per 1 emulated second and time was accelerated.\n");
    fprintf(stderr, "%lu metric data points were queried by %u reader threads.\n", queried_metrics_nr, QUERY_THREADS);
    fprintf(stderr, "Query starting time is randomly chosen from the beginning of the time-series up to the time of\n"
                    "the latest data point, and ending time from 1 second up to 1 hour after the starting time.\n");
    fprintf(stderr, "Performance is %lld written data points/sec and %lld read data points/sec.\n",
            (long long)(stored_metrics_nr / test_duration), (long long)(queried_metrics_nr / test_duration));

    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        freez(chart_threads[i]);
    }
    freez(chart_threads);
    for (i = 0 ; i < QUERY_THREADS ; ++i) {
        freez(query_threads[i]);
    }
    freez(query_threads);
    rrd_wrlock();
    rrdeng_quiesce((struct rrdengine_instance *)host->db[0].si, false);
    rrdeng_exit((struct rrdengine_instance *)host->db[0].si);
    rrdeng_enq_cmd(NULL, RRDENG_OPCODE_SHUTDOWN_EVLOOP, NULL, NULL, STORAGE_PRIORITY_BEST_EFFORT, NULL, NULL);
    rrd_wrunlock();
}

#endif
