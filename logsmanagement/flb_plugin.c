/** @file flb_plugin.c
 *  @brief This file includes functions to use the Fluent Bit library
 *
 *  @author Dimitris Pantazis
 */

#include "flb_plugin.h"
#include <lz4.h>
#include "helper.h"
#include "circular_buffer.h"
#include "daemon/common.h"
#include "../fluent-bit/lib/msgpack-c/include/msgpack/unpack.h"
#include "../fluent-bit/lib/monkey/include/monkey/mk_core/mk_list.h"
#include "../fluent-bit/include/fluent-bit/flb_macros.h"
#include <dlfcn.h>

#define LOG_REC_KEY "msg" /**< key to represent log message field **/
#define SYSLOG_TIMESTAMP_SIZE 16
#define UNKNOWN "unknown"


extern uv_loop_t *main_loop; 

/* Following structs are the same as defined in fluent-bit/flb_lib.h and fluent-bit/flb_time.h */

struct flb_time {
    struct timespec tm;
};

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

static flb_ctx_t *ctx;
static flb_ctx_t *(*flb_create)(void);
static int (*flb_service_set)(flb_ctx_t *ctx, ...);
static int (*flb_start)(flb_ctx_t *ctx);
static int (*flb_stop)(flb_ctx_t *ctx);
static void (*flb_destroy)(flb_ctx_t *ctx);
static int (*flb_time_pop_from_msgpack)(struct flb_time *time, msgpack_unpacked *upk, msgpack_object **map);
static int (*flb_lib_free)(void *data);
static struct flb_parser *(*flb_parser_create)(const char *name, const char *format, const char *p_regex,int skip_empty,
                                               const char *time_fmt, const char *time_key, const char *time_offset,
                                               int time_keep, int time_strict, struct flb_parser_types *types,
                                               int types_len, struct mk_list *decoders, struct flb_config *config);
static int (*flb_input)(flb_ctx_t *ctx, const char *input, void *data);
static int (*flb_input_set)(flb_ctx_t *ctx, int ffd, ...);
static int (*flb_output)(flb_ctx_t *ctx, const char *output, struct flb_lib_out_cb *cb);
static int (*flb_output_set)(flb_ctx_t *ctx, int ffd, ...);
static msgpack_unpack_return (*dl_msgpack_unpack_next)(msgpack_unpacked* result, const char* data, size_t len, size_t* off);
static void (*dl_msgpack_zone_free)(msgpack_zone* zone);

// TODO: Update "flush", "0.1" according to minimum update every?
int flb_init(void){

    /* Load Fluent-Bit functions from the shared library */
    void *handle;
    char *dl_error;

    char *flb_lib_path = strdupz_path_subpath(netdata_configured_stock_config_dir, "/../libfluent-bit.so");
    handle = dlopen(flb_lib_path, RTLD_LAZY);
    freez(flb_lib_path);
    if (unlikely(!handle)){
        if (likely((dl_error = dlerror()) != NULL)) error("dlopen() libfluent-bit.so error: %s", dl_error);
        m_assert(handle, "dlopen() libfluent-bit.so error");
        return -1;
    }

    dlerror();    /* Clear any existing error */

    *(void **) (&flb_create) = dlsym(handle, "flb_create");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading flb_create: %s", dl_error);
        return -1;
    }
    *(void **) (&flb_service_set) = dlsym(handle, "flb_service_set");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading flb_service_set: %s", dl_error);
        return -1;
    }
    *(void **) (&flb_start) = dlsym(handle, "flb_start");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading flb_start: %s", dl_error);
        return -1;
    }
    *(void **) (&flb_stop) = dlsym(handle, "flb_stop");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading flb_stop: %s", dl_error);
        return -1;
    }
    *(void **) (&flb_destroy) = dlsym(handle, "flb_destroy");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading flb_destroy: %s", dl_error);
        return -1;
    }   
    *(void **) (&flb_time_pop_from_msgpack) = dlsym(handle, "flb_time_pop_from_msgpack");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading flb_time_pop_from_msgpack: %s", dl_error);
        return -1;
    } 
    *(void **) (&flb_lib_free) = dlsym(handle, "flb_lib_free");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading flb_lib_free: %s", dl_error);
        return -1;
    }
    *(void **) (&flb_input) = dlsym(handle, "flb_input");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading flb_input: %s", dl_error);
        return -1;
    }  
    *(void **) (&flb_parser_create) = dlsym(handle, "flb_parser_create");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading flb_parser_create: %s", dl_error);
        return -1;
    } 
    *(void **) (&flb_input_set) = dlsym(handle, "flb_input_set");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading flb_input_set: %s", dl_error);
        return -1;
    } 
    *(void **) (&flb_output) = dlsym(handle, "flb_output");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading flb_output: %s", dl_error);
        return -1;
    } 
    *(void **) (&flb_output_set) = dlsym(handle, "flb_output_set");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading flb_output_set: %s", dl_error);
        return -1;
    } 
    *(void **) (&dl_msgpack_unpack_next) = dlsym(handle, "msgpack_unpack_next");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading msgpack_unpack_next: %s", dl_error);
        return -1;
    } 
    *(void **) (&dl_msgpack_zone_free) = dlsym(handle, "msgpack_zone_free");
    if ((dl_error = dlerror()) != NULL) {
        error("dlerror loading msgpack_zone_free: %s", dl_error);
        return -1;
    } 


    ctx = flb_create();
    if (unlikely(!ctx)) return -1;

    /* Global service settings */
    if(unlikely(flb_service_set(ctx,
        "flush", "0.1", 
        NULL) != 0 )) return -1; 

    return 0;
}

