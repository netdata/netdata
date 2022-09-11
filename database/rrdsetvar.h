// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSETVAR_H
#define NETDATA_RRDSETVAR_H 1

#include "rrd.h"

// variables linked to charts
// We link variables to point to the values that are already
// calculated / processed by the normal data collection process
// This means, there will be no speed penalty for using
// these variables

extern void rrdsetvar_index_init(RRDSET *st);
extern void rrdsetvar_index_destroy(RRDSET *st);
extern void rrdsetvar_free_all(RRDSET *st);

extern RRDSETVAR *rrdsetvar_custom_chart_variable_create(RRDSET *st, const char *name);
extern void rrdsetvar_custom_chart_variable_set(RRDSET *st, RRDSETVAR *rv, NETDATA_DOUBLE value);

extern void rrdsetvar_rename_all(RRDSET *st);
extern RRDSETVAR *rrdsetvar_create(RRDSET *st, const char *name, RRDVAR_TYPE type, void *value, RRDVAR_FLAGS options);

extern void rrdsetvar_print_to_streaming_custom_chart_variables(RRDSET *st, BUFFER *wb);

#endif //NETDATA_RRDSETVAR_H
