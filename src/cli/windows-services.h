// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_SERVICES_H
#define NETDATA_WINDOWS_SERVICES_H

#include <windows.h>
#include <wchar.h>

// -------------- Services

static inline SC_HANDLE GetWServiceMagagerHandle(DWORD desiredAccess)
{
    return OpenSCManagerW(NULL, NULL, desiredAccess);
}

static inline SC_HANDLE GetWServiceHandle(SC_HANDLE scm, wchar_t *name, DWORD desiredAccess)
{
    return OpenService(scm, name, desiredAccess);
}

static inline int IsServiceRunning(SC_HANDLE scm, SC_HANDLE service)
{
    SERVICE_STATUS_PROCESS status;
    DWORD bytes;
    if (!QueryServiceStatusEx(service,
                              SC_STATUS_PROCESS_INFO,
                              (LPBYTE) &status,
                              sizeof(SERVICE_STATUS_PROCESS),
                              &bytes) )
    {
        return 0;
    }

    return (status.dwCurrentState != SERVICE_STOPPED && status.dwCurrentState != SERVICE_STOP_PENDING);
}

#endif //NETDATA_WINDOWS_SERVICES_H
 
