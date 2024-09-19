// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_QUERY_BUILDER_H
#define NETDATA_WINDOWS_EVENTS_QUERY_BUILDER_H

#include "windows-events.h"

wchar_t *wevt_generate_query_no_xpath(LOGS_QUERY_STATUS *lqs, BUFFER *wb);

#endif //NETDATA_WINDOWS_EVENTS_QUERY_BUILDER_H
