#ifndef NETDATA_INLINED_H
#define NETDATA_INLINED_H

#include "common.h"

#ifdef HAVE_STMT_EXPR_NON_EXISTING
// GCC extension to define a function as a preprocessor macro

#define simple_hash(name) ({                                         \
    register unsigned char *__hash_source = (unsigned char *)(name); \
    register uint32_t __hash_value = 0x811c9dc5;                     \
    while (*__hash_source) {                                         \
        __hash_value *= 16777619;                                    \
        __hash_value ^= (uint32_t) *__hash_source++;                 \
    }                                                                \
    __hash_value;                                                    \
})

#define simple_uhash(name) ({                                        \
    register unsigned char *__hash_source = (unsigned char *)(name); \
    register uint32_t __hash_value = 0x811c9dc5, __hash_char;        \
    while ((__hash_char = *__hash_source++)) {                       \
        if (unlikely(__hash_char >= 'A' && __hash_char <= 'Z'))      \
            __hash_char += 'a' - 'A';                                \
        __hash_value *= 16777619;                                    \
        __hash_value ^= __hash_char;                                 \
    }                                                                \
    __hash_value;                                                    \
})

#else /* ! HAVE_STMT_EXPR */

// for faster execution, allow the compiler to inline
// these functions that are called to hash strings
static inline uint32_t simple_hash(const char *name) {
    register unsigned char *s = (unsigned char *) name;
    register uint32_t hval = 0x811c9dc5;
    while (*s) {
        hval *= 16777619;
        hval ^= (uint32_t) *s++;
    }
    return hval;
}

static inline uint32_t simple_uhash(const char *name) {
    register unsigned char *s = (unsigned char *) name;
    register uint32_t hval = 0x811c9dc5, c;
    while ((c = *s++)) {
        if (unlikely(c >= 'A' && c <= 'Z')) c += 'a' - 'A';
        hval *= 16777619;
        hval ^= c;
    }
    return hval;
}

#endif /* HAVE_STMT_EXPR */

static inline int str2i(const char *s) {
    register int n = 0;
    register char c, negative = (*s == '-');

    for(c = (negative)?*(++s):*s; c >= '0' && c <= '9' ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }

    if(unlikely(negative))
        return -n;

    return n;
}

static inline long str2l(const char *s) {
    register long n = 0;
    register char c, negative = (*s == '-');

    for(c = (negative)?*(++s):*s; c >= '0' && c <= '9' ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }

    if(unlikely(negative))
        return -n;

    return n;
}

static inline unsigned long str2ul(const char *s) {
    register unsigned long n = 0;
    register char c;
    for(c = *s; c >= '0' && c <= '9' ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

static inline unsigned long long str2ull(const char *s) {
    register unsigned long long n = 0;
    register char c;
    for(c = *s; c >= '0' && c <= '9' ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

static inline int read_single_number_file(const char *filename, unsigned long long *result) {
    char buffer[1024 + 1];

    int fd = open(filename, O_RDONLY, 0666);
    if(unlikely(fd == -1)) {
        *result = 0;
        return 1;
    }

    ssize_t r = read(fd, buffer, 1024);
    if(unlikely(r == -1)) {
        *result = 0;
        close(fd);
        return 2;
    }

    close(fd);
    *result = str2ull(buffer);
    return 0;
}

#endif //NETDATA_INLINED_H
