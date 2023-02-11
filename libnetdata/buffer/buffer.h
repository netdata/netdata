// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_BUFFER_H
#define NETDATA_WEB_BUFFER_H 1

#include "../libnetdata.h"

#define WEB_DATA_LENGTH_INCREASE_STEP 1024

#define BUFFER_JSON_MAX_DEPTH 32

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

typedef struct web_buffer {
    size_t size;            // allocation size of buffer, in bytes
    size_t len;             // current data length in buffer, in bytes
    char *buffer;           // the buffer itself
    uint8_t contenttype;    // the content type of the data in the buffer
    BUFFER_OPTIONS options; // options related to the content
    time_t date;            // the timestamp this content has been generated
    time_t expires;         // the timestamp this content expires
    size_t *statistics;

    struct {
        char key_quote[BUFFER_QUOTE_MAX_SIZE + 1];
        char value_quote[BUFFER_QUOTE_MAX_SIZE + 1];
        int depth;
        BUFFER_JSON_NODE stack[BUFFER_JSON_MAX_DEPTH];
    } json;
} BUFFER;

// content-types
#define CT_APPLICATION_JSON             1
#define CT_TEXT_PLAIN                   2
#define CT_TEXT_HTML                    3
#define CT_APPLICATION_X_JAVASCRIPT     4
#define CT_TEXT_CSS                     5
#define CT_TEXT_XML                     6
#define CT_APPLICATION_XML              7
#define CT_TEXT_XSL                     8
#define CT_APPLICATION_OCTET_STREAM     9
#define CT_APPLICATION_X_FONT_TRUETYPE  10
#define CT_APPLICATION_X_FONT_OPENTYPE  11
#define CT_APPLICATION_FONT_WOFF        12
#define CT_APPLICATION_FONT_WOFF2       13
#define CT_APPLICATION_VND_MS_FONTOBJ   14
#define CT_IMAGE_SVG_XML                15
#define CT_IMAGE_PNG                    16
#define CT_IMAGE_JPG                    17
#define CT_IMAGE_GIF                    18
#define CT_IMAGE_XICON                  19
#define CT_IMAGE_ICNS                   20
#define CT_IMAGE_BMP                    21
#define CT_PROMETHEUS                   22

#define buffer_cacheable(wb)    do { (wb)->options |= WB_CONTENT_CACHEABLE;    if((wb)->options & WB_CONTENT_NO_CACHEABLE) (wb)->options &= ~WB_CONTENT_NO_CACHEABLE; } while(0)
#define buffer_no_cacheable(wb) do { (wb)->options |= WB_CONTENT_NO_CACHEABLE; if((wb)->options & WB_CONTENT_CACHEABLE)    (wb)->options &= ~WB_CONTENT_CACHEABLE;  (wb)->expires = 0; } while(0)

#define buffer_strlen(wb) ((wb)->len)
const char *buffer_tostring(BUFFER *wb);

#define buffer_flush(wb) wb->buffer[(wb)->len = 0] = '\0'
void buffer_reset(BUFFER *wb);

void buffer_date(BUFFER *wb, int year, int month, int day, int hours, int minutes, int seconds);
void buffer_jsdate(BUFFER *wb, int year, int month, int day, int hours, int minutes, int seconds);

BUFFER *buffer_create(size_t size, size_t *statistics);
void buffer_free(BUFFER *b);
void buffer_increase(BUFFER *b, size_t free_size_required);

void buffer_snprintf(BUFFER *wb, size_t len, const char *fmt, ...) PRINTFLIKE(3, 4);
void buffer_vsprintf(BUFFER *wb, const char *fmt, va_list args);
void buffer_sprintf(BUFFER *wb, const char *fmt, ...) PRINTFLIKE(2,3);
void buffer_strcat_htmlescape(BUFFER *wb, const char *txt);

void buffer_char_replace(BUFFER *wb, char from, char to);

void buffer_print_sn_flags(BUFFER *wb, SN_FLAGS flags, bool send_anomaly_bit);

static inline void buffer_need_bytes(BUFFER *buffer, size_t needed_free_size) {
    if(unlikely(buffer->len + needed_free_size >= buffer->size))
        buffer_increase(buffer, needed_free_size + 1);
}

