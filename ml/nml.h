// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NML_H
#define NETDATA_NML_H

#include "dlib/matrix.h"
#include "ml/ml.h"

#include <vector>
#include <queue>

typedef double calculated_number_t;
typedef dlib::matrix<calculated_number_t, 6, 1> DSample;

/*
 * Features
 */

typedef struct {
    size_t diff_n;
    size_t smooth_n;
    size_t lag_n;

    calculated_number_t *dst;
    size_t dst_n;

    calculated_number_t *src;
    size_t src_n;

    std::vector<DSample> &preprocessed_features;
} nml_features_t;

/*
 * KMeans
 */
typedef struct {
    size_t num_clusters;
    size_t max_iterations;

    std::vector<DSample> cluster_centers;

    calculated_number_t min_dist;
    calculated_number_t max_dist;
} nml_kmeans_t;

#include "json/single_include/nlohmann/json.hpp"

typedef struct machine_learning_stats_t {
    size_t num_machine_learning_status_enabled;
    size_t num_machine_learning_status_disabled_sp;

    size_t num_metric_type_constant;
    size_t num_metric_type_variable;

    size_t num_training_status_untrained;
    size_t num_training_status_pending_without_model;
    size_t num_training_status_trained;
    size_t num_training_status_pending_with_model;

    size_t num_anomalous_dimensions;
    size_t num_normal_dimensions;
} nml_machine_learning_stats_t;

typedef struct training_stats_t {
    struct rusage training_ru;

    size_t queue_size;
    size_t num_popped_items;

    usec_t allotted_ut;
    usec_t consumed_ut;
    usec_t remaining_ut;

    size_t training_result_ok;
    size_t training_result_invalid_query_time_range;
    size_t training_result_not_enough_collected_values;
    size_t training_result_null_acquired_dimension;
    size_t training_result_chart_under_replication;
} nml_training_stats_t;

enum nml_metric_type {
    // The dimension has constant values, no need to train
    METRIC_TYPE_CONSTANT,

    // The dimension's values fluctuate, we need to generate a model
    METRIC_TYPE_VARIABLE,
};

enum nml_machine_learning_status {
    // Enable training/prediction
    MACHINE_LEARNING_STATUS_ENABLED,

    // Disable because configuration pattern matches the chart's id
    MACHINE_LEARNING_STATUS_DISABLED_DUE_TO_EXCLUDED_CHART,
};

enum nml_training_status {
    // We don't have a model for this dimension
    TRAINING_STATUS_UNTRAINED,

    // Request for training sent, but we don't have any models yet
    TRAINING_STATUS_PENDING_WITHOUT_MODEL,

    // Request to update existing models sent
    TRAINING_STATUS_PENDING_WITH_MODEL,

    // Have a valid, up-to-date model
    TRAINING_STATUS_TRAINED,
};

enum nml_training_result {
    // We managed to create a KMeans model
    TRAINING_RESULT_OK,

    // Could not query DB with a correct time range
    TRAINING_RESULT_INVALID_QUERY_TIME_RANGE,

    // Did not gather enough data from DB to run KMeans
    TRAINING_RESULT_NOT_ENOUGH_COLLECTED_VALUES,

    // Acquired a null dimension
    TRAINING_RESULT_NULL_ACQUIRED_DIMENSION,

    // Chart is under replication
    TRAINING_RESULT_CHART_UNDER_REPLICATION,
};

typedef struct {
    // Chart/dimension we want to train
    STRING *chart_id;
    STRING *dimension_id;

    // Creation time of request
    time_t request_time;

    // First/last entry of this dimension in DB
    // at the point the request was made
    time_t first_entry_on_request;
    time_t last_entry_on_request;
} nml_training_request_t;

typedef struct {
    // Time when the request for this response was made
    time_t request_time;

    // First/last entry of the dimension in DB when generating the request
    time_t first_entry_on_request;
    time_t last_entry_on_request;

    // First/last entry of the dimension in DB when generating the response
    time_t first_entry_on_response;
    time_t last_entry_on_response;

    // After/Before timestamps of our DB query
    time_t query_after_t;
    time_t query_before_t;

    // Actual after/before returned by the DB query ops
    time_t db_after_t;
    time_t db_before_t;

    // Number of doubles returned by the DB query
    size_t collected_values;

    // Number of values we return to the caller
    size_t total_values;

    // Result of training response
    enum nml_training_result result;
} nml_training_response_t;

/*
 * Queue
*/

typedef struct {
    std::queue<nml_training_request_t> internal;
    netdata_mutex_t mutex;
    pthread_cond_t cond_var;
    std::atomic<bool> exit;
} nml_queue_t;

nml_queue_t *nml_queue_init(void);
void nml_queue_destroy(nml_queue_t *q);

void nml_queue_push(nml_queue_t *q, const nml_training_request_t req);
nml_training_request_t nml_queue_pop(nml_queue_t *q);
size_t nml_queue_size(nml_queue_t *q);

void nml_queue_signal(nml_queue_t *q);

