// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_ENUMS_H
#define NETDATA_ML_ENUMS_H

enum ml_metric_type {
    // The dimension has constant values, no need to train
    METRIC_TYPE_CONSTANT,

    // The dimension's values fluctuate, we need to generate a model
    METRIC_TYPE_VARIABLE,
};

const char *ml_metric_type_to_string(enum ml_metric_type mt);

enum ml_machine_learning_status {
    // Enable training/prediction
    MACHINE_LEARNING_STATUS_ENABLED,

    // Disable because configuration pattern matches the chart's id
    MACHINE_LEARNING_STATUS_DISABLED_DUE_TO_EXCLUDED_CHART,
};

const char *ml_machine_learning_status_to_string(enum ml_machine_learning_status mls);

enum ml_training_status {
    // We don't have a model for this dimension
    TRAINING_STATUS_UNTRAINED,

    // Have a valid, up-to-date model
    TRAINING_STATUS_TRAINED,

    // Have a valid, up-to-date model that is silenced because its too noisy
    TRAINING_STATUS_SILENCED,
};

const char *ml_training_status_to_string(enum ml_training_status ts);

enum ml_worker_result {
    // We managed to create a KMeans model
    ML_WORKER_RESULT_OK,

    // Could not query DB with a correct time range
    ML_WORKER_RESULT_INVALID_QUERY_TIME_RANGE,

    // Did not gather enough data from DB to run KMeans
    ML_WORKER_RESULT_NOT_ENOUGH_COLLECTED_VALUES,

    // Acquired a null dimension
    ML_WORKER_RESULT_NULL_ACQUIRED_DIMENSION,

    // Chart is under replication
    ML_WORKER_RESULT_CHART_UNDER_REPLICATION,
};

const char *ml_worker_result_to_string(enum ml_worker_result tr);

enum ml_queue_item_type {
    ML_QUEUE_ITEM_TYPE_CREATE_NEW_MODEL,
    ML_QUEUE_ITEM_TYPE_ADD_EXISTING_MODEL,
    ML_QUEUE_ITEM_STOP_REQUEST,
};

const char *ml_queue_item_type_to_string(enum ml_queue_item_type qit);

#endif /* NETDATA_ML_ENUMS_H */
