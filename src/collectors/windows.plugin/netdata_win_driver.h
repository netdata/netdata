// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WIN_DRV_H
#define NETDATA_WIN_DRV_H

#if defined(OS_WINDOWS)

#include <windows.h>

// Device + symbolic link names used by the driver
#define MSR_USER_PATH        "\\\\.\\NDDrv"

// IOCTLs
#define IOCTL_MSR_READ   CTL_CODE(FILE_DEVICE_UNKNOWN, 0x800, METHOD_BUFFERED, FILE_ANY_ACCESS)
#define IOCTL_MSR_THERM  CTL_CODE(FILE_DEVICE_UNKNOWN, 0x801, METHOD_BUFFERED, FILE_ANY_ACCESS)

// Payloads
typedef struct _MSR_READ_INPUT {
    ULONG Reg;           // MSR index to read (e.g., 0x19C)
} MSR_READ_INPUT, *PMSR_READ_INPUT;

typedef struct _MSR_READ_OUTPUT {
    ULONGLONG Value;     // Raw 64-bit MSR value
} MSR_READ_OUTPUT, *PMSR_READ_OUTPUT;

typedef struct _MSR_THERM_OUTPUT {
    ULONG DeltaToTjMax;  // From IA32_THERM_STATUS[22:16]
} MSR_THERM_OUTPUT, *PMSR_THERM_OUTPUT;

#endif // OS_WINDOWS

#endif // NETDATA_WIN_DRV_H
