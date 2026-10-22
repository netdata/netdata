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
#include "libnetdata/user-auth/user-auth.h"

void nd_web_api_init(void);

void web_client_progress_functions_update(nd_uuid_t *transaction, void *data, size_t done, size_t all);

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

void web_api_command_hashes_init(struct web_api_command *api_commands, SPINLOCK *spinlock, bool *initialized);

int web_client_api_request_vX(RRDHOST *host, struct web_client *w, char *url_path_endpoint, struct web_api_command *api_commands);

// Google Visualization (tqx) and JSONP parameters of the data endpoints
typedef struct data_query_jsonp {
    char *google_version;
    char *google_reqId;
    char *google_sig;
    char *google_out;
    char *responseHandler;
    char *outFileName;
    time_t google_timestamp;
} DATA_QUERY_JSONP;

#define DATA_QUERY_JSONP_DEFAULTS (DATA_QUERY_JSONP){ \
        .google_version = "0.6",                       \
        .google_reqId = "0",                           \
        .google_sig = "0",                             \
        .google_out = "json",                          \
    }

void data_query_jsonp_parse_tqx(DATA_QUERY_JSONP *jp, char *value, DATASOURCE_FORMAT *format);
void data_query_jsonp_sanitize(DATA_QUERY_JSONP *jp);
int data_query_response_begin(struct web_client *w, DATA_QUERY_JSONP *jp, DATASOURCE_FORMAT format, int timeout, time_t sig);
int data_query_response_execute(ONEWAYALLOC *owa, struct web_client *w, struct query_target *qt, DATA_QUERY_JSONP *jp, DATASOURCE_FORMAT format);

int web_client_api_request_weights(RRDHOST *host, struct web_client *w, char *url, WEIGHTS_METHOD method, WEIGHTS_FORMAT format, size_t api_version);

bool web_client_interrupt_callback(void *data);

char *format_value_and_unit(char *value_string, size_t value_string_len,
                            NETDATA_DOUBLE value, const char *units, int precision);

void web_client_ensure_proper_authorization(struct web_client *w);

#include "web_api_v1.h"
#include "web_api_v2.h"
#include "web_api_v3.h"

#endif //NETDATA_WEB_API_H
