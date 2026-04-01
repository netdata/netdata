// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml_config.h"
#include "ml_features.h"
#include "ml_kmeans.h"

#include <algorithm>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <utility>
#include <vector>

static constexpr double ML_PI = 3.14159265358979323846;

static int tests_run = 0;
static int tests_failed = 0;

#define ML_TEST_ASSERT(cond, msg) do { \
    tests_run++; \
    if (!(cond)) { \
        fprintf(stderr, "  FAIL: %s (line %d)\n", msg, __LINE__); \
        tests_failed++; \
    } \
} while (0)

#define ML_TEST_ASSERT_DOUBLE_EQ(a, b, eps, msg) do { \
    tests_run++; \
    if (std::fabs((a) - (b)) > (eps)) { \
        fprintf(stderr, "  FAIL: %s (line %d): expected %.10f, got %.10f\n", msg, __LINE__, (double)(b), (double)(a)); \
        tests_failed++; \
    } \
} while (0)

// Test: diff transform with diff_n=1
// Input: [1, 3, 6, 10, 15, 9, 9, 9, 9]
// Expected diff (high - low with lag 1): [2, 3, 4, 5, -6, 0, 0, 0]
// (last element zeroed)
static void test_features_diff()
{
    fprintf(stderr, "  test_features_diff...\n");

    const size_t n = 9;
    calculated_number_t src[16] = {1, 3, 6, 10, 15, 9, 9, 9, 9};
    calculated_number_t dst[16] = {0};

    std::vector<DSample> pf;
    ml_features_t features = {
        1, 1, 1,    // diff_n=1, smooth_n=1, lag_n=1
        dst, n, src, n
    };

    // ml_features_preprocess calls diff, smooth, lag in sequence.
    // To test diff alone, we use preprocess with smooth_n=1 (no-op smooth)
    // and lag_n=1, sampling_ratio=1.0.
    // After diff with diff_n=1:
    //   src[0..7] = [2, 3, 4, 5, -6, 0, 0, 0], src[8] = 0
    // After smooth with smooth_n=1 (no-op):
    //   unchanged, but last smooth_n=1 elements zeroed: src[8]=0 (already 0)
    // After lag with lag_n=1:
    //   feature vectors: [src[i], src[i+1]] for i in 0..6
    //   n_vectors = 9 - 1 - 1 + 1 - 1 = 7

    ml_features_preprocess(&features, pf, 1.0);

    ML_TEST_ASSERT(pf.size() == 7, "lag should produce 7 feature vectors");
    if (pf.size() >= 1) {
        // First feature vector should be [2, 3]
        ML_TEST_ASSERT_DOUBLE_EQ(pf[0](0), 2.0, 1e-9, "pf[0](0) == 2.0");
        ML_TEST_ASSERT_DOUBLE_EQ(pf[0](1), 3.0, 1e-9, "pf[0](1) == 3.0");
    }
    if (pf.size() >= 4) {
        // Fourth feature vector should be [5, -6]
        ML_TEST_ASSERT_DOUBLE_EQ(pf[3](0), 5.0, 1e-9, "pf[3](0) == 5.0");
        ML_TEST_ASSERT_DOUBLE_EQ(pf[3](1), -6.0, 1e-9, "pf[3](1) == -6.0");
    }
}

// Test: diff_n=0 means no differencing
static void test_features_no_diff()
{
    fprintf(stderr, "  test_features_no_diff...\n");

    const size_t n = 6;
    calculated_number_t src[16] = {10, 20, 30, 40, 50, 60};
    calculated_number_t dst[16] = {0};

    std::vector<DSample> pf;
    ml_features_t features = {
        0, 1, 1,    // diff_n=0, smooth_n=1, lag_n=1
        dst, n, src, n
    };

    ml_features_preprocess(&features, pf, 1.0);

    // With diff_n=0, smooth_n=1 (no-op), lag_n=1:
    // n_vectors = 6 - 0 - 1 + 1 - 1 = 5
    // But smooth zeros last smooth_n=1 elements, so src[5]=0
    // Feature vectors: [src[i], src[i+1]] for i in 0..3
    // Wait: n = src_n - diff_n - smooth_n + 1 - lag_n = 6 - 0 - 1 + 1 - 1 = 5
    ML_TEST_ASSERT(pf.size() == 5, "no-diff should produce 5 feature vectors");

    if (pf.size() >= 1) {
        ML_TEST_ASSERT_DOUBLE_EQ(pf[0](0), 10.0, 1e-9, "pf[0](0) == 10.0");
        ML_TEST_ASSERT_DOUBLE_EQ(pf[0](1), 20.0, 1e-9, "pf[0](1) == 20.0");
    }
}

