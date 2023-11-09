// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_FORMATTER_CHARTS2JSON_H
#define NETDATA_API_FORMATTER_CHARTS2JSON_H

#include "rrd2json.h"

void charts2json(RRDHOST *host, BUFFER *wb);
const char* get_release_channel();

#endif //NETDATA_API_FORMATTER_CHARTS2JSON_H
