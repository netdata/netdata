// SPDX-License-Identifier: GPL-3.0-or-later

/** @file flb_plugin.c
 *  @brief This file includes all functions that act as an API to 
 *         the Fluent Bit library.
 */

#include "flb_plugin.h"
#include <lz4.h>
#include "helper.h"
#include "defaults.h"
#include "circular_buffer.h"
#include "daemon/common.h"
#include "libnetdata/libnetdata.h"
#include "../fluent-bit/lib/msgpack-c/include/msgpack/unpack.h"
#include "../fluent-bit/lib/msgpack-c/include/msgpack/object.h"
#include "../fluent-bit/lib/monkey/include/monkey/mk_core/mk_list.h"
#include <dlfcn.h>

#ifdef HAVE_SYSTEMD
#include <systemd/sd-journal.h>
#define SD_JOURNAL_SEND_DEFAULT_FIELDS \
            "%s_LOG_SOURCE=%s"  , sd_journal_field_prefix, log_src_t_str[p_file_info->log_source], \
            "%s_LOG_TYPE=%s"    , sd_journal_field_prefix, log_src_type_t_str[p_file_info->log_type]
#endif

#define LOG_REC_KEY "msg" /**< key to represent log message field in most log sources **/
#define LOG_REC_KEY_SYSTEMD "MESSAGE" /**< key to represent log message field in systemd log source **/
#define SYSLOG_TIMESTAMP_SIZE 16
#define UNKNOWN "unknown"


/* Including "../fluent-bit/include/fluent-bit/flb_macros.h" causes issues 
 * with CI, as it requires mk_core/mk_core_info.h which is generated only 
 * after Fluent Bit has been built. We can instead just redefined a couple 
 * of macros here: */
#define FLB_FALSE  0
#define FLB_TRUE   !FLB_FALSE

/* For similar reasons, (re)define the following macros from "flb_lib.h": */
/* Lib engine status */
#define FLB_LIB_ERROR     -1
#define FLB_LIB_NONE       0
#define FLB_LIB_OK         1
#define FLB_LIB_NO_CONFIG_MAP 2

/* Following structs are the same as defined in fluent-bit/flb_lib.h and 
 * fluent-bit/flb_time.h, but need to be redefined due to use of dlsym().  */

struct flb_time {
    struct timespec tm;
};

/* Library mode context data */
struct flb_lib_ctx {
    int status;
    struct mk_event_loop *event_loop;
    struct mk_event *event_channel;
    struct flb_config *config;
};

struct flb_parser_types {
    char *key;
    int  key_len;
    int type;
};

struct flb_parser {
    /* configuration */
    int type;             /* parser type */
    char *name;           /* format name */
    char *p_regex;        /* pattern for main regular expression */
    int skip_empty;       /* skip empty regex matches */
    char *time_fmt;       /* time format */
    char *time_fmt_full;  /* original given time format */
    char *time_key;       /* field name that contains the time */
    int time_offset;      /* fixed UTC offset */
    int time_keep;        /* keep time field */
    int time_strict;      /* parse time field strictly */
    int logfmt_no_bare_keys; /* in logfmt parsers, require all keys to have values */
    char *time_frac_secs; /* time format have fractional seconds ? */
    struct flb_parser_types *types; /* type casting */
    int types_len;

    /* Field decoders */
    struct mk_list *decoders;

    /* internal */
    int time_with_year;   /* do time_fmt consider a year (%Y) ? */
    char *time_fmt_year;
    int time_with_tz;     /* do time_fmt consider a timezone ?  */
    struct flb_regex *regex;
    struct mk_list _head;
};

struct flb_lib_out_cb {
    int (*cb) (void *record, size_t size, void *data);
    void *data;
};

typedef struct flb_lib_ctx flb_ctx_t;

static flb_ctx_t *(*flb_create)(void);
static int (*flb_service_set)(flb_ctx_t *ctx, ...);
static int (*flb_start)(flb_ctx_t *ctx);
static int (*flb_stop)(flb_ctx_t *ctx);
static void (*flb_destroy)(flb_ctx_t *ctx);
static int (*flb_time_pop_from_msgpack)(struct flb_time *time, msgpack_unpacked *upk, msgpack_object **map);
static int (*flb_lib_free)(void *data);
static struct flb_parser *(*flb_parser_create)( const char *name, const char *format, const char *p_regex, int skip_empty,
                                                const char *time_fmt, const char *time_key, const char *time_offset,
                                                int time_keep, int time_strict, int logfmt_no_bare_keys, 
                                                struct flb_parser_types *types, int types_len,struct mk_list *decoders,
                                                struct flb_config *config);
static int (*flb_input)(flb_ctx_t *ctx, const char *input, void *data);
static int (*flb_input_set)(flb_ctx_t *ctx, int ffd, ...);
// static int (*flb_filter)(flb_ctx_t *ctx, const char *filter, void *data);
// static int (*flb_filter_set)(flb_ctx_t *ctx, int ffd, ...);
static int (*flb_output)(flb_ctx_t *ctx, const char *output, struct flb_lib_out_cb *cb);
static int (*flb_output_set)(flb_ctx_t *ctx, int ffd, ...);
static msgpack_unpack_return (*dl_msgpack_unpack_next)(msgpack_unpacked* result, const char* data, size_t len, size_t* off);
static void (*dl_msgpack_zone_free)(msgpack_zone* zone);
static int (*dl_msgpack_object_print_buffer)(char *buffer, size_t buffer_size, msgpack_object o);

static flb_ctx_t *ctx = NULL;
static void *flb_lib_handle = NULL;

static struct flb_lib_out_cb *fwd_input_out_cb = NULL;

static const char *sd_journal_field_prefix = SD_JOURNAL_FIELD_PREFIX;

extern netdata_mutex_t stdout_mut;

int flb_init(flb_srvc_config_t flb_srvc_config, 
            const char *const stock_config_dir, 
            const char *const new_sd_journal_field_prefix){
    int rc = 0;
    char *dl_error;

    char *flb_lib_path = strdupz_path_subpath(stock_config_dir, "/../libfluent-bit.so");
    if (unlikely(NULL == (flb_lib_handle = dlopen(flb_lib_path, RTLD_LAZY)))){
        if (NULL != (dl_error = dlerror())) 
            collector_error("dlopen() libfluent-bit.so error: %s", dl_error);
        rc = -1;
        goto do_return;
    }

    dlerror();    /* Clear any existing error */

    /* Load Fluent-Bit functions from the shared library */
    #define load_function(FUNC_NAME){                                                       \
        *(void **) (&FUNC_NAME) = dlsym(flb_lib_handle, LOGS_MANAG_STR(FUNC_NAME));         \
        if ((dl_error = dlerror()) != NULL) {                                               \
            collector_error("dlerror loading %s: %s", LOGS_MANAG_STR(FUNC_NAME), dl_error); \
            rc = -1;                                                                        \
            goto do_return;                                                                 \
        }                                                                                   \
    }

    load_function(flb_create);
    load_function(flb_service_set);
    load_function(flb_start);
    load_function(flb_stop);
    load_function(flb_destroy);
    load_function(flb_time_pop_from_msgpack);
    load_function(flb_lib_free);
    load_function(flb_parser_create);
    load_function(flb_input);
    load_function(flb_input_set);
    // load_function(flb_filter);
    // load_function(flb_filter_set);
    load_function(flb_output);
    load_function(flb_output_set);
    *(void **) (&dl_msgpack_unpack_next) = dlsym(flb_lib_handle, "msgpack_unpack_next");
    if ((dl_error = dlerror()) != NULL) {
        collector_error("dlerror loading msgpack_unpack_next: %s", dl_error);
        rc = -1;
        goto do_return;
    }
    *(void **) (&dl_msgpack_zone_free) = dlsym(flb_lib_handle, "msgpack_zone_free");
    if ((dl_error = dlerror()) != NULL) {
        collector_error("dlerror loading msgpack_zone_free: %s", dl_error);
        rc = -1;
        goto do_return;
    }
    *(void **) (&dl_msgpack_object_print_buffer) = dlsym(flb_lib_handle, "msgpack_object_print_buffer");
    if ((dl_error = dlerror()) != NULL) {
        collector_error("dlerror loading msgpack_object_print_buffer: %s", dl_error);
        rc = -1;
        goto do_return;
    }

    ctx = flb_create();
    if (unlikely(!ctx)){
        rc = -1;
        goto do_return;
    }

    /* Global service settings */
    if(unlikely(flb_service_set(ctx,
        "Flush"             , flb_srvc_config.flush,
        "HTTP_Listen"       , flb_srvc_config.http_listen,
        "HTTP_Port"         , flb_srvc_config.http_port,
        "HTTP_Server"       , flb_srvc_config.http_server,
        "Log_File"          , flb_srvc_config.log_path,
        "Log_Level"         , flb_srvc_config.log_level,
        "Coro_stack_size"   , flb_srvc_config.coro_stack_size,
        NULL) != 0 )){
            rc = -1;
            goto do_return;
        }
    
    if(new_sd_journal_field_prefix && *new_sd_journal_field_prefix)
        sd_journal_field_prefix = new_sd_journal_field_prefix;

