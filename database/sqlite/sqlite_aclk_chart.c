// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_chart.h"

#ifndef ACLK_NG
#include "../../aclk/legacy/agent_cloud_link.h"
#else
#include "../../aclk/aclk.h"
#include "../../aclk/aclk_charts_api.h"
#include "../../aclk/aclk_alarm_api.h"
#endif


int chart_payload_sent(char *uuid_str, uuid_t *uuid, void *payload, size_t payload_size)
{
    sqlite3_stmt *res = NULL;
    int rc;
    int payload_sent = 0;

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql,"SELECT 1 FROM aclk_chart_latest_%s acl, aclk_chart_payload_%s acp "
                       "WHERE acl.unique_id = acp.unique_id AND acl.uuid = @uuid AND acp.payload = @payload;",
                        uuid_str, uuid_str);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to check payload data");
        buffer_free(sql);
        return 0;
    }

    rc = sqlite3_bind_blob(res, 1, uuid , sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res, 2, payload , payload_size, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    while (sqlite3_step(res) == SQLITE_ROW) {
        payload_sent = sqlite3_column_int(res, 0);
    }

    bind_fail:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to reset statement in check payload, rc = %d", rc);
    buffer_free(sql);
    return payload_sent;
}

int aclk_add_chart_payload(char *uuid_str, uuid_t *uuid, char *claim_id, int payload_type, void *payload, size_t payload_size)
{
    sqlite3_stmt *res_chart = NULL;
    int rc;

    rc = chart_payload_sent(uuid_str, uuid, payload, payload_size);
    char uuid_str1[GUID_LEN + 1];
    uuid_unparse_lower(*uuid, uuid_str1);
    //info("DEBUG: Checking %s if payload already sent (%s), RC = %d (payload type = %d)", uuid_str, uuid_str1, rc, payload_type);
    if (rc == 1)
        return 0;

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql,"INSERT INTO aclk_chart_payload_%s (unique_id, uuid, claim_id, date_created, type, payload) " \
                 "VALUES (@unique_id, @uuid, @claim_id, strftime('%%s'), @type, @payload);", uuid_str);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res_chart, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store chart payload data");
        buffer_free(sql);
        return 1;
    }

    uuid_t unique_uuid;
    uuid_generate(unique_uuid);

    uuid_t claim_uuid;
    uuid_parse(claim_id, claim_uuid);

    rc = sqlite3_bind_blob(res_chart, 1, &unique_uuid , sizeof(unique_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 2, uuid , sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 3, &claim_uuid , sizeof(claim_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res_chart, 4, payload_type);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 5, payload, payload_size, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res_chart);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed store chart payload event, rc = %d", rc);

    bind_fail:
    if (unlikely(sqlite3_finalize(res_chart) != SQLITE_OK))
        error_report("Failed to reset statement in store chart payload, rc = %d", rc);
    buffer_free(sql);
    return (rc != SQLITE_DONE);
}

int aclk_add_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 0;
    if (unlikely(!db_meta)) {
//        freez(cmd.data_param);
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return 1;
        }
        error_report("Database has not been initialized");
        return 1;
    }

#ifdef ACLK_NG
    char *claim_id = is_agent_claimed();

    RRDSET *st = cmd.data;

    if (likely(claim_id)) {
        struct chart_instance_updated chart_payload;
        memset(&chart_payload, 0, sizeof(chart_payload));
        chart_payload.config_hash = get_str_from_uuid(&st->state->hash_id);
        chart_payload.update_every = st->update_every;
        chart_payload.memory_mode = st->rrd_memory_mode;
        chart_payload.name = strdupz((char *)st->name);
        chart_payload.node_id = strdupz(wc->node_id); //get_str_from_uuid(st->rrdhost->node_id);
        chart_payload.claim_id = claim_id;
        chart_payload.id = strdupz(st->id);

        size_t size;
        char *payload = generate_chart_instance_updated(&size, &chart_payload);
        if (likely(payload))
            rc = aclk_add_chart_payload(wc->uuid_str, st->chart_uuid, claim_id, 0, (void *) payload, size);
        freez(payload);
        chart_instance_updated_destroy(&chart_payload);
    }
#else
    UNUSED(wc);
    UNUSED(cmd);
#endif
//    freez(cmd.data_param);
    return rc;
}