int flb_run(void){
    if (likely(flb_start(ctx)) == 0) return 0;
    else return -1;
}

void flb_stop_and_cleanup(void){
    flb_stop(ctx);
    flb_destroy(ctx);
}

void flb_tmp_buff_cpy_timer_cb(uv_timer_t *handle) {

    struct File_info *p_file_info = handle->data;
    Circ_buff_t *buff = p_file_info->circ_buff;
    
    uv_mutex_lock(&p_file_info->flb_tmp_buff_mut);
    if(!buff->in->data || !*buff->in->data || !buff->in->text_size){ // Nothing to do, just return
        uv_mutex_unlock(&p_file_info->flb_tmp_buff_mut);
        return; 
    }

    m_assert(buff->in->timestamp, "buff->in->timestamp cannot be 0");
    m_assert(buff->in->data, "buff->in->text cannot be NULL");
    m_assert(*buff->in->data, "*buff->in->text cannot be 0");
    m_assert(buff->in->text_size, "buff->in->text_size cannot be 0");

    /* Replace last '\n' with '\0' to null-terminate text */
    buff->in->data[buff->in->text_size - 1] = '\0'; 

    /* Store status (timestamp and text_size must have already been 
     * stored during flb_write_to_buff_cb() ). */
    buff->in->status = CIRC_BUFF_ITEM_STATUS_UNPROCESSED;

    /* Load max size of compressed buffer, as calculated previously */
    size_t text_compressed_buff_max_size = buff->in->text_compressed_size;

    /* Do compression */
    buff->in->text_compressed = buff->in->data + buff->in->text_size;
    buff->in->text_compressed_size = LZ4_compress_fast( buff->in->data, buff->in->text_compressed, 
                                                            buff->in->text_size, text_compressed_buff_max_size, 
                                                            p_file_info->compression_accel);
    m_assert(buff->in->text_compressed_size != 0, "Text_compressed_size should be != 0");

    // TODO: Validate compression option?

    circ_buff_insert(buff);

    /* Extract systemd, syslog and docker events metrics */
    if(p_file_info->log_type == FLB_SYSTEMD || p_file_info->log_type == FLB_SYSLOG) {
        uv_mutex_lock(p_file_info->parser_metrics_mut);
        p_file_info->parser_metrics->num_lines_total += p_file_info->flb_tmp_systemd_metrics.num_lines;
        p_file_info->parser_metrics->num_lines_rate = p_file_info->flb_tmp_systemd_metrics.num_lines;
        p_file_info->flb_tmp_systemd_metrics.num_lines = 0;
        for(int i = 0; i < SYSLOG_SEVER_ARR_SIZE; i++){
            p_file_info->parser_metrics->systemd->sever[i] = p_file_info->flb_tmp_systemd_metrics.sever[i];
            p_file_info->flb_tmp_systemd_metrics.sever[i] = 0;
        }
        for(int i = 0; i < SYSLOG_FACIL_ARR_SIZE; i++){
            p_file_info->parser_metrics->systemd->facil[i] = p_file_info->flb_tmp_systemd_metrics.facil[i];
            p_file_info->flb_tmp_systemd_metrics.facil[i] = 0;
        }
        for(int i = 0; i < SYSLOG_PRIOR_ARR_SIZE; i++){
            p_file_info->parser_metrics->systemd->prior[i] = p_file_info->flb_tmp_systemd_metrics.prior[i];
            p_file_info->flb_tmp_systemd_metrics.prior[i] = 0;
        }
        uv_mutex_unlock(p_file_info->parser_metrics_mut);
    } else if(p_file_info->log_type == FLB_DOCKER_EV) {
        uv_mutex_lock(p_file_info->parser_metrics_mut);
        p_file_info->parser_metrics->num_lines_total += p_file_info->flb_tmp_docker_ev_metrics.num_lines;
        p_file_info->parser_metrics->num_lines_rate = p_file_info->flb_tmp_docker_ev_metrics.num_lines;
        p_file_info->flb_tmp_docker_ev_metrics.num_lines = 0;
        for(int i = 0; i < NUM_OF_DOCKER_EV_TYPES; i++){
            p_file_info->parser_metrics->docker_ev->ev_type[i] = p_file_info->flb_tmp_docker_ev_metrics.ev_type[i];
            p_file_info->flb_tmp_docker_ev_metrics.ev_type[i] = 0;
        }
        uv_mutex_unlock(p_file_info->parser_metrics_mut);
    } 

    buff->in->timestamp = 0;
    buff->in->text_size = 0;
    // *buff->in->data = 0;

    uv_mutex_unlock(&p_file_info->flb_tmp_buff_mut);

    /* Instruct log parsing and metrics extraction (asynchronously) for web logs
     * and any custom charts */
    uv_mutex_lock(&p_file_info->notify_parser_thread_mut);
    p_file_info->log_batches_to_be_parsed++;
    uv_cond_signal(&p_file_info->notify_parser_thread_cond);
    uv_mutex_unlock(&p_file_info->notify_parser_thread_mut);
}

