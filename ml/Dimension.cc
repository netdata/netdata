// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Dimension.h"
#include "Query.h"

using namespace ml;

std::pair<CalculatedNumber *, size_t>
TrainableDimension::getCalculatedNumbers() {
    size_t MinN = Cfg.MinTrainSamples;
    size_t MaxN = Cfg.MaxTrainSamples;

    // Figure out what our time window should be.
    time_t BeforeT = now_realtime_sec() - 1;
    time_t AfterT = BeforeT - (MaxN * updateEvery());

    BeforeT -= (BeforeT % updateEvery());
    AfterT -= (AfterT % updateEvery());

    BeforeT = std::min(BeforeT, latestTime());
    AfterT = std::max(AfterT, oldestTime());

    if (AfterT >= BeforeT)
        return { nullptr, 0 };

    CalculatedNumber *CNs = new CalculatedNumber[MaxN * (Cfg.LagN + 1)]();

    // Start the query.
    unsigned Idx = 0;
    unsigned CollectedValues = 0;
    unsigned TotalValues = 0;

    CalculatedNumber LastValue = std::numeric_limits<CalculatedNumber>::quiet_NaN();
    Query Q = Query(getRD());

    Q.init(AfterT, BeforeT);
    while (!Q.isFinished()) {
        if (Idx == MaxN)
            break;

        auto P = Q.nextMetric();
        CalculatedNumber Value = P.second;

        if (calculated_number_isnumber(Value)) {
            CNs[Idx] = Value;
            LastValue = CNs[Idx];
            CollectedValues++;
        } else
            CNs[Idx] = LastValue;

        Idx++;
    }
    TotalValues = Idx;

    if (CollectedValues < MinN) {
        delete[] CNs;
        return { nullptr, 0 };
    }

    // Find first non-NaN value.
    for (Idx = 0; std::isnan(CNs[Idx]); Idx++, TotalValues--) { }

    // Overwrite NaN values.
    if (Idx != 0)
        memmove(CNs, &CNs[Idx], sizeof(CalculatedNumber) * TotalValues);

    return { CNs, TotalValues };
}

MLResult TrainableDimension::trainModel() {
    auto P = getCalculatedNumbers();
    CalculatedNumber *CNs = P.first;
    unsigned N = P.second;

    if (!CNs)
        return MLResult::MissingData;

    unsigned TargetNumSamples = Cfg.MaxTrainSamples * Cfg.RandomSamplingRatio;
    double SamplingRatio = std::min(static_cast<double>(TargetNumSamples) / N, 1.0);

    SamplesBuffer SB = SamplesBuffer(CNs, N, 1, Cfg.DiffN, Cfg.SmoothN, Cfg.LagN,
                                     SamplingRatio, Cfg.RandomNums);
    KM.train(SB, Cfg.MaxKMeansIters);

    Trained = true;
    ConstantModel = true;

    delete[] CNs;
    return MLResult::Success;
}

void PredictableDimension::addValue(CalculatedNumber Value, bool Exists) {
    if (!Exists) {
        CNs.clear();
        return;
    }

    unsigned N = Cfg.DiffN + Cfg.SmoothN + Cfg.LagN;
    if (CNs.size() < N) {
        CNs.push_back(Value);
        return;
    }

    std::rotate(std::begin(CNs), std::begin(CNs) + 1, std::end(CNs));

    if (CNs[N - 1] != Value)
        ConstantModel = false;

    CNs[N - 1] = Value;
}

std::pair<MLResult, bool> PredictableDimension::predict() {
    unsigned N = Cfg.DiffN + Cfg.SmoothN + Cfg.LagN;
    if (CNs.size() != N) {
        AnomalyBit = false;
        return { MLResult::MissingData, AnomalyBit };
    }

    CalculatedNumber *TmpCNs = new CalculatedNumber[N * (Cfg.LagN + 1)]();
    std::memcpy(TmpCNs, CNs.data(), N * sizeof(CalculatedNumber));

    SamplesBuffer SB = SamplesBuffer(TmpCNs, N, 1, Cfg.DiffN, Cfg.SmoothN, Cfg.LagN,
                                     1.0, Cfg.RandomNums);
    AnomalyScore = computeAnomalyScore(SB);
    delete[] TmpCNs;

    if (AnomalyScore == std::numeric_limits<CalculatedNumber>::quiet_NaN()) {
        AnomalyBit = false;
        return { MLResult::NaN, AnomalyBit };
    }

    AnomalyBit = AnomalyScore >= (100 * Cfg.DimensionAnomalyScoreThreshold);
    return { MLResult::Success, AnomalyBit };
}
