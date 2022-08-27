// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSETVAR_H
#define NETDATA_RRDSETVAR_H 1

#include "rrd.h"

// variables linked to charts
// We link variables to point to the values that are already
// calculated / processed by the normal data collection process
// This means, there will be no speed penalty for using
// these variables

struct rrdsetvar {
    STRING *variable;               // variable name

    STRING *key_fullid;             // chart type.chart id.variable
    STRING *key_fullname;           // chart type.chart name.variable

    RRDVAR_TYPE type;
    void *value;

    RRDVAR_OPTIONS options;

    RRDVAR *var_local;
    RRDVAR *var_family;
    RRDVAR *var_host;
    RRDVAR *var_family_name;
    RRDVAR *var_host_name;

    struct rrdset *rrdset;

    struct rrdsetvar *next;
};

extern RRDSETVAR *rrdsetvar_custom_chart_variable_create(RRDSET *st, const char *name);
extern void rrdsetvar_custom_chart_variable_set(RRDSETVAR *rv, NETDATA_DOUBLE value);

extern void rrdsetvar_rename_all(RRDSET *st);
extern RRDSETVAR *rrdsetvar_create(RRDSET *st, const char *variable, RRDVAR_TYPE type, void *value, RRDVAR_OPTIONS options);
extern void rrdsetvar_free(RRDSETVAR *rs);

#endif //NETDATA_RRDSETVAR_H
