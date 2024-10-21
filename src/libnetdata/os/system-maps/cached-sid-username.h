// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CACHED_SID_USERNAME_H
#define NETDATA_CACHED_SID_USERNAME_H

#include "../../libnetdata.h"

#if defined(OS_WINDOWS)
#include "../../string/utf8.h"

bool cached_sid_to_account_domain_sidstr(void *sid, TXT_UTF8 *dst_account, TXT_UTF8 *dst_domain, TXT_UTF8 *dst_sid_str);
bool cached_sid_to_buffer_append(void *sid, BUFFER *dst, const char *prefix);
void cached_sid_username_init(void);
STRING *cached_sid_fullname_or_sid_str(void *sid);
#endif

#endif //NETDATA_CACHED_SID_USERNAME_H
