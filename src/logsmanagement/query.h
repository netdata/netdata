// SPDX-License-Identifier: GPL-3.0-or-later

/** @file query.h
 *  @brief Header of query.c 
 */

#ifndef QUERY_H_
#define QUERY_H_

#include <inttypes.h>
#include <stdlib.h>
#include "libnetdata/libnetdata.h"
#include "defaults.h"

#define LOGS_QRY_VERSION "1"

#define LOGS_MANAG_FUNC_PARAM_AFTER     "after"
#define LOGS_MANAG_FUNC_PARAM_BEFORE    "before"
#define LOGS_QRY_KW_QUOTA               "quota"
#define LOGS_QRY_KW_CHARTNAME           "chartname"
#define LOGS_QRY_KW_FILENAME            "filename"
#define LOGS_QRY_KW_KEYWORD             "keyword"
#define LOGS_QRY_KW_IGNORE_CASE         "ignore_case"
#define LOGS_QRY_KW_SANITIZE_KW         "sanitize_keyword"

typedef struct {
    const enum {LOGS_QRY_RES_ERR_CODE_OK = 0, 
                LOGS_QRY_RES_ERR_CODE_INV_TS_ERR,
                LOGS_QRY_RES_ERR_CODE_NOT_FOUND_ERR,
                LOGS_QRY_RES_ERR_CODE_NOT_INIT_ERR,
                LOGS_QRY_RES_ERR_CODE_SERVER_ERR,
                LOGS_QRY_RES_ERR_CODE_UNMODIFIED,
                LOGS_QRY_RES_ERR_CODE_CANCELLED,
                LOGS_QRY_RES_ERR_CODE_TIMEOUT } err_code;
    char const *const err_str;
    const int http_code;
} logs_qry_res_err_t;

static const logs_qry_res_err_t logs_qry_res_err[] = {
    { LOGS_QRY_RES_ERR_CODE_OK,             "success",                              HTTP_RESP_OK                    },
    { LOGS_QRY_RES_ERR_CODE_INV_TS_ERR,     "invalid timestamp range",              HTTP_RESP_BAD_REQUEST           },
    { LOGS_QRY_RES_ERR_CODE_NOT_FOUND_ERR,  "no results found",                     HTTP_RESP_OK                    },
    { LOGS_QRY_RES_ERR_CODE_NOT_INIT_ERR,   "logs management engine not running",   HTTP_RESP_SERVICE_UNAVAILABLE   },
    { LOGS_QRY_RES_ERR_CODE_SERVER_ERR,     "server error",                         HTTP_RESP_INTERNAL_SERVER_ERROR },
    { LOGS_QRY_RES_ERR_CODE_UNMODIFIED,     "not modified",                         HTTP_RESP_NOT_MODIFIED          },
    { LOGS_QRY_RES_ERR_CODE_CANCELLED,      "cancelled",                            HTTP_RESP_CLIENT_CLOSED_REQUEST },
    { LOGS_QRY_RES_ERR_CODE_TIMEOUT,        "query timed out",                      HTTP_RESP_OK                    }
};

const logs_qry_res_err_t *fetch_log_sources(BUFFER *wb);


/**
 * @brief Parameters of the query.
 * @param req_from_ts Requested start timestamp of query in epoch 
 * milliseconds.
 * 
 * @param req_to_ts Requested end timestamp of query in epoch milliseconds. 
 * If it doesn't match the requested start timestamp, there may be more results 
 * to be retrieved (for descending timestamp order queries).
 * 
 * @param act_from_ts Actual start timestamp of query in epoch milliseconds.
 * 
 * @param act_to_ts Actual end timestamp of query in epoch milliseconds.
 * If it doesn't match the requested end timestamp, there may be more results to
 * be retrieved (for ascending timestamp order queries).
 * 
 * @param order_by_asc Equal to 1 if req_from_ts <= req_to_ts, otherwise 0.
 * 
 * @param quota Request quota for results. When exceeded, query will 
 * return, even if there are more pending results.
 * 
 * @param stop_monotonic_ut Monotonic time in usec after which the query
 * will be timed out.
 * 
 * @param chartname Chart name of log source to be queried, as it appears 
 * on the netdata dashboard. If this is defined and not an empty string, the 
 * filename parameter is ignored.
 * 
 * @param filename Full path of log source to be queried. Will only be used 
 * if the chartname is not used.
 * 
 * @param keyword The keyword to be searched. IMPORTANT! Regular expressions
 *  are supported (if sanitize_keyword is not set) but have not been tested 
 * extensively, so use with caution!
 * 
 * @param ignore_case If set to any integer other than 0, the query will be 
 * case-insensitive. If not set or if set to 0, the query will be case-sensitive
 * 
 * @param sanitize_keyword If set to any integer other than 0, the keyword 
 * will be sanitized before used by the regex engine (which means the keyword
 * cannot be a regular expression, as it will be taken as a literal input).
 * 
 * @param results_buff Buffer of BUFFER type to store the results of the
 *  query in. 
 * 
 * @param results_buff->size Defines the maximum quota of results to be 
 * expected. If exceeded, the query will return the results obtained so far.
 * 
 * @param results_buff->len The exact size of the results matched. 
 * 
 * @param results_buff->buffer String containing the results of the query.
 * 
 * @param num_lines Number of log records that match the keyword.
 * 
 * @warning results_buff->size argument must be <= MAX_LOG_MSG_SIZE.
 */
typedef struct logs_query_params {
    msec_t req_from_ts;
    msec_t req_to_ts;
    msec_t act_from_ts;
    msec_t act_to_ts;
    int order_by_asc;
    unsigned long quota;
    bool *cancelled;
    usec_t *stop_monotonic_ut;
    char *chartname[LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES];
    char *filename[LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES];
    char *keyword;
    int ignore_case;
    int sanitize_keyword;
    BUFFER *results_buff;
    unsigned long num_lines;
} logs_query_params_t;

typedef struct logs_query_res_hdr {
    msec_t timestamp;
    size_t text_size;
    int matches;
    char log_source[20];
    char log_type[20];
    char basename[20];
    char filename[50];
    char chartname[20];
} logs_query_res_hdr_t;

/**
 * @brief Check if query should be terminated.
 * @param p_query_params See documentation of logs_query_params_t struct.
 * @return true if query should be terminated of false otherwise.
*/
bool terminate_logs_manag_query(logs_query_params_t *p_query_params);

/** 
 * @brief Primary query API. 
 * @param p_query_params See documentation of logs_query_params_t struct.
 * @return enum of LOGS_QRY_RES_ERR_CODE with result of query
 * @todo Cornercase if filename not found in DB? Return specific message?
 */
const logs_qry_res_err_t *execute_logs_manag_query(logs_query_params_t *p_query_params);

#ifdef ENABLE_LOGSMANAGEMENT_TESTS
/* Used as public only for unit testing, normally defined as static */
char *sanitise_string(char *s); 
#endif // ENABLE_LOGSMANAGEMENT_TESTS

#endif  // QUERY_H_