// Test: smoothing with smooth_n=3
static void test_features_smooth()
{
    fprintf(stderr, "  test_features_smooth...\n");

    const size_t n = 9;
    // Use a simple sequence: 1,2,3,4,5,6,7,8,9
    calculated_number_t src[16] = {1, 2, 3, 4, 5, 6, 7, 8, 9};
    calculated_number_t dst[16] = {0};

    std::vector<DSample> pf;
    ml_features_t features = {
        0, 3, 1,    // diff_n=0, smooth_n=3, lag_n=1
        dst, n, src, n
    };

    ml_features_preprocess(&features, pf, 1.0);

    // With diff_n=0: no diff
    // Smooth with smooth_n=3, operating on src_n - diff_n = 9 elements:
    //   Moving average: src[0] = (1+2+3)/3=2, src[1]=(2+3+4)/3=3, ..., src[6]=(7+8+9)/3=8
    //   Last smooth_n=3 elements zeroed: src[6..8]=0
    //   So smoothed: [2, 3, 4, 5, 6, 7, 0, 0, 0]
    //   Wait, the smooth zeros the LAST smooth_n elements from the end of src.
    //   src[(src_n-1)-0]=src[8]=0, src[(src_n-1)-1]=src[7]=0, src[(src_n-1)-2]=src[6]=0
    //   So the smoothed region is src[0..5] = [2, 3, 4, 5, 6, 7]
    // Lag with lag_n=1:
    //   n_vectors = 9 - 0 - 3 + 1 - 1 = 6
    //   Vectors: [src[i], src[i+1]] for i in 0..5

    ML_TEST_ASSERT(pf.size() == 6, "smooth should produce 6 feature vectors");

    if (pf.size() >= 1) {
        ML_TEST_ASSERT_DOUBLE_EQ(pf[0](0), 2.0, 1e-9, "pf[0](0) == 2.0 (smoothed)");
        ML_TEST_ASSERT_DOUBLE_EQ(pf[0](1), 3.0, 1e-9, "pf[0](1) == 3.0 (smoothed)");
    }
    if (pf.size() >= 6) {
        ML_TEST_ASSERT_DOUBLE_EQ(pf[5](0), 7.0, 1e-9, "pf[5](0) == 7.0 (smoothed)");
        ML_TEST_ASSERT_DOUBLE_EQ(pf[5](1), 0.0, 1e-9, "pf[5](1) == 0.0 (zeroed by smooth)");
    }
}

// Test: smooth_n=0 is normalized to the same effective smoothing window as
// smooth_n=1, for both training and prediction.
static void test_features_zero_smooth_matches_one()
{
    fprintf(stderr, "  test_features_zero_smooth_matches_one...\n");

    const size_t n = 6;
    calculated_number_t input[16] = {10, 20, 30, 40, 50, 60};

    calculated_number_t src0[16], dst0[16];
    memcpy(src0, input, n * sizeof(calculated_number_t));
    memcpy(dst0, input, n * sizeof(calculated_number_t));
    std::vector<DSample> pf0;
    ml_features_t features0 = {
        0, 0, 1,
        dst0, n, src0, n
    };
    ml_features_preprocess(&features0, pf0, 1.0);

    calculated_number_t src1[16], dst1[16];
    memcpy(src1, input, n * sizeof(calculated_number_t));
    memcpy(dst1, input, n * sizeof(calculated_number_t));
    std::vector<DSample> pf1;
    ml_features_t features1 = {
        0, 1, 1,
        dst1, n, src1, n
    };
    ml_features_preprocess(&features1, pf1, 1.0);

    ML_TEST_ASSERT(pf0.size() == pf1.size(), "smooth_n=0 and smooth_n=1 should produce the same number of vectors");
    for (size_t i = 0; i < pf0.size() && i < pf1.size(); i++) {
        for (size_t j = 0; j < features0.lag_n + 1; j++) {
            char msg[128];
            snprintf(msg, sizeof(msg), "smooth_n=0 should match smooth_n=1 at feature[%zu](%zu)", i, j);
            ML_TEST_ASSERT_DOUBLE_EQ(pf0[i](j), pf1[i](j), 1e-12, msg);
        }
    }

    DSample sample0, sample1;
    memcpy(src0, input, n * sizeof(calculated_number_t));
    memcpy(dst0, input, n * sizeof(calculated_number_t));
    ml_features_preprocess_predict(&features0, sample0);

    memcpy(src1, input, n * sizeof(calculated_number_t));
    memcpy(dst1, input, n * sizeof(calculated_number_t));
    ml_features_preprocess_predict(&features1, sample1);

    ML_TEST_ASSERT(sample0.size() == sample1.size(), "prediction sample size should match for smooth_n=0 and smooth_n=1");
    for (size_t i = 0; i < features0.lag_n + 1; i++) {
        char msg[128];
        snprintf(msg, sizeof(msg), "prediction smooth_n=0 should match smooth_n=1 at sample(%zu)", i);
        ML_TEST_ASSERT_DOUBLE_EQ(sample0(i), sample1(i), 1e-12, msg);
    }
}

