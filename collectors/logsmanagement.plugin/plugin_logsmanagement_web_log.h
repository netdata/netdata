/** @file plugins_logsmanagement_web_log.h
 *  @brief Incudes the structure and function definitions to use web log charts.
 *
 *  @author Dimitris Pantazis
 */

#ifndef PLUGIN_LOGSMANAGEMENT_WEB_LOG_H_
#define PLUGIN_LOGSMANAGEMENT_WEB_LOG_H_

#include "daemon/common.h"
#include "logsmanagement/file_info.h"
#include "logsmanagement/circular_buffer.h"

typedef struct Chart_data_web_log chart_data_web_log_t;

#include "plugin_logsmanagement.h"

struct Chart_data_web_log {

    struct timeval tv;

    /* Number of collected log records */
    RRDSET *st_lines_total, *st_lines_rate;
    RRDDIM *dim_lines_total, *dim_lines_rate;
    collected_number num_lines;

    /* Vhosts */
    RRDSET *st_vhost;
    RRDDIM **dim_vhosts;
    collected_number *num_vhosts;
    int vhost_size, vhost_size_max; /**< Actual size and maximum allocated size of dim_vhosts, num_vhosts arrays **/ 

    /* Ports */
    RRDSET *st_port;
    RRDDIM **dim_ports;
    collected_number *num_ports;
    int *ports;                      /**< Array of port numbers  **/
    int port_size, port_size_max;    /**< Actual size and maximum allocated size of dim_ports, num_ports and ports arrays **/ 

    /* IP Version */
    RRDSET *st_ip_ver;
    RRDDIM *dim_ip_ver_4, *dim_ip_ver_6, *dim_ip_ver_invalid;
    collected_number num_ip_ver_4, num_ip_ver_6, num_ip_ver_invalid;

    /* Request client current poll */
    RRDSET *st_req_client_current;
    RRDDIM *dim_req_client_current_ipv4, *dim_req_client_current_ipv6;
    collected_number num_req_client_current_ipv4, num_req_client_current_ipv6;

    /* Request client all-time */
    RRDSET *st_req_client_all_time;
    RRDDIM *dim_req_client_all_time_ipv4, *dim_req_client_all_time_ipv6;
    collected_number num_req_client_all_time_ipv4, num_req_client_all_time_ipv6;

    /* Request methods */
    RRDSET *st_req_methods;
    RRDDIM *dim_req_method[REQ_METHOD_ARR_SIZE];
    collected_number num_req_method[REQ_METHOD_ARR_SIZE];

    /* Request protocol */
    RRDSET *st_req_proto;
    RRDDIM *dim_req_proto_http_1, *dim_req_proto_http_1_1, *dim_req_proto_http_2, *dim_req_proto_other;
    collected_number num_req_proto_http_1, num_req_proto_http_1_1, num_req_proto_http_2, num_req_proto_other;

    /* Request bandwidth */
    RRDSET *st_bandwidth;
    RRDDIM *dim_bandwidth_req_size, *dim_bandwidth_resp_size;
    collected_number num_bandwidth_req_size, num_bandwidth_resp_size;

    /* Request processing time */
    RRDSET *st_req_proc_time;
    RRDDIM *dim_req_proc_time_min, *dim_req_proc_time_max, *dim_req_proc_time_avg;
    collected_number num_req_proc_time_min, num_req_proc_time_max, num_req_proc_time_avg;

    /* Response code family */
    RRDSET *st_resp_code_family;
    RRDDIM *dim_resp_code_family_1xx, *dim_resp_code_family_2xx, *dim_resp_code_family_3xx, *dim_resp_code_family_4xx, *dim_resp_code_family_5xx, *dim_resp_code_family_other;
    collected_number num_resp_code_family_1xx, num_resp_code_family_2xx, num_resp_code_family_3xx, num_resp_code_family_4xx, num_resp_code_family_5xx, num_resp_code_family_other;

    /* Response code */
    RRDSET *st_resp_code;
    RRDDIM *dim_resp_code[RESP_CODE_ARR_SIZE];
    collected_number num_resp_code[RESP_CODE_ARR_SIZE];

    /* Response code type */
    RRDSET *st_resp_code_type;
    RRDDIM *dim_resp_code_type_success, *dim_resp_code_type_redirect, *dim_resp_code_type_bad, *dim_resp_code_type_error, *dim_resp_code_type_other;
    collected_number num_resp_code_type_success, num_resp_code_type_redirect, num_resp_code_type_bad, num_resp_code_type_error, num_resp_code_type_other;

    /* SSL protocol */
    RRDSET *st_ssl_proto;
    RRDDIM *dim_ssl_proto_tlsv1, *dim_ssl_proto_tlsv1_1, *dim_ssl_proto_tlsv1_2, *dim_ssl_proto_tlsv1_3, *dim_ssl_proto_sslv2, *dim_ssl_proto_sslv3, *dim_ssl_proto_other;
    collected_number num_ssl_proto_tlsv1, num_ssl_proto_tlsv1_1, num_ssl_proto_tlsv1_2, num_ssl_proto_tlsv1_3, num_ssl_proto_sslv2, num_ssl_proto_sslv3, num_ssl_proto_other;

    /* SSL cipher suite */
    RRDSET *st_ssl_cipher;
    RRDDIM **dim_ssl_ciphers;
    collected_number *num_ssl_ciphers;
    int ssl_cipher_size, ssl_cipher_size_max; /**< Actual size and maximum allocated size of dim_ssl_ciphers, num_ssl_ciphers arrays **/ 

};

void web_log_chart_init(struct File_info *p_file_info, struct Chart_meta *chart_meta);
void web_log_chart_update(struct File_info *p_file_info, struct Chart_meta *chart_meta);

#endif  // PLUGIN_LOGSMANAGEMENT_WEB_LOG_H_
