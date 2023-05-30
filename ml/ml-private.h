// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_PRIVATE_H
#define NETDATA_ML_PRIVATE_H

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
} ml_features_t;

/*
 * KMeans
 */

typedef struct {
    std::vector<DSample> cluster_centers;

    calculated_number_t min_dist;
    calculated_number_t max_dist;

    uint32_t after;
    uint32_t before;
} ml_kmeans_t;

typedef struct machine_learning_stats_t {
    size_t num_machine_learning_status_enabled;
    size_t num_machine_learning_status_disabled_sp;

    size_t num_metric_type_constant;
    size_t num_metric_type_variable;

    size_t num_training_status_untrained;
    size_t num_training_status_pending_without_model;
    size_t num_training_status_trained;
    size_t num_training_status_pending_with_model;
    size_t num_training_status_silenced;

    size_t num_anomalous_dimensions;
    size_t num_normal_dimensions;
} ml_machine_learning_stats_t;

typedef struct training_stats_t {
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
} ml_training_stats_t;

enum ml_metric_type {
    // The dimension has constant values, no need to train
    METRIC_TYPE_CONSTANT,

    // The dimension's values fluctuate, we need to generate a model
    METRIC_TYPE_VARIABLE,
};

enum ml_machine_learning_status {
    // Enable training/prediction
    MACHINE_LEARNING_STATUS_ENABLED,

    // Disable because configuration pattern matches the chart's id
    MACHINE_LEARNING_STATUS_DISABLED_DUE_TO_EXCLUDED_CHART,
};

enum ml_training_status {
    // We don't have a model for this dimension
    TRAINING_STATUS_UNTRAINED,

    // Request for training sent, but we don't have any models yet
    TRAINING_STATUS_PENDING_WITHOUT_MODEL,

    // Request to update existing models sent
    TRAINING_STATUS_PENDING_WITH_MODEL,

    // Have a valid, up-to-date model
    TRAINING_STATUS_TRAINED,

    // Have a valid, up-to-date model that is silenced because its too noisy
    TRAINING_STATUS_SILENCED,
};

enum ml_training_result {
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
    char machine_guid[GUID_LEN + 1];
    STRING *chart_id;
    STRING *dimension_id;

    // Creation time of request
    time_t request_time;

    // First/last entry of this dimension in DB
    // at the point the request was made
    time_t first_entry_on_request;
    time_t last_entry_on_request;
} ml_training_request_t;

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
    enum ml_training_result result;
} ml_training_response_t;

/*
 * Queue
*/

typedef struct {
    std::queue<ml_training_request_t> internal;
    netdata_mutex_t mutex;
    pthread_cond_t cond_var;
    std::atomic<bool> exit;
} ml_queue_t;

typedef struct {
    RRDDIM *rd;

    enum ml_metric_type mt;
    enum ml_training_status ts;
    enum ml_machine_learning_status mls;

    ml_training_response_t tr;
    time_t last_training_time;

    std::vector<calculated_number_t> cns;

    std::vector<ml_kmeans_t> km_contexts;
    netdata_mutex_t mutex;
    ml_kmeans_t kmeans;
    std::vector<DSample> feature;

    uint32_t suppression_window_counter;
    uint32_t suppression_anomaly_counter;
} ml_dimension_t;

typedef struct {
    RRDSET *rs;
    ml_machine_learning_stats_t mls;

    netdata_mutex_t mutex;
} ml_chart_t;

void ml_chart_update_dimension(ml_chart_t *chart, ml_dimension_t *dim, bool is_anomalous);

typedef struct {
    RRDHOST *rh;

    ml_machine_learning_stats_t mls;

    calculated_number_t host_anomaly_rate;

    netdata_mutex_t mutex;

    ml_queue_t *training_queue;

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
    RRDDIM *training_status_silenced_rd;

    RRDSET *dimensions_rs;
    RRDDIM *dimensions_anomalous_rd;
    RRDDIM *dimensions_normal_rd;

    RRDSET *anomaly_rate_rs;
    RRDDIM *anomaly_rate_rd;

    RRDSET *detector_events_rs;
    RRDDIM *detector_events_above_threshold_rd;
    RRDDIM *detector_events_new_anomaly_event_rd;
} ml_host_t;

typedef struct {
    uuid_t metric_uuid;
    ml_kmeans_t kmeans;
} ml_model_info_t;

typedef struct {
    size_t id;
    netdata_thread_t nd_thread;
    netdata_mutex_t nd_mutex;

    ml_queue_t *training_queue;
    ml_training_stats_t training_stats;

    calculated_number_t *training_cns;
    calculated_number_t *scratch_training_cns;
    std::vector<DSample> training_samples;

    std::vector<ml_model_info_t> pending_model_info;

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
} ml_training_thread_t;

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
    std::atomic<bool> detection_stop;

    size_t num_training_threads;
    size_t flush_models_batch_size;

    std::vector<ml_training_thread_t> training_threads;
    std::atomic<bool> training_stop;

    size_t suppression_window;
    size_t suppression_threshold;

    bool enable_statistics_charts;
} ml_config_t;

void ml_config_load(ml_config_t *cfg);

extern ml_config_t Cfg;

#endif /* NETDATA_ML_PRIVATE_H */