void buffer_json_initialize(BUFFER *wb, const char *key_quote, const char *value_quote, int depth, bool add_anonymous_object);
void buffer_json_finalize(BUFFER *wb);

static inline void _buffer_json_depth_push(BUFFER *wb, BUFFER_JSON_NODE_TYPE type) {
#ifdef NETDATA_INTERNAL_CHECKS
    assert(wb->json.depth <= BUFFER_JSON_MAX_DEPTH && "BUFFER JSON: max nesting reached");
#endif
    wb->json.depth++;
    wb->json.stack[wb->json.depth].count = 0;
    wb->json.stack[wb->json.depth].type = type;
}

static inline void _buffer_json_depth_pop(BUFFER *wb) {
    wb->json.depth--;
}

static inline void buffer_fast_strcat(BUFFER *wb, const char *txt, size_t len) {
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
}

static inline void buffer_strcat(BUFFER *wb, const char *txt) {
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
}

static inline void buffer_json_strcat(BUFFER *wb, const char *txt) {
    if(unlikely(!txt || !*txt)) return;

    const char *t = txt;
    while(*t) {
        buffer_need_bytes(wb, 100);
        char *s = &wb->buffer[wb->len];
        char *d = s;
        const char *e = &wb->buffer[wb->size - 1]; // remove 1 to make room for the escape character

        while(*t && d < e) {
            if(unlikely(*t == '\\' || *t == '\"'))
                *d++ = '\\';

            *d++ = *t++;
        }

        wb->len += d - s;
    }

    buffer_need_bytes(wb, 1);
    wb->buffer[wb->len] = '\0';
}

// This trick seems to give an 80% speed increase in 32bit systems
// print_number_llu_r() will just print the digits up to the
// point the remaining value fits in 32 bits, and then calls
// print_number_lu_r() to print the rest with 32 bit arithmetic.

static inline char *print_uint32_reversed(char *dst, uint32_t value) {
    char *d = dst;
    do *d++ = (char)('0' + (value % 10)); while((value /= 10));
    return d;
}

