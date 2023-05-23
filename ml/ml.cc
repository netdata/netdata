// SPDX-License-Identifier: GPL-3.0-or-later

#include <dlib/clustering.h>

#include "ml-private.h"

#include <random>

#include "ad_charts.h"
#include "database/sqlite/sqlite3.h"

#define WORKER_TRAIN_QUEUE_POP         0
#define WORKER_TRAIN_ACQUIRE_DIMENSION 1
#define WORKER_TRAIN_QUERY             2
#define WORKER_TRAIN_KMEANS            3
#define WORKER_TRAIN_UPDATE_MODELS     4
#define WORKER_TRAIN_RELEASE_DIMENSION 5
#define WORKER_TRAIN_UPDATE_HOST       6
#define WORKER_TRAIN_FLUSH_MODELS      7

static sqlite3 *db = NULL;
static netdata_mutex_t db_mutex = NETDATA_MUTEX_INITIALIZER;

/*
 * Functions to convert enums to strings
*/

__attribute__((unused)) static const char *
ml_machine_learning_status_to_string(enum ml_machine_learning_status mls)
{
    switch (mls) {
        case MACHINE_LEARNING_STATUS_ENABLED:
            return "enabled";
        case MACHINE_LEARNING_STATUS_DISABLED_DUE_TO_EXCLUDED_CHART:
            return "disabled-sp";
        default:
            return "unknown";
    }
}

__attribute__((unused)) static const char *
ml_metric_type_to_string(enum ml_metric_type mt)
{
    switch (mt) {
        case METRIC_TYPE_CONSTANT:
            return "constant";
        case METRIC_TYPE_VARIABLE:
            return "variable";
        default:
            return "unknown";
    }
}

__attribute__((unused)) static const char *
ml_training_status_to_string(enum ml_training_status ts)
{
    switch (ts) {
        case TRAINING_STATUS_PENDING_WITH_MODEL:
            return "pending-with-model";
        case TRAINING_STATUS_PENDING_WITHOUT_MODEL:
            return "pending-without-model";
        case TRAINING_STATUS_TRAINED:
            return "trained";
        case TRAINING_STATUS_UNTRAINED:
            return "untrained";
        case TRAINING_STATUS_SILENCED:
            return "silenced";
        default:
            return "unknown";
    }
}

__attribute__((unused)) static const char *
ml_training_result_to_string(enum ml_training_result tr)
{
    switch (tr) {
        case TRAINING_RESULT_OK:
            return "ok";
        case TRAINING_RESULT_INVALID_QUERY_TIME_RANGE:
            return "invalid-query";
        case TRAINING_RESULT_NOT_ENOUGH_COLLECTED_VALUES:
            return "missing-values";
        case TRAINING_RESULT_NULL_ACQUIRED_DIMENSION:
            return "null-acquired-dim";
        case TRAINING_RESULT_CHART_UNDER_REPLICATION:
            return "chart-under-replication";
        default:
            return "unknown";
    }
}

/*
 * Features
*/

// subtract elements that are `diff_n` positions apart
static void
ml_features_diff(ml_features_t *features)
{
    if (features->diff_n == 0)
        return;

    for (size_t idx = 0; idx != (features->src_n - features->diff_n); idx++) {
        size_t high = (features->src_n - 1) - idx;
        size_t low = high - features->diff_n;

        features->dst[low] = features->src[high] - features->src[low];
    }

    size_t n = features->src_n - features->diff_n;
    memcpy(features->src, features->dst, n * sizeof(calculated_number_t));

    for (size_t idx = features->src_n - features->diff_n; idx != features->src_n; idx++)
        features->src[idx] = 0.0;
}

// a function that computes the window average of an array inplace
static void
ml_features_smooth(ml_features_t *features)
{
    calculated_number_t sum = 0.0;

    size_t idx = 0;
    for (; idx != features->smooth_n - 1; idx++)
        sum += features->src[idx];

    for (; idx != (features->src_n - features->diff_n); idx++) {
        sum += features->src[idx];
        calculated_number_t prev_cn = features->src[idx - (features->smooth_n - 1)];
        features->src[idx - (features->smooth_n - 1)] = sum / features->smooth_n;
        sum -= prev_cn;
    }

    for (idx = 0; idx != features->smooth_n; idx++)
        features->src[(features->src_n - 1) - idx] = 0.0;
}

// create lag'd vectors out of the preprocessed buffer
static void
ml_features_lag(ml_features_t *features)
{
    size_t n = features->src_n - features->diff_n - features->smooth_n + 1 - features->lag_n;
    features->preprocessed_features.resize(n);

    unsigned target_num_samples = Cfg.max_train_samples * Cfg.random_sampling_ratio;
    double sampling_ratio = std::min(static_cast<double>(target_num_samples) / n, 1.0);

    uint32_t max_mt = std::numeric_limits<uint32_t>::max();
    uint32_t cutoff = static_cast<double>(max_mt) * sampling_ratio;

    size_t sample_idx = 0;

    for (size_t idx = 0; idx != n; idx++) {
        DSample &DS = features->preprocessed_features[sample_idx++];
        DS.set_size(features->lag_n);

        if (Cfg.random_nums[idx] > cutoff) {
            sample_idx--;
            continue;
        }

        for (size_t feature_idx = 0; feature_idx != features->lag_n + 1; feature_idx++)
            DS(feature_idx) = features->src[idx + feature_idx];
    }

    features->preprocessed_features.resize(sample_idx);
}

static void
ml_features_preprocess(ml_features_t *features)
{
    ml_features_diff(features);
    ml_features_smooth(features);
    ml_features_lag(features);
}

/*
 * KMeans
*/

static void
ml_kmeans_init(ml_kmeans_t *kmeans)
{
    kmeans->cluster_centers.reserve(2);
    kmeans->min_dist = std::numeric_limits<calculated_number_t>::max();
    kmeans->max_dist = std::numeric_limits<calculated_number_t>::min();
}

