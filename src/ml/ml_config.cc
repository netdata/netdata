// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml_config.h"

static void ml_config_migrate() {
    const char *config_section_ml = CONFIG_SECTION_ML;
    
    // Check if migration is needed by looking for old keys
    bool has_old_keys = false;
    if (inicfg_exists(&netdata_config, config_section_ml, "maximum num samples to train") ||
        inicfg_exists(&netdata_config, config_section_ml, "minimum num samples to train") ||
        inicfg_exists(&netdata_config, config_section_ml, "num samples to diff") ||
        inicfg_exists(&netdata_config, config_section_ml, "num samples to smooth") ||
        inicfg_exists(&netdata_config, config_section_ml, "num samples to lag") ||
        inicfg_exists(&netdata_config, config_section_ml, "random sampling ratio")) {
        has_old_keys = true;
    }
    
    // Check if new keys already exist (user manually migrated)
    bool has_new_keys = false;
    if (inicfg_exists(&netdata_config, config_section_ml, "training window") ||
        inicfg_exists(&netdata_config, config_section_ml, "max training vectors")) {
        has_new_keys = true;
    }
    
    // Only migrate if we have old keys but no new keys
    if (!has_old_keys || has_new_keys) {
        return;
    }
    
    // Get the user's "high resolution" setting
    // This is what their configuration was designed for
    time_t global_update_every = nd_profile.update_every;
    
    // Read all old configuration values with defaults
    // Users may have changed only some values, so we need proper defaults
    unsigned old_max_train_samples = inicfg_get_number(&netdata_config, config_section_ml, 
                                                      "maximum num samples to train", 21600);
    unsigned old_min_train_samples = inicfg_get_number(&netdata_config, config_section_ml, 
                                                      "minimum num samples to train", 900);
    unsigned old_train_every = inicfg_get_duration_seconds(&netdata_config, config_section_ml, 
                                                          "train every", 10800);
    unsigned old_diff_n = inicfg_get_number(&netdata_config, config_section_ml, 
                                           "num samples to diff", 1);
    unsigned old_smooth_n = inicfg_get_number(&netdata_config, config_section_ml, 
                                             "num samples to smooth", 3);
    unsigned old_lag_n = inicfg_get_number(&netdata_config, config_section_ml, 
                                          "num samples to lag", 5);
    double old_sampling_ratio = inicfg_get_double(&netdata_config, config_section_ml, 
                                                 "random sampling ratio", 0.2);
    
    // Calculate time-based equivalents
    // These preserve the exact behavior the user had configured
    time_t training_window = old_max_train_samples * global_update_every;
    time_t min_training_window = old_min_train_samples * global_update_every;
    
    // Calculate target training vectors based on old pipeline
    // Account for data reduction from diff, smooth, and sampling
    size_t effective_samples = old_max_train_samples;
    if (old_diff_n > 0) effective_samples--;  // Lose one sample to differencing
    size_t max_training_vectors = (size_t)(effective_samples * old_sampling_ratio);
    
    // Write new configuration values
    char window_str[32];
    snprintf(window_str, sizeof(window_str), "%ldh", training_window / 3600);
    inicfg_set(&netdata_config, config_section_ml, "training window", window_str);
    
    snprintf(window_str, sizeof(window_str), "%ldm", min_training_window / 60);
    inicfg_set(&netdata_config, config_section_ml, "min training window", window_str);
    
    inicfg_set_number(&netdata_config, config_section_ml, "max training vectors", max_training_vectors);
    inicfg_set_number(&netdata_config, config_section_ml, "max samples to smooth", old_smooth_n);
    
    // Migrate unchanged values
    inicfg_set_duration_seconds(&netdata_config, config_section_ml, "train every", old_train_every);
    inicfg_set_number(&netdata_config, config_section_ml, "num samples to diff", old_diff_n);
    inicfg_set_number(&netdata_config, config_section_ml, "num samples to lag", old_lag_n);
    
    // Mark old keys as migrated by moving them to avoid showing in netdata.conf
    // This uses Netdata's config migration pattern
    inicfg_move(&netdata_config, config_section_ml, "maximum num samples to train",
                config_section_ml, "obsolete maximum num samples to train");
    inicfg_move(&netdata_config, config_section_ml, "minimum num samples to train",
                config_section_ml, "obsolete minimum num samples to train");
    inicfg_move(&netdata_config, config_section_ml, "num samples to smooth",
                config_section_ml, "obsolete num samples to smooth");
    inicfg_move(&netdata_config, config_section_ml, "random sampling ratio",
                config_section_ml, "obsolete random sampling ratio");
    
    // Log the migration
    nd_log(NDLS_DAEMON, NDLP_NOTICE,
           "ML configuration migrated from sample-based to time-based:");
    nd_log(NDLS_DAEMON, NDLP_NOTICE,
           "  Training window: %ld seconds (%ld hours) - was %u samples at %ld second intervals",
           training_window, training_window / 3600, old_max_train_samples, global_update_every);
    nd_log(NDLS_DAEMON, NDLP_NOTICE,
           "  Target training vectors: %zu - calculated from smoothing and sampling",
           max_training_vectors);
}

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

    // Migrate old configuration if needed
    ml_config_migrate();

    int enable_anomaly_detection = inicfg_get_boolean_ondemand(&netdata_config, config_section_ml, "enabled", nd_profile.ml_enabled);

    /*
     * Read values
     */

    time_t training_window = inicfg_get_duration_seconds(&netdata_config, config_section_ml, "training window", 6 * 3600);
    time_t min_training_window = inicfg_get_duration_seconds(&netdata_config, config_section_ml, "min training window", 15 * 60);
    size_t max_training_vectors = inicfg_get_number(&netdata_config, config_section_ml, "max training vectors", 1440);
    size_t max_samples_to_smooth = inicfg_get_number(&netdata_config, config_section_ml, "max samples to smooth", 3);
    unsigned train_every = inicfg_get_duration_seconds(&netdata_config, config_section_ml, "train every", 3 * 3600);

    unsigned num_models_to_use = inicfg_get_number(&netdata_config, config_section_ml, "number of models per dimension", 18);
    unsigned delete_models_older_than = inicfg_get_duration_seconds(&netdata_config, config_section_ml, "delete models older than", 60 * 60 * 24 * 7);

    unsigned diff_n = inicfg_get_number(&netdata_config, config_section_ml, "num samples to diff", 1);
    unsigned lag_n = inicfg_get_number(&netdata_config, config_section_ml, "num samples to lag", 5);

    unsigned max_kmeans_iters = inicfg_get_number(&netdata_config, config_section_ml, "maximum number of k-means iterations", 1000);

    double dimension_anomaly_rate_threshold = inicfg_get_double(&netdata_config, config_section_ml, "dimension anomaly score threshold", 0.99);

    double host_anomaly_rate_threshold = inicfg_get_double(&netdata_config, config_section_ml, "host anomaly rate threshold", 1.0);
    std::string anomaly_detection_grouping_method = inicfg_get(&netdata_config, config_section_ml, "anomaly detection grouping method", "average");
    time_t anomaly_detection_query_duration = inicfg_get_duration_seconds(&netdata_config, config_section_ml, "anomaly detection grouping duration", 5 * 60);

    size_t num_worker_threads = netdata_conf_is_parent() ? netdata_conf_cpus() / 4 : 1;
    if (num_worker_threads < 1) num_worker_threads = 1;
    else if (num_worker_threads > 256) num_worker_threads = 256;
    num_worker_threads = inicfg_get_number(&netdata_config, config_section_ml, "num training threads", num_worker_threads);

    size_t flush_models_batch_size = inicfg_get_number(&netdata_config, config_section_ml, "flush models batch size", 256);

    size_t suppression_window =
        inicfg_get_duration_seconds(&netdata_config, config_section_ml, "dimension anomaly rate suppression window", 900);

    size_t suppression_threshold =
        inicfg_get_number(&netdata_config, config_section_ml, "dimension anomaly rate suppression threshold", suppression_window / 2);

    bool enable_statistics_charts = inicfg_get_boolean(&netdata_config, config_section_ml, "enable statistics charts", true);

    /*
     * Clamp
     */

    training_window = clamp<time_t>(training_window, 1 * 3600, 24 * 3600);
    min_training_window = clamp<time_t>(min_training_window, 1 * 900, 6 * 3600);
    train_every = clamp<unsigned>(train_every, 1 * 3600, 6 * 3600);

    num_models_to_use = clamp<unsigned>(num_models_to_use, 1, 7 * 24);
    delete_models_older_than = clamp<unsigned>(delete_models_older_than, 60 * 60 * 24 * 1, 60 * 60 * 24 * 7);

    diff_n = clamp(diff_n, 0u, 1u);
    max_samples_to_smooth = clamp<size_t>(max_samples_to_smooth, 0, 5);
    lag_n = clamp(lag_n, 1u, 5u);

    max_kmeans_iters = clamp(max_kmeans_iters, 500u, 1000u);

    dimension_anomaly_rate_threshold = clamp(dimension_anomaly_rate_threshold, 0.01, 5.00);

    host_anomaly_rate_threshold = clamp(host_anomaly_rate_threshold, 0.1, 10.0);
    anomaly_detection_query_duration = clamp<time_t>(anomaly_detection_query_duration, 60, 15 * 60);

    num_worker_threads = clamp<size_t>(num_worker_threads, 4, netdata_conf_cpus());
    flush_models_batch_size = clamp<size_t>(flush_models_batch_size, 8, 512);

    suppression_window = clamp<size_t>(suppression_window, 1, training_window);
    suppression_threshold = clamp<size_t>(suppression_threshold, 1, suppression_window);

     /*
     * Validate
     */

    if (min_training_window >= training_window) {
        netdata_log_error("invalid min/max training window found (%ld >= %ld)", min_training_window, training_window);

        min_training_window = 1 * 3600;
        training_window = 6 * 3600;
    }

    /*
     * Assign to config instance
     */

    cfg->enable_anomaly_detection = enable_anomaly_detection;

    cfg->training_window = training_window;
    cfg->min_training_window = min_training_window;
    cfg->max_training_vectors = max_training_vectors;
    cfg->max_samples_to_smooth = max_samples_to_smooth;
    cfg->train_every = train_every;

    cfg->num_models_to_use = num_models_to_use;
    cfg->delete_models_older_than = delete_models_older_than;

    cfg->diff_n = diff_n;
    cfg->lag_n = lag_n;

    cfg->max_kmeans_iters = max_kmeans_iters;

    cfg->host_anomaly_rate_threshold = host_anomaly_rate_threshold;
    cfg->anomaly_detection_grouping_method =
        time_grouping_parse(anomaly_detection_grouping_method.c_str(), RRDR_GROUPING_AVERAGE);
    cfg->anomaly_detection_query_duration = anomaly_detection_query_duration;
    cfg->dimension_anomaly_score_threshold = dimension_anomaly_rate_threshold;

    cfg->hosts_to_skip = inicfg_get(&netdata_config, config_section_ml, "hosts to skip from training", "!*");
    cfg->sp_host_to_skip = simple_pattern_create(cfg->hosts_to_skip.c_str(), NULL, SIMPLE_PATTERN_EXACT, true);

    // Always exclude anomaly_detection charts from training.
    cfg->charts_to_skip = "anomaly_detection.* ";
    cfg->charts_to_skip += inicfg_get(&netdata_config, config_section_ml, "charts to skip from training", "netdata.*");
    cfg->sp_charts_to_skip = simple_pattern_create(cfg->charts_to_skip.c_str(), NULL, SIMPLE_PATTERN_EXACT, true);

    cfg->stream_anomaly_detection_charts = inicfg_get_boolean(&netdata_config, config_section_ml, "stream anomaly detection charts", true);

    cfg->num_worker_threads = num_worker_threads;
    cfg->flush_models_batch_size = flush_models_batch_size;

    cfg->suppression_window = suppression_window;
    cfg->suppression_threshold = suppression_threshold;

    cfg->enable_statistics_charts = enable_statistics_charts;

    if (cfg->enable_anomaly_detection == CONFIG_BOOLEAN_AUTO && default_rrd_memory_mode != RRD_DB_MODE_DBENGINE) {
        Cfg.enable_anomaly_detection = 0;
        inicfg_set_boolean(&netdata_config, config_section_ml, "enabled", CONFIG_BOOLEAN_NO);
        return;
    }
}
