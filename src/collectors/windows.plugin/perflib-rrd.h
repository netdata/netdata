// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PERFLIB_RRD_H
#define NETDATA_PERFLIB_RRD_H

#include "database/rrd.h"

RRDDIM *perflib_rrddim_add(
    RRDSET *st,
    const char *id,
    const char *name,
    collected_number multiplier,
    collected_number divider,
    COUNTER_DATA *cd);
collected_number perflib_rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, COUNTER_DATA *cd);

#endif //NETDATA_PERFLIB_RRD_H
