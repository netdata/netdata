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

#define KiB * 1024
#define MiB * 1048576
#define GiB * 1073741824

#define MAX_LOG_MSG_SIZE 50 MiB                     /**< Maximum allowable log message size (in Bytes) to be stored in message queue and DB. **/

#define MAX_CUS_CHARTS_PER_SOURCE 100               /**< Hard limit of maximum custom charts per log source **/

/* -------------------------------------------------------------------------- */
/*                                  Database                                  */
/* -------------------------------------------------------------------------- */

#define SAVE_BLOB_TO_DB_DEFAULT 8                   /**< Default interval to save buffers from RAM to disk **/
#define SAVE_BLOB_TO_DB_MIN 2                       /**< Minimum allowed interval to save buffers from RAM to disk **/
#define SAVE_BLOB_TO_DB_MAX 256                     /**< Maximum allowed interval to save buffers from RAM to disk **/

#define BLOB_MAX_FILES 10	                        /**< Maximum allowed number of BLOB files (per collection) that are used to store compressed logs. When exceeded, the olderst one will be overwritten. **/

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                              Circular Buffer                               */
/* -------------------------------------------------------------------------- */

#define CIRCULAR_BUFF_SPARE_ITEMS 2                 /**< Additional circular buffers items to give time to the db engine to save buffers to disk **/

#define CIRCULAR_BUFF_DEFAULT_MAX_SIZE (64 MiB)     /**< Default circular_buffer_max_size **/
#define CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN (1 MiB)    /**< circular_buffer_max_size read from configuration cannot be smaller than this **/
#define CIRCULAR_BUFF_MAX_SIZE_RANGE_MAX (1024 MiB) /**< circular_buffer_max_size read from configuration cannot be larger than this **/

#define CIRC_BUFF_PREP_WR_RETRY_AFTER_MS 1000       /**< If circ_buff_prepare_write() fails due to not enough space, how many millisecs to wait before retrying **/
/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                                Compression                                 */
/* -------------------------------------------------------------------------- */

#define VALIDATE_COMPRESSION 0                      /**< For testing purposes only as it slows down compression considerably. **/

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                                Tail Plugin                                 */
/* -------------------------------------------------------------------------- */

#define FS_EVENTS_REENABLE_INTERVAL 1000U           /**< Interval to wait for before attempting to re-register a certain log file, after it was not found (due to rotation or other reason). **/

/* -------------------------------------------------------------------------- */


/* -------------------------------------------------------------------------- */
/*                                  Queries                                   */
/* -------------------------------------------------------------------------- */

#define MAX_COMPOUND_QUERY_SOURCES 10               /**< Maximum allowed number of log sources that can be searched in a single query **/

/* -------------------------------------------------------------------------- */


#endif  // LOGSMANAGEMENT_CONF_H_
