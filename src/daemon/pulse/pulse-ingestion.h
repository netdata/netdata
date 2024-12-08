// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_INGESTION_H
#define NETDATA_PULSE_INGESTION_H

#include "daemon/common.h"

void pulse_queries_rrdset_collection_completed(size_t *points_read_per_tier_array);

#if defined(PULSE_INTERNALS)
void pulse_ingestion_do(bool extended);
#endif

#endif //NETDATA_PULSE_INGESTION_H
