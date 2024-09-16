// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-query.h"

static uint64_t wevt_log_file_size(const wchar_t *channel);

#define FIELD_RECORD_NUMBER                 (0)
#define FIELD_EVENT_ID                      (1)
#define FIELD_LEVEL                         (2)
#define FIELD_OPCODE                        (3)
#define FIELD_KEYWORDS                      (4)
#define FIELD_VERSION                       (5)
#define FIELD_TASK                          (6)
#define FIELD_PROCESS_ID                    (7)
#define FIELD_THREAD_ID                     (8)
#define FIELD_TIME_CREATED                  (9)
#define FIELD_CHANNEL                       (10)
#define FIELD_COMPUTER_NAME                 (11)
#define FIELD_PROVIDER_NAME                 (12)
#define FIELD_EVENT_SOURCE_NAME             (13)
#define FIELD_PROVIDER_GUID                 (14)
#define FIELD_CORRELATION_ACTIVITY_ID       (15)
#define FIELD_USER_ID                       (16)

// These are the fields we extract from the logs
static const wchar_t *RENDER_ITEMS[] = {
    L"/Event/System/EventRecordID",
    L"/Event/System/EventID",
    L"/Event/System/Level",
    L"/Event/System/Opcode",
    L"/Event/System/Keywords",
    L"/Event/System/Version",
    L"/Event/System/Task",
    L"/Event/System/Execution/@ProcessID",
    L"/Event/System/Execution/@ThreadID",
    L"/Event/System/TimeCreated/@SystemTime",
    L"/Event/System/Channel",
    L"/Event/System/Computer",
    L"/Event/System/Provider/@Name",
    L"/Event/System/Provider/@EventSourceName",
    L"/Event/System/Provider/@Guid",
    L"/Event/System/Correlation/@ActivityID",
    L"/Event/System/Security/@UserID",
};

static const char *wevt_extended_status(void) {
    static __thread wchar_t wbuf[4096];
    static __thread char buf[4096];
    DWORD wbuf_used = 0;

    if(EvtGetExtendedStatus(sizeof(wbuf) / sizeof(wchar_t), wbuf, &wbuf_used) == ERROR_SUCCESS) {
        wbuf[sizeof(wbuf) / sizeof(wchar_t) - 1] = 0;
        unicode2utf8(buf, sizeof(buf), wbuf);
    }
    else
        buf[0] = '\0';

    // the EvtGetExtendedStatus() may be successful with an empty message
    if(!buf[0])
        strncpyz(buf, "no additional information", sizeof(buf) - 1);

    return buf;
}

bool wevt_get_message_unicode(TXT_UNICODE *unicode, EVT_HANDLE hMetadata, EVT_HANDLE bookmark, DWORD dwMessageId, EVT_FORMAT_MESSAGE_FLAGS flags) {
    unicode->used = 0;

    DWORD size = 0;
    if(!unicode->data) {
        EvtFormatMessage(hMetadata, bookmark, dwMessageId, 0, NULL, flags, 0, NULL, &size);
        if(!size) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() to get message size failed.");
            goto cleanup;
        }
        txt_unicode_resize(unicode, size);
    }

    // First, try to get the message using the existing buffer
    if (!EvtFormatMessage(hMetadata, bookmark, dwMessageId, 0, NULL, flags, unicode->size, unicode->data, &size) || !unicode->data) {
        if (unicode->data && GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() failed.");
            goto cleanup;
        }

        // Try again with the resized buffer
        txt_unicode_resize(unicode, size);
        if (!EvtFormatMessage(hMetadata, bookmark, dwMessageId, 0, NULL, flags, unicode->size, unicode->data, &size)) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() failed after resizing buffer.");
            goto cleanup;
        }
    }

    // make sure it is null terminated
    if(size <= unicode->size)
        unicode->data[size - 1] = 0;
    else
        unicode->data[unicode->size - 1] = 0;

    // unfortunately we have to calculate the length every time
    // the size returned may not be the length of the unicode string
    unicode->used = wcslen(unicode->data) + 1;
    return true;

