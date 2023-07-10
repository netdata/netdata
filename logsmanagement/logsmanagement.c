// SPDX-License-Identifier: GPL-3.0-or-later

/** @file logsmanagement.c
 *  @brief This is the main file of the Netdata logs management project
 *
 *  The aim of the project is to add the capability to collect, parse and
 *  query logs in the Netdata agent. For more information please refer 
 *  to the project's [README](README.md) file.
 */

#include "daemon/common.h"
#include <assert.h>
#include <inttypes.h>
#include <lz4.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <time.h>
#include <uv.h>
#include "circular_buffer.h"
#include "logsmanagement_conf.h"
#include "db_api.h"
#include "file_info.h"
#include "helper.h"
#include "query.h"
#include "parser.h"
#include "flb_plugin.h"
#include "rrd_api/rrd_api.h"
#include "rrd_api/rrd_api_stats.h"

#if defined(LOGS_MANAGEMENT_STRESS_TEST) && LOGS_MANAGEMENT_STRESS_TEST == 1
#include "query_test.h"
#endif  // defined(LOGS_MANAGEMENT_STRESS_TEST)

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


struct File_infos_arr *p_file_infos_arr = NULL;


static struct Chart_meta chart_types[] = {
    {.type = FLB_GENERIC,   .init = generic_chart_init,   .update = generic_chart_update},
    {.type = FLB_WEB_LOG,   .init = web_log_chart_init,   .update = web_log_chart_update},
    {.type = FLB_KMSG,      .init = kernel_chart_init,    .update = kernel_chart_update},
    {.type = FLB_SYSTEMD,   .init = systemd_chart_init,   .update = systemd_chart_update},
    {.type = FLB_DOCKER_EV, .init = docker_ev_chart_init, .update = docker_ev_chart_update},
    {.type = FLB_SYSLOG,    .init = systemd_chart_init,   .update = systemd_chart_update},
    {.type = FLB_SERIAL,    .init = generic_chart_init,   .update = generic_chart_update}
};

g_logs_manag_config_t g_logs_manag_config = {
    .update_every = 1,
    .update_timeout = UPDATE_TIMEOUT_DEFAULT,
    .use_log_timestamp = CONFIG_BOOLEAN_AUTO,
    .circ_buff_max_size_in_mib = CIRCULAR_BUFF_DEFAULT_MAX_SIZE / (1 MiB),
    .circ_buff_drop_logs = CIRCULAR_BUFF_DEFAULT_DROP_LOGS,
    .compression_acceleration = COMPRESSION_ACCELERATION_DEFAULT,
    .db_mode = GLOBAL_DB_MODE_DEFAULT,
    .disk_space_limit_in_mib = DISK_SPACE_LIMIT_DEFAULT,  
    .buff_flush_to_db_interval = SAVE_BLOB_TO_DB_DEFAULT,
    .enable_collected_logs_total = ENABLE_COLLECTED_LOGS_TOTAL_DEFAULT,
    .enable_collected_logs_rate = ENABLE_COLLECTED_LOGS_RATE_DEFAULT
};

static flb_srvc_config_t flb_srvc_config = {
    .flush          = "0.1",
    .http_listen    = "0.0.0.0",
    .http_port      = "2020",
    .http_server    = "false",
    .log_path       = "NULL",
    .log_level      = "info"
};

static logs_manag_db_mode_t db_mode_str_to_db_mode(const char *const db_mode_str){
    if(!db_mode_str || !*db_mode_str) return g_logs_manag_config.db_mode;
    else if(!strcasecmp(db_mode_str, "full")) return LOGS_MANAG_DB_MODE_FULL;
    else if(!strcasecmp(db_mode_str, "none")) return LOGS_MANAG_DB_MODE_NONE;
    else return g_logs_manag_config.db_mode;
}

static bool metrics_dict_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused){
    ((Kernel_metrics_dict_item_t *)old_value)->num += ((Kernel_metrics_dict_item_t *)new_value)->num;
    return true;
}

/** 
 * @brief Cleanup p_file_info struct
 * @param p_file_info The struct of File_info type to be cleaned up.
 * @todo  Pass p_file_info by reference, so that it can be set to NULL. */
