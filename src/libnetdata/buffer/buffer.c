// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

static ALWAYS_INLINE void buffer_overflow_init(BUFFER *b)
{
    b->buffer[b->size] = '\0';
    strcpy(&b->buffer[b->size + 1], BUFFER_OVERFLOW_EOF);
}

ALWAYS_INLINE void buffer_reset(BUFFER *wb) {
    buffer_flush(wb);

    wb->content_type = CT_TEXT_PLAIN;
    wb->options = 0;
    wb->date = 0;
    wb->expires = 0;
    buffer_no_cacheable(wb);

    buffer_overflow_check(wb);
}

void buffer_char_replace(BUFFER *wb, char from, char to) {
    char *s = wb->buffer, *end = &wb->buffer[wb->len];

    while(s != end) {
        if(*s == from) *s = to;
        s++;
    }

    buffer_overflow_check(wb);
}

ALWAYS_INLINE void buffer_print_sn_flags(BUFFER *wb, SN_FLAGS flags, bool send_anomaly_bit) {
    if(unlikely(flags == SN_EMPTY_SLOT)) {
        buffer_fast_strcat(wb, "E", 1);
        return;
    }

    size_t printed = 0;
    if(likely(send_anomaly_bit && (flags & SN_FLAG_NOT_ANOMALOUS))) {
        buffer_fast_strcat(wb, "A", 1);
        printed++;
    }

    if(unlikely(flags & SN_FLAG_RESET)) {
        buffer_fast_strcat(wb, "R", 1);
        printed++;
    }

    if(!printed)
        buffer_fast_strcat(wb, "''", 2);
}

void buffer_strcat_htmlescape(BUFFER *wb, const char *txt)
{
    while(*txt) {
        switch(*txt) {
            case '&': buffer_strcat(wb, "&amp;"); break;
            case '<': buffer_strcat(wb, "&lt;"); break;
            case '>': buffer_strcat(wb, "&gt;"); break;
            case '"': buffer_strcat(wb, "&quot;"); break;
            case '/': buffer_strcat(wb, "&#x2F;"); break;
            case '\'': buffer_strcat(wb, "&#x27;"); break;
            default: {
                buffer_need_bytes(wb, 1);
                wb->buffer[wb->len++] = *txt;
            }
        }
        txt++;
    }

    buffer_overflow_check(wb);
}

void buffer_snprintf(BUFFER *wb, size_t len, const char *fmt, ...)
{
    if(unlikely(!fmt || !*fmt)) return;

    buffer_need_bytes(wb, len + 1);

    va_list args;
    va_start(args, fmt);
    // vsnprintfz() returns the number of bytes actually written - after possible truncation
    wb->len += vsnprintfz(&wb->buffer[wb->len], len, fmt, args);
    va_end(args);

    buffer_overflow_check(wb);

    // the buffer is \0 terminated by vsnprintfz
}

void buffer_vsprintf(BUFFER *wb, const char *fmt, va_list args) {
    if(unlikely(!fmt || !*fmt)) return;

    size_t full_size_bytes = 0, need = 2, space_remaining = 0;

    do {
        need += full_size_bytes + 2;

        buffer_need_bytes(wb, need);

        space_remaining = wb->size - wb->len - 1;

        // Use the copy of va_list for vsnprintf
        va_list args_copy;
        va_copy(args_copy, args);
        // vsnprintf() returns the number of bytes required, even if bigger than the buffer provided
        full_size_bytes = (size_t) vsnprintf(&wb->buffer[wb->len], space_remaining, fmt, args_copy);
        va_end(args_copy);

    } while(full_size_bytes >= space_remaining);

    wb->len += full_size_bytes;

    wb->buffer[wb->len] = '\0';
    buffer_overflow_check(wb);
}

void buffer_sprintf(BUFFER *wb, const char *fmt, ...)
{
    va_list args;
    va_start(args, fmt);
    buffer_vsprintf(wb, fmt, args);
    va_end(args);
}