cleanup:
    unicode->used = 0;
    return false;
}

static bool wevt_get_field_from_events_log(
    WEVT_LOG *log, PROVIDER_META_HANDLE *p, EVT_HANDLE event_handle,
    TXT_UTF8 *dst, EVT_FORMAT_MESSAGE_FLAGS flags) {

    dst->src = TXT_SOURCE_EVENT_LOG;

    if(wevt_get_message_unicode(&log->ops.unicode, publisher_handle(p), event_handle, 0, flags))
        return wevt_str_unicode_to_utf8(dst, &log->ops.unicode);

    wevt_utf8_empty(dst);
    return false;
}

bool wevt_get_event_utf8(WEVT_LOG *log, PROVIDER_META_HANDLE *p, EVT_HANDLE event_handle, TXT_UTF8 *dst) {
    return wevt_get_field_from_events_log(log, p, event_handle, dst, EvtFormatMessageEvent);
}

bool wevt_get_xml_utf8(WEVT_LOG *log, PROVIDER_META_HANDLE *p, EVT_HANDLE event_handle, TXT_UTF8 *dst) {
    return wevt_get_field_from_events_log(log, p, event_handle, dst, EvtFormatMessageXml);
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

    log->ops.level.src = TXT_SOURCE_UNKNOWN;
    log->ops.keywords.src = TXT_SOURCE_UNKNOWN;
    log->ops.opcode.src = TXT_SOURCE_UNKNOWN;
    log->ops.task.src = TXT_SOURCE_UNKNOWN;
}

static void wevt_get_field_from_cache(
    WEVT_LOG *log, uint64_t value, PROVIDER_META_HANDLE *h,
    TXT_UTF8 *dst, const ND_UUID *provider,
    WEVT_FIELD_TYPE cache_type, EVT_FORMAT_MESSAGE_FLAGS flags) {

    if (field_cache_get(cache_type, provider, value, dst))
        return;

    wevt_get_field_from_events_log(log, h, log->bookmark, dst, flags);
    field_cache_set(cache_type, provider, value, dst);
}

static inline const char *wevt_level_hardcoded(uint64_t level, size_t *len) {
    switch(level) {
        case 0: *len = 4; return "None";
        case 1: *len = 8; return "Critical";
        case 2: *len = 5; return "Error";
        case 3: *len = 7; return "Warning";
        case 4: *len = 11; return "Information";
        case 5: *len = 7; return "Verbose";
        default: return NULL;
    }
}

static const char *wevt_level_default(uint64_t level, size_t *len) {
    static __thread char buf[6 + UINT64_MAX_LENGTH + 1];
    strcpy(buf, "Level ");
    *len = 6 + print_uint64(&buf[6], level);
    return buf;
}

static void wevt_get_level(WEVT_LOG *log, WEVT_EVENT *ev, PROVIDER_META_HANDLE *h) {
    TXT_UTF8 *dst = &log->ops.level;
    uint64_t value = ev->level;

    dst->used = 0; // empty our response

    EVT_FORMAT_MESSAGE_FLAGS flags = EvtFormatMessageLevel;
    WEVT_FIELD_TYPE cache_type = WEVT_FIELD_TYPE_LEVEL;
    bool is_publisher = is_valid_publisher_level(value, true);

    if(!is_publisher) {
        size_t len;
        const char *hardcoded = wevt_level_hardcoded(value, &len);
        if(hardcoded) {
            txt_utf8_resize(dst, len + 1,  false);
            memcpy(dst->data, hardcoded, len + 1);
            dst->used = len + 1;
            dst->src = TXT_SOURCE_HARDCODED;
        }
        else {
            // since this is not a publisher value
            // we expect to get the system description of it
            wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
        }
    }
    else if (!publisher_get_level(dst, h, value)) {
        // not found in the manifest, get it from the cache
        wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
    }

    if(dst->used <= 1) {
        // an empty value, provide something meaningful
        size_t len = 0;
        const char *s = wevt_level_default(value, &len);
        txt_utf8_resize(dst, len + 1,  false);
        memcpy(dst->data, s, len + 1);
        dst->used = len + 1;
    }
}