static void
ml_kmeans_train(ml_kmeans_t *kmeans, const ml_features_t *features, time_t after, time_t before)
{
    kmeans->after = (uint32_t) after;
    kmeans->before = (uint32_t) before;

    kmeans->min_dist = std::numeric_limits<calculated_number_t>::max();
    kmeans->max_dist  = std::numeric_limits<calculated_number_t>::min();

    kmeans->cluster_centers.clear();

    dlib::pick_initial_centers(2, kmeans->cluster_centers, features->preprocessed_features);
    dlib::find_clusters_using_kmeans(features->preprocessed_features, kmeans->cluster_centers, Cfg.max_kmeans_iters);

    for (const auto &preprocessed_feature : features->preprocessed_features) {
        calculated_number_t mean_dist = 0.0;

        for (const auto &cluster_center : kmeans->cluster_centers) {
            mean_dist += dlib::length(cluster_center - preprocessed_feature);
        }

        mean_dist /= kmeans->cluster_centers.size();

        if (mean_dist < kmeans->min_dist)
            kmeans->min_dist = mean_dist;

        if (mean_dist > kmeans->max_dist)
            kmeans->max_dist = mean_dist;
    }
}

static calculated_number_t
ml_kmeans_anomaly_score(const ml_kmeans_t *kmeans, const DSample &DS)
{
    calculated_number_t mean_dist = 0.0;
    for (const auto &CC: kmeans->cluster_centers)
        mean_dist += dlib::length(CC - DS);

    mean_dist /= kmeans->cluster_centers.size();

    if (kmeans->max_dist == kmeans->min_dist)
        return 0.0;

    calculated_number_t anomaly_score = 100.0 * std::abs((mean_dist - kmeans->min_dist) / (kmeans->max_dist - kmeans->min_dist));
    return (anomaly_score > 100.0) ? 100.0 : anomaly_score;
}

/*
 * Queue
*/

static ml_queue_t *
ml_queue_init()
{
    ml_queue_t *q = new ml_queue_t();

    netdata_mutex_init(&q->mutex);
    pthread_cond_init(&q->cond_var, NULL);
    q->exit = false;
    return q;
}

static void
ml_queue_destroy(ml_queue_t *q)
{
    netdata_mutex_destroy(&q->mutex);
    pthread_cond_destroy(&q->cond_var);
    delete q;
}

static void
ml_queue_push(ml_queue_t *q, const ml_training_request_t req)
{
    netdata_mutex_lock(&q->mutex);
    q->internal.push(req);
    pthread_cond_signal(&q->cond_var);
    netdata_mutex_unlock(&q->mutex);
}

static ml_training_request_t
ml_queue_pop(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);

    ml_training_request_t req = {
        {'\0'}, // machine_guid
        NULL, // chart id
        NULL, // dimension id
        0, // current time
        0, // first entry
        0  // last entry
    };

    while (q->internal.empty()) {
        pthread_cond_wait(&q->cond_var, &q->mutex);

        if (q->exit) {
            netdata_mutex_unlock(&q->mutex);

            // We return a dummy request because the queue has been signaled
            return req;
        }
    }

    req = q->internal.front();
    q->internal.pop();

    netdata_mutex_unlock(&q->mutex);
    return req;
}

static size_t
ml_queue_size(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);
    size_t size = q->internal.size();
    netdata_mutex_unlock(&q->mutex);
    return size;
}

static void
ml_queue_signal(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);
    q->exit = true;
    pthread_cond_signal(&q->cond_var);
    netdata_mutex_unlock(&q->mutex);
}

/*
 * Dimension
*/

static std::pair<calculated_number_t *, ml_training_response_t>
ml_dimension_calculated_numbers(ml_training_thread_t *training_thread, ml_dimension_t *dim, const ml_training_request_t &training_request)
{
    ml_training_response_t training_response = {};

    training_response.request_time = training_request.request_time;
    training_response.first_entry_on_request = training_request.first_entry_on_request;
    training_response.last_entry_on_request = training_request.last_entry_on_request;

    training_response.first_entry_on_response = rrddim_first_entry_s_of_tier(dim->rd, 0);
    training_response.last_entry_on_response = rrddim_last_entry_s_of_tier(dim->rd, 0);

    size_t min_n = Cfg.min_train_samples;
    size_t max_n = Cfg.max_train_samples;

    // Figure out what our time window should be.
    training_response.query_before_t = training_response.last_entry_on_response;
    training_response.query_after_t = std::max(
        training_response.query_before_t - static_cast<time_t>((max_n - 1) * dim->rd->update_every),
        training_response.first_entry_on_response
    );

    if (training_response.query_after_t >= training_response.query_before_t) {
        training_response.result = TRAINING_RESULT_INVALID_QUERY_TIME_RANGE;
        return { NULL, training_response };
    }

    if (rrdset_is_replicating(dim->rd->rrdset)) {
        training_response.result = TRAINING_RESULT_CHART_UNDER_REPLICATION;
        return { NULL, training_response };
    }

    /*
     * Execute the query
    */
    struct storage_engine_query_handle handle;

    storage_engine_query_init(dim->rd->tiers[0].backend, dim->rd->tiers[0].db_metric_handle, &handle,
              training_response.query_after_t, training_response.query_before_t,
              STORAGE_PRIORITY_BEST_EFFORT);

    size_t idx = 0;
    memset(training_thread->training_cns, 0, sizeof(calculated_number_t) * max_n * (Cfg.lag_n + 1));
    calculated_number_t last_value = std::numeric_limits<calculated_number_t>::quiet_NaN();

    while (!storage_engine_query_is_finished(&handle)) {
        if (idx == max_n)
            break;

        STORAGE_POINT sp = storage_engine_query_next_metric(&handle);

        time_t timestamp = sp.end_time_s;
        calculated_number_t value = sp.sum / sp.count;

        if (netdata_double_isnumber(value)) {
            if (!training_response.db_after_t)
                training_response.db_after_t = timestamp;
            training_response.db_before_t = timestamp;

            training_thread->training_cns[idx] = value;
            last_value = training_thread->training_cns[idx];
            training_response.collected_values++;
        } else
            training_thread->training_cns[idx] = last_value;

        idx++;
    }
    storage_engine_query_finalize(&handle);

    global_statistics_ml_query_completed(/* points_read */ idx);

    training_response.total_values = idx;
    if (training_response.collected_values < min_n) {
        training_response.result = TRAINING_RESULT_NOT_ENOUGH_COLLECTED_VALUES;
        return { NULL, training_response };
    }

    // Find first non-NaN value.
    for (idx = 0; std::isnan(training_thread->training_cns[idx]); idx++, training_response.total_values--) { }

    // Overwrite NaN values.
    if (idx != 0)
        memmove(training_thread->training_cns, &training_thread->training_cns[idx], sizeof(calculated_number_t) * training_response.total_values);

    training_response.result = TRAINING_RESULT_OK;
    return { training_thread->training_cns, training_response };
}