static inline char *print_uint64_reversed(char *dst, uint64_t value) {
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

static inline char *print_uint32_hex_reversed(char *dst, uint32_t value) {
    static const char *digits = "0123456789ABCDEF";
    char *d = dst;
    do *d++ = digits[value & 0xf]; while((value >>= 4));
    return d;
}

static inline char *print_uint64_hex_reversed(char *dst, uint64_t value) {
    static const char *digits = "0123456789ABCDEF";
#ifdef ENV32BIT
    if(value <= (uint64_t)0xffffffff)
        return print_uint32_hex_reversed(dst, value);

    char *d = dst;
    do *d++ = digits[value & 0xf]; while((value >>= 4) && value > (uint64_t)0xffffffff);
    if(value) return print_uint32_hex_reversed(d, value);
    return d;
#else
    char *d = dst;
    do *d++ = digits[value & 0xf]; while((value >>= 4));
    return d;
#endif
}

static inline void char_array_reverse(char *from, char *to) {
    // from and to are inclusive
    char *begin = from, *end = to, aux;
    while (end > begin) aux = *end, *end-- = *begin, *begin++ = aux;
}

static inline void buffer_print_uint64(BUFFER *wb, uint64_t value) {
    buffer_need_bytes(wb, 50);

    char *s = &wb->buffer[wb->len];
    char *d = print_uint64_reversed(s, value);
    char_array_reverse(s, d - 1);
    *d = '\0';
    wb->len += d - s;
}

static inline void buffer_print_int64(BUFFER *wb, int64_t value) {
    buffer_need_bytes(wb, 50);

    if(value < 0) {
        buffer_fast_strcat(wb, "-", 1);
        value = -value;
    }

    buffer_print_uint64(wb, (uint64_t)value);
}

static inline void buffer_print_uint64_hex(BUFFER *wb, uint64_t value) {
    buffer_need_bytes(wb, sizeof(uint64_t) * 2 + 2 + 1);

    buffer_fast_strcat(wb, "0x", 2);

    char *s = &wb->buffer[wb->len];
    char *d = print_uint64_hex_reversed(s, value);
    char_array_reverse(s, d - 1);
    *d = '\0';
    wb->len += d - s;
}

static inline void buffer_print_int64_hex(BUFFER *wb, int64_t value) {
    buffer_need_bytes(wb, sizeof(uint64_t) * 2 + 2 + 1 + 1);

    if(value < 0) {
        buffer_fast_strcat(wb, "-", 1);
        value = -value;
    }

    buffer_print_uint64_hex(wb, (uint64_t)value);
}

static inline void buffer_print_netdata_double(BUFFER *wb, NETDATA_DOUBLE value) {
    buffer_need_bytes(wb, 512);

    if(isnan(value) || isinf(value)) {
        buffer_strcat(wb, "null");
        return;
    }
    else
        wb->len += print_netdata_double(&wb->buffer[wb->len], value);

    // terminate it
    buffer_need_bytes(wb, 1);
    wb->buffer[wb->len] = '\0';
}

static inline void buffer_print_spaces(BUFFER *wb, size_t spaces) {
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
}

static inline void buffer_print_json_comma_newline_spacing(BUFFER *wb) {
    if(wb->json.stack[wb->json.depth].count)
        buffer_fast_strcat(wb, ",\n", 2);
    else
        buffer_fast_strcat(wb, "\n", 1);

    buffer_print_spaces(wb, wb->json.depth + 1);
}

static inline void buffer_print_json_key(BUFFER *wb, const char *key) {
    buffer_strcat(wb, wb->json.key_quote);
    buffer_json_strcat(wb, key);
    buffer_strcat(wb, wb->json.key_quote);
}

static inline void buffer_json_add_string_value(BUFFER *wb, const char *value) {
    if(value) {
        buffer_strcat(wb, wb->json.value_quote);
        buffer_json_strcat(wb, value);
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
    buffer_fast_strcat(wb, "\n", 1);
    buffer_print_spaces(wb, wb->json.depth);
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

static inline void buffer_json_member_add_boolean(BUFFER *wb, const char *key, bool value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);
    buffer_strcat(wb, value?"true":"false");

    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_member_add_array(BUFFER *wb, const char *key) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":[", 2);
    wb->json.stack[wb->json.depth].count++;

    _buffer_json_depth_push(wb, BUFFER_JSON_ARRAY);
}

static inline void buffer_json_add_array_item_array(BUFFER *wb) {
    if(wb->json.stack[wb->json.depth].count)
        buffer_fast_strcat(wb, ",", 1);

    buffer_fast_strcat(wb, "[", 1);
    wb->json.stack[wb->json.depth].count++;

    _buffer_json_depth_push(wb, BUFFER_JSON_ARRAY);
}

static inline void buffer_json_add_array_item_string(BUFFER *wb, const char *value) {
    if(wb->json.stack[wb->json.depth].count)
        buffer_fast_strcat(wb, ",", 1);

    buffer_json_add_string_value(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_add_array_item_double(BUFFER *wb, NETDATA_DOUBLE value) {
    if(wb->json.stack[wb->json.depth].count)
        buffer_fast_strcat(wb, ",", 1);

    buffer_print_netdata_double(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_add_array_item_uint64(BUFFER *wb, uint64_t value) {
    if(wb->json.stack[wb->json.depth].count)
        buffer_fast_strcat(wb, ",", 1);

    buffer_print_uint64(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_add_array_item_object(BUFFER *wb) {
    if(wb->json.stack[wb->json.depth].count)
        buffer_fast_strcat(wb, ",", 1);

    buffer_fast_strcat(wb, "{", 1);
    wb->json.stack[wb->json.depth].count++;

    _buffer_json_depth_push(wb, BUFFER_JSON_OBJECT);
}

static inline void buffer_json_member_add_time_t(BUFFER *wb, const char *key, time_t value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);
    buffer_print_uint64(wb, value);

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
    buffer_fast_strcat(wb, "]", 1);
    _buffer_json_depth_pop(wb);
}

#endif /* NETDATA_WEB_BUFFER_H */