int aclk_add_dimension_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 0;
    if (unlikely(!db_meta)) {
//        freez(cmd.data_param);
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return 1;
        }
        error_report("Database has not been initialized");
        return 1;
    }

#ifdef ACLK_NG
    char *claim_id = is_agent_claimed();

    RRDDIM *rd = cmd.data;

    time_t now = now_realtime_sec();

    if (likely(claim_id)) {
        const char *node_id = NULL;
        node_id = wc->node_id; // get_str_from_uuid(rd->rrdset->rrdhost->node_id);
        struct chart_dimension_updated dim_payload;
        memset(&dim_payload, 0, sizeof(dim_payload));
        dim_payload.node_id = strdupz(node_id);
        dim_payload.claim_id = claim_id;

        dim_payload.chart_id = strdupz(rd->rrdset->name);
        dim_payload.created_at = rd->last_collected_time; //TODO: Fix with creation time
        if ((now - rd->last_collected_time.tv_sec) < (RRDSET_MINIMUM_LIVE_COUNT * rd->update_every)) {
            dim_payload.last_timestamp.tv_usec = 0;
            dim_payload.last_timestamp.tv_sec = 0;
        }
        else
            dim_payload.last_timestamp = rd->last_collected_time;

        dim_payload.name = strdupz(rd->name);
        dim_payload.id = strdupz(rd->id);

        size_t size;
        char *payload = generate_chart_dimension_updated(&size, &dim_payload);
        if (likely(payload))
            rc = aclk_add_chart_payload(wc->uuid_str, rd->state->metric_uuid, claim_id, 1, (void *) payload, size);
        freez(payload);
        //chart_instance_updated_destroy(&dim_payload);
    }
#else
    UNUSED(wc);
    UNUSED(cmd);
#endif
//    freez(cmd.data_param);
    return rc;
}

// Add an event for a dimension that is not in memory
int aclk_add_offline_dimension_event(struct aclk_database_worker_config *wc,
                                     char *node_id, char *chart_name, uuid_t *dim_uuid, char *rd_id, char *rd_name,
                                     time_t first_entry_t, time_t last_entry_t)
{
    int rc = 0;

#ifdef ACLK_NG
    char *claim_id = is_agent_claimed();

//    time_t now = now_realtime_sec();

    if (likely(claim_id)) {
        struct chart_dimension_updated dim_payload;
        memset(&dim_payload, 0, sizeof(dim_payload));
        dim_payload.node_id = strdupz(node_id);
        dim_payload.claim_id = claim_id;

        dim_payload.chart_id = strdupz(chart_name);
        dim_payload.created_at.tv_usec = 0;
        dim_payload.created_at.tv_sec  = first_entry_t;
        dim_payload.last_timestamp.tv_usec = 0;
        dim_payload.last_timestamp.tv_sec = last_entry_t;

        dim_payload.name = strdupz(rd_name);
        dim_payload.id = strdupz(rd_id);

        size_t size;
        char *payload = generate_chart_dimension_updated(&size, &dim_payload);
        if (likely(payload))
            rc = aclk_add_chart_payload(wc->uuid_str, dim_uuid, claim_id, 3, (void *) payload, size);
        freez(payload);
        //chart_instance_updated_destroy(&dim_payload);
    }
#else
    UNUSED(wc);
    UNUSED(cmd);
#endif
    return rc;
}

void sql_reset_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(sql, "UPDATE aclk_chart_%s SET status = NULL, date_submitted = NULL WHERE sequence_id >= %"PRIu64";",
                   wc->uuid_str, cmd.param1);
    db_execute(buffer_tostring(sql));
    buffer_free(sql);
    sql_chart_deduplicate(wc, cmd);
    sql_get_last_chart_sequence(wc, cmd);
    // Start sending updates
    wc->chart_updates = 1;
    return;
}



