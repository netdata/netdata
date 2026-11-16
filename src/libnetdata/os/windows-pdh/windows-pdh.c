// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-pdh.h"

#if defined(OS_WINDOWS)
#include <pdh.h>
#include <pdhmsg.h>

// PdhGetRawCounterArrayW can write past the size it reports (the instance-name terminator of single-instance
// objects on Windows Server 2012 R2), so every buffer gets this much unreported room.
#define ND_PDH_BUFFER_SLACK 8

#define ND_PDH_PATH_MAX 1024

// Buffer resizes per counter read before the read is reported as failed.
#define ND_PDH_READ_ATTEMPTS 4

struct nd_pdh_counter {
    PDH_HCOUNTER handle; // NULL when the counter could not be added
    DWORD type;
    LONGLONG frequency;
};

struct nd_pdh_query {
    PDH_HQUERY handle;
    char *object;
    const char *const *names;
    size_t counters_count;
    struct nd_pdh_counter *counters;

    BYTE *buffer;
    DWORD buffer_size; // the size reported to PDH, without the slack
};

static DWORD pdh_counter_type(PDH_HCOUNTER counter)
{
    DWORD size = 0;
    if ((DWORD)PdhGetCounterInfoW(counter, FALSE, &size, NULL) != PDH_MORE_DATA || !size)
        return 0;

    PDH_COUNTER_INFO_W *info = mallocz(size);
    DWORD type = 0;
    if ((DWORD)PdhGetCounterInfoW(counter, FALSE, &size, info) == ERROR_SUCCESS)
        type = info->dwType;

    freez(info);
    return type;
}

static bool pdh_add_counter(ND_PDH_QUERY *q, size_t c)
{
    char path[ND_PDH_PATH_MAX];
    snprintfz(path, sizeof(path), "\\%s(*)\\%s", q->object, q->names[c]);

    wchar_t wpath[ND_PDH_PATH_MAX];
    bool truncated = false;
    if (!any_to_utf16(CP_UTF8, wpath, _countof(wpath), path, -1, &truncated) || truncated)
        return false;

    struct nd_pdh_counter *pc = &q->counters[c];
    DWORD status = (DWORD)PdhAddEnglishCounterW(q->handle, wpath, 0, &pc->handle);
    if (status != ERROR_SUCCESS) {
        pc->handle = NULL;
        // Callers retry while the object is unavailable, so this is limited.
        nd_log_limit_static_global_var(erl, 3600, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_NOTICE,
                     "WINDOWS: PDH: cannot add counter '%s' (status 0x%08lx)", path, (unsigned long)status);
        return false;
    }

    // Without a type the value cannot be charted; treat the counter as missing.
    pc->type = pdh_counter_type(pc->handle);
    if (!pc->type) {
        PdhRemoveCounter(pc->handle);
        pc->handle = NULL;
        nd_log_limit_static_global_var(erl, 3600, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_NOTICE, "WINDOWS: PDH: cannot get the type of counter '%s'", path);
        return false;
    }

    // Only some counter types have a time base; the others leave it at 0.
    if ((DWORD)PdhGetCounterTimeBase(pc->handle, &pc->frequency) != ERROR_SUCCESS)
        pc->frequency = 0;

    return true;
}

ND_PDH_QUERY *nd_pdh_query_create(const char *object, const char *const *counters, size_t counters_count)
{
    if (!object || !*object || !counters || !counters_count)
        return NULL;

    PDH_HQUERY handle = NULL;
    DWORD status = (DWORD)PdhOpenQueryW(NULL, 0, &handle);
    if (status != ERROR_SUCCESS) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "WINDOWS: PDH: cannot open a query for '%s' (status 0x%08lx)", object, (unsigned long)status);
        return NULL;
    }

    ND_PDH_QUERY *q = callocz(1, sizeof(*q));
    q->handle = handle;
    q->object = strdupz(object);
    q->names = counters;
    q->counters_count = counters_count;
    q->counters = callocz(counters_count, sizeof(*q->counters));

    size_t added = 0;
    for (size_t c = 0; c < counters_count; c++) {
        if (counters[c] && pdh_add_counter(q, c))
            added++;
    }

    if (!added) {
        nd_pdh_query_destroy(q);
        return NULL;
    }

    return q;
}

void nd_pdh_query_destroy(ND_PDH_QUERY *q)
{
    if (!q)
        return;

    // Closing the query also closes its counters.
    if (q->handle)
        PdhCloseQuery(q->handle);

    freez(q->counters);
    freez(q->object);
    freez(q->buffer);
    freez(q);
}

bool nd_pdh_query_has_counter(ND_PDH_QUERY *q, size_t counter)
{
    return q && counter < q->counters_count && q->counters[counter].handle;
}

