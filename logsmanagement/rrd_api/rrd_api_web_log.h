// SPDX-License-Identifier: GPL-3.0-or-later

/** @file  rrd_api_web_log.h
 *  @brief Incudes the structure and function definitions for
 *         the web log charts.
 */

#ifndef RRD_API_WEB_LOG_H_
#define RRD_API_WEB_LOG_H_

#include "daemon/common.h"

struct File_info;

typedef struct Chart_data_web_log chart_data_web_log_t;

#include "../file_info.h"
#include "../circular_buffer.h"

#include "rrd_api.h"

struct Chart_data_web_log {

    time_t last_update;

    /* Number of collected log records */
    collected_number num_lines;

    /* Vhosts */
    struct Chart_str cs_vhosts;
    collected_number *num_vhosts;
    int vhost_size, vhost_size_max; /**< Actual size and maximum allocated size of dim_vhosts, num_vhosts arrays **/ 

    /* Ports */
    struct Chart_str cs_ports;
    collected_number *num_ports;
    int port_size, port_size_max;    /**< Actual size and maximum allocated size of dim_ports, num_ports and ports arrays **/ 

    /* IP Version */
    collected_number num_ip_ver_4, num_ip_ver_6, num_ip_ver_invalid;

    /* Request client current poll */
    collected_number num_req_client_current_ipv4, num_req_client_current_ipv6;

    /* Request client all-time */
    collected_number num_req_client_all_time_ipv4, num_req_client_all_time_ipv6;

    /* Request methods */
    collected_number num_req_method[REQ_METHOD_ARR_SIZE];

    /* Request protocol */
    collected_number num_req_proto_http_1, num_req_proto_http_1_1, 
                     num_req_proto_http_2, num_req_proto_other;

    /* Request bandwidth */
    collected_number num_bandwidth_req_size, num_bandwidth_resp_size;

    /* Request processing time */
    collected_number num_req_proc_time_min, num_req_proc_time_max, num_req_proc_time_avg;

    /* Response code family */
    collected_number num_resp_code_family_1xx, num_resp_code_family_2xx, 
                     num_resp_code_family_3xx, num_resp_code_family_4xx, 
                     num_resp_code_family_5xx, num_resp_code_family_other;

    /* Response code */
    collected_number num_resp_code[RESP_CODE_ARR_SIZE];

    /* Response code type */
    collected_number num_resp_code_type_success, num_resp_code_type_redirect, 
                     num_resp_code_type_bad, num_resp_code_type_error, num_resp_code_type_other;

    /* SSL protocol */
    collected_number num_ssl_proto_tlsv1, num_ssl_proto_tlsv1_1, 
                     num_ssl_proto_tlsv1_2, num_ssl_proto_tlsv1_3, 
                     num_ssl_proto_sslv2, num_ssl_proto_sslv3, num_ssl_proto_other;

    /* SSL cipher suite */
    struct Chart_str cs_ssl_ciphers;
    collected_number *num_ssl_ciphers;
    int ssl_cipher_size;

};

void web_log_chart_init(struct File_info *p_file_info);
void web_log_chart_update(struct File_info *p_file_info);

#endif  // RRD_API_WEB_LOG_H_
