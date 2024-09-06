// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_SID_H
#define NETDATA_WINDOWS_EVENTS_SID_H

#include "windows-events.h"

struct wevt_log;
bool wevt_convert_user_id_to_name(struct wevt_log *log, PSID sid);

#endif //NETDATA_WINDOWS_EVENTS_SID_H
