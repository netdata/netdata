// SPDX-License-Identifier: GPL-3.0-or-later

#include "ADCharts.h"
#include "Config.h"

void ml::updateDimensionsChart(RRDHOST *RH,
                               collected_number NumTrainedDimensions,
                               collected_number NumNormalDimensions,
                               collected_number NumAnomalousDimensions) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *NumTotalDimensionsRD = nullptr;
    static thread_local RRDDIM *NumTrainedDimensionsRD = nullptr;
    static thread_local RRDDIM *NumNormalDimensionsRD = nullptr;
    static thread_local RRDDIM *NumAnomalousDimensionsRD = nullptr;

    if (!RS) {
        std::stringstream IdSS, NameSS;

        IdSS << "dimensions_on_" << localhost->machine_guid;
        NameSS << "dimensions_on_" << localhost->hostname;

        RS = rrdset_create(
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
            39183, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );
        rrdset_flag_set(RS, RRDSET_FLAG_ANOMALY_DETECTION);

        NumTotalDimensionsRD = rrddim_add(RS, "total", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        NumTrainedDimensionsRD = rrddim_add(RS, "trained", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        NumNormalDimensionsRD = rrddim_add(RS, "normal", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        NumAnomalousDimensionsRD = rrddim_add(RS, "anomalous", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, NumTotalDimensionsRD, NumNormalDimensions + NumAnomalousDimensions);
    rrddim_set_by_pointer(RS, NumTrainedDimensionsRD, NumTrainedDimensions);
    rrddim_set_by_pointer(RS, NumNormalDimensionsRD, NumNormalDimensions);
    rrddim_set_by_pointer(RS, NumAnomalousDimensionsRD, NumAnomalousDimensions);

    rrdset_done(RS);
}

void ml::updateHostAndDetectionRateCharts(RRDHOST *RH, collected_number AnomalyRate) {
    static thread_local RRDSET *HostRateRS = nullptr;
    static thread_local RRDDIM *AnomalyRateRD = nullptr;

    if (!HostRateRS) {
        std::stringstream IdSS, NameSS;

        IdSS << "anomaly_rate_on_" << localhost->machine_guid;
        NameSS << "anomaly_rate_on_" << localhost->hostname;

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
            39184, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );
        rrdset_flag_set(HostRateRS, RRDSET_FLAG_ANOMALY_DETECTION);

        AnomalyRateRD = rrddim_add(HostRateRS, "anomaly_rate", NULL,
                1, 100, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(HostRateRS);

    rrddim_set_by_pointer(HostRateRS, AnomalyRateRD, AnomalyRate);
    rrdset_done(HostRateRS);

    static thread_local RRDSET *AnomalyDetectionRS = nullptr;
    static thread_local RRDDIM *AboveThresholdRD = nullptr;
    static thread_local RRDDIM *NewAnomalyEventRD = nullptr;

    if (!AnomalyDetectionRS) {
        std::stringstream IdSS, NameSS;

        IdSS << "anomaly_detection_on_" << localhost->machine_guid;
        NameSS << "anomaly_detection_on_" << localhost->hostname;

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
            39185, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );
        rrdset_flag_set(AnomalyDetectionRS, RRDSET_FLAG_ANOMALY_DETECTION);

        AboveThresholdRD  = rrddim_add(AnomalyDetectionRS, "above_threshold", NULL,
                                       1, 1, RRD_ALGORITHM_ABSOLUTE);
        NewAnomalyEventRD = rrddim_add(AnomalyDetectionRS, "new_anomaly_event", NULL,
                                       1, 1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(AnomalyDetectionRS);

    /*
     * Compute the values of the dimensions based on the host rate chart
    */
    ONEWAYALLOC *OWA = onewayalloc_create(0);
    time_t Now = now_realtime_sec();
    time_t Before = Now - RH->rrd_update_every;
    time_t After = Before - Cfg.AnomalyDetectionQueryDuration;
    RRDR_OPTIONS Options = static_cast<RRDR_OPTIONS>(0x00000000);

    RRDR *R = rrd2rrdr(
        OWA, HostRateRS,
        1 /* points wanted */,
        After,
        Before,
        Cfg.AnomalyDetectionGroupingMethod,
        0 /* resampling time */,
        Options, "anomaly_rate",
        NULL /* context param list */,
        NULL /* group options */,
        0, /* timeout */
        0 /* tier */
    );
    assert(R->d == 1 && R->n == 1 && R->rows == 1);

    static thread_local bool PrevAboveThreshold = false;
    bool AboveThreshold = R->v[0] >= Cfg.HostAnomalyRateThreshold;
    bool NewAnomalyEvent = AboveThreshold && !PrevAboveThreshold;
    PrevAboveThreshold = AboveThreshold;

    rrddim_set_by_pointer(AnomalyDetectionRS, AboveThresholdRD, AboveThreshold);
    rrddim_set_by_pointer(AnomalyDetectionRS, NewAnomalyEventRD, NewAnomalyEvent);
    rrdset_done(AnomalyDetectionRS);

    rrdr_free(OWA, R);
    onewayalloc_destroy(OWA);
}

void ml::updateDetectionChart(RRDHOST *RH) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *UserRD, *SystemRD = nullptr;

    if (!RS) {
        std::stringstream IdSS, NameSS;

        IdSS << "prediction_stats_" << RH->machine_guid;
        NameSS << "prediction_stats_for_" << RH->hostname;

        RS = rrdset_create_localhost(
            "netdata", // type
            IdSS.str().c_str(), // id
            NameSS.str().c_str(), // name
            "ml", // family
            "netdata.prediction_stats", // ctx
            "Prediction thread CPU usage", // title
            "milliseconds/s", // units
            "netdata", // plugin
            "ml", // module
            136000, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_STACKED // chart_type
        );

        UserRD = rrddim_add(RS, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        SystemRD = rrddim_add(RS, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
    } else
        rrdset_next(RS);

    struct rusage TRU;
    getrusage(RUSAGE_THREAD, &TRU);

    rrddim_set_by_pointer(RS, UserRD, TRU.ru_utime.tv_sec * 1000000ULL + TRU.ru_utime.tv_usec);
    rrddim_set_by_pointer(RS, SystemRD, TRU.ru_stime.tv_sec * 1000000ULL + TRU.ru_stime.tv_usec);
    rrdset_done(RS);
}

void ml::updateTrainingChart(RRDHOST *RH, struct rusage *TRU) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *UserRD = nullptr;
    static thread_local RRDDIM *SystemRD = nullptr;

    if (!RS) {
        std::stringstream IdSS, NameSS;

        IdSS << "training_stats_" << RH->machine_guid;
        NameSS << "training_stats_for_" << RH->hostname;

        RS = rrdset_create_localhost(
            "netdata", // type
            IdSS.str().c_str(), // id
            NameSS.str().c_str(), // name
            "ml", // family
            "netdata.training_stats", // ctx
            "Training thread CPU usage", // title
            "milliseconds/s", // units
            "netdata", // plugin
            "ml", // module
            136001, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_STACKED // chart_type
        );

        UserRD = rrddim_add(RS, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        SystemRD = rrddim_add(RS, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, UserRD, TRU->ru_utime.tv_sec * 1000000ULL + TRU->ru_utime.tv_usec);
    rrddim_set_by_pointer(RS, SystemRD, TRU->ru_stime.tv_sec * 1000000ULL + TRU->ru_stime.tv_usec);
    rrdset_done(RS);
}
