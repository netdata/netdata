// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_DIMENSION_H
#define ML_DIMENSION_H

#include "ml-private.h"

namespace ml {

class RrdDimension {
public:
    RrdDimension(RRDDIM *RD) : RD(RD), Ops(&RD->state->query_ops) {}

    RRDDIM *getRD() const { return RD; }

    time_t latestTime() { return Ops->latest_time(RD); }

    time_t oldestTime() { return Ops->oldest_time(RD); }

    Seconds updateEvery() const { return Seconds{RD->update_every}; }

    virtual ~RrdDimension() {}

private:
    RRDDIM *RD;
    struct rrddim_volatile::rrddim_query_ops *Ops;
};

enum class MLResult {
    Success = 0,
    TryLockFailed,
    MissingData,
    ShouldNotTrainNow,
    NaN,
    NoModel,
};

class TrainableDimension : public RrdDimension {
public:
    TrainableDimension(RRDDIM *RD) : RrdDimension(RD) {}

    MLResult trainModel(TimePoint &Now);

    CalculatedNumber computeAnomalyScore(SamplesBuffer &SB) {
        return KM.anomalyScore(SB);
    }

private:
    std::pair<CalculatedNumber *, size_t> getCalculatedNumbers();

private:
    KMeans KM;
    TimePoint LastTrainedAt{SteadyClock::now()};
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

using Dimension = PredictableDimension;

} // namespace ml

#endif /* ML_DIMENSION_H */
