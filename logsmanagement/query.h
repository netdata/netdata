/** @file query.h
 *  @brief Header of query.c 
 *
 *  @author Dimitris Pantazis
 */

#ifndef QUERY_H_
#define QUERY_H_

#include <inttypes.h>
#include <stdlib.h>
#include "libnetdata/libnetdata.h"
#include "logsmanagement_conf.h"

#define QUERY_VERSION "1"

#define LOGS_QRY_KW_START_TIME  "from"
#define LOGS_QRY_KW_END_TIME    "until"
#define LOGS_QRY_KW_QUOTA       "quota"
#define LOGS_QRY_KW_FILENAME    "filename"
#define LOGS_QRY_KW_CHARTNAME   "chart_name"
#define LOGS_QRY_KW_KEYWORD     "keyword"
#define LOGS_QRY_KW_IGNORE_CASE "ignore_case"
#define LOGS_QRY_KW_SANITIZE_KW "sanitize_keyword"
#define LOGS_QRY_KW_DATA_FORMAT "data_format"
#define LOGS_QRY_KW_NEWLINE     "newline"

typedef enum logs_query_result_type {
    OK = 0,
    GENERIC_ERROR = -1,
    INVALID_REQUEST_ERROR = -2,
    NO_MATCHING_CHART_OR_FILENAME_ERROR = -3,
    NO_RESULTS_FOUND = -4,
} LOGS_QUERY_RESULT_TYPE;

LOGS_QUERY_RESULT_TYPE fetch_log_sources(BUFFER *wb);

typedef enum logs_query_data_format {
    LOGS_QUERY_DATA_FORMAT_JSON_ARRAY, // default
    LOGS_QUERY_DATA_FORMAT_NEW_LINE
} LOGS_QUERY_DATA_FORMAT;

/**
 * @brief Parameters of the query.
 * @param[in] start_timestamp Start timestamp of query in epoch milliseconds.
 * @param[in,out] end_timestamp End timestamp of query in epoch milliseconds. 
 * It returns the actual end timestamp of the query which may not match the end 
 * timestamp passed as input, in case there are more results to be retrieved 
 * that would exceed the desired max quota.
 * @param[in] chart_name Chart name of log source to be queried, as it appears 
 * on the netdata dashboard. If this is defined and not an empty string, the 
 * filename parameter is ignored.
 * @param[in] filename Full path of log source to be queried. Will only be used 
 * if the chart_name is not used.
 * @param[in] keyword The keyword to be searched. IMPORTANT! Regular expressions
 *  are supported (if sanitise_keyword is not set) but have not been tested 
 * extensively, so use with caution!
 * @param[in] ignore_case If set to any integer other than 0, the query will be 
 * case-insensitive. If not set or if set to 0, the query will be case-sensitive
 * @param[in] sanitise_keyword If set to any integer other than 0, the keyword 
 * will be sanitised before used by the regex engine (which means the keyword
 * cannot be a regular expression, as it will be taken as a literal input).
 * @param[in,out] results_buff Buffer of BUFFER type to store the results of the
 *  query in. 
 * @param[in] results_buff->size Defines the maximum quota of results to be expected. 
 * If exceeded, the query will return the results obtained so far.
 * @param[out] results_buff->len The exact size of the results matched. 
 * @param[out] results_buff->buffer String containing the results of the query.
 * @param[out] keyword_matches Number of log records that match the keyword.
 * @warning results_buff->size argument must be <= MAX_LOG_MSG_SIZE.
 */
typedef struct logs_query_params {
    uint64_t start_timestamp;
    uint64_t end_timestamp;
    size_t quota;
    char *chart_name[LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES];
    char *filename[LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES];
    char *keyword;
    int ignore_case;
    int sanitise_keyword;
    LOGS_QUERY_DATA_FORMAT data_format;
    BUFFER *results_buff;
    int keyword_matches;
} logs_query_params_t;

typedef struct logs_query_res_hdr {
    uint64_t timestamp;
    size_t text_size;
    int matches;
} logs_query_res_hdr_t;

/** 
 * @brief Primary query API. 
 * @param p_query_params See documentation of logs_query_params_t struct on how 
 * to use argument.
 * @return enum of LOGS_QUERY_RESULT_TYPE with result of query
 * @todo Cornercase if filename not found in DB? Return specific message?
 */
LOGS_QUERY_RESULT_TYPE execute_logs_manag_query(logs_query_params_t *p_query_params);

#ifdef ENABLE_LOGSMANAGEMENT_TESTS
/* Used as public only for unit testing, normally defined as static */
char *sanitise_string(char *s); 
#endif // ENABLE_LOGSMANAGEMENT_TESTS

#endif  // QUERY_H_
