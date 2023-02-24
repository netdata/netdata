// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_ADCHARTS_H
#define ML_ADCHARTS_H

#include "nml.h"

void nml_update_dimensions_chart(nml_host_t *host, const nml_machine_learning_stats_t &mls);

void nml_update_host_and_detection_rate_charts(nml_host_t *host, collected_number anomaly_rate);

void nml_update_training_statistics_chart(nml_host_t *host, const nml_training_stats_t &ts);

#endif /* ML_ADCHARTS_H */
