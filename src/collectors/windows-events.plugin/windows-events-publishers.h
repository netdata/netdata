// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_PUBLISHERS_H
#define NETDATA_WINDOWS_EVENTS_PUBLISHERS_H

#include "windows-events.h"

struct provider_meta_handle;
typedef struct provider_meta_handle PROVIDER_META_HANDLE;

PROVIDER_META_HANDLE *publisher_get(ND_UUID uuid, LPCWSTR providerName);
void publisher_release(PROVIDER_META_HANDLE *h);
EVT_HANDLE publisher_handle(PROVIDER_META_HANDLE *h);
PROVIDER_META_HANDLE *publisher_dup(PROVIDER_META_HANDLE *h);

void publisher_cache_init(void);

bool publisher_keywords_cacheable(PROVIDER_META_HANDLE *h);
bool publisher_tasks_cacheable(PROVIDER_META_HANDLE *h);
bool is_useful_publisher_for_levels(PROVIDER_META_HANDLE *h);
bool publisher_opcodes_cacheable(PROVIDER_META_HANDLE *h);

bool publisher_get_keywords(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value);
bool publisher_get_level(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value);
bool publisher_get_task(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value);
bool publisher_get_opcode(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value);

#endif //NETDATA_WINDOWS_EVENTS_PUBLISHERS_H
