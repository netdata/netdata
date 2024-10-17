// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDVAR_H
#define NETDATA_RRDVAR_H 1

#include "libnetdata/libnetdata.h"

#define RRDVAR_MAX_LENGTH 1024

#include "database/rrd.h"

STRING *rrdvar_name_to_string(const char *name);

const RRDVAR_ACQUIRED *rrdvar_host_variable_add_and_acquire(RRDHOST *host, const char *name);
void rrdvar_host_variable_set(RRDHOST *host, const RRDVAR_ACQUIRED *rva, NETDATA_DOUBLE value);

int rrdvar_walkthrough_read(DICTIONARY *dict, int (*callback)(const DICTIONARY_ITEM *item, void *rrdvar, void *data), void *data);

#define rrdvar_host_variable_release(host, rva) rrdvar_release((host)->rrdvars, rva)
#define rrdvar_chart_variable_release(st, rva) do { if(st) rrdvar_release((st)->rrdvars, rva); } while(0)
void rrdvar_release(DICTIONARY *dict, const RRDVAR_ACQUIRED *rva);

NETDATA_DOUBLE rrdvar2number(const RRDVAR_ACQUIRED *rva);

const RRDVAR_ACQUIRED *rrdvar_add_and_acquire(DICTIONARY *dict, STRING *name, NETDATA_DOUBLE value);

DICTIONARY *rrdvariables_create(void);
void rrdvariables_destroy(DICTIONARY *dict);

void rrdvar_delete_all(DICTIONARY *dict);

const char *rrdvar_name(const RRDVAR_ACQUIRED *rva);

void rrdvar_print_to_streaming_custom_chart_variables(RRDSET *st, BUFFER *wb);

const RRDVAR_ACQUIRED *rrdvar_chart_variable_add_and_acquire(RRDSET *st, const char *name);
void rrdvar_chart_variable_set(RRDSET *st, const RRDVAR_ACQUIRED *rva, NETDATA_DOUBLE value);

bool rrdvar_get_custom_host_variable_value(RRDHOST *host, STRING *variable, NETDATA_DOUBLE *result);
bool rrdvar_get_custom_chart_variable_value(RRDSET *st, STRING *variable, NETDATA_DOUBLE *result);

#endif //NETDATA_RRDVAR_H
