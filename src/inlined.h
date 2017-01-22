#ifndef NETDATA_INLINED_H
#define NETDATA_INLINED_H

#include "common.h"

/**
 * @file inlined.h
 * @brief Common methods the compiler should inline.
 *
 * This is done for faster execution
 */

#ifdef KERNEL_32BIT
typedef uint32_t kernel_uint_t;
#define str2kernel_uint_t(string) str2uint32_t(string)
#define KERNEL_UINT_FORMAT "%u"
#else
typedef uint64_t kernel_uint_t; ///< uint32_t or uint64_t dependent on the system.
/**
 * Convert a string to an `kernel_unit_t`.
 *
 * This does no error handling.
 * Only use this for strings representing integers (`-?[0-9]+`) and fit into an int.
 *
 * @param string to convert.
 * @return `kernel_uint_t` representation of `s` 
 */
#define str2kernel_uint_t(string) str2uint64_t(string)
#define KERNEL_UINT_FORMAT "%" PRIu64 ///< Formatter string of `kernel_uint_t`.
#endif

/**
 * Convert a string to an pid_t.
 *
 * This does no error handling.
 * Only use this for strings representing integers (`-?[0-9]+`) and fit into an int.
 *
 * @param string to convert.
 * @return pid_t representation of `s` 
 */
#define str2pid_t(string) str2uint32_t(string)

// for faster execution, allow the compiler to inline
// these functions that are called thousands of times per second
// these functions that are called to hash strings

/** 
 * A simple Hash function.
 *
 * @see http://isthe.com/chongo/tech/comp/fnv/#FNV-1a
 *
 * For faster execution, allow the compiler to inline.
 *
 * @param name string to generate hash for
 * @return hash for `name`
 */
static inline uint32_t simple_hash(const char *name) {
    unsigned char *s = (unsigned char *) name;
    uint32_t hval = 0x811c9dc5;
    while (*s) {
        hval *= 16777619;
        hval ^= (uint32_t) *s++;
    }
    return hval;
}

/**
 * A simple Hash function returning a positive value.
 *
 * @see http://isthe.com/chongo/tech/comp/fnv/#FNV-1a
 *
 * For faster execution, allow the compiler to inline.
 *
 * @param name string to generate hash for
 * @return hash for `name`
 */
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

/**
 * Calculate simple_hash() of `name` and compare `name` to `b`
 *
 * This can be used to enhance performance.
 * Instaed of
 * ```{c}
 * hash = simple_hash(string);
 * if(hash == myhash && strcmp(string, "mystring") == 0) ...
 * ```
 * one might call
 * ```{c}
 * if(simple_hash_strcmp(string, "mystring", &hash) == 0) 
 * ```
 *
 * @param name String to generate hash for.
 * @param b String to compare `name` for.
 * @param hash Pointer to store the hash.
 * @return an integer greater than, equal to, or less than 0, according `name` is greater than, equal to, or less than `b`.
 */
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

/**
 * Convert a string to an int.
 *
 * This does no error handling.
 * Only use this for strings representing integers (`-?[0-9]+`) and fit into an int.
 *
 * @param s String to convert.
 * @return int representation of `s` 
 */
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

/**
 * Convert a string to a long.
 *
 * This does no error handling.
 * Only use this for strings representing integers (`-?[0-9]+`) and fit into an long.
 *
 * @param s String to convert.
 * @return long representation of `s` 
 */
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

/**
 * Convert a string to a uint32_t.
 *
 * This does no error handling.
 * Only use this for strings representing positive numbers (`[0-9]+`) and fit into an unsigned long.
 *
 * If you are not shure if your system supports this use `str2kernel_unit_t()`
 *
 * @param s String to convert.
 * @return uint32_t representation of `s` 
 */
static inline uint32_t str2uint32_t(const char *s) {
    uint32_t n = 0;
    char c;
    for(c = *s; c >= '0' && c <= '9' ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

/**
 * Convert a string to a uint64_t.
 *
 * This does no error handling.
 * Only use this for strings representing positive numbers (`[0-9]+`) and fit into an unsigned long.
 *
 * If you are not shure if your system supports this use `str2kernel_unit_t()`
 *
 * @param s String to convert.
 * @return uint64_t representation of `s` 
 */
static inline uint64_t str2uint64_t(const char *s) {
    uint64_t n = 0;
    char c;
    for(c = *s; c >= '0' && c <= '9' ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

/**
 * Convert a string to a unsigned long.
 *
 * This does no error handling.
 * Only use this for strings representing positive numbers (`[0-9]+`) and fit into an unsigned long.
 *
 * @param s String to convert.
 * @return unsigned long representation of `s` 
 */
static inline unsigned long str2ul(const char *s) {
    unsigned long n = 0;
    char c;
    for(c = *s; c >= '0' && c <= '9' ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

/**
 * Convert a string to a unsigned long long.
 *
 * This does no error handling.
 * Only use this for strings representing positive numbers (`[0-9]+`) and fit into an unsigned long long.
 *
 * @param s String to convert.
 * @return unsigned long long representation of `s` 
 */
static inline unsigned long long str2ull(const char *s) {
    unsigned long long n = 0;
    char c;
    for(c = *s; c >= '0' && c <= '9' ; c = *(++s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

#ifdef NETDATA_STRCMP_OVERRIDE
#ifdef strcmp
#undef strcmp
#endif
#define strcmp(a, b) strsame(a, b)
#endif // NETDATA_STRCMP_OVERRIDE

/**
 * Alternative `strcmp()` implementation.
 *
 * @see man 3 strcmp
 *
 * @param a string
 * @param b string
 * @return an integer greater than, equal to, or less than 0, according `a` is greater than, equal to, or less than `b`.
 */
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

/**
 * Read `filename` containing a single number and return it.
 *
 * Open `filename`, read the first line, convert it to a number and store it in result.
 *
 * For faster execution, allow the compiler to inline.
 *
 * @param filename File to read.
 * @param result Long to store the number in.
 * @return 0 on success, -1 if could not open file, -2 if could not read from file.
 */
static inline int read_single_number_file(const char *filename, unsigned long long *result) {
    char buffer[30 + 1];

    int fd = open(filename, O_RDONLY, 0666);
    if(unlikely(fd == -1)) {
        *result = 0;
        return 1;
    }

    ssize_t r = read(fd, buffer, 30);
    if(unlikely(r == -1)) {
        *result = 0;
        close(fd);
        return 2;
    }

    close(fd);
    buffer[30] = '\0';
    *result = str2ull(buffer);
    return 0;
}

#endif //NETDATA_INLINED_H
