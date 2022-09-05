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

    return strdup(ConfigJson.dump(2, '\t').c_str());
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

bool ml_is_anomalous(RRDDIM *RD, double Value, bool Exists) {
    Dimension *D = static_cast<Dimension *>(RD->ml_dimension);
    if (!D)
        return false;

    D->addValue(Value, Exists);
    bool Result = D->predict().second;
    return Result;
}

char *ml_get_anomaly_events(RRDHOST *RH, const char *AnomalyDetectorName,
                            int AnomalyDetectorVersion, time_t After, time_t Before) {
    if (!RH || !RH->ml_host) {
        error("No host");
        return nullptr;
    }

    Host *H = static_cast<Host *>(RH->ml_host);
    std::vector<std::pair<time_t, time_t>> TimeRanges;

    bool Res = H->getAnomaliesInRange(TimeRanges, AnomalyDetectorName,
                                                  AnomalyDetectorVersion,
                                                  H->getUUID(),
                                                  After, Before);
    if (!Res) {
        error("DB result is empty");
        return nullptr;
    }

    nlohmann::json Json = TimeRanges;
    return strdup(Json.dump(4).c_str());
}

char *ml_get_anomaly_event_info(RRDHOST *RH, const char *AnomalyDetectorName,
                                int AnomalyDetectorVersion, time_t After, time_t Before) {
    if (!RH || !RH->ml_host) {
        error("No host");
        return nullptr;
    }

    Host *H = static_cast<Host *>(RH->ml_host);

    nlohmann::json Json;
    bool Res = H->getAnomalyInfo(Json, AnomalyDetectorName,
                                       AnomalyDetectorVersion,
                                       H->getUUID(),
                                       After, Before);
    if (!Res) {
        error("DB result is empty");
        return nullptr;
    }

    return strdup(Json.dump(4, '\t').c_str());
}

void ml_process_rrdr(RRDR *R, int MaxAnomalyRates) {
    if (R->rows != 1)
        return;

    if (MaxAnomalyRates < 1 || MaxAnomalyRates >= R->d)
        return;

    NETDATA_DOUBLE *CNs = R->v;
    RRDR_DIMENSION_FLAGS *DimFlags = R->od;

    std::vector<std::pair<NETDATA_DOUBLE, int>> V;

    V.reserve(R->d);
    for (int Idx = 0; Idx != R->d; Idx++)
        V.emplace_back(CNs[Idx], Idx);

    std::sort(V.rbegin(), V.rend());

    for (int Idx = MaxAnomalyRates; Idx != R->d; Idx++) {
        int UnsortedIdx = V[Idx].second;

        int OldFlags = static_cast<int>(DimFlags[UnsortedIdx]);
        int NewFlags = OldFlags | RRDR_DIMENSION_HIDDEN;

        DimFlags[UnsortedIdx] = static_cast<rrdr_dimension_flag>(NewFlags);
    }
}

void ml_dimension_update_name(RRDSET *RS, RRDDIM *RD, const char *Name) {
    (void) RS;

    Dimension *D = static_cast<Dimension *>(RD->ml_dimension);
    if (!D)
        return;

    D->setAnomalyRateRDName(Name);
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
