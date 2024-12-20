// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_FIELDS_CACHE_H
#define NETDATA_WINDOWS_EVENTS_FIELDS_CACHE_H

#include "windows-events.h"

typedef enum __attribute__((packed)) {
    WEVT_FIELD_TYPE_LEVEL = 0,
    WEVT_FIELD_TYPE_OPCODE,
    WEVT_FIELD_TYPE_KEYWORD,
    WEVT_FIELD_TYPE_TASK,

    // terminator
    WEVT_FIELD_TYPE_MAX,
} WEVT_FIELD_TYPE;

void field_cache_init(void);
bool field_cache_get(WEVT_FIELD_TYPE type, const ND_UUID *uuid, uint64_t value, TXT_UTF8 *dst);
void field_cache_set(WEVT_FIELD_TYPE type, const ND_UUID *uuid, uint64_t value, TXT_UTF8 *name);

#endif //NETDATA_WINDOWS_EVENTS_FIELDS_CACHE_H
