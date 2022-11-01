// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSETVAR_H
#define NETDATA_RRDSETVAR_H 1

#include "rrd.h"

// variables linked to charts
// We link variables to point to the values that are already
// calculated / processed by the normal data collection process
// This means, there will be no speed penalty for using
// these variables

void rrdsetvar_index_init(RRDSET *st);
void rrdsetvar_index_destroy(RRDSET *st);
void rrdsetvar_release_and_delete_all(RRDSET *st);

#define rrdsetvar_custom_chart_variable_release(st, rsa) rrdsetvar_release((st)->rrdsetvar_root_index, rsa)
void rrdsetvar_release(DICTIONARY *dict, const RRDSETVAR_ACQUIRED *rsa);

const RRDSETVAR_ACQUIRED *rrdsetvar_custom_chart_variable_add_and_acquire(RRDSET *st, const char *name);
void rrdsetvar_custom_chart_variable_set(RRDSET *st, const RRDSETVAR_ACQUIRED *rsa, NETDATA_DOUBLE value);

void rrdsetvar_rename_all(RRDSET *st);
const RRDSETVAR_ACQUIRED *rrdsetvar_add_and_acquire(RRDSET *st, const char *name, RRDVAR_TYPE type, void *value, RRDVAR_FLAGS flags);
void rrdsetvar_add_and_leave_released(RRDSET *st, const char *name, RRDVAR_TYPE type, void *value, RRDVAR_FLAGS flags);

void rrdsetvar_print_to_streaming_custom_chart_variables(RRDSET *st, BUFFER *wb);

#endif //NETDATA_RRDSETVAR_H
