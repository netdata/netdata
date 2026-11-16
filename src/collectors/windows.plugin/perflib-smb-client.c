// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define SMB_CLIENT_OBJECT_NAME "SMB Client Shares"
#define SMB_CLIENT_NAME_PART_MAX 512
#define SMB_CLIENT_MAX_DIMENSIONS 3

// A successful query may omit the object for a cycle; remove shares only after this many consecutive misses,
// matching perflib-iis-extra.c, so a transient omission does not obsolete and recreate every chart.
#define SMB_CLIENT_OBJECT_MISSING_CYCLES 12

enum smb_client_counter {
    SMBC_READ_BYTES,
    SMBC_WRITE_BYTES,
    SMBC_READ_REQUESTS,
    SMBC_WRITE_REQUESTS,
    SMBC_METADATA_REQUESTS,
    SMBC_SMBDIRECT_READ_BYTES,
    SMBC_SMBDIRECT_WRITE_BYTES,
    SMBC_SMBDIRECT_READ_REQUESTS,
    SMBC_SMBDIRECT_WRITE_REQUESTS,
    SMBC_TURBO_IO_READS,
    SMBC_TURBO_IO_WRITES,
    SMBC_CREDIT_STALLS,
    SMBC_CURRENT_DATA_QUEUE_LENGTH,
    SMBC_AVG_READ_QUEUE_LENGTH,
    SMBC_AVG_WRITE_QUEUE_LENGTH,
    SMBC_AVG_SEC_PER_READ,
    SMBC_AVG_SEC_PER_WRITE,
    SMBC_AVG_SEC_PER_DATA_REQUEST,

    SMBC_COUNTERS,
};

static const char *smb_client_counter_keys[SMBC_COUNTERS] = {
    [SMBC_READ_BYTES] = "Read Bytes/sec",
    [SMBC_WRITE_BYTES] = "Write Bytes/sec",
    [SMBC_READ_REQUESTS] = "Read Requests/sec",
    [SMBC_WRITE_REQUESTS] = "Write Requests/sec",
    [SMBC_METADATA_REQUESTS] = "Metadata Requests/sec",
    [SMBC_SMBDIRECT_READ_BYTES] = "Read Bytes transmitted via SMB Direct/sec",
    [SMBC_SMBDIRECT_WRITE_BYTES] = "Write Bytes transmitted via SMB Direct/sec",
    [SMBC_SMBDIRECT_READ_REQUESTS] = "Read Requests transmitted via SMB Direct/sec",
    [SMBC_SMBDIRECT_WRITE_REQUESTS] = "Write Requests transmitted via SMB Direct/sec",
    [SMBC_TURBO_IO_READS] = "Turbo I/O Reads/sec",
    [SMBC_TURBO_IO_WRITES] = "Turbo I/O Writes/sec",
    [SMBC_CREDIT_STALLS] = "Credit Stalls/sec",
    [SMBC_CURRENT_DATA_QUEUE_LENGTH] = "Current Data Queue Length",
    [SMBC_AVG_READ_QUEUE_LENGTH] = "Avg. Read Queue Length",
    [SMBC_AVG_WRITE_QUEUE_LENGTH] = "Avg. Write Queue Length",
    [SMBC_AVG_SEC_PER_READ] = "Avg. sec/Read",
    [SMBC_AVG_SEC_PER_WRITE] = "Avg. sec/Write",
    [SMBC_AVG_SEC_PER_DATA_REQUEST] = "Avg. sec/Data Request",
};

enum smb_client_value_kind {
    // PERF_COUNTER_COUNTER / PERF_COUNTER_BULK_COUNT: cumulative count, charted as a rate.
    SMBC_KIND_RATE,
    // PERF_COUNTER_RAWCOUNT: instantaneous value.
    SMBC_KIND_GAUGE,
    // PERF_COUNTER_100NS_QUEUELEN_TYPE: accumulated queue time in 100 ns ticks; its rate is the average queue length.
    SMBC_KIND_QUEUE,
    // PERF_AVERAGE_TIMER: interval mean latency per operation, in microseconds.
    SMBC_KIND_LATENCY,
};

struct smb_client_dimension {
    enum smb_client_counter counter;
    const char *id;
    collected_number multiplier;
};

struct smb_client_chart_definition {
    const char *suffix;
    const char *context;
    const char *title;
    const char *units;
    int priority;
    RRDSET_TYPE type;
    enum smb_client_value_kind kind;
    size_t dimensions_count;
    struct smb_client_dimension dimensions[SMB_CLIENT_MAX_DIMENSIONS];
};

