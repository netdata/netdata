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

bool GetWin32ComputerSystemInfo(Win32ComputerSystemInfo *out);

#endif

#endif //NETDATA_WINDOWS_WMI_GETSYSTEMINFO_H