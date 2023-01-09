// SPDX-License-Identifier: GPL-3.0-or-later

#include "ADCharts.h"
#include "Config.h"

void ml::updateDimensionsChart(RRDHOST *RH, const MachineLearningStats &MLS) {
    /*
     * Machine learning status
    */
    {
        static thread_local RRDSET *MachineLearningStatusRS = nullptr;

        static thread_local RRDDIM *Enabled = nullptr;
        static thread_local RRDDIM *DisabledUE = nullptr;
        static thread_local RRDDIM *DisabledSP = nullptr;

        if (!MachineLearningStatusRS) {
            std::stringstream IdSS, NameSS;

            IdSS << "machine_learning_status_on_" << localhost->machine_guid;
            NameSS << "machine_learning_status_on_" << rrdhost_hostname(localhost);

            MachineLearningStatusRS = rrdset_create(
                RH,
                "netdata", // type
                IdSS.str().c_str(), // id
                NameSS.str().c_str(), // name
                "ml", // family
                "netdata.machine_learning_status", // ctx
                "Machine learning status", // title
                "dimensions", // units
                "netdata", // plugin
                "ml", // module
                NETDATA_ML_CHART_PRIO_MACHINE_LEARNING_STATUS, // priority
                RH->rrd_update_every, // update_every
                RRDSET_TYPE_LINE // chart_type
            );
            rrdset_flag_set(MachineLearningStatusRS , RRDSET_FLAG_ANOMALY_DETECTION);

            Enabled = rrddim_add(MachineLearningStatusRS, "enabled", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            DisabledUE = rrddim_add(MachineLearningStatusRS, "disabled-ue", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            DisabledSP = rrddim_add(MachineLearningStatusRS, "disabled-sp", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(MachineLearningStatusRS, Enabled, MLS.NumMachineLearningStatusEnabled);
        rrddim_set_by_pointer(MachineLearningStatusRS, DisabledUE, MLS.NumMachineLearningStatusDisabledUE);
        rrddim_set_by_pointer(MachineLearningStatusRS, DisabledSP, MLS.NumMachineLearningStatusDisabledSP);

        rrdset_done(MachineLearningStatusRS);
    }

    /*
     * Metric type
    */
    {
        static thread_local RRDSET *MetricTypesRS = nullptr;

        static thread_local RRDDIM *Constant = nullptr;
        static thread_local RRDDIM *Variable = nullptr;

        if (!MetricTypesRS) {
            std::stringstream IdSS, NameSS;

            IdSS << "metric_types_on_" << localhost->machine_guid;
            NameSS << "metric_types_on_" << rrdhost_hostname(localhost);

            MetricTypesRS = rrdset_create(
                RH,
                "netdata", // type
                IdSS.str().c_str(), // id
                NameSS.str().c_str(), // name
                "ml", // family
                "netdata.metric_types", // ctx
                "Dimensions by metric type", // title
                "dimensions", // units
                "netdata", // plugin
                "ml", // module
                NETDATA_ML_CHART_PRIO_METRIC_TYPES, // priority
                RH->rrd_update_every, // update_every
                RRDSET_TYPE_LINE // chart_type
            );
            rrdset_flag_set(MetricTypesRS, RRDSET_FLAG_ANOMALY_DETECTION);

            Constant = rrddim_add(MetricTypesRS, "constant", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            Variable = rrddim_add(MetricTypesRS, "variable", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(MetricTypesRS, Constant, MLS.NumMetricTypeConstant);
        rrddim_set_by_pointer(MetricTypesRS, Variable, MLS.NumMetricTypeVariable);

        rrdset_done(MetricTypesRS);
    }

    /*
     * Training status
    */
    {
        static thread_local RRDSET *TrainingStatusRS = nullptr;

        static thread_local RRDDIM *Untrained = nullptr;
        static thread_local RRDDIM *PendingWithoutModel = nullptr;
        static thread_local RRDDIM *Trained = nullptr;
        static thread_local RRDDIM *PendingWithModel = nullptr;

        if (!TrainingStatusRS) {
            std::stringstream IdSS, NameSS;

            IdSS << "training_status_on_" << localhost->machine_guid;
            NameSS << "training_status_on_" << rrdhost_hostname(localhost);

            TrainingStatusRS = rrdset_create(
                RH,
                "netdata", // type
                IdSS.str().c_str(), // id
                NameSS.str().c_str(), // name
                "ml", // family
                "netdata.training_status", // ctx
                "Training status of dimensions", // title
                "dimensions", // units
                "netdata", // plugin
                "ml", // module
                NETDATA_ML_CHART_PRIO_TRAINING_STATUS, // priority
                RH->rrd_update_every, // update_every
                RRDSET_TYPE_LINE // chart_type
            );

            rrdset_flag_set(TrainingStatusRS, RRDSET_FLAG_ANOMALY_DETECTION);

            Untrained = rrddim_add(TrainingStatusRS, "untrained", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            PendingWithoutModel = rrddim_add(TrainingStatusRS, "pending-without-model", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            Trained = rrddim_add(TrainingStatusRS, "trained", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            PendingWithModel = rrddim_add(TrainingStatusRS, "pending-with-model", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(TrainingStatusRS, Untrained, MLS.NumTrainingStatusUntrained);
        rrddim_set_by_pointer(TrainingStatusRS, PendingWithoutModel, MLS.NumTrainingStatusPendingWithoutModel);
        rrddim_set_by_pointer(TrainingStatusRS, Trained, MLS.NumTrainingStatusTrained);
        rrddim_set_by_pointer(TrainingStatusRS, PendingWithModel, MLS.NumTrainingStatusPendingWithModel);

        rrdset_done(TrainingStatusRS);
    }

    /*
     * Prediction status
    */
    {
        static thread_local RRDSET *PredictionRS = nullptr;

        static thread_local RRDDIM *Anomalous = nullptr;
        static thread_local RRDDIM *Normal = nullptr;

        if (!PredictionRS) {
            std::stringstream IdSS, NameSS;

            IdSS << "dimensions_on_" << localhost->machine_guid;
            NameSS << "dimensions_on_" << rrdhost_hostname(localhost);

            PredictionRS = rrdset_create(
                RH,
                "anomaly_detection", // type
                IdSS.str().c_str(), // id
                NameSS.str().c_str(), // name
                "dimensions", // family
                "anomaly_detection.dimensions", // ctx
                "Anomaly detection dimensions", // title
                "dimensions", // units
                "netdata", // plugin
                "ml", // module
                ML_CHART_PRIO_DIMENSIONS, // priority
                RH->rrd_update_every, // update_every
                RRDSET_TYPE_LINE // chart_type
            );
            rrdset_flag_set(PredictionRS, RRDSET_FLAG_ANOMALY_DETECTION);

            Anomalous = rrddim_add(PredictionRS, "anomalous", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            Normal = rrddim_add(PredictionRS, "normal", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(PredictionRS, Anomalous, MLS.NumAnomalousDimensions);
        rrddim_set_by_pointer(PredictionRS, Normal, MLS.NumNormalDimensions);

        rrdset_done(PredictionRS);
    }

}

void ml::updateHostAndDetectionRateCharts(RRDHOST *RH, collected_number AnomalyRate) {
    static thread_local RRDSET *HostRateRS = nullptr;
    static thread_local RRDDIM *AnomalyRateRD = nullptr;

    if (!HostRateRS) {
        std::stringstream IdSS, NameSS;

        IdSS << "anomaly_rate_on_" << localhost->machine_guid;
        NameSS << "anomaly_rate_on_" << rrdhost_hostname(localhost);

        HostRateRS = rrdset_create(
            RH,
            "anomaly_detection", // type
            IdSS.str().c_str(), // id
            NameSS.str().c_str(), // name
            "anomaly_rate", // family
            "anomaly_detection.anomaly_rate", // ctx
            "Percentage of anomalous dimensions", // title
            "percentage", // units
            "netdata", // plugin
            "ml", // module
            ML_CHART_PRIO_ANOMALY_RATE, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );
        rrdset_flag_set(HostRateRS, RRDSET_FLAG_ANOMALY_DETECTION);

        AnomalyRateRD = rrddim_add(HostRateRS, "anomaly_rate", NULL,
                1, 100, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(HostRateRS, AnomalyRateRD, AnomalyRate);
    rrdset_done(HostRateRS);

    static thread_local RRDSET *AnomalyDetectionRS = nullptr;
    static thread_local RRDDIM *AboveThresholdRD = nullptr;
    static thread_local RRDDIM *NewAnomalyEventRD = nullptr;

    if (!AnomalyDetectionRS) {
        std::stringstream IdSS, NameSS;

        IdSS << "anomaly_detection_on_" << localhost->machine_guid;
        NameSS << "anomaly_detection_on_" << rrdhost_hostname(localhost);

        AnomalyDetectionRS = rrdset_create(
            RH,
            "anomaly_detection", // type
            IdSS.str().c_str(), // id
            NameSS.str().c_str(), // name
            "anomaly_detection", // family
            "anomaly_detection.detector_events", // ctx
            "Anomaly detection events", // title
            "percentage", // units
            "netdata", // plugin
            "ml", // module
            ML_CHART_PRIO_DETECTOR_EVENTS, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );
        rrdset_flag_set(AnomalyDetectionRS, RRDSET_FLAG_ANOMALY_DETECTION);

        AboveThresholdRD  = rrddim_add(AnomalyDetectionRS, "above_threshold", NULL,
                                       1, 1, RRD_ALGORITHM_ABSOLUTE);
        NewAnomalyEventRD = rrddim_add(AnomalyDetectionRS, "new_anomaly_event", NULL,
                                       1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    /*
     * Compute the values of the dimensions based on the host rate chart
    */
    ONEWAYALLOC *OWA = onewayalloc_create(0);
    time_t Now = now_realtime_sec();
    time_t Before = Now - RH->rrd_update_every;
    time_t After = Before - Cfg.AnomalyDetectionQueryDuration;
    RRDR_OPTIONS Options = static_cast<RRDR_OPTIONS>(0x00000000);

    RRDR *R = rrd2rrdr_legacy(
            OWA, HostRateRS,
            1 /* points wanted */,
            After,
            Before,
            Cfg.AnomalyDetectionGroupingMethod,
            0 /* resampling time */,
            Options, "anomaly_rate",
            NULL /* group options */,
            0, /* timeout */
            0, /* tier */
            QUERY_SOURCE_ML
    );

    if(R) {
        assert(R->d == 1 && R->n == 1 && R->rows == 1);

        static thread_local bool PrevAboveThreshold = false;
        bool AboveThreshold = R->v[0] >= Cfg.HostAnomalyRateThreshold;
        bool NewAnomalyEvent = AboveThreshold && !PrevAboveThreshold;
        PrevAboveThreshold = AboveThreshold;

        rrddim_set_by_pointer(AnomalyDetectionRS, AboveThresholdRD, AboveThreshold);
        rrddim_set_by_pointer(AnomalyDetectionRS, NewAnomalyEventRD, NewAnomalyEvent);
        rrdset_done(AnomalyDetectionRS);

        rrdr_free(OWA, R);
    }

    onewayalloc_destroy(OWA);
}

void ml::updateResourceUsageCharts(RRDHOST *RH, const struct rusage &PredictionRU, const struct rusage &TrainingRU) {
    /*
     * prediction rusage
    */
    {
        static thread_local RRDSET *RS = nullptr;

        static thread_local RRDDIM *User = nullptr;
        static thread_local RRDDIM *System = nullptr;

        if (!RS) {
            std::stringstream IdSS, NameSS;

            IdSS << "prediction_usage_for_" << RH->machine_guid;
            NameSS << "prediction_usage_for_" << rrdhost_hostname(RH);

            RS = rrdset_create_localhost(
                "netdata", // type
                IdSS.str().c_str(), // id
                NameSS.str().c_str(), // name
                "ml", // family
                "netdata.prediction_usage", // ctx
                "Prediction resource usage", // title
                "milliseconds/s", // units
                "netdata", // plugin
                "ml", // module
                NETDATA_ML_CHART_PRIO_PREDICTION_USAGE, // priority
                RH->rrd_update_every, // update_every
                RRDSET_TYPE_STACKED // chart_type
            );
            rrdset_flag_set(RS, RRDSET_FLAG_ANOMALY_DETECTION);

            User = rrddim_add(RS, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
            System = rrddim_add(RS, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(RS, User, PredictionRU.ru_utime.tv_sec * 1000000ULL + PredictionRU.ru_utime.tv_usec);
        rrddim_set_by_pointer(RS, System, PredictionRU.ru_stime.tv_sec * 1000000ULL + PredictionRU.ru_stime.tv_usec);

        rrdset_done(RS);
    }

    /*
     * training rusage
    */
    {
        static thread_local RRDSET *RS = nullptr;

        static thread_local RRDDIM *User = nullptr;
        static thread_local RRDDIM *System = nullptr;

        if (!RS) {
            std::stringstream IdSS, NameSS;

            IdSS << "training_usage_for_" << RH->machine_guid;
            NameSS << "training_usage_for_" << rrdhost_hostname(RH);

            RS = rrdset_create_localhost(
                "netdata", // type
                IdSS.str().c_str(), // id
                NameSS.str().c_str(), // name
                "ml", // family
                "netdata.training_usage", // ctx
                "Training resource usage", // title
                "milliseconds/s", // units
                "netdata", // plugin
                "ml", // module
                NETDATA_ML_CHART_PRIO_TRAINING_USAGE, // priority
                RH->rrd_update_every, // update_every
                RRDSET_TYPE_STACKED // chart_type
            );
            rrdset_flag_set(RS, RRDSET_FLAG_ANOMALY_DETECTION);

            User = rrddim_add(RS, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
            System = rrddim_add(RS, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(RS, User, TrainingRU.ru_utime.tv_sec * 1000000ULL + TrainingRU.ru_utime.tv_usec);
        rrddim_set_by_pointer(RS, System, TrainingRU.ru_stime.tv_sec * 1000000ULL + TrainingRU.ru_stime.tv_usec);

        rrdset_done(RS);
    }
}

void ml::updateTrainingStatisticsChart(RRDHOST *RH, const TrainingStats &TS) {
    /*
     * queue stats
    */
    {
        static thread_local RRDSET *RS = nullptr;

        static thread_local RRDDIM *QueueSize = nullptr;
        static thread_local RRDDIM *PoppedItems = nullptr;

        if (!RS) {
            std::stringstream IdSS, NameSS;

            IdSS << "queue_stats_on_" << localhost->machine_guid;
            NameSS << "queue_stats_on_" << rrdhost_hostname(localhost);

            RS = rrdset_create(
                RH,
                "netdata", // type
                IdSS.str().c_str(), // id
                NameSS.str().c_str(), // name
                "ml", // family
                "netdata.queue_stats", // ctx
                "Training queue stats", // title
                "items", // units
                "netdata", // plugin
                "ml", // module
                NETDATA_ML_CHART_PRIO_QUEUE_STATS, // priority
                RH->rrd_update_every, // update_every
                RRDSET_TYPE_LINE// chart_type
            );
            rrdset_flag_set(RS, RRDSET_FLAG_ANOMALY_DETECTION);

            QueueSize = rrddim_add(RS, "queue_size", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            PoppedItems = rrddim_add(RS, "popped_items", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(RS, QueueSize, TS.QueueSize);
        rrddim_set_by_pointer(RS, PoppedItems, TS.NumPoppedItems);

        rrdset_done(RS);
    }

    /*
     * training stats
    */
    {
        static thread_local RRDSET *RS = nullptr;

        static thread_local RRDDIM *Allotted = nullptr;
        static thread_local RRDDIM *Consumed = nullptr;
        static thread_local RRDDIM *Remaining = nullptr;

        if (!RS) {
            std::stringstream IdSS, NameSS;

            IdSS << "training_time_stats_on_" << localhost->machine_guid;
            NameSS << "training_time_stats_on_" << rrdhost_hostname(localhost);

            RS = rrdset_create(
                RH,
                "netdata", // type
                IdSS.str().c_str(), // id
                NameSS.str().c_str(), // name
                "ml", // family
                "netdata.training_time_stats", // ctx
                "Training time stats", // title
                "milliseconds", // units
                "netdata", // plugin
                "ml", // module
                NETDATA_ML_CHART_PRIO_TRAINING_TIME_STATS, // priority
                RH->rrd_update_every, // update_every
                RRDSET_TYPE_LINE// chart_type
            );
            rrdset_flag_set(RS, RRDSET_FLAG_ANOMALY_DETECTION);

            Allotted = rrddim_add(RS, "allotted", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
            Consumed = rrddim_add(RS, "consumed", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
            Remaining = rrddim_add(RS, "remaining", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(RS, Allotted, TS.AllottedUT);
        rrddim_set_by_pointer(RS, Consumed, TS.ConsumedUT);
        rrddim_set_by_pointer(RS, Remaining, TS.RemainingUT);

        rrdset_done(RS);
    }

    /*
     * training result stats
    */
    {
        static thread_local RRDSET *RS = nullptr;

        static thread_local RRDDIM *Ok  = nullptr;
        static thread_local RRDDIM *InvalidQueryTimeRange = nullptr;
        static thread_local RRDDIM *NotEnoughCollectedValues = nullptr;
        static thread_local RRDDIM *NullAcquiredDimension = nullptr;
        static thread_local RRDDIM *ChartUnderReplication = nullptr;

        if (!RS) {
            std::stringstream IdSS, NameSS;

            IdSS << "training_results_on_" << localhost->machine_guid;
            NameSS << "training_results_on_" << rrdhost_hostname(localhost);

            RS = rrdset_create(
                RH,
                "netdata", // type
                IdSS.str().c_str(), // id
                NameSS.str().c_str(), // name
                "ml", // family
                "netdata.training_results", // ctx
                "Training results", // title
                "events", // units
                "netdata", // plugin
                "ml", // module
                NETDATA_ML_CHART_PRIO_TRAINING_RESULTS, // priority
                RH->rrd_update_every, // update_every
                RRDSET_TYPE_LINE// chart_type
            );
            rrdset_flag_set(RS, RRDSET_FLAG_ANOMALY_DETECTION);

            Ok = rrddim_add(RS, "ok", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            InvalidQueryTimeRange = rrddim_add(RS, "invalid-queries", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            NotEnoughCollectedValues = rrddim_add(RS, "not-enough-values", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            NullAcquiredDimension = rrddim_add(RS, "null-acquired-dimensions", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ChartUnderReplication = rrddim_add(RS, "chart-under-replication", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(RS, Ok, TS.TrainingResultOk);
        rrddim_set_by_pointer(RS, InvalidQueryTimeRange, TS.TrainingResultInvalidQueryTimeRange);
        rrddim_set_by_pointer(RS, NotEnoughCollectedValues, TS.TrainingResultNotEnoughCollectedValues);
        rrddim_set_by_pointer(RS, NullAcquiredDimension, TS.TrainingResultNullAcquiredDimension);
        rrddim_set_by_pointer(RS, ChartUnderReplication, TS.TrainingResultChartUnderReplication);

        rrdset_done(RS);
    }
}
