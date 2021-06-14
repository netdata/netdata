#include "aclk_stats.h"

netdata_mutex_t legacy_aclk_stats_mutex = NETDATA_MUTEX_INITIALIZER;

int legacy_query_thread_count;

// data ACLK stats need per query thread
struct legacy_aclk_qt_data {
    RRDDIM *dim;
} *legacy_aclk_qt_data = NULL;

// ACLK per query thread cpu stats
struct legacy_aclk_cpu_data {
    RRDDIM *user;
    RRDDIM *system;
    RRDSET *st;
} *legacy_aclk_cpu_data = NULL;

uint32_t *legacy_aclk_queries_per_thread = NULL;
uint32_t *legacy_aclk_queries_per_thread_sample = NULL;
struct rusage *rusage_per_thread;
uint8_t *getrusage_called_this_tick = NULL;

static struct legacy_aclk_metrics legacy_aclk_metrics = {
    .online = 0,
};

struct legacy_aclk_metrics_per_sample legacy_aclk_metrics_per_sample;

struct aclk_mat_metrics aclk_mat_metrics = {
#ifdef NETDATA_INTERNAL_CHECKS
    .latency = { .name = "aclk_latency_mqtt",
                 .prio = 200002,
                 .st = NULL,
                 .rd_avg = NULL,
                 .rd_max = NULL,
                 .rd_total = NULL,
                 .unit = "ms",
                 .title = "ACLK Message Publish Latency" },
#endif

    .cloud_q_db_query_time = { .name = "aclk_db_query_time",
                               .prio = 200006,
                               .st = NULL,
                               .rd_avg = NULL,
                               .rd_max = NULL,
                               .rd_total = NULL,
                               .unit = "us",
                               .title = "Time it took to process cloud requested DB queries" },

    .cloud_q_recvd_to_processed = { .name = "aclk_cloud_q_recvd_to_processed",
                                    .prio = 200007,
                                    .st = NULL,
                                    .rd_avg = NULL,
                                    .rd_max = NULL,
                                    .rd_total = NULL,
                                    .unit = "us",
                                    .title = "Time from receiving the Cloud Query until it was picked up "
                                             "by query thread (just before passing to the database)." }
};

void legacy_aclk_metric_mat_update(struct aclk_metric_mat_data *metric, usec_t measurement)
{
    if (aclk_stats_enabled) {
        LEGACY_ACLK_STATS_LOCK;
        if (metric->max < measurement)
            metric->max = measurement;

        metric->total += measurement;
        metric->count++;
        LEGACY_ACLK_STATS_UNLOCK;
    }
}