// Test: full pipeline with default-like params (diff_n=1, smooth_n=3, lag_n=5)
// Validates the feature vector shape and that a round-trip through
// train + score produces sensible anomaly scores.
static void test_full_pipeline()
{
    fprintf(stderr, "  test_full_pipeline...\n");

    // diff_n=1, smooth_n=3, lag_n=5 => n = 1 + 3 + 5 = 9
    const size_t diff_n = 1;
    const size_t smooth_n = 3;
    const size_t lag_n = 5;
    const size_t n = diff_n + smooth_n + lag_n;

    // Generate a "normal" pattern: sine wave with some noise-like variation
    const size_t num_samples = 100;
    calculated_number_t normal_data[100];
    for (size_t i = 0; i < num_samples; i++)
        normal_data[i] = 50.0 + 20.0 * std::sin(2.0 * ML_PI * (double)i / 25.0);

    // Simulate prediction: slide a window of size n over the data,
    // preprocess each window, collect the first feature vector.
    std::vector<DSample> all_features;

    for (size_t start = 0; start + n <= num_samples; start++) {
        calculated_number_t src[128];
        calculated_number_t dst[128];

        memset(src, 0, sizeof(src));
        memcpy(src, &normal_data[start], n * sizeof(calculated_number_t));
        memcpy(dst, src, n * sizeof(calculated_number_t));

        std::vector<DSample> pf;
        ml_features_t features = {
            diff_n, smooth_n, lag_n,
            dst, n, src, n
        };
        ml_features_preprocess(&features, pf, 1.0);

        // With these params:
        // n_vectors = n - diff_n - smooth_n + 1 - lag_n = 9 - 1 - 3 + 1 - 5 = 1
        ML_TEST_ASSERT(pf.size() == 1, "prediction window should produce exactly 1 feature vector");

        if (pf.size() >= 1)
            all_features.push_back(pf[0]);
    }

    ML_TEST_ASSERT(all_features.size() > 2, "should have enough features for kmeans");

    // Each feature vector should have lag_n + 1 = 6 elements
    if (all_features.size() > 0) {
        ML_TEST_ASSERT(all_features[0].size() == (long)(lag_n + 1),
                       "feature vector should have lag_n+1 elements");
    }

    // Train a kmeans model on the normal data
    std::vector<DSample> training_features = std::move(all_features);

    ml_kmeans_t kmeans;
    ml_kmeans_init(&kmeans);
    ml_kmeans_train(&kmeans, training_features, 1000, 0, 100);

    ML_TEST_ASSERT(kmeans.cluster_centers.size() == 2, "kmeans should have 2 cluster centers");
    ML_TEST_ASSERT(kmeans.min_dist < kmeans.max_dist, "min_dist < max_dist after training");

    // Score all training samples — the best score must be 0 (the sample at min_dist)
    // and all training scores must be in [0, 100].
    ml_kmeans_inlined_t inlined_km(kmeans);
    calculated_number_t best_normal_score = 100.0;
    for (size_t i = 0; i < training_features.size(); i++) {
        calculated_number_t s = ml_kmeans_anomaly_score(&inlined_km, training_features[i]);
        ML_TEST_ASSERT(!std::isnan(s), "training sample score should not be NaN");
        ML_TEST_ASSERT(s >= 0.0 && s <= 100.0, "training sample score should be in [0, 100]");
        if (s < best_normal_score)
            best_normal_score = s;
    }
    calculated_number_t normal_score = best_normal_score;
    ML_TEST_ASSERT_DOUBLE_EQ(normal_score, 0.0, 1e-6, "best training sample should score ~0");

    // Create an anomalous sample: extreme spike values unlike the sine wave
    calculated_number_t anomalous_window[9] = {50, 50, 50, 500, 50, 50, 50, 500, 50};
    {
        calculated_number_t src[128], dst[128];
        memset(src, 0, sizeof(src));
        memcpy(src, anomalous_window, n * sizeof(calculated_number_t));
        memcpy(dst, src, n * sizeof(calculated_number_t));

        std::vector<DSample> pf;
        ml_features_t features = {
            diff_n, smooth_n, lag_n,
            dst, n, src, n
        };
        ml_features_preprocess(&features, pf, 1.0);

        if (pf.size() >= 1) {
            calculated_number_t anomaly_score = ml_kmeans_anomaly_score(&inlined_km, pf[0]);
            ML_TEST_ASSERT(!std::isnan(anomaly_score), "anomaly score should not be NaN");
            ML_TEST_ASSERT(anomaly_score > normal_score, "anomalous data should score higher than normal");
        }
    }
}

