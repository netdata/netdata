// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef _NETDATA_WIN_SYSTEM_INFO_H_
#define _NETDATA_WIN_SYSTEM_INFO_H_

// the netdata database
#include "database/rrd.h"

#define NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN "unknown"
#define NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE "none"
#define NETDATA_DEFAULT_SYSTEM_INFO_VALUE_FALSE "false"

#ifdef OS_WINDOWS
#include "windows.h"
#include "versionhelpers.h"

void netdata_windows_get_system_info(struct rrdhost_system_info *system_info);
#endif

#endif // _NETDATA_WIN_SYSTEM_INFO_H_
