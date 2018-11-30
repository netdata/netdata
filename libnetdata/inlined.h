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

static inline char *duration2str(long long usec, char *buffer, size_t len) {
    long long div;
    char *units;

    if(usec % (1000000ULL * 86400ULL * 365ULL) == 0) {
        div = 1000000ULL * 86400ULL * 365ULL;
        units = "Y";
    }
    else if(usec % (1000000ULL * 86400ULL * 30ULL) == 0) {
        div = 1000000ULL * 86400ULL * 30ULL;
        units = "M";
    }
    else if(usec % (1000000ULL * 86400ULL * 7ULL) == 0) {
        div = 1000000ULL * 86400ULL * 7ULL;
        units = "w";
    }
    else if(usec % (1000000ULL * 86400ULL * 1ULL) == 0) {
        div = 1000000ULL * 86400ULL * 1ULL;
        units = "d";
    }
    else if(usec % (1000000ULL * 3600ULL) == 0) {
        div = 1000000ULL * 3600ULL;
        units = "h";
    }
    else if(usec % (1000000ULL * 60ULL) == 0) {
        div = 1000000ULL * 60ULL;
        units = "m";
    }
    else if(usec % (1000000ULL) == 0) {
        div = 1000000ULL;
        units = "s";
    }
    else if(usec % (1000ULL) == 0) {
        div = 1000ULL;
        units = "ms";
    }
    else {
        div = 1;
        units = "u";
    }

    snprintfz(buffer, len, "%lld%s", usec / div, units);
    return buffer;
}

static inline long long str2duration(const char *s) {
    long long units = 1000000;
    char *endptr = NULL;

    long long value = strtoll(s, &endptr, 10);
    if(endptr && *endptr) {
        if(isspace(*endptr)) endptr++;

        switch(*endptr) {
            case 'Y':
                units = 1000000ULL * 86400ULL * 356ULL; // years
                break;

            case 'M':
                units = 1000000ULL * 86400ULL * 30ULL; // months
                break;

            case 'w':
                units = 1000000ULL * 86400ULL * 7ULL; // weeks
                break;

            case 'd':
                units = 1000000ULL * 86400ULL; // days
                break;

            case 'h':
                units = 1000000ULL * 3600ULL; // hours
                break;

            case 'm':
                if(endptr[1] == 's') {
                    units = 1000ULL; // milliseconds
                }
                else {
                    units = 1000000ULL * 60ULL; // minutes
                }
            break;

            default:
            case 's':
                units = 1000000ULL; // seconds
                break;

            case 'u':
                units = 1ULL;
                break;
        }
    }

    return value * units;
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

static inline long double str2ld(const char *s, char **endptr) {
    int negative = 0;
    const char *start = s;
    unsigned long long integer_part = 0;
    unsigned long decimal_part = 0;
    size_t decimal_digits = 0;

    switch(*s) {
        case '-':
            s++;
            negative = 1;
            break;

        case '+':
            s++;
            break;

        case 'n':
            if(s[1] == 'a' && s[2] == 'n') {
                if(endptr) *endptr = (char *)&s[3];
                return NAN;
            }
            break;

        case 'i':
            if(s[1] == 'n' && s[2] == 'f') {
                if(endptr) *endptr = (char *)&s[3];
                return INFINITY;
            }
            break;

        default:
            break;
    }

    while (*s >= '0' && *s <= '9') {
        integer_part = (integer_part * 10) + (*s - '0');
        s++;
    }

    if(unlikely(*s == '.')) {
        decimal_part = 0;
        s++;

        while (*s >= '0' && *s <= '9') {
            decimal_part = (decimal_part * 10) + (*s - '0');
            s++;
            decimal_digits++;
        }
    }

    if(unlikely(*s == 'e' || *s == 'E'))
        return strtold(start, endptr);

    if(unlikely(endptr))
        *endptr = (char *)s;

    if(unlikely(negative)) {
        if(unlikely(decimal_digits))
            return -((long double)integer_part + (long double)decimal_part / powl(10.0, decimal_digits));
        else
            return -((long double)integer_part);
    }
    else {
        if(unlikely(decimal_digits))
            return (long double)integer_part + (long double)decimal_part / powl(10.0, decimal_digits);
        else
            return (long double)integer_part;
    }
}

#ifdef NETDATA_STRCMP_OVERRIDE
#ifdef strcmp
#undef strcmp
#endif
#define strcmp(a, b) strsame(a, b)
#endif // NETDATA_STRCMP_OVERRIDE

static inline int strsame(const char *a, const char *b) {
    if(unlikely(a == b)) return 0;
    while(*a && *a == *b) { a++; b++; }
    return *a - *b;
}

static inline char *strncpyz(char *dst, const char *src, size_t n) {
    char *p = dst;

    while (*src && n--)
        *dst++ = *src++;

    *dst = '\0';

    return p;
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
