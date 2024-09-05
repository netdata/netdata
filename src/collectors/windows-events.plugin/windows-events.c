// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include <windows.h>
#include <winevt.h>
#include <stdio.h>
#include <stdlib.h>

// Link with Wevtapi.lib
#pragma comment(lib, "Wevtapi.lib")

typedef enum {
    WEVTS_NONE               = 0,
    WEVTS_ALL                = (1 << 0),
} WEVT_SOURCE_TYPE;

#define WEVT_FUNCTION_DESCRIPTION    "View, search and analyze Microsoft Windows events."
#define WEVT_FUNCTION_NAME           "windows-events"

// functions needed by LQS
static WEVT_SOURCE_TYPE wevts_internal_source_type(const char *value);
static void wevts_sources_to_json_array(BUFFER *wb);

// structures needed by LQS
struct lqs_extension {};

// prepare LQS
#define LQS_FUNCTION_NAME           WEVT_FUNCTION_NAME
#define LQS_FUNCTION_DESCRIPTION    WEVT_FUNCTION_DESCRIPTION
#define LQS_DEFAULT_ITEMS_PER_QUERY 200
#define LQS_DEFAULT_ITEMS_SAMPLING  1000000
#define LQS_SOURCE_TYPE             WEVT_SOURCE_TYPE
#define LQS_SOURCE_TYPE_ALL         WEVTS_ALL
#define LQS_SOURCE_TYPE_NONE        WEVTS_NONE
#define LQS_FUNCTION_GET_INTERNAL_SOURCE_TYPE(value) wevts_internal_source_type(value)
#define LQS_FUNCTION_SOURCE_TO_JSON_ARRAY(wb) wevts_sources_to_json_array(wb)
#include "libnetdata/facets/logs_query_status.h"

#define	VAR_PROVIDER_NAME(p)			    (p[0].StringVal)
#define	VAR_SOURCE_NAME(p)			        (p[1].StringVal)
#define	VAR_RECORD_NUMBER(p)			    (p[2].UInt64Val)
#define	VAR_EVENT_ID(p)				        (p[3].UInt16Val)
#define	VAR_LEVEL(p)				        (p[4].ByteVal)
#define	VAR_KEYWORDS(p)				        (p[5].UInt64Val)
#define	VAR_TIME_CREATED(p)			        (p[6].FileTimeVal)
#define	VAR_EVENT_DATA_STRING(p)		    (p[7].StringVal)
#define	VAR_EVENT_DATA_STRING_ARRAY(p, i)	(p[7].StringArr[i])
#define	VAR_EVENT_DATA_TYPE(p)			    (p[7].Type)
#define	VAR_EVENT_DATA_COUNT(p)			    (p[7].Count)

// Function to convert a wide string (UTF-16) to a multibyte string (UTF-8)
static char *channel2utf8(LPWSTR wideStr) {
    static __thread char buffer[1024];

    if (wideStr) {
        if(WideCharToMultiByte(CP_UTF8, 0, wideStr, -1, buffer, sizeof(buffer), NULL, NULL) == 0)
            strncpyz(buffer, "[failed]", sizeof(buffer) -1);
    }
    else
        strncpyz(buffer, "[null]", sizeof(buffer) -1);

    return buffer;
}

static WEVT_SOURCE_TYPE wevts_internal_source_type(const char *value) {
    if(strcmp(value, "all") == 0)
        return WEVTS_ALL;

    return WEVTS_NONE;
}

typedef struct {
    uint64_t id;
} ND_EVT_EVENT_INFO;

typedef struct {
    uint64_t entries;

    ND_EVT_EVENT_INFO first_event;
    ND_EVT_EVENT_INFO last_event;
} ND_EVT_CHANNEL_INFO;

static bool nd_evt_get_event_info(EVT_HANDLE *event_query, EVT_HANDLE *render_context, ND_EVT_EVENT_INFO *ev) {
    DWORD		size_required_next = 0, size_required = 0, size = 0, bookmarkedCount = 0;
    EVT_VARIANT	*renderedContent = NULL;
    EVT_HANDLE	event_bookmark = NULL;
    bool ret = false;

    if (TRUE != EvtNext(*event_query, 1, &event_bookmark, INFINITE, 0, &size_required_next)) {
        // no data in eventlog
        goto cleanup;
    }

    // obtain the information from selected events
    if (TRUE != EvtRender(*render_context, event_bookmark, EvtRenderEventValues, size, renderedContent, &size_required, &bookmarkedCount)) {
        // information exceeds the allocated space
        if (ERROR_INSUFFICIENT_BUFFER != GetLastError()) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtRender failed #1");
            goto cleanup;
        }

        size = size_required;
        renderedContent = (EVT_VARIANT *)mallocz(size);

        if (TRUE != EvtRender(*render_context, event_bookmark, EvtRenderEventValues, size, renderedContent, &size_required, &bookmarkedCount)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtRender failed #2");
            goto cleanup;
        }
    }

    ev->id = VAR_RECORD_NUMBER(renderedContent);
    ret = true;

