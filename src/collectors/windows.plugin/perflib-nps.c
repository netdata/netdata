// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

// Network Policy Server (RADIUS). Both Perflib objects are single-instance and exist only when the IAS service
// is installed. Every counter is PERF_COUNTER_RAWCOUNT: the event counters are cumulative since IAS started, so
// algorithms are set explicitly instead of using perflib_rrddim_add(), which maps raw counts to absolute.
// "Server Up Time" and "Server Reset Time" are already in seconds, although their help text says hundredths of a
// second: on Windows Server 2025 the raw value was 303 about 304 s after IAS started.

#define NPS_MAX_DIMS 6

struct nps_dim {
    const char *id;
    COUNTER_DATA cd;
    RRDDIM *rd;
    RRDDIM *rd_total;
};

struct nps_chart {
    const char *id; // also the context suffix; the totals chart appends "_total"
    const char *title;
    const char *units;
    const char *units_total; // NULL: elapsed-time gauge, no totals chart
    int priority;
    int priority_total;
    RRDSET_TYPE type;
    struct nps_dim dims[NPS_MAX_DIMS + 1];

    RRDSET *st;
    RRDSET *st_total;
};

struct nps_object {
    const char *name;
    const char *family;
    struct nps_chart *charts;
};

static struct nps_chart nps_authentication_charts[] = {
    {
        .id = "authentication_requests",
        .title = "NPS authentication requests and responses",
        .units = "requests/s",
        .units_total = "requests",
        .priority = PRIO_NPS_AUTHENTICATION_REQUESTS,
        .priority_total = PRIO_NPS_AUTHENTICATION_REQUESTS_TOTAL,
        .type = RRDSET_TYPE_LINE,
        .dims = {
            {.id = "requests", .cd = {.key = "Access-Requests"}},
            {.id = "accepts", .cd = {.key = "Access-Accepts"}},
            {.id = "rejects", .cd = {.key = "Access-Rejects"}},
            {.id = "challenges", .cd = {.key = "Access-Challenges"}},
        },
    },
    {
        .id = "authentication_packets",
        .title = "NPS authentication packets",
        .units = "packets/s",
        .units_total = "packets",
        .priority = PRIO_NPS_AUTHENTICATION_PACKETS,
        .priority_total = PRIO_NPS_AUTHENTICATION_PACKETS_TOTAL,
        .type = RRDSET_TYPE_LINE,
        .dims = {
            {.id = "received", .cd = {.key = "Packets Received"}},
            {.id = "sent", .cd = {.key = "Packets Sent"}},
        },
    },
    {
        .id = "authentication_errors",
        .title = "NPS authentication packet errors",
        .units = "packets/s",
        .units_total = "packets",
        .priority = PRIO_NPS_AUTHENTICATION_ERRORS,
        .priority_total = PRIO_NPS_AUTHENTICATION_ERRORS_TOTAL,
        .type = RRDSET_TYPE_STACKED,
        .dims = {
            {.id = "bad_authenticators", .cd = {.key = "Bad Authenticators"}},
            {.id = "dropped", .cd = {.key = "Dropped Packets"}},
            {.id = "invalid", .cd = {.key = "Invalid Requests"}},
            {.id = "malformed", .cd = {.key = "Malformed Packets"}},
            {.id = "unknown_type", .cd = {.key = "Unknown Type"}},
        },
    },
    {
        .id = "authentication_uptime",
        .title = "NPS authentication server uptime",
        .units = "seconds",
        .units_total = NULL,
        .priority = PRIO_NPS_AUTHENTICATION_UPTIME,
        .type = RRDSET_TYPE_LINE,
        .dims = {
            {.id = "uptime", .cd = {.key = "Server Up Time"}},
            {.id = "since_reset", .cd = {.key = "Server Reset Time"}},
        },
    },
    {.id = NULL},
};

