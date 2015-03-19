#include <stdlib.h>

#ifdef STORAGE_WITH_MATH
#include <math.h>
#endif

#include "web_buffer.h"

#include "common.h"
#include "log.h"

void web_buffer_strcpy(struct web_buffer *wb, const char *txt)
{
	char *buffer = wb->buffer;
	long bytes = wb->bytes, size = wb->size, i = 0;

	while(txt[i] && bytes < size)
		buffer[bytes++] = txt[i++];

	wb->bytes = bytes;
}

void web_buffer_rrd_value(struct web_buffer *wb, calculated_number value)
{
	if(wb->size - wb->bytes < 50) return;
	wb->bytes += print_calculated_number(&wb->buffer[wb->bytes], value);
}

// generate a javascript date, the fastest possible way...
void web_buffer_jsdate(struct web_buffer *wb, int year, int month, int day, int hours, int minutes, int seconds)
{
	//         10        20        30      = 35
	// 01234567890123456789012345678901234
	// Date(2014, 04, 01, 03, 28, 20, 065)

	if(wb->size - wb->bytes < 36) return;

	char *b = &wb->buffer[wb->bytes];

	int i = 0;
	b[i++]='D';
	b[i++]='a';
	b[i++]='t';
	b[i++]='e';
	b[i++]='(';
	b[i++]= 48 + year / 1000; year -= (year / 1000) * 1000;
	b[i++]= 48 + year / 100; year -= (year / 100) * 100;
	b[i++]= 48 + year / 10;
	b[i++]= 48 + year % 10;
	b[i++]=',';
	//b[i++]=' ';
	b[i]= 48 + month / 10; if(b[i] != '0') i++;
	b[i++]= 48 + month % 10;
	b[i++]=',';
	//b[i++]=' ';
	b[i]= 48 + day / 10; if(b[i] != '0') i++;
	b[i++]= 48 + day % 10;
	b[i++]=',';
	//b[i++]=' ';
	b[i]= 48 + hours / 10; if(b[i] != '0') i++;
	b[i++]= 48 + hours % 10;
	b[i++]=',';
	//b[i++]=' ';
	b[i]= 48 + minutes / 10; if(b[i] != '0') i++;
	b[i++]= 48 + minutes % 10;
	b[i++]=',';
	//b[i++]=' ';
	b[i]= 48 + seconds / 10; if(b[i] != '0') i++;
	b[i++]= 48 + seconds % 10;
	b[i++]=')';
	b[i]='\0';

	wb->bytes += i;
}

struct web_buffer *web_buffer_create(long size)
{
	struct web_buffer *b;

	debug(D_WEB_BUFFER, "Creating new web buffer of size %d.", size);

	b = calloc(1, sizeof(struct web_buffer));
	if(!b) {
		error("Cannot allocate a web_buffer.");
		return NULL;
	}

	b->buffer = malloc(size);
	if(!b->buffer) {
		error("Cannot allocate a buffer of size %u.", size);
		free(b);
		return NULL;
	}
	b->buffer[0] = '\0';
	b->size = size;
	b->contenttype = CT_TEXT_PLAIN;
	return(b);
}

void web_buffer_free(struct web_buffer *b)
{
	debug(D_WEB_BUFFER, "Freeing web buffer of size %d.", b->size);

	if(b->buffer) free(b->buffer);
	free(b);
}

void web_buffer_increase(struct web_buffer *b, long free_size_required)
{
	long left = b->size - b->bytes;

	if(left >= free_size_required) return;
	long increase = free_size_required - left;
	if(increase < WEB_DATA_LENGTH_INCREASE_STEP) increase = WEB_DATA_LENGTH_INCREASE_STEP;

	debug(D_WEB_BUFFER, "Increasing data buffer from size %d to %d.", b->size, b->size + increase);

	b->buffer = realloc(b->buffer, b->size + increase);
	if(!b->buffer) fatal("Failed to increase data buffer from size %d to %d.", b->size, b->size + increase);
	
	b->size += increase;
}
