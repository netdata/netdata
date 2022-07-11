// SPDX-License-Identifier: GPL-3.0-or-later

#include "ADCharts.h"

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

void ml::updateRateChart(RRDHOST *RH, collected_number AnomalyRate) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *AnomalyRateRD = nullptr;

    if (!RS) {
        std::stringstream IdSS, NameSS;

        IdSS << "anomaly_rate_on_" << localhost->machine_guid;
        NameSS << "anomaly_rate_on_" << localhost->hostname;

        RS = rrdset_create(
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
        rrdset_flag_set(RS, RRDSET_FLAG_ANOMALY_DETECTION);

        AnomalyRateRD = rrddim_add(RS, "anomaly_rate", NULL,
                1, 100, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, AnomalyRateRD, AnomalyRate);

    rrdset_done(RS);
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
