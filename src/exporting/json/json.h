// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_JSON_H
#define NETDATA_EXPORTING_JSON_H

#include "exporting/exporting_engine.h"

int init_json_instance(struct instance *instance);
int init_json_http_instance(struct instance *instance);

int format_host_labels_json_plaintext(struct instance *instance, RRDHOST *host);

int format_dimension_collected_json_plaintext(struct instance *instance, RRDDIM *rd);
int format_dimension_stored_json_plaintext(struct instance *instance, RRDDIM *rd);

int open_batch_json_http(struct instance *instance);
int close_batch_json_http(struct instance *instance);

void json_http_prepare_header(struct instance *instance);

#endif //NETDATA_EXPORTING_JSON_H
