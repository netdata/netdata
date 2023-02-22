// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api.h"

int web_client_api_request_vX(RRDHOST *host, struct web_client *w, char *url, struct web_api_command *api_commands) {
    if(unlikely(!url || !*url)) {
        buffer_flush(w->response.data);
        buffer_sprintf(w->response.data, "Which API command?");
        return HTTP_RESP_BAD_REQUEST;
    }

    uint32_t hash = simple_hash(url);

    for(int i = 0; api_commands[i].command ; i++) {
        if(unlikely(hash == api_commands[i].hash && !strcmp(url, api_commands[i].command))) {
            if(unlikely(api_commands[i].acl != WEB_CLIENT_ACL_NOCHECK) && !(w->acl & api_commands[i].acl))
                return web_client_permission_denied(w);

            return api_commands[i].callback(host, w, (w->decoded_query_string + 1));
        }
    }

    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, "Unsupported API command: ");
    buffer_strcat_htmlescape(w->response.data, url);
    return HTTP_RESP_NOT_FOUND;
}
