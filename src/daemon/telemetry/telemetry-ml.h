// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_ML_H
#define NETDATA_TELEMETRY_ML_H

#include "daemon/common.h"

void telemetry_ml_models_consulted(size_t models_consulted);
void telemetry_ml_models_received();
void telemetry_ml_models_ignored();
void telemetry_ml_models_sent();
void global_statistics_ml_models_deserialization_failures();

#if defined(TELEMETRY_INTERNALS)
void telemetry_ml_do(bool extended);
#endif

#endif //NETDATA_TELEMETRY_ML_H
