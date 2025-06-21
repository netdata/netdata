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

static inline void add_labels_to_power_supply(struct power_supply *ps, RRDSET *st) {
    rrdlabels_add(st->rrdlabels, "device", ps->name, RRDLABEL_SRC_AUTO);
}

static inline void rrdset_create_simple_prop(
    struct power_supply *ps,
    struct simple_property *prop,
    char *title,
    char *dim,
    collected_number divisor,
    char *units,
    long priority,
    int update_every)
{
    if (unlikely(!prop->st)) {
        char id[RRD_ID_LENGTH_MAX + 1], context[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "powersupply_%s", dim);
        snprintfz(context, RRD_ID_LENGTH_MAX, "powersupply.%s", dim);

        prop->st = rrdset_create_localhost(
            id,
            ps->name,
            NULL,
            dim,
            context,
            title,
            units,
            _COMMON_PLUGIN_NAME,
            _COMMON_PLUGIN_MODULE_NAME,
            priority,
            update_every,
            RRDSET_TYPE_LINE);

        add_labels_to_power_supply(ps, prop->st);
    }

    if (unlikely(!prop->rd))
        prop->rd = rrddim_add(prop->st, dim, NULL, 1, divisor, RRD_ALGORITHM_ABSOLUTE);
    rrddim_set_by_pointer(prop->st, prop->rd, prop->value);

    rrdset_done(prop->st);
}

#endif //NETDATA_POWER_SUPPLY_H
