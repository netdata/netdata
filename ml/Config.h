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

    unsigned DiffN;
    unsigned SmoothN;
    unsigned LagN;

    unsigned MaxKMeansIters;

    double DimensionAnomalyScoreThreshold;
    double HostAnomalyRateThreshold;

    double ADMinWindowSize;
    double ADMaxWindowSize;
    double ADIdleWindowSize;
    double ADWindowRateThreshold;
    double ADDimensionRateThreshold;
    //The window size of anomaly bit counting for anomaly percentage
    unsigned long SaveAnomalyPercentageEvery;

    SIMPLE_PATTERN *SP_HostsToSkip;
    SIMPLE_PATTERN *SP_ChartsToSkip;

    std::string AnomalyDBPath;
    void readMLConfig();
};

extern Config Cfg;

} // namespace ml

#endif /* ML_CONFIG_H */
