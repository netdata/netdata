// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_PROVIDERS_H
#define NETDATA_WINDOWS_EVENTS_PROVIDERS_H

typedef enum __attribute__((packed)) {
    WEVT_PLATFORM_UNKNOWN = 0,
    WEVT_PLATFORM_WEL,
    WEVT_PLATFORM_ETW,
    WEVT_PLATFORM_TL,
} WEVT_PROVIDER_PLATFORM;

#include "windows-events.h"

struct provider_meta_handle;
typedef struct provider_meta_handle PROVIDER_META_HANDLE;

PROVIDER_META_HANDLE *provider_get(ND_UUID uuid, LPCWSTR providerName);
void provider_release(PROVIDER_META_HANDLE *h);
EVT_HANDLE provider_handle(PROVIDER_META_HANDLE *h);
PROVIDER_META_HANDLE *provider_dup(PROVIDER_META_HANDLE *h);

void providers_release_unused_handles(void);

const char *provider_get_name(PROVIDER_META_HANDLE *p);
ND_UUID provider_get_uuid(PROVIDER_META_HANDLE *p);

void provider_cache_init(void);

bool provider_keyword_cacheable(PROVIDER_META_HANDLE *h);
bool provider_tasks_cacheable(PROVIDER_META_HANDLE *h);
bool is_useful_provider_for_levels(PROVIDER_META_HANDLE *h);
bool provider_opcodes_cacheable(PROVIDER_META_HANDLE *h);

bool provider_get_keywords(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value);
bool provider_get_level(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value);
bool provider_get_task(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value);
bool provider_get_opcode(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value);
WEVT_PROVIDER_PLATFORM provider_get_platform(PROVIDER_META_HANDLE *p);

#endif //NETDATA_WINDOWS_EVENTS_PROVIDERS_H
