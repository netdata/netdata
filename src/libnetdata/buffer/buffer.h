// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_BUFFER_H
#define NETDATA_WEB_BUFFER_H 1

#include "../uuid/uuid.h"
#include "../http/content_type.h"
#include "../string/utf8.h"
#include "../libnetdata.h"

#define BUFFER_JSON_MAX_DEPTH 32 // max is 255

extern const char hex_digits[16];
extern const char hex_digits_lower[16];
extern const char base64_digits[64];
extern unsigned char hex_value_from_ascii[256];
extern unsigned char base64_value_from_ascii[256];

typedef enum __attribute__ ((__packed__)) {
    BUFFER_JSON_EMPTY = 0,
    BUFFER_JSON_OBJECT,
    BUFFER_JSON_ARRAY,
} BUFFER_JSON_NODE_TYPE;

typedef struct web_buffer_json_node {
    BUFFER_JSON_NODE_TYPE type;
    uint32_t count:24;
} BUFFER_JSON_NODE;

#define BUFFER_QUOTE_MAX_SIZE 7

typedef enum __attribute__ ((__packed__)) {
    WB_CONTENT_CACHEABLE = (1 << 0),
    WB_CONTENT_NO_CACHEABLE = (1 << 1),
} BUFFER_OPTIONS;

typedef enum __attribute__ ((__packed__)) {
    BUFFER_JSON_OPTIONS_DEFAULT = 0,
    BUFFER_JSON_OPTIONS_MINIFY = (1 << 0),
    BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAY_ITEMS = (1 << 1),
    BUFFER_JSON_OPTIONS_NON_ANONYMOUS = (1 << 2),
} BUFFER_JSON_OPTIONS;

typedef struct web_buffer {
    uint32_t size;          // allocation size of buffer, in bytes
    uint32_t len;           // current data length in buffer, in bytes
    HTTP_CONTENT_TYPE content_type;    // the content type of the data in the buffer
    BUFFER_OPTIONS options; // options related to the content
    uint16_t response_code;
    time_t date;            // the timestamp this content has been generated
    time_t expires;         // the timestamp this content expires
    size_t *statistics;
    char *buffer;           // the buffer itself

    struct {
        char key_quote[BUFFER_QUOTE_MAX_SIZE + 1];
        char value_quote[BUFFER_QUOTE_MAX_SIZE + 1];
        int8_t depth;
        BUFFER_JSON_OPTIONS options;
        BUFFER_JSON_NODE stack[BUFFER_JSON_MAX_DEPTH];
    } json;
} BUFFER;

#define CLEAN_BUFFER _cleanup_(buffer_freep) BUFFER

#define buffer_cacheable(wb)    do { (wb)->options |= WB_CONTENT_CACHEABLE;    if((wb)->options & WB_CONTENT_NO_CACHEABLE) (wb)->options &= ~WB_CONTENT_NO_CACHEABLE; } while(0)
#define buffer_no_cacheable(wb) do { (wb)->options |= WB_CONTENT_NO_CACHEABLE; if((wb)->options & WB_CONTENT_CACHEABLE)    (wb)->options &= ~WB_CONTENT_CACHEABLE;  (wb)->expires = 0; } while(0)

#define buffer_strlen(wb) (size_t)((wb)->len)

#define BUFFER_OVERFLOW_EOF "EOF"

#ifdef NETDATA_INTERNAL_CHECKS
#define buffer_overflow_check(b) _buffer_overflow_check(b)
#else
#define buffer_overflow_check(b)
#endif

static inline void _buffer_overflow_check(BUFFER *b __maybe_unused) {
    assert(b->len <= b->size &&
                   "BUFFER: length is above buffer size.");

    assert(!(b->buffer && (b->buffer[b->size] != '\0' || strcmp(&b->buffer[b->size + 1], BUFFER_OVERFLOW_EOF) != 0)) &&
                   "BUFFER: detected overflow.");
}

static ALWAYS_INLINE void buffer_flush(BUFFER *wb) {
    wb->len = 0;

    wb->json.depth = 0;
    wb->json.stack[0].type = BUFFER_JSON_EMPTY;
    wb->json.stack[0].count = 0;

    if(wb->buffer)
        wb->buffer[0] = '\0';
}

void buffer_reset(BUFFER *wb);

void buffer_date(BUFFER *wb, int year, int month, int day, int hours, int minutes, int seconds);
void buffer_jsdate(BUFFER *wb, int year, int month, int day, int hours, int minutes, int seconds);

BUFFER *buffer_create(size_t size, size_t *statistics);
void buffer_free(BUFFER *b);
void buffer_increase(BUFFER *b, size_t free_size_required);

static inline void buffer_freep(BUFFER **bp) {
    if(bp) buffer_free(*bp);
}

void buffer_snprintf(BUFFER *wb, size_t len, const char *fmt, ...) PRINTFLIKE(3, 4);
void buffer_vsprintf(BUFFER *wb, const char *fmt, va_list args);
void buffer_sprintf(BUFFER *wb, const char *fmt, ...) PRINTFLIKE(2,3);
void buffer_strcat_htmlescape(BUFFER *wb, const char *txt);

void buffer_char_replace(BUFFER *wb, char from, char to);

void buffer_print_sn_flags(BUFFER *wb, SN_FLAGS flags, bool send_anomaly_bit);

static ALWAYS_INLINE void buffer_need_bytes(BUFFER *buffer, size_t needed_free_size) {
    if(unlikely(buffer->len + needed_free_size >= buffer->size))
        buffer_increase(buffer, needed_free_size + 1);
}

void buffer_json_initialize(BUFFER *wb, const char *key_quote, const char *value_quote, int depth,
                            bool add_anonymous_object, BUFFER_JSON_OPTIONS options);