void aclk_status_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc;

    BUFFER *sql = buffer_create(1024);
    BUFFER *resp = buffer_create(1024);

    buffer_sprintf(sql, "SELECT CASE WHEN status IS NULL AND date_submitted IS NULL THEN 'resync' " \
        "WHEN status IS NULL THEN 'submitted' ELSE status END, " \
        "COUNT(*), MIN(sequence_id), MAX(sequence_id) FROM " \
        "aclk_chart_%s GROUP BY 1;", wc->uuid_str);

    sqlite3_stmt *res = NULL;
    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to lookup chart UUID in the database");
        goto fail;
    }

    struct aclk_chart_payload_t *head = NULL;
    while (sqlite3_step(res) == SQLITE_ROW) {
        struct aclk_chart_payload_t *chart_payload = callocz(1, sizeof(*chart_payload));
        buffer_flush(resp);
        buffer_sprintf(resp, "Status: %s\n Count: %lld\n Min sequence_id: %lld\n Max sequence_id: %lld\n",
                       (char *) sqlite3_column_text(res, 0),
                       sqlite3_column_int64(res, 1), sqlite3_column_int64(res, 2), sqlite3_column_int64(res, 3));

        chart_payload->payload = strdupz(buffer_tostring(resp));

        chart_payload->next =  head;
        head = chart_payload;
    }

//    rc = sqlite3_finalize(res);
//    if (unlikely(rc != SQLITE_OK))
//        error_report("Failed to reset statement when searching for a chart UUID, rc = %d", rc);
//    buffer_reset(sql);
//    buffer_sprintf(sql, "SELECT sequence_id, date_submitted, date_updated FROM aclk_chart_%s "
//                        "WHERE date_submitted IS NOT NULL ORDER BY sequence_id ASC;" , wc->uuid_str);
//
//    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
//    if (rc != SQLITE_OK) {
//        error_report("Failed to prepare statement to lookup chart UUID in the database");
//        goto fail;
//    }
//
//    while (sqlite3_step(res) == SQLITE_ROW) {
//        struct aclk_chart_payload_t *chart_payload = callocz(1, sizeof(*chart_payload));
//        buffer_flush(resp);
//        uint64_t ack = sqlite3_column_bytes(res,2) > 0 ? sqlite3_column_int64(res, 2) : 0;
//        buffer_sprintf(resp, "Sequence_id: %lld, Submitted %lld, Acknowledged %lld  (delay %lld)",
//                       sqlite3_column_int64(res, 0), sqlite3_column_int64(res, 1),
//                       ack, ack ? ack-sqlite3_column_int64(res, 1): 0);
//
//        chart_payload->payload = strdupz(buffer_tostring(resp));
//
//        chart_payload->next =  head;
//        head = chart_payload;
//    }

    *(struct aclk_chart_payload_t **) cmd.data = head;

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when searching for a chart UUID, rc = %d", rc);


    fail:
    buffer_free(sql);
    buffer_free(resp);
    return;
}


int sql_queue_chart_payload(struct aclk_database_worker_config *wc, void *data, enum aclk_database_opcode opcode)
{
    if (unlikely(!wc))
        return 1;

    struct aclk_database_cmd cmd;
    cmd.opcode = opcode;
    cmd.data = data;
    cmd.completion = NULL;
    aclk_database_enq_cmd(wc, &cmd);
    return 0;
}

// ST is read locked
int sql_queue_chart_to_aclk(RRDSET *st)
{
    return sql_queue_chart_payload((struct aclk_database_worker_config *) st->rrdhost->dbsync_worker, st, ACLK_DATABASE_ADD_CHART);
}

int sql_queue_dimension_to_aclk(RRDDIM *rd)
{
    return sql_queue_chart_payload((struct aclk_database_worker_config *) rd->rrdset->rrdhost->dbsync_worker, rd, ACLK_DATABASE_ADD_DIMENSION);
}



void aclk_push_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
#ifndef ACLK_NG
    UNUSED(wc);
    UNUSED(cmd);
