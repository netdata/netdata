// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-query.h"

static uint64_t wevt_log_file_size(const wchar_t *channel);

#define FIELD_CHANNEL                       (0)
#define FIELD_PROVIDER_NAME                 (1)
#define FIELD_EVENT_SOURCE_NAME             (2)
#define FIELD_PROVIDER_GUID                 (3)
#define FIELD_RECORD_NUMBER                 (4)
#define FIELD_EVENT_ID                      (5)
#define FIELD_LEVEL                         (6)
#define FIELD_KEYWORDS                      (7)
#define FIELD_TIME_CREATED                  (8)
#define FIELD_COMPUTER_NAME                 (9)
#define FIELD_USER_ID                       (10)
#define FIELD_CORRELATION_ACTIVITY_ID       (11)
#define FIELD_OPCODE                        (12)
#define FIELD_VERSION                       (13)
#define FIELD_TASK                          (14)
#define FIELD_PROCESS_ID                    (15)
#define FIELD_THREAD_ID                     (16)

// These are the fields we extract from the logs
static const wchar_t *RENDER_ITEMS[] = {
        L"/Event/System/Channel",
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
        L"/Event/System/Version",
        L"/Event/System/Task",
        L"/Event/System/Execution/@ProcessID",
        L"/Event/System/Execution/@ThreadID",
};

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

static uint64_t wevt_get_unsigned_by_type(WEVT_LOG *log, size_t field) {
    switch(log->ops.content.data[field].Type & EVT_VARIANT_TYPE_MASK) {
        case EvtVarTypeHexInt64: return log->ops.content.data[field].UInt64Val;
        case EvtVarTypeHexInt32: return log->ops.content.data[field].UInt32Val;
        case EvtVarTypeUInt64: return log->ops.content.data[field].UInt64Val;
        case EvtVarTypeUInt32: return log->ops.content.data[field].UInt32Val;
        case EvtVarTypeUInt16: return log->ops.content.data[field].UInt16Val;
        case EvtVarTypeInt64: return ABS(log->ops.content.data[field].Int64Val);
        case EvtVarTypeInt32: return ABS(log->ops.content.data[field].Int32Val);
        case EvtVarTypeByte:  return log->ops.content.data[field].ByteVal;
        case EvtVarTypeInt16:  return ABS(log->ops.content.data[field].Int16Val);
        case EvtVarTypeSByte: return ABS(log->ops.content.data[field].SByteVal);
        case EvtVarTypeSingle: return ABS(log->ops.content.data[field].SingleVal);
        case EvtVarTypeDouble: return ABS(log->ops.content.data[field].DoubleVal);
        case EvtVarTypeBoolean: return log->ops.content.data[field].BooleanVal ? 1 : 0;
        case EvtVarTypeSizeT: return log->ops.content.data[field].SizeTVal;
        default: return 0;
    }
}

static uint64_t wevt_get_filetime_to_ns_by_type(WEVT_LOG *log, size_t field) {
    switch(log->ops.content.data[field].Type & EVT_VARIANT_TYPE_MASK) {
        case EvtVarTypeFileTime:
        case EvtVarTypeUInt64:
            return os_windows_ulonglong_to_unix_epoch_ns(log->ops.content.data[field].FileTimeVal);

        default: return 0;
    }
}

static bool wevt_get_uuid_by_type(WEVT_LOG *log, size_t field, ND_UUID *dst) {
    switch(log->ops.content.data[field].Type & EVT_VARIANT_TYPE_MASK) {
        case EvtVarTypeGuid:
            return wevt_GUID_to_ND_UUID(dst, log->ops.content.data[field].GuidVal);

        default:
            return wevt_GUID_to_ND_UUID(dst, NULL);
    }
}

static void wevt_empty_utf8(TXT_UTF8 *dst) {
    txt_utf8_resize(dst, 1);
    dst->data[0] = '\0';
    dst->used = 1;
}

