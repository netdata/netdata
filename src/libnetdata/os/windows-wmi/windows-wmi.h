// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_WMI_H
#define NETDATA_WINDOWS_WMI_H

#include "../../libnetdata.h"
#include <windows.h>
#include <wbemidl.h>

typedef struct {
    IWbemLocator *pLoc;
    IWbemServices *pSvc;
} ND_WMI;

extern __thread ND_WMI nd_wmi;

HRESULT InitializeWMI(void);
void CleanupWMI(void);

#include "windows-wmi-GetDiskDriveInfo.h"

#endif //NETDATA_WINDOWS_WMI_H
