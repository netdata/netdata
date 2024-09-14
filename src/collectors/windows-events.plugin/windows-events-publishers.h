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

#endif //NETDATA_WINDOWS_EVENTS_PUBLISHERS_H
