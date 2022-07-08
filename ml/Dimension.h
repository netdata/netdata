// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_DIMENSION_H
#define ML_DIMENSION_H

#include "Query.h"
#include "BitBufferCounter.h"
#include "Config.h"

#include "ml-private.h"

namespace ml {

enum class MLResult {
    Success = 0,
    MissingData,
    NaN,
};

class Dimension {
public:
    Dimension(RRDDIM *RD) :
        RD(RD),
        AnomalyRateRD(nullptr),
        LastTrainedAt(Seconds(0)),
        Trained(false),
        ConstantModel(false),
        AnomalyScore(0.0),
        AnomalyBit(0),
        AnomalyBitCounter(0),
        BBC(static_cast<size_t>(Cfg.ADMinWindowSize)),
        NumSetBits(0)
    {}

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

    void setAnomalyRateRD(RRDDIM *ARRD) {
        AnomalyRateRD = ARRD;
    }

    void setAnomalyRateRDName(const char *Name) const {
        rrddim_reset_name(AnomalyRateRD->rrdset, AnomalyRateRD, Name);
    }

    RRDDIM *getAnomalyRateRD() const {
        return AnomalyRateRD;
    }

    Seconds trainEvery() const {
        return Seconds{Cfg.TrainEvery * updateEvery()};
    }

    bool isTrained() const {
        return Trained;
    }

    CalculatedNumber computeAnomalyScore(SamplesBuffer &SB) {
        return isTrained() ? KM.anomalyScore(SB) : 0.0;
    }

    bool isAnomalous() { return AnomalyBit; }

    bool shouldTrain(const TimePoint &TP) const;

    std::string getID() const;

    bool isActive() const;

    MLResult trainModel();

    std::pair<MLResult, bool> predict();

    void addValue(CalculatedNumber Value, bool Exists);

    void updateAnomalyBitCounter(RRDSET *RS, unsigned Elapsed, bool IsAnomalous);

    std::pair<bool, double> detect(size_t WindowLength, bool Reset);

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

    BitBufferCounter BBC;
    size_t NumSetBits;

    std::vector<CalculatedNumber> CNs;
    KMeans KM;
};

} // namespace ml

#endif /* ML_DIMENSION_H */
