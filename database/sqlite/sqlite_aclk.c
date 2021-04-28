// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk.h"

#ifndef ACLK_NG
#include "../../aclk/legacy/agent_cloud_link.h"
#else
#include "../../aclk/aclk.h"
#endif

int aclk_architecture = 0;

void aclk_set_architecture(int mode)
{
    aclk_architecture = mode;
}

int aclk_add_chart_payload(char *uuid_str, uuid_t *unique_id, uuid_t *chart_id, char *payload_type, const char *payload, size_t payload_size)
{
    char sql[1024];
    sqlite3_stmt *res_chart = NULL;
    int rc;

    sprintf(sql,"insert into aclk_payload_%s (unique_id, chart_id, date_created, type, payload) " \
                 "values (@unique_id, @chart_id, strftime('%%s'), @type, @payload);", uuid_str);

    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res_chart, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store chart payload data");
        return 1;
    }

    rc = sqlite3_bind_blob(res_chart, 1, unique_id , sizeof(*unique_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 2, chart_id , sizeof(*chart_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res_chart, 3, payload_type ,-1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res_chart, 4, payload, payload_size, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res_chart);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed store chart payload event, rc = %d", rc);

bind_fail:
    if (unlikely(sqlite3_finalize(res_chart) != SQLITE_OK))
        error_report("Failed to reset statement in store dimension, rc = %d", rc);

    return (rc != SQLITE_DONE);
}


void aclk_add_chart_event(RRDSET *st, char *payload_type)
{
    char uuid_str[37];
    char sql[1024];
    sqlite3_stmt *res_chart = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return;
        }
        error_report("Database has not been initialized");
        return;
    }

    uuid_unparse_lower(st->rrdhost->host_uuid, uuid_str);
    uuid_str[8] = '_';
    uuid_str[13] = '_';
    uuid_str[18] = '_';
    uuid_str[23] = '_';

    uuid_t unique_uuid;
    uuid_generate(unique_uuid);
    BUFFER *tmp_buffer = NULL;
    tmp_buffer = buffer_create(4096);
    rrdset2json(st, tmp_buffer, NULL, NULL, 1);

    rc = aclk_add_chart_payload(
        uuid_str, &unique_uuid, st->chart_uuid, payload_type, buffer_tostring(tmp_buffer), strlen(buffer_tostring(tmp_buffer)));

    buffer_free(tmp_buffer);

    if (unlikely(rc))
        return;

    sprintf(sql,"insert into aclk_chart_%s (chart_id, unique_id, status, date_created) " \
                 "values (@chart_uuid, @unique_id, 'pending', strftime('%%s')) " \
                 "on conflict(chart_id, status) do update set unique_id = @unique_id, update_count = update_count + 1;" , uuid_str);

    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res_chart, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store chart event data");
        return;
    }

    rc = sqlite3_bind_blob(res_chart, 1, st->chart_uuid , sizeof(*st->chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 2, &unique_uuid , sizeof(unique_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res_chart);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed store chart event, rc = %d", rc);

bind_fail:
    rc = sqlite3_finalize(res_chart);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in store dimension, rc = %d", rc);

    return;
}


// ST is read locked
void sql_queue_chart_to_aclk(RRDSET *st, int cmd)
{
     if (!aclk_architecture)
        aclk_update_chart(st->rrdhost, st->id, ACLK_CMD_CHART);

    aclk_add_chart_event(st, "JSON");
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

    sprintf(sql, "create table if not exists aclk_chart_%s (sequence_id integer primary key, " \
        "date_created, date_updated, date_submitted, status, chart_id, unique_id, " \
        "update_count default 1, unique(chart_id, status));", uuid_str);

    db_execute(sql);
    sprintf(sql,"create table if not exists aclk_payload_%s (unique_id blob primary key, " \
                 "chart_id, type, date_created, payload);", uuid_str);
    db_execute(sql);
    sprintf(sql,"create table if not exists aclk_alert_%s (sequence_id integer primary key, " \
                 "date_created, date_updated, unique_id);", uuid_str);
    db_execute(sql);
}
