// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_BUFFER_H
#define NETDATA_WEB_BUFFER_H 1

#include "../uuid/uuid.h"
#include "../http/content_type.h"
#include "../clocks/clocks.h"
#include "../string/utf8.h"
#include "../libnetdata.h"

#define DOUBLE_MAX_LENGTH (512) // 318 should be enough, including null
#define UINT64_MAX_LENGTH (24) // 21 should be enough
#define UINT64_HEX_MAX_LENGTH ((sizeof(HEX_PREFIX) - 1) + (sizeof(uint64_t) * 2) + 1)

#define API_RELATIVE_TIME_MAX (time_t)(3 * 365 * 86400)

#define BUFFER_JSON_MAX_DEPTH 32 // max is 255

// gcc with libstdc++ may require this,
// but with libc++ it does not work correctly.
#if defined(__cplusplus) && !defined(_LIBCPP_VERSION)
#include <cmath>
using std::isinf;
using std::isnan;
#endif

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

void buffer_flush(BUFFER *wb);
void buffer_reset(BUFFER *wb);

void buffer_date(BUFFER *wb, int year, int month, int day, int hours, int minutes, int seconds);
void buffer_jsdate(BUFFER *wb, int year, int month, int day, int hours, int minutes, int seconds);

BUFFER *buffer_create(size_t size, size_t *statistics);
void buffer_free(BUFFER *b);
void buffer_increase(BUFFER *b, size_t free_size_required);

void buffer_freep(BUFFER **bp);

void buffer_snprintf(BUFFER *wb, size_t len, const char *fmt, ...) PRINTFLIKE(3, 4);
void buffer_vsprintf(BUFFER *wb, const char *fmt, va_list args);
void buffer_sprintf(BUFFER *wb, const char *fmt, ...) PRINTFLIKE(2,3);
void buffer_json_member_add_sprintf(BUFFER *wb, const char *key, const char *fmt, ...) PRINTFLIKE(3,4);
void buffer_json_add_array_item_sprintf(BUFFER *wb, const char *fmt, ...) PRINTFLIKE(2,3);
void buffer_strcat_htmlescape(BUFFER *wb, const char *txt);

void buffer_char_replace(BUFFER *wb, char from, char to);

void buffer_print_sn_flags(BUFFER *wb, SN_FLAGS flags, bool send_anomaly_bit);

void buffer_need_bytes(BUFFER *buffer, size_t needed_free_size);

void buffer_json_initialize(BUFFER *wb, const char *key_quote, const char *value_quote, int depth,
                            bool add_anonymous_object, BUFFER_JSON_OPTIONS options);

void buffer_json_finalize(BUFFER *wb);

const char *buffer_tostring(BUFFER *wb);

void _buffer_json_depth_push(BUFFER *wb, BUFFER_JSON_NODE_TYPE type);

void _buffer_json_depth_pop(BUFFER *wb);

void buffer_putc(BUFFER *wb, char c);

void buffer_fast_rawcat(BUFFER *wb, const char *txt, size_t len);

void buffer_fast_strcat(BUFFER *wb, const char *txt, size_t len);

void buffer_strcat(BUFFER *wb, const char *txt);

void buffer_contents_replace(BUFFER *wb, const char *txt, size_t len);

void buffer_strncat(BUFFER *wb, const char *txt, size_t len);

void buffer_memcat(BUFFER *wb, const void *mem, size_t bytes);

void buffer_json_strcat(BUFFER *wb, const char *txt);

void buffer_json_quoted_strcat(BUFFER *wb, const char *txt);

char *print_uint32_reversed(char *dst, uint32_t value);

char *print_uint64_reversed(char *dst, uint64_t value);

char *print_uint32_hex_reversed(char *dst, uint32_t value);

char *print_uint64_hex_reversed(char *dst, uint64_t value);

char *print_uint64_hex_reversed_full(char *dst, uint64_t value);

char *print_uint64_base64_reversed(char *dst, uint64_t value);

void char_array_reverse(char *from, char *to);

int print_netdata_double(char *dst, NETDATA_DOUBLE value);

size_t print_uint64(char *dst, uint64_t value);

size_t print_int64(char *dst, int64_t value);

void buffer_print_uint64(BUFFER *wb, uint64_t value);

void buffer_print_int64(BUFFER *wb, int64_t value);

size_t print_uint64_hex(char *dst, uint64_t value);

size_t print_uint64_hex_full(char *dst, uint64_t value);

void buffer_print_uint64_hex(BUFFER *wb, uint64_t value);

void buffer_print_uint64_hex_full(BUFFER *wb, uint64_t value);

void buffer_print_uint64_base64(BUFFER *wb, uint64_t value);

void buffer_print_int64_hex(BUFFER *wb, int64_t value); 

void buffer_print_int64_base64(BUFFER *wb, int64_t value);

void buffer_print_netdata_double(BUFFER *wb, NETDATA_DOUBLE value);

