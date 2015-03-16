#include <stdarg.h>
#include <time.h>
#include "storage_number.h"

#ifndef NETDATA_WEB_BUFFER_H
#define NETDATA_WEB_BUFFER_H 1

#define WEB_DATA_LENGTH_INCREASE_STEP 65536

struct web_buffer {
	long size;		// allocation size of buffer
	long bytes;		// current data length in buffer
	long sent;		// current data length sent to output
	char *buffer;	// the buffer
	int contenttype;
	long rbytes; 	// if non-zero, the excepted size of ifd
	time_t date;	// the date this content has been generated
};

// content-types
#define CT_APPLICATION_JSON				1
#define CT_TEXT_PLAIN					2
#define CT_TEXT_HTML					3
#define CT_APPLICATION_X_JAVASCRIPT		4
#define CT_TEXT_CSS						5
#define CT_TEXT_XML						6
#define CT_APPLICATION_XML				7
#define CT_TEXT_XSL						8
#define CT_APPLICATION_OCTET_STREAM		9
#define CT_APPLICATION_X_FONT_TRUETYPE	10
#define CT_APPLICATION_X_FONT_OPENTYPE	11
#define CT_APPLICATION_FONT_WOFF		12
#define CT_APPLICATION_VND_MS_FONTOBJ	13
#define CT_IMAGE_SVG_XML				14

#define web_buffer_printf(wb, args...) wb->bytes += snprintf(&wb->buffer[wb->bytes], (wb->size - wb->bytes), ##args)
#define web_buffer_reset(wb) wb->buffer[wb->bytes = 0] = '\0'

void web_buffer_strcpy(struct web_buffer *wb, const char *txt);
int print_calculated_number(char *str, calculated_number value);
void web_buffer_rrd_value(struct web_buffer *wb, calculated_number value);

void web_buffer_jsdate(struct web_buffer *wb, int year, int month, int day, int hours, int minutes, int seconds);

struct web_buffer *web_buffer_create(long size);
void web_buffer_free(struct web_buffer *b);
void web_buffer_increase(struct web_buffer *b, long free_size_required);

#endif /* NETDATA_WEB_BUFFER_H */
