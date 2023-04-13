/** @file file_info.h
 *  @brief Includes the File_info structure definition
 *
 *  @author Dimitris Pantazis
 */

#ifndef FILE_INFO_H_
#define FILE_INFO_H_

#include <uv.h>
#include "../database/sqlite/sqlite3.h"
#include "logsmanagement_conf.h"
#include "parser.h"

// Cool trick --> http://userpage.fu-berlin.de/~ram/pub/pub_jf47ht81Ht/c_preprocessor_applications_en
/* WARNING: DO NOT CHANGED THE ORDER OF LOG_SRC_TYPES, ONLY APPEND NEW TYPES */
#define LOG_SRC_TYPES   LST(GENERIC)LST(WEB_LOG)LST(FLB_GENERIC) \
                        LST(FLB_WEB_LOG)LST(FLB_KMSG)LST(FLB_SYSTEMD) \
                        LST(FLB_DOCKER_EV)LST(FLB_SYSLOG)LST(FLB_SERIAL)
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

// Forward declaration to break circular dependency
struct Circ_buff;
struct Circ_buff_item_ptrs;

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
    /* TODO: Struct needs refactoring, as a lot of members take up memory that
     * is not used, depending on the type of the log source. */

    /* Struct members core to any log source type */
    const char *chart_name;                         /**< Top level chart name for this log source on web dashboard **/ 
    char *filename;                                 /**< Full path of log source **/
    const char *file_basename;                      /**< Basename of log source **/
    const char *stream_guid;                        /**< Streaming input GUID **/
    enum log_src_t log_source;                      /**< Defines log source - see enum log_src_t for options **/
    enum log_src_type_t log_type;                   /**< Defines type of log source - see enum log_src_type_t for options **/
    struct Circ_buff *circ_buff;                    /**< Associated circular buffer - only one should exist per log source. **/
    int compression_accel;                          /**< LZ4 compression acceleration factor for collected logs, see also: https://github.com/lz4/lz4/blob/90d68e37093d815e7ea06b0ee3c168cccffc84b8/lib/lz4.h#L195 **/
    int update_every;                               /**< Interval (in sec) of how often to collect and update charts **/

    /* Struct members related to disk database */
    sqlite3 *db;                                    /**< SQLite3 DB connection to DB that contains metadata for this log source **/
    const char *db_dir;                             /**< Path to metadata DB and compressed log BLOBs directory **/
    const char *db_metadata;                        /**< Path to metadata DB file **/
    uv_mutex_t *db_mut;                             /**< DB access mutex **/
    uv_file blob_handles[BLOB_MAX_FILES + 1];       /**< File handles for BLOB files. Item 0 not used - just for matching 1-1 with DB ids **/
    logs_manag_db_mode_t db_mode;                   /**< DB mode. **/
    int blob_write_handle_offset;                   /**< File offset denoting HEAD of currently open database BLOB file **/
    int buff_flush_to_db_interval;                  /**< Frequency at which RAM buffers of this log source will be flushed to the database **/
    int64_t blob_max_size;                          /**< When the size of a BLOB exceeds this value, the BLOB gets rotated. **/
    int64_t blob_total_size;                        /**< This is the total disk space that all BLOBs occupy (for this log source) **/

    /* Struct members used only by log file sources using the tail_plugin */
    uv_file file_handle;                            /**< Log source file handle **/
    uint64_t filesize;                              /**< Offset of where the next log source read operation needs to start from **/
    uv_fs_event_t *fs_event_req;
    uv_timer_t *enable_file_changed_events_timer;
    uint64_t inode;                                 /**< inode of log source file **/
    int rotated;
    uint8_t force_file_changed_cb;                  /**< Boolean to indicate whether an immediate call of the file_changed_cb() function is needed **/
    int8_t access_lock;                             /**< Boolean used to forbid a new file read operation before the previous one has finished **/
    uv_fs_t read_req;
    uv_buf_t uvBuf;                                 /**< libuv buffer data type, primarily used to read the log messages into. Implemented using #buff and #buff_size. See also http://docs.libuv.org/en/v1.x/misc.html#c.uv_buf_t **/
    
    /* Struct members related to log parsing */
    uv_thread_t *log_parser_thread;                 /**< Log parsing thread. **/ 
    Log_parser_config_t *parser_config;             /**< Configuration to be user by log parser - read from logsmanagement.conf **/ 
    Log_parser_cus_config_t **parser_cus_config;    /**< Array of custom log parsing configurations **/
    Log_parser_metrics_t *parser_metrics;           /**< Extracted metrics **/
    uv_mutex_t *parser_metrics_mut;                 /**< Mutex controlling access to parser_metrics **/
    uv_mutex_t notify_parser_thread_mut;            /**< Mutex to be used alongside notify_parser_thread_cond **/
    uv_cond_t notify_parser_thread_cond;            /**< Condition variable used to implement producer/consumer log parsing mechanism **/
    int log_batches_to_be_parsed;                   /**< Number of pending log batches waiting to be parsed when notify_parser_thread_cond unblocks **/

    /* Struct members related to Fluent-Bit inputs, filters, buffers, outputs */
    int flb_input;                                  /**< Fluent-bit input interface property for this log source **/
    int flb_parser;                                 /**< Fluent-bit parser interface property for this log source **/
    int flb_lib_output;                             /**< Fluent-bit "lib" output interface property for this log source **/
    void *flb_config;                               /**< Any other Fluent-Bit configuration specific to this log source only **/
    uv_mutex_t flb_tmp_buff_mut;
    uv_timer_t flb_tmp_buff_cpy_timer;
    // TODO: The following structs need to be converted to pointers, to reduce memory consumption when not used
    Kernel_metrics_t flb_tmp_kernel_metrics;        /**< Temporarily store Kernel log metrics after each extraction in flb_write_to_buff_cb(), until they are synced to parser_metrics->kernel **/
    Systemd_metrics_t flb_tmp_systemd_metrics;      /**< Temporarily store Systemd metrics after each extraction in flb_write_to_buff_cb(), until they are synced to parser_metrics->systemd **/
    Docker_ev_metrics_t flb_tmp_docker_ev_metrics;  /**< Temporarily store Docker Events metrics after each extraction in flb_write_to_buff_cb(), until they are synced to parser_metrics->docker_ev **/
    Flb_output_config_t *flb_outputs;               /**< Linked list of Fluent Bit outputs for this log source **/

};

