// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml.h"

#if !defined(ENABLE_ML)

bool ml_capable() {
    return false;
}

bool ml_enabled(RRDHOST *RH) {
    (void) RH;
    return false;
}

void ml_init(void) {}

void ml_host_new(RRDHOST *RH) {
    UNUSED(RH);
}

void ml_host_delete(RRDHOST *RH) {
    UNUSED(RH);
}

void ml_chart_new(RRDSET *RS) {
    UNUSED(RS);
}

void ml_chart_delete(RRDSET *RS) {
    UNUSED(RS);
}

void ml_dimension_new(RRDDIM *RD) {
    UNUSED(RD);
}

void ml_dimension_delete(RRDDIM *RD) {
    UNUSED(RD);
}

void ml_start_anomaly_detection_threads(RRDHOST *RH) {
    UNUSED(RH);
}

void ml_stop_anomaly_detection_threads(RRDHOST *RH) {
    UNUSED(RH);
}

void ml_get_host_info(RRDHOST *RH, BUFFER *wb) {
    (void) RH;
    (void) wb;
}

char *ml_get_host_runtime_info(RRDHOST *RH) {
    (void) RH;
    return NULL;
}

bool ml_chart_update_begin(RRDSET *RS) {
    (void) RS;
    return false;
}

void ml_chart_update_end(RRDSET *RS) {
    (void) RS;
}

char *ml_get_host_models(RRDHOST *RH) {
    (void) RH;
    return NULL;
}

bool ml_is_anomalous(RRDDIM *RD, time_t CurrT, double Value, bool Exists) {
    (void) RD;
    (void) CurrT;
    (void) Value;
    (void) Exists;
    return false;
}

bool ml_streaming_enabled() {
    return false;
}

#endif
