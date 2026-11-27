// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_WMI_GETDISKDRIVEINFO_H
#define NETDATA_WINDOWS_WMI_GETDISKDRIVEINFO_H

#include "../../libnetdata.h"

#if defined(OS_WINDOWS)

typedef struct {
    int Index;
    char Model[256];
    char DeviceID[256];
} DiskDriveInfoWMI;

// Connects the caller-owned handle (reusing an open connection), queries Win32_DiskDrive and keeps the connection
// for the next call. *failed (required) is written on every return: after a failure the handle is disconnected, so
// the next call reconnects, and the reason is logged at error_priority. A timed-out enumeration is a failure that
// still returns the rows read before it. ConnectServer() can block for minutes: call this only from a thread that
// may wait that long (see nd_wmi_connect()).
size_t GetDiskDriveInfo(ND_WMI *wmi, DiskDriveInfoWMI *diskInfoArray, size_t array_size,
                        ND_LOG_FIELD_PRIORITY error_priority, bool *failed);

#endif

#endif //NETDATA_WINDOWS_WMI_GETDISKDRIVEINFO_H