static const char *wevt_opcode_hardcoded(uint64_t level, size_t *len) {
    switch(level) {
        case 0: *len = 4; return "Info";
        case 1: *len = 5; return "Start";
        case 2: *len = 4; return "Stop";
        case 3: *len = 8; return "DC Start";
        case 4: *len = 7; return "DC Stop";
        case 5: *len = 9; return "Extension";
        case 6: *len = 5; return "Reply";
        case 7: *len = 6; return "Resume";
        case 8: *len = 7; return "Suspend";
        case 9: *len = 4; return "Send";
        case 240: *len = 7; return "Receive";
        default: *len = 0; return NULL;
    }
}

static const char *wevt_opcode_default(uint64_t opcode, size_t *len) {
    static __thread char buf[7 + UINT64_MAX_LENGTH + 1];
    strcpy(buf, "Opcode ");
    *len = 7 + print_uint64(&buf[7], opcode);
    return buf;
}

static void wevt_get_opcode(WEVT_LOG *log, WEVT_EVENT *ev, PROVIDER_META_HANDLE *h) {
    TXT_UTF8 *dst = &log->ops.opcode;
    uint64_t value = ev->opcode;

    dst->used = 0; // empty our response

    EVT_FORMAT_MESSAGE_FLAGS flags = EvtFormatMessageOpcode;
    WEVT_FIELD_TYPE cache_type = WEVT_FIELD_TYPE_OPCODE;
    bool is_publisher = is_valid_publisher_opcode(value, true);

    if(!is_publisher) {
        size_t len;
        const char *hardcoded = wevt_opcode_hardcoded(value, &len);
        if(hardcoded) {
            txt_utf8_resize(dst, len + 1,  false);
            memcpy(dst->data, hardcoded, len + 1);
            dst->used = len + 1;
            dst->src = TXT_SOURCE_HARDCODED;
        }
        else {
            // since this is not a publisher value
            // we expect to get the system description of it
            wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
        }
    }
    else if (!publisher_get_opcode(dst, h, value)) {
        // not found in the manifest, get it from the cache
        wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
    }

    if(dst->used <= 1) {
        // an empty value, provide something meaningful
        size_t len = 0;
        const char *s = wevt_opcode_default(value, &len);
        txt_utf8_resize(dst, len + 1,  false);
        memcpy(dst->data, s, len + 1);
        dst->used = len + 1;
    }
}

static const char *wevt_task_hardcoded(uint64_t task, size_t *len) {
    switch(task) {
        case 0: *len = 4; return "None";
        default: *len = 0; return NULL;
    }
}

static const char *wevt_task_default(uint64_t task, size_t *len) {
    static __thread char buf[5 + UINT64_MAX_LENGTH + 1];
    strcpy(buf, "Task ");
    *len = 5 + print_uint64(&buf[5], task);
    return buf;
}

static void wevt_get_task(WEVT_LOG *log, WEVT_EVENT *ev, PROVIDER_META_HANDLE *h) {
    TXT_UTF8 *dst = &log->ops.task;
    uint64_t value = ev->task;

    dst->used = 0; // empty our response

    EVT_FORMAT_MESSAGE_FLAGS flags = EvtFormatMessageTask;
    WEVT_FIELD_TYPE cache_type = WEVT_FIELD_TYPE_TASK;
    bool is_publisher = is_valid_publisher_task(value, true);

    if(!is_publisher) {
        size_t len;
        const char *hardcoded = wevt_task_hardcoded(value, &len);
        if(hardcoded) {
            txt_utf8_resize(dst, len + 1,  false);
            memcpy(dst->data, hardcoded, len + 1);
            dst->used = len + 1;
            dst->src = TXT_SOURCE_HARDCODED;
        }
        else {
            // since this is not a publisher value
            // we expect to get the system description of it
            wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
        }
    }
    else if (!publisher_get_task(dst, h, value)) {
        // not found in the manifest, get it from the cache
        wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
    }

    if(dst->used <= 1) {
        // an empty value, provide something meaningful
        size_t len = 0;
        const char *s = wevt_task_default(value, &len);
        txt_utf8_resize(dst, len + 1,  false);
        memcpy(dst->data, s, len + 1);
        dst->used = len + 1;
    }
}

