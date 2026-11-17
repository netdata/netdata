// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

// Network Policy Server (RADIUS). Both Perflib objects are single-instance and exist only when the IAS service
// is installed. Every counter is PERF_COUNTER_RAWCOUNT: the event counters are cumulative since IAS started, so
// algorithms are set explicitly instead of using perflib_rrddim_add(), which maps raw counts to absolute.
// "Server Up Time" and "Server Reset Time" are already in seconds, although their help text says hundredths of a
// second: on Windows Server 2025 the raw value was 303 about 304 s after IAS started.

#define NPS_MAX_DIMS 6
#define NPS_CHARTS 4

enum nps_object_index {
    NPS_AUTHENTICATION,
    NPS_ACCOUNTING,
    NPS_OBJECTS,
};

// Both objects publish the same counters, except for the request/response pairs and "No Record", so the charts are
// defined once and each dimension names its counter per object.
struct nps_dim_def {
    const char *id;
    const char *key[NPS_OBJECTS]; // NULL: the object does not have this counter
};

struct nps_chart_def {
    const char *suffix; // chart id and context are "<family>_<suffix>"; the totals chart appends "_total"
    const char *title;  // prefixed with "NPS <family>"
    const char *units;
    const char *units_total; // NULL: elapsed-time gauge, no totals chart
    int priority[NPS_OBJECTS];
    int priority_total[NPS_OBJECTS];
    RRDSET_TYPE type;
    struct nps_dim_def dims[NPS_MAX_DIMS + 1];
};

static const struct nps_chart_def nps_chart_defs[NPS_CHARTS] = {
    {
        .suffix = "requests",
        .title = "requests and responses",
        .units = "requests/s",
        .units_total = "requests",
        .priority = {
            [NPS_AUTHENTICATION] = PRIO_NPS_AUTHENTICATION_REQUESTS,
            [NPS_ACCOUNTING] = PRIO_NPS_ACCOUNTING_REQUESTS,
        },
        .priority_total = {
            [NPS_AUTHENTICATION] = PRIO_NPS_AUTHENTICATION_REQUESTS_TOTAL,
            [NPS_ACCOUNTING] = PRIO_NPS_ACCOUNTING_REQUESTS_TOTAL,
        },
        .type = RRDSET_TYPE_LINE,
        .dims = {
            {.id = "requests", .key = {"Access-Requests", "Accounting-Requests"}},
            {.id = "accepts", .key = {"Access-Accepts", NULL}},
            {.id = "rejects", .key = {"Access-Rejects", NULL}},
            {.id = "challenges", .key = {"Access-Challenges", NULL}},
            {.id = "responses", .key = {NULL, "Accounting-Responses"}},
        },
    },
    {
        .suffix = "packets",
        .title = "packets",
        .units = "packets/s",
        .units_total = "packets",
        .priority = {
            [NPS_AUTHENTICATION] = PRIO_NPS_AUTHENTICATION_PACKETS,
            [NPS_ACCOUNTING] = PRIO_NPS_ACCOUNTING_PACKETS,
        },
        .priority_total = {
            [NPS_AUTHENTICATION] = PRIO_NPS_AUTHENTICATION_PACKETS_TOTAL,
            [NPS_ACCOUNTING] = PRIO_NPS_ACCOUNTING_PACKETS_TOTAL,
        },
        .type = RRDSET_TYPE_LINE,
        .dims = {
            {.id = "received", .key = {"Packets Received", "Packets Received"}},
            {.id = "sent", .key = {"Packets Sent", "Packets Sent"}},
        },
    },
    {
        .suffix = "errors",
        .title = "packet errors",
        .units = "packets/s",
        .units_total = "packets",
        .priority = {
            [NPS_AUTHENTICATION] = PRIO_NPS_AUTHENTICATION_ERRORS,
            [NPS_ACCOUNTING] = PRIO_NPS_ACCOUNTING_ERRORS,
        },
        .priority_total = {
            [NPS_AUTHENTICATION] = PRIO_NPS_AUTHENTICATION_ERRORS_TOTAL,
            [NPS_ACCOUNTING] = PRIO_NPS_ACCOUNTING_ERRORS_TOTAL,
        },
        .type = RRDSET_TYPE_STACKED,
        .dims = {
            {.id = "bad_authenticators", .key = {"Bad Authenticators", "Bad Authenticators"}},
            {.id = "dropped", .key = {"Dropped Packets", "Dropped Packets"}},
            {.id = "invalid", .key = {"Invalid Requests", "Invalid Requests"}},
            {.id = "malformed", .key = {"Malformed Packets", "Malformed Packets"}},
            {.id = "no_record", .key = {NULL, "No Record"}},
            {.id = "unknown_type", .key = {"Unknown Type", "Unknown Type"}},
        },
    },
    {
        .suffix = "uptime",
        .title = "server uptime",
        .units = "seconds",
        .units_total = NULL,
        .priority = {
            [NPS_AUTHENTICATION] = PRIO_NPS_AUTHENTICATION_UPTIME,
            [NPS_ACCOUNTING] = PRIO_NPS_ACCOUNTING_UPTIME,
        },
        .type = RRDSET_TYPE_LINE,
        .dims = {
            {.id = "uptime", .key = {"Server Up Time", "Server Up Time"}},
            {.id = "since_reset", .key = {"Server Reset Time", "Server Reset Time"}},
        },
    },
};

