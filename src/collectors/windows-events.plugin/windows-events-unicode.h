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

typedef struct {
    char *data;
    size_t size; // the allocated size of data buffer
    size_t used;  // the used size of the data buffer (including null terminators, if any)
    TXT_SOURCE src;
} TXT_UTF8;

typedef struct {
    wchar_t *data;
    size_t size; // the allocated size of data buffer
    size_t used;  // the used size of the data buffer (including null terminators, if any)
} TXT_UNICODE;

static inline size_t compute_new_size(size_t old_size, size_t required_size) {
    size_t size = (required_size % 2048 == 0) ? required_size : required_size + 2048;
    size = (size / 2048) * 2048;

    if(size < old_size * 2)
        size = old_size * 2;

    return size;
}

static inline void txt_utf8_cleanup(TXT_UTF8 *utf8) {
    freez(utf8->data);
}

static inline void txt_utf8_resize(TXT_UTF8 *utf8, size_t required_size, bool keep) {
    if(required_size < utf8->size)
        return;

    if(keep) {
        size_t new_size = compute_new_size(utf8->size, required_size);
        utf8->data = reallocz(utf8->data, new_size);
        utf8->size = new_size;
    }
    else {
        txt_utf8_cleanup(utf8);
        utf8->size = compute_new_size(utf8->size, required_size);
        utf8->data = mallocz(utf8->size);
    }
}

static inline void txt_unicode_cleanup(TXT_UNICODE *unicode) {
    freez(unicode->data);
}

static inline void txt_unicode_resize(TXT_UNICODE *unicode, size_t required_size) {
    if(required_size < unicode->size)
        return;

    txt_unicode_cleanup(unicode);
    unicode->size = compute_new_size(unicode->size, required_size);
    unicode->data = mallocz(unicode->size * sizeof(wchar_t));
}

bool wevt_str_unicode_to_utf8(TXT_UTF8 *utf8, TXT_UNICODE *unicode);
bool wevt_str_wchar_to_utf8(TXT_UTF8 *utf8, const wchar_t *src, int src_len_with_null);

void unicode2utf8(char *dst, size_t dst_size, const wchar_t *src);
void utf82unicode(wchar_t *dst, size_t dst_size, const char *src);

char *account2utf8(const wchar_t *user);
char *domain2utf8(const wchar_t *domain);

char *channel2utf8(const wchar_t *channel);
wchar_t *channel2unicode(const char *utf8str);

char *query2utf8(const wchar_t *query);

char *unicode2utf8_strdupz(const wchar_t *src, size_t *utf8_len);

static inline void wevt_utf8_empty(TXT_UTF8 *dst) {
    txt_utf8_resize(dst, 1, false);
    dst->data[0] = '\0';
    dst->used = 1;
}

#endif //NETDATA_WINDOWS_EVENTS_UNICODE_H