static bool wevt_get_message_utf8(WEVT_LOG *log, EVT_HANDLE hMetadata, EVT_HANDLE bookmark, TXT_UTF8 *dst, EVT_FORMAT_MESSAGE_FLAGS what) {
    DWORD size = 0;

    if(!log->ops.unicode.data) {
        EvtFormatMessage(hMetadata, bookmark, 0, 0, NULL, what, 0, NULL, &size);
        if(!size) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() to get message size failed.");
            goto cleanup;
        }
        txt_unicode_resize(&log->ops.unicode, size);
    }

    // First, try to get the message using the existing buffer
    if (!EvtFormatMessage(hMetadata, bookmark, 0, 0, NULL, what, log->ops.unicode.size, log->ops.unicode.data, &size) || !log->ops.unicode.data) {
        if (log->ops.unicode.data && GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() failed.");
            goto cleanup;
        }

        // Try again with the resized buffer
        txt_unicode_resize(&log->ops.unicode, size);
        if (!EvtFormatMessage(hMetadata, bookmark, 0, 0, NULL, what, log->ops.unicode.size, log->ops.unicode.data, &size)) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() failed after resizing buffer.");
            goto cleanup;
        }
    }

    // make sure it is null terminated
    if(size <= log->ops.unicode.size)
        log->ops.unicode.data[size - 1] = 0;
    else
        log->ops.unicode.data[log->ops.unicode.size - 1] = 0;

    // unfortunately we have to calculate the length every time
    // the size returned may not be the length of the unicode string
    log->ops.unicode.used = wcslen(log->ops.unicode.data) + 1;

    return wevt_str_unicode_to_utf8(dst, &log->ops.unicode);

cleanup:
    wevt_empty_utf8(dst);
    return false;
}

static bool wevt_get_event_utf8(WEVT_LOG *log, EVT_HANDLE hMetadata, EVT_HANDLE event_handle, TXT_UTF8 *dst) {
    return wevt_get_message_utf8(log, hMetadata, event_handle, dst, EvtFormatMessageEvent);
}

static bool wevt_get_level_utf8(WEVT_LOG *log, EVT_HANDLE hMetadata, EVT_HANDLE event_handle, TXT_UTF8 *dst) {
    return wevt_get_message_utf8(log, hMetadata, event_handle, dst, EvtFormatMessageLevel);
}

static bool wevt_get_task_utf8(WEVT_LOG *log, EVT_HANDLE hMetadata, EVT_HANDLE event_handle, TXT_UTF8 *dst) {
    return wevt_get_message_utf8(log, hMetadata, event_handle, dst, EvtFormatMessageTask);
}

static bool wevt_get_opcode_utf8(WEVT_LOG *log, EVT_HANDLE hMetadata, EVT_HANDLE event_handle, TXT_UTF8 *dst) {
    return wevt_get_message_utf8(log, hMetadata, event_handle, dst, EvtFormatMessageOpcode);
}

static bool wevt_get_keywords_utf8(WEVT_LOG *log, EVT_HANDLE hMetadata, EVT_HANDLE event_handle, TXT_UTF8 *dst) {
    return wevt_get_message_utf8(log, hMetadata, event_handle, dst, EvtFormatMessageKeyword);
}

static bool wevt_get_xml_utf8(WEVT_LOG *log, EVT_HANDLE hMetadata, EVT_HANDLE bookmark, TXT_UTF8 *dst) {
    return wevt_get_message_utf8(log, hMetadata, bookmark, dst, EvtFormatMessageXml);
}

static bool wevt_get_utf8_by_type(WEVT_LOG *log, size_t field, TXT_UTF8 *dst) {
    switch(log->ops.content.data[field].Type & EVT_VARIANT_TYPE_MASK) {
        case EvtVarTypeString:
            return wevt_str_wchar_to_utf8(dst, log->ops.content.data[field].StringVal, -1);

        default:
            wevt_empty_utf8(dst);
            return false;
    }
}

static bool wevt_get_sid_by_type(WEVT_LOG *log, size_t field, TXT_UTF8 *dst) {
    switch(log->ops.content.data[field].Type & EVT_VARIANT_TYPE_MASK) {
        case EvtVarTypeSid:
            return wevt_convert_user_id_to_name(log->ops.content.data[field].SidVal, dst);

        default:
            wevt_empty_utf8(dst);
            return false;
    }
}

static inline void wevt_event_done(WEVT_LOG *log) {
    if (log->publisher) {
        publisher_release(log->publisher);
        log->publisher = NULL;
    }

    if (log->bookmark) {
        EvtClose(log->bookmark);
        log->bookmark = NULL;
    }
}

