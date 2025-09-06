// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WIN_DRV_H
#define NETDATA_WIN_DRV_H

#if defined(OS_WINDOWS)

#include <windows.h>

// Device + symbolic link names used by the driver
#define MSR_USER_PATH        "\\\\.\\NDDrv"

// IOCTLs
#define IOCTL_MSR_READ   CTL_CODE(FILE_DEVICE_UNKNOWN, 0x800, METHOD_BUFFERED, FILE_ANY_ACCESS)

typedef struct {
    uint32_t msr;
    uint32_t cpu;
    uint32_t low;
    uint32_t high;
} MSR_REQUEST;

#endif // OS_WINDOWS

#endif // NETDATA_WIN_DRV_H
