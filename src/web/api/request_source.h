// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_REQUEST_SOURCE_H
#define NETDATA_REQUEST_SOURCE_H

#include "web_api.h"

typedef struct {
    char client_ip[INET6_ADDRSTRLEN];
    char forwarded_for[INET6_ADDRSTRLEN];
    char client_name[CLOUD_CLIENT_NAME_LENGTH];
    ND_UUID cloud_account_id;
    WEB_CLIENT_FLAGS auth;
    HTTP_USER_ROLE user_role;
    HTTP_ACCESS access;
} PARSED_REQUEST_SOURCE;

bool parse_request_source(const char *src, PARSED_REQUEST_SOURCE *parsed);

bool request_source_is_cloud(const char *source);

void web_client_api_request_vX_source_to_buffer(struct web_client *w, BUFFER *source);

#endif //NETDATA_REQUEST_SOURCE_H
