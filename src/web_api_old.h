// SPDX-License-Identifier: GPL-3.0+
#ifndef NETDATA_WEB_API_OLD_H
#define NETDATA_WEB_API_OLD_H

#include "common.h"

extern int web_client_api_old_data_request(RRDHOST *host, struct web_client *w, char *url, int datasource_type);
extern int web_client_api_old_data_request_json(RRDHOST *host, struct web_client *w, char *url);
extern int web_client_api_old_data_request_jsonp(RRDHOST *host, struct web_client *w, char *url);
extern int web_client_api_old_graph_request(RRDHOST *host, struct web_client *w, char *url);
extern int web_client_api_old_list_request(RRDHOST *host, struct web_client *w, char *url);
extern int web_client_api_old_all_json(RRDHOST *host, struct web_client *w, char *url);

#endif //NETDATA_WEB_API_OLD_H
