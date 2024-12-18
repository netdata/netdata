// SPDX-License-Identifier: GPL-3.0-or-later

#include "ad_charts.h"
#include "ml_config.h"

void ml_update_dimensions_chart(ml_host_t *host, const ml_machine_learning_stats_t &mls) {
    /*
     * Machine learning status
    */
    if (Cfg.enable_statistics_charts) {
        if (!host->machine_learning_status_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "machine_learning_status_on_%s", localhost->machine_guid);
            snprintfz(name_buf, 1024, "machine_learning_status_on_%s", rrdhost_hostname(localhost));

            host->machine_learning_status_rs = rrdset_create(
                    host->rh,
                    "netdata", // type
                    id_buf,
                    name_buf, // name
                    NETDATA_ML_CHART_FAMILY, // family
                    "netdata.ml_status", // ctx
                    "Machine learning status", // title
                    "dimensions", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_TRAINING, // module
                    NETDATA_ML_CHART_PRIO_MACHINE_LEARNING_STATUS, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE // chart_type
            );
            rrdset_flag_set(host->machine_learning_status_rs , RRDSET_FLAG_ANOMALY_DETECTION);

            host->machine_learning_status_enabled_rd =
                rrddim_add(host->machine_learning_status_rs, "enabled", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            host->machine_learning_status_disabled_sp_rd =
                rrddim_add(host->machine_learning_status_rs, "disabled-sp", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(host->machine_learning_status_rs,
                              host->machine_learning_status_enabled_rd, mls.num_machine_learning_status_enabled);
        rrddim_set_by_pointer(host->machine_learning_status_rs,
                              host->machine_learning_status_disabled_sp_rd, mls.num_machine_learning_status_disabled_sp);

        rrdset_done(host->machine_learning_status_rs);
    }

    /*
     * Metric type
    */
    if (Cfg.enable_statistics_charts) {
        if (!host->metric_type_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "metric_types_on_%s", localhost->machine_guid);
            snprintfz(name_buf, 1024, "metric_types_on_%s", rrdhost_hostname(localhost));

            host->metric_type_rs = rrdset_create(
                    host->rh,
                    "netdata", // type
                    id_buf, // id
                    name_buf, // name
                    NETDATA_ML_CHART_FAMILY, // family
                    "netdata.ml_metric_types", // ctx
                    "Dimensions by metric type", // title
                    "dimensions", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_TRAINING, // module
                    NETDATA_ML_CHART_PRIO_METRIC_TYPES, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE // chart_type
            );
            rrdset_flag_set(host->metric_type_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            host->metric_type_constant_rd =
                rrddim_add(host->metric_type_rs, "constant", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            host->metric_type_variable_rd =
                rrddim_add(host->metric_type_rs, "variable", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(host->metric_type_rs,
                              host->metric_type_constant_rd, mls.num_metric_type_constant);
        rrddim_set_by_pointer(host->metric_type_rs,
                              host->metric_type_variable_rd, mls.num_metric_type_variable);

        rrdset_done(host->metric_type_rs);
    }

    /*
     * Training status
    */
    if (Cfg.enable_statistics_charts) {
        if (!host->training_status_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "training_status_on_%s", localhost->machine_guid);
            snprintfz(name_buf, 1024, "training_status_on_%s", rrdhost_hostname(localhost));

            host->training_status_rs = rrdset_create(
                    host->rh,
                    "netdata", // type
                    id_buf, // id
                    name_buf, // name
                    NETDATA_ML_CHART_FAMILY, // family
                    "netdata.ml_training_status", // ctx
                    "Training status of dimensions", // title
                    "dimensions", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_TRAINING, // module
                    NETDATA_ML_CHART_PRIO_TRAINING_STATUS, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE // chart_type
            );

            rrdset_flag_set(host->training_status_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            host->training_status_untrained_rd =
                rrddim_add(host->training_status_rs, "untrained", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            host->training_status_pending_without_model_rd =
                rrddim_add(host->training_status_rs, "pending-without-model", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            host->training_status_trained_rd =
                rrddim_add(host->training_status_rs, "trained", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            host->training_status_pending_with_model_rd =
                rrddim_add(host->training_status_rs, "pending-with-model", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            host->training_status_silenced_rd =
                rrddim_add(host->training_status_rs, "silenced", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(host->training_status_rs,
                              host->training_status_untrained_rd, mls.num_training_status_untrained);
        rrddim_set_by_pointer(host->training_status_rs,
                              host->training_status_pending_without_model_rd, mls.num_training_status_pending_without_model);
        rrddim_set_by_pointer(host->training_status_rs,
                              host->training_status_trained_rd, mls.num_training_status_trained);
        rrddim_set_by_pointer(host->training_status_rs,
                              host->training_status_pending_with_model_rd, mls.num_training_status_pending_with_model);
        rrddim_set_by_pointer(host->training_status_rs,
                              host->training_status_silenced_rd, mls.num_training_status_silenced);

        rrdset_done(host->training_status_rs);
    }

    /*
     * Prediction status
    */
    {
        if (!host->dimensions_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "dimensions_on_%s", localhost->machine_guid);
            snprintfz(name_buf, 1024, "dimensions_on_%s", rrdhost_hostname(localhost));

            host->dimensions_rs = rrdset_create(
                    host->rh,
                    "anomaly_detection", // type
                    id_buf, // id
                    name_buf, // name
                    "dimensions", // family
                    "anomaly_detection.dimensions", // ctx
                    "Anomaly detection dimensions", // title
                    "dimensions", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_TRAINING, // module
                    ML_CHART_PRIO_DIMENSIONS, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE // chart_type
            );
            rrdset_flag_set(host->dimensions_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            host->dimensions_anomalous_rd =
                rrddim_add(host->dimensions_rs, "anomalous", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            host->dimensions_normal_rd =
                rrddim_add(host->dimensions_rs, "normal", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(host->dimensions_rs,
                              host->dimensions_anomalous_rd, mls.num_anomalous_dimensions);
        rrddim_set_by_pointer(host->dimensions_rs,
                              host->dimensions_normal_rd, mls.num_normal_dimensions);

        rrdset_done(host->dimensions_rs);
    }

    // ML running
    {
        if (!host->ml_running_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "ml_running_on_%s", localhost->machine_guid);
            snprintfz(name_buf, 1024, "ml_running_on_%s", rrdhost_hostname(localhost));

            host->ml_running_rs = rrdset_create(
                    host->rh,
                    "anomaly_detection", // type
                    id_buf, // id
                    name_buf, // name
                    "anomaly_detection", // family
                    "anomaly_detection.ml_running", // ctx
                    "ML running", // title
                    "boolean", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_DETECTION, // module
                    NETDATA_ML_CHART_RUNNING, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE // chart_type
            );
            rrdset_flag_set(host->ml_running_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            host->ml_running_rd =
                rrddim_add(host->ml_running_rs, "ml_running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(host->ml_running_rs,
                              host->ml_running_rd, host->ml_running);
        rrdset_done(host->ml_running_rs);
    }
}

void ml_update_host_and_detection_rate_charts(ml_host_t *host, collected_number AnomalyRate) {
    /*
     * Host anomaly rate
    */
    {
        if (!host->anomaly_rate_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "anomaly_rate_on_%s", localhost->machine_guid);
            snprintfz(name_buf, 1024, "anomaly_rate_on_%s", rrdhost_hostname(localhost));

            host->anomaly_rate_rs = rrdset_create(
                    host->rh,
                    "anomaly_detection", // type
                    id_buf, // id
                    name_buf, // name
                    "anomaly_rate", // family
                    "anomaly_detection.anomaly_rate", // ctx
                    "Percentage of anomalous dimensions", // title
                    "percentage", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_DETECTION, // module
                    ML_CHART_PRIO_ANOMALY_RATE, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE // chart_type
            );
            rrdset_flag_set(host->anomaly_rate_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            host->anomaly_rate_rd =
                rrddim_add(host->anomaly_rate_rs, "anomaly_rate", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(host->anomaly_rate_rs, host->anomaly_rate_rd, AnomalyRate);

        rrdset_done(host->anomaly_rate_rs);
    }

    /*
     * Type anomaly rate
    */
    {
        if (!host->type_anomaly_rate_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "type_anomaly_rate_on_%s", localhost->machine_guid);
            snprintfz(name_buf, 1024, "type_anomaly_rate_on_%s", rrdhost_hostname(localhost));

            host->type_anomaly_rate_rs = rrdset_create(
                    host->rh,
                    "anomaly_detection", // type
                    id_buf, // id
                    name_buf, // name
                    "anomaly_rate", // family
                    "anomaly_detection.type_anomaly_rate", // ctx
                    "Percentage of anomalous dimensions by type", // title
                    "percentage", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_DETECTION, // module
                    ML_CHART_PRIO_TYPE_ANOMALY_RATE, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_STACKED // chart_type
            );

            rrdset_flag_set(host->type_anomaly_rate_rs, RRDSET_FLAG_ANOMALY_DETECTION);
        }

        spinlock_lock(&host->type_anomaly_rate_spinlock);
        for (auto &entry : host->type_anomaly_rate) {
            ml_type_anomaly_rate_t &type_anomaly_rate = entry.second;

            if (!type_anomaly_rate.rd)
                type_anomaly_rate.rd = rrddim_add(host->type_anomaly_rate_rs, string2str(entry.first), NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);

            double ar = 0.0;
            size_t n = type_anomaly_rate.anomalous_dimensions + type_anomaly_rate.normal_dimensions;
            if (n)
                ar = static_cast<double>(type_anomaly_rate.anomalous_dimensions) / n;

            rrddim_set_by_pointer(host->type_anomaly_rate_rs, type_anomaly_rate.rd, ar * 10000.0);

            type_anomaly_rate.anomalous_dimensions = 0;
            type_anomaly_rate.normal_dimensions = 0;
        }
        spinlock_unlock(&host->type_anomaly_rate_spinlock);

        rrdset_done(host->type_anomaly_rate_rs);
    }

    /*
     * Detector Events
    */
    {
        if (!host->detector_events_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "anomaly_detection_on_%s", localhost->machine_guid);
            snprintfz(name_buf, 1024, "anomaly_detection_on_%s", rrdhost_hostname(localhost));

            host->detector_events_rs = rrdset_create(
                    host->rh,
                    "anomaly_detection", // type
                    id_buf, // id
                    name_buf, // name
                    "anomaly_detection", // family
                    "anomaly_detection.detector_events", // ctx
                    "Anomaly detection events", // title
                    "status", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_DETECTION, // module
                    ML_CHART_PRIO_DETECTOR_EVENTS, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE // chart_type
            );
            rrdset_flag_set(host->detector_events_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            host->detector_events_above_threshold_rd =
                rrddim_add(host->detector_events_rs, "above_threshold", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            host->detector_events_new_anomaly_event_rd =
                rrddim_add(host->detector_events_rs, "new_anomaly_event", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        /*
         * Compute the values of the dimensions based on the host rate chart
        */
        if (host->ml_running) {
            ONEWAYALLOC *OWA = onewayalloc_create(0);
            time_t Now = now_realtime_sec();
            time_t Before = Now - host->rh->rrd_update_every;
            time_t After = Before - Cfg.anomaly_detection_query_duration;
            RRDR_OPTIONS Options = static_cast<RRDR_OPTIONS>(0x00000000);

            RRDR *R = rrd2rrdr_legacy(
                    OWA,
                    host->anomaly_rate_rs,
                    1 /* points wanted */,
                    After,
                    Before,
                    Cfg.anomaly_detection_grouping_method,
                    0 /* resampling time */,
                    Options, "anomaly_rate",
                    NULL /* group options */,
                    0, /* timeout */
                    0, /* tier */
                    QUERY_SOURCE_ML,
                    STORAGE_PRIORITY_SYNCHRONOUS
            );

            if (R) {
                if (R->d == 1 && R->n == 1 && R->rows == 1) {
                    static thread_local bool prev_above_threshold = false;
                    bool above_threshold = R->v[0] >= Cfg.host_anomaly_rate_threshold;
                    bool new_anomaly_event = above_threshold && !prev_above_threshold;
                    prev_above_threshold = above_threshold;

                    rrddim_set_by_pointer(host->detector_events_rs,
                                          host->detector_events_above_threshold_rd, above_threshold);
                    rrddim_set_by_pointer(host->detector_events_rs,
                                          host->detector_events_new_anomaly_event_rd, new_anomaly_event);

                    rrdset_done(host->detector_events_rs);
                }

                rrdr_free(OWA, R);
            }

            onewayalloc_destroy(OWA);
        } else {
            rrddim_set_by_pointer(host->detector_events_rs,
                                  host->detector_events_above_threshold_rd, 0);
            rrddim_set_by_pointer(host->detector_events_rs,
                                  host->detector_events_new_anomaly_event_rd, 0);
            rrdset_done(host->detector_events_rs);
        }
    }
}

void ml_update_training_statistics_chart(ml_worker_t *worker, const ml_queue_stats_t &stats) {
    /*
     * queue stats
    */
    {
        if (!worker->queue_stats_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "training_queue_%zu_ops", worker->id);
            snprintfz(name_buf, 1024, "training_queue_%zu_ops", worker->id);

            worker->queue_stats_rs = rrdset_create(
                    localhost,
                    "netdata", // type
                    id_buf, // id
                    name_buf, // name
                    NETDATA_ML_CHART_FAMILY, // family
                    "netdata.ml_queue_ops", // ctx
                    "Training queue operations", // title
                    "count", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_TRAINING, // module
                    NETDATA_ML_CHART_PRIO_QUEUE_STATS, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE// chart_type
            );
            rrdset_flag_set(worker->queue_stats_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            worker->queue_stats_num_create_new_model_requests_rd =
                rrddim_add(worker->queue_stats_rs, "pushed create model", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            worker->queue_stats_num_create_new_model_requests_completed_rd =
                rrddim_add(worker->queue_stats_rs, "popped create model", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            worker->queue_stats_num_add_existing_model_requests_rd =
                rrddim_add(worker->queue_stats_rs, "pushed add model", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            worker->queue_stats_num_add_existing_model_requests_completed_rd =
                rrddim_add(worker->queue_stats_rs, "popped add models", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(worker->queue_stats_rs,
                              worker->queue_stats_num_create_new_model_requests_rd, stats.total_create_new_model_requests_pushed);
        rrddim_set_by_pointer(worker->queue_stats_rs,
                              worker->queue_stats_num_create_new_model_requests_completed_rd, stats.total_create_new_model_requests_popped);

        rrddim_set_by_pointer(worker->queue_stats_rs,
                              worker->queue_stats_num_add_existing_model_requests_rd, stats.total_add_existing_model_requests_pushed);
        rrddim_set_by_pointer(worker->queue_stats_rs,
                              worker->queue_stats_num_add_existing_model_requests_completed_rd, stats.total_add_existing_model_requests_popped);

        rrdset_done(worker->queue_stats_rs);
    }

    {
        if (!worker->queue_size_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "training_queue_%zu_size", worker->id);
            snprintfz(name_buf, 1024, "training_queue_%zu_size", worker->id);

            worker->queue_size_rs = rrdset_create(
                    localhost,
                    "netdata", // type
                    id_buf, // id
                    name_buf, // name
                    NETDATA_ML_CHART_FAMILY, // family
                    "netdata.ml_queue_size", // ctx
                    "Training queue size", // title
                    "count", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_TRAINING, // module
                    NETDATA_ML_CHART_PRIO_QUEUE_STATS, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE// chart_type
            );
            rrdset_flag_set(worker->queue_size_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            worker->queue_size_rd =
                rrddim_add(worker->queue_size_rs, "items", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        ml_queue_size_t qs = ml_queue_size(worker->queue);
        collected_number cn = qs.add_exisiting_model + qs.create_new_model;

        rrddim_set_by_pointer(worker->queue_size_rs, worker->queue_size_rd, cn);
        rrdset_done(worker->queue_size_rs);
    }

    /*
     * training stats
    */
    {
        if (!worker->training_time_stats_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "training_queue_%zu_time_stats", worker->id);
            snprintfz(name_buf, 1024, "training_queue_%zu_time_stats", worker->id);

            worker->training_time_stats_rs = rrdset_create(
                    localhost,
                    "netdata", // type
                    id_buf, // id
                    name_buf, // name
                    NETDATA_ML_CHART_FAMILY, // family
                    "netdata.ml_training_time_stats", // ctx
                    "Training time stats", // title
                    "microseconds", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_TRAINING, // module
                    NETDATA_ML_CHART_PRIO_TRAINING_TIME_STATS, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE// chart_type
            );
            rrdset_flag_set(worker->training_time_stats_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            worker->training_time_stats_allotted_rd =
                rrddim_add(worker->training_time_stats_rs, "allotted", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
            worker->training_time_stats_consumed_rd =
                rrddim_add(worker->training_time_stats_rs, "consumed", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
            worker->training_time_stats_remaining_rd =
                rrddim_add(worker->training_time_stats_rs, "remaining", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(worker->training_time_stats_rs,
                              worker->training_time_stats_allotted_rd, stats.allotted_ut);
        rrddim_set_by_pointer(worker->training_time_stats_rs,
                              worker->training_time_stats_consumed_rd, stats.consumed_ut);
        rrddim_set_by_pointer(worker->training_time_stats_rs,
                              worker->training_time_stats_remaining_rd, stats.remaining_ut);

        rrdset_done(worker->training_time_stats_rs);
    }

    /*
     * training result stats
    */
    {
        if (!worker->training_results_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "training_queue_%zu_results", worker->id);
            snprintfz(name_buf, 1024, "training_queue_%zu_results", worker->id);

            worker->training_results_rs = rrdset_create(
                    localhost,
                    "netdata", // type
                    id_buf, // id
                    name_buf, // name
                    NETDATA_ML_CHART_FAMILY, // family
                    "netdata.ml_training_results", // ctx
                    "Training results", // title
                    "events", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_TRAINING, // module
                    NETDATA_ML_CHART_PRIO_TRAINING_RESULTS, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE// chart_type
            );
            rrdset_flag_set(worker->training_results_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            worker->training_results_ok_rd =
                rrddim_add(worker->training_results_rs, "ok", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            worker->training_results_invalid_query_time_range_rd =
                rrddim_add(worker->training_results_rs, "invalid-queries", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            worker->training_results_not_enough_collected_values_rd =
                rrddim_add(worker->training_results_rs, "not-enough-values", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            worker->training_results_null_acquired_dimension_rd =
                rrddim_add(worker->training_results_rs, "null-acquired-dimensions", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            worker->training_results_chart_under_replication_rd =
                rrddim_add(worker->training_results_rs, "chart-under-replication", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(worker->training_results_rs,
                              worker->training_results_ok_rd, stats.item_result_ok);
        rrddim_set_by_pointer(worker->training_results_rs,
                              worker->training_results_invalid_query_time_range_rd, stats.item_result_invalid_query_time_range);
        rrddim_set_by_pointer(worker->training_results_rs,
                              worker->training_results_not_enough_collected_values_rd, stats.item_result_not_enough_collected_values);
        rrddim_set_by_pointer(worker->training_results_rs,
                              worker->training_results_null_acquired_dimension_rd, stats.item_result_null_acquired_dimension);
        rrddim_set_by_pointer(worker->training_results_rs,
                              worker->training_results_chart_under_replication_rd, stats.item_result_chart_under_replication);

        rrdset_done(worker->training_results_rs);
    }
}

void ml_update_global_statistics_charts(uint64_t models_consulted,
                                        uint64_t models_received,
                                        uint64_t models_sent,
                                        uint64_t models_ignored,
                                        uint64_t models_deserialization_failures,
                                        uint64_t memory_consumption,
                                        uint64_t memory_new,
                                        uint64_t memory_delete)
{
    if (!Cfg.enable_statistics_charts)
        return;

    {
        static RRDSET *st = NULL;
        static RRDDIM *rd = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost(
                    "netdata" // type
                    , "ml_models_consulted" // id
                    , NULL // name
                    , NETDATA_ML_CHART_FAMILY // family
                    , NULL // context
                    , "KMeans models used for prediction" // title
                    , "models" // units
                    , NETDATA_ML_PLUGIN // plugin
                    , NETDATA_ML_MODULE_DETECTION // module
                    , NETDATA_ML_CHART_PRIO_MACHINE_LEARNING_STATUS // priority
                    , localhost->rrd_update_every // update_every
                    , RRDSET_TYPE_AREA // chart_type
            );

            rd = rrddim_add(st, "num_models_consulted", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd, (collected_number) models_consulted);

        rrdset_done(st);
    }

    {
        static RRDSET *st = NULL;
        static RRDDIM *rd_received = NULL;
        static RRDDIM *rd_sent = NULL;
        static RRDDIM *rd_ignored = NULL;
        static RRDDIM *rd_deserialization_failures = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost(
                    "netdata" // type
                    , "ml_models_streamed" // id
                    , NULL // name
                    , NETDATA_ML_CHART_FAMILY // family
                    , NULL // context
                    , "KMeans models streamed" // title
                    , "models" // units
                    , NETDATA_ML_PLUGIN // plugin
                    , NETDATA_ML_MODULE_DETECTION // module
                    , NETDATA_ML_CHART_PRIO_MACHINE_LEARNING_STATUS // priority
                    , localhost->rrd_update_every // update_every
                    , RRDSET_TYPE_LINE // chart_type
            );

            rd_received = rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_sent = rrddim_add(st, "sent", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_ignored = rrddim_add(st, "ignored", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_deserialization_failures = rrddim_add(st, "deserialization failures", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_received, (collected_number) models_received);
        rrddim_set_by_pointer(st, rd_sent, (collected_number) models_sent);
        rrddim_set_by_pointer(st, rd_ignored, (collected_number) models_ignored);
        rrddim_set_by_pointer(st, rd_deserialization_failures, (collected_number) models_deserialization_failures);

        rrdset_done(st);
    }

    {
        static RRDSET *st = NULL;
        static RRDDIM *rd_memory_consumption = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost(
                    "netdata" // type
                    , "ml_memory_used" // id
                    , NULL // name
                    , NETDATA_ML_CHART_FAMILY // family
                    , NULL // context
                    , "ML memory usage" // title
                    , "bytes" // units
                    , NETDATA_ML_PLUGIN // plugin
                    , NETDATA_ML_MODULE_DETECTION // module
                    , NETDATA_ML_CHART_PRIO_MACHINE_LEARNING_STATUS // priority
                    , localhost->rrd_update_every // update_every
                    , RRDSET_TYPE_LINE // chart_type
            );

            rd_memory_consumption = rrddim_add(st, "used", NULL, 1024, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st, rd_memory_consumption, (collected_number) memory_consumption / (1024));
        rrdset_done(st);
    }

    {
        static RRDSET *st = NULL;
        static RRDDIM *rd_memory_new = NULL;
        static RRDDIM *rd_memory_delete = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost(
                    "netdata" // type
                    , "ml_memory_ops" // id
                    , NULL // name
                    , NETDATA_ML_CHART_FAMILY // family
                    , NULL // context
                    , "ML memory operations" // title
                    , "count" // units
                    , NETDATA_ML_PLUGIN // plugin
                    , NETDATA_ML_MODULE_DETECTION // module
                    , NETDATA_ML_CHART_PRIO_MACHINE_LEARNING_STATUS // priority
                    , localhost->rrd_update_every // update_every
                    , RRDSET_TYPE_LINE // chart_type
            );

            rd_memory_new = rrddim_add(st, "new", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_memory_delete = rrddim_add(st, "delete", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_memory_new, (collected_number) memory_new);
        rrddim_set_by_pointer(st, rd_memory_delete, (collected_number) memory_delete);
        rrdset_done(st);
    }
}
