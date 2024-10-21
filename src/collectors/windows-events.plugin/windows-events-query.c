// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events.h"

static void wevt_event_done(WEVT_LOG *log);

static uint64_t wevt_log_file_size(const wchar_t *channel);

// --------------------------------------------------------------------------------------------------------------------

static const char *EvtGetExtendedStatus_utf8(void) {
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

// --------------------------------------------------------------------------------------------------------------------

bool EvtFormatMessage_utf16(
    TXT_UTF16 *dst, EVT_HANDLE hMetadata, EVT_HANDLE hEvent, DWORD dwMessageId, EVT_FORMAT_MESSAGE_FLAGS flags) {
    dst->used = 0;

    DWORD size = 0;
    if(!dst->data) {
        EvtFormatMessage(hMetadata, hEvent, dwMessageId, 0, NULL, flags, 0, NULL, &size);
        if(!size) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() to get message size failed.");
            goto cleanup;
        }
        txt_utf16_resize(dst, size, false);
    }

    // First, try to get the message using the existing buffer
    if (!EvtFormatMessage(hMetadata, hEvent, dwMessageId, 0, NULL, flags, dst->size, dst->data, &size) || !dst->data) {
        if (dst->data && GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() failed.");
            goto cleanup;
        }

        // Try again with the resized buffer
        txt_utf16_resize(dst, size, false);
        if (!EvtFormatMessage(hMetadata, hEvent, dwMessageId, 0, NULL, flags, dst->size, dst->data, &size)) {
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() failed after resizing buffer.");
            goto cleanup;
        }
    }

    // make sure it is null terminated
    if(size <= dst->size)
        dst->data[size - 1] = 0;
    else
        dst->data[dst->size - 1] = 0;

    // unfortunately we have to calculate the length every time
    // the size returned may not be the length of the dst string
    dst->used = wcslen(dst->data) + 1;

    return true;

cleanup:
    dst->used = 0;
    return false;
}

static bool EvtFormatMessage_utf8(
    TXT_UTF16 *tmp, PROVIDER_META_HANDLE *p, EVT_HANDLE hEvent,
        TXT_UTF8 *dst, EVT_FORMAT_MESSAGE_FLAGS flags) {

    dst->src = TXT_SOURCE_EVENT_LOG;

    if(EvtFormatMessage_utf16(tmp, provider_handle(p), hEvent, 0, flags))
        return txt_utf16_to_utf8(dst, tmp);

    txt_utf8_empty(dst);
    return false;
}

bool EvtFormatMessage_Event_utf8(TXT_UTF16 *tmp, PROVIDER_META_HANDLE *p, EVT_HANDLE hEvent, TXT_UTF8 *dst) {
    return EvtFormatMessage_utf8(tmp, p, hEvent, dst, EvtFormatMessageEvent);
}

bool EvtFormatMessage_Xml_utf8(TXT_UTF16 *tmp, PROVIDER_META_HANDLE *p, EVT_HANDLE hEvent, TXT_UTF8 *dst) {
    return EvtFormatMessage_utf8(tmp, p, hEvent, dst, EvtFormatMessageXml);
}

// --------------------------------------------------------------------------------------------------------------------

static void wevt_get_field_from_cache(
        WEVT_LOG *log, uint64_t value, PROVIDER_META_HANDLE *h,
        TXT_UTF8 *dst, const ND_UUID *provider,
        WEVT_FIELD_TYPE cache_type, EVT_FORMAT_MESSAGE_FLAGS flags) {

    if (field_cache_get(cache_type, provider, value, dst))
        return;

    EvtFormatMessage_utf8(&log->ops.unicode, h, log->hEvent, dst, flags);
    field_cache_set(cache_type, provider, value, dst);
}

// --------------------------------------------------------------------------------------------------------------------
// Level

#define SET_LEN_AND_RETURN(constant) *len = sizeof(constant) - 1; return constant

