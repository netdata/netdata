// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CLAIM_H_
# define NETDATA_CLAIM_H_ 1

#include <wchar.h>
#include "ui.h"

extern LPWSTR token;
extern LPWSTR room;
extern LPWSTR proxy;

void netdata_claim_error_exit(wchar_t *function);
static inline void netdata_claim_convert_str(char *dst, wchar_t *src, size_t len) {
    size_t copied = wcstombs(dst, src, len);
    dst[copied] = '\0';
}

#endif //NETDATA_CLAIM_H_