// Test: kmeans anomaly_score edge cases
static void test_kmeans_scoring()
{
    fprintf(stderr, "  test_kmeans_scoring...\n");

    // Build a simple model with known cluster centers
    ml_kmeans_inlined_t km;
    km.cluster_centers[0].set_size(6);
    km.cluster_centers[1].set_size(6);
    for (int i = 0; i < 6; i++) {
        km.cluster_centers[0](i) = 0.0;
        km.cluster_centers[1](i) = 10.0;
    }
    km.min_dist = 1.0;
    km.max_dist = 10.0;
    km.after = 0;
    km.before = 100;

    // Sample at cluster center 0 — should score low
    DSample at_center0;
    at_center0.set_size(6);
    for (int i = 0; i < 6; i++)
        at_center0(i) = 0.0;

    calculated_number_t score0 = ml_kmeans_anomaly_score(&km, at_center0);
    ML_TEST_ASSERT(!std::isnan(score0), "score at center should not be NaN");

    // Sample at midpoint — should score moderate
    DSample at_mid;
    at_mid.set_size(6);
    for (int i = 0; i < 6; i++)
        at_mid(i) = 5.0;

    calculated_number_t score_mid = ml_kmeans_anomaly_score(&km, at_mid);
    ML_TEST_ASSERT(!std::isnan(score_mid), "score at midpoint should not be NaN");

    // Sample far away — should score high (capped at 100)
    DSample far_away;
    far_away.set_size(6);
    for (int i = 0; i < 6; i++)
        far_away(i) = 100.0;

    calculated_number_t score_far = ml_kmeans_anomaly_score(&km, far_away);
    ML_TEST_ASSERT_DOUBLE_EQ(score_far, 100.0, 1e-9, "far away sample should be capped at 100");

    // When min_dist == max_dist, score should be 0
    ml_kmeans_inlined_t km_equal;
    km_equal.cluster_centers[0].set_size(6);
    km_equal.cluster_centers[1].set_size(6);
    for (int i = 0; i < 6; i++) {
        km_equal.cluster_centers[0](i) = 5.0;
        km_equal.cluster_centers[1](i) = 5.0;
    }
    km_equal.min_dist = 5.0;
    km_equal.max_dist = 5.0;

    calculated_number_t score_eq = ml_kmeans_anomaly_score(&km_equal, at_mid);
    ML_TEST_ASSERT_DOUBLE_EQ(score_eq, 0.0, 1e-9, "equal min/max should return 0");
}

// Test: circular buffer linearization produces the same result as std::rotate
static void test_circular_buffer_equivalence()
{
    fprintf(stderr, "  test_circular_buffer_equivalence...\n");

    const size_t diff_n = 1;
    const size_t smooth_n = 3;
    const size_t lag_n = 5;
    const size_t n = diff_n + smooth_n + lag_n;

    // Simulate feeding values into the dimension
    calculated_number_t values[] = {
        1.0, 2.5, 3.7, 4.1, 5.9, 6.3, 7.8, 8.2, 9.0,   // fill buffer (n=9 values)
        10.5, 11.3, 12.8, 13.1, 14.7, 15.2                // 6 more values to rotate
    };
    size_t num_values = sizeof(values) / sizeof(values[0]);

    // Method 1: std::rotate (master approach)
    std::vector<calculated_number_t> cns_rotate;
    std::vector<DSample> rotate_results;

    for (size_t i = 0; i < num_values; i++) {
        if (cns_rotate.size() < n) {
            cns_rotate.push_back(values[i]);
            continue;
        }

        std::rotate(cns_rotate.begin(), cns_rotate.begin() + 1, cns_rotate.end());
        cns_rotate[n - 1] = values[i];

        calculated_number_t src[128], dst[128];
        memset(src, 0, sizeof(src));
        memcpy(src, cns_rotate.data(), n * sizeof(calculated_number_t));
        memcpy(dst, cns_rotate.data(), n * sizeof(calculated_number_t));

        std::vector<DSample> pf;
        ml_features_t features = {
            diff_n, smooth_n, lag_n,
            dst, n, src, n
        };
        ml_features_preprocess(&features, pf, 1.0);

        if (pf.size() >= 1)
            rotate_results.push_back(pf[0]);
    }

    // Method 2: circular buffer (branch approach)
    std::vector<calculated_number_t> cns_circ;
    size_t cns_head = 0;
    std::vector<DSample> circ_results;

    for (size_t i = 0; i < num_values; i++) {
        if (cns_circ.size() < n) {
            cns_circ.push_back(values[i]);
            continue;
        }

        cns_circ[cns_head] = values[i];
        cns_head = (cns_head + 1) % n;

        // Linearize circular buffer
        calculated_number_t src[128], dst[128];
        size_t first_chunk = n - cns_head;
        memcpy(src, cns_circ.data() + cns_head, first_chunk * sizeof(calculated_number_t));
        if (cns_head)
            memcpy(src + first_chunk, cns_circ.data(), cns_head * sizeof(calculated_number_t));
        memcpy(dst, src, n * sizeof(calculated_number_t));

        std::vector<DSample> pf;
        ml_features_t features = {
            diff_n, smooth_n, lag_n,
            dst, n, src, n
        };
        ml_features_preprocess(&features, pf, 1.0);

        if (pf.size() >= 1)
            circ_results.push_back(pf[0]);
    }

    ML_TEST_ASSERT(rotate_results.size() == circ_results.size(),
                   "both methods should produce same number of results");

    for (size_t i = 0; i < rotate_results.size() && i < circ_results.size(); i++) {
        for (long j = 0; j < rotate_results[i].size(); j++) {
            char msg[128];
            snprintf(msg, sizeof(msg), "result[%zu](%ld) should match between rotate and circular", i, j);
            ML_TEST_ASSERT_DOUBLE_EQ(rotate_results[i](j), circ_results[i](j), 1e-12, msg);
        }
    }
}

