// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#if defined(OS_WINDOWS)
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

bool EnableWindowsPrivilege(const char *privilegeName) {
    HANDLE hToken;
    LUID luid;
    TOKEN_PRIVILEGES tkp;

    // Open the process token with appropriate access rights
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_ADJUST_PRIVILEGES | TOKEN_QUERY, &hToken))
        return false;

    // Lookup the LUID for the specified privilege
    if (!LookupPrivilegeValue(NULL, privilegeName, &luid)) {
        CloseHandle(hToken);  // Close the token handle before returning
        return false;
    }

    // Set up the TOKEN_PRIVILEGES structure
    tkp.PrivilegeCount = 1;
    tkp.Privileges[0].Luid = luid;
    tkp.Privileges[0].Attributes = SE_PRIVILEGE_ENABLED;

    // Adjust the token's privileges
    if (!AdjustTokenPrivileges(hToken, FALSE, &tkp, sizeof(tkp), NULL, NULL)) {
        CloseHandle(hToken);  // Close the token handle before returning
        return false;
    }

    // Check if AdjustTokenPrivileges succeeded
    if (GetLastError() == ERROR_NOT_ALL_ASSIGNED) {
        CloseHandle(hToken);  // Close the token handle before returning
        return false;
    }

    // Close the handle to the token after success
    CloseHandle(hToken);

    return true;
}

#endif
