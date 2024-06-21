// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#if defined(OS_WINDOWS)
#include <windows.h>

bool netdata_registry_get_dword(unsigned int *out, void *hKey, char *subKey, char *name)
{
    HKEY lKey;
    bool status = true;
    long ret = RegOpenKeyEx(hKey,
                            subKey,
                            0,
                            KEY_READ,
                            &lKey);
    if (ret != ERROR_SUCCESS)
        return false;

    DWORD length = 260, value = 260;
    ret = RegQueryValueEx(lKey, name, NULL, NULL, (LPBYTE) &value, &length);
    if (ret != ERROR_SUCCESS)
        status = false;

    RegCloseKey(lKey);

    *out = value;

    return status;
}

bool netdata_registry_get_string(char *out, size_t length, void *hKey, char *subKey, char *name)
{
    HKEY lKey;
    bool status = true;
    long ret = RegOpenKeyEx(hKey,
                            subKey,
                            0,
                            KEY_READ,
                            &lKey);
    if (ret != ERROR_SUCCESS)
        return false;

    ret = RegQueryValueEx(lKey, name, NULL, NULL, out, &length);
    if (ret != ERROR_SUCCESS)
        status = false;

    RegCloseKey(lKey);

    return status;
}

#endif

