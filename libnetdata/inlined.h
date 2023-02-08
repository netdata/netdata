// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_INLINED_H
#define NETDATA_INLINED_H 1

#include "libnetdata.h"

#ifdef KERNEL_32BIT
typedef uint32_t kernel_uint_t;
#define str2kernel_uint_t(string) str2uint32_t(string)
#define KERNEL_UINT_FORMAT "%u"
#else
typedef uint64_t kernel_uint_t;
#define str2kernel_uint_t(string) str2uint64_t(string)
#define KERNEL_UINT_FORMAT "%" PRIu64
#endif

#define str2pid_t(string) str2uint32_t(string)


// for faster execution, allow the compiler to inline
// these functions that are called thousands of times per second

static inline uint32_t djb2_hash32(const char* name) {
    unsigned char *s = (unsigned char *) name;
    uint32_t hash = 5381;
    while (*s)
        hash = ((hash << 5) + hash) + (uint32_t) *s++; // hash * 33 + char
    return hash;
}

static inline uint32_t pluginsd_parser_hash32(const char *name) {
    unsigned char *s = (unsigned char *) name;
    uint32_t hash = 0;
    while (*s) {
        hash <<= 5;
        hash += *s++ - ' ';
    }
    return hash;
}

// https://stackoverflow.com/a/107657
static inline uint32_t larson_hash32(const char *name) {
    unsigned char *s = (unsigned char *) name;
    uint32_t hash = 0;
    while (*s)
        hash = hash * 101 + (uint32_t) *s++;
    return hash;
}

// http://isthe.com/chongo/tech/comp/fnv/
static inline uint32_t fnv1_hash32(const char *name) {
    unsigned char *s = (unsigned char *) name;
    uint32_t hash = 0x811c9dc5;
    while (*s) {
        hash *= 0x01000193; // 16777619
        hash ^= (uint32_t) *s++;
    }
    return hash;
}

// http://isthe.com/chongo/tech/comp/fnv/
static inline uint32_t fnv1a_hash32(const char *name) {
    unsigned char *s = (unsigned char *) name;
    uint32_t hash = 0x811c9dc5;
    while (*s) {
        hash ^= (uint32_t) *s++;
        hash *= 0x01000193; // 16777619
    }
    return hash;
}

static inline uint32_t fnv1a_uhash32(const char *name) {
    unsigned char *s = (unsigned char *) name;
    uint32_t hash = 0x811c9dc5, c;
    while ((c = *s++)) {
        if (unlikely(c >= 'A' && c <= 'Z')) c += 'a' - 'A';
        hash ^= c;
        hash *= 0x01000193; // 16777619
    }
    return hash;
}

#define simple_hash(s) fnv1a_hash32(s)
#define simple_uhash(s) fnv1a_uhash32(s)

static inline size_t indexing_partition_old(Word_t ptr, Word_t modulo) {
    size_t total = 0;

    total += (ptr & 0xff) >> 0;
    total += (ptr & 0xff00) >> 8;
    total += (ptr & 0xff0000) >> 16;
    total += (ptr & 0xff000000) >> 24;

    if(sizeof(Word_t) > 4) {
        total += (ptr & 0xff00000000) >> 32;
        total += (ptr & 0xff0000000000) >> 40;
        total += (ptr & 0xff000000000000) >> 48;
        total += (ptr & 0xff00000000000000) >> 56;
    }

    return (total % modulo);
}

static uint32_t murmur32(uint32_t k) __attribute__((const));
static inline uint32_t murmur32(uint32_t k) {
    k ^= k >> 16;
    k *= 0x85ebca6b;
    k ^= k >> 13;
    k *= 0xc2b2ae35;
    k ^= k >> 16;

    return k;
}

static uint64_t murmur64(uint64_t k) __attribute__((const));
static inline uint64_t murmur64(uint64_t k) {
    k ^= k >> 33;
    k *= 0xff51afd7ed558ccdUL;
    k ^= k >> 33;
    k *= 0xc4ceb9fe1a85ec53UL;
    k ^= k >> 33;

    return k;
}

static inline size_t indexing_partition(Word_t ptr, Word_t modulo) __attribute__((const));
static inline size_t indexing_partition(Word_t ptr, Word_t modulo) {
#ifdef ENV64BIT
    uint64_t hash = murmur64(ptr);
    return hash % modulo;
#else
    uint32_t hash = murmur32(ptr);
    return hash % modulo;
#endif
}

static inline int str2i(const char *s) {
    int n = 0;
    char c, negative = (char)(*s == '-');
    const char *e = s + 30; // max number of character to iterate

    for(c = (char)((negative)?*(++s):*s); c >= '0' && c <= '9' && s < e ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }

    if(unlikely(negative))
        return -n;

    return n;
}

static inline long str2l(const char *s) {
    long n = 0;
    char c, negative = (*s == '-');
    const char *e = &s[30]; // max number of character to iterate

    for(c = (negative)?*(++s):*s; c >= '0' && c <= '9' && s < e ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }

    if(unlikely(negative))
        return -n;

    return n;
}

