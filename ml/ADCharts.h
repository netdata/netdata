// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_ADCHARTS_H
#define ML_ADCHARTS_H

#include "ml-private.h"

namespace ml {

void updateDimensionsChart(RRDHOST *RH,
                           collected_number NumTrainedDimensions,
                           collected_number NumNormalDimensions,
                           collected_number NumAnomalousDimensions);

void updateHostAndDetectionRateCharts(RRDHOST *RH, collected_number AnomalyRate);

void updateDetectionChart(RRDHOST *RH);

void updateTrainingChart(RRDHOST *RH, struct rusage *TRU);

} // namespace ml

#endif /* ML_ADCHARTS_H */
