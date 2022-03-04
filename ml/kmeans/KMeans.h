// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef KMEANS_H
#define KMEANS_H

#include <atomic>
#include <vector>
#include <limits>
#include <mutex>

#include "SamplesBuffer.h"

class KMeans {
public:
    KMeans(size_t NumClusters = 2) : NumClusters(NumClusters) {
        MinDist = std::numeric_limits<CalculatedNumber>::max();
        MaxDist = std::numeric_limits<CalculatedNumber>::min();
    };

    void train(SamplesBuffer &SB, size_t MaxIterations, bool ReuseClusterCenters);
    CalculatedNumber anomalyScore(SamplesBuffer &SB);

private:
    size_t NumClusters;

    std::vector<DSample> ClusterCenters;

    CalculatedNumber MinDist;
    CalculatedNumber MaxDist;

    std::mutex Mutex;
};

#endif /* KMEANS_H */
