// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_H
#define NETDATA_ML_H

#ifdef __cplusplus
extern "C" {
#endif

#include "daemon/common.h"

typedef void* ml_host_t;
typedef void* ml_dimension_t;

void ml_init(void);

void ml_new_host(RRDHOST *RH);
void ml_delete_host(RRDHOST *RH);

char *ml_get_host_info(RRDHOST *RH);

void ml_new_dimension(RRDDIM *RD);
void ml_delete_dimension(RRDDIM *RD);

bool ml_is_anomalous(RRDDIM *RD, double value, bool exists);

char *ml_get_anomaly_events(RRDHOST *RH, const char *AnomalyDetectorName,
                            int AnomalyDetectorVersion, time_t After, time_t Before);

char *ml_get_anomaly_event_info(RRDHOST *RH, const char *AnomalyDetectorName,
                                int AnomalyDetectorVersion, time_t After, time_t Before);

char *ml_get_anomaly_rate_info(RRDHOST *RH, time_t After, time_t Before);

#if defined(ENABLE_ML_TESTS)
int test_ml(int argc, char *argv[]);
int test_ml_anomaly_info_api_sql(void);
char *read_file_content_path(const char *filepath);
int test_ml_callback(void *IgnoreMe, int argc, char **argv, char **TableField);
#endif
#ifdef __cplusplus
};
#endif

#endif /* NETDATA_ML_H */
