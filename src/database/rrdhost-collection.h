// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDHOST_COLLECTION_H
#define NETDATA_RRDHOST_COLLECTION_H

#include "rrdhost.h"

void rrdhost_finalize_collection(RRDHOST *host);
void rrd_finalize_collection_for_all_hosts(void);

#endif //NETDATA_RRDHOST_COLLECTION_H
