// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#if defined(OS_WINDOWS)
#include <windows.h>

long netdata_registry_get_dword_from_open_key(unsigned int *out, void *lKey, char *name)
{
    DWORD length = 260;
    return RegQueryValueEx(lKey, name, NULL, NULL, (LPBYTE) out, &length);
}

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

    ret = netdata_registry_get_dword_from_open_key(out, lKey, name);
    if (ret != ERROR_SUCCESS)
        status = false;

    RegCloseKey(lKey);

    return status;
}

long netdata_registry_get_string_from_open_key(char *out, unsigned int length, void *lKey, char *name)
{
    return RegQueryValueEx(lKey, name, NULL, NULL, (LPBYTE) out, &length);
}

bool netdata_registry_get_string(char *out, unsigned int length, void *hKey, char *subKey, char *name)
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

    ret = netdata_registry_get_string_from_open_key(out, length, lKey, name);
    if (ret != ERROR_SUCCESS)
        status = false;

    RegCloseKey(lKey);

    return status;
}

#endif
