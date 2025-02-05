// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_INLINED_H
#define NETDATA_INLINED_H 1

#include "libnetdata.h"

#ifdef KERNEL_32BIT
typedef uint32_t kernel_uint_t;
#define str2kernel_uint_t(string) str2uint32_t(string, NULL)
#define KERNEL_UINT_FORMAT "%u"
#else
typedef uint64_t kernel_uint_t;
#define str2kernel_uint_t(string) str2uint64_t(string, NULL)
#define KERNEL_UINT_FORMAT "%" PRIu64
#endif

#define str2pid_t(string) str2uint32_t(string, NULL)


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

static inline unsigned int str2u(const char *s) {
    unsigned int n = 0;

    while(*s >= '0' && *s <= '9')
        n = n * 10 + (*s++ - '0');

    return n;
}

static inline int str2i(const char *s) {
    if(unlikely(*s == '-')) {
        s++;
        return -(int) str2u(s);
    }
    else {
        if(unlikely(*s == '+')) s++;
        return (int) str2u(s);
    }
}

static inline unsigned long str2ul(const char *s) {
    unsigned long n = 0;

    while(*s >= '0' && *s <= '9')
        n = n * 10 + (*s++ - '0');

    return n;
}

static inline long str2l(const char *s) {
    if(unlikely(*s == '-')) {
        s++;
        return -(long) str2ul(s);
    }
    else {
        if(unlikely(*s == '+')) s++;
        return (long) str2ul(s);
    }
}

static inline uint32_t str2uint32_t(const char *s, char **endptr) {
    uint32_t n = 0;

    while(*s >= '0' && *s <= '9')
        n = n * 10 + (*s++ - '0');

    if(unlikely(endptr))
        *endptr = (char *)s;

    return n;
}

static inline uint64_t str2uint64_t(const char *s, char **endptr) {
    uint64_t n = 0;

#ifdef ENV32BIT
    unsigned long n32 = 0;
    while (*s >= '0' && *s <= '9' && n32 < (ULONG_MAX / 10))
        n32 = n32 * 10 + (*s++ - '0');

    n = n32;
#endif

    while(*s >= '0' && *s <= '9')
        n = n * 10 + (*s++ - '0');

    if(unlikely(endptr))
        *endptr = (char *)s;

    return n;
}

static inline unsigned long long int str2ull(const char *s, char **endptr) {
    return str2uint64_t(s, endptr);
}

static inline long long str2ll(const char *s, char **endptr) {
    if(unlikely(*s == '-')) {
        s++;
        return -(long long) str2uint64_t(s, endptr);
    }
    else {
        if(unlikely(*s == '+')) s++;
        return (long long) str2uint64_t(s, endptr);
    }
}

static inline uint32_t str2uint32_hex(const char *src, char **endptr) {
    uint32_t num = 0;
    const unsigned char *s = (const unsigned char *)src;
    unsigned char c;

    while ((c = hex_value_from_ascii[(uint8_t)*s]) != 255) {
        num = (num << 4) | c;
        s++;
    }

    if(endptr)
        *endptr = (char *)s;

    return num;
}

static inline uint64_t str2uint64_hex(const char *src, char **endptr) {
    uint64_t num = 0;
    const unsigned char *s = (const unsigned char *)src;
    unsigned char c;

    while ((c = hex_value_from_ascii[(uint8_t)*s]) != 255) {
        num = (num << 4) | c;
        s++;
    }

    if(endptr)
        *endptr = (char *)s;

    return num;
}

static inline uint64_t str2uint64_base64(const char *src, char **endptr) {
    uint64_t num = 0;
    const unsigned char *s = (const unsigned char *)src;
    unsigned char c;

    while ((c = base64_value_from_ascii[*s]) != 255) {
        num = (num << 6) | c;
        s++;
    }

    if(endptr)
        *endptr = (char *)s;

    return num;
}

