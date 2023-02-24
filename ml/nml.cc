// SPDX-License-Identifier: GPL-3.0-or-later

#include <dlib/clustering.h>

#include "nml.h"

#include <random>

#include "ad_charts.h"

typedef struct {
    calculated_number_t *training_cns;
    calculated_number_t *scratch_training_cns;

    std::vector<DSample> training_samples;
} nml_tls_data_t;

static thread_local nml_tls_data_t tls_data;

/*
 * Functions to convert enums to strings
*/

static const char *nml_machine_learning_status_to_string(enum nml_machine_learning_status mls) {
    switch (mls) {
        case MACHINE_LEARNING_STATUS_ENABLED:
            return "enabled";
        case MACHINE_LEARNING_STATUS_DISABLED_DUE_TO_EXCLUDED_CHART:
            return "disabled-sp";
        default:
            return "unknown";
    }
}

static const char *nml_metric_type_to_string(enum nml_metric_type mt) {
    switch (mt) {
        case METRIC_TYPE_CONSTANT:
            return "constant";
        case METRIC_TYPE_VARIABLE:
            return "variable";
        default:
            return "unknown";
    }
}

static const char *nml_training_status_to_string(enum nml_training_status ts) {
    switch (ts) {
        case TRAINING_STATUS_PENDING_WITH_MODEL:
            return "pending-with-model";
        case TRAINING_STATUS_PENDING_WITHOUT_MODEL:
            return "pending-without-model";
        case TRAINING_STATUS_TRAINED:
            return "trained";
        case TRAINING_STATUS_UNTRAINED:
            return "untrained";
        default:
            return "unknown";
    }
}