#else
    int rc;

    if (unlikely(!wc->chart_updates)) {
        info("DEBUG: Ignoring chart push event, updates have been turned off %s", wc->uuid_str);
        return;
    }

    int limit = cmd.count > 0 ? cmd.count : 1;
    uint64_t first_sequence;
    uint64_t last_sequence;
    time_t last_timestamp;

    BUFFER *sql = buffer_create(1024);

    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "SELECT ac.sequence_id, acp.payload, ac.date_created, ac.type, ac.uuid  " \
        "FROM aclk_chart_%s ac, aclk_chart_payload_%s acp " \
        "WHERE ac.date_submitted IS NULL AND ac.unique_id = acp.unique_id AND ac.update_count > 0 " \
        "ORDER BY ac.sequence_id ASC LIMIT %d;", wc->uuid_str, wc->uuid_str, limit);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to send a chart update via ACLK");
        buffer_free(sql);
        return;
    }

    char **payload_list = callocz(limit+1, sizeof(char *));
    size_t *payload_list_size = callocz(limit+1, sizeof(size_t));
    size_t *payload_list_max_size = callocz(limit+1, sizeof(size_t));
    struct aclk_message_position *position_list =  callocz(limit+1, sizeof(*position_list));
    int *is_dim = callocz(limit+1, sizeof(*is_dim));

    int loop = cmd.param1;

    while (loop > 0) {
        uint64_t previous_sequence_id = wc->chart_sequence_id;
        int count = 0;
        first_sequence = 0;
        last_sequence = 0;
        while (count < limit && sqlite3_step(res) == SQLITE_ROW) {
            size_t payload_size = sqlite3_column_bytes(res, 1);
            if (payload_list_max_size[count] < payload_size) {
                freez(payload_list[count]);
                payload_list_max_size[count] = payload_size;
                payload_list[count] = mallocz(payload_size);
            }
            payload_list_size[count] = payload_size;
            memcpy(payload_list[count], sqlite3_column_blob(res, 1), payload_size);
            position_list[count].sequence_id = (uint64_t)sqlite3_column_int64(res, 0);
            position_list[count].previous_sequence_id = previous_sequence_id;
            position_list[count].seq_id_creation_time.tv_sec = sqlite3_column_int64(res, 2);
            position_list[count].seq_id_creation_time.tv_usec = 0;
            if (!first_sequence)
                first_sequence = position_list[count].sequence_id;
            last_sequence = position_list[count].sequence_id;
            last_timestamp = position_list[count].seq_id_creation_time.tv_sec;
            previous_sequence_id = last_sequence;
            is_dim[count] = sqlite3_column_int(res, 3) > 0; // 0 chart else 1 dimension
            count++;
        }
        freez(payload_list[count]);
        payload_list_max_size[count] = 0;
        payload_list[count] = NULL;

        rc = sqlite3_reset(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement when pushing chart events, rc = %d", rc);

        if (likely(first_sequence)) {
            buffer_reset(sql);
            buffer_sprintf(sql, "UPDATE aclk_chart_%s SET status = NULL, date_submitted=strftime('%%s') "
                            "WHERE date_submitted IS NULL AND sequence_id BETWEEN %" PRIu64 " AND %" PRIu64 ";",
                       wc->uuid_str, first_sequence, last_sequence);
            db_execute(buffer_tostring(sql));

            buffer_reset(sql);
//            buffer_sprintf(sql, "INSERT INTO aclk_chart_%s (uuid, status, type, unique_id, update_count, date_created) "
//                " SELECT uuid, 'pending', type, unique_id, 0, strftime('%%s') FROM aclk_chart_%s s "
//                " WHERE date_submitted IS NOT NULL AND sequence_id BETWEEN %" PRIu64 " AND %" PRIu64
//                " AND s.uuid NOT IN (SELECT t.uuid FROM aclk_chart_%s t WHERE t.uuid = s.uuid AND t.status = 'pending');",
//                wc->uuid_str, wc->uuid_str, first_sequence, last_sequence, wc->uuid_str);

            buffer_sprintf(sql, "INSERT OR REPLACE INTO aclk_chart_latest_%s (uuid, unique_id, date_submitted) "
                                " SELECT uuid, unique_id, date_submitted FROM aclk_chart_%s s "
                                " WHERE date_submitted IS NOT NULL AND sequence_id BETWEEN %" PRIu64 " AND %" PRIu64
                                " ;",
                           wc->uuid_str, wc->uuid_str, first_sequence, last_sequence);

            db_execute(buffer_tostring(sql));

            info("DEBUG: %s Loop %d chart seq %" PRIu64 " - %" PRIu64", t=%ld batch=%"PRIu64, wc->uuid_str,
                 loop, first_sequence, last_sequence, last_timestamp, wc->batch_id);

            aclk_chart_inst_and_dim_update(payload_list, payload_list_size, is_dim, position_list, wc->batch_id);
            wc->chart_sequence_id = last_sequence;
            wc->chart_timestamp = last_timestamp;
        }
        --loop;
    }
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when pushing chart events, rc = %d", rc);

    for (int i = 0; i <= limit; ++i)
        freez(payload_list[i]);

    freez(payload_list);
    freez(payload_list_size);
    freez(position_list);
    freez(is_dim);
    buffer_free(sql);
#endif

    return;
}

