// SPDX-License-Identifier: GPL-3.0-or-later

#include "KMeans.h"
#include <dlib/clustering.h>

void KMeans::train(const std::vector<DSample> &Samples, size_t MaxIterations) {
    MinDist = std::numeric_limits<CalculatedNumber>::max();
    MaxDist = std::numeric_limits<CalculatedNumber>::min();

    ClusterCenters.clear();

    dlib::pick_initial_centers(NumClusters, ClusterCenters, Samples);
    dlib::find_clusters_using_kmeans(Samples, ClusterCenters, MaxIterations);

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

CalculatedNumber KMeans::anomalyScore(const DSample &Sample) const {
    CalculatedNumber MeanDist = 0.0;
    for (const auto &CC: ClusterCenters)
        MeanDist += dlib::length(CC - Sample);

    MeanDist /= NumClusters;

    if (MaxDist == MinDist)
        return 0.0;

    CalculatedNumber AnomalyScore = 100.0 * std::abs((MeanDist - MinDist) / (MaxDist - MinDist));
    return (AnomalyScore > 100.0) ? 100.0 : AnomalyScore;
}