static inline const char *wevt_level_hardcoded(uint64_t level, size_t *len) {
    switch(level) {
        case WEVT_LEVEL_NONE:        SET_LEN_AND_RETURN(WEVT_LEVEL_NAME_NONE);
        case WEVT_LEVEL_CRITICAL:    SET_LEN_AND_RETURN(WEVT_LEVEL_NAME_CRITICAL);
        case WEVT_LEVEL_ERROR:       SET_LEN_AND_RETURN(WEVT_LEVEL_NAME_ERROR);
        case WEVT_LEVEL_WARNING:     SET_LEN_AND_RETURN(WEVT_LEVEL_NAME_WARNING);
        case WEVT_LEVEL_INFORMATION: SET_LEN_AND_RETURN(WEVT_LEVEL_NAME_INFORMATION);
        case WEVT_LEVEL_VERBOSE:     SET_LEN_AND_RETURN(WEVT_LEVEL_NAME_VERBOSE);
        default: *len = 0; return NULL;
    }
}

static void wevt_get_level(WEVT_LOG *log, WEVT_EVENT *ev, PROVIDER_META_HANDLE *h) {
    TXT_UTF8 *dst = &log->ops.level;
    uint64_t value = ev->level;

    txt_utf8_empty(dst);

    EVT_FORMAT_MESSAGE_FLAGS flags = EvtFormatMessageLevel;
    WEVT_FIELD_TYPE cache_type = WEVT_FIELD_TYPE_LEVEL;
    bool is_provider = is_valid_provider_level(value, true);

    if(!is_provider) {
        size_t len;
        const char *hardcoded = wevt_level_hardcoded(value, &len);
        if(hardcoded) {
            txt_utf8_set(dst, hardcoded,  len);
            dst->src = TXT_SOURCE_HARDCODED;
        }
        else {
            // since this is not a provider value
            // we expect to get the system description of it
            wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
        }
    }
    else if (!provider_get_level(dst, h, value)) {
        // not found in the manifest, get it from the cache
        wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
    }

    txt_utf8_set_numeric_if_empty(
            dst, WEVT_PREFIX_LEVEL, sizeof(WEVT_PREFIX_LEVEL) - 1, ev->level);
}

// --------------------------------------------------------------------------------------------------------------------
// Opcode

static inline const char *wevt_opcode_hardcoded(uint64_t opcode, size_t *len) {
    switch(opcode) {
        case WEVT_OPCODE_INFO:      SET_LEN_AND_RETURN(WEVT_OPCODE_NAME_INFO);
        case WEVT_OPCODE_START:     SET_LEN_AND_RETURN(WEVT_OPCODE_NAME_START);
        case WEVT_OPCODE_STOP:      SET_LEN_AND_RETURN(WEVT_OPCODE_NAME_STOP);
        case WEVT_OPCODE_DC_START:  SET_LEN_AND_RETURN(WEVT_OPCODE_NAME_DC_START);
        case WEVT_OPCODE_DC_STOP:   SET_LEN_AND_RETURN(WEVT_OPCODE_NAME_DC_STOP);
        case WEVT_OPCODE_EXTENSION: SET_LEN_AND_RETURN(WEVT_OPCODE_NAME_EXTENSION);
        case WEVT_OPCODE_REPLY:     SET_LEN_AND_RETURN(WEVT_OPCODE_NAME_REPLY);
        case WEVT_OPCODE_RESUME:    SET_LEN_AND_RETURN(WEVT_OPCODE_NAME_RESUME);
        case WEVT_OPCODE_SUSPEND:   SET_LEN_AND_RETURN(WEVT_OPCODE_NAME_SUSPEND);
        case WEVT_OPCODE_SEND:      SET_LEN_AND_RETURN(WEVT_OPCODE_NAME_SEND);
        case WEVT_OPCODE_RECEIVE:   SET_LEN_AND_RETURN(WEVT_OPCODE_NAME_RECEIVE);
        default: *len = 0; return NULL;
    }
}

