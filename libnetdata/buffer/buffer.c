// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

static inline void buffer_overflow_init(BUFFER *b)
{
    b->buffer[b->size] = '\0';
    strcpy(&b->buffer[b->size + 1], BUFFER_OVERFLOW_EOF);
}

void buffer_reset(BUFFER *wb) {
    buffer_flush(wb);

    wb->contenttype = CT_TEXT_PLAIN;
    wb->options = 0;
    wb->date = 0;
    wb->expires = 0;

    buffer_overflow_check(wb);
}

const char *buffer_tostring(BUFFER *wb)
{
    buffer_need_bytes(wb, 1);
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);

    return(wb->buffer);
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
    wb->len += vsnprintfz(&wb->buffer[wb->len], len, fmt, args);
    va_end(args);

    buffer_overflow_check(wb);

    // the buffer is \0 terminated by vsnprintfz
}

void buffer_vsprintf(BUFFER *wb, const char *fmt, va_list args)
{
    if(unlikely(!fmt || !*fmt)) return;

    size_t wrote = 0, need = 2, space_remaining = 0;

    do {
        need += space_remaining * 2;

        debug(D_WEB_BUFFER, "web_buffer_sprintf(): increasing web_buffer at position %zu, size = %zu, by %zu bytes (wrote = %zu)\n", wb->len, wb->size, need, wrote);
        buffer_need_bytes(wb, need);

        space_remaining = wb->size - wb->len - 1;

        wrote = (size_t) vsnprintfz(&wb->buffer[wb->len], space_remaining, fmt, args);

    } while(wrote >= space_remaining);

    wb->len += wrote;

    // the buffer is \0 terminated by vsnprintf
}

void buffer_sprintf(BUFFER *wb, const char *fmt, ...)
{
    if(unlikely(!fmt || !*fmt)) return;

    va_list args;
    size_t wrote = 0, need = 2, space_remaining = 0;

    do {
        need += space_remaining * 2;

        debug(D_WEB_BUFFER, "web_buffer_sprintf(): increasing web_buffer at position %zu, size = %zu, by %zu bytes (wrote = %zu)\n", wb->len, wb->size, need, wrote);
        buffer_need_bytes(wb, need);

        space_remaining = wb->size - wb->len - 1;

        va_start(args, fmt);
        wrote = (size_t) vsnprintfz(&wb->buffer[wb->len], space_remaining, fmt, args);
        va_end(args);

    } while(wrote >= space_remaining);

    wb->len += wrote;

    // the buffer is \0 terminated by vsnprintf
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

    debug(D_WEB_BUFFER, "Creating new web buffer of size %zu.", size);

    b = callocz(1, sizeof(BUFFER));
    b->buffer = mallocz(size + sizeof(BUFFER_OVERFLOW_EOF) + 2);
    b->buffer[0] = '\0';
    b->size = size;
    b->contenttype = CT_TEXT_PLAIN;
    b->statistics = statistics;
    buffer_overflow_init(b);
    buffer_overflow_check(b);

    if(b->statistics)
        __atomic_add_fetch(b->statistics, b->size + sizeof(BUFFER) + sizeof(BUFFER_OVERFLOW_EOF) + 2, __ATOMIC_RELAXED);

    return(b);
}

void buffer_free(BUFFER *b) {
    if(unlikely(!b)) return;

    buffer_overflow_check(b);

    debug(D_WEB_BUFFER, "Freeing web buffer of size %zu.", b->size);

    if(b->statistics)
        __atomic_sub_fetch(b->statistics, b->size + sizeof(BUFFER) + sizeof(BUFFER_OVERFLOW_EOF) + 2, __ATOMIC_RELAXED);

    freez(b->buffer);
    freez(b);
}

void buffer_increase(BUFFER *b, size_t free_size_required) {
    buffer_overflow_check(b);

    size_t left = b->size - b->len;
    if(left >= free_size_required) return;

    size_t wanted = free_size_required - left;
    size_t minimum = WEB_DATA_LENGTH_INCREASE_STEP;
    if(minimum > wanted) wanted = minimum;

    size_t optimal = (b->size > 5*1024*1024) ? b->size / 2 : b->size;
    if(optimal > wanted) wanted = optimal;

    debug(D_WEB_BUFFER, "Increasing data buffer from size %zu to %zu.", b->size, b->size + wanted);

    b->buffer = reallocz(b->buffer, b->size + wanted + sizeof(BUFFER_OVERFLOW_EOF) + 2);
    b->size += wanted;

    if(b->statistics)
        __atomic_add_fetch(b->statistics, wanted, __ATOMIC_RELAXED);

    buffer_overflow_init(b);
    buffer_overflow_check(b);
}

// ----------------------------------------------------------------------------

void buffer_json_initialize(BUFFER *wb, const char *key_quote, const char *value_quote, int depth, bool add_anonymous_object) {
    strncpyz(wb->json.key_quote, key_quote, BUFFER_QUOTE_MAX_SIZE);
    strncpyz(wb->json.value_quote,  value_quote, BUFFER_QUOTE_MAX_SIZE);

    wb->json.depth = depth - 1;
    _buffer_json_depth_push(wb, BUFFER_JSON_OBJECT);

    if(add_anonymous_object)
        buffer_fast_strcat(wb, "{", 1);
}

void buffer_json_finalize(BUFFER *wb) {
    while(wb->json.depth >= 0) {
        switch(wb->json.stack[wb->json.depth].type) {
            case BUFFER_JSON_OBJECT:
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
}

// ----------------------------------------------------------------------------
// unit test

static int buffer_expect(BUFFER *wb, const char *expected) {
    const char *generated = buffer_tostring(wb);

    if(strcmp(generated, expected) != 0) {
        error("BUFFER: json mismatch.\nGenerated:\n%s\nExpected:\n%s\n",
              generated, expected);
        return 1;
    }

    return 0;
}

int buffer_unittest(void) {
    int errors = 0;
    BUFFER *wb = buffer_create(0, NULL);

    buffer_print_uint64_hex(wb, 1676071986);
    errors += buffer_expect(wb, "0x63E6D432");

    buffer_flush(wb);

    buffer_print_int64_hex(wb, -1676071986);
    errors += buffer_expect(wb, "-0x63E6D432");

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

    buffer_json_initialize(wb, "\"", "\"", 0, true);
    buffer_json_finalize(wb);
    errors += buffer_expect(wb, "{\n}");

    buffer_flush(wb);

    buffer_json_initialize(wb, "\"", "\"", 0, true);
    buffer_json_member_add_string(wb, "hello", "world");
    buffer_json_member_add_string(wb, "alpha", "this: \" is a double quote");
    buffer_json_member_add_object(wb, "object1");
    buffer_json_member_add_string(wb, "hello", "world");
    buffer_json_finalize(wb);
    errors += buffer_expect(wb, "{\n \"hello\":\"world\",\n \"alpha\":\"this: \\\" is a double quote\",\n \"object1\":{\n  \"hello\":\"world\"\n }\n}");

    return errors;
}

