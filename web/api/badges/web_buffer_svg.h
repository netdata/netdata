// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_BUFFER_SVG_H
#define NETDATA_WEB_BUFFER_SVG_H 1

#include "libnetdata/libnetdata.h"
#include "web/server/web_client.h"

extern void buffer_svg(BUFFER *wb, const char *label, calculated_number value, const char *units, const char *label_color, const char *value_color, int precision, int scale, uint32_t options, int fixed_width_lbl, int fixed_width_val, const char* text_color_lbl, const char* text_color_val);
extern char *format_value_and_unit(char *value_string, size_t value_string_len, calculated_number value, const char *units, int precision);

extern int web_client_api_request_v1_badge(struct rrdhost *host, struct web_client *w, char *url);

#include "web/api/web_api_v1.h"

#endif /* NETDATA_WEB_BUFFER_SVG_H */
