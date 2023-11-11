// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

void uuid_unparse_lower_compact(const uuid_t uuid, char *out) {
    static const char *hex_chars = "0123456789abcdef";
    for (int i = 0; i < 16; i++) {
        out[i * 2] = hex_chars[(uuid[i] >> 4) & 0x0F];
        out[i * 2 + 1] = hex_chars[uuid[i] & 0x0F];
    }
    out[32] = '\0'; // Null-terminate the string
}
