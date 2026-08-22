// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CLAIM_H_
# define NETDATA_CLAIM_H_ 1

#include <wchar.h>
#include "ui.h"

extern LPWSTR token;
extern LPWSTR room;
extern LPWSTR proxy;

// Set to 0 whenever arguments are present, so dialogs stay out of the installer's session-0 context.
extern int nd_claim_interactive;

// Exit codes reported to the caller (the MSI logs the custom action's return value).
#define ND_CLAIM_OK           0  // claim.conf written
#define ND_CLAIM_BAD_ARGS     1  // no token supplied, or the token was empty after trimming
#define ND_CLAIM_INTERNAL     2  // allocation failure, or the target path did not fit
#define ND_CLAIM_WRITE_FAILED 3  // claim.conf could not be created or written completely

void netdata_claim_error_exit(wchar_t *function, int code);
static inline void netdata_claim_convert_str(char *dst, wchar_t *src, size_t dst_size) {
    if (!dst_size)
        return;
    // reserve one byte for the NUL: wcstombs() does not terminate when it fills the buffer
    size_t copied = wcstombs(dst, src, dst_size - 1);
    if (copied == (size_t)-1) copied = 0;
    dst[copied] = '\0';
}

#endif //NETDATA_CLAIM_H_