typedef struct {
    RRDDIM *rd;

    enum nml_metric_type mt;
    enum nml_training_status ts;
    enum nml_machine_learning_status mls;

    nml_training_response_t tr;
    time_t last_training_time;

    std::vector<calculated_number_t> cns;

    std::vector<nml_kmeans_t> km_contexts;
    netdata_mutex_t mutex;
    nml_kmeans_t kmeans;
    std::vector<DSample> feature;
} nml_dimension_t;

nml_dimension_t *nml_dimension_new(RRDDIM *rd);
void nml_dimension_delete(nml_dimension_t *dim);

bool nml_dimension_predict(nml_dimension_t *d, time_t curr_t, calculated_number_t value, bool exists);

typedef struct {
    RRDSET *rs;
    nml_machine_learning_stats_t mls;

    netdata_mutex_t mutex;
} nml_chart_t;

nml_chart_t *nml_chart_new(RRDSET *rs);
void nml_chart_delete(nml_chart_t *chart);

void nml_chart_update_begin(nml_chart_t *chart);
void nml_chart_update_end(nml_chart_t *chart);
void nml_chart_update_dimension(nml_chart_t *chart, nml_dimension_t *dim, bool is_anomalous);

typedef struct {
    RRDHOST *rh;

    nml_machine_learning_stats_t mls;
    nml_training_stats_t ts;

    calculated_number_t host_anomaly_rate;

    std::atomic<bool> threads_running;
    std::atomic<bool> threads_cancelled;
    std::atomic<bool> threads_joined;

    nml_queue_t *training_queue;

    netdata_mutex_t mutex;

    netdata_thread_t training_thread;

    /*
     * bookkeeping for anomaly detection charts
    */

    RRDSET *machine_learning_status_rs;
    RRDDIM *machine_learning_status_enabled_rd;
    RRDDIM *machine_learning_status_disabled_sp_rd;

    RRDSET *metric_type_rs;
    RRDDIM *metric_type_constant_rd;
    RRDDIM *metric_type_variable_rd;

    RRDSET *training_status_rs;
    RRDDIM *training_status_untrained_rd;
    RRDDIM *training_status_pending_without_model_rd;
    RRDDIM *training_status_trained_rd;
    RRDDIM *training_status_pending_with_model_rd;

    RRDSET *dimensions_rs;
    RRDDIM *dimensions_anomalous_rd;
    RRDDIM *dimensions_normal_rd;

    RRDSET *anomaly_rate_rs;
    RRDDIM *anomaly_rate_rd;

    RRDSET *detector_events_rs;
    RRDDIM *detector_events_above_threshold_rd;
    RRDDIM *detector_events_new_anomaly_event_rd;

    RRDSET *queue_stats_rs;
    RRDDIM *queue_stats_queue_size_rd;
    RRDDIM *queue_stats_popped_items_rd;

    RRDSET *training_time_stats_rs;
    RRDDIM *training_time_stats_allotted_rd;
    RRDDIM *training_time_stats_consumed_rd;
    RRDDIM *training_time_stats_remaining_rd;

    RRDSET *training_results_rs;
    RRDDIM *training_results_ok_rd;
    RRDDIM *training_results_invalid_query_time_range_rd;
    RRDDIM *training_results_not_enough_collected_values_rd;
    RRDDIM *training_results_null_acquired_dimension_rd;
    RRDDIM *training_results_chart_under_replication_rd;
} nml_host_t;

nml_host_t *nml_host_new(RRDHOST *rh);
void nml_host_delete(nml_host_t *host);

void nml_host_start_training_thread(nml_host_t *host);
void nml_host_stop_training_thread(nml_host_t *host, bool join);

void nml_host_get_config_as_json(nml_host_t *host, BUFFER *wb);
void nml_host_get_models_as_json(nml_host_t *host, nlohmann::json &j);
void nml_host_get_detection_info_as_json(nml_host_t *host, nlohmann::json &j);

typedef struct {
    bool enable_anomaly_detection;

    unsigned max_train_samples;
    unsigned min_train_samples;
    unsigned train_every;

    unsigned num_models_to_use;

    unsigned db_engine_anomaly_rate_every;

    unsigned diff_n;
    unsigned smooth_n;
    unsigned lag_n;

    double random_sampling_ratio;
    unsigned max_kmeans_iters;

    double dimension_anomaly_score_threshold;

    double host_anomaly_rate_threshold;
    RRDR_TIME_GROUPING anomaly_detection_grouping_method;
    time_t anomaly_detection_query_duration;

    bool stream_anomaly_detection_charts;

    std::string hosts_to_skip;
    SIMPLE_PATTERN *sp_host_to_skip;

    std::string charts_to_skip;
    SIMPLE_PATTERN *sp_charts_to_skip;

    std::vector<uint32_t> random_nums;

    netdata_thread_t detection_thread;
} nml_config_t;

void nml_config_load(nml_config_t *cfg);

void *nml_detect_main(void *arg);

extern nml_config_t Cfg;

#endif /* NETDATA_NML_H */
