// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDVAR_H
#define NETDATA_RRDVAR_H 1

#include "libnetdata/libnetdata.h"

typedef enum rrdvar_type {
    RRDVAR_TYPE_CALCULATED              = 1,
    RRDVAR_TYPE_TIME_T                  = 2,
    RRDVAR_TYPE_COLLECTED               = 3,
    RRDVAR_TYPE_TOTAL                   = 4,
    RRDVAR_TYPE_INT                     = 5
} RRDVAR_TYPE;

typedef enum rrdvar_options {
    RRDVAR_FLAG_NONE = 0,
    RRDVAR_FLAG_ALLOCATED = (1 << 0), // the value ptr is allocated (not a reference)
    RRDVAR_FLAG_CUSTOM_HOST_VAR = (1 << 1), // this is a custom host variable, not associated with a dimension
    RRDVAR_FLAG_CUSTOM_CHART_VAR = (1 << 2), // this is a custom chart variable, not associated with a dimension
    RRDVAR_FLAG_RRDCALC_LOCAL_VAR = (1 << 3), // this is a an alarm variable, attached to a chart
    RRDVAR_FLAG_RRDCALC_FAMILY_VAR = (1 << 4), // this is a an alarm variable, attached to a family
    RRDVAR_FLAG_RRDCALC_HOST_CHARTID_VAR = (1 << 5), // this is a an alarm variable, attached to the host, using the chart id
    RRDVAR_FLAG_RRDCALC_HOST_CHARTNAME_VAR = (1 << 6), // this is a an alarm variable, attached to the host, using the chart name
} RRDVAR_FLAGS;

#define RRDVAR_OPTIONS_REMOVED_ON_NEW_OBJECTS \
    (RRDVAR_FLAG_ALLOCATED)

#define RRDVAR_OPTIONS_REMOVED_WHEN_PROPAGATING_TO_RRDVAR \
    (RRDVAR_FLAG_ALLOCATED)

#define RRDVAR_MAX_LENGTH 1024

extern int rrdvar_fix_name(char *variable);

#include "rrd.h"

extern STRING *rrdvar_name_to_string(const char *name);

extern RRDVAR *rrdvar_custom_host_variable_create(RRDHOST *host, const char *name);
extern void rrdvar_custom_host_variable_set(RRDHOST *host, RRDVAR *rv, NETDATA_DOUBLE value);

extern int rrdvar_walkthrough_read(DICTIONARY *dict, int (*callback)(const DICTIONARY_ITEM *item, void *rrdvar, void *data), void *data);

extern NETDATA_DOUBLE rrdvar2number(RRDVAR *rv);

extern RRDVAR *rrdvar_add(const char *scope, DICTIONARY *dict, STRING *name, RRDVAR_TYPE type, RRDVAR_FLAGS options, void *value);
extern void rrdvar_del(DICTIONARY *dict, RRDVAR *rv);

extern DICTIONARY *rrdvariables_create(void);
extern void rrdvariables_destroy(DICTIONARY *dict);

extern void rrdvar_free_all(DICTIONARY *dict);

extern const char *rrdvar_name(RRDVAR *rv);
extern RRDVAR_FLAGS rrdvar_flags(RRDVAR *rv);
extern RRDVAR_TYPE rrdvar_type(RRDVAR *rv);

#endif //NETDATA_RRDVAR_H