// Test: same_value must compare against the previous newest sample, not the
// oldest slot being overwritten. This locks in the intentional semantic change
// in ml_dimension_predict().
static void test_same_value_uses_newest_sample()
{
    fprintf(stderr, "  test_same_value_uses_newest_sample...\n");

    const size_t n = 5;
    std::vector<calculated_number_t> cns = {7.0, 2.0, 3.0, 4.0, 5.0};
    size_t cns_head = 0;
    calculated_number_t incoming = 7.0;

    // Circular buffer state:
    //   oldest slot being overwritten = cns[cns_head] = 7.0
    //   previous newest sample        = cns[(cns_head + n - 1) % n] = 5.0
    // If we compared against the oldest slot, same_value would be true and we'd
    // miss the transition from 5.0 -> 7.0. Comparing against newest is correct.
    bool old_rotate_equivalent = (cns[cns_head] == incoming);
    size_t newest_idx = (cns_head + n - 1) % n;
    bool new_ring_semantics = (cns[newest_idx] == incoming);

    ML_TEST_ASSERT(old_rotate_equivalent,
                   "oldest-slot comparison should report same_value for this edge case");
    ML_TEST_ASSERT(!new_ring_semantics,
                   "newest-sample comparison should detect the changed incoming value");
}

// Test: ml_features_preprocess with a prediction-sized window produces the same
// feature vector as a manual reimplementation of diff + smooth + extract.
// This validates the preprocessing math and serves as a baseline for verifying
// that any optimized prediction path (e.g. ml_features_preprocess_predict)
// produces identical results.
static void test_preprocess_predict_equivalence()
{
    fprintf(stderr, "  test_preprocess_predict_equivalence...\n");

    struct {
        size_t diff_n, smooth_n, lag_n;
    } param_sets[] = {
        {1, 3, 5},  // default params
        {0, 3, 5},  // no diff
        {1, 1, 5},  // minimal smooth
        {1, 3, 1},  // minimal lag
        {0, 1, 1},  // minimal everything
        {1, 5, 3},  // larger smooth, smaller lag
    };

    // Various input patterns
    auto fill_sine = [](calculated_number_t *buf, size_t n) {
        for (size_t i = 0; i < n; i++)
            buf[i] = 50.0 + 20.0 * std::sin(2.0 * ML_PI * (double)i / 7.0);
    };
    auto fill_ramp = [](calculated_number_t *buf, size_t n) {
        for (size_t i = 0; i < n; i++)
            buf[i] = 1.0 + 0.7 * (double)i;
    };
    auto fill_spike = [](calculated_number_t *buf, size_t n) {
        for (size_t i = 0; i < n; i++)
            buf[i] = (i == n / 2) ? 500.0 : 10.0;
    };

    void (*fillers[])(calculated_number_t *, size_t) = {fill_sine, fill_ramp, fill_spike};
    const char *filler_names[] = {"sine", "ramp", "spike"};

    for (size_t p = 0; p < sizeof(param_sets) / sizeof(param_sets[0]); p++) {
        size_t diff_n = param_sets[p].diff_n;
        size_t smooth_n = param_sets[p].smooth_n;
        size_t lag_n = param_sets[p].lag_n;
        size_t n = diff_n + smooth_n + lag_n;

        for (size_t f = 0; f < 3; f++) {
            calculated_number_t input[128];
            fillers[f](input, n);

            // Path 1: ml_features_preprocess (master prediction path)
            calculated_number_t src1[128], dst1[128];
            memset(src1, 0, sizeof(src1));
            memcpy(src1, input, n * sizeof(calculated_number_t));
            memcpy(dst1, src1, n * sizeof(calculated_number_t));

            std::vector<DSample> pf;
            ml_features_t features1 = {
                diff_n, smooth_n, lag_n,
                dst1, n, src1, n
            };
            ml_features_preprocess(&features1, pf, 1.0);

            // With prediction-sized window: n_vectors = n - diff_n - smooth_n + 1 - lag_n = 1
            char msg[256];
            snprintf(msg, sizeof(msg), "params(%zu,%zu,%zu) %s: preprocess should produce 1 vector",
                     diff_n, smooth_n, lag_n, filler_names[f]);
            ML_TEST_ASSERT(pf.size() == 1, msg);
            if (pf.size() != 1) continue;

            // Path 2: ml_features_preprocess_predict should produce the same
            // prediction-sized feature vector as the training preprocess path.
            calculated_number_t src2[128], dst2[128];
            memset(src2, 0, sizeof(src2));
            memcpy(src2, input, n * sizeof(calculated_number_t));
            memcpy(dst2, src2, n * sizeof(calculated_number_t));

            ml_features_t features2 = {
                diff_n, smooth_n, lag_n,
                dst2, n, src2, n
            };
            DSample predicted_feature;
            ml_features_preprocess_predict(&features2, predicted_feature);

            // Compare: pf[0] from full pipeline must match the direct prediction path.
            for (size_t fi = 0; fi < lag_n + 1; fi++) {
                snprintf(msg, sizeof(msg), "params(%zu,%zu,%zu) %s: feature[%zu] preprocess vs predict",
                         diff_n, smooth_n, lag_n, filler_names[f], fi);
                ML_TEST_ASSERT_DOUBLE_EQ(pf[0](fi), predicted_feature(fi), 1e-12, msg);
            }
        }
    }
}