cleanup:
    if (NULL != event_bookmark)
        EvtClose(event_bookmark);

    free(renderedContent);
    return ret;
}

/* opens Event Log using API 6 and returns number of records */
static bool nd_evt_get_channel_info(const wchar_t *channel, ND_EVT_CHANNEL_INFO *ch) {
    const wchar_t	*RENDER_ITEMS[] = {
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
    EVT_HANDLE tmp_render_context = NULL, tmp_first_event_query = NULL, tmp_last_event_query = NULL;
    bool ret = false;

    // get the number of the oldest record in the log
    // "EvtGetLogInfo()" does not work properly with "EvtLogOldestRecordNumber"
    // we have to get it from the first EventRecordID

    // create the system render
    if (NULL == (tmp_render_context = EvtCreateRenderContext(RENDER_ITEMS_count, RENDER_ITEMS, EvtRenderContextValues))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtCreateRenderContext failed.");
        goto cleanup;
    }

    // query the eventlog
    if (NULL == (tmp_first_event_query = EvtQuery(NULL, channel, NULL, EvtQueryChannelPath))) {
        if (ERROR_EVT_CHANNEL_NOT_FOUND == GetLastError())
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery channel missed");
        else
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery failed");

        goto cleanup;
    }

    if (!nd_evt_get_event_info(&tmp_first_event_query, &tmp_render_context, &ch->first_event))
        goto cleanup;

    if (!ch->first_event.id) {
        // no data in the event log
        ch->first_event.id = ch->last_event.id = 0;
        ret = true;
        goto cleanup;
    }

    if (NULL == (tmp_last_event_query = EvtQuery(NULL, channel, NULL, EvtQueryChannelPath | EvtQueryReverseDirection))) {
        if (ERROR_EVT_CHANNEL_NOT_FOUND == GetLastError())
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery channel missed");
        else
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtQuery failed");

        goto cleanup;
    }

    if (!nd_evt_get_event_info(&tmp_last_event_query, &tmp_render_context, &ch->last_event) || ch->last_event.id == 0) {
        // no data in eventlog
        ch->last_event.id = ch->first_event.id;
    }
    else
        ch->last_event.id += 1;	// we should read the last record

    ret = true;

cleanup:
    if(ret)
        ch->entries = ch->last_event.id - ch->first_event.id;
    else
        ch->entries = 0;

    if (NULL != tmp_first_event_query)
        EvtClose(tmp_first_event_query);
    if (NULL != tmp_last_event_query)
        EvtClose(tmp_last_event_query);
    if (NULL != tmp_render_context)
        EvtClose(tmp_render_context);

    return ret;
}

static void wevts_sources_to_json_array(BUFFER *wb) {
    EVT_HANDLE hChannelEnum = NULL;
    LPWSTR pChannelPath = NULL;
    DWORD dwChannelBufferSize = 0;
    DWORD dwChannelBufferUsed = 0;
    DWORD status = ERROR_SUCCESS;

    // Open a handle to enumerate the event channels
    hChannelEnum = EvtOpenChannelEnum(NULL, 0);
    if (NULL == hChannelEnum) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "WINDOWS EVENTS: EvtOpenChannelEnum() failed with %" PRIu64 "\n",
               (uint64_t)GetLastError());
        goto cleanup;
    }

    while (true) {
        if (!EvtNextChannelPath(hChannelEnum, dwChannelBufferSize, pChannelPath, &dwChannelBufferUsed)) {
            status = GetLastError();
            if (status == ERROR_NO_MORE_ITEMS)
                break; // No more channels
            else if (status == ERROR_INSUFFICIENT_BUFFER) {
                dwChannelBufferSize = dwChannelBufferUsed;
                pChannelPath = (LPWSTR)reallocz(pChannelPath, dwChannelBufferSize * sizeof(WCHAR));
                continue;
            } else {
                nd_log(NDLS_COLLECTORS, NDLP_ERR,
                       "WINDOWS EVENTS: EvtNextChannelPath() failed\n");
                goto cleanup;
            }
        }

        ND_EVT_CHANNEL_INFO ch;
        if(!nd_evt_get_channel_info(pChannelPath, &ch) || !ch.entries)
            continue;

        const char *name = channel2utf8(pChannelPath);
        buffer_json_add_array_item_object(wb);
        {
            char info[1024];
            snprintfz(info, sizeof(info), "%"PRIu64" entries", ch.entries);

            buffer_json_member_add_string(wb, "id", name);
            buffer_json_member_add_string(wb, "name", name);
            buffer_json_member_add_string(wb, "pill", "???");
            buffer_json_member_add_string(wb, "info", info);
        }
        buffer_json_object_close(wb); // options object
    }

cleanup:
    freez(pChannelPath);
    EvtClose(hChannelEnum);
}

int main(void) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_array(wb, "sources");
    {
        wevts_sources_to_json_array(wb);
    }
    buffer_json_array_close(wb); // sources
    buffer_json_finalize(wb);

    printf("%s\n", buffer_tostring(wb));
    return 0;
}