static const char *wevt_keywords_hardcoded(uint64_t keywords, size_t *len) {
    switch(keywords) {
        case 0: *len = 4; return "None";
        default: *len = 0; return NULL;
    }
}

static const char *wevt_keywords_default(uint64_t task, size_t *len) {
    static __thread char buf[9 + UINT64_MAX_LENGTH + 1];
    strcpy(buf, "Keywords ");
    *len = 9 + print_uint64(&buf[9], task);
    return buf;
}

static void wevt_get_keywords(WEVT_LOG *log, WEVT_EVENT *ev, PROVIDER_META_HANDLE *h) {
    TXT_UTF8 *dst = &log->ops.keywords;

    // microsoft suggests the high 16 bits are for internal use
    uint64_t value = ev->keywords & 0x0000FFFFFFFFFFFF;

    dst->used = 0; // empty our response

    EVT_FORMAT_MESSAGE_FLAGS flags = EvtFormatMessageKeyword;
    WEVT_FIELD_TYPE cache_type = WEVT_FIELD_TYPE_KEYWORDS;
    bool is_publisher = is_valid_publisher_keywords(value, true);

    if(!is_publisher) {
        size_t len;
        const char *hardcoded = wevt_keywords_hardcoded(value, &len);
        if(hardcoded) {
            txt_utf8_resize(dst, len + 1,  false);
            memcpy(dst->data, hardcoded, len + 1);
            dst->used = len + 1;
            dst->src = TXT_SOURCE_HARDCODED;
        }
        else {
            // since this is not a publisher value
            // we expect to get the system description of it
            wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
        }
    }
    else if (!publisher_get_keywords(dst, h, value)) {
        // not found in the manifest, get it from the cache
        wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
    }

    if(dst->used <= 1) {
        // an empty value, provide something meaningful
        size_t len = 0;
        const char *s = wevt_keywords_default(value, &len);
        txt_utf8_resize(dst, len + 1,  false);
        memcpy(dst->data, s, len + 1);
        dst->used = len + 1;
    }
}

