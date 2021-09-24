// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "ml-private.h"

using namespace ml;

/*
 * Global configuration instance to be shared between training and
 * prediction threads.
 */
Config ml::Cfg;

template <typename T>
static T clamp(const T& Value, const T& Min, const T& Max) {
  return std::max(Min, std::min(Value, Max));
}

/*
 * Initialize global configuration variable.
 */
void Config::readMLConfig(void) {
    const char *ConfigSectionML = "ml";

    Cfg.EnableAnomalyDetection = config_get_boolean(ConfigSectionML, "enabled", false);
    if (!Cfg.EnableAnomalyDetection)
        return;

    /*
     * Read values
     */

    int MaxTrainSecs = config_get_number(ConfigSectionML, "maximum num secs to train", 4 * 3600);
    int MinTrainSecs = config_get_number(ConfigSectionML, "minimum num secs to train", 1 * 3600);
    int TrainEvery = config_get_number(ConfigSectionML, "train every secs", 1 * 3600);

    unsigned DiffN = config_get_number(ConfigSectionML, "num samples to diff", 1);
    unsigned SmoothN = config_get_number(ConfigSectionML, "num samples to smooth", 3);
    unsigned LagN = config_get_number(ConfigSectionML, "num samples to lag", 5);

    unsigned MaxKMeansIters = config_get_number(ConfigSectionML, "maximum number of k-means iterations", 1000);

    double DimensionAnomalyScoreThreshold = config_get_float(ConfigSectionML, "dimension anomaly score threshold", 0.99);
    double HostAnomalyRateThreshold = config_get_float(ConfigSectionML, "host anomaly rate threshold", 0.01);

    double ADMinWindowSize = config_get_float(ConfigSectionML, "minimum window size", 30);
    double ADMaxWindowSize = config_get_float(ConfigSectionML, "maximum window size", 600);
    double ADIdleWindowSize = config_get_float(ConfigSectionML, "idle window size", 30);
    double ADWindowRateThreshold = config_get_float(ConfigSectionML, "window minimum anomaly rate", 0.25);
    double ADDimensionRateThreshold = config_get_float(ConfigSectionML, "anomaly event min dimension rate threshold", 0.05);

    std::string HostsToSkip = config_get(ConfigSectionML, "hosts to skip from training", "!*");
    std::string ChartsToSkip = config_get(ConfigSectionML, "charts to skip from training",
            "!system.* !cpu.* !mem.* !disk.* !disk_* "
            "!ip.* !ipv4.* !ipv6.* !net.* !net_* !netfilter.* "
            "!services.* !apps.* !groups.* !user.* !ebpf.* !netdata.* *");

    std::stringstream SS;
    SS << netdata_configured_cache_dir << "/anomaly-detection.db";
    Cfg.AnomalyDBPath = SS.str();

    /*
     * Clamp
     */

    MaxTrainSecs = clamp(MaxTrainSecs, 1 * 3600, 6 * 3600);
    MinTrainSecs = clamp(MinTrainSecs, 1 * 3600, 6 * 3600);
    TrainEvery = clamp(TrainEvery, 1 * 3600, 6 * 3600);

    DiffN = clamp(DiffN, 0u, 1u);
    SmoothN = clamp(SmoothN, 0u, 5u);
    LagN = clamp(LagN, 0u, 5u);

    MaxKMeansIters = clamp(MaxKMeansIters, 500u, 1000u);

    DimensionAnomalyScoreThreshold = clamp(DimensionAnomalyScoreThreshold, 0.01, 5.00);
    HostAnomalyRateThreshold = clamp(HostAnomalyRateThreshold, 0.01, 1.0);

    ADMinWindowSize = clamp(ADMinWindowSize, 30.0, 300.0);
    ADMaxWindowSize = clamp(ADMaxWindowSize, 60.0, 900.0);
    ADIdleWindowSize = clamp(ADIdleWindowSize, 30.0, 900.0);
    ADWindowRateThreshold = clamp(ADWindowRateThreshold, 0.01, 0.99);
    ADDimensionRateThreshold = clamp(ADDimensionRateThreshold, 0.01, 0.99);

    /*
     * Validate
     */

    if (MinTrainSecs >= MaxTrainSecs) {
        error("invalid min/max train seconds found (%d >= %d)", MinTrainSecs, MaxTrainSecs);

        MinTrainSecs = 1 * 3600;
        MaxTrainSecs = 4 * 3600;
    }

    ADMinWindowSize = clamp(ADMinWindowSize, 30.0, 300.0);
    ADMaxWindowSize = clamp(ADMaxWindowSize, 60.0, 900.0);
    if (ADMinWindowSize >= ADMaxWindowSize) {
        error("invalid min/max anomaly window size found (%lf >= %lf)", ADMinWindowSize, ADMaxWindowSize);

        ADMinWindowSize = 30.0;
        ADMaxWindowSize = 600.0;
    }

    /*
     * Assign to config instance
     */

    Cfg.MaxTrainSecs = Seconds{clamp(MaxTrainSecs, 1 * 3600, 6 * 3600)};
    Cfg.MinTrainSecs = Seconds{clamp(MinTrainSecs, 1 * 3600, 6 * 3600)};
    Cfg.TrainEvery = Seconds{clamp(TrainEvery, 1 * 3600, 6 * 3600)};

    Cfg.DiffN = clamp(DiffN, 0u, 1u);
    Cfg.SmoothN = clamp(SmoothN, 0u, 5u);
    Cfg.LagN = clamp(LagN, 0u, 5u);

    Cfg.MaxKMeansIters = clamp(MaxKMeansIters, 500u, 1000u);

    Cfg.DimensionAnomalyScoreThreshold = clamp(DimensionAnomalyScoreThreshold, 0.01, 5.00);
    Cfg.HostAnomalyRateThreshold = clamp(HostAnomalyRateThreshold, 0.01, 1.0);

    Cfg.ADMinWindowSize = clamp(ADMinWindowSize, 30.0, 300.0);
    Cfg.ADMaxWindowSize = clamp(ADMaxWindowSize, 60.0, 900.0);
    Cfg.ADIdleWindowSize = clamp(ADIdleWindowSize, 30.0, 900.0);
    Cfg.ADWindowRateThreshold = clamp(ADWindowRateThreshold, 0.01, 0.99);
    Cfg.ADDimensionRateThreshold = clamp(ADDimensionRateThreshold, 0.01, 0.99);

    Cfg.SP_HostsToSkip = simple_pattern_create(HostsToSkip.c_str(), NULL, SIMPLE_PATTERN_EXACT);
    Cfg.SP_ChartsToSkip = simple_pattern_create(ChartsToSkip.c_str(), NULL, SIMPLE_PATTERN_EXACT);
}
