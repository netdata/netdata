// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_DIMENSION_H
#define ML_DIMENSION_H

#include "Query.h"
#include "Config.h"

#include "ml-private.h"

namespace ml {

enum class MLResult {
    Success = 0,
    MissingData,
    NaN,
};

static inline std::string getMLDimensionID(RRDDIM *RD) {
    RRDSET *RS = RD->rrdset;

    std::stringstream SS;
    SS << rrdset_context(RS) << "|" << rrdset_id(RS) << "|" << rrddim_name(RD);
    return SS.str();
}

class Dimension {
public:
    Dimension(RRDDIM *RD, RRDSET *AnomalyRateRS) :
        RD(RD),
        AnomalyRateRD(rrddim_add(AnomalyRateRS, ml::getMLDimensionID(RD).c_str(), NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE)),
        LastTrainedAt(Seconds(0)),
        Trained(false),
        ConstantModel(false),
        AnomalyScore(0.0),
        AnomalyBit(0),
        AnomalyBitCounter(0)
    { }

    RRDDIM *getRD() const {
        return RD;
    }

    unsigned updateEvery() const {
        return RD->update_every;
    }

    time_t latestTime() const {
        return Query(RD).latestTime();
    }

    time_t oldestTime() const {
        return Query(RD).oldestTime();
    }

    void setAnomalyRateRDName(const char *Name) const {
        rrddim_reset_name(AnomalyRateRD->rrdset, AnomalyRateRD, Name);
    }

    RRDDIM *getAnomalyRateRD() const {
        return AnomalyRateRD;
    }

    bool isTrained() const {
        return Trained;
    }

    bool isAnomalous() const {
        return AnomalyBit;
    }

    bool shouldTrain(const TimePoint &TP) const;

    bool isActive() const;

    MLResult trainModel();

    bool predict(CalculatedNumber Value, bool Exists);

    void updateAnomalyBitCounter(RRDSET *RS, unsigned Elapsed, bool IsAnomalous);

    std::pair<bool, double> detect(size_t WindowLength, bool Reset);

    std::array<KMeans, 1> getModels();

private:
    std::pair<CalculatedNumber *, size_t> getCalculatedNumbers();

public:
    RRDDIM *RD;
    RRDDIM *AnomalyRateRD;

    TimePoint LastTrainedAt;
    std::atomic<bool> Trained;
    std::atomic<bool> ConstantModel;

    CalculatedNumber AnomalyScore;
    std::atomic<bool> AnomalyBit;
    unsigned AnomalyBitCounter;

    std::vector<CalculatedNumber> CNs;
    std::array<KMeans, 1> Models;
    std::mutex Mutex;
};

} // namespace ml

#endif /* ML_DIMENSION_H */