void sql_check_dimension_state(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);
#ifndef ACLK_NG
    UNUSED(wc);
#else
    int rc;

    if (unlikely(!wc->chart_updates)) {
        info("DEBUG: Ignoring sql_check_dimension_state for %s", wc->uuid_str);
        return;
    }

    BUFFER *sql = buffer_create(1024);

    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "SELECT d.dim_id, d.id, d.name, c.type||'.'||c.id FROM dimension d, chart c "
                        "WHERE d.chart_id = c.chart_id AND c.host_id = @host AND d.dim_id NOT IN "
                        "(SELECT uuid FROM aclk_chart_%s WHERE uuid = d.dim_id AND date_created > @start_time);", wc->uuid_str);
    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement count sequence ids in the database");
        goto fail_complete;
    }
    if (wc->host)
        rc = sqlite3_bind_blob(res, 1, &wc->host->host_uuid , sizeof(wc->host->host_uuid), SQLITE_STATIC);
    else {
        uuid_t host_uuid;
        rc = uuid_parse(wc->host_guid, host_uuid);
        rc = sqlite3_bind_blob(res, 1, &host_uuid, sizeof(host_uuid), SQLITE_STATIC);
    }
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int64(res, 2, (uint64_t) wc->startup_time);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    uuid_t  *dim_uuid;
    char dim_uuid_str[GUID_LEN + 1];
    time_t first_entry_t, last_entry_t;
    info("DEBUG: Checking dimensions %s", wc->uuid_str);
    int count = 0;
    while (sqlite3_step(res) == SQLITE_ROW) {
        dim_uuid = (uuid_t *) sqlite3_column_blob(res, 0);
        uuid_unparse_lower(*dim_uuid, dim_uuid_str);
#ifdef ENABLE_DBENGINE
        rc = rrdeng_metric_latest_time_by_uuid(dim_uuid, &first_entry_t, &last_entry_t);

        if (!rc)
            info("DEBUG: %d Checking dimension %s (id=%s, name=%s, chart=%s) --> %s (%ld, %ld)", count, wc->uuid_str,   sqlite3_column_text(res, 1), sqlite3_column_text(res, 2),  sqlite3_column_text(res, 3),
                 dim_uuid_str, !rc ? first_entry_t : 0, !rc ? last_entry_t : 0);
        if (!rc)
            aclk_add_offline_dimension_event(wc, wc->node_id, (char *) sqlite3_column_text(res, 3), dim_uuid, (char *) sqlite3_column_text(res, 1), (char *) sqlite3_column_text(res, 2), first_entry_t, last_entry_t);
#endif
        ++count;
    }

    // Cleanup complete
//    wc->cleanup_after = 0;

    bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when checking dimensions, rc = %d", rc);

    fail_complete:
    buffer_free(sql);
#endif

    return;
}

void sql_check_rotation_state(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);
#ifndef ACLK_NG
    UNUSED(wc);
#else
    int rc;

