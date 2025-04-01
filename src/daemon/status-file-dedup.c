// SPDX-License-Identifier: GPL-3.0-or-later

#include "status-file-dedup.h"
#include "status-file-io.h"

#define DEDUP_FILENAME "dedup-netdata.dat"
#define DEDUP_VERSION 1
#define DEDUP_MAGIC 0x1DEDA9F17EDA7150 // 1(x) DEDUPFILEDAT (v)1 50(entries)

#define REPORT_EVENTS_EVERY (86400 - 3600) // -1 hour to tolerate cron randomness

typedef struct {
    uint64_t magic;
    size_t v;
    uint64_t hash;
    struct {
        bool sentry;
        uint64_t hash;
        usec_t timestamp_ut;
    } slot[50];
} DAEMON_STATUS_DEDUP;

static DAEMON_STATUS_DEDUP dedup = { 0 };

static void stack_trace_anonymize(char *s) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

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
        ND_MACHINE_GUID host_id;
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

// --------------------------------------------------------------------------------------------------------------------
// read and write the dedup hashes

static bool status_file_dedup_load_and_parse(const char *filename, void *data __maybe_unused) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    int fp = open(filename, O_RDONLY);
    if(fp == -1)
        goto failed;

    memset(&dedup, 0, sizeof(dedup));
    ssize_t r = read(fp, &dedup, sizeof(dedup));
    close(fp);

    if(r != sizeof(dedup))
        goto failed;

    if(dedup.magic != DEDUP_MAGIC)
        goto failed;

    if(dedup.v != DEDUP_VERSION)
        goto failed;

    uint64_t hash = fnv1a_hash_bin64(&dedup.slot, sizeof(dedup.slot));
    if(dedup.hash != hash)
        goto failed;

    return true;

failed:
    memset(&dedup, 0, sizeof(dedup));
    return false;
}

bool daemon_status_dedup_load(void) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    return status_file_io_load(DEDUP_FILENAME, status_file_dedup_load_and_parse, NULL);
}

static bool daemon_status_dedup_save(void) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    dedup.magic = DEDUP_MAGIC;
    dedup.v = DEDUP_VERSION;
    dedup.hash = fnv1a_hash_bin64(&dedup.slot, sizeof(dedup.slot));
    return status_file_io_save(DEDUP_FILENAME, &dedup, sizeof(dedup), false);
}

// --------------------------------------------------------------------------------------------------------------------
// deduplication hashes management

bool dedup_already_posted(DAEMON_STATUS_FILE *ds __maybe_unused, uint64_t hash, bool sentry) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    daemon_status_dedup_load();

    usec_t now_ut = now_realtime_usec();

    for(size_t i = 0; i < _countof(dedup.slot); i++) {
        if(dedup.slot[i].timestamp_ut == 0)
            continue;

        if(hash == dedup.slot[i].hash &&
            sentry == dedup.slot[i].sentry &&
            now_ut - dedup.slot[i].timestamp_ut < REPORT_EVENTS_EVERY * USEC_PER_SEC) {
            // we have already posted this crash
            return true;
        }
    }

    return false;
}

void dedup_keep_hash(DAEMON_STATUS_FILE *ds __maybe_unused, uint64_t hash, bool sentry) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    daemon_status_dedup_load();

    // find the same hash
    for(size_t i = 0; i < _countof(dedup.slot); i++) {
        if(dedup.slot[i].hash == hash && dedup.slot[i].sentry == sentry) {
            dedup.slot[i].hash = hash;
            dedup.slot[i].sentry = sentry;
            dedup.slot[i].timestamp_ut = now_realtime_usec();
            goto save;
        }
    }

    // find an empty slot
    for(size_t i = 0; i < _countof(dedup.slot); i++) {
        if(!dedup.slot[i].hash) {
            dedup.slot[i].hash = hash;
            dedup.slot[i].sentry = sentry;
            dedup.slot[i].timestamp_ut = now_realtime_usec();
            goto save;
        }
    }

    // find the oldest slot
    size_t store_at_slot = 0;
    for(size_t i = 1; i < _countof(dedup.slot); i++) {
        if(dedup.slot[i].timestamp_ut < dedup.slot[store_at_slot].timestamp_ut)
            store_at_slot = i;
    }

    dedup.slot[store_at_slot].hash = hash;
    dedup.slot[store_at_slot].sentry = sentry;
    dedup.slot[store_at_slot].timestamp_ut = now_realtime_usec();

save:
    daemon_status_dedup_save();
}
