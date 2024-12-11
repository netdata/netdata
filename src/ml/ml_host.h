// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_HOST_H
#define NETDATA_ML_HOST_H

#include "ml_calculated_number.h"

#include "database/rrd.h"

#include <atomic>
#include <unordered_map>

struct ml_queue_t;

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

typedef struct {
    RRDDIM *rd;
    size_t normal_dimensions;
    size_t anomalous_dimensions;
} ml_type_anomaly_rate_t;

typedef struct {
    RRDHOST *rh;

    std::atomic<bool> ml_running;

    ml_machine_learning_stats_t mls;

    calculated_number_t host_anomaly_rate;

    netdata_mutex_t mutex;

    ml_queue_t *queue;

    /*
     * bookkeeping for anomaly detection charts
    */

    RRDSET *ml_running_rs;
    RRDDIM *ml_running_rd;

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

    RRDSET *type_anomaly_rate_rs;
    SPINLOCK type_anomaly_rate_spinlock;
    std::unordered_map<STRING *, ml_type_anomaly_rate_t> type_anomaly_rate;
} ml_host_t;

#endif /* NETDATA_ML_HOST_H */
