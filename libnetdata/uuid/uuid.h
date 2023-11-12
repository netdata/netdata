// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UUID_H
#define NETDATA_UUID_H

UUID_DEFINE(streaming_from_child_msgid, 0xed,0x4c,0xdb, 0x8f, 0x1b, 0xeb, 0x4a, 0xd3, 0xb5, 0x7c, 0xb3, 0xca, 0xe2, 0xd1, 0x62, 0xfa);
UUID_DEFINE(streaming_to_parent_msgid, 0x6e, 0x2e, 0x38, 0x39, 0x06, 0x76, 0x48, 0x96, 0x8b, 0x64, 0x60, 0x45, 0xdb, 0xf2, 0x8d, 0x66);

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