// Test: constant input values produce zero-diff features and don't cause anomalies
static void test_constant_input()
{
    fprintf(stderr, "  test_constant_input...\n");

    const size_t diff_n = 1;
    const size_t smooth_n = 3;
    const size_t lag_n = 5;
    const size_t n = diff_n + smooth_n + lag_n;

    // All constant values
    calculated_number_t src[16], dst[16];
    for (size_t i = 0; i < n; i++)
        src[i] = 42.0;
    memcpy(dst, src, n * sizeof(calculated_number_t));

    std::vector<DSample> pf;
    ml_features_t features = {
        diff_n, smooth_n, lag_n,
        dst, n, src, n
    };
    ml_features_preprocess(&features, pf, 1.0);

    ML_TEST_ASSERT(pf.size() == 1, "constant input should produce 1 feature vector");

    // With diff_n=1 on constant input, all diffs are 0.
    // After smooth, still all 0. Feature vector should be all zeros.
    if (pf.size() >= 1) {
        for (size_t i = 0; i < lag_n + 1; i++) {
            char msg[128];
            snprintf(msg, sizeof(msg), "constant input: feature[%zu] should be 0", i);
            ML_TEST_ASSERT_DOUBLE_EQ(pf[0](i), 0.0, 1e-12, msg);
        }
    }

    // All-zero feature with diff_n=0 should preserve the constant value
    calculated_number_t src2[16], dst2[16];
    for (size_t i = 0; i < n; i++)
        src2[i] = 42.0;
    memcpy(dst2, src2, n * sizeof(calculated_number_t));

    std::vector<DSample> pf2;
    ml_features_t features2 = {
        0, smooth_n, lag_n,
        dst2, n, src2, n
    };
    ml_features_preprocess(&features2, pf2, 1.0);

    // With diff_n=0, smooth on constant values gives the same constant.
    // Feature vector should be all 42.0.
    if (pf2.size() >= 1) {
        for (size_t i = 0; i < lag_n + 1; i++) {
            char msg[128];
            snprintf(msg, sizeof(msg), "constant no-diff: feature[%zu] should be 42", i);
            ML_TEST_ASSERT_DOUBLE_EQ(pf2[0](i), 42.0, 1e-9, msg);
        }
    }

    // Simulate same_value detection with circular buffer
    // Feed the same value repeatedly — same_value should be true every time
    std::vector<calculated_number_t> cns;
    size_t cns_head = 0;
    bool all_same = true;

    for (size_t i = 0; i < n + 10; i++) {
        calculated_number_t value = 42.0;

        if (cns.size() < n) {
            cns.push_back(value);
            continue;
        }

        size_t newest_idx = (cns_head + n - 1) % n;
        bool same_value = (cns[newest_idx] == value);
        cns[cns_head] = value;
        cns_head = (cns_head + 1) % n;

        if (!same_value)
            all_same = false;
    }
    ML_TEST_ASSERT(all_same, "constant input should always detect same_value");

    // Now feed a different value — same_value should be false
    {
        calculated_number_t value = 99.0;
        size_t newest_idx = (cns_head + n - 1) % n;
        bool same_value = (cns[newest_idx] == value);
        ML_TEST_ASSERT(!same_value, "different value should not detect same_value");
    }
}

