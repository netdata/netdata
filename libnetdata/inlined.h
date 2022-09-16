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

static inline uint32_t simple_hash(const char *name) {
    unsigned char *s = (unsigned char *) name;
    uint32_t hval = 0x811c9dc5;
    while (*s) {
        hval *= 16777619;
        hval ^= (uint32_t) *s++;
    }
    return hval;
}

static inline uint32_t simple_uhash(const char *name) {
    unsigned char *s = (unsigned char *) name;
    uint32_t hval = 0x811c9dc5, c;
    while ((c = *s++)) {
        if (unlikely(c >= 'A' && c <= 'Z')) c += 'a' - 'A';
        hval *= 16777619;
        hval ^= c;
    }
    return hval;
}

static inline int simple_hash_strcmp(const char *name, const char *b, uint32_t *hash) {
    unsigned char *s = (unsigned char *) name;
    uint32_t hval = 0x811c9dc5;
    int ret = 0;
    while (*s) {
        if(!ret) ret = *s - *b++;
        hval *= 16777619;
        hval ^= (uint32_t) *s++;
    }
    *hash = hval;
    return ret;
}

static inline int str2i(const char *s) {
    int n = 0;
    char c, negative = (*s == '-');

    for(c = (negative)?*(++s):*s; c >= '0' && c <= '9' ; c = *(++s)) {
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

    for(c = (negative)?*(++s):*s; c >= '0' && c <= '9' ; c = *(++s)) {
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
    for(c = *s; c >= '0' && c <= '9' ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

static inline uint64_t str2uint64_t(const char *s) {
    uint64_t n = 0;
    char c;
    for(c = *s; c >= '0' && c <= '9' ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

static inline unsigned long str2ul(const char *s) {
    unsigned long n = 0;
    char c;
    for(c = *s; c >= '0' && c <= '9' ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

static inline unsigned long long str2ull(const char *s) {
    unsigned long long n = 0;
    char c;
    for(c = *s; c >= '0' && c <= '9' ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
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
    for(c = *s; c >= '0' && c <= '9' ; c = *(++s)) {
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
