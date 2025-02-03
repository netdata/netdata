// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define _COMMON_PLUGIN_NAME "windows.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "GetSystemRam"
#include "../common-contexts/common-contexts.h"

int do_GetSystemRAM(int update_every, usec_t dt __maybe_unused)
{
    MEMORYSTATUSEX memStat = {0};
    memStat.dwLength = sizeof(memStat);

    if (!GlobalMemoryStatusEx(&memStat)) {
        netdata_log_error("GlobalMemoryStatusEx() failed.");
        return 1;
    }

    {
        ULONGLONG total_bytes = memStat.ullTotalPhys;
        ULONGLONG free_bytes = memStat.ullAvailPhys;
        ULONGLONG used_bytes = total_bytes - free_bytes;
        common_system_ram(free_bytes, used_bytes, update_every);
    }

    {
        DWORDLONG total_bytes = memStat.ullTotalPageFile;
        DWORDLONG free_bytes = memStat.ullAvailPageFile;
        DWORDLONG used_bytes = total_bytes - free_bytes;
        common_mem_swap(free_bytes, used_bytes, update_every);
    }

    return 0;
}
