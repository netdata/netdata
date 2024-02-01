// SPDX-License-Identifier: GPL-3.0-or-later

/** @file file_info.h
 *  @brief Includes the File_info structure that is the primary
 *         structure for configuring each log source.
 */

#ifndef FILE_INFO_H_
#define FILE_INFO_H_

#include <uv.h>
#include "database/sqlite/sqlite3.h"
#include "defaults.h"
#include "parser.h"

// Cool trick --> http://userpage.fu-berlin.de/~ram/pub/pub_jf47ht81Ht/c_preprocessor_applications_en
/* WARNING: DO NOT CHANGED THE ORDER OF LOG_SRC_TYPES, ONLY APPEND NEW TYPES */
#define LOG_SRC_TYPES   LST(FLB_TAIL)LST(FLB_WEB_LOG)LST(FLB_KMSG) \
                        LST(FLB_SYSTEMD)LST(FLB_DOCKER_EV)LST(FLB_SYSLOG) \
                        LST(FLB_SERIAL)LST(FLB_MQTT)
#define LST(x) x,
enum log_src_type_t {LOG_SRC_TYPES};
#undef LST
#define LST(x) #x,   
static const char * const log_src_type_t_str[] = {LOG_SRC_TYPES};
#undef LST

#define LOG_SRCS    LST(LOG_SOURCE_LOCAL)LST(LOG_SOURCE_FORWARD)
#define LST(x) x,
enum log_src_t {LOG_SRCS};
#undef LST
#define LST(x) #x,   
static const char * const log_src_t_str[] = {LOG_SRCS};
#undef LST

#include "rrd_api/rrd_api.h"

typedef enum log_src_state {
    LOG_SRC_UNINITIALIZED = 0,      /*!< config not initialized */
    LOG_SRC_READY,                  /*!< config initialized (monitoring may have started or not) */
    LOG_SRC_EXITING                 /*!< cleanup and destroy stage */
} LOG_SRC_STATE;

typedef struct flb_tail_config {
    int use_inotify;
} Flb_tail_config_t;

typedef struct flb_kmsg_config {
    char *prio_level;
} Flb_kmsg_config_t;

typedef struct flb_serial_config {
    char *bitrate;
    char *min_bytes;
    char *separator;
    char *format;
} Flb_serial_config_t;

typedef struct flb_socket_config {
    char *mode;
    char *unix_path;
    char *unix_perm;
    char *listen;
    char *port;
} Flb_socket_config_t;

typedef struct syslog_parser_config {
    char *log_format;
    Flb_socket_config_t *socket_config;
} Syslog_parser_config_t;

typedef struct flb_output_config {
    char *plugin;                                   /**< Fluent Bit output plugin name, see: https://docs.fluentbit.io/manual/pipeline/outputs **/
    int id;                                         /**< Incremental id of plugin configuration in linked list, starting from 1 **/
    struct flb_output_config_param {
        char *key;                                  /**< Key of the parameter configuration **/
        char *val;                                  /**< Value of the parameter configuration **/
        struct flb_output_config_param *next;       /**< Next output parameter configuration in the linked list of parameters **/
    } *param;
    struct flb_output_config *next;                 /**< Next output plugin configuration in the linked list of output plugins **/
} Flb_output_config_t;

struct File_info {

    /* Struct members core to any log source type */
    const char *chartname;                          /**< Top level chart name for this log source on web dashboard **/ 
    char *filename;                                 /**< Full path of log source **/
    const char *file_basename;                      /**< Basename of log source **/
    const char *stream_guid;                        /**< Streaming input GUID **/
    enum log_src_t log_source;                      /**< Defines log source origin - see enum log_src_t for options **/
    enum log_src_type_t log_type;                   /**< Defines type of log source - see enum log_src_type_t for options **/
    struct Circ_buff *circ_buff;                    /**< Associated circular buffer - only one should exist per log source. **/
    int compression_accel;                          /**< LZ4 compression acceleration factor for collected logs, see also: https://github.com/lz4/lz4/blob/90d68e37093d815e7ea06b0ee3c168cccffc84b8/lib/lz4.h#L195 **/
    int update_every;                               /**< Interval (in sec) of how often to collect and update charts **/
    int update_timeout;                             /**< Timeout to update charts after, since last update */
    int use_log_timestamp;                          /**< Use log timestamps instead of collection timestamps, if available **/
    int do_sd_journal_send;                         /**< Write to system journal - not applicable to all log source types **/
    struct Chart_meta *chart_meta;
    LOG_SRC_STATE state;                            /**< State of log source, used to sync status among threads **/

