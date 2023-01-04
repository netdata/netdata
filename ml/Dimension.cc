// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Dimension.h"
#include "Query.h"
#include "Host.h"

using namespace ml;

static const char *mls2str(MachineLearningStatus MLS) {
    switch (MLS) {
        case ml::MachineLearningStatus::Enabled:
            return "enabled";
        case ml::MachineLearningStatus::DisabledDueToUniqueUpdateEvery:
            return "disabled-ue";
        case ml::MachineLearningStatus::DisabledDueToExcludedChart:
            return "disabled-sp";
        default:
            return "unknown";
    }
}

static const char *mt2str(MetricType MT) {
    switch (MT) {
        case ml::MetricType::Constant:
            return "constant";
        case ml::MetricType::Variable:
            return "variable";
        default:
            return "unknown";
    }
}

static const char *ts2str(TrainingStatus TS) {
    switch (TS) {
        case ml::TrainingStatus::PendingWithModel:
            return "pending-with-model";
        case ml::TrainingStatus::PendingWithoutModel:
            return "pending-without-model";
        case ml::TrainingStatus::Trained:
            return "trained";
        case ml::TrainingStatus::Untrained:
            return "untrained";
        default:
            return "unknown";
    }
}

static const char *tr2str(TrainingResult TR) {
    switch (TR) {
        case ml::TrainingResult::Ok:
            return "ok";
        case ml::TrainingResult::InvalidQueryTimeRange:
            return "invalid-query";
        case ml::TrainingResult::NotEnoughCollectedValues:
            return "missing-values";
        case ml::TrainingResult::NullAcquiredDimension:
            return "null-acquired-dim";
        case ml::TrainingResult::ChartUnderReplication:
            return "chart-under-replication";
        default:
            return "unknown";
    }
}

std::pair<CalculatedNumber *, TrainingResponse> Dimension::getCalculatedNumbers(const TrainingRequest &TrainingReq) {
    TrainingResponse TrainingResp = {};

    TrainingResp.RequestTime = TrainingReq.RequestTime;
    TrainingResp.FirstEntryOnRequest = TrainingReq.FirstEntryOnRequest;
    TrainingResp.LastEntryOnRequest = TrainingReq.LastEntryOnRequest;

    TrainingResp.FirstEntryOnResponse = rrddim_first_entry_t_of_tier(RD, 0);
    TrainingResp.LastEntryOnResponse = rrddim_last_entry_t_of_tier(RD, 0);

    size_t MinN = Cfg.MinTrainSamples;
    size_t MaxN = Cfg.MaxTrainSamples;

    // Figure out what our time window should be.
    TrainingResp.QueryBeforeT = TrainingResp.LastEntryOnResponse;
    TrainingResp.QueryAfterT = std::max(
        TrainingResp.QueryBeforeT - static_cast<time_t>((MaxN - 1) * updateEvery()),
        TrainingResp.FirstEntryOnResponse
    );

    if (TrainingResp.QueryAfterT >= TrainingResp.QueryBeforeT) {
        TrainingResp.Result = TrainingResult::InvalidQueryTimeRange;
        return { nullptr, TrainingResp };
    }

    if (rrdset_is_replicating(RD->rrdset)) {
        TrainingResp.Result = TrainingResult::ChartUnderReplication;
        return { nullptr, TrainingResp };
    }

    CalculatedNumber *CNs = new CalculatedNumber[MaxN * (Cfg.LagN + 1)]();

    // Start the query.
    size_t Idx = 0;

    CalculatedNumber LastValue = std::numeric_limits<CalculatedNumber>::quiet_NaN();
    Query Q = Query(getRD());

    Q.init(TrainingResp.QueryAfterT, TrainingResp.QueryBeforeT);
    while (!Q.isFinished()) {
        if (Idx == MaxN)
            break;

        auto P = Q.nextMetric();

        CalculatedNumber Value = P.second;

        if (netdata_double_isnumber(Value)) {
            if (!TrainingResp.DbAfterT)
                TrainingResp.DbAfterT = P.first;
            TrainingResp.DbBeforeT = P.first;

            CNs[Idx] = Value;
            LastValue = CNs[Idx];
            TrainingResp.CollectedValues++;
        } else
            CNs[Idx] = LastValue;

        Idx++;
    }
    TrainingResp.TotalValues = Idx;

    if (TrainingResp.CollectedValues < MinN) {
        TrainingResp.Result = TrainingResult::NotEnoughCollectedValues;

        delete[] CNs;
        return { nullptr, TrainingResp };
    }

    // Find first non-NaN value.
    for (Idx = 0; std::isnan(CNs[Idx]); Idx++, TrainingResp.TotalValues--) { }

    // Overwrite NaN values.
    if (Idx != 0)
        memmove(CNs, &CNs[Idx], sizeof(CalculatedNumber) * TrainingResp.TotalValues);

    TrainingResp.Result = TrainingResult::Ok;
    return { CNs, TrainingResp };
}

