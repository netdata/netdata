// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_DIMENSION_H
#define ML_DIMENSION_H

#include "BitBufferCounter.h"
#include "Config.h"

#include "ml-private.h"

namespace ml {

class RrdDimension {
public:
    RrdDimension(RRDDIM *RD) : RD(RD), Ops(&RD->tiers[0]->query_ops) { }

    RRDDIM *getRD() const { return RD; }

    time_t latestTime() { return Ops->latest_time(RD->tiers[0]->db_metric_handle); }

    time_t oldestTime() { return Ops->oldest_time(RD->tiers[0]->db_metric_handle); }

    unsigned updateEvery() const { return RD->update_every; }

    const std::string getID() const {
        RRDSET *RS = RD->rrdset;

        std::stringstream SS;
        SS << rrdset_context(RS) << "|" << RS->id << "|" << rrddim_name(RD);
        return SS.str();
    }

    bool isActive() const {
        if (rrdset_flag_check(RD->rrdset, RRDSET_FLAG_OBSOLETE))
            return false;

        if (rrddim_flag_check(RD, RRDDIM_FLAG_OBSOLETE))
            return false;

        return true;
    }

    void setAnomalyRateRD(RRDDIM *ARRD) { AnomalyRateRD = ARRD; }
    RRDDIM *getAnomalyRateRD() const { return AnomalyRateRD; }

    void setAnomalyRateRDName(const char *Name) const {
        rrddim_set_name(AnomalyRateRD->rrdset, AnomalyRateRD, Name);
    }

    virtual ~RrdDimension() {
        rrddim_free(AnomalyRateRD->rrdset, AnomalyRateRD);
    }

private:
    RRDDIM *RD;
    RRDDIM *AnomalyRateRD;

    struct rrddim_query_ops *Ops;

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
};

class PredictableDimension : public TrainableDimension {
public:
    PredictableDimension(RRDDIM *RD) : TrainableDimension(RD) {}

    std::pair<MLResult, bool> predict();

    void addValue(CalculatedNumber Value, bool Exists);

    bool isAnomalous() { return AnomalyBit; }

    void updateAnomalyBitCounter(RRDSET *RS, unsigned Elapsed, bool IsAnomalous) {
        AnomalyBitCounter += IsAnomalous;

        if (Elapsed == Cfg.DBEngineAnomalyRateEvery) {
            double AR = static_cast<double>(AnomalyBitCounter) / Cfg.DBEngineAnomalyRateEvery;
            rrddim_set_by_pointer(RS, getAnomalyRateRD(), AR * 1000);
            AnomalyBitCounter = 0;
        }
    }

private:
    CalculatedNumber AnomalyScore{0.0};
    std::atomic<bool> AnomalyBit{false};
    unsigned AnomalyBitCounter{0};

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
