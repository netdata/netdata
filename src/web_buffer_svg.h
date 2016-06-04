#include "web_buffer.h"
#include "dictionary.h"

#ifndef NETDATA_WEB_BUFFER_SVG_H
#define NETDATA_WEB_BUFFER_SVG_H 1

extern void buffer_svg(BUFFER *wb, const char *label, calculated_number value, const char *units, const char *label_color, const char *value_color, int value_is_null);

#endif /* NETDATA_WEB_BUFFER_SVG_H */
