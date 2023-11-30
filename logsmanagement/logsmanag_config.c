// SPDX-License-Identifier: GPL-3.0-or-later

/** @file   logsmanag_config.c
 *  @brief  This file includes functions to manage 
 *          the logs management configuration.
 */

#include "logsmanag_config.h"
#include "db_api.h"
#include "rrd_api/rrd_api.h"
#include "helper.h"

g_logs_manag_config_t g_logs_manag_config = {
    .update_every = UPDATE_EVERY,
    .update_timeout = UPDATE_TIMEOUT_DEFAULT,
    .use_log_timestamp = CONFIG_BOOLEAN_AUTO,
    .circ_buff_max_size_in_mib = CIRCULAR_BUFF_DEFAULT_MAX_SIZE / (1 MiB),
    .circ_buff_drop_logs = CIRCULAR_BUFF_DEFAULT_DROP_LOGS,
    .compression_acceleration = COMPRESSION_ACCELERATION_DEFAULT,
    .db_mode = GLOBAL_DB_MODE_DEFAULT,
    .disk_space_limit_in_mib = DISK_SPACE_LIMIT_DEFAULT,  
    .buff_flush_to_db_interval = SAVE_BLOB_TO_DB_DEFAULT,
    .enable_collected_logs_total = ENABLE_COLLECTED_LOGS_TOTAL_DEFAULT,
    .enable_collected_logs_rate = ENABLE_COLLECTED_LOGS_RATE_DEFAULT,
    .sd_journal_field_prefix = SD_JOURNAL_FIELD_PREFIX,
    .do_sd_journal_send = SD_JOURNAL_SEND_DEFAULT
};

static logs_manag_db_mode_t db_mode_str_to_db_mode(const char *const db_mode_str){
    if(!db_mode_str || !*db_mode_str) return g_logs_manag_config.db_mode;
    else if(!strcasecmp(db_mode_str, "full")) return LOGS_MANAG_DB_MODE_FULL;
    else if(!strcasecmp(db_mode_str, "none")) return LOGS_MANAG_DB_MODE_NONE;
    else return g_logs_manag_config.db_mode;
}

static struct config log_management_config = {
    .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = {
            .avl_tree = {
                    .root = NULL,
                    .compar = appconfig_section_compare
            },
            .rwlock = AVL_LOCK_INITIALIZER
    }
};

static struct Chart_meta chart_types[] = {
    {.type = FLB_TAIL,      .init = generic_chart_init,   .update = generic_chart_update},
    {.type = FLB_WEB_LOG,   .init = web_log_chart_init,   .update = web_log_chart_update},
    {.type = FLB_KMSG,      .init = kernel_chart_init,    .update = kernel_chart_update},
    {.type = FLB_SYSTEMD,   .init = systemd_chart_init,   .update = systemd_chart_update},
    {.type = FLB_DOCKER_EV, .init = docker_ev_chart_init, .update = docker_ev_chart_update},
    {.type = FLB_SYSLOG,    .init = generic_chart_init,   .update = generic_chart_update},
    {.type = FLB_SERIAL,    .init = generic_chart_init,   .update = generic_chart_update},
    {.type = FLB_MQTT,      .init = mqtt_chart_init,      .update = mqtt_chart_update}
};

char *get_user_config_dir(void){
    char *dir = getenv("NETDATA_USER_CONFIG_DIR");

    return dir ? dir : CONFIG_DIR;
}

char *get_stock_config_dir(void){
    char *dir = getenv("NETDATA_STOCK_CONFIG_DIR");

    return dir ? dir : LIBCONFIG_DIR;
}

char *get_log_dir(void){
    char *dir = getenv("NETDATA_LOG_DIR");

    return dir ? dir : LOG_DIR;
}

char *get_cache_dir(void){
    char *dir = getenv("NETDATA_CACHE_DIR");

    return dir ? dir : CACHE_DIR;
}

/** 
 * @brief Cleanup p_file_info struct
 * @param p_file_info The struct of File_info type to be cleaned up.
 * @todo  Pass p_file_info by reference, so that it can be set to NULL. */
static void p_file_info_destroy(void *arg){
    struct File_info *p_file_info = (struct File_info *) arg;

    // TODO: Clean up rrd / chart stuff.
    // p_file_info->chart_meta

    if(unlikely(!p_file_info)){
        collector_info("p_file_info_destroy() called but p_file_info == NULL - already destroyed?");
        return;
    }

    char chartname[100];
    snprintfz(chartname, 100, "%s", p_file_info->chartname ? p_file_info->chartname : "Unknown");
    collector_info("[%s]: p_file_info_destroy() cleanup...", chartname);

    __atomic_store_n(&p_file_info->state, LOG_SRC_EXITING, __ATOMIC_RELAXED);

    if(uv_is_active((uv_handle_t *) &p_file_info->flb_tmp_buff_cpy_timer)){
        uv_timer_stop(&p_file_info->flb_tmp_buff_cpy_timer);
        if (!uv_is_closing((uv_handle_t *) &p_file_info->flb_tmp_buff_cpy_timer))
            uv_close((uv_handle_t *) &p_file_info->flb_tmp_buff_cpy_timer, NULL);
    }

    // TODO: Need to do proper termination of DB threads and allocated memory.
    if(p_file_info->db_writer_thread){
        uv_thread_join(p_file_info->db_writer_thread);
        sqlite3_finalize(p_file_info->stmt_get_log_msg_metadata_asc);
        sqlite3_finalize(p_file_info->stmt_get_log_msg_metadata_desc);
        if(sqlite3_close(p_file_info->db) != SQLITE_OK)
            collector_error("[%s]: Failed to close database", chartname);
        freez(p_file_info->db_mut);
        freez((void *) p_file_info->db_metadata);
        freez((void *) p_file_info->db_dir);
        freez(p_file_info->db_writer_thread);
    }

    freez((void *) p_file_info->chartname);
    freez(p_file_info->filename);
    freez((void *) p_file_info->file_basename);
    freez((void *) p_file_info->stream_guid);

    for(int i = 1; i <= BLOB_MAX_FILES; i++){
        if(p_file_info->blob_handles[i]){
            uv_fs_close(NULL, NULL, p_file_info->blob_handles[i], NULL);
            p_file_info->blob_handles[i] = 0;
        }
    }

    if(p_file_info->circ_buff) 
        circ_buff_destroy(p_file_info->circ_buff);
    
    if(p_file_info->parser_metrics){
        switch(p_file_info->log_type){
            case FLB_WEB_LOG: {
                if(p_file_info->parser_metrics->web_log)
                    freez(p_file_info->parser_metrics->web_log);
                break;
            }
            case FLB_KMSG: {
                if(p_file_info->parser_metrics->kernel){
                    dictionary_destroy(p_file_info->parser_metrics->kernel->subsystem);
                    dictionary_destroy(p_file_info->parser_metrics->kernel->device);
                    freez(p_file_info->parser_metrics->kernel);
                }
                break;
            }
            case FLB_SYSTEMD: 
            case FLB_SYSLOG: {
                if(p_file_info->parser_metrics->systemd)
                    freez(p_file_info->parser_metrics->systemd);
                break;
            }
            case FLB_DOCKER_EV: {
                if(p_file_info->parser_metrics->docker_ev)
                    freez(p_file_info->parser_metrics->docker_ev);
                break;
            }
            case FLB_MQTT: {
                if(p_file_info->parser_metrics->mqtt){
                    dictionary_destroy(p_file_info->parser_metrics->mqtt->topic);
                    freez(p_file_info->parser_metrics->mqtt);
                }
                break;
            }
            default:
                break;
        }   

        for(int i = 0; p_file_info->parser_cus_config && 
                       p_file_info->parser_metrics->parser_cus && 
                       p_file_info->parser_cus_config[i]; i++){
            freez(p_file_info->parser_cus_config[i]->chartname);
            freez(p_file_info->parser_cus_config[i]->regex_str);
            freez(p_file_info->parser_cus_config[i]->regex_name);
            regfree(&p_file_info->parser_cus_config[i]->regex);
            freez(p_file_info->parser_cus_config[i]);
            freez(p_file_info->parser_metrics->parser_cus[i]);
        }    

        freez(p_file_info->parser_cus_config);
        freez(p_file_info->parser_metrics->parser_cus);

        freez(p_file_info->parser_metrics);
    }

    if(p_file_info->parser_config){
        freez(p_file_info->parser_config->gen_config);
        freez(p_file_info->parser_config);
    }

    Flb_output_config_t *output_next = p_file_info->flb_outputs;
    while(output_next){
        Flb_output_config_t *output = output_next;
        output_next = output_next->next;

        struct flb_output_config_param *param_next = output->param;
        while(param_next){
            struct flb_output_config_param *param = param_next;
            param_next = param->next;
            freez(param->key);
            freez(param->val);
            freez(param);
        }
        freez(output->plugin);
        freez(output);
    }

    freez(p_file_info->flb_config);
    
    freez(p_file_info);

    collector_info("[%s]: p_file_info_destroy() cleanup done", chartname);
}

