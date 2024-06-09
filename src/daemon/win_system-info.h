#ifndef _NETDATA_WIN_SYSTEM_INFO_H_
#define _NETDATA_WIN_SYSTEM_INFO_H_

// the netdata database
#include "database/rrd.h"

#ifdef OS_WINDOWS
#include "windows.h"
#include "versionhelpers.h"

void netdata_windows_get_system_info(struct rrdhost_system_info *system_info);
#endif

#endif // _NETDATA_WIN_SYSTEM_INFO_H_
