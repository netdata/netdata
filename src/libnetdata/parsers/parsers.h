// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PARSERS_H
#define NETDATA_PARSERS_H

#include "../libnetdata.h"

static inline bool parser_round_number_to_int64(NETDATA_DOUBLE value, int64_t *result) {
    value = roundndd(value);
    const NETDATA_DOUBLE limit = (NETDATA_DOUBLE)(UINT64_C(1) << 63);

    if(!netdata_double_isnumber(value) || value < -limit || value >= limit)
        return false;

    *result = (int64_t)value;
    return true;
}

static inline bool parser_round_number_to_uint64(NETDATA_DOUBLE value, uint64_t *result) {
    value = roundndd(value);
    const NETDATA_DOUBLE limit = (NETDATA_DOUBLE)(UINT64_C(1) << 63) * 2.0;

    if(!netdata_double_isnumber(value) || value < 0.0 || value >= limit)
        return false;

    *result = (uint64_t)value;
    return true;
}

#include "size.h"
#include "entries.h"
#include "duration.h"
#include "timeframe.h"

#endif //NETDATA_PARSERS_H
