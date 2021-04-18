// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk.h"

#ifndef ACLK_NG
#include "../../aclk/legacy/agent_cloud_link.h"
#else
#include "../../aclk/aclk.h"
#endif

void sql_queue_chart_to_aclk(RRDSET *st, int cmd)
{
    info("DEBUG: Queue %s to ACLK", st->name);
    aclk_update_chart(st->rrdhost, st->id, ACLK_CMD_CHART);
    return;
}

// Hosts that have tables
// select h2u(host_id), hostname from host h, sqlite_schema s where name = "aclk_" || replace(h2u(h.host_id),"-","_") and s.type = "table";

// One query thread per host (host_id, table name)
// Prepare read / write sql statements

// Load nodes on startup (ask those that do not have node id)
// Start thread event loop (R/W)

void sql_create_aclk_table(RRDHOST *host)
{
    char uuid_str[37];
    char sql[256];

    uuid_unparse_lower(host->host_uuid, uuid_str);
    uuid_str[8] = '_';
    uuid_str[13] = '_';
    uuid_str[18] = '_';
    uuid_str[23] = '_';

    info("Debug: creating table ACLK_%s", uuid_str);
    sprintf(sql,"create table if not exists aclk_chart_%s (sequence_id integer primary key, payload text);", uuid_str);
    db_execute(sql);
    sprintf(sql,"create table if not exists aclk_alert_%s (sequence_id integer primary key, payload text);", uuid_str);
    db_execute(sql);
}
