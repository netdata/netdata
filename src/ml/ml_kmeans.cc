// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml_kmeans.h"
#include "libnetdata/libnetdata.h"

#include <algorithm>
#include <limits>
#include <vector>

// gcc with libstdc++ may require this,
// but with libc++ it does not work correctly.
#if !defined(_LIBCPP_VERSION)
#include <cmath>
using std::isinf;
using std::isnan;
#endif

namespace {

// Guard against out-of-range timestamps when time_t is narrower than int64_t
// (e.g. 32-bit builds). if constexpr ensures the upper-bound check is elided
// at compile time on 64-bit platforms, avoiding constant-expression warnings.
static inline bool ml_int64_fits_nonnegative_time_t(int64_t value)
{
    if (value < 0)
        return false;
    if constexpr (sizeof(time_t) < sizeof(int64_t)) {
        // coverity[CONSTANT_EXPRESSION_RESULT] - on 64-bit, Coverity still analyzes
        // this dead branch; the check is genuinely reachable only on 32-bit builds
        // where time_t is narrower than int64_t.
        if (value > (int64_t) std::numeric_limits<time_t>::max())
            return false;
    }
    return true;
}

}

// The k=2 k-means below replaced the dlib library (https://dlib.net, Boost
// Software License; the agent used v19.24.8). It follows the behaviour of
// dlib's pick_initial_centers() (linear kernel, percentile 0.01) and
// find_clusters_using_kmeans(), down to the order of floating-point
// operations, so that models trained, stored or streamed by agents that used
// dlib keep producing the same results. Do not reorder the arithmetic.

static constexpr size_t ML_KMEANS_K = 2;

// Euclidean distance, summed from zero in row order.
static inline calculated_number_t ml_sample_distance(const DSample &a, const DSample &b)
{
    calculated_number_t sum = 0.0;
    for (size_t i = 0; i != DSample::size(); i++) {
        calculated_number_t d = a(i) - b(i);
        sum += d * d;
    }
    return std::sqrt(sum);
}

// Dot product, starting from the first product.
static inline calculated_number_t ml_sample_dot(const DSample &a, const DSample &b)
{
    calculated_number_t sum = a(0) * b(0);
    for (size_t i = 1; i != DSample::size(); i++)
        sum += a(i) * b(i);
    return sum;
}

// Deterministic seeding in the spirit of k-means++: the first sample is one
// center, and the other is the sample at the 99th percentile of squared
// distance from it (the 1% farthest are skipped as likely outliers).
static void ml_kmeans_pick_initial_centers(std::vector<DSample> &centers, const std::vector<DSample> &samples)
{
    struct candidate {
        size_t idx;
        calculated_number_t dist;
        bool operator<(const candidate &other) const { return dist < other.dist; }
    };

    const size_t n = samples.size();
    std::vector<candidate> scores(n, candidate{0, std::numeric_limits<calculated_number_t>::infinity()});

    centers.clear();
    centers.push_back(samples[0]);

    const DSample &first = centers[0];
    const calculated_number_t first_sq = ml_sample_dot(first, first);
    for (size_t s = 0; s != n; s++) {
        const calculated_number_t dist = first_sq + ml_sample_dot(samples[s], samples[s]) - 2 * ml_sample_dot(samples[s], first);
        if (dist < scores[s].dist)
            scores[s] = candidate{s, dist};
    }

    std::sort(scores.begin(), scores.end());

    const size_t pick = static_cast<size_t>(std::max(0.0, (double) n - (double) n * 0.01 - 1));
    centers.push_back(samples[scores[pick].idx]);
}

// Lloyd's algorithm: assign every sample to its nearest center, move each
// center to the mean of its samples, stop when no assignment changes.
static void ml_kmeans_find_clusters(std::vector<DSample> &centers, const std::vector<DSample> &samples, unsigned max_iters)
{
    const size_t n = samples.size();
    std::vector<size_t> assignments(n, n);

    bool changed = true;
    for (unsigned iter = 0; changed && iter < max_iters; iter++) {
        changed = false;
        size_t counts[ML_KMEANS_K] = {};

        for (size_t i = 0; i != n; i++) {
            calculated_number_t best_dist = std::numeric_limits<calculated_number_t>::max();
            size_t best = 0;
            for (size_t c = 0; c != ML_KMEANS_K; c++) {
                calculated_number_t dist = ml_sample_distance(centers[c], samples[i]);
                if (dist < best_dist) {
                    best_dist = dist;
                    best = c;
                }
            }

            if (assignments[i] != best) {
                changed = true;
                assignments[i] = best;
            }
            counts[best]++;
        }

        for (DSample &center : centers)
            center.fill(0.0);

        for (size_t i = 0; i != n; i++) {
            DSample &center = centers[assignments[i]];
            for (size_t r = 0; r != DSample::size(); r++)
                center(r) += samples[i](r);
        }

        for (size_t c = 0; c != ML_KMEANS_K; c++) {
            if (counts[c] == 0)
                continue;

            const calculated_number_t scale = 1.0 / (calculated_number_t) counts[c];
            for (calculated_number_t &x : centers[c])
                x = x * scale;
        }
    }
}