static void p_file_info_destroy(struct File_info *p_file_info){

    // TODO: Clean up rrd / chart stuff.

    if(unlikely(!p_file_info)){
        collector_info("p_file_info_destroy() called but p_file_info == NULL - already destroyed?");
        return;
    }

    collector_info("[%s]: p_file_info_destroy() cleanup", p_file_info->chart_name ? p_file_info->chart_name : "Unknown");

    if(p_file_info->db_writer_thread){
        uv_thread_join(p_file_info->db_writer_thread);
        m_assert(0, "db_writer_thread joined");
    }   

    freez((void *) p_file_info->chart_name);
    freez(p_file_info->filename);
    freez((void *) p_file_info->file_basename);
    freez((void *) p_file_info->stream_guid);

    freez((void *) p_file_info->db_dir);
    freez((void *) p_file_info->db_metadata);

    for(int i = 1; i <= BLOB_MAX_FILES; i++){
        uv_fs_t close_req;
        uv_fs_close(NULL, &close_req, p_file_info->blob_handles[i], NULL);
    }

    if(p_file_info->circ_buff) circ_buff_destroy(p_file_info->circ_buff);
    
    if(p_file_info->parser_metrics){
        switch(p_file_info->log_type){
            case FLB_WEB_LOG: {
                freez(p_file_info->parser_metrics->web_log);
                break;
            }
            case FLB_KMSG: {
                freez(p_file_info->parser_metrics->kernel);
                break;
            }
            case FLB_SYSTEMD: 
            case FLB_SYSLOG: {
                freez(p_file_info->parser_metrics->systemd);
                break;
            }
            case FLB_DOCKER_EV: {
                freez(p_file_info->parser_metrics->docker_ev);
                break;
            }
            default:
                break;
        }   

        for(int i = 0; p_file_info->parser_cus_config && 
                       p_file_info->parser_metrics->parser_cus && 
                       p_file_info->parser_cus_config[i]; i++){
            freez(p_file_info->parser_cus_config[i]->chart_name);
            freez(p_file_info->parser_cus_config[i]->regex_str);
            freez(p_file_info->parser_cus_config[i]->regex_name);
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
    
    freez(p_file_info);
}

/**
 * @brief Load logs management configuration.
 * @return 0 if success, -1 if disabled in global config, 
 * -2 if config file not found
 */
static int logs_manag_config_load(Flb_socket_config_t **forward_in_config_p){
    char temp_path[FILENAME_MAX + 1];
    int rc = 0;

    if(!config_get_boolean(CONFIG_SECTION_LOGS_MANAGEMENT, "enabled", ENABLE_LOGS_MANAGEMENT_DEFAULT)){
        collector_info("CONFIG: Logs management disabled due to configuration option.");
        rc = -1;
    }

    g_logs_manag_config.update_every = (int)config_get_number(  CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                "update every", 
                                                                localhost->rrd_update_every);
    if(g_logs_manag_config.update_every < localhost->rrd_update_every) 
        g_logs_manag_config.update_every = localhost->rrd_update_every;
    collector_info("CONFIG: global logs management update_every: %d", g_logs_manag_config.update_every);

    g_logs_manag_config.update_timeout = (int)config_get_number(CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                "update timeout", 
                                                                UPDATE_TIMEOUT_DEFAULT);
    if(g_logs_manag_config.update_timeout < g_logs_manag_config.update_every) 
        g_logs_manag_config.update_timeout = g_logs_manag_config.update_every;
    collector_info("CONFIG: global logs management update_timeout: %d", g_logs_manag_config.update_timeout);

    g_logs_manag_config.use_log_timestamp = config_get_boolean_ondemand(CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                        "use log timestamp", 
                                                                        g_logs_manag_config.use_log_timestamp);
    collector_info("CONFIG: global logs management use_log_timestamp: %d", g_logs_manag_config.use_log_timestamp);

    g_logs_manag_config.circ_buff_max_size_in_mib = config_get_number(  CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                        "circular buffer max size MiB", 
                                                                        g_logs_manag_config.circ_buff_max_size_in_mib);
    collector_info("CONFIG: global logs management circ_buff_max_size_in_mib: %d", g_logs_manag_config.circ_buff_max_size_in_mib);

    g_logs_manag_config.circ_buff_drop_logs = config_get_boolean(   CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                    "circular buffer drop logs if full", 
                                                                    g_logs_manag_config.circ_buff_drop_logs);
    collector_info("CONFIG: global logs management circ_buff_drop_logs: %d", g_logs_manag_config.circ_buff_drop_logs);

    g_logs_manag_config.compression_acceleration = config_get_number(   CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                        "compression acceleration", 
                                                                        g_logs_manag_config.compression_acceleration);
    collector_info("CONFIG: global logs management compression_acceleration: %d", g_logs_manag_config.compression_acceleration);

    snprintfz(temp_path, FILENAME_MAX, "%s" LOGS_MANAG_DB_SUBPATH, netdata_configured_cache_dir);
    db_set_main_dir(config_get(CONFIG_SECTION_LOGS_MANAGEMENT, "db dir", temp_path));


    const char *const db_mode_str = config_get( CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                "db mode", 
                                                GLOBAL_DB_MODE_DEFAULT_STR);
    g_logs_manag_config.db_mode = db_mode_str_to_db_mode(db_mode_str);


    char *filename = strdupz_path_subpath(netdata_configured_user_config_dir, "logsmanagement.conf");
    if(!appconfig_load(&log_management_config, filename, 0, NULL)) {
        collector_info("CONFIG: cannot load user config '%s'. Will try stock config.", filename);
        freez(filename);

        filename = strdupz_path_subpath(netdata_configured_stock_config_dir, "logsmanagement.conf");
        if(!appconfig_load(&log_management_config, filename, 0, NULL)){
            collector_error("CONFIG: cannot load stock config '%s'. Logs management will be disabled.", filename);
            rc = -2;
        }
    }
    freez(filename);


    g_logs_manag_config.buff_flush_to_db_interval = config_get_number(  CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                        "circular buffer flush to db", 
                                                                        g_logs_manag_config.buff_flush_to_db_interval);
    collector_info( "CONFIG: global logs management buff_flush_to_db_interval: %d", 
                    g_logs_manag_config.buff_flush_to_db_interval);


    g_logs_manag_config.disk_space_limit_in_mib = config_get_number(CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                    "disk space limit MiB", 
                                                                    g_logs_manag_config.disk_space_limit_in_mib);
    collector_info( "CONFIG: global logs management disk_space_limit_in_mib: %d", 
                    g_logs_manag_config.disk_space_limit_in_mib);

    g_logs_manag_config.enable_collected_logs_total = config_get_boolean(CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                        "collected logs total chart enable", 
                                                                        g_logs_manag_config.enable_collected_logs_total);
    collector_info( "CONFIG: global logs management collected logs total chart enable: %d", 
                    g_logs_manag_config.enable_collected_logs_total);

    g_logs_manag_config.enable_collected_logs_rate = config_get_boolean(CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                        "collected logs rate chart enable", 
                                                                        g_logs_manag_config.enable_collected_logs_rate);
    collector_info( "CONFIG: global logs management collected logs rate chart enable: %d", 
                    g_logs_manag_config.enable_collected_logs_rate);

    *forward_in_config_p = (Flb_socket_config_t *) callocz(1, sizeof(Flb_socket_config_t));
    const int fwd_enable = config_get_boolean(CONFIG_SECTION_LOGS_MANAGEMENT, "forward in enable", CONFIG_BOOLEAN_NO);
    
    (*forward_in_config_p)->unix_path = config_get(CONFIG_SECTION_LOGS_MANAGEMENT, "forward in unix path", FLB_FORWARD_UNIX_PATH_DEFAULT);
    collector_info("forward in unix path = %s", (*forward_in_config_p)->unix_path);
    (*forward_in_config_p)->unix_perm = config_get(CONFIG_SECTION_LOGS_MANAGEMENT, "forward in unix perm", FLB_FORWARD_UNIX_PERM_DEFAULT);
    collector_info("forward in unix perm = %s", (*forward_in_config_p)->unix_perm);
    // TODO: Check if listen is in valid format
    (*forward_in_config_p)->listen = config_get(CONFIG_SECTION_LOGS_MANAGEMENT, "forward in listen", FLB_FORWARD_ADDR_DEFAULT);
    collector_info("forward in listen = %s", (*forward_in_config_p)->listen);
    (*forward_in_config_p)->port = config_get(CONFIG_SECTION_LOGS_MANAGEMENT, "forward in port", FLB_FORWARD_PORT_DEFAULT);
    collector_info("forward in port = %s", (*forward_in_config_p)->port);

    snprintfz(temp_path, FILENAME_MAX, "%s/fluentbit.log", netdata_configured_log_dir);
    
    flb_srvc_config.flush = config_get(         CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                "fluent bit flush", flb_srvc_config.flush);
    flb_srvc_config.http_listen = config_get(   CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                "fluent bit http listen", flb_srvc_config.http_listen);
    flb_srvc_config.http_port = config_get(     CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                "fluent bit http port", flb_srvc_config.http_port);
    flb_srvc_config.http_server = config_get(   CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                "fluent bit http server", flb_srvc_config.http_server);
    flb_srvc_config.log_path = config_get(      CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                "fluent bit log file", temp_path);
    flb_srvc_config.log_level = config_get(     CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                "fluent bit log level", flb_srvc_config.log_level);


    // if(!fwd_enable) flb_socket_config_destroy((*forward_in_config_p));

    return rc;
}

#define FLB_OUTPUT_PLUGIN_NAME_KEY "plugin"

static int flb_output_param_get_cb(void *entry, void *data){
    struct config_option *option = (struct config_option *) entry;
    Flb_output_config_t *flb_output = (Flb_output_config_t *) data;
    
    char *param_prefix = callocz(1, snprintf(NULL, 0, "output %d", MAX_OUTPUTS_PER_SOURCE) + 1);
    sprintf(param_prefix, "output %d", flb_output->id);
    size_t param_prefix_len = strlen(param_prefix);
    
    if(!strncasecmp(option->name, param_prefix, param_prefix_len)){ // param->name looks like "output 1 host"
        char *param_key = &option->name[param_prefix_len]; // param_key should look like " host"
        while(*param_key == ' ') param_key++; // remove whitespace so it looks like "host"
        
        if(*param_key && strcasecmp(param_key, FLB_OUTPUT_PLUGIN_NAME_KEY)){ // ignore param_key "plugin" 
            // debug(D_LOGS_MANAG, "config_option: name[%s], value[%s]", option->name, option->value);
            // debug(D_LOGS_MANAG, "config option kv:[%s][%s]", param_key, option->value);

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
static void logs_management_init(uv_loop_t *main_loop, 
                                struct section *config_section, 
                                Flb_socket_config_t *forward_in_config){

    struct File_info *p_file_info = callocz(1, sizeof(struct File_info));

    /* -------------------------------------------------------------------------
     * Check if config_section->name is valid and if so, use it as chart_name.
     * ------------------------------------------------------------------------- */
    if(config_section->name && *config_section->name){
        p_file_info->chart_name = strdupz(config_section->name);
        collector_info("[%s]: Initializing config loading", p_file_info->chart_name);
    } else {
        collector_error("Invalid logs management config section.");
        return p_file_info_destroy(p_file_info);
    }
    

    /* -------------------------------------------------------------------------
     * Check if this log source is enabled.
     * ------------------------------------------------------------------------- */
    if(appconfig_get_boolean(&log_management_config, config_section->name, "enabled", CONFIG_BOOLEAN_NO)){
        collector_info("[%s]: enabled = yes", p_file_info->chart_name);
    } else {
        collector_info("[%s]: enabled = no", p_file_info->chart_name);
        return p_file_info_destroy(p_file_info);
    }


    /* -------------------------------------------------------------------------
     * Check log type.
     * ------------------------------------------------------------------------- */
    char *type = appconfig_get(&log_management_config, config_section->name, "log type", "flb_generic");
    if(!type || !*type) p_file_info->log_type = FLB_GENERIC; // Default
    else{
        if(!strcasecmp(type, "flb_generic")) p_file_info->log_type = FLB_GENERIC;
        else if (!strcasecmp(type, "flb_web_log")) p_file_info->log_type = FLB_WEB_LOG;
        else if (!strcasecmp(type, "flb_kmsg")) p_file_info->log_type = FLB_KMSG;
        else if (!strcasecmp(type, "flb_systemd")) p_file_info->log_type = FLB_SYSTEMD;
        else if (!strcasecmp(type, "flb_docker_events")) p_file_info->log_type = FLB_DOCKER_EV;
        else if (!strcasecmp(type, "flb_syslog")) p_file_info->log_type = FLB_SYSLOG;
        else if (!strcasecmp(type, "flb_serial")) p_file_info->log_type = FLB_SERIAL;
        else p_file_info->log_type = FLB_GENERIC;
    }
    freez(type);
    collector_info("[%s]: log type = %s", p_file_info->chart_name, log_src_type_t_str[p_file_info->log_type]);


    /* -------------------------------------------------------------------------
     * Read log source.
     * ------------------------------------------------------------------------- */
    char *source = appconfig_get(&log_management_config, config_section->name, "log source", "local");
    if(!source || !*source) p_file_info->log_source = LOG_SOURCE_LOCAL; // Default
    else if(!strcasecmp(source, "forward")) p_file_info->log_source = LOG_SOURCE_FORWARD;
    else p_file_info->log_source = LOG_SOURCE_LOCAL;
    freez(source);
    collector_info("[%s]: log source = %s", p_file_info->chart_name, log_src_t_str[p_file_info->log_source]);

    if(p_file_info->log_source == LOG_SOURCE_FORWARD && !forward_in_config){
        collector_info("[%s]: forward_in_config == NULL - this log source will be disabled", p_file_info->chart_name);
        return p_file_info_destroy(p_file_info);
    }


    /* -------------------------------------------------------------------------
     * Read stream uuid.
     * ------------------------------------------------------------------------- */
    p_file_info->stream_guid = appconfig_get(&log_management_config, config_section->name, "stream guid", "");
    collector_info("[%s]: stream guid = %s", p_file_info->chart_name, p_file_info->stream_guid);


    /* -------------------------------------------------------------------------
     * Read log path configuration and check if it is valid.
     * ------------------------------------------------------------------------- */
    p_file_info->filename = appconfig_get(&log_management_config, config_section->name, "log path", "auto");
    m_assert(p_file_info->filename, "appconfig_get() should never return log path == NULL");
    if( /* path doesn't matter when log source is not local */
        (p_file_info->log_source == LOG_SOURCE_LOCAL) &&
        
        /* FLB_SYSLOG is special case, may or may not require path */
        (p_file_info->log_type != FLB_SYSLOG) &&
        
        (!p_file_info->filename /* Sanity check */ || 
         !*p_file_info->filename || 
         !strcmp(p_file_info->filename, "auto") || 
         access(p_file_info->filename, R_OK)
        )){ 

        freez(p_file_info->filename);
        p_file_info->filename = NULL;
            
        switch(p_file_info->log_type){
            case FLB_GENERIC:
                if(!strcmp(p_file_info->chart_name, "Netdata error.log")){
                    char path[FILENAME_MAX + 1];
                    snprintfz(path, FILENAME_MAX, "%s/error.log", netdata_configured_log_dir);
                    if(access(path, R_OK)) {
                        collector_error("[%s]: Netdata error.log path (%s) invalid, unknown or needs permissions", 
                            p_file_info->chart_name, path);
                        return p_file_info_destroy(p_file_info);
                    } else p_file_info->filename = strdupz(path);
                } else if(!strcasecmp(p_file_info->chart_name, "Netdata fluentbit.log")){
                    if(access(flb_srvc_config.log_path, R_OK)){
                        collector_error("[%s]: Netdata fluentbit.log path (%s) invalid, unknown or needs permissions", 
                            p_file_info->chart_name, flb_srvc_config.log_path);
                        return p_file_info_destroy(p_file_info);
                    } else p_file_info->filename = strdupz(flb_srvc_config.log_path);
                } else if(!strcasecmp(p_file_info->chart_name, "Auth.log tail")){
                    const char * const auth_path_default[] = {
                        "/var/log/auth.log",
                        NULL
                    };
                    int i = 0;
                    while(auth_path_default[i] && access(auth_path_default[i], R_OK)){i++;};
                    if(!auth_path_default[i]){
                        collector_error("[%s]: auth.log path invalid, unknown or needs permissions", p_file_info->chart_name);
                        return p_file_info_destroy(p_file_info);
                    } else p_file_info->filename = strdupz(auth_path_default[i]);
                } else if(!strcasecmp(p_file_info->chart_name, "syslog tail")){
                    const char * const syslog_path_default[] = {
                        "/var/log/syslog",   /* Debian, Ubuntu */
                        "/var/log/messages", /* RHEL, Red Hat, CentOS, Fedora */
                        NULL
                    };
                    int i = 0;
                    while(syslog_path_default[i] && access(syslog_path_default[i], R_OK)){i++;};
                    if(!syslog_path_default[i]){
                        collector_error("[%s]: syslog path invalid, unknown or needs permissions", p_file_info->chart_name);
                        return p_file_info_destroy(p_file_info);
                    } else p_file_info->filename = strdupz(syslog_path_default[i]);
                }
                break;
            case FLB_WEB_LOG:
                if(!strcasecmp(p_file_info->chart_name, "Apache access.log")){
                    const char * const apache_access_path_default[] = {
                        "/var/log/apache/access.log",
                        "/var/log/apache2/access.log", /* Debian, Ubuntu */
                        "/var/log/apache2/access_log", /* Gentoo ? */
                        "/var/log/httpd/access_log",  /* RHEL, Red Hat, CentOS, Fedora */
                        "/var/log/httpd-access.log",   /* FreeBSD */
                        NULL
                    };
                    int i = 0;
                    while(apache_access_path_default[i] && access(apache_access_path_default[i], R_OK)){i++;};
                    if(!apache_access_path_default[i]){
                        collector_error("[%s]: Apache access.log path invalid, unknown or needs permissions", p_file_info->chart_name);
                        return p_file_info_destroy(p_file_info);
                    } else p_file_info->filename = strdupz(apache_access_path_default[i]);
                } else if(!strcasecmp(p_file_info->chart_name, "Nginx access.log")){
                    const char * const nginx_access_path_default[] = {
                        "/var/log/nginx/access.log",
                        NULL
                    };
                    int i = 0;
                    while(nginx_access_path_default[i] && access(nginx_access_path_default[i], R_OK)){i++;};
                    if(!nginx_access_path_default[i]){
                        collector_error("[%s]: Nginx access.log path invalid, unknown or needs permissions", p_file_info->chart_name);
                        return p_file_info_destroy(p_file_info);
                    } else p_file_info->filename = strdupz(nginx_access_path_default[i]);
                }
                break;
            case FLB_KMSG:
                p_file_info->filename = strdupz(KMSG_DEFAULT_PATH);
                break;
            case FLB_SYSTEMD:
                p_file_info->filename = strdupz(SYSTEMD_DEFAULT_PATH);
                break;
            case FLB_DOCKER_EV:
                if(access(DOCKER_EV_DEFAULT_PATH, R_OK)){
                    collector_error("[%s]: Docker socket Unix path invalid, unknown or needs permissions", p_file_info->chart_name);
                    return p_file_info_destroy(p_file_info);
                } else p_file_info->filename = strdupz(DOCKER_EV_DEFAULT_PATH);
                break;
            default:
                collector_error("[%s]: log path invalid or unknown", p_file_info->chart_name);
                return p_file_info_destroy(p_file_info);
        }
    }
    p_file_info->file_basename = get_basename(p_file_info->filename); 
    collector_info("[%s]: p_file_info->filename: %s", p_file_info->chart_name, 
                                            p_file_info->filename ? p_file_info->filename : "NULL");
    collector_info("[%s]: p_file_info->file_basename: %s", p_file_info->chart_name, 
                                                 p_file_info->file_basename ? p_file_info->file_basename : "NULL");
    if(unlikely(!p_file_info->filename)) return p_file_info_destroy(p_file_info);


    /* -------------------------------------------------------------------------
     * Read "update every" and "update timeout" configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->update_every = appconfig_get_number(   &log_management_config, config_section->name, 
                                                        "update every", g_logs_manag_config.update_every);
    collector_info("[%s]: update every = %d", p_file_info->chart_name, p_file_info->update_every);

    p_file_info->update_timeout = appconfig_get_number( &log_management_config, config_section->name, 
                                                        "update timeout", g_logs_manag_config.update_timeout);
    if(p_file_info->update_timeout < p_file_info->update_every) p_file_info->update_timeout = p_file_info->update_every;
    collector_info("[%s]: update timeout = %d", p_file_info->chart_name, p_file_info->update_timeout);


    /* -------------------------------------------------------------------------
     * Read "use log timestamp" configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->use_log_timestamp = appconfig_get_boolean_ondemand(&log_management_config, config_section->name, 
                                                                    "use log timestamp", 
                                                                    g_logs_manag_config.use_log_timestamp);
    collector_info("[%s]: use log timestamp = %s", p_file_info->chart_name, 
                                                    p_file_info->use_log_timestamp ? "auto or yes" : "no");


    /* -------------------------------------------------------------------------
     * Read compression acceleration configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->compression_accel = appconfig_get_number(  &log_management_config, config_section->name, 
                                                            "compression acceleration", 
                                                            g_logs_manag_config.compression_acceleration);
    collector_info("[%s]: compression acceleration = %d", p_file_info->chart_name, p_file_info->compression_accel);


    /* -------------------------------------------------------------------------
     * Read DB mode.
     * ------------------------------------------------------------------------- */
    const char *const db_mode_str = appconfig_get(&log_management_config, config_section->name, "db mode", NULL);
    collector_info("[%s]: db mode = %s", p_file_info->chart_name, db_mode_str ? db_mode_str : "NULL");
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
                p_file_info->chart_name, p_file_info->buff_flush_to_db_interval);

    } else if(p_file_info->buff_flush_to_db_interval < SAVE_BLOB_TO_DB_MIN) {
        p_file_info->buff_flush_to_db_interval = SAVE_BLOB_TO_DB_MIN;
        collector_info("[%s]: circular buffer flush to db out of range. Using minimum permitted value: %d",
                p_file_info->chart_name, p_file_info->buff_flush_to_db_interval);
    } 
    collector_info("[%s]: circular buffer flush to db = %d", p_file_info->chart_name, p_file_info->buff_flush_to_db_interval);


    /* -------------------------------------------------------------------------
     * Read BLOB max size configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->blob_max_size  = appconfig_get_number( &log_management_config, config_section->name, 
                                                        "disk space limit MiB", 
                                                        g_logs_manag_config.disk_space_limit_in_mib) MiB / BLOB_MAX_FILES;
    collector_info("[%s]: BLOB max size = %lld", p_file_info->chart_name, (long long)p_file_info->blob_max_size);


    /* -------------------------------------------------------------------------
     * Read collected logs chart configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->parser_config = callocz(1, sizeof(Log_parser_config_t));

    if(appconfig_get_boolean(&log_management_config, config_section->name, 
                             "collected logs total chart enable",
                             g_logs_manag_config.enable_collected_logs_total)){
        p_file_info->parser_config->chart_config |= CHART_COLLECTED_LOGS_TOTAL;
    }
    collector_info( "[%s]: collected logs total chart enable = %s",  p_file_info->chart_name, 
                    (p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_TOTAL) ? "yes" : "no");

    if(appconfig_get_boolean(&log_management_config, config_section->name, 
                             "collected logs rate chart enable",
                             g_logs_manag_config.enable_collected_logs_rate)){
        p_file_info->parser_config->chart_config |= CHART_COLLECTED_LOGS_RATE;
    }
    collector_info( "[%s]: collected logs rate chart enable = %s",  p_file_info->chart_name, 
                    (p_file_info->parser_config->chart_config & CHART_COLLECTED_LOGS_RATE) ? "yes" : "no");


    /* -------------------------------------------------------------------------
     * Deal with log-type-specific configuration options.
     * ------------------------------------------------------------------------- */
    
    if(p_file_info->log_type == FLB_GENERIC){/* Do nothing */}
    else if(p_file_info->log_type == FLB_WEB_LOG){
        /* Check if a valid web log format configuration is detected */
        char *log_format = appconfig_get(&log_management_config, config_section->name, "log format", "auto");
        const char delimiter = ' '; // TODO!!: TO READ FROM CONFIG
        collector_info("[%s]: log format = %s", p_file_info->chart_name, log_format ? log_format : "NULL!");

        /* If "log format = auto" or no "log format" config is detected, 
            * try log format autodetection based on last log file line.
            * TODO 1: Add another case in OR where log_format is compared with a valid reg exp.
            * TODO 2: Set default log format and delimiter if not found in config? Or auto-detect? */ 
        if(!log_format || !*log_format || !strcmp(log_format, "auto")){ 
            collector_info("[%s]: Attempting auto-detection of log format", p_file_info->chart_name);
            char *line = read_last_line(p_file_info->filename, 0);
            if(!line){
                collector_error("[%s]: read_last_line() returned NULL", p_file_info->chart_name);
                return p_file_info_destroy(p_file_info);
            }
            p_file_info->parser_config->gen_config = auto_detect_web_log_parser_config(line, delimiter);
            freez(line);
        }
        else{
            p_file_info->parser_config->gen_config = read_web_log_parser_config(log_format, delimiter);
            collector_info( "[%s]: Read web log parser config: %s", p_file_info->chart_name, 
                    p_file_info->parser_config->gen_config ? "success!" : "failed!");
        }
        freez(log_format);

        if(!p_file_info->parser_config->gen_config){
            collector_error("[%s]: No valid web log parser config found", p_file_info->chart_name);
            return p_file_info_destroy(p_file_info); 
        }

        /* Check whether metrics verification during parsing is required */
        Web_log_parser_config_t *wblp_config = (Web_log_parser_config_t *) p_file_info->parser_config->gen_config;
        wblp_config->verify_parsed_logs = appconfig_get_boolean( &log_management_config, config_section->name, 
                                                                    "verify parsed logs", CONFIG_BOOLEAN_NO);
        collector_info("[%s]: verify parsed logs = %d", p_file_info->chart_name, wblp_config->verify_parsed_logs);
        
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
            Syslog_parser_config_t *syslog_config = (Syslog_parser_config_t *) callocz(1, sizeof(Syslog_parser_config_t));

            /* Read syslog format */
            syslog_config->log_format = appconfig_get(  &log_management_config, 
                                                        config_section->name, 
                                                        "log format", NULL);
            collector_info("[%s]: log format = %s", p_file_info->chart_name, 
                                                    syslog_config->log_format ? syslog_config->log_format : "NULL!");
            if(!syslog_config->log_format || !*syslog_config->log_format || !strcasecmp(syslog_config->log_format, "auto")){
                freez(syslog_config->log_format);
                freez(syslog_config);
                return p_file_info_destroy(p_file_info);
            }

            syslog_config->socket_config = (Flb_socket_config_t *) callocz(1, sizeof(Flb_socket_config_t));

            /* Read syslog socket mode
             * see also https://docs.fluentbit.io/manual/pipeline/inputs/syslog#configuration-parameters */
            syslog_config->socket_config->mode = appconfig_get( &log_management_config, 
                                                                config_section->name, 
                                                                "mode", "unix_udp");
            collector_info("[%s]: mode = %s", p_file_info->chart_name, syslog_config->socket_config->mode);

            /* Check for valid socket path if (mode == unix_udp) or 
             * (mode == unix_tcp), else read syslog network interface to bind, 
             * if (mode == udp) or (mode == tcp). */
            if( !strcasecmp(syslog_config->socket_config->mode, "unix_udp") || 
                !strcasecmp(syslog_config->socket_config->mode, "unix_tcp")){
                if(!p_file_info->filename || !*p_file_info->filename || !strcasecmp(p_file_info->filename, "auto")){
                    // freez(syslog_config->socket_config->mode);
                    freez(syslog_config->socket_config);
                    freez(syslog_config->log_format);
                    freez(syslog_config);
                    return p_file_info_destroy(p_file_info);
                }
                syslog_config->socket_config->unix_perm = appconfig_get(&log_management_config, 
                                                                        config_section->name, 
                                                                        "unix_perm", "0644");
                collector_info("[%s]: unix_perm = %s", p_file_info->chart_name, syslog_config->socket_config->unix_perm);
            } else if(  !strcasecmp(syslog_config->socket_config->mode, "udp") || 
                        !strcasecmp(syslog_config->socket_config->mode, "tcp")){
                // TODO: Check if listen is in valid format
                syslog_config->socket_config->listen = appconfig_get(   &log_management_config, 
                                                                        config_section->name, 
                                                                        "listen", "0.0.0.0");
                collector_info("[%s]: listen = %s", p_file_info->chart_name, syslog_config->socket_config->listen);
                syslog_config->socket_config->port = appconfig_get( &log_management_config, 
                                                                    config_section->name, 
                                                                    "port", "5140");
                collector_info("[%s]: port = %s", p_file_info->chart_name, syslog_config->socket_config->port);
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
    }
    else if(p_file_info->log_type == FLB_SERIAL){
        Flb_serial_config_t *serial_config = (Flb_serial_config_t *) callocz(1, sizeof(Flb_serial_config_t));

        serial_config->bitrate = appconfig_get(&log_management_config, config_section->name, "bitrate", "115200");
        serial_config->min_bytes = appconfig_get(&log_management_config, config_section->name, "min bytes", "1");
        serial_config->separator = appconfig_get(&log_management_config, config_section->name, "separator", "");
        serial_config->format = appconfig_get(&log_management_config, config_section->name, "format", "");

        p_file_info->flb_config = serial_config;
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
        debug(D_LOGS_MANAG, "cus chart: (%s:%s)", cus_chart_k, cus_chart_v ? cus_chart_v : "NULL");
        freez(cus_chart_k);
        if(unlikely(!cus_chart_v)){
            collector_error("[%s]: custom %d chart = NULL, custom charts for this log source will be disabled.", 
                            p_file_info->chart_name, cus_off);
            break;
        }

        /* Read regex config */
        char *cus_regex_k = mallocz(snprintf(NULL, 0, "custom %d regex", MAX_CUS_CHARTS_PER_SOURCE) + 1);
        sprintf(cus_regex_k, "custom %d regex", cus_off);
        char *cus_regex_v = appconfig_get(&log_management_config, config_section->name, cus_regex_k, NULL);
        debug(D_LOGS_MANAG, "cus regex:(%s:%s)", cus_regex_k, cus_regex_v ? cus_regex_v : "NULL");
        freez(cus_regex_k);
        if(unlikely(!cus_regex_v)) {
            collector_error("[%s]: custom %d regex = NULL, custom charts for this log source will be disabled.", 
                            p_file_info->chart_name, cus_off);
            freez(cus_chart_v);
            break;
        }

        /* Read regex name config */
        char *cus_regex_name_k = mallocz(snprintf(NULL, 0, "custom %d regex name", MAX_CUS_CHARTS_PER_SOURCE) + 1);
        sprintf(cus_regex_name_k, "custom %d regex name", cus_off);
        char *cus_regex_name_v = appconfig_get( &log_management_config, config_section->name, 
                                                cus_regex_name_k, strdupz(cus_regex_v));
        debug(D_LOGS_MANAG, "cus regex name: (%s:%s)", cus_regex_name_k, cus_regex_name_v ? cus_regex_name_v : "NULL");
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
        debug(D_LOGS_MANAG, "cus case: (%s:%s)", cus_ignore_case_k, cus_ignore_case_v ? "yes" : "no");
        freez(cus_ignore_case_k); 

        int regex_flags = cus_ignore_case_v ? REG_EXTENDED | REG_NEWLINE | REG_ICASE : REG_EXTENDED | REG_NEWLINE;
        
        int rc;
        regex_t regex;
        if (unlikely((rc = regcomp(&regex, cus_regex_v, regex_flags)))){
            size_t regcomp_err_str_size = regerror(rc, &regex, 0, 0);
            char *regcomp_err_str = mallocz(regcomp_err_str_size);
            regerror(rc, &regex, regcomp_err_str, regcomp_err_str_size);
            collector_error("[%s]: could not compile regex for custom %d chart: %s, custom charts for this log source will be disabled.", 
                            p_file_info->chart_name, cus_off, cus_chart_v);
            freez(cus_chart_v);
            freez(cus_regex_v);
            freez(cus_regex_name_v);
            break;
        };

        /* Allocate memory and copy config to p_file_info->parser_cus_config struct */
        p_file_info->parser_cus_config = reallocz(  p_file_info->parser_cus_config, 
                                                    (cus_off + 1) * sizeof(Log_parser_cus_config_t *));
        p_file_info->parser_cus_config[cus_off - 1] = callocz(1, sizeof(Log_parser_cus_config_t));

        p_file_info->parser_cus_config[cus_off - 1]->chart_name = cus_chart_v;
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
        debug(D_LOGS_MANAG, "output %d "FLB_OUTPUT_PLUGIN_NAME_KEY": %s", out_off, out_plugin_v ? out_plugin_v : "NULL");
        freez(out_plugin_k);
        if(unlikely(!out_plugin_v)){
            collector_error("[%s]: output %d "FLB_OUTPUT_PLUGIN_NAME_KEY" = NULL, outputs for this log source will be disabled.", 
                            p_file_info->chart_name, out_off);
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
                p_file_info->chart_name, (size_t) (circular_buffer_max_size / (1 MiB)));
    } else if(circular_buffer_max_size < CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN) {
        circular_buffer_max_size = CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN;
        collector_info( "[%s]: circular buffer max size out of range. Using minimum permitted value (MiB): %zu", 
                p_file_info->chart_name, (size_t) (circular_buffer_max_size / (1 MiB)));
    } 
    collector_info("[%s]: circular buffer max size MiB = %zu", p_file_info->chart_name, (size_t) (circular_buffer_max_size / (1 MiB)));

    int circular_buffer_allow_dropped_logs = appconfig_get_boolean( &log_management_config, 
                                                                    config_section->name,
                                                                    "circular buffer drop logs if full", 
                                                                    g_logs_manag_config.circ_buff_drop_logs);
    collector_info("[%s]: circular buffer drop logs if full = %s", p_file_info->chart_name, 
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
    p_file_info->chart_meta->init(p_file_info);


    /* -------------------------------------------------------------------------
     * Initialize input plugin for local log sources.
     * ------------------------------------------------------------------------- */
    switch(p_file_info->log_type){
        int rc;
        case FLB_GENERIC:
        case FLB_WEB_LOG:
        case FLB_KMSG:
        case FLB_SYSTEMD:
        case FLB_DOCKER_EV:
        case FLB_SYSLOG:
        case FLB_SERIAL: {
            if(p_file_info->log_source == LOG_SOURCE_LOCAL){
                rc = flb_add_input(p_file_info);
                if(unlikely(rc)){
                    collector_error("[%s]: flb_add_input() error: %d", p_file_info->chart_name, rc);
                    return p_file_info_destroy(p_file_info);
                }
            }

            /* flb_complete_item_timer_timeout_cb() is needed for 
             * both local and non-local sources. */
            p_file_info->flb_tmp_buff_cpy_timer.data = p_file_info;
            if(unlikely(0 != uv_mutex_init(&p_file_info->flb_tmp_buff_mut))){
                fatal("uv_mutex_init(&p_file_info->flb_tmp_buff_mut) failed");
            }
            uv_timer_init(main_loop, &p_file_info->flb_tmp_buff_cpy_timer);
            uv_timer_start( &p_file_info->flb_tmp_buff_cpy_timer, 
                            (uv_timer_cb)flb_complete_item_timer_timeout_cb, 
                            0, p_file_info->update_timeout * MSEC_PER_SEC);            
            break;
        }
        default: 
            return p_file_info_destroy(p_file_info);
    }


    /* -------------------------------------------------------------------------
     * All set up successfully - add p_file_info to list of all p_file_info structs.
     * ------------------------------------------------------------------------- */
    p_file_infos_arr->data = reallocz(p_file_infos_arr->data, (++p_file_infos_arr->count) * (sizeof p_file_info));
    p_file_infos_arr->data[p_file_infos_arr->count - 1] = p_file_info;

    collector_info("[%s]: initialization completed", p_file_info->chart_name);
}

static void logsmanagement_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    collector_info("cleaning up...");

    flb_terminate();

    if(p_file_infos_arr){
        for(int i = 0; i < p_file_infos_arr->count; i++){
            p_file_info_destroy(p_file_infos_arr->data[i]);
        }
        freez(p_file_infos_arr);
        p_file_infos_arr = NULL;
    }

    // TODO: Clean up stats charts memory

    // TODO: Additional work to do here on exit? Maybe flush buffers to DB?

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

/**
 * @brief The main function of the program.
 * @details Any static asserts are most likely going to be inluded here. After 
 * any initialisation routines, the default uv_loop_t is executed indefinitely. 
 * @todo Any cleanup required on program exit? 
 */
void *logsmanagement_main(void *ptr) {
    netdata_thread_cleanup_push(logsmanagement_main_cleanup, ptr);

    Flb_socket_config_t *forward_in_config = NULL;

    uv_loop_t *main_loop = mallocz(sizeof(uv_loop_t));
    fatal_assert(uv_loop_init(main_loop) == 0);

    if(logs_manag_config_load(&forward_in_config)) goto cleanup;

    /* Static asserts */
    #pragma GCC diagnostic push
    #pragma GCC diagnostic ignored "-Wunused-local-typedefs" 
    /* Ensure VALIDATE_COMPRESSION is disabled in release versions. */
    // COMPILE_TIME_ASSERT(LOGS_MANAG_DEBUG ? 1 : !VALIDATE_COMPRESSION);
    COMPILE_TIME_ASSERT(SAVE_BLOB_TO_DB_MIN <= SAVE_BLOB_TO_DB_MAX);
    COMPILE_TIME_ASSERT(CIRCULAR_BUFF_DEFAULT_MAX_SIZE >= CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN); 
    COMPILE_TIME_ASSERT(CIRCULAR_BUFF_DEFAULT_MAX_SIZE <= CIRCULAR_BUFF_MAX_SIZE_RANGE_MAX);
    #pragma GCC diagnostic pop

    /* Initialize array of File_Info pointers. */
    p_file_infos_arr = callocz(1, sizeof(struct File_infos_arr));

    if(flb_init(flb_srvc_config)){
        collector_error("flb_init() failed - logs management will be disabled");
        goto cleanup;
    }

    if(flb_add_fwd_input(forward_in_config)){
        collector_error("flb_add_fwd_input() failed - logs management forward input will be disabled");
        flb_socket_config_destroy(forward_in_config);
    }

    /* Initialize logs management for each configuration section  */
    struct section *config_section = log_management_config.first_section;
    do {
        logs_management_init(main_loop, config_section, forward_in_config);
        config_section = config_section->next;
    } while(config_section);
    if(p_file_infos_arr->count == 0){
        collector_info("No valid configuration could be found for any log source - logs management will be disabled");
        goto cleanup; // No log sources - nothing to do
    }

    stats_charts_init();
    uv_timer_t stats_charts_timer;
    uv_timer_init(main_loop, &stats_charts_timer);

    // TODO: stats_charts_update() is run as a timer callback, so not as precise as if heartbeat_next() was used.
    // Needs to be changed ideally.
    uv_timer_start(&stats_charts_timer, stats_charts_update, 0, g_logs_manag_config.update_every * MSEC_PER_SEC); 

    /* Run Fluent Bit engine
     * NOTE: flb_run() ideally would be executed after db_init(), but in case of
     * a db_init() failure, it is easier to call flb_stop_and_cleanup() rather 
     * than the other way round (i.e. cleaning up after db_init(), if flb_run() 
     * fails). */
    if(flb_run()){
        collector_error("flb_run() failed - logs management will be disabled");
        goto cleanup;
    }

    if(db_init()){
        collector_error("db_init() failed - logs management will be disabled");
        goto cleanup;
    }
    
#if defined(__STDC_VERSION__)
    debug(D_LOGS_MANAG, "__STDC_VERSION__: %ld", __STDC_VERSION__);
#else
    debug(D_LOGS_MANAG, "__STDC_VERSION__ undefined");
#endif // defined(__STDC_VERSION__)
    debug(D_LOGS_MANAG, "libuv version: %s", uv_version_string());
    debug(D_LOGS_MANAG, "LZ4 version: %s\n", LZ4_versionString());
#if defined(D_LOGS_MANAG)
    char *sqlite_version = db_get_sqlite_version();
    debug(D_LOGS_MANAG, "SQLITE version: %s\n", sqlite_version ? sqlite_version : "NULL");
    freez(sqlite_version);
#endif // defined(D_LOGS_MANAG)

#if defined(LOGS_MANAGEMENT_STRESS_TEST) && LOGS_MANAGEMENT_STRESS_TEST == 1
    debug(D_LOGS_MANAG, "Running Netdata with logs_management stress test enabled!");
    static uv_thread_t run_stress_test_queries_thread_id;
    uv_thread_create(&run_stress_test_queries_thread_id, run_stress_test_queries_thread, NULL);
#endif  // LOGS_MANAGEMENT_STRESS_TEST

    collector_info("logsmanagement_main() setup completed successfully");

    /* Run uvlib loop. */
    uv_run(main_loop, UV_RUN_DEFAULT);

    /* If there are valid log sources, there should always be valid handles */
    collector_error("uv_run(main_loop, ...); - no handles or requests - exiting");

cleanup:
    flb_socket_config_destroy(forward_in_config);
    uv_stop(main_loop);
    uv_loop_close(main_loop);
    freez(main_loop);
    netdata_thread_cleanup_pop(1);
    return NULL;
}
