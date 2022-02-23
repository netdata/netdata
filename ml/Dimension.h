// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_DIMENSION_H
#define ML_DIMENSION_H

#include "BitBufferCounter.h"
#include "Config.h"

#include "ml-private.h"

namespace ml {

class RrdDimension {
public:
    RrdDimension(RRDDIM *RD) : RD(RD), Ops(&RD->state->query_ops) {
        std::stringstream SS;
        SS << RD->rrdset->id << "|" << RD->name;
        ID = SS.str();
    }

    RRDDIM *getRD() const { return RD; }

    time_t latestTime() { return Ops->latest_time(RD); }

    time_t oldestTime() { return Ops->oldest_time(RD); }

    unsigned updateEvery() const { return RD->update_every; }

    const std::string getID() const { return ID; }

    virtual ~RrdDimension() {}

private:
    RRDDIM *RD;
    struct rrddim_volatile::rrddim_query_ops *Ops;

    std::string ID;
};

enum class MLResult {
    Success = 0,
    MissingData,
    NaN,
};

class TrainableDimension : public RrdDimension {
public:
    TrainableDimension(RRDDIM *RD) :
        RrdDimension(RD), TrainEvery(Cfg.TrainEvery * updateEvery()) {}

    MLResult trainModel();

    CalculatedNumber computeAnomalyScore(SamplesBuffer &SB) {
        return Trained ? KM.anomalyScore(SB) : 0.0;
    }

    bool shouldTrain(const TimePoint &TP) const {
        if (ConstantModel)
            return false;

        return (LastTrainedAt + TrainEvery) < TP;
    }

    bool isTrained() const { return Trained; }

    double updateTrainingDuration(double Duration) {
        return TrainingDuration.exchange(Duration);
    }

private:
    std::pair<CalculatedNumber *, size_t> getCalculatedNumbers();

public:
    TimePoint LastTrainedAt{Seconds{0}};

protected:
    std::atomic<bool> ConstantModel{false};

private:
    Seconds TrainEvery;
    KMeans KM;

    std::atomic<bool> Trained{false};
    std::atomic<double> TrainingDuration{0.0};
};

class PredictableDimension : public TrainableDimension {
public:
    PredictableDimension(RRDDIM *RD) : TrainableDimension(RD) {}

    std::pair<MLResult, bool> predict();

    void addValue(CalculatedNumber Value, bool Exists);

    bool isAnomalous() { return AnomalyBit; }

private:
    CalculatedNumber AnomalyScore{0.0};
    std::atomic<bool> AnomalyBit{false};

    std::vector<CalculatedNumber> CNs;
};

class DetectableDimension : public PredictableDimension {
public:
    DetectableDimension(RRDDIM *RD) : PredictableDimension(RD) {}

    std::pair<bool, double> detect(size_t WindowLength, bool Reset) {
        bool AnomalyBit = isAnomalous();

        if (Reset)
            NumSetBits = BBC.numSetBits();

        NumSetBits += AnomalyBit;
        BBC.insert(AnomalyBit);

        double AnomalyRate = static_cast<double>(NumSetBits) / WindowLength;
        return { AnomalyBit, AnomalyRate };
    }

private:
    BitBufferCounter BBC{static_cast<size_t>(Cfg.ADMinWindowSize)};
    size_t NumSetBits{0};
};

using Dimension = DetectableDimension;

} // namespace ml

#endif /* ML_DIMENSION_H */
