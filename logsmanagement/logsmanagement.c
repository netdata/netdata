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
#if defined(LOGS_MANAGEMENT_STRESS_TEST) && LOGS_MANAGEMENT_STRESS_TEST == 1
#include "query_test.h"
#endif  // LOGS_MANAGEMENT_STRESS_TEST
#include "parser.h"
#include "flb_plugin.h"
#include "tail_plugin.h"

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
uv_loop_t *main_loop; 

volatile sig_atomic_t p_file_infos_arr_ready = 0;
int g_logs_manag_update_every = 1;


static void p_file_info_destroy(struct File_info *p_file_info){

    freez((void *) p_file_info->chart_name);
    freez(p_file_info->filename);
    freez((void *) p_file_info->file_basename);

    if(p_file_info->circ_buff) circ_buff_destroy(p_file_info->circ_buff);
    
    if(p_file_info->parser_metrics){
        switch(p_file_info->log_type){
            case WEB_LOG: 
            case FLB_WEB_LOG:{
                freez(p_file_info->parser_metrics->web_log);
                break;
            }
            case FLB_SYSTEMD: {
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
}

/**
 * @brief Load logs management configuration.
 * @return 0 if success, -1 if disabled in global config, 
 * -2 if config file not found
 */
static int logs_manag_config_load(void){

    if(config_get_boolean(CONFIG_SECTION_LOGS_MANAGEMENT, "enabled", 1) == 0){
        info("CONFIG: Logs management disabled globally.");
        return -1;
    }

    g_logs_manag_update_every = (int)config_get_number(CONFIG_SECTION_LOGS_MANAGEMENT, "update every", localhost->rrd_update_every);
    if(g_logs_manag_update_every < localhost->rrd_update_every) g_logs_manag_update_every = localhost->rrd_update_every;
    info("CONFIG: global logs management update every: %d", g_logs_manag_update_every);

    char db_default_dir[FILENAME_MAX + 1];
    snprintfz(db_default_dir, FILENAME_MAX, "%s" LOGS_MANAG_DB_SUBPATH, netdata_configured_cache_dir);
    db_set_main_dir(config_get(CONFIG_SECTION_LOGS_MANAGEMENT, "db dir", db_default_dir));

    char *filename = strdupz_path_subpath(netdata_configured_user_config_dir, "logsmanagement.conf");
    if(!appconfig_load(&log_management_config, filename, 0, NULL)) {
        info("CONFIG: cannot load user config '%s'. Will try stock config.", filename);
        freez(filename);

        filename = strdupz_path_subpath(netdata_configured_stock_config_dir, "logsmanagement.conf");
        if(!appconfig_load(&log_management_config, filename, 0, NULL)){
            error("CONFIG: cannot load stock config '%s'. Logs management will be disabled.", filename);
            freez(filename);
            return -2;
        }
    }
    freez(filename);
    return 0;
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
        info("[%s]: Initializing config loading", p_file_info->chart_name);
    } else {
        error("Invalid logs management config section.");
        return p_file_info_destroy(p_file_info);
    }
    

    /* -------------------------------------------------------------------------
     * Check if this management for this log source is enabled.
     * ------------------------------------------------------------------------- */
    if(appconfig_get_boolean(&log_management_config, config_section->name, "enabled", 0)){
        info("[%s]: enabled = yes", p_file_info->chart_name);
    } else {
        info("[%s]: enabled = no", p_file_info->chart_name);
        return p_file_info_destroy(p_file_info);
    }


    /* -------------------------------------------------------------------------
     * Check log source type.
     * TODO: There can be only one log_type = FLB_SYSTEMD, catch this edge case.
     * ------------------------------------------------------------------------- */
    char *type = appconfig_get(&log_management_config, config_section->name, "log type", "flb_generic");
    if(!type || !*type) p_file_info->log_type = FLB_GENERIC; // Default
    else{
        if(!strcmp(type, "flb_generic")) p_file_info->log_type = FLB_GENERIC;
        else if (!strcmp(type, "web_log")) p_file_info->log_type = WEB_LOG;
        else if (!strcmp(type, "flb_web_log")) p_file_info->log_type = FLB_WEB_LOG;
        else if (!strcmp(type, "flb_systemd")) p_file_info->log_type = FLB_SYSTEMD;
        else if (!strcmp(type, "flb_docker_events")) p_file_info->log_type = FLB_DOCKER_EV;
        else p_file_info->log_type = FLB_GENERIC;
    }
    freez(type);
    info("[%s]: log type = %s", p_file_info->chart_name, log_source_t_str[p_file_info->log_type]);


    /* -------------------------------------------------------------------------
     * Read log path configuration and check if it is valid.
     * ------------------------------------------------------------------------- */
    p_file_info->filename = appconfig_get(&log_management_config, config_section->name, "log path", "auto");
    if( !p_file_info->filename || 
        !*p_file_info->filename || 
        !strcmp(p_file_info->filename, "auto") /* Only valid for FLB_SYSTEMD or FLB_DOCKER_EV */ || 
        access(p_file_info->filename, F_OK)){ 
            
        freez(p_file_info->filename);
        switch(p_file_info->log_type){
            case FLB_SYSTEMD:
                p_file_info->filename = strdupz(SYSTEMD_DEFAULT_PATH);
                break;
            case FLB_DOCKER_EV:
                p_file_info->filename = strdupz(DOCKER_EV_DEFAULT_PATH);
                break;
            default:
                error(  "[%s]: log type = %s", p_file_info->chart_name, 
                        p_file_info->filename ? p_file_info->filename : "NULL!");
                return p_file_info_destroy(p_file_info);
        }
    }
    p_file_info->file_basename = get_basename(p_file_info->filename); 
    info("[%s]: p_file_info->filename: %s", p_file_info->chart_name, p_file_info->filename);
    info("[%s]: p_file_info->file_basename: %s", p_file_info->chart_name, p_file_info->file_basename);


    /* -------------------------------------------------------------------------
     * Read "update every" configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->update_every = appconfig_get_number(   &log_management_config, config_section->name, 
                                                        "update every", g_logs_manag_update_every);
    if(p_file_info->update_every < g_logs_manag_update_every) p_file_info->update_every = g_logs_manag_update_every;
    info("[%s]: update every = %d", p_file_info->chart_name, p_file_info->update_every);


    /* -------------------------------------------------------------------------
     * Read compression acceleration configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->compression_accel = appconfig_get_number(  &log_management_config, config_section->name, 
                                                            "compression acceleration", 1);
    info("[%s]: compression acceleration = %d", p_file_info->chart_name, p_file_info->compression_accel);
    

    /* -------------------------------------------------------------------------
     * Read BLOB max size configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->blob_max_size  = (appconfig_get_number( &log_management_config, config_section->name, 
                                                            "disk space limit", DISK_SPACE_LIMIT_DEFAULT) MiB
                                                            ) / BLOB_MAX_FILES;
    info("[%s]: BLOB max size = %lld", p_file_info->chart_name, (long long)p_file_info->blob_max_size);


    /* -------------------------------------------------------------------------
     * Read save logs from buffers to DB interval configuration.
     * ------------------------------------------------------------------------- */
    p_file_info->buff_flush_to_db_interval = appconfig_get_number(  &log_management_config, config_section->name, 
                                                                    "buffer flush to DB", SAVE_BLOB_TO_DB_DEFAULT);
    if(p_file_info->buff_flush_to_db_interval > SAVE_BLOB_TO_DB_MAX) {
        p_file_info->buff_flush_to_db_interval = SAVE_BLOB_TO_DB_MAX;
        info("[%s]: buffer flush to DB out of range. Using maximum permitted value: %d", 
                p_file_info->chart_name, p_file_info->buff_flush_to_db_interval);

    } else if(p_file_info->buff_flush_to_db_interval < SAVE_BLOB_TO_DB_MIN) {
        p_file_info->buff_flush_to_db_interval = SAVE_BLOB_TO_DB_MIN;
        info("[%s]: buffer flush to DB out of range. Using minimum permitted value: %d",
                p_file_info->chart_name, p_file_info->buff_flush_to_db_interval);
    } 
    info("[%s]: buffer flush to DB = %d", p_file_info->chart_name, p_file_info->buff_flush_to_db_interval);


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
        info("[%s]: log format = %s", p_file_info->chart_name, log_format ? log_format : "NULL!");

        /* If "log format = auto" or no "log format" config is detected, 
            * try log format autodetection based on last log file line.
            * TODO 1: Add another case in OR where log_format is compared with a valid reg exp.
            * TODO 2: Set default log format and delimiter if not found in config? Or auto-detect? */ 
        if(!log_format || !*log_format || !strcmp(log_format, "auto")){ 
            info("[%s]: Attempting auto-detection of log format", p_file_info->chart_name);
            char *line = read_last_line(p_file_info->filename, 0);
            if(!line){
                error("[%s]: read_last_line() returned NULL", p_file_info->chart_name);
                return p_file_info_destroy(p_file_info);
            }
            p_file_info->parser_config->gen_config = auto_detect_web_log_parser_config(line, delimiter);
            freez(line);
        }
        else{
            p_file_info->parser_config->gen_config = read_web_log_parser_config(log_format, delimiter);
            info( "[%s]: Read web log parser config: %s", p_file_info->chart_name, 
                    p_file_info->parser_config->gen_config ? "success!" : "failed!");
        }
        freez(log_format);

        if(!p_file_info->parser_config->gen_config){
            error("[%s]: No valid web log parser config found", p_file_info->chart_name);
            return p_file_info_destroy(p_file_info); 
        }

        /* Check whether metrics verification during parsing is required */
        Web_log_parser_config_t *wblp_config = (Web_log_parser_config_t *) p_file_info->parser_config->gen_config;
        wblp_config->verify_parsed_logs = appconfig_get_boolean( &log_management_config, config_section->name, 
                                                                    "verify parsed logs", 0);
        info("[%s]: verify parsed logs = %d", p_file_info->chart_name, wblp_config->verify_parsed_logs);
        
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
    else if(p_file_info->log_type == FLB_SYSTEMD){
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
        case FLB_SYSTEMD: {
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
        if(unlikely(!cus_chart_v)) break; // or could we continue instead of breaking ?

        /* Read regex name config - OK if NULL */
        char *cus_regex_name_k = mallocz(snprintf(NULL, 0, "custom %d regex name", MAX_CUS_CHARTS_PER_SOURCE) + 1);
        sprintf(cus_regex_name_k, "custom %d regex name", cus_off);
        char *cus_regex_name_v = appconfig_get(&log_management_config, config_section->name, cus_regex_name_k, NULL);
        debug(D_LOGS_MANAG, "cus regex name: (%s:%s)", cus_regex_name_k, cus_regex_name_v ? cus_regex_name_v : "NULL");
        freez(cus_regex_name_k);

        /* Read regex config */
        char *cus_regex_k = mallocz(snprintf(NULL, 0, "custom %d regex", MAX_CUS_CHARTS_PER_SOURCE) + 1);
        sprintf(cus_regex_k, "custom %d regex", cus_off);
        char *cus_regex_v = appconfig_get(&log_management_config, config_section->name, cus_regex_k, NULL);
        debug(D_LOGS_MANAG, "cus regex:(%s:%s)", cus_regex_k, cus_regex_v ? cus_regex_v : "NULL");
        freez(cus_regex_k);
        if(unlikely(!cus_regex_v)) {
            freez(cus_chart_v);
            freez(cus_regex_name_v); // Might be NULL but free(NULL) is OK
            break;
        }

        /* Read ignore case config */
        char *cus_ignore_case_k = mallocz(snprintf(NULL, 0, "custom %d ignore case", MAX_CUS_CHARTS_PER_SOURCE) + 1);
        sprintf(cus_ignore_case_k, "custom %d ignore case", cus_off);
        int cus_ignore_case_v = appconfig_get_boolean(  &log_management_config, 
                                                        config_section->name, cus_ignore_case_k, 1);
        debug(D_LOGS_MANAG, "cus case: (%s:%s)", cus_ignore_case_k, cus_ignore_case_v ? "yes" : "no");
        freez(cus_ignore_case_k);

        /* Allocate memory and copy config to p_file_info->parser_cus_config struct */
        p_file_info->parser_cus_config = reallocz(  p_file_info->parser_cus_config, 
                                                    (cus_off + 1) * sizeof(Log_parser_cus_config_t *));
        p_file_info->parser_cus_config[cus_off - 1] = callocz(1, sizeof(Log_parser_cus_config_t));

        p_file_info->parser_cus_config[cus_off - 1]->chart_name = cus_chart_v;
        p_file_info->parser_cus_config[cus_off - 1]->regex_name = cus_regex_name_v ? 
                                                                    cus_regex_name_v : strdupz(cus_regex_v);
        p_file_info->parser_cus_config[cus_off - 1]->regex_str = cus_regex_v;         
        
        /* Escape any backslashes in the regex name, to ensure dimension is displayed correctly in charts */
        int regex_name_bslashes = 0;
        char **p_regex_name = &p_file_info->parser_cus_config[cus_off - 1]->regex_name;
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

        debug(D_LOGS_MANAG, "cus regex_str: %s", p_file_info->parser_cus_config[cus_off - 1]->regex_str);

        int regex_flags = cus_ignore_case_v ?   REG_EXTENDED | REG_NEWLINE | REG_ICASE : 
                                                REG_EXTENDED | REG_NEWLINE;
        if (unlikely(regcomp( &p_file_info->parser_cus_config[cus_off - 1]->regex, 
                                p_file_info->parser_cus_config[cus_off - 1]->regex_str, 
                                regex_flags))){
            fatal("Could not compile regular expression:%s", p_file_info->parser_cus_config[cus_off - 1]->regex_str);
        };

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
                                                                    "circular buffer max size", 
                                                                    CIRCULAR_BUFF_DEFAULT_MAX_SIZE)) MiB;
    if(circular_buffer_max_size > CIRCULAR_BUFF_MAX_SIZE_RANGE_MAX) {
        circular_buffer_max_size = CIRCULAR_BUFF_MAX_SIZE_RANGE_MAX;
        info( "[%s]: circular buffer max size out of range. Using maximum permitted value: %zu", 
                p_file_info->chart_name, circular_buffer_max_size);
    } else if(circular_buffer_max_size < CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN) {
        circular_buffer_max_size = CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN;
        info( "[%s]: circular buffer max size out of range. Using minimum permitted value: %zu", 
                p_file_info->chart_name, circular_buffer_max_size);
    } 
    info("[%s]: circular buffer max size = %zu", p_file_info->chart_name, circular_buffer_max_size);

    int circular_buffer_allow_dropped_logs = appconfig_get_boolean( &log_management_config, 
                                                                    config_section->name,
                                                                    "circular buffer drop logs if full", 0);
    info("[%s]: circular buffer drop logs if full = %d", p_file_info->chart_name, circular_buffer_allow_dropped_logs);

    p_file_info->circ_buff = circ_buff_init(p_file_info->buff_flush_to_db_interval + CIRCULAR_BUFF_SPARE_ITEMS,
                                            circular_buffer_max_size, circular_buffer_allow_dropped_logs);


    /* -------------------------------------------------------------------------
     * Initialize input plugin.
     * ------------------------------------------------------------------------- */
    switch(p_file_info->log_type){
        int rc;
        case GENERIC:
        case WEB_LOG: {
            rc = tail_plugin_add_input(p_file_info);
            if(unlikely(rc)){
                error("[%s]: tail_plugin_add_input() error: %d", p_file_info->chart_name, rc);
                return p_file_info_destroy(p_file_info);
            }
            break;
        }
        case FLB_GENERIC:
        case FLB_WEB_LOG:
        case FLB_SYSTEMD:
        case FLB_DOCKER_EV: {
            rc = flb_add_input(p_file_info);
            if(unlikely(rc)){
                error("[%s]: flb_add_input() error: %d", p_file_info->chart_name, rc);
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

}

static void logsmanagement_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    if(p_file_infos_arr){
        for(int i = 0; i < p_file_infos_arr->count; i++){
            p_file_info_destroy(p_file_infos_arr->data[i]);
        }
        freez(p_file_infos_arr);
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

    // putenv("UV_THREADPOOL_SIZE=16");

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
    p_file_infos_arr = mallocz(sizeof(struct File_infos_arr));
    *p_file_infos_arr = (struct File_infos_arr){0};

    tail_plugin_init(p_file_infos_arr);

    if(flb_init()) goto cleanup;

    /* Initialize logs management for each configuration section  */
    struct section *config_section = log_management_config.first_section;
    do {
        logs_management_init(config_section);
        config_section = config_section->next;
    } while(config_section);

    db_init();

    debug(D_LOGS_MANAG, "File monitoring setup completed. Running db_init().");
    
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

    /* Run Fluent Bit engine 
     * TODO: If flb_run() fails, some memory will be leaked due to above 
     * *_init() functions and threads by db_init() will need to be terminated.
     * All this should be handled in logsmanagement_main_cleanup() ideally. */
    if(flb_run()) goto cleanup;

    p_file_infos_arr_ready = 1;

    info("Logs management main() setup completed successfully");

    /* Run uvlib loop. */
    uv_run(main_loop, UV_RUN_DEFAULT);

cleanup:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
