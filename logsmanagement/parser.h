// SPDX-License-Identifier: GPL-3.0-or-later

/** @file parser.h
 *  @brief Header of parser.c 
 */

#ifndef PARSER_H_
#define PARSER_H_

#include <regex.h> 
#include "daemon/common.h"
#include "libnetdata/libnetdata.h"

// Forward decleration
typedef struct log_parser_metrics Log_parser_metrics_t;


/* -------------------------------------------------------------------------- */
/*                           Configuration-related                            */
/* -------------------------------------------------------------------------- */

typedef enum{

    CHART_COLLECTED_LOGS_TOTAL =    1 << 0,
    CHART_COLLECTED_LOGS_RATE =     1 << 1,

    /* FLB_WEB_LOG charts */
    CHART_VHOST =                   1 << 2,            
    CHART_PORT =                    1 << 3,             
    CHART_IP_VERSION =              1 << 4,
    CHART_REQ_CLIENT_CURRENT =      1 << 5,
    CHART_REQ_CLIENT_ALL_TIME =     1 << 6,
    CHART_REQ_METHODS =             1 << 7,
    CHART_REQ_PROTO =               1 << 8,
    CHART_BANDWIDTH =               1 << 9,
    CHART_REQ_PROC_TIME =           1 << 10,
    CHART_RESP_CODE_FAMILY =        1 << 11,
    CHART_RESP_CODE =               1 << 12,
    CHART_RESP_CODE_TYPE =          1 << 13,
    CHART_SSL_PROTO =               1 << 14,
    CHART_SSL_CIPHER =              1 << 15,

    /* FLB_SYSTEMD or FLB_SYSLOG charts */
    CHART_SYSLOG_PRIOR =            1 << 16,
    CHART_SYSLOG_SEVER =            1 << 17,
    CHART_SYSLOG_FACIL =            1 << 18,

    /* FLB_KMSG */
    CHART_KMSG_SUBSYSTEM =          1 << 19,
    CHART_KMSG_DEVICE =             1 << 20,

    /* FLB_DOCKER_EV charts */
    CHART_DOCKER_EV_TYPE =          1 << 21

} chart_type_t;

typedef struct log_parser_config{
    void *gen_config;					/**< Pointer to (optional) generic configuration, as per use case. */
    unsigned long int chart_config;		/**< Configuration of which charts to enable according to chart_type_t **/
} Log_parser_config_t;

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                        Web Log parsing and metrics                         */
/* -------------------------------------------------------------------------- */

#define VHOST_MAX_LEN 255               /**< Max vhost string length, inclding terminating \0 **/
#define PORT_MAX_LEN 6			        /**< Max port string length, inclding terminating \0 **/
#define REQ_SCHEME_MAX_LEN 6			/**< Max request scheme length, including terminating \0 **/
#define REQ_CLIENT_MAX_LEN 46           /**< https://superuser.com/questions/381022/how-many-characters-can-an-ip-address-be#comment2219013_381029 **/
#define REQ_METHOD_MAX_LEN 18           /**< Max request method length, including terminating \0 **/
#define REQ_URL_MAX_LEN 128             /**< Max request URL length, including terminating \0 **/
#define REQ_PROTO_PREF_SIZE (sizeof("HTTP/") - 1)
#define REQ_PROTO_MAX_LEN 4             /**< Max request protocol numerical part length, including terminating \0 **/
#define REQ_SIZE_MAX_LEN 11             /**< Max size of bytes received, including terminating \0 **/
#define REQ_PROC_TIME_MAX_LEN 11        /**< Max size of request processing time, including terminating \0 **/
#define REQ_RESP_CODE_MAX_LEN 4         /**< Max size of response code, including terminating \0 **/
#define REQ_RESP_SIZE_MAX_LEN 11        /**< Max size of request response size, including terminating \0 **/
#define UPS_RESP_TIME_MAX_LEN 10        /**< Max size of upstream response time, including terminating \0 **/ 
#define SSL_PROTO_MAX_LEN 8             /**< Max SSL protocol length, inclding terminating \0 **/
#define SSL_CIPHER_SUITE_MAX_LEN 256    /**< TODO: Check max len for ssl cipher suite string is indeed 256 **/

