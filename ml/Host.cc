// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Host.h"

#include "json/single_include/nlohmann/json.hpp"

using namespace ml;

static void updateDimensionsChart(RRDHOST *RH,
                                  collected_number NumNormalDimensions,
                                  collected_number NumAnomalousDimensions,
                                  collected_number AnomalyRate) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *NumNormalDimensionsRD = nullptr;
    static thread_local RRDDIM *NumAnomalousDimensionsRD = nullptr;
    // static thread_local RRDDIM *AnomalyRateRD = nullptr;
    (void) AnomalyRate;

    if (!RS) {
        RS = rrdset_create(
            RH, // host
            "anomaly_detection", // type
            "dimensions", // id
            NULL, // name
            "anomaly_detection", // family
            NULL, // ctx
            "Anomaly detection dimensions", // title
            "dimensions", // units
            "netdata", // plugin
            "ml", // module
            39183, // priority
            Cfg.UpdateEvery, // update_every
            RRDSET_TYPE_LINE // chart_type
        );

        NumNormalDimensionsRD = rrddim_add(RS, "normal", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        NumAnomalousDimensionsRD = rrddim_add(RS, "anomalous", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, NumNormalDimensionsRD, NumNormalDimensions);
    rrddim_set_by_pointer(RS, NumAnomalousDimensionsRD, NumAnomalousDimensions);

    rrdset_done(RS);
}

static void updateRateChart(RRDHOST *RH, collected_number AnomalyRate) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *AnomalyRateRD = nullptr;

    if (!RS) {
        RS = rrdset_create(
            RH, // host
            "anomaly_detection", // type
            "anomaly_rate", // id
            NULL, // name
            "anomaly_detection", // family
            NULL, // ctx
            "Percentage of anomalous dimensions", // title
            "percentage", // units
            "netdata", // plugin
            "ml", // module
            39184, // priority
            Cfg.UpdateEvery, // update_every
            RRDSET_TYPE_LINE // chart_type
        );

        AnomalyRateRD = rrddim_add(RS, "anomaly_rate", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, AnomalyRateRD, AnomalyRate);

    rrdset_done(RS);
}

static void updateWindowLengthChart(RRDHOST *RH, collected_number WindowLength) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *WindowLengthRD = nullptr;

    if (!RS) {
        RS = rrdset_create(
            RH, // host
            "anomaly_detection", // type
            "detector_window", // id
            NULL, // name
            "anomaly_detection", // family
            NULL, // ctx
            "Anomaly detector window length", // title
            "seconds", // units
            "netdata", // plugin
            "ml", // module
            39185, // priority
            Cfg.UpdateEvery, // update_every
            RRDSET_TYPE_LINE // chart_type
        );

        WindowLengthRD = rrddim_add(RS, "duration", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, WindowLengthRD, WindowLength * Cfg.UpdateEvery);
    rrdset_done(RS);
}

static void updateEventsChart(RRDHOST *RH,
                              std::pair<BitRateWindow::Edge, size_t> P,
                              bool ResetBitCounter,
                              bool NewAnomalyEvent) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *AboveThresholdRD = nullptr;
    static thread_local RRDDIM *ResetBitCounterRD = nullptr;
    static thread_local RRDDIM *NewAnomalyEventRD = nullptr;

    if (!RS) {
        RS = rrdset_create(
            RH, // host
            "anomaly_detection", // type
            "detector_events", // id
            NULL, // name
            "anomaly_detection", // family
            NULL, // ctx
            "Anomaly events triggerred", // title
            "boolean", // units
            "netdata", // plugin
            "ml", // module
            39186, // priority
            Cfg.UpdateEvery, // update_every
            RRDSET_TYPE_LINE // chart_type
        );

        AboveThresholdRD = rrddim_add(RS, "above_threshold", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        ResetBitCounterRD = rrddim_add(RS, "reset_bit_counter", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        NewAnomalyEventRD = rrddim_add(RS, "new_anomaly_event", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(RS);

    BitRateWindow::Edge E = P.first;
    bool AboveThreshold = E.second == BitRateWindow::State::AboveThreshold;

    rrddim_set_by_pointer(RS, AboveThresholdRD, AboveThreshold);
    rrddim_set_by_pointer(RS, ResetBitCounterRD, ResetBitCounter);
    rrddim_set_by_pointer(RS, NewAnomalyEventRD, NewAnomalyEvent);

    rrdset_done(RS);
}

void RrdHost::addDimension(Dimension *D) {
    std::lock_guard<std::mutex> Lock(Mutex);
    DimensionsMap[D->getRD()] = D;
}

void RrdHost::removeDimension(Dimension *D) {
    std::lock_guard<std::mutex> Lock(Mutex);
    DimensionsMap.erase(D->getRD());
}

void TrainableHost::trainOne(TimePoint &Now) {
    for (auto &DP : DimensionsMap) {
        Dimension *D = DP.second;

        MLResult Result = D->trainModel(Now);

        switch (Result) {
        case MLResult::Success:
            return;
        case MLResult::TryLockFailed:
        case MLResult::ShouldNotTrainNow:
        case MLResult::MissingData:
            continue;
        default:
            error("Unhandled MLError enumeration value");
        }
    }
}

void TrainableHost::train() {
    while (!netdata_exit) {
        size_t NumDimensions;

        TimePoint StartTP = SteadyClock::now();
        {
            std::lock_guard<std::mutex> Lock(Mutex);

            NumDimensions = DimensionsMap.size();
            trainOne(StartTP);
        }
        Duration<double> RealDuration = SteadyClock::now() - StartTP;
        Duration<double> AllottedDuration = Duration<double>{Cfg.TrainEvery} / (NumDimensions + 1);

        Duration<double> SleepFor;
        if ((2 * RealDuration) >= AllottedDuration) {
            error("\"train every secs\" configuration option is too low"
                  " (training dration: %lf seconds, allotted duration: %lf seconds)",
                  RealDuration.count(), AllottedDuration.count());

            SleepFor = AllottedDuration;
        } else {
            SleepFor = AllottedDuration - RealDuration;
        }

        Duration<double> MaxSleepFor = Seconds{1};
        std::this_thread::sleep_for(std::min(SleepFor, MaxSleepFor));
    }
}

void DetectableHost::detectOnce() {
    auto P = BRW.insert(AnomalyRate >= Cfg.HostAnomalyRateThreshold);
    BitRateWindow::Edge Edge = P.first;
    size_t WindowLength = P.second;

    bool ResetBitCounter = (Edge.first != BitRateWindow::State::AboveThreshold);
    bool NewAnomalyEvent = (Edge.first == BitRateWindow::State::AboveThreshold) &&
                           (Edge.second == BitRateWindow::State::Idle);

    std::vector<std::pair<double, std::string>> DimsOverThreshold;

    {
        std::lock_guard<std::mutex> Lock(Mutex);

        size_t NumAnomalousDimensions = 0;

        DimsOverThreshold.reserve(DimensionsMap.size());

        for (auto &DP : DimensionsMap) {
            Dimension *D = DP.second;

            auto P = D->detect(WindowLength, ResetBitCounter);
            bool IsAnomalous = P.first;
            double AnomalyRate = P.second;

            if (IsAnomalous)
                NumAnomalousDimensions += 1;

            if (NewAnomalyEvent && (AnomalyRate >= Cfg.ADDimensionRateThreshold))
                DimsOverThreshold.push_back({ AnomalyRate, D->getID() });
        }

        if (NumAnomalousDimensions)
            AnomalyRate = static_cast<double>(NumAnomalousDimensions) / DimensionsMap.size();
        else
            AnomalyRate = 0.0;

        error("Host anomaly: "
                "rate=%lf, length=%zu,"
                "anomalous-dimensions=%zu, total-dimensions= %zu",
                AnomalyRate, WindowLength,
                NumAnomalousDimensions, DimensionsMap.size());

        updateDimensionsChart(getRH(), DimensionsMap.size() - NumAnomalousDimensions, NumAnomalousDimensions, 100 * AnomalyRate);
        updateRateChart(getRH(), AnomalyRate * 100.0);
        updateWindowLengthChart(getRH(), WindowLength);
        updateEventsChart(getRH(), P, ResetBitCounter, NewAnomalyEvent);
    }

    if (!NewAnomalyEvent || (DimsOverThreshold.size() == 0))
        return;

    std::sort(DimsOverThreshold.begin(), DimsOverThreshold.end());
    std::reverse(DimsOverThreshold.begin(), DimsOverThreshold.end());

    nlohmann::json JsonResult = DimsOverThreshold;

    time_t Before = now_realtime_sec();
    time_t After = Before - (WindowLength * Cfg.UpdateEvery);
    DB.insertAnomaly("AD1", 1, getUUID(), After, Before, JsonResult.dump(4));
}

void DetectableHost::detect() {
    std::this_thread::sleep_for(Seconds{10});

    while (!netdata_exit) {
        TimePoint StartTP = SteadyClock::now();
        detectOnce();
        TimePoint EndTP = SteadyClock::now();

        Duration<double> Dur = EndTP - StartTP;
        error("Detection took %lf seconds", Dur.count());

        std::this_thread::sleep_for(Seconds{Cfg.UpdateEvery});
    }
}

void DetectableHost::startAnomalyDetectionThreads() {
    TrainingThread = std::thread(&TrainableHost::train, this);
    DetectionThread = std::thread(&DetectableHost::detect, this);
}

void DetectableHost::stopAnomalyDetectionThreads() {
    TrainingThread.join();
    DetectionThread.join();
}
