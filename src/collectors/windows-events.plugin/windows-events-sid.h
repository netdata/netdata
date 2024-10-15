// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_SID_H
#define NETDATA_WINDOWS_EVENTS_SID_H

#include "windows-events.h"

struct wevt_log;
bool wevt_convert_user_id_to_name(PSID sid, TXT_UTF8 *dst_account, TXT_UTF8 *dst_domain, TXT_UTF8 *dst_sid_str);
bool buffer_sid_to_sid_str_and_name(PSID sid, BUFFER *dst, const char *prefix);
void sid_cache_init(void);

#endif //NETDATA_WINDOWS_EVENTS_SID_H
