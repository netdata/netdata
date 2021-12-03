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

    double SaveAnomalyPercentageEvery = config_get_number(ConfigSectionML, "window size of anomaly bit counting for anomaly percentage", 15 * 60);

    double MaxAnomalyRateInfoTableSize = config_get_number(ConfigSectionML, "the maximum size, in rows, of the data table that holds anomaly rate information", 10000.0);
    double MaxAnomalyRateInfoAge = config_get_number(ConfigSectionML, "the oldest age, in hours, of the anomaly rate information allowed to stay in the table", 240.0);

    std::string HostsToSkip = config_get(ConfigSectionML, "hosts to skip from training", "!*");
    std::string ChartsToSkip = config_get(ConfigSectionML, "charts to skip from training",
            "!system.* !cpu.* !mem.* !disk.* !disk_* "
            "!ip.* !ipv4.* !ipv6.* !net.* !net_* !netfilter.* "
            "!services.* !apps.* !groups.* !user.* !ebpf.* !netdata.* *");

    std::stringstream SS;
    SS << netdata_configured_cache_dir << "/anomaly-detection.db";
    Cfg.AnomalyDBPath = SS.str();

    #if defined(ENABLE_ML_TESTS)
    std::stringstream TestDBDirectory, TestDataDirectory, TestQuery1Directory, TestCheck1Directory, TestQuery2Directory, TestCheck2Directory;
    TestDBDirectory << netdata_configured_cache_dir << "/ml_test_anomaly_info.db";
    AnomalyTestDBPath = TestDBDirectory.str();
    Cfg.AnomalyTestDBPath = AnomalyTestDBPath;

    TestDataDirectory << netdata_configured_cache_dir << "/ml_test_data.sql";
    AnomalyTestDataPath = TestDataDirectory.str();
    Cfg.AnomalyTestDataPath = AnomalyTestDataPath;
    
    TestQuery1Directory << netdata_configured_cache_dir << "/ml_test_query_1.sql";
    AnomalyTestQuery1Path = TestQuery1Directory.str();
    Cfg.AnomalyTestQuery1Path = AnomalyTestQuery1Path;

    TestCheck1Directory << netdata_configured_cache_dir << "/ml_test_check_1.sql";
    AnomalyTestCheck1Path = TestCheck1Directory.str();
    Cfg.AnomalyTestCheck1Path = AnomalyTestCheck1Path;

    TestQuery2Directory << netdata_configured_cache_dir << "/ml_test_query_2.sql";
    AnomalyTestQuery2Path = TestQuery2Directory.str();
    Cfg.AnomalyTestQuery2Path = AnomalyTestQuery2Path;

    TestCheck2Directory << netdata_configured_cache_dir << "/ml_test_check_2.sql";
    AnomalyTestCheck2Path = TestCheck2Directory.str();
    Cfg.AnomalyTestCheck2Path = AnomalyTestCheck2Path;
    #endif // ENABLE_ML_TESTS
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

    SaveAnomalyPercentageEvery = clamp(SaveAnomalyPercentageEvery, 60.0, 240.0);

    MaxAnomalyRateInfoTableSize = clamp(MaxAnomalyRateInfoTableSize, 10.0, 100000.0);
    MaxAnomalyRateInfoAge = clamp(MaxAnomalyRateInfoAge, 0.1, 2400.0);
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

    Cfg.SaveAnomalyPercentageEvery = SaveAnomalyPercentageEvery;

    Cfg.MaxAnomalyRateInfoTableSize = MaxAnomalyRateInfoTableSize;
    Cfg.MaxAnomalyRateInfoAge = MaxAnomalyRateInfoAge;

    Cfg.SP_HostsToSkip = simple_pattern_create(HostsToSkip.c_str(), NULL, SIMPLE_PATTERN_EXACT);
    Cfg.SP_ChartsToSkip = simple_pattern_create(ChartsToSkip.c_str(), NULL, SIMPLE_PATTERN_EXACT);

    #if 0
        Cfg.MinTrainSamples = 1 * 60;
        Cfg.MaxTrainSamples = 4 * 60;
        Cfg.TrainEvery = 1 * 60;

        Cfg.SP_ChartsToSkip = simple_pattern_create("!system.cpu *", NULL, SIMPLE_PATTERN_EXACT);
    #endif
 
}
