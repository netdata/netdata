// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Dimension.h"
#include "Chart.h"
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

void ml_host_new(RRDHOST *RH) {
    if (!ml_enabled(RH))
        return;

    Host *H = new Host(RH);
    RH->ml_host = reinterpret_cast<ml_host_t *>(H);
}

void ml_host_delete(RRDHOST *RH) {
    Host *H = reinterpret_cast<Host *>(RH->ml_host);
    if (!H)
        return;

    delete H;
    RH->ml_host = nullptr;
}

void ml_chart_new(RRDSET *RS) {
    Host *H = reinterpret_cast<Host *>(RS->rrdhost->ml_host);
    if (!H)
        return;

    Chart *C = new Chart(RS);
    RS->ml_chart = reinterpret_cast<ml_chart_t *>(C);

    H->addChart(C);
}

void ml_chart_delete(RRDSET *RS) {
    Host *H = reinterpret_cast<Host *>(RS->rrdhost->ml_host);
    if (!H)
        return;

    Chart *C = reinterpret_cast<Chart *>(RS->ml_chart);
    H->removeChart(C);

    delete C;
    RS->ml_chart = nullptr;
}

void ml_dimension_new(RRDDIM *RD) {
    Chart *C = reinterpret_cast<Chart *>(RD->rrdset->ml_chart);
    if (!C)
        return;

    Dimension *D = new Dimension(RD);
    RD->ml_dimension = reinterpret_cast<ml_dimension_t *>(D);
    C->addDimension(D);
}

void ml_dimension_delete(RRDDIM *RD) {
    Dimension *D = reinterpret_cast<Dimension *>(RD->ml_dimension);
    if (!D)
        return;

    Chart *C = reinterpret_cast<Chart *>(RD->rrdset->ml_chart);
    C->removeDimension(D);

    delete D;
    RD->ml_dimension = nullptr;
}

void ml_get_host_info(RRDHOST *RH, BUFFER *wb) {
    if (RH && RH->ml_host) {
        Host *H = reinterpret_cast<Host *>(RH->ml_host);
        H->getConfigAsJson(wb);
    } else {
        buffer_json_member_add_boolean(wb, "enabled", false);
    }
}

char *ml_get_host_runtime_info(RRDHOST *RH) {
    nlohmann::json ConfigJson;

    if (RH && RH->ml_host) {
        Host *H = reinterpret_cast<Host *>(RH->ml_host);
        H->getDetectionInfoAsJson(ConfigJson);
    } else {
        return nullptr;
    }

    return strdup(ConfigJson.dump(1, '\t').c_str());
}

char *ml_get_host_models(RRDHOST *RH) {
    nlohmann::json ModelsJson;

    if (RH && RH->ml_host) {
        Host *H = reinterpret_cast<Host *>(RH->ml_host);
        H->getModelsAsJson(ModelsJson);
        return strdup(ModelsJson.dump(2, '\t').c_str());
    }

    return nullptr;
}

void ml_start_anomaly_detection_threads(RRDHOST *RH) {
    if (RH && RH->ml_host) {
        Host *H = reinterpret_cast<Host *>(RH->ml_host);
        H->startAnomalyDetectionThreads();
    }
}

void ml_stop_anomaly_detection_threads(RRDHOST *RH) {
    if (RH && RH->ml_host) {
        Host *H = reinterpret_cast<Host *>(RH->ml_host);
        H->stopAnomalyDetectionThreads(true);
    }
}

void ml_cancel_anomaly_detection_threads(RRDHOST *RH) {
    if (RH && RH->ml_host) {
        Host *H = reinterpret_cast<Host *>(RH->ml_host);
        H->stopAnomalyDetectionThreads(false);
    }
}

bool ml_chart_update_begin(RRDSET *RS) {
    Chart *C = reinterpret_cast<Chart *>(RS->ml_chart);
    if (!C)
        return false;

    C->updateBegin();

    return true;
}

void ml_chart_update_end(RRDSET *RS) {
    Chart *C = reinterpret_cast<Chart *>(RS->ml_chart);
    if (!C)
        return;

    C->updateEnd();
}

bool ml_is_anomalous(RRDDIM *RD, time_t CurrT, double Value, bool Exists) {
    Dimension *D = reinterpret_cast<Dimension *>(RD->ml_dimension);
    if (!D)
        return false;

    Chart *C = reinterpret_cast<Chart *>(RD->rrdset->ml_chart);

    bool IsAnomalous = D->predict(CurrT, Value, Exists);
    C->updateDimension(D, IsAnomalous);
    return IsAnomalous;
}

bool ml_streaming_enabled() {
    return Cfg.StreamADCharts;
}

#include "ml-private.h"
