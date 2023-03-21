// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_logsmanagement_web_log.h"

void web_log_chart_init(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_meta->chart_data_web_log = callocz(1, sizeof (struct Chart_data_web_log));
    chart_data_web_log_t *chart_data = chart_meta->chart_data_web_log;
    long chart_prio = chart_meta->base_prio;

    /* Number of collected logs - initialise */
    chart_data->st_lines = rrdset_create_localhost(
            (char *) p_file_info->chart_name
            , "collected_logs"
            , NULL
            , "collected_logs"
            , NULL
            , "Collected log records"
            , "records"
            , "logsmanagement.plugin"
            , NULL
            , ++chart_prio
            , p_file_info->update_every
            , RRDSET_TYPE_AREA
    );
    chart_data->dim_lines_total = rrddim_add(chart_data->st_lines, "Total records", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    chart_data->dim_lines_rate = rrddim_add(chart_data->st_lines, "New records", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    /* Vhost - initialise */
    if(p_file_info->parser_config->chart_config & CHART_VHOST){
        chart_data->st_vhost = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "vhost"
                , NULL
                , "vhost"
                , NULL
                , "Requests by Vhost"
                , "requests"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
    }

    /* Port - initialise */
    if(p_file_info->parser_config->chart_config & CHART_PORT){
        chart_data->st_port = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "port"
                , NULL
                , "port"
                , NULL
                , "Requests by Port"
                , "requests"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
    }

    /* IP Version - initialise */
    if(p_file_info->parser_config->chart_config & CHART_IP_VERSION){
        chart_data->st_ip_ver = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "ip_version"
                , NULL
                , "ip_version"
                , NULL
                , "Requests by IP version"
                , "requests"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_ip_ver_4 = rrddim_add(chart_data->st_ip_ver, "ipv4", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_ip_ver_6 = rrddim_add(chart_data->st_ip_ver, "ipv6", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_ip_ver_invalid = rrddim_add(chart_data->st_ip_ver, "invalid", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    /* Request client current poll - initialise */
    if(p_file_info->parser_config->chart_config & CHART_REQ_CLIENT_CURRENT){
        chart_data->st_req_client_current = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "clients"
                , NULL
                , "clients"
                , NULL
                , "Current Poll Unique Client IPs"
                , "unique ips"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_req_client_current_ipv4 = rrddim_add(chart_data->st_req_client_current, "ipv4", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_req_client_current_ipv6 = rrddim_add(chart_data->st_req_client_current, "ipv6", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    /* Request client all-time - initialise */
    if(p_file_info->parser_config->chart_config & CHART_REQ_CLIENT_ALL_TIME){
        chart_data->st_req_client_all_time = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "clients_all"
                , NULL
                , "clients"
                , NULL
                , "All Time Unique Client IPs"
                , "unique ips"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_req_client_all_time_ipv4 = rrddim_add(chart_data->st_req_client_all_time, "ipv4", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        chart_data->dim_req_client_all_time_ipv6 = rrddim_add(chart_data->st_req_client_all_time, "ipv6", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    /* Request methods - initialise */
    if(p_file_info->parser_config->chart_config & CHART_REQ_METHODS){
        chart_data->st_req_methods = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "http_methods"
                , NULL
                , "http_methods"
                , NULL
                , "Requests Per HTTP Method"
                , "requests"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        // Too many method dimensions, create only "GET" so that the chart is visible
        // and the remaining ones as they become non-zero.
        chart_data->dim_req_method_get = rrddim_add(chart_data->st_req_methods, "GET", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    /* Request protocol - initialise */
    if(p_file_info->parser_config->chart_config & CHART_REQ_PROTO){
        chart_data->st_req_proto = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "http_versions"
                , NULL
                , "http_versions"
                , NULL
                , "Requests Per HTTP Version"
                , "requests"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_req_proto_http_1 = rrddim_add(chart_data->st_req_proto, "1.0", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_req_proto_http_1_1 = rrddim_add(chart_data->st_req_proto, "1.1", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_req_proto_http_2 = rrddim_add(chart_data->st_req_proto, "2.0", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_req_proto_other = rrddim_add(chart_data->st_req_proto, "other", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    /* Request bandwidth - initialise */
    if(p_file_info->parser_config->chart_config & CHART_BANDWIDTH){
        chart_data->st_bandwidth = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "bandwidth"
                , NULL
                , "bandwidth"
                , NULL
                , "Bandwidth"
                , "kilobits"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_bandwidth_req_size = rrddim_add(chart_data->st_bandwidth, "received", NULL, 8, 1000, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_bandwidth_resp_size = rrddim_add(chart_data->st_bandwidth, "sent", NULL, -8, 1000, RRD_ALGORITHM_INCREMENTAL);
    }

    /* Request processing time - initialise */
    if(p_file_info->parser_config->chart_config & CHART_REQ_PROC_TIME){
        chart_data->st_req_proc_time = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "timings"
                , NULL
                , "timings"
                , NULL
                , "Request Processing Time"
                , "milliseconds"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_LINE
        );
        chart_data->dim_req_proc_time_min = rrddim_add(chart_data->st_req_proc_time, "min", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        chart_data->dim_req_proc_time_max = rrddim_add(chart_data->st_req_proc_time, "max", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        chart_data->dim_req_proc_time_avg = rrddim_add(chart_data->st_req_proc_time, "avg", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }

    /* Response code family - initialise */
    if(p_file_info->parser_config->chart_config & CHART_RESP_CODE_FAMILY){
        chart_data->st_resp_code_family = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "responses"
                , NULL
                , "responses"
                , NULL
                , "Response Codes"
                , "requests"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_resp_code_family_1xx = rrddim_add(chart_data->st_resp_code_family, "1xx", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_resp_code_family_2xx = rrddim_add(chart_data->st_resp_code_family, "2xx", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_resp_code_family_3xx = rrddim_add(chart_data->st_resp_code_family, "3xx", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL); 
        chart_data->dim_resp_code_family_4xx = rrddim_add(chart_data->st_resp_code_family, "4xx", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL); 
        chart_data->dim_resp_code_family_5xx = rrddim_add(chart_data->st_resp_code_family, "5xx", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL); 
        chart_data->dim_resp_code_family_other = rrddim_add(chart_data->st_resp_code_family, "other", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);     
    }   

    /* Response code - initialise */
    if(p_file_info->parser_config->chart_config & CHART_RESP_CODE){
        chart_data->st_resp_code = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "detailed_responses"
                , NULL
                , "responses"
                , NULL
                , "Detailed Response Codes"
                , "requests"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        // Too many response code dimensions, create only "other" so that the 
        // chart is visible and the remaining ones as they become non-zero.
        chart_data->dim_resp_code[RESP_CODE_ARR_SIZE - 1] = rrddim_add(chart_data->st_resp_code, "other", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    /* Response code type - initialise */
    if(p_file_info->parser_config->chart_config & CHART_RESP_CODE_TYPE){
        chart_data->st_resp_code_type = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "response_types"
                , NULL
                , "responses"
                , NULL
                , "Response Statuses"
                , "requests"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_resp_code_type_success = rrddim_add(chart_data->st_resp_code_type, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_resp_code_type_redirect = rrddim_add(chart_data->st_resp_code_type, "redirect", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_resp_code_type_bad = rrddim_add(chart_data->st_resp_code_type, "bad", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL); 
        chart_data->dim_resp_code_type_error = rrddim_add(chart_data->st_resp_code_type, "error", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL); 
        chart_data->dim_resp_code_type_other = rrddim_add(chart_data->st_resp_code_type, "other", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL); 
    }

    /* SSL protocol - initialise */
    if(p_file_info->parser_config->chart_config & CHART_SSL_PROTO){
        chart_data->st_ssl_proto = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "ssl_protocol"
                , NULL
                , "ssl_protocol"
                , NULL
                , "Requests Per SSL Protocol"
                , "requests"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_ssl_proto_tlsv1 = rrddim_add(chart_data->st_ssl_proto, "TLSV1", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_ssl_proto_tlsv1_1 = rrddim_add(chart_data->st_ssl_proto, "TLSV1.1", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_ssl_proto_tlsv1_2 = rrddim_add(chart_data->st_ssl_proto, "TLSV1.2", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_ssl_proto_tlsv1_3 = rrddim_add(chart_data->st_ssl_proto, "TLSV1.3", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_ssl_proto_sslv2 = rrddim_add(chart_data->st_ssl_proto, "SSLV2", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_ssl_proto_sslv3 = rrddim_add(chart_data->st_ssl_proto, "SSLV3", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        chart_data->dim_ssl_proto_other = rrddim_add(chart_data->st_ssl_proto, "other", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    /* SSL cipher suite - initialise */
    if(p_file_info->parser_config->chart_config & CHART_SSL_CIPHER){
        chart_data->st_ssl_cipher = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "ssl_cipher_suite"
                , NULL
                , "ssl_cipher_suite"
                , NULL
                , "Requests by SSL cipher suite"
                , "requests"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
    }

}


void web_log_chart_collect(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_data_web_log_t *chart_data = chart_meta->chart_data_web_log;

    /* Number of lines - collect */
    chart_data->num_lines_total = p_file_info->parser_metrics->num_lines_total;
    chart_data->num_lines_rate += p_file_info->parser_metrics->num_lines_rate;
    p_file_info->parser_metrics->num_lines_rate = 0;

    /* Vhost - collect */
    if(p_file_info->parser_config->chart_config & CHART_VHOST){
        for(int j = 0; j < p_file_info->parser_metrics->web_log->vhost_arr.size; j++){
            int k;
            for(k = 0; k < chart_data->vhost_size; k++){
                if(!strcmp(p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].name, string2str(chart_data->dim_vhosts[k]->name))){
                    chart_data->num_vhosts[k] += p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].count;
                    p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].count = 0;
                    // debug(D_LOGS_MANAG, "vhost found:%s", p_file_info->parser_metrics->vhost_arr.vhosts[j].name);
                    break;
                }
            }
            if(chart_data->vhost_size == k){ // New vhost not in existing dimensions
                chart_data->vhost_size++;

                if(chart_data->vhost_size >= chart_data->vhost_size_max){
                    chart_data->vhost_size_max = chart_data->vhost_size * LOG_PARSER_METRICS_VHOST_BUFFS_SCALE_FACTOR + 1;

                    chart_data->dim_vhosts = reallocz(chart_data->dim_vhosts, chart_data->vhost_size_max * sizeof(RRDDIM));
                    chart_data->num_vhosts = reallocz(chart_data->num_vhosts, chart_data->vhost_size_max * sizeof(collected_number));
                }

                debug(D_LOGS_MANAG, "New vhost:%s", p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].name);
                
                chart_data->dim_vhosts[chart_data->vhost_size - 1] = rrddim_add(chart_data->st_vhost, 
                    p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                
                chart_data->num_vhosts[chart_data->vhost_size - 1] = p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].count;
                p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].count = 0;
            }
        }
    }

    /* Port - collect */
    if(p_file_info->parser_config->chart_config & CHART_PORT){
        for(int j = 0; j < p_file_info->parser_metrics->web_log->port_arr.size; j++){
            int k;
            for(k = 0; k < chart_data->port_size; k++){
                if(p_file_info->parser_metrics->web_log->port_arr.ports[j].port == chart_data->ports[k]){
                    chart_data->num_ports[k] += p_file_info->parser_metrics->web_log->port_arr.ports[j].count;
                    p_file_info->parser_metrics->web_log->port_arr.ports[j].count = 0;
                    break;
                }
            }
            if(chart_data->port_size == k){ // New port not in existing dimensions
                chart_data->port_size++;

                if(chart_data->port_size >= chart_data->port_size_max){
                    chart_data->port_size_max = chart_data->port_size * LOG_PARSER_METRICS_PORT_BUFFS_SCALE_FACTOR + 1;

                    chart_data->ports = reallocz(chart_data->ports, chart_data->port_size_max * sizeof(int));
                    chart_data->dim_ports = reallocz(chart_data->dim_ports, chart_data->port_size_max * sizeof(RRDDIM));
                    chart_data->num_ports = reallocz(chart_data->num_ports, chart_data->port_size_max * sizeof(collected_number));
                }

                chart_data->ports[chart_data->port_size - 1] = p_file_info->parser_metrics->web_log->port_arr.ports[j].port;

                if(unlikely(chart_data->ports[chart_data->port_size - 1] == WEB_LOG_INVALID_PORT)){
                chart_data->dim_ports[chart_data->port_size - 1] = rrddim_add(chart_data->st_port, 
                    "invalid", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else {
                    char port_name[6] = "";
                    snprintf(port_name, 6, "%d", chart_data->ports[chart_data->port_size - 1]);
                    chart_data->dim_ports[chart_data->port_size - 1] = rrddim_add(chart_data->st_port, 
                        port_name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                chart_data->num_ports[chart_data->port_size - 1] = p_file_info->parser_metrics->web_log->port_arr.ports[j].count;
                p_file_info->parser_metrics->web_log->port_arr.ports[j].count = 0;
            }
        }
    }

    /* IP Version - collect */
    if(p_file_info->parser_config->chart_config & CHART_IP_VERSION){
        chart_data->num_ip_ver_4 += p_file_info->parser_metrics->web_log->ip_ver.v4;
        chart_data->num_ip_ver_6 += p_file_info->parser_metrics->web_log->ip_ver.v6;
        chart_data->num_ip_ver_invalid += p_file_info->parser_metrics->web_log->ip_ver.invalid;
        memset(&p_file_info->parser_metrics->web_log->ip_ver, 0, sizeof(p_file_info->parser_metrics->web_log->ip_ver));
    }

    /* Request client current poll - collect */
    if(p_file_info->parser_config->chart_config & CHART_REQ_CLIENT_CURRENT){
        chart_data->num_req_client_current_ipv4 += p_file_info->parser_metrics->web_log->req_clients_current_arr.ipv4_size;
        p_file_info->parser_metrics->web_log->req_clients_current_arr.ipv4_size = 0;  
        chart_data->num_req_client_current_ipv6 += p_file_info->parser_metrics->web_log->req_clients_current_arr.ipv6_size;
        p_file_info->parser_metrics->web_log->req_clients_current_arr.ipv6_size = 0;
    }

    /* Request client all-time - collect */
    if(p_file_info->parser_config->chart_config & CHART_REQ_CLIENT_ALL_TIME){
        chart_data->num_req_client_all_time_ipv4 = p_file_info->parser_metrics->web_log->req_clients_alltime_arr.ipv4_size;  
        chart_data->num_req_client_all_time_ipv6 = p_file_info->parser_metrics->web_log->req_clients_alltime_arr.ipv6_size;
    }
    
    /* Request methods - collect */
    if(p_file_info->parser_config->chart_config & CHART_REQ_METHODS){
        chart_data->num_req_method_acl += p_file_info->parser_metrics->web_log->req_method.acl;
        chart_data->num_req_method_baseline_control += p_file_info->parser_metrics->web_log->req_method.baseline_control;
        chart_data->num_req_method_bind += p_file_info->parser_metrics->web_log->req_method.bind;
        chart_data->num_req_method_checkin += p_file_info->parser_metrics->web_log->req_method.checkin;
        chart_data->num_req_method_checkout += p_file_info->parser_metrics->web_log->req_method.checkout;
        chart_data->num_req_method_connect += p_file_info->parser_metrics->web_log->req_method.connect;
        chart_data->num_req_method_copy += p_file_info->parser_metrics->web_log->req_method.copy;
        chart_data->num_req_method_delet += p_file_info->parser_metrics->web_log->req_method.delet;
        chart_data->num_req_method_get += p_file_info->parser_metrics->web_log->req_method.get;
        chart_data->num_req_method_head += p_file_info->parser_metrics->web_log->req_method.head;
        chart_data->num_req_method_label += p_file_info->parser_metrics->web_log->req_method.label;
        chart_data->num_req_method_link += p_file_info->parser_metrics->web_log->req_method.link;
        chart_data->num_req_method_lock += p_file_info->parser_metrics->web_log->req_method.lock;
        chart_data->num_req_method_merge += p_file_info->parser_metrics->web_log->req_method.merge;
        chart_data->num_req_method_mkactivity += p_file_info->parser_metrics->web_log->req_method.mkactivity;
        chart_data->num_req_method_mkcalendar += p_file_info->parser_metrics->web_log->req_method.mkcalendar;
        chart_data->num_req_method_mkcol += p_file_info->parser_metrics->web_log->req_method.mkcol;
        chart_data->num_req_method_mkredirectref += p_file_info->parser_metrics->web_log->req_method.mkredirectref;
        chart_data->num_req_method_mkworkspace += p_file_info->parser_metrics->web_log->req_method.mkworkspace;
        chart_data->num_req_method_move += p_file_info->parser_metrics->web_log->req_method.move;
        chart_data->num_req_method_options += p_file_info->parser_metrics->web_log->req_method.options;
        chart_data->num_req_method_orderpatch += p_file_info->parser_metrics->web_log->req_method.orderpatch;
        chart_data->num_req_method_patch += p_file_info->parser_metrics->web_log->req_method.patch;
        chart_data->num_req_method_post += p_file_info->parser_metrics->web_log->req_method.post;
        chart_data->num_req_method_pri += p_file_info->parser_metrics->web_log->req_method.pri;
        chart_data->num_req_method_propfind += p_file_info->parser_metrics->web_log->req_method.propfind;
        chart_data->num_req_method_proppatch += p_file_info->parser_metrics->web_log->req_method.proppatch;
        chart_data->num_req_method_put += p_file_info->parser_metrics->web_log->req_method.put;
        chart_data->num_req_method_rebind += p_file_info->parser_metrics->web_log->req_method.rebind;
        chart_data->num_req_method_report += p_file_info->parser_metrics->web_log->req_method.report;
        chart_data->num_req_method_search += p_file_info->parser_metrics->web_log->req_method.search;
        chart_data->num_req_method_trace += p_file_info->parser_metrics->web_log->req_method.trace;
        chart_data->num_req_method_unbind += p_file_info->parser_metrics->web_log->req_method.unbind;
        chart_data->num_req_method_uncheckout += p_file_info->parser_metrics->web_log->req_method.uncheckout;
        chart_data->num_req_method_unlink += p_file_info->parser_metrics->web_log->req_method.unlink;
        chart_data->num_req_method_unlock += p_file_info->parser_metrics->web_log->req_method.unlock;
        chart_data->num_req_method_update += p_file_info->parser_metrics->web_log->req_method.update;
        chart_data->num_req_method_updateredirectref += p_file_info->parser_metrics->web_log->req_method.updateredirectref;
        memset(&p_file_info->parser_metrics->web_log->req_method, 0, sizeof(p_file_info->parser_metrics->web_log->req_method));
    }

    /* Request protocol - collect */
    if(p_file_info->parser_config->chart_config & CHART_REQ_PROTO){
        chart_data->num_req_proto_http_1 += p_file_info->parser_metrics->web_log->req_proto.http_1;
        chart_data->num_req_proto_http_1_1 += p_file_info->parser_metrics->web_log->req_proto.http_1_1;
        chart_data->num_req_proto_http_2 += p_file_info->parser_metrics->web_log->req_proto.http_2;
        chart_data->num_req_proto_other += p_file_info->parser_metrics->web_log->req_proto.other;
        memset(&p_file_info->parser_metrics->web_log->req_proto, 0, sizeof(p_file_info->parser_metrics->web_log->req_proto));
    }

    /* Request bandwidth - collect */
    if(p_file_info->parser_config->chart_config & CHART_BANDWIDTH){
        chart_data->num_bandwidth_req_size += p_file_info->parser_metrics->web_log->bandwidth.req_size;
        chart_data->num_bandwidth_resp_size += p_file_info->parser_metrics->web_log->bandwidth.resp_size;
        memset(&p_file_info->parser_metrics->web_log->bandwidth, 0, sizeof(p_file_info->parser_metrics->web_log->bandwidth));
    }

    /* Request proc time - collect */
    if(p_file_info->parser_config->chart_config & CHART_REQ_PROC_TIME){
        chart_data->num_req_proc_time_min = p_file_info->parser_metrics->web_log->req_proc_time.min;
        chart_data->num_req_proc_time_max = p_file_info->parser_metrics->web_log->req_proc_time.max;
        chart_data->num_req_proc_time_avg = p_file_info->parser_metrics->web_log->req_proc_time.count ? 
            p_file_info->parser_metrics->web_log->req_proc_time.sum / p_file_info->parser_metrics->web_log->req_proc_time.count : 0;
        memset(&p_file_info->parser_metrics->web_log->req_proc_time, 0, sizeof(p_file_info->parser_metrics->web_log->req_proc_time));
    }

    /* Response code family - collect */
    if(p_file_info->parser_config->chart_config & CHART_RESP_CODE_FAMILY){
        chart_data->num_resp_code_family_1xx += p_file_info->parser_metrics->web_log->resp_code_family.resp_1xx;
        chart_data->num_resp_code_family_2xx += p_file_info->parser_metrics->web_log->resp_code_family.resp_2xx;
        chart_data->num_resp_code_family_3xx += p_file_info->parser_metrics->web_log->resp_code_family.resp_3xx;
        chart_data->num_resp_code_family_4xx += p_file_info->parser_metrics->web_log->resp_code_family.resp_4xx;
        chart_data->num_resp_code_family_5xx += p_file_info->parser_metrics->web_log->resp_code_family.resp_5xx;
        chart_data->num_resp_code_family_other += p_file_info->parser_metrics->web_log->resp_code_family.other;
        memset(&p_file_info->parser_metrics->web_log->resp_code_family, 0, sizeof(p_file_info->parser_metrics->web_log->resp_code_family));
    }

    /* Response code - collect */
    if(p_file_info->parser_config->chart_config & CHART_RESP_CODE){
        for(int j = 0; j < RESP_CODE_ARR_SIZE; j++){
            chart_data->num_resp_code[j] += p_file_info->parser_metrics->web_log->resp_code[j];
            p_file_info->parser_metrics->web_log->resp_code[j] = 0;
        }
    }

    /* Response code type - collect */
    if(p_file_info->parser_config->chart_config & CHART_RESP_CODE_TYPE){
        chart_data->num_resp_code_type_success += p_file_info->parser_metrics->web_log->resp_code_type.resp_success;
        chart_data->num_resp_code_type_redirect += p_file_info->parser_metrics->web_log->resp_code_type.resp_redirect;
        chart_data->num_resp_code_type_bad += p_file_info->parser_metrics->web_log->resp_code_type.resp_bad;
        chart_data->num_resp_code_type_error += p_file_info->parser_metrics->web_log->resp_code_type.resp_error;
        chart_data->num_resp_code_type_other += p_file_info->parser_metrics->web_log->resp_code_type.other;
        memset(&p_file_info->parser_metrics->web_log->resp_code_type, 0, sizeof(p_file_info->parser_metrics->web_log->resp_code_type));
    }

    /* SSL protocol - collect */
    if(p_file_info->parser_config->chart_config & CHART_SSL_PROTO){
        chart_data->num_ssl_proto_tlsv1 += p_file_info->parser_metrics->web_log->ssl_proto.tlsv1;
        chart_data->num_ssl_proto_tlsv1_1 += p_file_info->parser_metrics->web_log->ssl_proto.tlsv1_1;
        chart_data->num_ssl_proto_tlsv1_2 += p_file_info->parser_metrics->web_log->ssl_proto.tlsv1_2;
        chart_data->num_ssl_proto_tlsv1_3 += p_file_info->parser_metrics->web_log->ssl_proto.tlsv1_3;
        chart_data->num_ssl_proto_sslv2 += p_file_info->parser_metrics->web_log->ssl_proto.sslv2;
        chart_data->num_ssl_proto_sslv3 += p_file_info->parser_metrics->web_log->ssl_proto.sslv3;
        chart_data->num_ssl_proto_other += p_file_info->parser_metrics->web_log->ssl_proto.other;
        memset(&p_file_info->parser_metrics->web_log->ssl_proto, 0, sizeof(p_file_info->parser_metrics->web_log->ssl_proto));
    }

    /* SSL cipher suite - collect */
    if(p_file_info->parser_config->chart_config & CHART_SSL_CIPHER){
        for(int j = 0; j < p_file_info->parser_metrics->web_log->ssl_cipher_arr.size; j++){
            int k;
            for(k = 0; k < chart_data->ssl_cipher_size; k++){
                if(!strcmp(p_file_info->parser_metrics->web_log->ssl_cipher_arr.ssl_ciphers[j].string, string2str(chart_data->dim_ssl_ciphers[k]->name))){
                    chart_data->num_ssl_ciphers[k] += p_file_info->parser_metrics->web_log->ssl_cipher_arr.ssl_ciphers[j].count;
                    p_file_info->parser_metrics->web_log->ssl_cipher_arr.ssl_ciphers[j].count = 0;
                    break;
                }
            }
            if(chart_data->ssl_cipher_size == k){ // New SSL cipher suite not in existing dimensions
                chart_data->ssl_cipher_size++;

                if(chart_data->ssl_cipher_size >= chart_data->ssl_cipher_size_max){
                    chart_data->ssl_cipher_size_max = chart_data->ssl_cipher_size * LOG_PARSER_METRICS_SLL_CIPHER_BUFFS_SCALE_FACTOR + 1;

                    chart_data->dim_ssl_ciphers = reallocz(chart_data->dim_ssl_ciphers, chart_data->ssl_cipher_size_max * sizeof(RRDDIM));
                    chart_data->num_ssl_ciphers = reallocz(chart_data->num_ssl_ciphers, chart_data->ssl_cipher_size_max * sizeof(collected_number));
                }
                
                chart_data->dim_ssl_ciphers[chart_data->ssl_cipher_size - 1] = rrddim_add(chart_data->st_ssl_cipher, 
                    p_file_info->parser_metrics->web_log->ssl_cipher_arr.ssl_ciphers[j].string, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                
                chart_data->num_ssl_ciphers[chart_data->ssl_cipher_size - 1] = p_file_info->parser_metrics->web_log->ssl_cipher_arr.ssl_ciphers[j].count;
                p_file_info->parser_metrics->web_log->ssl_cipher_arr.ssl_ciphers[j].count = 0;
            }
        }
    }
}

void web_log_chart_update(struct File_info *p_file_info, struct Chart_meta *chart_meta){
    chart_data_web_log_t *chart_data = chart_meta->chart_data_web_log;

    /* Number of lines - update chart */
    rrddim_set_by_pointer(  chart_data->st_lines, 
                            chart_data->dim_lines_total, 
                            chart_data->num_lines_total);
    rrddim_set_by_pointer(  chart_data->st_lines, 
                            chart_data->dim_lines_rate, 
                            chart_data->num_lines_rate);
    rrdset_done(chart_data->st_lines);

    /* Vhost - update chart */
    if(p_file_info->parser_config->chart_config & CHART_VHOST){
        for(int j = 0; j < chart_data->vhost_size; j++){
            rrddim_set_by_pointer(  chart_data->st_vhost, 
                                    chart_data->dim_vhosts[j], 
                                    chart_data->num_vhosts[j]);
        }
        rrdset_done(chart_data->st_vhost);
    }

    /* Port - update chart */
    if(p_file_info->parser_config->chart_config & CHART_PORT){
        for(int j = 0; j < chart_data->port_size; j++){
            rrddim_set_by_pointer(  chart_data->st_port, 
                                    chart_data->dim_ports[j], 
                                    chart_data->num_ports[j]);
        }
        rrdset_done(chart_data->st_port);
    }

    /* IP Version - update chart */
    if(p_file_info->parser_config->chart_config & CHART_IP_VERSION){
        rrddim_set_by_pointer(  chart_data->st_ip_ver, 
                                chart_data->dim_ip_ver_4, 
                                chart_data->num_ip_ver_4);
        rrddim_set_by_pointer(  chart_data->st_ip_ver, 
                                chart_data->dim_ip_ver_6, 
                                chart_data->num_ip_ver_6);
        rrddim_set_by_pointer(  chart_data->st_ip_ver, 
                                chart_data->dim_ip_ver_invalid, 
                                chart_data->num_ip_ver_invalid);
        rrdset_done(chart_data->st_ip_ver);
    }

    /* Request client current poll - update chart */
    if(p_file_info->parser_config->chart_config & CHART_REQ_CLIENT_CURRENT){
        rrddim_set_by_pointer(  chart_data->st_req_client_current, 
                                chart_data->dim_req_client_current_ipv4, 
                                chart_data->num_req_client_current_ipv4);
        rrddim_set_by_pointer(  chart_data->st_req_client_current, 
                                chart_data->dim_req_client_current_ipv6, 
                                chart_data->num_req_client_current_ipv6);
        rrdset_done(chart_data->st_req_client_current);
    }

    /* Request client all-time - update chart */
    if(p_file_info->parser_config->chart_config & CHART_REQ_CLIENT_ALL_TIME){
        rrddim_set_by_pointer(  chart_data->st_req_client_all_time, 
                                chart_data->dim_req_client_all_time_ipv4, 
                                chart_data->num_req_client_all_time_ipv4);
        rrddim_set_by_pointer(  chart_data->st_req_client_all_time, 
                                chart_data->dim_req_client_all_time_ipv6, 
                                chart_data->num_req_client_all_time_ipv6);
        rrdset_done(chart_data->st_req_client_all_time);
    }

    /* Request methods - update chart */
    if(p_file_info->parser_config->chart_config & CHART_REQ_METHODS){
        if(unlikely(!chart_data->dim_req_method_acl && chart_data->num_req_method_acl)) 
            chart_data->dim_req_method_acl = rrddim_add(chart_data->st_req_methods, "ACL", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_acl) rrddim_set_by_pointer(   chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_acl, 
                                                                    chart_data->num_req_method_acl);
        
        if(unlikely(!chart_data->dim_req_method_baseline_control && chart_data->num_req_method_baseline_control))
            chart_data->dim_req_method_baseline_control = rrddim_add(chart_data->st_req_methods, "BASELINE-CONTROL", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_baseline_control) rrddim_set_by_pointer(  chart_data->st_req_methods, 
                                                                                chart_data->dim_req_method_baseline_control, 
                                                                                chart_data->num_req_method_baseline_control);

        if(unlikely(!chart_data->dim_req_method_bind && chart_data->num_req_method_bind))
            chart_data->dim_req_method_bind = rrddim_add(chart_data->st_req_methods, "BIND", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_bind) rrddim_set_by_pointer(  chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_bind, 
                                                                    chart_data->num_req_method_bind);
        
        if(unlikely(!chart_data->dim_req_method_checkin && chart_data->num_req_method_checkin))
            chart_data->dim_req_method_checkin = rrddim_add(chart_data->st_req_methods, "CHECKIN", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_checkin) rrddim_set_by_pointer(   chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_checkin, 
                                                                        chart_data->num_req_method_checkin);
        
        if(unlikely(!chart_data->dim_req_method_checkout && chart_data->num_req_method_checkout))
            chart_data->dim_req_method_checkout = rrddim_add(chart_data->st_req_methods, "CHECKOUT", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_checkin) rrddim_set_by_pointer(   chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_checkout, 
                                                                        chart_data->num_req_method_checkout);

        if(unlikely(!chart_data->dim_req_method_connect && chart_data->num_req_method_connect))
            chart_data->dim_req_method_connect = rrddim_add(chart_data->st_req_methods, "CONNECT", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_connect) rrddim_set_by_pointer(   chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_connect, 
                                                                        chart_data->num_req_method_connect);

        if(unlikely(!chart_data->dim_req_method_copy && chart_data->num_req_method_copy))
            chart_data->dim_req_method_copy = rrddim_add(chart_data->st_req_methods, "COPY", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_copy) rrddim_set_by_pointer(  chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_copy, 
                                                                    chart_data->num_req_method_copy);
        
        if(unlikely(!chart_data->dim_req_method_delet && chart_data->num_req_method_delet))
            chart_data->dim_req_method_delet = rrddim_add(chart_data->st_req_methods, "DELETE", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_delet) rrddim_set_by_pointer( chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_delet, 
                                                                    chart_data->num_req_method_delet);

        // There needs to exist at least one dimension for the chart to show
        // Let's use "GET" which is the most common one.          
        rrddim_set_by_pointer( chart_data->st_req_methods, chart_data->dim_req_method_get, chart_data->num_req_method_get);
        
        if(unlikely(!chart_data->dim_req_method_head && chart_data->num_req_method_head))
            chart_data->dim_req_method_head = rrddim_add(chart_data->st_req_methods, "HEAD", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_head) rrddim_set_by_pointer(  chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_head, 
                                                                    chart_data->num_req_method_head);

        if(unlikely(!chart_data->dim_req_method_label && chart_data->num_req_method_label))
            chart_data->dim_req_method_label = rrddim_add(chart_data->st_req_methods, "LABEL", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_label) rrddim_set_by_pointer( chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_label, 
                                                                    chart_data->num_req_method_label);

        if(unlikely(!chart_data->dim_req_method_link && chart_data->num_req_method_link))
            chart_data->dim_req_method_link = rrddim_add(chart_data->st_req_methods, "LINK", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_link) rrddim_set_by_pointer(  chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_label, 
                                                                    chart_data->num_req_method_label);

        if(unlikely(!chart_data->dim_req_method_lock && chart_data->num_req_method_lock))
            chart_data->dim_req_method_lock = rrddim_add(chart_data->st_req_methods, "LOCK", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_lock) rrddim_set_by_pointer(  chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_lock, 
                                                                    chart_data->num_req_method_lock);
        
        if(unlikely(!chart_data->dim_req_method_merge && chart_data->num_req_method_merge))
            chart_data->dim_req_method_merge = rrddim_add(chart_data->st_req_methods, "MERGE", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_merge) rrddim_set_by_pointer( chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_merge, 
                                                                    chart_data->num_req_method_merge);
        
        if(unlikely(!chart_data->dim_req_method_mkactivity && chart_data->num_req_method_mkactivity))
            chart_data->dim_req_method_mkactivity = rrddim_add(chart_data->st_req_methods, "MKACTIVITY", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_mkactivity) rrddim_set_by_pointer(    chart_data->st_req_methods, 
                                                                            chart_data->dim_req_method_mkactivity, 
                                                                            chart_data->num_req_method_mkactivity);
        
        if(unlikely(!chart_data->dim_req_method_mkcalendar && chart_data->num_req_method_mkcalendar))
            chart_data->dim_req_method_mkcalendar = rrddim_add(chart_data->st_req_methods, "MKCALENDAR", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_mkcalendar) rrddim_set_by_pointer(    chart_data->st_req_methods, 
                                                                            chart_data->dim_req_method_mkcalendar, 
                                                                            chart_data->num_req_method_mkcalendar);
        
        if(unlikely(!chart_data->dim_req_method_mkcol && chart_data->num_req_method_mkcol))
            chart_data->dim_req_method_mkcol = rrddim_add(chart_data->st_req_methods, "MKCOL", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_mkcol) rrddim_set_by_pointer( chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_mkcol, 
                                                                    chart_data->num_req_method_mkcol);

        if(unlikely(!chart_data->dim_req_method_mkredirectref && chart_data->num_req_method_mkredirectref))
            chart_data->dim_req_method_mkredirectref = rrddim_add(chart_data->st_req_methods, "MKREDIRECTREF", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_mkredirectref) rrddim_set_by_pointer( chart_data->st_req_methods, 
                                                                            chart_data->dim_req_method_mkredirectref, 
                                                                            chart_data->num_req_method_mkredirectref);
        
        if(unlikely(!chart_data->dim_req_method_mkworkspace && chart_data->num_req_method_mkworkspace))
            chart_data->dim_req_method_mkworkspace = rrddim_add(chart_data->st_req_methods, "MKWORKSPACE", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_mkworkspace) rrddim_set_by_pointer(   chart_data->st_req_methods, 
                                                                            chart_data->dim_req_method_mkworkspace, 
                                                                            chart_data->num_req_method_mkworkspace);
        
        if(unlikely(!chart_data->dim_req_method_move && chart_data->num_req_method_move))
            chart_data->dim_req_method_move = rrddim_add(chart_data->st_req_methods, "MOVE", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_move) rrddim_set_by_pointer(  chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_move, 
                                                                    chart_data->num_req_method_move);
        
        if(unlikely(!chart_data->dim_req_method_options && chart_data->num_req_method_options)) 
            chart_data->dim_req_method_options = rrddim_add(chart_data->st_req_methods, "OPTIONS", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_options) rrddim_set_by_pointer(   chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_options, 
                                                                        chart_data->num_req_method_options);
        
        if(unlikely(!chart_data->dim_req_method_orderpatch && chart_data->num_req_method_orderpatch))
            chart_data->dim_req_method_orderpatch = rrddim_add(chart_data->st_req_methods, "ORDERPATCH", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_orderpatch) rrddim_set_by_pointer(    chart_data->st_req_methods, 
                                                                            chart_data->dim_req_method_orderpatch, 
                                                                            chart_data->num_req_method_orderpatch);
        
        if(unlikely(!chart_data->dim_req_method_patch && chart_data->num_req_method_patch))
            chart_data->dim_req_method_patch = rrddim_add(chart_data->st_req_methods, "PATCH", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_patch) rrddim_set_by_pointer( chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_patch, 
                                                                    chart_data->num_req_method_patch);

        if(unlikely(!chart_data->dim_req_method_post && chart_data->num_req_method_post))
            chart_data->dim_req_method_post = rrddim_add(chart_data->st_req_methods, "POST", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_post){rrddim_set_by_pointer(  chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_post, 
                                                                    chart_data->num_req_method_post);
        }

        if(unlikely(!chart_data->dim_req_method_pri && chart_data->num_req_method_pri))
            chart_data->dim_req_method_pri = rrddim_add(chart_data->st_req_methods, "PRI", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_pri) rrddim_set_by_pointer(   chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_pri, 
                                                                    chart_data->num_req_method_pri);

        if(unlikely(!chart_data->dim_req_method_propfind && chart_data->num_req_method_propfind))
            chart_data->dim_req_method_propfind = rrddim_add(chart_data->st_req_methods, "PROPFIND", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_propfind) rrddim_set_by_pointer(  chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_propfind, 
                                                                        chart_data->num_req_method_propfind);
        
        if(unlikely(!chart_data->dim_req_method_proppatch && chart_data->num_req_method_proppatch))
            chart_data->dim_req_method_proppatch = rrddim_add(chart_data->st_req_methods, "PROPPATCH", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->num_req_method_proppatch) rrddim_set_by_pointer( chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_proppatch, 
                                                                        chart_data->num_req_method_proppatch);
        
        if(unlikely(!chart_data->dim_req_method_put && chart_data->num_req_method_put))
            chart_data->dim_req_method_put = rrddim_add(chart_data->st_req_methods, "PUT", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_put) rrddim_set_by_pointer(   chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_put, 
                                                                    chart_data->num_req_method_put);
        
        if(unlikely(!chart_data->dim_req_method_rebind && chart_data->num_req_method_rebind))
            chart_data->dim_req_method_rebind = rrddim_add(chart_data->st_req_methods, "REBIND", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_rebind) rrddim_set_by_pointer(    chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_rebind, 
                                                                        chart_data->num_req_method_rebind);
        
        if(unlikely(!chart_data->dim_req_method_report && chart_data->num_req_method_report))
            chart_data->dim_req_method_report = rrddim_add(chart_data->st_req_methods, "REPORT", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_report) rrddim_set_by_pointer(    chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_report, 
                                                                        chart_data->num_req_method_report);
        
        if(unlikely(!chart_data->dim_req_method_search && chart_data->num_req_method_search))
            chart_data->dim_req_method_search = rrddim_add(chart_data->st_req_methods, "SEARCH", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_search) rrddim_set_by_pointer(    chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_search, 
                                                                        chart_data->num_req_method_search);

        if(unlikely(!chart_data->dim_req_method_trace && chart_data->num_req_method_trace))
            chart_data->dim_req_method_trace = rrddim_add(chart_data->st_req_methods, "TRACE", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_trace) rrddim_set_by_pointer( chart_data->st_req_methods, 
                                                                    chart_data->dim_req_method_trace, 
                                                                    chart_data->num_req_method_trace);

        if(unlikely(!chart_data->dim_req_method_unbind && chart_data->num_req_method_unbind))                                                                    
            chart_data->dim_req_method_unbind = rrddim_add(chart_data->st_req_methods, "UNBIND", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_unbind) rrddim_set_by_pointer(    chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_unbind, 
                                                                        chart_data->num_req_method_unbind);
        
        if(unlikely(!chart_data->dim_req_method_uncheckout && chart_data->num_req_method_uncheckout))
            chart_data->dim_req_method_uncheckout = rrddim_add(chart_data->st_req_methods, "UNCHECKOUT", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_uncheckout) rrddim_set_by_pointer(    chart_data->st_req_methods, 
                                                                            chart_data->dim_req_method_uncheckout, 
                                                                            chart_data->num_req_method_uncheckout);
        
        if(unlikely(!chart_data->dim_req_method_unlink && chart_data->num_req_method_unlink))
            chart_data->dim_req_method_unlink = rrddim_add(chart_data->st_req_methods, "UNLINK", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_unlink) rrddim_set_by_pointer(    chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_unlink, 
                                                                        chart_data->num_req_method_unlink);
        
        if(unlikely(!chart_data->dim_req_method_unlock && chart_data->num_req_method_unlock))
            chart_data->dim_req_method_unlock = rrddim_add(chart_data->st_req_methods, "UNLOCK", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_unlock) rrddim_set_by_pointer(    chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_unlock, 
                                                                        chart_data->num_req_method_unlock);
        
        if(unlikely(!chart_data->dim_req_method_update && chart_data->num_req_method_update))
            chart_data->dim_req_method_update = rrddim_add(chart_data->st_req_methods, "UPDATE", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_update) rrddim_set_by_pointer(    chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method_update, 
                                                                        chart_data->num_req_method_update);

        if(unlikely(!chart_data->dim_req_method_updateredirectref && chart_data->num_req_method_updateredirectref))
            chart_data->dim_req_method_updateredirectref = rrddim_add(chart_data->st_req_methods, "UPDATEREDIRECTREF", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        if(chart_data->dim_req_method_updateredirectref) rrddim_set_by_pointer( chart_data->st_req_methods, 
                                                                                chart_data->dim_req_method_updateredirectref, 
                                                                                chart_data->num_req_method_updateredirectref);

        rrdset_done(chart_data->st_req_methods);
    }

    /* Request protocol - update chart */
    if(p_file_info->parser_config->chart_config & CHART_REQ_PROTO){
        rrddim_set_by_pointer(  chart_data->st_req_proto, 
                                chart_data->dim_req_proto_http_1, 
                                chart_data->num_req_proto_http_1);
        rrddim_set_by_pointer(  chart_data->st_req_proto, 
                                chart_data->dim_req_proto_http_1_1, 
                                chart_data->num_req_proto_http_1_1);
        rrddim_set_by_pointer(  chart_data->st_req_proto, 
                                chart_data->dim_req_proto_http_2, 
                                chart_data->num_req_proto_http_2);
        rrddim_set_by_pointer(  chart_data->st_req_proto, 
                                chart_data->dim_req_proto_other, 
                                chart_data->num_req_proto_other);
        rrdset_done(chart_data->st_req_proto);
    }

    /* Request bandwidth - update chart */
    if(p_file_info->parser_config->chart_config & CHART_BANDWIDTH){
        rrddim_set_by_pointer(  chart_data->st_bandwidth, 
                                chart_data->dim_bandwidth_req_size, 
                                chart_data->num_bandwidth_req_size);
        rrddim_set_by_pointer(  chart_data->st_bandwidth, 
                                chart_data->dim_bandwidth_resp_size, 
                                chart_data->num_bandwidth_resp_size);
        rrdset_done(chart_data->st_bandwidth);
    }

    /* Request proc time - update chart */
    if(p_file_info->parser_config->chart_config & CHART_REQ_PROC_TIME){
        rrddim_set_by_pointer(  chart_data->st_req_proc_time, 
                                chart_data->dim_req_proc_time_min, 
                                chart_data->num_req_proc_time_min);
        rrddim_set_by_pointer(  chart_data->st_req_proc_time, 
                                chart_data->dim_req_proc_time_max, 
                                chart_data->num_req_proc_time_max);
        rrddim_set_by_pointer(  chart_data->st_req_proc_time, 
                                chart_data->dim_req_proc_time_avg, 
                                chart_data->num_req_proc_time_avg);
        rrdset_done(chart_data->st_req_proc_time);
    }

    /* Response code family - update chart */
    if(p_file_info->parser_config->chart_config & CHART_RESP_CODE_FAMILY){
        rrddim_set_by_pointer(  chart_data->st_resp_code_family, 
                                chart_data->dim_resp_code_family_1xx, 
                                chart_data->num_resp_code_family_1xx);
        rrddim_set_by_pointer(  chart_data->st_resp_code_family, 
                                chart_data->dim_resp_code_family_2xx, 
                                chart_data->num_resp_code_family_2xx);
        rrddim_set_by_pointer(  chart_data->st_resp_code_family, 
                                chart_data->dim_resp_code_family_3xx, 
                                chart_data->num_resp_code_family_3xx);
        rrddim_set_by_pointer(  chart_data->st_resp_code_family, 
                                chart_data->dim_resp_code_family_4xx, 
                                chart_data->num_resp_code_family_4xx);
        rrddim_set_by_pointer(  chart_data->st_resp_code_family, 
                                chart_data->dim_resp_code_family_5xx, 
                                chart_data->num_resp_code_family_5xx);
        rrddim_set_by_pointer(  chart_data->st_resp_code_family, 
                                chart_data->dim_resp_code_family_other, 
                                chart_data->num_resp_code_family_other);
        rrdset_done(chart_data->st_resp_code_family);
    }

    /* Response code - update chart */
    if(p_file_info->parser_config->chart_config & CHART_RESP_CODE){
        for(int j = 0; j < RESP_CODE_ARR_SIZE - 1; j++){
            if(unlikely(!chart_data->dim_resp_code[j] && chart_data->num_resp_code[j])){
                char dim_resp_code_name[4];
                snprintfz(dim_resp_code_name, 4, "%d", j + 100);
                chart_data->dim_resp_code[j] = rrddim_add(chart_data->st_resp_code, dim_resp_code_name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            if(chart_data->dim_resp_code[j]) rrddim_set_by_pointer( chart_data->st_resp_code, 
                                                                    chart_data->dim_resp_code[j], 
                                                                    chart_data->num_resp_code[j]);
        }
        rrddim_set_by_pointer(  chart_data->st_resp_code,
                                chart_data->dim_resp_code[RESP_CODE_ARR_SIZE - 1], // "other"
                                chart_data->num_resp_code[RESP_CODE_ARR_SIZE - 1]);       
        rrdset_done(chart_data->st_resp_code);
    }
    
    /* Response code family - update chart */
    if(p_file_info->parser_config->chart_config & CHART_RESP_CODE_TYPE){
        rrddim_set_by_pointer(  chart_data->st_resp_code_type, 
                                chart_data->dim_resp_code_type_success, 
                                chart_data->num_resp_code_type_success);
        rrddim_set_by_pointer(  chart_data->st_resp_code_type, 
                                chart_data->dim_resp_code_type_redirect, 
                                chart_data->num_resp_code_type_redirect);
        rrddim_set_by_pointer(  chart_data->st_resp_code_type, 
                                chart_data->dim_resp_code_type_bad, 
                                chart_data->num_resp_code_type_bad);
        rrddim_set_by_pointer(  chart_data->st_resp_code_type, 
                                chart_data->dim_resp_code_type_error, 
                                chart_data->num_resp_code_type_error);
        rrddim_set_by_pointer(  chart_data->st_resp_code_type, 
                                chart_data->dim_resp_code_type_other, 
                                chart_data->num_resp_code_type_other);
        rrdset_done(chart_data->st_resp_code_type);
    }

    /* SSL protocol - update chart */
    if(p_file_info->parser_config->chart_config & CHART_SSL_PROTO){
        rrddim_set_by_pointer(  chart_data->st_ssl_proto, 
                                chart_data->dim_ssl_proto_tlsv1, 
                                chart_data->num_ssl_proto_tlsv1);
        rrddim_set_by_pointer(  chart_data->st_ssl_proto, 
                                chart_data->dim_ssl_proto_tlsv1_1, 
                                chart_data->num_ssl_proto_tlsv1_1);
        rrddim_set_by_pointer(  chart_data->st_ssl_proto, 
                                chart_data->dim_ssl_proto_tlsv1_2, 
                                chart_data->num_ssl_proto_tlsv1_2);
        rrddim_set_by_pointer(  chart_data->st_ssl_proto, 
                                chart_data->dim_ssl_proto_tlsv1_3, 
                                chart_data->num_ssl_proto_tlsv1_3);
        rrddim_set_by_pointer(  chart_data->st_ssl_proto, 
                                chart_data->dim_ssl_proto_sslv2, 
                                chart_data->num_ssl_proto_sslv2);
        rrddim_set_by_pointer(  chart_data->st_ssl_proto, 
                                chart_data->dim_ssl_proto_sslv3, 
                                chart_data->num_ssl_proto_sslv3);
        rrddim_set_by_pointer(  chart_data->st_ssl_proto, 
                                chart_data->dim_ssl_proto_other, 
                                chart_data->num_ssl_proto_other);
        rrdset_done(chart_data->st_ssl_proto);
    }

    /* SSL cipher suite - update chart */
    if(p_file_info->parser_config->chart_config & CHART_SSL_CIPHER){
        for(int j = 0; j < chart_data->ssl_cipher_size; j++){
            rrddim_set_by_pointer(  chart_data->st_ssl_cipher, 
                                    chart_data->dim_ssl_ciphers[j], 
                                    chart_data->num_ssl_ciphers[j]);
        }
        rrdset_done(chart_data->st_ssl_cipher);
    }
}
