// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_FORMATTER_RRDSET2JSON_H
#define NETDATA_API_FORMATTER_RRDSET2JSON_H

#include "rrd2json.h"

extern void rrdset2json(RRDSET *st, BUFFER *wb, size_t *dimensions_count, size_t *memory_used);

#endif //NETDATA_API_FORMATTER_RRDSET2JSON_H
