// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_FEATURES_H
#define ML_FEATURES_H

#include "ml_calculated_number.h"

#include <vector>

typedef struct {
    size_t diff_n;
    size_t smooth_n;
    size_t lag_n;

    calculated_number_t *dst;
    size_t dst_n;

    calculated_number_t *src;
    size_t src_n;
} ml_features_t;

// Training path: diff + smooth + lag into preprocessed_features.
void ml_features_preprocess(ml_features_t *features, std::vector<DSample> &preprocessed_features, double sampling_ratio);

// Prediction path: diff + smooth, then fill a single DSample from the first lag_n+1 values.
void ml_features_preprocess_predict(ml_features_t *features, DSample &sample);

#endif /* ML_FEATURES_H */
