// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STRING_UTF8_H
#define NETDATA_STRING_UTF8_H 1

#define IS_UTF8_BYTE(x) ((uint8_t)(x) & (uint8_t)0x80)
#define IS_UTF8_STARTBYTE(x) (IS_UTF8_BYTE(x) && ((uint8_t)(x) & (uint8_t)0x40))

#ifndef _countof
#define _countof(x) (sizeof(x) / sizeof(*(x)))
#endif

#if defined(OS_WINDOWS)

// return an always null terminated wide string, truncate to given size if destination is not big enough,
// src_len can be -1 use all of it.
// returns zero on errors, > 0 otherwise (including the null, even if src is not null terminated).
size_t any_to_utf16(uint32_t CodePage, wchar_t *dst, size_t dst_size, const char *src, int src_len);

// always null terminated, truncated if it does not fit, src_len can be -1 to use all of it.
// returns zero on errors, > 0 otherwise (including the null, even if src is not null terminated).
#define utf8_to_utf16(utf16, utf16_count, src, src_len) any_to_utf16(CP_UTF8, utf16, utf16_count, src, src_len)

// always null terminated, truncated if it does not fit, src_len can be -1 to use all of it.
// returns zero on errors, > 0 otherwise (including the null, even if src is not null terminated).
size_t utf16_to_utf8(char *dst, size_t dst_size, const wchar_t *src, int src_len);

#endif

#endif /* NETDATA_STRING_UTF8_H */
