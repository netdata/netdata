// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDFUNCTIONS_EXPORTERS_H
#define NETDATA_RRDFUNCTIONS_EXPORTERS_H

#include "rrd.h"

void rrd_chart_functions_expose_rrdpush(RRDSET *st, BUFFER *wb);
void rrd_global_functions_expose_rrdpush(RRDHOST *host, BUFFER *wb, bool dyncfg);

void chart_functions2json(RRDSET *st, BUFFER *wb);
void chart_functions_to_dict(DICTIONARY *rrdset_functions_view, DICTIONARY *dst, void *value, size_t value_size);
void host_functions_to_dict(RRDHOST *host, DICTIONARY *dst, void *value, size_t value_size, STRING **help, STRING **tags,
                            HTTP_ACCESS *access, int *priority);
void host_functions2json(RRDHOST *host, BUFFER *wb);

#endif //NETDATA_RRDFUNCTIONS_EXPORTERS_H
