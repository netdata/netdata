// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_DB_DBENGINE_RETENTION_H
#define NETDATA_PULSE_DB_DBENGINE_RETENTION_H

#include "libnetdata/libnetdata.h"

#ifdef ENABLE_DBENGINE
void dbengine_retention_statistics(bool extended __maybe_unused);
#endif

#endif //NETDATA_PULSE_DB_DBENGINE_RETENTION_H
