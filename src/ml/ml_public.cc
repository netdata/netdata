// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml_private.h"

#include "database/sqlite/sqlite_db_migration.h"

#include <random>

#define ML_METADATA_VERSION 2

bool ml_capable()
{
    return true;
}

bool ml_enabled(RRDHOST *rh)
{
    if (!rh)
        return false;

    if (!Cfg.enable_anomaly_detection)
        return false;

    if (simple_pattern_matches(Cfg.sp_host_to_skip, rrdhost_hostname(rh)))
        return false;

    return true;
}

bool ml_streaming_enabled()
{
    return Cfg.stream_anomaly_detection_charts;
}

void ml_host_new(RRDHOST *rh)
{
    if (!ml_enabled(rh))
        return;

    ml_host_t *host = new ml_host_t();

    host->rh = rh;
    host->mls = ml_machine_learning_stats_t();
    host->host_anomaly_rate = 0.0;
    host->anomaly_rate_rs = NULL;

    static std::atomic<size_t> times_called(0);
    host->queue = Cfg.workers[times_called++ % Cfg.num_worker_threads].queue;

    netdata_mutex_init(&host->mutex);
    spinlock_init(&host->type_anomaly_rate_spinlock);

    host->ml_running = false;
    rh->ml_host = (rrd_ml_host_t *) host;
}

void ml_host_delete(RRDHOST *rh)
{
    ml_host_t *host = (ml_host_t *) rh->ml_host;
    if (!host)
        return;

    netdata_mutex_destroy(&host->mutex);

    delete host;
    rh->ml_host = NULL;
}

void ml_host_start(RRDHOST *rh) {
    ml_host_t *host = (ml_host_t *) rh->ml_host;
    if (!host)
        return;

    host->ml_running = true;
}

void ml_host_stop(RRDHOST *rh) {
    ml_host_t *host = (ml_host_t *) rh->ml_host;
    if (!host || !host->ml_running)
        return;

    netdata_mutex_lock(&host->mutex);

    // reset host stats
    host->mls = ml_machine_learning_stats_t();

    // reset charts/dims
    void *rsp = NULL;
    rrdset_foreach_read(rsp, host->rh) {
        RRDSET *rs = static_cast<RRDSET *>(rsp);

        ml_chart_t *chart = (ml_chart_t *) rs->ml_chart;
        if (!chart)
            continue;

        // reset chart
        chart->mls = ml_machine_learning_stats_t();

        void *rdp = NULL;
        rrddim_foreach_read(rdp, rs) {
            RRDDIM *rd = static_cast<RRDDIM *>(rdp);

            ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
            if (!dim)
                continue;

            spinlock_lock(&dim->slock);

            dim->mt = METRIC_TYPE_CONSTANT;
            dim->ts = TRAINING_STATUS_UNTRAINED;

            // TODO: Check if we can remove this field.
            dim->last_training_time = 0;

            dim->suppression_anomaly_counter = 0;
            dim->suppression_window_counter = 0;
            dim->cns.clear();

            ml_kmeans_init(&dim->kmeans);

            spinlock_unlock(&dim->slock);
        }
        rrddim_foreach_done(rdp);
    }
    rrdset_foreach_done(rsp);

    netdata_mutex_unlock(&host->mutex);

    host->ml_running = false;
}

