// SPDX-License-Identifier: GPL-3.0-or-later

#include "status-file-dedup.h"

#define REPORT_EVENTS_EVERY (86400 - 3600) // -1 hour to tolerate cron randomness

static void stack_trace_anonymize(char *s) {
    char *p = s;
    while (*p && (p = strstr(p, "0x"))) {
        p[1] = '0';
        p += 2;
        while(isxdigit((uint8_t)*p)) *p++ = '0';
    }
}

uint64_t daemon_status_file_hash(DAEMON_STATUS_FILE *ds, const char *msg, const char *cause) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    struct {
        uint32_t v;
        DAEMON_STATUS status;
        SIGNAL_CODE signal_code;
        ND_PROFILE profile;
        EXIT_REASON exit_reason;
        RRD_DB_MODE db_mode;
        uint32_t worker_job_id;
        uint8_t db_tiers;
        bool kubernetes;
        bool sentry_available;
        bool sentry_fatal;
        ND_UUID host_id;
        ND_UUID machine_id;
        long line;
        char version[sizeof(ds->version)];
        char filename[sizeof(ds->fatal.filename)];
        char function[sizeof(ds->fatal.function)];
        char stack_trace[sizeof(ds->fatal.stack_trace)];
        char thread[sizeof(ds->fatal.thread)];
        char msg[128];
        char cause[32];
    } to_hash;

    // this is important to remove any random bytes from the structure
    memset(&to_hash, 0, sizeof(to_hash));

    dsf_acquire(*ds);

    to_hash.v = ds->v,
    to_hash.status = ds->status,
    to_hash.signal_code = ds->fatal.signal_code,
    to_hash.profile = ds->profile,
    to_hash.exit_reason = ds->exit_reason,
    to_hash.db_mode = ds->db_mode,
    to_hash.db_tiers = ds->db_tiers,
    to_hash.kubernetes = ds->kubernetes,
    to_hash.sentry_available = ds->sentry_available,
    to_hash.sentry_fatal = ds->fatal.sentry,
    to_hash.host_id = ds->host_id,
    to_hash.machine_id = ds->machine_id,
    to_hash.worker_job_id = ds->fatal.worker_job_id,

    strncpyz(to_hash.version, ds->version, sizeof(to_hash.version) - 1);
    strncpyz(to_hash.filename, ds->fatal.filename, sizeof(to_hash.filename) - 1);
    strncpyz(to_hash.filename, ds->fatal.function, sizeof(to_hash.function) - 1);
    strncpyz(to_hash.stack_trace, ds->fatal.stack_trace, sizeof(to_hash.stack_trace) - 1);
    strncpyz(to_hash.thread, ds->fatal.thread, sizeof(to_hash.thread) - 1);

    if(msg)
        strncpyz(to_hash.msg, msg, sizeof(to_hash.msg) - 1);

    if(cause)
        strncpyz(to_hash.cause, cause, sizeof(to_hash.cause) - 1);

    stack_trace_anonymize(to_hash.stack_trace);

    uint64_t hash = fnv1a_hash_bin64(&to_hash, sizeof(to_hash));

    dsf_release(*ds);
    return hash;
}

void daemon_status_dedup_to_json(BUFFER *wb, DAEMON_STATUS_DEDUP *dp) {
    buffer_json_member_add_array(wb, "dedup");
    {
        for(size_t i = 0; i < _countof(dp->slot); i++) {
            if (dp->slot[i].timestamp_ut == 0)
                continue;

            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_datetime_rfc3339(wb, "@timestamp", dp->slot[i].timestamp_ut, true);
                buffer_json_member_add_uint64(wb, "hash", dp->slot[i].hash);
                buffer_json_member_add_boolean(wb, "sentry", dp->slot[i].sentry);
            }
            buffer_json_object_close(wb);
        }
    }
    buffer_json_array_close(wb);
}

bool daemon_status_dedup_from_json(json_object *jobj, void *data, BUFFER *error) {
    char path[1024]; path[0] = '\0';
    DAEMON_STATUS_DEDUP *dp = data;

    bool required = false;
    JSONC_PARSE_ARRAY(jobj, path, "dedup", error, required, {
        size_t i = 0;
        JSONC_PARSE_ARRAY_ITEM_OBJECT(jobj, path, i, required, {
            if(i < _countof(dp->slot)) {
                JSONC_PARSE_TXT2RFC3339_USEC_OR_ERROR_AND_RETURN(jobj, path, "@timestamp", dp->slot[i].timestamp_ut, error, required);
                JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "hash", dp->slot[i].hash, error, required);
                JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, "sentry", dp->slot[i].sentry, error, required);
            }
        });
    });

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// deduplication hashes management

bool dedup_already_posted(DAEMON_STATUS_FILE *ds, uint64_t hash, bool sentry) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    usec_t now_ut = now_realtime_usec();

    for(size_t i = 0; i < _countof(ds->dedup.slot); i++) {
        if(ds->dedup.slot[i].timestamp_ut == 0)
            continue;

        if(hash == ds->dedup.slot[i].hash &&
            sentry == ds->dedup.slot[i].sentry &&
            now_ut - ds->dedup.slot[i].timestamp_ut < REPORT_EVENTS_EVERY * USEC_PER_SEC) {
            // we have already posted this crash
            return true;
        }
    }

    return false;
}

void dedup_keep_hash(DAEMON_STATUS_FILE *ds, uint64_t hash, bool sentry) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    // find the same hash
    for(size_t i = 0; i < _countof(ds->dedup.slot); i++) {
        if(ds->dedup.slot[i].hash == hash && ds->dedup.slot[i].sentry == sentry) {
            ds->dedup.slot[i].hash = hash;
            ds->dedup.slot[i].sentry = sentry;
            ds->dedup.slot[i].timestamp_ut = now_realtime_usec();
            return;
        }
    }

    // find an empty slot
    for(size_t i = 0; i < _countof(ds->dedup.slot); i++) {
        if(!ds->dedup.slot[i].hash) {
            ds->dedup.slot[i].hash = hash;
            ds->dedup.slot[i].sentry = sentry;
            ds->dedup.slot[i].timestamp_ut = now_realtime_usec();
            return;
        }
    }

    // find the oldest slot
    size_t store_at_slot = 0;
    for(size_t i = 1; i < _countof(ds->dedup.slot); i++) {
        if(ds->dedup.slot[i].timestamp_ut < ds->dedup.slot[store_at_slot].timestamp_ut)
            store_at_slot = i;
    }

    ds->dedup.slot[store_at_slot].hash = hash;
    ds->dedup.slot[store_at_slot].sentry = sentry;
    ds->dedup.slot[store_at_slot].timestamp_ut = now_realtime_usec();
}
