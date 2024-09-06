// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-query.h"

static uint64_t wevt_log_file_size(const wchar_t *channel);

#define VAR_PROVIDER_NAME(p)                ((p)[0].StringVal)
#define VAR_SOURCE_NAME(p)                  ((p)[1].StringVal)
#define VAR_RECORD_NUMBER(p)                ((p)[2].UInt64Val)
#define VAR_EVENT_ID(p)                     ((p)[3].UInt16Val)
#define VAR_LEVEL(p)                        ((p)[4].ByteVal)
#define VAR_KEYWORDS(p)                     ((p)[5].UInt64Val)
#define VAR_TIME_CREATED(p)                 ((p)[6].FileTimeVal)
#define VAR_COMPUTER_NAME(p)                ((p)[7].StringVal)
#define VAR_USER_ID(p)                      ((p)[8].StringVal)
#define VAR_CORRELATION_ACTIVITY_ID(p)      ((p)[9].GuidVal)
#define	VAR_EVENT_DATA_STRING(p)		    ((p)[10].StringVal)
#define	VAR_EVENT_DATA_STRING_ARRAY(p, i)	((p)[10].StringArr[i])
#define	VAR_EVENT_DATA_TYPE(p)			    ((p)[10].Type)
#define	VAR_EVENT_DATA_COUNT(p)			    ((p)[10].Count)

bool wevt_str_wchar_to_utf8(TXT_UTF8 *utf8, const wchar_t *src, int src_len) {
    if(!src || !src_len) goto cleanup;

    // Try to convert using the existing buffer (if it exists, otherwise get the required buffer size)
    int size = WideCharToMultiByte(CP_UTF8, 0, src, src_len, utf8->data, (int)utf8->size, NULL, NULL);
    if(size <= 0 || !utf8->data) {
        // we have to set a buffer, or increase it

        if(utf8->data) {
            // we need to increase it the buffer size

            if(GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
                nd_log(NDLS_COLLECTORS, NDLP_ERR, "WideCharToMultiByte() failed.");
                goto cleanup;
            }

            // we have to find the required buffer size
            size = WideCharToMultiByte(CP_UTF8, 0, src, src_len, NULL, 0, NULL, NULL);
            if(size <= 0) goto cleanup;
        }

        // Retry conversion with the new buffer
        txt_utf8_resize(utf8, size);
        size = WideCharToMultiByte(CP_UTF8, 0, src, src_len, utf8->data, (int)utf8->size, NULL, NULL);
        if (size <= 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "WideCharToMultiByte() failed after resizing.");
            goto cleanup;
        }
    }

    utf8->len = (size_t)size;
    return true;

    cleanup:
    txt_utf8_resize(utf8, 128);
    utf8->len = snprintfz(utf8->data, utf8->size, "[failed to convert UNICODE message to UTF8]") + 1;
    return false;
}

bool wevt_str_unicode_to_utf8(TXT_UTF8 *utf8, TXT_UNICODE *unicode) {
    assert(utf8 && ((utf8->data && utf8->size) || (!utf8->data && !utf8->size)));
    assert(unicode && ((unicode->data && unicode->size) || (!unicode->data && !unicode->size)));
    return wevt_str_wchar_to_utf8(utf8, unicode->data, (int)unicode->len);
}

bool wevt_get_message_utf8(WEVT_LOG *log, EVT_HANDLE event_handle) {
    DWORD size = 0;

    // First, try to get the message using the existing buffer
    if (!EvtFormatMessage(NULL, event_handle, 0, 0, NULL, EvtFormatMessageEvent, log->ops.unicode.size, log->ops.unicode.data, &size)) {
        if (GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() failed.");
            goto cleanup;
        }

        // Try again with the resized buffer
        txt_unicode_resize(&log->ops.unicode, size);
        if (!EvtFormatMessage(NULL, event_handle, 0, 0, NULL, EvtFormatMessageEvent, log->ops.unicode.size, log->ops.unicode.data, &size)) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() failed after resizing buffer.");
            goto cleanup;
        }
    }

    log->ops.unicode.len = size;
    return wevt_str_unicode_to_utf8(&log->ops.message, &log->ops.unicode);

cleanup:
    txt_utf8_resize(&log->ops.message, 128);
    log->ops.message.len = snprintfz(log->ops.message.data, log->ops.message.size, "[failed to get UNICODE message for this event]") + 1;
    return false;
}

static bool wevt_get_next_event(WEVT_LOG *log, WEVT_EVENT *ev) {
    DWORD		size_required_next = 0, size = 0, bookmarkedCount = 0;
    EVT_HANDLE	tmp_event_bookmark = NULL;
    bool ret = false;

    assert(log && log->event_query && log->render_context);

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

    ev->id = VAR_RECORD_NUMBER(log->ops.content.data);
    ev->created_ns = os_windows_ulonglong_to_unix_epoch_ns(VAR_TIME_CREATED(log->ops.content.data));
    wevt_get_message_utf8(log, tmp_event_bookmark);
    wevt_str_wchar_to_utf8(&log->ops.provider, VAR_PROVIDER_NAME(log->ops.content.data), -1);
    wevt_str_wchar_to_utf8(&log->ops.source, VAR_SOURCE_NAME(log->ops.content.data), -1);
    wevt_str_wchar_to_utf8(&log->ops.computer, VAR_COMPUTER_NAME(log->ops.content.data), -1);
    wevt_str_wchar_to_utf8(&log->ops.user, VAR_USER_ID(log->ops.content.data), -1);
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
    txt_utf8_cleanup(&log->ops.message);
    txt_utf8_cleanup(&log->ops.provider);
    txt_utf8_cleanup(&log->ops.source);
    txt_utf8_cleanup(&log->ops.computer);
    txt_utf8_cleanup(&log->ops.user);
    freez(log);
}

WEVT_LOG *wevt_openlog6(const wchar_t *channel, bool file_size) {
    static const wchar_t *RENDER_ITEMS[] = {
            L"/Event/System/Provider/@Name",
            L"/Event/System/Provider/@EventSourceName",
            L"/Event/System/EventRecordID",
            L"/Event/System/EventID",
            L"/Event/System/Level",
            L"/Event/System/Keywords",
            L"/Event/System/TimeCreated/@SystemTime",
            L"/Event/System/Computer",
            L"/Event/System/Security/@UserID",
            L"/Event/System/Correlation/@ActivityID",
            L"/Event/EventData/Data"
    };

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
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() failed, channel '%s' not found", channel2utf8(channel));
        else
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() on channel '%s' failed", channel2utf8(channel));

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
