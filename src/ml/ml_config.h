// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_CONFIG_H
#define ML_CONFIG_H

#include "ml_worker.h"

typedef struct {
    int enable_anomaly_detection;

    unsigned max_train_samples;
    unsigned min_train_samples;
    unsigned train_every;

    unsigned num_models_to_use;
    unsigned delete_models_older_than;

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

    ND_THREAD *detection_thread;
    std::atomic<bool> detection_stop;

    size_t num_worker_threads;
    size_t flush_models_batch_size;

    std::vector<ml_worker_t> workers;
    std::atomic<bool> training_stop;

    size_t suppression_window;
    size_t suppression_threshold;

    bool enable_statistics_charts;
} ml_config_t;

void ml_config_load(ml_config_t *cfg);

extern ml_config_t Cfg;

#endif /* ML_CONFIG_H */
