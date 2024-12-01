// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_INGESTION_H
#define NETDATA_TELEMETRY_INGESTION_H

#include "daemon/common.h"

void telemetry_queries_rrdset_collection_completed(size_t *points_read_per_tier_array);

#if defined(TELEMETRY_INTERNALS)
void telemetry_ingestion_do(bool extended);
#endif

#endif //NETDATA_TELEMETRY_INGESTION_H