do_return:
    freez(flb_lib_path);
    if(unlikely(rc && flb_lib_handle))
        dlclose(flb_lib_handle);

    return rc;
}

int flb_run(void){
    if (likely(flb_start(ctx)) == 0) return 0;
    else return -1;
}

void flb_terminate(void){
    if(ctx){
        flb_stop(ctx);
        flb_destroy(ctx);
        ctx = NULL;
    }
    if(flb_lib_handle) 
        dlclose(flb_lib_handle);
}

static void flb_complete_buff_item(struct File_info *p_file_info){

    Circ_buff_t *buff = p_file_info->circ_buff;

    m_assert(buff->in->timestamp, "buff->in->timestamp cannot be 0");
    m_assert(buff->in->data, "buff->in->text cannot be NULL");
    m_assert(*buff->in->data, "*buff->in->text cannot be 0");
    m_assert(buff->in->text_size, "buff->in->text_size cannot be 0");

    /* Replace last '\n' with '\0' to null-terminate text */
    buff->in->data[buff->in->text_size - 1] = '\0'; 

    /* Store status (timestamp and text_size must have already been 
     * stored during flb_collect_logs_cb() ). */
    buff->in->status = CIRC_BUFF_ITEM_STATUS_UNPROCESSED;

    /* Load max size of compressed buffer, as calculated previously */
    size_t text_compressed_buff_max_size = buff->in->text_compressed_size;

    /* Do compression.
     * TODO: Validate compression option? */
    buff->in->text_compressed = buff->in->data + buff->in->text_size;
    buff->in->text_compressed_size = LZ4_compress_fast( buff->in->data, 
                                                        buff->in->text_compressed, 
                                                        buff->in->text_size, 
                                                        text_compressed_buff_max_size, 
                                                        p_file_info->compression_accel);
    m_assert(buff->in->text_compressed_size != 0, "Text_compressed_size should be != 0");

    p_file_info->parser_metrics->last_update = buff->in->timestamp / MSEC_PER_SEC;

    p_file_info->parser_metrics->num_lines += buff->in->num_lines;

    /* Perform custom log chart parsing */
    for(int i = 0; p_file_info->parser_cus_config[i]; i++){
        p_file_info->parser_metrics->parser_cus[i]->count += 
            search_keyword( buff->in->data, buff->in->text_size, NULL, NULL, 
                            NULL, &p_file_info->parser_cus_config[i]->regex, 0);
    }

    /* Update charts */
    netdata_mutex_lock(&stdout_mut);
    p_file_info->chart_meta->update(p_file_info);
    fflush(stdout);
    netdata_mutex_unlock(&stdout_mut);

    circ_buff_insert(buff);

    uv_timer_again(&p_file_info->flb_tmp_buff_cpy_timer);
}

void flb_complete_item_timer_timeout_cb(uv_timer_t *handle) {

    struct File_info *p_file_info = handle->data;
    Circ_buff_t *buff = p_file_info->circ_buff;
    
    uv_mutex_lock(&p_file_info->flb_tmp_buff_mut);
    if(!buff->in->data || !*buff->in->data || !buff->in->text_size){
        p_file_info->parser_metrics->last_update = now_realtime_sec();
        netdata_mutex_lock(&stdout_mut);
        p_file_info->chart_meta->update(p_file_info);
        fflush(stdout);
        netdata_mutex_unlock(&stdout_mut);
        uv_mutex_unlock(&p_file_info->flb_tmp_buff_mut);
        return; 
    }

    flb_complete_buff_item(p_file_info);

    uv_mutex_unlock(&p_file_info->flb_tmp_buff_mut);    
}