static struct nps_chart nps_accounting_charts[] = {
    {
        .id = "accounting_requests",
        .title = "NPS accounting requests and responses",
        .units = "requests/s",
        .units_total = "requests",
        .priority = PRIO_NPS_ACCOUNTING_REQUESTS,
        .priority_total = PRIO_NPS_ACCOUNTING_REQUESTS_TOTAL,
        .type = RRDSET_TYPE_LINE,
        .dims = {
            {.id = "requests", .cd = {.key = "Accounting-Requests"}},
            {.id = "responses", .cd = {.key = "Accounting-Responses"}},
        },
    },
    {
        .id = "accounting_packets",
        .title = "NPS accounting packets",
        .units = "packets/s",
        .units_total = "packets",
        .priority = PRIO_NPS_ACCOUNTING_PACKETS,
        .priority_total = PRIO_NPS_ACCOUNTING_PACKETS_TOTAL,
        .type = RRDSET_TYPE_LINE,
        .dims = {
            {.id = "received", .cd = {.key = "Packets Received"}},
            {.id = "sent", .cd = {.key = "Packets Sent"}},
        },
    },
    {
        .id = "accounting_errors",
        .title = "NPS accounting packet errors",
        .units = "packets/s",
        .units_total = "packets",
        .priority = PRIO_NPS_ACCOUNTING_ERRORS,
        .priority_total = PRIO_NPS_ACCOUNTING_ERRORS_TOTAL,
        .type = RRDSET_TYPE_STACKED,
        .dims = {
            {.id = "bad_authenticators", .cd = {.key = "Bad Authenticators"}},
            {.id = "dropped", .cd = {.key = "Dropped Packets"}},
            {.id = "invalid", .cd = {.key = "Invalid Requests"}},
            {.id = "malformed", .cd = {.key = "Malformed Packets"}},
            {.id = "no_record", .cd = {.key = "No Record"}},
            {.id = "unknown_type", .cd = {.key = "Unknown Type"}},
        },
    },
    {
        .id = "accounting_uptime",
        .title = "NPS accounting server uptime",
        .units = "seconds",
        .units_total = NULL,
        .priority = PRIO_NPS_ACCOUNTING_UPTIME,
        .type = RRDSET_TYPE_LINE,
        .dims = {
            {.id = "uptime", .cd = {.key = "Server Up Time"}},
            {.id = "since_reset", .cd = {.key = "Server Reset Time"}},
        },
    },
    {.id = NULL},
};

static struct nps_object nps_objects[] = {
    {.name = "NPS Authentication Server", .family = "authentication", .charts = nps_authentication_charts},
    {.name = "NPS Accounting Server", .family = "accounting", .charts = nps_accounting_charts},
};

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
    const char *family,
    struct nps_chart *ch,
    int update_every)
{
    bool read[NPS_MAX_DIMS];
    bool all_read = true, any_read = false;

    for (size_t i = 0; ch->dims[i].id; i++) {
        read[i] = perflibGetObjectCounter(pDataBlock, pObjectType, &ch->dims[i].cd);
        all_read = all_read && read[i];
        any_read = any_read || read[i];
    }

    // Create a chart only from a complete sample; afterwards, unread dimensions become gaps.
    if (!any_read || (!ch->st && !all_read))
        return;

    if (unlikely(!ch->st)) {
        ch->st = nps_chart_create(family, ch->id, ch->title, ch->units, ch->priority, ch->type, update_every);

        RRD_ALGORITHM algorithm = ch->units_total ? RRD_ALGORITHM_INCREMENTAL : RRD_ALGORITHM_ABSOLUTE;
        for (struct nps_dim *d = ch->dims; d->id; d++)
            d->rd = rrddim_add(ch->st, d->id, NULL, 1, 1, algorithm);

        if (ch->units_total) {
            char id[RRD_ID_LENGTH_MAX + 1];
            char title[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_total", ch->id);
            snprintfz(title, RRD_ID_LENGTH_MAX, "%s since NPS start", ch->title);
            ch->st_total =
                nps_chart_create(family, id, title, ch->units_total, ch->priority_total, ch->type, update_every);

            for (struct nps_dim *d = ch->dims; d->id; d++)
                d->rd_total = rrddim_add(ch->st_total, d->id, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
    }

    for (size_t i = 0; ch->dims[i].id; i++) {
        if (!read[i])
            continue;

        struct nps_dim *d = &ch->dims[i];
        collected_number value = (collected_number)d->cd.current.Data;
        rrddim_set_by_pointer(ch->st, d->rd, value);
        if (ch->st_total)
            rrddim_set_by_pointer(ch->st_total, d->rd_total, value);
    }

    rrdset_done(ch->st);
    if (ch->st_total)
        rrdset_done(ch->st_total);
}

static void nps_object_collect(struct nps_object *obj, int update_every)
{
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

    for (struct nps_chart *ch = obj->charts; ch->id; ch++)
        nps_chart_update(pDataBlock, pObjectType, obj->family, ch, update_every);
}

int do_PerflibNPS(int update_every, usec_t dt __maybe_unused)
{
    // Each object is read and fully processed before the next query reuses the Perflib buffer.
    for (size_t i = 0; i < _countof(nps_objects); i++)
        nps_object_collect(&nps_objects[i], update_every);

    // Never disable: a missing role or a failed read is reported as a gap and may recover.
    return 0;
}