void ml_host_get_info(RRDHOST *rh, BUFFER *wb)
{
    ml_host_t *host = (ml_host_t *) rh->ml_host;
    if (!host) {
        buffer_json_member_add_boolean(wb, "enabled", false);
        return;
    }

    buffer_json_member_add_uint64(wb, "version", 1);

    buffer_json_member_add_boolean(wb, "enabled", Cfg.enable_anomaly_detection);

    buffer_json_member_add_uint64(wb, "min-train-samples", Cfg.min_train_samples);
    buffer_json_member_add_uint64(wb, "max-train-samples", Cfg.max_train_samples);
    buffer_json_member_add_uint64(wb, "train-every", Cfg.train_every);

    buffer_json_member_add_uint64(wb, "diff-n", Cfg.diff_n);
    buffer_json_member_add_uint64(wb, "smooth-n", Cfg.smooth_n);
    buffer_json_member_add_uint64(wb, "lag-n", Cfg.lag_n);

    buffer_json_member_add_double(wb, "random-sampling-ratio", Cfg.random_sampling_ratio);
    buffer_json_member_add_uint64(wb, "max-kmeans-iters", Cfg.random_sampling_ratio);

    buffer_json_member_add_double(wb, "dimension-anomaly-score-threshold", Cfg.dimension_anomaly_score_threshold);

    buffer_json_member_add_string(wb, "anomaly-detection-grouping-method", time_grouping_id2txt(Cfg.anomaly_detection_grouping_method));

    buffer_json_member_add_int64(wb, "anomaly-detection-query-duration", Cfg.anomaly_detection_query_duration);

    buffer_json_member_add_string(wb, "hosts-to-skip", Cfg.hosts_to_skip.c_str());
    buffer_json_member_add_string(wb, "charts-to-skip", Cfg.charts_to_skip.c_str());
}

void ml_host_get_detection_info(RRDHOST *rh, BUFFER *wb)
{
    ml_host_t *host = (ml_host_t *) rh->ml_host;
    if (!host)
        return;

    netdata_mutex_lock(&host->mutex);

    buffer_json_member_add_uint64(wb, "version", 2);
    buffer_json_member_add_uint64(wb, "ml-running", host->ml_running);
    buffer_json_member_add_uint64(wb, "anomalous-dimensions", host->mls.num_anomalous_dimensions);
    buffer_json_member_add_uint64(wb, "normal-dimensions", host->mls.num_normal_dimensions);
    buffer_json_member_add_uint64(wb, "total-dimensions", host->mls.num_anomalous_dimensions +
                                                          host->mls.num_normal_dimensions);
    buffer_json_member_add_uint64(wb, "trained-dimensions", host->mls.num_training_status_trained +
                                                            host->mls.num_training_status_pending_with_model);
    netdata_mutex_unlock(&host->mutex);
}

bool ml_host_get_host_status(RRDHOST *rh, struct ml_metrics_statistics *mlm) {
    ml_host_t *host = (ml_host_t *) rh->ml_host;
    if (!host) {
        memset(mlm, 0, sizeof(*mlm));
        return false;
    }

    netdata_mutex_lock(&host->mutex);

    mlm->anomalous = host->mls.num_anomalous_dimensions;
    mlm->normal = host->mls.num_normal_dimensions;
    mlm->trained = host->mls.num_training_status_trained + host->mls.num_training_status_pending_with_model;
    mlm->pending = host->mls.num_training_status_untrained + host->mls.num_training_status_pending_without_model;
    mlm->silenced = host->mls.num_training_status_silenced;

    netdata_mutex_unlock(&host->mutex);

    return true;
}

bool ml_host_running(RRDHOST *rh) {
    ml_host_t *host = (ml_host_t *) rh->ml_host;
    if(!host)
        return false;

    return true;
}

void ml_host_get_models(RRDHOST *rh, BUFFER *wb)
{
    UNUSED(rh);
    UNUSED(wb);

    // TODO: To be implemented
    netdata_log_error("Fetching KMeans models is not supported yet");
}

void ml_chart_new(RRDSET *rs)
{
    ml_host_t *host = (ml_host_t *) rs->rrdhost->ml_host;
    if (!host)
        return;

    ml_chart_t *chart = new ml_chart_t();

    chart->rs = rs;
    chart->mls = ml_machine_learning_stats_t();

    rs->ml_chart = (rrd_ml_chart_t *) chart;
}

void ml_chart_delete(RRDSET *rs)
{
    ml_host_t *host = (ml_host_t *) rs->rrdhost->ml_host;
    if (!host)
        return;

    ml_chart_t *chart = (ml_chart_t *) rs->ml_chart;

    delete chart;
    rs->ml_chart = NULL;
}

