// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_WORKER_H
#define ML_WORKER_H

#include "ml_queue.h"

typedef struct {
    nd_uuid_t metric_uuid;
    ml_kmeans_inlined_t inlined_kmeans;
} ml_model_info_t;

typedef struct {
    size_t id;
    ND_THREAD *nd_thread;
    netdata_mutex_t nd_mutex;

    ml_queue_t *queue;
    ml_queue_stats_t queue_stats;

    calculated_number_t *training_cns;
    calculated_number_t *scratch_training_cns;
    std::vector<DSample> training_samples;

    std::vector<ml_model_info_t> pending_model_info;

    // Reusable buffers for streaming kmeans models
    BUFFER *stream_payload_buffer;
    BUFFER *stream_wb_buffer;

    RRDSET *queue_stats_rs;
    RRDDIM *queue_stats_num_create_new_model_requests_rd;
    RRDDIM *queue_stats_num_add_existing_model_requests_rd;
    RRDDIM *queue_stats_num_create_new_model_requests_completed_rd;
    RRDDIM *queue_stats_num_add_existing_model_requests_completed_rd;

    RRDSET *queue_size_rs;
    RRDDIM *queue_size_rd;

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

    size_t num_db_transactions;
    size_t num_models_to_prune;
} ml_worker_t;

#endif /* ML_WORKER_H */