static inline uint32_t str2uint32_t(const char *s) {
    uint32_t n = 0;
    char c;
    const char *e = &s[30]; // max number of character to iterate

    for(c = *s; c >= '0' && c <= '9' && s < e ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

static inline uint64_t str2uint64_t(const char *s) {
    uint64_t n = 0;
    char c;
    const char *e = &s[30]; // max number of character to iterate

    for(c = *s; c >= '0' && c <= '9' && s < e ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

static inline unsigned long str2ul(const char *s) {
    unsigned long n = 0;
    char c;
    const char *e = &s[30]; // max number of character to iterate

    for(c = *s; c >= '0' && c <= '9' && s < e ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

static inline unsigned long long str2ull(const char *s) {
    unsigned long long n = 0;
    char c;
    const char *e = &s[30]; // max number of character to iterate

    for(c = *s; c >= '0' && c <= '9' && s < e ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

static inline unsigned long long str2ull_hex_or_dec(const char *s) {
    unsigned long long n = 0;
    char c;

    if(likely(s[0] == '0' && s[1] == 'x')) {
        const char *e = &s[sizeof(unsigned long long) * 2 + 2 + 1]; // max number of character to iterate: 8 bytes * 2 + '0x' + '\0'

        // skip 0x
        s += 2;

        for (c = *s; ((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')) && s < e; c = *(++s)) {
            n = n << 4;

            if (c <= '9')
                n += c - '0';
            else
                n += c - 'A' + 10;
        }
        return n;
    }
    else
        return str2ull(s);
}

static inline long long str2ll_hex_or_dec(const char *s) {
    if(*s == '-')
        return -(long long)str2ull_hex_or_dec(&s[1]);
    else
        return (long long)str2ull_hex_or_dec(s);
}

static inline long long str2ll(const char *s, char **endptr) {
    int negative = 0;

    if(unlikely(*s == '-')) {
        s++;
        negative = 1;
    }
    else if(unlikely(*s == '+'))
        s++;

    long long n = 0;
    char c;
    const char *e = &s[30]; // max number of character to iterate

    for(c = *s; c >= '0' && c <= '9' && s < e ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }

    if(unlikely(endptr))
        *endptr = (char *)s;

    if(unlikely(negative))
        return -n;
    else
        return n;
}

static inline char *strncpyz(char *dst, const char *src, size_t n) {
    char *p = dst;

    while (*src && n--)
        *dst++ = *src++;

    *dst = '\0';

    return p;
}

static inline void sanitize_json_string(char *dst, const char *src, size_t dst_size) {
    while (*src != '\0' && dst_size > 1) {
        if (*src < 0x1F) {
            *dst++ = '_';
            src++;
            dst_size--;
        }
        else if (*src == '\\' || *src == '\"') {
            *dst++ = '\\';
            *dst++ = *src++;
            dst_size -= 2;
        }
        else {
            *dst++ = *src++;
            dst_size--;
        }
    }
    *dst = '\0';
}

static inline bool sanitize_command_argument_string(char *dst, const char *src, size_t dst_size) {
    // skip leading dashes
    while (src[0] == '-')
        src++;

    // escape single quotes
    while (src[0] != '\0') {
        if (src[0] == '\'') {
            if (dst_size < 4)
                return false;

            dst[0] = '\''; dst[1] = '\\'; dst[2] = '\''; dst[3] = '\'';

            dst += 4;
            dst_size -= 4;
        } else {
            if (dst_size < 1)
                return false;

            dst[0] = src[0];

            dst += 1;
            dst_size -= 1;
        }

        src++;
    }

    // make sure we have space to terminate the string
    if (dst_size == 0)
        return false;
    *dst = '\0';

    return true;
}

static inline int read_file(const char *filename, char *buffer, size_t size) {
    if(unlikely(!size)) return 3;

    int fd = open(filename, O_RDONLY, 0666);
    if(unlikely(fd == -1)) {
        buffer[0] = '\0';
        return 1;
    }

    ssize_t r = read(fd, buffer, size);
    if(unlikely(r == -1)) {
        buffer[0] = '\0';
        close(fd);
        return 2;
    }
    buffer[r] = '\0';

    close(fd);
    return 0;
}

static inline int read_single_number_file(const char *filename, unsigned long long *result) {
    char buffer[30 + 1];

    int ret = read_file(filename, buffer, 30);
    if(unlikely(ret)) {
        *result = 0;
        return ret;
    }

    buffer[30] = '\0';
    *result = str2ull(buffer);
    return 0;
}

static inline int read_single_signed_number_file(const char *filename, long long *result) {
    char buffer[30 + 1];

    int ret = read_file(filename, buffer, 30);
    if(unlikely(ret)) {
        *result = 0;
        return ret;
    }

    buffer[30] = '\0';
    *result = atoll(buffer);
    return 0;
}

#endif //NETDATA_INLINED_H
