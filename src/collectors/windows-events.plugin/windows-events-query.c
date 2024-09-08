// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-query.h"

static uint64_t wevt_log_file_size(const wchar_t *channel);

#define VAR_PROVIDER_NAME(p)                ((p)[0].StringVal)
#define VAR_SOURCE_NAME(p)                  ((p)[1].StringVal)
#define VAR_PROVIDER_GUID(p)                ((p)[2].GuidVal)
#define VAR_RECORD_NUMBER(p)                ((p)[3].UInt64Val)
#define VAR_EVENT_ID(p)                     ((p)[4].UInt16Val)
#define VAR_LEVEL(p)                        ((p)[5].ByteVal)
#define VAR_KEYWORDS(p)                     ((p)[6].UInt64Val)
#define VAR_TIME_CREATED(p)                 ((p)[7].FileTimeVal)
#define VAR_COMPUTER_NAME(p)                ((p)[8].StringVal)
#define VAR_USER_ID(p)                      ((p)[9].SidVal)
#define VAR_CORRELATION_ACTIVITY_ID(p)      ((p)[10].GuidVal)
#define VAR_OPCODE(p)                       ((p)[11].UInt16Val)
#define	VAR_EVENT_DATA_STRING(p)		    ((p)[12].StringVal)
#define	VAR_EVENT_DATA_STRING_ARRAY(p, i)	((p)[12].StringArr[i])
#define	VAR_EVENT_DATA_TYPE(p)			    ((p)[12].Type)
#define	VAR_EVENT_DATA_COUNT(p)			    ((p)[12].Count)

// These are the fields we extract from the logs
static const wchar_t *RENDER_ITEMS[] = {
    L"/Event/System/Provider/@Name",
    L"/Event/System/Provider/@EventSourceName",
    L"/Event/System/Provider/@Guid",
    L"/Event/System/EventRecordID",
    L"/Event/System/EventID",
    L"/Event/System/Level",
    L"/Event/System/Keywords",
    L"/Event/System/TimeCreated/@SystemTime",
    L"/Event/System/Computer",
    L"/Event/System/Security/@UserID",
    L"/Event/System/Correlation/@ActivityID",
    L"/Event/System/Opcode",
    L"/Event/EventData/Data"
};

static void wevt_empty_utf8(TXT_UTF8 *dst) {
    txt_utf8_resize(dst, 1);
    dst->data[0] = '\0';
    dst->used = 1;
}

bool wevt_get_message_utf8(WEVT_LOG *log, EVT_HANDLE event_handle, TXT_UTF8 *dst, EVT_FORMAT_MESSAGE_FLAGS what) {
    DWORD size = 0;

    // First, try to get the message using the existing buffer
    if (!EvtFormatMessage(NULL, event_handle, 0, 0, NULL, what, log->ops.unicode.size, log->ops.unicode.data, &size)) {
        if (GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() failed.");
            goto cleanup;
        }

        // Try again with the resized buffer
        txt_unicode_resize(&log->ops.unicode, size);
        if (!EvtFormatMessage(NULL, event_handle, 0, 0, NULL, what, log->ops.unicode.size, log->ops.unicode.data, &size)) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() failed after resizing buffer.");
            goto cleanup;
        }
    }

    // EvtFormatMessage pads it with zeros at the end
    while(size >= 2 && log->ops.unicode.data[size - 2] == 0)
        size--;

    log->ops.unicode.used = size;

    internal_fatal(wcslen(log->ops.unicode.data) + 1 != (size_t)log->ops.unicode.used,
                   "Wrong unicode string length!");

    return wevt_str_unicode_to_utf8(dst, &log->ops.unicode);

cleanup:
    wevt_empty_utf8(dst);
    return false;
}

static bool wevt_GUID_to_ND_UUID(ND_UUID *nd_uuid, const GUID *guid) {
    if(guid && sizeof(GUID) == sizeof(ND_UUID)) {
        memcpy(nd_uuid->uuid, guid, sizeof(ND_UUID));
        return true;
    }
    else {
        *nd_uuid = UUID_ZERO;
        return false;
    }
}

