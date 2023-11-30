// SPDX-License-Identifier: GPL-3.0-or-later

/** @file defaults.h
 *  @brief Hard-coded configuration settings for the Logs Management engine
 */

#ifndef LOGSMANAG_DEFAULTS_H_
#define LOGSMANAG_DEFAULTS_H_

/* -------------------------------------------------------------------------- */
/*                                  General                                   */
/* -------------------------------------------------------------------------- */

#define KiB * 1024ULL
#define MiB * 1048576ULL
#define GiB * 1073741824ULL

#define MAX_LOG_MSG_SIZE 50 MiB                                 /**< Maximum allowable log message size (in Bytes) to be stored in message queue and DB. **/

#define MAX_CUS_CHARTS_PER_SOURCE 100                           /**< Hard limit of maximum custom charts per log source **/

#define MAX_OUTPUTS_PER_SOURCE 100                              /**< Hard limit of maximum Fluent Bit outputs per log source **/

#define UPDATE_TIMEOUT_DEFAULT 10                               /**< Default timeout to use to update charts if they haven't been updated in the meantime. **/

#if !defined(LOGS_MANAGEMENT_STRESS_TEST)
#define ENABLE_COLLECTED_LOGS_TOTAL_DEFAULT CONFIG_BOOLEAN_NO   /**< Default value to enable (or not) metrics of total collected log records **/
#else 
#define ENABLE_COLLECTED_LOGS_TOTAL_DEFAULT CONFIG_BOOLEAN_YES  /**< Default value to enable (or not) metrics of total collected log records, if stress tests are enabled **/
#endif
#define ENABLE_COLLECTED_LOGS_RATE_DEFAULT CONFIG_BOOLEAN_YES   /**< Default value to enable (or not) metrics of rate of collected log records **/

#define SD_JOURNAL_FIELD_PREFIX "LOGS_MANAG_"                   /**< Default systemd journal field prefix for sources that log to the system journal */

#define SD_JOURNAL_SEND_DEFAULT CONFIG_BOOLEAN_NO               /**< Default value to enable (or not) submission of logs to the system journal (where applicable) **/

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                                  Database                                  */
/* -------------------------------------------------------------------------- */

typedef enum {
    LOGS_MANAG_DB_MODE_FULL = 0,
    LOGS_MANAG_DB_MODE_NONE
} logs_manag_db_mode_t;

#define SAVE_BLOB_TO_DB_DEFAULT 6                       /**< Global default configuration interval to save buffers from RAM to disk **/
#define SAVE_BLOB_TO_DB_MIN 2                           /**< Minimum allowed interval to save buffers from RAM to disk **/
#define SAVE_BLOB_TO_DB_MAX 1800                        /**< Maximum allowed interval to save buffers from RAM to disk **/

#define BLOB_MAX_FILES 10	                            /**< Maximum allowed number of BLOB files (per collection) that are used to store compressed logs. When exceeded, the olderst one will be overwritten. **/

#define DISK_SPACE_LIMIT_DEFAULT 500                    /**< Global default configuration maximum database disk space limit per log source **/

#if !defined(LOGS_MANAGEMENT_STRESS_TEST)
#define GLOBAL_DB_MODE_DEFAULT_STR "none"               /**< db mode string to be used as global default in configuration **/
#define GLOBAL_DB_MODE_DEFAULT LOGS_MANAG_DB_MODE_NONE  /**< db mode to be used as global default, matching GLOBAL_DB_MODE_DEFAULT_STR **/
#else 
#define GLOBAL_DB_MODE_DEFAULT_STR "full"               /**< db mode string to be used as global default in configuration, if stress tests are enabled **/
#define GLOBAL_DB_MODE_DEFAULT LOGS_MANAG_DB_MODE_FULL  /**< db mode to be used as global default, matching GLOBAL_DB_MODE_DEFAULT_STR, if stress tests are enabled **/
#endif

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                              Circular Buffer                               */
/* -------------------------------------------------------------------------- */

#define CIRCULAR_BUFF_SPARE_ITEMS_DEFAULT 2             /**< Additional circular buffers items to give time to the db engine to save buffers to disk **/