// generate a javascript date, the fastest possible way...
void buffer_jsdate(BUFFER *wb, int year, int month, int day, int hours, int minutes, int seconds)
{
  //         10        20        30      = 35
    // 01234567890123456789012345678901234
    // Date(2014,04,01,03,28,20)

    buffer_need_bytes(wb, 30);

    char *b = &wb->buffer[wb->len], *p;
  unsigned int *q = (unsigned int *)b;  

  #if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
    *q++ = 0x65746144;  // "Date" backwards.
  #else
    *q++ = 0x44617465;  // "Date"
  #endif
  p = (char *)q;

  *p++ = '(';
  *p++ = '0' + year / 1000; year %= 1000;
  *p++ = '0' + year / 100;  year %= 100;
  *p++ = '0' + year / 10;
  *p++ = '0' + year % 10;
  *p++ = ',';
  *p   = '0' + month / 10; if (*p != '0') p++;
  *p++ = '0' + month % 10;
  *p++ = ',';
  *p   = '0' + day / 10; if (*p != '0') p++;
  *p++ = '0' + day % 10;
  *p++ = ',';
  *p   = '0' + hours / 10; if (*p != '0') p++;
  *p++ = '0' + hours % 10;
  *p++ = ',';
  *p   = '0' + minutes / 10; if (*p != '0') p++;
  *p++ = '0' + minutes % 10;
  *p++ = ',';
  *p   = '0' + seconds / 10; if (*p != '0') p++;
  *p++ = '0' + seconds % 10;

  unsigned short *r = (unsigned short *)p;

#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
    *r++ = 0x0029;  // ")\0" backwards.  
  #else
    *r++ = 0x2900;  // ")\0"
  #endif

    wb->len += (size_t)((char *)r - b - 1);

    // terminate it
    wb->buffer[wb->len] = '\0';
    buffer_overflow_check(wb);
}

// generate a date, the fastest possible way...
void buffer_date(BUFFER *wb, int year, int month, int day, int hours, int minutes, int seconds)
{
    //         10        20        30      = 35
    // 01234567890123456789012345678901234
    // 2014-04-01 03:28:20

    buffer_need_bytes(wb, 36);

    char *b = &wb->buffer[wb->len];
    char *p = b;

    *p++ = '0' + year / 1000; year %= 1000;
    *p++ = '0' + year / 100;  year %= 100;
    *p++ = '0' + year / 10;
    *p++ = '0' + year % 10;
    *p++ = '-';
    *p++ = '0' + month / 10;
    *p++ = '0' + month % 10;
    *p++ = '-';
    *p++ = '0' + day / 10;
    *p++ = '0' + day % 10;
    *p++ = ' ';
    *p++ = '0' + hours / 10;
    *p++ = '0' + hours % 10;
    *p++ = ':';
    *p++ = '0' + minutes / 10;
    *p++ = '0' + minutes % 10;
    *p++ = ':';
    *p++ = '0' + seconds / 10;
    *p++ = '0' + seconds % 10;
    *p = '\0';

    wb->len += (size_t)(p - b);

    // terminate it
    wb->buffer[wb->len] = '\0';
    buffer_overflow_check(wb);
}

BUFFER *buffer_create(size_t size, size_t *statistics)
{
    BUFFER *b;

    if(!size)
        size = 1024 - sizeof(BUFFER_OVERFLOW_EOF) - 2;
    else
        size++; // make room for the terminator

    netdata_log_debug(D_WEB_BUFFER, "Creating new web buffer of size %zu.", size);

    b = callocz(1, sizeof(BUFFER));
    b->buffer = mallocz(size + sizeof(BUFFER_OVERFLOW_EOF) + 2);
    b->buffer[0] = '\0';
    b->size = size;
    b->content_type = CT_TEXT_PLAIN;
    b->statistics = statistics;
    buffer_no_cacheable(b);
    buffer_overflow_init(b);
    buffer_overflow_check(b);

    if(b->statistics)
        __atomic_add_fetch(b->statistics, b->size + sizeof(BUFFER) + sizeof(BUFFER_OVERFLOW_EOF) + 2, __ATOMIC_RELAXED);

    return(b);
}