void p_file_info_destroy_all(void){
    if(p_file_infos_arr){
        uv_thread_t thread_id[p_file_infos_arr->count];
        for(int i = 0; i < p_file_infos_arr->count; i++){
            fatal_assert(0 == uv_thread_create(&thread_id[i], p_file_info_destroy, p_file_infos_arr->data[i]));
        }
        for(int i = 0; i < p_file_infos_arr->count; i++){
            uv_thread_join(&thread_id[i]);
        }
        freez(p_file_infos_arr);
        p_file_infos_arr = NULL;
    }
}

/**
 * @brief Load logs management configuration.
 * @returns  0 if success, 
 *          -1 if config file not found
 *          -2 if p_flb_srvc_config if is NULL (no flb_srvc_config_t provided)
 */
int logs_manag_config_load( flb_srvc_config_t *p_flb_srvc_config, 
                            Flb_socket_config_t **forward_in_config_p,
                            int g_update_every){
    int rc = LOGS_MANAG_CONFIG_LOAD_ERROR_OK;
    char section[100];
    char temp_path[FILENAME_MAX + 1];

    struct config logsmanagement_d_conf = {
        .first_section = NULL,
        .last_section = NULL,
        .mutex = NETDATA_MUTEX_INITIALIZER,
        .index = {
                .avl_tree = {
                        .root = NULL,
                        .compar = appconfig_section_compare
                },
                .rwlock = AVL_LOCK_INITIALIZER
        }
    };

    char *filename = strdupz_path_subpath(get_user_config_dir(), "logsmanagement.d.conf");
    if(!appconfig_load(&logsmanagement_d_conf, filename, 0, NULL)) {
        collector_info("CONFIG: cannot load user config '%s'. Will try stock config.", filename);
        freez(filename);

        filename = strdupz_path_subpath(get_stock_config_dir(), "logsmanagement.d.conf");
        if(!appconfig_load(&logsmanagement_d_conf, filename, 0, NULL)){
            collector_error("CONFIG: cannot load stock config '%s'. Logs management will be disabled.", filename);
            rc = LOGS_MANAG_CONFIG_LOAD_ERROR_NO_STOCK_CONFIG;
        }
    }
    freez(filename);
    

    /* [global] section */

    snprintfz(section, 100, "global");

    g_logs_manag_config.update_every = appconfig_get_number(
        &logsmanagement_d_conf, 
        section, 
        "update every", 
        g_logs_manag_config.update_every);
    
    g_logs_manag_config.update_every = 
        g_update_every && g_update_every > g_logs_manag_config.update_every ? 
        g_update_every : g_logs_manag_config.update_every;

    g_logs_manag_config.update_timeout = appconfig_get_number(  
        &logsmanagement_d_conf, 
        section, 
        "update timeout", 
        UPDATE_TIMEOUT_DEFAULT);

    if(g_logs_manag_config.update_timeout < g_logs_manag_config.update_every) 
        g_logs_manag_config.update_timeout = g_logs_manag_config.update_every;

    g_logs_manag_config.use_log_timestamp = appconfig_get_boolean_ondemand( 
        &logsmanagement_d_conf,
        section,
        "use log timestamp", 
        g_logs_manag_config.use_log_timestamp);
    
    g_logs_manag_config.circ_buff_max_size_in_mib = appconfig_get_number(   
        &logsmanagement_d_conf,
        section, 
        "circular buffer max size MiB", 
        g_logs_manag_config.circ_buff_max_size_in_mib);
    
    g_logs_manag_config.circ_buff_drop_logs = appconfig_get_boolean(    
        &logsmanagement_d_conf,
        section, 
        "circular buffer drop logs if full", 
        g_logs_manag_config.circ_buff_drop_logs);

    g_logs_manag_config.compression_acceleration = appconfig_get_number(    
        &logsmanagement_d_conf,
        section,
        "compression acceleration", 
        g_logs_manag_config.compression_acceleration);

    g_logs_manag_config.enable_collected_logs_total = appconfig_get_boolean(
        &logsmanagement_d_conf,
        section, 
        "collected logs total chart enable", 
        g_logs_manag_config.enable_collected_logs_total);

    g_logs_manag_config.enable_collected_logs_rate = appconfig_get_boolean(
        &logsmanagement_d_conf,
        section, 
        "collected logs rate chart enable", 
        g_logs_manag_config.enable_collected_logs_rate);

    g_logs_manag_config.do_sd_journal_send = appconfig_get_boolean(    
        &logsmanagement_d_conf,
        section, 
        "submit logs to system journal", 
        g_logs_manag_config.do_sd_journal_send);

    g_logs_manag_config.sd_journal_field_prefix = appconfig_get(
        &logsmanagement_d_conf,
        section,
        "systemd journal fields prefix",
        g_logs_manag_config.sd_journal_field_prefix);
    
    if(!rc){
        collector_info("CONFIG: [%s] update every: %d",                       section,  g_logs_manag_config.update_every);
        collector_info("CONFIG: [%s] update timeout: %d",                     section,  g_logs_manag_config.update_timeout);
        collector_info("CONFIG: [%s] use log timestamp: %d",                  section,  g_logs_manag_config.use_log_timestamp);
        collector_info("CONFIG: [%s] circular buffer max size MiB: %d",       section,  g_logs_manag_config.circ_buff_max_size_in_mib);
        collector_info("CONFIG: [%s] circular buffer drop logs if full: %d",  section,  g_logs_manag_config.circ_buff_drop_logs);
        collector_info("CONFIG: [%s] compression acceleration: %d",           section,  g_logs_manag_config.compression_acceleration);
        collector_info("CONFIG: [%s] collected logs total chart enable: %d",  section,  g_logs_manag_config.enable_collected_logs_total);
        collector_info("CONFIG: [%s] collected logs rate chart enable: %d",   section,  g_logs_manag_config.enable_collected_logs_rate);
        collector_info("CONFIG: [%s] submit logs to system journal: %d",      section,  g_logs_manag_config.do_sd_journal_send);
        collector_info("CONFIG: [%s] systemd journal fields prefix: %s",      section,  g_logs_manag_config.sd_journal_field_prefix);
    }


    /* [db] section */

    snprintfz(section, 100, "db");

    const char *const db_mode_str = appconfig_get(
        &logsmanagement_d_conf,
        section,
        "db mode",
        GLOBAL_DB_MODE_DEFAULT_STR);
    g_logs_manag_config.db_mode = db_mode_str_to_db_mode(db_mode_str);

    snprintfz(temp_path, FILENAME_MAX, "%s" LOGS_MANAG_DB_SUBPATH, get_cache_dir());
    db_set_main_dir(appconfig_get(&logsmanagement_d_conf, section, "db dir", temp_path));

    g_logs_manag_config.buff_flush_to_db_interval = appconfig_get_number(  
        &logsmanagement_d_conf,
        section, 
        "circular buffer flush to db", 
        g_logs_manag_config.buff_flush_to_db_interval);
    
    g_logs_manag_config.disk_space_limit_in_mib = appconfig_get_number(
        &logsmanagement_d_conf,
        section, 
        "disk space limit MiB", 
        g_logs_manag_config.disk_space_limit_in_mib);

    if(!rc){
        collector_info("CONFIG: [%s] db mode: %s [%d]",                 section, db_mode_str, (int) g_logs_manag_config.db_mode);
        collector_info("CONFIG: [%s] db dir: %s",                       section, temp_path);
        collector_info("CONFIG: [%s] circular buffer flush to db: %d",  section, g_logs_manag_config.buff_flush_to_db_interval);
        collector_info("CONFIG: [%s] disk space limit MiB: %d",         section, g_logs_manag_config.disk_space_limit_in_mib);
    }


    /* [forward input] section */

    snprintfz(section, 100, "forward input");

    const int fwd_enable = appconfig_get_boolean(
        &logsmanagement_d_conf, 
        section,
        "enabled", 
        CONFIG_BOOLEAN_NO);
    
    *forward_in_config_p = (Flb_socket_config_t *) callocz(1, sizeof(Flb_socket_config_t));

    (*forward_in_config_p)->unix_path = appconfig_get(
        &logsmanagement_d_conf,
        section, 
        "unix path", 
        FLB_FORWARD_UNIX_PATH_DEFAULT);
    
    (*forward_in_config_p)->unix_perm = appconfig_get(
        &logsmanagement_d_conf, 
        section,
        "unix perm", 
        FLB_FORWARD_UNIX_PERM_DEFAULT);
    
    // TODO: Check if listen is in valid format
    (*forward_in_config_p)->listen = appconfig_get(
        &logsmanagement_d_conf, 
        section,
        "listen", 
        FLB_FORWARD_ADDR_DEFAULT);
    
    (*forward_in_config_p)->port = appconfig_get(
        &logsmanagement_d_conf, 
        section, 
        "port", 
        FLB_FORWARD_PORT_DEFAULT);

    if(!rc){
        collector_info("CONFIG: [%s] enabled: %s",      section, fwd_enable ? "yes" : "no");
        collector_info("CONFIG: [%s] unix path: %s",    section, (*forward_in_config_p)->unix_path);
        collector_info("CONFIG: [%s] unix perm: %s",    section, (*forward_in_config_p)->unix_perm);
        collector_info("CONFIG: [%s] listen: %s",       section, (*forward_in_config_p)->listen);
        collector_info("CONFIG: [%s] port: %s",         section, (*forward_in_config_p)->port);
    }

    if(!fwd_enable) {
        freez(*forward_in_config_p);
        *forward_in_config_p = NULL;
    }


    /* [fluent bit] section */

    snprintfz(section, 100, "fluent bit");

    snprintfz(temp_path, FILENAME_MAX, "%s/%s", get_log_dir(), FLB_LOG_FILENAME_DEFAULT);
    
    if(p_flb_srvc_config){
        p_flb_srvc_config->flush = appconfig_get(
            &logsmanagement_d_conf, 
            section, 
            "flush", 
            p_flb_srvc_config->flush);
        
        p_flb_srvc_config->http_listen = appconfig_get(
            &logsmanagement_d_conf, 
            section, 
            "http listen", 
            p_flb_srvc_config->http_listen);

        p_flb_srvc_config->http_port = appconfig_get(
            &logsmanagement_d_conf, 
            section, 
            "http port", 
            p_flb_srvc_config->http_port);
        
        p_flb_srvc_config->http_server = appconfig_get(
            &logsmanagement_d_conf, 
            section, 
            "http server", 
            p_flb_srvc_config->http_server);
        
        p_flb_srvc_config->log_path = appconfig_get(
            &logsmanagement_d_conf, 
            section, 
            "log file", 
            temp_path);
        
        p_flb_srvc_config->log_level = appconfig_get(
            &logsmanagement_d_conf, 
            section, 
            "log level", 
            p_flb_srvc_config->log_level);
        
        p_flb_srvc_config->coro_stack_size = appconfig_get(
            &logsmanagement_d_conf, 
            section, 
            "coro stack size", 
            p_flb_srvc_config->coro_stack_size);
    }
    else
        rc = LOGS_MANAG_CONFIG_LOAD_ERROR_P_FLB_SRVC_NULL;

    if(!rc){
        collector_info("CONFIG: [%s] flush: %s", section, p_flb_srvc_config->flush);
        collector_info("CONFIG: [%s] http listen: %s", section, p_flb_srvc_config->http_listen);
        collector_info("CONFIG: [%s] http port: %s", section, p_flb_srvc_config->http_port);
        collector_info("CONFIG: [%s] http server: %s", section, p_flb_srvc_config->http_server);
        collector_info("CONFIG: [%s] log file: %s", section, p_flb_srvc_config->log_path);
        collector_info("CONFIG: [%s] log level: %s", section, p_flb_srvc_config->log_level);
        collector_info("CONFIG: [%s] coro stack size: %s", section, p_flb_srvc_config->coro_stack_size);
    }

    return rc;
}