static const char *nml_training_result_to_string(enum nml_training_result tr) {
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
static void nml_features_diff(nml_features_t *features) {
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
static void nml_features_smooth(nml_features_t *features) {
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
static void nml_features_lag(nml_features_t *features) {
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

static void nml_features_preprocess(nml_features_t *features) {
    nml_features_diff(features);
    nml_features_smooth(features);
    nml_features_lag(features);
}

/*
 * KMeans
*/

static void nml_kmeans_init(nml_kmeans_t *kmeans, size_t num_clusters, size_t max_iterations) {
    kmeans->num_clusters = num_clusters;
    kmeans->max_iterations = max_iterations;

    kmeans->cluster_centers.reserve(kmeans->num_clusters);
    kmeans->min_dist = std::numeric_limits<calculated_number_t>::max();
    kmeans->max_dist = std::numeric_limits<calculated_number_t>::min();
}

static void nml_kmeans_train(nml_kmeans_t *kmeans, const nml_features_t *features) {
    kmeans->min_dist = std::numeric_limits<calculated_number_t>::max();
    kmeans->max_dist  = std::numeric_limits<calculated_number_t>::min();

    kmeans->cluster_centers.clear();

    dlib::pick_initial_centers(kmeans->num_clusters, kmeans->cluster_centers, features->preprocessed_features);
    dlib::find_clusters_using_kmeans(features->preprocessed_features, kmeans->cluster_centers, kmeans->max_iterations);

    for (const auto &preprocessed_feature : features->preprocessed_features) {
        calculated_number_t mean_dist = 0.0;

        for (const auto &cluster_center : kmeans->cluster_centers) {
            mean_dist += dlib::length(cluster_center - preprocessed_feature);
        }

        mean_dist /= kmeans->num_clusters;

        if (mean_dist < kmeans->min_dist)
            kmeans->min_dist = mean_dist;

        if (mean_dist > kmeans->max_dist)
            kmeans->max_dist = mean_dist;
    }
}

static calculated_number_t nml_kmeans_anomaly_score(const nml_kmeans_t *kmeans, const DSample &DS) {
    calculated_number_t mean_dist = 0.0;
    for (const auto &CC: kmeans->cluster_centers)
        mean_dist += dlib::length(CC - DS);

    mean_dist /= kmeans->num_clusters;

    if (kmeans->max_dist == kmeans->min_dist)
        return 0.0;

    calculated_number_t anomaly_score = 100.0 * std::abs((mean_dist - kmeans->min_dist) / (kmeans->max_dist - kmeans->min_dist));
    return (anomaly_score > 100.0) ? 100.0 : anomaly_score;
}

/*
 * Queue
*/

nml_queue_t *nml_queue_init() {
    nml_queue_t *q = new nml_queue_t();

    netdata_mutex_init(&q->mutex);
    pthread_cond_init(&q->cond_var, NULL);
    q->exit = false;
    return q;
}

void nml_queue_destroy(nml_queue_t *q) {
    netdata_mutex_destroy(&q->mutex);
    pthread_cond_destroy(&q->cond_var);
    delete q;
}

void nml_queue_push(nml_queue_t *q, const nml_training_request_t req) {
    netdata_mutex_lock(&q->mutex);
    q->internal.push(req);
    pthread_cond_signal(&q->cond_var);
    netdata_mutex_unlock(&q->mutex);
}

nml_training_request_t nml_queue_pop(nml_queue_t *q) {
    netdata_mutex_lock(&q->mutex);

    nml_training_request_t req = { NULL, NULL, 0, 0, 0 };

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

size_t nml_queue_size(nml_queue_t *q) {
    netdata_mutex_lock(&q->mutex);
    size_t size = q->internal.size();
    netdata_mutex_unlock(&q->mutex);
    return size;
}

void nml_queue_signal(nml_queue_t *q) {
    netdata_mutex_lock(&q->mutex);
    q->exit = true;
    pthread_cond_signal(&q->cond_var);
    netdata_mutex_unlock(&q->mutex);
}

/*
 * Dimension
*/

static std::pair<calculated_number_t *, nml_training_response_t>
nml_dimension_calculated_numbers(nml_dimension_t *dim, const nml_training_request_t &training_request) {
    nml_training_response_t training_response = {};

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
    struct storage_engine_query_ops *ops = dim->rd->tiers[0].query_ops;
    struct storage_engine_query_handle handle;

    ops->init(dim->rd->tiers[0].db_metric_handle,
              &handle,
              training_response.query_after_t,
              training_response.query_before_t,
              STORAGE_PRIORITY_BEST_EFFORT);

    size_t idx = 0;
    memset(tls_data.training_cns, 0, sizeof(calculated_number_t) * max_n * (Cfg.lag_n + 1));
    calculated_number_t last_value = std::numeric_limits<calculated_number_t>::quiet_NaN();

    while (!ops->is_finished(&handle)) {
        if (idx == max_n)
            break;

        STORAGE_POINT sp = ops->next_metric(&handle);

        time_t timestamp = sp.end_time_s;
        calculated_number_t value = sp.sum / sp.count;

        if (netdata_double_isnumber(value)) {
            if (!training_response.db_after_t)
                training_response.db_after_t = timestamp;
            training_response.db_before_t = timestamp;

            tls_data.training_cns[idx] = value;
            last_value = tls_data.training_cns[idx];
            training_response.collected_values++;
        } else
            tls_data.training_cns[idx] = last_value;

        idx++;
    }
    ops->finalize(&handle);

    global_statistics_ml_query_completed(/* points_read */ idx);

    training_response.total_values = idx;
    if (training_response.collected_values < min_n) {
        training_response.result = TRAINING_RESULT_NOT_ENOUGH_COLLECTED_VALUES;
        return { NULL, training_response };
    }

    // Find first non-NaN value.
    for (idx = 0; std::isnan(tls_data.training_cns[idx]); idx++, training_response.total_values--) { }

    // Overwrite NaN values.
    if (idx != 0)
        memmove(tls_data.training_cns, &tls_data.training_cns[idx], sizeof(calculated_number_t) * training_response.total_values);

    training_response.result = TRAINING_RESULT_OK;
    return { tls_data.training_cns, training_response };
}

static enum nml_training_result
nml_dimension_train_model(nml_dimension_t *dim, const nml_training_request_t &training_request) {
    auto P = nml_dimension_calculated_numbers(dim, training_request);
    nml_training_response_t training_response = P.second;

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

        dim->tr = training_response;

        dim->last_training_time = training_response.last_entry_on_response;
        enum nml_training_result result = training_response.result;
        netdata_mutex_unlock(&dim->mutex);

        return result;
    }

    // compute kmeans
    {
        memcpy(tls_data.scratch_training_cns, tls_data.training_cns,
               training_response.total_values * sizeof(calculated_number_t));

        nml_features_t features = {
            Cfg.diff_n, Cfg.smooth_n, Cfg.lag_n,
            tls_data.scratch_training_cns, training_response.total_values,
            tls_data.training_cns, training_response.total_values,
            tls_data.training_samples
        };
        nml_features_preprocess(&features);

        nml_kmeans_init(&dim->kmeans, 2, 1000);
        nml_kmeans_train(&dim->kmeans, &features);
    }

    // update kmeans models
    {
        netdata_mutex_lock(&dim->mutex);

        if (dim->km_contexts.size() < Cfg.num_models_to_use) {
            dim->km_contexts.push_back(std::move(dim->kmeans));
        } else {
            std::rotate(std::begin(dim->km_contexts), std::begin(dim->km_contexts) + 1, std::end(dim->km_contexts));
            dim->km_contexts[dim->km_contexts.size() - 1] = std::move(dim->kmeans);
        }

        dim->mt = METRIC_TYPE_CONSTANT;
        dim->ts = TRAINING_STATUS_TRAINED;
        dim->tr = training_response;
        dim->last_training_time = rrddim_last_entry_s(dim->rd);

        netdata_mutex_unlock(&dim->mutex);
    }

    return training_response.result;
}

static void nml_dimension_schedule_for_training(nml_dimension_t *dim, time_t curr_time) {
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
    case TRAINING_STATUS_TRAINED:
        if ((dim->last_training_time + (Cfg.train_every * dim->rd->update_every)) < curr_time) {
            schedule_for_training = true;
            dim->ts = TRAINING_STATUS_PENDING_WITH_MODEL;
        }
        break;
    }

    if (schedule_for_training) {
        nml_host_t *host = reinterpret_cast<nml_host_t *>(dim->rd->rrdset->rrdhost->ml_host);
        nml_training_request_t req = {
            string_dup(dim->rd->rrdset->id), string_dup(dim->rd->id),
            curr_time, rrddim_first_entry_s(dim->rd), rrddim_last_entry_s(dim->rd),
        };
        nml_queue_push(host->training_queue, req);
    }
}

bool nml_dimension_predict(nml_dimension_t *dim, time_t curr_time, calculated_number_t value, bool exists) {
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

    nml_features_t features = {
        Cfg.diff_n, Cfg.smooth_n, Cfg.lag_n,
        dst_cns, n, src_cns, n,
        dim->feature
    };
    nml_features_preprocess(&features);

    /*
     * Lock to predict and possibly schedule the dimension for training
    */
    if (netdata_mutex_trylock(&dim->mutex) != 0)
        return false;

    // Mark the metric time as variable if we received different values
    if (!same_value)
        dim->mt = METRIC_TYPE_VARIABLE;

    // Decide if the dimension needs to be scheduled for training
    nml_dimension_schedule_for_training(dim, curr_time);

    // Nothing to do if we don't have a model
    switch (dim->ts) {
        case TRAINING_STATUS_UNTRAINED:
        case TRAINING_STATUS_PENDING_WITHOUT_MODEL: {
            netdata_mutex_unlock(&dim->mutex);
            return false;
        }
        default:
            break;
    }

    /*
     * Use the KMeans models to check if the value is anomalous
    */

    size_t sum = 0;
    size_t models_consulted = 0;

    for (const auto &km_ctx : dim->km_contexts) {
        models_consulted++;

        calculated_number_t anomaly_score = nml_kmeans_anomaly_score(&km_ctx, features.preprocessed_features[0]);
        if (anomaly_score == std::numeric_limits<calculated_number_t>::quiet_NaN())
            continue;

        if (anomaly_score < (100 * Cfg.dimension_anomaly_score_threshold)) {
            global_statistics_ml_models_consulted(models_consulted);
            netdata_mutex_unlock(&dim->mutex);
            return false;
        }

        sum += 1;
    }

    netdata_mutex_unlock(&dim->mutex);

    global_statistics_ml_models_consulted(models_consulted);
    return sum;
}

void nml_dimension_dump(nml_dimension_t *dim) {
    const char *chart_id = rrdset_id(dim->rd->rrdset);
    const char *dimension_id = rrddim_id(dim->rd);

    const char *mls_str = nml_machine_learning_status_to_string(dim->mls);
    const char *mt_str = nml_metric_type_to_string(dim->mt);
    const char *ts_str = nml_training_status_to_string(dim->ts);
    const char *tr_str = nml_training_result_to_string(dim->tr.result);

    const char *fmt =
        "[ML] %s.%s: MLS=%s, MT=%s, TS=%s, Result=%s, "
        "ReqTime=%ld, FEOReq=%ld, LEOReq=%ld, "
        "FEOResp=%ld, LEOResp=%ld, QTR=<%ld, %ld>, DBTR=<%ld, %ld>, Collected=%zu, Total=%zu";

    error(fmt,
          chart_id, dimension_id, mls_str, mt_str, ts_str, tr_str,
          dim->tr.request_time, dim->tr.first_entry_on_request, dim->tr.last_entry_on_request,
          dim->tr.first_entry_on_response, dim->tr.last_entry_on_response,
          dim->tr.query_after_t, dim->tr.query_before_t, dim->tr.db_after_t, dim->tr.db_before_t, dim->tr.collected_values, dim->tr.total_values
    );
}

nml_dimension_t *nml_dimension_new(RRDDIM *rd) {
    nml_dimension_t *dim = new nml_dimension_t();

    dim->rd = rd;

    dim->mt = METRIC_TYPE_CONSTANT;
    dim->ts = TRAINING_STATUS_UNTRAINED;

    dim->last_training_time = 0;

    nml_kmeans_init(&dim->kmeans, 2, 1000);

    if (simple_pattern_matches(Cfg.sp_charts_to_skip, rrdset_name(rd->rrdset)))
        dim->mls = MACHINE_LEARNING_STATUS_DISABLED_DUE_TO_EXCLUDED_CHART;
    else
        dim->mls = MACHINE_LEARNING_STATUS_ENABLED;

    netdata_mutex_init(&dim->mutex);

    dim->km_contexts.reserve(Cfg.num_models_to_use);

    return dim;
}

void nml_dimension_delete(nml_dimension_t *dim) {
    netdata_mutex_destroy(&dim->mutex);
    delete dim;
}

nml_chart_t *nml_chart_new(RRDSET *rs) {
    nml_chart_t *chart = new nml_chart_t();

    chart->rs = rs;
    chart->mls = nml_machine_learning_stats_t();

    netdata_mutex_init(&chart->mutex);

    return chart;
}

void nml_chart_delete(nml_chart_t *chart) {
    netdata_mutex_destroy(&chart->mutex);
    delete chart;
}

static bool nml_chart_is_available_for_ml(nml_chart_t *chart) {
    return rrdset_is_available_for_exporting_and_alarms(chart->rs);
}

static std::string ml_dimension_get_id(RRDDIM *rd) {
    RRDSET *rs = rd->rrdset;

    std::stringstream ss;
    ss << rrdset_context(rs) << "|" << rrdset_id(rs) << "|" << rrddim_name(rd);
    return ss.str();
}

static void nml_chart_get_models_as_json(nml_chart_t *chart, nlohmann::json &j) {
    netdata_mutex_lock(&chart->mutex);

    void *rdp = NULL;
    rrddim_foreach_read(rdp, chart->rs) {
        RRDDIM *rd = static_cast<RRDDIM *>(rdp);
        nml_dimension_t *dim = reinterpret_cast<nml_dimension_t *>(rd->ml_dimension);
        if (!dim)
            continue;

        nlohmann::json jarray = nlohmann::json::array();
#if 0
        for (const KMeans &KM : nml_dimension_models(D)) {
            nlohmann::json tmp;

            KM.toJson(tmp);
            jarray.push_back(tmp);
            j[ml_dimension_get_id(D->rd)] = jarray;
        }
#else
        j[ml_dimension_get_id(dim->rd)] = jarray;
#endif
    }
    rrdset_foreach_done(rdp);

    netdata_mutex_unlock(&chart->mutex);
}

void nml_chart_update_begin(nml_chart_t *chart) {
    netdata_mutex_lock(&chart->mutex);
    chart->mls = {};
}

void nml_chart_update_end(nml_chart_t *chart) {
    netdata_mutex_unlock(&chart->mutex);
}

void nml_chart_update_dimension(nml_chart_t *chart, nml_dimension_t *dim, bool is_anomalous) {
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
            }

            return;
        }
    }
}

