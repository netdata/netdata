// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_UNICODE_H
#define NETDATA_WINDOWS_EVENTS_UNICODE_H

#include "libnetdata/libnetdata.h"
#include <windows.h>
#include <wchar.h>

typedef enum __attribute__((packed)) {
    TXT_SOURCE_UNKNOWN = 0,
    TXT_SOURCE_PUBLISHER,
    TXT_SOURCE_FIELD_CACHE,
    TXT_SOURCE_EVENT_LOG,
    TXT_SOURCE_HARDCODED,

    // terminator
    TXT_SOURCE_MAX,
} TXT_SOURCE;

static inline size_t compute_new_size(size_t old_size, size_t required_size) {
    size_t size = (required_size % 2048 == 0) ? required_size : required_size + 2048;
    size = (size / 2048) * 2048;

    if(size < old_size * 2)
        size = old_size * 2;

    return size;
}

// --------------------------------------------------------------------------------------------------------------------
// TXT_UTF8

typedef struct {
    char *data;
    size_t size; // the allocated size of data buffer
    size_t used;  // the used size of the data buffer (including null terminators, if any)
    TXT_SOURCE src;
} TXT_UTF8;

static inline void txt_utf8_cleanup(TXT_UTF8 *dst) {
    freez(dst->data);
    dst->data = NULL;
    dst->used = 0;
}

static inline void txt_utf8_resize(TXT_UTF8 *dst, size_t required_size, bool keep) {
    if(required_size <= dst->size)
        return;

    size_t new_size = compute_new_size(dst->size, required_size);

    if(keep && dst->data)
        dst->data = reallocz(dst->data, new_size);
    else {
        txt_utf8_cleanup(dst);
        dst->data = mallocz(new_size);
        dst->used = 0;
    }

    dst->size = new_size;
}

static inline void wevt_utf8_empty(TXT_UTF8 *dst) {
    txt_utf8_resize(dst, 1, false);
    dst->data[0] = '\0';
    dst->used = 1;
}

static inline void txt_utf8_set(TXT_UTF8 *dst, const char *txt, size_t txt_len) {
    txt_utf8_resize(dst, dst->used + txt_len + 1, true);
    memcpy(dst->data, txt, txt_len);
    dst->used = txt_len + 1;
    dst->data[dst->used - 1] = '\0';
}

static inline void txt_utf8_append(TXT_UTF8 *dst, const char *txt, size_t txt_len) {
    if(dst->used <= 1) {
        // the destination is empty
        txt_utf8_set(dst, txt, txt_len);
    }
    else {
        // there is something already in the buffer
        txt_utf8_resize(dst, dst->used + txt_len, true);
        memcpy(&dst->data[dst->used - 1], txt, txt_len);
        dst->used += txt_len; // the null was already counted
        dst->data[dst->used - 1] = '\0';
    }
}

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
// TXT_UNICODE

typedef struct {
    wchar_t *data;
    size_t size; // the allocated size of data buffer
    size_t used;  // the used size of the data buffer (including null terminators, if any)
} TXT_UNICODE;

static inline void txt_unicode_cleanup(TXT_UNICODE *dst) {
    freez(dst->data);
}

static inline void txt_unicode_resize(TXT_UNICODE *dst, size_t required_size, bool keep) {
    if(required_size <= dst->size)
        return;

    size_t new_size = compute_new_size(dst->size, required_size);

    if (keep && dst->data) {
        dst->data = reallocz(dst->data, new_size * sizeof(wchar_t));
    } else {
        txt_unicode_cleanup(dst);
        dst->data = mallocz(new_size * sizeof(wchar_t));
        dst->used = 0;
    }

    dst->size = new_size;
}

static inline void txt_unicode_set(TXT_UNICODE *dst, const wchar_t *txt, size_t txt_len) {
    txt_unicode_resize(dst, dst->used + txt_len + 1, true);
    memcpy(dst->data, txt, txt_len * sizeof(wchar_t));
    dst->used = txt_len + 1;
    dst->data[dst->used - 1] = '\0';
}

static inline void txt_unicode_append(TXT_UNICODE *dst, const wchar_t *txt, size_t txt_len) {
    if(dst->used <= 1) {
        // the destination is empty
        txt_unicode_set(dst, txt, txt_len);
    }
    else {
        // there is something already in the buffer
        txt_unicode_resize(dst, dst->used + txt_len, true);
        memcpy(&dst->data[dst->used - 1], txt, txt_len * sizeof(wchar_t));
        dst->used += txt_len; // the null was already counted
        dst->data[dst->used - 1] = '\0';
    }
}

// --------------------------------------------------------------------------------------------------------------------
// conversions

bool wevt_str_unicode_to_utf8(TXT_UTF8 *dst, TXT_UNICODE *unicode);
bool wevt_str_wchar_to_utf8(TXT_UTF8 *dst, const wchar_t *src, int src_len_with_null);

void unicode2utf8(char *dst, size_t dst_size, const wchar_t *src);
void utf82unicode(wchar_t *dst, size_t dst_size, const char *src);

char *account2utf8(const wchar_t *user);
char *domain2utf8(const wchar_t *domain);

char *channel2utf8(const wchar_t *channel);
wchar_t *channel2unicode(const char *utf8str);

char *query2utf8(const wchar_t *query);
char *provider2utf8(const wchar_t *provider);

char *unicode2utf8_strdupz(const wchar_t *src, size_t *utf8_len);

#endif //NETDATA_WINDOWS_EVENTS_UNICODE_H