bool wevt_get_next_event_one(WEVT_LOG *log, WEVT_EVENT *ev, bool full) {
    bool ret = false;

    // obtain the information from selected events
    DWORD bytes_used = 0, property_count = 0;
    if (!EvtRender(log->render_context, log->bookmark, EvtRenderEventValues, log->ops.content.size, log->ops.content.data, &bytes_used, &property_count)) {
        // information exceeds the allocated space
        if (GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtRender() failed, render_context: 0x%lx, bookmark: 0x%lx, content: 0x%lx, size: %zu, extended info: %s",
                   (uintptr_t)log->render_context, (uintptr_t)log->bookmark, (uintptr_t)log->ops.content.data, log->ops.content.size, wevt_extended_status());
            goto cleanup;
        }

        wevt_variant_resize(&log->ops.content, bytes_used);
        if (!EvtRender(log->render_context, log->bookmark, EvtRenderEventValues, log->ops.content.size, log->ops.content.data, &bytes_used, &property_count)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtRender() failed, after bytes_used increase, extended info: %s",
                   wevt_extended_status());
            goto cleanup;
        }
    }
    log->ops.content.used = bytes_used;

    EVT_VARIANT *content = log->ops.content.data;

    ev->id          = wevt_field_get_uint64(&content[FIELD_RECORD_NUMBER]);
    ev->event_id    = wevt_field_get_uint16(&content[FIELD_EVENT_ID]);
    ev->level       = wevt_field_get_uint8(&content[FIELD_LEVEL]);
    ev->opcode      = wevt_field_get_uint8(&content[FIELD_OPCODE]);
    ev->keywords    = wevt_field_get_uint64_hex(&content[FIELD_KEYWORDS]);
    ev->version     = wevt_field_get_uint8(&content[FIELD_VERSION]);
    ev->task        = wevt_field_get_uint16(&content[FIELD_TASK]);
    ev->process_id  = wevt_field_get_uint32(&content[FIELD_PROCESS_ID]);
    ev->thread_id   = wevt_field_get_uint32(&content[FIELD_THREAD_ID]);
    ev->created_ns  = wevt_field_get_filetime_to_ns(&content[FIELD_TIME_CREATED]);

    if(full) {
        wevt_field_get_string_utf8(&content[FIELD_CHANNEL], &log->ops.channel);
        wevt_field_get_string_utf8(&content[FIELD_COMPUTER_NAME], &log->ops.computer);
        wevt_field_get_string_utf8(&content[FIELD_PROVIDER_NAME], &log->ops.provider);
        wevt_field_get_string_utf8(&content[FIELD_EVENT_SOURCE_NAME], &log->ops.source);
        wevt_get_uuid_by_type(&content[FIELD_PROVIDER_GUID], &ev->provider);
        wevt_get_uuid_by_type(&content[FIELD_CORRELATION_ACTIVITY_ID], &ev->correlation_activity_id);
        wevt_field_get_sid(&content[FIELD_USER_ID], &log->ops.user);

        PROVIDER_META_HANDLE *h = log->publisher =
            publisher_get(ev->provider, content[FIELD_PROVIDER_NAME].StringVal);

        wevt_get_level(log, ev, h);
        wevt_get_task(log, ev, h);
        wevt_get_opcode(log, ev, h);
        wevt_get_keywords(log, ev, h);
    }

    ret = true;

cleanup:
    return ret;
}

bool wevt_get_next_event(WEVT_LOG *log, WEVT_EVENT *ev, bool full) {
    DWORD size = full ? BATCH_NEXT_EVENT : 1;
    DWORD max_failures = 10;

    fatal_assert(log && log->event_query && log->render_context);

    while(max_failures > 0) {
        if (log->batch.used >= log->batch.size) {
            log->batch.size = 0;
            log->batch.used = 0;
            DWORD err;
            if(!EvtNext(log->event_query, size, log->batch.bk, INFINITE, 0, &log->batch.size)) {
                err = GetLastError();
                if(err == ERROR_NO_MORE_ITEMS)
                    return false; // no data available, return failure
            }

            if(!log->batch.size) {
                if(size == 1) {
                    nd_log(NDLS_COLLECTORS, NDLP_ERR,
                           "EvtNext() failed, event_query: 0x%lx, size: %zu, extended info: %s",
                           (uintptr_t)log->event_query, (size_t)size, wevt_extended_status());
                    return false;
                }

                // EvtNext() returns true when it can full the array
                // so, let's retry with a smaller array.
                size /= 2;
                if(size < 1) size = 1;
                continue;
            }
        }

        log->query_stats.event_count++;
        log->log_stats.event_count++;

        // cleanup any previous event data
        wevt_event_done(log);

        log->bookmark = log->batch.bk[log->batch.used];
        log->batch.bk[log->batch.used] = NULL;
        log->batch.used++;

        if(wevt_get_next_event_one(log, ev, full))
            return true;
        else {
            log->query_stats.failed_count++;
            log->log_stats.failed_count++;
            max_failures--;
        }
    }

    return false;
}