#define DOUBLE_HEX_MAX_LENGTH ((sizeof(IEEE754_DOUBLE_HEX_PREFIX) - 1) + (sizeof(uint64_t) * 2) + 1)
void buffer_print_netdata_double_hex(BUFFER *wb, NETDATA_DOUBLE value);

#define DOUBLE_B64_MAX_LENGTH ((sizeof(IEEE754_DOUBLE_B64_PREFIX) - 1) + (sizeof(uint64_t) * 2) + 1)
void buffer_print_netdata_double_base64(BUFFER *wb, NETDATA_DOUBLE value);

typedef enum {
    NUMBER_ENCODING_DECIMAL,
    NUMBER_ENCODING_HEX,
    NUMBER_ENCODING_BASE64,
} NUMBER_ENCODING;

void buffer_print_int64_encoded(BUFFER *wb, NUMBER_ENCODING encoding, int64_t value);

void buffer_print_uint64_encoded(BUFFER *wb, NUMBER_ENCODING encoding, uint64_t value);

void buffer_print_netdata_double_encoded(BUFFER *wb, NUMBER_ENCODING encoding, NETDATA_DOUBLE value);

void buffer_print_spaces(BUFFER *wb, size_t spaces);

void buffer_print_json_comma(BUFFER *wb);

void buffer_print_json_comma_newline_spacing(BUFFER *wb);

void buffer_print_json_key(BUFFER *wb, const char *key);

void buffer_json_add_string_value(BUFFER *wb, const char *value);

void buffer_json_add_quoted_string_value(BUFFER *wb, const char *value);

void buffer_json_member_add_object(BUFFER *wb, const char *key);

void buffer_json_object_close(BUFFER *wb);

void buffer_json_member_add_string(BUFFER *wb, const char *key, const char *value);

void buffer_json_member_add_string_or_omit(BUFFER *wb, const char *key, const char *value) ;

void buffer_json_member_add_string_or_empty(BUFFER *wb, const char *key, const char *value);

void buffer_json_member_add_datetime_rfc3339(BUFFER *wb, const char *key, uint64_t datetime_ut, bool utc);
void buffer_json_member_add_duration_ut(BUFFER *wb, const char *key, int64_t duration_ut);

void buffer_json_member_add_quoted_string(BUFFER *wb, const char *key, const char *value);

void buffer_json_member_add_uuid_ptr(BUFFER *wb, const char *key, nd_uuid_t *value);

void buffer_json_member_add_uuid(BUFFER *wb, const char *key, nd_uuid_t value);

void buffer_json_member_add_uuid_compact(BUFFER *wb, const char *key, nd_uuid_t value);

void buffer_json_member_add_boolean(BUFFER *wb, const char *key, bool value);

void buffer_json_member_add_array(BUFFER *wb, const char *key);

void buffer_json_add_array_item_array(BUFFER *wb);

void buffer_json_add_array_item_string(BUFFER *wb, const char *value);

void buffer_json_add_array_item_uuid(BUFFER *wb, nd_uuid_t *value);

void buffer_json_add_array_item_uuid_compact(BUFFER *wb, nd_uuid_t *value);

void buffer_json_add_array_item_double(BUFFER *wb, NETDATA_DOUBLE value);

void buffer_json_add_array_item_int64(BUFFER *wb, int64_t value);

void buffer_json_add_array_item_uint64(BUFFER *wb, uint64_t value);

void buffer_json_add_array_item_boolean(BUFFER *wb, bool value);

void buffer_json_add_array_item_time_t(BUFFER *wb, time_t value);

void buffer_json_add_array_item_time_ms(BUFFER *wb, time_t value);

void buffer_json_add_array_item_datetime_rfc3339(BUFFER *wb, uint64_t datetime_ut, bool utc);

void buffer_json_add_array_item_time_t_formatted(BUFFER *wb, time_t value, bool rfc3339);

void buffer_json_add_array_item_object(BUFFER *wb);

void buffer_json_member_add_time_t(BUFFER *wb, const char *key, time_t value);

void buffer_json_member_add_time_t_formatted(BUFFER *wb, const char *key, time_t value, bool rfc3339);

void buffer_json_member_add_uint64(BUFFER *wb, const char *key, uint64_t value);

void buffer_json_member_add_int64(BUFFER *wb, const char *key, int64_t value);

void buffer_json_member_add_double(BUFFER *wb, const char *key, NETDATA_DOUBLE value);

void buffer_json_array_close(BUFFER *wb);

void buffer_copy(BUFFER *dst, BUFFER *src);

BUFFER *buffer_dup(BUFFER *src);

char *url_encode(const char *str);
void buffer_key_value_urlencode(BUFFER *wb, const char *key, const char *value);

// Include functions_fields.h for field-related functionalities
#include "functions_fields.h"

#endif /* NETDATA_WEB_BUFFER_H */