static int flb_write_to_buff_cb(void *record, size_t size, void *data){

    struct File_info *p_file_info = (struct File_info *) data;
    Circ_buff_t *buff = p_file_info->circ_buff;

    msgpack_unpacked result;
    size_t off = 0; 
    struct flb_time tmp_time;
    msgpack_object *x;   

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
    static struct Docker_ev_attr {
        char **key;
        char **val;
        size_t *key_size; 
        size_t *val_size;
        int size, max_size;
    } docker_ev_attr = {0};
    docker_ev_attr.size = 0;
    /* FLB_DOCKER_EV case end */

    size_t new_tmp_text_size = 0;

    msgpack_unpacked_init(&result);

    uv_mutex_lock(&p_file_info->flb_tmp_buff_mut);

    int iter = 0;
    while (dl_msgpack_unpack_next(&result, record, size, &off) == MSGPACK_UNPACK_SUCCESS) {
        iter++;
        m_assert(iter == 1, "We do not expect more than one loop iteration here");

        flb_time_pop_from_msgpack(&tmp_time, &result, &x);
        if(buff->in->timestamp == 0) {
            // printf("timestamp text_size:%zu\n", buff->in->text_size);
            // printf("timestamp text:%s\n", buff->in->text ? buff->in->text : "NULL");
            // fflush(stdout);
            m_assert(buff->in->text_size == 0, "buff->in->timestamp == 0 but buff->in->text_size != 0");
            // m_assert(buff->in->data == 0 || 
            //         *buff->in->data == 0, "buff->in->timestamp == 0 but *buff->in->text != 0");

            buff->in->timestamp = (msec_t) tmp_time.tm.tv_sec * MSEC_PER_SEC + (msec_t) tmp_time.tm.tv_nsec / (NSEC_PER_MSEC);
            m_assert(TEST_MS_TIMESTAMP_VALID(buff->in->timestamp), "buff->in->timestamp is invalid"); // Timestamp within valid range up to 2050
        }

        if(likely(x->type == MSGPACK_OBJECT_MAP)){
            if(likely(x->via.map.size != 0)) {
                msgpack_object_kv* p = x->via.map.ptr;
                msgpack_object_kv* const pend = x->via.map.ptr + x->via.map.size;
                do{
                    // if (unlikely(p->key.type != MSGPACK_OBJECT_STR || p->val.type != MSGPACK_OBJECT_STR)) {
                    //     m_assert(0, "Remaining logs?");
                    //     break;
                    // }
                    
                    m_assert(buff->in->timestamp, "buff->in->timestamp is 0");

                    /* FLB_GENERIC, FLB_WEB_LOG and FLB_SERIAL case */
                    if((p_file_info->log_type == FLB_GENERIC || 
                        p_file_info->log_type == FLB_WEB_LOG || 
                        p_file_info->log_type == FLB_SERIAL) && 
                       !strncmp(p->key.via.str.ptr, LOG_REC_KEY, (size_t) p->key.via.str.size)){

                        char *text = (char *) p->val.via.str.ptr;
                        size_t text_size = p->val.via.str.size;

                        m_assert(text_size, "text_size is 0");
                        m_assert(text, "text is NULL");

                        new_tmp_text_size = buff->in->text_size + text_size + 1; // +1 for '\n'
                        if(unlikely(!circ_buff_prepare_write(buff, new_tmp_text_size))) goto skip_collect_and_drop_logs;

                        memcpy(&buff->in->data[buff->in->text_size], text, text_size);
                        buff->in->text_size = new_tmp_text_size;
                        buff->in->data[buff->in->text_size - 1] = '\n';

                        ++p;
                        continue;
                    }
                    /* FLB_GENERIC, FLB_WEB_LOG and FLB_SERIAL case end */

                    /* FLB_SYSTEMD or FLB_SYSLOG case */
                    if(p_file_info->log_type == FLB_SYSTEMD || p_file_info->log_type == FLB_SYSLOG){
                        if(!new_tmp_text_size){
                            /* set new_tmp_text_size to previous size of buffer, do only once */
                            new_tmp_text_size = buff->in->text_size; 
                        }
                        if(!strncmp(p->key.via.str.ptr, "PRIVAL", (size_t) p->key.via.str.size)){
                            m_assert(p->val.via.str.size <= 3, "p->val.via.str.size > 3");
                            strncpy(syslog_prival, p->val.via.str.ptr, (size_t) p->val.via.str.size);
                            syslog_prival[p->val.via.str.size] = '\0';
                            syslog_prival_size = (size_t) p->val.via.str.size;
                            
                            m_assert(syslog_prival, "syslog_prival is NULL");
                        }
                        else if(!strncmp(p->key.via.str.ptr, "PRIORITY", (size_t) p->key.via.str.size)){
                            m_assert(p->val.via.str.size <= 1, "p->val.via.str.size > 1");
                            strncpy(syslog_severity, p->val.via.str.ptr, (size_t) p->val.via.str.size);
                            syslog_severity[p->val.via.str.size] = '\0';
                            
                            m_assert(syslog_severity, "syslog_severity is NULL");
                        }
                        else if(!strncmp(p->key.via.str.ptr, "SYSLOG_FACILITY", (size_t) p->key.via.str.size)){
                            m_assert(p->val.via.str.size <= 2, "p->val.via.str.size > 2");
                            strncpy(syslog_facility, p->val.via.str.ptr, (size_t) p->val.via.str.size);
                            syslog_facility[p->val.via.str.size] = '\0';
                            
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

                            m_assert(syslog_identifier, "syslog_identifier is NULL");
                            m_assert(syslog_identifier_size, "syslog_identifier_size is 0");

                            new_tmp_text_size += syslog_identifier_size;
                        }
                        else if(!strncmp(p->key.via.str.ptr, "PID", (size_t) p->key.via.str.size)){
                            pid = (char *) p->val.via.str.ptr;
                            pid_size = p->val.via.str.size;

                            m_assert(pid, "pid is NULL");
                            m_assert(pid_size, "pid_size is 0");

                            new_tmp_text_size += pid_size;
                        }
                        else if(!strncmp(p->key.via.str.ptr, "MESSAGE", (size_t) p->key.via.str.size)){
                            message = (char *) p->val.via.str.ptr;
                            message_size = p->val.via.str.size;

                            m_assert(message, "message is NULL");
                            m_assert(message_size, "message_size is 0");

                            new_tmp_text_size += message_size;
                        }
                        ++p;
                        continue;
                    }
                    /* FLB_SYSTEMD or FLB_SYSLOG case end */

                    /* FLB_DOCKER_EV case */
                    if(p_file_info->log_type == FLB_DOCKER_EV){ 

                        if(!new_tmp_text_size){
                            /* set new_tmp_text_size to previous size of buffer, do only once */
                            new_tmp_text_size = buff->in->text_size; 
                        }
                        if(!strncmp(p->key.via.str.ptr, "time", (size_t) p->key.via.str.size)){
                            docker_ev_time = p->val.via.i64;

                            m_assert(docker_ev_time, "docker_ev_time is 0");

                            // debug(D_LOGS_MANAG,"docker_ev_time: %ld", p->val.via.i64);
                        }
                        else if(!strncmp(p->key.via.str.ptr, "timeNano", (size_t) p->key.via.str.size)){
                            docker_ev_timeNano = p->val.via.i64;

                            m_assert(docker_ev_timeNano, "docker_ev_timeNano is 0");
                        }
                        else if(!strncmp(p->key.via.str.ptr, "Type", (size_t) p->key.via.str.size)){
                            docker_ev_type = (char *) p->val.via.str.ptr;
                            docker_ev_type_size = p->val.via.str.size;

                            m_assert(docker_ev_type, "docker_ev_type is NULL");
                            m_assert(docker_ev_type_size, "docker_ev_type_size is 0");

                            // debug(D_LOGS_MANAG,"docker_ev_type: %.*s", docker_ev_type_size, docker_ev_type);
                        }
                        else if(!strncmp(p->key.via.str.ptr, "Action", (size_t) p->key.via.str.size)){
                            docker_ev_action = (char *) p->val.via.str.ptr;
                            docker_ev_action_size = p->val.via.str.size;

                            m_assert(docker_ev_action, "docker_ev_action is NULL");
                            m_assert(docker_ev_action_size, "docker_ev_action_size is 0");

                            // debug(D_LOGS_MANAG,"docker_ev_action: %.*s", docker_ev_action_size, docker_ev_action);
                        }
                        else if(!strncmp(p->key.via.str.ptr, "id", (size_t) p->key.via.str.size)){
                            docker_ev_id = (char *) p->val.via.str.ptr;
                            docker_ev_id_size = p->val.via.str.size;

                            m_assert(docker_ev_id, "docker_ev_id is NULL");
                            m_assert(docker_ev_id_size, "docker_ev_id_size is 0");

                            // debug(D_LOGS_MANAG,"docker_ev_id: %.*s", docker_ev_id_size, docker_ev_id);
                        }
                        else if(!strncmp(p->key.via.str.ptr, "Actor", (size_t) p->key.via.str.size)){
                            if(likely(p->val.type == MSGPACK_OBJECT_MAP && p->val.via.map.size != 0)){
                                msgpack_object_kv* ac = p->val.via.map.ptr;
                                msgpack_object_kv* const ac_pend= p->val.via.map.ptr + p->val.via.map.size;
                                do{
                                    if(!strncmp(ac->key.via.str.ptr, "ID", (size_t) ac->key.via.str.size)){
                                        docker_ev_id = (char *) ac->val.via.str.ptr;
                                        docker_ev_id_size = ac->val.via.str.size;

                                        m_assert(docker_ev_id, "docker_ev_id is NULL");
                                        m_assert(docker_ev_id_size, "docker_ev_id_size is 0");

                                        // debug(D_LOGS_MANAG,"docker_ev_id: %.*s", docker_ev_id_size, docker_ev_id);
                                    }
                                    else if(!strncmp(ac->key.via.str.ptr, "Attributes", (size_t) ac->key.via.str.size)){
                                        if(likely(ac->val.type == MSGPACK_OBJECT_MAP && ac->val.via.map.size != 0)){
                                            msgpack_object_kv* att = ac->val.via.map.ptr;
                                            msgpack_object_kv* const att_pend = ac->val.via.map.ptr + ac->val.via.map.size;
                                            do{
                                                if(unlikely(++docker_ev_attr.size > docker_ev_attr.max_size)){
                                                    docker_ev_attr.max_size = docker_ev_attr.size;
                                                    docker_ev_attr.key = reallocz(docker_ev_attr.key, 
                                                                                docker_ev_attr.max_size * sizeof(char*));
                                                    docker_ev_attr.val = reallocz(docker_ev_attr.val, 
                                                                                docker_ev_attr.max_size * sizeof(char*));   
                                                    docker_ev_attr.key_size = reallocz(docker_ev_attr.key_size, 
                                                                                docker_ev_attr.max_size * sizeof(size_t));    
                                                    docker_ev_attr.val_size = reallocz(docker_ev_attr.val_size, 
                                                                                docker_ev_attr.max_size * sizeof(size_t));                                               
                                                }

                                                docker_ev_attr.key[docker_ev_attr.size - 1] =  (char *) att->key.via.str.ptr;
                                                docker_ev_attr.val[docker_ev_attr.size - 1] =  (char *) att->val.via.str.ptr;
                                                docker_ev_attr.key_size[docker_ev_attr.size - 1] = att->key.via.str.size;
                                                docker_ev_attr.val_size[docker_ev_attr.size - 1] = att->val.via.str.size;

                                                debug(D_LOGS_MANAG, "att_key:%.*s, att_val:%.*s", 
                                                (int) docker_ev_attr.key_size[docker_ev_attr.size - 1], 
                                                docker_ev_attr.key[docker_ev_attr.size - 1],
                                                (int) docker_ev_attr.val_size[docker_ev_attr.size - 1], 
                                                docker_ev_attr.val[docker_ev_attr.size - 1]);

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
                        ++p;
                        continue;
                    }
                    /* FLB_DOCKER_EV case end */

                } while(p < pend);
            }
        } 
    }
    
    /* FLB_SYSTEMD or FLB_SYSLOG case - extract metrics and reconstruct log message */
    if(p_file_info->log_type == FLB_SYSTEMD || p_file_info->log_type == FLB_SYSLOG){

        /* Parse number of log lines */
        p_file_info->flb_tmp_systemd_metrics.num_lines++;

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
                    p_file_info->flb_tmp_systemd_metrics.sever[syslog_severity_d]++;
                } // else parsing errors ++ ??
            } else p_file_info->flb_tmp_systemd_metrics.sever[SYSLOG_SEVER_ARR_SIZE - 1]++; // 'unknown'

            /* Parse syslog_facility char* field into int and extract metrics. 
            * syslog_facility_s will consist of up to 2 chars (plus '\0'), 
            * see https://datatracker.ietf.org/doc/html/rfc5424#section-6.2.1 */
            if(likely(syslog_facility[0])){
                if(likely(str2int(&syslog_facility_d, syslog_facility, 10) == STR2XX_SUCCESS)){
                    p_file_info->flb_tmp_systemd_metrics.facil[syslog_facility_d]++;
                } // else parsing errors ++ ??
            } else p_file_info->flb_tmp_systemd_metrics.facil[SYSLOG_FACIL_ARR_SIZE - 1]++; // 'unknown'

            if(likely(syslog_severity[0] && syslog_facility[0])){
                /* Definition of syslog priority value == facility * 8 + severity */
                syslog_prival_d = syslog_facility_d * 8 + syslog_severity_d; 
                syslog_prival_size = snprintfz(syslog_prival, 4, "%d", syslog_prival_d);
                m_assert(syslog_prival_size < 4 && syslog_prival_size > 0, "error with snprintf()");
        
                new_tmp_text_size += syslog_prival_size + 2; // +2 for '<' and '>'

                p_file_info->flb_tmp_systemd_metrics.prior[syslog_prival_d]++;
            } else {
                new_tmp_text_size += 3; // +3 for "<->" string
                p_file_info->flb_tmp_systemd_metrics.prior[SYSLOG_PRIOR_ARR_SIZE - 1]++; // 'unknown'
            } 

        } else if(p_file_info->log_type == FLB_SYSLOG){

            if(likely(syslog_prival[0])){            
                if(likely(str2int(&syslog_prival_d, syslog_prival, 10) == STR2XX_SUCCESS)){
                    syslog_severity_d = syslog_prival_d % 8;
                    syslog_facility_d = syslog_prival_d / 8;

                    p_file_info->flb_tmp_systemd_metrics.prior[syslog_prival_d]++;
                    p_file_info->flb_tmp_systemd_metrics.sever[syslog_severity_d]++;
                    p_file_info->flb_tmp_systemd_metrics.facil[syslog_facility_d]++;

                    new_tmp_text_size += syslog_prival_size + 2; // +2 for '<' and '>'

                } // else parsing errors ++ ??
            } else {
                new_tmp_text_size += 3; // +3 for "<->" string
                p_file_info->flb_tmp_systemd_metrics.prior[SYSLOG_PRIOR_ARR_SIZE - 1]++; // 'unknown'
                p_file_info->flb_tmp_systemd_metrics.sever[SYSLOG_SEVER_ARR_SIZE - 1]++; // 'unknown'
                p_file_info->flb_tmp_systemd_metrics.facil[SYSLOG_FACIL_ARR_SIZE - 1]++; // 'unknown'
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
        if(unlikely(!circ_buff_prepare_write(buff, new_tmp_text_size))) goto skip_collect_and_drop_logs;

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
            memcpy(&buff->in->data[tmp_item_off], message, message_size);
            tmp_item_off += message_size;  
        }

        buff->in->data[tmp_item_off++] = '\n';
        m_assert(tmp_item_off == new_tmp_text_size, "tmp_item_off should be == new_tmp_text_size");
        buff->in->text_size = new_tmp_text_size;
    }
    /* FLB_SYSTEMD or FLB_SYSLOG case end */

    /* FLB_DOCKER_EV case */
    if(p_file_info->log_type == FLB_DOCKER_EV){

        /* Extract docker events metrics */
        p_file_info->flb_tmp_docker_ev_metrics.num_lines++;

        const size_t docker_ev_datetime_size = sizeof "2022-08-26T15:33:20.802840200+0000";
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

            // debug(D_LOGS_MANAG, "docker_time:%s", docker_ev_datetime);
        }

        if(likely(docker_ev_type)){
            // debug(D_LOGS_MANAG,"docker_ev_type: %.*s", (int) docker_ev_type_size, docker_ev_type);
            int i;
            for(i = 0; i < NUM_OF_DOCKER_EV_TYPES - 1; i++){
                if(!strncmp(docker_ev_type, docker_ev_type_string[i], docker_ev_type_size)){
                    p_file_info->flb_tmp_docker_ev_metrics.ev_type[i]++;
                    break;
                }
            }
            if(unlikely(i >= NUM_OF_DOCKER_EV_TYPES - 1)){
                p_file_info->flb_tmp_docker_ev_metrics.ev_type[i]++; // 'unknown'
            }

            new_tmp_text_size += docker_ev_type_size + 1; // +1 for ' ' char
        }

        if(likely(docker_ev_action)){
            debug(D_LOGS_MANAG,"docker_ev_action: %.*s", (int) docker_ev_action_size, docker_ev_action);
        //     int i;
        //     for(i = 0; i < NUM_OF_DOCKER_EV_TYPES - 1; i++){
        //         if(!strncmp(docker_ev_action, docker_ev_action_string[i], docker_ev_action_size)){
        //             p_file_info->flb_tmp_docker_ev_metrics.ev_action[i]++;
        //             break;
        //         }
        //     }
        //     if(unlikely(i >= NUM_OF_DOCKER_EV_TYPES - 1)){
        //         p_file_info->flb_tmp_docker_ev_metrics.ev_action[i]++; // 'unknown'
        //     }

            new_tmp_text_size += docker_ev_action_size + 1; // +1 for ' ' char
        }

        if(likely(docker_ev_id)){
            debug(D_LOGS_MANAG,"docker_ev_id: %.*s", (int) docker_ev_id_size, docker_ev_id);
        //     int i;
        //     for(i = 0; i < NUM_OF_DOCKER_EV_TYPES - 1; i++){
        //         if(!strncmp(docker_ev_action, docker_ev_action_string[i], docker_ev_action_size)){
        //             p_file_info->flb_tmp_docker_ev_metrics.ev_action[i]++;
        //             break;
        //         }
        //     }
        //     if(unlikely(i >= NUM_OF_DOCKER_EV_TYPES - 1)){
        //         p_file_info->flb_tmp_docker_ev_metrics.ev_action[i]++; // 'unknown'
        //     }

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

        new_tmp_text_size += 1; // +1 fpr '\n' character at the end
        
        /* Metrics extracted, now prepare circular buffer for write */
        // TODO: Fix: Metrics will still be collected if circ_buff_prepare_write() returns 0.
        if(unlikely(!circ_buff_prepare_write(buff, new_tmp_text_size))) goto skip_collect_and_drop_logs;

        size_t tmp_item_off = buff->in->text_size;

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
    }
    /* FLB_DOCKER_EV case end */

skip_collect_and_drop_logs:
    /* Following code is equivalent to msgpack_unpacked_destroy(&result) due 
     * to that function call being unavailable when using dl_open() */
    if(result.zone != NULL) {
        dl_msgpack_zone_free(result.zone);
        result.zone = NULL;
        memset(&result.data, 0, sizeof(msgpack_object));
    }

    uv_mutex_unlock(&p_file_info->flb_tmp_buff_mut);
    
    flb_lib_free(record);
    // FLB_OUTPUT_RETURN(FLB_OK); // Watch out! This breaks output - won't flush all pending logs
    return 0;
    
}

/**
 * @brief Add a Fluent-Bit input that outputs to the "lib" Fluent-Bit plugin.
 * @param[in] p_file_info Pointer to the log source struct where the input will
 * be registered to.
 * @return 0 on success, a negative number for any errors.
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

    // TODO: freez(callback) on error
    struct flb_lib_out_cb *callback = mallocz(sizeof(struct flb_lib_out_cb));

    switch(p_file_info->log_type){
        case GENERIC: {
            m_assert(0, "GENERIC case cannot exist in flb_add_input()");
            return INVALID_LOG_TYPE; // log_type cannot be GENERIC here
        }
        case WEB_LOG: {
            m_assert(0, "WEB_LOG case cannot exist in flb_add_input()");
            return INVALID_LOG_TYPE; // log_type cannot be WEB_LOG here
        }
        case FLB_GENERIC:
        case FLB_WEB_LOG: {

            char update_every_str[10]; 
            snprintfz(update_every_str, 10, "%d", p_file_info->update_every);

            debug(D_LOGS_MANAG, "Setting up FLB_WEB_LOG tail for %s (basename:%s)", 
                  p_file_info->filename, p_file_info->file_basename);
        
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
                // TODO: Add configuration option for user, to either use inotify or stat.
                "Inotify_Watcher", "true", 
#endif
                NULL) != 0) return FLB_INPUT_SET_ERROR;

            /* Set up output */
            callback->cb = flb_write_to_buff_cb;
            callback->data = p_file_info;
            p_file_info->flb_output = flb_output(ctx, "lib", callback);
            if(p_file_info->flb_output < 0 ) return FLB_OUTPUT_ERROR;
            if(flb_output_set(ctx, p_file_info->flb_output, 
                "Match", tag_s,
                NULL) != 0) return FLB_OUTPUT_SET_ERROR;

            break;
        }
        case FLB_SYSTEMD: {
            debug(D_LOGS_MANAG, "Setting up FLB_SYSTEMD collector");
        
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
            

            /* Set up output */
            callback->cb = flb_write_to_buff_cb;
            callback->data = p_file_info;
            p_file_info->flb_output = flb_output(ctx, "lib", callback);
            if(p_file_info->flb_output < 0 ) return FLB_OUTPUT_ERROR;
            if(flb_output_set(ctx, p_file_info->flb_output, 
                "Match", tag_s,
                NULL) != 0) return FLB_OUTPUT_SET_ERROR;

            break;
        }
        case FLB_DOCKER_EV: {
            debug(D_LOGS_MANAG, "Setting up FLB_DOCKER_EV collector");

            /* Set up Docker Events parser */
            if(flb_parser_create( "docker_events_parser", "json", NULL,
                FLB_TRUE, NULL, NULL, NULL, FLB_TRUE, FLB_TRUE, NULL, 0,
                NULL, ctx->config) == NULL) return FLB_PARSER_CREATE_ERROR;
        
            /* Set up Docker Events input */
            p_file_info->flb_input = flb_input(ctx, "docker_events", NULL);
            if(p_file_info->flb_input < 0 ) return FLB_INPUT_ERROR;
            if(!strcmp(p_file_info->filename, DOCKER_EV_DEFAULT_PATH)){
                if(flb_input_set(ctx, p_file_info->flb_input, 
                    "Tag", tag_s,
                    "Parser", "docker_events_parser",
                    NULL) != 0) return FLB_INPUT_SET_ERROR;
            } else {
                if(flb_input_set(ctx, p_file_info->flb_input, 
                    "Tag", tag_s,
                    "Parser", "docker_events_parser",
                    "Unix_Path", p_file_info->filename,
                    NULL) != 0) return FLB_INPUT_SET_ERROR;
            }
            
            
            /* Set up output */
            callback->cb = flb_write_to_buff_cb;
            callback->data = p_file_info;
            p_file_info->flb_output = flb_output(ctx, "lib", callback);
            if(p_file_info->flb_output < 0 ) return FLB_OUTPUT_ERROR;
            if(flb_output_set(ctx, p_file_info->flb_output, 
                "Match", tag_s,
                NULL) != 0) return FLB_OUTPUT_SET_ERROR;
            
            break;
        }
        case FLB_SYSLOG: {
            debug(D_LOGS_MANAG, "Setting up FLB_SYSLOG collector");

            /* Set up syslog parser */
            const char syslog_parser_prfx[] = "syslog_parser_";
            size_t parser_name_size = sizeof(syslog_parser_prfx) + tag_max_size - 1;
            char parser_name[parser_name_size];
            snprintfz(parser_name, parser_name_size, "%s%u", syslog_parser_prfx, tag);

            Syslog_parser_config_t *syslog_config = (Syslog_parser_config_t *) p_file_info->parser_config->gen_config;
            if(unlikely(!syslog_config || !syslog_config->mode || !p_file_info->filename)) return CONFIG_READ_ERROR;
            if(flb_parser_create( parser_name, "regex", syslog_config->log_format,
                FLB_TRUE, NULL, NULL, NULL, FLB_TRUE, FLB_TRUE, NULL, 0,
                NULL, ctx->config) == NULL) return FLB_PARSER_CREATE_ERROR;
        
            /* Set up syslog input */
            p_file_info->flb_input = flb_input(ctx, "syslog", NULL);
            if(p_file_info->flb_input < 0 ) return FLB_INPUT_ERROR;
            if(!strcmp(syslog_config->mode, "unix_udp") || !strcmp(syslog_config->mode, "unix_tcp")){
                m_assert(syslog_config->unix_perm, "unix_perm is not set");
                if(flb_input_set(ctx, p_file_info->flb_input, 
                    "Tag", tag_s,
                    "Path", p_file_info->filename,
                    "Parser", parser_name,
                    "Mode", syslog_config->mode,
                    "Unix_Perm", syslog_config->unix_perm,
                    NULL) != 0) return FLB_INPUT_SET_ERROR;
            } else if(!strcmp(syslog_config->mode, "udp") || !strcmp(syslog_config->mode, "tcp")){
                m_assert(syslog_config->listen, "listen is not set");
                m_assert(syslog_config->port, "port is not set");
                if(flb_input_set(ctx, p_file_info->flb_input, 
                    "Tag", tag_s,
                    "Parser", parser_name,
                    "Mode", syslog_config->mode,
                    "Listen", syslog_config->listen,
                    "Port", syslog_config->port,
                    NULL) != 0) return FLB_INPUT_SET_ERROR;
            } else 
            return FLB_INPUT_SET_ERROR; // should never reach this line
            
            
            /* Set up output */
            callback->cb = flb_write_to_buff_cb;
            callback->data = p_file_info;
            p_file_info->flb_output = flb_output(ctx, "lib", callback);
            if(p_file_info->flb_output < 0 ) return FLB_OUTPUT_ERROR;
            if(flb_output_set(ctx, p_file_info->flb_output, 
                "Match", tag_s,
                NULL) != 0) return FLB_OUTPUT_SET_ERROR;

            break;
        }
        case FLB_SERIAL: {
            debug(D_LOGS_MANAG, "Setting up FLB_SERIAL collector");

            Flb_serial_config_t *serial_config = (Flb_serial_config_t *) p_file_info->flb_config;
            if(unlikely(!serial_config || !serial_config->bitrate || !*serial_config->bitrate ||
                        !serial_config->min_bytes || !p_file_info->filename)) return CONFIG_READ_ERROR;
        
            /* Set up serial input */
            p_file_info->flb_input = flb_input(ctx, "serial", NULL);
            if(p_file_info->flb_input < 0 ) return FLB_INPUT_ERROR;
            if(flb_input_set(ctx, p_file_info->flb_input, 
                "Tag", tag_s,
                "File", p_file_info->filename,
                "Bitrate", serial_config->bitrate,
                "Separator", serial_config->separator,
                "Format", serial_config->format,
                NULL) != 0) return FLB_INPUT_SET_ERROR;


            /* Set up output */
            callback->cb = flb_write_to_buff_cb;
            callback->data = p_file_info;
            p_file_info->flb_output = flb_output(ctx, "lib", callback);
            if(p_file_info->flb_output < 0 ) return FLB_OUTPUT_ERROR;
            if(flb_output_set(ctx, p_file_info->flb_output, 
                "Match", tag_s,
                NULL) != 0) return FLB_OUTPUT_SET_ERROR;

            break;
        }
        default: {
            m_assert(0, "default: case in flb_add_input() error");
            return DEFAULT_ERROR; // Shouldn't reach here
            break;
        }
    }    

    return SUCCESS;
}
