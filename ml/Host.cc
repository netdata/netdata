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
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );

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
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );

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
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );

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
        RS = rrdset_create(
            RH, // host
            "anomaly_detection", // type
            "detector_events", // id
            NULL, // name
            "detector_events", // family
            NULL, // ctx
            "Anomaly events triggered", // title
            "boolean", // units
            "netdata", // plugin
            "ml", // module
            39186, // priority
            RH->rrd_update_every, // update_every
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
            RH->rrd_update_every, // update_every
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
                                collected_number TotalTrainingDuration,
                                collected_number MaxTrainingDuration)
{
    static thread_local RRDSET *RS = nullptr;
    static thread_local RRDDIM *TotalTrainingDurationRD = nullptr;
    static thread_local RRDDIM *MaxTrainingDurationRD = nullptr;

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
            RH->rrd_update_every, // update_every
            RRDSET_TYPE_LINE // chart_type
        );

        TotalTrainingDurationRD = rrddim_add(RS, "total_training_duration", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
        MaxTrainingDurationRD = rrddim_add(RS, "max_training_duration", NULL,
                1, 1, RRD_ALGORITHM_ABSOLUTE);
    } else
        rrdset_next(RS);

    rrddim_set_by_pointer(RS, TotalTrainingDurationRD, TotalTrainingDuration);
    rrddim_set_by_pointer(RS, MaxTrainingDurationRD, MaxTrainingDuration);

    rrdset_done(RS);
}

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

    TimePoint StartTP = SteadyClock::now();
    D->trainModel();
    Duration<double> Duration = SteadyClock::now() - StartTP;
    D->updateTrainingDuration(Duration.count());

    {
        std::lock_guard<std::mutex> Lock(Mutex);
        LocksMap[D].unlock();
    }
}

