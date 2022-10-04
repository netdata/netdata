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

void ml_new_host(RRDHOST *RH) { (void) RH; }

void ml_delete_host(RRDHOST *RH) { (void) RH; }

char *ml_get_host_info(RRDHOST *RH) {
    (void) RH;
    return NULL;
}

char *ml_get_host_runtime_info(RRDHOST *RH) {
    (void) RH;
    return NULL;
}

char *ml_get_host_models(RRDHOST *RH) {
    (void) RH;
    return NULL;
}

void ml_new_dimension(RRDDIM *RD) { (void) RD; }

void ml_delete_dimension(RRDDIM *RD) { (void) RD; }

bool ml_is_anomalous(RRDDIM *RD, double Value, bool Exists) {
    (void) RD; (void) Value; (void) Exists;
    return false;
}

void ml_process_rrdr(RRDR *R, int MaxAnomalyRates) {
    (void) R;
    (void) MaxAnomalyRates;
}

void ml_dimension_update_name(RRDSET *RS, RRDDIM *RD, const char *name) {
    (void) RS;
    (void) RD;
    (void) name;
}

bool ml_streaming_enabled() {
    return false;
}

#endif
