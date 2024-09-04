// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include <windows.h>
#include <winevt.h>
#include <stdio.h>
#include <stdlib.h>

// Link with Wevtapi.lib
#pragma comment(lib, "Wevtapi.lib")

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

// Function to enumerate and list all event channels
void ListEventChannels() {
    EVT_HANDLE hChannelEnum = NULL;
    LPWSTR pChannelPath = NULL;
    DWORD dwChannelBufferSize = 0;
    DWORD dwChannelBufferUsed = 0;
    DWORD status = ERROR_SUCCESS;

    // Open a handle to enumerate the event channels
    hChannelEnum = EvtOpenChannelEnum(NULL, 0);
    if (NULL == hChannelEnum) {
        printf("WINDOWS EVENTS: EvtOpenChannelEnum() failed with %"PRIu64"\n", (uint64_t)GetLastError());
        return;
    }

    // Enumerate all channels
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
                printf("WINDOWS EVENTS: EvtNextChannelPath() failed with %lu\n", status);
                goto Cleanup;
            }
        }

        // Convert channel path to UTF-8 and print it
        printf("Channel: %s\n", channel2utf8(pChannelPath));
    }

Cleanup:
    // Free allocated resources
    freez(pChannelPath);
    EvtClose(hChannelEnum);
}

int main(void) {
    ListEventChannels();
    return 0;
}
