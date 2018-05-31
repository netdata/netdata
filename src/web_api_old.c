// SPDX-License-Identifier: GPL-3.0+
#include "common.h"

int web_client_api_old_data_request(RRDHOST *host, struct web_client *w, char *url, int datasource_type) {
    if(!url || !*url) {
        buffer_flush(w->response.data);
        buffer_sprintf(w->response.data, "Incomplete request.");
        return 400;
    }

    RRDSET *st = NULL;

    char *args = strchr(url, '?');
    if(args) {
        *args='\0';
        args = &args[1];
    }

    // get the name of the data to show
    char *tok = mystrsep(&url, "/");
    if(!tok) tok = "";

    // do we have such a data set?
    if(*tok) {
        debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);
        st = rrdset_find_byname(host, tok);
        if(!st) st = rrdset_find(host, tok);
    }

    if(!st) {
        // we don't have it
        // try to send a file with that name
        buffer_flush(w->response.data);
        return(mysendfile(w, tok));
    }

    // we have it
    debug(D_WEB_CLIENT, "%llu: Found RRD data with name '%s'.", w->id, tok);

    // how many entries does the client want?
    int lines = (int)st->entries;
    int group_count = 1;
    time_t after = 0, before = 0;
    int group_method = GROUP_AVERAGE;
    int nonzero = 0;

    if(url) {
        // parse the lines required
        tok = mystrsep(&url, "/");
        if(tok) lines = str2i(tok);
        if(lines < 1) lines = 1;
    }
    if(url) {
        // parse the group count required
        tok = mystrsep(&url, "/");
        if(tok && *tok) group_count = str2i(tok);
        if(group_count < 1) group_count = 1;
        //if(group_count > save_history / 20) group_count = save_history / 20;
    }
    if(url) {
        // parse the grouping method required
        tok = mystrsep(&url, "/");
        if(tok && *tok) {
            if(strcmp(tok, "max") == 0) group_method = GROUP_MAX;
            else if(strcmp(tok, "average") == 0) group_method = GROUP_AVERAGE;
            else if(strcmp(tok, "sum") == 0) group_method = GROUP_SUM;
            else debug(D_WEB_CLIENT, "%llu: Unknown group method '%s'", w->id, tok);
        }
    }
    if(url) {
        // parse after time
        tok = mystrsep(&url, "/");
        if(tok && *tok) after = str2ul(tok);
        if(after < 0) after = 0;
    }
    if(url) {
        // parse before time
        tok = mystrsep(&url, "/");
        if(tok && *tok) before = str2ul(tok);
        if(before < 0) before = 0;
    }
    if(url) {
        // parse nonzero
        tok = mystrsep(&url, "/");
        if(tok && *tok && strcmp(tok, "nonzero") == 0) nonzero = 1;
    }

    w->response.data->contenttype = CT_APPLICATION_JSON;
    buffer_flush(w->response.data);

    char *google_version = "0.6";
    char *google_reqId = "0";
    char *google_sig = "0";
    char *google_out = "json";
    char *google_responseHandler = "google.visualization.Query.setResponse";
    char *google_outFileName = NULL;
    time_t last_timestamp_in_data = 0;
    if(datasource_type == DATASOURCE_DATATABLE_JSON || datasource_type == DATASOURCE_DATATABLE_JSONP) {

        w->response.data->contenttype = CT_APPLICATION_X_JAVASCRIPT;

        while(args) {
            tok = mystrsep(&args, "&");
            if(tok && *tok) {
                char *name = mystrsep(&tok, "=");
                if(name && *name && strcmp(name, "tqx") == 0) {
                    char *key = mystrsep(&tok, ":");
                    char *value = mystrsep(&tok, ";");
                    if(key && value && *key && *value) {
                        if(strcmp(key, "version") == 0)
                            google_version = value;

                        else if(strcmp(key, "reqId") == 0)
                            google_reqId = value;

                        else if(strcmp(key, "sig") == 0)
                            google_sig = value;

                        else if(strcmp(key, "out") == 0)
                            google_out = value;

                        else if(strcmp(key, "responseHandler") == 0)
                            google_responseHandler = value;

                        else if(strcmp(key, "outFileName") == 0)
                            google_outFileName = value;
                    }
                }
            }
        }

        debug(D_WEB_CLIENT_ACCESS, "%llu: GOOGLE JSONP: version = '%s', reqId = '%s', sig = '%s', out = '%s', responseHandler = '%s', outFileName = '%s'",
                w->id, google_version, google_reqId, google_sig, google_out, google_responseHandler, google_outFileName
        );

        if(datasource_type == DATASOURCE_DATATABLE_JSONP) {
            last_timestamp_in_data = strtoul(google_sig, NULL, 0);

            // check the client wants json
            if(strcmp(google_out, "json") != 0) {
                buffer_sprintf(w->response.data,
                        "%s({version:'%s',reqId:'%s',status:'error',errors:[{reason:'invalid_query',message:'output format is not supported',detailed_message:'the format %s requested is not supported by netdata.'}]});",
                        google_responseHandler, google_version, google_reqId, google_out);
                return 200;
            }
        }
    }

    if(datasource_type == DATASOURCE_DATATABLE_JSONP) {
        buffer_sprintf(w->response.data,
                "%s({version:'%s',reqId:'%s',status:'ok',sig:'%ld',table:",
                google_responseHandler, google_version, google_reqId, st->last_updated.tv_sec);
    }

    debug(D_WEB_CLIENT_ACCESS, "%llu: Sending RRD data '%s' (id %s, %d lines, %d group, %d group_method, %ld after, %ld before).",
            w->id, st->name, st->id, lines, group_count, group_method, after, before);

    time_t timestamp_in_data = rrdset2json_api_old(datasource_type, st, w->response.data, lines, group_count
                                                   , group_method, (unsigned long) after, (unsigned long) before
                                                   , nonzero);

    if(datasource_type == DATASOURCE_DATATABLE_JSONP) {
        if(timestamp_in_data > last_timestamp_in_data)
            buffer_strcat(w->response.data, "});");

        else {
            // the client already has the latest data
            buffer_flush(w->response.data);
            buffer_sprintf(w->response.data,
                    "%s({version:'%s',reqId:'%s',status:'error',errors:[{reason:'not_modified',message:'Data not modified'}]});",
                    google_responseHandler, google_version, google_reqId);
        }
    }

    return 200;
}

