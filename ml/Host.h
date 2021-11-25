// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_HOST_H
#define ML_HOST_H

#include "BitRateWindow.h"
#include "Config.h"
#include "Database.h"
#include "Dimension.h"

#include "ml-private.h"

namespace ml {

class RrdHost {
public:
    RrdHost(RRDHOST *RH) : RH(RH) {}

    RRDHOST *getRH() { return RH; }

    unsigned updateEvery() { return RH->rrd_update_every; }

    std::string getUUID() {
        char S[UUID_STR_LEN];
        uuid_unparse_lower(RH->host_uuid, S);
        return S;
    }

    void addDimension(Dimension *D);
    void removeDimension(Dimension *D);

    void getConfigAsJson(nlohmann::json &Json) const;

    virtual ~RrdHost() {};

protected:
    RRDHOST *RH;

    // Protect dimension and lock maps
    std::mutex Mutex;

    std::map<RRDDIM *, Dimension *> DimensionsMap;
    std::map<Dimension *, std::mutex> LocksMap;
};

class TrainableHost : public RrdHost {
public:
    TrainableHost(RRDHOST *RH) : RrdHost(RH) {}

    void train();

private:
    std::pair<Dimension *, Duration<double>> findDimensionToTrain(const TimePoint &NowTP);
    void trainDimension(Dimension *D, const TimePoint &NowTP);
};

class DetectableHost : public TrainableHost {
public:
    DetectableHost(RRDHOST *RH) : TrainableHost(RH) {}

    void startAnomalyDetectionThreads();
    void stopAnomalyDetectionThreads();

    template<typename ...ArgTypes>
    bool getAnomalyInfo(ArgTypes&&... Args) {
        return DB.getAnomalyInfo(Args...);
    }

    template<typename ...ArgTypes>
    bool getAnomaliesInRange(ArgTypes&&... Args) {
        return DB.getAnomaliesInRange(Args...);
    }

    //template<typename ...ArgTypes>
    //bool getAnomalyRateInfoInRange(ArgTypes&&... Args) {
    bool getAnomalyRateInfoInRange(std::vector<std::pair<std::string, double>> V, std::string HostUUID, time_t After, time_t Before) { 
        /*time_t CompletePeriod = static_cast<size_t>(Cfg.SaveAnomalyPercentageEvery * updateEvery());
        time_t CompleteAfter = 0;
        time_t CompleteBefore = 0;
        time_t PreCompleteAfter = 0;
        time_t PostCompleteBefore = 0;
        time_t FromNowToAfter = 0;
        time_t FromNowToBefore = 0;
        size_t NumOfCompletePeriods = 0;
        size_t BeforePortionCoeff = 1;
        size_t AfterPortionCoeff = 1;
        //Work out CompleteBefore and the remaining after it
        info("Now = %ld, After = %ld, Before = %ld", now_realtime_sec(), After, Before);
        FromNowToBefore = now_realtime_sec() - Before;
        //info("FromNowToBefore = %ld", FromNowToBefore);
        NumOfCompletePeriods = static_cast<size_t>(FromNowToBefore / CompletePeriod);
        //info("NumOfCompletePeriods = %ld", NumOfCompletePeriods);
        CompleteBefore = now_realtime_sec() - ((NumOfCompletePeriods + 1) * CompletePeriod) - ((Cfg.SaveAnomalyPercentageEvery - AnomalyBitCounterWindow) * updateEvery());
        info("CompleteBefore = %ld", CompleteBefore);
        BeforePortionCoeff = static_cast<size_t>(Cfg.SaveAnomalyPercentageEvery / (Before - CompleteBefore));
        //info("BeforePortionCoeff = %ld", BeforePortionCoeff);
        //Work out CompleteAfter and the remaining before it
        FromNowToAfter = now_realtime_sec() - After;
        //info("FromNowToAfter = %ld", FromNowToAfter);
        NumOfCompletePeriods = static_cast<size_t>(FromNowToAfter / CompletePeriod);
        //info("NumOfCompletePeriods = %ld", NumOfCompletePeriods);
        CompleteAfter = now_realtime_sec() - ((NumOfCompletePeriods) * CompletePeriod) - ((Cfg.SaveAnomalyPercentageEvery - AnomalyBitCounterWindow) * updateEvery());
        info("CompleteAfter = %ld", CompleteAfter);
        AfterPortionCoeff = static_cast<size_t>(Cfg.SaveAnomalyPercentageEvery / (CompleteAfter - After));
        //info("AfterPortionCoeff = %ld", AfterPortionCoeff);
        //Now, work out how many complete period exists between the two complete pointers, i.e. CompleteAfter and CompleteBefore
        NumOfCompletePeriods = static_cast<size_t>((CompleteBefore - CompleteAfter) / CompletePeriod);
        //info("NumOfCompletePeriods = %ld", NumOfCompletePeriods);
        PreCompleteAfter = After - CompletePeriod; // using After instead of CompleteAfter is a bit safer as if CompleteAfter is not sharp on a saved timestamp
        info("PreCompleteAfter = %ld", PreCompleteAfter);
        PostCompleteBefore = Before + CompletePeriod;  // using Before instead of CompleteBefore is a bit safer as if CompleteBefore is not sharp on a saved timestamp
        info("PostCompleteBefore = %ld", PostCompleteBefore);
        //return DB.getAnomalyRateInfoInRange(V, HostUUID, CompleteAfter, CompleteBefore, PreCompleteAfter, PostCompleteBefore, NumOfCompletePeriods, AfterPortionCoeff, BeforePortionCoeff);*/
        return DB.getAnomalyRateInfoInRange(V, HostUUID, After, Before);
    }

    void getDetectionInfoAsJson(nlohmann::json &Json) const;

private:
    void detect();
    void detectOnce();

private:
    std::thread TrainingThread;
    std::thread DetectionThread;

    BitRateWindow BRW{
        static_cast<size_t>(Cfg.ADMinWindowSize),
        static_cast<size_t>(Cfg.ADMaxWindowSize),
        static_cast<size_t>(Cfg.ADIdleWindowSize),
        static_cast<size_t>(Cfg.ADMinWindowSize * Cfg.ADWindowRateThreshold)
    };

    CalculatedNumber AnomalyRate{0.0};

    size_t NumAnomalousDimensions{0};
    size_t NumNormalDimensions{0};
    size_t NumTrainedDimensions{0};

    Database DB{Cfg.AnomalyDBPath};

    /*the counter variable to downcount the time window for anomaly bit counting*/
    size_t AnomalyBitCounterWindow{static_cast<size_t>(Cfg.SaveAnomalyPercentageEvery)};


};

using Host = DetectableHost;

} // namespace ml

#endif /* ML_HOST_H */
