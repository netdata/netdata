// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_BUFFER_SVG_H
#define NETDATA_WEB_BUFFER_SVG_H 1

#include "web_api_v1.h"

extern void buffer_svg(BUFFER *wb, const char *label, calculated_number value, const char *units, const char *label_color, const char *value_color, int precision, int scale, uint32_t options);
extern char *format_value_and_unit(char *value_string, size_t value_string_len, calculated_number value, const char *units, int precision);

#endif /* NETDATA_WEB_BUFFER_SVG_H */
