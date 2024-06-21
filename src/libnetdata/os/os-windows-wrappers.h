// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_WINDOWS_WRAPPERS_H
#define NETDATA_OS_WINDOWS_WRAPPERS_H

#include "../libnetdata.h"

#if defined(OS_WINDOWS)

bool netdata_registry_get_dword(unsigned int *out, void *hKey, char *subKey, char *name);

#endif // OS_WINDOWS
#endif //NETDATA_OS_WINDOWS_WRAPPERS_H
