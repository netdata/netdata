// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDIMVAR_H
#define NETDATA_RRDDIMVAR_H 1

#include "rrd.h"

// variables linked to individual dimensions
// We link variables to point the values that are already
// calculated / processed by the normal data collection process
// This means, there will be no speed penalty for using
// these variables

void rrddimvar_rename_all(RRDDIM *rd);
void rrddimvar_add_and_leave_released(RRDDIM *rd, RRDVAR_TYPE type, const char *prefix, const char *suffix, void *value, RRDVAR_FLAGS flags);
void rrddimvar_delete_all(RRDDIM *rd);

void rrddimvar_index_init(RRDSET *st);
void rrddimvar_index_destroy(RRDSET *st);

#endif //NETDATA_RRDDIMVAR_H