void TrainableHost::train() {
    Duration<double> MaxSleepFor = Seconds{updateEvery()};

    while (!netdata_exit) {
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
    auto P = BRW.insert(AnomalyRate >= Cfg.HostAnomalyRateThreshold);
    BitRateWindow::Edge Edge = P.first;
    size_t WindowLength = P.second;

    bool ResetBitCounter = (Edge.first != BitRateWindow::State::AboveThreshold);
    bool NewAnomalyEvent = (Edge.first == BitRateWindow::State::AboveThreshold) &&
                           (Edge.second == BitRateWindow::State::Idle);

    std::vector<std::pair<double, std::string>> DimsOverThreshold;
    /*the following vector takes care of the count of the set anomaly bits per dimension*/
    std::vector<std::pair<double, std::string>> DimsAnomalyRate;
    
    size_t NumAnomalousDimensions = 0;
    size_t NumNormalDimensions = 0;
    size_t NumTrainedDimensions = 0;

    double TotalTrainingDuration = 0.0;
    double MaxTrainingDuration = 0.0;

    /*Time variable to hold the oldest time that dbengine holds data, so that 
    ...the records older than this may be deleted from anomaly rate info*/
    time_t OldestTimeOfAllDims = now_realtime_sec();

    {
        std::lock_guard<std::mutex> Lock(Mutex);

        DimsOverThreshold.reserve(DimensionsMap.size());
        DimsAnomalyRate.reserve(DimensionsMap.size());
        
        for (auto &DP : DimensionsMap) {
            Dimension *D = DP.second;

            auto P = D->detect(WindowLength, ResetBitCounter);
            bool IsAnomalous = P.first;
            double AnomalyRate = P.second;

            NumTrainedDimensions += D->isTrained();

            double DimTrainingDuration = D->updateTrainingDuration(0.0);
            MaxTrainingDuration = std::max(MaxTrainingDuration, DimTrainingDuration);
            TotalTrainingDuration += DimTrainingDuration;

            if (IsAnomalous) {
                NumAnomalousDimensions += 1;
                /*count up the number of anomalies for this dimension*/
                D->setAnomalousBitCount(D->getAnomalousBitCount() + 1);
            }

            /*regardless the dimension value was anomalous or not, update the value of the percentage of anomalous dimension*/
            if(AnomalyBitCounterWindow < Cfg.SaveAnomalyPercentageEvery) {           
                D->setAnomalyPercentage((D->getAnomalousBitCount() / (static_cast<double>(Cfg.SaveAnomalyPercentageEvery - AnomalyBitCounterWindow) * static_cast<double>(updateEvery()))) * 100.0);
            }
            /*Register the oldest time of this dimension*/
            OldestTimeOfAllDims = MIN(D->oldestTime(), OldestTimeOfAllDims);
            
            /*if the counting window is exhausted, push and then reset the counter*/
            if(AnomalyBitCounterWindow == 0) {
                double AnomalyPercentage = (D->getAnomalousBitCount() / (static_cast<double>(Cfg.SaveAnomalyPercentageEvery) * static_cast<double>(updateEvery()))) * 100.0;
                DimsAnomalyRate.push_back({AnomalyPercentage , D->getID() });                
                D->setAnomalousBitCount(0.0);
            }

            if (NewAnomalyEvent && (AnomalyRate >= Cfg.ADDimensionRateThreshold))
                DimsOverThreshold.push_back({ AnomalyRate, D->getID() });

            
        }

        if (NumAnomalousDimensions)
            AnomalyRate = static_cast<double>(NumAnomalousDimensions) / DimensionsMap.size();
        else
            AnomalyRate = 0.0;

        NumNormalDimensions = DimensionsMap.size() - NumAnomalousDimensions;
    }

    this->NumAnomalousDimensions = NumAnomalousDimensions;
    this->NumNormalDimensions = NumNormalDimensions;
    this->NumTrainedDimensions = NumTrainedDimensions;

    updateDimensionsChart(getRH(), NumTrainedDimensions, NumNormalDimensions, NumAnomalousDimensions);
    updateRateChart(getRH(), AnomalyRate * 10000.0);
    updateWindowLengthChart(getRH(), WindowLength);
    updateEventsChart(getRH(), P, ResetBitCounter, NewAnomalyEvent);
    updateTrainingChart(getRH(), TotalTrainingDuration * 1000.0, MaxTrainingDuration * 1000.0);
    
    /*code snippet to keep account of the count of the anomalous values of each dimension
    ...in the anomaly-precentage period configured by (Cfg.SaveAnomalyPercentageEvery)*/
    if(AnomalyBitCounterWindow == 0) {
        /*one period is completed, save in the DB the vector that holds the values of the percentages 
        (of the set anomaly bits) for each dimension*/
        nlohmann::json JsonResult = DimsAnomalyRate;

        time_t Before = now_realtime_sec();
        time_t After = Before - ((Cfg.SaveAnomalyPercentageEvery+1) * updateEvery());
        
        DB.insertBulkAnomalyRateInfo(getUUID(), After, Before, JsonResult.dump(4));
        /*and reset the window size to restart down-counting*/
        AnomalyBitCounterWindow = Cfg.SaveAnomalyPercentageEvery;
        /*Save the value of the Before time tag for when it will be checked for timeranges including current time*/
        setLastSavedBefore(Before);

        //Delete the old records based on the oldest time of dim data in dbengine, i.e. OldestTimeOfAllDims
        DB.removeOldAnomalyRateInfo(OldestTimeOfAllDims);            
    }
    else {
        AnomalyBitCounterWindow--;
    }
    

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
    
    while (!netdata_exit) {
        TimePoint StartTP = SteadyClock::now();
        detectOnce();
        TimePoint EndTP = SteadyClock::now();

        Duration<double> Dur = EndTP - StartTP;
        updateDetectionChart(getRH(), Dur.count() * 1000);

        std::this_thread::sleep_for(Seconds{updateEvery()});
    }
}

void DetectableHost::getAnomalyRateInfoCurrentRange(std::vector<std::pair<std::string, double>> &V, time_t After, time_t Before) {
    {
        std::lock_guard<std::mutex> Lock(Mutex);
        for (auto &DP : DimensionsMap) {
            Dimension *D = DP.second;
            if(D->getAnomalyPercentage() > 0) {
                V.push_back({D->getID(), (D->getAnomalyPercentage() * abs(Before - After) / (Cfg.SaveAnomalyPercentageEvery * static_cast<double>(updateEvery())))});
            }
        }
    }
}

void DetectableHost::getAnomalyRateInfoMixedRange(std::vector<std::pair<std::string, double>> &V, std::string HostUUID,time_t After, time_t Before) {
    std::vector<std::pair<std::string, double>> DimAndAnomalyRateInRange;
    bool Res = getAnomalyRateInfoInRange(DimAndAnomalyRateInRange, HostUUID, After, getLastSavedBefore());
    std::vector<std::pair<std::string, double>>::iterator it;
    
    if (Res) {
        {
        std::lock_guard<std::mutex> Lock(Mutex);
            for (auto &DP : DimensionsMap) {
                Dimension *D = DP.second;
                
                /*Search in vector for corresponding dimension IDs, if found, combine and insert*/
                auto it = std::find_if( DimAndAnomalyRateInRange.begin(), DimAndAnomalyRateInRange.end(),
                [&D](const std::pair<std::string, int>& element){ return element.first == D->getID();} );
                
                if( it != DimAndAnomalyRateInRange.end())
                {
                    double CurrentPercentage = (D->getAnomalyPercentage() * (Before - getLastSavedBefore()) / (Cfg.SaveAnomalyPercentageEvery * static_cast<double>(updateEvery())));
                    if((CurrentPercentage > 0) || (it->second > 0)){
                        V.push_back({D->getID(), abs(((CurrentPercentage * (Before - getLastSavedBefore())) + (it->second * (getLastSavedBefore() - After)))/(Before - After))});
                    }
                }
            }
        }    
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