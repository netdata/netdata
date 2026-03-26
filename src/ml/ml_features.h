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

    // Non-null for training (ml_features_preprocess); null for prediction
    // (ml_features_preprocess_predict writes to a separate DSample instead).
    std::vector<DSample> *preprocessed_features;
} ml_features_t;

// Training path: diff + smooth + lag into preprocessed_features (must be non-null).
void ml_features_preprocess(ml_features_t *features, double sampling_ratio);

// Prediction path: diff + smooth, then fill a single DSample from the first lag_n+1 values.
void ml_features_preprocess_predict(ml_features_t *features, DSample *sample);

#endif /* ML_FEATURES_H */
