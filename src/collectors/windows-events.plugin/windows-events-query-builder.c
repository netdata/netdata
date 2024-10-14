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

static bool source_is_mine(LOGS_QUERY_SOURCE *src, LOGS_QUERY_STATUS *lqs) {
    if((lqs->rq.source_type == WEVTS_NONE && !lqs->rq.sources) || (src->source_type & lqs->rq.source_type) ||
        (lqs->rq.sources && simple_pattern_matches(lqs->rq.sources, string2str(src->source)))) {

        if(!src->msg_last_ut)
            // the file is not scanned yet, or the timestamps have not been updated,
            // so we don't know if it can contribute or not - let's add it.
            return true;

        usec_t anchor_delta = ANCHOR_DELTA_UT;
        usec_t first_ut = src->msg_first_ut - anchor_delta;
        usec_t last_ut = src->msg_last_ut + anchor_delta;

        if(last_ut >= lqs->rq.after_ut && first_ut <= lqs->rq.before_ut)
            return true;
    }

    return false;
}

static bool wevt_xpath_time_filter(LOGS_QUERY_STATUS *lqs, BUFFER *f) {
    usec_t seek_to = lqs->query.start_ut;
    if(lqs->rq.direction == FACETS_ANCHOR_DIRECTION_BACKWARD)
        // windows events queries are limited to millisecond resolution
        // so, in order not to lose data, we have to add
        // a millisecond when the direction is backward
        seek_to += USEC_PER_MS;

    // Convert the microseconds since Unix epoch to FILETIME (used in Windows APIs)
    FILETIME aT = os_unix_epoch_ut_to_filetime(seek_to);
    FILETIME bT = os_unix_epoch_ut_to_filetime(lqs->query.stop_ut);

    // Convert FILETIME to SYSTEMTIME for use in XPath
    SYSTEMTIME aTM;
    if (!FileTimeToSystemTime(&aT, &aTM)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "FileTimeToSystemTime() failed");
        return false;
    }

    SYSTEMTIME bTM;
    if (!FileTimeToSystemTime(&bT, &bTM)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "FileTimeToSystemTime() failed");
        return false;
    }

    // Format SYSTEMTIME into ISO 8601 format (YYYY-MM-DDTHH:MM:SS.sssZ)
    buffer_sprintf(f,
                   "TimeCreated["
                   "@SystemTime%s'%04d-%02d-%02dT%02d:%02d:%02d.%03dZ' and "
                   "@SystemTime%s'%04d-%02d-%02dT%02d:%02d:%02d.%03dZ'",
                   lqs->rq.direction == FACETS_ANCHOR_DIRECTION_BACKWARD ? "&lt;=" : "&gt;=",
                   aTM.wYear, aTM.wMonth, aTM.wDay, aTM.wHour, aTM.wMinute, aTM.wSecond, aTM.wMilliseconds,
                   lqs->rq.direction == FACETS_ANCHOR_DIRECTION_BACKWARD ? "&gt;=" : "&lt;=",
                   bTM.wYear, bTM.wMonth, bTM.wDay, bTM.wHour, bTM.wMinute, bTM.wSecond, bTM.wMilliseconds);

    return true;
}

static BUFFER *wevt_xpath_query_filter(LOGS_QUERY_STATUS *lqs) {
    BUFFER *f = buffer_create(8192, NULL);

    buffer_strcat(f, "*[System[");

    if(!wevt_xpath_time_filter(lqs, f)) {
        buffer_free(f);
        return NULL;
    }

    buffer_strcat(f, "]]");

    return f;
}

bool wevt_xpath_query_build(LOGS_QUERY_STATUS *lqs) {
    lqs->c.files_matched = 0;
    lqs->c.file_working = 0;
    lqs->c.rows_useful = 0;
    lqs->c.rows_read = 0;
    lqs->c.bytes_read = 0;

    CLEAN_BUFFER *f = wevt_xpath_query_filter(lqs);
    if(!f) return false;

    if(!lqs->c.query)
        lqs->c.query = buffer_create(8192, NULL);

    BUFFER *q = lqs->c.query;

    buffer_flush(q);
    buffer_strcat(q, "<QueryList><Query Id='0'>");

    bool added = 0;
    LOGS_QUERY_SOURCE *src;
    dfe_start_read(wevt_sources, src) {
        if(!source_is_mine(src, lqs))
            continue;

        buffer_strcat(q, "<Select Path='");
        buffer_strcat_xml(q, src->fullname);
        buffer_strcat(q, "'>");
        buffer_fast_strcat(q, buffer_tostring(f), buffer_strlen(f));
        buffer_strcat(q, "</Select>");

        lqs->c.progress.entries.total += src->entries;
        added++;
    }
    dfe_done(jf);

    buffer_strcat(q, "</Query></QueryList>");

    return added > 0;
}
