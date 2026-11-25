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

size_t GetDiskDriveInfo(DiskDriveInfoWMI *diskInfoArray, size_t array_size);

#endif

#endif //NETDATA_WINDOWS_WMI_GETDISKDRIVEINFO_H