nml_host_t *nml_host_new(RRDHOST *rh) {
    nml_host_t *host = new nml_host_t();

    host->rh = rh;
    host->mls = nml_machine_learning_stats_t();
    host->ts = nml_training_stats_t();

    host->host_anomaly_rate = 0.0;
    host->threads_running = false;
    host->threads_cancelled = false;
    host->threads_joined = false;

    host->training_queue = nml_queue_init();

    netdata_mutex_init(&host->mutex);

    return host;
}

void nml_host_delete(nml_host_t *host) {
    netdata_mutex_destroy(&host->mutex);
    nml_queue_destroy(host->training_queue);
    delete host;
}

void nml_host_get_config_as_json(nml_host_t *host, BUFFER *wb) {
    // Unused for now, until we add support for per-host configs
    (void) host;

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

void nml_host_get_models_as_json(nml_host_t *host, nlohmann::json &j) {
    netdata_mutex_lock(&host->mutex);

    void* rsp = NULL;
    rrdset_foreach_read(rsp, host->rh) {
        RRDSET *rs = static_cast<RRDSET *>(rsp);
        nml_chart_t *chart = reinterpret_cast<nml_chart_t *>(rs->ml_chart);

        if (!chart)
            continue;

        nml_chart_get_models_as_json(chart, j);
    }
    rrdset_foreach_done(rsp);

    netdata_mutex_unlock(&host->mutex);
}

#define WORKER_JOB_DETECTION_PREP 0
#define WORKER_JOB_DETECTION_DIM_CHART 1
#define WORKER_JOB_DETECTION_HOST_CHART 2
#define WORKER_JOB_DETECTION_STATS 3
#define WORKER_JOB_DETECTION_RESOURCES 4

static void nml_host_detect_once(nml_host_t *host) {
    worker_is_busy(WORKER_JOB_DETECTION_PREP);

    host->mls = {};
    nml_machine_learning_stats_t mls_copy = {};
    nml_training_stats_t ts_copy = {};

    {
        netdata_mutex_lock(&host->mutex);

        /*
         * prediction/detection stats
        */
        void *rsp = NULL;
        rrdset_foreach_read(rsp, host->rh) {
            RRDSET *rs = static_cast<RRDSET *>(rsp);

            nml_chart_t *chart = reinterpret_cast<nml_chart_t *>(rs->ml_chart);
            if (!chart)
                continue;

            if (!nml_chart_is_available_for_ml(chart))
                continue;

            nml_machine_learning_stats_t chart_mls = chart->mls;

            host->mls.num_machine_learning_status_enabled += chart_mls.num_machine_learning_status_enabled;
            host->mls.num_machine_learning_status_disabled_sp += chart_mls.num_machine_learning_status_disabled_sp;

            host->mls.num_metric_type_constant += chart_mls.num_metric_type_constant;
            host->mls.num_metric_type_variable += chart_mls.num_metric_type_variable;

            host->mls.num_training_status_untrained += chart_mls.num_training_status_untrained;
            host->mls.num_training_status_pending_without_model += chart_mls.num_training_status_pending_without_model;
            host->mls.num_training_status_trained += chart_mls.num_training_status_trained;
            host->mls.num_training_status_pending_with_model += chart_mls.num_training_status_pending_with_model;

            host->mls.num_anomalous_dimensions += chart_mls.num_anomalous_dimensions;
            host->mls.num_normal_dimensions += chart_mls.num_normal_dimensions;
        }
        rrdset_foreach_done(rsp);

        host->host_anomaly_rate = 0.0;
        size_t NumActiveDimensions = host->mls.num_anomalous_dimensions + host->mls.num_normal_dimensions;
        if (NumActiveDimensions)
              host->host_anomaly_rate = static_cast<double>(host->mls.num_anomalous_dimensions) / NumActiveDimensions;

        mls_copy = host->mls;

        /*
         * training stats
        */
        ts_copy = host->ts;

        host->ts.queue_size = 0;
        host->ts.num_popped_items = 0;

        host->ts.allotted_ut = 0;
        host->ts.consumed_ut = 0;
        host->ts.remaining_ut = 0;

        host->ts.training_result_ok = 0;
        host->ts.training_result_invalid_query_time_range = 0;
        host->ts.training_result_not_enough_collected_values = 0;
        host->ts.training_result_null_acquired_dimension = 0;
        host->ts.training_result_chart_under_replication = 0;

        netdata_mutex_unlock(&host->mutex);
    }

    // Calc the avg values
    if (ts_copy.num_popped_items) {
        ts_copy.queue_size /= ts_copy.num_popped_items;
        ts_copy.allotted_ut /= ts_copy.num_popped_items;
        ts_copy.consumed_ut /= ts_copy.num_popped_items;
        ts_copy.remaining_ut /= ts_copy.num_popped_items;

        ts_copy.training_result_ok /= ts_copy.num_popped_items;
        ts_copy.training_result_invalid_query_time_range /= ts_copy.num_popped_items;
        ts_copy.training_result_not_enough_collected_values /= ts_copy.num_popped_items;
        ts_copy.training_result_null_acquired_dimension /= ts_copy.num_popped_items;
        ts_copy.training_result_chart_under_replication /= ts_copy.num_popped_items;
    } else {
        ts_copy.queue_size = 0;
        ts_copy.allotted_ut = 0;
        ts_copy.consumed_ut = 0;
        ts_copy.remaining_ut = 0;
    }

    worker_is_busy(WORKER_JOB_DETECTION_DIM_CHART);
    nml_update_dimensions_chart(host, mls_copy);

    worker_is_busy(WORKER_JOB_DETECTION_HOST_CHART);
    nml_update_host_and_detection_rate_charts(host, host->host_anomaly_rate * 10000.0);

#ifdef NETDATA_ML_RESOURCE_CHARTS
    worker_is_busy(WORKER_JOB_DETECTION_RESOURCES);
    struct rusage PredictionRU;
    getrusage(RUSAGE_THREAD, &PredictionRU);
    updateResourceUsageCharts(RH, PredictionRU, TSCopy.TrainingRU);
#endif

    worker_is_busy(WORKER_JOB_DETECTION_STATS);
    nml_update_training_statistics_chart(host, ts_copy);
}

typedef struct {
    RRDDIM_ACQUIRED *acq_rd;
    nml_dimension_t *dim;
} nml_acquired_dimension_t;

static nml_acquired_dimension_t nml_acquired_dimension_get(RRDHOST *rh, STRING *chart_id, STRING *dimension_id) {
    RRDDIM_ACQUIRED *acq_rd = NULL;
    nml_dimension_t *dim = NULL;

    RRDSET *rs = rrdset_find(rh, string2str(chart_id));
    if (rs) {
        acq_rd = rrddim_find_and_acquire(rs, string2str(dimension_id));
        if (acq_rd) {
            RRDDIM *rd = rrddim_acquired_to_rrddim(acq_rd);
            if (rd)
                dim = reinterpret_cast<nml_dimension_t *>(rd->ml_dimension);
        }
    }

    nml_acquired_dimension_t acq_dim = {
        acq_rd, dim
    };

    return acq_dim;
}

static void nml_acquired_dimension_release(nml_acquired_dimension_t acq_dim) {
    if (!acq_dim.acq_rd)
        return;

    rrddim_acquired_release(acq_dim.acq_rd);
}

static enum nml_training_result nml_acquired_dimension_train(nml_acquired_dimension_t acq_dim, const nml_training_request_t &TR) {
    if (!acq_dim.dim)
        return TRAINING_RESULT_NULL_ACQUIRED_DIMENSION;

    return nml_dimension_train_model(acq_dim.dim, TR);
}

#define WORKER_JOB_TRAINING_FIND 0
#define WORKER_JOB_TRAINING_TRAIN 1
#define WORKER_JOB_TRAINING_STATS 2

void nml_host_get_detection_info_as_json(nml_host_t *host, nlohmann::json &j) {
    j["version"] = 1;
    j["anomalous-dimensions"] = host->mls.num_anomalous_dimensions;
    j["normal-dimensions"] = host->mls.num_normal_dimensions;
    j["total-dimensions"] = host->mls.num_anomalous_dimensions + host->mls.num_normal_dimensions;
    j["trained-dimensions"] = host->mls.num_training_status_trained + host->mls.num_training_status_pending_with_model;
}

void nml_host_train(nml_host_t *host) {
    worker_register("MLTRAIN");
    worker_register_job_name(WORKER_JOB_TRAINING_FIND, "find");
    worker_register_job_name(WORKER_JOB_TRAINING_TRAIN, "train");
    worker_register_job_name(WORKER_JOB_TRAINING_STATS, "stats");

    service_register(SERVICE_THREAD_TYPE_NETDATA, NULL, (force_quit_t )ml_cancel_training_thread, host->rh, true);

    while (service_running(SERVICE_ML_TRAINING)) {
        nml_training_request_t training_req = nml_queue_pop(host->training_queue);
        size_t queue_size = nml_queue_size(host->training_queue) + 1;
        
        if (host->threads_cancelled) {
            info("Stopping training thread for host %s because it was cancelled", rrdhost_hostname(host->rh));
            break;
        }

        usec_t allotted_ut = (Cfg.train_every * host->rh->rrd_update_every * USEC_PER_SEC) / queue_size;
        if (allotted_ut > USEC_PER_SEC)
            allotted_ut = USEC_PER_SEC;

        usec_t start_ut = now_monotonic_usec();
        enum nml_training_result training_res;
        {
            worker_is_busy(WORKER_JOB_TRAINING_FIND);
            nml_acquired_dimension_t acq_dim = nml_acquired_dimension_get(host->rh, training_req.chart_id, training_req.dimension_id);

            worker_is_busy(WORKER_JOB_TRAINING_TRAIN);
            training_res = nml_acquired_dimension_train(acq_dim, training_req);

            string_freez(training_req.chart_id);
            string_freez(training_req.dimension_id);

            nml_acquired_dimension_release(acq_dim);
        }
        usec_t consumed_ut = now_monotonic_usec() - start_ut;

        worker_is_busy(WORKER_JOB_TRAINING_STATS);

        usec_t remaining_ut = 0;
        if (consumed_ut < allotted_ut)
            remaining_ut = allotted_ut - consumed_ut;

        {
            netdata_mutex_lock(&host->mutex);

            if (host->ts.allotted_ut == 0) {
                struct rusage TRU;
                getrusage(RUSAGE_THREAD, &TRU);
                host->ts.training_ru = TRU;
            }

            host->ts.queue_size += queue_size;
            host->ts.num_popped_items += 1;

            host->ts.allotted_ut += allotted_ut;
            host->ts.consumed_ut += consumed_ut;
            host->ts.remaining_ut += remaining_ut;

            switch (training_res) {
                case TRAINING_RESULT_OK:
                    host->ts.training_result_ok += 1;
                    break;
                case TRAINING_RESULT_INVALID_QUERY_TIME_RANGE:
                    host->ts.training_result_invalid_query_time_range += 1;
                    break;
                case TRAINING_RESULT_NOT_ENOUGH_COLLECTED_VALUES:
                    host->ts.training_result_not_enough_collected_values += 1;
                    break;
                case TRAINING_RESULT_NULL_ACQUIRED_DIMENSION:
                    host->ts.training_result_null_acquired_dimension += 1;
                    break;
                case TRAINING_RESULT_CHART_UNDER_REPLICATION:
                    host->ts.training_result_chart_under_replication += 1;
                    break;
            }

            netdata_mutex_unlock(&host->mutex);
        }

        worker_is_idle();
        std::this_thread::sleep_for(std::chrono::microseconds{remaining_ut});
        worker_is_busy(0);
    }
}

static void *train_main(void *arg) {
    size_t max_elements_needed_for_training = Cfg.max_train_samples * (Cfg.lag_n + 1);
    tls_data.training_cns = new calculated_number_t[max_elements_needed_for_training]();
    tls_data.scratch_training_cns = new calculated_number_t[max_elements_needed_for_training]();

    nml_host_t *host = reinterpret_cast<nml_host_t *>(arg);
    nml_host_train(host);
    return NULL;
}

void nml_host_start_training_thread(nml_host_t *host) {
    if (host->threads_running) {
        error("Anomaly detections threads for host %s are already-up and running.", rrdhost_hostname(host->rh));
        return;
    }

    host->threads_running = true;
    host->threads_cancelled = false;
    host->threads_joined = false;

    char tag[NETDATA_THREAD_TAG_MAX + 1];

    snprintfz(tag, NETDATA_THREAD_TAG_MAX, "MLTR[%s]", rrdhost_hostname(host->rh));
    netdata_thread_create(&host->training_thread, tag, NETDATA_THREAD_OPTION_JOINABLE, train_main, static_cast<void *>(host));
}

void nml_host_stop_training_thread(nml_host_t *host, bool join) {
    if (!host->threads_running) {
        error("Anomaly detections threads for host %s have already been stopped.", rrdhost_hostname(host->rh));
        return;
    }

    if (!host->threads_cancelled) {
        host->threads_cancelled = true;

        // Signal the training queue to stop popping-items
        nml_queue_signal(host->training_queue);
        netdata_thread_cancel(host->training_thread);
    }

    if (join && !host->threads_joined) {
        host->threads_joined = true;
        host->threads_running = false;

        delete[] tls_data.training_cns;
        delete[] tls_data.scratch_training_cns;

        netdata_thread_join(host->training_thread, NULL);
    }
}

void *nml_detect_main(void *arg) {
    UNUSED(arg);

    worker_register("MLDETECT");
    worker_register_job_name(WORKER_JOB_DETECTION_PREP, "prep");
    worker_register_job_name(WORKER_JOB_DETECTION_DIM_CHART, "dim chart");
    worker_register_job_name(WORKER_JOB_DETECTION_HOST_CHART, "host chart");
    worker_register_job_name(WORKER_JOB_DETECTION_STATS, "stats");
    worker_register_job_name(WORKER_JOB_DETECTION_RESOURCES, "resources");

    service_register(SERVICE_THREAD_TYPE_NETDATA, NULL, NULL, NULL, true);

    heartbeat_t hb;
    heartbeat_init(&hb);

    while (service_running((SERVICE_TYPE)(SERVICE_ML_PREDICTION | SERVICE_COLLECTORS))) {
        worker_is_idle();
        heartbeat_next(&hb, USEC_PER_SEC);

        rrd_rdlock();

        RRDHOST *rh;
        rrdhost_foreach_read(rh) {
            if (!rh->ml_host)
                continue;

            nml_host_detect_once(reinterpret_cast<nml_host_t *>(rh->ml_host));
        }

        rrd_unlock();
    }

    return NULL;
}