    /* Struct members related to disk database */
    sqlite3 *db;                                    /**< SQLite3 DB connection to DB that contains metadata for this log source **/
    const char *db_dir;                             /**< Path to metadata DB and compressed log BLOBs directory **/
    const char *db_metadata;                        /**< Path to metadata DB file **/
    uv_mutex_t *db_mut;                             /**< DB access mutex **/
    uv_thread_t *db_writer_thread;                  /**< Thread responsible for handling the DB writes **/
    uv_file blob_handles[BLOB_MAX_FILES + 1];       /**< File handles for BLOB files. Item 0 not used - just for matching 1-1 with DB ids **/
    logs_manag_db_mode_t db_mode;                   /**< DB mode as enum. **/
    int blob_write_handle_offset;                   /**< File offset denoting HEAD of currently open database BLOB file **/
    int buff_flush_to_db_interval;                  /**< Frequency at which RAM buffers of this log source will be flushed to the database **/
    int64_t blob_max_size;                          /**< When the size of a BLOB exceeds this value, the BLOB gets rotated. **/
    int64_t blob_total_size;                        /**< This is the total disk space that all BLOBs occupy (for this log source) **/
    int64_t db_write_duration;                      /**< Holds timing details related to duration of DB write operations **/
    int64_t db_rotate_duration;                     /**< Holds timing details related to duration of DB rorate operations **/
    sqlite3_stmt *stmt_get_log_msg_metadata_asc;    /**< SQLITE3 statement used to retrieve metadata from database during queries in ascending order **/
    sqlite3_stmt *stmt_get_log_msg_metadata_desc;   /**< SQLITE3 statement used to retrieve metadata from database during queries in descending order **/

    /* Struct members related to queries */
    struct {
        usec_t user;
        usec_t sys;
    } cpu_time_per_mib;

    /* Struct members related to log parsing */
    Log_parser_config_t *parser_config;             /**< Configuration to be user by log parser - read from logsmanagement.conf **/ 
    Log_parser_cus_config_t **parser_cus_config;    /**< Array of custom log parsing configurations **/
    Log_parser_metrics_t *parser_metrics;           /**< Extracted metrics **/

    /* Struct members related to Fluent-Bit inputs, filters, buffers, outputs */
    int flb_input;                                  /**< Fluent-bit input interface property for this log source **/
    int flb_parser;                                 /**< Fluent-bit parser interface property for this log source **/
    int flb_lib_output;                             /**< Fluent-bit "lib" output interface property for this log source **/
    void *flb_config;                               /**< Any other Fluent-Bit configuration specific to this log source only **/
    uv_mutex_t flb_tmp_buff_mut;
    uv_timer_t flb_tmp_buff_cpy_timer;
    Flb_output_config_t *flb_outputs;               /**< Linked list of Fluent Bit outputs for this log source **/

};

struct File_infos_arr {
    struct File_info **data;
    uint8_t count;                                  /**< Number of items in array **/
};

extern struct File_infos_arr *p_file_infos_arr;     /**< Array that contains all p_file_info structs for all log sources **/

typedef struct {
    int update_every;
    int update_timeout;
    int use_log_timestamp;
    int circ_buff_max_size_in_mib;
    int circ_buff_drop_logs;
    int compression_acceleration;
    logs_manag_db_mode_t db_mode;
    int disk_space_limit_in_mib;
    int buff_flush_to_db_interval;
    int enable_collected_logs_total;
    int enable_collected_logs_rate;
    char *sd_journal_field_prefix;
    int do_sd_journal_send;
} g_logs_manag_config_t;

extern g_logs_manag_config_t g_logs_manag_config;

#endif  // FILE_INFO_H_