ALWAYS_INLINE_ONLY bool ml_chart_update_begin(RRDSET *rs)
{
    ml_chart_t *chart = (ml_chart_t *)rs->ml_chart;
    if (!chart)
        return false;

    chart->mls = {};
    return true;
}

void ml_chart_update_end(RRDSET *rs)
{
    ml_chart_t *chart = (ml_chart_t *) rs->ml_chart;
    if (!chart)
        return;
}

void ml_dimension_new(RRDDIM *rd)
{
    ml_chart_t *chart = (ml_chart_t *) rd->rrdset->ml_chart;
    if (!chart)
        return;

    ml_dimension_t *dim = new ml_dimension_t();

    dim->rd = rd;

    dim->mt = METRIC_TYPE_CONSTANT;
    dim->ts = TRAINING_STATUS_UNTRAINED;
    dim->last_training_time = 0;
    dim->suppression_anomaly_counter = 0;
    dim->suppression_window_counter = 0;

    ml_kmeans_init(&dim->kmeans);

    if (simple_pattern_matches(Cfg.sp_charts_to_skip, rrdset_name(rd->rrdset)))
        dim->mls = MACHINE_LEARNING_STATUS_DISABLED_DUE_TO_EXCLUDED_CHART;
    else
        dim->mls = MACHINE_LEARNING_STATUS_ENABLED;

    spinlock_init(&dim->slock);

    dim->km_contexts.reserve(Cfg.num_models_to_use);

    rd->ml_dimension = (rrd_ml_dimension_t *) dim;

    metaqueue_ml_load_models(rd);

    // add to worker queue
    {
        RRDHOST *rh = rd->rrdset->rrdhost;
        ml_host_t *host = (ml_host_t *) rh->ml_host;

        ml_queue_item_t item;
        item.type = ML_QUEUE_ITEM_TYPE_CREATE_NEW_MODEL;

        ml_request_create_new_model_t req;
        req.DLI = DimensionLookupInfo(
            &rh->machine_guid[0],
            rd->rrdset->id,
            rd->id
        );
        item.create_new_model = req;

        ml_queue_push(host->queue, item);
    }
}

void ml_dimension_delete(RRDDIM *rd)
{
    ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
    if (!dim)
        return;

    delete dim;
    rd->ml_dimension = NULL;
}

ALWAYS_INLINE_ONLY void ml_dimension_received_anomaly(RRDDIM *rd, bool is_anomalous) {
    ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
    if (!dim)
        return;

    ml_host_t *host = (ml_host_t *) rd->rrdset->rrdhost->ml_host;
    if (!host->ml_running)
        return;

    ml_chart_t *chart = (ml_chart_t *) rd->rrdset->ml_chart;

    ml_chart_update_dimension(chart, dim, is_anomalous);
}

bool ml_dimension_is_anomalous(RRDDIM *rd, time_t curr_time, double value, bool exists)
{
    UNUSED(curr_time);

    ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
    if (!dim)
        return false;

    ml_host_t *host = (ml_host_t *) rd->rrdset->rrdhost->ml_host;
    if (!host->ml_running)
        return false;

    ml_chart_t *chart = (ml_chart_t *) rd->rrdset->ml_chart;

    bool is_anomalous = ml_dimension_predict(dim, value, exists);
    ml_chart_update_dimension(chart, dim, is_anomalous);

    return is_anomalous;
}