TrainingResult Dimension::trainModel(const TrainingRequest &TrainingReq) {
    auto P = getCalculatedNumbers(TrainingReq);
    CalculatedNumber *CNs = P.first;
    TrainingResponse TrainingResp = P.second;

    if (TrainingResp.Result != TrainingResult::Ok) {
        std::lock_guard<Mutex> L(M);

        MT = MetricType::Constant;

        switch (TS) {
            case TrainingStatus::PendingWithModel:
                TS = TrainingStatus::Trained;
                break;
            case TrainingStatus::PendingWithoutModel:
                TS = TrainingStatus::Untrained;
                break;
            default:
                break;
        }

        TR = TrainingResp;

        LastTrainingTime = TrainingResp.LastEntryOnResponse;
        return TrainingResp.Result;
    }

    unsigned N = TrainingResp.TotalValues;
    unsigned TargetNumSamples = Cfg.MaxTrainSamples * Cfg.RandomSamplingRatio;
    double SamplingRatio = std::min(static_cast<double>(TargetNumSamples) / N, 1.0);

    SamplesBuffer SB = SamplesBuffer(CNs, N, 1, Cfg.DiffN, Cfg.SmoothN, Cfg.LagN,
                                     SamplingRatio, Cfg.RandomNums);
    std::vector<DSample> Samples;
    SB.preprocess(Samples);

    KMeans KM;
    KM.train(Samples, Cfg.MaxKMeansIters);

    {
        std::lock_guard<Mutex> L(M);

        if (Models.size() < Cfg.NumModelsToUse) {
            Models.push_back(std::move(KM));
        } else {
            std::rotate(std::begin(Models), std::begin(Models) + 1, std::end(Models));
            Models[Models.size() - 1] = std::move(KM);
        }

        MT = MetricType::Constant;
        TS = TrainingStatus::Trained;
        TR = TrainingResp;
        LastTrainingTime = rrddim_last_entry_t(RD);
    }

    delete[] CNs;
    return TrainingResp.Result;
}

void Dimension::scheduleForTraining(time_t CurrT) {
    switch (MT) {
        case MetricType::Constant: {
            return;
        } default:
            break;
    }

    switch (TS) {
        case TrainingStatus::PendingWithModel:
        case TrainingStatus::PendingWithoutModel:
            break;
        case TrainingStatus::Untrained: {
            Host *H = reinterpret_cast<Host *>(RD->rrdset->rrdhost->ml_host);
            TS = TrainingStatus::PendingWithoutModel;
            H->scheduleForTraining(getTrainingRequest(CurrT));
            break;
        }
        case TrainingStatus::Trained: {
            bool NeedsTraining = LastTrainingTime + (Cfg.TrainEvery * updateEvery()) < CurrT;

            if (NeedsTraining) {
                Host *H = reinterpret_cast<Host *>(RD->rrdset->rrdhost->ml_host);
                TS = TrainingStatus::PendingWithModel;
                H->scheduleForTraining(getTrainingRequest(CurrT));
            }
            break;
        }
    }
}