static void wevt_get_opcode(WEVT_LOG *log, WEVT_EVENT *ev, PROVIDER_META_HANDLE *h) {
    TXT_UTF8 *dst = &log->ops.opcode;
    uint64_t value = ev->opcode;

    txt_utf8_empty(dst);

    EVT_FORMAT_MESSAGE_FLAGS flags = EvtFormatMessageOpcode;
    WEVT_FIELD_TYPE cache_type = WEVT_FIELD_TYPE_OPCODE;
    bool is_provider = is_valid_provider_opcode(value, true);

    if(!is_provider) {
        size_t len;
        const char *hardcoded = wevt_opcode_hardcoded(value, &len);
        if(hardcoded) {
            txt_utf8_set(dst, hardcoded,  len);
            dst->src = TXT_SOURCE_HARDCODED;
        }
        else {
            // since this is not a provider value
            // we expect to get the system description of it
            wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
        }
    }
    else if (!provider_get_opcode(dst, h, value)) {
        // not found in the manifest, get it from the cache
        wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
    }

    txt_utf8_set_numeric_if_empty(
            dst, WEVT_PREFIX_OPCODE, sizeof(WEVT_PREFIX_OPCODE) - 1, ev->opcode);
}

// --------------------------------------------------------------------------------------------------------------------
// Task

static const char *wevt_task_hardcoded(uint64_t task, size_t *len) {
    switch(task) {
        case WEVT_TASK_NONE: SET_LEN_AND_RETURN(WEVT_TASK_NAME_NONE);
        default: *len = 0; return NULL;
    }
}

static void wevt_get_task(WEVT_LOG *log, WEVT_EVENT *ev, PROVIDER_META_HANDLE *h) {
    TXT_UTF8 *dst = &log->ops.task;
    uint64_t value = ev->task;

    txt_utf8_empty(dst);

    EVT_FORMAT_MESSAGE_FLAGS flags = EvtFormatMessageTask;
    WEVT_FIELD_TYPE cache_type = WEVT_FIELD_TYPE_TASK;
    bool is_provider = is_valid_provider_task(value, true);

    if(!is_provider) {
        size_t len;
        const char *hardcoded = wevt_task_hardcoded(value, &len);
        if(hardcoded) {
            txt_utf8_set(dst, hardcoded,  len);
            dst->src = TXT_SOURCE_HARDCODED;
        }
        else {
            // since this is not a provider value
            // we expect to get the system description of it
            wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
        }
    }
    else if (!provider_get_task(dst, h, value)) {
        // not found in the manifest, get it from the cache
        wevt_get_field_from_cache(log, value, h, dst, &ev->provider, cache_type, flags);
    }

    txt_utf8_set_numeric_if_empty(
            dst, WEVT_PREFIX_TASK, sizeof(WEVT_PREFIX_TASK) - 1, ev->task);
}

// --------------------------------------------------------------------------------------------------------------------
// Keyword

#define SET_BITS(msk, txt) { .mask = msk, .name = txt, .len = sizeof(txt) - 1, }

static uint64_t wevt_keyword_handle_reserved(uint64_t value, TXT_UTF8 *dst) {
    struct {
        uint64_t mask;
        const char *name;
        size_t len;
    } bits[] = {
        SET_BITS(WEVT_KEYWORD_EVENTLOG_CLASSIC, WEVT_KEYWORD_NAME_EVENTLOG_CLASSIC),
        SET_BITS(WEVT_KEYWORD_CORRELATION_HINT, WEVT_KEYWORD_NAME_CORRELATION_HINT),
        SET_BITS(WEVT_KEYWORD_AUDIT_SUCCESS, WEVT_KEYWORD_NAME_AUDIT_SUCCESS),
        SET_BITS(WEVT_KEYWORD_AUDIT_FAILURE, WEVT_KEYWORD_NAME_AUDIT_FAILURE),
        SET_BITS(WEVT_KEYWORD_SQM, WEVT_KEYWORD_NAME_SQM),
        SET_BITS(WEVT_KEYWORD_WDI_DIAG, WEVT_KEYWORD_NAME_WDI_DIAG),
        SET_BITS(WEVT_KEYWORD_WDI_CONTEXT, WEVT_KEYWORD_NAME_WDI_CONTEXT),
        SET_BITS(WEVT_KEYWORD_RESPONSE_TIME, WEVT_KEYWORD_NAME_RESPONSE_TIME),
    };

    txt_utf8_empty(dst);

    for(size_t i = 0; i < sizeof(bits) / sizeof(bits[0]) ;i++) {
        if((value & bits[i].mask) == bits[i].mask) {
            txt_utf8_add_keywords_separator_if_needed(dst);
            txt_utf8_append(dst, bits[i].name, bits[i].len);
            value &= ~(bits[i].mask);
            dst->src = TXT_SOURCE_HARDCODED;
        }
    }

    // return it without any remaining reserved bits
    return value & 0x0000FFFFFFFFFFFF;
}