static bool metrics_dict_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused){
    ((metrics_dict_item_t *)old_value)->num_new += ((metrics_dict_item_t *)new_value)->num_new;
    return true;
}

#define FLB_OUTPUT_PLUGIN_NAME_KEY "name"

static int flb_output_param_get_cb(void *entry, void *data){
    struct config_option *option = (struct config_option *) entry;
    Flb_output_config_t *flb_output = (Flb_output_config_t *) data;
    
    char *param_prefix = callocz(1, snprintf(NULL, 0, "output %d", MAX_OUTPUTS_PER_SOURCE) + 1);
    sprintf(param_prefix, "output %d", flb_output->id);
    size_t param_prefix_len = strlen(param_prefix);
    
    if(!strncasecmp(option->name, param_prefix, param_prefix_len)){ // param->name looks like "output 1 host"
        char *param_key = &option->name[param_prefix_len]; // param_key should look like " host"
        while(*param_key == ' ') param_key++; // remove whitespace so it looks like "host"
        
        if(*param_key && strcasecmp(param_key, FLB_OUTPUT_PLUGIN_NAME_KEY)){ // ignore param_key "name" 
            // debug_log( "config_option: name[%s], value[%s]", option->name, option->value);
            // debug_log( "config option kv:[%s][%s]", param_key, option->value);

            struct flb_output_config_param **p = &flb_output->param;
            while((*p) != NULL) p = &((*p)->next); // Go to last param of linked list

            (*p) = callocz(1, sizeof(struct flb_output_config_param));
            (*p)->key = strdupz(param_key);
            (*p)->val = strdupz(option->value);
        }
    }

    freez(param_prefix);

    return 0;
}

/**
 * @brief Initialize logs management based on a section configuration.
 * @note On error, calls p_file_info_destroy() to clean up before returning. 
 * @param config_section Section to read configuration from.
 * @todo How to handle duplicate entries?
 */