#define RESP_CODE_ARR_SIZE 501          /**< Size of resp_code array, assuming 500 valid resp codes + 1 for "other" **/

#define WEB_LOG_INVALID_HOST_STR "invalid"
#define WEB_LOG_INVALID_PORT -1
#define WEB_LOG_INVALID_CLIENT_IP_STR "inv"

/* Web log configuration */
#define ENABLE_PARSE_WEB_LOG_LINE_DEBUG 0
#define MEASURE_WEB_LOG_PARSE_EXTRACT_TIME 0
#define SKIP_WEB_LOG_TIME_PARSING 0

#define LOG_PARSER_METRICS_VHOST_BUFFS_SCALE_FACTOR 1.5
#define LOG_PARSER_METRICS_PORT_BUFFS_SCALE_FACTOR 8 // Unlike Vhosts, ports are stored as integers, so scale factor can be much bigger without significant waste of memory
#define LOG_PARSER_METRICS_SLL_CIPHER_BUFFS_SCALE_FACTOR 1.5


typedef enum{
    VHOST_WITH_PORT,  // nginx: $host:$server_port      apache: %v:%p
    VHOST, 		      // nginx: $host ($http_host)      apache: %v
    PORT,             // nginx: $server_port            apache: %p
    REQ_SCHEME,       // nginx: $scheme                 apache: -
    REQ_CLIENT,       // nginx: $remote_addr            apache: %a (%h)
    REQ,			  // nginx: $request                apache: %r
    REQ_METHOD,       // nginx: $request_method         apache: %m
    REQ_URL,          // nginx: $request_uri            apache: %U
    REQ_PROTO,        // nginx: $server_protocol        apache: %H
    REQ_SIZE,         // nginx: $request_length         apache: %I
    REQ_PROC_TIME,    // nginx: $request_time           apache: %D  
    RESP_CODE,        // nginx: $status                 apache: %s, %>s
    RESP_SIZE,        // nginx: $bytes_sent, $body_bytes_sent apache: %b, %O, %B // Should separate %b from %O ?
    UPS_RESP_TIME,    // nginx: $upstream_response_time apache: -
    SSL_PROTO,        // nginx: $ssl_protocol           apache: -
    SSL_CIPHER_SUITE, // nginx: $ssl_cipher             apache: -
    TIME,             // nginx: $time_local             apache: %t
    CUSTOM
} web_log_line_field_t;

typedef struct web_log_parser_config{
    web_log_line_field_t *fields;  
    int num_fields;             		/**< Number of strings in the fields array. **/
    char delimiter;       				/**< Delimiter that separates the fields in the log format. **/
    int verify_parsed_logs;				/**< Boolean whether to try and verify parsed log fields or not **/
} Web_log_parser_config_t;

static const char *const req_method_str[] = {
    "ACL",
    "BASELINE-CONTROL",
    "BIND",
    "CHECKIN",
    "CHECKOUT",
    "CONNECT",
    "COPY",
    "DELETE",
    "GET",
    "HEAD",
    "LABEL",
    "LINK",
    "LOCK",
    "MERGE",
    "MKACTIVITY",
    "MKCALENDAR",
    "MKCOL",
    "MKREDIRECTREF",
    "MKWORKSPACE",
    "MOVE",
    "OPTIONS",
    "ORDERPATCH",
    "PATCH",
    "POST",
    "PRI",
    "PROPFIND",
    "PROPPATCH",
    "PUT",
    "REBIND",
    "REPORT",
    "SEARCH",
    "TRACE",
    "UNBIND",
    "UNCHECKOUT",
    "UNLINK",
    "UNLOCK",
    "UPDATE",
    "UPDATEREDIRECTREF"
};

#define REQ_METHOD_ARR_SIZE (int)(sizeof(req_method_str) / sizeof(req_method_str[0]))

