// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_DIMENSION_H
#define ML_DIMENSION_H

#include "Mutex.h"
#include "Stats.h"
#include "Query.h"
#include "Config.h"

#include "ml-private.h"

namespace ml {

static inline std::string getMLDimensionID(RRDDIM *RD) {
    RRDSET *RS = RD->rrdset;

    std::stringstream SS;
    SS << rrdset_context(RS) << "|" << rrdset_id(RS) << "|" << rrddim_name(RD);
    return SS.str();
}

enum class MachineLearningStatus {
    // Enable training/prediction
    Enabled,

    // Disable due to update every being different from the host's
    DisabledDueToUniqueUpdateEvery,

    // Disable because configuration pattern matches the chart's id
    DisabledDueToExcludedChart,
};

enum class TrainingStatus {
    // We don't have a model for this dimension
    Untrained,

    // Request for training sent, but we don't have any models yet
    PendingWithoutModel,

    // Request to update existing models sent
    PendingWithModel,

    // Have a valid, up-to-date model
    Trained,
};

enum class MetricType {
    // The dimension has constant values, no need to train
    Constant,

    // The dimension's values fluctuate, we need to generate a model
    Variable,
};

struct TrainingRequest {
    // Chart/dimension we want to train
    STRING *ChartId;
    STRING *DimensionId;
    
    // Creation time of request
    time_t RequestTime;
    
    // First/last entry of this dimension in DB
    // at the point the request was made
    time_t FirstEntryOnRequest;
    time_t LastEntryOnRequest;
};

void dumpTrainingRequest(const TrainingRequest &TrainingReq, const char *Prefix);

enum TrainingResult {
    // We managed to create a KMeans model
    Ok,
    // Could not query DB with a correct time range
    InvalidQueryTimeRange,
    // Did not gather enough data from DB to run KMeans
    NotEnoughCollectedValues,
    // Acquired a null dimension
    NullAcquiredDimension,
    // Chart is under replication
    ChartUnderReplication,
};

struct TrainingResponse {
    // Time when the request for this response was made
    time_t RequestTime;

    // First/last entry of the dimension in DB when generating the request
    time_t FirstEntryOnRequest;
    time_t LastEntryOnRequest;
    
    // First/last entry of the dimension in DB when generating the response
    time_t FirstEntryOnResponse;
    time_t LastEntryOnResponse;
    
    // After/Before timestamps of our DB query
    time_t QueryAfterT;
    time_t QueryBeforeT;
    
    // Actual after/before returned by the DB query ops
    time_t DbAfterT;
    time_t DbBeforeT;
    
    // Number of doubles returned by the DB query
    size_t CollectedValues;
    
    // Number of values we return to the caller
    size_t TotalValues;

    // Result of training response
    TrainingResult Result;
};

void dumpTrainingResponse(const TrainingResponse &TrainingResp, const char *Prefix);

class Dimension {
public:
    Dimension(RRDDIM *RD) :
        RD(RD),
        MT(MetricType::Constant),
        TS(TrainingStatus::Untrained),
        TR(),
        LastTrainingTime(0)
    {
        if (simple_pattern_matches(Cfg.SP_ChartsToSkip, rrdset_name(RD->rrdset)))
            MLS = MachineLearningStatus::DisabledDueToExcludedChart;
        else if (RD->update_every != RD->rrdset->rrdhost->rrd_update_every)
            MLS = MachineLearningStatus::DisabledDueToUniqueUpdateEvery;
        else
            MLS = MachineLearningStatus::Enabled;

        Models.reserve(Cfg.NumModelsToUse);
    }

    RRDDIM *getRD() const {
        return RD;
    }

    unsigned updateEvery() const {
        return RD->update_every;
    }

    MetricType getMT() const {
        return MT;
    }

    TrainingStatus getTS() const {
        return TS;
    }

    MachineLearningStatus getMLS() const {
        return MLS;
    }

    TrainingResult trainModel(const TrainingRequest &TR);

    void scheduleForTraining(time_t CurrT);

    bool predict(time_t CurrT, CalculatedNumber Value, bool Exists);

    std::vector<KMeans> getModels();
    
    void dump() const;

private:
    TrainingRequest getTrainingRequest(time_t CurrT) const {
        return TrainingRequest {
                string_dup(RD->rrdset->id),
                string_dup(RD->id),
                CurrT,
                rrddim_first_entry_s(RD),
                rrddim_last_entry_s(RD)
        };
    }

private:
    std::pair<CalculatedNumber *, TrainingResponse> getCalculatedNumbers(const TrainingRequest &TrainingReq);

public:
    RRDDIM *RD;
    MetricType MT;
    TrainingStatus TS;
    TrainingResponse TR;

    time_t LastTrainingTime;

    MachineLearningStatus MLS;

    std::vector<CalculatedNumber> CNs;
    DSample Feature;
    std::vector<KMeans> Models;
    Mutex M;
};

} // namespace ml

#endif /* ML_DIMENSION_H */
