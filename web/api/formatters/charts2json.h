// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_FORMATTER_CHARTS2JSON_H
#define NETDATA_API_FORMATTER_CHARTS2JSON_H

#include "rrd2json.h"

extern void charts2json(RRDHOST *host, BUFFER *wb, int skip_volatile);
extern void chartcollectors2json(RRDHOST *host, BUFFER *wb);

#endif //NETDATA_API_FORMATTER_CHARTS2JSON_H