typedef struct web_log_metrics{
    /* Web log metrics */
    struct log_parser_metrics_vhosts_array{
        struct log_parser_metrics_vhost{
            char name[VHOST_MAX_LEN];   /**< Name of the vhost **/
            int count;					/**< Occurences of the vhost **/
        } *vhosts;
        int size;						/**< Size of vhosts array **/
        int size_max;
    } vhost_arr;
    struct log_parser_metrics_ports_array{
        struct log_parser_metrics_port{
            int port;   				/**< Number of port **/
            int count;					/**< Occurences of the port **/
        } *ports;
        int size;						/**< Size of ports array **/
        int size_max;
    } port_arr;
    struct log_parser_metrics_ip_ver{
        int v4, v6, invalid;
    } ip_ver;
    /**< req_clients_current_arr is used by parser.c to save unique client IPs 
     * extracted per circular buffer item and also in p_file_info to save unique 
     * client IPs per collection (poll) iteration of plugin_logsmanagement.c. 
     * req_clients_alltime_arr is used in p_file_info to save unique client IPs 
     * of all time (and so ipv4_size and ipv6_size can only grow and are never reset to 0). **/
    struct log_parser_metrics_req_clients_array{
        char (*ipv4_req_clients)[REQ_CLIENT_MAX_LEN];
        int ipv4_size;						   		 
        int ipv4_size_max;
        char (*ipv6_req_clients)[REQ_CLIENT_MAX_LEN];
        int ipv6_size;						   		 
        int ipv6_size_max;
    } req_clients_current_arr, req_clients_alltime_arr; 
    int req_method[REQ_METHOD_ARR_SIZE]; 
    struct log_parser_metrics_req_proto{
        int http_1, http_1_1, http_2, other;
    } req_proto;
    struct log_parser_metrics_bandwidth{
        long long int req_size, resp_size;
    } bandwidth;
    struct log_parser_metrics_req_proc_time{
        int min, max, sum, count;
    } req_proc_time;
    struct log_parser_metrics_resp_code_family{
        int resp_1xx, resp_2xx, resp_3xx, resp_4xx, resp_5xx, other; // TODO: Can there be "other"?
    } resp_code_family; 
    /**< Array counting occurences of response codes. Each item represents the 
     * respective response code by adding 100 to its index, e.g. resp_code[102] 
     * counts how many 202 codes were detected. 501st item represents "other" */  
    unsigned int resp_code[RESP_CODE_ARR_SIZE]; 
    struct log_parser_metrics_resp_code_type{ /* Note: 304 and 401 should be treated as resp_success */
        int resp_success, resp_redirect, resp_bad, resp_error, other; // TODO: Can there be "other"?
    } resp_code_type;
    struct log_parser_metrics_ssl_proto{
        int tlsv1, tlsv1_1, tlsv1_2, tlsv1_3, sslv2, sslv3, other;
    } ssl_proto;
    struct log_parser_metrics_ssl_cipher_array{
        struct log_parser_metrics_ssl_cipher{
            char string[SSL_CIPHER_SUITE_MAX_LEN];    /**< SSL cipher suite string **/
            int count;								/**< Occurences of the SSL cipher **/
        } *ssl_ciphers;
        int size;									/**< Size of SSL ciphers array **/
        int size_max;
    } ssl_cipher_arr;
    int64_t timestamp;
} Web_log_metrics_t;

Web_log_parser_config_t *read_web_log_parser_config(const char *log_format, const char delimiter);
Web_log_parser_config_t *auto_detect_web_log_parser_config(char *line, const char delimiter);
int parse_web_log_buf(  char *text, size_t text_size, 
                        Log_parser_config_t *parser_config, 
                        Web_log_metrics_t *parser_metrics);

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                       Kernel logs (kmsg) metrics                           */
/* -------------------------------------------------------------------------- */

#define SYSLOG_SEVER_ARR_SIZE 9         /**< Number of severity levels plus 1 for 'unknown' **/

typedef struct kernel_metrics_dict_item{
    RRDDIM *dim;
    int num;
} Kernel_metrics_dict_item_t;

typedef struct kernel_metrics{
    unsigned int sever[SYSLOG_SEVER_ARR_SIZE];      /**< Syslog severity, 0-7 plus 1 space for 'unknown' **/
    DICTIONARY *subsystem;
    DICTIONARY *device;
} Kernel_metrics_t;

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                        Systemd and Syslog metrics                          */
/* -------------------------------------------------------------------------- */