static void wevt_get_keyword(WEVT_LOG *log, WEVT_EVENT *ev, PROVIDER_META_HANDLE *h) {
    TXT_UTF8 *dst = &log->ops.keywords;

    if(ev->keywords == WEVT_KEYWORD_NONE) {
        txt_utf8_set(dst, WEVT_KEYWORD_NAME_NONE, sizeof(WEVT_KEYWORD_NAME_NONE) - 1);
        dst->src = TXT_SOURCE_HARDCODED;
    }

    uint64_t value = wevt_keyword_handle_reserved(ev->keywords, dst);

    EVT_FORMAT_MESSAGE_FLAGS flags = EvtFormatMessageKeyword;
    WEVT_FIELD_TYPE cache_type = WEVT_FIELD_TYPE_KEYWORD;

    if(!value && dst->used <= 1) {
        // no hardcoded info in the buffer, make it None
        txt_utf8_set(dst, WEVT_KEYWORD_NAME_NONE, sizeof(WEVT_KEYWORD_NAME_NONE) - 1);
        dst->src = TXT_SOURCE_HARDCODED;
    }
    else if (value && !provider_get_keywords(dst, h, value) && dst->used <= 1) {
        // the provider did not provide any info and the description is still empty.
        // the system returns 1 keyword, the highest bit, not a list
        // so, when we call the system, we pass the original value (ev->keywords)
        wevt_get_field_from_cache(log, ev->keywords, h, dst, &ev->provider, cache_type, flags);
    }

    txt_utf8_set_hex_if_empty(
            dst, WEVT_PREFIX_KEYWORDS, sizeof(WEVT_PREFIX_KEYWORDS) - 1, ev->keywords);
}

// --------------------------------------------------------------------------------------------------------------------
// Fetching Events

static inline bool wEvtRender(WEVT_LOG *log, EVT_HANDLE context, WEVT_VARIANT *raw) {
    DWORD bytes_used = 0, property_count = 0;
    if (!EvtRender(context, log->hEvent, EvtRenderEventValues, raw->size, raw->data, &bytes_used, &property_count)) {
        // information exceeds the allocated space
        if (GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "EvtRender() failed, hRenderSystemContext: 0x%lx, hEvent: 0x%lx, content: 0x%lx, size: %u, extended info: %s",
                   (uintptr_t)context, (uintptr_t)log->hEvent, (uintptr_t)raw->data, raw->size,
                   EvtGetExtendedStatus_utf8());
            return false;
        }

        wevt_variant_resize(raw, bytes_used);
        if (!EvtRender(context, log->hEvent, EvtRenderEventValues, raw->size, raw->data, &bytes_used, &property_count)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "EvtRender() failed, after bytes_used increase, extended info: %s",
                   EvtGetExtendedStatus_utf8());
            return false;
        }
    }
    raw->used = bytes_used;
    raw->count = property_count;

    return true;
}