bool wevt_get_next_event(WEVT_LOG *log, WEVT_EVENT *ev) {
    DWORD		size_required_next = 0, size = 0, bookmarkedCount = 0;
    EVT_HANDLE	tmp_event_bookmark = NULL;
    bool ret = false;

    fatal_assert(log && log->event_query && log->render_context);

    if (!EvtNext(log->event_query, 1, &tmp_event_bookmark, INFINITE, 0, &size_required_next))
        goto cleanup; // no data available, return failure

    // obtain the information from selected events
    if (!EvtRender(log->render_context, tmp_event_bookmark, EvtRenderEventValues, log->ops.content.size, log->ops.content.data, &size, &bookmarkedCount)) {
        // information exceeds the allocated space
        if (GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtRender() failed");
            goto cleanup;
        }

        freez(log->ops.content.data);
        log->ops.content.size = size;
        log->ops.content.data = (EVT_VARIANT *)mallocz(log->ops.content.size);

        if (!EvtRender(log->render_context, tmp_event_bookmark, EvtRenderEventValues, log->ops.content.size, log->ops.content.data, &size, &bookmarkedCount)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtRender() failed, after size increase");
            goto cleanup;
        }
    }
    log->ops.content.len = size;

    // LogName
    // it is the same as the channel

    // ProviderName
    wevt_str_wchar_to_utf8(&log->ops.provider, VAR_PROVIDER_NAME(log->ops.content.data), -1);

    // ProviderSourceName
    wevt_str_wchar_to_utf8(&log->ops.source, VAR_SOURCE_NAME(log->ops.content.data), -1);

    // ProviderGUID
    // we keep this in case we need to cache EventIDs, Keywords, Opcodes, per provider
    wevt_GUID_to_ND_UUID(&ev->provider, VAR_PROVIDER_GUID(log->ops.content.data));

    // EventRecordID
    // This is indexed and can be queried with slicing - but not consistent across channels
    ev->id = VAR_RECORD_NUMBER(log->ops.content.data);

    // EventID (it defines the template for formatting the message)
    // This is indexed and can be queried with slicing - but not consistent across channels
    ev->event_id = VAR_EVENT_ID(log->ops.content.data);
    if(ev->event_id)
        wevt_get_message_utf8(log, tmp_event_bookmark, &log->ops.event, EvtFormatMessageEvent);
    else
        wevt_empty_utf8(&log->ops.event);

    // Level (the severity / priority)
    // This is indexed and can be queried with slicing - probably consistent across channels
    ev->level = VAR_LEVEL(log->ops.content.data);
    wevt_get_message_utf8(log, tmp_event_bookmark, &log->ops.level, EvtFormatMessageLevel);

    // Keywords (categorization of events)
    // This is indexed and can be queried with slicing - but not consistent across channels
    ev->keyword = VAR_KEYWORDS(log->ops.content.data);
    if(ev->keyword)
        wevt_get_message_utf8(log, tmp_event_bookmark, &log->ops.keyword, EvtFormatMessageKeyword);
    else
        wevt_empty_utf8(&log->ops.keyword);

    // TimeCreated
    // This is indexed and can be queried with slicing - it is consistent across channels
    ev->created_ns = os_windows_ulonglong_to_unix_epoch_ns(VAR_TIME_CREATED(log->ops.content.data));

    // Opcode
    // Not indexed
    ev->opcode = VAR_OPCODE(log->ops.content.data);
    if(ev->opcode)
        wevt_get_message_utf8(log, tmp_event_bookmark, &log->ops.opcode, EvtFormatMessageOpcode);
    else
        wevt_empty_utf8(&log->ops.keyword);

    // ComputerName
    wevt_str_wchar_to_utf8(&log->ops.computer, VAR_COMPUTER_NAME(log->ops.content.data), -1);

    // User
    wevt_convert_user_id_to_name(log, VAR_USER_ID(log->ops.content.data));

    // CorrelationActivityID
    wevt_GUID_to_ND_UUID(&ev->correlation_activity_id, VAR_CORRELATION_ACTIVITY_ID(log->ops.content.data));

    ret = true;

cleanup:
    if (tmp_event_bookmark)
        EvtClose(tmp_event_bookmark);

    return ret;
}

void wevt_closelog6(WEVT_LOG *log) {
    if (log->event_query)
        EvtClose(log->event_query);

    if (log->render_context)
        EvtClose(log->render_context);

    freez(log->ops.content.data);
    txt_unicode_cleanup(&log->ops.unicode);
    txt_utf8_cleanup(&log->ops.event);
    txt_utf8_cleanup(&log->ops.provider);
    txt_utf8_cleanup(&log->ops.source);
    txt_utf8_cleanup(&log->ops.computer);
    txt_utf8_cleanup(&log->ops.user);
    txt_utf8_cleanup(&log->ops.opcode);
    txt_utf8_cleanup(&log->ops.level);
    txt_utf8_cleanup(&log->ops.keyword);
    freez(log);
}

