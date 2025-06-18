// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_POWER_SUPPLY_H
#define NETDATA_POWER_SUPPLY_H

#include "common-contexts.h"

struct ps_property_dim {
    char *name;
    char *filename;
    int fd;

    RRDDIM *rd;
    unsigned long long value;
    int always_zero;

    struct ps_property_dim *next;
};

struct ps_property {
    char *name;
    char *title;
    char *units;

    long priority;

    RRDSET *st;

    struct ps_property_dim *property_dim_root;

    struct ps_property *next;
};

struct simple_property {
    char *filename;
    int fd;

    RRDSET *st;
    RRDDIM *rd;
    bool ok;
    unsigned long long value;
};

struct power_supply {
    char *name;
    uint32_t hash;
    int found;

    struct simple_property *capacity, *power;

    struct ps_property *property_root;

    struct power_supply *next;
};


#endif //NETDATA_POWER_SUPPLY_H