static bool wevt_get_next_event_one(WEVT_LOG *log, WEVT_EVENT *ev) {
    bool ret = false;

    if(!wEvtRender(log, log->hRenderSystemContext, &log->ops.raw.system))
        goto cleanup;

    EVT_VARIANT *content = log->ops.raw.system.data;

    ev->id          = wevt_field_get_uint64(&content[EvtSystemEventRecordId]);
    ev->event_id    = wevt_field_get_uint16(&content[EvtSystemEventID]);
    ev->level       = wevt_field_get_uint8(&content[EvtSystemLevel]);
    ev->opcode      = wevt_field_get_uint8(&content[EvtSystemOpcode]);
    ev->keywords    = wevt_field_get_uint64_hex(&content[EvtSystemKeywords]);
    ev->version     = wevt_field_get_uint8(&content[EvtSystemVersion]);
    ev->task        = wevt_field_get_uint16(&content[EvtSystemTask]);
    ev->qualifiers  = wevt_field_get_uint16(&content[EvtSystemQualifiers]);
    ev->process_id  = wevt_field_get_uint32(&content[EvtSystemProcessID]);
    ev->thread_id   = wevt_field_get_uint32(&content[EvtSystemThreadID]);
    ev->created_ns  = wevt_field_get_filetime_to_ns(&content[EvtSystemTimeCreated]);

    if(log->type & WEVT_QUERY_EXTENDED) {
        wevt_field_get_string_utf8(&content[EvtSystemChannel], &log->ops.channel);
        wevt_field_get_string_utf8(&content[EvtSystemComputer], &log->ops.computer);
        wevt_field_get_string_utf8(&content[EvtSystemProviderName], &log->ops.provider);
        wevt_get_uuid_by_type(&content[EvtSystemProviderGuid], &ev->provider);
        wevt_get_uuid_by_type(&content[EvtSystemActivityID], &ev->activity_id);
        wevt_get_uuid_by_type(&content[EvtSystemRelatedActivityID], &ev->related_activity_id);
        wevt_field_get_sid(&content[EvtSystemUserID], &log->ops.account, &log->ops.domain, &log->ops.sid);

        PROVIDER_META_HANDLE *p = log->provider =
                provider_get(ev->provider, content[EvtSystemProviderName].StringVal);

        ev->platform = provider_get_platform(p);

        wevt_get_level(log, ev, p);
        wevt_get_task(log, ev, p);
        wevt_get_opcode(log, ev, p);
        wevt_get_keyword(log, ev, p);

        if(log->type & WEVT_QUERY_EVENT_DATA && wEvtRender(log, log->hRenderUserContext, &log->ops.raw.user)) {
#if (ON_FTS_PRELOAD_MESSAGE == 1)
            EvtFormatMessage_Event_utf8(&log->ops.unicode, log->provider, log->hEvent, &log->ops.event);
#endif
#if (ON_FTS_PRELOAD_XML == 1)
            EvtFormatMessage_Xml_utf8(&log->ops.unicode, log->provider, log->hEvent, &log->ops.xml);
#endif
#if (ON_FTS_PRELOAD_EVENT_DATA == 1)
            for(size_t i = 0; i < log->ops.raw.user.count ;i++)
                evt_variant_to_buffer(log->ops.event_data, &log->ops.raw.user.data[i], " ||| ");
#endif
        }
    }

    ret = true;

cleanup:
    return ret;
}

