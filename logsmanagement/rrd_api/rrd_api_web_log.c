// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd_api_web_log.h"

void web_log_chart_init(struct File_info *p_file_info){
    p_file_info->chart_meta->chart_data_web_log = callocz(1, sizeof (struct Chart_data_web_log));
    chart_data_web_log_t *chart_data = p_file_info->chart_meta->chart_data_web_log;
    chart_data->last_update = now_realtime_sec(); // initial value shouldn't be 0
    long chart_prio = p_file_info->chart_meta->base_prio;

    lgs_mng_do_num_of_logs_charts_init(p_file_info, chart_prio);

    /* Vhost - initialise */
    if(p_file_info->parser_config->chart_config & CHART_VHOST){
        chart_data->cs_vhosts = lgs_mng_create_chart(
            (char *) p_file_info->chartname    // type
            , "vhost"                           // id
            , "Requests by Vhost"               // title
            , "requests"                        // units
            , "vhost"                           // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );
    }

    /* Port - initialise */
    if(p_file_info->parser_config->chart_config & CHART_PORT){
        chart_data->cs_ports = lgs_mng_create_chart(
            (char *) p_file_info->chartname    // type
            , "port"                            // id
            , "Requests by Port"                // title
            , "requests"                        // units
            , "port"                            // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );
    }

    /* IP Version - initialise */
    if(p_file_info->parser_config->chart_config & CHART_IP_VERSION){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname    // type
            , "ip_version"                      // id
            , "Requests by IP version"          // title
            , "requests"                        // units
            , "ip_version"                      // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        lgs_mng_add_dim("ipv4", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("ipv6", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("invalid", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
    }

    /* Request client current poll - initialise */
    if(p_file_info->parser_config->chart_config & CHART_REQ_CLIENT_CURRENT){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname    // type
            , "clients"                         // id
            , "Current Poll Unique Client IPs"  // title
            , "unique ips"                      // units
            , "clients"                         // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        lgs_mng_add_dim("ipv4", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("ipv6", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
    }

    /* Request client all-time - initialise */
    if(p_file_info->parser_config->chart_config & CHART_REQ_CLIENT_ALL_TIME){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname    // type
            , "clients_all"                     // id
            , "All Time Unique Client IPs"      // title
            , "unique ips"                      // units
            , "clients"                         // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        lgs_mng_add_dim("ipv4", RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1);
        lgs_mng_add_dim("ipv6", RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1);
    }

    /* Request methods - initialise */
    if(p_file_info->parser_config->chart_config & CHART_REQ_METHODS){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "http_methods"                    // id
            , "Requests Per HTTP Method"        // title
            , "requests"                        // units
            , "http_methods"                    // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        for(int j = 0; j < REQ_METHOD_ARR_SIZE; j++)
            lgs_mng_add_dim(req_method_str[j], RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
    }

    /* Request protocol - initialise */
    if(p_file_info->parser_config->chart_config & CHART_REQ_PROTO){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "http_versions"                   // id
            , "Requests Per HTTP Version"       // title
            , "requests"                        // units
            , "http_versions"                   // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        lgs_mng_add_dim("1.0", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("1.1", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("2.0", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("other", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
    }

    /* Request bandwidth - initialise */
    if(p_file_info->parser_config->chart_config & CHART_BANDWIDTH){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "bandwidth"                       // id
            , "Bandwidth"                       // title
            , "kilobits"                        // units
            , "bandwidth"                       // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        lgs_mng_add_dim("received", RRD_ALGORITHM_INCREMENTAL_NAME, 8, 1000);
        lgs_mng_add_dim("sent", RRD_ALGORITHM_INCREMENTAL_NAME, -8, 1000);
    }

    /* Request processing time - initialise */
    if(p_file_info->parser_config->chart_config & CHART_REQ_PROC_TIME){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "timings"                         // id
            , "Request Processing Time"         // title
            , "milliseconds"                    // units
            , "timings"                         // family
            , NULL                              // context
            , RRDSET_TYPE_LINE_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        lgs_mng_add_dim("min", RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1000);
        lgs_mng_add_dim("max", RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1000);
        lgs_mng_add_dim("avg", RRD_ALGORITHM_ABSOLUTE_NAME, 1, 1000);
    }

    /* Response code family - initialise */
    if(p_file_info->parser_config->chart_config & CHART_RESP_CODE_FAMILY){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "responses"                       // id
            , "Response Codes"                  // title
            , "requests"                        // units
            , "responses"                       // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        lgs_mng_add_dim("1xx", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("2xx", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("3xx", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("4xx", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("5xx", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("other", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
    }   

    /* Response code - initialise */
    if(p_file_info->parser_config->chart_config & CHART_RESP_CODE){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "detailed_responses"              // id
            , "Detailed Response Codes"         // title
            , "requests"                        // units
            , "responses"                       // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        for(int idx = 0; idx < RESP_CODE_ARR_SIZE - 1; idx++){
            char dim_name[4];
            snprintfz(dim_name, 4, "%d", idx + 100);
            lgs_mng_add_dim(dim_name, RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        }
    }

    /* Response code type - initialise */
    if(p_file_info->parser_config->chart_config & CHART_RESP_CODE_TYPE){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "response_types"                  // id
            , "Response Statuses"               // title
            , "requests"                        // units
            , "responses"                       // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        lgs_mng_add_dim("success", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("redirect", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("bad", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("error", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("other", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
    }

    /* SSL protocol - initialise */
    if(p_file_info->parser_config->chart_config & CHART_SSL_PROTO){
        lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "ssl_protocol"                    // id
            , "Requests Per SSL Protocol"       // title
            , "requests"                        // units
            , "ssl_protocol"                    // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );

        lgs_mng_add_dim("TLSV1", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("TLSV1.1", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("TLSV1.2", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("TLSV1.3", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("SSLV2", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("SSLV3", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
        lgs_mng_add_dim("other", RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
    }

    /* SSL cipher suite - initialise */
    if(p_file_info->parser_config->chart_config & CHART_SSL_CIPHER){
        chart_data->cs_ssl_ciphers = lgs_mng_create_chart(
            (char *) p_file_info->chartname     // type
            , "ssl_cipher_suite"                // id
            , "Requests by SSL cipher suite"    // title
            , "requests"                        // units
            , "ssl_cipher_suite"                // family
            , NULL                              // context
            , RRDSET_TYPE_AREA_NAME             // chart_type
            , ++chart_prio                      // priority
            , p_file_info->update_every         // update_every
        );
    }

    lgs_mng_do_custom_charts_init(p_file_info);
}


void web_log_chart_update(struct File_info *p_file_info){
    chart_data_web_log_t *chart_data = p_file_info->chart_meta->chart_data_web_log;
    Web_log_metrics_t *wlm = p_file_info->parser_metrics->web_log;

    if(chart_data->last_update != p_file_info->parser_metrics->last_update){

        time_t lag_in_sec = p_file_info->parser_metrics->last_update - chart_data->last_update - 1;

        lgs_mng_do_num_of_logs_charts_update(p_file_info, lag_in_sec, chart_data);

        /* Vhost - update */
        if(p_file_info->parser_config->chart_config & CHART_VHOST){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){

                lgs_mng_update_chart_begin(p_file_info->chartname, "vhost");
                for(int idx = 0; idx < chart_data->vhost_size; idx++)
                    lgs_mng_update_chart_set(wlm->vhost_arr.vhosts[idx].name, chart_data->num_vhosts[idx]);
                lgs_mng_update_chart_end(sec);
            }

            if(wlm->vhost_arr.size > chart_data->vhost_size){
                if(wlm->vhost_arr.size >= chart_data->vhost_size_max){
                    chart_data->vhost_size_max = wlm->vhost_arr.size * VHOST_BUFFS_SCALE_FACTOR + 1;
                    chart_data->num_vhosts = reallocz(  chart_data->num_vhosts, 
                                                        chart_data->vhost_size_max * sizeof(collected_number));

                }

                for(int idx = chart_data->vhost_size; idx < wlm->vhost_arr.size; idx++){
                    chart_data->num_vhosts[idx] = 0;
                    lgs_mng_add_dim_post_init(  &chart_data->cs_vhosts, 
                                        wlm->vhost_arr.vhosts[idx].name, 
                                        RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
                }
                
                chart_data->vhost_size = wlm->vhost_arr.size;
            }

            lgs_mng_update_chart_begin(p_file_info->chartname, "vhost");
            for(int idx = 0; idx < chart_data->vhost_size; idx++){
                chart_data->num_vhosts[idx] += wlm->vhost_arr.vhosts[idx].count;
                wlm->vhost_arr.vhosts[idx].count = 0;
                lgs_mng_update_chart_set(wlm->vhost_arr.vhosts[idx].name, chart_data->num_vhosts[idx]);
            }
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update); 
        }

        /* Port - update */
        if(p_file_info->parser_config->chart_config & CHART_PORT){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){

                lgs_mng_update_chart_begin(p_file_info->chartname, "port");
                for(int idx = 0; idx < chart_data->port_size; idx++)
                    lgs_mng_update_chart_set(wlm->port_arr.ports[idx].name, chart_data->num_ports[idx]);
                lgs_mng_update_chart_end(sec);
            }

            if(wlm->port_arr.size > chart_data->port_size){
                if(wlm->port_arr.size >= chart_data->port_size_max){
                    chart_data->port_size_max = wlm->port_arr.size * PORT_BUFFS_SCALE_FACTOR + 1;
                    chart_data->num_ports = reallocz(   chart_data->num_ports, 
                                                        chart_data->port_size_max * sizeof(collected_number));
                }

                for(int idx = chart_data->port_size; idx < wlm->port_arr.size; idx++){
                    chart_data->num_ports[idx] = 0;
                    lgs_mng_add_dim_post_init(  &chart_data->cs_ports, 
                                                wlm->port_arr.ports[idx].name, 
                                                RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
                }
                
                chart_data->port_size = wlm->port_arr.size;
            }

            lgs_mng_update_chart_begin(p_file_info->chartname, "port");
            for(int idx = 0; idx < chart_data->port_size; idx++){
                chart_data->num_ports[idx] += wlm->port_arr.ports[idx].count;
                wlm->port_arr.ports[idx].count = 0;
                lgs_mng_update_chart_set(wlm->port_arr.ports[idx].name, chart_data->num_ports[idx]);
            }
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update); 
        }

        /* IP Version - update */
        if(p_file_info->parser_config->chart_config & CHART_IP_VERSION){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){

                lgs_mng_update_chart_begin(p_file_info->chartname, "ip_version");
                lgs_mng_update_chart_set("ipv4", chart_data->num_ip_ver_4);
                lgs_mng_update_chart_set("ipv6", chart_data->num_ip_ver_6);
                lgs_mng_update_chart_set("invalid", chart_data->num_ip_ver_invalid);
                lgs_mng_update_chart_end(sec);
            }

            chart_data->num_ip_ver_4 += wlm->ip_ver.v4;
            chart_data->num_ip_ver_6 += wlm->ip_ver.v6;
            chart_data->num_ip_ver_invalid += wlm->ip_ver.invalid;
            memset(&wlm->ip_ver, 0, sizeof(wlm->ip_ver));

            lgs_mng_update_chart_begin(p_file_info->chartname, "ip_version");
            lgs_mng_update_chart_set("ipv4", chart_data->num_ip_ver_4);
            lgs_mng_update_chart_set("ipv6", chart_data->num_ip_ver_6);
            lgs_mng_update_chart_set("invalid", chart_data->num_ip_ver_invalid);
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update); 
        }

        /* Request client current poll - update */
        if(p_file_info->parser_config->chart_config & CHART_REQ_CLIENT_CURRENT){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
                
                lgs_mng_update_chart_begin(p_file_info->chartname, "clients");
                lgs_mng_update_chart_set("ipv4", chart_data->num_req_client_current_ipv4);
                lgs_mng_update_chart_set("ipv6", chart_data->num_req_client_current_ipv6);
                lgs_mng_update_chart_end(sec);
            }

            chart_data->num_req_client_current_ipv4 += wlm->req_clients_current_arr.ipv4_size;
            wlm->req_clients_current_arr.ipv4_size = 0;
            chart_data->num_req_client_current_ipv6 += wlm->req_clients_current_arr.ipv6_size;
            wlm->req_clients_current_arr.ipv6_size = 0;

            lgs_mng_update_chart_begin(p_file_info->chartname, "clients");
            lgs_mng_update_chart_set("ipv4", chart_data->num_req_client_current_ipv4);
            lgs_mng_update_chart_set("ipv6", chart_data->num_req_client_current_ipv6);
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update); 
        }

        /* Request client all-time - update */
        if(p_file_info->parser_config->chart_config & CHART_REQ_CLIENT_ALL_TIME){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
                
                lgs_mng_update_chart_begin(p_file_info->chartname, "clients_all");
                lgs_mng_update_chart_set("ipv4", chart_data->num_req_client_all_time_ipv4);
                lgs_mng_update_chart_set("ipv6", chart_data->num_req_client_all_time_ipv6);
                lgs_mng_update_chart_end(sec);
            }

            chart_data->num_req_client_all_time_ipv4 = wlm->req_clients_alltime_arr.ipv4_size;  
            chart_data->num_req_client_all_time_ipv6 = wlm->req_clients_alltime_arr.ipv6_size;

            lgs_mng_update_chart_begin(p_file_info->chartname, "clients_all");
            lgs_mng_update_chart_set("ipv4", chart_data->num_req_client_all_time_ipv4);
            lgs_mng_update_chart_set("ipv6", chart_data->num_req_client_all_time_ipv6);
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);
        }

        /* Request methods - update */
        if(p_file_info->parser_config->chart_config & CHART_REQ_METHODS){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
                
                lgs_mng_update_chart_begin(p_file_info->chartname, "http_methods");
                for(int idx = 0; idx < REQ_METHOD_ARR_SIZE; idx++){
                    if(chart_data->num_req_method[idx])
                        lgs_mng_update_chart_set(req_method_str[idx], chart_data->num_req_method[idx]);
                }
                lgs_mng_update_chart_end(sec);
            }

            lgs_mng_update_chart_begin(p_file_info->chartname, "http_methods");
            for(int idx = 0; idx < REQ_METHOD_ARR_SIZE; idx++){
                chart_data->num_req_method[idx] += wlm->req_method[idx];
                wlm->req_method[idx] = 0;
                if(chart_data->num_req_method[idx])
                    lgs_mng_update_chart_set(req_method_str[idx], chart_data->num_req_method[idx]);
            }
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);
        }

        /* Request protocol - update */
        if(p_file_info->parser_config->chart_config & CHART_REQ_PROTO){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
                
                lgs_mng_update_chart_begin(p_file_info->chartname, "http_versions");
                lgs_mng_update_chart_set("1.0", chart_data->num_req_proto_http_1);
                lgs_mng_update_chart_set("1.1", chart_data->num_req_proto_http_1_1);
                lgs_mng_update_chart_set("2.0", chart_data->num_req_proto_http_2);
                lgs_mng_update_chart_set("other", chart_data->num_req_proto_other);
                lgs_mng_update_chart_end(sec);
            }

            chart_data->num_req_proto_http_1 += wlm->req_proto.http_1;
            chart_data->num_req_proto_http_1_1 += wlm->req_proto.http_1_1;
            chart_data->num_req_proto_http_2 += wlm->req_proto.http_2;
            chart_data->num_req_proto_other += wlm->req_proto.other;
            memset(&wlm->req_proto, 0, sizeof(wlm->req_proto));

            lgs_mng_update_chart_begin(p_file_info->chartname, "http_versions");
            lgs_mng_update_chart_set("1.0", chart_data->num_req_proto_http_1);
            lgs_mng_update_chart_set("1.1", chart_data->num_req_proto_http_1_1);
            lgs_mng_update_chart_set("2.0", chart_data->num_req_proto_http_2);
            lgs_mng_update_chart_set("other", chart_data->num_req_proto_other);
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);
        }

        /* Request bandwidth - update */
        if(p_file_info->parser_config->chart_config & CHART_BANDWIDTH){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
                
                lgs_mng_update_chart_begin(p_file_info->chartname, "bandwidth");
                lgs_mng_update_chart_set("received", chart_data->num_bandwidth_req_size);
                lgs_mng_update_chart_set("sent", chart_data->num_bandwidth_resp_size);
                lgs_mng_update_chart_end(sec);
            }

            chart_data->num_bandwidth_req_size += wlm->bandwidth.req_size;
            chart_data->num_bandwidth_resp_size += wlm->bandwidth.resp_size;
            memset(&wlm->bandwidth, 0, sizeof(wlm->bandwidth));

            lgs_mng_update_chart_begin(p_file_info->chartname, "bandwidth");
            lgs_mng_update_chart_set("received", chart_data->num_bandwidth_req_size);
            lgs_mng_update_chart_set("sent", chart_data->num_bandwidth_resp_size);
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);
        }

        /* Request proc time - update */
        if(p_file_info->parser_config->chart_config & CHART_REQ_PROC_TIME){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
                
                lgs_mng_update_chart_begin(p_file_info->chartname, "timings");
                lgs_mng_update_chart_set("min", chart_data->num_req_proc_time_min);
                lgs_mng_update_chart_set("max", chart_data->num_req_proc_time_max);
                lgs_mng_update_chart_set("avg", chart_data->num_req_proc_time_avg);
                lgs_mng_update_chart_end(sec);
            }

            chart_data->num_req_proc_time_min = wlm->req_proc_time.min;
            chart_data->num_req_proc_time_max = wlm->req_proc_time.max;
            chart_data->num_req_proc_time_avg = wlm->req_proc_time.count ? 
                wlm->req_proc_time.sum / wlm->req_proc_time.count : 0;
            memset(&wlm->req_proc_time, 0, sizeof(wlm->req_proc_time));

            lgs_mng_update_chart_begin(p_file_info->chartname, "timings");
            lgs_mng_update_chart_set("min", chart_data->num_req_proc_time_min);
            lgs_mng_update_chart_set("max", chart_data->num_req_proc_time_max);
            lgs_mng_update_chart_set("avg", chart_data->num_req_proc_time_avg);
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);
        }

        /* Response code family - update */
        if(p_file_info->parser_config->chart_config & CHART_RESP_CODE_FAMILY){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
            
                lgs_mng_update_chart_begin(p_file_info->chartname, "responses");
                lgs_mng_update_chart_set("1xx", chart_data->num_resp_code_family_1xx);
                lgs_mng_update_chart_set("2xx", chart_data->num_resp_code_family_2xx);
                lgs_mng_update_chart_set("3xx", chart_data->num_resp_code_family_3xx);
                lgs_mng_update_chart_set("4xx", chart_data->num_resp_code_family_4xx);
                lgs_mng_update_chart_set("5xx", chart_data->num_resp_code_family_5xx);
                lgs_mng_update_chart_set("other", chart_data->num_resp_code_family_other);
                lgs_mng_update_chart_end(sec);
            }

            chart_data->num_resp_code_family_1xx += wlm->resp_code_family.resp_1xx;
            chart_data->num_resp_code_family_2xx += wlm->resp_code_family.resp_2xx;
            chart_data->num_resp_code_family_3xx += wlm->resp_code_family.resp_3xx;
            chart_data->num_resp_code_family_4xx += wlm->resp_code_family.resp_4xx;
            chart_data->num_resp_code_family_5xx += wlm->resp_code_family.resp_5xx;
            chart_data->num_resp_code_family_other += wlm->resp_code_family.other;
            memset(&wlm->resp_code_family, 0, sizeof(wlm->resp_code_family));
            
            lgs_mng_update_chart_begin(p_file_info->chartname, "responses");
            lgs_mng_update_chart_set("1xx", chart_data->num_resp_code_family_1xx);
            lgs_mng_update_chart_set("2xx", chart_data->num_resp_code_family_2xx);
            lgs_mng_update_chart_set("3xx", chart_data->num_resp_code_family_3xx);
            lgs_mng_update_chart_set("4xx", chart_data->num_resp_code_family_4xx);
            lgs_mng_update_chart_set("5xx", chart_data->num_resp_code_family_5xx);
            lgs_mng_update_chart_set("other", chart_data->num_resp_code_family_other);
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);
        }

        /* Response code - update */
        if(p_file_info->parser_config->chart_config & CHART_RESP_CODE){
            char dim_name[4];

            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
                
                lgs_mng_update_chart_begin(p_file_info->chartname, "detailed_responses");
                for(int idx = 0; idx < RESP_CODE_ARR_SIZE - 1; idx++){
                    if(chart_data->num_resp_code[idx]){
                        snprintfz(dim_name, 4, "%d", idx + 100);
                        lgs_mng_update_chart_set(dim_name, chart_data->num_resp_code[idx]);
                    }
                }
                if(chart_data->num_resp_code[RESP_CODE_ARR_SIZE - 1])
                    lgs_mng_update_chart_set("other", chart_data->num_resp_code[RESP_CODE_ARR_SIZE - 1]);
                lgs_mng_update_chart_end(sec);
            }

            lgs_mng_update_chart_begin(p_file_info->chartname, "detailed_responses");
            for(int idx = 0; idx < RESP_CODE_ARR_SIZE - 1; idx++){
                chart_data->num_resp_code[idx] += wlm->resp_code[idx];
                wlm->resp_code[idx] = 0;
                if(chart_data->num_resp_code[idx]){
                    snprintfz(dim_name, 4, "%d", idx + 100);
                    lgs_mng_update_chart_set(dim_name, chart_data->num_resp_code[idx]);
                }
            }
            chart_data->num_resp_code[RESP_CODE_ARR_SIZE - 1] += wlm->resp_code[RESP_CODE_ARR_SIZE - 1];
            wlm->resp_code[RESP_CODE_ARR_SIZE - 1] = 0;
            if(chart_data->num_resp_code[RESP_CODE_ARR_SIZE - 1])
                lgs_mng_update_chart_set("other", chart_data->num_resp_code[RESP_CODE_ARR_SIZE - 1]);
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);
        }

        /* Response code type - update */
        if(p_file_info->parser_config->chart_config & CHART_RESP_CODE_TYPE){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
                
                lgs_mng_update_chart_begin(p_file_info->chartname, "response_types");
                lgs_mng_update_chart_set("success", chart_data->num_resp_code_type_success);
                lgs_mng_update_chart_set("redirect", chart_data->num_resp_code_type_redirect);
                lgs_mng_update_chart_set("bad", chart_data->num_resp_code_type_bad);
                lgs_mng_update_chart_set("error", chart_data->num_resp_code_type_error);
                lgs_mng_update_chart_set("other", chart_data->num_resp_code_type_other);
                lgs_mng_update_chart_end(sec);
            }

            chart_data->num_resp_code_type_success += wlm->resp_code_type.resp_success;
            chart_data->num_resp_code_type_redirect += wlm->resp_code_type.resp_redirect;
            chart_data->num_resp_code_type_bad += wlm->resp_code_type.resp_bad;
            chart_data->num_resp_code_type_error += wlm->resp_code_type.resp_error;
            chart_data->num_resp_code_type_other += wlm->resp_code_type.other;
            memset(&wlm->resp_code_type, 0, sizeof(wlm->resp_code_type));

            lgs_mng_update_chart_begin(p_file_info->chartname, "response_types");
            lgs_mng_update_chart_set("success", chart_data->num_resp_code_type_success);
            lgs_mng_update_chart_set("redirect", chart_data->num_resp_code_type_redirect);
            lgs_mng_update_chart_set("bad", chart_data->num_resp_code_type_bad);
            lgs_mng_update_chart_set("error", chart_data->num_resp_code_type_error);
            lgs_mng_update_chart_set("other", chart_data->num_resp_code_type_other);
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);
        }

        /* SSL protocol - update */
        if(p_file_info->parser_config->chart_config & CHART_SSL_PROTO){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
                
                lgs_mng_update_chart_begin(p_file_info->chartname, "ssl_protocol");
                lgs_mng_update_chart_set("TLSV1", chart_data->num_ssl_proto_tlsv1);
                lgs_mng_update_chart_set("TLSV1.1", chart_data->num_ssl_proto_tlsv1_1);
                lgs_mng_update_chart_set("TLSV1.2", chart_data->num_ssl_proto_tlsv1_2);
                lgs_mng_update_chart_set("TLSV1.3", chart_data->num_ssl_proto_tlsv1_3);
                lgs_mng_update_chart_set("SSLV2", chart_data->num_ssl_proto_sslv2);
                lgs_mng_update_chart_set("SSLV3", chart_data->num_ssl_proto_sslv3);
                lgs_mng_update_chart_set("other", chart_data->num_ssl_proto_other);
                lgs_mng_update_chart_end(sec);
            }

            chart_data->num_ssl_proto_tlsv1 += wlm->ssl_proto.tlsv1;
            chart_data->num_ssl_proto_tlsv1_1 += wlm->ssl_proto.tlsv1_1;
            chart_data->num_ssl_proto_tlsv1_2 += wlm->ssl_proto.tlsv1_2;
            chart_data->num_ssl_proto_tlsv1_3 += wlm->ssl_proto.tlsv1_3;
            chart_data->num_ssl_proto_sslv2 += wlm->ssl_proto.sslv2;
            chart_data->num_ssl_proto_sslv3 += wlm->ssl_proto.sslv3;
            chart_data->num_ssl_proto_other += wlm->ssl_proto.other;
            memset(&wlm->ssl_proto, 0, sizeof(wlm->ssl_proto));

            lgs_mng_update_chart_begin(p_file_info->chartname, "ssl_protocol");
            lgs_mng_update_chart_set("TLSV1", chart_data->num_ssl_proto_tlsv1);
            lgs_mng_update_chart_set("TLSV1.1", chart_data->num_ssl_proto_tlsv1_1);
            lgs_mng_update_chart_set("TLSV1.2", chart_data->num_ssl_proto_tlsv1_2);
            lgs_mng_update_chart_set("TLSV1.3", chart_data->num_ssl_proto_tlsv1_3);
            lgs_mng_update_chart_set("SSLV2", chart_data->num_ssl_proto_sslv2);
            lgs_mng_update_chart_set("SSLV3", chart_data->num_ssl_proto_sslv3);
            lgs_mng_update_chart_set("other", chart_data->num_ssl_proto_other);
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update);
        }

        /* SSL cipher suite - update */
        if(p_file_info->parser_config->chart_config & CHART_SSL_CIPHER){
            for(time_t  sec = p_file_info->parser_metrics->last_update - lag_in_sec;
                        sec < p_file_info->parser_metrics->last_update;
                        sec++){
                
                lgs_mng_update_chart_begin(p_file_info->chartname, "ssl_cipher_suite");
                for(int idx = 0; idx < chart_data->ssl_cipher_size; idx++){
                    lgs_mng_update_chart_set(   wlm->ssl_cipher_arr.ssl_ciphers[idx].name, 
                                                chart_data->num_ssl_ciphers[idx]);
                }
                lgs_mng_update_chart_end(sec);
            }

            if(wlm->ssl_cipher_arr.size > chart_data->ssl_cipher_size){
                chart_data->ssl_cipher_size = wlm->ssl_cipher_arr.size;
                chart_data->num_ssl_ciphers = reallocz( chart_data->num_ssl_ciphers, 
                                                        chart_data->ssl_cipher_size * sizeof(collected_number));

                for(int idx = chart_data->ssl_cipher_size; idx < wlm->ssl_cipher_arr.size; idx++){
                    chart_data->num_ssl_ciphers[idx] = 0;
                    lgs_mng_add_dim_post_init(  &chart_data->cs_ssl_ciphers, 
                                                wlm->ssl_cipher_arr.ssl_ciphers[idx].name, 
                                                RRD_ALGORITHM_INCREMENTAL_NAME, 1, 1);
                }

                chart_data->ssl_cipher_size = wlm->ssl_cipher_arr.size;
            }

            lgs_mng_update_chart_begin(p_file_info->chartname, "ssl_cipher_suite");
            for(int idx = 0; idx < chart_data->ssl_cipher_size; idx++){
                chart_data->num_ssl_ciphers[idx] += wlm->ssl_cipher_arr.ssl_ciphers[idx].count;
                wlm->ssl_cipher_arr.ssl_ciphers[idx].count = 0;
                lgs_mng_update_chart_set(   wlm->ssl_cipher_arr.ssl_ciphers[idx].name, 
                                            chart_data->num_ssl_ciphers[idx]);
            }
            lgs_mng_update_chart_end(p_file_info->parser_metrics->last_update); 
        }
        
        lgs_mng_do_custom_charts_update(p_file_info, lag_in_sec);

        chart_data->last_update = p_file_info->parser_metrics->last_update;
    }
}