WEVT_LOG *wevt_openlog6(const wchar_t *channel, bool file_size) {
    size_t RENDER_ITEMS_count = (sizeof(RENDER_ITEMS) / sizeof(const wchar_t *));
    bool ret = false;

    WEVT_LOG *log = callocz(1, sizeof(*log));

    // get the number of the oldest record in the log
    // "EvtGetLogInfo()" does not work properly with "EvtLogOldestRecordNumber"
    // we have to get it from the first EventRecordID

    // create the system render
    log->render_context = EvtCreateRenderContext(RENDER_ITEMS_count, RENDER_ITEMS, EvtRenderContextValues);
    if (!log->render_context) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtCreateRenderContext failed.");
        goto cleanup;
    }

    // query the eventlog
    log->event_query = EvtQuery(NULL, channel, NULL, EvtQueryChannelPath);
    if (!log->event_query) {
        if (GetLastError() == ERROR_EVT_CHANNEL_NOT_FOUND)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() failed, channel '%s' not found, cannot open log", channel2utf8(channel));
        else
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() on channel '%s' failed, cannot open log", channel2utf8(channel));

        goto cleanup;
    }

    if (!wevt_get_next_event(log, &log->retention.first_event))
        goto cleanup;

    if (!log->retention.first_event.id) {
        // no data in the event log
        log->retention.first_event = log->retention.last_event = WEVT_EVENT_EMPTY;
        ret = true;
        goto cleanup;
    }
    EvtClose(log->event_query);

    log->event_query = EvtQuery(NULL, channel, NULL, EvtQueryChannelPath | EvtQueryReverseDirection);
    if (!log->event_query) {
        if (GetLastError() == ERROR_EVT_CHANNEL_NOT_FOUND)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() failed, channel '%s' not found", channel2utf8(channel));
        else
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() on channel '%s' failed", channel2utf8(channel));

        goto cleanup;
    }

    if (!wevt_get_next_event(log, &log->retention.last_event) || log->retention.last_event.id == 0) {
        // no data in eventlog
        log->retention.last_event = log->retention.first_event;
    }
    log->retention.last_event.id += 1;	// we should read the last record
    ret = true;

cleanup:
    if(log->event_query) {
        EvtClose(log->event_query);
        log->event_query = NULL;
    }

    if(ret) {
        log->retention.entries = log->retention.last_event.id - log->retention.first_event.id;

        if(log->retention.last_event.created_ns >= log->retention.first_event.created_ns)
            log->retention.duration_ns = log->retention.last_event.created_ns - log->retention.first_event.created_ns;
        else
            log->retention.duration_ns = log->retention.first_event.created_ns - log->retention.last_event.created_ns;

        if(file_size)
            log->retention.size_bytes = wevt_log_file_size(channel);

        return log;
    }
    else {
        wevt_closelog6(log);
        return NULL;
    }
}

static uint64_t wevt_log_file_size(const wchar_t *channel) {
    EVT_HANDLE hLog = NULL;
    EVT_VARIANT evtVariant;
    DWORD bufferUsed = 0;
    uint64_t file_size = 0;

    // Open the event log channel
    hLog = EvtOpenLog(NULL, channel, EvtOpenChannelPath);
    if (!hLog) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtOpenLog() on channel '%s' failed", channel2utf8(channel));
        goto cleanup;
    }

    // Get the file size of the log
    if (!EvtGetLogInfo(hLog, EvtLogFileSize, sizeof(evtVariant), &evtVariant, &bufferUsed)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtGetLogInfo() on channel '%s' failed", channel2utf8(channel));
        goto cleanup;
    }

    // Extract the file size from the EVT_VARIANT structure
    file_size = evtVariant.UInt64Val;

cleanup:
    if (hLog)
        EvtClose(hLog);

    return file_size;
}

EVT_HANDLE wevt_query(LPCWSTR channel, usec_t seek_to, bool backward) {
    // Convert microseconds to nanoseconds first (correct handling of precision)
    if(backward) seek_to += USEC_PER_MS;  // for backward mode, add a millisecond to the seek time.

    // Convert the microseconds since Unix epoch to FILETIME (used in Windows APIs)
    FILETIME fileTime = os_unix_epoch_ut_to_filetime(seek_to);

    // Convert FILETIME to SYSTEMTIME for use in XPath
    SYSTEMTIME systemTime;
    if (!FileTimeToSystemTime(&fileTime, &systemTime)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "FileTimeToSystemTime() failed");
        return NULL;
    }

    // Format SYSTEMTIME into ISO 8601 format (YYYY-MM-DDTHH:MM:SS.sssZ)
    static __thread WCHAR query[4096];
    swprintf(query, 4096,
             L"Event/System[TimeCreated[@SystemTime%ls\"%04d-%02d-%02dT%02d:%02d:%02d.%03dZ\"]]",
             backward ? L"<=" : L">=",  // Use <= if backward, >= if forward
             systemTime.wYear, systemTime.wMonth, systemTime.wDay,
             systemTime.wHour, systemTime.wMinute, systemTime.wSecond, systemTime.wMilliseconds);

    // Execute the query
    EVT_HANDLE hQuery = EvtQuery(NULL, channel, query, EvtQueryChannelPath | (backward ? EvtQueryReverseDirection : EvtQueryForwardDirection));
    if (!hQuery) {
        wchar_t wbuf[1024];
        DWORD wbuf_used;
        EvtGetExtendedStatus(sizeof(wbuf), wbuf, &wbuf_used);

        char buf[1024];
        rfc3339_datetime_ut(buf, sizeof(buf), seek_to, 3, true);
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() failed, seek to '%s', query: %s | extended info: %ls", buf, query2utf8(query), wbuf);
    }

    return hQuery;
}
