// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_UNICODE_H
#define NETDATA_WINDOWS_EVENTS_UNICODE_H

#include "libnetdata/libnetdata.h"

#define WINEVENT_NAME_KEYWORDS_SEPARATOR    ", "
static inline void txt_utf8_add_keywords_separator_if_needed(TXT_UTF8 *dst) {
    if(dst->used > 1)
        txt_utf8_append(dst, WINEVENT_NAME_KEYWORDS_SEPARATOR, sizeof(WINEVENT_NAME_KEYWORDS_SEPARATOR) - 1);
}

static inline void txt_utf8_set_numeric_if_empty(TXT_UTF8 *dst, const char *prefix, size_t len, uint64_t value) {
    if(dst->used <= 1) {
        txt_utf8_resize(dst, len + UINT64_MAX_LENGTH + 1,  false);
        memcpy(dst->data, prefix, len);
        dst->used = len + print_uint64(&dst->data[len], value) + 1;
    }
}

static inline void txt_utf8_set_hex_if_empty(TXT_UTF8 *dst, const char *prefix, size_t len, uint64_t value) {
    if(dst->used <= 1) {
        txt_utf8_resize(dst, len + UINT64_HEX_MAX_LENGTH + 1,  false);
        memcpy(dst->data, prefix, len);
        dst->used = len + print_uint64_hex_full(&dst->data[len], value) + 1;
    }
}

// --------------------------------------------------------------------------------------------------------------------
// conversions

void unicode2utf8(char *dst, size_t dst_size, const wchar_t *src);
void utf82unicode(wchar_t *dst, size_t dst_size, const char *src);

char *channel2utf8(const wchar_t *channel);
wchar_t *channel2unicode(const char *utf8str);

char *query2utf8(const wchar_t *query);
char *provider2utf8(const wchar_t *provider);

#endif //NETDATA_WINDOWS_EVENTS_UNICODE_H