static void aclk_stats_collect(struct legacy_aclk_metrics_per_sample *per_sample, struct legacy_aclk_metrics *permanent)
{
    static RRDSET *st_aclkstats = NULL;
    static RRDDIM *rd_online_status = NULL;

    if (unlikely(!st_aclkstats)) {
        st_aclkstats = rrdset_create_localhost(
            "netdata", "aclk_status", NULL, "aclk", NULL, "ACLK/Cloud connection status",
            "connected", "netdata", "stats", 200000, localhost->rrd_update_every, RRDSET_TYPE_LINE);

        rd_online_status = rrddim_add(st_aclkstats, "online", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(st_aclkstats);

    rrddim_set_by_pointer(st_aclkstats, rd_online_status, per_sample->offline_during_sample ? 0 : permanent->online);

    rrdset_done(st_aclkstats);
}

static void aclk_stats_query_queue(struct legacy_aclk_metrics_per_sample *per_sample)
{
    static RRDSET *st_query_thread = NULL;
    static RRDDIM *rd_queued = NULL;
    static RRDDIM *rd_dispatched = NULL;

    if (unlikely(!st_query_thread)) {
        st_query_thread = rrdset_create_localhost(
            "netdata", "aclk_query_per_second", NULL, "aclk", NULL, "ACLK Queries per second", "queries/s",
            "netdata", "stats", 200001, localhost->rrd_update_every, RRDSET_TYPE_AREA);

        rd_queued = rrddim_add(st_query_thread, "added", NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        rd_dispatched = rrddim_add(st_query_thread, "dispatched", NULL, -1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(st_query_thread);

    rrddim_set_by_pointer(st_query_thread, rd_queued, per_sample->queries_queued);
    rrddim_set_by_pointer(st_query_thread, rd_dispatched, per_sample->queries_dispatched);

    rrdset_done(st_query_thread);
}

static void aclk_stats_write_q(struct legacy_aclk_metrics_per_sample *per_sample)
{
    static RRDSET *st = NULL;
    static RRDDIM *rd_wq_add = NULL;
    static RRDDIM *rd_wq_consumed = NULL;

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_write_q", NULL, "aclk", NULL, "Write Queue Mosq->Libwebsockets", "KiB/s",
            "netdata", "stats", 200003, localhost->rrd_update_every, RRDSET_TYPE_AREA);

        rd_wq_add = rrddim_add(st, "added", NULL, 1, 1024 * localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        rd_wq_consumed = rrddim_add(st, "consumed", NULL, 1, -1024 * localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(st);

    rrddim_set_by_pointer(st, rd_wq_add, per_sample->write_q_added);
    rrddim_set_by_pointer(st, rd_wq_consumed, per_sample->write_q_consumed);

    rrdset_done(st);
}

static void aclk_stats_read_q(struct legacy_aclk_metrics_per_sample *per_sample)
{
    static RRDSET *st = NULL;
    static RRDDIM *rd_rq_add = NULL;
    static RRDDIM *rd_rq_consumed = NULL;

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_read_q", NULL, "aclk", NULL, "Read Queue Libwebsockets->Mosq", "KiB/s",
            "netdata", "stats", 200004, localhost->rrd_update_every, RRDSET_TYPE_AREA);

        rd_rq_add = rrddim_add(st, "added", NULL, 1, 1024 * localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        rd_rq_consumed = rrddim_add(st, "consumed", NULL, 1, -1024 * localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(st);

    rrddim_set_by_pointer(st, rd_rq_add, per_sample->read_q_added);
    rrddim_set_by_pointer(st, rd_rq_consumed, per_sample->read_q_consumed);

    rrdset_done(st);
}

static void aclk_stats_cloud_req(struct legacy_aclk_metrics_per_sample *per_sample)
{
    static RRDSET *st = NULL;
    static RRDDIM *rd_rq_ok = NULL;
    static RRDDIM *rd_rq_err = NULL;

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_cloud_req", NULL, "aclk", NULL, "Requests received from cloud", "req/s",
            "netdata", "stats", 200005, localhost->rrd_update_every, RRDSET_TYPE_STACKED);

        rd_rq_ok = rrddim_add(st, "accepted", NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        rd_rq_err = rrddim_add(st, "rejected", NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(st);

    rrddim_set_by_pointer(st, rd_rq_ok, per_sample->cloud_req_ok);
    rrddim_set_by_pointer(st, rd_rq_err, per_sample->cloud_req_err);

    rrdset_done(st);
}

static void aclk_stats_cloud_req_version(struct legacy_aclk_metrics_per_sample *per_sample)
{
    static RRDSET *st = NULL;
    static RRDDIM *rd_rq_v1 = NULL;
    static RRDDIM *rd_rq_v2 = NULL;

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_cloud_req_version", NULL, "aclk", NULL, "Requests received from cloud by their version", "req/s",
            "netdata", "stats", 200006, localhost->rrd_update_every, RRDSET_TYPE_STACKED);

        rd_rq_v1 = rrddim_add(st, "v1",  NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        rd_rq_v2 = rrddim_add(st, "v2+", NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(st);

    rrddim_set_by_pointer(st, rd_rq_v1, per_sample->cloud_req_v1);
    rrddim_set_by_pointer(st, rd_rq_v2, per_sample->cloud_req_v2);

    rrdset_done(st);
}

static char *cloud_req_type_names[ACLK_STATS_CLOUD_REQ_TYPE_CNT] = {
    "other",   
    "info",
    "data",
    "alarms",
    "alarm_log",
    "chart",
    "charts"
    // if you change update:
    // #define ACLK_STATS_CLOUD_REQ_TYPE_CNT 7
};

int aclk_cloud_req_type_to_idx(const char *name)
{
    for (int i = 1; i < ACLK_STATS_CLOUD_REQ_TYPE_CNT; i++)
        if (!strcmp(cloud_req_type_names[i], name))
            return i;
    return 0;
}

static void aclk_stats_cloud_req_cmd(struct legacy_aclk_metrics_per_sample *per_sample)
{
    static RRDSET *st;
    static int initialized = 0;
    static RRDDIM *rd_rq_types[ACLK_STATS_CLOUD_REQ_TYPE_CNT];

    if (unlikely(!initialized)) {
        initialized = 1;
        st = rrdset_create_localhost(
            "netdata", "aclk_cloud_req_cmd", NULL, "aclk", NULL, "Requests received from cloud by their type (api endpoint queried)", "req/s",
            "netdata", "stats", 200007, localhost->rrd_update_every, RRDSET_TYPE_STACKED);

        for (int i = 0; i < ACLK_STATS_CLOUD_REQ_TYPE_CNT; i++)
            rd_rq_types[i] = rrddim_add(st, cloud_req_type_names[i], NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(st);

    for (int i = 0; i < ACLK_STATS_CLOUD_REQ_TYPE_CNT; i++)
        rrddim_set_by_pointer(st, rd_rq_types[i], per_sample->cloud_req_by_type[i]);

    rrdset_done(st);
}

#define MAX_DIM_NAME 16
static void aclk_stats_query_threads(uint32_t *queries_per_thread)
{
    static RRDSET *st = NULL;

    char dim_name[MAX_DIM_NAME];

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_query_threads", NULL, "aclk", NULL, "Queries Processed Per Thread", "req/s",
            "netdata", "stats", 200008, localhost->rrd_update_every, RRDSET_TYPE_STACKED);

        for (int i = 0; i < legacy_query_thread_count; i++) {
            if (snprintf(dim_name, MAX_DIM_NAME, "Query %d", i) < 0)
                error("snprintf encoding error");
            legacy_aclk_qt_data[i].dim = rrddim_add(st, dim_name, NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        }
    } else
        rrdset_next(st);

    for (int i = 0; i < legacy_query_thread_count; i++) {
        rrddim_set_by_pointer(st, legacy_aclk_qt_data[i].dim, queries_per_thread[i]);
    }

    rrdset_done(st);
}

static void aclk_stats_mat_metric_process(struct aclk_metric_mat *metric, struct aclk_metric_mat_data *data)
{
    if(unlikely(!metric->st)) {
        metric->st = rrdset_create_localhost(
            "netdata", metric->name, NULL, "aclk", NULL, metric->title, metric->unit, "netdata", "stats", metric->prio,
            localhost->rrd_update_every, RRDSET_TYPE_LINE);

        metric->rd_avg   = rrddim_add(metric->st, "avg",   NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        metric->rd_max   = rrddim_add(metric->st, "max",   NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        metric->rd_total = rrddim_add(metric->st, "total", NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(metric->st);

    if(data->count)
        rrddim_set_by_pointer(metric->st, metric->rd_avg, roundf((float)data->total / data->count));
    else
        rrddim_set_by_pointer(metric->st, metric->rd_avg, 0);
    rrddim_set_by_pointer(metric->st, metric->rd_max, data->max);
    rrddim_set_by_pointer(metric->st, metric->rd_total, data->total);

    rrdset_done(metric->st);
}

static void aclk_stats_cpu_threads(void)
{
    char id[100 + 1];
    char title[100 + 1];

    for (int i = 0; i < legacy_query_thread_count; i++) {
        if (unlikely(!legacy_aclk_cpu_data[i].st)) {

            snprintfz(id, 100, "aclk_thread%d_cpu", i);
            snprintfz(title, 100, "Cpu Usage For Thread No %d", i);

            legacy_aclk_cpu_data[i].st = rrdset_create_localhost(
                                         "netdata", id, NULL, "aclk", NULL, title, "milliseconds/s",
                                         "netdata", "stats", 200020 + i, localhost->rrd_update_every, RRDSET_TYPE_STACKED);

            legacy_aclk_cpu_data[i].user   = rrddim_add(legacy_aclk_cpu_data[i].st, "user",   NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
            legacy_aclk_cpu_data[i].system = rrddim_add(legacy_aclk_cpu_data[i].st, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);

        } else
            rrdset_next(legacy_aclk_cpu_data[i].st);
    }

    for (int i = 0; i < legacy_query_thread_count; i++) {
        rrddim_set_by_pointer(legacy_aclk_cpu_data[i].st, legacy_aclk_cpu_data[i].user, rusage_per_thread[i].ru_utime.tv_sec * 1000000ULL + rusage_per_thread[i].ru_utime.tv_usec);
        rrddim_set_by_pointer(legacy_aclk_cpu_data[i].st, legacy_aclk_cpu_data[i].system, rusage_per_thread[i].ru_stime.tv_sec * 1000000ULL + rusage_per_thread[i].ru_stime.tv_usec);
        rrdset_done(legacy_aclk_cpu_data[i].st);
    }
}

void legacy_aclk_stats_thread_cleanup()
{
    freez(legacy_aclk_qt_data);
    freez(legacy_aclk_queries_per_thread);
    freez(legacy_aclk_queries_per_thread_sample);
    freez(legacy_aclk_cpu_data);
    freez(rusage_per_thread);
}

void *legacy_aclk_stats_main_thread(void *ptr)
{
    struct aclk_stats_thread *args = ptr;

    legacy_query_thread_count = args->query_thread_count;
    legacy_aclk_qt_data = callocz(legacy_query_thread_count, sizeof(struct legacy_aclk_qt_data));
    legacy_aclk_cpu_data = callocz(legacy_query_thread_count, sizeof(struct legacy_aclk_cpu_data));
    legacy_aclk_queries_per_thread = callocz(legacy_query_thread_count, sizeof(uint32_t));
    legacy_aclk_queries_per_thread_sample = callocz(legacy_query_thread_count, sizeof(uint32_t));
    rusage_per_thread = callocz(legacy_query_thread_count, sizeof(struct rusage));
    getrusage_called_this_tick = callocz(legacy_query_thread_count, sizeof(uint8_t));

    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step_ut = localhost->rrd_update_every * USEC_PER_SEC;

    memset(&legacy_aclk_metrics_per_sample, 0, sizeof(struct legacy_aclk_metrics_per_sample));

    struct legacy_aclk_metrics_per_sample per_sample;
    struct legacy_aclk_metrics permanent;

    while (!netdata_exit) {
        netdata_thread_testcancel();
        // ------------------------------------------------------------------------
        // Wait for the next iteration point.

        heartbeat_next(&hb, step_ut);
        if (netdata_exit) break;

        LEGACY_ACLK_STATS_LOCK;
        // to not hold lock longer than necessary, especially not to hold it
        // during database rrd* operations
        memcpy(&per_sample, &legacy_aclk_metrics_per_sample, sizeof(struct legacy_aclk_metrics_per_sample));
        memcpy(&permanent, &legacy_aclk_metrics, sizeof(struct legacy_aclk_metrics));
        memset(&legacy_aclk_metrics_per_sample, 0, sizeof(struct legacy_aclk_metrics_per_sample));

        memcpy(legacy_aclk_queries_per_thread_sample, legacy_aclk_queries_per_thread, sizeof(uint32_t) * legacy_query_thread_count);
        memset(legacy_aclk_queries_per_thread, 0, sizeof(uint32_t) * legacy_query_thread_count);
        memset(getrusage_called_this_tick, 0, sizeof(uint8_t) * legacy_query_thread_count);
        LEGACY_ACLK_STATS_UNLOCK;

        aclk_stats_collect(&per_sample, &permanent);
        aclk_stats_query_queue(&per_sample);

        aclk_stats_write_q(&per_sample);
        aclk_stats_read_q(&per_sample);

        aclk_stats_cloud_req(&per_sample);
        aclk_stats_cloud_req_version(&per_sample);

        aclk_stats_cloud_req_cmd(&per_sample);

        aclk_stats_query_threads(legacy_aclk_queries_per_thread_sample);

        aclk_stats_cpu_threads();

#ifdef NETDATA_INTERNAL_CHECKS
        aclk_stats_mat_metric_process(&aclk_mat_metrics.latency, &per_sample.latency);
#endif
        aclk_stats_mat_metric_process(&aclk_mat_metrics.cloud_q_db_query_time, &per_sample.cloud_q_db_query_time);
        aclk_stats_mat_metric_process(&aclk_mat_metrics.cloud_q_recvd_to_processed, &per_sample.cloud_q_recvd_to_processed);
    }

    return 0;
}

void legacy_aclk_stats_upd_online(int online) {
    if(!aclk_stats_enabled)
        return;

    LEGACY_ACLK_STATS_LOCK;
    legacy_aclk_metrics.online = online;

    if(!online)
        legacy_aclk_metrics_per_sample.offline_during_sample = 1;
    LEGACY_ACLK_STATS_UNLOCK;
}
