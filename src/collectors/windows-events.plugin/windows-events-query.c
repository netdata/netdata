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
#define	FIELD_EVENT_DATA                    (17)

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
        L"/Event/EventData/Data"
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

static bool wevt_get_message_utf8(WEVT_LOG *log, EVT_HANDLE event_handle, TXT_UTF8 *dst, EVT_FORMAT_MESSAGE_FLAGS what) {
    DWORD size = 0;

    if(!log->ops.unicode.data) {
        EvtFormatMessage(NULL, event_handle, 0, 0, NULL, what, 0, NULL, &size);
        if(!size) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() to get message size failed.");
            goto cleanup;
        }
        txt_unicode_resize(&log->ops.unicode, size);
    }

    // First, try to get the message using the existing buffer
    if (!EvtFormatMessage(NULL, event_handle, 0, 0, NULL, what, log->ops.unicode.size, log->ops.unicode.data, &size) || !log->ops.unicode.data) {
        if (log->ops.unicode.data && GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
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

void wevt_field_to_buffer_append(BUFFER *wb, EVT_VARIANT *d, const char *prefix, const char *suffix) {
    buffer_strcat(wb, prefix);

    switch(d->Type & EVT_VARIANT_TYPE_MASK) {
        case EvtVarTypeNull:
            buffer_strcat(wb, "(null)");
            break;

        case EvtVarTypeString:
            buffer_sprintf(wb, "%ls", d->StringVal);
            break;

        case EvtVarTypeSByte:
            buffer_sprintf(wb, "%d", (int) d->SByteVal);
            break;

        case EvtVarTypeByte:
            buffer_sprintf(wb, "%u", (unsigned) d->ByteVal);
            break;

        case EvtVarTypeInt16:
            buffer_sprintf(wb, "%d", (int) d->Int16Val);
            break;

        case EvtVarTypeUInt16:
            buffer_sprintf(wb, "%u", (unsigned) d->UInt16Val);
            break;

        case EvtVarTypeInt32:
            buffer_sprintf(wb, "%d", (int) d->Int32Val);
            break;

        case EvtVarTypeUInt32:
        case EvtVarTypeHexInt32:
            buffer_sprintf(wb, "%u", (unsigned) d->UInt32Val);
            break;

        case EvtVarTypeInt64:
            buffer_sprintf(wb, "%" PRIi64, (uint64_t)d->Int64Val);
            break;

        case EvtVarTypeUInt64:
        case EvtVarTypeHexInt64:
            buffer_sprintf(wb, "%" PRIu64, (uint64_t)d->UInt64Val);
            break;

        case EvtVarTypeSizeT:
            buffer_sprintf(wb, "%zu", d->SizeTVal);
            break;

        case EvtVarTypeSysTime:
            buffer_sprintf(wb, "%04d-%02d-%02dT%02d:%02d:%02d.%03d (SystemTime)",
                           d->SysTimeVal->wYear, d->SysTimeVal->wMonth, d->SysTimeVal->wDay,
                           d->SysTimeVal->wHour, d->SysTimeVal->wMinute, d->SysTimeVal->wSecond,
                           d->SysTimeVal->wMilliseconds);
            break;

        case EvtVarTypeGuid: {
            char uuid_str[UUID_COMPACT_STR_LEN];
            ND_UUID uuid;
            if (wevt_GUID_to_ND_UUID(&uuid, d->GuidVal)) {
                uuid_unparse_lower_compact(uuid.uuid, uuid_str);
                buffer_strcat(wb, uuid_str);
                buffer_strcat(wb, " (GUID)");
            }
            break;
        }

        case EvtVarTypeSingle:
            buffer_print_netdata_double(wb, d->SingleVal);
            break;

        case EvtVarTypeDouble:
            buffer_print_netdata_double(wb, d->DoubleVal);
            break;

        case EvtVarTypeBoolean:
            buffer_strcat(wb, d->BooleanVal ? "true" : "false");
            break;

        case EvtVarTypeFileTime: {
            nsec_t ns = os_windows_ulonglong_to_unix_epoch_ns(d->FileTimeVal);
            char buf[RFC3339_MAX_LENGTH];
            rfc3339_datetime_ut(buf, sizeof(buf), ns, 2, true);
            buffer_strcat(wb, buf);
            buffer_strcat(wb, " (FileTime)");
            break;
        }

        case EvtVarTypeBinary:
            buffer_strcat(wb, "(binary data)");
            break;

        case EvtVarTypeSid:
            buffer_strcat(wb, "(user id data)");
            break;

        case EvtVarTypeAnsiString:
        case EvtVarTypeEvtHandle:
        case EvtVarTypeEvtXml:
        default:
            buffer_sprintf(wb, "(unsupported data type: %u)", d->Type);
            break;
    }

    buffer_strcat(wb, suffix);
}

static bool wevt_format_summary_array_traversal(WEVT_LOG *log, size_t field, WEVT_EVENT *ev) {
    EVT_VARIANT *d = &log->ops.content.data[field];

    if (!log->ops.message)
        log->ops.message = buffer_create(0, NULL);

    BUFFER *wb = log->ops.message;
    buffer_flush(wb);

    // Check if there is an event description
    if (log->ops.event.data && *log->ops.event.data) {
        buffer_strcat(wb, log->ops.event.data);
        buffer_strcat(wb, "\nRelated data:\n");
    } else {
        buffer_sprintf(wb, "Event %" PRIu64 ", with the following related data:\n", (uint64_t)ev->event_id);
    }

    // Check if the field contains an array or a single value
    bool is_array = (d->Type & EVT_VARIANT_TYPE_ARRAY) != 0;
    EVT_VARIANT_TYPE base_type = (EVT_VARIANT_TYPE)(d->Type & EVT_VARIANT_TYPE_MASK);

    if (is_array) {
        DWORD count = d->Count; // Number of elements in the array

        for (DWORD i = 0; i < count; i++) {
            EVT_VARIANT array_element = {
                    .Type = base_type,
                    .Count = 0,
            };

            // Point the array element to the correct data
            switch (base_type) {
                case EvtVarTypeBoolean:
                    array_element.BooleanVal = d->BooleanArr[i];
                    break;
                case EvtVarTypeSByte:
                    array_element.SByteVal = d->SByteArr[i];
                    break;
                case EvtVarTypeByte:
                    array_element.ByteVal = d->ByteArr[i];
                    break;
                case EvtVarTypeInt16:
                    array_element.Int16Val = d->Int16Arr[i];
                    break;
                case EvtVarTypeUInt16:
                    array_element.UInt16Val = d->UInt16Arr[i];
                    break;
                case EvtVarTypeInt32:
                    array_element.Int32Val = d->Int32Arr[i];
                    break;
                case EvtVarTypeUInt32:
                    array_element.UInt32Val = d->UInt32Arr[i];
                    break;
                case EvtVarTypeInt64:
                    array_element.Int64Val = d->Int64Arr[i];
                    break;
                case EvtVarTypeUInt64:
                    array_element.UInt64Val = d->UInt64Arr[i];
                    break;
                case EvtVarTypeSingle:
                    array_element.SingleVal = d->SingleArr[i];
                    break;
                case EvtVarTypeDouble:
                    array_element.DoubleVal = d->DoubleArr[i];
                    break;
                case EvtVarTypeFileTime:
                    array_element.FileTimeVal = ((ULONGLONG)d->FileTimeArr[i].dwLowDateTime |
                                                 ((ULONGLONG)d->FileTimeArr[i].dwHighDateTime << 32));
                    break;
                case EvtVarTypeSysTime:
                    array_element.SysTimeVal = &d->SysTimeArr[i];
                    break;
                case EvtVarTypeGuid:
                    array_element.GuidVal = &d->GuidArr[i];
                    break;
                case EvtVarTypeString:
                    array_element.StringVal = d->StringArr[i];
                    break;
                case EvtVarTypeAnsiString:
                    array_element.AnsiStringVal = d->AnsiStringArr[i];
                    break;
                case EvtVarTypeSid:
                    array_element.SidVal = d->SidArr[i];
                    break;
                case EvtVarTypeSizeT:
                    array_element.SizeTVal = d->SizeTArr[i];
                    break;
                case EvtVarTypeEvtXml:
                    array_element.XmlVal = d->XmlValArr[i];
                    break;
                default:
                    buffer_sprintf(wb, "  - Unsupported array type: %u\n", base_type);
                    continue;
            }

            // Call the field appending function for each array element
            wevt_field_to_buffer_append(wb, &array_element, "  - ", "\n");
        }
    } else {
        // Handle single values, pass the EVT_VARIANT directly
        wevt_field_to_buffer_append(wb, d, "  - ", "\n");
    }

    return true;
}

static bool wevt_format_summary(WEVT_LOG *log, WEVT_EVENT *ev) {
    if (!log->ops.message)
        log->ops.message = buffer_create(0, NULL);

    BUFFER *wb = log->ops.message;
    buffer_flush(wb);

    // Check if there is an event description
    if (log->ops.event.data && *log->ops.event.data)
        buffer_strcat(wb, log->ops.event.data);
    else
        buffer_sprintf(wb, "Event %" PRIu64, (uint64_t) ev->event_id);

    const char *xml = log->ops.xml.data;
    const char *event_data_start = strstr(xml, "<EventData>");
    if(event_data_start)
        event_data_start = &event_data_start[11];

    const char *event_data_end = event_data_start ? strstr(xml, "</EventData>") : NULL;

    if(event_data_start && event_data_end) {
        buffer_strcat(wb, "\n\nRelated data:\n\n");
        // copy the event data block
        buffer_fast_strcat(wb, event_data_start, event_data_end - event_data_start);
    }

    return true;
}

bool wevt_get_next_event(WEVT_LOG *log, WEVT_EVENT *ev, bool full) {
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

    ev->id = wevt_get_unsigned_by_type(log, FIELD_RECORD_NUMBER);
    ev->event_id = wevt_get_unsigned_by_type(log, FIELD_EVENT_ID);
    ev->level = wevt_get_unsigned_by_type(log, FIELD_LEVEL);
    ev->keyword = wevt_get_unsigned_by_type(log, FIELD_KEYWORDS);
    ev->created_ns = wevt_get_filetime_to_ns_by_type(log, FIELD_TIME_CREATED);
    ev->opcode = wevt_get_unsigned_by_type(log, FIELD_OPCODE);
    ev->version = wevt_get_unsigned_by_type(log, FIELD_VERSION);
    ev->task = wevt_get_unsigned_by_type(log, FIELD_TASK);
    ev->process_id = wevt_get_unsigned_by_type(log, FIELD_PROCESS_ID);
    ev->thread_id = wevt_get_unsigned_by_type(log, FIELD_THREAD_ID);

    if(full) {
        wevt_get_utf8_by_type(log, FIELD_CHANNEL, &log->ops.channel);
        wevt_get_utf8_by_type(log, FIELD_PROVIDER_NAME, &log->ops.provider);
        wevt_get_utf8_by_type(log, FIELD_EVENT_SOURCE_NAME, &log->ops.source);
        wevt_get_uuid_by_type(log, FIELD_PROVIDER_GUID, &ev->provider);
        wevt_get_message_utf8(log, tmp_event_bookmark, &log->ops.level, EvtFormatMessageLevel);

        if(ev->event_id)
            wevt_get_message_utf8(log, tmp_event_bookmark, &log->ops.event, EvtFormatMessageEvent);
        else
            wevt_empty_utf8(&log->ops.event);

        if(ev->keyword)
            wevt_get_message_utf8(log, tmp_event_bookmark, &log->ops.keyword, EvtFormatMessageKeyword);
        else
            wevt_empty_utf8(&log->ops.keyword);

        if(ev->opcode)
            wevt_get_message_utf8(log, tmp_event_bookmark, &log->ops.opcode, EvtFormatMessageOpcode);
        else
            wevt_empty_utf8(&log->ops.keyword);

        // ComputerName
        wevt_get_utf8_by_type(log, FIELD_COMPUTER_NAME, &log->ops.computer);

        // User
        wevt_get_sid_by_type(log, FIELD_USER_ID, &log->ops.user);

        // CorrelationActivityID
        wevt_get_uuid_by_type(log, FIELD_CORRELATION_ACTIVITY_ID, &ev->correlation_activity_id);

        // Full XML of the entire message
        wevt_get_message_utf8(log, tmp_event_bookmark, &log->ops.xml, EvtFormatMessageXml);

        // Format a text message for the users to see
        // wevt_format_summary(log, ev);
    }

    ret = true;

cleanup:
    if (tmp_event_bookmark)
        EvtClose(tmp_event_bookmark);

    return ret;
}

void wevt_query_done(WEVT_LOG *log) {
    if (!log->event_query) return;

    EvtClose(log->event_query);
    log->event_query = NULL;
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
    txt_utf8_cleanup(&log->ops.event);
    txt_utf8_cleanup(&log->ops.user);
    txt_utf8_cleanup(&log->ops.opcode);
    txt_utf8_cleanup(&log->ops.level);
    txt_utf8_cleanup(&log->ops.keyword);
    txt_utf8_cleanup(&log->ops.xml);
    buffer_free(log->ops.message);
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
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() failed, channel '%s' not found, cannot open log", channel2utf8(channel));
        else
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() on channel '%s' failed, cannot open log", channel2utf8(channel));

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
        goto cleanup;
    }

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
