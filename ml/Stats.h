// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_STATS_H
#define ML_STATS_H

#include "ml-private.h"

namespace ml {

struct MachineLearningStats {
    size_t NumMachineLearningStatusEnabled;
    size_t NumMachineLearningStatusDisabledUE;
    size_t NumMachineLearningStatusDisabledSP;

    size_t NumMetricTypeConstant;
    size_t NumMetricTypeVariable;

    size_t NumTrainingStatusUntrained;
    size_t NumTrainingStatusPendingWithoutModel;
    size_t NumTrainingStatusTrained;
    size_t NumTrainingStatusPendingWithModel;

    size_t NumAnomalousDimensions;
    size_t NumNormalDimensions;
};

struct TrainingStats {
    struct rusage TrainingRU;

    size_t QueueSize;
    size_t NumPoppedItems;

    usec_t AllottedUT;
    usec_t ConsumedUT;
    usec_t RemainingUT;

    size_t TrainingResultOk;
    size_t TrainingResultInvalidQueryTimeRange;
    size_t TrainingResultNotEnoughCollectedValues;
    size_t TrainingResultNullAcquiredDimension;
    size_t TrainingResultChartUnderReplication;
};

} // namespace ml

#endif /* ML_STATS_H */
