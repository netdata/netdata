// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_ADCHARTS_H
#define ML_ADCHARTS_H

#include "ml-private.h"

void ml_update_dimensions_chart(ml_host_t *host, const ml_machine_learning_stats_t &mls);

void ml_update_host_and_detection_rate_charts(ml_host_t *host, collected_number anomaly_rate);

void ml_update_training_statistics_chart(ml_training_thread_t *training_thread, const ml_training_stats_t &ts);

#endif /* ML_ADCHARTS_H */
