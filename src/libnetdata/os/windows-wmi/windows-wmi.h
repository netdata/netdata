// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_WMI_H
#define NETDATA_WINDOWS_WMI_H

#include "../../libnetdata.h"

#if defined(OS_WINDOWS)
typedef struct {
    IWbemLocator *pLoc;
    IWbemServices *pSvc;
} ND_WMI;

extern __thread ND_WMI nd_wmi;

HRESULT InitializeWMI(void);
void CleanupWMI(void);

// Convert a WMI UTF-16 BSTR into a UTF-8, always null-terminated destination buffer.
// dst is emptied on a NULL src or on conversion failure. Safe with dst_size == 0.
void wmi_bstr_to_multibyte(char *dst, size_t dst_size, BSTR src);

#include "windows-wmi-GetDiskDriveInfo.h"
#include "windows-wmi-GetSystemInfo.h"

#endif

#endif //NETDATA_WINDOWS_WMI_H
