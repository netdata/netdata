// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd_api_web_log.h"

void web_log_chart_init(struct File_info *p_file_info){
    p_file_info->chart_meta->chart_data_web_log = callocz(1, sizeof (struct Chart_data_web_log));
    chart_data_web_log_t *chart_data = p_file_info->chart_meta->chart_data_web_log;
    chart_data->tv.tv_sec = now_realtime_sec(); // initial value shouldn't be 0
    long chart_prio = p_file_info->chart_meta->base_prio;

    /* Number of collected logs total - initialise */
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_TOTAL){
        chart_data->st_lines_total = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "collected_logs_total"
                , NULL
                , "collected_logs"
                , NULL
                , CHART_TITLE_TOTAL_COLLECTED_LOGS
                , "log records"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_AREA
        );
        chart_data->dim_lines_total = rrddim_add(chart_data->st_lines_total, "total records", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    /* Number of collected logs rate - initialise */
    if(p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_RATE){
        chart_data->st_lines_rate = rrdset_create_localhost(
                (char *) p_file_info->chart_name
                , "collected_logs_rate"
                , NULL
                , "collected_logs"
                , NULL
                , CHART_TITLE_RATE_COLLECTED_LOGS
                , "log records"
                , "logsmanagement.plugin"
                , NULL
                , ++chart_prio
                , p_file_info->update_every
                , RRDSET_TYPE_LINE
        );
        chart_data->dim_lines_rate = rrddim_add(chart_data->st_lines_rate, "records", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

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

    do_custom_charts_init();
}


void web_log_chart_update(struct File_info *p_file_info){
    chart_data_web_log_t *chart_data = p_file_info->chart_meta->chart_data_web_log;

    if(chart_data->tv.tv_sec != p_file_info->parser_metrics->tv.tv_sec){

        netdata_log_debug(D_LOGS_MANAG, "Updating: chart_data->tv.tv_sec:%ld p_file_info->parser_metrics->tv.tv_sec:%ld", chart_data->tv.tv_sec, p_file_info->parser_metrics->tv.tv_sec);

        time_t lag_in_sec = p_file_info->parser_metrics->tv.tv_sec - chart_data->tv.tv_sec - 1;

        chart_data->tv = p_file_info->parser_metrics->tv;

        struct timeval tv = {
            .tv_sec = chart_data->tv.tv_sec - lag_in_sec,
            .tv_usec = chart_data->tv.tv_usec
        };

        do_num_of_logs_charts_update(p_file_info, chart_data, tv, lag_in_sec);

        /* Vhost - update */
        if(p_file_info->parser_config->chart_config & CHART_VHOST){
            if(likely(chart_data->st_vhost->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    for(int j = 0; j < chart_data->vhost_size; j++)
                        rrddim_set_by_pointer(  chart_data->st_vhost, 
                                                chart_data->dim_vhosts[j], 
                                                chart_data->num_vhosts[j]);
                    rrdset_timed_done(chart_data->st_vhost, tv, true);
                    tv.tv_sec++;
                }
            }

            for(int j = 0; j < p_file_info->parser_metrics->web_log->vhost_arr.size; j++){
                int k;
                for(k = 0; k < chart_data->vhost_size; k++){
                    if( !strcmp( p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].name, 
                            string2str(chart_data->dim_vhosts[k]->name))){
                        chart_data->num_vhosts[k] += p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].count;
                        p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].count = 0;
                        // netdata_log_debug(D_LOGS_MANAG, "vhost found:%s", p_file_info->parser_metrics->vhost_arr.vhosts[j].name);
                        break;
                    }
                }
                if(chart_data->vhost_size == k){ // New vhost not in existing dimensions
                    chart_data->vhost_size++;

                    if(chart_data->vhost_size >= chart_data->vhost_size_max){
                        chart_data->vhost_size_max = chart_data->vhost_size * LOG_PARSER_METRICS_VHOST_BUFFS_SCALE_FACTOR + 1;

                        chart_data->dim_vhosts = reallocz(  chart_data->dim_vhosts, 
                                                            chart_data->vhost_size_max * sizeof(RRDDIM));
                        chart_data->num_vhosts = reallocz(  chart_data->num_vhosts, 
                                                            chart_data->vhost_size_max * sizeof(collected_number));
                    }

                    // netdata_log_debug(D_LOGS_MANAG, "New vhost:%s", p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].name);
                    
                    chart_data->dim_vhosts[chart_data->vhost_size - 1] = rrddim_add(chart_data->st_vhost, 
                                                                                    p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].name, 
                                                                                    NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    
                    chart_data->num_vhosts[chart_data->vhost_size - 1] = p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].count;
                    p_file_info->parser_metrics->web_log->vhost_arr.vhosts[j].count = 0;
                }
            }

            for(int j = 0; j < chart_data->vhost_size; j++)
                rrddim_set_by_pointer(  chart_data->st_vhost, 
                                        chart_data->dim_vhosts[j], 
                                        chart_data->num_vhosts[j]);
            rrdset_timed_done(  chart_data->st_vhost, 
                                chart_data->tv, 
                                chart_data->st_vhost->counter_done != 0);
        }

        /* Port - update */
        if(p_file_info->parser_config->chart_config & CHART_PORT){
            if(likely(chart_data->st_port->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    for(int j = 0; j < chart_data->port_size; j++){
                        rrddim_set_by_pointer(  chart_data->st_port, 
                                                chart_data->dim_ports[j], 
                                                chart_data->num_ports[j]);
                    }
                    rrdset_timed_done(chart_data->st_port, tv, true);
                    tv.tv_sec++;
                }
            }

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

                        chart_data->ports = reallocz(       chart_data->ports, 
                                                            chart_data->port_size_max * sizeof(int));
                        chart_data->dim_ports = reallocz(   chart_data->dim_ports, 
                                                            chart_data->port_size_max * sizeof(RRDDIM));
                        chart_data->num_ports = reallocz(   chart_data->num_ports, 
                                                            chart_data->port_size_max * sizeof(collected_number));
                    }

                    chart_data->ports[chart_data->port_size - 1] = p_file_info->parser_metrics->web_log->port_arr.ports[j].port;

                    if(unlikely(chart_data->ports[chart_data->port_size - 1] == WEB_LOG_INVALID_PORT)){
                    chart_data->dim_ports[chart_data->port_size - 1] = rrddim_add(chart_data->st_port, 
                        "invalid", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    } else {
                        char port_name[PORT_MAX_LEN] = "";
                        snprintfz(port_name, PORT_MAX_LEN, "%d", chart_data->ports[chart_data->port_size - 1] % (10 * (PORT_MAX_LEN - 1)));
                        chart_data->dim_ports[chart_data->port_size - 1] = rrddim_add(chart_data->st_port, 
                            port_name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    }

                    chart_data->num_ports[chart_data->port_size - 1] = p_file_info->parser_metrics->web_log->port_arr.ports[j].count;
                    p_file_info->parser_metrics->web_log->port_arr.ports[j].count = 0;
                }
            }

            for(int j = 0; j < chart_data->port_size; j++)
                rrddim_set_by_pointer(  chart_data->st_port, 
                                        chart_data->dim_ports[j], 
                                        chart_data->num_ports[j]);
            rrdset_timed_done(  chart_data->st_port, 
                                chart_data->tv, 
                                chart_data->st_port->counter_done != 0);
        }

        if(p_file_info->parser_config->chart_config & CHART_IP_VERSION){
            if(likely(chart_data->st_ip_ver->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    rrddim_set_by_pointer(  chart_data->st_ip_ver, 
                                            chart_data->dim_ip_ver_4, 
                                            chart_data->num_ip_ver_4);
                    rrddim_set_by_pointer(  chart_data->st_ip_ver, 
                                            chart_data->dim_ip_ver_6, 
                                            chart_data->num_ip_ver_6);
                    rrddim_set_by_pointer(  chart_data->st_ip_ver, 
                                            chart_data->dim_ip_ver_invalid, 
                                            chart_data->num_ip_ver_invalid);
                    rrdset_timed_done(chart_data->st_ip_ver, tv, true);
                    tv.tv_sec++;
                }
            }

            chart_data->num_ip_ver_4 += p_file_info->parser_metrics->web_log->ip_ver.v4;
            chart_data->num_ip_ver_6 += p_file_info->parser_metrics->web_log->ip_ver.v6;
            chart_data->num_ip_ver_invalid += p_file_info->parser_metrics->web_log->ip_ver.invalid;
            memset(&p_file_info->parser_metrics->web_log->ip_ver, 0, sizeof(p_file_info->parser_metrics->web_log->ip_ver));

            rrddim_set_by_pointer(  chart_data->st_ip_ver, 
                                    chart_data->dim_ip_ver_4, 
                                    chart_data->num_ip_ver_4);
            rrddim_set_by_pointer(  chart_data->st_ip_ver, 
                                    chart_data->dim_ip_ver_6, 
                                    chart_data->num_ip_ver_6);
            rrddim_set_by_pointer(  chart_data->st_ip_ver, 
                                    chart_data->dim_ip_ver_invalid, 
                                    chart_data->num_ip_ver_invalid);
            rrdset_timed_done(  chart_data->st_ip_ver, 
                                chart_data->tv, 
                                chart_data->st_ip_ver->counter_done != 0);
        }

        /* Request client current poll - update */
        if(p_file_info->parser_config->chart_config & CHART_REQ_CLIENT_CURRENT){
            if(likely(chart_data->st_req_client_current->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    rrddim_set_by_pointer(  chart_data->st_req_client_current, 
                                            chart_data->dim_req_client_current_ipv4, 
                                            chart_data->num_req_client_current_ipv4);
                    rrddim_set_by_pointer(  chart_data->st_req_client_current, 
                                            chart_data->dim_req_client_current_ipv6, 
                                            chart_data->num_req_client_current_ipv6);
                    rrdset_timed_done(chart_data->st_req_client_current, tv, true);
                    tv.tv_sec++;
                }
            }

            chart_data->num_req_client_current_ipv4 += p_file_info->parser_metrics->web_log->req_clients_current_arr.ipv4_size;
            p_file_info->parser_metrics->web_log->req_clients_current_arr.ipv4_size = 0;  
            chart_data->num_req_client_current_ipv6 += p_file_info->parser_metrics->web_log->req_clients_current_arr.ipv6_size;
            p_file_info->parser_metrics->web_log->req_clients_current_arr.ipv6_size = 0;
            rrddim_set_by_pointer(  chart_data->st_req_client_current, 
                                    chart_data->dim_req_client_current_ipv4, 
                                    chart_data->num_req_client_current_ipv4);
            rrddim_set_by_pointer(  chart_data->st_req_client_current, 
                                    chart_data->dim_req_client_current_ipv6, 
                                    chart_data->num_req_client_current_ipv6);
            rrdset_timed_done(  chart_data->st_req_client_current, 
                                chart_data->tv, 
                                chart_data->st_req_client_current->counter_done != 0);
        }

        /* Request client all-time - update */
        if(p_file_info->parser_config->chart_config & CHART_REQ_CLIENT_ALL_TIME){
            if(likely(chart_data->st_req_client_all_time->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    rrddim_set_by_pointer(  chart_data->st_req_client_all_time, 
                                            chart_data->dim_req_client_all_time_ipv4, 
                                            chart_data->num_req_client_all_time_ipv4);
                    rrddim_set_by_pointer(  chart_data->st_req_client_all_time, 
                                            chart_data->dim_req_client_all_time_ipv6, 
                                            chart_data->num_req_client_all_time_ipv6);
                    rrdset_timed_done(chart_data->st_req_client_all_time, tv, true);
                    tv.tv_sec++;
                }
            }

            chart_data->num_req_client_all_time_ipv4 = p_file_info->parser_metrics->web_log->req_clients_alltime_arr.ipv4_size;  
            chart_data->num_req_client_all_time_ipv6 = p_file_info->parser_metrics->web_log->req_clients_alltime_arr.ipv6_size;
            rrddim_set_by_pointer(  chart_data->st_req_client_all_time, 
                                    chart_data->dim_req_client_all_time_ipv4, 
                                    chart_data->num_req_client_all_time_ipv4);
            rrddim_set_by_pointer(  chart_data->st_req_client_all_time, 
                                    chart_data->dim_req_client_all_time_ipv6, 
                                    chart_data->num_req_client_all_time_ipv6);
            rrdset_timed_done(  chart_data->st_req_client_all_time, 
                                chart_data->tv, 
                                chart_data->st_req_client_all_time->counter_done != 0);
        }

        /* Request methods - update */
        if(p_file_info->parser_config->chart_config & CHART_REQ_METHODS){
            if(likely(chart_data->st_req_methods->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    for(int j = 0; j < REQ_METHOD_ARR_SIZE; j++){
                        if(chart_data->dim_req_method[j]) 
                            rrddim_set_by_pointer(  chart_data->st_req_methods, 
                                                    chart_data->dim_req_method[j], 
                                                    chart_data->num_req_method[j]);
                    }
                    rrdset_timed_done(chart_data->st_req_methods, tv, true);
                    tv.tv_sec++;
                }
            }

            for(int j = 0; j < REQ_METHOD_ARR_SIZE; j++){
                chart_data->num_req_method[j] += p_file_info->parser_metrics->web_log->req_method[j];
                p_file_info->parser_metrics->web_log->req_method[j] = 0;

                if(unlikely(!chart_data->dim_req_method[j] && chart_data->num_req_method[j]))
                    chart_data->dim_req_method[j] = rrddim_add( chart_data->st_req_methods, 
                                                                req_method_str[j], NULL, 1, 1, 
                                                                RRD_ALGORITHM_INCREMENTAL);
                if(chart_data->dim_req_method[j]) rrddim_set_by_pointer(chart_data->st_req_methods, 
                                                                        chart_data->dim_req_method[j], 
                                                                        chart_data->num_req_method[j]);
            }
            rrdset_timed_done(  chart_data->st_req_methods, 
                                chart_data->tv, 
                                chart_data->st_req_methods->counter_done != 0);
        }

        /* Request protocol - update */
        if(p_file_info->parser_config->chart_config & CHART_REQ_PROTO){
            if(likely(chart_data->st_req_proto->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
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
                    rrdset_timed_done(chart_data->st_req_proto, tv, true);
                    tv.tv_sec++;
                }
            }

            chart_data->num_req_proto_http_1 += p_file_info->parser_metrics->web_log->req_proto.http_1;
            chart_data->num_req_proto_http_1_1 += p_file_info->parser_metrics->web_log->req_proto.http_1_1;
            chart_data->num_req_proto_http_2 += p_file_info->parser_metrics->web_log->req_proto.http_2;
            chart_data->num_req_proto_other += p_file_info->parser_metrics->web_log->req_proto.other;
            memset(&p_file_info->parser_metrics->web_log->req_proto, 0, sizeof(p_file_info->parser_metrics->web_log->req_proto));

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
            rrdset_timed_done(      chart_data->st_req_proto, chart_data->tv, 
                                    chart_data->st_req_proto->counter_done != 0);
                
        }

        /* Request bandwidth - update */
        if(p_file_info->parser_config->chart_config & CHART_BANDWIDTH){
            if(likely(chart_data->st_bandwidth->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    rrddim_set_by_pointer(  chart_data->st_bandwidth, 
                                            chart_data->dim_bandwidth_req_size, 
                                            chart_data->num_bandwidth_req_size);
                    rrddim_set_by_pointer(  chart_data->st_bandwidth, 
                                            chart_data->dim_bandwidth_resp_size, 
                                            chart_data->num_bandwidth_resp_size);
                    rrdset_timed_done(chart_data->st_bandwidth, tv, true);
                    tv.tv_sec++;
                }
            }

            chart_data->num_bandwidth_req_size += p_file_info->parser_metrics->web_log->bandwidth.req_size;
            chart_data->num_bandwidth_resp_size += p_file_info->parser_metrics->web_log->bandwidth.resp_size;
            memset(&p_file_info->parser_metrics->web_log->bandwidth, 0, sizeof(p_file_info->parser_metrics->web_log->bandwidth));

            rrddim_set_by_pointer(  chart_data->st_bandwidth, 
                                    chart_data->dim_bandwidth_req_size, 
                                    chart_data->num_bandwidth_req_size);
            rrddim_set_by_pointer(  chart_data->st_bandwidth, 
                                    chart_data->dim_bandwidth_resp_size, 
                                    chart_data->num_bandwidth_resp_size);
            rrdset_timed_done(      chart_data->st_bandwidth, chart_data->tv, 
                                    chart_data->st_bandwidth->counter_done != 0);
        }

        /* Request proc time - update */
        if(p_file_info->parser_config->chart_config & CHART_REQ_PROC_TIME){
            if(likely(chart_data->st_req_proc_time->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    rrddim_set_by_pointer(  chart_data->st_req_proc_time, 
                                            chart_data->dim_req_proc_time_min, 
                                            chart_data->num_req_proc_time_min);
                    rrddim_set_by_pointer(  chart_data->st_req_proc_time, 
                                            chart_data->dim_req_proc_time_max, 
                                            chart_data->num_req_proc_time_max);
                    rrddim_set_by_pointer(  chart_data->st_req_proc_time, 
                                            chart_data->dim_req_proc_time_avg, 
                                            chart_data->num_req_proc_time_avg);
                    rrdset_timed_done(chart_data->st_req_proc_time, tv, true);
                    tv.tv_sec++;
                }
            }

            chart_data->num_req_proc_time_min = p_file_info->parser_metrics->web_log->req_proc_time.min;
            chart_data->num_req_proc_time_max = p_file_info->parser_metrics->web_log->req_proc_time.max;
            chart_data->num_req_proc_time_avg = p_file_info->parser_metrics->web_log->req_proc_time.count ? 
                p_file_info->parser_metrics->web_log->req_proc_time.sum / p_file_info->parser_metrics->web_log->req_proc_time.count : 0;
            memset(&p_file_info->parser_metrics->web_log->req_proc_time, 0, sizeof(p_file_info->parser_metrics->web_log->req_proc_time));

            rrddim_set_by_pointer(  chart_data->st_req_proc_time, 
                                    chart_data->dim_req_proc_time_min, 
                                    chart_data->num_req_proc_time_min);
            rrddim_set_by_pointer(  chart_data->st_req_proc_time, 
                                    chart_data->dim_req_proc_time_max, 
                                    chart_data->num_req_proc_time_max);
            rrddim_set_by_pointer(  chart_data->st_req_proc_time, 
                                    chart_data->dim_req_proc_time_avg, 
                                    chart_data->num_req_proc_time_avg);
            rrdset_timed_done(      chart_data->st_req_proc_time, chart_data->tv, 
                                    chart_data->st_req_proc_time->counter_done != 0);
        }

        /* Response code family - update */
        if(p_file_info->parser_config->chart_config & CHART_RESP_CODE_FAMILY){
            if(likely(chart_data->st_resp_code_family->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
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
                    rrdset_timed_done(chart_data->st_resp_code_family, tv, true);
                    tv.tv_sec++;
                }
            }

            chart_data->num_resp_code_family_1xx += p_file_info->parser_metrics->web_log->resp_code_family.resp_1xx;
            chart_data->num_resp_code_family_2xx += p_file_info->parser_metrics->web_log->resp_code_family.resp_2xx;
            chart_data->num_resp_code_family_3xx += p_file_info->parser_metrics->web_log->resp_code_family.resp_3xx;
            chart_data->num_resp_code_family_4xx += p_file_info->parser_metrics->web_log->resp_code_family.resp_4xx;
            chart_data->num_resp_code_family_5xx += p_file_info->parser_metrics->web_log->resp_code_family.resp_5xx;
            chart_data->num_resp_code_family_other += p_file_info->parser_metrics->web_log->resp_code_family.other;
            memset(&p_file_info->parser_metrics->web_log->resp_code_family, 0, sizeof(p_file_info->parser_metrics->web_log->resp_code_family));

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
            rrdset_timed_done(  chart_data->st_resp_code_family, chart_data->tv, 
                                chart_data->st_resp_code_family->counter_done != 0);
        }

        /* Response code - update */
        if(p_file_info->parser_config->chart_config & CHART_RESP_CODE){
            if(likely(chart_data->st_resp_code->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    for(int j = 0; j < RESP_CODE_ARR_SIZE; j++){
                        if(chart_data->dim_resp_code[j]) 
                            rrddim_set_by_pointer(  chart_data->st_resp_code, 
                                                    chart_data->dim_resp_code[j], 
                                                    chart_data->num_resp_code[j]);
                    }
                    rrdset_timed_done(chart_data->st_resp_code, tv, true);
                    tv.tv_sec++;
                }
            }

            for(int j = 0; j < RESP_CODE_ARR_SIZE - 1; j++){
                chart_data->num_resp_code[j] += p_file_info->parser_metrics->web_log->resp_code[j];
                p_file_info->parser_metrics->web_log->resp_code[j] = 0;

                if(unlikely(!chart_data->dim_resp_code[j] && chart_data->num_resp_code[j])){
                    char dim_resp_code_name[4];
                    snprintfz(dim_resp_code_name, 4, "%d", j + 100);
                    chart_data->dim_resp_code[j] = rrddim_add(  chart_data->st_resp_code, 
                                                                dim_resp_code_name, NULL, 
                                                                1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                if(chart_data->dim_resp_code[j]) rrddim_set_by_pointer( chart_data->st_resp_code, 
                                                                        chart_data->dim_resp_code[j], 
                                                                        chart_data->num_resp_code[j]);
            }
            chart_data->num_resp_code[RESP_CODE_ARR_SIZE - 1] += p_file_info->parser_metrics->web_log->resp_code[RESP_CODE_ARR_SIZE - 1]; // "other"
            p_file_info->parser_metrics->web_log->resp_code[RESP_CODE_ARR_SIZE - 1] = 0;

            rrddim_set_by_pointer(  chart_data->st_resp_code,
                                    chart_data->dim_resp_code[RESP_CODE_ARR_SIZE - 1], // "other"
                                    chart_data->num_resp_code[RESP_CODE_ARR_SIZE - 1]); 
            rrdset_timed_done(  chart_data->st_resp_code, chart_data->tv, 
                                chart_data->st_resp_code->counter_done != 0);
        }

        /* Response code type - update */
        if(p_file_info->parser_config->chart_config & CHART_RESP_CODE_TYPE){
            if(likely(chart_data->st_resp_code_type->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
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
                    rrdset_timed_done(chart_data->st_resp_code_type, tv, true);
                    tv.tv_sec++;
                }
            }

            chart_data->num_resp_code_type_success += p_file_info->parser_metrics->web_log->resp_code_type.resp_success;
            chart_data->num_resp_code_type_redirect += p_file_info->parser_metrics->web_log->resp_code_type.resp_redirect;
            chart_data->num_resp_code_type_bad += p_file_info->parser_metrics->web_log->resp_code_type.resp_bad;
            chart_data->num_resp_code_type_error += p_file_info->parser_metrics->web_log->resp_code_type.resp_error;
            chart_data->num_resp_code_type_other += p_file_info->parser_metrics->web_log->resp_code_type.other;
            memset(&p_file_info->parser_metrics->web_log->resp_code_type, 0, sizeof(p_file_info->parser_metrics->web_log->resp_code_type));

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
            rrdset_timed_done(  chart_data->st_resp_code_type, chart_data->tv, 
                                chart_data->st_resp_code_type->counter_done != 0);
        }

        /* SSL protocol - update */
        if(p_file_info->parser_config->chart_config & CHART_SSL_PROTO){
            if(likely(chart_data->st_ssl_proto->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
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
                    rrdset_timed_done(chart_data->st_ssl_proto, tv, true);
                    tv.tv_sec++;
                }
            }

            chart_data->num_ssl_proto_tlsv1 += p_file_info->parser_metrics->web_log->ssl_proto.tlsv1;
            chart_data->num_ssl_proto_tlsv1_1 += p_file_info->parser_metrics->web_log->ssl_proto.tlsv1_1;
            chart_data->num_ssl_proto_tlsv1_2 += p_file_info->parser_metrics->web_log->ssl_proto.tlsv1_2;
            chart_data->num_ssl_proto_tlsv1_3 += p_file_info->parser_metrics->web_log->ssl_proto.tlsv1_3;
            chart_data->num_ssl_proto_sslv2 += p_file_info->parser_metrics->web_log->ssl_proto.sslv2;
            chart_data->num_ssl_proto_sslv3 += p_file_info->parser_metrics->web_log->ssl_proto.sslv3;
            chart_data->num_ssl_proto_other += p_file_info->parser_metrics->web_log->ssl_proto.other;
            memset(&p_file_info->parser_metrics->web_log->ssl_proto, 0, sizeof(p_file_info->parser_metrics->web_log->ssl_proto));

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
            rrdset_timed_done(  chart_data->st_ssl_proto, chart_data->tv, 
                                chart_data->st_ssl_proto->counter_done != 0);
        }

        /* SSL cipher suite - update */
        if(p_file_info->parser_config->chart_config & CHART_SSL_CIPHER){
            if(likely(chart_data->st_ssl_cipher->counter_done)){

                tv.tv_sec = chart_data->tv.tv_sec - lag_in_sec;
                
                while(tv.tv_sec < chart_data->tv.tv_sec){
                    for(int j = 0; j < chart_data->ssl_cipher_size; j++)
                        rrddim_set_by_pointer(  chart_data->st_ssl_cipher, 
                                                chart_data->dim_ssl_ciphers[j], 
                                                chart_data->num_ssl_ciphers[j]);
                    rrdset_timed_done(chart_data->st_ssl_cipher, tv, true);
                    tv.tv_sec++;
                }
            }

            for(int j = 0; j < p_file_info->parser_metrics->web_log->ssl_cipher_arr.size; j++){
                int k;
                for(k = 0; k < chart_data->ssl_cipher_size; k++){
                    if(!strcmp(p_file_info->parser_metrics->web_log->ssl_cipher_arr.ssl_ciphers[j].string, 
                            string2str(chart_data->dim_ssl_ciphers[k]->name))){
                        chart_data->num_ssl_ciphers[k] += p_file_info->parser_metrics->web_log->ssl_cipher_arr.ssl_ciphers[j].count;
                        p_file_info->parser_metrics->web_log->ssl_cipher_arr.ssl_ciphers[j].count = 0;
                        break;
                    }
                }
                if(chart_data->ssl_cipher_size == k){ // New SSL cipher suite not in existing dimensions
                    chart_data->ssl_cipher_size++;

                    if(chart_data->ssl_cipher_size >= chart_data->ssl_cipher_size_max){
                        chart_data->ssl_cipher_size_max = chart_data->ssl_cipher_size * LOG_PARSER_METRICS_SLL_CIPHER_BUFFS_SCALE_FACTOR + 1;

                        chart_data->dim_ssl_ciphers = reallocz( chart_data->dim_ssl_ciphers, 
                                                                chart_data->ssl_cipher_size_max * sizeof(RRDDIM));
                        chart_data->num_ssl_ciphers = reallocz( chart_data->num_ssl_ciphers, 
                                                                chart_data->ssl_cipher_size_max * sizeof(collected_number));
                    }
                    
                    chart_data->dim_ssl_ciphers[chart_data->ssl_cipher_size - 1] = rrddim_add(chart_data->st_ssl_cipher, 
                        p_file_info->parser_metrics->web_log->ssl_cipher_arr.ssl_ciphers[j].string, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    
                    chart_data->num_ssl_ciphers[chart_data->ssl_cipher_size - 1] = p_file_info->parser_metrics->web_log->ssl_cipher_arr.ssl_ciphers[j].count;
                    p_file_info->parser_metrics->web_log->ssl_cipher_arr.ssl_ciphers[j].count = 0;
                }
            }

            for(int j = 0; j < chart_data->ssl_cipher_size; j++){
                rrddim_set_by_pointer(  chart_data->st_ssl_cipher, 
                                        chart_data->dim_ssl_ciphers[j], 
                                        chart_data->num_ssl_ciphers[j]);
            }
            rrdset_timed_done(  chart_data->st_ssl_cipher, chart_data->tv, 
                                chart_data->st_ssl_cipher->counter_done != 0);
        }

        do_custom_charts_update();
    }
}