static inline NETDATA_DOUBLE str2ndd_parse_double_decimal_digits_internal(const char *src, int *digits) {
    const char *s = src;
    NETDATA_DOUBLE n = 0.0;

    while(*s >= '0' && *s <= '9') {

        // this works for both 32-bit and 64-bit systems
        unsigned long ni = 0;
        unsigned exponent = 0;
        while (*s >= '0' && *s <= '9' && ni < (ULONG_MAX / 10)) {
            ni = (ni * 10) + (*s++ - '0');
            exponent++;
        }

        n = n * powndd(10.0, exponent) + (NETDATA_DOUBLE)ni;
    }

    *digits = (int)(s - src);
    return n;
}

static inline NETDATA_DOUBLE str2ndd(const char *src, char **endptr) {
    const char *s = src;

    NETDATA_DOUBLE sign = 1.0;
    NETDATA_DOUBLE result;
    int integral_digits = 0;

    NETDATA_DOUBLE fractional = 0.0;
    int fractional_digits = 0;

    NETDATA_DOUBLE exponent = 0.0;
    int exponent_digits = 0;

    switch(*s) {
        case '-':
            s++;
            sign = -1.0;
            break;

        case '+':
            s++;
            break;

        case 'n':
            if(s[1] == 'a' && s[2] == 'n') {
                if(endptr) *endptr = (char *)&s[3];
                return NAN;
            }
            if(s[1] == 'u' && s[2] == 'l' && s[3] == 'l') {
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

    result = str2ndd_parse_double_decimal_digits_internal(s, &integral_digits);
    s += integral_digits;

    if(unlikely(*s == '.')) {
        s++;
        fractional = str2ndd_parse_double_decimal_digits_internal(s, &fractional_digits);
        s += fractional_digits;
    }

    if (unlikely(*s == 'e' || *s == 'E')) {
        const char *e_ptr = s;
        s++;

        int exponent_sign = 1;
        if (*s == '-') {
            exponent_sign = -1;
            s++;
        }
        else if(*s == '+')
            s++;

        exponent = str2ndd_parse_double_decimal_digits_internal(s, &exponent_digits);
        if(unlikely(!exponent_digits)) {
            exponent = 0;
            s = e_ptr;
        }
        else {
            s += exponent_digits;
            exponent *= exponent_sign;
        }
    }

    if(unlikely(endptr))
        *endptr = (char *)s;

    if (unlikely(exponent_digits))
        result *= powndd(10.0, exponent);

    if (unlikely(fractional_digits))
        result += fractional / powndd(10.0, fractional_digits) * (exponent_digits ? powndd(10.0, exponent) : 1.0);

    return sign * result;
}

static ALWAYS_INLINE unsigned long long str2ull_encoded(const char *s) {
    if(*s == IEEE754_UINT64_B64_PREFIX[0])
        return str2uint64_base64(s + sizeof(IEEE754_UINT64_B64_PREFIX) - 1, NULL);

    if(s[0] == HEX_PREFIX[0] && s[1] == HEX_PREFIX[1])
        return str2uint64_hex(s + 2, NULL);

    return str2uint64_t(s, NULL);
}

static ALWAYS_INLINE long long str2ll_encoded(const char *s) {
    if(*s == '-')
        return -(long long) str2ull_encoded(&s[1]);
    else
        return (long long) str2ull_encoded(s);
}

static ALWAYS_INLINE NETDATA_DOUBLE str2ndd_encoded(const char *src, char **endptr) {
    if (*src == IEEE754_DOUBLE_B64_PREFIX[0]) {
        // double parsing from base64
        uint64_t n = str2uint64_base64(src + sizeof(IEEE754_DOUBLE_B64_PREFIX) - 1, endptr);
        NETDATA_DOUBLE *ptr = (NETDATA_DOUBLE *) (&n);
        return *ptr;
    }

    if (*src == IEEE754_DOUBLE_HEX_PREFIX[0]) {
        // double parsing from hex
        uint64_t n = str2uint64_hex(src + sizeof(IEEE754_DOUBLE_HEX_PREFIX) - 1, endptr);
        NETDATA_DOUBLE *ptr = (NETDATA_DOUBLE *) (&n);
        return *ptr;
    }

    double sign = 1.0;

    if(*src == '-') {
        sign = -1.0;
        src++;
    }

    if(unlikely(*src == IEEE754_UINT64_B64_PREFIX[0]))
        return (NETDATA_DOUBLE) str2uint64_base64(src + sizeof(IEEE754_UINT64_B64_PREFIX) - 1, endptr) * sign;

    if(unlikely(*src == HEX_PREFIX[0] && src[1] == HEX_PREFIX[1]))
        return (NETDATA_DOUBLE) str2uint64_hex(src + sizeof(HEX_PREFIX) - 1, endptr) * sign;

    return str2ndd(src, endptr) * sign;
}

static inline char *strncpyz(char *dst, const char *src, size_t dst_size_minus_1) {
    char *p = dst;

    while (*src && dst_size_minus_1--)
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
    if(dst_size)
        *dst = '\0';

    // skip leading dashes
    while (*src == '-')
        src++;

    while (*src != '\0') {
        if (dst_size < 1)
            return false;

        if (iscntrl((uint8_t)*src) || *src == '$') {
            // remove control characters and characters that are expanded by bash
            *dst++ = '_';
            dst_size--;
        }
        else if (*src == '\'' || *src == '`') {
            // escape single quotes
            if (dst_size < 4)
                return false;

            dst[0] = '\''; dst[1] = '\\'; dst[2] = '\''; dst[3] = '\'';

            dst += 4;
            dst_size -= 4;
        }
        else {
            *dst++ = *src;
            dst_size--;
        }

        src++;
    }

    // make sure we have space to terminate the string
    if (dst_size == 0)
        return false;

    *dst = '\0';

    return true;
}

static inline int read_txt_file(const char *filename, char *buffer, size_t size) {
    if(unlikely(!size)) return 3;

    int fd = open(filename, O_RDONLY | O_CLOEXEC, 0666);
    if(unlikely(fd == -1)) {
        buffer[0] = '\0';
        return 1;
    }

    ssize_t r = read(fd, buffer, size - 1); // leave space of the final zero
    if(unlikely(r == -1)) {
        buffer[0] = '\0';
        close(fd);
        return 2;
    }
    buffer[r] = '\0';

    close(fd);
    return 0;
}

static inline bool read_txt_file_to_buffer(const char *filename, BUFFER *wb, size_t max_size) {
    // Open the file
    int fd = open(filename, O_RDONLY | O_CLOEXEC);
    if (fd == -1)
        return false;

    // Get the file size
    struct stat st;
    if (fstat(fd, &st) == -1) {
        close(fd);
        return false;
    }

    size_t file_size = st.st_size;

    // Check if the file size exceeds the maximum allowed size
    if (file_size > max_size) {
        close(fd);
        return false; // File size too large
    }

    buffer_need_bytes(wb, file_size + 1);

    // Read the file contents into the buffer
    ssize_t r = read(fd, &wb->buffer[wb->len], file_size);
    if (r != (ssize_t)file_size) {
        close(fd);
        return false; // Read error
    }
    wb->len = r;

    // Close the file descriptor
    close(fd);

    return true; // Success
}

static inline int read_proc_cmdline(const char *filename, char *buffer, size_t size) {
    if (unlikely(!size)) return 3;

    int fd = open(filename, O_RDONLY | O_CLOEXEC, 0666);
    if (unlikely(fd == -1)) {
        buffer[0] = '\0';
        return 1;
    }

    ssize_t r = read(fd, buffer, size - 1); // Leave space for final null character
    if (unlikely(r == -1)) {
        buffer[0] = '\0';
        close(fd);
        return 2;
    }

    if (r > 0) {
        // Replace null characters with spaces, except for the last one
        for (ssize_t i = 0; i < r - 1; i++) {
            if (buffer[i] == '\0') {
                buffer[i] = ' ';
            }
        }
        buffer[r] = '\0'; // Null-terminate the string
    }
    else {
        buffer[0] = '\0'; // Empty cmdline
    }

    close(fd);
    return 0;
}

static inline int read_single_number_file(const char *filename, unsigned long long *result) {
    char buffer[30 + 1];

    int ret = read_txt_file(filename, buffer, sizeof(buffer));
    if(unlikely(ret)) {
        *result = 0;
        return ret;
    }

    buffer[30] = '\0';
    *result = str2ull(buffer, NULL);
    return 0;
}

static inline int read_single_signed_number_file(const char *filename, long long *result) {
    char buffer[30 + 1];

    int ret = read_txt_file(filename, buffer, sizeof(buffer));
    if(unlikely(ret)) {
        *result = 0;
        return ret;
    }

    buffer[30] = '\0';
    *result = atoll(buffer);
    return 0;
}

static inline int read_single_base64_or_hex_number_file(const char *filename, unsigned long long *result) {
    char buffer[30 + 1];

    int ret = read_txt_file(filename, buffer, sizeof(buffer));
    if(unlikely(ret)) {
        *result = 0;
        return ret;
    }

    buffer[30] = '\0';

    if(likely(buffer[0])){
        *result = str2ull_encoded(buffer);
        return 0;
    }
    else {
        *result = 0;
        return -1;
    }
}

static inline char *strsep_skip_consecutive_separators(char **ptr, char *s) {
    char *p = (char *)"";
    while (p && !p[0] && *ptr) p = strsep(ptr, s);
    return (p);
}

// remove leading and trailing spaces; may return NULL
static inline char *trim(char *s) {
    char *buf = s;

    // skip leading spaces
    while (*s && isspace((uint8_t)*s)) s++;
    if (!*s) {
        *buf = '\0';
        return NULL;
    }

    // skip tailing spaces
    // this way is way faster. Writes only one NUL char.
    ssize_t l = (ssize_t)strlen(s);
    if (--l >= 0) {
        char *p = s + l;
        while (p > s && isspace((uint8_t)*p)) p--;
        *++p = '\0';
    }

    if (!*s) {
        *buf = '\0';
        return NULL;
    }

    return s;
}

// like trim(), but also remove duplicate spaces inside the string; may return NULL
static inline char *trim_all(char *buffer) {
    char *d = buffer, *s = buffer;

    // skip spaces
    while(isspace((uint8_t)*s)) s++;

    while(*s) {
        // copy the non-space part
        while(*s && !isspace((uint8_t)*s)) *d++ = *s++;

        // add a space if we have to
        if(*s && isspace((uint8_t)*s)) {
            *d++ = ' ';
            s++;
        }

        // skip spaces
        while(isspace((uint8_t)*s)) s++;
    }

    *d = '\0';

    if(d > buffer) {
        d--;
        if(isspace((uint8_t)*d)) *d = '\0';
    }

    if(!buffer[0]) return NULL;
    return buffer;
}

static inline bool streq(const char *a, const char *b) {
    if (a == b)
        return true;

    if (a == NULL || b == NULL)
        return false;

    return strcmp(a, b) == 0;
}

static inline bool strstartswith(const char *string, const char *prefix) {
    if (string == NULL || prefix == NULL)
        return false;

    size_t string_len = strlen(string);
    size_t prefix_len = strlen(prefix);

    if (prefix_len > string_len)
        return false;

    return strncmp(string, prefix, prefix_len) == 0;
}

static inline bool strendswith(const char *string, const char *suffix) {
    if (string == NULL || suffix == NULL)
        return false;

    size_t string_len = strlen(string);
    size_t suffix_len = strlen(suffix);

    if (suffix_len > string_len)
        return false;

    return strcmp(string + string_len - suffix_len, suffix) == 0;
}

static inline bool strendswith_lengths(const char *string, size_t string_len, const char *suffix, size_t suffix_len) {
    if (string == NULL || suffix == NULL)
        return false;

    if (suffix_len > string_len)
        return false;

    return strcmp(string + string_len - suffix_len, suffix) == 0;
}

#endif //NETDATA_INLINED_H
