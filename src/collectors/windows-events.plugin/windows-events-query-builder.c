// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-query-builder.h"

// --------------------------------------------------------------------------------------------------------------------
// query without XPath

typedef struct static_utf8_8k {
    char buffer[8192];
    size_t size;
    size_t len;
} STATIC_BUF_8K;

typedef struct static_unicode_16k {
    wchar_t buffer[16384];
    size_t size;
    size_t len;
} STATIC_UNI_16K;

static bool wevt_foreach_selected_value_cb(FACETS *facets __maybe_unused, size_t id, const char *key, const char *value, void *data) {
    STATIC_BUF_8K *b = data;

    b->len += snprintfz(&b->buffer[b->len], b->size - b->len,
                        "%s%s=%s",
                        id ? " or " : "", key, value);

    return b->len < b->size;
}

wchar_t *wevt_generate_query_no_xpath(LOGS_QUERY_STATUS *lqs, BUFFER *wb) {
    static __thread STATIC_UNI_16K q = {
            .size = sizeof(q.buffer) / sizeof(wchar_t),
            .len = 0,
    };
    static __thread STATIC_BUF_8K b = {
            .size = sizeof(b.buffer) / sizeof(char),
            .len = 0,
    };

    lqs_query_timeframe(lqs, ANCHOR_DELTA_UT);

    usec_t seek_to = lqs->query.start_ut;
    if(lqs->rq.direction == FACETS_ANCHOR_DIRECTION_BACKWARD)
        // windows events queries are limited to millisecond resolution
        // so, in order not to lose data, we have to add
        // a millisecond when the direction is backward
        seek_to += USEC_PER_MS;

    // Convert the microseconds since Unix epoch to FILETIME (used in Windows APIs)
    FILETIME fileTime = os_unix_epoch_ut_to_filetime(seek_to);

    // Convert FILETIME to SYSTEMTIME for use in XPath
    SYSTEMTIME systemTime;
    if (!FileTimeToSystemTime(&fileTime, &systemTime)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "FileTimeToSystemTime() failed");
        return NULL;
    }

    // Format SYSTEMTIME into ISO 8601 format (YYYY-MM-DDTHH:MM:SS.sssZ)
    q.len = swprintf(q.buffer, q.size,
                     L"Event/System[TimeCreated[@SystemTime%ls\"%04d-%02d-%02dT%02d:%02d:%02d.%03dZ\"]",
                     lqs->rq.direction == FACETS_ANCHOR_DIRECTION_BACKWARD ? L"<=" : L">=",
                     systemTime.wYear, systemTime.wMonth, systemTime.wDay,
                     systemTime.wHour, systemTime.wMinute, systemTime.wSecond, systemTime.wMilliseconds);

    if(lqs->rq.slice) {
        b.len = snprintf(b.buffer, b.size, " and (");
        if (facets_foreach_selected_value_in_key(
                lqs->facets,
                WEVT_FIELD_LEVEL,
                sizeof(WEVT_FIELD_LEVEL) - 1,
                used_hashes_registry,
                wevt_foreach_selected_value_cb,
                &b)) {
            b.len += snprintf(&b.buffer[b.len], b.size - b.len, ")");
            if (b.len < b.size) {
                utf82unicode(&q.buffer[q.len], q.size - q.len, b.buffer);
                q.len = wcslen(q.buffer);
            }
        }

        b.len = snprintf(b.buffer, b.size, " and (");
        if (facets_foreach_selected_value_in_key(
                lqs->facets,
                WEVT_FIELD_EVENTID,
                sizeof(WEVT_FIELD_EVENTID) - 1,
                used_hashes_registry,
                wevt_foreach_selected_value_cb,
                &b)) {
            b.len += snprintf(&b.buffer[b.len], b.size - b.len, ")");
            if (b.len < b.size) {
                utf82unicode(&q.buffer[q.len], q.size - q.len, b.buffer);
                q.len = wcslen(q.buffer);
            }
        }
    }

    q.len += swprintf(&q.buffer[q.len], q.size - q.len, L"]");

    buffer_json_member_add_string(wb, "_query", channel2utf8(q.buffer));

    return q.buffer;
}

// --------------------------------------------------------------------------------------------------------------------
// query with XPath