void
ml_kmeans_init(ml_kmeans_t *kmeans)
{
    kmeans->cluster_centers.reserve(2);
    kmeans->cluster_centers.clear();
    kmeans->min_dist = std::numeric_limits<calculated_number_t>::max();
    kmeans->max_dist = 0.0;
}

void
ml_kmeans_train(ml_kmeans_t *kmeans, const std::vector<DSample> &preprocessed_features, unsigned max_iters, time_t after, time_t before)
{
    kmeans->after = after;
    kmeans->before = before;

    // Distances are never negative, so max_dist starts at 0: when every
    // distance is 0 the bounds end up equal and the model is flat.
    kmeans->min_dist = std::numeric_limits<calculated_number_t>::max();
    kmeans->max_dist = 0.0;

    kmeans->cluster_centers.clear();

    if (preprocessed_features.size() < 2) {
        netdata_log_error("ml_kmeans_train: not enough features to train kmeans (size=%zu)",
                          preprocessed_features.size());
        return;
    }

    ml_kmeans_pick_initial_centers(kmeans->cluster_centers, preprocessed_features);
    ml_kmeans_find_clusters(kmeans->cluster_centers, preprocessed_features, max_iters);

    for (const auto &preprocessed_feature : preprocessed_features) {
        calculated_number_t mean_dist = 0.0;

        for (const auto &cluster_center : kmeans->cluster_centers) {
            mean_dist += ml_sample_distance(cluster_center, preprocessed_feature);
        }

        mean_dist /= kmeans->cluster_centers.size();

        if (mean_dist < kmeans->min_dist)
            kmeans->min_dist = mean_dist;

        if (mean_dist > kmeans->max_dist)
            kmeans->max_dist = mean_dist;
    }
}

// A model is flat when its spread (max_dist - min_dist) is below this
// fraction of max_dist. Most often the spread is zero or a few ULP of
// rounding, because every training point sat on a centre (zeros plus one
// repeated spike), so every point had the same mean distance to the centres.
// Dividing by such a spread makes the score jump to 100 on any tiny wobble,
// so a flat model has no usable scale. Measured on a large parent,
// rounding-only spreads stay below ~1e-11 of max_dist and real ones start
// above ~1e-8.
#define ML_KMEANS_FLAT_RELATIVE_SPREAD 1e-9

// Returns NaN when the model is flat: it abstains, and the dimension's other
// models decide whether a sample is anomalous.
calculated_number_t
ml_kmeans_anomaly_score(const ml_kmeans_inlined_t *inlined_km, const DSample &DS)
{
    calculated_number_t spread = inlined_km->max_dist - inlined_km->min_dist;
    if (inlined_km->max_dist == inlined_km->min_dist ||
        (spread > 0.0 && spread < ML_KMEANS_FLAT_RELATIVE_SPREAD * std::abs(inlined_km->max_dist)))
        return std::numeric_limits<calculated_number_t>::quiet_NaN();

    // Training used to start max_dist at DBL_MIN, so a model whose distances
    // were all 0 was stored in ml.db as (0, DBL_MIN) instead of (0, 0): treat
    // it as flat too. Streamed models print DBL_MIN as 0 and arrive as (0, 0).
    if (inlined_km->min_dist == 0.0 && inlined_km->max_dist == std::numeric_limits<calculated_number_t>::min())
        return std::numeric_limits<calculated_number_t>::quiet_NaN();

    calculated_number_t mean_dist = 0.0;
    for (const auto &CC: inlined_km->cluster_centers)
        mean_dist += ml_sample_distance(CC, DS);

    mean_dist /= inlined_km->cluster_centers.size();

    calculated_number_t anomaly_score = 100.0 * std::abs((mean_dist - inlined_km->min_dist) / spread);
    return (anomaly_score > 100.0) ? 100.0 : anomaly_score;
}

static void ml_buffer_json_member_add_double(BUFFER *wb, const char *key, calculated_number_t cn) {
    if (!isnan(cn) && !isinf(cn)) {
        buffer_json_member_add_double(wb, key, cn);
        return;
    }

    const char *classification = nullptr;
    if (isnan(cn)) {
        classification = "nan";
    } else if (isinf(cn)) {
        if (cn > 0) {
            classification = "+inf";
        } else {
            classification = "-inf";
        }
    }

    buffer_json_member_add_string(wb, key, classification);
}

static void ml_buffer_json_add_array_item_double(BUFFER *wb, calculated_number_t cn) {
    if (!isnan(cn) && !isinf(cn)) {
        buffer_json_add_array_item_double(wb, cn);
        return;
    }

    const char *classification = nullptr;
    if (isnan(cn)) {
        classification = "nan";
    } else if (isinf(cn)) {
        if (cn > 0) {
            classification = "+inf";
        } else if (cn < 0) {
            classification = "-inf";
        }
    }

    buffer_json_add_array_item_string(wb, classification);
}

