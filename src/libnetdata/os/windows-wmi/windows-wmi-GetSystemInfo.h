// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_WMI_GETSYSTEMINFO_H
#define NETDATA_WINDOWS_WMI_GETSYSTEMINFO_H

#include "../../libnetdata.h"

#if defined(OS_WINDOWS)

typedef struct {
    char Model[256];
    char Manufacturer[256];
    bool Populated;
} Win32ComputerSystemInfo;

typedef struct {
    char Caption[256];
    bool Populated;
} Win32OperatingSystemInfo;

bool GetWin32ComputerSystemInfo(Win32ComputerSystemInfo *out);
bool GetWin32OperatingSystemInfo(Win32OperatingSystemInfo *out);

#endif

#endif //NETDATA_WINDOWS_WMI_GETSYSTEMINFO_H