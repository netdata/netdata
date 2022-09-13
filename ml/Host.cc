// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Host.h"
#include "ADCharts.h"

#include "json/single_include/nlohmann/json.hpp"

using namespace ml;

void RrdHost::addDimension(Dimension *D) {
    std::lock_guard<std::mutex> Lock(Mutex);

    DimensionsMap[D->getRD()] = D;

    // Default construct mutex for dimension
    LocksMap[D];
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
    Json["anomaly-detection-grouping-method"] = group_method2string(Cfg.AnomalyDetectionGroupingMethod);
    Json["anomaly-detection-query-duration"] = Cfg.AnomalyDetectionQueryDuration;

    Json["hosts-to-skip"] = Cfg.HostsToSkip;
    Json["charts-to-skip"] = Cfg.ChartsToSkip;
}

void TrainableHost::getModelsAsJson(nlohmann::json &Json) {
    std::lock_guard<std::mutex> Lock(Mutex);

    for (auto &DP : DimensionsMap) {
        Dimension *D = DP.second;

        nlohmann::json JsonArray = nlohmann::json::array();
        for (const KMeans &KM : D->getModels()) {
            nlohmann::json J;
            KM.toJson(J);
            JsonArray.push_back(J);
        }
        Json[getMLDimensionID(D->getRD())] = JsonArray;
    }

    return;
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

    worker_register("MLTRAIN");
    worker_register_job_name(0, "dimensions");

    worker_is_busy(0);
    while (!netdata_exit) {
        netdata_thread_testcancel();
        netdata_thread_disable_cancelability();

        updateResourceUsage();

        TimePoint NowTP = SteadyClock::now();

        auto P = findDimensionToTrain(NowTP);
        trainDimension(P.first, NowTP);

        netdata_thread_enable_cancelability();

        Duration<double> AllottedDuration = P.second;
        Duration<double> RealDuration = SteadyClock::now() - NowTP;

        Duration<double> SleepFor;
        if (RealDuration >= AllottedDuration)
            continue;

        worker_is_idle();
        SleepFor = std::min(AllottedDuration - RealDuration, MaxSleepFor);
        std::this_thread::sleep_for(SleepFor);
        worker_is_busy(0);
    }
}

#define WORKER_JOB_DETECT_DIMENSION       0
#define WORKER_JOB_UPDATE_DETECTION_CHART 1
#define WORKER_JOB_UPDATE_ANOMALY_RATES   2
#define WORKER_JOB_UPDATE_CHARTS          3

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 5
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 5
#endif

void DetectableHost::detectOnce() {
    size_t NumAnomalousDimensions = 0;
    size_t NumNormalDimensions = 0;
    size_t NumTrainedDimensions = 0;
    size_t NumActiveDimensions = 0;

    bool CollectAnomalyRates = (++AnomalyRateTimer == Cfg.DBEngineAnomalyRateEvery);
    if (CollectAnomalyRates)
        rrdset_next(AnomalyRateRS);

    {
        std::lock_guard<std::mutex> Lock(Mutex);

        for (auto &DP : DimensionsMap) {
            worker_is_busy(WORKER_JOB_DETECT_DIMENSION);

            Dimension *D = DP.second;

            if (!D->isActive()) {
                D->updateAnomalyBitCounter(AnomalyRateRS, AnomalyRateTimer, false);
                continue;
            }

            NumActiveDimensions++;
            NumTrainedDimensions += D->isTrained();

            bool IsAnomalous = D->isAnomalous();
            if (IsAnomalous)
                NumAnomalousDimensions += 1;
            D->updateAnomalyBitCounter(AnomalyRateRS, AnomalyRateTimer, IsAnomalous);
        }

        if (NumAnomalousDimensions)
            HostAnomalyRate = static_cast<double>(NumAnomalousDimensions) / NumActiveDimensions;
        else
            HostAnomalyRate = 0.0;

        NumNormalDimensions = NumActiveDimensions - NumAnomalousDimensions;
    }

    if (CollectAnomalyRates) {
        worker_is_busy(WORKER_JOB_UPDATE_ANOMALY_RATES);
        AnomalyRateTimer = 0;
        rrdset_done(AnomalyRateRS);
    }

    this->NumAnomalousDimensions = NumAnomalousDimensions;
    this->NumNormalDimensions = NumNormalDimensions;
    this->NumTrainedDimensions = NumTrainedDimensions;
    this->NumActiveDimensions = NumActiveDimensions;

    worker_is_busy(WORKER_JOB_UPDATE_CHARTS);
    updateDimensionsChart(getRH(), NumTrainedDimensions, NumNormalDimensions, NumAnomalousDimensions);
    updateHostAndDetectionRateCharts(getRH(), HostAnomalyRate * 10000.0);

    struct rusage TRU;
    getResourceUsage(&TRU);
    updateTrainingChart(getRH(), &TRU);
}

void DetectableHost::detect() {
    worker_register("MLDETECT");
    worker_register_job_name(WORKER_JOB_DETECT_DIMENSION,       "dimensions");
    worker_register_job_name(WORKER_JOB_UPDATE_DETECTION_CHART, "detection chart");
    worker_register_job_name(WORKER_JOB_UPDATE_ANOMALY_RATES,   "anomaly rates");
    worker_register_job_name(WORKER_JOB_UPDATE_CHARTS,          "charts");

    std::this_thread::sleep_for(Seconds{10});

    heartbeat_t HB;
    heartbeat_init(&HB);

    while (!netdata_exit) {
        netdata_thread_testcancel();
        worker_is_idle();
        heartbeat_next(&HB, updateEvery() * USEC_PER_SEC);

        netdata_thread_disable_cancelability();
        detectOnce();

        worker_is_busy(WORKER_JOB_UPDATE_DETECTION_CHART);
        updateDetectionChart(getRH());
        netdata_thread_enable_cancelability();
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
    netdata_thread_cancel(TrainingThread.native_handle());
    netdata_thread_cancel(DetectionThread.native_handle());

    TrainingThread.join();
    DetectionThread.join();
}
