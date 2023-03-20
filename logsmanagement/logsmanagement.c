/** @file main.c
 *  @brief This is the main file of the netdata-logs project
 *
 *  The aim of the project is to add the capability to collect
 *  logs in the netdata agent and store them in a database for
 *  querying. main.c uses libuv and its callbacks mechanism to
 *  setup a listener for each log source. 
 *
 *  @author Dimitris Pantazis
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
#include "tail_plugin.h"

/* Extra options and includes when building with stress test support */
#if defined(LOGS_MANAGEMENT_STRESS_TEST) 
#define NETDATA_CONF_ENABLE_LOGS_MANAGEMENT_DEFAULT 1
#if LOGS_MANAGEMENT_STRESS_TEST == 1
#include "query_test.h"
#endif
#else 
#define NETDATA_CONF_ENABLE_LOGS_MANAGEMENT_DEFAULT 0
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
uv_loop_t *main_loop = NULL; 

volatile sig_atomic_t p_file_infos_arr_ready = 0;

g_logs_manag_config_t g_logs_manag_config = {
    .update_every = 1,
    .circ_buff_spare_items = CIRCULAR_BUFF_SPARE_ITEMS_DEFAULT,
    .circ_buff_max_size_in_mib = CIRCULAR_BUFF_DEFAULT_MAX_SIZE / (1 MiB),
    .circ_buff_drop_logs = CIRCULAR_BUFF_DEFAULT_DROP_LOGS,
    .compression_acceleration = COMPRESSION_ACCELERATION_DEFAULT,
    .db_mode = GLOBAL_DB_MODE_DEFAULT,
    .disk_space_limit_in_mib = DISK_SPACE_LIMIT_DEFAULT,  
    .buff_flush_to_db_interval = SAVE_BLOB_TO_DB_DEFAULT
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

static void p_file_info_destroy(struct File_info *p_file_info){

    collector_info("[%s]: p_file_info_destroy() cleanup", p_file_info->chart_name ? p_file_info->chart_name : "Unknown");

    freez((void *) p_file_info->chart_name);
    freez(p_file_info->filename);
    freez((void *) p_file_info->file_basename);

    if(p_file_info->circ_buff) circ_buff_destroy(p_file_info->circ_buff);
    
    if(p_file_info->parser_metrics){
        switch(p_file_info->log_type){
            case WEB_LOG: 
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

    // freez(p_file_info->parser_metrics_mut); // not yet allocated
    // freez(p_file_info->log_parser_thread); // not yet allocated
    
    freez(p_file_info);
    p_file_info = NULL;
}

/**
 * @brief Load logs management configuration.
 * @return 0 if success, -1 if disabled in global config, 
 * -2 if config file not found
 */
static int logs_manag_config_load(void){
    int rc = 0;

    if(!config_get_boolean(CONFIG_SECTION_LOGS_MANAGEMENT, "enabled", NETDATA_CONF_ENABLE_LOGS_MANAGEMENT_DEFAULT)){
        collector_info("CONFIG: Logs management disabled due to configuration option.");
        rc = -1;
    }

    g_logs_manag_config.update_every = (int)config_get_number(  CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                "update every", 
                                                                localhost->rrd_update_every);
    if(g_logs_manag_config.update_every < localhost->rrd_update_every) 
        g_logs_manag_config.update_every = localhost->rrd_update_every;
    collector_info("CONFIG: global logs management update_every: %d", g_logs_manag_config.update_every);


    g_logs_manag_config.circ_buff_spare_items = (int)config_get_number( CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                        "circular buffer spare items", 
                                                                        g_logs_manag_config.circ_buff_spare_items);
    collector_info("CONFIG: global logs management circ_buff_spare_items: %d", g_logs_manag_config.circ_buff_spare_items);


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

    char db_default_dir[FILENAME_MAX + 1];
    snprintfz(db_default_dir, FILENAME_MAX, "%s" LOGS_MANAG_DB_SUBPATH, netdata_configured_cache_dir);
    db_set_main_dir(config_get(CONFIG_SECTION_LOGS_MANAGEMENT, "db dir", db_default_dir));


    const char *const db_mode_str = config_get(CONFIG_SECTION_LOGS_MANAGEMENT, "db mode", GLOBAL_DB_MODE_DEFAULT_STR);
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
    collector_info("CONFIG: global logs management buff_flush_to_db_interval: %d", g_logs_manag_config.buff_flush_to_db_interval);


    g_logs_manag_config.disk_space_limit_in_mib = config_get_number(CONFIG_SECTION_LOGS_MANAGEMENT, 
                                                                    "disk space limit MiB", 
                                                                    g_logs_manag_config.disk_space_limit_in_mib);
    collector_info("CONFIG: global logs management disk_space_limit_in_mib: %d", g_logs_manag_config.disk_space_limit_in_mib);


    return rc;
}

/**
 * @brief Initialize logs management based on a section configuration.
 * @note On error, calls p_file_info_destroy() to clean up before returning. 
 * @param config_section Section to read configuration from.
 * @todo How to handle duplicate entries?
 */
static void logs_management_init(struct section *config_section){

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
     * Check if this management for this log source is enabled.
     * ------------------------------------------------------------------------- */
    if(appconfig_get_boolean(&log_management_config, config_section->name, "enabled", 0)){
        collector_info("[%s]: enabled = yes", p_file_info->chart_name);
    } else {
        collector_info("[%s]: enabled = no", p_file_info->chart_name);
        return p_file_info_destroy(p_file_info);
    }


    /* -------------------------------------------------------------------------
     * Check log source type.
     * TODO: There can be only one log_type = FLB_KMSG and only one FLB_SYSTEMD, 
     * catch this edge case.
     * ------------------------------------------------------------------------- */
    char *type = appconfig_get(&log_management_config, config_section->name, "log type", "flb_generic");
    if(!type || !*type) p_file_info->log_type = FLB_GENERIC; // Default
    else{
        if(!strcmp(type, "flb_generic")) p_file_info->log_type = FLB_GENERIC;
        else if (!strcmp(type, "web_log")) p_file_info->log_type = WEB_LOG;
        else if (!strcmp(type, "flb_web_log")) p_file_info->log_type = FLB_WEB_LOG;
        else if (!strcmp(type, "flb_kmsg")) p_file_info->log_type = FLB_KMSG;
        else if (!strcmp(type, "flb_systemd")) p_file_info->log_type = FLB_SYSTEMD;
        else if (!strcmp(type, "flb_docker_events")) p_file_info->log_type = FLB_DOCKER_EV;
        else if (!strcmp(type, "flb_syslog")) p_file_info->log_type = FLB_SYSLOG;
        else if (!strcmp(type, "flb_serial")) p_file_info->log_type = FLB_SERIAL;
        else p_file_info->log_type = FLB_GENERIC;
    }
    freez(type);
    collector_info("[%s]: log type = %s", p_file_info->chart_name, log_source_t_str[p_file_info->log_type]);


    /* -------------------------------------------------------------------------
     * Read log path configuration and check if it is valid.
     * ------------------------------------------------------------------------- */
    p_file_info->filename = appconfig_get(&log_management_config, config_section->name, "log path", "auto");
    m_assert(p_file_info->filename, "appconfig_get() should never return log path == NULL");
    if( (p_file_info->log_type != FLB_SYSLOG) /* FLB_SYSLOG is a special case, may or may not require path */ &&
        (!p_file_info->filename || /* Sanity check */
        !*p_file_info->filename || 
        !strcmp(p_file_info->filename, "auto") || 
        access(p_file_info->filename, R_OK))){ 

        freez(p_file_info->filename);
        p_file_info->filename = NULL;
            
        switch(p_file_info->log_type){
            case GENERIC:
            case FLB_GENERIC:
                if(!strcmp(p_file_info->chart_name, "Auth.log tail") || !strcmp(p_file_info->chart_name, "auth.log tail")){
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
                } else if(!strcmp(p_file_info->chart_name, "syslog tail")){
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
            case WEB_LOG:
            case FLB_WEB_LOG:
                if(!strcmp(p_file_info->chart_name, "Apache access.log")){
                    const char * const apache_access_path_default[] = {
                        "/var/log/apache/access.log",
                        "/var/log/apache2/access.log", /* Debian, Ubuntu */
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
                } else if(!strcmp(p_file_info->chart_name, "Nginx access.log")){
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
     * Read "update every" configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->update_every = appconfig_get_number(   &log_management_config, config_section->name, 
                                                        "update every", g_logs_manag_config.update_every);
    if(p_file_info->update_every < g_logs_manag_config.update_every) p_file_info->update_every = g_logs_manag_config.update_every;
    collector_info("[%s]: update every = %d", p_file_info->chart_name, p_file_info->update_every);


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
     * Deal with log-type-specific configuration options.
     * ------------------------------------------------------------------------- */
    p_file_info->parser_config = callocz(1, sizeof(Log_parser_config_t));
    if(p_file_info->log_type == GENERIC || p_file_info->log_type == FLB_GENERIC){
        // Do nothing
    }
    else if(p_file_info->log_type == WEB_LOG || p_file_info->log_type == FLB_WEB_LOG){
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
                                                                    "verify parsed logs", 0);
        collector_info("[%s]: verify parsed logs = %d", p_file_info->chart_name, wblp_config->verify_parsed_logs);
        
        for(int j = 0; j < wblp_config->num_fields; j++){
            if((wblp_config->fields[j] == VHOST_WITH_PORT || wblp_config->fields[j] == VHOST) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "vhosts chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_VHOST;
            }
            if((wblp_config->fields[j] == VHOST_WITH_PORT || wblp_config->fields[j] == PORT) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "ports chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_PORT;
            }
            if((wblp_config->fields[j] == REQ_CLIENT) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "IP versions chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_IP_VERSION;
            }
            if((wblp_config->fields[j] == REQ_CLIENT) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "unique client IPs - current poll chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_REQ_CLIENT_CURRENT;
            }
            if((wblp_config->fields[j] == REQ_CLIENT) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "unique client IPs - all-time chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_REQ_CLIENT_ALL_TIME;
            }
            if((wblp_config->fields[j] == REQ || wblp_config->fields[j] == REQ_METHOD) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "http request methods chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_REQ_METHODS;
            }
            if((wblp_config->fields[j] == REQ || wblp_config->fields[j] == REQ_PROTO) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "http protocol versions chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_REQ_PROTO;
            }
            if((wblp_config->fields[j] == REQ_SIZE || wblp_config->fields[j] == RESP_SIZE) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "bandwidth chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_BANDWIDTH;
            }
            if((wblp_config->fields[j] == REQ_PROC_TIME) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "timings chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_REQ_PROC_TIME;
            }
            if((wblp_config->fields[j] == RESP_CODE) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "response code families chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_RESP_CODE_FAMILY;
            }
            if((wblp_config->fields[j] == RESP_CODE) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "response codes chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_RESP_CODE;
            }
            if((wblp_config->fields[j] == RESP_CODE) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "response code types chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_RESP_CODE_TYPE;
            }
            if((wblp_config->fields[j] == SSL_PROTO) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "SSL protocols chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_SSL_PROTO;
            }
            if((wblp_config->fields[j] == SSL_CIPHER_SUITE) 
                && appconfig_get_boolean(&log_management_config, config_section->name, "SSL chipher suites chart", 0)){ 
                p_file_info->parser_config->chart_config |= CHART_SSL_CIPHER;
            }
        }
    }
    else if(p_file_info->log_type == FLB_KMSG){
        if(appconfig_get_boolean(&log_management_config, config_section->name, "severity chart", 0)) {
            p_file_info->parser_config->chart_config |= CHART_SYSLOG_SEVER;
        }
        if(appconfig_get_boolean(&log_management_config, config_section->name, "subsystem chart", 0)) {
            p_file_info->parser_config->chart_config |= CHART_KMSG_SUBSYSTEM;
        }
        if(appconfig_get_boolean(&log_management_config, config_section->name, "device chart", 0)) {
            p_file_info->parser_config->chart_config |= CHART_KMSG_DEVICE;
        }
    }
    else if(p_file_info->log_type == FLB_SYSTEMD || p_file_info->log_type == FLB_SYSLOG){
        if(p_file_info->log_type == FLB_SYSLOG){
            Syslog_parser_config_t *syslog_config = (Syslog_parser_config_t *) callocz(1, sizeof(Syslog_parser_config_t));

            /* Read syslog format */
            syslog_config->log_format = appconfig_get(&log_management_config, config_section->name, "log format", NULL);
            collector_info("[%s]: log format = %s", p_file_info->chart_name, syslog_config->log_format ? syslog_config->log_format : "NULL!");
            if(!syslog_config->log_format || !*syslog_config->log_format || !strcmp(syslog_config->log_format, "auto")){
                freez(syslog_config->log_format);
                freez(syslog_config);
                return p_file_info_destroy(p_file_info);
            }

            /* Read syslog socket mode
             * see also https://docs.fluentbit.io/manual/pipeline/inputs/syslog#configuration-parameters */
            syslog_config->mode = appconfig_get(&log_management_config, config_section->name, "mode", "unix_udp");
            collector_info("[%s]: mode = %s", p_file_info->chart_name, syslog_config->mode);

            /* Check for valid socket path if (mode == unix_udp) or 
             * (mode == unix_tcp), else read syslog network interface to bind, 
             * if (mode == udp) or (mode == tcp). */
            if(!strcmp(syslog_config->mode, "unix_udp") || !strcmp(syslog_config->mode, "unix_tcp")){
                if(!p_file_info->filename || !*p_file_info->filename || !strcmp(p_file_info->filename, "auto")){
                    freez(syslog_config->log_format);
                    freez(syslog_config->mode);
                    freez(syslog_config);
                    return p_file_info_destroy(p_file_info);
                }
                syslog_config->unix_perm = appconfig_get(&log_management_config, config_section->name, "unix_perm", "0644");
                collector_info("[%s]: unix_perm = %s", p_file_info->chart_name, syslog_config->unix_perm);
            } else if(!strcmp(syslog_config->mode, "udp") || !strcmp(syslog_config->mode, "tcp")){
                // TODO: Check if listen is in valid format
                syslog_config->listen = appconfig_get(&log_management_config, config_section->name, "listen", "0.0.0.0");
                collector_info("[%s]: listen = %s", p_file_info->chart_name, syslog_config->listen);
                syslog_config->port = appconfig_get(&log_management_config, config_section->name, "port", "5140");
                collector_info("[%s]: port = %s", p_file_info->chart_name, syslog_config->port);
            } else { 
                /* Any other modes are invalid */
                freez(syslog_config->log_format);
                freez(syslog_config);
                return p_file_info_destroy(p_file_info);
            }

            p_file_info->parser_config->gen_config = syslog_config;
        }
        if(appconfig_get_boolean(&log_management_config, config_section->name, "priority value chart", 0)) {
            p_file_info->parser_config->chart_config |= CHART_SYSLOG_PRIOR;
        }
        if(appconfig_get_boolean(&log_management_config, config_section->name, "severity chart", 0)) {
            p_file_info->parser_config->chart_config |= CHART_SYSLOG_SEVER;
        }
        if(appconfig_get_boolean(&log_management_config, config_section->name, "facility chart", 0)) {
            p_file_info->parser_config->chart_config |= CHART_SYSLOG_FACIL;
        }
    }
    else if(p_file_info->log_type == FLB_DOCKER_EV){
        if(appconfig_get_boolean(&log_management_config, config_section->name, "event type chart", 0)) {
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
        case WEB_LOG: 
        case FLB_WEB_LOG:{
            p_file_info->parser_metrics->web_log = callocz(1, sizeof(Web_log_metrics_t));
            break;
        }
        case FLB_KMSG: {
            p_file_info->parser_metrics->kernel = callocz(1, sizeof(Kernel_metrics_t));
            p_file_info->parser_metrics->kernel->subsystem = dictionary_create(DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE);
            dictionary_register_conflict_callback(p_file_info->parser_metrics->kernel->subsystem, metrics_dict_conflict_cb, NULL);
            p_file_info->parser_metrics->kernel->device = dictionary_create(DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE);
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
            collector_error("[%s]: custom %d chart = NULL", p_file_info->chart_name, cus_off);
            break;
        }

        /* Read regex config */
        char *cus_regex_k = mallocz(snprintf(NULL, 0, "custom %d regex", MAX_CUS_CHARTS_PER_SOURCE) + 1);
        sprintf(cus_regex_k, "custom %d regex", cus_off);
        char *cus_regex_v = appconfig_get(&log_management_config, config_section->name, cus_regex_k, NULL);
        debug(D_LOGS_MANAG, "cus regex:(%s:%s)", cus_regex_k, cus_regex_v ? cus_regex_v : "NULL");
        freez(cus_regex_k);
        if(unlikely(!cus_regex_v)) {
            collector_error("[%s]: custom %d regex = NULL", p_file_info->chart_name, cus_off);
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
                                                        config_section->name, cus_ignore_case_k, 1);
        debug(D_LOGS_MANAG, "cus case: (%s:%s)", cus_ignore_case_k, cus_ignore_case_v ? "yes" : "no");
        freez(cus_ignore_case_k); 

        int regex_flags = cus_ignore_case_v ? REG_EXTENDED | REG_NEWLINE | REG_ICASE : REG_EXTENDED | REG_NEWLINE;
        
        int rc;
        regex_t regex;
        if (unlikely((rc = regcomp(&regex, cus_regex_v, regex_flags)))){
            size_t regcomp_err_str_size = regerror(rc, &regex, 0, 0);
            char *regcomp_err_str = mallocz(regcomp_err_str_size);
            regerror(rc, &regex, regcomp_err_str, regcomp_err_str_size);
            collector_error("[%s]: could not compile regex for custom %d chart: %s", p_file_info->chart_name, cus_off, cus_chart_v);
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

    p_file_info->circ_buff = circ_buff_init(p_file_info->buff_flush_to_db_interval + 
                                                g_logs_manag_config.circ_buff_spare_items,
                                            circular_buffer_max_size,
                                            circular_buffer_allow_dropped_logs);


    /* -------------------------------------------------------------------------
     * Initialize input plugin.
     * ------------------------------------------------------------------------- */
    switch(p_file_info->log_type){
        int rc;
        case GENERIC:
        case WEB_LOG: {
            rc = tail_plugin_add_input(p_file_info);
            if(unlikely(rc)){
                collector_error("[%s]: tail_plugin_add_input() error: %d", p_file_info->chart_name, rc);
                return p_file_info_destroy(p_file_info);
            }
            break;
        }
        case FLB_GENERIC:
        case FLB_WEB_LOG:
        case FLB_KMSG:
        case FLB_SYSTEMD:
        case FLB_DOCKER_EV: 
        case FLB_SYSLOG: 
        case FLB_SERIAL: {
            rc = flb_add_input(p_file_info);
            if(unlikely(rc)){
                collector_error("[%s]: flb_add_input() error: %d", p_file_info->chart_name, rc);
                return p_file_info_destroy(p_file_info);
            }

            p_file_info->flb_tmp_buff_cpy_timer.data = p_file_info;
            if(unlikely(0 != uv_mutex_init(&p_file_info->flb_tmp_buff_mut))){
                fatal("uv_mutex_init(&p_file_info->flb_tmp_buff_mut) failed");
            }
            uv_timer_init(main_loop, &p_file_info->flb_tmp_buff_cpy_timer);
            uv_timer_start( &p_file_info->flb_tmp_buff_cpy_timer, 
                            (uv_timer_cb)flb_tmp_buff_cpy_timer_cb, 
                            0, p_file_info->update_every * MSEC_PER_SEC);            
            break;
        }
        default: 
            return p_file_info_destroy(p_file_info);
    }


    /* -------------------------------------------------------------------------
     * Allocate and initialise parser mutex.
     * ------------------------------------------------------------------------- */
    p_file_info->parser_metrics_mut = callocz(1, sizeof(uv_mutex_t));
    if(unlikely(uv_mutex_init(p_file_info->parser_metrics_mut)))
        fatal("Failed to initialise parser_metrics_mut for %s", p_file_info->chart_name);


    /* -------------------------------------------------------------------------
     * Initialise and create parser thread notifier condition variable and mutex.
     * ------------------------------------------------------------------------- */
    if(unlikely(uv_mutex_init(&p_file_info->notify_parser_thread_mut))) 
        fatal("Failed to initialise notify_parser_thread_mut for %s", p_file_info->chart_name);
    if(unlikely(uv_cond_init(&p_file_info->notify_parser_thread_cond))) 
        fatal("Failed to initialise notify_parser_thread_cond for %s", p_file_info->chart_name);
    p_file_info->log_parser_thread = mallocz(sizeof(uv_thread_t));
    if(unlikely(uv_thread_create(p_file_info->log_parser_thread, generic_parser, p_file_info))) 
        fatal("libuv uv_thread_create() for %s", p_file_info->chart_name);


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

    if(p_file_infos_arr){
        for(int i = 0; i < p_file_infos_arr->count; i++){
            p_file_info_destroy(p_file_infos_arr->data[i]);
        }
        freez(p_file_infos_arr);
        p_file_infos_arr = NULL;
    }

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
    UNUSED(ptr);
    netdata_thread_cleanup_push(logsmanagement_main_cleanup, ptr);

    if(logs_manag_config_load()) goto cleanup;

    main_loop = mallocz(sizeof(uv_loop_t));
    fatal_assert(uv_loop_init(main_loop) == 0);

    /* Static asserts */
    #pragma GCC diagnostic push
    #pragma GCC diagnostic ignored "-Wunused-local-typedefs" 
    /* Ensure VALIDATE_COMPRESSION is disabled in release versions. */
    COMPILE_TIME_ASSERT(LOGS_MANAG_DEBUG ? 1 : !VALIDATE_COMPRESSION);
    COMPILE_TIME_ASSERT(CIRCULAR_BUFF_DEFAULT_MAX_SIZE >= CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN); 
    COMPILE_TIME_ASSERT(CIRCULAR_BUFF_DEFAULT_MAX_SIZE <= CIRCULAR_BUFF_MAX_SIZE_RANGE_MAX);
    #pragma GCC diagnostic pop

    /* Initialize array of File_Info pointers. */
    p_file_infos_arr = callocz(1, sizeof(struct File_infos_arr));
    *p_file_infos_arr = (struct File_infos_arr){0};

    tail_plugin_init(p_file_infos_arr);

    if(flb_init()){
        collector_error("flb_init() failed - logs management will be disabled");
        goto cleanup;
    }

    /* Initialize logs management for each configuration section  */
    struct section *config_section = log_management_config.first_section;
    do {
        logs_management_init(config_section);
        config_section = config_section->next;
    } while(config_section);
    if(p_file_infos_arr->count == 0) goto cleanup; // No log sources - nothing to do

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
        flb_stop_and_cleanup();
        goto cleanup;
    }
    
#if defined(__STDC_VERSION__)
    debug(D_LOGS_MANAG, "__STDC_VERSION__: %ld", __STDC_VERSION__);
#else
    debug(D_LOGS_MANAG, "__STDC_VERSION__ undefined");
#endif // defined(__STDC_VERSION__)
    debug(D_LOGS_MANAG, "libuv version: %s", uv_version_string());
    debug(D_LOGS_MANAG, "LZ4 version: %s\n", LZ4_versionString());
    char *sqlite_version = db_get_sqlite_version();
    debug(D_LOGS_MANAG, "SQLITE version: %s\n", sqlite_version);
    freez(sqlite_version);

#if defined(LOGS_MANAGEMENT_STRESS_TEST) && LOGS_MANAGEMENT_STRESS_TEST == 1
    debug(D_LOGS_MANAG, "Running Netdata with logs_management stress test enabled!");
    static uv_thread_t run_stress_test_queries_thread_id;
    uv_thread_create(&run_stress_test_queries_thread_id, run_stress_test_queries_thread, NULL);
#endif  // LOGS_MANAGEMENT_STRESS_TEST

    p_file_infos_arr_ready = 1; // All good, inform other threads of ready state

    collector_info("logsmanagement_main() setup completed successfully");

    /* Run uvlib loop. */
    uv_run(main_loop, UV_RUN_DEFAULT);

    /* If there are valid log sources, there should always be valid handles */
    collector_error("uv_run(main_loop, ...); - no handles or requests - exiting");

cleanup:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