void buffer_free(BUFFER *b) {
    if(unlikely(!b)) return;

    buffer_overflow_check(b);

    netdata_log_debug(D_WEB_BUFFER, "Freeing web buffer of size %zu.", (size_t)b->size);

    if(b->statistics)
        __atomic_sub_fetch(b->statistics, b->size + sizeof(BUFFER) + sizeof(BUFFER_OVERFLOW_EOF) + 2, __ATOMIC_RELAXED);

    freez(b->buffer);
    freez(b);
}

void buffer_increase(BUFFER *b, size_t free_size_required) {
    buffer_overflow_check(b);

    size_t remaining = b->size - b->len;
    if(remaining >= free_size_required) return;

    size_t increase = free_size_required - remaining;
    size_t minimum = 1024;
    if(minimum > increase) increase = minimum;

    size_t optimal = (b->size > 5 * 1024 * 1024) ? b->size / 2 : b->size;
    if(optimal > increase) increase = optimal;

    netdata_log_debug(D_WEB_BUFFER, "Increasing data buffer from size %zu to %zu.", (size_t)b->size, (size_t)(b->size + increase));

    b->buffer = reallocz(b->buffer, b->size + increase + sizeof(BUFFER_OVERFLOW_EOF) + 2);
    b->size += increase;

    if(b->statistics)
        __atomic_add_fetch(b->statistics, increase, __ATOMIC_RELAXED);

    buffer_overflow_init(b);
    buffer_overflow_check(b);
}

// ----------------------------------------------------------------------------

void buffer_json_initialize(BUFFER *wb, const char *key_quote, const char *value_quote, int depth,
                       bool add_anonymous_object, BUFFER_JSON_OPTIONS options) {
    strncpyz(wb->json.key_quote, key_quote, BUFFER_QUOTE_MAX_SIZE);
    strncpyz(wb->json.value_quote,  value_quote, BUFFER_QUOTE_MAX_SIZE);

    wb->json.depth = (int8_t)(depth - 1);
    _buffer_json_depth_push(wb, BUFFER_JSON_OBJECT);

    if(add_anonymous_object)
        buffer_fast_strcat(wb, "{", 1);
    else
        options |= BUFFER_JSON_OPTIONS_NON_ANONYMOUS;

    wb->json.options = options;

    wb->content_type = CT_APPLICATION_JSON;
    buffer_no_cacheable(wb);
}

void buffer_json_finalize(BUFFER *wb) {
    while(wb->json.depth >= 0) {
        switch(wb->json.stack[wb->json.depth].type) {
            case BUFFER_JSON_OBJECT:
                if (wb->json.depth == 0)
                    if (!(wb->json.options & BUFFER_JSON_OPTIONS_NON_ANONYMOUS))
                        buffer_json_object_close(wb);
                    else
                        _buffer_json_depth_pop(wb);
                else
                    buffer_json_object_close(wb);
                break;
            case BUFFER_JSON_ARRAY:
                buffer_json_array_close(wb);
                break;

            default:
                internal_fatal(true, "BUFFER: unknown json member type in stack");
                break;
        }
    }

    if(!(wb->json.options & BUFFER_JSON_OPTIONS_MINIFY))
        buffer_fast_strcat(wb, "\n", 1);
}

// ----------------------------------------------------------------------------

const char hex_digits[16] = "0123456789ABCDEF";
const char hex_digits_lower[16] = "0123456789abcdef";
const char base64_digits[64] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
unsigned char hex_value_from_ascii[256];
unsigned char base64_value_from_ascii[256];

__attribute__((constructor)) void initialize_ascii_maps(void) {
    for(size_t i = 0 ; i < 256 ; i++) {
        hex_value_from_ascii[i] = 255;
        base64_value_from_ascii[i] = 255;
    }

    for(size_t i = 0; i < 16 ; i++) {
        hex_value_from_ascii[(int)toupper(hex_digits[i])] = i;
        hex_value_from_ascii[(int)tolower(hex_digits[i])] = i;
    }

    for(size_t i = 0; i < 64 ; i++)
        base64_value_from_ascii[(int)base64_digits[i]] = i;
}

