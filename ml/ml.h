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
void rrddim_free(RRDSET *rs, RRDDIM *rd);

bool ml_capable();

bool ml_enabled(RRDHOST *rh);

void ml_init(void);

void ml_host_new(RRDHOST *rh);
void ml_host_delete(RRDHOST *rh);

void ml_chart_new(RRDSET *rs);
void ml_chart_delete(RRDSET *rs);

void ml_dimension_new(RRDDIM *rd);
void ml_dimension_delete(RRDDIM *rd);

void ml_get_host_info(RRDHOST *RH, BUFFER *wb);
char *ml_get_host_runtime_info(RRDHOST *RH);
char *ml_get_host_models(RRDHOST *RH);

bool ml_chart_update_begin(RRDSET *rs);
void ml_chart_update_end(RRDSET *rs);

bool ml_is_anomalous(RRDDIM *rd, time_t curr_time, double value, bool exists);

bool ml_streaming_enabled();

void ml_start_training_thread(RRDHOST *rh);
void ml_cancel_training_thread(RRDHOST *rh);
void ml_stop_training_thread(RRDHOST *rh);

#ifdef __cplusplus
};
#endif

#endif /* NETDATA_ML_H */
