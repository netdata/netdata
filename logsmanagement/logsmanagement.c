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

#include "../daemon/common.h"
#include "../libnetdata/libnetdata.h"
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

/**
 * @brief Initialise monitoring of a log source.
 * @param[in] filename The filename (if the log source is a file) of the log source.
 * @param[in] log_type The type of the log source (web log, system logs etc.)
 * @param[in] circular_buffer_initial_size Initial size of the buffer dedicated to
 * this log source.
 * @param[in] circular_buffer_max_size Maximum memory the buffer dedicated to this
 * log source can occupy.
 * @param[in] compression_accel Compression acceleration factor for the lz4 
 * compression step (see following link for more information)
 * https://github.com/lz4/lz4/blob/90d68e37093d815e7ea06b0ee3c168cccffc84b8/lib/lz4.h#L195)
 * @param[in] buff_flush_to_db_interval Interval of how often to write compressed
 * logs to disk.
 * @param[in] blob_max_size Maximum disk space that compressed logs can occupy 
 * before the old ones get overwritten. 
 * @return Pointer to initialised struct File_info p_file_info associated to
 * this log source, or NULL if any errors are encountered.
 */
static struct File_info *monitor_log_file_init(const char *filename, 
                                               const enum log_source_t log_type,
                                               const size_t circular_buffer_max_size, 
                                               const int compression_accel, 
                                               const int buff_flush_to_db_interval,
                                               const int64_t blob_max_size,
                                               const int update_every) {
    int rc = 0;

    info("Initializing log source collection for %s", filename);

    struct File_info *p_file_info = callocz(1, sizeof(struct File_info));

    p_file_info->filename = (char *) filename;            
    p_file_info->file_basename = get_basename(filename); // NOTE: get_basename uses strdupz. freez() if necessary!
    p_file_info->compression_accel = compression_accel;
    p_file_info->buff_flush_to_db_interval = buff_flush_to_db_interval;
    p_file_info->blob_max_size = blob_max_size;
    p_file_info->log_type = log_type;
    p_file_info->update_every = update_every;
    p_file_info->circ_buff = circ_buff_init( buff_flush_to_db_interval + CIRCULAR_BUFF_SPARE_ITEMS,
                                             circular_buffer_max_size);

    /* Add input */
    switch(log_type){
        case GENERIC:
        case WEB_LOG: {
            rc = tail_plugin_add_input(p_file_info);
            if(unlikely(rc)){
                error("tail_plugin_add_input() error for %s: (%d)\n", filename, rc);
                goto return_on_error;
            }
            break;
        }

        case FLB_GENERIC:
        case FLB_WEB_LOG:
        case FLB_SYSTEMD:
        case FLB_DOCKER_EV: {
            rc = flb_add_input(p_file_info);
            if(unlikely(rc)){
                error("flb_add_input() error for %s: (%d)\n", filename, rc);
                // m_assert(!rc, "flb_add_input() failed during monitor_log_file_init()");
                goto return_on_error;
            }

            p_file_info->flb_tmp_buff_cpy_timer.data = p_file_info;
            if(unlikely(0 != uv_mutex_init(&p_file_info->flb_tmp_buff_mut))) fatal("uv_mutex_init() failed");
            uv_timer_init(main_loop, &p_file_info->flb_tmp_buff_cpy_timer);
            uv_timer_start(&p_file_info->flb_tmp_buff_cpy_timer,
                        (uv_timer_cb)flb_tmp_buff_cpy_timer_cb, 0, p_file_info->update_every * MSEC_PER_SEC);            
            break;
        }

        default: 
            goto return_on_error;
    }

    /* Allocate p_file_info->parser_metrics memory */
    p_file_info->parser_metrics = callocz(1, sizeof(Log_parser_metrics_t));
    switch(log_type){
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
        }
        default:
            break;
    }

    /* Initialise and create parser thread notifier condition variable and mutex */
    rc = uv_mutex_init(&p_file_info->notify_parser_thread_mut);
    if(unlikely(rc)) fatal("Failed to initialise notify_parser_thread_mut for %s\n", p_file_info->filename);
    rc = uv_cond_init(&p_file_info->notify_parser_thread_cond);
    if(unlikely(rc)) fatal("Failed to initialise notify_parser_thread_cond for %s\n", p_file_info->filename);
    p_file_info->log_parser_thread = mallocz(sizeof(uv_thread_t));
    rc = uv_thread_create(p_file_info->log_parser_thread, generic_parser, p_file_info);
    if (unlikely(rc)) fatal("libuv error: %s \n", uv_strerror(rc));

    /* All set up successfully - add p_file_info to list of all p_file_info structs */
    p_file_infos_arr->data = reallocz(p_file_infos_arr->data, (++p_file_infos_arr->count) * (sizeof p_file_info));
    p_file_infos_arr->data[p_file_infos_arr->count - 1] = p_file_info;

    /* All successful */
    return p_file_info;

