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

    // this is 8 bit
    // to increase it you have to set change the bitfield in
    // rrdvar, rrdsetvar, rrddimvar
} RRDVAR_TYPE;

typedef enum rrdvar_options {
    RRDVAR_FLAG_NONE                        = 0,
    RRDVAR_FLAG_ALLOCATED                   = (1 << 0), // the value ptr is allocated (not a reference)
    RRDVAR_FLAG_CUSTOM_HOST_VAR             = (1 << 1), // this is a custom host variable, not associated with a dimension
    RRDVAR_FLAG_CUSTOM_CHART_VAR            = (1 << 2), // this is a custom chart variable, not associated with a dimension
    RRDVAR_FLAG_RRDCALC_LOCAL_VAR           = (1 << 3), // this is a an alarm variable, attached to a chart
    RRDVAR_FLAG_RRDCALC_FAMILY_VAR          = (1 << 4), // this is a an alarm variable, attached to a family
    RRDVAR_FLAG_RRDCALC_HOST_CHARTID_VAR    = (1 << 5), // this is a an alarm variable, attached to the host, using the chart id
    RRDVAR_FLAG_RRDCALC_HOST_CHARTNAME_VAR  = (1 << 6), // this is a an alarm variable, attached to the host, using the chart name
    RRDVAR_FLAG_CONFIG_VAR                  = (1 << 7), // this is a an alarm variable, read from alarm config

    // this is 24 bit
    // to increase it you have to set change the bitfield in
    // rrdvar, rrdsetvar, rrddimvar
} RRDVAR_FLAGS;

#define RRDVAR_OPTIONS_REMOVED_ON_NEW_OBJECTS \
    (RRDVAR_FLAG_ALLOCATED)

#define RRDVAR_OPTIONS_REMOVED_WHEN_PROPAGATING_TO_RRDVAR \
    (RRDVAR_FLAG_ALLOCATED)

#define RRDVAR_MAX_LENGTH 1024

int rrdvar_fix_name(char *variable);

#include "rrd.h"

STRING *rrdvar_name_to_string(const char *name);

const RRDVAR_ACQUIRED *rrdvar_custom_host_variable_add_and_acquire(RRDHOST *host, const char *name);
void rrdvar_add(const char *scope __maybe_unused, DICTIONARY *dict, STRING *name, RRDVAR_TYPE type, RRDVAR_FLAGS options, void *value);
void rrdvar_custom_host_variable_set(RRDHOST *host, const RRDVAR_ACQUIRED *rva, NETDATA_DOUBLE value);

int rrdvar_walkthrough_read(DICTIONARY *dict, int (*callback)(const DICTIONARY_ITEM *item, void *rrdvar, void *data), void *data);

#define rrdvar_custom_host_variable_release(host, rva) rrdvar_release((host)->rrdvars, rva)
void rrdvar_release(DICTIONARY *dict, const RRDVAR_ACQUIRED *rva);

NETDATA_DOUBLE rrdvar2number(const RRDVAR_ACQUIRED *rva);

const RRDVAR_ACQUIRED *rrdvar_add_and_acquire(const char *scope, DICTIONARY *dict, STRING *name, RRDVAR_TYPE type, RRDVAR_FLAGS options, void *value);
void rrdvar_release_and_del(DICTIONARY *dict, const RRDVAR_ACQUIRED *rva);

DICTIONARY *rrdvariables_create(void);
DICTIONARY *health_rrdvariables_create(void);
void rrdvariables_destroy(DICTIONARY *dict);

void rrdvar_store_for_chart(RRDHOST *host, RRDSET *st);
int health_variable_check(DICTIONARY *dict, RRDSET *st, RRDDIM *rd);

void rrdvar_delete_all(DICTIONARY *dict);

const char *rrdvar_name(const RRDVAR_ACQUIRED *rva);
RRDVAR_FLAGS rrdvar_flags(const RRDVAR_ACQUIRED *rva);
RRDVAR_TYPE rrdvar_type(const RRDVAR_ACQUIRED *rva);

#endif //NETDATA_RRDVAR_H