static void config_section_init(uv_loop_t *main_loop,
                                struct section *config_section, 
                                Flb_socket_config_t *forward_in_config,
                                flb_srvc_config_t *p_flb_srvc_config,
                                netdata_mutex_t *stdout_mut){

    struct File_info *p_file_info = callocz(1, sizeof(struct File_info));

    /* -------------------------------------------------------------------------
     * Check if config_section->name is valid and if so, use it as chartname.
     * ------------------------------------------------------------------------- */
    if(config_section->name && *config_section->name){
        p_file_info->chartname = strdupz(config_section->name);
        netdata_fix_chart_id((char *) p_file_info->chartname);
        collector_info("[%s]: Initializing config loading", p_file_info->chartname);
    } else {
        collector_error("Invalid logs management config section.");
        return p_file_info_destroy(p_file_info);
    }
    

    /* -------------------------------------------------------------------------
     * Check if this log source is enabled.
     * ------------------------------------------------------------------------- */
    if(appconfig_get_boolean(&log_management_config, config_section->name, "enabled", CONFIG_BOOLEAN_NO)){
        collector_info("[%s]: enabled = yes", p_file_info->chartname);
    } else {
        collector_info("[%s]: enabled = no", p_file_info->chartname);
        return p_file_info_destroy(p_file_info);
    }


    /* -------------------------------------------------------------------------
     * Check log type.
     * ------------------------------------------------------------------------- */
    char *type = appconfig_get(&log_management_config, config_section->name, "log type", "flb_tail");
    if(!type || !*type) p_file_info->log_type = FLB_TAIL; // Default
    else{
        if(!strcasecmp(type, "flb_tail")) p_file_info->log_type = FLB_TAIL;
        else if (!strcasecmp(type, "flb_web_log")) p_file_info->log_type = FLB_WEB_LOG;
        else if (!strcasecmp(type, "flb_kmsg")) p_file_info->log_type = FLB_KMSG;
        else if (!strcasecmp(type, "flb_systemd")) p_file_info->log_type = FLB_SYSTEMD;
        else if (!strcasecmp(type, "flb_docker_events")) p_file_info->log_type = FLB_DOCKER_EV;
        else if (!strcasecmp(type, "flb_syslog")) p_file_info->log_type = FLB_SYSLOG;
        else if (!strcasecmp(type, "flb_serial")) p_file_info->log_type = FLB_SERIAL;
        else if (!strcasecmp(type, "flb_mqtt")) p_file_info->log_type = FLB_MQTT;
        else p_file_info->log_type = FLB_TAIL;
    }
    freez(type);
    collector_info("[%s]: log type = %s", p_file_info->chartname, log_src_type_t_str[p_file_info->log_type]);


    /* -------------------------------------------------------------------------
     * Read log source.
     * ------------------------------------------------------------------------- */
    char *source = appconfig_get(&log_management_config, config_section->name, "log source", "local");
    if(!source || !*source) p_file_info->log_source = LOG_SOURCE_LOCAL; // Default
    else if(!strcasecmp(source, "forward")) p_file_info->log_source = LOG_SOURCE_FORWARD;
    else p_file_info->log_source = LOG_SOURCE_LOCAL;
    freez(source);
    collector_info("[%s]: log source = %s", p_file_info->chartname, log_src_t_str[p_file_info->log_source]);

    if(p_file_info->log_source == LOG_SOURCE_FORWARD && !forward_in_config){
        collector_info("[%s]: forward_in_config == NULL - this log source will be disabled", p_file_info->chartname);
        return p_file_info_destroy(p_file_info);
    }


    /* -------------------------------------------------------------------------
     * Read stream uuid.
     * ------------------------------------------------------------------------- */
    p_file_info->stream_guid = appconfig_get(&log_management_config, config_section->name, "stream guid", "");
    collector_info("[%s]: stream guid = %s", p_file_info->chartname, p_file_info->stream_guid);


    /* -------------------------------------------------------------------------
     * Read log path configuration and check if it is valid.
     * ------------------------------------------------------------------------- */
    p_file_info->filename = appconfig_get(&log_management_config, config_section->name, "log path", LOG_PATH_AUTO);
    if( /* path doesn't matter when log source is not local */
        (p_file_info->log_source == LOG_SOURCE_LOCAL) &&
        
        /* FLB_SYSLOG is special case, may or may not require a path */
        (p_file_info->log_type != FLB_SYSLOG) &&

        /* FLB_MQTT is special case, does not require a path */
        (p_file_info->log_type != FLB_MQTT) &&
        
        (!p_file_info->filename /* Sanity check */ || 
         !*p_file_info->filename || 
         !strcmp(p_file_info->filename, LOG_PATH_AUTO) || 
         access(p_file_info->filename, R_OK)
        )){ 

        freez(p_file_info->filename);
        p_file_info->filename = NULL;
            
        switch(p_file_info->log_type){
            case FLB_TAIL:
                if(!strcasecmp(p_file_info->chartname, "Netdata_daemon.log")){
                    char path[FILENAME_MAX + 1];
                    snprintfz(path, FILENAME_MAX, "%s/daemon.log", get_log_dir());
                    if(access(path, R_OK)) {
                        collector_error("[%s]: 'Netdata_daemon.log' path (%s) invalid, unknown or needs permissions", 
                            p_file_info->chartname, path);
                        return p_file_info_destroy(p_file_info);
                    } else p_file_info->filename = strdupz(path);
                } else if(!strcasecmp(p_file_info->chartname, "Netdata_fluentbit.log")){
                    if(access(p_flb_srvc_config->log_path, R_OK)){
                        collector_error("[%s]: Netdata_fluentbit.log path (%s) invalid, unknown or needs permissions", 
                            p_file_info->chartname, p_flb_srvc_config->log_path);
                        return p_file_info_destroy(p_file_info);
                    } else p_file_info->filename = strdupz(p_flb_srvc_config->log_path);
                } else if(!strcasecmp(p_file_info->chartname, "Auth.log_tail")){
                    const char * const auth_path_default[] = {
                        "/var/log/auth.log",
                        NULL
                    };
                    int i = 0;
                    while(auth_path_default[i] && access(auth_path_default[i], R_OK)){i++;};
                    if(!auth_path_default[i]){
                        collector_error("[%s]: auth.log path invalid, unknown or needs permissions", p_file_info->chartname);
                        return p_file_info_destroy(p_file_info);
                    } else p_file_info->filename = strdupz(auth_path_default[i]);
                } else if(!strcasecmp(p_file_info->chartname, "syslog_tail")){
                    const char * const syslog_path_default[] = {
                        "/var/log/syslog",   /* Debian, Ubuntu */
                        "/var/log/messages", /* RHEL, Red Hat, CentOS, Fedora */
                        NULL
                    };
                    int i = 0;
                    while(syslog_path_default[i] && access(syslog_path_default[i], R_OK)){i++;};
                    if(!syslog_path_default[i]){
                        collector_error("[%s]: syslog path invalid, unknown or needs permissions", p_file_info->chartname);
                        return p_file_info_destroy(p_file_info);
                    } else p_file_info->filename = strdupz(syslog_path_default[i]);
                }
                break;
            case FLB_WEB_LOG:
                if(!strcasecmp(p_file_info->chartname, "Apache_access.log")){
                    const char * const apache_access_path_default[] = {
                        "/var/log/apache/access.log",
                        "/var/log/apache2/access.log",
                        "/var/log/apache2/access_log",
                        "/var/log/httpd/access_log",
                        "/var/log/httpd-access.log",
                        NULL
                    };
                    int i = 0;
                    while(apache_access_path_default[i] && access(apache_access_path_default[i], R_OK)){i++;};
                    if(!apache_access_path_default[i]){
                        collector_error("[%s]: Apache access.log path invalid, unknown or needs permissions", p_file_info->chartname);
                        return p_file_info_destroy(p_file_info);
                    } else p_file_info->filename = strdupz(apache_access_path_default[i]);
                } else if(!strcasecmp(p_file_info->chartname, "Nginx_access.log")){
                    const char * const nginx_access_path_default[] = {
                        "/var/log/nginx/access.log",
                        NULL
                    };
                    int i = 0;
                    while(nginx_access_path_default[i] && access(nginx_access_path_default[i], R_OK)){i++;};
                    if(!nginx_access_path_default[i]){
                        collector_error("[%s]: Nginx access.log path invalid, unknown or needs permissions", p_file_info->chartname);
                        return p_file_info_destroy(p_file_info);
                    } else p_file_info->filename = strdupz(nginx_access_path_default[i]);
                }
                break;
            case FLB_KMSG:
                if(access(KMSG_DEFAULT_PATH, R_OK)){
                    collector_error("[%s]: kmsg default path invalid, unknown or needs permissions", p_file_info->chartname);
                    return p_file_info_destroy(p_file_info);
                } else p_file_info->filename = strdupz(KMSG_DEFAULT_PATH);
                break;
            case FLB_SYSTEMD:
                p_file_info->filename = strdupz(SYSTEMD_DEFAULT_PATH);
                break;
            case FLB_DOCKER_EV:
                if(access(DOCKER_EV_DEFAULT_PATH, R_OK)){
                    collector_error("[%s]: Docker socket default Unix path invalid, unknown or needs permissions", p_file_info->chartname);
                    return p_file_info_destroy(p_file_info);
                } else p_file_info->filename = strdupz(DOCKER_EV_DEFAULT_PATH);
                break;
            default:
                collector_error("[%s]: log path invalid or unknown", p_file_info->chartname);
                return p_file_info_destroy(p_file_info);
        }
    }
    p_file_info->file_basename = get_basename(p_file_info->filename); 
    collector_info("[%s]: p_file_info->filename: %s", p_file_info->chartname, 
                                            p_file_info->filename ? p_file_info->filename : "NULL");
    collector_info("[%s]: p_file_info->file_basename: %s", p_file_info->chartname, 
                                                 p_file_info->file_basename ? p_file_info->file_basename : "NULL");
    if(unlikely(!p_file_info->filename)) return p_file_info_destroy(p_file_info);


    /* -------------------------------------------------------------------------
     * Read "update every" and "update timeout" configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->update_every = appconfig_get_number(   &log_management_config, config_section->name, 
                                                        "update every", g_logs_manag_config.update_every);
    collector_info("[%s]: update every = %d", p_file_info->chartname, p_file_info->update_every);

    p_file_info->update_timeout = appconfig_get_number( &log_management_config, config_section->name, 
                                                        "update timeout", g_logs_manag_config.update_timeout);
    if(p_file_info->update_timeout < p_file_info->update_every) p_file_info->update_timeout = p_file_info->update_every;
    collector_info("[%s]: update timeout = %d", p_file_info->chartname, p_file_info->update_timeout);


    /* -------------------------------------------------------------------------
     * Read "use log timestamp" configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->use_log_timestamp = appconfig_get_boolean_ondemand(&log_management_config, config_section->name, 
                                                                    "use log timestamp", 
                                                                    g_logs_manag_config.use_log_timestamp);
    collector_info("[%s]: use log timestamp = %s", p_file_info->chartname, 
                                                    p_file_info->use_log_timestamp ? "auto or yes" : "no");


    /* -------------------------------------------------------------------------
     * Read compression acceleration configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->compression_accel = appconfig_get_number(  &log_management_config, config_section->name, 
                                                            "compression acceleration", 
                                                            g_logs_manag_config.compression_acceleration);
    collector_info("[%s]: compression acceleration = %d", p_file_info->chartname, p_file_info->compression_accel);


    /* -------------------------------------------------------------------------
     * Read DB mode.
     * ------------------------------------------------------------------------- */
    const char *const db_mode_str = appconfig_get(&log_management_config, config_section->name, "db mode", NULL);
    collector_info("[%s]: db mode = %s", p_file_info->chartname, db_mode_str ? db_mode_str : "NULL");
    p_file_info->db_mode = db_mode_str_to_db_mode(db_mode_str);
    freez((void *)db_mode_str);


    /* -------------------------------------------------------------------------
     * Read save logs from buffers to DB interval configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->buff_flush_to_db_interval = appconfig_get_number(  &log_management_config, config_section->name, 
                                                                    "circular buffer flush to db", 
                                                                    g_logs_manag_config.buff_flush_to_db_interval);
    if(p_file_info->buff_flush_to_db_interval > SAVE_BLOB_TO_DB_MAX) {
        p_file_info->buff_flush_to_db_interval = SAVE_BLOB_TO_DB_MAX;
        collector_info("[%s]: circular buffer flush to db out of range. Using maximum permitted value: %d", 
                p_file_info->chartname, p_file_info->buff_flush_to_db_interval);

    } else if(p_file_info->buff_flush_to_db_interval < SAVE_BLOB_TO_DB_MIN) {
        p_file_info->buff_flush_to_db_interval = SAVE_BLOB_TO_DB_MIN;
        collector_info("[%s]: circular buffer flush to db out of range. Using minimum permitted value: %d",
                p_file_info->chartname, p_file_info->buff_flush_to_db_interval);
    } 
    collector_info("[%s]: circular buffer flush to db = %d", p_file_info->chartname, p_file_info->buff_flush_to_db_interval);


    /* -------------------------------------------------------------------------
     * Read BLOB max size configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->blob_max_size  = appconfig_get_number( &log_management_config, config_section->name, 
                                                        "disk space limit MiB", 
                                                        g_logs_manag_config.disk_space_limit_in_mib) MiB / BLOB_MAX_FILES;
    collector_info("[%s]: BLOB max size = %lld", p_file_info->chartname, (long long)p_file_info->blob_max_size);


    /* -------------------------------------------------------------------------
     * Read configuration about sending logs to system journal.
     * ------------------------------------------------------------------------- */
    p_file_info->do_sd_journal_send = appconfig_get_boolean(&log_management_config, config_section->name,
                                                            "submit logs to system journal",
                                                            g_logs_manag_config.do_sd_journal_send);

    /* -------------------------------------------------------------------------
     * Read collected logs chart configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->parser_config = callocz(1, sizeof(Log_parser_config_t));

    if(appconfig_get_boolean(&log_management_config, config_section->name, 
                             "collected logs total chart enable",
                             g_logs_manag_config.enable_collected_logs_total)){
        p_file_info->parser_config->chart_config |= CHART_COLLECTED_LOGS_TOTAL;
    }
    collector_info( "[%s]: collected logs total chart enable = %s",  p_file_info->chartname, 
                    (p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_TOTAL) ? "yes" : "no");

    if(appconfig_get_boolean(&log_management_config, config_section->name, 
                             "collected logs rate chart enable",
                             g_logs_manag_config.enable_collected_logs_rate)){
        p_file_info->parser_config->chart_config |= CHART_COLLECTED_LOGS_RATE;
    }
    collector_info( "[%s]: collected logs rate chart enable = %s",  p_file_info->chartname, 
                    (p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_RATE) ? "yes" : "no");


    /* -------------------------------------------------------------------------
     * Deal with log-type-specific configuration options.
     * ------------------------------------------------------------------------- */
    
    if(p_file_info->log_type == FLB_TAIL || p_file_info->log_type == FLB_WEB_LOG){
        Flb_tail_config_t *tail_config = callocz(1, sizeof(Flb_tail_config_t));
        if(appconfig_get_boolean(&log_management_config, config_section->name, "use inotify", CONFIG_BOOLEAN_YES))
            tail_config->use_inotify = 1;
        collector_info( "[%s]: use inotify = %s",  p_file_info->chartname, tail_config->use_inotify? "yes" : "no");

        p_file_info->flb_config = tail_config;
    }
    
    if(p_file_info->log_type == FLB_WEB_LOG){
        /* Check if a valid web log format configuration is detected */
        char *log_format = appconfig_get(&log_management_config, config_section->name, "log format", LOG_PATH_AUTO);
        const char delimiter = ' '; // TODO!!: TO READ FROM CONFIG
        collector_info("[%s]: log format = %s", p_file_info->chartname, log_format ? log_format : "NULL!");

        /* If "log format = auto" or no "log format" config is detected, 
            * try log format autodetection based on last log file line.
            * TODO 1: Add another case in OR where log_format is compared with a valid reg exp.
            * TODO 2: Set default log format and delimiter if not found in config? Or auto-detect? */ 
        if(!log_format || !*log_format || !strcmp(log_format, LOG_PATH_AUTO)){ 
            collector_info("[%s]: Attempting auto-detection of log format", p_file_info->chartname);
            char *line = read_last_line(p_file_info->filename, 0);
            if(!line){
                collector_error("[%s]: read_last_line() returned NULL", p_file_info->chartname);
                return p_file_info_destroy(p_file_info);
            }
            p_file_info->parser_config->gen_config = auto_detect_web_log_parser_config(line, delimiter);
            freez(line);
        }
        else{
            p_file_info->parser_config->gen_config = read_web_log_parser_config(log_format, delimiter);
            collector_info( "[%s]: Read web log parser config: %s", p_file_info->chartname, 
                    p_file_info->parser_config->gen_config ? "success!" : "failed!");
        }
        freez(log_format);

        if(!p_file_info->parser_config->gen_config){
            collector_error("[%s]: No valid web log parser config found", p_file_info->chartname);
            return p_file_info_destroy(p_file_info); 
        }

        /* Check whether metrics verification during parsing is required */
        Web_log_parser_config_t *wblp_config = (Web_log_parser_config_t *) p_file_info->parser_config->gen_config;
        wblp_config->verify_parsed_logs = appconfig_get_boolean( &log_management_config, config_section->name, 
                                                                    "verify parsed logs", CONFIG_BOOLEAN_NO);
        collector_info("[%s]: verify parsed logs = %d", p_file_info->chartname, wblp_config->verify_parsed_logs);

        wblp_config->skip_timestamp_parsing = p_file_info->use_log_timestamp ? 0 : 1;
        collector_info("[%s]: skip_timestamp_parsing = %d", p_file_info->chartname, wblp_config->skip_timestamp_parsing);
        
        for(int j = 0; j < wblp_config->num_fields; j++){
            if((wblp_config->fields[j] == VHOST_WITH_PORT || wblp_config->fields[j] == VHOST) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "vhosts chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_VHOST;
            }
            if((wblp_config->fields[j] == VHOST_WITH_PORT || wblp_config->fields[j] == PORT) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "ports chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_PORT;
            }
            if((wblp_config->fields[j] == REQ_CLIENT) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "IP versions chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_IP_VERSION;
            }
            if((wblp_config->fields[j] == REQ_CLIENT) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "unique client IPs - current poll chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_REQ_CLIENT_CURRENT;
            }
            if((wblp_config->fields[j] == REQ_CLIENT) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "unique client IPs - all-time chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_REQ_CLIENT_ALL_TIME;
            }
            if((wblp_config->fields[j] == REQ || wblp_config->fields[j] == REQ_METHOD) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "http request methods chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_REQ_METHODS;
            }
            if((wblp_config->fields[j] == REQ || wblp_config->fields[j] == REQ_PROTO) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "http protocol versions chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_REQ_PROTO;
            }
            if((wblp_config->fields[j] == REQ_SIZE || wblp_config->fields[j] == RESP_SIZE) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "bandwidth chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_BANDWIDTH;
            }
            if((wblp_config->fields[j] == REQ_PROC_TIME) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "timings chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_REQ_PROC_TIME;
            }
            if((wblp_config->fields[j] == RESP_CODE) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "response code families chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_RESP_CODE_FAMILY;
            }
            if((wblp_config->fields[j] == RESP_CODE) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "response codes chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_RESP_CODE;
            }
            if((wblp_config->fields[j] == RESP_CODE) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "response code types chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_RESP_CODE_TYPE;
            }
            if((wblp_config->fields[j] == SSL_PROTO) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "SSL protocols chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_SSL_PROTO;
            }
            if((wblp_config->fields[j] == SSL_CIPHER_SUITE) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "SSL chipher suites chart", CONFIG_BOOLEAN_NO)){ 
                p_file_info->parser_config->chart_config |= CHART_SSL_CIPHER;
            }
        }
    }
    else if(p_file_info->log_type == FLB_KMSG){
        Flb_kmsg_config_t *kmsg_config = callocz(1, sizeof(Flb_kmsg_config_t));

        kmsg_config->prio_level = appconfig_get(&log_management_config, config_section->name, "prio level", "8");

        p_file_info->flb_config = kmsg_config;

        if(appconfig_get_boolean(&log_management_config, config_section->name, "severity chart", CONFIG_BOOLEAN_NO)) {
            p_file_info->parser_config->chart_config |= CHART_SYSLOG_SEVER;
        }
        if(appconfig_get_boolean(&log_management_config, config_section->name, "subsystem chart", CONFIG_BOOLEAN_NO)) {
            p_file_info->parser_config->chart_config |= CHART_KMSG_SUBSYSTEM;
        }
        if(appconfig_get_boolean(&log_management_config, config_section->name, "device chart", CONFIG_BOOLEAN_NO)) {
            p_file_info->parser_config->chart_config |= CHART_KMSG_DEVICE;
        }
    }
    else if(p_file_info->log_type == FLB_SYSTEMD || p_file_info->log_type == FLB_SYSLOG){
        if(p_file_info->log_type == FLB_SYSLOG){
            Syslog_parser_config_t *syslog_config = callocz(1, sizeof(Syslog_parser_config_t));

            /* Read syslog format */
            syslog_config->log_format = appconfig_get(  &log_management_config, 
                                                        config_section->name, 
                                                        "log format", NULL);
            collector_info("[%s]: log format = %s", p_file_info->chartname, 
                                                    syslog_config->log_format ? syslog_config->log_format : "NULL!");
            if(!syslog_config->log_format || !*syslog_config->log_format || !strcasecmp(syslog_config->log_format, "auto")){
                freez(syslog_config->log_format);
                freez(syslog_config);
                return p_file_info_destroy(p_file_info);
            }

            syslog_config->socket_config = callocz(1, sizeof(Flb_socket_config_t));

            /* Read syslog socket mode
             * see also https://docs.fluentbit.io/manual/pipeline/inputs/syslog#configuration-parameters */
            syslog_config->socket_config->mode = appconfig_get( &log_management_config, 
                                                                config_section->name, 
                                                                "mode", "unix_udp");
            collector_info("[%s]: mode = %s", p_file_info->chartname, syslog_config->socket_config->mode);

            /* Check for valid socket path if (mode == unix_udp) or 
             * (mode == unix_tcp), else read syslog network interface to bind, 
             * if (mode == udp) or (mode == tcp). */
            if( !strcasecmp(syslog_config->socket_config->mode, "unix_udp") || 
                !strcasecmp(syslog_config->socket_config->mode, "unix_tcp")){
                if(!p_file_info->filename || !*p_file_info->filename || !strcasecmp(p_file_info->filename, LOG_PATH_AUTO)){
                    // freez(syslog_config->socket_config->mode);
                    freez(syslog_config->socket_config);
                    freez(syslog_config->log_format);
                    freez(syslog_config);
                    return p_file_info_destroy(p_file_info);
                }
                syslog_config->socket_config->unix_perm = appconfig_get(&log_management_config, 
                                                                        config_section->name, 
                                                                        "unix_perm", "0644");
                collector_info("[%s]: unix_perm = %s", p_file_info->chartname, syslog_config->socket_config->unix_perm);
            } else if(  !strcasecmp(syslog_config->socket_config->mode, "udp") || 
                        !strcasecmp(syslog_config->socket_config->mode, "tcp")){
                // TODO: Check if listen is in valid format
                syslog_config->socket_config->listen = appconfig_get(   &log_management_config, 
                                                                        config_section->name, 
                                                                        "listen", "0.0.0.0");
                collector_info("[%s]: listen = %s", p_file_info->chartname, syslog_config->socket_config->listen);
                syslog_config->socket_config->port = appconfig_get( &log_management_config, 
                                                                    config_section->name, 
                                                                    "port", "5140");
                collector_info("[%s]: port = %s", p_file_info->chartname, syslog_config->socket_config->port);
            } else { 
                /* Any other modes are invalid */
                // freez(syslog_config->socket_config->mode);
                freez(syslog_config->socket_config);
                freez(syslog_config->log_format);
                freez(syslog_config);
                return p_file_info_destroy(p_file_info);
            }

            p_file_info->parser_config->gen_config = syslog_config;
        }
        if(appconfig_get_boolean(&log_management_config, config_section->name, "priority value chart", CONFIG_BOOLEAN_NO)) {
            p_file_info->parser_config->chart_config |= CHART_SYSLOG_PRIOR;
        }
        if(appconfig_get_boolean(&log_management_config, config_section->name, "severity chart", CONFIG_BOOLEAN_NO)) {
            p_file_info->parser_config->chart_config |= CHART_SYSLOG_SEVER;
        }
        if(appconfig_get_boolean(&log_management_config, config_section->name, "facility chart", CONFIG_BOOLEAN_NO)) {
            p_file_info->parser_config->chart_config |= CHART_SYSLOG_FACIL;
        }
    }
    else if(p_file_info->log_type == FLB_DOCKER_EV){
        if(appconfig_get_boolean(&log_management_config, config_section->name, "event type chart", CONFIG_BOOLEAN_NO)) {
            p_file_info->parser_config->chart_config |= CHART_DOCKER_EV_TYPE;
        }
        if(appconfig_get_boolean(&log_management_config, config_section->name, "event action chart", CONFIG_BOOLEAN_NO)) {
            p_file_info->parser_config->chart_config |= CHART_DOCKER_EV_ACTION;
        }
    }
    else if(p_file_info->log_type == FLB_SERIAL){
        Flb_serial_config_t *serial_config = callocz(1, sizeof(Flb_serial_config_t));

        serial_config->bitrate = appconfig_get(&log_management_config, config_section->name, "bitrate", "115200");
        serial_config->min_bytes = appconfig_get(&log_management_config, config_section->name, "min bytes", "1");
        serial_config->separator = appconfig_get(&log_management_config, config_section->name, "separator", "");
        serial_config->format = appconfig_get(&log_management_config, config_section->name, "format", "");

        p_file_info->flb_config = serial_config;
    }
    else if(p_file_info->log_type == FLB_MQTT){
        Flb_socket_config_t *socket_config = callocz(1, sizeof(Flb_socket_config_t));

        socket_config->listen = appconfig_get(&log_management_config, config_section->name, "listen", "0.0.0.0");
        socket_config->port = appconfig_get(&log_management_config, config_section->name, "port", "1883");

        p_file_info->flb_config = socket_config;

        if(appconfig_get_boolean(&log_management_config, config_section->name, "topic chart", CONFIG_BOOLEAN_NO)) {
            p_file_info->parser_config->chart_config |= CHART_MQTT_TOPIC;
        }
    }


    /* -------------------------------------------------------------------------
     * Allocate p_file_info->parser_metrics memory.
     * ------------------------------------------------------------------------- */
    p_file_info->parser_metrics = callocz(1, sizeof(Log_parser_metrics_t));
    switch(p_file_info->log_type){
        case FLB_WEB_LOG:{
            p_file_info->parser_metrics->web_log = callocz(1, sizeof(Web_log_metrics_t));
            break;
        }
        case FLB_KMSG: {
            p_file_info->parser_metrics->kernel = callocz(1, sizeof(Kernel_metrics_t));
            p_file_info->parser_metrics->kernel->subsystem = dictionary_create( DICT_OPTION_SINGLE_THREADED | 
                                                                                DICT_OPTION_NAME_LINK_DONT_CLONE | 
                                                                                DICT_OPTION_DONT_OVERWRITE_VALUE);
            dictionary_register_conflict_callback(p_file_info->parser_metrics->kernel->subsystem, metrics_dict_conflict_cb, NULL);
            p_file_info->parser_metrics->kernel->device = dictionary_create(DICT_OPTION_SINGLE_THREADED | 
                                                                            DICT_OPTION_NAME_LINK_DONT_CLONE | 
                                                                            DICT_OPTION_DONT_OVERWRITE_VALUE);
            dictionary_register_conflict_callback(p_file_info->parser_metrics->kernel->device, metrics_dict_conflict_cb, NULL);
            break;
        }
        case FLB_SYSTEMD: 
        case FLB_SYSLOG: {
            p_file_info->parser_metrics->systemd = callocz(1, sizeof(Systemd_metrics_t));
            break;
        }
        case FLB_DOCKER_EV: {
            p_file_info->parser_metrics->docker_ev = callocz(1, sizeof(Docker_ev_metrics_t));
            break;
        }
        case FLB_MQTT: {
            p_file_info->parser_metrics->mqtt = callocz(1, sizeof(Mqtt_metrics_t));
            p_file_info->parser_metrics->mqtt->topic = dictionary_create(   DICT_OPTION_SINGLE_THREADED | 
                                                                            DICT_OPTION_NAME_LINK_DONT_CLONE | 
                                                                            DICT_OPTION_DONT_OVERWRITE_VALUE);
            dictionary_register_conflict_callback(p_file_info->parser_metrics->mqtt->topic, metrics_dict_conflict_cb, NULL);
            break;
        }
        default:
            break;
    }


    /* -------------------------------------------------------------------------
     * Configure (optional) custom charts.
     * ------------------------------------------------------------------------- */
    p_file_info->parser_cus_config = callocz(1, sizeof(Log_parser_cus_config_t *));
    p_file_info->parser_metrics->parser_cus = callocz(1, sizeof(Log_parser_cus_metrics_t *));
    for(int cus_off = 1; cus_off <= MAX_CUS_CHARTS_PER_SOURCE; cus_off++){

        /* Read chart name config */
        char *cus_chart_k = mallocz(snprintf(NULL, 0, "custom %d chart", MAX_CUS_CHARTS_PER_SOURCE) + 1);
        sprintf(cus_chart_k, "custom %d chart", cus_off);
        char *cus_chart_v = appconfig_get(&log_management_config, config_section->name, cus_chart_k, NULL);
        debug_log( "cus chart: (%s:%s)", cus_chart_k, cus_chart_v ? cus_chart_v : "NULL");
        freez(cus_chart_k);
        if(unlikely(!cus_chart_v)){
            collector_error("[%s]: custom %d chart = NULL, custom charts for this log source will be disabled.", 
                            p_file_info->chartname, cus_off);
            break;
        }
        netdata_fix_chart_id(cus_chart_v);

        /* Read regex config */
        char *cus_regex_k = mallocz(snprintf(NULL, 0, "custom %d regex", MAX_CUS_CHARTS_PER_SOURCE) + 1);
        sprintf(cus_regex_k, "custom %d regex", cus_off);
        char *cus_regex_v = appconfig_get(&log_management_config, config_section->name, cus_regex_k, NULL);
        debug_log( "cus regex: (%s:%s)", cus_regex_k, cus_regex_v ? cus_regex_v : "NULL");
        freez(cus_regex_k);
        if(unlikely(!cus_regex_v)) {
            collector_error("[%s]: custom %d regex = NULL, custom charts for this log source will be disabled.", 
                            p_file_info->chartname, cus_off);
            freez(cus_chart_v);
            break;
        }

        /* Read regex name config */
        char *cus_regex_name_k = mallocz(snprintf(NULL, 0, "custom %d regex name", MAX_CUS_CHARTS_PER_SOURCE) + 1);
        sprintf(cus_regex_name_k, "custom %d regex name", cus_off);
        char *cus_regex_name_v = appconfig_get( &log_management_config, config_section->name, 
                                                cus_regex_name_k, cus_regex_v);
        debug_log( "cus regex name: (%s:%s)", cus_regex_name_k, cus_regex_name_v ? cus_regex_name_v : "NULL");
        freez(cus_regex_name_k);
        m_assert(cus_regex_name_v, "cus_regex_name_v cannot be NULL, should be cus_regex_v");
             
        
        /* Escape any backslashes in the regex name, to ensure dimension is displayed correctly in charts */
        int regex_name_bslashes = 0;
        char **p_regex_name = &cus_regex_name_v;
        for(char *p = *p_regex_name; *p; p++) if(unlikely(*p == '\\')) regex_name_bslashes++;
        if(regex_name_bslashes) {
            *p_regex_name = reallocz(*p_regex_name, strlen(*p_regex_name) + 1 + regex_name_bslashes);
            for(char *p = *p_regex_name; *p; p++){
                if(unlikely(*p == '\\')){
                    memmove(p + 1, p, strlen(p) + 1);
                    *p++ = '\\';
                }
            }
        } 

        /* Read ignore case config */
        char *cus_ignore_case_k = mallocz(snprintf(NULL, 0, "custom %d ignore case", MAX_CUS_CHARTS_PER_SOURCE) + 1);
        sprintf(cus_ignore_case_k, "custom %d ignore case", cus_off);
        int cus_ignore_case_v = appconfig_get_boolean(  &log_management_config, 
                                                        config_section->name, cus_ignore_case_k, CONFIG_BOOLEAN_YES);
        debug_log( "cus case: (%s:%s)", cus_ignore_case_k, cus_ignore_case_v ? "yes" : "no");
        freez(cus_ignore_case_k); 

        int regex_flags = cus_ignore_case_v ? REG_EXTENDED | REG_NEWLINE | REG_ICASE : REG_EXTENDED | REG_NEWLINE;
        
        int rc;
        regex_t regex;
        if (unlikely((rc = regcomp(&regex, cus_regex_v, regex_flags)))){
            size_t regcomp_err_str_size = regerror(rc, &regex, 0, 0);
            char *regcomp_err_str = mallocz(regcomp_err_str_size);
            regerror(rc, &regex, regcomp_err_str, regcomp_err_str_size);
            collector_error("[%s]: could not compile regex for custom %d chart: %s due to error: %s. "
                            "Custom charts for this log source will be disabled.", 
                            p_file_info->chartname, cus_off, cus_chart_v, regcomp_err_str);
            freez(regcomp_err_str);
            freez(cus_chart_v);
            freez(cus_regex_v);
            freez(cus_regex_name_v);
            break;
        };

        /* Allocate memory and copy config to p_file_info->parser_cus_config struct */
        p_file_info->parser_cus_config = reallocz(  p_file_info->parser_cus_config, 
                                                    (cus_off + 1) * sizeof(Log_parser_cus_config_t *));
        p_file_info->parser_cus_config[cus_off - 1] = callocz(1, sizeof(Log_parser_cus_config_t));

        p_file_info->parser_cus_config[cus_off - 1]->chartname = cus_chart_v;
        p_file_info->parser_cus_config[cus_off - 1]->regex_str = cus_regex_v;
        p_file_info->parser_cus_config[cus_off - 1]->regex_name = cus_regex_name_v;
        p_file_info->parser_cus_config[cus_off - 1]->regex = regex;

        /* Initialise custom log parser metrics struct array */
        p_file_info->parser_metrics->parser_cus = reallocz( p_file_info->parser_metrics->parser_cus, 
                                                            (cus_off + 1) * sizeof(Log_parser_cus_metrics_t *));
        p_file_info->parser_metrics->parser_cus[cus_off - 1] = callocz(1, sizeof(Log_parser_cus_metrics_t));


        p_file_info->parser_cus_config[cus_off] = NULL;
        p_file_info->parser_metrics->parser_cus[cus_off] = NULL;
    }


    /* -------------------------------------------------------------------------
     * Configure (optional) Fluent Bit outputs.
     * ------------------------------------------------------------------------- */
    
    Flb_output_config_t **output_next_p = &p_file_info->flb_outputs;
    for(int out_off = 1; out_off <= MAX_OUTPUTS_PER_SOURCE; out_off++){

        /* Read output plugin */
        char *out_plugin_k = callocz(1, snprintf(NULL, 0, "output %d " FLB_OUTPUT_PLUGIN_NAME_KEY, MAX_OUTPUTS_PER_SOURCE) + 1);
        sprintf(out_plugin_k, "output %d " FLB_OUTPUT_PLUGIN_NAME_KEY, out_off);
        char *out_plugin_v = appconfig_get(&log_management_config, config_section->name, out_plugin_k, NULL);
        debug_log( "output %d "FLB_OUTPUT_PLUGIN_NAME_KEY": %s", out_off, out_plugin_v ? out_plugin_v : "NULL");
        freez(out_plugin_k);
        if(unlikely(!out_plugin_v)){
            collector_error("[%s]: output %d "FLB_OUTPUT_PLUGIN_NAME_KEY" = NULL, outputs for this log source will be disabled.", 
                            p_file_info->chartname, out_off);
            break;
        }

        Flb_output_config_t *output = callocz(1, sizeof(Flb_output_config_t));
        output->id = out_off;
        output->plugin = out_plugin_v;

        /* Read parameters for this output */
        avl_traverse_lock(&config_section->values_index, flb_output_param_get_cb, output);

        *output_next_p = output;
        output_next_p = &output->next;
    }
    
    
    /* -------------------------------------------------------------------------
     * Read circular buffer configuration and initialize the buffer.
     * ------------------------------------------------------------------------- */
    size_t circular_buffer_max_size = ((size_t)appconfig_get_number(&log_management_config, 
                                                                    config_section->name,
                                                                    "circular buffer max size MiB", 
                                                                    g_logs_manag_config.circ_buff_max_size_in_mib)) MiB;
    if(circular_buffer_max_size > CIRCULAR_BUFF_MAX_SIZE_RANGE_MAX) {
        circular_buffer_max_size = CIRCULAR_BUFF_MAX_SIZE_RANGE_MAX;
        collector_info( "[%s]: circular buffer max size out of range. Using maximum permitted value (MiB): %zu", 
                p_file_info->chartname, (size_t) (circular_buffer_max_size / (1 MiB)));
    } else if(circular_buffer_max_size < CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN) {
        circular_buffer_max_size = CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN;
        collector_info( "[%s]: circular buffer max size out of range. Using minimum permitted value (MiB): %zu", 
                p_file_info->chartname, (size_t) (circular_buffer_max_size / (1 MiB)));
    } 
    collector_info("[%s]: circular buffer max size MiB = %zu", p_file_info->chartname, (size_t) (circular_buffer_max_size / (1 MiB)));

    int circular_buffer_allow_dropped_logs = appconfig_get_boolean( &log_management_config, 
                                                                    config_section->name,
                                                                    "circular buffer drop logs if full", 
                                                                    g_logs_manag_config.circ_buff_drop_logs);
    collector_info("[%s]: circular buffer drop logs if full = %s", p_file_info->chartname, 
        circular_buffer_allow_dropped_logs ? "yes" : "no");

    p_file_info->circ_buff = circ_buff_init(p_file_info->buff_flush_to_db_interval,
                                            circular_buffer_max_size,
                                            circular_buffer_allow_dropped_logs);


    /* -------------------------------------------------------------------------
     * Initialize rrd related structures.
     * ------------------------------------------------------------------------- */
    p_file_info->chart_meta = callocz(1, sizeof(struct Chart_meta));
    memcpy(p_file_info->chart_meta, &chart_types[p_file_info->log_type], sizeof(struct Chart_meta));
    p_file_info->chart_meta->base_prio = NETDATA_CHART_PRIO_LOGS_BASE + p_file_infos_arr->count * NETDATA_CHART_PRIO_LOGS_INCR;
    netdata_mutex_lock(stdout_mut);
    p_file_info->chart_meta->init(p_file_info);
    fflush(stdout);
    netdata_mutex_unlock(stdout_mut);

    /* -------------------------------------------------------------------------
     * Initialize input plugin for local log sources.
     * ------------------------------------------------------------------------- */
    if(p_file_info->log_source == LOG_SOURCE_LOCAL){
        int rc = flb_add_input(p_file_info);
        if(unlikely(rc)){
            collector_error("[%s]: flb_add_input() error: %d", p_file_info->chartname, rc);
            return p_file_info_destroy(p_file_info);
        }
    }

    /* flb_complete_item_timer_timeout_cb() is needed for both local and 
     * non-local sources. */
    p_file_info->flb_tmp_buff_cpy_timer.data = p_file_info;
    if(unlikely(0 != uv_mutex_init(&p_file_info->flb_tmp_buff_mut)))
        fatal("uv_mutex_init(&p_file_info->flb_tmp_buff_mut) failed");

    fatal_assert(0 == uv_timer_init(    main_loop, 
                                        &p_file_info->flb_tmp_buff_cpy_timer));

    fatal_assert(0 == uv_timer_start(   &p_file_info->flb_tmp_buff_cpy_timer, 
                                        flb_complete_item_timer_timeout_cb, 0, 
                                        p_file_info->update_timeout * MSEC_PER_SEC));

    
    /* -------------------------------------------------------------------------
     * All set up successfully - add p_file_info to list of all p_file_info structs.
     * ------------------------------------------------------------------------- */
    p_file_infos_arr->data = reallocz(p_file_infos_arr->data, (++p_file_infos_arr->count) * (sizeof p_file_info));
    p_file_infos_arr->data[p_file_infos_arr->count - 1] = p_file_info;

    __atomic_store_n(&p_file_info->state, LOG_SRC_READY, __ATOMIC_RELAXED);

    collector_info("[%s]: initialization completed", p_file_info->chartname);
}

