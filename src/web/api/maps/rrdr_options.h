// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDR_OPTIONS_H
#define NETDATA_RRDR_OPTIONS_H

#include "../web_api.h"

void rrdr_options_to_buffer(BUFFER *wb, RRDR_OPTIONS options);
void rrdr_options_to_buffer_json_array(BUFFER *wb, const char *key, RRDR_OPTIONS options);
void web_client_api_request_data_vX_options_to_string(char *buf, size_t size, RRDR_OPTIONS options);
void rrdr_option_init(void);

#endif //NETDATA_RRDR_OPTIONS_H
