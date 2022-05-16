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
extern void rrddim_free(RRDSET *st, RRDDIM *rd);

typedef void* ml_host_t;
typedef void* ml_dimension_t;

bool ml_capable();

bool ml_enabled(RRDHOST *RH);

void ml_init(void);

void ml_new_host(RRDHOST *RH);
void ml_delete_host(RRDHOST *RH);

char *ml_get_host_info(RRDHOST *RH);
char *ml_get_host_runtime_info(RRDHOST *RH);

void ml_new_dimension(RRDDIM *RD);
void ml_delete_dimension(RRDDIM *RD);

bool ml_is_anomalous(RRDDIM *RD, double value, bool exists);

char *ml_get_anomaly_events(RRDHOST *RH, const char *AnomalyDetectorName,
                            int AnomalyDetectorVersion, time_t After, time_t Before);

char *ml_get_anomaly_event_info(RRDHOST *RH, const char *AnomalyDetectorName,
                                int AnomalyDetectorVersion, time_t After, time_t Before);

void ml_process_rrdr(RRDR *R, int MaxAnomalyRates);

void ml_dimension_update_name(RRDSET *RS, RRDDIM *RD, const char *name);

bool ml_streaming_enabled();

#define ML_ANOMALY_RATES_CHART_ID  "anomaly_detection.anomaly_rates"

#if defined(ENABLE_ML_TESTS)
int test_ml(int argc, char *argv[]);
#endif

#ifdef __cplusplus
};
#endif

#endif /* NETDATA_ML_H */
