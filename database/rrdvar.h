// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDVAR_H
#define NETDATA_RRDVAR_H 1

#include "libnetdata/libnetdata.h"

extern int rrdvar_compare(void *a, void *b);

typedef enum rrdvar_type {
    RRDVAR_TYPE_CALCULATED              = 1,
    RRDVAR_TYPE_TIME_T                  = 2,
    RRDVAR_TYPE_COLLECTED               = 3,
    RRDVAR_TYPE_TOTAL                   = 4,
    RRDVAR_TYPE_INT                     = 5
} RRDVAR_TYPE;

typedef enum rrdvar_options {
    RRDVAR_OPTION_DEFAULT                    = 0,
    RRDVAR_OPTION_ALLOCATED                  = (1 << 0), // the value ptr is allocated (not a reference)
    RRDVAR_OPTION_CUSTOM_HOST_VAR            = (1 << 1), // this is a custom host variable, not associated with a dimension
    RRDVAR_OPTION_CUSTOM_CHART_VAR           = (1 << 2), // this is a custom chart variable, not associated with a dimension
    RRDVAR_OPTION_RRDCALC_LOCAL_VAR          = (1 << 3), // this is a an alarm variable, attached to a chart
    RRDVAR_OPTION_RRDCALC_FAMILY_VAR         = (1 << 4), // this is a an alarm variable, attached to a family
    RRDVAR_OPTION_RRDCALC_HOST_CHARTID_VAR   = (1 << 5), // this is a an alarm variable, attached to the host, using the chart id
    RRDVAR_OPTION_RRDCALC_HOST_CHARTNAME_VAR = (1 << 6), // this is a an alarm variable, attached to the host, using the chart name
} RRDVAR_OPTIONS;

// the variables as stored in the variables indexes
// there are 3 indexes:
// 1. at each chart   (RRDSET.rrdvar_root_index)
// 2. at each context (RRDFAMILY.rrdvar_root_index)
// 3. at each host    (RRDHOST.rrdvar_root_index)
struct rrdvar {
    avl_t avl;

    char *name;
    uint32_t hash;

    RRDVAR_TYPE type;
    RRDVAR_OPTIONS options;

    void *value;

    time_t last_updated;
};

#define RRDVAR_MAX_LENGTH 1024

extern int rrdvar_fix_name(char *variable);

#include "rrd.h"

extern RRDVAR *rrdvar_custom_host_variable_create(RRDHOST *host, const char *name);
extern void rrdvar_custom_host_variable_set(RRDHOST *host, RRDVAR *rv, calculated_number value);
extern int foreach_host_variable_callback(RRDHOST *host, int (*callback)(RRDVAR *rv, void *data), void *data);
extern void rrdvar_free_remaining_variables(RRDHOST *host, avl_tree_lock *tree_lock);

extern int  rrdvar_callback_for_all_host_variables(RRDHOST *host, int (*callback)(void *rrdvar, void *data), void *data);

extern calculated_number rrdvar2number(RRDVAR *rv);

extern RRDVAR *rrdvar_create_and_index(const char *scope, avl_tree_lock *tree, const char *name, RRDVAR_TYPE type, RRDVAR_OPTIONS options, void *value);
extern void rrdvar_free(RRDHOST *host, avl_tree_lock *tree, RRDVAR *rv);

#endif //NETDATA_RRDVAR_H