struct File_infos_arr {
    struct File_info **data;
    uint8_t count;                                  /**< Number of items in array **/

    /* Related to tail_plugin.c */
    /* TODO: What is the maximum number of log files we can be monitoring? 
     * Currently limited only by fs_events_reenable_list */
    uint64_t fs_events_reenable_list;               /**< Binary list indicating offset of file to attempt to reopen in File_infos_arr. Up to 64 files. **/
    uv_mutex_t fs_events_reenable_lock;             /**< Mutex for fs_events_reenable_list **/
    uv_cond_t fs_events_reenable_cond;              /**< Condition variable for fs_events_reenable_list **/
};

extern struct File_infos_arr *p_file_infos_arr;     /**< Array that contains all p_file_info structs for all log sources **/

extern volatile sig_atomic_t p_file_infos_arr_ready; /**< Variable to synchronise chart creation in logsmanagement.plugin, once the main logs management engine is ready **/

extern int g_logs_manag_update_every;               /**< Variable defining global "update every" value for logs management **/

typedef struct {
    int update_every;
    int circ_buff_spare_items;
    int circ_buff_max_size_in_mib;
    int circ_buff_drop_logs;
    int compression_acceleration;
    logs_manag_db_mode_t db_mode;
    int disk_space_limit_in_mib;
    int buff_flush_to_db_interval;
} g_logs_manag_config_t;

extern g_logs_manag_config_t g_logs_manag_config;

#endif  // FILE_INFO_H_