void wevt_query_done(WEVT_LOG *log) {
    // close the last working bookmark
    wevt_event_done(log);

    // close all batched bookmarks
    for(DWORD i = log->batch.used; i < log->batch.size ;i++) {
        if(log->batch.bk[i])
            EvtClose(log->batch.bk[i]);

        log->batch.bk[i] = NULL;
    }
    log->batch.used = 0;
    log->batch.size = 0;

    if (log->event_query) {
        EvtClose(log->event_query);
        log->event_query = NULL;
    }

    log->query_stats.event_count = 0;
    log->query_stats.failed_count = 0;
}

void wevt_closelog6(WEVT_LOG *log) {
    wevt_query_done(log);

    if (log->render_context)
        EvtClose(log->render_context);

    wevt_variant_cleanup(&log->ops.content);
    txt_unicode_cleanup(&log->ops.unicode);
    txt_utf8_cleanup(&log->ops.channel);
    txt_utf8_cleanup(&log->ops.provider);
    txt_utf8_cleanup(&log->ops.source);
    txt_utf8_cleanup(&log->ops.computer);
    txt_utf8_cleanup(&log->ops.user);

    txt_utf8_cleanup(&log->ops.event);
    txt_utf8_cleanup(&log->ops.level);
    txt_utf8_cleanup(&log->ops.keywords);
    txt_utf8_cleanup(&log->ops.opcode);
    txt_utf8_cleanup(&log->ops.task);
    txt_utf8_cleanup(&log->ops.xml);
    freez(log);
}

bool wevt_channel_retention(WEVT_LOG *log, const wchar_t *channel, EVT_RETENTION *retention) {
    bool ret = false;

    // get the number of the oldest record in the log
    // "EvtGetLogInfo()" does not work properly with "EvtLogOldestRecordNumber"
    // we have to get it from the first EventRecordID

    // query the eventlog
    log->event_query = EvtQuery(NULL, channel, NULL, EvtQueryChannelPath | EvtQueryForwardDirection);
    if (!log->event_query) {
        if (GetLastError() == ERROR_EVT_CHANNEL_NOT_FOUND)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() for retention failed, channel '%s' not found, cannot get retention, extended info: %s",
                   channel2utf8(channel), wevt_extended_status());
        else
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() for retention on channel '%s' failed, cannot get retention, extended info: %s",
                   channel2utf8(channel), wevt_extended_status());

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
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() for retention failed, channel '%s' not found, extended info: %s",
                   channel2utf8(channel), wevt_extended_status());
        else
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() for retention on channel '%s' failed, extended info: %s",
                   channel2utf8(channel), wevt_extended_status());

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
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtCreateRenderContext failed, extended info: %s", wevt_extended_status());
        freez(log);
        log = NULL;
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
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtOpenLog() on channel '%s' failed, extended info: %s",
               channel2utf8(channel), wevt_extended_status());
        goto cleanup;
    }

    // Get the file size of the log
    if (!EvtGetLogInfo(hLog, EvtLogFileSize, sizeof(evtVariant), &evtVariant, &bufferUsed)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtGetLogInfo() on channel '%s' failed, extended info: %s",
               channel2utf8(channel), wevt_extended_status());
        goto cleanup;
    }

    // Extract the file size from the EVT_VARIANT structure
    file_size = evtVariant.UInt64Val;

cleanup:
    if (hLog)
        EvtClose(hLog);

    return file_size;
}

bool wevt_query(WEVT_LOG *log, LPCWSTR channel, LPCWSTR query, EVT_QUERY_FLAGS direction) {
    wevt_query_done(log);
    log->log_stats.queries_count++;

    EVT_HANDLE hQuery = EvtQuery(NULL, channel, query, EvtQueryChannelPath | (direction & (EvtQueryReverseDirection | EvtQueryForwardDirection)) | EvtQueryTolerateQueryErrors);
    if (!hQuery) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() failed, query: %s | extended info: %s",
               query2utf8(query), wevt_extended_status());

        log->log_stats.queries_failed++;
        return false;
    }

    log->event_query = hQuery;
    return true;
}