const char *db_models_create_table =
    "CREATE TABLE IF NOT EXISTS models("
    "    dim_id BLOB, after INT, before INT,"
    "    min_dist REAL, max_dist REAL,"
    "    c00 REAL, c01 REAL, c02 REAL, c03 REAL, c04 REAL, c05 REAL,"
    "    c10 REAL, c11 REAL, c12 REAL, c13 REAL, c14 REAL, c15 REAL,"
    "    PRIMARY KEY(dim_id, after)"
    ");";

const char *db_models_add_model =
    "INSERT OR REPLACE INTO models("
    "    dim_id, after, before,"
    "    min_dist, max_dist,"
    "    c00, c01, c02, c03, c04, c05,"
    "    c10, c11, c12, c13, c14, c15)"
    "VALUES("
    "    @dim_id, @after, @before,"
    "    @min_dist, @max_dist,"
    "    @c00, @c01, @c02, @c03, @c04, @c05,"
    "    @c10, @c11, @c12, @c13, @c14, @c15);";

const char *db_models_load =
    "SELECT * FROM models "
    "WHERE dim_id = @dim_id AND after >= @after ORDER BY before ASC;";

const char *db_models_delete =
    "DELETE FROM models "
    "WHERE dim_id = @dim_id AND before < @before;";

static int
ml_dimension_add_model(const uuid_t *metric_uuid, const ml_kmeans_t *km)
{
    static __thread sqlite3_stmt *res = NULL;
    int param = 0;
    int rc = 0;

    if (unlikely(!db)) {
        error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db, db_models_add_model, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store model, rc = %d", rc);
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, ++param, metric_uuid, sizeof(*metric_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, (int) km->after);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, (int) km->before);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_double(res, ++param, km->min_dist);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_double(res, ++param, km->max_dist);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (km->cluster_centers.size() != 2)
        fatal("Expected 2 cluster centers, got %zu", km->cluster_centers.size());

    for (const DSample &ds : km->cluster_centers) {
        if (ds.size() != 6)
            fatal("Expected dsample with 6 dimensions, got %ld", ds.size());

        for (long idx = 0; idx != ds.size(); idx++) {
            calculated_number_t cn = ds(idx);
            int rc = sqlite3_bind_double(res, ++param, cn);
            if (unlikely(rc != SQLITE_OK))
                goto bind_fail;
        }
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to store model, rc = %d", rc);
        return rc;
    }

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to reset statement when storing model, rc = %d", rc);
        return rc;
    }

    return 0;

bind_fail:
    error_report("Failed to bind parameter %d to store model, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to store model, rc = %d", rc);
    return rc;
}

static int
ml_dimension_delete_models(const uuid_t *metric_uuid, time_t before)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc = 0;
    int param = 0;

    if (unlikely(!db)) {
        error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db, db_models_delete, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to delete models, rc = %d", rc);
            return rc;
        }
    }

    rc = sqlite3_bind_blob(res, ++param, metric_uuid, sizeof(*metric_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, (int) before);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to delete models, rc = %d", rc);
        return rc;
    }

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to reset statement when deleting models, rc = %d", rc);
        return rc;
    }

    return 0;

bind_fail:
    error_report("Failed to bind parameter %d to delete models, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to delete models, rc = %d", rc);
    return rc;
}

int ml_dimension_load_models(RRDDIM *rd) {
    ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
    if (!dim)
        return 0;

    netdata_mutex_lock(&dim->mutex);
    bool is_empty = dim->km_contexts.empty();
    netdata_mutex_unlock(&dim->mutex);

    if (!is_empty)
        return 0;

    std::vector<ml_kmeans_t> V;

    static __thread sqlite3_stmt *res = NULL;
    int rc = 0;
    int param = 0;

    if (unlikely(!db)) {
        error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db, db_models_load, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to load models, rc = %d", rc);
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, ++param, &dim->rd->metric_uuid, sizeof(dim->rd->metric_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, now_realtime_usec() - (Cfg.num_models_to_use * Cfg.max_train_samples));
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    netdata_mutex_lock(&dim->mutex);

    dim->km_contexts.reserve(Cfg.num_models_to_use);
    while ((rc = sqlite3_step_monitored(res)) == SQLITE_ROW) {
        ml_kmeans_t km;

        km.after = sqlite3_column_int(res, 2);
        km.before = sqlite3_column_int(res, 3);

        km.min_dist = sqlite3_column_int(res, 4);
        km.max_dist = sqlite3_column_int(res, 5);

        km.cluster_centers.resize(2);

        km.cluster_centers[0].set_size(Cfg.lag_n + 1);
        km.cluster_centers[0](0) = sqlite3_column_double(res, 6);
        km.cluster_centers[0](1) = sqlite3_column_double(res, 7);
        km.cluster_centers[0](2) = sqlite3_column_double(res, 8);
        km.cluster_centers[0](3) = sqlite3_column_double(res, 9);
        km.cluster_centers[0](4) = sqlite3_column_double(res, 10);
        km.cluster_centers[0](5) = sqlite3_column_double(res, 11);

        km.cluster_centers[1].set_size(Cfg.lag_n + 1);
        km.cluster_centers[1](0) = sqlite3_column_double(res, 12);
        km.cluster_centers[1](1) = sqlite3_column_double(res, 13);
        km.cluster_centers[1](2) = sqlite3_column_double(res, 14);
        km.cluster_centers[1](3) = sqlite3_column_double(res, 15);
        km.cluster_centers[1](4) = sqlite3_column_double(res, 16);
        km.cluster_centers[1](5) = sqlite3_column_double(res, 17);

        dim->km_contexts.push_back(km);
    }

    if (!dim->km_contexts.empty()) {
        dim->ts = TRAINING_STATUS_TRAINED;
    }

    netdata_mutex_unlock(&dim->mutex);

    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to load models, rc = %d", rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when loading models, rc = %d", rc);

    return 0;

bind_fail:
    error_report("Failed to bind parameter %d to load models, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to load models, rc = %d", rc);
    return 1;
}