#define SYSLOG_FACIL_ARR_SIZE 25        /**< Number of facility levels plus 1 for 'unknown' **/
#define SYSLOG_PRIOR_ARR_SIZE 193       /**< Number of priority values plus 1 for 'unknown' **/

typedef struct systemd_metrics{
    unsigned int sever[SYSLOG_SEVER_ARR_SIZE];      /**< Syslog severity, 0-7 plus 1 space for 'unknown' **/
    unsigned int facil[SYSLOG_FACIL_ARR_SIZE];      /**< Syslog facility, 0-23 plus 1 space for 'unknown' **/
    unsigned int prior[SYSLOG_PRIOR_ARR_SIZE];      /**< Syslog priority value, 0-191 plus 1 space for 'unknown' **/
} Systemd_metrics_t;

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                          Docker Events metrics                             */
/* -------------------------------------------------------------------------- */

static const char *docker_ev_type_string[] = {
    "container", "image", "plugin", "volume", "network", "daemon", "service", "node", "secret", "config", "unknown"
};

#define NUM_OF_DOCKER_EV_TYPES ((int) (sizeof docker_ev_type_string / sizeof docker_ev_type_string[0]))

// static const char *docker_ev_action_string[] = {
// 	/* Containers actions */
// 	"attach", "commit", "copy", "create", "destroy", "detach", "die", "exec_create", "exec_detach", "exec_die", 
// 	"exec_start", "export", "health_status", "kill", "oom", "pause", "rename", "resize", "restart", "start", "stop", 
// 	"top", "unpause", "update",
// 	/* Images actions */
// 	"delete", "import", "load", "pull", "push", "save", "tag", "untag",
// 	/* Plugins actions */
// 	"enable", "disable", "install", "remove",
// 	/* Volumes actions */
// 	/*"create",*/ /*"destroy",*/ "mount", "unmount",
// 	/* Networks actions */
// 	/*"create",*/ "connect", /*"destroy",*/ "disconnect", /*"remove"*/
// 	/* Daemons actions */
// 	"reload",
// 	/* Services actions */
// 	/*"create", "remove", "update",*/
// 	/* Nodes actions */
// 	/*"create", "remove", "update",*/
// 	/* Secrets actions */
// 	/*"create", "remove", "update",*/
// 	/* Configs actions */
// 	/*"create", "remove", "update",*/
// };

// #define NUM_OF_DOCKER_EV_ACTIONS ((int) (sizeof docker_ev_action_string / sizeof docker_ev_action_string[0]))

typedef struct docker_ev_metrics{
    unsigned int ev_type[NUM_OF_DOCKER_EV_TYPES];				
} Docker_ev_metrics_t;

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                         Regex / Keyword search                             */
/* -------------------------------------------------------------------------- */

#define MAX_KEYWORD_LEN 100 /**< Max size of keyword used in keyword search, in bytes */
#define MAX_REGEX_SIZE MAX_KEYWORD_LEN + 7 /**< Max size of regular expression (used in keyword search) in bytes **/

int search_keyword(	char *src, size_t src_sz, 
                    char *dest, size_t *dest_sz, 
                    const char *keyword, regex_t *regex, 
                    const int ignore_case);

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                   Custom Charts configuration and metrics                  */
/* -------------------------------------------------------------------------- */

typedef struct log_parser_cus_config{
    char *chart_name;					/**< Chart name where the regex will be placed in **/
    char *regex_str;					/**< String representation of the regex **/
    char *regex_name;					/**< If regex is named, this is where its name is stored **/
    regex_t regex;						/**< The compiled regex **/
} Log_parser_cus_config_t;

typedef struct log_parser_cus_metrics{
    unsigned long long count;
} Log_parser_cus_metrics_t;

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                             General / Other                                */
/* -------------------------------------------------------------------------- */

struct log_parser_metrics{
    unsigned long long num_lines;
    struct timeval tv;
    union {
        Web_log_metrics_t *web_log;
        Kernel_metrics_t *kernel;
        Systemd_metrics_t *systemd;
        Docker_ev_metrics_t *docker_ev;
    };	
    Log_parser_cus_metrics_t **parser_cus; /**< Array storing custom chart metrics structs **/
} ;

#endif  // PARSER_H_