void ml_init()
{
    // Read config values
    ml_config_load(&Cfg);

    if (!Cfg.enable_anomaly_detection)
        return;

    // Generate random numbers to efficiently sample the features we need
    // for KMeans clustering.
    std::random_device RD;
    std::mt19937 Gen(RD());

    Cfg.random_nums.reserve(Cfg.max_train_samples);
    for (size_t Idx = 0; Idx != Cfg.max_train_samples; Idx++)
        Cfg.random_nums.push_back(Gen());

    // init training thread-specific data
    Cfg.workers.resize(Cfg.num_worker_threads);
    for (size_t idx = 0; idx != Cfg.num_worker_threads; idx++) {
        ml_worker_t *worker = &Cfg.workers[idx];

        size_t max_elements_needed_for_training = (size_t) Cfg.max_train_samples * (size_t) (Cfg.lag_n + 1);
        worker->training_cns = new calculated_number_t[max_elements_needed_for_training]();
        worker->scratch_training_cns = new calculated_number_t[max_elements_needed_for_training]();

        worker->id = idx;
        worker->queue = ml_queue_init();
        worker->pending_model_info.reserve(Cfg.flush_models_batch_size);
        netdata_mutex_init(&worker->nd_mutex);
    }

    // open sqlite db
    char path[FILENAME_MAX];
    snprintfz(path, FILENAME_MAX - 1, "%s/%s", netdata_configured_cache_dir, "ml.db");
    int rc = sqlite3_open(path, &ml_db);
    if (rc != SQLITE_OK) {
        error_report("Failed to initialize database at %s, due to \"%s\"", path, sqlite3_errstr(rc));
        sqlite3_close(ml_db);
        ml_db = NULL;
    }

    // create table
    if (ml_db) {
        int target_version = perform_ml_database_migration(ml_db, ML_METADATA_VERSION);
        if (configure_sqlite_database(ml_db, target_version, "ml_config")) {
            error_report("Failed to setup ML database");
            sqlite3_close(ml_db);
            ml_db = NULL;
        }
        else {
            char *err = NULL;
            int rc = sqlite3_exec(ml_db, db_models_create_table, NULL, NULL, &err);
            if (rc != SQLITE_OK) {
                error_report("Failed to create models table (%s, %s)", sqlite3_errstr(rc), err ? err : "");
                sqlite3_close(ml_db);
                sqlite3_free(err);
                ml_db = NULL;
            }
        }
    }
}

uint64_t sqlite_get_ml_space(void)
{
    return sqlite_get_db_space(ml_db);
}

void ml_fini() {
    if (!Cfg.enable_anomaly_detection || !ml_db)
        return;

    sql_close_database(ml_db, "ML");
    ml_db = NULL;
}

void ml_start_threads() {
    if (!Cfg.enable_anomaly_detection)
        return;

    // start detection & training threads
    Cfg.detection_stop = false;
    Cfg.training_stop = false;

    char tag[NETDATA_THREAD_TAG_MAX + 1];

    snprintfz(tag, NETDATA_THREAD_TAG_MAX, "%s", "PREDICT");
    Cfg.detection_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_JOINABLE,
                                            ml_detect_main, NULL);

    for (size_t idx = 0; idx != Cfg.num_worker_threads; idx++) {
        ml_worker_t *worker = &Cfg.workers[idx];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "TRAIN[%zu]", worker->id);
        worker->nd_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_JOINABLE,
                                                      ml_train_main, worker);
    }
}

void ml_stop_threads()
{
    if (!Cfg.enable_anomaly_detection)
        return;

    Cfg.detection_stop = true;
    Cfg.training_stop = true;

    if (!Cfg.detection_thread)
        return;

    nd_thread_join(Cfg.detection_thread);
    Cfg.detection_thread = 0;

    // signal the worker queue of each thread
    for (size_t idx = 0; idx != Cfg.num_worker_threads; idx++) {
        ml_worker_t *worker = &Cfg.workers[idx];
        ml_queue_signal(worker->queue);
    }

    // join worker threads
    for (size_t idx = 0; idx != Cfg.num_worker_threads; idx++) {
        ml_worker_t *worker = &Cfg.workers[idx];

        nd_thread_join(worker->nd_thread);
    }

    // clear worker thread data
    for (size_t idx = 0; idx != Cfg.num_worker_threads; idx++) {
        ml_worker_t *worker = &Cfg.workers[idx];

        delete[] worker->training_cns;
        delete[] worker->scratch_training_cns;
        ml_queue_destroy(worker->queue);
        netdata_mutex_destroy(&worker->nd_mutex);
    }
}

bool ml_model_received_from_child(RRDHOST *host, const char *json)
{
    UNUSED(host);

    bool ok = ml_dimension_deserialize_kmeans(json);
    if (!ok) {
        global_statistics_ml_models_deserialization_failures();
    }

    return ok;
}
