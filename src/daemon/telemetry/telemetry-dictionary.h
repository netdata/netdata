// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_DICTIONARY_H
#define NETDATA_TELEMETRY_DICTIONARY_H

#include "daemon/common.h"

extern struct dictionary_stats dictionary_stats_category_collectors;
extern struct dictionary_stats dictionary_stats_category_rrdhost;
extern struct dictionary_stats dictionary_stats_category_rrdset;
extern struct dictionary_stats dictionary_stats_category_rrddim;
extern struct dictionary_stats dictionary_stats_category_rrdcontext;
extern struct dictionary_stats dictionary_stats_category_rrdlabels;
extern struct dictionary_stats dictionary_stats_category_rrdhealth;
extern struct dictionary_stats dictionary_stats_category_functions;
extern struct dictionary_stats dictionary_stats_category_replication;

extern size_t rrddim_db_memory_size;

#if defined(TELEMETRY_INTERNALS)
void telemetry_dictionary_do(bool extended);
#endif

#endif //NETDATA_TELEMETRY_DICTIONARY_H
