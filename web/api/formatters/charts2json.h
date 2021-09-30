// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_FORMATTER_CHARTS2JSON_H
#define NETDATA_API_FORMATTER_CHARTS2JSON_H

#include "rrd2json.h"

extern void charts2json(RRDHOST *host, BUFFER *wb, int skip_volatile, int show_archived);
extern void chartcollectors2json(RRDHOST *host, BUFFER *wb);
extern const char* get_release_channel();

#ifdef ENABLE_JSONC
#include <json-c/json.h>
json_object *chartcollectors_json(RRDHOST *host);
#else
extern void chartcollectors2json(RRDHOST *host, BUFFER *wb);
#endif

#endif //NETDATA_API_FORMATTER_CHARTS2JSON_H
