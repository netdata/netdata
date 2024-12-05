// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_CHART_H
#define NETDATA_ML_CHART_H

#include "ml_host.h"

struct ml_dimension_t;

struct ml_chart_t {
    RRDSET *rs;
    ml_machine_learning_stats_t mls;
};

void ml_chart_update_dimension(ml_chart_t *chart, ml_dimension_t *dim, bool is_anomalous);

#endif /* NETDATA_ML_CHART_H */
