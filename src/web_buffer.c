#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdlib.h>
#include <string.h>

#ifdef STORAGE_WITH_MATH
#include <math.h>
#endif

#include "common.h"
#include "web_buffer.h"
#include "log.h"

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
		error("BUFFER: length %ld is above size %ld, at line %lu, at function %s() of file '%s'.", b->len, b->size, line, function, file);
		b->len = b->size;
	}

	if(b->buffer[b->size] != '\0' || strcmp(&b->buffer[b->size + 1], BUFFER_OVERFLOW_EOF)) {
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

	buffer_overflow_check(wb);
}

const char *buffer_tostring(BUFFER *wb)
{
	buffer_need_bytes(wb, (size_t)1);
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


void buffer_strcat(BUFFER *wb, const char *txt)
{
	if(unlikely(!txt || !*txt)) return;

	buffer_need_bytes(wb, (size_t)(1));

	char *s = &wb->buffer[wb->len], *end = &wb->buffer[wb->size];
	long len = wb->len;

	while(*txt && s != end) {
		*s++ = *txt++;
		len++;
	}

	wb->len = len;
	buffer_overflow_check(wb);

	if(*txt) {
		debug(D_WEB_BUFFER, "strcat(): increasing web_buffer at position %ld, size = %ld\n", wb->len, wb->size);
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


void buffer_snprintf(BUFFER *wb, size_t len, const char *fmt, ...)
{
	if(unlikely(!fmt || !*fmt)) return;

	buffer_need_bytes(wb, len+1);

	va_list args;
	va_start(args, fmt);
	wb->len += vsnprintf(&wb->buffer[wb->len], len+1, fmt, args);
	va_end(args);

	buffer_overflow_check(wb);

	// the buffer is \0 terminated by vsnprintf
}

void buffer_vsprintf(BUFFER *wb, const char *fmt, va_list args)
{
	if(unlikely(!fmt || !*fmt)) return;

	buffer_need_bytes(wb, 1);

	size_t len = wb->size - wb->len;

	wb->len += vsnprintf(&wb->buffer[wb->len], len, fmt, args);

	buffer_overflow_check(wb);

	// the buffer is \0 terminated by vsnprintf
}

void buffer_sprintf(BUFFER *wb, const char *fmt, ...)
{
	if(unlikely(!fmt || !*fmt)) return;

	buffer_need_bytes(wb, 1);

	size_t len = wb->size - wb->len, wrote;

	va_list args;
	va_start(args, fmt);
	wrote = (size_t) vsnprintf(&wb->buffer[wb->len], len, fmt, args);
	va_end(args);

	if(unlikely(wrote >= len)) {
		// there is bug in vsnprintf() and it returns
		// a number higher to len, but it does not
		// overflow the buffer.
		// our buffer overflow detector will log it
		// if it does.
		buffer_overflow_check(wb);

		debug(D_WEB_BUFFER, "web_buffer_sprintf(): increasing web_buffer at position %ld, size = %ld\n", wb->len, wb->size);
		buffer_need_bytes(wb, len + WEB_DATA_LENGTH_INCREASE_STEP);

		va_start(args, fmt);
		buffer_vsprintf(wb, fmt, args);
		va_end(args);
	}
	else
		wb->len += wrote;

	// the buffer is \0 terminated by vsnprintf
}


void buffer_rrd_value(BUFFER *wb, calculated_number value)
{
	buffer_need_bytes(wb, 50);
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

	char *b = &wb->buffer[wb->len];

	int i = 0;
	b[i++]='D';
	b[i++]='a';
	b[i++]='t';
	b[i++]='e';
	b[i++]='(';
	b[i++]= (char) (48 + year / 1000); year -= (year / 1000) * 1000;
	b[i++]= (char) (48 + year / 100); year -= (year / 100) * 100;
	b[i++]= (char) (48 + year / 10);
	b[i++]= (char) (48 + year % 10);
	b[i++]=',';
	b[i]= (char) (48 + month / 10); if(b[i] != '0') i++;
	b[i++]= (char) (48 + month % 10);
	b[i++]=',';
	b[i]= (char) (48 + day / 10); if(b[i] != '0') i++;
	b[i++]= (char) (48 + day % 10);
	b[i++]=',';
	b[i]= (char) (48 + hours / 10); if(b[i] != '0') i++;
	b[i++]= (char) (48 + hours % 10);
	b[i++]=',';
	b[i]= (char) (48 + minutes / 10); if(b[i] != '0') i++;
	b[i++]= (char) (48 + minutes % 10);
	b[i++]=',';
	b[i]= (char) (48 + seconds / 10); if(b[i] != '0') i++;
	b[i++]= (char) (48 + seconds % 10);
	b[i++]=')';
	b[i]='\0';

	wb->len += i;

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

	int i = 0;
	b[i++]= (char) (48 + year / 1000); year -= (year / 1000) * 1000;
	b[i++]= (char) (48 + year / 100); year -= (year / 100) * 100;
	b[i++]= (char) (48 + year / 10);
	b[i++]= (char) (48 + year % 10);
	b[i++]='-';
	b[i++]= (char) (48 + month / 10);
	b[i++]= (char) (48 + month % 10);
	b[i++]='-';
	b[i++]= (char) (48 + day / 10);
	b[i++]= (char) (48 + day % 10);
	b[i++]=' ';
	b[i++]= (char) (48 + hours / 10);
	b[i++]= (char) (48 + hours % 10);
	b[i++]=':';
	b[i++]= (char) (48 + minutes / 10);
	b[i++]= (char) (48 + minutes % 10);
	b[i++]=':';
	b[i++]= (char) (48 + seconds / 10);
	b[i++]= (char) (48 + seconds % 10);
	b[i]='\0';

	wb->len += i;

	// terminate it
	wb->buffer[wb->len] = '\0';
	buffer_overflow_check(wb);
}

BUFFER *buffer_create(long size)
{
	BUFFER *b;

	debug(D_WEB_BUFFER, "Creating new web buffer of size %d.", size);

	b = calloc(1, sizeof(BUFFER));
	if(!b) {
		error("Cannot allocate a web_buffer.");
		return NULL;
	}

	b->buffer = malloc(size + sizeof(BUFFER_OVERFLOW_EOF) + 2);
	if(!b->buffer) {
		error("Cannot allocate a buffer of size %u.", size + sizeof(BUFFER_OVERFLOW_EOF) + 2);
		free(b);
		return NULL;
	}
	b->buffer[0] = '\0';
	b->size = size;
	b->contenttype = CT_TEXT_PLAIN;
	buffer_overflow_init(b);
	buffer_overflow_check(b);

	return(b);
}

void buffer_free(BUFFER *b)
{
	buffer_overflow_check(b);

	debug(D_WEB_BUFFER, "Freeing web buffer of size %d.", b->size);

	if(b->buffer) free(b->buffer);
	free(b);
}

void buffer_increase(BUFFER *b, size_t free_size_required)
{
	buffer_overflow_check(b);

	size_t left = b->size - b->len;

	if(left >= free_size_required) return;

	size_t increase = free_size_required - left;
	if(increase < WEB_DATA_LENGTH_INCREASE_STEP) increase = WEB_DATA_LENGTH_INCREASE_STEP;

	debug(D_WEB_BUFFER, "Increasing data buffer from size %d to %d.", b->size, b->size + increase);

	b->buffer = realloc(b->buffer, b->size + increase + sizeof(BUFFER_OVERFLOW_EOF) + 2);
	if(!b->buffer) fatal("Failed to increase data buffer from size %d to %d.", b->size + sizeof(BUFFER_OVERFLOW_EOF) + 2, b->size + increase + sizeof(BUFFER_OVERFLOW_EOF) + 2);

	b->size += increase;

	buffer_overflow_init(b);
	buffer_overflow_check(b);
}
