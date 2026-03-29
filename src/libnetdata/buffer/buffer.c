// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

static void buffer_overflow_init(BUFFER *b)
{
    b->buffer[b->size] = '\0';
    strcpy(&b->buffer[b->size + 1], BUFFER_OVERFLOW_EOF);
}

void buffer_reset(BUFFER *wb) {
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

void buffer_print_sn_flags(BUFFER *wb, SN_FLAGS flags, bool send_anomaly_bit) {
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

void buffer_json_member_add_sprintf(BUFFER *wb, const char *key, const char *fmt, ...)
{
    va_list args;
    
    // Create a temporary buffer for the formatted string
    BUFFER *tmp = buffer_create(0, NULL);
    
    va_start(args, fmt);
    buffer_vsprintf(tmp, fmt, args);
    va_end(args);
    
    // Add as JSON member (which will handle escaping)
    buffer_json_member_add_string(wb, key, buffer_tostring(tmp));
    
    // Free the temporary buffer
    buffer_free(tmp);
}

void buffer_json_add_array_item_sprintf(BUFFER *wb, const char *fmt, ...)
{
    va_list args;
    
    // Create a temporary buffer for the formatted string
    BUFFER *tmp = buffer_create(0, NULL);
    
    va_start(args, fmt);
    buffer_vsprintf(tmp, fmt, args);
    va_end(args);
    
    // Add as array item (which will handle escaping)
    buffer_json_add_array_item_string(wb, buffer_tostring(tmp));
    
    // Free the temporary buffer
    buffer_free(tmp);
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

__attribute__((nonstring))
const char hex_digits[16] = "0123456789ABCDEF";

__attribute__((nonstring))
const char hex_digits_lower[16] = "0123456789abcdef";

__attribute__((nonstring))
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

void buffer_json_add_array_item_datetime_rfc3339(BUFFER *wb, uint64_t datetime_ut, bool utc) {
    char buf[RFC3339_MAX_LENGTH];
    rfc3339_datetime_ut(buf, sizeof(buf), datetime_ut, 2, utc);
    buffer_json_add_array_item_string(wb, buf);
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

void buffer_flush(BUFFER *wb) {
    wb->len = 0;

    wb->json.depth = 0;
    wb->json.stack[0].type = BUFFER_JSON_EMPTY;
    wb->json.stack[0].count = 0;

    if(wb->buffer)
        wb->buffer[0] = '\0';
}

void buffer_freep(BUFFER **bp) {
    if(bp) buffer_free(*bp);
}

void buffer_need_bytes(BUFFER *buffer, size_t needed_free_size) {
    if(unlikely(buffer->len + needed_free_size >= buffer->size))
        buffer_increase(buffer, needed_free_size + 1);
}

const char *buffer_tostring(BUFFER *wb)
{
    if(unlikely(!wb))
        return NULL;

    buffer_need_bytes(wb, 1);
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);

    return(wb->buffer);
}

void _buffer_json_depth_push(BUFFER *wb, BUFFER_JSON_NODE_TYPE type) {
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

void _buffer_json_depth_pop(BUFFER *wb) {
    wb->json.depth--;
}


void buffer_putc(BUFFER *wb, char c) {
    buffer_need_bytes(wb, 2);
    wb->buffer[wb->len++] = c;
    wb->buffer[wb->len] = '\0';
    buffer_overflow_check(wb);
}

void buffer_fast_rawcat(BUFFER *wb, const char *txt, size_t len) {
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

void buffer_fast_strcat(BUFFER *wb, const char *txt, size_t len) {
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

void buffer_strcat(BUFFER *wb, const char *txt) {
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

void buffer_contents_replace(BUFFER *wb, const char *txt, size_t len) {
    wb->len = 0;
    buffer_need_bytes(wb, len + 1);

    memcpy(wb->buffer, txt, len);
    wb->len = len;
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
}

void buffer_strncat(BUFFER *wb, const char *txt, size_t len) {
    if(unlikely(!txt || !*txt)) return;

    buffer_need_bytes(wb, len + 1);

    memcpy(&wb->buffer[wb->len], txt, len);

    wb->len += len;
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
}

void buffer_memcat(BUFFER *wb, const void *mem, size_t bytes) {
    if(unlikely(!mem)) return;

    buffer_need_bytes(wb, bytes + 1);

    memcpy(&wb->buffer[wb->len], mem, bytes);

    wb->len += bytes;
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
}

void buffer_json_strcat(BUFFER *wb, const char *txt)
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
                switch (v) {
                    case '\n': *d++ = 'n'; break;
                    case '\r': *d++ = 'r'; break;
                    case '\t': *d++ = 't'; break;
                    case '\b': *d++ = 'b'; break;
                    case '\f': *d++ = 'f'; break;
                    default:
                        *d++ = 'u';
                        *d++ = hex_digits[(v >> 12) & 0xf];
                        *d++ = hex_digits[(v >> 8) & 0xf];
                        *d++ = hex_digits[(v >> 4) & 0xf];
                        *d++ = hex_digits[v & 0xf];
                        break;
                }
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

void buffer_json_quoted_strcat(BUFFER *wb, const char *txt) {
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

char *print_uint32_reversed(char *dst, uint32_t value) {
    char *d = dst;
    do *d++ = (char)('0' + (value % 10)); while((value /= 10));
    return d;
}

char *print_uint64_reversed(char *dst, uint64_t value) {
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

char *print_uint32_hex_reversed(char *dst, uint32_t value) {
    static const char *digits = "0123456789ABCDEF";
    char *d = dst;
    do *d++ = digits[value & 0xf]; while((value >>= 4));
    return d;
}

char *print_uint64_hex_reversed(char *dst, uint64_t value) {
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

char *print_uint64_hex_reversed_full(char *dst, uint64_t value) {
    char *d = dst;
    for(size_t c = 0; c < sizeof(uint64_t) * 2; c++) {
        *d++ = hex_digits[value & 0xf];
        value >>= 4;
    }

    return d;
}

char *print_uint64_base64_reversed(char *dst, uint64_t value) {
    char *d = dst;
    do *d++ = base64_digits[value & 63]; while ((value >>= 6));
    return d;
}


void char_array_reverse(char *from, char *to) {
    // from and to are inclusive
    char *begin = from, *end = to, aux;
    while (end > begin) aux = *end, *end-- = *begin, *begin++ = aux;
}

int print_netdata_double(char *dst, NETDATA_DOUBLE value) {
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


size_t print_uint64(char *dst, uint64_t value) {
    char *s = dst;
    char *d = print_uint64_reversed(s, value);
    char_array_reverse(s, d - 1);
    *d = '\0';
    return d - s;
}


size_t print_int64(char *dst, int64_t value) {
    size_t len = 0;

    if(value < 0) {
        *dst++ = '-';
        value = -value;
        len++;
    }

    return print_uint64(dst, value) + len;
}

void buffer_print_uint64(BUFFER *wb, uint64_t value) {
    buffer_need_bytes(wb, UINT64_MAX_LENGTH);
    wb->len += print_uint64(&wb->buffer[wb->len], value);
    buffer_overflow_check(wb);
}


void buffer_print_int64(BUFFER *wb, int64_t value) {
    buffer_need_bytes(wb, UINT64_MAX_LENGTH);
    wb->len += print_int64(&wb->buffer[wb->len], value);
    buffer_overflow_check(wb);
}

size_t print_uint64_hex(char *dst, uint64_t value) {
    char *d = dst;

    const char *s = HEX_PREFIX;
    while(*s) *d++ = *s++;

    char *e = print_uint64_hex_reversed(d, value);
    char_array_reverse(d, e - 1);
    *e = '\0';
    return e - dst;
}

size_t print_uint64_hex_full(char *dst, uint64_t value) {
    char *d = dst;

    const char *s = HEX_PREFIX;
    while(*s) *d++ = *s++;

    char *e = print_uint64_hex_reversed_full(d, value);
    char_array_reverse(d, e - 1);
    *e = '\0';
    return e - dst;
}


void buffer_print_uint64_hex(BUFFER *wb, uint64_t value) {
    buffer_need_bytes(wb, UINT64_HEX_MAX_LENGTH);
    wb->len += print_uint64_hex(&wb->buffer[wb->len], value);
    buffer_overflow_check(wb);
}


void buffer_print_uint64_hex_full(BUFFER *wb, uint64_t value) {
    buffer_need_bytes(wb, UINT64_HEX_MAX_LENGTH);
    wb->len += print_uint64_hex_full(&wb->buffer[wb->len], value);
    buffer_overflow_check(wb);
}

#define UINT64_B64_MAX_LENGTH ((sizeof(IEEE754_UINT64_B64_PREFIX) - 1) + (sizeof(uint64_t) * 2) + 1)
void buffer_print_uint64_base64(BUFFER *wb, uint64_t value) {
    buffer_need_bytes(wb, UINT64_B64_MAX_LENGTH);

    buffer_fast_strcat(wb, IEEE754_UINT64_B64_PREFIX, sizeof(IEEE754_UINT64_B64_PREFIX) - 1);

    char *s = &wb->buffer[wb->len];
    char *d = print_uint64_base64_reversed(s, value);
    char_array_reverse(s, d - 1);
    *d = '\0';
    wb->len += d - s;

    buffer_overflow_check(wb);
}

void buffer_print_int64_hex(BUFFER *wb, int64_t value) {
    buffer_need_bytes(wb, 2);

    if(value < 0) {
        buffer_putc(wb, '-');
        value = -value;
    }

    buffer_print_uint64_hex(wb, (uint64_t)value);

    buffer_overflow_check(wb);
}

void buffer_print_int64_base64(BUFFER *wb, int64_t value) {
    buffer_need_bytes(wb, 2);

    if(value < 0) {
        buffer_putc(wb, '-');
        value = -value;
    }

    buffer_print_uint64_base64(wb, (uint64_t)value);

    buffer_overflow_check(wb);
}

void buffer_print_netdata_double(BUFFER *wb, NETDATA_DOUBLE value) {
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
void buffer_print_netdata_double_hex(BUFFER *wb, NETDATA_DOUBLE value) {
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
void buffer_print_netdata_double_base64(BUFFER *wb, NETDATA_DOUBLE value) {
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

void buffer_print_int64_encoded(BUFFER *wb, NUMBER_ENCODING encoding, int64_t value) {
    if(encoding == NUMBER_ENCODING_BASE64)
        return buffer_print_int64_base64(wb, value);

    if(encoding == NUMBER_ENCODING_HEX)
        return buffer_print_int64_hex(wb, value);

    return buffer_print_int64(wb, value);
}

void buffer_print_uint64_encoded(BUFFER *wb, NUMBER_ENCODING encoding, uint64_t value) {
    if(encoding == NUMBER_ENCODING_BASE64)
        return buffer_print_uint64_base64(wb, value);

    if(encoding == NUMBER_ENCODING_HEX)
        return buffer_print_uint64_hex(wb, value);

    return buffer_print_uint64(wb, value);
}

void buffer_print_netdata_double_encoded(BUFFER *wb, NUMBER_ENCODING encoding, NETDATA_DOUBLE value) {
    if(encoding == NUMBER_ENCODING_BASE64)
        return buffer_print_netdata_double_base64(wb, value);

    if(encoding == NUMBER_ENCODING_HEX)
        return buffer_print_netdata_double_hex(wb, value);

    return buffer_print_netdata_double(wb, value);
}

void buffer_print_spaces(BUFFER *wb, size_t spaces) {
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

void buffer_print_json_comma(BUFFER *wb) {
    if(wb->json.stack[wb->json.depth].count)
        buffer_putc(wb, ',');
}

void buffer_print_json_comma_newline_spacing(BUFFER *wb) {
    buffer_print_json_comma(wb);

    if((wb->json.options & BUFFER_JSON_OPTIONS_MINIFY) ||
        (wb->json.stack[wb->json.depth].type == BUFFER_JSON_ARRAY && !(wb->json.options & BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAY_ITEMS)))
        return;

    buffer_putc(wb, '\n');
    buffer_print_spaces(wb, wb->json.depth + 1);
}

void buffer_print_json_key(BUFFER *wb, const char *key) {
    buffer_strcat(wb, wb->json.key_quote);
    buffer_json_strcat(wb, key);
    buffer_strcat(wb, wb->json.key_quote);
}

void buffer_json_add_string_value(BUFFER *wb, const char *value) {
    if(value) {
        buffer_strcat(wb, wb->json.value_quote);
        buffer_json_strcat(wb, value);
        buffer_strcat(wb, wb->json.value_quote);
    }
    else
        buffer_fast_strcat(wb, "null", 4);
}

void buffer_json_add_quoted_string_value(BUFFER *wb, const char *value) {
    if(value) {
        buffer_strcat(wb, wb->json.value_quote);
        buffer_json_quoted_strcat(wb, value);
        buffer_strcat(wb, wb->json.value_quote);
    }
    else
        buffer_fast_strcat(wb, "null", 4);
}

void buffer_json_member_add_object(BUFFER *wb, const char *key) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":{", 2);
    wb->json.stack[wb->json.depth].count++;

    _buffer_json_depth_push(wb, BUFFER_JSON_OBJECT);
}

void buffer_json_object_close(BUFFER *wb) {
#ifdef NETDATA_INTERNAL_CHECKS
    assert(wb->json.depth >= 0 && "BUFFER JSON: nothing is open to close it");
    assert(wb->json.stack[wb->json.depth].type == BUFFER_JSON_OBJECT && "BUFFER JSON: an object is not open to close it");
#endif
    if(!(wb->json.options & BUFFER_JSON_OPTIONS_MINIFY)) {
        buffer_putc(wb, '\n');
        buffer_print_spaces(wb, wb->json.depth);
    }
    buffer_putc(wb, '}');
    _buffer_json_depth_pop(wb);
}

void buffer_json_member_add_string(BUFFER *wb, const char *key, const char *value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_putc(wb, ':');
    buffer_json_add_string_value(wb, value);

    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_member_add_string_or_omit(BUFFER *wb, const char *key, const char *value) {
    if(value && *value)
        buffer_json_member_add_string(wb, key, value);
}

void buffer_json_member_add_string_or_empty(BUFFER *wb, const char *key, const char *value) {
    if(!value)
        value = "";

    buffer_json_member_add_string(wb, key, value);
}

void buffer_json_member_add_datetime_rfc3339(BUFFER *wb, const char *key, uint64_t datetime_ut, bool utc);
void buffer_json_member_add_duration_ut(BUFFER *wb, const char *key, int64_t duration_ut);

void buffer_json_member_add_quoted_string(BUFFER *wb, const char *key, const char *value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_putc(wb, ':');

    if(!value || strcmp(value, "null") == 0)
        buffer_fast_strcat(wb, "null", 4);
    else
        buffer_json_add_quoted_string_value(wb, value);

    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_member_add_uuid_ptr(BUFFER *wb, const char *key, nd_uuid_t *value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_putc(wb, ':');

    if(value && !uuid_is_null(*value)) {
        char uuid[GUID_LEN + 1];
        uuid_unparse_lower(*value, uuid);
        buffer_json_add_string_value(wb, uuid);
    }
    else
        buffer_json_add_string_value(wb, NULL);

    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_member_add_uuid(BUFFER *wb, const char *key, nd_uuid_t value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_putc(wb, ':');

    if(!uuid_is_null(value)) {
        char uuid[UUID_STR_LEN];
        uuid_unparse_lower(value, uuid);
        buffer_json_add_string_value(wb, uuid);
    }
    else
        buffer_json_add_string_value(wb, NULL);

    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_member_add_uuid_compact(BUFFER *wb, const char *key, nd_uuid_t value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_putc(wb, ':');

    if(!uuid_is_null(value)) {
        char uuid[UUID_COMPACT_STR_LEN];
        uuid_unparse_lower_compact(value, uuid);
        buffer_json_add_string_value(wb, uuid);
    }
    else
        buffer_json_add_string_value(wb, NULL);

    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_member_add_boolean(BUFFER *wb, const char *key, bool value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_putc(wb, ':');
    buffer_strcat(wb, value?"true":"false");

    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_member_add_array(BUFFER *wb, const char *key) {
    buffer_print_json_comma_newline_spacing(wb);
    if (key) {
        buffer_print_json_key(wb, key);
        buffer_fast_strcat(wb, ":[", 2);
    }
    else
        buffer_putc(wb, '[');

    wb->json.stack[wb->json.depth].count++;

    _buffer_json_depth_push(wb, BUFFER_JSON_ARRAY);
}

void buffer_json_add_array_item_array(BUFFER *wb) {
    if(!(wb->json.options & BUFFER_JSON_OPTIONS_MINIFY) && wb->json.stack[wb->json.depth].type == BUFFER_JSON_ARRAY) {
        // an array inside another array
        buffer_print_json_comma(wb);
        buffer_putc(wb, '\n');
        buffer_print_spaces(wb, wb->json.depth + 1);
    }
    else
        buffer_print_json_comma_newline_spacing(wb);

    buffer_putc(wb, '[');
    wb->json.stack[wb->json.depth].count++;

    _buffer_json_depth_push(wb, BUFFER_JSON_ARRAY);
}

void buffer_json_add_array_item_string(BUFFER *wb, const char *value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_json_add_string_value(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_add_array_item_uuid(BUFFER *wb, nd_uuid_t *value) {
    if(value && !uuid_is_null(*value)) {
        char uuid[GUID_LEN + 1];
        uuid_unparse_lower(*value, uuid);
        buffer_json_add_array_item_string(wb, uuid);
    }
    else
        buffer_json_add_array_item_string(wb, NULL);
}

void buffer_json_add_array_item_uuid_compact(BUFFER *wb, nd_uuid_t *value) {
    if(value && !uuid_is_null(*value)) {
        char uuid[GUID_LEN + 1];
        uuid_unparse_lower_compact(*value, uuid);
        buffer_json_add_array_item_string(wb, uuid);
    }
    else
        buffer_json_add_array_item_string(wb, NULL);
}

void buffer_json_add_array_item_double(BUFFER *wb, NETDATA_DOUBLE value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_print_netdata_double(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_add_array_item_int64(BUFFER *wb, int64_t value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_print_int64(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_add_array_item_uint64(BUFFER *wb, uint64_t value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_print_uint64(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_add_array_item_boolean(BUFFER *wb, bool value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_strcat(wb, value ? "true" : "false");
    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_add_array_item_time_t(BUFFER *wb, time_t value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_print_int64(wb, value);
    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_add_array_item_time_ms(BUFFER *wb, time_t value) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_print_int64(wb, value);
    buffer_fast_strcat(wb, "000", 3);
    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_add_array_item_datetime_rfc3339(BUFFER *wb, uint64_t datetime_ut, bool utc);

void buffer_json_add_array_item_time_t_formatted(BUFFER *wb, time_t value, bool rfc3339) {
    if(unlikely(rfc3339 && (!value || value > API_RELATIVE_TIME_MAX))) {
        if (!value)
            buffer_json_add_array_item_string(wb, NULL);
        else
            buffer_json_add_array_item_datetime_rfc3339(wb, (uint64_t)value * USEC_PER_SEC, true);
    }
    else
        buffer_json_add_array_item_time_t(wb, value);
}

void buffer_json_add_array_item_object(BUFFER *wb) {
    buffer_print_json_comma_newline_spacing(wb);

    buffer_putc(wb, '{');
    wb->json.stack[wb->json.depth].count++;

    _buffer_json_depth_push(wb, BUFFER_JSON_OBJECT);
}

void buffer_json_member_add_time_t(BUFFER *wb, const char *key, time_t value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_putc(wb, ':');
    buffer_print_int64(wb, value);

    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_member_add_time_t_formatted(BUFFER *wb, const char *key, time_t value, bool rfc3339) {
    if(unlikely(rfc3339 && (!value || value > API_RELATIVE_TIME_MAX))) {
        if(!value)
            buffer_json_member_add_string(wb, key, NULL);
        else
            buffer_json_member_add_datetime_rfc3339(wb, key, (uint64_t)value * USEC_PER_SEC, true);
    }
    else
        buffer_json_member_add_time_t(wb, key, value);
}

void buffer_json_member_add_uint64(BUFFER *wb, const char *key, uint64_t value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_putc(wb, ':');
    buffer_print_uint64(wb, value);

    wb->json.stack[wb->json.depth].count++;
}


void buffer_json_member_add_int64(BUFFER *wb, const char *key, int64_t value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_putc(wb, ':');
    buffer_print_int64(wb, value);

    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_member_add_double(BUFFER *wb, const char *key, NETDATA_DOUBLE value) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_putc(wb, ':');
    buffer_print_netdata_double(wb, value);

    wb->json.stack[wb->json.depth].count++;
}

void buffer_json_array_close(BUFFER *wb) {
#ifdef NETDATA_INTERNAL_CHECKS
    assert(wb->json.depth >= 0 && "BUFFER JSON: nothing is open to close it");
    assert(wb->json.stack[wb->json.depth].type == BUFFER_JSON_ARRAY && "BUFFER JSON: an array is not open to close it");
#endif
    if(wb->json.options & BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAY_ITEMS) {
        buffer_putc(wb, '\n');
        buffer_print_spaces(wb, wb->json.depth);
    }

    buffer_putc(wb, ']');
    _buffer_json_depth_pop(wb);
}

void buffer_copy(BUFFER *dst, BUFFER *src) {
    if(!src || !dst)
        return;

    buffer_contents_replace(dst, buffer_tostring(src), buffer_strlen(src));

    dst->content_type = src->content_type;
    dst->options = src->options;
    dst->date = src->date;
    dst->expires = src->expires;
    dst->json = src->json;
}

BUFFER *buffer_dup(BUFFER *src) {
    if(!src)
        return NULL;

    BUFFER *dst = buffer_create(buffer_strlen(src) + 1, src->statistics);
    buffer_copy(dst, src);
    return dst;
}

char *url_encode(const char *str);

void buffer_key_value_urlencode(BUFFER *wb, const char *key, const char *value) {
    char *encoded = NULL;

    if(value && *value)
        encoded = url_encode(value);

    buffer_sprintf(wb, "%s=%s", key, encoded ? encoded : "");

    freez(encoded);
}
