// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_DICTIONARY_H
#define NETDATA_PULSE_DICTIONARY_H

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
extern struct dictionary_stats dictionary_stats_category_dyncfg;

#if defined(PULSE_INTERNALS)
void pulse_dictionary_do(bool extended);
#endif

#endif //NETDATA_PULSE_DICTIONARY_H
