// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_GRAPHITE_H
#define NETDATA_EXPORTING_GRAPHITE_H

#include "exporting/exporting_engine.h"

int init_graphite_instance(struct instance *instance);

void sanitize_graphite_label_value(char *dst, char *src, size_t len);
int format_host_labels_graphite_plaintext(struct instance *instance, RRDHOST *host);

int format_dimension_collected_graphite_plaintext(struct instance *instance, RRDDIM *rd);
int format_dimension_stored_graphite_plaintext(struct instance *instance, RRDDIM *rd);

#endif //NETDATA_EXPORTING_GRAPHITE_H
