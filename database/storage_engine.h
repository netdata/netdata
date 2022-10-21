// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STORAGEENGINEAPI_H
#define NETDATA_STORAGEENGINEAPI_H

#include "rrd.h"

STORAGE_ENGINE* storage_engine_get(RRD_MEMORY_MODE mmode);
STORAGE_ENGINE* storage_engine_find(const char* name);

// Iterator over existing engines
STORAGE_ENGINE* storage_engine_foreach_init();
STORAGE_ENGINE* storage_engine_foreach_next(STORAGE_ENGINE* it);

#endif