bool wevt_get_next_event(WEVT_LOG *log, WEVT_EVENT *ev) {
    DWORD size = (log->type & WEVT_QUERY_EXTENDED) ? BATCH_NEXT_EVENT : 1;
    DWORD max_failures = 10;

    fatal_assert(log && log->hQuery && log->hRenderSystemContext);

    while(max_failures > 0) {
        if (log->batch.used >= log->batch.size) {
            log->batch.size = 0;
            log->batch.used = 0;
            DWORD err;
            if(!EvtNext(log->hQuery, size, log->batch.hEvents, INFINITE, 0, &log->batch.size)) {
                err = GetLastError();
                if(err == ERROR_NO_MORE_ITEMS)
                    return false; // no data available, return failure
            }

            if(!log->batch.size) {
                if(size == 1) {
                    nd_log(NDLS_COLLECTORS, NDLP_ERR,
                           "EvtNext() failed, hQuery: 0x%lx, size: %zu, extended info: %s",
                           (uintptr_t)log->hQuery, (size_t)size, EvtGetExtendedStatus_utf8());
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

        log->hEvent = log->batch.hEvents[log->batch.used];
        log->batch.hEvents[log->batch.used] = NULL;
        log->batch.used++;

        if(wevt_get_next_event_one(log, ev))
            return true;
        else {
            log->query_stats.failed_count++;
            log->log_stats.failed_count++;
            max_failures--;
        }
    }

    return false;
}

static void wevt_event_done(WEVT_LOG *log) {
    if (log->provider) {
        provider_release(log->provider);
        log->provider = NULL;
    }

    if (log->hEvent) {
        EvtClose(log->hEvent);
        log->hEvent = NULL;
    }

    log->ops.channel.src = TXT_SOURCE_UNKNOWN;
    log->ops.provider.src = TXT_SOURCE_UNKNOWN;
    log->ops.computer.src = TXT_SOURCE_UNKNOWN;
    log->ops.account.src = TXT_SOURCE_UNKNOWN;
    log->ops.domain.src = TXT_SOURCE_UNKNOWN;
    log->ops.sid.src = TXT_SOURCE_UNKNOWN;

    log->ops.event.src = TXT_SOURCE_UNKNOWN;
    log->ops.level.src = TXT_SOURCE_UNKNOWN;
    log->ops.keywords.src = TXT_SOURCE_UNKNOWN;
    log->ops.opcode.src = TXT_SOURCE_UNKNOWN;
    log->ops.task.src = TXT_SOURCE_UNKNOWN;
    log->ops.xml.src = TXT_SOURCE_UNKNOWN;

    log->ops.channel.used = 0;
    log->ops.provider.used = 0;
    log->ops.computer.used = 0;
    log->ops.account.used = 0;
    log->ops.domain.used = 0;
    log->ops.sid.used = 0;

    log->ops.event.used = 0;
    log->ops.level.used = 0;
    log->ops.keywords.used = 0;
    log->ops.opcode.used = 0;
    log->ops.task.used = 0;
    log->ops.xml.used = 0;

    if(log->ops.event_data)
        log->ops.event_data->len = 0;
}

// --------------------------------------------------------------------------------------------------------------------
// Query management

bool wevt_query(WEVT_LOG *log, LPCWSTR channel, LPCWSTR query, EVT_QUERY_FLAGS direction) {
    wevt_query_done(log);
    log->log_stats.queries_count++;

    EVT_HANDLE hQuery = EvtQuery(NULL, channel, query, EvtQueryChannelPath | (direction & (EvtQueryReverseDirection | EvtQueryForwardDirection)) | EvtQueryTolerateQueryErrors);
    if (!hQuery) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() failed, query: %s | extended info: %s",
               query2utf8(query), EvtGetExtendedStatus_utf8());

        log->log_stats.queries_failed++;
        return false;
    }

    log->hQuery = hQuery;
    return true;
}

void wevt_query_done(WEVT_LOG *log) {
    // close the last working hEvent
    wevt_event_done(log);

    // close all batched hEvents
    for(DWORD i = log->batch.used; i < log->batch.size ;i++) {
        if(log->batch.hEvents[i])
            EvtClose(log->batch.hEvents[i]);

        log->batch.hEvents[i] = NULL;
    }
    log->batch.used = 0;
    log->batch.size = 0;

    if (log->hQuery) {
        EvtClose(log->hQuery);
        log->hQuery = NULL;
    }

    log->query_stats.event_count = 0;
    log->query_stats.failed_count = 0;
}

// --------------------------------------------------------------------------------------------------------------------
// Log management

WEVT_LOG *wevt_openlog6(WEVT_QUERY_TYPE type) {
    WEVT_LOG *log = callocz(1, sizeof(*log));
    log->type = type;

    // create the system render
    log->hRenderSystemContext = EvtCreateRenderContext(0, NULL, EvtRenderContextSystem);
    if (!log->hRenderSystemContext) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "EvtCreateRenderContext() on system context failed, extended info: %s",
               EvtGetExtendedStatus_utf8());
        goto cleanup;
    }

    if(type & WEVT_QUERY_EVENT_DATA) {
        log->hRenderUserContext = EvtCreateRenderContext(0, NULL, EvtRenderContextUser);
        if (!log->hRenderUserContext) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "EvtCreateRenderContext failed, on user context failed, extended info: %s",
                   EvtGetExtendedStatus_utf8());
            goto cleanup;
        }

        log->ops.event_data = buffer_create(4096, NULL);
    }

    return log;

cleanup:
    wevt_closelog6(log);
    return NULL;
}

