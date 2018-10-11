// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata.h"

#define BUFFER_OVERFLOW_EOF "EOF"

static inline void buffer_overflow_init(BUFFER *b)
{
    b->buffer[b->size] = '\0';
    strcpy(&b->buffer[b->size + 1], BUFFER_OVERFLOW_EOF);
}

#ifdef NETDATA_INTERNAL_CHECKS
#define buffer_overflow_check(b) _buffer_overflow_check(b, __FILE__, __FUNCTION__, __LINE__)
#else
#define buffer_overflow_check(b)
#endif

static inline void _buffer_overflow_check(BUFFER *b, const char *file, const char *function, const unsigned long line)
{
    if(b->len > b->size) {
        error("BUFFER: length %zu is above size %zu, at line %lu, at function %s() of file '%s'.", b->len, b->size, line, function, file);
        b->len = b->size;
    }

    if(b->buffer[b->size] != '\0' || strcmp(&b->buffer[b->size + 1], BUFFER_OVERFLOW_EOF) != 0) {
        error("BUFFER: detected overflow at line %lu, at function %s() of file '%s'.", line, function, file);
        buffer_overflow_init(b);
    }
}


void buffer_reset(BUFFER *wb)
{
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

void buffer_char_replace(BUFFER *wb, char from, char to)
{
    char *s = wb->buffer, *end = &wb->buffer[wb->len];

    while(s != end) {
        if(*s == from) *s = to;
        s++;
    }

    buffer_overflow_check(wb);
}

// This trick seems to give an 80% speed increase in 32bit systems
// print_calculated_number_llu_r() will just print the digits up to the
// point the remaining value fits in 32 bits, and then calls
// print_calculated_number_lu_r() to print the rest with 32 bit arithmetic.

inline char *print_number_lu_r(char *str, unsigned long uvalue) {
    char *wstr = str;

    // print each digit
    do *wstr++ = (char)('0' + (uvalue % 10)); while(uvalue /= 10);
    return wstr;
}

inline char *print_number_llu_r(char *str, unsigned long long uvalue) {
    char *wstr = str;

    // print each digit
    do *wstr++ = (char)('0' + (uvalue % 10)); while((uvalue /= 10) && uvalue > (unsigned long long)0xffffffff);
    if(uvalue) return print_number_lu_r(wstr, uvalue);
    return wstr;
}

inline char *print_number_llu_r_smart(char *str, unsigned long long uvalue) {
#ifdef ENVIRONMENT32
    if(uvalue > (unsigned long long)0xffffffff)
        str = print_number_llu_r(str, uvalue);
    else
        str = print_number_lu_r(str, uvalue);
#else
    do *str++ = (char)('0' + (uvalue % 10)); while(uvalue /= 10);
#endif

    return str;
}

void buffer_print_llu(BUFFER *wb, unsigned long long uvalue)
{
    buffer_need_bytes(wb, 50);

    char *str = &wb->buffer[wb->len];
    char *wstr = str;

#ifdef ENVIRONMENT32
    if(uvalue > (unsigned long long)0xffffffff)
        wstr = print_number_llu_r(wstr, uvalue);
    else
        wstr = print_number_lu_r(wstr, uvalue);
#else
    do *wstr++ = (char)('0' + (uvalue % 10)); while(uvalue /= 10);
#endif

    // terminate it
    *wstr = '\0';

    // reverse it
    char *begin = str, *end = wstr - 1, aux;
    while (end > begin) aux = *end, *end-- = *begin, *begin++ = aux;

    // return the buffer length
    wb->len += wstr - str;
}

void buffer_strcat(BUFFER *wb, const char *txt)
{
    // buffer_sprintf(wb, "%s", txt);

    if(unlikely(!txt || !*txt)) return;

    buffer_need_bytes(wb, 1);

    char *s = &wb->buffer[wb->len], *start, *end = &wb->buffer[wb->size];
    size_t len = wb->len;

    start = s;
    while(*txt && s != end)
        *s++ = *txt++;

    len += s - start;

    wb->len = len;
    buffer_overflow_check(wb);

    if(*txt) {
        debug(D_WEB_BUFFER, "strcat(): increasing web_buffer at position %zu, size = %zu\n", wb->len, wb->size);
        len = strlen(txt);
        buffer_increase(wb, len);
        buffer_strcat(wb, txt);
    }
    else {
        // terminate the string
        // without increasing the length
        buffer_need_bytes(wb, (size_t)1);
        wb->buffer[wb->len] = '\0';
    }
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

    buffer_need_bytes(wb, 2);

    size_t len = wb->size - wb->len - 1;

    wb->len += vsnprintfz(&wb->buffer[wb->len], len, fmt, args);

    buffer_overflow_check(wb);

    // the buffer is \0 terminated by vsnprintfz
}

void buffer_sprintf(BUFFER *wb, const char *fmt, ...)
{
    if(unlikely(!fmt || !*fmt)) return;

    va_list args;
    size_t wrote = 0, need = 2, multiplier = 0, len;

    do {
        need += wrote + multiplier * WEB_DATA_LENGTH_INCREASE_STEP;
        multiplier++;

        debug(D_WEB_BUFFER, "web_buffer_sprintf(): increasing web_buffer at position %zu, size = %zu, by %zu bytes (wrote = %zu)\n", wb->len, wb->size, need, wrote);
        buffer_need_bytes(wb, need);

        len = wb->size - wb->len - 1;

        va_start(args, fmt);
        wrote = (size_t) vsnprintfz(&wb->buffer[wb->len], len, fmt, args);
        va_end(args);

    } while(wrote >= len);

    wb->len += wrote;

    // the buffer is \0 terminated by vsnprintf
}


void buffer_rrd_value(BUFFER *wb, calculated_number value)
{
    buffer_need_bytes(wb, 50);

    if(isnan(value) || isinf(value)) {
        buffer_strcat(wb, "null");
        return;
    }
    else
        wb->len += print_calculated_number(&wb->buffer[wb->len], value);

    // terminate it
    buffer_need_bytes(wb, 1);
    wb->buffer[wb->len] = '\0';

    buffer_overflow_check(wb);
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

BUFFER *buffer_create(size_t size)
{
    BUFFER *b;

    debug(D_WEB_BUFFER, "Creating new web buffer of size %zu.", size);

    b = callocz(1, sizeof(BUFFER));
    b->buffer = mallocz(size + sizeof(BUFFER_OVERFLOW_EOF) + 2);
    b->buffer[0] = '\0';
    b->size = size;
    b->contenttype = CT_TEXT_PLAIN;
    buffer_overflow_init(b);
    buffer_overflow_check(b);

    return(b);
}

void buffer_free(BUFFER *b) {
    if(unlikely(!b)) return;

    buffer_overflow_check(b);

    debug(D_WEB_BUFFER, "Freeing web buffer of size %zu.", b->size);

    freez(b->buffer);
    freez(b);
}

void buffer_increase(BUFFER *b, size_t free_size_required)
{
    buffer_overflow_check(b);

    size_t left = b->size - b->len;

    if(left >= free_size_required) return;

    size_t increase = free_size_required - left;
    if(increase < WEB_DATA_LENGTH_INCREASE_STEP) increase = WEB_DATA_LENGTH_INCREASE_STEP;

    debug(D_WEB_BUFFER, "Increasing data buffer from size %zu to %zu.", b->size, b->size + increase);

    b->buffer = reallocz(b->buffer, b->size + increase + sizeof(BUFFER_OVERFLOW_EOF) + 2);
    b->size += increase;

    buffer_overflow_init(b);
    buffer_overflow_check(b);
}
