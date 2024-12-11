// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml_enums.h"

const char *
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

const char *
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

const char *
ml_training_status_to_string(enum ml_training_status ts)
{
    switch (ts) {
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

const char *
ml_worker_result_to_string(enum ml_worker_result tr)
{
    switch (tr) {
        case ML_WORKER_RESULT_OK:
            return "ok";
        case ML_WORKER_RESULT_INVALID_QUERY_TIME_RANGE:
            return "invalid-query";
        case ML_WORKER_RESULT_NOT_ENOUGH_COLLECTED_VALUES:
            return "missing-values";
        case ML_WORKER_RESULT_NULL_ACQUIRED_DIMENSION:
            return "null-acquired-dim";
        case ML_WORKER_RESULT_CHART_UNDER_REPLICATION:
            return "chart-under-replication";
        default:
            return "unknown";
    }
}

const char *
ml_queue_item_type_to_string(enum ml_queue_item_type qit)
{
    switch (qit) {
        case ML_QUEUE_ITEM_TYPE_CREATE_NEW_MODEL:
            return "create-new-model";
        case ML_QUEUE_ITEM_TYPE_ADD_EXISTING_MODEL:
            return "add-existing-model";
        case ML_QUEUE_ITEM_STOP_REQUEST:
            return "stop-request";
        default:
            return "unknown";
    }
}