#define CIRCULAR_BUFF_DEFAULT_MAX_SIZE (64 MiB)         /**< Default circular_buffer_max_size **/
#define CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN (1 MiB)        /**< circular_buffer_max_size read from configuration cannot be smaller than this **/
#define CIRCULAR_BUFF_MAX_SIZE_RANGE_MAX (4 GiB)        /**< circular_buffer_max_size read from configuration cannot be larger than this **/

#define CIRCULAR_BUFF_DEFAULT_DROP_LOGS 0               /**< Global default configuration value whether to drop logs if circular buffer is full **/

#define CIRC_BUFF_PREP_WR_RETRY_AFTER_MS 1000           /**< If circ_buff_prepare_write() fails due to not enough space, how many millisecs to wait before retrying **/

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                                Compression                                 */
/* -------------------------------------------------------------------------- */

#define COMPRESSION_ACCELERATION_DEFAULT 1              /**< Global default value for compression acceleration **/

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                         Kernel logs (kmsg) plugin                          */
/* -------------------------------------------------------------------------- */

#define KERNEL_LOGS_COLLECT_INIT_WAIT 5                 /**< Wait time (in sec) before kernel log collection starts. Required in order to skip collection and processing of pre-existing logs at Netdata boot. **/

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                                Fluent Bit                                  */
/* -------------------------------------------------------------------------- */

#define FLB_FLUSH_DEFAULT "0.1"                          /**< Default Fluent Bit flush interval **/
#define FLB_HTTP_LISTEN_DEFAULT "0.0.0.0"                /**< Default Fluent Bit server listening socket **/
#define FLB_HTTP_PORT_DEFAULT "2020"                     /**< Default Fluent Bit server listening port **/
#define FLB_HTTP_SERVER_DEFAULT "false"                  /**< Default Fluent Bit server enable status **/
#define FLB_LOG_FILENAME_DEFAULT "fluentbit.log"         /**< Default Fluent Bit log filename **/
#define FLB_LOG_LEVEL_DEFAULT "info"                     /**< Default Fluent Bit log level **/
#define FLB_CORO_STACK_SIZE_DEFAULT "24576"              /**< Default Fluent Bit coro stack size - do not change this value unless there is a good reason **/

#define FLB_FORWARD_UNIX_PATH_DEFAULT ""                 /**< Default path for Forward unix socket configuration, see also https://docs.fluentbit.io/manual/pipeline/inputs/forward#configuration-parameters **/
#define FLB_FORWARD_UNIX_PERM_DEFAULT "0644"             /**< Default permissions for Forward unix socket configuration, see also https://docs.fluentbit.io/manual/pipeline/inputs/forward#configuration-parameters **/
#define FLB_FORWARD_ADDR_DEFAULT "0.0.0.0"               /**< Default listen address for Forward socket configuration, see also https://docs.fluentbit.io/manual/pipeline/inputs/forward#configuration-parameters **/
#define FLB_FORWARD_PORT_DEFAULT "24224"                 /**< Default listen port for Forward socket configuration, see also https://docs.fluentbit.io/manual/pipeline/inputs/forward#configuration-parameters **/

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                                  Queries                                   */
/* -------------------------------------------------------------------------- */

#define LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES 10U       /**< Maximum allowed number of log sources that can be searched in a single query **/
#define LOGS_MANAG_QUERY_QUOTA_DEFAULT (10 MiB)         /**< Default logs management query quota **/
#define LOGS_MANAG_QUERY_QUOTA_MAX MAX_LOG_MSG_SIZE     /**< Max logs management query quota **/
#define LOGS_MANAG_QUERY_IGNORE_CASE_DEFAULT 0          /**< Boolean to indicate whether to ignore case for keyword or not **/
#define LOGS_MANAG_QUERY_SANITIZE_KEYWORD_DEFAULT 0     /**< Boolean to indicate whether to sanitize keyword or not **/
#define LOGS_MANAG_QUERY_TIMEOUT_DEFAULT 30             /**< Default timeout of logs management queries (in secs) **/

/* -------------------------------------------------------------------------- */


#endif  // LOGSMANAG_DEFAULTS_H_
