// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_WORKER_H
#define ML_WORKER_H

#include "ml_queue.h"

typedef struct {
    nd_uuid_t metric_uuid;
    ml_kmeans_inlined_t inlined_kmeans;
} ml_model_info_t;

// What one training thread did, cumulative; read by the detection thread under nd_mutex for the per-thread charts.
typedef struct {
    usec_t allotted_ut;
    usec_t consumed_ut;
    usec_t remaining_ut;

    // resolving and sorting the passes this thread sorted
    usec_t pass_resolve_ut;
    usec_t pass_sort_ut;

    size_t item_result_ok;
    size_t item_result_invalid_query_time_range;
    size_t item_result_not_enough_collected_values;
    size_t item_result_null_acquired_dimension;
    size_t item_result_chart_under_replication;
} ml_worker_stats_t;

typedef struct {
    size_t id;
    ND_THREAD *nd_thread;
    netdata_mutex_t nd_mutex;

    // the queue-level values (push/pop totals, last pass) are read from Cfg.training_queue instead
    ml_worker_stats_t training_stats;

    calculated_number_t *training_cns;
    calculated_number_t *scratch_training_cns;
    std::vector<DSample> training_samples;

    // the batch of Cfg.pending_models this thread is writing to ml.db (see ml_flush_pending_models())
    std::vector<ml_model_info_t> pending_model_info;

    // Reusable buffers for streaming kmeans models
    BUFFER *stream_payload_buffer;
    BUFFER *stream_wb_buffer;

    RRDSET *training_time_stats_rs;
    RRDDIM *training_time_stats_allotted_rd;
    RRDDIM *training_time_stats_consumed_rd;
    RRDDIM *training_time_stats_remaining_rd;
    RRDDIM *training_time_stats_pass_resolve_rd;
    RRDDIM *training_time_stats_pass_sort_rd;

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