// ----------------------------------------------------------------------------

void buffer_json_member_add_datetime_rfc3339(BUFFER *wb, const char *key, uint64_t datetime_ut, bool utc) {
    char buf[RFC3339_MAX_LENGTH];
    rfc3339_datetime_ut(buf, sizeof(buf), datetime_ut, 2, utc);
    buffer_json_member_add_string(wb, key, buf);
}

void buffer_json_member_add_duration_ut(BUFFER *wb, const char *key, int64_t duration_ut) {
    char buf[64];
    duration_snprintf(buf, sizeof(buf), duration_ut, "us", true);
    buffer_json_member_add_string(wb, key, buf);
}

// ----------------------------------------------------------------------------
// unit test

static int buffer_expect(BUFFER *wb, const char *expected) {
    const char *generated = buffer_tostring(wb);

    if(strcmp(generated, expected) != 0) {
        netdata_log_error("BUFFER: mismatch.\nGenerated:\n%s\nExpected:\n%s\n",
                          generated, expected);
        return 1;
    }

    return 0;
}

static int buffer_uint64_roundtrip(BUFFER *wb, NUMBER_ENCODING encoding, uint64_t value, const char *expected) {
    int errors = 0;
    buffer_flush(wb);
    buffer_print_uint64_encoded(wb, encoding, value);

    if(expected)
        errors += buffer_expect(wb, expected);

    uint64_t v = str2ull_encoded(buffer_tostring(wb));
    if(v != value) {
        netdata_log_error("BUFFER: string '%s' does resolves to %llu, expected %llu",
                          buffer_tostring(wb), (unsigned long long)v, (unsigned long long)value);
        errors++;
    }
    buffer_flush(wb);
    return errors;
}

static int buffer_int64_roundtrip(BUFFER *wb, NUMBER_ENCODING encoding, int64_t value, const char *expected) {
    int errors = 0;
    buffer_flush(wb);
    buffer_print_int64_encoded(wb, encoding, value);

    if(expected)
        errors += buffer_expect(wb, expected);

    int64_t v = str2ll_encoded(buffer_tostring(wb));
    if(v != value) {
        netdata_log_error("BUFFER: string '%s' does resolves to %lld, expected %lld",
                          buffer_tostring(wb), (long long)v, (long long)value);
        errors++;
    }
    buffer_flush(wb);
    return errors;
}

static int buffer_double_roundtrip(BUFFER *wb, NUMBER_ENCODING encoding, NETDATA_DOUBLE value, const char *expected) {
    int errors = 0;
    buffer_flush(wb);
    buffer_print_netdata_double_encoded(wb, encoding, value);

    if(expected)
        errors += buffer_expect(wb, expected);

    NETDATA_DOUBLE v = str2ndd_encoded(buffer_tostring(wb), NULL);
    if(v != value) {
        netdata_log_error("BUFFER: string '%s' does resolves to %.12f, expected %.12f",
                          buffer_tostring(wb), v, value);
        errors++;
    }
    buffer_flush(wb);
    return errors;
}

