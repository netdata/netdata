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

static void wevts_sources_to_json_array(BUFFER *wb) {
    EVT_HANDLE hChannelEnum = NULL;
    LPWSTR pChannelPath = NULL;
    DWORD dwChannelBufferSize = 0;
    DWORD dwChannelBufferUsed = 0;
    DWORD status = ERROR_SUCCESS;

    // Open a handle to enumerate the event channels
    hChannelEnum = EvtOpenChannelEnum(NULL, 0);
    if (NULL == hChannelEnum) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "WINDOWS EVENTS: EvtOpenChannelEnum() failed with %"PRIu64"\n",
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
            }
            else {
                nd_log(NDLS_COLLECTORS, NDLP_ERR,
                       "WINDOWS EVENTS: EvtNextChannelPath() failed with %"PRIu64"\n",
                       (uint64_t)status);
                goto cleanup;
            }
        }

        const char *name = channel2utf8(pChannelPath);
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "id", name);
            buffer_json_member_add_string(wb, "name", name);
            buffer_json_member_add_string(wb, "pill", "size");
            buffer_json_member_add_string(wb, "info", "");
        }
        buffer_json_object_close(wb); // options object
    }

cleanup:
    freez(pChannelPath);
    EvtClose(hChannelEnum);
}

int main(void) {
    return 0;
}