bool wevt_get_next_event(WEVT_LOG *log, WEVT_EVENT *ev, bool full) {
    DWORD returned = 0, bytes_used = 0, property_count = 0;
    bool ret = false;

    fatal_assert(log && log->event_query && log->render_context);

    wevt_event_done(log);

    if (!EvtNext(log->event_query, 1, &log->bookmark, INFINITE, 0, &returned))
        goto cleanup; // no data available, return failure

    // obtain the information from selected events
    if (!EvtRender(log->render_context, log->bookmark, EvtRenderEventValues, log->ops.content.size, log->ops.content.data, &bytes_used, &property_count)) {
        // information exceeds the allocated space
        if (GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtRender() failed");
            goto cleanup;
        }

        freez(log->ops.content.data);
        log->ops.content.size = bytes_used;
        log->ops.content.data = (EVT_VARIANT *)mallocz(log->ops.content.size);

        if (!EvtRender(log->render_context, log->bookmark, EvtRenderEventValues, log->ops.content.size, log->ops.content.data, &bytes_used, &property_count)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtRender() failed, after bytes_used increase");
            goto cleanup;
        }
    }
    log->ops.content.len = bytes_used;

    ev->id = wevt_get_unsigned_by_type(log, FIELD_RECORD_NUMBER);
    ev->event_id = wevt_get_unsigned_by_type(log, FIELD_EVENT_ID);
    ev->level = wevt_get_unsigned_by_type(log, FIELD_LEVEL);
    ev->keywords = wevt_get_unsigned_by_type(log, FIELD_KEYWORDS);
    ev->created_ns = wevt_get_filetime_to_ns_by_type(log, FIELD_TIME_CREATED);
    ev->opcode = wevt_get_unsigned_by_type(log, FIELD_OPCODE);
    ev->version = wevt_get_unsigned_by_type(log, FIELD_VERSION);
    ev->task = wevt_get_unsigned_by_type(log, FIELD_TASK);
    ev->process_id = wevt_get_unsigned_by_type(log, FIELD_PROCESS_ID);
    ev->thread_id = wevt_get_unsigned_by_type(log, FIELD_THREAD_ID);

    if(full) {
        wevt_get_utf8_by_type(log, FIELD_CHANNEL, &log->ops.channel);
        wevt_get_utf8_by_type(log, FIELD_PROVIDER_NAME, &log->ops.provider);
        wevt_get_utf8_by_type(log, FIELD_COMPUTER_NAME, &log->ops.computer);
        wevt_get_utf8_by_type(log, FIELD_EVENT_SOURCE_NAME, &log->ops.source);

        wevt_get_uuid_by_type(log, FIELD_PROVIDER_GUID, &ev->provider);
        wevt_get_uuid_by_type(log, FIELD_CORRELATION_ACTIVITY_ID, &ev->correlation_activity_id);

        // User
        wevt_get_sid_by_type(log, FIELD_USER_ID, &log->ops.user);

        PROVIDER_META_HANDLE *p = log->publisher =
                publisher_get(ev->provider, log->ops.content.data[FIELD_PROVIDER_NAME].StringVal);

//        if(!field_cache_get(WEVT_FIELD_TYPE_LEVEL, ev->provider, ev->level, &log->ops.level2)) {
//            wevt_get_level_utf8(log, publisher_handle(p), log->bookmark, &log->ops.level2);
//            field_cache_set(WEVT_FIELD_TYPE_LEVEL, ev->provider, ev->level, &log->ops.level2);
//        }
//
//        if(!field_cache_get(WEVT_FIELD_TYPE_TASK, ev->provider, ev->task, &log->ops.task2)) {
//            wevt_get_task_utf8(log, publisher_handle(p), log->bookmark, &log->ops.task2);
//            field_cache_set(WEVT_FIELD_TYPE_TASK, ev->provider, ev->task, &log->ops.task2);
//        }
//
//        if(!field_cache_get(WEVT_FIELD_TYPE_OPCODE, ev->provider, ev->opcode, &log->ops.opcode2)) {
//            wevt_get_opcode_utf8(log, publisher_handle(p), log->bookmark, &log->ops.opcode2);
//            field_cache_set(WEVT_FIELD_TYPE_OPCODE, ev->provider, ev->opcode, &log->ops.opcode2);
//        }
//
//        if(!field_cache_get(WEVT_FIELD_TYPE_KEYWORDS, ev->provider, ev->keywords, &log->ops.keywords2)) {
//            wevt_get_keywords_utf8(log, publisher_handle(p), log->bookmark, &log->ops.keywords2);
//            field_cache_set(WEVT_FIELD_TYPE_KEYWORDS, ev->provider, ev->keywords, &log->ops.keywords2);
//        }

        wevt_get_xml_utf8(log, publisher_handle(p), log->bookmark, &log->ops.xml);
    }

    ret = true;

cleanup:
    return ret;
}

