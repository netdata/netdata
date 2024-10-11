// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_WINDOWS_WRAPPERS_H
#define NETDATA_OS_WINDOWS_WRAPPERS_H

#include "../libnetdata.h"

#if defined(OS_WINDOWS)
#define NETDATA_WIN_DETECTION_METHOD "Windows API/Registry"

long netdata_registry_get_dword_from_open_key(unsigned int *out, void *lKey, char *name);
bool netdata_registry_get_dword(unsigned int *out, void *hKey, char *subKey, char *name);

long netdata_registry_get_string_from_open_key(char *out, unsigned int length, void *lKey, char *name);
bool netdata_registry_get_string(char *out, unsigned int length, void *hKey, char *subKey, char *name);

bool EnableWindowsPrivilege(const char *privilegeName);

#endif // OS_WINDOWS
#endif //NETDATA_OS_WINDOWS_WRAPPERS_H
