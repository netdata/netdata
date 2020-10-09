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

const char *database_config[] = {
    "PRAGMA auto_vacuum=incremental; PRAGMA synchronous=1 ; PRAGMA journal_mode=WAL; PRAGMA temp_store=MEMORY;",
    "PRAGMA journal_size_limit=17179869184;",
    "ATTACH ':memory:' as ram;",
    "CREATE TABLE IF NOT EXISTS host (host_id blob PRIMARY KEY, hostname text, " \
        "registry_hostname text, update_every int, os text, timezone text, tags text);",
    "CREATE TABLE IF NOT EXISTS chart (chart_id blob PRIMARY KEY, host_id blob, type text, id text, name text, "  \
        "family text, context text, title text, unit text, plugin text, module text, priority int, update_every int, " \
        "chart_type int, memory_mode int, history_entries);",
    "CREATE TABLE IF NOT EXISTS dimension(dim_id blob PRIMARY KEY, chart_id blob, id text, name text, " \
        "multiplier int, divisor int , algorithm int, options text);",

    "CREATE TABLE IF NOT EXISTS ram.chart (chart_id blob PRIMARY KEY, host_id blob, type text, id text, name text, " \
        "family text, context text, title text, unit text, plugin text, module text, priority int, update_every int, " \
        "chart_type int, memory_mode int, history_entries);",
    "CREATE TABLE IF NOT EXISTS ram.dimension(dim_id blob PRIMARY KEY, chart_id blob, id text, name text, " \
        "multiplier int, divisor int , algorithm int, options text);",

    "CREATE TEMPORARY TRIGGER tr_dim after delete on ram.dimension begin insert into dimension " \
        "(dim_id, chart_id, id, name, multiplier, divisor, algorithm, options) values " \
        "(old.dim_id, old.chart_id, old.id, old.name, old.multiplier, old.divisor, old.algorithm, old.options);  end;",
    "CREATE TEMPORARY TRIGGER tr_chart after delete on ram.chart begin insert into chart "
        "(chart_id,host_id,type,id,name,family,context,title,unit,plugin,module,priority,update_every,chart_type," \
        "memory_mode,history_entries) values (old.chart_id,old.host_id,old.type,old.id,old.name,old.family," \
        "old.context,old.title,old.unit,old.plugin,old.module,old.priority,old.update_every,old.chart_type," \
        "old.memory_mode,old.history_entries); end;",
    NULL
};

#define SQL_STORE_HOST "insert or replace into host (host_id,hostname,registry_hostname,update_every,os,timezone,tags) values (?1,?2,?3,?4,?5,?6,?7);"

#define SQL_STORE_CHART "insert or replace into ram.chart (chart_uuid, host_uuid, type, id, " \
    "name, family, context, title, unit, plugin, module, priority, update_every , chart_type , memory_mode , " \
    "history_entries) values (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12,?13,?14,?15,?16);"

#define SQL_STORE_DIMENSION                                                                                           \
    "INSERT OR REPLACE into ram.dimension (dim_id, chart_id, id, name, multiplier, divisor , algorithm) values (?0001,?0002,?0003,?0004,?0005,?0006,?0007);"

extern int sql_init_database();
extern int sql_close_database();

extern int sql_store_host(const char *guid, const char *hostname, const char *registry_hostname, int update_every, const char *os, const char *timezone, const char *tags);
extern int sql_store_chart(
    uuid_t *chart_uuid, uuid_t *host_uuid, const char *type, const char *id, const char *name, const char *family,
    const char *context, const char *title, const char *units, const char *plugin, const char *module, long priority,
    int update_every, int chart_type, int memory_mode, long history_entries);
extern int sql_store_dimension(uuid_t *dim_uuid, uuid_t *chart_uuid, const char *id, const char *name, collected_number multiplier,
                               collected_number divisor, int algorithm);


GUID_TYPE sql_find_object_by_guid(uuid_t *uuid, char *object, int max_size);

#endif //NETDATA_SQLITE_FUNCTIONS_H
