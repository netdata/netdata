// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_WMI_GETOSINFO_H
#define NETDATA_WINDOWS_WMI_GETOSINFO_H

#include "windows-wmi.h"

#if defined(OS_WINDOWS)

typedef struct {
    char Caption[512];
    DWORD ProductType;
} OsInfoWMI;

bool GetOsInfo(OsInfoWMI *osInfo);

#endif

#endif //NETDATA_WINDOWS_WMI_GETOSINFO_H
