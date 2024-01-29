// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HTTP_AUTH_H
#define NETDATA_HTTP_AUTH_H

#include "web_api.h"

struct web_client;

extern bool netdata_is_protected_by_bearer;

bool extract_bearer_token_from_request(struct web_client *w, char *dst, size_t dst_len);

time_t bearer_create_token(uuid_t *uuid, struct web_client *w);
bool web_client_bearer_token_auth(struct web_client *w, const char *v);

static inline bool http_access_user_has_enough_access_level_for_endpoint(HTTP_ACCESS user, HTTP_ACCESS endpoint) {
    return ((user & endpoint) == endpoint);
}

#endif //NETDATA_HTTP_AUTH_H
