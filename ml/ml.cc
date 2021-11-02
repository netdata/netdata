// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Dimension.h"
#include "Host.h"

using namespace ml;

/*
 * Assumptions:
 *  1) hosts outlive their sets, and sets outlive their dimensions,
 *  2) dimensions always have a set that has a host.
 */

void ml_init(void) {
    Cfg.readMLConfig();
}

void ml_new_host(RRDHOST *RH) {
    if (!Cfg.EnableAnomalyDetection)
        return;

    if (simple_pattern_matches(Cfg.SP_HostsToSkip, RH->hostname))
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

    if (simple_pattern_matches(Cfg.SP_ChartsToSkip, RS->name))
        return;

    Dimension *D = new Dimension(RD);
    RD->state->ml_dimension = static_cast<ml_dimension_t>(D);
    H->addDimension(D);
}

void ml_delete_dimension(RRDDIM *RD) {
    Dimension *D = static_cast<Dimension *>(RD->state->ml_dimension);
    if (!D)
        return;

    Host *H = static_cast<Host *>(RD->rrdset->rrdhost->ml_host);
    H->removeDimension(D);

    RD->state->ml_dimension = nullptr;
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
    Dimension *D = static_cast<Dimension *>(RD->state->ml_dimension);
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

char *ml_get_anomaly_rate_info(RRDHOST *RH, time_t After, time_t Before) {
    if (!RH || !RH->ml_host) {
        error("No host");
        return nullptr;
    }

    Host *H = static_cast<Host *>(RH->ml_host);
    std::vector<std::pair<std::string, double>> DimAndAnomalyRate;
    if(Before > After) {
        
        if(Before <= H->getLastSavedBefore()) {
            //Only information from saved data is inquired
            bool Res = H->getAnomalyRateInfoInRange(DimAndAnomalyRate, H->getUUID(),
                                                        After, Before);
            if (!Res) {
                error("DB result is empty");
                return nullptr;
            }
        }
        else {
            //Information from unsaved data is also inquired
            if(Before > now_realtime_sec()) { 
                Before = now_realtime_sec();
            }
            if(After >= H->getLastSavedBefore()) {
                //Only the information from unsaved data is inquired
                H->getAnomalyRateInfoCurrentRange(DimAndAnomalyRate, After, Before);
            }
            else {
                //Mix information from saved and unsaved data is inquired
                H->getAnomalyRateInfoMixedRange(DimAndAnomalyRate, H->getUUID(),
                                                        After, Before);
            }
        }
    }
    else
    {
        error("Incorrect time range; Before time tag is not larger than After time tag!");
        return nullptr;
    }

    nlohmann::json Json = DimAndAnomalyRate;
    return strdup(Json.dump(4).c_str());
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
