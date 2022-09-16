// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_OPENTSDB_H
#define NETDATA_EXPORTING_OPENTSDB_H

#include "exporting/exporting_engine.h"

int init_opentsdb_telnet_instance(struct instance *instance);
int init_opentsdb_http_instance(struct instance *instance);

void sanitize_opentsdb_label_value(char *dst, const char *src, size_t len);
int format_host_labels_opentsdb_telnet(struct instance *instance, RRDHOST *host);
int format_host_labels_opentsdb_http(struct instance *instance, RRDHOST *host);

int format_dimension_collected_opentsdb_telnet(struct instance *instance, RRDDIM *rd);
int format_dimension_stored_opentsdb_telnet(struct instance *instance, RRDDIM *rd);

int format_dimension_collected_opentsdb_http(struct instance *instance, RRDDIM *rd);
int format_dimension_stored_opentsdb_http(struct instance *instance, RRDDIM *rd);

int open_batch_opentsdb_http(struct instance *instance);
int close_batch_opentsdb_http(struct instance *instance);

void opentsdb_http_prepare_header(struct instance *instance);

#endif //NETDATA_EXPORTING_OPENTSDB_H
