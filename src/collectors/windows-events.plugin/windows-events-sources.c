// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-sources.h"

void wevt_sources_to_json_array(BUFFER *wb) {
    EVT_HANDLE hChannelEnum = NULL;
    LPWSTR channel = NULL;
    DWORD dwChannelBufferSize = 0;
    DWORD dwChannelBufferUsed = 0;
    DWORD status = ERROR_SUCCESS;

    // Open a handle to enumerate the event channels
    hChannelEnum = EvtOpenChannelEnum(NULL, 0);
    if (!hChannelEnum) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "WINDOWS EVENTS: EvtOpenChannelEnum() failed with %" PRIu64 "\n",
                (uint64_t)GetLastError());
        goto cleanup;
    }

    while (true) {
        if (!EvtNextChannelPath(hChannelEnum, dwChannelBufferSize, channel, &dwChannelBufferUsed)) {
            status = GetLastError();
            if (status == ERROR_NO_MORE_ITEMS)
                break; // No more channels
            else if (status == ERROR_INSUFFICIENT_BUFFER) {
                dwChannelBufferSize = dwChannelBufferUsed;
                channel = (LPWSTR)reallocz(channel, dwChannelBufferSize * sizeof(WCHAR));
                continue;
            } else {
                nd_log(NDLS_COLLECTORS, NDLP_ERR,
                       "WINDOWS EVENTS: EvtNextChannelPath() failed\n");
                goto cleanup;
            }
        }

        WEVT_LOG *log = wevt_openlog6(channel, true);
        if(!log) continue;

        const char *name = channel2utf8(channel);
        buffer_json_add_array_item_object(wb);
        {
            char info[1024], duration[128], pill[128];
            duration_snprintf(duration, sizeof(duration), (int64_t)(log->retention.duration_ns / NSEC_PER_SEC), "s", true);
            snprintfz(info, sizeof(info), "%"PRIu64" entries, covering %s", log->retention.entries, duration);

            if(log->retention.size_bytes)
                size_snprintf(pill, sizeof(pill), log->retention.size_bytes, "B", false);
            else
                pill[0] = '\0';

            buffer_json_member_add_string(wb, "id", name);
            buffer_json_member_add_string(wb, "name", name);
            buffer_json_member_add_string(wb, "pill", pill);
            buffer_json_member_add_string(wb, "info", info);
        }
        buffer_json_object_close(wb); // options object
    }

    cleanup:
    freez(channel);
    EvtClose(hChannelEnum);
}
