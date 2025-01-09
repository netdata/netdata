// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_KMEANS_H
#define ML_KMEANS_H

#include "ml_features.h"

typedef struct web_buffer BUFFER;

struct ml_kmeans_inlined_t;

struct ml_kmeans_t {
    std::vector<DSample> cluster_centers;
    calculated_number_t min_dist;
    calculated_number_t max_dist;
    uint32_t after;
    uint32_t before;

    ml_kmeans_t() : min_dist(0), max_dist(0), after(0), before(0)
    {
    }

    explicit ml_kmeans_t(const ml_kmeans_inlined_t &inlined);
    ml_kmeans_t &operator=(const ml_kmeans_inlined_t &inlined);
};

struct ml_kmeans_inlined_t {
    std::array<DSample, 2> cluster_centers;
    calculated_number_t min_dist;
    calculated_number_t max_dist;
    uint32_t after;
    uint32_t before;

    ml_kmeans_inlined_t() : min_dist(0), max_dist(0), after(0), before(0)
    {
    }

    explicit ml_kmeans_inlined_t(const ml_kmeans_t &km)
    {
        if (km.cluster_centers.size() == 2) {
            cluster_centers[0] = km.cluster_centers[0];
            cluster_centers[1] = km.cluster_centers[1];
        }

        min_dist = km.min_dist;
        max_dist = km.max_dist;
        after = km.after;
        before = km.before;
    }

    ml_kmeans_inlined_t &operator=(const ml_kmeans_t &km)
    {
        if (km.cluster_centers.size() == 2) {
            cluster_centers[0] = km.cluster_centers[0];
            cluster_centers[1] = km.cluster_centers[1];
        }
        min_dist = km.min_dist;
        max_dist = km.max_dist;
        after = km.after;
        before = km.before;
        return *this;
    }
};

inline ml_kmeans_t::ml_kmeans_t(const ml_kmeans_inlined_t &inlined_km)
{
    cluster_centers.reserve(2);
    cluster_centers.push_back(inlined_km.cluster_centers[0]);
    cluster_centers.push_back(inlined_km.cluster_centers[1]);

    min_dist = inlined_km.min_dist;
    max_dist = inlined_km.max_dist;

    after = inlined_km.after;
    before = inlined_km.before;
}

inline ml_kmeans_t &ml_kmeans_t::operator=(const ml_kmeans_inlined_t &inlined_km)
{
    cluster_centers.clear();
    cluster_centers.reserve(2);
    cluster_centers.push_back(inlined_km.cluster_centers[0]);
    cluster_centers.push_back(inlined_km.cluster_centers[1]);

    min_dist = inlined_km.min_dist;
    max_dist = inlined_km.max_dist;

    after = inlined_km.after;
    before = inlined_km.before;
    return *this;
}

void ml_kmeans_init(ml_kmeans_t *kmeans);

void ml_kmeans_train(ml_kmeans_t *kmeans, const ml_features_t *features, unsigned max_iters, time_t after, time_t before);

calculated_number_t ml_kmeans_anomaly_score(const ml_kmeans_inlined_t *kmeans, const DSample &DS);

void ml_kmeans_serialize(const ml_kmeans_inlined_t *inlined_km, BUFFER *wb);

bool ml_kmeans_deserialize(ml_kmeans_inlined_t *inlined_km, struct json_object *root);

#endif /* ML_KMEANS_H */