static enum ml_training_result
ml_dimension_train_model(ml_training_thread_t *training_thread, ml_dimension_t *dim, const ml_training_request_t &training_request)
{
    worker_is_busy(WORKER_TRAIN_QUERY);
    auto P = ml_dimension_calculated_numbers(training_thread, dim, training_request);
    ml_training_response_t training_response = P.second;

    if (training_response.result != TRAINING_RESULT_OK) {
        netdata_mutex_lock(&dim->mutex);

        dim->mt = METRIC_TYPE_CONSTANT;

        switch (dim->ts) {
            case TRAINING_STATUS_PENDING_WITH_MODEL:
                dim->ts = TRAINING_STATUS_TRAINED;
                break;
            case TRAINING_STATUS_PENDING_WITHOUT_MODEL:
                dim->ts = TRAINING_STATUS_UNTRAINED;
                break;
            default:
                break;
        }

        dim->suppression_anomaly_counter = 0;
        dim->suppression_window_counter = 0;
        dim->tr = training_response;

        dim->last_training_time = training_response.last_entry_on_response;
        enum ml_training_result result = training_response.result;
        netdata_mutex_unlock(&dim->mutex);

        return result;
    }

    // compute kmeans
    worker_is_busy(WORKER_TRAIN_KMEANS);
    {
        memcpy(training_thread->scratch_training_cns, training_thread->training_cns,
               training_response.total_values * sizeof(calculated_number_t));

        ml_features_t features = {
            Cfg.diff_n, Cfg.smooth_n, Cfg.lag_n,
            training_thread->scratch_training_cns, training_response.total_values,
            training_thread->training_cns, training_response.total_values,
            training_thread->training_samples
        };
        ml_features_preprocess(&features);

        ml_kmeans_init(&dim->kmeans);
        ml_kmeans_train(&dim->kmeans, &features, training_response.query_after_t, training_response.query_before_t);
    }

    // update models
    worker_is_busy(WORKER_TRAIN_UPDATE_MODELS);
    {
        netdata_mutex_lock(&dim->mutex);

        if (dim->km_contexts.size() < Cfg.num_models_to_use) {
            dim->km_contexts.push_back(std::move(dim->kmeans));
        } else {
            bool can_drop_middle_km = false;

            if (Cfg.num_models_to_use > 2) {
                const ml_kmeans_t *old_km = &dim->km_contexts[dim->km_contexts.size() - 1];
                const ml_kmeans_t *middle_km = &dim->km_contexts[dim->km_contexts.size() - 2];
                const ml_kmeans_t *new_km = &dim->kmeans;

                can_drop_middle_km = (middle_km->after < old_km->before) &&
                                     (middle_km->before > new_km->after);
            }

            if (can_drop_middle_km) {
                dim->km_contexts.back() = dim->kmeans;
            } else {
                std::rotate(std::begin(dim->km_contexts), std::begin(dim->km_contexts) + 1, std::end(dim->km_contexts));
                dim->km_contexts[dim->km_contexts.size() - 1] = std::move(dim->kmeans);
            }
        }

        dim->mt = METRIC_TYPE_CONSTANT;
        dim->ts = TRAINING_STATUS_TRAINED;

        dim->suppression_anomaly_counter = 0;
        dim->suppression_window_counter = 0;

        dim->tr = training_response;
        dim->last_training_time = rrddim_last_entry_s(dim->rd);

        // Add the newly generated model to the list of pending models to flush
        ml_model_info_t model_info;
        uuid_copy(model_info.metric_uuid, dim->rd->metric_uuid);
        model_info.kmeans = dim->km_contexts.back();
        training_thread->pending_model_info.push_back(model_info);

        netdata_mutex_unlock(&dim->mutex);
    }

    return training_response.result;
}

static void
ml_dimension_schedule_for_training(ml_dimension_t *dim, time_t curr_time)
{
    switch (dim->mt) {
    case METRIC_TYPE_CONSTANT:
        return;
    default:
        break;
    }

    bool schedule_for_training = false;

    switch (dim->ts) {
    case TRAINING_STATUS_PENDING_WITH_MODEL:
    case TRAINING_STATUS_PENDING_WITHOUT_MODEL:
        schedule_for_training = false;
        break;
    case TRAINING_STATUS_UNTRAINED:
        schedule_for_training = true;
        dim->ts = TRAINING_STATUS_PENDING_WITHOUT_MODEL;
        break;
    case TRAINING_STATUS_SILENCED:
    case TRAINING_STATUS_TRAINED:
        if ((dim->last_training_time + (Cfg.train_every * dim->rd->update_every)) < curr_time) {
            schedule_for_training = true;
            dim->ts = TRAINING_STATUS_PENDING_WITH_MODEL;
        }
        break;
    }

    if (schedule_for_training) {
        ml_training_request_t req;

        memcpy(req.machine_guid, dim->rd->rrdset->rrdhost->machine_guid, GUID_LEN + 1);
        req.chart_id = string_dup(dim->rd->rrdset->id);
        req.dimension_id = string_dup(dim->rd->id);
        req.request_time = curr_time;
        req.first_entry_on_request = rrddim_first_entry_s(dim->rd);
        req.last_entry_on_request = rrddim_last_entry_s(dim->rd);

        ml_host_t *host = (ml_host_t *) dim->rd->rrdset->rrdhost->ml_host;
        ml_queue_push(host->training_queue, req);
    }
}

