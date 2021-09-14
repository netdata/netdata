// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml.h"

#if !defined(ENABLE_ML)

void ml_init(void) {}

void ml_new_host(RRDHOST *RH) { (void) RH; }

void ml_delete_host(RRDHOST *RH) { (void) RH; }

void ml_new_dimension(RRDDIM *RD) { (void) RD; }

void ml_delete_dimension(RRDDIM *RD) { (void) RD; }

bool ml_is_anomalous(RRDDIM *RD, double Value, bool Exists) {
    (void) RD; (void) Value; (void) Exists;
    return false;
}

#endif
