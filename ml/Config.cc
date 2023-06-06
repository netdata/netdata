// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml-private.h"

/*
 * Global configuration instance to be shared between training and
 * prediction threads.
 */
ml_config_t Cfg;

template <typename T>
static T clamp(const T& Value, const T& Min, const T& Max) {
  return std::max(Min, std::min(Value, Max));
}

/*
 * Initialize global configuration variable.
 */
void ml_config_load(ml_config_t *cfg) {
    const char *config_section_ml = CONFIG_SECTION_ML;

    bool enable_anomaly_detection = config_get_boolean(config_section_ml, "enabled", true);

    /*
     * Read values
     */

    unsigned max_train_samples = config_get_number(config_section_ml, "maximum num samples to train", 6 * 3600);
    unsigned min_train_samples = config_get_number(config_section_ml, "minimum num samples to train", 1 * 900);
    unsigned train_every = config_get_number(config_section_ml, "train every", 3 * 3600);
    unsigned num_models_to_use = config_get_number(config_section_ml, "number of models per dimension", 9);

    unsigned diff_n = config_get_number(config_section_ml, "num samples to diff", 1);
    unsigned smooth_n = config_get_number(config_section_ml, "num samples to smooth", 3);
    unsigned lag_n = config_get_number(config_section_ml, "num samples to lag", 5);

    double random_sampling_ratio = config_get_float(config_section_ml, "random sampling ratio", 1.0 / 5.0 /* default lag_n */);
    unsigned max_kmeans_iters = config_get_number(config_section_ml, "maximum number of k-means iterations", 1000);

    double dimension_anomaly_rate_threshold = config_get_float(config_section_ml, "dimension anomaly score threshold", 0.99);

    double host_anomaly_rate_threshold = config_get_float(config_section_ml, "host anomaly rate threshold", 1.0);
    std::string anomaly_detection_grouping_method = config_get(config_section_ml, "anomaly detection grouping method", "average");
    time_t anomaly_detection_query_duration = config_get_number(config_section_ml, "anomaly detection grouping duration", 5 * 60);

    size_t num_training_threads = config_get_number(config_section_ml, "num training threads", 4);
    size_t flush_models_batch_size = config_get_number(config_section_ml, "flush models batch size", 128);

    size_t suppression_window = config_get_number(config_section_ml, "dimension anomaly rate suppression window", 900);
    size_t suppression_threshold = config_get_number(config_section_ml, "dimension anomaly rate suppression threshold", suppression_window / 2);

    bool enable_statistics_charts = config_get_boolean(config_section_ml, "enable statistics charts", true);

    /*
     * Clamp
     */

    max_train_samples = clamp<unsigned>(max_train_samples, 1 * 3600, 24 * 3600);
    min_train_samples = clamp<unsigned>(min_train_samples, 1 * 900, 6 * 3600);
    train_every = clamp<unsigned>(train_every, 1 * 3600, 6 * 3600);
    num_models_to_use = clamp<unsigned>(num_models_to_use, 1, 7 * 24);

    diff_n = clamp(diff_n, 0u, 1u);
    smooth_n = clamp(smooth_n, 0u, 5u);
    lag_n = clamp(lag_n, 1u, 5u);

    random_sampling_ratio = clamp(random_sampling_ratio, 0.2, 1.0);
    max_kmeans_iters = clamp(max_kmeans_iters, 500u, 1000u);

    dimension_anomaly_rate_threshold = clamp(dimension_anomaly_rate_threshold, 0.01, 5.00);

    host_anomaly_rate_threshold = clamp(host_anomaly_rate_threshold, 0.1, 10.0);
    anomaly_detection_query_duration = clamp<time_t>(anomaly_detection_query_duration, 60, 15 * 60);

    num_training_threads = clamp<size_t>(num_training_threads, 1, 128);
    flush_models_batch_size = clamp<size_t>(flush_models_batch_size, 8, 512);

    suppression_window = clamp<size_t>(suppression_window, 1, max_train_samples);
    suppression_threshold = clamp<size_t>(suppression_threshold, 1, suppression_window);

     /*
     * Validate
     */

    if (min_train_samples >= max_train_samples) {
        error("invalid min/max train samples found (%u >= %u)", min_train_samples, max_train_samples);

        min_train_samples = 1 * 3600;
        max_train_samples = 6 * 3600;
    }

    /*
     * Assign to config instance
     */

    cfg->enable_anomaly_detection = enable_anomaly_detection;

    cfg->max_train_samples = max_train_samples;
    cfg->min_train_samples = min_train_samples;
    cfg->train_every = train_every;

    cfg->num_models_to_use = num_models_to_use;

    cfg->diff_n = diff_n;
    cfg->smooth_n = smooth_n;
    cfg->lag_n = lag_n;

    cfg->random_sampling_ratio = random_sampling_ratio;
    cfg->max_kmeans_iters = max_kmeans_iters;

    cfg->host_anomaly_rate_threshold = host_anomaly_rate_threshold;
    cfg->anomaly_detection_grouping_method =
        time_grouping_parse(anomaly_detection_grouping_method.c_str(), RRDR_GROUPING_AVERAGE);
    cfg->anomaly_detection_query_duration = anomaly_detection_query_duration;
    cfg->dimension_anomaly_score_threshold = dimension_anomaly_rate_threshold;

    cfg->hosts_to_skip = config_get(config_section_ml, "hosts to skip from training", "!*");
    cfg->sp_host_to_skip = simple_pattern_create(cfg->hosts_to_skip.c_str(), NULL, SIMPLE_PATTERN_EXACT, true);

    // Always exclude anomaly_detection charts from training.
    cfg->charts_to_skip = "anomaly_detection.* ";
    cfg->charts_to_skip += config_get(config_section_ml, "charts to skip from training", "netdata.*");
    cfg->sp_charts_to_skip = simple_pattern_create(cfg->charts_to_skip.c_str(), NULL, SIMPLE_PATTERN_EXACT, true);

    cfg->stream_anomaly_detection_charts = config_get_boolean(config_section_ml, "stream anomaly detection charts", true);

    cfg->num_training_threads = num_training_threads;
    cfg->flush_models_batch_size = flush_models_batch_size;

    cfg->suppression_window = suppression_window;
    cfg->suppression_threshold = suppression_threshold;

    cfg->enable_statistics_charts = enable_statistics_charts;
}