bool Dimension::predict(time_t CurrT, CalculatedNumber Value, bool Exists) {
    // Nothing to do if ML is disabled for this dimension
    if (MLS != MachineLearningStatus::Enabled)
        return false;

    // Don't treat values that don't exist as anomalous
    if (!Exists) {
        CNs.clear();
        return false;
    }

    // Save the value and return if we don't have enough values for a sample
    unsigned N = Cfg.DiffN + Cfg.SmoothN + Cfg.LagN;
    if (CNs.size() < N) {
        CNs.push_back(Value);
        return false;
    }

    // Push the value and check if it's different from the last one
    bool SameValue = true;
    std::rotate(std::begin(CNs), std::begin(CNs) + 1, std::end(CNs));
    if (CNs[N - 1] != Value)
        SameValue = false;
    CNs[N - 1] = Value;

    // Create the sample
    CalculatedNumber TmpCNs[N * (Cfg.LagN + 1)];
    memset(TmpCNs, 0, N * (Cfg.LagN + 1) * sizeof(CalculatedNumber));
    std::memcpy(TmpCNs, CNs.data(), N * sizeof(CalculatedNumber));
    SamplesBuffer SB = SamplesBuffer(TmpCNs, N, 1,
                                     Cfg.DiffN, Cfg.SmoothN, Cfg.LagN,
                                     1.0, Cfg.RandomNums);
    SB.preprocess(Feature);

    /*
     * Lock to predict and possibly schedule the dimension for training
    */

    std::unique_lock<Mutex> L(M, std::defer_lock);
    if (!L.try_lock()) {
        return false;
    }

    // Mark the metric time as variable if we received different values
    if (!SameValue)
        MT = MetricType::Variable;

    // Decide if the dimension needs to be scheduled for training
    scheduleForTraining(CurrT);

    // Nothing to do if we don't have a model
    switch (TS) {
        case TrainingStatus::Untrained:
        case TrainingStatus::PendingWithoutModel:
            return false;
        default:
            break;
    }

    /*
     * Use the KMeans models to check if the value is anomalous
    */

    size_t ModelsConsulted = 0;
    size_t Sum = 0;

    for (const auto &KM : Models) {
        ModelsConsulted++;

        double AnomalyScore = KM.anomalyScore(Feature);
        if (AnomalyScore == std::numeric_limits<CalculatedNumber>::quiet_NaN())
            continue;

        if (AnomalyScore < (100 * Cfg.DimensionAnomalyScoreThreshold)) {
            global_statistics_ml_models_consulted(ModelsConsulted);
            return false;
        }

        Sum += 1;
    }

    global_statistics_ml_models_consulted(ModelsConsulted);
    return Sum;
}

std::vector<KMeans> Dimension::getModels() {
    std::unique_lock<Mutex> L(M);
    return Models;
}

void Dimension::dump() const {
    const char *ChartId = rrdset_id(RD->rrdset);
    const char *DimensionId = rrddim_id(RD);

    const char *MLS_Str = mls2str(MLS);
    const char *MT_Str = mt2str(MT);
    const char *TS_Str = ts2str(TS);
    const char *TR_Str = tr2str(TR.Result);

    const char *fmt =
        "[ML] %s.%s: MLS=%s, MT=%s, TS=%s, Result=%s, "
        "ReqTime=%ld, FEOReq=%ld, LEOReq=%ld, "
        "FEOResp=%ld, LEOResp=%ld, QTR=<%ld, %ld>, DBTR=<%ld, %ld>, Collected=%zu, Total=%zu";

    error(fmt,
          ChartId, DimensionId, MLS_Str, MT_Str, TS_Str, TR_Str,
          TR.RequestTime, TR.FirstEntryOnRequest, TR.LastEntryOnRequest,
          TR.FirstEntryOnResponse, TR.LastEntryOnResponse,
          TR.QueryAfterT, TR.QueryBeforeT, TR.DbAfterT, TR.DbBeforeT, TR.CollectedValues, TR.TotalValues
    );
}