int buffer_unittest(void) {
    int errors = 0;
    BUFFER *wb = buffer_create(0, NULL);

    buffer_uint64_roundtrip(wb, NUMBER_ENCODING_DECIMAL, 0, "0");
    buffer_uint64_roundtrip(wb, NUMBER_ENCODING_HEX, 0, "0x0");
    buffer_uint64_roundtrip(wb, NUMBER_ENCODING_BASE64, 0, "#A");

    buffer_uint64_roundtrip(wb, NUMBER_ENCODING_DECIMAL, 1676071986, "1676071986");
    buffer_uint64_roundtrip(wb, NUMBER_ENCODING_HEX, 1676071986, "0x63E6D432");
    buffer_uint64_roundtrip(wb, NUMBER_ENCODING_BASE64, 1676071986, "#Bj5tQy");

    buffer_uint64_roundtrip(wb, NUMBER_ENCODING_DECIMAL, 18446744073709551615ULL, "18446744073709551615");
    buffer_uint64_roundtrip(wb, NUMBER_ENCODING_HEX, 18446744073709551615ULL, "0xFFFFFFFFFFFFFFFF");
    buffer_uint64_roundtrip(wb, NUMBER_ENCODING_BASE64, 18446744073709551615ULL, "#P//////////");

    buffer_int64_roundtrip(wb, NUMBER_ENCODING_DECIMAL, 0, "0");
    buffer_int64_roundtrip(wb, NUMBER_ENCODING_HEX, 0, "0x0");
    buffer_int64_roundtrip(wb, NUMBER_ENCODING_BASE64, 0, "#A");

    buffer_int64_roundtrip(wb, NUMBER_ENCODING_DECIMAL, -1676071986, "-1676071986");
    buffer_int64_roundtrip(wb, NUMBER_ENCODING_HEX, -1676071986, "-0x63E6D432");
    buffer_int64_roundtrip(wb, NUMBER_ENCODING_BASE64, -1676071986, "-#Bj5tQy");

    buffer_int64_roundtrip(wb, NUMBER_ENCODING_DECIMAL, (int64_t)-9223372036854775807ULL, "-9223372036854775807");
    buffer_int64_roundtrip(wb, NUMBER_ENCODING_HEX, (int64_t)-9223372036854775807ULL, "-0x7FFFFFFFFFFFFFFF");
    buffer_int64_roundtrip(wb, NUMBER_ENCODING_BASE64, (int64_t)-9223372036854775807ULL, "-#H//////////");

    buffer_double_roundtrip(wb, NUMBER_ENCODING_DECIMAL, 0, "0");
    buffer_double_roundtrip(wb, NUMBER_ENCODING_HEX, 0, "%0");
    buffer_double_roundtrip(wb, NUMBER_ENCODING_BASE64, 0, "@A");

    buffer_double_roundtrip(wb, NUMBER_ENCODING_DECIMAL, 1.5, "1.5");
    buffer_double_roundtrip(wb, NUMBER_ENCODING_HEX, 1.5, "%3FF8000000000000");
    buffer_double_roundtrip(wb, NUMBER_ENCODING_BASE64, 1.5, "@D/4AAAAAAAA");

    buffer_double_roundtrip(wb, NUMBER_ENCODING_DECIMAL, 1.23e+14, "123000000000000");
    buffer_double_roundtrip(wb, NUMBER_ENCODING_HEX, 1.23e+14, "%42DBF78AD3AC0000");
    buffer_double_roundtrip(wb, NUMBER_ENCODING_BASE64, 1.23e+14, "@ELb94rTrAAA");

    buffer_double_roundtrip(wb, NUMBER_ENCODING_DECIMAL, 9.12345678901234567890123456789e+45, "9.123456789012346128e+45");
    buffer_double_roundtrip(wb, NUMBER_ENCODING_HEX, 9.12345678901234567890123456789e+45, "%497991C25C9E4309");
    buffer_double_roundtrip(wb, NUMBER_ENCODING_BASE64, 9.12345678901234567890123456789e+45, "@El5kcJcnkMJ");

    buffer_flush(wb);

    {
        char buf[1024 + 1];
        for(size_t i = 0; i < 1024 ;i++)
            buf[i] = (char)(i % 26) + 'A';
        buf[1024] = '\0';

        buffer_strcat(wb, buf);
        errors += buffer_expect(wb, buf);
    }

    buffer_flush(wb);

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_finalize(wb);
    errors += buffer_expect(wb, "{\n}\n");

    buffer_flush(wb);

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_string(wb, "hello", "world");
    buffer_json_member_add_string(wb, "alpha", "this: \" is a double quote");
    buffer_json_member_add_object(wb, "object1");
    buffer_json_member_add_string(wb, "hello", "world");
    buffer_json_finalize(wb);
    errors += buffer_expect(wb, "{\n    \"hello\":\"world\",\n    \"alpha\":\"this: \\\" is a double quote\",\n    \"object1\":{\n        \"hello\":\"world\"\n    }\n}\n");

    buffer_free(wb);
    return errors;
}
