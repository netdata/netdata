// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml_config.h"
#include "ml_features.h"

static void ml_features_diff(ml_features_t *features)
{
    if (features->diff_n == 0)
        return;

    for (size_t idx = 0; idx != (features->src_n - features->diff_n); idx++) {
        size_t high = (features->src_n - 1) - idx;
        size_t low = high - features->diff_n;

        features->dst[low] = features->src[high] - features->src[low];
    }

    size_t n = features->src_n - features->diff_n;
    memcpy(features->src, features->dst, n * sizeof(calculated_number_t));

    for (size_t idx = features->src_n - features->diff_n; idx != features->src_n; idx++)
        features->src[idx] = 0.0;
}

static void ml_features_smooth(ml_features_t *features)
{
    calculated_number_t sum = 0.0;

    size_t idx = 0;
    for (; idx != features->smooth_n - 1; idx++)
        sum += features->src[idx];

    for (; idx != (features->src_n - features->diff_n); idx++) {
        sum += features->src[idx];
        calculated_number_t prev_cn = features->src[idx - (features->smooth_n - 1)];
        features->src[idx - (features->smooth_n - 1)] = sum / features->smooth_n;
        sum -= prev_cn;
    }

    for (idx = 0; idx != features->smooth_n; idx++)
        features->src[(features->src_n - 1) - idx] = 0.0;
}

static void ml_features_lag(ml_features_t *features, double sampling_ratio)
{
    size_t n = features->src_n - features->diff_n - features->smooth_n + 1 - features->lag_n;
    features->preprocessed_features.resize(n);

    uint32_t max_mt = std::numeric_limits<uint32_t>::max();
    uint32_t cutoff = static_cast<double>(max_mt) * sampling_ratio;

    size_t sample_idx = 0;

    for (size_t idx = 0; idx != n; idx++) {
        DSample &DS = features->preprocessed_features[sample_idx++];
        DS.set_size(features->lag_n);

        if (Cfg.random_nums[idx] > cutoff) {
            sample_idx--;
            continue;
        }

        for (size_t feature_idx = 0; feature_idx != features->lag_n + 1; feature_idx++)
            DS(feature_idx) = features->src[idx + feature_idx];
    }

    features->preprocessed_features.resize(sample_idx);
}

void ml_features_preprocess(ml_features_t *features, double sampling_ratio)
{
    ml_features_diff(features);
    ml_features_smooth(features);
    ml_features_lag(features, sampling_ratio);
}
