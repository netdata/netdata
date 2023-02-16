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

    /*
     * Read values
     */

    unsigned MaxTrainSamples = config_get_number(ConfigSectionML, "maximum num samples to train", 4 * 3600);
    unsigned MinTrainSamples = config_get_number(ConfigSectionML, "minimum num samples to train", 1 * 900);
    unsigned TrainEvery = config_get_number(ConfigSectionML, "train every", 1 * 3600);
    unsigned NumModelsToUse = config_get_number(ConfigSectionML, "number of models per dimension", 1);

    unsigned DiffN = config_get_number(ConfigSectionML, "num samples to diff", 1);
    unsigned SmoothN = config_get_number(ConfigSectionML, "num samples to smooth", 3);
    unsigned LagN = config_get_number(ConfigSectionML, "num samples to lag", 5);

    double RandomSamplingRatio = config_get_float(ConfigSectionML, "random sampling ratio", 1.0 / LagN);
    unsigned MaxKMeansIters = config_get_number(ConfigSectionML, "maximum number of k-means iterations", 1000);

    double DimensionAnomalyScoreThreshold = config_get_float(ConfigSectionML, "dimension anomaly score threshold", 0.99);

    double HostAnomalyRateThreshold = config_get_float(ConfigSectionML, "host anomaly rate threshold", 1.0);
    std::string AnomalyDetectionGroupingMethod = config_get(ConfigSectionML, "anomaly detection grouping method", "average");
    time_t AnomalyDetectionQueryDuration = config_get_number(ConfigSectionML, "anomaly detection grouping duration", 5 * 60);

    /*
     * Clamp
     */

    MaxTrainSamples = clamp<unsigned>(MaxTrainSamples, 1 * 3600, 24 * 3600);
    MinTrainSamples = clamp<unsigned>(MinTrainSamples, 1 * 900, 6 * 3600);
    TrainEvery = clamp<unsigned>(TrainEvery, 1 * 3600, 6 * 3600);
    NumModelsToUse = clamp<unsigned>(NumModelsToUse, 1, 7 * 24);

    DiffN = clamp(DiffN, 0u, 1u);
    SmoothN = clamp(SmoothN, 0u, 5u);
    LagN = clamp(LagN, 1u, 5u);

    RandomSamplingRatio = clamp(RandomSamplingRatio, 0.2, 1.0);
    MaxKMeansIters = clamp(MaxKMeansIters, 500u, 1000u);

    DimensionAnomalyScoreThreshold = clamp(DimensionAnomalyScoreThreshold, 0.01, 5.00);

    HostAnomalyRateThreshold = clamp(HostAnomalyRateThreshold, 0.1, 10.0);
    AnomalyDetectionQueryDuration = clamp<time_t>(AnomalyDetectionQueryDuration, 60, 15 * 60);

    /*
     * Validate
     */

    if (MinTrainSamples >= MaxTrainSamples) {
        error("invalid min/max train samples found (%u >= %u)", MinTrainSamples, MaxTrainSamples);

        MinTrainSamples = 1 * 3600;
        MaxTrainSamples = 4 * 3600;
    }

    /*
     * Assign to config instance
     */

    Cfg.EnableAnomalyDetection = EnableAnomalyDetection;

    Cfg.MaxTrainSamples = MaxTrainSamples;
    Cfg.MinTrainSamples = MinTrainSamples;
    Cfg.TrainEvery = TrainEvery;
    Cfg.NumModelsToUse = NumModelsToUse;

    Cfg.DiffN = DiffN;
    Cfg.SmoothN = SmoothN;
    Cfg.LagN = LagN;

    Cfg.RandomSamplingRatio = RandomSamplingRatio;
    Cfg.MaxKMeansIters = MaxKMeansIters;

    Cfg.DimensionAnomalyScoreThreshold = DimensionAnomalyScoreThreshold;

    Cfg.HostAnomalyRateThreshold = HostAnomalyRateThreshold;
    Cfg.AnomalyDetectionGroupingMethod = time_grouping_parse(
            AnomalyDetectionGroupingMethod.c_str(), RRDR_GROUPING_AVERAGE);
    Cfg.AnomalyDetectionQueryDuration = AnomalyDetectionQueryDuration;

    Cfg.HostsToSkip = config_get(ConfigSectionML, "hosts to skip from training", "!*");
    Cfg.SP_HostsToSkip = simple_pattern_create(Cfg.HostsToSkip.c_str(), NULL, SIMPLE_PATTERN_EXACT);

    // Always exclude anomaly_detection charts from training.
    Cfg.ChartsToSkip = "anomaly_detection.* ";
    Cfg.ChartsToSkip += config_get(ConfigSectionML, "charts to skip from training", "netdata.*");
    Cfg.SP_ChartsToSkip = simple_pattern_create(Cfg.ChartsToSkip.c_str(), NULL, SIMPLE_PATTERN_EXACT);

    Cfg.StreamADCharts = config_get_boolean(ConfigSectionML, "stream anomaly detection charts", true);
}
