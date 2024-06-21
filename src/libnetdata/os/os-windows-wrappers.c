// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#if defined(OS_WINDOWS)

bool netdata_registry_get_dword(DWORD *out, HKEY hKey, char *subKey, char *name)
{
    HKEY lKey;
    bool status = true;
    long ret = RegOpenKeyEx(hKey,
                            subKey,
                            0,
                            KEY_READ,
                            &lKey);
    if (ret != ERROR_SUCCESS)
        return 0;

    DWORD length = 260, value = 260, type;
    ret = RegQueryValueEx(lKey, name, NULL, NULL, (LPBYTE) &value, &length);
    if (ret != ERROR_SUCCESS)
        status = false;

    RegCloseKey(lKey);

    *out = value;

    return status;
}

#endif