inline int web_client_api_old_data_request_json(RRDHOST *host, struct web_client *w, char *url) {
    return web_client_api_old_data_request(host, w, url, DATASOURCE_JSON);
}

inline int web_client_api_old_data_request_jsonp(RRDHOST *host, struct web_client *w, char *url) {
    return web_client_api_old_data_request(host, w, url, DATASOURCE_DATATABLE_JSONP);
}

inline int web_client_api_old_graph_request(RRDHOST *host, struct web_client *w, char *url) {
    // get the name of the data to show
    char *tok = mystrsep(&url, "/?&");
    if(tok && *tok) {
        debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);

        // do we have such a data set?
        RRDSET *st = rrdset_find_byname(host, tok);
        if(!st) st = rrdset_find(host, tok);
        if(!st) {
            // we don't have it
            // try to send a file with that name
            buffer_flush(w->response.data);
            return mysendfile(w, tok);
        }
        st->last_accessed_time = now_realtime_sec();

        debug(D_WEB_CLIENT_ACCESS, "%llu: Sending %s.json of RRD_STATS...", w->id, st->name);
        w->response.data->contenttype = CT_APPLICATION_JSON;
        buffer_flush(w->response.data);
        rrd_graph2json_api_old(st, url, w->response.data);
        return 200;
    }

    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, "Graph name?\r\n");
    return 400;
}

inline int web_client_api_old_list_request(RRDHOST *host, struct web_client *w, char *url) {
    (void)url;

    buffer_flush(w->response.data);
    RRDSET *st;

    rrdhost_rdlock(host);
    rrdset_foreach_read(st, host) {
        if(rrdset_is_available_for_viewers(st))
            buffer_sprintf(w->response.data, "%s\n", st->name);
    }
    rrdhost_unlock(host);

    return 200;
}

inline int web_client_api_old_all_json(RRDHOST *host, struct web_client *w, char *url) {
    (void)url;

    w->response.data->contenttype = CT_APPLICATION_JSON;
    buffer_flush(w->response.data);
    rrd_all2json_api_old(host, w->response.data);
    return 200;
}