struct nps_dim {
    COUNTER_DATA cd;
    RRDDIM *rd;
    RRDDIM *rd_total;
    bool discontinuous; // missed a committed sample; its last collected value is stale
};

struct nps_chart {
    RRDSET *st;
    RRDSET *st_total;
    struct nps_dim dims[NPS_MAX_DIMS];
};

struct nps_object {
    const char *name;
    const char *family;
    struct nps_chart charts[NPS_CHARTS];
};

static struct nps_object nps_objects[NPS_OBJECTS] = {
    [NPS_AUTHENTICATION] = {.name = "NPS Authentication Server", .family = "authentication"},
    [NPS_ACCOUNTING] = {.name = "NPS Accounting Server", .family = "accounting"},
};

static void nps_initialize(void)
{
    for (size_t o = 0; o < NPS_OBJECTS; o++)
        for (size_t c = 0; c < NPS_CHARTS; c++)
            for (size_t i = 0; nps_chart_defs[c].dims[i].id; i++)
                nps_objects[o].charts[c].dims[i].cd.key = nps_chart_defs[c].dims[i].key[o];
}

static RRDSET *nps_chart_create(
    const char *family,
    const char *id,
    const char *title,
    const char *units,
    int priority,
    RRDSET_TYPE type,
    int update_every)
{
    char context[RRD_ID_LENGTH_MAX + 1];
    snprintfz(context, RRD_ID_LENGTH_MAX, "nps.%s", id);

    return rrdset_create_localhost(
        "nps",
        id,
        NULL,
        family,
        context,
        title,
        units,
        PLUGIN_WINDOWS_NAME,
        "PerflibNPS",
        priority,
        update_every,
        type);
}

static void nps_chart_update(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    size_t o,
    const struct nps_chart_def *def,
    struct nps_chart *ch,
    int update_every)
{
    const char *family = nps_objects[o].family;
    bool read[NPS_MAX_DIMS] = {false};
    bool all_read = true, any_read = false;

    for (size_t i = 0; def->dims[i].id; i++) {
        if (!def->dims[i].key[o])
            continue;

        read[i] = perflibGetObjectCounter(pDataBlock, pObjectType, &ch->dims[i].cd);
        all_read = all_read && read[i];
        any_read = any_read || read[i];
    }

    // Create a chart only from a complete sample; afterwards, unread dimensions become gaps.
    if (!any_read || (!ch->st && !all_read))
        return;

    if (unlikely(!ch->st)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        char title[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "%s_%s", family, def->suffix);
        snprintfz(title, RRD_ID_LENGTH_MAX, "NPS %s %s", family, def->title);
        ch->st = nps_chart_create(family, id, title, def->units, def->priority[o], def->type, update_every);

        RRD_ALGORITHM algorithm = def->units_total ? RRD_ALGORITHM_INCREMENTAL : RRD_ALGORITHM_ABSOLUTE;
        for (size_t i = 0; def->dims[i].id; i++) {
            if (def->dims[i].key[o])
                ch->dims[i].rd = rrddim_add(ch->st, def->dims[i].id, NULL, 1, 1, algorithm);
        }

        if (def->units_total) {
            char id_total[RRD_ID_LENGTH_MAX + 1];
            char title_total[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id_total, RRD_ID_LENGTH_MAX, "%s_total", id);
            snprintfz(title_total, RRD_ID_LENGTH_MAX, "%s since NPS start", title);
            ch->st_total = nps_chart_create(
                family, id_total, title_total, def->units_total, def->priority_total[o], def->type, update_every);

            for (size_t i = 0; def->dims[i].id; i++) {
                if (def->dims[i].key[o])
                    ch->dims[i].rd_total =
                        rrddim_add(ch->st_total, def->dims[i].id, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
        }
    }

    for (size_t i = 0; def->dims[i].id; i++) {
        if (!def->dims[i].key[o])
            continue;

        struct nps_dim *d = &ch->dims[i];
        if (!read[i]) {
            d->discontinuous = true;
            continue;
        }

        collected_number value = (collected_number)d->cd.current.Data;

        // The chart kept advancing while this dimension was missing, so the delta against its stale value would
        // be charged to a single interval. Rebaseline it to emit zero for this interval instead of a spike.
        if (d->discontinuous && def->units_total)
            rrddim_set_last_collected_int(d->rd, value);
        d->discontinuous = false;

        rrddim_set_by_pointer(ch->st, d->rd, value);
        if (ch->st_total)
            rrddim_set_by_pointer(ch->st_total, d->rd_total, value);
    }

    rrdset_done(ch->st);
    if (ch->st_total)
        rrdset_done(ch->st_total);
}

static void nps_object_collect(size_t o, int update_every)
{
    struct nps_object *obj = &nps_objects[o];

    // Absent while the NPS role is not installed; the name table refreshes when it is, so keep probing.
    DWORD id = RegistryFindIDByName(obj->name);
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, obj->name);
    if (!pObjectType)
        return;

    for (size_t c = 0; c < NPS_CHARTS; c++)
        nps_chart_update(pDataBlock, pObjectType, o, &nps_chart_defs[c], &obj->charts[c], update_every);
}

int do_PerflibNPS(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;
    if (unlikely(!initialized)) {
        nps_initialize();
        initialized = true;
    }

    // Each object is read and fully processed before the next query reuses the Perflib buffer.
    for (size_t o = 0; o < NPS_OBJECTS; o++)
        nps_object_collect(o, update_every);

    // Never disable: a missing role or a failed read is reported as a gap and may recover.
    return 0;
}
