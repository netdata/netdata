// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIMPLE_PATTERN_INTERNALS_H
#define NETDATA_SIMPLE_PATTERN_INTERNALS_H

#include "simple_pattern.h"

struct simple_pattern {
    const char *match;
    uint32_t len;

    SIMPLE_PREFIX_MODE mode;
    bool negative;
    bool case_sensitive;

    struct simple_pattern *child;
    struct simple_pattern *next;
};

#endif // NETDATA_SIMPLE_PATTERN_INTERNALS_H
