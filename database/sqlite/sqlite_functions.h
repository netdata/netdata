// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_FUNCTIONS_H
#define NETDATA_SQLITE_FUNCTIONS_H

#include "../../daemon/common.h"
#include "sqlite3.h"


/*
 * Initialize a database
 */

#define SQLITE_WAL_SIZE (8 * 1024 * 1024 * 1024)
#define SQLITE_SELECT_MAX   (10)        // Max select retries
#define SQLITE_SELECT_DELAY (50)        // select delay in MS between retries

#define SQL_STORE_HOST "insert or replace into host (host_id,hostname,registry_hostname,update_every,os,timezone,tags) values (?1,?2,?3,?4,?5,?6,?7);"

#define SQL_STORE_CHART "insert or replace into chart (chart_id, host_id, type, id, " \
    "name, family, context, title, unit, plugin, module, priority, update_every , chart_type , memory_mode , " \
    "history_entries) values (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12,?13,?14,?15,?16);"

#define SQL_STORE_DIMENSION                                                                                           \
    "INSERT OR REPLACE into dimension (dim_id, chart_id, id, name, multiplier, divisor , algorithm) values (?0001,?0002,?0003,?0004,?0005,?0006,?0007);"

extern int sql_init_database();
extern int sql_close_database();

extern int sql_store_host(const char *guid, const char *hostname, const char *registry_hostname, int update_every, const char *os, const char *timezone, const char *tags);
extern int sql_store_chart(
    uuid_t *chart_uuid, uuid_t *host_uuid, const char *type, const char *id, const char *name, const char *family,
    const char *context, const char *title, const char *units, const char *plugin, const char *module, long priority,
    int update_every, int chart_type, int memory_mode, long history_entries);
extern int sql_store_dimension(uuid_t *dim_uuid, uuid_t *chart_uuid, const char *id, const char *name, collected_number multiplier,
                               collected_number divisor, int algorithm);

// Lookup UUIDs in the database
extern uuid_t *sql_find_dim_uuid(RRDSET *st, RRDDIM *rd); //char *id, char *name, collected_number multiplier, collected_number divisor, int algorithm);
extern uuid_t *sql_find_chart_uuid(RRDHOST *host, RRDSET *st, const char *type, const char *id, const char *name);
extern int sql_cache_chart_dimensions(RRDSET *st);
extern int sql_cache_host_charts(RRDHOST *host);

//GUID_TYPE sql_find_object_by_guid(uuid_t *uuid, char *object, int max_size);

#endif //NETDATA_SQLITE_FUNCTIONS_H
