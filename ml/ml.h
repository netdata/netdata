// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_H
#define NETDATA_ML_H

#ifdef __cplusplus
extern "C" {
#endif

#include "daemon/common.h"
#include "web/api/queries/rrdr.h"

// This is a DBEngine function redeclared here so that we can free
// the anomaly rate dimension, whenever its backing dimension is freed.
void rrddim_free(RRDSET *st, RRDDIM *rd);

bool ml_capable();

bool ml_enabled(RRDHOST *RH);

void ml_init(void);

void ml_host_new(RRDHOST *RH);
void ml_host_delete(RRDHOST *RH);

void ml_chart_new(RRDSET *RS);
void ml_chart_delete(RRDSET *RS);

void ml_dimension_new(RRDDIM *RD);
void ml_dimension_delete(RRDDIM *RD);

void ml_start_anomaly_detection_threads(RRDHOST *RH);
void ml_stop_anomaly_detection_threads(RRDHOST *RH);
void ml_cancel_anomaly_detection_threads(RRDHOST *RH);

void ml_get_host_info(RRDHOST *RH, BUFFER *wb);
char *ml_get_host_runtime_info(RRDHOST *RH);
char *ml_get_host_models(RRDHOST *RH);

bool ml_chart_update_begin(RRDSET *RS);
void ml_chart_update_end(RRDSET *RS);

bool ml_is_anomalous(RRDDIM *RD, time_t curr_t, double value, bool exists);

bool ml_streaming_enabled();

#ifdef __cplusplus
};
#endif

#endif /* NETDATA_ML_H */
