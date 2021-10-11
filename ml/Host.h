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
};

using Host = DetectableHost;

} // namespace ml

#endif /* ML_HOST_H */
