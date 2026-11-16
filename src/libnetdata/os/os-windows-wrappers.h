// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_WINDOWS_WRAPPERS_H
#define NETDATA_OS_WINDOWS_WRAPPERS_H

#include "../libnetdata.h"

#if defined(OS_WINDOWS)
long netdata_registry_get_dword_from_open_key(unsigned int *out, void *lKey, char *name);
bool netdata_registry_get_dword(unsigned int *out, void *hKey, char *subKey, char *name);

long netdata_registry_get_string_from_open_key(char *out, unsigned int length, void *lKey, char *name);
bool netdata_registry_get_string(char *out, unsigned int length, void *hKey, char *subKey, char *name);

bool netdata_registry_key_exists(void *hKey, const char *subKey);

bool EnableWindowsPrivilege(const char *privilegeName);

// True only when the service is SERVICE_RUNNING. *current_state (optional) receives the SERVICE_* state, or 0 when
// the service is not installed or cannot be queried.
bool os_windows_service_is_running(const wchar_t *service_name, DWORD *current_state);

#endif // OS_WINDOWS
#endif //NETDATA_OS_WINDOWS_WRAPPERS_H
