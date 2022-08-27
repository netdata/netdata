// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDIMVAR_H
#define NETDATA_RRDDIMVAR_H 1

#include "rrd.h"

// variables linked to individual dimensions
// We link variables to point the values that are already
// calculated / processed by the normal data collection process
// This means, there will be no speed penalty for using
// these variables
struct rrddimvar {
    STRING *prefix;
    STRING *suffix;

    STRING *key_id;                   // dimension id
    STRING *key_name;                 // dimension name
    STRING *key_contextid;            // context + dimension id
    STRING *key_contextname;          // context + dimension name
    STRING *key_fullidid;             // chart type.chart id + dimension id
    STRING *key_fullidname;           // chart type.chart id + dimension name
    STRING *key_fullnameid;           // chart type.chart name + dimension id
    STRING *key_fullnamename;         // chart type.chart name + dimension name

    RRDVAR_TYPE type;
    void *value;

    RRDVAR_OPTIONS options;

    RRDVAR *var_local_id;
    RRDVAR *var_local_name;

    RRDVAR *var_family_id;
    RRDVAR *var_family_name;
    RRDVAR *var_family_contextid;
    RRDVAR *var_family_contextname;

    RRDVAR *var_host_chartidid;
    RRDVAR *var_host_chartidname;
    RRDVAR *var_host_chartnameid;
    RRDVAR *var_host_chartnamename;

    struct rrddim *rrddim;

    struct rrddimvar *next;
};


extern void rrddimvar_rename_all(RRDDIM *rd);
extern RRDDIMVAR *rrddimvar_create(RRDDIM *rd, RRDVAR_TYPE type, const char *prefix, const char *suffix, void *value, RRDVAR_OPTIONS options);
extern void rrddimvar_free(RRDDIMVAR *rs);



#endif //NETDATA_RRDDIMVAR_H
