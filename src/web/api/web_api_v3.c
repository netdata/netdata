// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api_v3.h"
#include "v1/api_v1_calls.h"
#include "v2/api_v2_calls.h"

static struct web_api_command api_commands_v3[] = {

    {// terminator
     .api = NULL,
     .hash = 0,
     .acl = HTTP_ACL_NONE,
     .access = HTTP_ACCESS_NONE,
     .callback = NULL,
     .allow_subpaths = 0
    },
};

inline int web_client_api_request_v3(RRDHOST *host, struct web_client *w, char *url_path_endpoint) {
    static int initialized = 0;

    if(unlikely(initialized == 0)) {
        initialized = 1;

        for(int i = 0; api_commands_v3[i].api ; i++)
            api_commands_v3[i].hash = simple_hash(api_commands_v3[i].api);
    }

    return web_client_api_request_vX(host, w, url_path_endpoint, api_commands_v3);
}
