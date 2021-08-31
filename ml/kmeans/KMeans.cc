// SPDX-License-Identifier: GPL-3.0-or-later

#include "KMeans.h"
#include <dlib/clustering.h>

void KMeans::train(SamplesBuffer &SB) {
    std::vector<DSample> Samples = SB.preprocess();

    MinDist = std::numeric_limits<CalculatedNumber>::max();
    MaxDist = std::numeric_limits<CalculatedNumber>::min();

    {
        std::lock_guard<std::mutex> Lock(Mutex);

        ClusterCenters.clear();

        dlib::pick_initial_centers(NumClusters, ClusterCenters, Samples);
        dlib::find_clusters_using_kmeans(Samples, ClusterCenters);

        for (const auto &S : Samples) {
            CalculatedNumber MeanDist = 0.0;

            for (const auto &KMCenter : ClusterCenters)
                MeanDist += dlib::length(KMCenter - S);

            MeanDist /= NumClusters;

            if (MeanDist < MinDist)
                MinDist = MeanDist;

            if (MeanDist > MaxDist)
                MaxDist = MeanDist;
        }
    }

    HasClusterCenters = true;
}

CalculatedNumber KMeans::anomalyScore(SamplesBuffer &SB) {
    if (!HasClusterCenters)
        return std::numeric_limits<CalculatedNumber>::quiet_NaN();

    std::vector<DSample> DSamples = SB.preprocess();

    std::unique_lock<std::mutex> Lock(Mutex, std::defer_lock);
    if (!Lock.try_lock())
        return std::numeric_limits<CalculatedNumber>::quiet_NaN();

    CalculatedNumber MeanDist = 0.0;
    for (const auto &CC: ClusterCenters)
        MeanDist += dlib::length(CC - DSamples.back());

    MeanDist /= NumClusters;

    if (MaxDist == MinDist)
        return 0.0;

    CalculatedNumber AnomalyScore = 100.0 * std::abs((MeanDist - MinDist) / (MaxDist - MinDist));
    return (AnomalyScore > 100.0) ? 100.0 : AnomalyScore;
}
