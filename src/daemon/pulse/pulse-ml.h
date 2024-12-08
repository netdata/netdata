// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_ML_H
#define NETDATA_PULSE_ML_H

#include "daemon/common.h"

#ifdef __cplusplus
extern "C" {
#endif

void pulse_ml_models_consulted(size_t models_consulted);
void pulse_ml_models_received();
void pulse_ml_models_ignored();
void pulse_ml_models_sent();

void pulse_ml_memory_allocated(size_t n);
void pulse_ml_memory_freed(size_t n);

void global_statistics_ml_models_deserialization_failures();

uint64_t pulse_ml_get_current_memory_usage(void);

#if defined(PULSE_INTERNALS)
void pulse_ml_do(bool extended);
#endif

#ifdef __cplusplus
}
#endif


#endif //NETDATA_PULSE_ML_H
