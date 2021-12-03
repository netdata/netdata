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

    /*The time window size within which the set anomalous results per dimension are to be counted*/
    double SaveAnomalyPercentageEvery;

    /*The maximum size, and the associated threshold, of the data table that holds anomaly rate information*/
    double MaxAnomalyRateInfoTableSize;
    double MaxAnomalyRateInfoAge;

    SIMPLE_PATTERN *SP_HostsToSkip;
    SIMPLE_PATTERN *SP_ChartsToSkip;

    std::string AnomalyDBPath;
    #if defined(ENABLE_ML_TESTS)
    std::string AnomalyTestDBPath;
    std::string AnomalyTestDataPath;
    std::string AnomalyTestQuery1Path;
    std::string AnomalyTestCheck1Path;
    std::string AnomalyTestQuery2Path;
    std::string AnomalyTestCheck2Path;
    #endif // ENABLE_ML_TESTS
    void readMLConfig();
};

extern Config Cfg;

} // namespace ml

#endif /* ML_CONFIG_H */
