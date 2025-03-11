// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_API_H
#define NETDATA_WEB_API_H 1

#define ENABLE_API_V1 1
#define ENABLE_API_v2 1

struct web_client;

#include "database/rrd.h"
#include "maps/maps.h"
#include "functions/functions.h"

#include "web/api/http_header.h"
#include "web/api/http_auth.h"
#include "web/api/formatters/rrd2json.h"
#include "web/api/queries/weights.h"
#include "web/api/request_source.h"

void nd_web_api_init(void);

void web_client_progress_functions_update(void *data, size_t done, size_t all);

void host_labels2json(RRDHOST *host, BUFFER *wb, const char *key);

struct web_api_command {
    const char *api;
    uint32_t hash;
    HTTP_ACL acl;
    HTTP_ACCESS access;
    int (*callback)(RRDHOST *host, struct web_client *w, char *url);
    unsigned int allow_subpaths;
};

struct web_client;

int web_client_api_request_vX(RRDHOST *host, struct web_client *w, char *url_path_endpoint, struct web_api_command *api_commands);

static inline void fix_google_param(char *s) {
    if(unlikely(!s || !*s)) return;

    for( ; *s ;s++) {
        if(!isalnum((uint8_t)*s) && *s != '.' && *s != '_' && *s != '-')
            *s = '_';
    }
}

int web_client_api_request_weights(RRDHOST *host, struct web_client *w, char *url, WEIGHTS_METHOD method, WEIGHTS_FORMAT format, size_t api_version);

bool web_client_interrupt_callback(void *data);

char *format_value_and_unit(char *value_string, size_t value_string_len,
                            NETDATA_DOUBLE value, const char *units, int precision);

#include "web_api_v1.h"
#include "web_api_v2.h"
#include "web_api_v3.h"

#endif //NETDATA_WEB_API_H