static int flb_collect_logs_cb(void *record, size_t size, void *data){
    
    /* "data" is NULL for Forward-type sources and non-NULL for local sources */
    struct File_info *p_file_info = (struct File_info *) data;
    Circ_buff_t *buff = NULL;

    msgpack_unpacked result;
    size_t off = 0; 
    struct flb_time tmp_time;
    msgpack_object *x;

    char timestamp_str[TIMESTAMP_MS_STR_SIZE] = "";
    msec_t timestamp = 0;

    struct resizable_key_val_arr {
        char **key;
        char **val;
        size_t *key_size;
        size_t *val_size;
        int size, max_size;
    };

    /* FLB_WEB_LOG case */
    Log_line_parsed_t line_parsed = (Log_line_parsed_t) {0};
    /* FLB_WEB_LOG case end */

    /* FLB_KMSG case */
    static int skip_kmsg_log_buffering = 1;
    int kmsg_sever = -1; // -1 equals invalid
    /* FLB_KMSG case end */

    /* FLB_SYSTEMD or FLB_SYSLOG case */
    char syslog_prival[4] = "";
    size_t syslog_prival_size = 0;
    char syslog_severity[2] = "";
    char syslog_facility[3] = "";
    char *syslog_timestamp = NULL;
    size_t syslog_timestamp_size = 0;
    char *hostname = NULL;
    size_t hostname_size = 0;
    char *syslog_identifier = NULL;
    size_t syslog_identifier_size = 0;
    char *pid = NULL;
    size_t pid_size = 0;
    char *message = NULL;
    size_t message_size = 0;
    /* FLB_SYSTEMD or FLB_SYSLOG case end */

    /* FLB_DOCKER_EV case */
    long docker_ev_time = 0;
    long docker_ev_timeNano = 0;
    char *docker_ev_type = NULL;
    size_t docker_ev_type_size = 0;
    char *docker_ev_action = NULL;
    size_t docker_ev_action_size = 0;
    char *docker_ev_id = NULL;
    size_t docker_ev_id_size = 0;
    static struct resizable_key_val_arr docker_ev_attr = {0};
    docker_ev_attr.size = 0;
    /* FLB_DOCKER_EV case end */

    /* FLB_MQTT case */
    char *mqtt_topic = NULL;
    size_t mqtt_topic_size = 0;
    static char *mqtt_message = NULL;
    static size_t mqtt_message_size_max = 0;
    /* FLB_MQTT case end */

    size_t new_tmp_text_size = 0;

    msgpack_unpacked_init(&result);

    int iter = 0;
    while (dl_msgpack_unpack_next(&result, record, size, &off) == MSGPACK_UNPACK_SUCCESS) {
        iter++;
        m_assert(iter == 1, "We do not expect more than one loop iteration here");

        flb_time_pop_from_msgpack(&tmp_time, &result, &x);
        
        if(likely(x->type == MSGPACK_OBJECT_MAP && x->via.map.size != 0)){
            msgpack_object_kv* p = x->via.map.ptr;
            msgpack_object_kv* pend = x->via.map.ptr + x->via.map.size;

            /* ================================================================ 
             * If p_file_info == NULL, it means it is a "Forward" source, so
             * we need to search for the associated p_file_info. This code can 
             * be optimized further.
             * ============================================================== */ 
            if(p_file_info == NULL){
                do{
                    if(!strncmp(p->key.via.str.ptr, "stream guid", (size_t) p->key.via.str.size)){
                        char *stream_guid = (char *) p->val.via.str.ptr;
                        size_t stream_guid_size = p->val.via.str.size;
                        debug_log( "stream guid:%.*s", (int) stream_guid_size, stream_guid);

                        for (int i = 0; i < p_file_infos_arr->count; i++) {
                            if(!strncmp(p_file_infos_arr->data[i]->stream_guid, stream_guid, stream_guid_size)){
                                p_file_info = p_file_infos_arr->data[i];
                                // debug_log( "p_file_info match found: %s type[%s]", 
                                //                      p_file_info->stream_guid, 
                                //                      log_src_type_t_str[p_file_info->log_type]);
                                break;
                            }
                        }
                    }
                    ++p;
                    // continue;
                } while(p < pend);
            }
            if(unlikely(p_file_info == NULL)) 
                goto skip_collect_and_drop_logs;
            

            uv_mutex_lock(&p_file_info->flb_tmp_buff_mut);
            buff = p_file_info->circ_buff;


            p = x->via.map.ptr;
            pend = x->via.map.ptr + x->via.map.size;
            do{
                switch(p_file_info->log_type){

                    case FLB_TAIL:
                    case FLB_WEB_LOG:
                    case FLB_SERIAL: 
                    {
                        if( !strncmp(p->key.via.str.ptr, LOG_REC_KEY, (size_t) p->key.via.str.size) ||
                            /* The following line is in case we collect systemd logs 
                            * (tagged as "MESSAGE") or docker_events (tagged as
                            * "message") via a "Forward" source to an FLB_TAIL parent. */
                            !strncasecmp(p->key.via.str.ptr, LOG_REC_KEY_SYSTEMD, (size_t) p->key.via.str.size)){

                            message = (char *) p->val.via.str.ptr;
                            message_size = p->val.via.str.size;

                            if(p_file_info->log_type == FLB_WEB_LOG){
                                parse_web_log_line( (Web_log_parser_config_t *) p_file_info->parser_config->gen_config, 
                                                    message, message_size, &line_parsed);

                                if(likely(p_file_info->use_log_timestamp)){
                                    timestamp = line_parsed.timestamp * MSEC_PER_SEC; // convert to msec from sec

                                    { /* ------------------ FIXME ------------------------  
                                    * Temporary kludge so that metrics don't break when
                                    * a new record has timestamp before the current one. 
                                    */
                                        static msec_t previous_timestamp = 0;
                                        if((((long long) timestamp - (long long) previous_timestamp) < 0))
                                            timestamp = previous_timestamp;
                                        
                                        previous_timestamp = timestamp;
                                    }
                                }
                            }

                            new_tmp_text_size = message_size + 1; // +1 for '\n'

                            m_assert(message_size, "message_size is 0");
                            m_assert(message, "message is NULL");
                        }

                        break;
                    }

                    case FLB_KMSG:
                    {
                        if(unlikely(skip_kmsg_log_buffering)){
                            static time_t start_time = 0;
                            if (!start_time) start_time = now_boottime_sec();
                            if(now_boottime_sec() - start_time < KERNEL_LOGS_COLLECT_INIT_WAIT)
                                goto skip_collect_and_drop_logs;
                            else skip_kmsg_log_buffering = 0;
                        }

                        /* NOTE/WARNING: 
                        * kmsg timestamps are tricky. The timestamp will be 
                        * *wrong** if the system has gone into hibernation since 
                        * last boot and "p_file_info->use_log_timestamp" is set. 
                        * Even if "p_file_info->use_log_timestamp" is NOT set, we 
                        * need to use now_realtime_msec() as Fluent Bit timestamp
                        * will also be wrong. */
                        if( !strncmp(p->key.via.str.ptr, "sec", (size_t) p->key.via.str.size)){
                            if(p_file_info->use_log_timestamp){
                                timestamp += (now_realtime_sec() - now_boottime_sec() + p->val.via.i64) * MSEC_PER_SEC;
                            }
                            else if(!timestamp)
                                timestamp = now_realtime_msec();
                        }
                        else if(!strncmp(p->key.via.str.ptr, "usec", (size_t) p->key.via.str.size) && 
                                p_file_info->use_log_timestamp){
                            timestamp += p->val.via.i64 / USEC_PER_MS;
                        }
                        else if(!strncmp(p->key.via.str.ptr, LOG_REC_KEY, (size_t) p->key.via.str.size)){
                            message = (char *) p->val.via.str.ptr;
                            message_size = p->val.via.str.size;

                            m_assert(message, "message is NULL");
                            m_assert(message_size, "message_size is 0");

                            new_tmp_text_size += message_size + 1; // +1 for '\n'
                        }
                        else if(!strncmp(p->key.via.str.ptr, "priority", (size_t) p->key.via.str.size)){
                            kmsg_sever = (int) p->val.via.u64;
                        }

                        break;
                    }

                    case FLB_SYSTEMD:
                    case FLB_SYSLOG:
                    {
                        if( p_file_info->use_log_timestamp && !strncmp( p->key.via.str.ptr, 
                                                                        "SOURCE_REALTIME_TIMESTAMP", 
                                                                        (size_t) p->key.via.str.size)){

                            m_assert(p->val.via.str.size - 3 == TIMESTAMP_MS_STR_SIZE - 1, 
                                    "p->val.via.str.size - 3 != TIMESTAMP_MS_STR_SIZE");
                            
                            strncpyz(timestamp_str, p->val.via.str.ptr, (size_t) p->val.via.str.size);

                            char *endptr = NULL;
                            timestamp = str2ll(timestamp_str, &endptr);
                            timestamp = *endptr ? 0 : timestamp / USEC_PER_MS;
                        }
                        else if(!strncmp(p->key.via.str.ptr, "PRIVAL", (size_t) p->key.via.str.size)){
                            m_assert(p->val.via.str.size <= 3, "p->val.via.str.size > 3");
                            strncpyz(syslog_prival, p->val.via.str.ptr, (size_t) p->val.via.str.size);
                            syslog_prival_size = (size_t) p->val.via.str.size;
                            
                            m_assert(syslog_prival, "syslog_prival is NULL");
                        }
                        else if(!strncmp(p->key.via.str.ptr, "PRIORITY", (size_t) p->key.via.str.size)){
                            m_assert(p->val.via.str.size <= 1, "p->val.via.str.size > 1");
                            strncpyz(syslog_severity, p->val.via.str.ptr, (size_t) p->val.via.str.size);
                            
                            m_assert(syslog_severity, "syslog_severity is NULL");
                        }
                        else if(!strncmp(p->key.via.str.ptr, "SYSLOG_FACILITY", (size_t) p->key.via.str.size)){
                            m_assert(p->val.via.str.size <= 2, "p->val.via.str.size > 2");
                            strncpyz(syslog_facility, p->val.via.str.ptr, (size_t) p->val.via.str.size);
                            
                            m_assert(syslog_facility, "syslog_facility is NULL");
                        }
                        else if(!strncmp(p->key.via.str.ptr, "SYSLOG_TIMESTAMP", (size_t) p->key.via.str.size)){
                            syslog_timestamp = (char *) p->val.via.str.ptr;
                            syslog_timestamp_size = p->val.via.str.size;

                            m_assert(syslog_timestamp, "syslog_timestamp is NULL");
                            m_assert(syslog_timestamp_size, "syslog_timestamp_size is 0");
                            
                            new_tmp_text_size += syslog_timestamp_size;
                        }
                        else if(!strncmp(p->key.via.str.ptr, "HOSTNAME", (size_t) p->key.via.str.size)){
                            hostname = (char *) p->val.via.str.ptr;
                            hostname_size = p->val.via.str.size;

                            m_assert(hostname, "hostname is NULL");
                            m_assert(hostname_size, "hostname_size is 0");

                            new_tmp_text_size += hostname_size + 1; // +1 for ' ' char
                        }
                        else if(!strncmp(p->key.via.str.ptr, "SYSLOG_IDENTIFIER", (size_t) p->key.via.str.size)){
                            syslog_identifier = (char *) p->val.via.str.ptr;
                            syslog_identifier_size = p->val.via.str.size;

                            new_tmp_text_size += syslog_identifier_size;
                        }
                        else if(!strncmp(p->key.via.str.ptr, "PID", (size_t) p->key.via.str.size)){
                            pid = (char *) p->val.via.str.ptr;
                            pid_size = p->val.via.str.size;

                            new_tmp_text_size += pid_size;
                        }
                        else if(!strncmp(p->key.via.str.ptr, LOG_REC_KEY_SYSTEMD, (size_t) p->key.via.str.size)){
                            
                            message = (char *) p->val.via.str.ptr;
                            message_size = p->val.via.str.size;

                            m_assert(message, "message is NULL");
                            m_assert(message_size, "message_size is 0");

                            new_tmp_text_size += message_size;
                        }
                        
                        break;
                    }

                    case FLB_DOCKER_EV:
                    { 
                        if(!strncmp(p->key.via.str.ptr, "time", (size_t) p->key.via.str.size)){
                            docker_ev_time = p->val.via.i64;

                            m_assert(docker_ev_time, "docker_ev_time is 0");
                        }
                        else if(!strncmp(p->key.via.str.ptr, "timeNano", (size_t) p->key.via.str.size)){
                            docker_ev_timeNano = p->val.via.i64;

                            m_assert(docker_ev_timeNano, "docker_ev_timeNano is 0");

                            if(likely(p_file_info->use_log_timestamp))
                                timestamp = docker_ev_timeNano / NSEC_PER_MSEC;
                        }
                        else if(!strncmp(p->key.via.str.ptr, "Type", (size_t) p->key.via.str.size)){
                            docker_ev_type = (char *) p->val.via.str.ptr;
                            docker_ev_type_size = p->val.via.str.size;

                            m_assert(docker_ev_type, "docker_ev_type is NULL");
                            m_assert(docker_ev_type_size, "docker_ev_type_size is 0");

                            // debug_log("docker_ev_type: %.*s", docker_ev_type_size, docker_ev_type);
                        }
                        else if(!strncmp(p->key.via.str.ptr, "Action", (size_t) p->key.via.str.size)){
                            docker_ev_action = (char *) p->val.via.str.ptr;
                            docker_ev_action_size = p->val.via.str.size;

                            m_assert(docker_ev_action, "docker_ev_action is NULL");
                            m_assert(docker_ev_action_size, "docker_ev_action_size is 0");

                            // debug_log("docker_ev_action: %.*s", docker_ev_action_size, docker_ev_action);
                        }
                        else if(!strncmp(p->key.via.str.ptr, "id", (size_t) p->key.via.str.size)){
                            docker_ev_id = (char *) p->val.via.str.ptr;
                            docker_ev_id_size = p->val.via.str.size;

                            m_assert(docker_ev_id, "docker_ev_id is NULL");
                            m_assert(docker_ev_id_size, "docker_ev_id_size is 0");

                            // debug_log("docker_ev_id: %.*s", docker_ev_id_size, docker_ev_id);
                        }
                        else if(!strncmp(p->key.via.str.ptr, "Actor", (size_t) p->key.via.str.size)){
                            // debug_log( "msg key:[%.*s]val:[%.*s]", (int)   p->key.via.str.size, 
                            //                                                                     p->key.via.str.ptr, 
                            //                                                             (int)   p->val.via.str.size, 
                            //                                                                     p->val.via.str.ptr);
                            if(likely(p->val.type == MSGPACK_OBJECT_MAP && p->val.via.map.size != 0)){
                                msgpack_object_kv* ac = p->val.via.map.ptr;
                                msgpack_object_kv* const ac_pend= p->val.via.map.ptr + p->val.via.map.size;
                                do{
                                    if(!strncmp(ac->key.via.str.ptr, "ID", (size_t) ac->key.via.str.size)){
                                        docker_ev_id = (char *) ac->val.via.str.ptr;
                                        docker_ev_id_size = ac->val.via.str.size;

                                        m_assert(docker_ev_id, "docker_ev_id is NULL");
                                        m_assert(docker_ev_id_size, "docker_ev_id_size is 0");

                                        // debug_log("docker_ev_id: %.*s", docker_ev_id_size, docker_ev_id);
                                    }
                                    else if(!strncmp(ac->key.via.str.ptr, "Attributes", (size_t) ac->key.via.str.size)){
                                        if(likely(ac->val.type == MSGPACK_OBJECT_MAP && ac->val.via.map.size != 0)){
                                            msgpack_object_kv* att = ac->val.via.map.ptr;
                                            msgpack_object_kv* const att_pend = ac->val.via.map.ptr + ac->val.via.map.size;
                                            do{
                                                if(unlikely(++docker_ev_attr.size > docker_ev_attr.max_size)){
                                                    docker_ev_attr.max_size = docker_ev_attr.size;
                                                    docker_ev_attr.key = reallocz(docker_ev_attr.key, 
                                                                                docker_ev_attr.max_size * sizeof(char *));
                                                    docker_ev_attr.val = reallocz(docker_ev_attr.val, 
                                                                                docker_ev_attr.max_size * sizeof(char *));
                                                    docker_ev_attr.key_size = reallocz(docker_ev_attr.key_size, 
                                                                                docker_ev_attr.max_size * sizeof(size_t));
                                                    docker_ev_attr.val_size = reallocz(docker_ev_attr.val_size, 
                                                                                docker_ev_attr.max_size * sizeof(size_t));
                                                }

                                                docker_ev_attr.key[docker_ev_attr.size - 1] =  (char *) att->key.via.str.ptr;
                                                docker_ev_attr.val[docker_ev_attr.size - 1] =  (char *) att->val.via.str.ptr;
                                                docker_ev_attr.key_size[docker_ev_attr.size - 1] = (size_t) att->key.via.str.size;
                                                docker_ev_attr.val_size[docker_ev_attr.size - 1] = (size_t) att->val.via.str.size;

                                                att++;
                                                continue;
                                            } while(att < att_pend);
                                        }
                                    }
                                    ac++;
                                    continue;
                                } while(ac < ac_pend);
                            }
                        }
                        
                        break;
                    }

                    case FLB_MQTT:
                    {
                        if(!strncmp(p->key.via.str.ptr, "topic", (size_t) p->key.via.str.size)){
                            mqtt_topic = (char *) p->val.via.str.ptr;
                            mqtt_topic_size = (size_t) p->val.via.str.size;

                            while(0 == (message_size = dl_msgpack_object_print_buffer(mqtt_message, mqtt_message_size_max, *x)))
                                mqtt_message = reallocz(mqtt_message, (mqtt_message_size_max += 10));

                            new_tmp_text_size = message_size + 1; // +1 for '\n'

                            m_assert(message_size, "message_size is 0");
                            m_assert(mqtt_message, "mqtt_message is NULL");

                            break; // watch out, MQTT requires a 'break' here, as we parse the entire 'x' msgpack_object
                        }
                        else m_assert(0, "missing mqtt topic");

                        break;
                    }

                    default:
                        break;
                }
                
            } while(++p < pend);
        } 
    }

    /* If no log timestamp was found, use Fluent Bit collection timestamp. */
    if(timestamp == 0) 
        timestamp = (msec_t) tmp_time.tm.tv_sec * MSEC_PER_SEC + (msec_t) tmp_time.tm.tv_nsec / (NSEC_PER_MSEC);

    m_assert(TEST_MS_TIMESTAMP_VALID(timestamp), "timestamp is invalid");

    /* If input buffer timestamp is not set, now is the time to set it,
     * else just be done with the previous buffer */
    if(unlikely(buff->in->timestamp == 0)) buff->in->timestamp = timestamp / 1000 * 1000; // rounding down
    else if((timestamp - buff->in->timestamp) >= MSEC_PER_SEC) {
        flb_complete_buff_item(p_file_info);
        buff->in->timestamp = timestamp / 1000 * 1000; // rounding down
    }

    m_assert(TEST_MS_TIMESTAMP_VALID(buff->in->timestamp), "buff->in->timestamp is invalid");

    new_tmp_text_size += buff->in->text_size; 

    /* ======================================================================== 
     * Step 2: Extract metrics and reconstruct log record 
     * ====================================================================== */

    /* Parse number of log lines - common for all log source types */
    buff->in->num_lines++;

    /* FLB_TAIL, FLB_WEB_LOG and FLB_SERIAL case */
    if( p_file_info->log_type == FLB_TAIL || 
        p_file_info->log_type == FLB_WEB_LOG || 
        p_file_info->log_type == FLB_SERIAL){

        if(p_file_info->log_type == FLB_WEB_LOG)
            extract_web_log_metrics(p_file_info->parser_config, &line_parsed, 
                                    p_file_info->parser_metrics->web_log);

        // TODO: Fix: Metrics will still be collected if circ_buff_prepare_write() returns 0.
        if(unlikely(!circ_buff_prepare_write(buff, new_tmp_text_size))) 
            goto skip_collect_and_drop_logs;

        size_t tmp_item_off = buff->in->text_size;

        memcpy_iscntrl_fix(&buff->in->data[tmp_item_off], message, message_size);
        tmp_item_off += message_size;  

        buff->in->data[tmp_item_off++] = '\n';
        m_assert(tmp_item_off == new_tmp_text_size, "tmp_item_off should be == new_tmp_text_size");
        buff->in->text_size = new_tmp_text_size;

#ifdef HAVE_SYSTEMD
        if(p_file_info->do_sd_journal_send){
            if(p_file_info->log_type == FLB_WEB_LOG){
                sd_journal_send(
                    SD_JOURNAL_SEND_DEFAULT_FIELDS,
                    *line_parsed.vhost          ?   "%sWEB_LOG_VHOST=%s"          : "_%s=%s", sd_journal_field_prefix, line_parsed.vhost,
                    line_parsed.port            ?   "%sWEB_LOG_PORT=%d"           : "_%s=%d", sd_journal_field_prefix, line_parsed.port,
                    *line_parsed.req_scheme     ?   "%sWEB_LOG_REQ_SCHEME=%s"     : "_%s=%s", sd_journal_field_prefix, line_parsed.req_scheme,
                    *line_parsed.req_client     ?   "%sWEB_LOG_REQ_CLIENT=%s"     : "_%s=%s", sd_journal_field_prefix, line_parsed.req_client,
                                                    "%sWEB_LOG_REQ_METHOD=%s"               , sd_journal_field_prefix, line_parsed.req_method,
                    *line_parsed.req_URL        ?   "%sWEB_LOG_REQ_URL=%s"        : "_%s=%s", sd_journal_field_prefix, line_parsed.req_URL,
                    *line_parsed.req_proto      ?   "%sWEB_LOG_REQ_PROTO=%s"      : "_%s=%s", sd_journal_field_prefix, line_parsed.req_proto,
                    line_parsed.req_size        ?   "%sWEB_LOG_REQ_SIZE=%d"       : "_%s=%d", sd_journal_field_prefix, line_parsed.req_size,
                    line_parsed.req_proc_time   ?   "%sWEB_LOG_REC_PROC_TIME=%d"  : "_%s=%d", sd_journal_field_prefix, line_parsed.req_proc_time,
                    line_parsed.resp_code       ?   "%sWEB_LOG_RESP_CODE=%d"      : "_%s=%d", sd_journal_field_prefix ,line_parsed.resp_code,
                    line_parsed.ups_resp_time   ?   "%sWEB_LOG_UPS_RESP_TIME=%d"  : "_%s=%d", sd_journal_field_prefix ,line_parsed.ups_resp_time,
                    *line_parsed.ssl_proto      ?   "%sWEB_LOG_SSL_PROTO=%s"      : "_%s=%s", sd_journal_field_prefix ,line_parsed.ssl_proto,
                    *line_parsed.ssl_cipher     ?   "%sWEB_LOB_SSL_CIPHER=%s"     : "_%s=%s", sd_journal_field_prefix ,line_parsed.ssl_cipher,
                    LOG_REC_KEY_SYSTEMD "=%.*s", (int) message_size, message,
                    NULL
                );
            }
            else if(p_file_info->log_type == FLB_SERIAL){
                Flb_serial_config_t *serial_config = (Flb_serial_config_t *) p_file_info->flb_config;
                sd_journal_send(
                    SD_JOURNAL_SEND_DEFAULT_FIELDS,
                    serial_config->bitrate && *serial_config->bitrate ? 
                        "%sSERIAL_BITRATE=%s" : "_%s=%s", sd_journal_field_prefix, serial_config->bitrate,
                    LOG_REC_KEY_SYSTEMD "=%.*s", (int) message_size, message, 
                    NULL
                );
            }
            else{
                sd_journal_send(
                    SD_JOURNAL_SEND_DEFAULT_FIELDS, 
                    LOG_REC_KEY_SYSTEMD "=%.*s", (int) message_size, message, 
                    NULL
                );
            }
        }
#endif

    } /* FLB_TAIL, FLB_WEB_LOG and FLB_SERIAL case end */

    /* FLB_KMSG case */
    else if(p_file_info->log_type == FLB_KMSG){

        char *c;

        // see https://www.kernel.org/doc/Documentation/ABI/testing/dev-kmsg
        if((c = memchr(message, '\n', message_size))){

            const char  subsys_str[] = "SUBSYSTEM=",
                        device_str[] = "DEVICE=";
            const size_t subsys_str_len = sizeof(subsys_str) - 1,
                         device_str_len = sizeof(device_str) - 1;

            size_t bytes_remain = message_size - (c - message);

            /* Extract machine-readable info for charts, such as subsystem and device. */
            while(bytes_remain){
                size_t sz = 0;
                while(--bytes_remain && c[++sz] != '\n');
                if(bytes_remain) --sz;
                *(c++) = '\\';
                *(c++) = 'n'; 
                sz--;

                DICTIONARY *dict = NULL;
                char *str = NULL;
                size_t str_len = 0;
                if(!strncmp(c, subsys_str, subsys_str_len)){
                    dict = p_file_info->parser_metrics->kernel->subsystem;
                    str = &c[subsys_str_len];
                    str_len = (sz - subsys_str_len);
                }
                else if (!strncmp(c, device_str, device_str_len)){
                    dict = p_file_info->parser_metrics->kernel->device;
                    str = &c[device_str_len];
                    str_len = (sz - device_str_len);
                }

                if(likely(str)){
                    char *const key = mallocz(str_len + 1);
                    memcpy(key, str, str_len);
                    key[str_len] = '\0';
                    metrics_dict_item_t item = {.dim_initialized = false, .num_new = 1};
                    dictionary_set_advanced(dict, key, str_len, &item, sizeof(item), NULL);
                }
                c = &c[sz];
            }
        }

        if(likely(kmsg_sever >= 0))
            p_file_info->parser_metrics->kernel->sever[kmsg_sever]++;

        // TODO: Fix: Metrics will still be collected if circ_buff_prepare_write() returns 0.
        if(unlikely(!circ_buff_prepare_write(buff, new_tmp_text_size))) 
            goto skip_collect_and_drop_logs;

        size_t tmp_item_off = buff->in->text_size;

        memcpy_iscntrl_fix(&buff->in->data[tmp_item_off], message, message_size);
        tmp_item_off += message_size;  

        buff->in->data[tmp_item_off++] = '\n';
        m_assert(tmp_item_off == new_tmp_text_size, "tmp_item_off should be == new_tmp_text_size");
        buff->in->text_size = new_tmp_text_size;
    } /* FLB_KMSG case end */
    
    /* FLB_SYSTEMD or FLB_SYSLOG case */
    else if(p_file_info->log_type == FLB_SYSTEMD || 
            p_file_info->log_type == FLB_SYSLOG){

        int syslog_prival_d = SYSLOG_PRIOR_ARR_SIZE - 1; // Initialise to 'unknown'
        int syslog_severity_d = SYSLOG_SEVER_ARR_SIZE - 1; // Initialise to 'unknown'
        int syslog_facility_d = SYSLOG_FACIL_ARR_SIZE - 1; // Initialise to 'unknown'


        /* FLB_SYSTEMD case has syslog_severity and syslog_facility values that
         * are used to calculate syslog_prival from. FLB_SYSLOG is the opposite
         * case, as it has a syslog_prival value that is used to calculate 
         * syslog_severity and syslog_facility from. */
        if(p_file_info->log_type == FLB_SYSTEMD){

            /* Parse syslog_severity char* field into int and extract metrics. 
            * syslog_severity_s will consist of 1 char (plus '\0'), 
            * see https://datatracker.ietf.org/doc/html/rfc5424#section-6.2.1 */
            if(likely(syslog_severity[0])){            
                if(likely(str2int(&syslog_severity_d, syslog_severity, 10) == STR2XX_SUCCESS)){
                    p_file_info->parser_metrics->systemd->sever[syslog_severity_d]++;
                } // else parsing errors ++ ??
            } else p_file_info->parser_metrics->systemd->sever[SYSLOG_SEVER_ARR_SIZE - 1]++; // 'unknown'

            /* Parse syslog_facility char* field into int and extract metrics. 
            * syslog_facility_s will consist of up to 2 chars (plus '\0'), 
            * see https://datatracker.ietf.org/doc/html/rfc5424#section-6.2.1 */
            if(likely(syslog_facility[0])){
                if(likely(str2int(&syslog_facility_d, syslog_facility, 10) == STR2XX_SUCCESS)){
                    p_file_info->parser_metrics->systemd->facil[syslog_facility_d]++;
                } // else parsing errors ++ ??
            } else p_file_info->parser_metrics->systemd->facil[SYSLOG_FACIL_ARR_SIZE - 1]++; // 'unknown'

            if(likely(syslog_severity[0] && syslog_facility[0])){
                /* Definition of syslog priority value == facility * 8 + severity */
                syslog_prival_d = syslog_facility_d * 8 + syslog_severity_d; 
                syslog_prival_size = snprintfz(syslog_prival, 4, "%d", syslog_prival_d);
                m_assert(syslog_prival_size < 4 && syslog_prival_size > 0, "error with snprintf()");
        
                new_tmp_text_size += syslog_prival_size + 2; // +2 for '<' and '>'

                p_file_info->parser_metrics->systemd->prior[syslog_prival_d]++;
            } else {
                new_tmp_text_size += 3; // +3 for "<->" string
                p_file_info->parser_metrics->systemd->prior[SYSLOG_PRIOR_ARR_SIZE - 1]++; // 'unknown'
            } 

        } else if(p_file_info->log_type == FLB_SYSLOG){

            if(likely(syslog_prival[0])){            
                if(likely(str2int(&syslog_prival_d, syslog_prival, 10) == STR2XX_SUCCESS)){
                    syslog_severity_d = syslog_prival_d % 8;
                    syslog_facility_d = syslog_prival_d / 8;

                    p_file_info->parser_metrics->systemd->prior[syslog_prival_d]++;
                    p_file_info->parser_metrics->systemd->sever[syslog_severity_d]++;
                    p_file_info->parser_metrics->systemd->facil[syslog_facility_d]++;

                    new_tmp_text_size += syslog_prival_size + 2; // +2 for '<' and '>'

                } // else parsing errors ++ ??
            } else {
                new_tmp_text_size += 3; // +3 for "<->" string
                p_file_info->parser_metrics->systemd->prior[SYSLOG_PRIOR_ARR_SIZE - 1]++; // 'unknown'
                p_file_info->parser_metrics->systemd->sever[SYSLOG_SEVER_ARR_SIZE - 1]++; // 'unknown'
                p_file_info->parser_metrics->systemd->facil[SYSLOG_FACIL_ARR_SIZE - 1]++; // 'unknown'
            }

        } else m_assert(0, "shoudn't get here");

        char syslog_time_from_flb_time[25]; // 25 just to be on the safe side, but 16 + 1 chars bytes needed only.
        if(unlikely(!syslog_timestamp)){
            const time_t ts = tmp_time.tm.tv_sec;
            struct tm *const tm = localtime(&ts);

            strftime(syslog_time_from_flb_time, sizeof(syslog_time_from_flb_time), "%b %d %H:%M:%S ", tm);
            new_tmp_text_size += SYSLOG_TIMESTAMP_SIZE;
        }

        if(unlikely(!syslog_identifier)) new_tmp_text_size += sizeof(UNKNOWN) - 1;
        if(unlikely(!pid)) new_tmp_text_size += sizeof(UNKNOWN) - 1;

        new_tmp_text_size += 5; // +5 for '[', ']', ':' and ' ' characters around and after pid and '\n' at the end

        /* Metrics extracted, now prepare circular buffer for write */
        // TODO: Fix: Metrics will still be collected if circ_buff_prepare_write() returns 0.
        if(unlikely(!circ_buff_prepare_write(buff, new_tmp_text_size))) 
            goto skip_collect_and_drop_logs;

        size_t tmp_item_off = buff->in->text_size;

        buff->in->data[tmp_item_off++] = '<';
        if(likely(syslog_prival[0])){
            memcpy(&buff->in->data[tmp_item_off], syslog_prival, syslog_prival_size);
            m_assert(syslog_prival_size, "syslog_prival_size cannot be 0");
            tmp_item_off += syslog_prival_size;
        } else buff->in->data[tmp_item_off++] = '-';
        buff->in->data[tmp_item_off++] = '>';

        if(likely(syslog_timestamp)){
            memcpy(&buff->in->data[tmp_item_off], syslog_timestamp, syslog_timestamp_size);
            // FLB_SYSLOG doesn't add space, but FLB_SYSTEMD does:
            // if(buff->in->data[tmp_item_off] != ' ') buff->in->data[tmp_item_off++] = ' '; 
            tmp_item_off += syslog_timestamp_size;
        } else {
            memcpy(&buff->in->data[tmp_item_off], syslog_time_from_flb_time, SYSLOG_TIMESTAMP_SIZE);
            tmp_item_off += SYSLOG_TIMESTAMP_SIZE;
        }

        if(likely(hostname)){
            memcpy(&buff->in->data[tmp_item_off], hostname, hostname_size);
            tmp_item_off += hostname_size;
            buff->in->data[tmp_item_off++] = ' ';
        }

        if(likely(syslog_identifier)){
            memcpy(&buff->in->data[tmp_item_off], syslog_identifier, syslog_identifier_size);
            tmp_item_off += syslog_identifier_size;
        } else {
            memcpy(&buff->in->data[tmp_item_off], UNKNOWN, sizeof(UNKNOWN) - 1);
            tmp_item_off += sizeof(UNKNOWN) - 1;
        }

        buff->in->data[tmp_item_off++] = '[';
        if(likely(pid)){
            memcpy(&buff->in->data[tmp_item_off], pid, pid_size);
            tmp_item_off += pid_size;
        } else {
            memcpy(&buff->in->data[tmp_item_off], UNKNOWN, sizeof(UNKNOWN) - 1);
            tmp_item_off += sizeof(UNKNOWN) - 1;
        }
        buff->in->data[tmp_item_off++] = ']';
        
        buff->in->data[tmp_item_off++] = ':';
        buff->in->data[tmp_item_off++] = ' ';

        if(likely(message)){
            memcpy_iscntrl_fix(&buff->in->data[tmp_item_off], message, message_size);
            tmp_item_off += message_size;  
        }

        buff->in->data[tmp_item_off++] = '\n';
        m_assert(tmp_item_off == new_tmp_text_size, "tmp_item_off should be == new_tmp_text_size");
        buff->in->text_size = new_tmp_text_size;
    } /* FLB_SYSTEMD or FLB_SYSLOG case end */
    
    /* FLB_DOCKER_EV case */
    else if(p_file_info->log_type == FLB_DOCKER_EV){ 

        const size_t docker_ev_datetime_size = sizeof "2022-08-26T15:33:20.802840200+0000" /* example datetime */;
        char docker_ev_datetime[docker_ev_datetime_size];
        docker_ev_datetime[0] = 0;
        if(likely(docker_ev_time && docker_ev_timeNano)){
            struct timespec ts;
            ts.tv_sec = docker_ev_time;
            if(unlikely(0 == strftime( docker_ev_datetime, docker_ev_datetime_size, 
                              "%Y-%m-%dT%H:%M:%S.000000000%z", localtime(&ts.tv_sec)))) { /* TODO: do what if error? */};
            const size_t docker_ev_timeNano_s_size = sizeof "802840200";
            char docker_ev_timeNano_s[docker_ev_timeNano_s_size];
            snprintfz(  docker_ev_timeNano_s, docker_ev_timeNano_s_size, "%0*ld", 
                        (int) docker_ev_timeNano_s_size, docker_ev_timeNano % 1000000000);
            memcpy(&docker_ev_datetime[20], &docker_ev_timeNano_s, docker_ev_timeNano_s_size - 1);

            new_tmp_text_size += docker_ev_datetime_size; // -1 for null terminator, +1 for ' ' character
        }

        if(likely(docker_ev_type && docker_ev_action)){
            int ev_off = -1;
            while(++ev_off < NUM_OF_DOCKER_EV_TYPES){
                if(!strncmp(docker_ev_type, docker_ev_type_string[ev_off], docker_ev_type_size)){
                    p_file_info->parser_metrics->docker_ev->ev_type[ev_off]++;

                    int act_off = -1;
                    while(docker_ev_action_string[ev_off][++act_off] != NULL){
                        if(!strncmp(docker_ev_action, docker_ev_action_string[ev_off][act_off], docker_ev_action_size)){
                            p_file_info->parser_metrics->docker_ev->ev_action[ev_off][act_off]++;
                            break;
                        }
                    }
                    if(unlikely(docker_ev_action_string[ev_off][act_off] == NULL))
                        p_file_info->parser_metrics->docker_ev->ev_action[NUM_OF_DOCKER_EV_TYPES - 1][0]++; // 'unknown'

                    break;
                }
            }
            if(unlikely(ev_off >= NUM_OF_DOCKER_EV_TYPES - 1)){
                p_file_info->parser_metrics->docker_ev->ev_type[ev_off]++; // 'unknown'
                p_file_info->parser_metrics->docker_ev->ev_action[NUM_OF_DOCKER_EV_TYPES - 1][0]++; // 'unknown'
            }

            new_tmp_text_size += docker_ev_type_size + docker_ev_action_size + 2; // +2 for ' ' chars
        }

        if(likely(docker_ev_id)){
            // debug_log("docker_ev_id: %.*s", (int) docker_ev_id_size, docker_ev_id);

            new_tmp_text_size += docker_ev_id_size + 1; // +1 for ' ' char
        }

        if(likely(docker_ev_attr.size)){
            for(int i = 0; i < docker_ev_attr.size; i++){
                new_tmp_text_size += docker_ev_attr.key_size[i] + 
                                     docker_ev_attr.val_size[i] + 3; // +3 for '=' ',' ' ' characters
            }
            /* new_tmp_text_size = -2 + 2; 
             * -2 due to missing ',' ' ' from last attribute and +2 for the two 
             * '(' and ')' characters, so no need to add or subtract */
        }

        new_tmp_text_size += 1; // +1 for '\n' character at the end
        
        /* Metrics extracted, now prepare circular buffer for write */
        // TODO: Fix: Metrics will still be collected if circ_buff_prepare_write() returns 0.
        if(unlikely(!circ_buff_prepare_write(buff, new_tmp_text_size))) 
            goto skip_collect_and_drop_logs;

        size_t tmp_item_off = buff->in->text_size;
        message_size = new_tmp_text_size - 1 - tmp_item_off;

        if(likely(*docker_ev_datetime)){
            memcpy(&buff->in->data[tmp_item_off], docker_ev_datetime, docker_ev_datetime_size - 1);
            tmp_item_off += docker_ev_datetime_size - 1; // -1 due to null terminator
            buff->in->data[tmp_item_off++] = ' ';
        }

        if(likely(docker_ev_type)){
            memcpy(&buff->in->data[tmp_item_off], docker_ev_type, docker_ev_type_size);
            tmp_item_off += docker_ev_type_size;
            buff->in->data[tmp_item_off++] = ' ';
        }

        if(likely(docker_ev_action)){
            memcpy(&buff->in->data[tmp_item_off], docker_ev_action, docker_ev_action_size);
            tmp_item_off += docker_ev_action_size;
            buff->in->data[tmp_item_off++] = ' ';
        }

        if(likely(docker_ev_id)){
            memcpy(&buff->in->data[tmp_item_off], docker_ev_id, docker_ev_id_size);
            tmp_item_off += docker_ev_id_size;
            buff->in->data[tmp_item_off++] = ' ';
        }

        if(likely(docker_ev_attr.size)){
            buff->in->data[tmp_item_off++] = '(';
            for(int i = 0; i < docker_ev_attr.size; i++){
                memcpy(&buff->in->data[tmp_item_off], docker_ev_attr.key[i], docker_ev_attr.key_size[i]);
                tmp_item_off += docker_ev_attr.key_size[i];
                buff->in->data[tmp_item_off++] = '=';
                memcpy(&buff->in->data[tmp_item_off], docker_ev_attr.val[i], docker_ev_attr.val_size[i]);
                tmp_item_off += docker_ev_attr.val_size[i];
                buff->in->data[tmp_item_off++] = ',';
                buff->in->data[tmp_item_off++] = ' ';
            }
            tmp_item_off -= 2; // overwrite last ',' and ' ' characters with a ')' character
            buff->in->data[tmp_item_off++] = ')';
        }

        buff->in->data[tmp_item_off++] = '\n';
        m_assert(tmp_item_off == new_tmp_text_size, "tmp_item_off should be == new_tmp_text_size");
        buff->in->text_size = new_tmp_text_size;

#ifdef HAVE_SYSTEMD
        if(p_file_info->do_sd_journal_send){
            sd_journal_send(
                SD_JOURNAL_SEND_DEFAULT_FIELDS,
                "%sDOCKER_EVENTS_TYPE=%.*s",    sd_journal_field_prefix, (int) docker_ev_type_size,    docker_ev_type,
                "%sDOCKER_EVENTS_ACTION=%.*s",  sd_journal_field_prefix, (int) docker_ev_action_size,  docker_ev_action,
                "%sDOCKER_EVENTS_ID=%.*s",      sd_journal_field_prefix, (int) docker_ev_id_size,      docker_ev_id,
                LOG_REC_KEY_SYSTEMD "=%.*s",                 (int) message_size, &buff->in->data[tmp_item_off - 1 - message_size], 
                NULL
            );
        }
#endif

    } /* FLB_DOCKER_EV case end */

    /* FLB_MQTT case */
    else if(p_file_info->log_type == FLB_MQTT){
        if(likely(mqtt_topic)){
            char *const key = mallocz(mqtt_topic_size + 1);
            memcpy(key, mqtt_topic, mqtt_topic_size);
            key[mqtt_topic_size] = '\0';
            metrics_dict_item_t item = {.dim_initialized = false, .num_new = 1};
            dictionary_set_advanced(p_file_info->parser_metrics->mqtt->topic, key, mqtt_topic_size, &item, sizeof(item), NULL);

            // TODO: Fix: Metrics will still be collected if circ_buff_prepare_write() returns 0.
            if(unlikely(!circ_buff_prepare_write(buff, new_tmp_text_size))) 
                goto skip_collect_and_drop_logs;

            size_t tmp_item_off = buff->in->text_size;

            memcpy(&buff->in->data[tmp_item_off], mqtt_message, message_size);
            tmp_item_off += message_size;

            buff->in->data[tmp_item_off++] = '\n';
            m_assert(tmp_item_off == new_tmp_text_size, "tmp_item_off should be == new_tmp_text_size");
            buff->in->text_size = new_tmp_text_size;

#ifdef HAVE_SYSTEMD
            if(p_file_info->do_sd_journal_send){
                sd_journal_send(
                    SD_JOURNAL_SEND_DEFAULT_FIELDS,
                    "%sMQTT_TOPIC=%s", key, 
                    LOG_REC_KEY_SYSTEMD "=%.*s", (int) message_size, mqtt_message, 
                    NULL
                );
            }
#endif

        }
        else m_assert(0, "missing mqtt topic");
    }

skip_collect_and_drop_logs:
    /* Following code is equivalent to msgpack_unpacked_destroy(&result) due 
     * to that function call being unavailable when using dl_open() */
    if(result.zone != NULL) {
        dl_msgpack_zone_free(result.zone);
        result.zone = NULL;
        memset(&result.data, 0, sizeof(msgpack_object));
    }

    if(p_file_info) 
        uv_mutex_unlock(&p_file_info->flb_tmp_buff_mut);
    
    flb_lib_free(record);
    return 0;
    
}

