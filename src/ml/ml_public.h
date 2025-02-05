// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_PUBLIC_H
#define NETDATA_ML_PUBLIC_H

#ifdef __cplusplus
extern "C" {
#endif

#include "database/rrd.h"
#include "web/api/queries/rrdr.h"
#include "database/sqlite/vendored/sqlite3.h"

bool ml_capable();
bool ml_enabled(RRDHOST *rh);
bool ml_streaming_enabled();

void ml_init(void);
void ml_fini(void);

void ml_start_threads(void);
void ml_stop_threads(void);

void ml_host_new(RRDHOST *rh);
void ml_host_delete(RRDHOST *rh);

void ml_host_start(RRDHOST *RH);
void ml_host_stop(RRDHOST *RH);

void ml_host_get_info(RRDHOST *RH, BUFFER *wb);
void ml_host_get_detection_info(RRDHOST *RH, BUFFER *wb);
void ml_host_get_models(RRDHOST *RH, BUFFER *wb);

void ml_chart_new(RRDSET *rs);
void ml_chart_delete(RRDSET *rs);
bool ml_chart_update_begin(RRDSET *rs);
void ml_chart_update_end(RRDSET *rs);

void ml_dimension_new(RRDDIM *rd);
void ml_dimension_delete(RRDDIM *rd);
bool ml_dimension_is_anomalous(RRDDIM *rd, time_t curr_time, double value, bool exists);
void ml_dimension_received_anomaly(RRDDIM *rd, bool is_anomalous);

int ml_dimension_load_models(RRDDIM *rd, sqlite3_stmt **stmt);

void ml_update_global_statistics_charts(uint64_t models_consulted,
                                        uint64_t models_received,
                                        uint64_t models_sent,
                                        uint64_t models_ignored,
                                        uint64_t models_deserialization_failures,
                                        uint64_t memory_consumption,
                                        uint64_t memory_new,
                                        uint64_t memory_delete);

bool ml_host_get_host_status(RRDHOST *rh, struct ml_metrics_statistics *mlm);
bool ml_host_running(RRDHOST *rh);
uint64_t sqlite_get_ml_space(void);

bool ml_model_received_from_child(RRDHOST *host, const char *json);

#ifdef __cplusplus
};
#endif

#endif /* NETDATA_ML_PUBLIC_H */
