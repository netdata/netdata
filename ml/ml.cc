// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Dimension.h"
#include "Host.h"

#include <random>

using namespace ml;

bool ml_capable() {
    return true;
}

bool ml_enabled(RRDHOST *RH) {
    if (!Cfg.EnableAnomalyDetection)
        return false;

    if (simple_pattern_matches(Cfg.SP_HostsToSkip, rrdhost_hostname(RH)))
        return false;

    return true;
}

/*
 * Assumptions:
 *  1) hosts outlive their sets, and sets outlive their dimensions,
 *  2) dimensions always have a set that has a host.
 */

void ml_init(void) {
    // Read config values
    Cfg.readMLConfig();

    if (!Cfg.EnableAnomalyDetection)
        return;

    // Generate random numbers to efficiently sample the features we need
    // for KMeans clustering.
    std::random_device RD;
    std::mt19937 Gen(RD());

    Cfg.RandomNums.reserve(Cfg.MaxTrainSamples);
    for (size_t Idx = 0; Idx != Cfg.MaxTrainSamples; Idx++)
        Cfg.RandomNums.push_back(Gen());
}

void ml_new_host(RRDHOST *RH) {
    if (!ml_enabled(RH))
        return;

    Host *H = new Host(RH);
    RH->ml_host = static_cast<ml_host_t>(H);

    H->startAnomalyDetectionThreads();
}

void ml_delete_host(RRDHOST *RH) {
    Host *H = static_cast<Host *>(RH->ml_host);
    if (!H)
        return;

    H->stopAnomalyDetectionThreads();

    delete H;
    RH->ml_host = nullptr;
}

void ml_new_dimension(RRDDIM *RD) {
    RRDSET *RS = RD->rrdset;

    Host *H = static_cast<Host *>(RD->rrdset->rrdhost->ml_host);
    if (!H)
        return;

    if (static_cast<unsigned>(RD->update_every) != H->updateEvery())
        return;

    if (simple_pattern_matches(Cfg.SP_ChartsToSkip, rrdset_name(RS)))
        return;

    Dimension *D = new Dimension(RD);
    RD->ml_dimension = static_cast<ml_dimension_t>(D);
    H->addDimension(D);
}

void ml_delete_dimension(RRDDIM *RD) {
    Dimension *D = static_cast<Dimension *>(RD->ml_dimension);
    if (!D)
        return;

    Host *H = static_cast<Host *>(RD->rrdset->rrdhost->ml_host);
    if (!H)
        delete D;
    else
        H->removeDimension(D);

    RD->ml_dimension = nullptr;
}

char *ml_get_host_info(RRDHOST *RH) {
    nlohmann::json ConfigJson;

    if (RH && RH->ml_host) {
        Host *H = static_cast<Host *>(RH->ml_host);
        H->getConfigAsJson(ConfigJson);
    } else {
        ConfigJson["enabled"] = false;
    }

    return strdupz(ConfigJson.dump(2, '\t').c_str());
}

char *ml_get_host_runtime_info(RRDHOST *RH) {
    nlohmann::json ConfigJson;

    if (RH && RH->ml_host) {
        Host *H = static_cast<Host *>(RH->ml_host);
        H->getDetectionInfoAsJson(ConfigJson);
    } else {
        return nullptr;
    }

    return strdup(ConfigJson.dump(1, '\t').c_str());
}

char *ml_get_host_models(RRDHOST *RH) {
    nlohmann::json ModelsJson;

    if (RH && RH->ml_host) {
        Host *H = static_cast<Host *>(RH->ml_host);
        H->getModelsAsJson(ModelsJson);
        return strdup(ModelsJson.dump(2, '\t').c_str());
    }

    return nullptr;
}

bool ml_is_anomalous(RRDDIM *RD, double Value, bool Exists) {
    Dimension *D = static_cast<Dimension *>(RD->ml_dimension);
    if (!D)
        return false;

    return D->predict(Value, Exists);
}

bool ml_streaming_enabled() {
    return Cfg.StreamADCharts;
}

#if defined(ENABLE_ML_TESTS)

#include "gtest/gtest.h"

int test_ml(int argc, char *argv[]) {
    (void) argc;
    (void) argv;

    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}

#endif // ENABLE_ML_TESTS

#include "ml-private.h"
