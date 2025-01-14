// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDIM_COLLECTION_H
#define NETDATA_RRDDIM_COLLECTION_H

#include "rrddim.h"

void store_metric_at_tier(RRDDIM *rd, size_t tier, struct rrddim_tier *t, STORAGE_POINT sp, usec_t now_ut);
void store_metric_collection_completed(void);

#endif //NETDATA_RRDDIM_COLLECTION_H