return_on_error:
    // TODO: circ_buff_destroy()
    freez((char *) p_file_info->file_basename);
    freez(p_file_info);
    return NULL;
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
 * @brief Set up configuration of log sources to monitor.
 * @todo How to handle duplicate entries?
 * @todo Free resources whenever an error happens that results in skipping to next section.
 */
static void logs_manag_config_init(){

    struct section *config_section = log_management_config.first_section;
    do{
        /* Check if config_section->name is valid and if so, use it as chart_name. */
        if(!config_section->name || config_section->name[0] == '\0'){
            error("Invalid logs management config section found:'%s'. Skipping.", config_section->name);
            goto next_section;
        } 
        debug(D_LOGS_MANAG, "Processing logs management config section: %s", config_section->name);
        
        /* Check if log parsing is enabled in configuration */
        int enabled = appconfig_get_boolean(&log_management_config, config_section->name, "enabled", 0);
        debug(D_LOGS_MANAG, "Config section: %s %s", config_section->name, enabled ? "enabled!" : "disabled. Skipping.");
        if(!enabled) goto next_section;

        /* Check log source type */
        enum log_source_t log_type;
        char *type = appconfig_get(&log_management_config, config_section->name, "log type", NULL);
        if(!type || type[0] == '\0') log_type = GENERIC;
        else{
            if(!strcmp(type, "flb_generic")) log_type = FLB_GENERIC;
            else if (!strcmp(type, "web_log")) log_type = WEB_LOG;
            else if (!strcmp(type, "flb_web_log")) log_type = FLB_WEB_LOG;
            else if (!strcmp(type, "flb_systemd")) log_type = FLB_SYSTEMD;
            else if (!strcmp(type, "flb_docker_events")) log_type = FLB_DOCKER_EV;
            else log_type = GENERIC;
        }
        debug(D_LOGS_MANAG, "Log type of %s is: %s (ENUM:%u)", config_section->name, type ? type : "generic", log_type);
        freez(type);

        /* TODO: There can be only one log_type = FLB_SYSTEMD, catch this edge case */

        /* Initialize circular buffer configuration parameters - Max size*/
        size_t circular_buffer_max_size = ((size_t)appconfig_get_number(&log_management_config, 
                                                                        config_section->name,
                                                                        "circular buffer max size", 
                                                                        CIRCULAR_BUFF_DEFAULT_MAX_SIZE)) MiB;
        if (circular_buffer_max_size == 0) {
            circular_buffer_max_size = CIRCULAR_BUFF_DEFAULT_MAX_SIZE;
            info("Circular buffer max size for %s is invalid or 0. Using default value: %zu", 
                        config_section->name, circular_buffer_max_size);

        } else if(circular_buffer_max_size > CIRCULAR_BUFF_MAX_SIZE_RANGE_MAX) {
            circular_buffer_max_size = CIRCULAR_BUFF_MAX_SIZE_RANGE_MAX;
            info("Circular buffer max size for %s out of range. Using maximum permitted value: %zu", 
                        config_section->name, circular_buffer_max_size);

        } else if(circular_buffer_max_size < CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN) {
            circular_buffer_max_size = CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN;
            info("Circular buffer max size for %s out of range. Using minimum permitted value: %zu", 
                        config_section->name, circular_buffer_max_size);
        } 

        info("Circular buffer max size for %s will be set to: %zu.", config_section->name, circular_buffer_max_size);

        /* Get compression acceleration*/
        int compression_accel = (int) appconfig_get_number( &log_management_config, 
                                                            config_section->name, 
                                                            "compression acceleration", 1);
        info("Compression acceleration for %s will be set to: %d.", config_section->name, compression_accel);

        /* Get save logs from buffers to DB interval */
        int buff_flush_to_db_interval = (int) appconfig_get_number( &log_management_config, 
                                                                    config_section->name, 
                                                                    "buffer flush to DB", SAVE_BLOB_TO_DB_DEFAULT);

        if (buff_flush_to_db_interval == 0) {
            buff_flush_to_db_interval = SAVE_BLOB_TO_DB_DEFAULT;
            info("Buffer flush to DB for %s is invalid or == 0. Using default value: %d", config_section->name, buff_flush_to_db_interval);

        } else if(buff_flush_to_db_interval > SAVE_BLOB_TO_DB_MAX) {
            buff_flush_to_db_interval = SAVE_BLOB_TO_DB_MAX;
            info("Buffer flush to DB for %s out of range. Using maximum permitted value: %d", config_section->name, buff_flush_to_db_interval);

        } else if(buff_flush_to_db_interval < SAVE_BLOB_TO_DB_MIN) {
            buff_flush_to_db_interval = SAVE_BLOB_TO_DB_MIN;
            info("Buffer flush to DB for %s out of range. Using minimum permitted value: %d", config_section->name, buff_flush_to_db_interval);
        } 
        
        info("Buffers flush to DB interval (in sec) for %s will be set to: %d.\n", config_section->name, buff_flush_to_db_interval);

        int64_t blob_max_size =  (appconfig_get_number(  &log_management_config, 
                                                        config_section->name, 
                                                        "disk space limit", 500) MiB) / BLOB_MAX_FILES;
        
        int update_every = appconfig_get_number(&log_management_config, config_section->name, 
                                                "update every", g_logs_manag_update_every);
        if(update_every < g_logs_manag_update_every) update_every = g_logs_manag_update_every;
        info("Update every for %s: %d", config_section->name, update_every);

        /* Check if log source path exists and is valid */  
        char *log_path = appconfig_get(&log_management_config, config_section->name, "log path", NULL);
        info("Log path (for %s):%s\n", config_section->name, log_path ? log_path : "NULL!");
        if(!log_path || !*log_path || !strcmp(log_path, "auto") || access(log_path, F_OK)){ 
            freez(log_path);
            switch(log_type){
                case FLB_SYSTEMD:
                    log_path = strdupz(SYSTEMD_DEFAULT_PATH);
                    break;
                case FLB_DOCKER_EV:
                    log_path = strdupz(DOCKER_EV_DEFAULT_PATH);
                    break;
                default:
                    error("%s type requires a path.", log_source_t_str[log_type]);
                    goto next_section;
            }
        } 

        /* Check if log monitoring initialisation is successful */
        // TODO: Add option to enable parser only without log storage and queries??
        struct File_info *p_file_info = monitor_log_file_init( log_path, log_type, circular_buffer_max_size, 
                                                               compression_accel, buff_flush_to_db_interval,
                                                               blob_max_size, update_every);
        if(p_file_info) info("Monitoring for %s initialized successfully.", config_section->name);
        else {
            error("Monitoring initialization for %s failed.", config_section->name);
            goto next_section; // monitor_log_file_init() was unsuccessful
        }

        /* Initialise chart name */
        p_file_info->chart_name = strdupz(config_section->name);

        /* Configure (optional) custom charts */
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

        /* Initialise parser mutex */
        p_file_info->parser_metrics_mut = mallocz(sizeof(uv_mutex_t));
        if(uv_mutex_init(p_file_info->parser_metrics_mut))
            fatal("Failed to initialise parser_metrics_mut for %s\n", p_file_info->filename);

        /* Deal with remaining log-type-specific configuration options */
        p_file_info->parser_config = callocz(1, sizeof(Log_parser_config_t));

        if(log_type == GENERIC || log_type == FLB_GENERIC){
            // Do nothing
        }
        else if(log_type == WEB_LOG || log_type == FLB_WEB_LOG){
            /* Check if a valid web log format configuration is detected */
            char *log_format = appconfig_get(&log_management_config, config_section->name, "log format", NULL);
            const char delimiter = ' '; // TODO!!: TO READ FROM CONFIG
            info("log format value: %s for section: %s\n==== \n", log_format ? log_format : "NULL!", 
                                                                  config_section->name);


            /* If "log format = auto" or no "log format" config is detected, 
             * try log format autodetection based on last log file line.
             * TODO 1: Add another case in OR where log_format is compared with a valid reg exp.
             * TODO 2: Set default log format and delimiter if not found in config? Or auto-detect? */ 
            if(!log_format || !strcmp(log_format, "auto")){ 
                info("Attempting auto-detection of log format for:%s", p_file_info->filename);

                char *line = read_last_line(p_file_info->filename, 0);
                if(!line){
                    freez(p_file_info->parser_config);
                    goto next_section; // TODO: Terminate monitor_log_file_init() if !parser_buff->line? 
                }

                p_file_info->parser_config->gen_config = auto_detect_web_log_parser_config(line, delimiter);
                freez(line);
            }
            else{
                p_file_info->parser_config->gen_config = read_web_log_parser_config(log_format, delimiter);
                freez(log_format);
                info("Read web log parser config for %s: %s\n", p_file_info->filename, 
                                                                p_file_info->parser_config ? "success!" : "failed!");
            }

            if(!p_file_info->parser_config->gen_config){
                // TODO: Terminate monitor_log_file_init() if p_file_info->parser_config is NULL? 
                freez(p_file_info->parser_config);
                goto next_section; 
            } 

            /* Check whether metrics verification during parsing is required */
            Web_log_parser_config_t *wblp_config = (Web_log_parser_config_t *) p_file_info->parser_config->gen_config;
            wblp_config->verify_parsed_logs = appconfig_get_boolean( &log_management_config, 
                                                                     config_section->name, 
                                                                     "verify parsed logs", 0);
            info( "Log parsing verification: %s (%d) for %s.", 
                  wblp_config->verify_parsed_logs ? "enabled" : "disabled", 
                  wblp_config->verify_parsed_logs, config_section->name);
            
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
        else if(log_type == FLB_SYSTEMD){
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
        else if(log_type == FLB_DOCKER_EV){
            if(appconfig_get_boolean(&log_management_config, config_section->name, "event type chart", 0)) {
                p_file_info->parser_config->chart_config |= CHART_DOCKER_EV_TYPE;
            }
        }



next_section:
        config_section = config_section->next;

    } while(config_section);
}

static void logsmanagement_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

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

    if(logs_manag_config_load()) return NULL;

    main_loop = mallocz(sizeof(uv_loop_t));
    fatal_assert(uv_loop_init(main_loop) == 0);

    /* Static asserts */
    #pragma GCC diagnostic push
    #pragma GCC diagnostic ignored "-Wunused-local-typedefs" 
    /* Ensure VALIDATE_COMPRESSION is disabled in release versions. */
    COMPILE_TIME_ASSERT(LOGS_MANAG_DEBUG ? 1 : !VALIDATE_COMPRESSION);                                         
    #pragma GCC diagnostic pop

    /* Initialise array of File_Info pointers. */
    p_file_infos_arr = mallocz(sizeof(struct File_infos_arr));
    *p_file_infos_arr = (struct File_infos_arr){0};

    tail_plugin_init(p_file_infos_arr);

    flb_init();

    logs_manag_config_init();

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

    /* Run Fluent Bit engine */
    flb_run();

    p_file_infos_arr_ready = 1;

    info("Logs management main() setup completed successfully");

    /* Run uvlib loop. */
    uv_run(main_loop, UV_RUN_DEFAULT);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