static bool
ml_dimension_predict(ml_dimension_t *dim, time_t curr_time, calculated_number_t value, bool exists)
{
    // Nothing to do if ML is disabled for this dimension
    if (dim->mls != MACHINE_LEARNING_STATUS_ENABLED)
        return false;

    // Don't treat values that don't exist as anomalous
    if (!exists) {
        dim->cns.clear();
        return false;
    }

    // Save the value and return if we don't have enough values for a sample
    unsigned n = Cfg.diff_n + Cfg.smooth_n + Cfg.lag_n;
    if (dim->cns.size() < n) {
        dim->cns.push_back(value);
        return false;
    }

    // Push the value and check if it's different from the last one
    bool same_value = true;
    std::rotate(std::begin(dim->cns), std::begin(dim->cns) + 1, std::end(dim->cns));
    if (dim->cns[n - 1] != value)
        same_value = false;
    dim->cns[n - 1] = value;

    // Create the sample
    assert((n * (Cfg.lag_n + 1) <= 128) &&
           "Static buffers too small to perform prediction. "
           "This should not be possible with the default clamping of feature extraction options");
    calculated_number_t src_cns[128];
    calculated_number_t dst_cns[128];

    memset(src_cns, 0, n * (Cfg.lag_n + 1) * sizeof(calculated_number_t));
    memcpy(src_cns, dim->cns.data(), n * sizeof(calculated_number_t));
    memcpy(dst_cns, dim->cns.data(), n * sizeof(calculated_number_t));

    ml_features_t features = {
        Cfg.diff_n, Cfg.smooth_n, Cfg.lag_n,
        dst_cns, n, src_cns, n,
        dim->feature
    };
    ml_features_preprocess(&features);

    /*
     * Lock to predict and possibly schedule the dimension for training
    */
    if (netdata_mutex_trylock(&dim->mutex) != 0)
        return false;

    // Mark the metric time as variable if we received different values
    if (!same_value)
        dim->mt = METRIC_TYPE_VARIABLE;

    // Decide if the dimension needs to be scheduled for training
    ml_dimension_schedule_for_training(dim, curr_time);

    // Nothing to do if we don't have a model
    switch (dim->ts) {
        case TRAINING_STATUS_UNTRAINED:
        case TRAINING_STATUS_PENDING_WITHOUT_MODEL: {
        case TRAINING_STATUS_SILENCED:
            netdata_mutex_unlock(&dim->mutex);
            return false;
        }
        default:
            break;
    }

    dim->suppression_window_counter++;

    /*
     * Use the KMeans models to check if the value is anomalous
    */

    size_t sum = 0;
    size_t models_consulted = 0;

    for (const auto &km_ctx : dim->km_contexts) {
        models_consulted++;

        calculated_number_t anomaly_score = ml_kmeans_anomaly_score(&km_ctx, features.preprocessed_features[0]);
        if (anomaly_score == std::numeric_limits<calculated_number_t>::quiet_NaN())
            continue;

        if (anomaly_score < (100 * Cfg.dimension_anomaly_score_threshold)) {
            global_statistics_ml_models_consulted(models_consulted);
            netdata_mutex_unlock(&dim->mutex);
            return false;
        }

        sum += 1;
    }

    dim->suppression_anomaly_counter += sum ? 1 : 0;

    if ((dim->suppression_anomaly_counter >= Cfg.suppression_threshold) &&
        (dim->suppression_window_counter >= Cfg.suppression_window)) {
        dim->ts = TRAINING_STATUS_SILENCED;
    }

    netdata_mutex_unlock(&dim->mutex);

    global_statistics_ml_models_consulted(models_consulted);
    return sum;
}

/*
 * Chart
*/

static bool
ml_chart_is_available_for_ml(ml_chart_t *chart)
{
    return rrdset_is_available_for_exporting_and_alarms(chart->rs);
}

void
ml_chart_update_dimension(ml_chart_t *chart, ml_dimension_t *dim, bool is_anomalous)
{
    switch (dim->mls) {
        case MACHINE_LEARNING_STATUS_DISABLED_DUE_TO_EXCLUDED_CHART:
            chart->mls.num_machine_learning_status_disabled_sp++;
            return;
        case MACHINE_LEARNING_STATUS_ENABLED: {
            chart->mls.num_machine_learning_status_enabled++;

            switch (dim->mt) {
                case METRIC_TYPE_CONSTANT:
                    chart->mls.num_metric_type_constant++;
                    chart->mls.num_training_status_trained++;
                    chart->mls.num_normal_dimensions++;
                    return;
                case METRIC_TYPE_VARIABLE:
                    chart->mls.num_metric_type_variable++;
                    break;
            }

            switch (dim->ts) {
                case TRAINING_STATUS_UNTRAINED:
                    chart->mls.num_training_status_untrained++;
                    return;
                case TRAINING_STATUS_PENDING_WITHOUT_MODEL:
                    chart->mls.num_training_status_pending_without_model++;
                    return;
                case TRAINING_STATUS_TRAINED:
                    chart->mls.num_training_status_trained++;

                    chart->mls.num_anomalous_dimensions += is_anomalous;
                    chart->mls.num_normal_dimensions += !is_anomalous;
                    return;
                case TRAINING_STATUS_PENDING_WITH_MODEL:
                    chart->mls.num_training_status_pending_with_model++;

                    chart->mls.num_anomalous_dimensions += is_anomalous;
                    chart->mls.num_normal_dimensions += !is_anomalous;
                    return;
                case TRAINING_STATUS_SILENCED:
                    chart->mls.num_training_status_silenced++;
                    chart->mls.num_training_status_trained++;

                    chart->mls.num_anomalous_dimensions += is_anomalous;
                    chart->mls.num_normal_dimensions += !is_anomalous;
                    return;
            }

            return;
        }
    }
}

/*
 * Host detection & training functions
*/

#define WORKER_JOB_DETECTION_COLLECT_STATS 0
#define WORKER_JOB_DETECTION_DIM_CHART 1
#define WORKER_JOB_DETECTION_HOST_CHART 2
#define WORKER_JOB_DETECTION_STATS 3

static void
ml_host_detect_once(ml_host_t *host)
{
    worker_is_busy(WORKER_JOB_DETECTION_COLLECT_STATS);

    host->mls = {};
    ml_machine_learning_stats_t mls_copy = {};

    {
        netdata_mutex_lock(&host->mutex);

        /*
         * prediction/detection stats
        */
        void *rsp = NULL;
        rrdset_foreach_read(rsp, host->rh) {
            RRDSET *rs = static_cast<RRDSET *>(rsp);

            ml_chart_t *chart = (ml_chart_t *) rs->ml_chart;
            if (!chart)
                continue;

            if (!ml_chart_is_available_for_ml(chart))
                continue;

            ml_machine_learning_stats_t chart_mls = chart->mls;

            host->mls.num_machine_learning_status_enabled += chart_mls.num_machine_learning_status_enabled;
            host->mls.num_machine_learning_status_disabled_sp += chart_mls.num_machine_learning_status_disabled_sp;

            host->mls.num_metric_type_constant += chart_mls.num_metric_type_constant;
            host->mls.num_metric_type_variable += chart_mls.num_metric_type_variable;

            host->mls.num_training_status_untrained += chart_mls.num_training_status_untrained;
            host->mls.num_training_status_pending_without_model += chart_mls.num_training_status_pending_without_model;
            host->mls.num_training_status_trained += chart_mls.num_training_status_trained;
            host->mls.num_training_status_pending_with_model += chart_mls.num_training_status_pending_with_model;
            host->mls.num_training_status_silenced += chart_mls.num_training_status_silenced;

            host->mls.num_anomalous_dimensions += chart_mls.num_anomalous_dimensions;
            host->mls.num_normal_dimensions += chart_mls.num_normal_dimensions;
        }
        rrdset_foreach_done(rsp);

        host->host_anomaly_rate = 0.0;
        size_t NumActiveDimensions = host->mls.num_anomalous_dimensions + host->mls.num_normal_dimensions;
        if (NumActiveDimensions)
              host->host_anomaly_rate = static_cast<double>(host->mls.num_anomalous_dimensions) / NumActiveDimensions;

        mls_copy = host->mls;

        netdata_mutex_unlock(&host->mutex);
    }

    worker_is_busy(WORKER_JOB_DETECTION_DIM_CHART);
    ml_update_dimensions_chart(host, mls_copy);

    worker_is_busy(WORKER_JOB_DETECTION_HOST_CHART);
    ml_update_host_and_detection_rate_charts(host, host->host_anomaly_rate * 10000.0);
}

