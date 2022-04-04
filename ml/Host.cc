// SPDX-License-Identifier: GPL-3.0-or-later

#include <dlib/statistics.h>

#include "Config.h"
#include "Host.h"

#include "json/single_include/nlohmann/json.hpp"

using namespace ml;

static void updateDimensionsChart(RRDHOST *RH,
                                  collected_number NumTrainedDimensions,
                                  collected_number NumNormalDimensions,
                                  collected_number NumAnomalousDimensions) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *NumTotalDimensionsRD = nullptr;
    static thread_local RRDDIM *NumTrainedDimensionsRD = nullptr;
    static thread_local RRDDIM *NumNormalDimensionsRD = nullptr;
    static thread_local RRDDIM *NumAnomalousDimensionsRD = nullptr;

    if (!RS) {
        std::stringstream IdSS, NameSS, TitleSS;

        IdSS << "dimensions_on_" << localhost->machine_guid;
        NameSS << "dimensions_on_" << localhost->hostname;
        TitleSS << "Anomaly detection dimensions for host " << RH->hostname;

        RS = rrdset_create(
            RH,
            "anomaly_detection", // type
            IdSS.str().c_str(), // id
            NameSS.str().c_str(), // name
            "dimensions", // family
            "anomaly_detection.dimensions", // ctx
            TitleSS.str().c_str(), // title
            "dimensions", // units
            "netdata", // plugin
            "ml", // module
            39183, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );
        rrdset_flag_set(RS, RRDSET_FLAG_ANOMALY_DETECTION);

        NumTotalDimensionsRD = rrddim_add(RS, "total", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        NumTrainedDimensionsRD = rrddim_add(RS, "trained", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        NumNormalDimensionsRD = rrddim_add(RS, "normal", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        NumAnomalousDimensionsRD = rrddim_add(RS, "anomalous", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, NumTotalDimensionsRD, NumNormalDimensions + NumAnomalousDimensions);
    rrddim_set_by_pointer(RS, NumTrainedDimensionsRD, NumTrainedDimensions);
    rrddim_set_by_pointer(RS, NumNormalDimensionsRD, NumNormalDimensions);
    rrddim_set_by_pointer(RS, NumAnomalousDimensionsRD, NumAnomalousDimensions);

    rrdset_done(RS);
}

static void updateRateChart(RRDHOST *RH, collected_number AnomalyRate) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *AnomalyRateRD = nullptr;

    if (!RS) {
        std::stringstream IdSS, NameSS, TitleSS;

        IdSS << "anomaly_rate_on_" << localhost->machine_guid;
        NameSS << "anomaly_rate_on_" << localhost->hostname;
        TitleSS << "Percentage of anomalous dimensions for host " << RH->hostname;

        RS = rrdset_create(
            RH,
            "anomaly_detection", // type
            IdSS.str().c_str(), // id
            NameSS.str().c_str(), // name
            "anomaly_rate", // family
            "anomaly_detection.anomaly_rate", // ctx
            TitleSS.str().c_str(), // title
            "percentage", // units
            "netdata", // plugin
            "ml", // module
            39184, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );
        rrdset_flag_set(RS, RRDSET_FLAG_ANOMALY_DETECTION);

        AnomalyRateRD = rrddim_add(RS, "anomaly_rate", NULL,
                1, 100, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, AnomalyRateRD, AnomalyRate);

    rrdset_done(RS);
}

static void updateWindowLengthChart(RRDHOST *RH, collected_number WindowLength) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *WindowLengthRD = nullptr;

    if (!RS) {
        std::stringstream IdSS, NameSS, TitleSS;

        IdSS << "detector_window_on_" << localhost->machine_guid;
        NameSS << "detector_window_on_" << localhost->hostname;
        TitleSS << "Anomaly detector window length for host " << RH->hostname;

        RS = rrdset_create(
            RH,
            "anomaly_detection", // type
            IdSS.str().c_str(), // id
            NameSS.str().c_str(), // name
            "detector_window", // family
            "anomaly_detection.detector_window", // ctx
            TitleSS.str().c_str(), // title
            "seconds", // units
            "netdata", // plugin
            "ml", // module
            39185, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );
        rrdset_flag_set(RS, RRDSET_FLAG_ANOMALY_DETECTION);

        WindowLengthRD = rrddim_add(RS, "duration", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, WindowLengthRD, WindowLength * RH->rrd_update_every);
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
        std::stringstream IdSS, NameSS, TitleSS;

        IdSS << "detector_events_on_" << localhost->machine_guid;
        NameSS << "detector_events_on_" << localhost->hostname;
        TitleSS << "Anomaly events triggered for host " << RH->hostname;

        RS = rrdset_create(
            RH,
            "anomaly_detection", // type
            IdSS.str().c_str(), // id
            NameSS.str().c_str(), // name
            "detector_events", // family
            "anomaly_detection.detector_events", // ctx
            TitleSS.str().c_str(), // title
            "boolean", // units
            "netdata", // plugin
            "ml", // module
            39186, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );
        rrdset_flag_set(RS, RRDSET_FLAG_ANOMALY_DETECTION);

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

static void updateDetectionChart(RRDHOST *RH) {
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *UserRD, *SystemRD = nullptr;

    if (!RS) {
        std::stringstream IdSS, NameSS, TitleSS;

        IdSS << "prediction_stats_" << RH->machine_guid;
        NameSS << "prediction_stats_for_" << RH->hostname;
        TitleSS << "Prediction thread CPU usage for host " << RH->hostname;

        RS = rrdset_create_localhost(
            "netdata", // type
            IdSS.str().c_str(), // id
            NameSS.str().c_str(), // name
            "ml", // family
            "prediction_stats", // ctx
            TitleSS.str().c_str(), // title
            "milliseconds/s", // units
            "netdata", // plugin
            "ml", // module
            136000, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_STACKED // chart_type
        );

        UserRD = rrddim_add(RS, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        SystemRD = rrddim_add(RS, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
    } else
        rrdset_next(RS);

    struct rusage TRU;
    getrusage(RUSAGE_THREAD, &TRU);

    rrddim_set_by_pointer(RS, UserRD, TRU.ru_utime.tv_sec * 1000000ULL + TRU.ru_utime.tv_usec);
    rrddim_set_by_pointer(RS, SystemRD, TRU.ru_stime.tv_sec * 1000000ULL + TRU.ru_stime.tv_usec);
    rrdset_done(RS);
}

static void updateTrainingChart(RRDHOST *RH, struct rusage *TRU)
{
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *UserRD = nullptr;
    static thread_local RRDDIM *SystemRD = nullptr;

    if (!RS) {
        std::stringstream IdSS, NameSS, TitleSS;

        IdSS << "training_stats_" << RH->machine_guid;
        NameSS << "training_stats_for_" << RH->hostname;
        TitleSS << "Training thread CPU usage for host " << RH->hostname;

        RS = rrdset_create_localhost(
            "netdata", // type
            IdSS.str().c_str(), // id
            NameSS.str().c_str(), // name
            "ml", // family
            "training_stats", // ctx
            TitleSS.str().c_str(), // title
            "milliseconds/s", // units
            "netdata", // plugin
            "ml", // module
            136001, // priority
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_STACKED // chart_type
        );

        UserRD = rrddim_add(RS, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        SystemRD = rrddim_add(RS, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, UserRD, TRU->ru_utime.tv_sec * 1000000ULL + TRU->ru_utime.tv_usec);
    rrddim_set_by_pointer(RS, SystemRD, TRU->ru_stime.tv_sec * 1000000ULL + TRU->ru_stime.tv_usec);
    rrdset_done(RS);
}

void RrdHost::addDimension(Dimension *D) {
	RRDDIM *AnomalyRateRD = rrddim_add(AnomalyRateRS, D->getID().c_str(), NULL,
                                       1, 1000, RRD_ALGORITHM_ABSOLUTE);
    D->setAnomalyRateRD(AnomalyRateRD);

	{
		std::lock_guard<std::mutex> Lock(Mutex);

		DimensionsMap[D->getRD()] = D;

		// Default construct mutex for dimension
	    LocksMap[D];
	}
}

void RrdHost::removeDimension(Dimension *D) {
    // Remove the dimension from the hosts map.
    {
        std::lock_guard<std::mutex> Lock(Mutex);
        DimensionsMap.erase(D->getRD());
    }

    // Delete the dimension by locking the mutex that protects it.
    {
        std::lock_guard<std::mutex> Lock(LocksMap[D]);
        delete D;
    }

    // Remove the lock entry for the deleted dimension.
    {
        std::lock_guard<std::mutex> Lock(Mutex);
        LocksMap.erase(D);
    }
}

void RrdHost::getConfigAsJson(nlohmann::json &Json) const {
    Json["version"] = 1;

    Json["enabled"] = Cfg.EnableAnomalyDetection;

    Json["min-train-samples"] = Cfg.MinTrainSamples;
    Json["max-train-samples"] = Cfg.MaxTrainSamples;
    Json["train-every"] = Cfg.TrainEvery;

    Json["diff-n"] = Cfg.DiffN;
    Json["smooth-n"] = Cfg.SmoothN;
    Json["lag-n"] = Cfg.LagN;

    Json["random-sampling-ratio"] = Cfg.RandomSamplingRatio;
    Json["max-kmeans-iters"] = Cfg.MaxKMeansIters;

    Json["dimension-anomaly-score-threshold"] = Cfg.DimensionAnomalyScoreThreshold;
    Json["host-anomaly-rate-threshold"] = Cfg.HostAnomalyRateThreshold;

    Json["min-window-size"] = Cfg.ADMinWindowSize;
    Json["max-window-size"] = Cfg.ADMaxWindowSize;
    Json["idle-window-size"] = Cfg.ADIdleWindowSize;
    Json["window-rate-threshold"] = Cfg.ADWindowRateThreshold;
    Json["dimension-rate-threshold"] = Cfg.ADDimensionRateThreshold;

    Json["hosts-to-skip"] = Cfg.HostsToSkip;
    Json["charts-to-skip"] = Cfg.ChartsToSkip;
}

std::pair<Dimension *, Duration<double>>
TrainableHost::findDimensionToTrain(const TimePoint &NowTP) {
    std::lock_guard<std::mutex> Lock(Mutex);

    Duration<double> AllottedDuration = Duration<double>{Cfg.TrainEvery * updateEvery()} / (DimensionsMap.size()  + 1);

    for (auto &DP : DimensionsMap) {
        Dimension *D = DP.second;

        if (D->shouldTrain(NowTP)) {
            LocksMap[D].lock();
            return { D, AllottedDuration };
        }
    }

    return { nullptr, AllottedDuration };
}

void TrainableHost::trainDimension(Dimension *D, const TimePoint &NowTP) {
    if (D == nullptr)
        return;

    D->LastTrainedAt = NowTP + Seconds{D->updateEvery()};
    D->trainModel();

    {
        std::lock_guard<std::mutex> Lock(Mutex);
        LocksMap[D].unlock();
    }
}

void TrainableHost::train() {
    Duration<double> MaxSleepFor = Seconds{10 * updateEvery()};

    while (!netdata_exit) {
        updateResourceUsage();

        TimePoint NowTP = SteadyClock::now();

        auto P = findDimensionToTrain(NowTP);
        trainDimension(P.first, NowTP);

        Duration<double> AllottedDuration = P.second;
        Duration<double> RealDuration = SteadyClock::now() - NowTP;

        Duration<double> SleepFor;
        if (RealDuration >= AllottedDuration)
            continue;

        SleepFor = std::min(AllottedDuration - RealDuration, MaxSleepFor);
        std::this_thread::sleep_for(SleepFor);
    }
}

void DetectableHost::detectOnce() {
    auto P = BRW.insert(WindowAnomalyRate >= Cfg.HostAnomalyRateThreshold);
    BitRateWindow::Edge Edge = P.first;
    size_t WindowLength = P.second;

    bool ResetBitCounter = (Edge.first != BitRateWindow::State::AboveThreshold);
    bool NewAnomalyEvent = (Edge.first == BitRateWindow::State::AboveThreshold) &&
                           (Edge.second == BitRateWindow::State::Idle);

    std::vector<std::pair<double, std::string>> DimsOverThreshold;

    size_t NumAnomalousDimensions = 0;
    size_t NumNormalDimensions = 0;
    size_t NumTrainedDimensions = 0;

    bool CollectAnomalyRates = (++AnomalyRateTimer == Cfg.DBEngineAnomalyRateEvery);
    if (CollectAnomalyRates)
        rrdset_next(AnomalyRateRS);

    {
        std::lock_guard<std::mutex> Lock(Mutex);

        DimsOverThreshold.reserve(DimensionsMap.size());

        for (auto &DP : DimensionsMap) {
            Dimension *D = DP.second;

            auto P = D->detect(WindowLength, ResetBitCounter);
            bool IsAnomalous = P.first;
            double AnomalyScore = P.second;

            NumTrainedDimensions += D->isTrained();

            if (IsAnomalous)
                NumAnomalousDimensions += 1;

            if (NewAnomalyEvent && (AnomalyScore >= Cfg.ADDimensionRateThreshold))
                DimsOverThreshold.push_back({ AnomalyScore, D->getID() });

            D->updateAnomalyBitCounter(AnomalyRateRS, AnomalyRateTimer, IsAnomalous);
        }

        if (NumAnomalousDimensions)
            WindowAnomalyRate = static_cast<double>(NumAnomalousDimensions) / DimensionsMap.size();
        else
            WindowAnomalyRate = 0.0;

        NumNormalDimensions = DimensionsMap.size() - NumAnomalousDimensions;
    }

    if (CollectAnomalyRates) {
        AnomalyRateTimer = 0;
        rrdset_done(AnomalyRateRS);
    }

    this->NumAnomalousDimensions = NumAnomalousDimensions;
    this->NumNormalDimensions = NumNormalDimensions;
    this->NumTrainedDimensions = NumTrainedDimensions;

    updateDimensionsChart(getRH(), NumTrainedDimensions, NumNormalDimensions, NumAnomalousDimensions);
    updateRateChart(getRH(), WindowAnomalyRate * 10000.0);
    updateWindowLengthChart(getRH(), WindowLength);
    updateEventsChart(getRH(), P, ResetBitCounter, NewAnomalyEvent);

    struct rusage TRU;
    getResourceUsage(&TRU);
    updateTrainingChart(getRH(), &TRU);

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
    time_t After = Before - (WindowLength * updateEvery());
    DB.insertAnomaly("AD1", 1, getUUID(), After, Before, JsonResult.dump(4));
}

void DetectableHost::detect() {
    std::this_thread::sleep_for(Seconds{10});

    heartbeat_t HB;
    heartbeat_init(&HB);

    while (!netdata_exit) {
        heartbeat_next(&HB, updateEvery() * USEC_PER_SEC);

        detectOnce();

        updateDetectionChart(getRH());
    }
}

void DetectableHost::getDetectionInfoAsJson(nlohmann::json &Json) const {
    Json["version"] = 1;
    Json["anomalous-dimensions"] = NumAnomalousDimensions;
    Json["normal-dimensions"] = NumNormalDimensions;
    Json["total-dimensions"] = NumAnomalousDimensions + NumNormalDimensions;
    Json["trained-dimensions"] = NumTrainedDimensions;
}

void DetectableHost::startAnomalyDetectionThreads() {
    TrainingThread = std::thread(&TrainableHost::train, this);
    DetectionThread = std::thread(&DetectableHost::detect, this);
}

void DetectableHost::stopAnomalyDetectionThreads() {
    TrainingThread.join();
    DetectionThread.join();
}
