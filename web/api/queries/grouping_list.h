// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_DATA_GROUPING_LIST_H
#define NETDATA_API_DATA_GROUPING_LIST_H

#include "libnetdata/libnetdata.h"

typedef struct grouping_functions {
    void *(*grouping_create)(struct rrdresult *r);
    void (*grouping_reset)(struct rrdresult *r, unsigned int index);
    void (*grouping_free)(struct rrdresult *r, unsigned int index);
    void (*grouping_add)(struct rrdresult *r, calculated_number value, unsigned int index);
    calculated_number (
        *grouping_flush)(struct rrdresult *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr, unsigned int index);
    void *grouping_data;
} GROUPING_FUNCTIONS;

#endif //NETDATA_API_DATA_GROUPING_LIST_H