enum smb_client_chart {
    SMBC_CHART_TRAFFIC,
    SMBC_CHART_REQUESTS,
    SMBC_CHART_SMBDIRECT_TRAFFIC,
    SMBC_CHART_SMBDIRECT_REQUESTS,
    SMBC_CHART_TURBO_IO,
    SMBC_CHART_CREDIT_STALLS,
    SMBC_CHART_OUTSTANDING_REQUESTS,
    SMBC_CHART_AVG_QUEUE_LENGTH,
    SMBC_CHART_LATENCY,

    SMBC_CHARTS,
};

static const struct smb_client_chart_definition smb_client_charts[SMBC_CHARTS] = {
    [SMBC_CHART_TRAFFIC] = {
        .suffix = "traffic",
        .context = "smb.client_share_traffic",
        .title = "SMB client share traffic",
        .units = "bytes/s",
        .priority = PRIO_SMB_CLIENT_SHARE_TRAFFIC,
        .type = RRDSET_TYPE_AREA,
        .kind = SMBC_KIND_RATE,
        .dimensions_count = 2,
        .dimensions = {
            {SMBC_READ_BYTES, "read", 1},
            {SMBC_WRITE_BYTES, "write", -1},
        },
    },
    [SMBC_CHART_REQUESTS] = {
        .suffix = "requests",
        .context = "smb.client_share_requests",
        .title = "SMB client share requests",
        .units = "requests/s",
        .priority = PRIO_SMB_CLIENT_SHARE_REQUESTS,
        .type = RRDSET_TYPE_LINE,
        .kind = SMBC_KIND_RATE,
        .dimensions_count = 3,
        .dimensions = {
            {SMBC_READ_REQUESTS, "read", 1},
            {SMBC_WRITE_REQUESTS, "write", 1},
            {SMBC_METADATA_REQUESTS, "metadata", 1},
        },
    },
    [SMBC_CHART_SMBDIRECT_TRAFFIC] = {
        .suffix = "smbdirect_traffic",
        .context = "smb.client_share_smbdirect_traffic",
        .title = "SMB client share traffic via SMB Direct",
        .units = "bytes/s",
        .priority = PRIO_SMB_CLIENT_SHARE_SMBDIRECT_TRAFFIC,
        .type = RRDSET_TYPE_AREA,
        .kind = SMBC_KIND_RATE,
        .dimensions_count = 2,
        .dimensions = {
            {SMBC_SMBDIRECT_READ_BYTES, "read", 1},
            {SMBC_SMBDIRECT_WRITE_BYTES, "write", -1},
        },
    },
    [SMBC_CHART_SMBDIRECT_REQUESTS] = {
        .suffix = "smbdirect_requests",
        .context = "smb.client_share_smbdirect_requests",
        .title = "SMB client share requests via SMB Direct",
        .units = "requests/s",
        .priority = PRIO_SMB_CLIENT_SHARE_SMBDIRECT_REQUESTS,
        .type = RRDSET_TYPE_LINE,
        .kind = SMBC_KIND_RATE,
        .dimensions_count = 2,
        .dimensions = {
            {SMBC_SMBDIRECT_READ_REQUESTS, "read", 1},
            {SMBC_SMBDIRECT_WRITE_REQUESTS, "write", 1},
        },
    },
    [SMBC_CHART_TURBO_IO] = {
        .suffix = "turbo_io",
        .context = "smb.client_share_turbo_io",
        .title = "SMB client share Turbo I/O requests",
        .units = "requests/s",
        .priority = PRIO_SMB_CLIENT_SHARE_TURBO_IO,
        .type = RRDSET_TYPE_LINE,
        .kind = SMBC_KIND_RATE,
        .dimensions_count = 2,
        .dimensions = {
            {SMBC_TURBO_IO_READS, "read", 1},
            {SMBC_TURBO_IO_WRITES, "write", 1},
        },
    },
    [SMBC_CHART_CREDIT_STALLS] = {
        .suffix = "credit_stalls",
        .context = "smb.client_share_credit_stalls",
        .title = "SMB client share requests delayed by insufficient credits",
        .units = "stalls/s",
        .priority = PRIO_SMB_CLIENT_SHARE_CREDIT_STALLS,
        .type = RRDSET_TYPE_LINE,
        .kind = SMBC_KIND_RATE,
        .dimensions_count = 1,
        .dimensions = {
            {SMBC_CREDIT_STALLS, "stalls", 1},
        },
    },
    [SMBC_CHART_OUTSTANDING_REQUESTS] = {
        .suffix = "outstanding_requests",
        .context = "smb.client_share_outstanding_requests",
        .title = "SMB client share outstanding requests",
        .units = "requests",
        .priority = PRIO_SMB_CLIENT_SHARE_OUTSTANDING_REQUESTS,
        .type = RRDSET_TYPE_LINE,
        .kind = SMBC_KIND_GAUGE,
        .dimensions_count = 1,
        .dimensions = {
            {SMBC_CURRENT_DATA_QUEUE_LENGTH, "outstanding", 1},
        },
    },
    [SMBC_CHART_AVG_QUEUE_LENGTH] = {
        .suffix = "avg_queue_length",
        .context = "smb.client_share_avg_queue_length",
        .title = "SMB client share average queue length",
        .units = "requests",
        .priority = PRIO_SMB_CLIENT_SHARE_AVG_QUEUE_LENGTH,
        .type = RRDSET_TYPE_LINE,
        .kind = SMBC_KIND_QUEUE,
        .dimensions_count = 2,
        .dimensions = {
            {SMBC_AVG_READ_QUEUE_LENGTH, "read", 1},
            {SMBC_AVG_WRITE_QUEUE_LENGTH, "write", 1},
        },
    },
    [SMBC_CHART_LATENCY] = {
        .suffix = "latency",
        .context = "smb.client_share_latency",
        .title = "SMB client share average request latency",
        .units = "milliseconds",
        .priority = PRIO_SMB_CLIENT_SHARE_LATENCY,
        .type = RRDSET_TYPE_LINE,
        .kind = SMBC_KIND_LATENCY,
        .dimensions_count = 3,
        .dimensions = {
            {SMBC_AVG_SEC_PER_READ, "read", 1},
            {SMBC_AVG_SEC_PER_WRITE, "write", 1},
            // Count-weighted mean over reads and writes; not derivable from the read and write lines.
            {SMBC_AVG_SEC_PER_DATA_REQUEST, "data", 1},
        },
    },
};

