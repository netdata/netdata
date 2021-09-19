// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "ml-private.h"

using namespace ml;

/*
 * Global configuration instance to be shared between training and
 * prediction threads.
 */
Config ml::Cfg;

/*
 * Initialize global configuration variable.
 */
void Config::readMLConfig(void) {
    const char *ConfigSectionML = "ml";

    Cfg.TrainSecs = Seconds{config_get_number(ConfigSectionML, "num secs to train", 4 * 60)};
    Cfg.MinTrainSecs = Seconds{config_get_number(ConfigSectionML, "minimum num secs to train", 1 * 60)};
    Cfg.TrainEvery = Seconds{config_get_number(ConfigSectionML, "train every secs", 1 * 60 )};

    Cfg.DiffN = config_get_number(ConfigSectionML, "num samples to diff", 1);
    Cfg.SmoothN = config_get_number(ConfigSectionML, "num samples to smooth", 3);
    Cfg.LagN = config_get_number(ConfigSectionML, "num samples to lag", 5);

    Cfg.DimensionAnomalyScoreThreshold = config_get_float(ConfigSectionML, "dimension anomaly score threshold", 0.99);
    Cfg.HostAnomalyRateThreshold = config_get_float(ConfigSectionML, "host anomaly rate threshold", 0.2);

    Cfg.ADWindowSize = config_get_float(ConfigSectionML, "window minimum size", 30);
    Cfg.ADWindowRateThreshold = config_get_float(ConfigSectionML, "window minimum anomaly rate", 0.25);
    Cfg.ADDimensionRateThreshold = config_get_float(ConfigSectionML, "anomaly event min dimension rate threshold", 0.1);

    std::string HostsToSkip = config_get(ConfigSectionML, "hosts to skip from training", "!*");
    Cfg.SP_HostsToSkip = simple_pattern_create(HostsToSkip.c_str(), NULL, SIMPLE_PATTERN_EXACT);

    std::string ChartsToSkip = config_get(ConfigSectionML, "charts to skip from training", "!system.cpu *");
    Cfg.SP_ChartsToSkip = simple_pattern_create(ChartsToSkip.c_str(), NULL, SIMPLE_PATTERN_EXACT);
}