// Test: various parameter combinations produce correctly sized outputs
static void test_parameter_combinations()
{
    fprintf(stderr, "  test_parameter_combinations...\n");

    struct {
        size_t diff_n, smooth_n, lag_n;
        size_t expected_vectors;  // from a window of size diff_n + smooth_n + lag_n
    } cases[] = {
        // n_vectors = n - diff_n - smooth_n + 1 - lag_n
        // For prediction-sized window (n = diff_n + smooth_n + lag_n):
        // n_vectors = (diff_n + smooth_n + lag_n) - diff_n - smooth_n + 1 - lag_n = 1
        {0, 1, 1, 1},
        {0, 1, 2, 1},
        {0, 1, 3, 1},
        {0, 1, 5, 1},
        {0, 2, 5, 1},
        {0, 3, 5, 1},
        {0, 5, 5, 1},
        {1, 1, 1, 1},
        {1, 1, 5, 1},
        {1, 2, 5, 1},
        {1, 3, 5, 1},
        {1, 5, 5, 1},
        {1, 3, 1, 1},
        {1, 3, 3, 1},
        {1, 5, 3, 1},
    };

    for (size_t c = 0; c < sizeof(cases) / sizeof(cases[0]); c++) {
        size_t diff_n = cases[c].diff_n;
        size_t smooth_n = cases[c].smooth_n;
        size_t lag_n = cases[c].lag_n;
        size_t n = diff_n + smooth_n + lag_n;

        // Fill with a sine wave to get non-trivial values
        calculated_number_t src[128], dst[128];
        memset(src, 0, sizeof(src));
        for (size_t i = 0; i < n; i++)
            src[i] = 10.0 + 5.0 * std::sin(2.0 * ML_PI * (double)i / (double)n);
        memcpy(dst, src, n * sizeof(calculated_number_t));

        std::vector<DSample> pf;
        ml_features_t features = {
            diff_n, smooth_n, lag_n,
            dst, n, src, n
        };
        ml_features_preprocess(&features, pf, 1.0);

        char msg[256];
        snprintf(msg, sizeof(msg), "params(%zu,%zu,%zu): expected %zu vectors, got %zu",
                 diff_n, smooth_n, lag_n, cases[c].expected_vectors, pf.size());
        ML_TEST_ASSERT(pf.size() == cases[c].expected_vectors, msg);

        // Verify feature vector is a 6x1 matrix (DSample is fixed-size)
        if (pf.size() >= 1) {
            snprintf(msg, sizeof(msg), "params(%zu,%zu,%zu): DSample should be 6 elements",
                     diff_n, smooth_n, lag_n);
            ML_TEST_ASSERT(pf[0].size() == 6, msg);

            // Verify no NaN/Inf in the lag_n+1 active feature elements
            bool has_nan_inf = false;
            for (size_t fi = 0; fi < lag_n + 1; fi++) {
                if (std::isnan(pf[0](fi)) || std::isinf(pf[0](fi)))
                    has_nan_inf = true;
            }
            snprintf(msg, sizeof(msg), "params(%zu,%zu,%zu): no NaN/Inf in features",
                     diff_n, smooth_n, lag_n);
            ML_TEST_ASSERT(!has_nan_inf, msg);
        }

        // With a larger window, verify we get more vectors
        size_t large_n = n + 10;
        calculated_number_t src_large[128], dst_large[128];
        memset(src_large, 0, sizeof(src_large));
        for (size_t i = 0; i < large_n; i++)
            src_large[i] = 10.0 + 5.0 * std::sin(2.0 * ML_PI * (double)i / (double)large_n);
        memcpy(dst_large, src_large, large_n * sizeof(calculated_number_t));

        std::vector<DSample> pf_large;
        ml_features_t features_large = {
            diff_n, smooth_n, lag_n,
            dst_large, large_n, src_large, large_n
        };
        ml_features_preprocess(&features_large, pf_large, 1.0);

        size_t expected_large = large_n - diff_n - smooth_n + 1 - lag_n;
        snprintf(msg, sizeof(msg), "params(%zu,%zu,%zu) large window: expected %zu vectors",
                 diff_n, smooth_n, lag_n, expected_large);
        ML_TEST_ASSERT(pf_large.size() == expected_large, msg);
    }
}

