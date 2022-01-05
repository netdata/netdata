// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml.h"

#if !defined(ENABLE_ML)

void ml_init(void) {}

void ml_new_host(RRDHOST *RH) { (void) RH; }

void ml_delete_host(RRDHOST *RH) { (void) RH; }

char *ml_get_host_info(RRDHOST *RH) { (void) RH; }

void ml_new_dimension(RRDDIM *RD) { (void) RD; }

void ml_delete_dimension(RRDDIM *RD) { (void) RD; }

bool ml_is_anomalous(RRDDIM *RD, double Value, bool Exists) {
    (void) RD; (void) Value; (void) Exists;
    return false;
}

char *ml_get_anomaly_events(RRDHOST *RH, const char *AnomalyDetectorName,
                            int AnomalyDetectorVersion, time_t After, time_t Before) {
    (void) RH; (void) AnomalyDetectorName;
    (void) AnomalyDetectorVersion; (void) After; (void) Before;
    return NULL;
}

char *ml_get_anomaly_event_info(RRDHOST *RH, const char *AnomalyDetectorName,
                                int AnomalyDetectorVersion, time_t After, time_t Before) {
    (void) RH; (void) AnomalyDetectorName;
    (void) AnomalyDetectorVersion; (void) After; (void) Before;
    return NULL;
}

char *ml_get_anomaly_rate_info(RRDHOST *RH, time_t After, time_t Before) {
    (void) RH; (void) After; (void) Before;
    return NULL;
}

#endif