/**
 * @brief Add a Fluent-Bit input that outputs to the "lib" Fluent-Bit plugin.
 * @param[in] p_file_info Pointer to the log source struct where the input will
 * be registered to.
 * @return 0 on success, a negative number for any errors (see enum).
 */
int flb_add_input(struct File_info *const p_file_info){

    enum return_values {
        SUCCESS = 0,
        INVALID_LOG_TYPE = -1,
        CONFIG_READ_ERROR = -2,
        FLB_PARSER_CREATE_ERROR = -3,
        FLB_INPUT_ERROR = -4,
        FLB_INPUT_SET_ERROR = -5,
        FLB_OUTPUT_ERROR = -6,
        FLB_OUTPUT_SET_ERROR = -7,
        DEFAULT_ERROR = -8
    };

    const int tag_max_size = 5;
    static unsigned tag = 0; // incremental tag id to link flb inputs to outputs
    char tag_s[tag_max_size];
    snprintfz(tag_s, tag_max_size, "%u", tag++);


    switch(p_file_info->log_type){
        case FLB_TAIL:
        case FLB_WEB_LOG: {

            char update_every_str[10]; 
            snprintfz(update_every_str, 10, "%d", p_file_info->update_every);

            debug_log("Setting up %s tail for %s (basename:%s)", 
                    p_file_info->log_type == FLB_TAIL ? "FLB_TAIL" : "FLB_WEB_LOG",
                    p_file_info->filename, p_file_info->file_basename);

            Flb_tail_config_t *tail_config = (Flb_tail_config_t *) p_file_info->flb_config;
            if(unlikely(!tail_config)) return CONFIG_READ_ERROR;
        
            /* Set up input from log source */
            p_file_info->flb_input = flb_input(ctx, "tail", NULL);
            if(p_file_info->flb_input < 0 ) return FLB_INPUT_ERROR;
            if(flb_input_set(ctx, p_file_info->flb_input, 
                "Tag", tag_s, 
                "Path", p_file_info->filename, 
                "Key", LOG_REC_KEY,
                "Refresh_Interval", update_every_str,
                "Skip_Long_Lines", "On",
                "Skip_Empty_Lines", "On",
#if defined(FLB_HAVE_INOTIFY)
                "Inotify_Watcher", tail_config->use_inotify ? "true" : "false", 
#endif
                NULL) != 0) return FLB_INPUT_SET_ERROR;

            break;
        }
        case FLB_KMSG: {
            debug_log( "Setting up FLB_KMSG collector");

            Flb_kmsg_config_t *kmsg_config = (Flb_kmsg_config_t *) p_file_info->flb_config;
            if(unlikely(!kmsg_config || 
                        !kmsg_config->prio_level || 
                        !*kmsg_config->prio_level)) return CONFIG_READ_ERROR;
        
            /* Set up kmsg input */
            p_file_info->flb_input = flb_input(ctx, "kmsg", NULL);
            if(p_file_info->flb_input < 0 ) return FLB_INPUT_ERROR;
            if(flb_input_set(ctx, p_file_info->flb_input, 
                "Tag", tag_s,
                "Prio_Level", kmsg_config->prio_level,
                NULL) != 0) return FLB_INPUT_SET_ERROR;
            
            break;
        }
        case FLB_SYSTEMD: {
            debug_log( "Setting up FLB_SYSTEMD collector");
        
            /* Set up systemd input */
            p_file_info->flb_input = flb_input(ctx, "systemd", NULL);
            if(p_file_info->flb_input < 0 ) return FLB_INPUT_ERROR;
            if(!strcmp(p_file_info->filename, SYSTEMD_DEFAULT_PATH)){
                if(flb_input_set(ctx, p_file_info->flb_input, 
                    "Tag", tag_s,
                    "Read_From_Tail", "On",
                    "Strip_Underscores", "On",
                    NULL) != 0) return FLB_INPUT_SET_ERROR;
            } else {
                if(flb_input_set(ctx, p_file_info->flb_input, 
                    "Tag", tag_s,
                    "Read_From_Tail", "On",
                    "Strip_Underscores", "On",
                    "Path", p_file_info->filename,
                    NULL) != 0) return FLB_INPUT_SET_ERROR;
            }

            break;
        }
        case FLB_DOCKER_EV: {
            debug_log( "Setting up FLB_DOCKER_EV collector");

            /* Set up Docker Events parser */
            if(flb_parser_create(   "docker_events_parser", /* parser name */
                                    "json",                 /* backend type */
                                    NULL,                   /* regex */
                                    FLB_TRUE,               /* skip_empty */
                                    NULL,                   /* time format */
                                    NULL,                   /* time key */
                                    NULL,                   /* time offset */
                                    FLB_TRUE,               /* time keep */
                                    FLB_FALSE,              /* time strict */
                                    FLB_FALSE,              /* no bare keys */
                                    NULL,                   /* parser types */
                                    0,                      /* types len */
                                    NULL,                   /* decoders */
                                    ctx->config) == NULL) return FLB_PARSER_CREATE_ERROR;
        
            /* Set up Docker Events input */
            p_file_info->flb_input = flb_input(ctx, "docker_events", NULL);
            if(p_file_info->flb_input < 0 ) return FLB_INPUT_ERROR;
            if(flb_input_set(ctx, p_file_info->flb_input, 
                "Tag", tag_s,
                "Parser", "docker_events_parser",
                "Unix_Path", p_file_info->filename,
                NULL) != 0) return FLB_INPUT_SET_ERROR;
            
            break;
        }
        case FLB_SYSLOG: {
            debug_log( "Setting up FLB_SYSLOG collector");

            /* Set up syslog parser */
            const char syslog_parser_prfx[] = "syslog_parser_";
            size_t parser_name_size = sizeof(syslog_parser_prfx) + tag_max_size - 1;
            char parser_name[parser_name_size];
            snprintfz(parser_name, parser_name_size, "%s%u", syslog_parser_prfx, tag);

            Syslog_parser_config_t *syslog_config = (Syslog_parser_config_t *) p_file_info->parser_config->gen_config;
            if(unlikely(!syslog_config || 
                        !syslog_config->socket_config || 
                        !syslog_config->socket_config->mode || 
                        !p_file_info->filename)) return CONFIG_READ_ERROR;
                        
            if(flb_parser_create(   parser_name,                /* parser name */
                                    "regex",                    /* backend type */
                                    syslog_config->log_format,  /* regex */
                                    FLB_TRUE,                   /* skip_empty */
                                    NULL,                       /* time format */
                                    NULL,                       /* time key */
                                    NULL,                       /* time offset */
                                    FLB_TRUE,                   /* time keep */
                                    FLB_TRUE,                   /* time strict */
                                    FLB_FALSE,                  /* no bare keys */
                                    NULL,                       /* parser types */
                                    0,                          /* types len */
                                    NULL,                       /* decoders */
                                    ctx->config) == NULL) return FLB_PARSER_CREATE_ERROR;
        
            /* Set up syslog input */
            p_file_info->flb_input = flb_input(ctx, "syslog", NULL);
            if(p_file_info->flb_input < 0 ) return FLB_INPUT_ERROR;
            if( !strcmp(syslog_config->socket_config->mode, "unix_udp") || 
                !strcmp(syslog_config->socket_config->mode, "unix_tcp")){
                m_assert(syslog_config->socket_config->unix_perm, "unix_perm is not set");
                if(flb_input_set(ctx, p_file_info->flb_input, 
                    "Tag", tag_s,
                    "Path", p_file_info->filename,
                    "Parser", parser_name,
                    "Mode", syslog_config->socket_config->mode,
                    "Unix_Perm", syslog_config->socket_config->unix_perm,
                    NULL) != 0) return FLB_INPUT_SET_ERROR;
            } else if(  !strcmp(syslog_config->socket_config->mode, "udp") || 
                        !strcmp(syslog_config->socket_config->mode, "tcp")){
                m_assert(syslog_config->socket_config->listen, "listen is not set");
                m_assert(syslog_config->socket_config->port, "port is not set");
                if(flb_input_set(ctx, p_file_info->flb_input, 
                    "Tag", tag_s,
                    "Parser", parser_name,
                    "Mode", syslog_config->socket_config->mode,
                    "Listen", syslog_config->socket_config->listen,
                    "Port", syslog_config->socket_config->port,
                    NULL) != 0) return FLB_INPUT_SET_ERROR;
            } else return FLB_INPUT_SET_ERROR; // should never reach this line

            break;
        }
        case FLB_SERIAL: {
            debug_log( "Setting up FLB_SERIAL collector");

            Flb_serial_config_t *serial_config = (Flb_serial_config_t *) p_file_info->flb_config;
            if(unlikely(!serial_config || 
                        !serial_config->bitrate || 
                        !*serial_config->bitrate ||
                        !serial_config->min_bytes || 
                        !*serial_config->min_bytes || 
                        !p_file_info->filename)) return CONFIG_READ_ERROR;
        
            /* Set up serial input */
            p_file_info->flb_input = flb_input(ctx, "serial", NULL);
            if(p_file_info->flb_input < 0 ) return FLB_INPUT_ERROR;
            if(flb_input_set(ctx, p_file_info->flb_input, 
                "Tag", tag_s,
                "File", p_file_info->filename,
                "Bitrate", serial_config->bitrate,
                "Min_Bytes", serial_config->min_bytes,
                "Separator", serial_config->separator,
                "Format", serial_config->format,
                NULL) != 0) return FLB_INPUT_SET_ERROR;
            
            break;
        }
        case FLB_MQTT: {
            debug_log( "Setting up FLB_MQTT collector");

            Flb_socket_config_t *socket_config = (Flb_socket_config_t *) p_file_info->flb_config;
            if(unlikely(!socket_config || !socket_config->listen || !*socket_config->listen ||
                        !socket_config->port || !*socket_config->port)) return CONFIG_READ_ERROR;
        
            /* Set up MQTT input */
            p_file_info->flb_input = flb_input(ctx, "mqtt", NULL);
            if(p_file_info->flb_input < 0 ) return FLB_INPUT_ERROR;
            if(flb_input_set(ctx, p_file_info->flb_input, 
                "Tag", tag_s,
                "Listen", socket_config->listen,
                "Port", socket_config->port,
                NULL) != 0) return FLB_INPUT_SET_ERROR;
            
            break;
        }
        default: {
            m_assert(0, "default: case in flb_add_input() error");
            return DEFAULT_ERROR; // Shouldn't reach here
        }
    }

    /* Set up user-configured outputs */
    for(Flb_output_config_t *output = p_file_info->flb_outputs; output; output = output->next){
        debug_log( "setting up user output [%s]", output->plugin);

        int out = flb_output(ctx, output->plugin, NULL);
        if(out < 0) return FLB_OUTPUT_ERROR;
        if(flb_output_set(ctx, out, 
            "Match", tag_s,
            NULL) != 0) return FLB_OUTPUT_SET_ERROR; 
        for(struct flb_output_config_param *param = output->param; param; param = param->next){
            debug_log( "setting up param [%s][%s] of output [%s]", param->key, param->val, output->plugin);
            if(flb_output_set(ctx, out, 
                param->key, param->val,
                NULL) != 0) return FLB_OUTPUT_SET_ERROR; 
        }
    }

    /* Set up "lib" output */
    struct flb_lib_out_cb *callback = mallocz(sizeof(struct flb_lib_out_cb));
    callback->cb = flb_collect_logs_cb;
    callback->data = p_file_info;
    if(((p_file_info->flb_lib_output = flb_output(ctx, "lib", callback)) < 0) ||
        (flb_output_set(ctx, p_file_info->flb_lib_output, "Match", tag_s, NULL) != 0)){
        freez(callback);
        return FLB_OUTPUT_ERROR;
    }
        
    return SUCCESS;
}

