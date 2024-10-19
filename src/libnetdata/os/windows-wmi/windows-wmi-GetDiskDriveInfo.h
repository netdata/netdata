// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_WMI_GETDISKDRIVEINFO_H
#define NETDATA_WINDOWS_WMI_GETDISKDRIVEINFO_H

#include "windows-wmi.h"

#if defined(OS_WINDOWS)

typedef struct {
    char DeviceID[256];
    char Model[256];
    char Caption[256];
    char Name[256];
    int Partitions;
    unsigned long long Size;
    char Status[64];
    int Availability;
    int Index;
    char Manufacturer[256];
    char InstallDate[64];
    char MediaType[128];
    bool NeedsCleaning;
} DiskDriveInfoWMI;

size_t GetDiskDriveInfo(DiskDriveInfoWMI *diskInfoArray, size_t array_size);

#endif

#endif //NETDATA_WINDOWS_WMI_GETDISKDRIVEINFO_H