void wevt_closelog6(WEVT_LOG *log) {
    wevt_query_done(log);

    if (log->hRenderSystemContext)
        EvtClose(log->hRenderSystemContext);

    if (log->hRenderUserContext)
        EvtClose(log->hRenderUserContext);

    wevt_variant_cleanup(&log->ops.raw.system);
    wevt_variant_cleanup(&log->ops.raw.user);
    txt_utf16_cleanup(&log->ops.unicode);
    txt_utf8_cleanup(&log->ops.channel);
    txt_utf8_cleanup(&log->ops.provider);
    txt_utf8_cleanup(&log->ops.computer);
    txt_utf8_cleanup(&log->ops.account);
    txt_utf8_cleanup(&log->ops.domain);
    txt_utf8_cleanup(&log->ops.sid);

    txt_utf8_cleanup(&log->ops.event);
    txt_utf8_cleanup(&log->ops.level);
    txt_utf8_cleanup(&log->ops.keywords);
    txt_utf8_cleanup(&log->ops.opcode);
    txt_utf8_cleanup(&log->ops.task);
    txt_utf8_cleanup(&log->ops.xml);

    buffer_free(log->ops.event_data);

    freez(log);
}

// --------------------------------------------------------------------------------------------------------------------
// Retention

bool wevt_channel_retention(WEVT_LOG *log, const wchar_t *channel, const wchar_t *query, EVT_RETENTION *retention) {
    bool ret = false;

    // get the number of the oldest record in the log
    // "EvtGetLogInfo()" does not work properly with "EvtLogOldestRecordNumber"
    // we have to get it from the first EventRecordID

    // query the eventlog
    log->hQuery = EvtQuery(NULL, channel, query, EvtQueryChannelPath | EvtQueryForwardDirection | EvtQueryTolerateQueryErrors);
    if (!log->hQuery) {
        if (GetLastError() == ERROR_EVT_CHANNEL_NOT_FOUND)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() for retention failed, channel '%s' not found, cannot get retention, extended info: %s",
                   channel2utf8(channel), EvtGetExtendedStatus_utf8());
        else
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() for retention on channel '%s' failed, cannot get retention, extended info: %s",
                   channel2utf8(channel), EvtGetExtendedStatus_utf8());

        goto cleanup;
    }

    if (!wevt_get_next_event(log, &retention->first_event))
        goto cleanup;

    if (!retention->first_event.id) {
        // no data in the event log
        retention->first_event = retention->last_event = WEVT_EVENT_EMPTY;
        ret = true;
        goto cleanup;
    }
    EvtClose(log->hQuery);

    log->hQuery = EvtQuery(NULL, channel, query, EvtQueryChannelPath | EvtQueryReverseDirection | EvtQueryTolerateQueryErrors);
    if (!log->hQuery) {
        if (GetLastError() == ERROR_EVT_CHANNEL_NOT_FOUND)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() for retention failed, channel '%s' not found, extended info: %s",
                   channel2utf8(channel), EvtGetExtendedStatus_utf8());
        else
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery() for retention on channel '%s' failed, extended info: %s",
                   channel2utf8(channel), EvtGetExtendedStatus_utf8());

        goto cleanup;
    }

    if (!wevt_get_next_event(log, &retention->last_event) || retention->last_event.id == 0) {
        // no data in eventlog
        retention->last_event = retention->first_event;
    }
    retention->last_event.id += 1;	// we should read the last record
    ret = true;

cleanup:
    wevt_query_done(log);

    if(ret) {
        retention->entries = (channel && !query) ? retention->last_event.id - retention->first_event.id : 0;

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

static uint64_t wevt_log_file_size(const wchar_t *channel) {
    EVT_HANDLE hLog = NULL;
    EVT_VARIANT evtVariant;
    DWORD bufferUsed = 0;
    uint64_t file_size = 0;

    // Open the event log channel
    hLog = EvtOpenLog(NULL, channel, EvtOpenChannelPath);
    if (!hLog) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtOpenLog() on channel '%s' failed, extended info: %s",
               channel2utf8(channel), EvtGetExtendedStatus_utf8());
        goto cleanup;
    }

    // Get the file size of the log
    if (!EvtGetLogInfo(hLog, EvtLogFileSize, sizeof(evtVariant), &evtVariant, &bufferUsed)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtGetLogInfo() on channel '%s' failed, extended info: %s",
               channel2utf8(channel), EvtGetExtendedStatus_utf8());
        goto cleanup;
    }

    // Extract the file size from the EVT_VARIANT structure
    file_size = evtVariant.UInt64Val;

cleanup:
    if (hLog)
        EvtClose(hLog);

    return file_size;
}