typedef struct {
    RRDHOST_ACQUIRED *acq_rh;
    RRDSET_ACQUIRED *acq_rs;
    RRDDIM_ACQUIRED *acq_rd;
    ml_dimension_t *dim;
} ml_acquired_dimension_t;

static ml_acquired_dimension_t
ml_acquired_dimension_get(char *machine_guid, STRING *chart_id, STRING *dimension_id)
{
    RRDHOST_ACQUIRED *acq_rh = NULL;
    RRDSET_ACQUIRED *acq_rs = NULL;
    RRDDIM_ACQUIRED *acq_rd = NULL;
    ml_dimension_t *dim = NULL;

    rrd_rdlock();

    acq_rh = rrdhost_find_and_acquire(machine_guid);
    if (acq_rh) {
        RRDHOST *rh = rrdhost_acquired_to_rrdhost(acq_rh);
        if (rh && !rrdhost_flag_check(rh, RRDHOST_FLAG_ORPHAN | RRDHOST_FLAG_ARCHIVED)) {
            acq_rs = rrdset_find_and_acquire(rh, string2str(chart_id));
            if (acq_rs) {
                RRDSET *rs = rrdset_acquired_to_rrdset(acq_rs);
                if (rs && !rrdset_flag_check(rs, RRDSET_FLAG_ARCHIVED | RRDSET_FLAG_OBSOLETE)) {
                    acq_rd = rrddim_find_and_acquire(rs, string2str(dimension_id));
                    if (acq_rd) {
                        RRDDIM *rd = rrddim_acquired_to_rrddim(acq_rd);
                        if (rd)
                            dim = (ml_dimension_t *) rd->ml_dimension;
                    }
                }
            }
        }
    }

    rrd_unlock();

    ml_acquired_dimension_t acq_dim = {
        acq_rh, acq_rs, acq_rd, dim
    };

    return acq_dim;
}

static void
ml_acquired_dimension_release(ml_acquired_dimension_t acq_dim)
{
    if (acq_dim.acq_rd)
        rrddim_acquired_release(acq_dim.acq_rd);

    if (acq_dim.acq_rs)
        rrdset_acquired_release(acq_dim.acq_rs);

    if (acq_dim.acq_rh)
        rrdhost_acquired_release(acq_dim.acq_rh);
}

static enum ml_training_result
ml_acquired_dimension_train(ml_training_thread_t *training_thread, ml_acquired_dimension_t acq_dim, const ml_training_request_t &tr)
{
    if (!acq_dim.dim)
        return TRAINING_RESULT_NULL_ACQUIRED_DIMENSION;

    return ml_dimension_train_model(training_thread, acq_dim.dim, tr);
}

static void *
ml_detect_main(void *arg)
{
    UNUSED(arg);

    worker_register("MLDETECT");
    worker_register_job_name(WORKER_JOB_DETECTION_COLLECT_STATS, "collect stats");
    worker_register_job_name(WORKER_JOB_DETECTION_DIM_CHART, "dim chart");
    worker_register_job_name(WORKER_JOB_DETECTION_HOST_CHART, "host chart");
    worker_register_job_name(WORKER_JOB_DETECTION_STATS, "training stats");

    heartbeat_t hb;
    heartbeat_init(&hb);

    while (!Cfg.detection_stop) {
        worker_is_idle();
        heartbeat_next(&hb, USEC_PER_SEC);

        RRDHOST *rh;
        rrd_rdlock();
        rrdhost_foreach_read(rh) {
            if (!rh->ml_host)
                continue;

            ml_host_detect_once((ml_host_t *) rh->ml_host);
        }
        rrd_unlock();

        if (Cfg.enable_statistics_charts) {
            // collect and update training thread stats
            for (size_t idx = 0; idx != Cfg.num_training_threads; idx++) {
                ml_training_thread_t *training_thread = &Cfg.training_threads[idx];

                netdata_mutex_lock(&training_thread->nd_mutex);
                ml_training_stats_t training_stats = training_thread->training_stats;
                training_thread->training_stats = {};
                netdata_mutex_unlock(&training_thread->nd_mutex);

                // calc the avg values
                if (training_stats.num_popped_items) {
                    training_stats.queue_size /= training_stats.num_popped_items;
                    training_stats.allotted_ut /= training_stats.num_popped_items;
                    training_stats.consumed_ut /= training_stats.num_popped_items;
                    training_stats.remaining_ut /= training_stats.num_popped_items;
                } else {
                    training_stats.queue_size = ml_queue_size(training_thread->training_queue);
                    training_stats.consumed_ut = 0;
                    training_stats.remaining_ut = training_stats.allotted_ut;

                    training_stats.training_result_ok = 0;
                    training_stats.training_result_invalid_query_time_range = 0;
                    training_stats.training_result_not_enough_collected_values = 0;
                    training_stats.training_result_null_acquired_dimension = 0;
                    training_stats.training_result_chart_under_replication = 0;
                }

                ml_update_training_statistics_chart(training_thread, training_stats);
            }
        }
    }

    return NULL;
}

/*
 * Public API
*/

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
    //host->ts = ml_training_stats_t();

    static std::atomic<size_t> times_called(0);
    host->training_queue = Cfg.training_threads[times_called++ % Cfg.num_training_threads].training_queue;

    host->host_anomaly_rate = 0.0;

    netdata_mutex_init(&host->mutex);

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

    buffer_json_member_add_string(wb, "anomaly-detection-grouping-method",
                                  time_grouping_method2string(Cfg.anomaly_detection_grouping_method));

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

    buffer_json_member_add_uint64(wb, "version", 1);
    buffer_json_member_add_uint64(wb, "anomalous-dimensions", host->mls.num_anomalous_dimensions);
    buffer_json_member_add_uint64(wb, "normal-dimensions", host->mls.num_normal_dimensions);
    buffer_json_member_add_uint64(wb, "total-dimensions", host->mls.num_anomalous_dimensions +
                                                          host->mls.num_normal_dimensions);
    buffer_json_member_add_uint64(wb, "trained-dimensions", host->mls.num_training_status_trained +
                                                            host->mls.num_training_status_pending_with_model);
    netdata_mutex_unlock(&host->mutex);
}

