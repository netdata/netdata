// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_WINDOWS_WRAPPERS_H
#define NETDATA_OS_WINDOWS_WRAPPERS_H

#include "../libnetdata.h"

#if defined(OS_WINDOWS)
#include <windows.h>

bool netdata_registry_get_dword(DWORD *out, HKEY hKey, char *subKey, char *name);
#endif
#endif //NETDATA_OS_WINDOWS_WRAPPERS_H