//    if (unlikely(!wc->chart_updates)) {
//        info("DEBUG: Ignoring sql_check_dimension_state for %s", wc->uuid_str);
//        return;
//    }

    BUFFER *sql = buffer_create(1024);

    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "SELECT dim_id, dimension_id, dimension_name, chart_name FROM delete_dimension WHERE host_id = @host_id;");

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to check for rotated metrics");
        goto fail_complete;
    }
    if (wc->host)
        rc = sqlite3_bind_blob(res, 1, &wc->host->host_uuid , sizeof(wc->host->host_uuid), SQLITE_STATIC);
    else {
        uuid_t host_uuid;
        rc = uuid_parse(wc->host_guid, host_uuid);
        rc = sqlite3_bind_blob(res, 1, &host_uuid, sizeof(host_uuid), SQLITE_STATIC);
    }
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    uuid_t  *dim_uuid;
    char dim_uuid_str[GUID_LEN + 1];
    time_t first_entry_t, last_entry_t;
    info("DEBUG: Checking rotated dimensions %s", wc->uuid_str);
    int count = 0;
    while (sqlite3_step(res) == SQLITE_ROW) {
        dim_uuid = (uuid_t *) sqlite3_column_blob(res, 0);
        uuid_unparse_lower(*dim_uuid, dim_uuid_str);
#ifdef ENABLE_DBENGINE
        rc = rrdeng_metric_latest_time_by_uuid(dim_uuid, &first_entry_t, &last_entry_t);
        if (!rc)
            info("DEBUG: %d Checking rotated dimension %s (id=%s, name=%s, chart=%s) --> %s (%ld, %ld)", count, wc->uuid_str,   sqlite3_column_text(res, 1), sqlite3_column_text(res, 2),  sqlite3_column_text(res, 3),
                 dim_uuid_str, !rc ? first_entry_t : 0, !rc ? last_entry_t : 0);
        else
            info("DEBUG: %d Checking rotated dimension %s (id=%s, name=%s, chart=%s) --> %s (%d, %d) NOT FOUND", count, wc->uuid_str,   sqlite3_column_text(res, 1), sqlite3_column_text(res, 2),  sqlite3_column_text(res, 3),
                 dim_uuid_str, 0, 0);
        //if (!rc)
        //    aclk_add_offline_dimension_event(wc, wc->node_id, (char *) sqlite3_column_text(res, 3), dim_uuid, (char *) sqlite3_column_text(res, 1), (char *) sqlite3_column_text(res, 2), first_entry_t, last_entry_t);
#endif
        ++count;
    }

    bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when checking rotated dimensions, rc = %d", rc);

    fail_complete:
    buffer_free(sql);
#endif

    return;
}


void aclk_get_chart_config(char **hash_id)
{
    if (unlikely(!hash_id))
        return;

    struct aclk_database_worker_config *wc = (struct aclk_database_worker_config *) localhost->dbsync_worker;

    if (unlikely(!wc)) {
        return;
    }

    struct aclk_database_cmd cmd;
    cmd.opcode = ACLK_DATABASE_PUSH_CHART_CONFIG;

    for (int i=0; hash_id[i]; ++i) {
        info("Request for chart config %d -- %s received", i, hash_id[i]);
        cmd.data_param = (void *) strdupz(hash_id[i]);
        cmd.count = 1;
        cmd.completion = NULL;
        aclk_database_enq_cmd(wc, &cmd);
    }

    return;
}

int aclk_push_chart_config_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(wc);
    sqlite3_stmt *res = NULL;

    int rc = 0;
    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return 1;
        }
        error_report("Database has not been initialized");
        return 1;
    }

    char *hash_id = (char *) cmd.data_param;
    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(sql, "SELECT type, family, context, title, priority, plugin, module, unit, chart_type " \
                        "FROM chart_hash WHERE hash_id = @hash_id;");

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to fetch a chart hash configuration");
        goto fail;
    }

    uuid_t hash_uuid;
    uuid_parse(hash_id, hash_uuid);
    rc = sqlite3_bind_blob(res, 1, &hash_uuid , sizeof(hash_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    struct chart_config_updated chart_config;

    while (sqlite3_step(res) == SQLITE_ROW) {
        chart_config.type = strdupz((char *)sqlite3_column_text(res, 0));
        chart_config.family = strdupz((char *)sqlite3_column_text(res, 1));
        chart_config.context = strdupz((char *)sqlite3_column_text(res, 2));
        chart_config.title = strdupz((char *)sqlite3_column_text(res, 3));
        chart_config.priority = sqlite3_column_int64(res, 4);
        chart_config.plugin = strdupz((char *)sqlite3_column_text(res, 5));
        chart_config.module = sqlite3_column_bytes(res, 6) > 0 ? strdupz((char *)sqlite3_column_text(res, 6)) : NULL;
        chart_config.chart_type = (RRDSET_TYPE) sqlite3_column_int(res,8);
        chart_config.units = strdupz((char *)sqlite3_column_text(res, 7));
        chart_config.config_hash = strdupz(hash_id);
    }

    debug(D_ACLK_SYNC,"Sending chart config for %s", hash_id);

    aclk_chart_config_updated(&chart_config, 1);
    destroy_chart_config_updated(&chart_config);
    freez((char *) cmd.data_param);

    bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when pushing chart config hash, rc = %d", rc);

    fail:
    buffer_free(sql);

    return rc;
}

void sql_set_chart_ack(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc;
    sqlite3_stmt *res = NULL;

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql, "UPDATE aclk_chart_%s set date_updated=strftime('%%s') WHERE sequence_id <= @sequence_id "
                        "AND date_submitted IS NOT NULL AND date_updated IS NULL;", wc->uuid_str);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement count sequence ids in the database");
        goto prepare_fail;
    }

    rc = sqlite3_bind_int64(res, 1, (uint64_t) cmd.param1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (rc)
        error_report("Failed to delete sequence ids, rc = %d", rc);

    bind_fail:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize statement to delete older sequence ids, rc = %d", rc);

    prepare_fail:
    buffer_free(sql);
    return;
}


