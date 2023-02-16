// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef KMEANS_H
#define KMEANS_H

#include <atomic>
#include <vector>
#include <limits>
#include <mutex>

#include "SamplesBuffer.h"
#include "json/single_include/nlohmann/json.hpp"

class KMeans {
public:
    KMeans(size_t NumClusters = 2) : NumClusters(NumClusters) {
        MinDist = std::numeric_limits<CalculatedNumber>::max();
        MaxDist = std::numeric_limits<CalculatedNumber>::min();
    };

    void train(const std::vector<DSample> &Samples, size_t MaxIterations);
    CalculatedNumber anomalyScore(const DSample &Sample) const;

    void toJson(nlohmann::json &J) const {
        J = nlohmann::json{
            {"CCs", ClusterCenters},
            {"MinDist", MinDist},
            {"MaxDist", MaxDist}
        };
    }

private:
    size_t NumClusters;

    std::vector<DSample> ClusterCenters;

    CalculatedNumber MinDist;
    CalculatedNumber MaxDist;
};

#endif /* KMEANS_H */
