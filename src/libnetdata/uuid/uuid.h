// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UUID_H
#define NETDATA_UUID_H

typedef unsigned char uuid_t[16];

#ifdef __GNUC__
#define UUID_DEFINE(name,u0,u1,u2,u3,u4,u5,u6,u7,u8,u9,u10,u11,u12,u13,u14,u15) \
	static const uuid_t name __attribute__ ((unused)) = {u0,u1,u2,u3,u4,u5,u6,u7,u8,u9,u10,u11,u12,u13,u14,u15}
#else
#define UUID_DEFINE(name,u0,u1,u2,u3,u4,u5,u6,u7,u8,u9,u10,u11,u12,u13,u14,u15) \
	static const uuid_t name = {u0,u1,u2,u3,u4,u5,u6,u7,u8,u9,u10,u11,u12,u13,u14,u15}
#endif

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
} ND_UUID;
ND_UUID UUID_generate_from_hash(const void *payload, size_t payload_len);

#define UUIDeq(a, b) ((a).parts.hig64 == (b).parts.hig64 && (a).parts.low64 == (b).parts.low64)

static inline ND_UUID uuid2UUID(uuid_t uu1) {
    ND_UUID *ret = (ND_UUID *)uu1;
    return *ret;
}

#ifndef UUID_STR_LEN
// CentOS 7 has older version that doesn't define this
// same goes for MacOS
#define UUID_STR_LEN	37
#endif

#define UUID_COMPACT_STR_LEN 33

void uuid_unparse_lower_compact(const uuid_t uuid, char *out);
int uuid_parse_compact(const char *in, uuid_t uuid);

int uuid_parse_flexi(const char *in, uuid_t uuid);
#define uuid_parse(in, uuid) uuid_parse_flexi(in, uuid)

static inline int hex_char_to_int(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1; // Invalid hexadecimal character
}

static inline int nd_uuid_is_null(const uuid_t uu) {
    const ND_UUID *u = (ND_UUID *)uu;
    return u->parts.hig64 == 0 && u->parts.low64 == 0;
}

static inline void nd_uuid_clear(uuid_t uu) {
    ND_UUID *u = (ND_UUID *)uu;
    u->parts.low64 = u->parts.hig64 = 0;
}

static inline int nd_uuid_compare(const uuid_t uu1, const uuid_t uu2) {
    ND_UUID *u1 = (ND_UUID *)uu1;
    ND_UUID *u2 = (ND_UUID *)uu2;

    if(u1->parts.hig64 == u2->parts.hig64) {
        if(u1->parts.low64 < u2->parts.low64) return -1;
        if(u1->parts.low64 > u2->parts.low64) return 1;
        return 0;
    }

    if(u1->parts.hig64 < u2->parts.hig64) return -1;
    if(u1->parts.hig64 > u2->parts.hig64) return 1;
    return 0;
}

static inline void nd_uuid_copy(uuid_t dst, const uuid_t src) {
    ND_UUID *d = (ND_UUID *)dst;
    const ND_UUID *s = (const ND_UUID *)src;
    *d = *s;
}

static inline int nd_uuid_memcmp(const uuid_t *uu1, const uuid_t *uu2) {
    return memcmp(uu1, uu2, sizeof(uuid_t));
}

void nd_uuid_unparse_lower(const uuid_t uuid, char *out);
void nd_uuid_unparse_upper(const uuid_t uuid, char *out);

#define uuid_is_null(uu) nd_uuid_is_null(uu)
#define uuid_clear(uu) nd_uuid_clear(uu)
#define uuid_compare(uu1, uu2) nd_uuid_compare(uu1, uu2)
#define uuid_memcmp(puu1, puu2) nd_uuid_memcmp(puu1, puu2)
#define uuid_copy(dst, src) nd_uuid_copy(dst, src)

#ifdef COMPILED_FOR_WINDOWS
#define uuid_generate(out) os_uuid_generate(out)
#define uuid_generate_random(out) os_uuid_generate(out)
#define uuid_generate_time(out) os_uuid_generate(out)
#else
void uuid_generate(uuid_t out);
void uuid_generate_random(uuid_t out);
void uuid_generate_time(uuid_t out);
#endif

#define uuid_unparse(uu, out) nd_uuid_unparse_lower(uu, out)
#define uuid_unparse_lower(uu, out) nd_uuid_unparse_lower(uu, out)
#define uuid_unparse_upper(uu, out) nd_uuid_unparse_upper(uu, out)

#endif //NETDATA_UUID_H