void aclk_submit_param_command(char *node_id, enum aclk_database_opcode aclk_command, uint64_t param)
{
    if (unlikely(!node_id))
        return;

    struct aclk_database_worker_config *wc  = NULL;
    rrd_wrlock();
    RRDHOST *host = find_host_by_node_id(node_id);
    if (likely(host)) {
        wc = (struct aclk_database_worker_config *)host->dbsync_worker;
        rrd_unlock();
        if (likely(wc)) {
            struct aclk_database_cmd cmd;
            cmd.opcode = aclk_command;
            cmd.param1 = param;
            cmd.completion = NULL;
            aclk_database_enq_cmd(wc, &cmd);
        } else
            error("ACLK synchronization thread is not active for host %s", host->hostname);
    }
    else
        rrd_unlock();
    return;
}

void aclk_ack_chart_sequence_id(char *node_id, uint64_t last_sequence_id)
{
    if (unlikely(!node_id))
        return;

    debug(D_ACLK_SYNC, "NODE %s reports last sequence id received %"PRIu64, node_id, last_sequence_id);
    info("DEBUG: NODE %s reports last sequence id received %"PRIu64, node_id, last_sequence_id);
    aclk_submit_param_command(node_id, ACLK_DATABASE_CHART_ACK, last_sequence_id);
    return;
}

void aclk_reset_chart_event(char *node_id, uint64_t last_sequence_id)
{
    if (unlikely(!node_id))
        return;

    debug(D_ACLK_SYNC, "NODE %s wants to resync from %"PRIu64, node_id, last_sequence_id);
    aclk_submit_param_command(node_id, ACLK_DATABASE_RESET_CHART, last_sequence_id);
    return;
}

void sql_chart_deduplicate(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql, "DROP TABLE IF EXISTS t_%s;", wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "DELETE FROM aclk_chart_%s WHERE status is NULL and update_count = 0;", wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "CREATE TABLE ts_%s AS SELECT date_created, date_updated, date_submitted, "
                        " status, uuid, type, unique_id, update_count"
                        " FROM aclk_chart_%s WHERE date_submitted IS NULL AND update_count = 0;", wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

//    buffer_sprintf(sql, "INSERT INTO aclk_chart_%s (uuid, status, update_count) VALUES ('MARK', \"%s\", 0)", wc->uuid_str, wc->uuid_str);
//    db_execute(buffer_tostring(sql));
//    buffer_reset(sql);

    buffer_sprintf(sql, "CREATE TABLE t_%s AS SELECT * FROM aclk_chart_payload_%s WHERE unique_id IN "
                        "(SELECT unique_id from aclk_chart_%s WHERE date_submitted IS NULL AND update_count > 0);", wc->uuid_str, wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "DELETE FROM aclk_chart_payload_%s WHERE unique_id IN (SELECT unique_id FROM t_%s) "
                        "AND unique_id NOT IN (SELECT unique_id FROM ts_%s);", wc->uuid_str, wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "DELETE FROM aclk_chart_%s WHERE unique_id IN (SELECT unique_id FROM t_%s);",
                   wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "INSERT INTO aclk_chart_payload_%s SELECT * FROM t_%s ORDER BY DATE_CREATED ASC;",
                   wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "DELETE FROM ts_%s WHERE uuid IN (SELECT uuid FROM aclk_chart_%s "
                        "WHERE date_submitted IS NULL);", wc->uuid_str, wc->uuid_str);

    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "INSERT INTO aclk_chart_%s (date_created, date_updated, date_submitted, status, uuid, type, unique_id, update_count) "
                        "SELECT * FROM ts_%s ORDER BY DATE_CREATED ASC;", wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

//    buffer_sprintf(sql, "DELETE FROM aclk_chart_%s WHERE uuid = 'MARK' AND status=\"%s\"; ", wc->uuid_str, wc->uuid_str);
//    db_execute(buffer_tostring(sql));
//    buffer_reset(sql);

    buffer_sprintf(sql, "DROP TABLE IF EXISTS ts_%s;", wc->uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_sprintf(sql, "DROP TABLE IF EXISTS t_%s;", wc->uuid_str);
    db_execute(buffer_tostring(sql));

    sql_get_last_chart_sequence(wc, cmd);

    buffer_free(sql);
    return;
}

void sql_get_last_chart_sequence(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql,"SELECT ac.sequence_id, ac.date_created FROM aclk_chart_%s ac " \
                        "WHERE ac.date_submitted IS NOT NULL ORDER BY ac.sequence_id DESC LIMIT 1;", wc->uuid_str);

    int rc;
    sqlite3_stmt *res = NULL;
    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to find last chart sequence id");
        goto fail;
    }

    wc->chart_sequence_id = 0;
    wc->chart_timestamp = 0;
    while (sqlite3_step(res) == SQLITE_ROW) {
        wc->chart_sequence_id = (uint64_t) sqlite3_column_int64(res, 0);
        wc->chart_timestamp  = (time_t) sqlite3_column_int64(res, 1);
    }

    debug(D_ACLK_SYNC,"Chart %s reports last seq=%"PRIu64" t=%ld", wc->uuid_str,
          wc->chart_sequence_id, wc->chart_timestamp);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when fetching chart sequence info, rc = %d", rc);

    fail:
    buffer_free(sql);
    return;
}