/**
 * @brief Add a Fluent-Bit Forward input.
 * @details This creates a unix or network socket to accept logs using 
 * Fluent Bit's Forward protocol. For more information see:
 * https://docs.fluentbit.io/manual/pipeline/inputs/forward
 * @param[in] forward_in_config Configuration of the Forward input socket.
 * @return 0 on success, -1 on error.
 */
int flb_add_fwd_input(Flb_socket_config_t *forward_in_config){
    
    if(forward_in_config == NULL){
        debug_log( "forward: forward_in_config is NULL");
        collector_info("forward_in_config is NULL");
        return 0;
    }

    do{
        debug_log( "forward: Setting up flb_add_fwd_input()");

        int input, output;
        
        if((input = flb_input(ctx, "forward", NULL)) < 0) break;

        if( forward_in_config->unix_path && *forward_in_config->unix_path && 
            forward_in_config->unix_perm && *forward_in_config->unix_perm){
            if(flb_input_set(ctx, input, 
                "Tag_Prefix", "fwd",
                "Unix_Path", forward_in_config->unix_path,
                "Unix_Perm", forward_in_config->unix_perm,
                NULL) != 0) break;
        } else if( forward_in_config->listen && *forward_in_config->listen &&
                   forward_in_config->port && *forward_in_config->port){
            if(flb_input_set(ctx, input, 
                "Tag_Prefix", "fwd",
                "Listen", forward_in_config->listen,
                "Port", forward_in_config->port,
                NULL) != 0) break;
        } else break; // should never reach this line

        fwd_input_out_cb = mallocz(sizeof(struct flb_lib_out_cb));

        /* Set up output */
        fwd_input_out_cb->cb = flb_collect_logs_cb;
        fwd_input_out_cb->data = NULL;
        if((output = flb_output(ctx, "lib", fwd_input_out_cb)) < 0) break;
        if(flb_output_set(ctx, output, 
            "Match", "fwd*",
            NULL) != 0) break; 

        debug_log( "forward: Set up flb_add_fwd_input() with success");
        return 0;
    } while(0);

    /* Error */
    if(fwd_input_out_cb) freez(fwd_input_out_cb);
    return -1;
}

void flb_free_fwd_input_out_cb(void){
    freez(fwd_input_out_cb);
}