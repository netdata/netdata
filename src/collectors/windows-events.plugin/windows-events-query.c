// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-query.h"

static uint64_t wevt_log_file_size(const wchar_t *channel);

#define	VAR_PROVIDER_NAME(p)			    ((p)[0].StringVal)
#define	VAR_SOURCE_NAME(p)			        ((p)[1].StringVal)
#define	VAR_RECORD_NUMBER(p)			    ((p)[2].UInt64Val)
#define	VAR_EVENT_ID(p)				        ((p)[3].UInt16Val)
#define	VAR_LEVEL(p)				        ((p)[4].ByteVal)
#define	VAR_KEYWORDS(p)				        ((p)[5].UInt64Val)
#define	VAR_TIME_CREATED(p)			        ((p)[6].FileTimeVal)
#define	VAR_EVENT_DATA_STRING(p)		    ((p)[7].StringVal)
#define	VAR_EVENT_DATA_STRING_ARRAY(p, i)	((p)[7].StringArr[i])
#define	VAR_EVENT_DATA_TYPE(p)			    ((p)[7].Type)
#define	VAR_EVENT_DATA_COUNT(p)			    ((p)[7].Count)

static bool wevt_get_next_event(WEVT_LOG *log, WEVT_EVENT *ev) {
    DWORD		size_required_next = 0, size_required = 0, size = 0, bookmarkedCount = 0;
    EVT_HANDLE	tmp_event_bookmark = NULL;
    bool ret = false;

    assert(log && log->event_query && log->render_context);

    if (!EvtNext(log->event_query, 1, &tmp_event_bookmark, INFINITE, 0, &size_required_next))
        goto cleanup; // no data available, return failure

    // obtain the information from selected events
    if (!EvtRender(log->render_context, tmp_event_bookmark, EvtRenderEventValues, size, log->ops.renderedContent, &size_required, &bookmarkedCount)) {
        // information exceeds the allocated space
        if (ERROR_INSUFFICIENT_BUFFER != GetLastError()) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtRender() failed");
            goto cleanup;
        }

        size = size_required;
        freez(log->ops.renderedContent);
        log->ops.renderedContent = (EVT_VARIANT *)mallocz(size);

        if (!EvtRender(log->render_context, tmp_event_bookmark, EvtRenderEventValues, size, log->ops.renderedContent, &size_required, &bookmarkedCount)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtRender() failed, after size increase");
            goto cleanup;
        }
    }

    ev->id = VAR_RECORD_NUMBER(log->ops.renderedContent);
    ev->created_ns = os_windows_ulonglong_to_unix_epoch_ns(VAR_TIME_CREATED(log->ops.renderedContent));
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

    if (log->ops.renderedContent)
        freez(log->ops.renderedContent);

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
