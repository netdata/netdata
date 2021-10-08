// SPDX-License-Identifier: GPL-3.0-or-later

#include <dlib/statistics.h>

#include "Config.h"
#include "Host.h"

#include "json/single_include/nlohmann/json.hpp"

using namespace ml;

static void updateDimensionsChart(RRDHOST *RH,
                                  collected_number NumNormalDimensions,
                                  collected_number NumAnomalousDimensions) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *NumNormalDimensionsRD = nullptr;
    static thread_local RRDDIM *NumAnomalousDimensionsRD = nullptr;

    if (!RS) {
        RS = rrdset_create(
            RH, // host
            "anomaly_detection", // type
            "dimensions", // id
            NULL, // name
            "dimensions", // family
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
            "anomaly_rate", // family
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
            "detector_window", // family
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
            "detector_events", // family
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

static void updateDetectionChart(RRDHOST *RH, collected_number PredictionDuration) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *PredictiobDurationRD = nullptr;

    if (!RS) {
        RS = rrdset_create(
            RH, // host
            "anomaly_detection", // type
            "prediction_stats", // id
            NULL, // name
            "prediction_stats", // family
            NULL, // ctx
            "Time it took to run prediction", // title
            "milliseconds", // units
            "netdata", // plugin
            "ml", // module
            39187, // priority
            Cfg.UpdateEvery, // update_every
            RRDSET_TYPE_LINE // chart_type
        );

        PredictiobDurationRD  = rrddim_add(RS, "duration", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, PredictiobDurationRD, PredictionDuration);

    rrdset_done(RS);
}

static void updateTrainingChart(RRDHOST *RH,
                                collected_number MaxTrainingDuration,
                                collected_number AvgTrainingDuration,
                                collected_number StdDevTrainingDuration,
                                collected_number AvgSleepForDuration)
{
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *MaxTrainingDurationRD = nullptr;
    static thread_local RRDDIM *AvgTrainingDurationRD = nullptr;
    static thread_local RRDDIM *StdDevTrainingDurationRD = nullptr;
    static thread_local RRDDIM *AvgSleepForDurationRD = nullptr;

    if (!RS) {
        RS = rrdset_create(
            RH, // host
            "anomaly_detection", // type
            "training_stats", // id
            NULL, // name
            "training_stats", // family
            NULL, // ctx
            "Training step statistics", // title
            "milliseconds", // units
            "netdata", // plugin
            "ml", // module
            39188, // priority
            Cfg.UpdateEvery, // update_every
            RRDSET_TYPE_LINE // chart_type
        );

        MaxTrainingDurationRD = rrddim_add(RS, "max_training_duration", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        AvgTrainingDurationRD = rrddim_add(RS, "avg_training_duration", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        StdDevTrainingDurationRD = rrddim_add(RS, "stddev_training_duration", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        AvgSleepForDurationRD = rrddim_add(RS, "avg_sleep_duration", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, MaxTrainingDurationRD, MaxTrainingDuration);
    rrddim_set_by_pointer(RS, AvgTrainingDurationRD, AvgTrainingDuration);
    rrddim_set_by_pointer(RS, StdDevTrainingDurationRD, StdDevTrainingDuration);
    rrddim_set_by_pointer(RS, AvgSleepForDurationRD, AvgSleepForDuration);

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

void RrdHost::getConfigAsJson(nlohmann::json &Json) const {
    Json["version"] = 1;

    Json["enabled"] = Cfg.EnableAnomalyDetection;

    Json["min-train-samples"] = Cfg.MinTrainSecs.count();
    Json["max-train-samples"] = Cfg.MaxTrainSecs.count();
    Json["train-every"] = Cfg.MaxTrainSecs.count();

    Json["diff-n"] = Cfg.DiffN;
    Json["smooth-n"] = Cfg.SmoothN;
    Json["lag-n"] = Cfg.LagN;

    Json["max-kmeans-iters"] = Cfg.MaxKMeansIters;

    Json["dimension-anomaly-score-threshold"] = Cfg.DimensionAnomalyScoreThreshold;
    Json["host-anomaly-rate-threshold"] = Cfg.HostAnomalyRateThreshold;

    Json["min-window-size"] = Cfg.ADMinWindowSize;
    Json["max-window-size"] = Cfg.ADMaxWindowSize;
    Json["idle-window-size"] = Cfg.ADIdleWindowSize;
    Json["window-rate-threshold"] = Cfg.ADWindowRateThreshold;
    Json["dimension-rate-threshold"] = Cfg.ADDimensionRateThreshold;
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
    dlib::running_stats<double> TrainingRS;
    dlib::running_stats<double> AllottedRS;
    dlib::running_stats<double> SleepForRS;

    Duration<double> MaxSleepFor = Seconds{1};

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
        SleepFor = std::min(SleepFor, MaxSleepFor);

        TrainingRS.add(RealDuration.count());
        TrainingDurationMax = TrainingRS.max();
        TrainingDurationAvg = TrainingRS.mean();
        TrainingDurationStdDev = TrainingRS.stddev();

        SleepForRS.add(SleepFor.count());
        SleepForDurationAvg = SleepForRS.mean();

        std::this_thread::sleep_for(SleepFor);
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

    size_t NumAnomalousDimensions = 0;
    size_t NumNormalDimensions = 0;

    {
        std::lock_guard<std::mutex> Lock(Mutex);

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

        NumNormalDimensions = DimensionsMap.size() - NumAnomalousDimensions;

        error("Host anomaly: rate=%lf, length=%zu, anomalous-dimensions=%zu, normal-dimensions= %zu",
              AnomalyRate, WindowLength, NumAnomalousDimensions, NumNormalDimensions);
    }

    updateDimensionsChart(getRH(), NumNormalDimensions, NumAnomalousDimensions);
    updateRateChart(getRH(), AnomalyRate * 100.0);
    updateWindowLengthChart(getRH(), WindowLength);
    updateEventsChart(getRH(), P, ResetBitCounter, NewAnomalyEvent);
    updateTrainingChart(getRH(), TrainingDurationMax * 1000.0,
                                 TrainingDurationAvg * 1000.0,
                                 TrainingDurationStdDev * 1000.0,
                                 SleepForDurationAvg * 1000.0);

    if (!NewAnomalyEvent || (DimsOverThreshold.size() == 0))
        return;

    std::sort(DimsOverThreshold.begin(), DimsOverThreshold.end());
    std::reverse(DimsOverThreshold.begin(), DimsOverThreshold.end());

    // Make sure the JSON response won't grow beyond a specific number
    // of dimensions. Log an error message if this happens, because it
    // most likely means that the user specified a very-low anomaly rate
    // threshold.
    size_t NumMaxDimsOverThreshold = 2000;
    if (DimsOverThreshold.size() > NumMaxDimsOverThreshold) {
        error("Found %zu dimensions over threshold. Reducing JSON result to %zu dimensions.",
              DimsOverThreshold.size(), NumMaxDimsOverThreshold);
        DimsOverThreshold.resize(NumMaxDimsOverThreshold);
    }

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
        updateDetectionChart(getRH(), Dur.count() * 1000);

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