bool ml_json_parse_double(struct json_object *jo, calculated_number_t *cn) {
    switch(json_object_get_type(jo)) {
        case json_type_string: {
            const char *s = json_object_get_string(jo);
            if (strcmp(s, "nan") == 0) {
                *cn = NAN;
                return true;
            }
            else if (strcmp(s, "+inf") == 0) {
                *cn = INFINITY;
                return true;
            }
            else if (strcmp(s, "-inf") == 0) {
                *cn = -INFINITY;
                return true;
            }

            return false;
        }
        case json_type_double: {
            *cn = json_object_get_double(jo);
            return true;
        }
        case json_type_int: {
            *cn = json_object_get_int64(jo);
            return true;
        }
        default:
            return false;
    }
}

void
ml_kmeans_serialize(const ml_kmeans_inlined_t *inlined_km, BUFFER *wb)
{
    buffer_json_member_add_uint64(wb, "after", inlined_km->after);
    buffer_json_member_add_uint64(wb, "before", inlined_km->before);

    ml_buffer_json_member_add_double(wb, "min_dist", inlined_km->min_dist);
    ml_buffer_json_member_add_double(wb, "max_dist", inlined_km->max_dist);

    buffer_json_member_add_array(wb, "cluster_centers");
    for (const auto &cc: inlined_km->cluster_centers) {
        buffer_json_add_array_item_array(wb);

        for (const auto &d: cc) {
            ml_buffer_json_add_array_item_double(wb, d);
        }

        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);
}

bool ml_kmeans_deserialize(ml_kmeans_inlined_t *inlined_km, struct json_object *root)
{
    struct json_object *value;

    if (!json_object_object_get_ex(root, "after", &value)) {
        netdata_log_error("Failed to deserialize kmeans: missing key 'after'");
        return false;
    }
    if (!json_object_is_type(value, json_type_int)) {
        netdata_log_error("Failed to deserialize kmeans: failed to parse int for 'after'");
        return false;
    }
    int64_t raw_after = json_object_get_int64(value);
    // Timestamps must be non-negative Unix epoch seconds and fit in time_t.
    if (!ml_int64_fits_nonnegative_time_t(raw_after)) {
        netdata_log_error("Failed to deserialize kmeans: out-of-range value for 'after': %" PRId64, raw_after);
        return false;
    }
    inlined_km->after = (time_t) raw_after;

    if (!json_object_object_get_ex(root, "before", &value)) {
        netdata_log_error("Failed to deserialize kmeans: missing key 'before'");
        return false;
    }
    if (!json_object_is_type(value, json_type_int)) {
        netdata_log_error("Failed to deserialize kmeans: failed to parse int for 'before'");
        return false;
    }
    int64_t raw_before = json_object_get_int64(value);
    // Same contract as 'after': non-negative and fits in time_t.
    if (!ml_int64_fits_nonnegative_time_t(raw_before)) {
        netdata_log_error("Failed to deserialize kmeans: out-of-range value for 'before': %" PRId64, raw_before);
        return false;
    }
    inlined_km->before = (time_t) raw_before;

    if (!json_object_object_get_ex(root, "min_dist", &value)) {
        netdata_log_error("Failed to deserialize kmeans: missing key 'min_dist'");
        return false;
    }
    if (!ml_json_parse_double(value, &inlined_km->min_dist)) {
        netdata_log_error("Failed to deserialize kmeans: failed to parse double for 'min_dist'");
        return false;
    }

    if (!json_object_object_get_ex(root, "max_dist", &value)) {
        netdata_log_error("Failed to deserialize kmeans: missing key 'max_dist'");
        return false;
    }
    if (!ml_json_parse_double(value, &inlined_km->max_dist)) {
        netdata_log_error("Failed to deserialize kmeans: failed to parse double for 'max_dist'");
        return false;
    }

    struct json_object *cc_root;
    if (!json_object_object_get_ex(root, "cluster_centers", &cc_root)) {
        netdata_log_error("Failed to deserialize kmeans: missing key 'cluster_centers'");
        return false;
    }
    if (!json_object_is_type(cc_root, json_type_array)) {
        netdata_log_error("Failed to deserialize kmeans: failed to parse array for 'cluster_centers'");
        return false;
    }

    size_t num_centers = json_object_array_length(cc_root);
    if (num_centers != 2) {
        netdata_log_error("Failed to deserialize kmeans: expected cluster centers array of size 2");
        return false;
    }

    for (size_t i = 0; i < num_centers; i++) {
        struct json_object *cc_obj = json_object_array_get_idx(cc_root, i);
        if (!cc_obj || !json_object_is_type(cc_obj, json_type_array)) {
            netdata_log_error("Failed to deserialize kmeans: expected cluster center array");
            return false;
        }

        size_t size = json_object_array_length(cc_obj);
        if (size != DSample::size()) {
            netdata_log_error("Failed to deserialize kmeans: expected cluster center array of size 6");
            return false;
        }

        for (size_t j = 0; j < size; j++) {
            struct json_object *value = json_object_array_get_idx(cc_obj, j);
            calculated_number_t cn;

            if (!ml_json_parse_double(value, &cn)) {
                netdata_log_error("Failed to deserialize kmeans: failed to parse double %zu for cluster center %zu", j, i);
                return false;
            }

            inlined_km->cluster_centers[i](j) = cn;
        }
    }

    return true;
}
