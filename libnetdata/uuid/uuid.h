// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UUID_H
#define NETDATA_UUID_H

UUID_DEFINE(streaming_from_child_msgid, 0xed,0x4c,0xdb, 0x8f, 0x1b, 0xeb, 0x4a, 0xd3, 0xb5, 0x7c, 0xb3, 0xca, 0xe2, 0xd1, 0x62, 0xfa);
UUID_DEFINE(streaming_to_parent_msgid, 0x6e, 0x2e, 0x38, 0x39, 0x06, 0x76, 0x48, 0x96, 0x8b, 0x64, 0x60, 0x45, 0xdb, 0xf2, 0x8d, 0x66);
UUID_DEFINE(health_alert_transition_msgid, 0x9c, 0xe0, 0xcb, 0x58, 0xab, 0x8b, 0x44, 0xdf, 0x82, 0xc4, 0xbf, 0x1a, 0xd9, 0xee, 0x22, 0xde);

// this is also defined in alarm-notify.sh.in
UUID_DEFINE(health_alert_notification_msgid, 0x6d, 0xb0, 0x01, 0x8e, 0x83, 0xe3, 0x43, 0x20, 0xae, 0x2a, 0x65, 0x9d, 0x78, 0x01, 0x9f, 0xb7);

typedef struct {
    union {
        uuid_t uuid;
        struct {
            uint64_t hig64;
            uint64_t low64;
        } parts;
    };
} UUID;
UUID UUID_generate_from_hash(const void *payload, size_t payload_len);

#define UUIDeq(a, b) ((a).parts.hig64 == (b).parts.hig64 && (a).parts.low64 == (b).parts.low64)

static inline UUID uuid2UUID(uuid_t uu1) {
    UUID *ret = (UUID *)uu1;
    return *ret;
}

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