void ml_host_get_models(RRDHOST *rh, BUFFER *wb)
{
    UNUSED(rh);
    UNUSED(wb);

    // TODO: To be implemented
    error("Fetching KMeans models is not supported yet");
}

void ml_chart_new(RRDSET *rs)
{
    ml_host_t *host = (ml_host_t *) rs->rrdhost->ml_host;
    if (!host)
        return;

    ml_chart_t *chart = new ml_chart_t();

    chart->rs = rs;
    chart->mls = ml_machine_learning_stats_t();

    netdata_mutex_init(&chart->mutex);

    rs->ml_chart = (rrd_ml_chart_t *) chart;
}

void ml_chart_delete(RRDSET *rs)
{
    ml_host_t *host = (ml_host_t *) rs->rrdhost->ml_host;
    if (!host)
        return;

    ml_chart_t *chart = (ml_chart_t *) rs->ml_chart;

    netdata_mutex_destroy(&chart->mutex);

    delete chart;
    rs->ml_chart = NULL;
}

bool ml_chart_update_begin(RRDSET *rs)
{
    ml_chart_t *chart = (ml_chart_t *) rs->ml_chart;
    if (!chart)
        return false;

    netdata_mutex_lock(&chart->mutex);
    chart->mls = {};
    return true;
}

void ml_chart_update_end(RRDSET *rs)
{
    ml_chart_t *chart = (ml_chart_t *) rs->ml_chart;
    if (!chart)
        return;

    netdata_mutex_unlock(&chart->mutex);
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

    ml_kmeans_init(&dim->kmeans);

    if (simple_pattern_matches(Cfg.sp_charts_to_skip, rrdset_name(rd->rrdset)))
        dim->mls = MACHINE_LEARNING_STATUS_DISABLED_DUE_TO_EXCLUDED_CHART;
    else
        dim->mls = MACHINE_LEARNING_STATUS_ENABLED;

    netdata_mutex_init(&dim->mutex);

    dim->km_contexts.reserve(Cfg.num_models_to_use);

    rd->ml_dimension = (rrd_ml_dimension_t *) dim;

    metaqueue_ml_load_models(rd);
}

void ml_dimension_delete(RRDDIM *rd)
{
    ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
    if (!dim)
        return;

    netdata_mutex_destroy(&dim->mutex);

    delete dim;
    rd->ml_dimension = NULL;
}

bool ml_dimension_is_anomalous(RRDDIM *rd, time_t curr_time, double value, bool exists)
{
    ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
    if (!dim)
        return false;

    ml_chart_t *chart = (ml_chart_t *) rd->rrdset->ml_chart;

    bool is_anomalous = ml_dimension_predict(dim, curr_time, value, exists);
    ml_chart_update_dimension(chart, dim, is_anomalous);

    return is_anomalous;
}

static void ml_flush_pending_models(ml_training_thread_t *training_thread) {
    int rc = db_execute(db, "BEGIN TRANSACTION;");
    int op_no = 1;

    if (!rc) {
        op_no++;

        for (const auto &pending_model: training_thread->pending_model_info) {
            if (!rc)
                rc = ml_dimension_add_model(&pending_model.metric_uuid, &pending_model.kmeans);

            if (!rc)
                rc = ml_dimension_delete_models(&pending_model.metric_uuid, pending_model.kmeans.before - (Cfg.num_models_to_use * Cfg.train_every));
        }
    }

    if (!rc) {
        op_no++;
        rc = db_execute(db, "COMMIT TRANSACTION;");
    }

    // try to rollback transaction if we got any failures
    if (rc) {
        error("Trying to rollback ML transaction because it failed with rc=%d, op_no=%d", rc, op_no);
        op_no++;
        rc = db_execute(db, "ROLLBACK;");
        if (rc)
            error("ML transaction rollback failed with rc=%d", rc);
    }

    training_thread->pending_model_info.clear();
}

