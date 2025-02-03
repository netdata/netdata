// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTIONS_H
#define NETDATA_FUNCTIONS_H

#include "database/rrd.h"

#include "function-metrics-cardinality.h"
#include "function-streaming.h"
#include "function-progress.h"
#include "function-bearer_get_token.h"

void global_functions_add(void);

#endif //NETDATA_FUNCTIONS_H
