// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_H
#define NETDATA_ML_H

#ifdef __cplusplus
extern "C" {
#endif

#include "daemon/common.h"
#include "web/api/queries/rrdr.h"

bool ml_capable();
bool ml_enabled(RRDHOST *rh);
bool ml_streaming_enabled();
void ml_init(void);

void ml_host_new(RRDHOST *rh);
void ml_host_delete(RRDHOST *rh);

void ml_host_get_info(RRDHOST *RH, BUFFER *wb);
void ml_host_get_detection_info(RRDHOST *RH, BUFFER *wb);
void ml_host_get_models(RRDHOST *RH, BUFFER *wb);

void ml_host_start_training_thread(RRDHOST *rh);
void ml_host_cancel_training_thread(RRDHOST *rh);
void ml_host_stop_training_thread(RRDHOST *rh);

void ml_chart_new(RRDSET *rs);
void ml_chart_delete(RRDSET *rs);
bool ml_chart_update_begin(RRDSET *rs);
void ml_chart_update_end(RRDSET *rs);

void ml_dimension_new(RRDDIM *rd);
void ml_dimension_delete(RRDDIM *rd);
bool ml_dimension_is_anomalous(RRDDIM *rd, time_t curr_time, double value, bool exists);

#ifdef __cplusplus
};
#endif

#endif /* NETDATA_ML_H */
