// SPDX-License-Identifier: GPL-3.0-or-later

#include "ad_charts.h"

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
                    "netdata.machine_learning_status", // ctx
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
                    "netdata.metric_types", // ctx
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
                    "netdata.training_status", // ctx
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
}

void ml_update_host_and_detection_rate_charts(ml_host_t *host, collected_number AnomalyRate) {
    /*
     * Anomaly rate
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
                    "percentage", // units
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
    }
}

void ml_update_training_statistics_chart(ml_training_thread_t *training_thread, const ml_training_stats_t &ts) {
    /*
     * queue stats
    */
    {
        if (!training_thread->queue_stats_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "training_queue_%zu_stats", training_thread->id);
            snprintfz(name_buf, 1024, "training_queue_%zu_stats", training_thread->id);

            training_thread->queue_stats_rs = rrdset_create(
                    localhost,
                    "netdata", // type
                    id_buf, // id
                    name_buf, // name
                    NETDATA_ML_CHART_FAMILY, // family
                    "netdata.queue_stats", // ctx
                    "Training queue stats", // title
                    "items", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_TRAINING, // module
                    NETDATA_ML_CHART_PRIO_QUEUE_STATS, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE// chart_type
            );
            rrdset_flag_set(training_thread->queue_stats_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            training_thread->queue_stats_queue_size_rd =
                rrddim_add(training_thread->queue_stats_rs, "queue_size", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            training_thread->queue_stats_popped_items_rd =
                rrddim_add(training_thread->queue_stats_rs, "popped_items", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(training_thread->queue_stats_rs,
                              training_thread->queue_stats_queue_size_rd, ts.queue_size);
        rrddim_set_by_pointer(training_thread->queue_stats_rs,
                              training_thread->queue_stats_popped_items_rd, ts.num_popped_items);

        rrdset_done(training_thread->queue_stats_rs);
    }

    /*
     * training stats
    */
    {
        if (!training_thread->training_time_stats_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "training_queue_%zu_time_stats", training_thread->id);
            snprintfz(name_buf, 1024, "training_queue_%zu_time_stats", training_thread->id);

            training_thread->training_time_stats_rs = rrdset_create(
                    localhost,
                    "netdata", // type
                    id_buf, // id
                    name_buf, // name
                    NETDATA_ML_CHART_FAMILY, // family
                    "netdata.training_time_stats", // ctx
                    "Training time stats", // title
                    "milliseconds", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_TRAINING, // module
                    NETDATA_ML_CHART_PRIO_TRAINING_TIME_STATS, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE// chart_type
            );
            rrdset_flag_set(training_thread->training_time_stats_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            training_thread->training_time_stats_allotted_rd =
                rrddim_add(training_thread->training_time_stats_rs, "allotted", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
            training_thread->training_time_stats_consumed_rd =
                rrddim_add(training_thread->training_time_stats_rs, "consumed", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
            training_thread->training_time_stats_remaining_rd =
                rrddim_add(training_thread->training_time_stats_rs, "remaining", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(training_thread->training_time_stats_rs,
                              training_thread->training_time_stats_allotted_rd, ts.allotted_ut);
        rrddim_set_by_pointer(training_thread->training_time_stats_rs,
                              training_thread->training_time_stats_consumed_rd, ts.consumed_ut);
        rrddim_set_by_pointer(training_thread->training_time_stats_rs,
                              training_thread->training_time_stats_remaining_rd, ts.remaining_ut);

        rrdset_done(training_thread->training_time_stats_rs);
    }

    /*
     * training result stats
    */
    {
        if (!training_thread->training_results_rs) {
            char id_buf[1024];
            char name_buf[1024];

            snprintfz(id_buf, 1024, "training_queue_%zu_results", training_thread->id);
            snprintfz(name_buf, 1024, "training_queue_%zu_results", training_thread->id);

            training_thread->training_results_rs = rrdset_create(
                    localhost,
                    "netdata", // type
                    id_buf, // id
                    name_buf, // name
                    NETDATA_ML_CHART_FAMILY, // family
                    "netdata.training_results", // ctx
                    "Training results", // title
                    "events", // units
                    NETDATA_ML_PLUGIN, // plugin
                    NETDATA_ML_MODULE_TRAINING, // module
                    NETDATA_ML_CHART_PRIO_TRAINING_RESULTS, // priority
                    localhost->rrd_update_every, // update_every
                    RRDSET_TYPE_LINE// chart_type
            );
            rrdset_flag_set(training_thread->training_results_rs, RRDSET_FLAG_ANOMALY_DETECTION);

            training_thread->training_results_ok_rd =
                rrddim_add(training_thread->training_results_rs, "ok", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            training_thread->training_results_invalid_query_time_range_rd =
                rrddim_add(training_thread->training_results_rs, "invalid-queries", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            training_thread->training_results_not_enough_collected_values_rd =
                rrddim_add(training_thread->training_results_rs, "not-enough-values", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            training_thread->training_results_null_acquired_dimension_rd =
                rrddim_add(training_thread->training_results_rs, "null-acquired-dimensions", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            training_thread->training_results_chart_under_replication_rd =
                rrddim_add(training_thread->training_results_rs, "chart-under-replication", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(training_thread->training_results_rs,
                              training_thread->training_results_ok_rd, ts.training_result_ok);
        rrddim_set_by_pointer(training_thread->training_results_rs,
                              training_thread->training_results_invalid_query_time_range_rd, ts.training_result_invalid_query_time_range);
        rrddim_set_by_pointer(training_thread->training_results_rs,
                              training_thread->training_results_not_enough_collected_values_rd, ts.training_result_not_enough_collected_values);
        rrddim_set_by_pointer(training_thread->training_results_rs,
                              training_thread->training_results_null_acquired_dimension_rd, ts.training_result_null_acquired_dimension);
        rrddim_set_by_pointer(training_thread->training_results_rs,
                              training_thread->training_results_chart_under_replication_rd, ts.training_result_chart_under_replication);

        rrdset_done(training_thread->training_results_rs);
    }
}

void ml_update_global_statistics_charts(uint64_t models_consulted) {
    if (Cfg.enable_statistics_charts) {
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
}
