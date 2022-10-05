// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Dimension.h"
#include "Query.h"

using namespace ml;

bool Dimension::isActive() const {
    bool SetObsolete = rrdset_flag_check(RD->rrdset, RRDSET_FLAG_OBSOLETE);
    bool DimObsolete = rrddim_flag_check(RD, RRDDIM_FLAG_OBSOLETE);
    return !SetObsolete && !DimObsolete;
}

std::pair<CalculatedNumber *, size_t> Dimension::getCalculatedNumbers() {
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

        if (netdata_double_isnumber(Value)) {
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

MLResult Dimension::trainModel() {
    auto P = getCalculatedNumbers();
    CalculatedNumber *CNs = P.first;
    unsigned N = P.second;

    if (!CNs)
        return MLResult::MissingData;

    unsigned TargetNumSamples = Cfg.MaxTrainSamples * Cfg.RandomSamplingRatio;
    double SamplingRatio = std::min(static_cast<double>(TargetNumSamples) / N, 1.0);

    SamplesBuffer SB = SamplesBuffer(CNs, N, 1, Cfg.DiffN, Cfg.SmoothN, Cfg.LagN,
                                     SamplingRatio, Cfg.RandomNums);
    std::vector<DSample> Samples = SB.preprocess();

    KMeans KM;
    KM.train(Samples, Cfg.MaxKMeansIters);

    {
        std::lock_guard<std::mutex> Lock(Mutex);
        Models[0] = KM;
    }

    Trained = true;
    ConstantModel = true;

    delete[] CNs;
    return MLResult::Success;
}

bool Dimension::shouldTrain(const TimePoint &TP) const {
    if (ConstantModel)
        return false;

    return (LastTrainedAt + Seconds(Cfg.TrainEvery * updateEvery())) < TP;
}

bool Dimension::predict(CalculatedNumber Value, bool Exists) {
    if (!Exists) {
        CNs.clear();
        AnomalyBit = false;
        return false;
    }

    unsigned N = Cfg.DiffN + Cfg.SmoothN + Cfg.LagN;
    if (CNs.size() < N) {
        CNs.push_back(Value);
        AnomalyBit = false;
        return false;
    }

    std::rotate(std::begin(CNs), std::begin(CNs) + 1, std::end(CNs));

    if (CNs[N - 1] != Value)
        ConstantModel = false;

    CNs[N - 1] = Value;

    if (!isTrained() || ConstantModel) {
        AnomalyBit = false;
        return false;
    }

    CalculatedNumber *TmpCNs = new CalculatedNumber[N * (Cfg.LagN + 1)]();
    std::memcpy(TmpCNs, CNs.data(), N * sizeof(CalculatedNumber));
    SamplesBuffer SB = SamplesBuffer(TmpCNs, N, 1,
                                     Cfg.DiffN, Cfg.SmoothN, Cfg.LagN,
                                     1.0, Cfg.RandomNums);
    const DSample Sample = SB.preprocess().back();
    delete[] TmpCNs;

    std::unique_lock<std::mutex> Lock(Mutex, std::defer_lock);
    if (!Lock.try_lock()) {
        AnomalyBit = false;
        return false;
    }

    for (const auto &KM : Models) {
        double AnomalyScore = KM.anomalyScore(Sample);
        if (AnomalyScore == std::numeric_limits<CalculatedNumber>::quiet_NaN()) {
            AnomalyBit = false;
            continue;
        }

        if (AnomalyScore < (100 * Cfg.DimensionAnomalyScoreThreshold)) {
            AnomalyBit = false;
            return false;
        }
    }

    AnomalyBit = true;
    return true;
}

void Dimension::updateAnomalyBitCounter(RRDSET *RS, unsigned Elapsed, bool IsAnomalous) {
    AnomalyBitCounter += IsAnomalous;

    if (Elapsed == Cfg.DBEngineAnomalyRateEvery) {
        double AR = static_cast<double>(AnomalyBitCounter) / Cfg.DBEngineAnomalyRateEvery;
        rrddim_set_by_pointer(RS, getAnomalyRateRD(), AR * 1000);
        AnomalyBitCounter = 0;
    }
}

std::array<KMeans, 1> Dimension::getModels() {
    std::unique_lock<std::mutex> Lock(Mutex);
    return Models;
}
