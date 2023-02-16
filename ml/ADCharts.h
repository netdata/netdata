// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_ADCHARTS_H
#define ML_ADCHARTS_H

#include "Stats.h"
#include "ml-private.h"

namespace ml {

void updateDimensionsChart(RRDHOST *RH, const MachineLearningStats &MLS);

void updateHostAndDetectionRateCharts(RRDHOST *RH, collected_number AnomalyRate);

void updateResourceUsageCharts(RRDHOST *RH, const struct rusage &PredictionRU, const struct rusage &TrainingRU);

void updateTrainingStatisticsChart(RRDHOST *RH, const TrainingStats &TS);

} // namespace ml

#endif /* ML_ADCHARTS_H */