// Start streaming charts / dimensions for node_id
void aclk_start_streaming(char *node_id, uint64_t sequence_id, time_t created_at, uint64_t batch_id)
{
    if (unlikely(!node_id))
        return;

    info("DEBUG: START streaming charts for %s from sequence %"PRIu64" t=%ld, batch=%"PRIu64, node_id, sequence_id, created_at, batch_id);
    uuid_t node_uuid;
    uuid_parse(node_id, node_uuid);

    struct aclk_database_worker_config *wc  = NULL;
    rrd_wrlock();
    RRDHOST *host = localhost;
    while(host) {
        if (host->node_id && !(uuid_compare(*host->node_id, node_uuid))) {
            wc = (struct aclk_database_worker_config *)host->dbsync_worker;
            if (likely(wc)) {

                if (unlikely(!wc->chart_updates)) {
                    struct aclk_database_cmd cmd;
                    cmd.opcode = ACLK_DATABASE_NODE_INFO;
                    cmd.completion = NULL;
                    aclk_database_enq_cmd(wc, &cmd);
                }

                wc->batch_id = batch_id;
                wc->batch_created = now_realtime_sec();
                info("DEBUG: START streaming charts for %s (%s) enabled -- last streamed sequence %"PRIu64" t=%ld", node_id, wc->uuid_str,
                     wc->chart_sequence_id, wc->chart_timestamp);
                // If mismatch detected
                if (sequence_id > wc->chart_sequence_id) {
                    info("DEBUG: Full resync requested");
                    chart_reset_t chart_reset;
                    chart_reset.node_id = strdupz(node_id);
                    chart_reset.claim_id = is_agent_claimed();
                    chart_reset.reason = SEQ_ID_NOT_EXISTS;
                    aclk_chart_reset(chart_reset);
                    wc->chart_updates = 0;
                } else {
                    struct aclk_database_cmd cmd;
                    // TODO: handle timestamp
                    if (sequence_id < wc->chart_sequence_id) { // || created_at != wc->chart_timestamp) {
                        wc->chart_updates = 0;
                        info("DEBUG: Synchonization mismatch detected");
                        cmd.opcode = ACLK_DATABASE_RESET_CHART;
                        cmd.param1 = sequence_id + 1;
                        cmd.completion = NULL;
                        aclk_database_enq_cmd(wc, &cmd);
                    }
                    else
                        wc->chart_updates = 1;
                }
            }
            else
                error("ACLK synchronization thread is not active for host %s", host->hostname);
            break;
        }
        host = host->next;
    }
    rrd_unlock();

    return;
}

