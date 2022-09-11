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
    void *value;

    RRDVAR_FLAGS flags:16;
    RRDVAR_TYPE type:8;

    const RRDVAR_ACQUIRED *rrdvar_local_dim_id;
    const RRDVAR_ACQUIRED *rrdvar_local_dim_name;

    const RRDVAR_ACQUIRED *rrdvar_family_id;
    const RRDVAR_ACQUIRED *rrdvar_family_name;
    const RRDVAR_ACQUIRED *rrdvar_family_context_dim_id;
    const RRDVAR_ACQUIRED *rrdvar_family_context_dim_name;

    const RRDVAR_ACQUIRED *rrdvar_host_chart_id_dim_id;
    const RRDVAR_ACQUIRED *rrdvar_host_chart_id_dim_name;
    const RRDVAR_ACQUIRED *rrdvar_host_chart_name_dim_id;
    const RRDVAR_ACQUIRED *rrdvar_host_chart_name_dim_name;

    struct rrddim *rrddim;

    struct rrddimvar *next;
    struct rrddimvar *prev;
};


extern void rrddimvar_rename_all(RRDDIM *rd);
extern RRDDIMVAR *rrddimvar_create(RRDDIM *rd, RRDVAR_TYPE type, const char *prefix, const char *suffix, void *value,
    RRDVAR_FLAGS options);
extern void rrddimvar_free(RRDDIMVAR *rs);



#endif //NETDATA_RRDDIMVAR_H
