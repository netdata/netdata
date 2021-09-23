// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_CONFIG_H
#define ML_CONFIG_H

#include "ml-private.h"

namespace ml {

class Config {
public:
    bool EnableAnomalyDetection;

    Seconds TrainSecs;
    Seconds MinTrainSecs;
    Seconds TrainEvery;

    unsigned DiffN;
    unsigned SmoothN;
    unsigned LagN;

    double DimensionAnomalyScoreThreshold;
    double HostAnomalyRateThreshold;

    double ADWindowSize;
    double ADWindowRateThreshold;
    double ADDimensionRateThreshold;

    SIMPLE_PATTERN *SP_HostsToSkip;
    SIMPLE_PATTERN *SP_ChartsToSkip;

    std::string AnomalyDBPath;

    void readMLConfig();
};

extern Config Cfg;

} // namespace ml

#endif /* ML_CONFIG_H */
