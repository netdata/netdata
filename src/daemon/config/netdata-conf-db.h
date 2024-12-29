// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_NETDATA_CONF_DBENGINE_H
#define NETDATA_DAEMON_NETDATA_CONF_DBENGINE_H

#include "libnetdata/libnetdata.h"

extern bool dbengine_enabled;
extern bool dbengine_use_direct_io;

extern int default_rrd_history_entries;
extern int gap_when_lost_iterations_above;
extern time_t rrdset_free_obsolete_time_s;

size_t get_tier_grouping(size_t tier);

void netdata_conf_section_db(void);
void netdata_conf_dbengine_init(const char *hostname);

#include "netdata-conf.h"

#endif //NETDATA_DAEMON_NETDATA_CONF_DBENGINE_H