void config_file_load(  uv_loop_t *main_loop, 
                        Flb_socket_config_t *p_forward_in_config, 
                        flb_srvc_config_t *p_flb_srvc_config,
                        netdata_mutex_t *stdout_mut){
                            
    int user_default_conf_found = 0;

    struct section *config_section;

    char tmp_name[FILENAME_MAX + 1];
    snprintfz(tmp_name, FILENAME_MAX, "%s/logsmanagement.d", get_user_config_dir());
    DIR *dir = opendir(tmp_name);

    if(dir){
        struct dirent *de = NULL;
        while ((de = readdir(dir))) {
            size_t d_name_len = strlen(de->d_name);
            if (de->d_type == DT_DIR || d_name_len < 6 || strncmp(&de->d_name[d_name_len - 5], ".conf", sizeof(".conf")))
                continue;

            if(!user_default_conf_found && !strncmp(de->d_name, "default.conf", sizeof("default.conf")))
                user_default_conf_found = 1;

            snprintfz(tmp_name, FILENAME_MAX, "%s/logsmanagement.d/%s", get_user_config_dir(), de->d_name);
            collector_info("loading config:%s", tmp_name);
            log_management_config = (struct config){
                .first_section = NULL,
                .last_section = NULL,
                .mutex = NETDATA_MUTEX_INITIALIZER,
                .index = {
                        .avl_tree = {
                                .root = NULL,
                                .compar = appconfig_section_compare
                        },
                        .rwlock = AVL_LOCK_INITIALIZER
                }
            };
            if(!appconfig_load(&log_management_config, tmp_name, 0, NULL))
                continue;

            config_section = log_management_config.first_section;
            do {
                config_section_init(main_loop, config_section, p_forward_in_config, p_flb_srvc_config, stdout_mut);
                config_section = config_section->next;
            } while(config_section);

        }
        closedir(dir);
    }

    if(!user_default_conf_found){
        collector_info("CONFIG: cannot load user config '%s/logsmanagement.d/default.conf'. Will try stock config.", get_user_config_dir());
        snprintfz(tmp_name, FILENAME_MAX, "%s/logsmanagement.d/default.conf", get_stock_config_dir());
        if(!appconfig_load(&log_management_config, tmp_name, 0, NULL)){
            collector_error("CONFIG: cannot load stock config '%s/logsmanagement.d/default.conf'. Logs management will be disabled.", get_stock_config_dir());
            exit(1);
        }

        config_section = log_management_config.first_section;
        do {
            config_section_init(main_loop, config_section, p_forward_in_config, p_flb_srvc_config, stdout_mut);
            config_section = config_section->next;
        } while(config_section);
    }
}