static void *ml_train_main(void *arg) {
    ml_training_thread_t *training_thread = (ml_training_thread_t *) arg;

    char worker_name[1024];
    snprintfz(worker_name, 1024, "training_thread_%zu", training_thread->id);
    worker_register("MLTRAIN");

    worker_register_job_name(WORKER_TRAIN_QUEUE_POP, "pop queue");
    worker_register_job_name(WORKER_TRAIN_ACQUIRE_DIMENSION, "acquire");
    worker_register_job_name(WORKER_TRAIN_QUERY, "query");
    worker_register_job_name(WORKER_TRAIN_KMEANS, "kmeans");
    worker_register_job_name(WORKER_TRAIN_UPDATE_MODELS, "update models");
    worker_register_job_name(WORKER_TRAIN_RELEASE_DIMENSION, "release");
    worker_register_job_name(WORKER_TRAIN_UPDATE_HOST, "update host");
    worker_register_job_name(WORKER_TRAIN_FLUSH_MODELS, "flush models");

    while (!Cfg.training_stop) {
        worker_is_busy(WORKER_TRAIN_QUEUE_POP);

        ml_training_request_t training_req = ml_queue_pop(training_thread->training_queue);

        // we know this thread has been cancelled, when the queue starts
        // returning "null" requests without blocking on queue's pop().
        if (training_req.chart_id == NULL)
            break;

        size_t queue_size = ml_queue_size(training_thread->training_queue) + 1;

        usec_t allotted_ut = (Cfg.train_every * USEC_PER_SEC) / queue_size;
        if (allotted_ut > USEC_PER_SEC)
            allotted_ut = USEC_PER_SEC;

        usec_t start_ut = now_monotonic_usec();

        enum ml_training_result training_res;
        {
            worker_is_busy(WORKER_TRAIN_ACQUIRE_DIMENSION);
            ml_acquired_dimension_t acq_dim = ml_acquired_dimension_get(
                training_req.machine_guid,
                training_req.chart_id,
                training_req.dimension_id);

            training_res = ml_acquired_dimension_train(training_thread, acq_dim, training_req);

            string_freez(training_req.chart_id);
            string_freez(training_req.dimension_id);

            worker_is_busy(WORKER_TRAIN_RELEASE_DIMENSION);
            ml_acquired_dimension_release(acq_dim);
        }

        usec_t consumed_ut = now_monotonic_usec() - start_ut;

        usec_t remaining_ut = 0;
        if (consumed_ut < allotted_ut)
            remaining_ut = allotted_ut - consumed_ut;

        if (Cfg.enable_statistics_charts) {
            worker_is_busy(WORKER_TRAIN_UPDATE_HOST);

            netdata_mutex_lock(&training_thread->nd_mutex);

            training_thread->training_stats.queue_size += queue_size;
            training_thread->training_stats.num_popped_items += 1;

            training_thread->training_stats.allotted_ut += allotted_ut;
            training_thread->training_stats.consumed_ut += consumed_ut;
            training_thread->training_stats.remaining_ut += remaining_ut;

            switch (training_res) {
                case TRAINING_RESULT_OK:
                    training_thread->training_stats.training_result_ok += 1;
                    break;
                case TRAINING_RESULT_INVALID_QUERY_TIME_RANGE:
                    training_thread->training_stats.training_result_invalid_query_time_range += 1;
                    break;
                case TRAINING_RESULT_NOT_ENOUGH_COLLECTED_VALUES:
                    training_thread->training_stats.training_result_not_enough_collected_values += 1;
                    break;
                case TRAINING_RESULT_NULL_ACQUIRED_DIMENSION:
                    training_thread->training_stats.training_result_null_acquired_dimension += 1;
                    break;
                case TRAINING_RESULT_CHART_UNDER_REPLICATION:
                    training_thread->training_stats.training_result_chart_under_replication += 1;
                    break;
            }

            netdata_mutex_unlock(&training_thread->nd_mutex);
        }

        if (training_thread->pending_model_info.size() >= Cfg.flush_models_batch_size) {
            worker_is_busy(WORKER_TRAIN_FLUSH_MODELS);
            netdata_mutex_lock(&db_mutex);
            ml_flush_pending_models(training_thread);
            netdata_mutex_unlock(&db_mutex);
            continue;
        }

        worker_is_idle();
        std::this_thread::sleep_for(std::chrono::microseconds{remaining_ut});
    }

    return NULL;
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
    Cfg.training_threads.resize(Cfg.num_training_threads);
    for (size_t idx = 0; idx != Cfg.num_training_threads; idx++) {
        ml_training_thread_t *training_thread = &Cfg.training_threads[idx];

        size_t max_elements_needed_for_training = (size_t) Cfg.max_train_samples * (size_t) (Cfg.lag_n + 1);
        training_thread->training_cns = new calculated_number_t[max_elements_needed_for_training]();
        training_thread->scratch_training_cns = new calculated_number_t[max_elements_needed_for_training]();

        training_thread->id = idx;
        training_thread->training_queue = ml_queue_init();
        training_thread->pending_model_info.reserve(Cfg.flush_models_batch_size);
        netdata_mutex_init(&training_thread->nd_mutex);
    }

    // open sqlite db
    char path[FILENAME_MAX];
    snprintfz(path, FILENAME_MAX - 1, "%s/%s", netdata_configured_cache_dir, "ml.db");
    int rc = sqlite3_open(path, &db);
    if (rc != SQLITE_OK) {
        error_report("Failed to initialize database at %s, due to \"%s\"", path, sqlite3_errstr(rc));
        sqlite3_close(db);
        db = NULL;
    }

    if (db) {
        char *err = NULL;
        int rc = sqlite3_exec(db, db_models_create_table, NULL, NULL, &err);
        if (rc != SQLITE_OK) {
            error_report("Failed to create models table (%s, %s)", sqlite3_errstr(rc), err ? err : "");
            sqlite3_close(db);
            sqlite3_free(err);
            db = NULL;
        }
    }
}

void ml_fini() {
    if (!Cfg.enable_anomaly_detection)
        return;

    int rc = sqlite3_close_v2(db);
    if (unlikely(rc != SQLITE_OK))
        error_report("Error %d while closing the SQLite database, %s", rc, sqlite3_errstr(rc));
}

void ml_start_threads() {
    if (!Cfg.enable_anomaly_detection)
        return;

    // start detection & training threads
    Cfg.detection_stop = false;
    Cfg.training_stop = false;

    char tag[NETDATA_THREAD_TAG_MAX + 1];

    snprintfz(tag, NETDATA_THREAD_TAG_MAX, "%s", "PREDICT");
    netdata_thread_create(&Cfg.detection_thread, tag, NETDATA_THREAD_OPTION_JOINABLE, ml_detect_main, NULL);

    for (size_t idx = 0; idx != Cfg.num_training_threads; idx++) {
        ml_training_thread_t *training_thread = &Cfg.training_threads[idx];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "TRAIN[%zu]", training_thread->id);
        netdata_thread_create(&training_thread->nd_thread, tag, NETDATA_THREAD_OPTION_JOINABLE, ml_train_main, training_thread);
    }
}

void ml_stop_threads()
{
    if (!Cfg.enable_anomaly_detection)
        return;

    Cfg.detection_stop = true;
    Cfg.training_stop = true;

    netdata_thread_cancel(Cfg.detection_thread);
    netdata_thread_join(Cfg.detection_thread, NULL);

    // signal the training queue of each thread
    for (size_t idx = 0; idx != Cfg.num_training_threads; idx++) {
        ml_training_thread_t *training_thread = &Cfg.training_threads[idx];

        ml_queue_signal(training_thread->training_queue);
    }

    // cancel training threads
    for (size_t idx = 0; idx != Cfg.num_training_threads; idx++) {
        ml_training_thread_t *training_thread = &Cfg.training_threads[idx];

        netdata_thread_cancel(training_thread->nd_thread);
    }

    // join training threads
    for (size_t idx = 0; idx != Cfg.num_training_threads; idx++) {
        ml_training_thread_t *training_thread = &Cfg.training_threads[idx];

        netdata_thread_join(training_thread->nd_thread, NULL);
    }

    // clear training thread data
    for (size_t idx = 0; idx != Cfg.num_training_threads; idx++) {
        ml_training_thread_t *training_thread = &Cfg.training_threads[idx];

        delete[] training_thread->training_cns;
        delete[] training_thread->scratch_training_cns;
        ml_queue_destroy(training_thread->training_queue);
        netdata_mutex_destroy(&training_thread->nd_mutex);
    }
}
