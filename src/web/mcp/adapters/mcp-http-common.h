// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_HTTP_COMMON_H
#define NETDATA_MCP_HTTP_COMMON_H

#include "web/server/web_client.h"

#include <stdbool.h>
#include <string.h>

static inline bool mcp_http_extract_api_key(struct web_client *w, char *buffer, size_t buffer_len) {
    if (!w || !buffer || buffer_len == 0)
        return false;

    if (!w->url_query_string_decoded)
        return false;

    const char *query = buffer_tostring(w->url_query_string_decoded);
    if (!query || !*query)
        return false;

    if (*query == '?')
        query++;

    const char *api_key_str = strstr(query, "api_key=");
    if (!api_key_str)
        return false;

    api_key_str += strlen("api_key=");

    size_t i = 0;
    while (api_key_str[i] && api_key_str[i] != '&' && i < buffer_len - 1) {
        buffer[i] = api_key_str[i];
        i++;
    }

    buffer[i] = '\0';
    return i > 0;
}

#endif // NETDATA_MCP_HTTP_COMMON_H
