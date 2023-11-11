// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UUID_H
#define NETDATA_UUID_H

#define UUID_COMPACT_STR_LEN 33
void uuid_unparse_lower_compact(const uuid_t uuid, char *out);
int uuid_parse_compact(const char *in, uuid_t uuid);
int uuid_parse_flexi(const char *in, uuid_t uuid);

static inline int uuid_memcmp(const uuid_t *uu1, const uuid_t *uu2) {
    return memcmp(uu1, uu2, sizeof(uuid_t));
}

static inline int hex_char_to_int(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1; // Invalid hexadecimal character
}

#endif //NETDATA_UUID_H