struct smb_client_share {
    usec_t last_collected;

    COUNTER_DATA counters[SMBC_COUNTERS];

    RRDSET *st[SMBC_CHARTS];
    RRDDIM *rd[SMBC_CHARTS][SMB_CLIENT_MAX_DIMENSIONS];
};

static DICTIONARY *smb_client_shares = NULL;

static void smb_client_share_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct smb_client_share *share = value;

    for (size_t i = 0; i < SMBC_COUNTERS; i++)
        share->counters[i].key = smb_client_counter_keys[i];
}

static void smb_client_share_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct smb_client_share *share = value;

    for (size_t i = 0; i < SMBC_CHARTS; i++)
        rrdset_is_obsolete___safe_from_collector_thread(share->st[i]);
}

static void initialize(void)
{
    smb_client_shares = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct smb_client_share));

    dictionary_register_insert_callback(smb_client_shares, smb_client_share_insert_cb, NULL);
    dictionary_register_delete_callback(smb_client_shares, smb_client_share_delete_cb, NULL);
}

// Splits an instance name such as "\server\share" into exactly two non-empty parts, as Windows Exporter does.
static bool smb_client_parse_instance(const char *name, char *server, char *share)
{
    const char *parts[2];
    size_t lengths[2];
    size_t count = 0;

    for (const char *s = name; *s;) {
        while (*s == '\\')
            s++;

        if (!*s)
            break;

        const char *start = s;
        while (*s && *s != '\\')
            s++;

        if (count == 2)
            return false;

        parts[count] = start;
        lengths[count] = (size_t)(s - start);
        count++;
    }

    if (count != 2 || lengths[0] >= SMB_CLIENT_NAME_PART_MAX || lengths[1] >= SMB_CLIENT_NAME_PART_MAX)
        return false;

    memcpy(server, parts[0], lengths[0]);
    server[lengths[0]] = '\0';
    memcpy(share, parts[1], lengths[1]);
    share[lengths[1]] = '\0';
    return true;
}

static void smb_client_charts_create(
    struct smb_client_share *share,
    const char *instance,
    const char *server,
    const char *share_name,
    int update_every)
{
    // Sanitizing merges the server/share boundary and maps symbols such as '$' to '_', so distinct shares
    // (e.g. "C$" and "C_") could share chart IDs. A hash of the raw instance name keeps them apart across restarts.
    uint32_t instance_hash = (uint32_t)XXH3_64bits(instance, strlen(instance));

    for (size_t c = 0; c < SMBC_CHARTS; c++) {
        const struct smb_client_chart_definition *def = &smb_client_charts[c];

        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(
            id, sizeof(id), "smb_client_%s_%s_%08x_%s", server, share_name, (unsigned)instance_hash, def->suffix);
        netdata_fix_chart_name(id);

        RRDSET *st = rrdset_create_localhost(
            "smb",
            id,
            NULL,
            "client shares",
            def->context,
            def->title,
            def->units,
            PLUGIN_WINDOWS_NAME,
            "PerflibSMBClient",
            def->priority,
            update_every,
            def->type);

        rrdlabels_add(st->rrdlabels, "server", server, RRDLABEL_SRC_AUTO);
        rrdlabels_add(st->rrdlabels, "share", share_name, RRDLABEL_SRC_AUTO);

        RRD_ALGORITHM algorithm = RRD_ALGORITHM_INCREMENTAL;
        collected_number divider = 1;
        switch (def->kind) {
            case SMBC_KIND_RATE:
                break;
            case SMBC_KIND_GAUGE:
                algorithm = RRD_ALGORITHM_ABSOLUTE;
                break;
            case SMBC_KIND_QUEUE:
                divider = (collected_number)(NS100_PER_MS * MSEC_PER_SEC);
                break;
            case SMBC_KIND_LATENCY:
                algorithm = RRD_ALGORITHM_ABSOLUTE;
                divider = (collected_number)USEC_PER_MS;
                break;
        }

        for (size_t d = 0; d < def->dimensions_count; d++)
            share->rd[c][d] =
                rrddim_add(st, def->dimensions[d].id, NULL, def->dimensions[d].multiplier, divider, algorithm);

        share->st[c] = st;
    }
}

