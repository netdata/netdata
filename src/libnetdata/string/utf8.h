// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STRING_UTF8_H
#define NETDATA_STRING_UTF8_H 1

#include "../libnetdata.h"

#define IS_UTF8_BYTE(x) ((uint8_t)(x) & (uint8_t)0x80)
#define IS_UTF8_STARTBYTE(x) (IS_UTF8_BYTE(x) && ((uint8_t)(x) & (uint8_t)0x40))

#ifndef _countof
#define _countof(x) (sizeof(x) / sizeof(*(x)))
#endif

#if defined(OS_WINDOWS)

// return an always null terminated wide string, truncate to given size if destination is not big enough,
// src_len can be -1 use all of it.
// returns zero on errors, > 0 otherwise (including the null, even if src is not null terminated).
size_t any_to_utf16(uint32_t CodePage, wchar_t *dst, size_t dst_size, const char *src, int src_len, bool *truncated);

// always null terminated, truncated if it does not fit, src_len can be -1 to use all of it.
// returns zero on errors, > 0 otherwise (including the null, even if src is not null terminated).
#define utf8_to_utf16(utf16, utf16_count, src, src_len) any_to_utf16(CP_UTF8, utf16, utf16_count, src, src_len, NULL)

// always null terminated, truncated if it does not fit, src_len can be -1 to use all of it.
// returns zero on errors, > 0 otherwise (including the null, even if src is not null terminated).
size_t utf16_to_utf8(char *dst, size_t dst_size, const wchar_t *src, int src_len, bool *truncated);

// --------------------------------------------------------------------------------------------------------------------
// TXT_UTF8

typedef enum __attribute__((packed)) {
    TXT_SOURCE_UNKNOWN = 0,
    TXT_SOURCE_PROVIDER,
    TXT_SOURCE_FIELD_CACHE,
    TXT_SOURCE_EVENT_LOG,
    TXT_SOURCE_HARDCODED,

    // terminator
    TXT_SOURCE_MAX,
} TXT_SOURCE;

typedef struct {
    char *data;
    uint32_t size; // the allocated size of data buffer
    uint32_t used;  // the used size of the data buffer (including null terminators, if any)
    TXT_SOURCE src;
} TXT_UTF8;

void txt_utf8_append(TXT_UTF8 *dst, const char *txt, size_t txt_len);
void txt_utf8_set(TXT_UTF8 *dst, const char *txt, size_t txt_len);
void txt_utf8_empty(TXT_UTF8 *dst);
void txt_utf8_resize(TXT_UTF8 *dst, size_t required_size, bool keep);
void txt_utf8_cleanup(TXT_UTF8 *dst);

// --------------------------------------------------------------------------------------------------------------------
// TXT_UTF16

typedef struct {
    wchar_t *data;
    uint32_t size; // the allocated size of data buffer
    uint32_t used;  // the used size of the data buffer (including null terminators, if any)
} TXT_UTF16;

void txt_utf16_cleanup(TXT_UTF16 *dst);
void txt_utf16_resize(TXT_UTF16 *dst, size_t required_size, bool keep);
void txt_utf16_set(TXT_UTF16 *dst, const wchar_t *txt, size_t txt_len);
void txt_utf16_append(TXT_UTF16 *dst, const wchar_t *txt, size_t txt_len);

// --------------------------------------------------------------------------------------------------------------------

size_t txt_compute_new_size(size_t old_size, size_t required_size);

bool txt_utf16_to_utf8(TXT_UTF8 *utf8, TXT_UTF16 *utf16);
bool wchar_to_txt_utf8(TXT_UTF8 *dst, const wchar_t *src, int src_len);
char *utf16_to_utf8_strdupz(const wchar_t *src, size_t *dst_len);

// --------------------------------------------------------------------------------------------------------------------

#endif // OS_WINDOWS

#endif /* NETDATA_STRING_UTF8_H */
