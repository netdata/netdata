// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_H
#define NETDATA_WINDOWS_EVENTS_H

#include "libnetdata/libnetdata.h"
#include <windows.h>
#include <winevt.h>

#include "windows-events-query.h"
#include "windows-events-sources.h"

char *channel2utf8(const wchar_t *channel);

#endif //NETDATA_WINDOWS_EVENTS_H
