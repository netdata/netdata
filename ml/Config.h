// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_CONFIG_H
#define ML_CONFIG_H

#include "ml-private.h"

namespace ml {

class Config {
public:
    bool EnableAnomalyDetection;

    unsigned MaxTrainSamples;
    unsigned MinTrainSamples;
    unsigned TrainEvery;

    unsigned NumModelsToUse;

    unsigned DBEngineAnomalyRateEvery;

    unsigned DiffN;
    unsigned SmoothN;
    unsigned LagN;

    double RandomSamplingRatio;
    unsigned MaxKMeansIters;

    double DimensionAnomalyScoreThreshold;

    double HostAnomalyRateThreshold;
    RRDR_TIME_GROUPING AnomalyDetectionGroupingMethod;
    time_t AnomalyDetectionQueryDuration;

    bool StreamADCharts;

    std::string HostsToSkip;
    SIMPLE_PATTERN *SP_HostsToSkip;

    std::string ChartsToSkip;
    SIMPLE_PATTERN *SP_ChartsToSkip;

    std::vector<uint32_t> RandomNums;

    void readMLConfig();
};

extern Config Cfg;

} // namespace ml

#endif /* ML_CONFIG_H */
