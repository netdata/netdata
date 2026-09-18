// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_NETDATA_CONF_DBENGINE_H
#define NETDATA_DAEMON_NETDATA_CONF_DBENGINE_H

#include "libnetdata/libnetdata.h"

extern bool dbengine_enabled;
extern bool dbengine_datafiles_present; // dbengine datafiles exist on disk, even if the agent is not currently running dbengine

#ifdef ENABLE_DBENGINE
#include "database/storage-engines/dbengine/include/dbengine/dbengine-config.h"
// the engine's process-wide configuration as the daemon resolved it from netdata.conf; the
// daemon reads its own copy, the engine gets it through netdata_conf_dbengine_apply()
extern struct dbengine_config netdata_conf_dbengine;

// the configuration with the settings the daemon resolves elsewhere snapshotted into it, ready for the engine
// or for a test that needs only the configuration
const struct dbengine_config *netdata_conf_dbengine_resolved(void);

// per-tier configuration for dbengine_tier_init(): tier, page type and grouping; the caller adds path, quota
// and retention
void netdata_conf_dbengine_tier_config(size_t tier, struct dbengine_tier_config *out);
uint8_t netdata_conf_dbengine_page_type(size_t tier);

// dbengine tier sizing knobs, consumed by the daemon (tier setup, /api/v1/info, analytics, tests) - not by the engine
extern int default_dbengine_disk_quota_mb;
extern int default_multidb_disk_quota_mb;
extern bool new_dbengine_defaults;
extern bool legacy_multihost_db_space;
#endif

extern int default_rrd_history_entries;
extern int gap_when_lost_iterations_above;
extern time_t rrdset_free_obsolete_time_s;

size_t get_tier_grouping(size_t tier);

void netdata_conf_section_db(void);
void netdata_conf_dbengine_init(const char *hostname);
void netdata_conf_dbengine_apply(void);

#include "netdata-conf.h"

#endif //NETDATA_DAEMON_NETDATA_CONF_DBENGINE_H