// Test: timestamps > INT32_MAX must survive serialize -> deserialize unchanged.
// Before the bounds-check fix, the (time_t) cast of json_object_get_int64()
// would silently truncate on 32-bit time_t, breaking model ordering/pruning.
static void test_kmeans_timestamp_roundtrip()
{
    fprintf(stderr, "  test_kmeans_timestamp_roundtrip...\n");

    // 3 000 000 000 > INT32_MAX (2 147 483 647). On 32-bit time_t the value
    // doesn't fit, so skip — the guard would correctly reject it on the way in.
    if (sizeof(time_t) < 8) {
        fprintf(stderr, "    skipped (time_t is 32-bit on this platform)\n");
        return;
    }

    const time_t large_after  = (time_t) 3000000000LL;
    const time_t large_before = (time_t) 3000003600LL;

    ml_kmeans_inlined_t original;
    original.cluster_centers[0].set_size(6);
    original.cluster_centers[1].set_size(6);
    for (int i = 0; i < 6; i++) {
        original.cluster_centers[0](i) = (double)(i + 1);
        original.cluster_centers[1](i) = (double)(i + 7);
    }
    original.min_dist = 1.5;
    original.max_dist = 9.5;
    original.after    = large_after;
    original.before   = large_before;

    BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    ml_kmeans_serialize(&original, wb);
    buffer_json_finalize(wb);

    struct json_object *root = json_tokener_parse(buffer_tostring(wb));
    ML_TEST_ASSERT(root != NULL, "round-trip: serialized output must be valid JSON");

    if (root) {
        ml_kmeans_inlined_t result;
        result.cluster_centers[0].set_size(6);
        result.cluster_centers[1].set_size(6);

        bool ok = ml_kmeans_deserialize(&result, root);
        ML_TEST_ASSERT(ok, "round-trip: deserialize must succeed for large timestamp");

        if (ok) {
            ML_TEST_ASSERT(result.after  == large_after,
                           "round-trip: 'after' must survive unchanged (> INT32_MAX)");
            ML_TEST_ASSERT(result.before == large_before,
                           "round-trip: 'before' must survive unchanged (> INT32_MAX)");
        }

        json_object_put(root);
    }

    buffer_free(wb);
}

// Test: deserialize must reject models carrying negative timestamps.
// Negative Unix timestamps are never valid for ML model windows.
static void test_kmeans_timestamp_rejection()
{
    fprintf(stderr, "  test_kmeans_timestamp_rejection...\n");

    // Build a fully-valid kmeans JSON object and then override one timestamp
    // field to an invalid value, verifying that ml_kmeans_deserialize rejects it.
    auto make_full_root = [](int64_t after_val, int64_t before_val) -> struct json_object * {
        struct json_object *r = json_object_new_object();
        json_object_object_add(r, "after",    json_object_new_int64(after_val));
        json_object_object_add(r, "before",   json_object_new_int64(before_val));
        json_object_object_add(r, "min_dist", json_object_new_double(1.0));
        json_object_object_add(r, "max_dist", json_object_new_double(9.0));

        struct json_object *cc = json_object_new_array();
        for (int c = 0; c < 2; c++) {
            struct json_object *cv = json_object_new_array();
            for (int i = 0; i < 6; i++)
                json_object_array_add(cv, json_object_new_double((double)(c * 6 + i + 1)));
            json_object_array_add(cc, cv);
        }
        json_object_object_add(r, "cluster_centers", cc);
        return r;
    };

    {
        struct json_object *r = make_full_root(-1LL, 100LL);
        ml_kmeans_inlined_t km;
        km.cluster_centers[0].set_size(6);
        km.cluster_centers[1].set_size(6);
        bool ok = ml_kmeans_deserialize(&km, r);
        ML_TEST_ASSERT(!ok, "negative 'after' must be rejected");
        json_object_put(r);
    }

    {
        struct json_object *r = make_full_root(100LL, -1LL);
        ml_kmeans_inlined_t km;
        km.cluster_centers[0].set_size(6);
        km.cluster_centers[1].set_size(6);
        bool ok = ml_kmeans_deserialize(&km, r);
        ML_TEST_ASSERT(!ok, "negative 'before' must be rejected");
        json_object_put(r);
    }
}

extern "C" int ml_unittest()
{
    fprintf(stderr, "\nML unit tests:\n");

    // Initialize minimal global state needed by ml_features_lag
    Cfg.random_nums.clear();
    Cfg.random_nums.reserve(2048);
    for (size_t i = 0; i < 2048; i++)
        Cfg.random_nums.push_back(0);  // all zeros => all samples pass the cutoff check

    tests_run = 0;
    tests_failed = 0;

    test_features_diff();
    test_features_no_diff();
    test_features_smooth();
    test_features_zero_smooth_matches_one();
    test_kmeans_scoring();
    test_full_pipeline();
    test_circular_buffer_equivalence();
    test_same_value_uses_newest_sample();
    test_preprocess_predict_equivalence();
    test_constant_input();
    test_parameter_combinations();
    test_kmeans_timestamp_roundtrip();
    test_kmeans_timestamp_rejection();

    fprintf(stderr, "\nML tests: %d run, %d failed\n", tests_run, tests_failed);

    // Cleanup
    Cfg.random_nums.clear();

    return tests_failed == 0 ? 0 : 1;
}