// Leaves the dimension unset (a gap) when this cycle's sample cannot produce a value.
static void smb_client_dimension_set(RRDSET *st, RRDDIM *rd, const COUNTER_DATA *cd, enum smb_client_value_kind kind)
{
    if (!cd->updated)
        return;

    if (kind == SMBC_KIND_LATENCY) {
        double ms;
        if (!perflib_average_timer_ms(cd, &ms))
            return;

        double usec = ms * (double)USEC_PER_MS;
        if (usec > (double)INT64_MAX)
            return;

        rrddim_set_by_pointer(st, rd, (collected_number)usec);
        return;
    }

    rrddim_set_by_pointer(st, rd, (collected_number)cd->current.Data);
}

static void smb_client_charts_update(struct smb_client_share *share)
{
    for (size_t c = 0; c < SMBC_CHARTS; c++) {
        const struct smb_client_chart_definition *def = &smb_client_charts[c];

        for (size_t d = 0; d < def->dimensions_count; d++)
            smb_client_dimension_set(
                share->st[c], share->rd[c][d], &share->counters[def->dimensions[d].counter], def->kind);

        rrdset_done(share->st[c]);
    }
}

static void smb_client_shares_remove(bool all, usec_t now_ut)
{
    struct smb_client_share *share;
    dfe_start_write(smb_client_shares, share)
    {
        if (all || share->last_collected < now_ut)
            dictionary_del(smb_client_shares, share_dfe.name);
    }
    dfe_done(share);
    dictionary_garbage_collect(smb_client_shares);
}

static void do_smb_client_shares(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    static size_t missing_object_cycles = 0;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, SMB_CLIENT_OBJECT_NAME);
    if (!pObjectType) {
        // Charts gap while the object is missing; a persistent absence means no client shares remain.
        if (++missing_object_cycles >= SMB_CLIENT_OBJECT_MISSING_CYCLES) {
            smb_client_shares_remove(true, 0);
            missing_object_cycles = 0;
        }
        return;
    }
    missing_object_cycles = 0;

    usec_t now_ut = now_monotonic_usec();
    bool complete = true;

    char server[SMB_CLIENT_NAME_PART_MAX];
    char share_name[SMB_CLIENT_NAME_PART_MAX];

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi) {
            complete = false;
            break;
        }

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer))) {
            // Keep this cycle's valid shares, but do not evict from an incomplete snapshot.
            complete = false;
            continue;
        }

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        if (!smb_client_parse_instance(windows_shared_buffer, server, share_name)) {
            nd_log_limit_static_global_var(erl, 3600, 0);
            nd_log_limit(
                &erl,
                NDLS_COLLECTORS,
                NDLP_WARNING,
                "WINDOWS: PERFLIB: skipping '%s' instance with unexpected name '%s'",
                SMB_CLIENT_OBJECT_NAME,
                windows_shared_buffer);
            continue;
        }

        struct smb_client_share *share =
            dictionary_set(smb_client_shares, windows_shared_buffer, NULL, sizeof(*share));
        if (!share) {
            complete = false;
            continue;
        }

        share->last_collected = now_ut;

        for (size_t c = 0; c < SMBC_COUNTERS; c++)
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->counters[c]);

        if (unlikely(!share->st[0]))
            smb_client_charts_create(share, windows_shared_buffer, server, share_name, update_every);

        smb_client_charts_update(share);
    }

    if (complete)
        smb_client_shares_remove(false, now_ut);
}

void do_PerflibSMBClient_cleanup(void)
{
    dictionary_destroy(smb_client_shares);
    smb_client_shares = NULL;
}

int do_PerflibSMBClient(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName(SMB_CLIENT_OBJECT_NAME);
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        // A failed query is transient: keep the charts and retry next cycle.
        return 0;

    do_smb_client_shares(pDataBlock, update_every);

    return 0;
}
