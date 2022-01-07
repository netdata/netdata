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
    const char *ConfigSectionML = CONFIG_SECTION_ML;

    bool EnableAnomalyDetection = config_get_boolean(ConfigSectionML, "enabled", true);

    unsigned long SaveAnomalyPercentageEvery = 15 * 60;

    /*
     * Read values
     */

    unsigned MaxTrainSamples = config_get_number(ConfigSectionML, "maximum num samples to train", 4 * 3600);
    unsigned MinTrainSamples = config_get_number(ConfigSectionML, "minimum num samples to train", 1 * 3600);
    unsigned TrainEvery = config_get_number(ConfigSectionML, "train every", 1 * 3600);

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

    MaxTrainSamples = clamp(MaxTrainSamples, 1 * 3600u, 6 * 3600u);
    MinTrainSamples = clamp(MinTrainSamples, 1 * 3600u, 6 * 3600u);
    TrainEvery = clamp(TrainEvery, 1 * 3600u, 6 * 3600u);

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

    if (MinTrainSamples >= MaxTrainSamples) {
        error("invalid min/max train samples found (%d >= %d)", MinTrainSamples, MaxTrainSamples);

        MinTrainSamples = 1 * 3600;
        MaxTrainSamples = 4 * 3600;
    }

    if (ADMinWindowSize >= ADMaxWindowSize) {
        error("invalid min/max anomaly window size found (%lf >= %lf)", ADMinWindowSize, ADMaxWindowSize);

        ADMinWindowSize = 30.0;
        ADMaxWindowSize = 600.0;
    }

    /*
     * Assign to config instance
     */

    Cfg.EnableAnomalyDetection = EnableAnomalyDetection;

    Cfg.MaxTrainSamples = MaxTrainSamples;
    Cfg.MinTrainSamples = MinTrainSamples;
    Cfg.TrainEvery = TrainEvery;

    Cfg.DiffN = DiffN;
    Cfg.SmoothN = SmoothN;
    Cfg.LagN = LagN;

    Cfg.MaxKMeansIters = MaxKMeansIters;

    Cfg.DimensionAnomalyScoreThreshold = DimensionAnomalyScoreThreshold;
    Cfg.HostAnomalyRateThreshold = HostAnomalyRateThreshold;

    Cfg.ADMinWindowSize = ADMinWindowSize;
    Cfg.ADMaxWindowSize = ADMaxWindowSize;
    Cfg.ADIdleWindowSize = ADIdleWindowSize;
    Cfg.ADWindowRateThreshold = ADWindowRateThreshold;
    Cfg.ADDimensionRateThreshold = ADDimensionRateThreshold;

    Cfg.SP_HostsToSkip = simple_pattern_create(HostsToSkip.c_str(), NULL, SIMPLE_PATTERN_EXACT);
    Cfg.SP_ChartsToSkip = simple_pattern_create(ChartsToSkip.c_str(), NULL, SIMPLE_PATTERN_EXACT);
 
    Cfg.SaveAnomalyPercentageEvery = SaveAnomalyPercentageEvery;
}