void wevt_query_done(WEVT_LOG *log) {
    wevt_event_done(log);

    if (log->event_query) {
        EvtClose(log->event_query);
        log->event_query = NULL;
    }
}

void wevt_closelog6(WEVT_LOG *log) {
    wevt_query_done(log);

    if (log->render_context)
        EvtClose(log->render_context);

    freez(log->ops.content.data);
    txt_unicode_cleanup(&log->ops.unicode);
    txt_utf8_cleanup(&log->ops.channel);
    txt_utf8_cleanup(&log->ops.provider);
    txt_utf8_cleanup(&log->ops.source);
    txt_utf8_cleanup(&log->ops.computer);
    txt_utf8_cleanup(&log->ops.user);

    txt_utf8_cleanup(&log->ops.event2);
    txt_utf8_cleanup(&log->ops.level2);
    txt_utf8_cleanup(&log->ops.keywords2);
    txt_utf8_cleanup(&log->ops.opcode2);
    txt_utf8_cleanup(&log->ops.task2);
    txt_utf8_cleanup(&log->ops.xml);

    buffer_free(log->ops.keywords);
    buffer_free(log->ops.opcode);
    buffer_free(log->ops.task);
    freez(log);
}

bool wevt_channel_retention(WEVT_LOG *log, const wchar_t *channel, EVT_RETENTION *retention) {
    bool ret = false;

    // get the number of the oldest record in the log
    // "EvtGetLogInfo()" does not work properly with "EvtLogOldestRecordNumber"
    // we have to get it from the first EventRecordID

    // query the eventlog
    log->event_query = EvtQuery(NULL, channel, NULL, EvtQueryChannelPath);
    if (!log->event_query) {
        if (GetLastError() == ERROR_EVT_CHANNEL_NOT_FOUND)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() failed, channel '%s' not found, cannot get retention", channel2utf8(channel));
        else
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() on channel '%s' failed, cannot get retention", channel2utf8(channel));

        goto cleanup;
    }

    if (!wevt_get_next_event(log, &retention->first_event, false))
        goto cleanup;

    if (!retention->first_event.id) {
        // no data in the event log
        retention->first_event = retention->last_event = WEVT_EVENT_EMPTY;
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

    if (!wevt_get_next_event(log, &retention->last_event, false) || retention->last_event.id == 0) {
        // no data in eventlog
        retention->last_event = retention->first_event;
    }
    retention->last_event.id += 1;	// we should read the last record
    ret = true;

cleanup:
    wevt_query_done(log);

    if(ret) {
        retention->entries = retention->last_event.id - retention->first_event.id;

        if(retention->last_event.created_ns >= retention->first_event.created_ns)
            retention->duration_ns = retention->last_event.created_ns - retention->first_event.created_ns;
        else
            retention->duration_ns = retention->first_event.created_ns - retention->last_event.created_ns;

        retention->size_bytes = wevt_log_file_size(channel);
    }
    else
        memset(retention, 0, sizeof(*retention));

    return ret;
}

WEVT_LOG *wevt_openlog6(void) {
    size_t RENDER_ITEMS_count = (sizeof(RENDER_ITEMS) / sizeof(const wchar_t *));

    WEVT_LOG *log = callocz(1, sizeof(*log));

    // create the system render
    log->render_context = EvtCreateRenderContext(RENDER_ITEMS_count, RENDER_ITEMS, EvtRenderContextValues);
    if (!log->render_context) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtCreateRenderContext failed.");
        freez(log);
        log = NULL;
        goto cleanup;
    }

    log->ops.keywords = buffer_create(4096, NULL);
    log->ops.opcode = buffer_create(4096, NULL);
    log->ops.task = buffer_create(4096, NULL);

cleanup:
    return log;
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

EVT_HANDLE wevt_query(LPCWSTR channel, LPCWSTR query, EVT_QUERY_FLAGS direction) {
    EVT_HANDLE hQuery = EvtQuery(NULL, channel, query, EvtQueryChannelPath | (direction & (EvtQueryReverseDirection | EvtQueryForwardDirection)));
    if (!hQuery) {
        wchar_t wbuf[1024];
        DWORD wbuf_used;
        EvtGetExtendedStatus(sizeof(wbuf), wbuf, &wbuf_used);
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() failed, query: %s | extended info: %ls",
               query2utf8(query), wbuf);
    }

    return hQuery;
}