void buffer_json_finalize(BUFFER *wb);

static ALWAYS_INLINE const char *buffer_tostring(BUFFER *wb)
{
    if(unlikely(!wb))
        return NULL;

    buffer_need_bytes(wb, 1);
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);

    return(wb->buffer);
}

static ALWAYS_INLINE void _buffer_json_depth_push(BUFFER *wb, BUFFER_JSON_NODE_TYPE type) {
#ifdef NETDATA_INTERNAL_CHECKS
    assert(wb->json.depth <= BUFFER_JSON_MAX_DEPTH && "BUFFER JSON: max nesting reached");
#endif
    wb->json.depth++;
#ifdef NETDATA_INTERNAL_CHECKS
    assert(wb->json.depth >= 0 && "Depth wrapped around and is negative");
#endif
    wb->json.stack[wb->json.depth].count = 0;
    wb->json.stack[wb->json.depth].type = type;
}

static ALWAYS_INLINE void _buffer_json_depth_pop(BUFFER *wb) {
    wb->json.depth--;
}

static ALWAYS_INLINE void buffer_putc(BUFFER *wb, char c) {
    buffer_need_bytes(wb, 2);
    wb->buffer[wb->len++] = c;
    wb->buffer[wb->len] = '\0';
    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_fast_rawcat(BUFFER *wb, const char *txt, size_t len) {
    if(unlikely(!txt || !*txt || !len)) return;

    buffer_need_bytes(wb, len + 1);

    const char *t = txt;
    const char *e = &txt[len];

    char *d = &wb->buffer[wb->len];

    while(t != e)
        *d++ = *t++;

    wb->len += len;
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_fast_strcat(BUFFER *wb, const char *txt, size_t len) {
    if(unlikely(!txt || !*txt || !len)) return;

    buffer_need_bytes(wb, len + 1);

    const char *t = txt;
    const char *e = &txt[len];

    char *d = &wb->buffer[wb->len];

    while(t != e
#ifdef NETDATA_INTERNAL_CHECKS
          && *t
#endif
            )
        *d++ = *t++;

#ifdef NETDATA_INTERNAL_CHECKS
    assert(!(t != e && !*t) && "BUFFER: source string is shorter than the length given.");
#endif

    wb->len += len;
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_strcat(BUFFER *wb, const char *txt) {
    if(unlikely(!txt || !*txt)) return;

    const char *t = txt;
    while(*t) {
        buffer_need_bytes(wb, 100);
        char *s = &wb->buffer[wb->len];
        char *d = s;
        const char *e = &wb->buffer[wb->size];

        while(*t && d < e)
            *d++ = *t++;

        wb->len += d - s;
    }

    buffer_need_bytes(wb, 1);
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_contents_replace(BUFFER *wb, const char *txt, size_t len) {
    wb->len = 0;
    buffer_need_bytes(wb, len + 1);

    memcpy(wb->buffer, txt, len);
    wb->len = len;
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_strncat(BUFFER *wb, const char *txt, size_t len) {
    if(unlikely(!txt || !*txt)) return;

    buffer_need_bytes(wb, len + 1);

    memcpy(&wb->buffer[wb->len], txt, len);

    wb->len += len;
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_memcat(BUFFER *wb, const void *mem, size_t bytes) {
    if(unlikely(!mem)) return;

    buffer_need_bytes(wb, bytes + 1);

    memcpy(&wb->buffer[wb->len], mem, bytes);

    wb->len += bytes;
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_json_strcat(BUFFER *wb, const char *txt)
{
    if(unlikely(!txt || !*txt)) return;

    const unsigned char *t = (const unsigned char *)txt;
    while(*t) {
        buffer_need_bytes(wb, 110);
        unsigned char *s = (unsigned char *)&wb->buffer[wb->len];
        unsigned char *d = s;
        const unsigned char *e = (unsigned char *)&wb->buffer[wb->size - 10]; // make room for the max escape sequence

        while(*t && d < e) {
#ifdef BUFFER_JSON_ESCAPE_UTF
            if(unlikely(IS_UTF8_STARTBYTE(*t) && IS_UTF8_BYTE(t[1]))) {
                // UTF-8 multi-byte encoded character

                // find how big this character is (2-4 bytes)
                size_t utf_character_size = 2;
                while(utf_character_size < 4 && t[utf_character_size] && IS_UTF8_BYTE(t[utf_character_size]) && !IS_UTF8_STARTBYTE(t[utf_character_size]))
                    utf_character_size++;

                uint32_t code_point = 0;
                for (size_t i = 0; i < utf_character_size; i++) {
                    code_point <<= 6;
                    code_point |= (t[i] & 0x3F);
                }

                t += utf_character_size;

                // encode as \u escape sequence
                *d++ = '\\';
                *d++ = 'u';
                *d++ = hex_digits[(code_point >> 12) & 0xf];
                *d++ = hex_digits[(code_point >> 8) & 0xf];
                *d++ = hex_digits[(code_point >> 4) & 0xf];
                *d++ = hex_digits[code_point & 0xf];
            }
            else
#endif
            if(unlikely(*t < ' ')) {
                uint32_t v = *t++;
                *d++ = '\\';
                *d++ = 'u';
                *d++ = hex_digits[(v >> 12) & 0xf];
                *d++ = hex_digits[(v >> 8) & 0xf];
                *d++ = hex_digits[(v >> 4) & 0xf];
                *d++ = hex_digits[v & 0xf];
            }
            else {
                if (unlikely(*t == '\\' || *t == '\"'))
                    *d++ = '\\';

                *d++ = *t++;
            }
        }

        wb->len += d - s;
    }

    buffer_need_bytes(wb, 1);
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_json_quoted_strcat(BUFFER *wb, const char *txt) {
    if(unlikely(!txt || !*txt)) return;

    if(*txt == '"')
        txt++;

    const char *t = txt;
    while(*t) {
        buffer_need_bytes(wb, 100);
        char *s = &wb->buffer[wb->len];
        char *d = s;
        const char *e = &wb->buffer[wb->size - 1]; // remove 1 to make room for the escape character

        while(*t && d < e) {
            if(unlikely(*t == '"' && !t[1])) {
                t++;
                continue;
            }

            if(unlikely(*t == '\\' || *t == '"'))
                *d++ = '\\';

            *d++ = *t++;
        }

        wb->len += d - s;
    }

    buffer_need_bytes(wb, 1);
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
}

// This trick seems to give an 80% speed increase in 32bit systems
// print_number_llu_r() will just print the digits up to the
// point the remaining value fits in 32 bits, and then calls
// print_number_lu_r() to print the rest with 32 bit arithmetic.

static ALWAYS_INLINE char *print_uint32_reversed(char *dst, uint32_t value) {
    char *d = dst;
    do *d++ = (char)('0' + (value % 10)); while((value /= 10));
    return d;
}

static ALWAYS_INLINE char *print_uint64_reversed(char *dst, uint64_t value) {
#ifdef ENV32BIT
    if(value <= (uint64_t)0xffffffff)
        return print_uint32_reversed(dst, value);

    char *d = dst;
    do *d++ = (char)('0' + (value % 10)); while((value /= 10) && value > (uint64_t)0xffffffff);
    if(value) return print_uint32_reversed(d, value);
    return d;
#else
    char *d = dst;
    do *d++ = (char)('0' + (value % 10)); while((value /= 10));
    return d;
#endif
}

static ALWAYS_INLINE char *print_uint32_hex_reversed(char *dst, uint32_t value) {
    static const char *digits = "0123456789ABCDEF";
    char *d = dst;
    do *d++ = digits[value & 0xf]; while((value >>= 4));
    return d;
}

static ALWAYS_INLINE char *print_uint64_hex_reversed(char *dst, uint64_t value) {
#ifdef ENV32BIT
    if(value <= (uint64_t)0xffffffff)
        return print_uint32_hex_reversed(dst, value);

    char *d = dst;
    do *d++ = hex_digits[value & 0xf]; while((value >>= 4) && value > (uint64_t)0xffffffff);
    if(value) return print_uint32_hex_reversed(d, value);
    return d;
#else
    char *d = dst;
    do *d++ = hex_digits[value & 0xf]; while((value >>= 4));
    return d;
#endif
}

static ALWAYS_INLINE char *print_uint64_hex_reversed_full(char *dst, uint64_t value) {
    char *d = dst;
    for(size_t c = 0; c < sizeof(uint64_t) * 2; c++) {
        *d++ = hex_digits[value & 0xf];
        value >>= 4;
    }

    return d;
}

static ALWAYS_INLINE char *print_uint64_base64_reversed(char *dst, uint64_t value) {
    char *d = dst;
    do *d++ = base64_digits[value & 63]; while ((value >>= 6));
    return d;
}

static ALWAYS_INLINE void char_array_reverse(char *from, char *to) {
    // from and to are inclusive
    char *begin = from, *end = to, aux;
    while (end > begin) aux = *end, *end-- = *begin, *begin++ = aux;
}

static ALWAYS_INLINE int print_netdata_double(char *dst, NETDATA_DOUBLE value) {
    char *s = dst;

    if(unlikely(value < 0)) {
        *s++ = '-';
        value = fabsndd(value);
    }

    uint64_t fractional_precision = 10000000ULL; // fractional part 7 digits
    int fractional_wanted_digits = 7;
    int exponent = 0;
    if(unlikely(value >= (NETDATA_DOUBLE)(UINT64_MAX / 10))) {
        // the number is too big to print using 64bit numbers
        // so, let's convert it to exponential notation
        exponent = (int)(floorndd(log10ndd(value)));
        value /= powndd(10, exponent);

        // the max precision we can support is 18 digits
        // (UINT64_MAX is 20, but the first is 1)
        fractional_precision = 1000000000000000000ULL; // fractional part 18 digits
        fractional_wanted_digits = 18;
    }

    char *d = s;
    NETDATA_DOUBLE integral_d, fractional_d;
    fractional_d = modfndd(value, &integral_d);

    // get the integral and the fractional parts as 64-bit integers
    uint64_t integral = (uint64_t)integral_d;
    uint64_t fractional = (uint64_t)llrintndd(fractional_d * (NETDATA_DOUBLE)fractional_precision);
    if(unlikely(fractional >= fractional_precision)) {
        integral++;
        fractional -= fractional_precision;
    }

    // convert the integral part to string (reversed)
    d = print_uint64_reversed(d, integral);
    char_array_reverse(s, d - 1);      // copy reversed the integral string

    if(likely(fractional != 0)) {
        *d++ = '.'; // add the dot

        // convert the fractional part to string (reversed)
        d = print_uint64_reversed(s = d, fractional);

        while(d - s < fractional_wanted_digits) *d++ = '0'; // prepend zeros to reach precision
        char_array_reverse(s, d - 1);   // copy reversed the fractional string

        // remove trailing zeros from the fractional part
        while(*(d - 1) == '0') d--;
    }

    if(unlikely(exponent != 0)) {
        *d++ = 'e';
        *d++ = '+';
        d = print_uint32_reversed(s = d, exponent);
        char_array_reverse(s, d - 1);
    }

    *d = '\0';
    return (int)(d - dst);
}

static ALWAYS_INLINE size_t print_uint64(char *dst, uint64_t value) {
    char *s = dst;
    char *d = print_uint64_reversed(s, value);
    char_array_reverse(s, d - 1);
    *d = '\0';
    return d - s;
}

static ALWAYS_INLINE size_t print_int64(char *dst, int64_t value) {
    size_t len = 0;

    if(value < 0) {
        *dst++ = '-';
        value = -value;
        len++;
    }

    return print_uint64(dst, value) + len;
}

#define UINT64_MAX_LENGTH (24) // 21 should be enough
static ALWAYS_INLINE void buffer_print_uint64(BUFFER *wb, uint64_t value) {
    buffer_need_bytes(wb, UINT64_MAX_LENGTH);
    wb->len += print_uint64(&wb->buffer[wb->len], value);
    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_print_int64(BUFFER *wb, int64_t value) {
    buffer_need_bytes(wb, UINT64_MAX_LENGTH);
    wb->len += print_int64(&wb->buffer[wb->len], value);
    buffer_overflow_check(wb);
}

#define UINT64_HEX_MAX_LENGTH ((sizeof(HEX_PREFIX) - 1) + (sizeof(uint64_t) * 2) + 1)
static ALWAYS_INLINE size_t print_uint64_hex(char *dst, uint64_t value) {
    char *d = dst;

    const char *s = HEX_PREFIX;
    while(*s) *d++ = *s++;

    char *e = print_uint64_hex_reversed(d, value);
    char_array_reverse(d, e - 1);
    *e = '\0';
    return e - dst;
}

static ALWAYS_INLINE size_t print_uint64_hex_full(char *dst, uint64_t value) {
    char *d = dst;

    const char *s = HEX_PREFIX;
    while(*s) *d++ = *s++;

    char *e = print_uint64_hex_reversed_full(d, value);
    char_array_reverse(d, e - 1);
    *e = '\0';
    return e - dst;
}

static ALWAYS_INLINE void buffer_print_uint64_hex(BUFFER *wb, uint64_t value) {
    buffer_need_bytes(wb, UINT64_HEX_MAX_LENGTH);
    wb->len += print_uint64_hex(&wb->buffer[wb->len], value);
    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_print_uint64_hex_full(BUFFER *wb, uint64_t value) {
    buffer_need_bytes(wb, UINT64_HEX_MAX_LENGTH);
    wb->len += print_uint64_hex_full(&wb->buffer[wb->len], value);
    buffer_overflow_check(wb);
}

#define UINT64_B64_MAX_LENGTH ((sizeof(IEEE754_UINT64_B64_PREFIX) - 1) + (sizeof(uint64_t) * 2) + 1)
static ALWAYS_INLINE void buffer_print_uint64_base64(BUFFER *wb, uint64_t value) {
    buffer_need_bytes(wb, UINT64_B64_MAX_LENGTH);

    buffer_fast_strcat(wb, IEEE754_UINT64_B64_PREFIX, sizeof(IEEE754_UINT64_B64_PREFIX) - 1);

    char *s = &wb->buffer[wb->len];
    char *d = print_uint64_base64_reversed(s, value);
    char_array_reverse(s, d - 1);
    *d = '\0';
    wb->len += d - s;

    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_print_int64_hex(BUFFER *wb, int64_t value) {
    buffer_need_bytes(wb, 2);

    if(value < 0) {
        buffer_fast_strcat(wb, "-", 1);
        value = -value;
    }

    buffer_print_uint64_hex(wb, (uint64_t)value);

    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_print_int64_base64(BUFFER *wb, int64_t value) {
    buffer_need_bytes(wb, 2);

    if(value < 0) {
        buffer_fast_strcat(wb, "-", 1);
        value = -value;
    }

    buffer_print_uint64_base64(wb, (uint64_t)value);

    buffer_overflow_check(wb);
}

#define DOUBLE_MAX_LENGTH (512) // 318 should be enough, including null
static ALWAYS_INLINE void buffer_print_netdata_double(BUFFER *wb, NETDATA_DOUBLE value) {
    buffer_need_bytes(wb, DOUBLE_MAX_LENGTH);

    if(isnan(value) || isinf(value)) {
        buffer_fast_strcat(wb, "null", 4);
        return;
    }
    else
        wb->len += print_netdata_double(&wb->buffer[wb->len], value);

    // terminate it
    buffer_need_bytes(wb, 1);
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
}

#define DOUBLE_HEX_MAX_LENGTH ((sizeof(IEEE754_DOUBLE_HEX_PREFIX) - 1) + (sizeof(uint64_t) * 2) + 1)
static ALWAYS_INLINE void buffer_print_netdata_double_hex(BUFFER *wb, NETDATA_DOUBLE value) {
    buffer_need_bytes(wb, DOUBLE_HEX_MAX_LENGTH);

    uint64_t *ptr = (uint64_t *) (&value);
    buffer_fast_strcat(wb, IEEE754_DOUBLE_HEX_PREFIX, sizeof(IEEE754_DOUBLE_HEX_PREFIX) - 1);

    char *s = &wb->buffer[wb->len];
    char *d = print_uint64_hex_reversed(s, *ptr);
    char_array_reverse(s, d - 1);
    *d = '\0';
    wb->len += d - s;

    buffer_overflow_check(wb);
}

#define DOUBLE_B64_MAX_LENGTH ((sizeof(IEEE754_DOUBLE_B64_PREFIX) - 1) + (sizeof(uint64_t) * 2) + 1)
static ALWAYS_INLINE void buffer_print_netdata_double_base64(BUFFER *wb, NETDATA_DOUBLE value) {
    buffer_need_bytes(wb, DOUBLE_B64_MAX_LENGTH);

    uint64_t *ptr = (uint64_t *) (&value);
    buffer_fast_strcat(wb, IEEE754_DOUBLE_B64_PREFIX, sizeof(IEEE754_DOUBLE_B64_PREFIX) - 1);

    char *s = &wb->buffer[wb->len];
    char *d = print_uint64_base64_reversed(s, *ptr);
    char_array_reverse(s, d - 1);
    *d = '\0';
    wb->len += d - s;

    buffer_overflow_check(wb);
}

typedef enum {
    NUMBER_ENCODING_DECIMAL,
    NUMBER_ENCODING_HEX,
    NUMBER_ENCODING_BASE64,
} NUMBER_ENCODING;

static ALWAYS_INLINE void buffer_print_int64_encoded(BUFFER *wb, NUMBER_ENCODING encoding, int64_t value) {
    if(encoding == NUMBER_ENCODING_BASE64)
        return buffer_print_int64_base64(wb, value);

    if(encoding == NUMBER_ENCODING_HEX)
        return buffer_print_int64_hex(wb, value);

    return buffer_print_int64(wb, value);
}

static ALWAYS_INLINE void buffer_print_uint64_encoded(BUFFER *wb, NUMBER_ENCODING encoding, uint64_t value) {
    if(encoding == NUMBER_ENCODING_BASE64)
        return buffer_print_uint64_base64(wb, value);

    if(encoding == NUMBER_ENCODING_HEX)
        return buffer_print_uint64_hex(wb, value);

    return buffer_print_uint64(wb, value);
}

static ALWAYS_INLINE void buffer_print_netdata_double_encoded(BUFFER *wb, NUMBER_ENCODING encoding, NETDATA_DOUBLE value) {
    if(encoding == NUMBER_ENCODING_BASE64)
        return buffer_print_netdata_double_base64(wb, value);

    if(encoding == NUMBER_ENCODING_HEX)
        return buffer_print_netdata_double_hex(wb, value);

    return buffer_print_netdata_double(wb, value);
}

static ALWAYS_INLINE void buffer_print_spaces(BUFFER *wb, size_t spaces) {
    buffer_need_bytes(wb, spaces * 4 + 1);

    char *d = &wb->buffer[wb->len];
    for(size_t i = 0; i < spaces; i++) {
        *d++ = ' ';
        *d++ = ' ';
        *d++ = ' ';
        *d++ = ' ';
    }

    *d = '\0';
    wb->len += spaces * 4;

    buffer_overflow_check(wb);
}

static ALWAYS_INLINE void buffer_print_json_comma(BUFFER *wb) {
    if(wb->json.stack[wb->json.depth].count)
        buffer_fast_strcat(wb, ",", 1);
}

static ALWAYS_INLINE void buffer_print_json_comma_newline_spacing(BUFFER *wb) {
    buffer_print_json_comma(wb);

    if((wb->json.options & BUFFER_JSON_OPTIONS_MINIFY) ||
        (wb->json.stack[wb->json.depth].type == BUFFER_JSON_ARRAY && !(wb->json.options & BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAY_ITEMS)))
        return;

    buffer_fast_strcat(wb, "\n", 1);
    buffer_print_spaces(wb, wb->json.depth + 1);
}

static ALWAYS_INLINE void buffer_print_json_key(BUFFER *wb, const char *key) {
    buffer_strcat(wb, wb->json.key_quote);
    buffer_json_strcat(wb, key);
    buffer_strcat(wb, wb->json.key_quote);
}

static ALWAYS_INLINE void buffer_json_add_string_value(BUFFER *wb, const char *value) {
    if(value) {
        buffer_strcat(wb, wb->json.value_quote);
        buffer_json_strcat(wb, value);
        buffer_strcat(wb, wb->json.value_quote);
    }
    else
        buffer_fast_strcat(wb, "null", 4);
}

static ALWAYS_INLINE void buffer_json_add_quoted_string_value(BUFFER *wb, const char *value) {
    if(value) {
        buffer_strcat(wb, wb->json.value_quote);
        buffer_json_quoted_strcat(wb, value);
        buffer_strcat(wb, wb->json.value_quote);
    }
    else
        buffer_fast_strcat(wb, "null", 4);
}

static inline void buffer_json_member_add_object(BUFFER *wb, const char *key) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":{", 2);
    wb->json.stack[wb->json.depth].count++;

    _buffer_json_depth_push(wb, BUFFER_JSON_OBJECT);
}

static inline void buffer_json_object_close(BUFFER *wb) {
#ifdef NETDATA_INTERNAL_CHECKS
    assert(wb->json.depth >= 0 && "BUFFER JSON: nothing is open to close it");
    assert(wb->json.stack[wb->json.depth].type == BUFFER_JSON_OBJECT && "BUFFER JSON: an object is not open to close it");
#endif
    if(!(wb->json.options & BUFFER_JSON_OPTIONS_MINIFY)) {
        buffer_fast_strcat(wb, "\n", 1);
        buffer_print_spaces(wb, wb->json.depth);
    }
    buffer_fast_strcat(wb, "}", 1);
    _buffer_json_depth_pop(wb);
}

static inline void buffer_json_member_add_string(BUFFER *wb, const char *key, const char *value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);
    buffer_json_add_string_value(wb, value);

    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_member_add_string_or_omit(BUFFER *wb, const char *key, const char *value) {
    if(value && *value)
        buffer_json_member_add_string(wb, key, value);
}

static inline void buffer_json_member_add_string_or_empty(BUFFER *wb, const char *key, const char *value) {
    if(!value)
        value = "";

    buffer_json_member_add_string(wb, key, value);
}

void buffer_json_member_add_datetime_rfc3339(BUFFER *wb, const char *key, uint64_t datetime_ut, bool utc);
void buffer_json_member_add_duration_ut(BUFFER *wb, const char *key, int64_t duration_ut);

static inline void buffer_json_member_add_quoted_string(BUFFER *wb, const char *key, const char *value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);

    if(!value || strcmp(value, "null") == 0)
        buffer_fast_strcat(wb, "null", 4);
    else
        buffer_json_add_quoted_string_value(wb, value);

    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_member_add_uuid_ptr(BUFFER *wb, const char *key, nd_uuid_t *value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);

    if(value && !uuid_is_null(*value)) {
        char uuid[GUID_LEN + 1];
        uuid_unparse_lower(*value, uuid);
        buffer_json_add_string_value(wb, uuid);
    }
    else
        buffer_json_add_string_value(wb, NULL);

    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_member_add_uuid(BUFFER *wb, const char *key, nd_uuid_t value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);

    if(!uuid_is_null(value)) {
        char uuid[GUID_LEN + 1];
        uuid_unparse_lower(value, uuid);
        buffer_json_add_string_value(wb, uuid);
    }
    else
        buffer_json_add_string_value(wb, NULL);

    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_member_add_boolean(BUFFER *wb, const char *key, bool value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);
    buffer_strcat(wb, value?"true":"false");

    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_member_add_array(BUFFER *wb, const char *key) {
    buffer_print_json_comma_newline_spacing(wb);
    if (key) {
        buffer_print_json_key(wb, key);
        buffer_fast_strcat(wb, ":[", 2);
    }
    else
        buffer_fast_strcat(wb, "[", 1);

    wb->json.stack[wb->json.depth].count++;

    _buffer_json_depth_push(wb, BUFFER_JSON_ARRAY);
}

static inline void buffer_json_add_array_item_array(BUFFER *wb) {
    if(!(wb->json.options & BUFFER_JSON_OPTIONS_MINIFY) && wb->json.stack[wb->json.depth].type == BUFFER_JSON_ARRAY) {
        // an array inside another array
        buffer_print_json_comma(wb);
        buffer_fast_strcat(wb, "\n", 1);
        buffer_print_spaces(wb, wb->json.depth + 1);
    }
    else
        buffer_print_json_comma_newline_spacing(wb);

    buffer_fast_strcat(wb, "[", 1);
    wb->json.stack[wb->json.depth].count++;

    _buffer_json_depth_push(wb, BUFFER_JSON_ARRAY);
}

static inline void buffer_json_add_array_item_string(BUFFER *wb, const char *value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_json_add_string_value(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_add_array_item_uuid(BUFFER *wb, nd_uuid_t *value) {
    if(value && !uuid_is_null(*value)) {
        char uuid[GUID_LEN + 1];
        uuid_unparse_lower(*value, uuid);
        buffer_json_add_array_item_string(wb, uuid);
    }
    else
        buffer_json_add_array_item_string(wb, NULL);
}

static inline void buffer_json_add_array_item_uuid_compact(BUFFER *wb, nd_uuid_t *value) {
    if(value && !uuid_is_null(*value)) {
        char uuid[GUID_LEN + 1];
        uuid_unparse_lower_compact(*value, uuid);
        buffer_json_add_array_item_string(wb, uuid);
    }
    else
        buffer_json_add_array_item_string(wb, NULL);
}

static inline void buffer_json_add_array_item_double(BUFFER *wb, NETDATA_DOUBLE value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_print_netdata_double(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_add_array_item_int64(BUFFER *wb, int64_t value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_print_int64(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_add_array_item_uint64(BUFFER *wb, uint64_t value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_print_uint64(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_add_array_item_boolean(BUFFER *wb, bool value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_strcat(wb, value ? "true" : "false");
    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_add_array_item_time_t(BUFFER *wb, time_t value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_print_int64(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_add_array_item_time_ms(BUFFER *wb, time_t value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_print_int64(wb, value);
    buffer_fast_strcat(wb, "000", 3);
    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_add_array_item_time_t2ms(BUFFER *wb, time_t value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_print_int64(wb, value);
    buffer_fast_strcat(wb, "000", 3);
    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_add_array_item_object(BUFFER *wb) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_fast_strcat(wb, "{", 1);
    wb->json.stack[wb->json.depth].count++;

    _buffer_json_depth_push(wb, BUFFER_JSON_OBJECT);
}

static inline void buffer_json_member_add_time_t(BUFFER *wb, const char *key, time_t value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);
    buffer_print_int64(wb, value);

    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_member_add_time_t2ms(BUFFER *wb, const char *key, time_t value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);
    buffer_print_int64(wb, value);
    buffer_fast_strcat(wb, "000", 3);

    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_member_add_uint64(BUFFER *wb, const char *key, uint64_t value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);
    buffer_print_uint64(wb, value);

    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_member_add_int64(BUFFER *wb, const char *key, int64_t value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);
    buffer_print_int64(wb, value);

    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_member_add_double(BUFFER *wb, const char *key, NETDATA_DOUBLE value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);
    buffer_print_netdata_double(wb, value);

    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_array_close(BUFFER *wb) {
#ifdef NETDATA_INTERNAL_CHECKS
    assert(wb->json.depth >= 0 && "BUFFER JSON: nothing is open to close it");
    assert(wb->json.stack[wb->json.depth].type == BUFFER_JSON_ARRAY && "BUFFER JSON: an array is not open to close it");
#endif
    if(wb->json.options & BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAY_ITEMS) {
        buffer_fast_strcat(wb, "\n", 1);
        buffer_print_spaces(wb, wb->json.depth);
    }

    buffer_fast_strcat(wb, "]", 1);
    _buffer_json_depth_pop(wb);
}

typedef enum __attribute__((packed)) {
    RRDF_FIELD_OPTS_NONE         = 0,
    RRDF_FIELD_OPTS_UNIQUE_KEY   = (1 << 0), // the field is the unique key of the row
    RRDF_FIELD_OPTS_VISIBLE      = (1 << 1), // the field should be visible by default
    RRDF_FIELD_OPTS_STICKY       = (1 << 2), // the field should be sticky
    RRDF_FIELD_OPTS_FULL_WIDTH   = (1 << 3), // the field should get full width
    RRDF_FIELD_OPTS_WRAP         = (1 << 4), // the field should wrap
    RRDF_FIELD_OPTS_DUMMY        = (1 << 5), // not a presentable field
    RRDF_FIELD_OPTS_EXPANDED_FILTER = (1 << 6), // show the filter expanded
} RRDF_FIELD_OPTIONS;

typedef enum __attribute__((packed)) {
    RRDF_FIELD_TYPE_NONE,
    RRDF_FIELD_TYPE_INTEGER,
    RRDF_FIELD_TYPE_BOOLEAN,
    RRDF_FIELD_TYPE_STRING,
    RRDF_FIELD_TYPE_DETAIL_STRING,
    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
    RRDF_FIELD_TYPE_DURATION,
    RRDF_FIELD_TYPE_TIMESTAMP,
    RRDF_FIELD_TYPE_ARRAY,
} RRDF_FIELD_TYPE;

static inline const char *rrdf_field_type_to_string(RRDF_FIELD_TYPE type) {
    switch(type) {
        default:
        case RRDF_FIELD_TYPE_NONE:
            return "none";

        case RRDF_FIELD_TYPE_INTEGER:
            return "integer";

        case RRDF_FIELD_TYPE_BOOLEAN:
            return "boolean";

        case RRDF_FIELD_TYPE_STRING:
            return "string";

        case RRDF_FIELD_TYPE_DETAIL_STRING:
            return "detail-string";

        case RRDF_FIELD_TYPE_BAR_WITH_INTEGER:
            return "bar-with-integer";

        case RRDF_FIELD_TYPE_DURATION:
            return "duration";

        case RRDF_FIELD_TYPE_TIMESTAMP:
            return "timestamp";

        case RRDF_FIELD_TYPE_ARRAY:
            return "array";
    }
}

typedef enum __attribute__((packed)) {
    RRDF_FIELD_VISUAL_VALUE,        // show the value, possibly applying a transformation
    RRDF_FIELD_VISUAL_BAR,          // show the value and a bar, respecting the max field to fill the bar at 100%
    RRDF_FIELD_VISUAL_PILL,         //
    RRDF_FIELD_VISUAL_RICH,         //
    RRDR_FIELD_VISUAL_ROW_OPTIONS,  // this is a dummy column that is used for row options
} RRDF_FIELD_VISUAL;

static inline const char *rrdf_field_visual_to_string(RRDF_FIELD_VISUAL visual) {
    switch(visual) {
        default:
        case RRDF_FIELD_VISUAL_VALUE:
            return "value";

        case RRDF_FIELD_VISUAL_BAR:
            return "bar";

        case RRDF_FIELD_VISUAL_PILL:
            return "pill";

        case RRDF_FIELD_VISUAL_RICH:
            return "richValue";

        case RRDR_FIELD_VISUAL_ROW_OPTIONS:
            return "rowOptions";
    }
}

typedef enum __attribute__((packed)) {
    RRDF_FIELD_TRANSFORM_NONE,      // show the value as-is
    RRDF_FIELD_TRANSFORM_NUMBER,    // show the value respecting the decimal_points
    RRDF_FIELD_TRANSFORM_DURATION_S,  // transform as duration in second to a human-readable duration
    RRDF_FIELD_TRANSFORM_DATETIME_MS,  // UNIX epoch timestamp in ms
    RRDF_FIELD_TRANSFORM_DATETIME_USEC,  // UNIX epoch timestamp in usec
    RRDF_FIELD_TRANSFORM_XML,   // format the field with an XML prettifier
} RRDF_FIELD_TRANSFORM;

static inline const char *rrdf_field_transform_to_string(RRDF_FIELD_TRANSFORM transform) {
    switch(transform) {
        default:
        case RRDF_FIELD_TRANSFORM_NONE:
            return "none";

        case RRDF_FIELD_TRANSFORM_NUMBER:
            return "number";

        case RRDF_FIELD_TRANSFORM_DURATION_S:
            return "duration";

        case RRDF_FIELD_TRANSFORM_DATETIME_MS:
            return "datetime";

        case RRDF_FIELD_TRANSFORM_DATETIME_USEC:
            return "datetime_usec";

        case RRDF_FIELD_TRANSFORM_XML:
            return "xml";
    }
}

typedef enum __attribute__((packed)) {
    RRDF_FIELD_SORT_ASCENDING  = (1 << 0),
    RRDF_FIELD_SORT_DESCENDING = (1 << 1),

    RRDF_FIELD_SORT_FIXED      = (1 << 7),
} RRDF_FIELD_SORT;

static inline const char *rrdf_field_sort_to_string(RRDF_FIELD_SORT sort) {
    if(sort & RRDF_FIELD_SORT_DESCENDING)
        return "descending";

    else
        return "ascending";
}

typedef enum __attribute__((packed)) {
    RRDF_FIELD_SUMMARY_UNIQUECOUNT,     // Finds the number of unique values of a group of rows
    RRDF_FIELD_SUMMARY_SUM,             // Sums the values of a group of rows
    RRDF_FIELD_SUMMARY_MIN,             // Finds the minimum value of a group of rows
    RRDF_FIELD_SUMMARY_MAX,             // Finds the maximum value of a group of rows
    // RRDF_FIELD_SUMMARY_EXTENT,          // Finds the minimum and maximum values of a group of rows
    RRDF_FIELD_SUMMARY_MEAN,            // Finds the mean/average value of a group of rows
    RRDF_FIELD_SUMMARY_MEDIAN,          // Finds the median value of a group of rows
    // RRDF_FIELD_SUMMARY_UNIQUE,         // Finds the unique values of a group of rows
    RRDF_FIELD_SUMMARY_COUNT,           // Calculates the number of rows in a group
} RRDF_FIELD_SUMMARY;

static inline const char *rrdf_field_summary_to_string(RRDF_FIELD_SUMMARY summary) {
    switch(summary) {
        default:
        case RRDF_FIELD_SUMMARY_COUNT:
            return "count";

        case RRDF_FIELD_SUMMARY_UNIQUECOUNT:
            return "uniqueCount";

        case RRDF_FIELD_SUMMARY_SUM:
            return "sum";

        case RRDF_FIELD_SUMMARY_MIN:
            return "min";

        case RRDF_FIELD_SUMMARY_MEAN:
            return "mean";

        case RRDF_FIELD_SUMMARY_MEDIAN:
            return "median";

        case RRDF_FIELD_SUMMARY_MAX:
            return "max";
    }
}

typedef enum __attribute__((packed)) {
    RRDF_FIELD_FILTER_NONE = 0,
    RRDF_FIELD_FILTER_RANGE,
    RRDF_FIELD_FILTER_MULTISELECT,
    RRDF_FIELD_FILTER_FACET,
} RRDF_FIELD_FILTER;

static inline const char *rrdf_field_filter_to_string(RRDF_FIELD_FILTER filter) {
    switch(filter) {
        case RRDF_FIELD_FILTER_RANGE:
            return "range";

        case RRDF_FIELD_FILTER_MULTISELECT:
            return "multiselect";

        case RRDF_FIELD_FILTER_FACET:
            return "facet";

        default:
        case RRDF_FIELD_FILTER_NONE:
            return "none";
    }
}

static inline void
buffer_rrdf_table_add_field(BUFFER *wb, size_t field_id, const char *key, const char *name, RRDF_FIELD_TYPE type,
                            RRDF_FIELD_VISUAL visual, RRDF_FIELD_TRANSFORM transform, size_t decimal_points,
                            const char *units, NETDATA_DOUBLE max, RRDF_FIELD_SORT sort, const char *pointer_to,
                            RRDF_FIELD_SUMMARY summary, RRDF_FIELD_FILTER filter, RRDF_FIELD_OPTIONS options,
                            const char *default_value) {

    buffer_json_member_add_object(wb, key);
    {
        buffer_json_member_add_uint64(wb, "index", field_id);
        buffer_json_member_add_boolean(wb, "unique_key", options & RRDF_FIELD_OPTS_UNIQUE_KEY);
        buffer_json_member_add_string(wb, "name", name);
        buffer_json_member_add_boolean(wb, "visible", options & RRDF_FIELD_OPTS_VISIBLE);
        buffer_json_member_add_string(wb, "type", rrdf_field_type_to_string(type));
        buffer_json_member_add_string_or_omit(wb, "units", units);
        buffer_json_member_add_string(wb, "visualization", rrdf_field_visual_to_string(visual));

        buffer_json_member_add_object(wb, "value_options");
        {
            buffer_json_member_add_string_or_omit(wb, "units", units);
            buffer_json_member_add_string(wb, "transform", rrdf_field_transform_to_string(transform));
            buffer_json_member_add_uint64(wb, "decimal_points", decimal_points);
            buffer_json_member_add_string(wb, "default_value", default_value);
        }
        buffer_json_object_close(wb);

        if (!isnan((NETDATA_DOUBLE) (max)))
            buffer_json_member_add_double(wb, "max", (NETDATA_DOUBLE) (max));

        buffer_json_member_add_string_or_omit(wb, "pointer_to", pointer_to);
        buffer_json_member_add_string(wb, "sort", rrdf_field_sort_to_string(sort));
        buffer_json_member_add_boolean(wb, "sortable", !(sort & RRDF_FIELD_SORT_FIXED));
        buffer_json_member_add_boolean(wb, "sticky", options & RRDF_FIELD_OPTS_STICKY);
        buffer_json_member_add_string(wb, "summary", rrdf_field_summary_to_string(summary));
        buffer_json_member_add_string(wb, "filter", rrdf_field_filter_to_string(filter));

        buffer_json_member_add_boolean(wb, "full_width", options & RRDF_FIELD_OPTS_FULL_WIDTH);
        buffer_json_member_add_boolean(wb, "wrap", options & RRDF_FIELD_OPTS_WRAP);
        buffer_json_member_add_boolean(wb, "default_expanded_filter", options & RRDF_FIELD_OPTS_EXPANDED_FILTER);

        if(options & RRDF_FIELD_OPTS_DUMMY)
            buffer_json_member_add_boolean(wb, "dummy", true);
    }
    buffer_json_object_close(wb);
}

static inline void buffer_copy(BUFFER *dst, BUFFER *src) {
    if(!src || !dst)
        return;

    buffer_contents_replace(dst, buffer_tostring(src), buffer_strlen(src));

    dst->content_type = src->content_type;
    dst->options = src->options;
    dst->date = src->date;
    dst->expires = src->expires;
    dst->json = src->json;
}

static inline BUFFER *buffer_dup(BUFFER *src) {
    if(!src)
        return NULL;

    BUFFER *dst = buffer_create(buffer_strlen(src) + 1, src->statistics);
    buffer_copy(dst, src);
    return dst;
}

char *url_encode(const char *str);
static inline void buffer_key_value_urlencode(BUFFER *wb, const char *key, const char *value) {
    char *encoded = NULL;

    if(value && *value)
        encoded = url_encode(value);

    buffer_sprintf(wb, "%s=%s", key, encoded ? encoded : "");

    freez(encoded);
}

#endif /* NETDATA_WEB_BUFFER_H */
