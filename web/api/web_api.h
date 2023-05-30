// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_API_H
#define NETDATA_WEB_API_H 1

#include "daemon/common.h"
#include "web/api/badges/web_buffer_svg.h"
#include "web/api/formatters/rrd2json.h"
#include "web/api/health/health_cmdapi.h"
#include "web/api/queries/weights.h"

struct web_api_command {
    const char *command;
    uint32_t hash;
    WEB_CLIENT_ACL acl;
    int (*callback)(RRDHOST *host, struct web_client *w, char *url);
};

struct web_client;

int web_client_api_request_vX(RRDHOST *host, struct web_client *w, char *url_path_endpoint, struct web_api_command *api_commands);

static inline void fix_google_param(char *s) {
    if(unlikely(!s || !*s)) return;

    for( ; *s ;s++) {
        if(!isalnum(*s) && *s != '.' && *s != '_' && *s != '-')
            *s = '_';
    }
}

int web_client_api_request_weights(RRDHOST *host, struct web_client *w, char *url, WEIGHTS_METHOD method, WEIGHTS_FORMAT format, size_t api_version);

bool web_client_interrupt_callback(void *data);

#include "web_api_v1.h"
#include "web_api_v2.h"

#endif //NETDATA_WEB_API_H
