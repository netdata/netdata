/** @file logsmanagement_conf.h
 *  @brief Configuration settings for the logs management engine
 *
 *  @author Dimitris Pantazis
 */

#ifndef LOGSMANAGEMENT_CONF_H_
#define LOGSMANAGEMENT_CONF_H_

/* -------------------------------------------------------------------------- */
/*                                  General                                   */
/* -------------------------------------------------------------------------- */

#define KiB * 1024ULL
#define MiB * 1048576ULL
#define GiB * 1073741824ULL

#define MAX_LOG_MSG_SIZE 50 MiB                     /**< Maximum allowable log message size (in Bytes) to be stored in message queue and DB. **/

#define MAX_CUS_CHARTS_PER_SOURCE 100               /**< Hard limit of maximum custom charts per log source **/

#define MAX_OUTPUTS_PER_SOURCE 100                  /**< Hard limit of maximum Fluent Bit outputs per log source **/

#define ENABLE_COLLECTED_LOGS_TOTAL_DEFAULT 0       /**< Default value to enable (or not) metrics of total collected log records **/
#define ENABLE_COLLECTED_LOGS_RATE_DEFAULT 1        /**< Default value to enable (or not) metrics of rate of collected log records */

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

#define GLOBAL_DB_MODE_DEFAULT_STR "none"               /**< db mode string to be used as global default in configuration **/
#define GLOBAL_DB_MODE_DEFAULT LOGS_MANAG_DB_MODE_NONE  /**< db mode to be used as global default, matching GLOBAL_DB_MODE_DEFAULT_STR **/

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

#define VALIDATE_COMPRESSION 0                          /**< For testing purposes only as it slows down compression considerably. **/
#define COMPRESSION_ACCELERATION_DEFAULT 1              /**< Global default value for compression acceleration **/

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                                Tail Plugin                                 */
/* -------------------------------------------------------------------------- */

#define FS_EVENTS_REENABLE_INTERVAL 1U                 /**< Interval (in sec) to wait for before attempting to re-register a certain log file, after it was not found (due to rotation or other reason). **/

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                         Kernel logs (kmsg) plugin                          */
/* -------------------------------------------------------------------------- */

#define KERNEL_LOGS_COLLECT_INIT_WAIT 5                 /**< Wait time (in sec) before kernel log collection starts. Required in order to skip collection and processing of pre-existing logs at Netdata boot. **/

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                                  Queries                                   */
/* -------------------------------------------------------------------------- */

#define LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES 10U       /**< Maximum allowed number of log sources that can be searched in a single query **/
#define LOGS_MANAG_QUERY_START_DEFAULT 1000UL           /**< Default start timestamp for logs management queries (in ms) **/
#define LOGS_MANAG_QUERY_END_DEFAULT 99999999999999UL   /**< Default end timestamp for logs management queries (in ms) **/
#define LOGS_MANAG_QUERY_QUOTA_DEFAULT 1048576UL        /**< Default logs management query quota (1MB) **/
#define LOGS_MANAG_QUERY_IGNORE_CASE_DEFAULT 0          /**< Boolean to indicate whether to ignore case for keyword or not **/
#define LOGS_MANAG_QUERY_SANITIZE_KEYWORD_DEFAULT 0     /**< Boolean to indicate whether to sanitize keyword or not **/

/* -------------------------------------------------------------------------- */


#endif  // LOGSMANAGEMENT_CONF_H_