bool nd_pdh_query_collect(ND_PDH_QUERY *q)
{
    if (!q)
        return false;

    DWORD status = (DWORD)PdhCollectQueryData(q->handle);

    // PDH_NO_DATA: no counter has an instance now; each read reports it.
    if (status == ERROR_SUCCESS || status == PDH_NO_DATA)
        return true;

    nd_log_limit_static_global_var(erl, 60, 0);
    nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
                 "WINDOWS: PDH: cannot collect '%s' (status 0x%08lx)", q->object, (unsigned long)status);
    return false;
}

// Statuses that mean the counter has no valid data in this sample, not that reading failed.
static bool pdh_status_is_no_data(DWORD status)
{
    switch (status) {
        case PDH_NO_DATA:
        case PDH_INVALID_DATA:
        case PDH_CSTATUS_INVALID_DATA:
        case PDH_CSTATUS_NO_INSTANCE:
        case PDH_CALC_NEGATIVE_DENOMINATOR:
        case PDH_CALC_NEGATIVE_VALUE:
            return true;

        default:
            return false;
    }
}

ND_PDH_READ_STATUS nd_pdh_query_read(ND_PDH_QUERY *q, size_t counter, nd_pdh_instance_cb cb, void *data)
{
    if (!nd_pdh_query_has_counter(q, counter) || !cb)
        return ND_PDH_READ_FAILED;

    struct nd_pdh_counter *pc = &q->counters[counter];
    DWORD items = 0;
    DWORD status;

    for (int attempt = 0;; attempt++) {
        DWORD size = q->buffer_size;

        // Until a size is known, pass no buffer: PDH writes 8 bytes into any non-NULL buffer, whatever its size.
        status = (DWORD)PdhGetRawCounterArrayW(
            pc->handle, &size, &items, q->buffer_size ? (PPDH_RAW_COUNTER_ITEM_W)q->buffer : NULL);

        if (status == ERROR_SUCCESS)
            break;

        // Instances can appear between calls, so the buffer may need to grow more than once; the bound stops a
        // provider that never settles.
        if ((status == PDH_MORE_DATA || status == PDH_INSUFFICIENT_BUFFER) && attempt < ND_PDH_READ_ATTEMPTS) {
            // The size PDH returns alongside a too-small buffer is not reliable; ask again without a buffer.
            if (q->buffer_size) {
                size = 0;
                DWORD probe_items = 0;
                status = (DWORD)PdhGetRawCounterArrayW(pc->handle, &size, &probe_items, NULL);

                // Success without a buffer means no instance is left.
                if (status == ERROR_SUCCESS)
                    return ND_PDH_READ_NO_DATA;
            }

            if (status == PDH_MORE_DATA || status == PDH_INSUFFICIENT_BUFFER) {
                // A size that did not grow would only repeat the failure.
                if (size <= q->buffer_size)
                    size = q->buffer_size * 2;

                if (size) {
                    freez(q->buffer);
                    q->buffer = mallocz((size_t)size + ND_PDH_BUFFER_SLACK);
                    q->buffer_size = size;
                    continue;
                }
            }
        }

        if (pdh_status_is_no_data(status))
            return ND_PDH_READ_NO_DATA;

        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
                     "WINDOWS: PDH: cannot read counter '\\%s(*)\\%s' (status 0x%08lx)",
                     q->object, q->names[counter], (unsigned long)status);
        return ND_PDH_READ_FAILED;
    }

    if (!items || !q->buffer)
        return ND_PDH_READ_NO_DATA;

    PDH_RAW_COUNTER_ITEM_W *item = (PDH_RAW_COUNTER_ITEM_W *)q->buffer;
    for (DWORD i = 0; i < items; i++) {
        char name[PERFLIB_MAX_NAME_LENGTH];
        bool truncated = false;
        const char *instance = NULL;

        // A truncated name could collide with another instance, so it is reported as undecodable.
        if (item[i].szName && utf16_to_utf8(name, sizeof(name), item[i].szName, -1, &truncated) && !truncated)
            instance = name;

        DWORD cstatus = item[i].RawValue.CStatus;
        if (cstatus != PDH_CSTATUS_VALID_DATA && cstatus != PDH_CSTATUS_NEW_DATA) {
            cb(instance, NULL, data);
            continue;
        }

        // PDH's second value is the time or the base value, as in RAW_DATA.
        RAW_DATA raw = {
            .CounterType = pc->type,
            .MultiCounterData = item[i].RawValue.MultiCount,
            .Data = (ULONGLONG)item[i].RawValue.FirstValue,
            .Time = item[i].RawValue.SecondValue,
            .Frequency = pc->frequency,
        };

        cb(instance, &raw, data);
    }

    return ND_PDH_READ_OK;
}

#endif // OS_WINDOWS
