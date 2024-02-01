// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDFUNCTIONS_INTERNALS_H
#define NETDATA_RRDFUNCTIONS_INTERNALS_H

#include "rrd.h"

#include "rrdcollector-internals.h"

typedef enum __attribute__((packed)) {
    RRD_FUNCTION_LOCAL  = (1 << 0),
    RRD_FUNCTION_GLOBAL = (1 << 1),
    RRD_FUNCTION_DYNCFG = (1 << 2),

    // this is 8-bit
} RRD_FUNCTION_OPTIONS;

struct rrd_host_function {
    bool sync;                      // when true, the function is called synchronously
    RRD_FUNCTION_OPTIONS options;   // RRD_FUNCTION_OPTIONS
    HTTP_ACCESS access;
    STRING *help;
    STRING *tags;
    int timeout;                    // the default timeout of the function
    int priority;

    rrd_function_execute_cb_t execute_cb;
    void *execute_cb_data;

    struct rrd_collector *collector;
};

size_t rrd_functions_sanitize(char *dst, const char *src, size_t dst_len);
int rrd_functions_find_by_name(RRDHOST *host, BUFFER *wb, const char *name, size_t key_length, const DICTIONARY_ITEM **item);

#endif //NETDATA_RRDFUNCTIONS_INTERNALS_H
